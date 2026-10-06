package controller

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
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

func setupStripeOrderQuoteFixture(t *testing.T) *gorm.DB {
	db := setupEpayQuoteFixture(t)
	require.NoError(t, model.UpdateOption("general_setting.quota_display_type", "USD"))
	publishPaymentOptionsForTest(t, map[string]string{"StripeApiSecret": "sk_test_synthetic_v1", "StripePriceId": "price_synthetic_v1", "StripeMinTopUp": "1"})
	return db
}

func requestStripeOrderFixture(ctx context.Context) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/user/stripe/pay", nil).WithContext(ctx)
	c.Set("id", 1)
	c.Set(userFundingEpochContextKey, operation_setting.GetUserFundingSetting().Epoch)
	stripeAdaptor.RequestPay(c, &StripePayRequest{Amount: 2, PaymentMethod: model.PaymentMethodStripe})
	return recorder
}

func TestStripeCheckoutSeesPersistedOrderAndFrozenCredentialUnit(t *testing.T) {
	db := setupStripeOrderQuoteFixture(t)
	previous := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = previous })
	requests := 0
	http.DefaultTransport = paymentSnapshotTransport(func(r *http.Request) (*http.Response, error) {
		requests++
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		form, err := url.ParseQuery(string(body))
		require.NoError(t, err)
		trade := form.Get("client_reference_id")
		var order model.TopUp
		require.NoError(t, db.Where("trade_no = ?", trade).First(&order).Error)
		assert.Equal(t, "100", order.QuotaPerUnitSnapshot)
		assert.Equal(t, 4.0, order.Money)
		assert.Equal(t, trade, r.Header.Get("Idempotency-Key"))
		assert.Equal(t, "price_synthetic_v1", form.Get("line_items[0][price]"))
		assert.Equal(t, "2", form.Get("line_items[0][quantity]"))
		require.NoError(t, model.UpdateOptionsBulk(map[string]string{"QuotaPerUnit": "200", "TopupGroupRatio": `{"default":1}`}))
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Request: r, Body: io.NopCloser(strings.NewReader(`{"id":"cs_synthetic","object":"checkout.session","url":"https://checkout-fixture.invalid"}`))}, nil
	})
	response := requestStripeOrderFixture(context.Background())
	assert.Equal(t, 1, requests)
	assert.Contains(t, response.Body.String(), "https://checkout-fixture.invalid")
	var order model.TopUp
	require.NoError(t, db.First(&order).Error)
	require.NoError(t, model.RechargeTrusted(order.TradeNo, "synthetic-customer", "127.0.0.1"))
	var user model.User
	require.NoError(t, db.First(&user, 1).Error)
	assert.Equal(t, 500, user.Quota)
}

func TestStripeInsertFailureNeverCallsCheckout(t *testing.T) {
	db := setupStripeOrderQuoteFixture(t)
	previous := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = previous })
	requests := 0
	http.DefaultTransport = paymentSnapshotTransport(func(*http.Request) (*http.Response, error) { requests++; return nil, errors.New("unexpected request") })
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("fixture_stripe_insert", func(tx *gorm.DB) {
		if tx.Statement.Table == "top_ups" {
			tx.AddError(errors.New("synthetic insertion failure"))
		}
	}))
	response := requestStripeOrderFixture(context.Background())
	assert.Zero(t, requests)
	assert.Contains(t, response.Body.String(), "创建订单失败")
}

func TestStripeLostCheckoutResponseKeepsPendingOrderForCallback(t *testing.T) {
	db := setupStripeOrderQuoteFixture(t)
	previous := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = previous })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	requests := 0
	http.DefaultTransport = paymentSnapshotTransport(func(*http.Request) (*http.Response, error) { requests++; cancel(); return nil, context.Canceled })
	response := requestStripeOrderFixture(ctx)
	assert.Positive(t, requests)
	assert.Contains(t, response.Body.String(), "拉起支付失败")
	var order model.TopUp
	require.NoError(t, db.First(&order).Error)
	assert.Equal(t, common.TopUpStatusPending, order.Status)
	assert.Equal(t, "100", order.QuotaPerUnitSnapshot)
	stripeWebhookFixture(t)
	assert.Equal(t, http.StatusOK, deliverStripeFixture(t, "checkout.session.completed", order.TradeNo, "complete", "whsec_fixture"))
	var user model.User
	require.NoError(t, db.First(&user, 1).Error)
	assert.Equal(t, 500, user.Quota)
}
