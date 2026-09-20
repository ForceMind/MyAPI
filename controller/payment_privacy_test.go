package controller

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/middleware"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/service"
	"github.com/ForceMind/MyAPI/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v81/webhook"
	waffoutils "github.com/waffo-com/waffo-go/utils"
)

type paymentPrivacyLogs struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *paymentPrivacyLogs) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *paymentPrivacyLogs) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

func capturePaymentPrivacyLogs(t *testing.T) *paymentPrivacyLogs {
	t.Helper()
	logs := &paymentPrivacyLogs{}
	common.LogWriterMu.Lock()
	writer, errorWriter := gin.DefaultWriter, gin.DefaultErrorWriter
	gin.DefaultWriter, gin.DefaultErrorWriter = logs, logs
	common.LogWriterMu.Unlock()
	t.Cleanup(func() {
		common.LogWriterMu.Lock()
		gin.DefaultWriter, gin.DefaultErrorWriter = writer, errorWriter
		common.LogWriterMu.Unlock()
	})
	return logs
}

func TestPaymentWebhookLogPrivacy(t *testing.T) {
	subscriptionWebhookSettings(t)
	keys, err := waffoutils.GenerateKeyPair()
	require.NoError(t, err)
	enabled, sandbox := setting.WaffoEnabled, setting.WaffoSandbox
	api, private, public := setting.WaffoApiKey, setting.WaffoPrivateKey, setting.WaffoPublicCert
	setting.WaffoEnabled, setting.WaffoSandbox = true, false
	setting.WaffoApiKey, setting.WaffoPrivateKey, setting.WaffoPublicCert = "waffo-fixture", keys.PrivateKey, keys.PublicKey
	t.Cleanup(func() {
		setting.WaffoEnabled, setting.WaffoSandbox = enabled, sandbox
		setting.WaffoApiKey, setting.WaffoPrivateKey, setting.WaffoPublicCert = api, private, public
	})
	for _, provider := range []struct {
		name, header string
		handler      gin.HandlerFunc
		invalidCode  int
		sign         func([]byte) string
	}{
		{"stripe", "Stripe-Signature", StripeWebhook, http.StatusBadRequest, func(body []byte) string {
			return webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{Payload: body, Secret: setting.StripeWebhookSecret}).Header
		}},
		{"creem", CreemSignatureHeader, CreemWebhook, http.StatusUnauthorized, func(body []byte) string {
			return generateCreemSignature(string(body), setting.CreemWebhookSecret)
		}},
		{"waffo", "X-SIGNATURE", WaffoWebhook, http.StatusBadRequest, func(body []byte) string {
			sig, err := waffoutils.Sign(string(body), keys.PrivateKey)
			require.NoError(t, err)
			return sig
		}},
	} {
		for _, mode := range []string{"valid", "missing", "invalid", "signed malformed"} {
			t.Run(provider.name+"/"+mode, func(t *testing.T) {
				logs := capturePaymentPrivacyLogs(t)
				body := []byte(`{"id":"evt_fixture","object":"event","type":"fixture.ignored","eventType":"fixture.ignored","data":{"object":{"customer":{"email":"private-email@example.test","name":"private-customer-name"},"metadata":{"token":"private-body-token","buyer_identity":"private-buyer-identity"}}},"customer":{"email":"private-email@example.test","name":"private-customer-name"},"metadata":{"token":"private-body-token","buyer_identity":"private-buyer-identity"}}`)
				if provider.name == "creem" {
					body = []byte(`{"id":"evt_fixture","eventType":"fixture.ignored","object":{"customer":{"email":"private-email@example.test","name":"private-customer-name"},"metadata":{"token":"private-body-token","buyer_identity":"private-buyer-identity"}}}`)
				}
				want := http.StatusOK
				if mode == "signed malformed" {
					body = []byte(`{"id":"private-body-token","eventType": "private-email@example.test", INVALID}`)
					want = http.StatusBadRequest
					if provider.name == "waffo" {
						want = http.StatusOK // Waffo returns its signed failed ACK.
					}
				}
				signature := provider.sign(body)
				if mode == "missing" {
					signature, want = "", provider.invalidCode
				} else if mode == "invalid" {
					signature, want = "private-invalid-signature", provider.invalidCode
				}
				route := "/api/" + provider.name + "/webhook"
				request := httptest.NewRequest(http.MethodPost, route+"?secret=private-query-token", bytes.NewReader(body))
				request.Header.Set(provider.header, signature)
				response := httptest.NewRecorder()
				router := gin.New()
				middleware.SetUpLogger(router)
				router.POST(route, provider.handler)
				router.ServeHTTP(response, request)
				require.Equal(t, want, response.Code, response.Body.String())
				output := logs.String()
				assert.Contains(t, output, "[GIN]")
				assert.Contains(t, output, "webhook")
				if mode != "valid" {
					assert.True(t, strings.Contains(output, "[WARN]") || strings.Contains(output, "[ERR]"), "failures must retain a safe warning: %s", output)
				}
				for _, secret := range []string{"private-body-token", "private-query-token", "private-email@example.test", "private-customer-name", "private-buyer-identity", signature} {
					if secret != "" {
						assert.NotContains(t, output, secret)
					}
				}
			})
		}
	}
}

