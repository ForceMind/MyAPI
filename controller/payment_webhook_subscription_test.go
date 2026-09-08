package controller

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/setting"
	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/ForceMind/MyAPI/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func subscriptionWebhookSettings(t *testing.T) {
	t.Helper()
	paymentSetting := config.GlobalConfig.Get("payment_setting")
	require.NotNil(t, paymentSetting)
	baseline, err := config.ConfigToMap(paymentSetting)
	require.NoError(t, err)
	stripeAPI, stripeSecret, stripePrice := setting.StripeApiSecret, setting.StripeWebhookSecret, setting.StripePriceId
	creemAPI, creemSecret, creemProducts, creemTestMode := setting.CreemApiKey, setting.CreemWebhookSecret, setting.CreemProducts, setting.CreemTestMode
	t.Cleanup(func() {
		require.NoError(t, config.UpdateConfigFromMap(paymentSetting, baseline))
		setting.StripeApiSecret, setting.StripeWebhookSecret, setting.StripePriceId = stripeAPI, stripeSecret, stripePrice
		setting.CreemApiKey, setting.CreemWebhookSecret, setting.CreemProducts, setting.CreemTestMode = creemAPI, creemSecret, creemProducts, creemTestMode
	})
	require.NoError(t, config.UpdateConfigFromMap(paymentSetting, map[string]string{
		"compliance_confirmed":     "true",
		"compliance_terms_version": operation_setting.CurrentComplianceTermsVersion,
	}))
	setting.StripeApiSecret = "sk_test_subscription_fixture"
	setting.StripeWebhookSecret = "whsec_subscription_fixture"
	setting.StripePriceId = ""
	setting.CreemApiKey = "creem_subscription_fixture"
	setting.CreemWebhookSecret = "creem_subscription_secret"
	setting.CreemProducts = ""
	setting.CreemTestMode = false
}

func createSubscriptionWebhookOrder(t *testing.T, db *gorm.DB, provider, method, tradeNo string) model.SubscriptionOrder {
	t.Helper()
	plan := model.SubscriptionPlan{Title: "Webhook subscription", PriceAmount: 10, Currency: "USD", DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1, Enabled: true, TotalAmount: 1000}
	if provider == model.PaymentProviderStripe {
		plan.StripePriceId = "price_subscription_fixture"
	} else {
		plan.CreemProductId = "creem_subscription_fixture"
	}
	require.NoError(t, db.Create(&plan).Error)
	order := model.SubscriptionOrder{UserId: 1, PlanId: plan.Id, Money: 10, TradeNo: tradeNo, PaymentMethod: method, PaymentProvider: provider, Status: common.TopUpStatusPending, CreateTime: common.GetTimestamp()}
	require.NoError(t, db.Create(&order).Error)
	return order
}

func deliverCreemSubscriptionFixture(t *testing.T, tradeNo, secret string) int {
	t.Helper()
	payload, err := common.Marshal(map[string]any{
		"id": "evt_creem_subscription", "eventType": "checkout.completed",
		"object": map[string]any{
			"id": "checkout_creem_subscription", "request_id": tradeNo,
			"order":    map[string]any{"id": "order_creem_subscription", "status": "paid", "type": "subscription", "amount_paid": 1000, "currency": "USD"},
			"product":  map[string]any{"id": "creem_subscription_fixture", "name": "Subscription"},
			"customer": map[string]any{"id": "customer_creem_subscription", "email": "subscription@example.test", "name": "Subscription User"},
		},
	})
	require.NoError(t, err)
	mac := hmac.New(sha256.New, []byte(secret))
	_, err = mac.Write(payload)
	require.NoError(t, err)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/creem/webhook", strings.NewReader(string(payload)))
	request.Header.Set(CreemSignatureHeader, hex.EncodeToString(mac.Sum(nil)))
	router := gin.New()
	router.POST("/api/creem/webhook", CreemWebhook)
	router.ServeHTTP(response, request)
	return response.Code
}

func subscriptionEntitlementCount(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var count int64
	require.NoError(t, db.Model(&model.UserSubscription{}).Where("user_id = ?", 1).Count(&count).Error)
	return count
}

