package controller

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupCreemQuoteFixture(t *testing.T) *gorm.DB {
	db := setupEpayQuoteFixture(t)
	publishPaymentOptionsForTest(t, map[string]string{"CreemApiKey": "synthetic-creem-key", "CreemWebhookSecret": "synthetic-webhook-key", "CreemTestMode": "true", "CreemProducts": `[{"productId":"prod_synthetic","name":"Synthetic product","price":2,"currency":"USD","quota":321}]`})
	return db
}

func requestCreemQuoteFixture(ctx context.Context) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/user/creem/pay", nil).WithContext(ctx)
	c.Set("id", 1)
	c.Set(userFundingEpochContextKey, operation_setting.GetUserFundingSetting().Epoch)
	creemAdaptor.RequestPay(c, &CreemPayRequest{ProductId: "prod_synthetic", PaymentMethod: model.PaymentMethodCreem})
	return recorder
}

func deliverCreemQuoteNotification(t *testing.T, trade, product, currency string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := common.Marshal(gin.H{"id": "evt_synthetic", "eventType": "checkout.completed", "object": gin.H{"request_id": trade,
		"product": gin.H{"id": product}, "order": gin.H{"status": "paid", "type": "onetime", "product": product, "currency": currency},
		"customer": gin.H{"email": "synthetic@example.invalid", "name": "Synthetic"}}})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/user/creem/webhook", strings.NewReader(string(body)))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set(CreemSignatureHeader, generateCreemSignature(string(body), "synthetic-webhook-key"))
	CreemWebhook(c)
	return recorder
}

func TestCreemCheckoutFreezesProductAndQuotaBeforeRemoteCall(t *testing.T) {
	db := setupCreemQuoteFixture(t)
	previous := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = previous })
	trade := ""
	http.DefaultTransport = paymentSnapshotTransport(func(r *http.Request) (*http.Response, error) {
		var request CreemCheckoutRequest
		require.NoError(t, common.DecodeJson(r.Body, &request))
		trade = request.RequestId
		var stored model.TopUp
		require.NoError(t, db.Where("trade_no = ?", trade).First(&stored).Error)
		assert.EqualValues(t, 321, stored.Amount)
		assert.Equal(t, "prod_synthetic", stored.CreemProductID)
		assert.Equal(t, "USD", stored.CreemCurrency)
		assert.Equal(t, "321", request.Metadata["quota"])
		assert.Equal(t, "synthetic-creem-key", r.Header.Get("x-api-key"))
		require.NoError(t, model.UpdateOptionsBulk(map[string]string{"CreemProducts": `[{"productId":"prod_synthetic","name":"Changed","price":4,"currency":"USD","quota":654}]`, "QuotaPerUnit": "200"}))
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Request: r, Body: io.NopCloser(strings.NewReader(`{"id":"ch_synthetic","checkout_url":"https://checkout-fixture.invalid"}`))}, nil
	})
	response := requestCreemQuoteFixture(context.Background())
	assert.Contains(t, response.Body.String(), "https://checkout-fixture.invalid")
	for i := 0; i < 2; i++ {
		assert.Equal(t, http.StatusOK, deliverCreemQuoteNotification(t, trade, "prod_synthetic", "USD").Code)
	}
	var user model.User
	require.NoError(t, db.First(&user, 1).Error)
	assert.Equal(t, 421, user.Quota, "Creem grants fixed quota, never multiplied by live unit")
}

func TestCreemCanceledCheckoutRemainsPendingAndRejectsForeignProduct(t *testing.T) {
	db := setupCreemQuoteFixture(t)
	previous := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = previous })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	http.DefaultTransport = paymentSnapshotTransport(func(*http.Request) (*http.Response, error) { cancel(); return nil, context.Canceled })
	response := requestCreemQuoteFixture(ctx)
	assert.Contains(t, response.Body.String(), "拉起支付失败")
	var order model.TopUp
	require.NoError(t, db.First(&order).Error)
	assert.Equal(t, common.TopUpStatusPending, order.Status)
	assert.Equal(t, http.StatusBadRequest, deliverCreemQuoteNotification(t, order.TradeNo, "prod_foreign", "USD").Code)
	assert.Equal(t, http.StatusBadRequest, deliverCreemQuoteNotification(t, order.TradeNo, "prod_synthetic", "EUR").Code)
	var user model.User
	require.NoError(t, db.First(&user, 1).Error)
	assert.Equal(t, 100, user.Quota)
	assert.Equal(t, http.StatusOK, deliverCreemQuoteNotification(t, order.TradeNo, "prod_synthetic", "USD").Code)
	require.NoError(t, db.First(&user, 1).Error)
	assert.Equal(t, 421, user.Quota)
}

func TestCreemCheckoutDoesNotForwardCredentialOnRedirect(t *testing.T) {
	_ = setupCreemQuoteFixture(t)
	previous := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = previous })
	requests := 0
	http.DefaultTransport = paymentSnapshotTransport(func(r *http.Request) (*http.Response, error) {
		requests++
		return &http.Response{StatusCode: http.StatusTemporaryRedirect, Header: http.Header{"Location": []string{"https://attacker.invalid/collect"}}, Request: r, Body: io.NopCloser(strings.NewReader("redirect"))}, nil
	})
	_, err := genCreemLinkFromValues(context.Background(), "synthetic-key", true, "synthetic-order", &CreemProduct{ProductId: "prod_synthetic", Quota: 321}, "synthetic@example.invalid", "Synthetic")
	require.Error(t, err)
	assert.Equal(t, 1, requests)
}

func TestCreemInsertFailureDoesNotCallRemoteCheckout(t *testing.T) {
	db := setupCreemQuoteFixture(t)
	previous := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = previous })
	requests := 0
	http.DefaultTransport = paymentSnapshotTransport(func(*http.Request) (*http.Response, error) {
		requests++
		return nil, errors.New("unexpected network request")
	})
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("fixture_creem_insert", func(tx *gorm.DB) {
		if tx.Statement.Table == "top_ups" {
			tx.AddError(errors.New("synthetic insertion failure"))
		}
	}))
	response := requestCreemQuoteFixture(context.Background())
	assert.Zero(t, requests)
	assert.Contains(t, response.Body.String(), "创建订单失败")
}

func TestCreemOversizedCheckoutResponseDoesNotReturnLink(t *testing.T) {
	_ = setupCreemQuoteFixture(t)
	previous := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = previous })
	http.DefaultTransport = paymentSnapshotTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Request: r, Body: io.NopCloser(strings.NewReader(strings.Repeat("x", int(maxPaymentCheckoutResponseBytes)+1)))}, nil
	})
	link, err := genCreemLinkFromValues(context.Background(), "synthetic", true, "synthetic-order", &CreemProduct{ProductId: "prod_synthetic", Quota: 321}, "synthetic@example.invalid", "Synthetic")
	require.Error(t, err)
	assert.Empty(t, link)
}