func TestCreemCustomerAndCheckoutLogPrivacy(t *testing.T) {
	db := paymentWebhookTestDB(t)
	subscriptionWebhookSettings(t)
	logs := capturePaymentPrivacyLogs(t)
	order := model.TopUp{UserId: 1, TradeNo: "creem-private-fixture", Amount: 25, Money: 1, PaymentProvider: model.PaymentProviderCreem, Status: common.TopUpStatusPending}
	require.NoError(t, db.Create(&order).Error)
	body := `{"id":"evt_private_fixture","eventType":"checkout.completed","object":{"request_id":"creem-private-fixture","order":{"id":"order_fixture","status":"paid","type":"onetime","amount_paid":100,"currency":"USD"},"product":{"id":"product_fixture","name":"private-product-name"},"customer":{"email":"private-email@example.test","name":"private-customer-name"}}}`
	router := gin.New()
	middleware.SetUpLogger(router)
	router.POST("/api/creem/webhook", CreemWebhook)
	for range 2 {
		request := httptest.NewRequest(http.MethodPost, "/api/creem/webhook?token=private-query-token", strings.NewReader(body))
		request.Header.Set(CreemSignatureHeader, generateCreemSignature(body, setting.CreemWebhookSecret))
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		require.Equal(t, http.StatusOK, response.Code)
	}
	var user model.User
	require.NoError(t, db.First(&user, 1).Error)
	assert.Equal(t, 125, user.Quota)
	for _, secret := range []string{"private-email@example.test", "private-customer-name", "private-product-name", "private-query-token", generateCreemSignature(body, setting.CreemWebhookSecret)} {
		assert.NotContains(t, logs.String(), secret)
	}
}

type paymentPrivacyTransport func(*http.Request) (*http.Response, error)