func TestSubscriptionOnlyPaymentWebhooksFulfillExactlyOnce(t *testing.T) {
	for _, tc := range []struct {
		name     string
		provider string
		method   string
		deliver  func(*testing.T, string, string) int
		secret   func() string
		enabled  func() bool
		topUp    func() bool
		badCode  int
	}{
		{
			name: "stripe", provider: model.PaymentProviderStripe, method: model.PaymentMethodStripe,
			deliver: func(t *testing.T, tradeNo, secret string) int {
				return deliverStripeFixture(t, "checkout.session.completed", tradeNo, "complete", secret)
			},
			secret: func() string { return setting.StripeWebhookSecret }, enabled: isStripeWebhookEnabled, topUp: isStripeTopUpEnabled, badCode: http.StatusBadRequest,
		},
		{
			name: "creem", provider: model.PaymentProviderCreem, method: model.PaymentMethodCreem,
			deliver: deliverCreemSubscriptionFixture,
			secret:  func() string { return setting.CreemWebhookSecret }, enabled: isCreemWebhookEnabled, topUp: isCreemTopUpEnabled, badCode: http.StatusUnauthorized,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := paymentWebhookTestDB(t)
			subscriptionWebhookSettings(t)
			order := createSubscriptionWebhookOrder(t, db, tc.provider, tc.method, "subscription-only-"+tc.name)
			require.True(t, tc.enabled())
			require.False(t, tc.topUp())

			assert.Equal(t, tc.badCode, tc.deliver(t, order.TradeNo, "wrong-fixture-secret"))
			assert.Zero(t, subscriptionEntitlementCount(t, db))
			assert.Equal(t, common.TopUpStatusPending, model.GetSubscriptionOrderByTradeNo(order.TradeNo).Status)

			assert.Equal(t, http.StatusOK, tc.deliver(t, order.TradeNo, tc.secret()))
			assert.Equal(t, http.StatusOK, tc.deliver(t, order.TradeNo, tc.secret()))
			assert.Equal(t, int64(1), subscriptionEntitlementCount(t, db))
			assert.Equal(t, common.TopUpStatusSuccess, model.GetSubscriptionOrderByTradeNo(order.TradeNo).Status)
		})
	}
}

func TestSubscriptionOnlyPaymentWebhooksRejectIncompleteConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name      string
		configure func()
		deliver   func(*testing.T) int
	}{
		{
			name:      "stripe api missing",
			configure: func() { setting.StripeApiSecret = "" },
			deliver: func(t *testing.T) int {
				return deliverStripeFixture(t, "checkout.session.completed", "subscription-disabled", "complete", setting.StripeWebhookSecret)
			},
		},
		{
			name:      "stripe signature secret missing",
			configure: func() { setting.StripeWebhookSecret = "" },
			deliver: func(t *testing.T) int {
				return deliverStripeFixture(t, "checkout.session.completed", "subscription-disabled", "complete", "fixture")
			},
		},
		{
			name:      "creem api missing",
			configure: func() { setting.CreemApiKey = "" },
			deliver:   func(t *testing.T) int { return deliverCreemSubscriptionFixture(t, "subscription-disabled", "fixture") },
		},
		{
			name:      "creem signature secret missing",
			configure: func() { setting.CreemWebhookSecret = "" },
			deliver:   func(t *testing.T) int { return deliverCreemSubscriptionFixture(t, "subscription-disabled", "fixture") },
		},
		{
			name: "compliance false",
			configure: func() {
				require.NoError(t, config.UpdateConfigFromMap(config.GlobalConfig.Get("payment_setting"), map[string]string{"compliance_confirmed": "false"}))
			},
			deliver: func(t *testing.T) int {
				return deliverStripeFixture(t, "checkout.session.completed", "subscription-disabled", "complete", setting.StripeWebhookSecret)
			},
		},
		{
			name: "terms expired",
			configure: func() {
				require.NoError(t, config.UpdateConfigFromMap(config.GlobalConfig.Get("payment_setting"), map[string]string{"compliance_terms_version": "expired"}))
			},
			deliver: func(t *testing.T) int {
				return deliverCreemSubscriptionFixture(t, "subscription-disabled", setting.CreemWebhookSecret)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := paymentWebhookTestDB(t)
			subscriptionWebhookSettings(t)
			tc.configure()
			assert.Equal(t, http.StatusForbidden, tc.deliver(t))
			assert.Zero(t, subscriptionEntitlementCount(t, db))
		})
	}
}
