package controller

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/service"
	"github.com/ForceMind/MyAPI/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupPancakeQuoteFixture(t *testing.T) *gorm.DB {
	db := setupEpayQuoteFixture(t)
	confirmPaymentComplianceForTest(t)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	der, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	publishPaymentOptionsForTest(t, map[string]string{"WaffoPancakeMerchantID": "MER_AbCdEfGhIjKlMnOpQrStUv", "WaffoPancakePrivateKey": string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})),
		"WaffoPancakeStoreID":   "STO_synthetic",
		"WaffoPancakeProductID": "PROD_AbCdEfGhIjKlMnOpQrStUv", "WaffoPancakeUnitPrice": "2", "WaffoPancakeMinTopUp": "1"})
	return db
}

func requestPancakeQuoteFixture(ctx context.Context) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/user/waffo/pancake/pay", strings.NewReader(`{"amount":250}`)).WithContext(ctx)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("id", 1)
	c.Set(userFundingEpochContextKey, operation_setting.GetUserFundingSetting().Epoch)
	RequestWaffoPancakePay(c)
	return recorder
}

func TestPancakeSDKUsesPersistedFrozenOrderAfterConfigurationChanges(t *testing.T) {
	db := setupPancakeQuoteFixture(t)
	previous := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = previous })
	http.DefaultTransport = paymentSnapshotTransport(func(r *http.Request) (*http.Response, error) {
		body := `{"data":{"token":"synthetic-token","expiresAt":"2026-10-01T00:00:00Z"}}`
		if strings.HasSuffix(r.URL.Path, "/create-session") {
			var request struct {
				Trade string `json:"orderMerchantExternalId"`
				Price struct {
					Amount string `json:"amount"`
				} `json:"priceSnapshot"`
				Product string `json:"productId"`
			}
			if err := common.DecodeJson(r.Body, &request); err != nil {
				t.Error(err)
				return nil, err
			}
			var stored model.TopUp
			if err := db.Where("trade_no = ?", request.Trade).First(&stored).Error; err != nil {
				t.Error(err)
				return nil, err
			}
			assert.Equal(t, "100", stored.QuotaPerUnitSnapshot)
			assert.EqualValues(t, 2, stored.Amount)
			assert.Equal(t, 5.0, stored.Money)
			assert.Equal(t, "5.00", request.Price.Amount)
			if err := model.UpdateOptionsBulk(map[string]string{"QuotaPerUnit": "200", "TopupGroupRatio": `{"default":1}`, "general_setting.quota_display_type": "USD"}); err != nil {
				t.Error(err)
				return nil, err
			}
			body = `{"data":{"sessionId":"ses_synthetic","checkoutUrl":"https://pancake-fixture.invalid","expiresAt":"2026-10-01T00:00:00Z"}}`
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Request: r, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	response := requestPancakeQuoteFixture(context.Background())
	assert.Contains(t, response.Body.String(), "pancake-fixture.invalid")
	var order model.TopUp
	require.NoError(t, db.First(&order).Error)
	event := &service.WaffoPancakeWebhookEvent{StoreID: "STO_synthetic", EventType: "order.completed", Mode: "test", Data: service.WaffoPancakeWebhookData{Currency: "USD", OrderMerchantExternalID: order.TradeNo, MerchantProvidedBuyerIdentity: "my-api-user-1"}}
	for i := 0; i < 2; i++ {
		assert.Equal(t, http.StatusOK, deliverVerifiedPancakeFixture(t, event))
	}
	var user model.User
	require.NoError(t, db.First(&user, 1).Error)
	assert.Equal(t, 300, user.Quota)
}

func TestPancakeCanceledCheckoutKeepsOrderAvailableForLaterCallback(t *testing.T) {
	db := setupPancakeQuoteFixture(t)
	previous := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = previous })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	http.DefaultTransport = paymentSnapshotTransport(func(*http.Request) (*http.Response, error) { cancel(); return nil, context.Canceled })
	response := requestPancakeQuoteFixture(ctx)
	assert.Contains(t, response.Body.String(), "拉起支付失败")
	var order model.TopUp
	require.NoError(t, db.First(&order).Error)
	assert.Equal(t, common.TopUpStatusPending, order.Status)
	assert.Equal(t, "100", order.QuotaPerUnitSnapshot)
	event := &service.WaffoPancakeWebhookEvent{StoreID: "STO_synthetic", EventType: "order.completed", Mode: "test", Data: service.WaffoPancakeWebhookData{Currency: "USD", OrderMerchantExternalID: order.TradeNo, MerchantProvidedBuyerIdentity: "my-api-user-1"}}
	assert.Equal(t, http.StatusOK, deliverVerifiedPancakeFixture(t, event))
	var user model.User
	require.NoError(t, db.First(&user, 1).Error)
	assert.Equal(t, 300, user.Quota)
}

func TestPancakeSourceMismatchAndLegacyMissingSourceNeverCredit(t *testing.T) {
	for _, scenario := range []string{"foreign store", "foreign currency", "legacy missing source"} {
		t.Run(scenario, func(t *testing.T) {
			db := paymentWebhookTestDB(t)
			order := model.TopUp{UserId: 1, TradeNo: "source-" + scenario, Amount: 2, Money: 5, PaymentProvider: model.PaymentProviderWaffoPancake, QuotaPerUnitSnapshot: "100", WaffoPancakeStoreID: "STO_saved", WaffoPancakeProductID: "PROD_saved", WaffoPancakeCurrency: "USD", Status: common.TopUpStatusPending}
			if scenario == "legacy missing source" {
				order.WaffoPancakeStoreID = ""
				order.WaffoPancakeProductID = ""
				order.WaffoPancakeCurrency = ""
			}
			require.NoError(t, db.Create(&order).Error)
			event := &service.WaffoPancakeWebhookEvent{StoreID: "STO_saved", EventType: "order.completed", Mode: "test", Data: service.WaffoPancakeWebhookData{Currency: "USD", OrderMerchantExternalID: order.TradeNo, MerchantProvidedBuyerIdentity: "my-api-user-1"}}
			if scenario == "foreign store" {
				event.StoreID = "STO_foreign"
				publishPaymentOptionsForTest(t, map[string]string{"WaffoPancakeStoreID": "STO_foreign"})
			}
			if scenario == "foreign currency" {
				event.Data.Currency = "EUR"
			}
			assert.Equal(t, http.StatusOK, deliverVerifiedPancakeFixture(t, event))
			var user model.User
			require.NoError(t, db.First(&user, 1).Error)
			assert.Equal(t, 100, user.Quota)
			require.NoError(t, db.First(&order, order.Id).Error)
			assert.Equal(t, common.TopUpStatusPending, order.Status)
			if scenario == "legacy missing source" {
				assert.Empty(t, order.WaffoPancakeStoreID)
				return
			}
			event.StoreID, event.Data.Currency = "STO_saved", "USD"
			assert.Equal(t, http.StatusOK, deliverVerifiedPancakeFixture(t, event), "stored source stays authoritative after live configuration changes")
			require.NoError(t, db.First(&user, 1).Error)
			assert.Equal(t, 300, user.Quota)
		})
	}
}