func (f paymentPrivacyTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCreemRequestAndResponseLogPrivacy(t *testing.T) {
	subscriptionWebhookSettings(t)
	logs := capturePaymentPrivacyLogs(t)
	request := httptest.NewRequest(http.MethodPost, "/api/user/creem/pay", strings.NewReader(`{"product_id":"private-body-token", INVALID}`))
	response := httptest.NewRecorder()
	router := gin.New()
	middleware.SetUpLogger(router)
	router.POST("/api/user/creem/pay", RequestCreemPay)
	router.ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), `"message":"error"`)

	const checkoutURL = "https://checkout.example.test/session?token=private-checkout-token"
	transport := http.DefaultTransport
	http.DefaultTransport = paymentPrivacyTransport(func(r *http.Request) (*http.Response, error) {
		assert.Equal(t, "https://api.creem.io/v1/checkouts", r.URL.String())
		requestBody, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		assert.Contains(t, string(requestBody), "private-email@example.test", "private data must still reach the payment provider")
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"id":"checkout_fixture","checkout_url":"` + checkoutURL + `","customer_email":"private-email@example.test"}`))}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = transport })
	checkout, err := genCreemLink(context.Background(), setting.CapturePaymentConfig(), "trade_fixture", &CreemProduct{ProductId: "product_fixture", Name: "fixture", Quota: 25}, "private-email@example.test", "private-customer-name")
	require.NoError(t, err)
	assert.Equal(t, checkoutURL, checkout)
	for _, secret := range []string{"private-body-token", "private-email@example.test", "private-customer-name", "private-checkout-token", checkoutURL} {
		assert.NotContains(t, logs.String(), secret)
	}
}

func TestCreemMissingSecretVerifierLogPrivacy(t *testing.T) {
	old := setting.CreemTestMode
	t.Cleanup(func() { setting.CreemTestMode = old })
	for _, testMode := range []bool{false, true} {
		logs := capturePaymentPrivacyLogs(t)
		setting.CreemTestMode = testMode
		assert.Equal(t, testMode, verifyCreemSignature("private-body-token", "private-signature-token", setting.CapturePaymentConfig()))
		assert.NotContains(t, logs.String(), "private-body-token")
		assert.NotContains(t, logs.String(), "private-signature-token")
		assert.Contains(t, logs.String(), "secret")
	}
}

type paymentPrivateReadFailure struct{}

func (paymentPrivateReadFailure) Read([]byte) (int, error) {
	return 0, errors.New("private-read-error-token")
}
func (paymentPrivateReadFailure) Close() error { return nil }

func TestPaymentWebhookReadErrorLogPrivacy(t *testing.T) {
	subscriptionWebhookSettings(t)
	for _, tc := range []struct {
		name    string
		handler gin.HandlerFunc
		status  int
	}{{"stripe", StripeWebhook, http.StatusServiceUnavailable}, {"creem", CreemWebhook, http.StatusBadRequest}} {
		t.Run(tc.name, func(t *testing.T) {
			logs := capturePaymentPrivacyLogs(t)
			request := httptest.NewRequest(http.MethodPost, "/api/"+tc.name+"/webhook?token=private-query-token", nil)
			request.Body = paymentPrivateReadFailure{}
			response := httptest.NewRecorder()
			router := gin.New()
			middleware.SetUpLogger(router)
			router.POST("/api/"+tc.name+"/webhook", tc.handler)
			router.ServeHTTP(response, request)
			assert.Equal(t, tc.status, response.Code)
			assert.NotContains(t, logs.String(), "private-read-error-token")
			assert.NotContains(t, logs.String(), "private-query-token")
		})
	}
}

func TestWaffoPancakeWebhookLogPrivacy(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	merchant, key, product := setting.WaffoPancakeMerchantID, setting.WaffoPancakePrivateKey, setting.WaffoPancakeProductID
	setting.WaffoPancakeMerchantID, setting.WaffoPancakePrivateKey, setting.WaffoPancakeProductID = "fixture", "fixture", "fixture"
	t.Cleanup(func() {
		setting.WaffoPancakeMerchantID, setting.WaffoPancakePrivateKey, setting.WaffoPancakeProductID = merchant, key, product
	})
	for _, signature := range []string{"", "private-invalid-signature"} {
		logs := capturePaymentPrivacyLogs(t)
		request := httptest.NewRequest(http.MethodPost, "/api/waffo-pancake/webhook/test?secret=private-query-token", strings.NewReader(`{"mode":"test","data":{"buyerEmail":"private-email@example.test","merchantProvidedBuyerIdentity":"private-buyer-identity"},"metadata":"private-body-token"}`))
		request.Header.Set("X-Waffo-Signature", signature)
		response := httptest.NewRecorder()
		router := gin.New()
		middleware.SetUpLogger(router)
		router.POST("/api/waffo-pancake/webhook/:env", WaffoPancakeWebhook)
		router.ServeHTTP(response, request)
		assert.Equal(t, http.StatusUnauthorized, response.Code)
		assert.Contains(t, logs.String(), "[GIN]")
		for _, secret := range []string{"private-query-token", "private-email@example.test", "private-buyer-identity", "private-body-token", "private-invalid-signature"} {
			assert.NotContains(t, logs.String(), secret)
		}
	}
	t.Run("unknown environment", func(t *testing.T) {
		logs := capturePaymentPrivacyLogs(t)
		request := httptest.NewRequest(http.MethodPost, "/api/waffo-pancake/webhook/private-env-token?secret=private-query-token", nil)
		response := httptest.NewRecorder()
		router := gin.New()
		middleware.SetUpLogger(router)
		router.POST("/api/waffo-pancake/webhook/:env", WaffoPancakeWebhook)
		router.ServeHTTP(response, request)
		assert.Equal(t, http.StatusNotFound, response.Code)
		assert.Contains(t, logs.String(), "[GIN]")
		assert.NotContains(t, logs.String(), "private-env-token")
		assert.NotContains(t, logs.String(), "private-query-token")
	})
}

func TestWaffoPancakeVerifiedIdentityRejectionLogPrivacy(t *testing.T) {
	for _, subscription := range []bool{false, true} {
		db := paymentWebhookTestDB(t)
		logs := capturePaymentPrivacyLogs(t)
		tradeNo := "WAFFO_PANCAKE-private-fixture"
		if subscription {
			tradeNo = "WAFFO_PANCAKE_SUB-private-fixture"
			require.NoError(t, db.Create(&model.SubscriptionOrder{UserId: 1, TradeNo: tradeNo, PaymentProvider: model.PaymentProviderWaffoPancake, Status: common.TopUpStatusPending}).Error)
		} else {
			require.NoError(t, db.Create(&model.TopUp{UserId: 1, TradeNo: tradeNo, Amount: 1, PaymentProvider: model.PaymentProviderWaffoPancake, Status: common.TopUpStatusPending}).Error)
		}
		event := &service.WaffoPancakeWebhookEvent{ID: "event_fixture", EventType: "order.completed", Mode: "test", Data: service.WaffoPancakeWebhookData{
			OrderID: "order_fixture", OrderMerchantExternalID: tradeNo, MerchantProvidedBuyerIdentity: "private-buyer-identity", BuyerEmail: "private-email@example.test",
		}}
		// This is a synthetic already-verified event, not a gateway signature
		// test. The public handler and its bundled provider keys stay untouched.
		assert.Equal(t, http.StatusOK, deliverVerifiedPancakeFixture(t, event))
		assert.Contains(t, logs.String(), "[ERR]")
		assert.NotContains(t, logs.String(), "private-buyer-identity")
		assert.NotContains(t, logs.String(), "private-email@example.test")
		var user model.User
		require.NoError(t, db.First(&user, 1).Error)
		assert.Equal(t, 100, user.Quota)
	}
}
