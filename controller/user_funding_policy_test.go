package controller

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/service"
	"github.com/ForceMind/MyAPI/setting"
	"github.com/ForceMind/MyAPI/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setUserFundingModeForTest(t *testing.T, mode operation_setting.UserFundingMode) {
	t.Helper()
	_, err := model.TransitionUserFundingMode(mode)
	require.NoError(t, err)
}

func TestUserFundingMutationGateEntryMatrix(t *testing.T) {
	paths := []string{
		"/api/user/topup",
		"/api/user/pay",
		"/api/user/amount",
		"/api/user/stripe/pay",
		"/api/user/stripe/amount",
		"/api/user/creem/pay",
		"/api/user/waffo/amount",
		"/api/user/waffo/pay",
		"/api/user/waffo-pancake/amount",
		"/api/user/waffo-pancake/pay",
		"/api/user/aff_transfer",
		"/api/subscription/balance/pay",
		"/api/subscription/epay/pay",
		"/api/subscription/stripe/pay",
		"/api/subscription/creem/pay",
		"/api/subscription/waffo-pancake/pay",
	}

	for _, mode := range []operation_setting.UserFundingMode{
		operation_setting.UserFundingModeRetirement,
		operation_setting.UserFundingModeDisabled,
	} {
		t.Run(string(mode), func(t *testing.T) {
			paymentWebhookTestDB(t)
			setUserFundingModeForTest(t, mode)
			for _, path := range paths {
				response := httptest.NewRecorder()
				called := false
				router := gin.New()
				router.POST(path, UserFundingMutationGate, func(c *gin.Context) { called = true })
				router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`)))

				assert.Equal(t, http.StatusForbidden, response.Code, path)
				assert.False(t, called, path)
				var payload map[string]any
				require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
				assert.Equal(t, service.UserFundingUnavailableCode, payload["code"], path)
			}
		})
	}
}

func TestUserFundingMutationGateAllowsEnabledMode(t *testing.T) {
	paymentWebhookTestDB(t)
	response := httptest.NewRecorder()
	called := false
	router := gin.New()
	router.POST("/api/user/topup", UserFundingMutationGate, func(c *gin.Context) {
		called = true
		c.Status(http.StatusNoContent)
	})
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/user/topup", strings.NewReader(`{}`)))
	assert.True(t, called)
	assert.Equal(t, http.StatusNoContent, response.Code)
}

func TestUnavailableTopUpInfoExposesOnlyCapabilities(t *testing.T) {
	paymentWebhookTestDB(t)
	setUserFundingModeForTest(t, operation_setting.UserFundingModeDisabled)
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/user/topup/info", nil)
	GetTopUpInfo(context)

	require.Equal(t, http.StatusOK, response.Code)
	var payload struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	assert.Equal(t, "disabled", payload.Data["user_funding_mode"])
	assert.Equal(t, false, payload.Data["enable_online_topup"])
	assert.Equal(t, false, payload.Data["enable_redemption"])
	assert.NotContains(t, payload.Data, "creem_products")
	assert.NotContains(t, payload.Data, "topup_link")
	assert.NotContains(t, payload.Data, "min_topup")
}

func TestStatusAndTopUpInfoExposeMatchingCapabilities(t *testing.T) {
	paymentWebhookTestDB(t)
	setUserFundingModeForTest(t, operation_setting.UserFundingModeRetirement)
	read := func(handler gin.HandlerFunc, path string) map[string]any {
		response := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(response)
		context.Request = httptest.NewRequest(http.MethodGet, path, nil)
		handler(context)
		require.Equal(t, http.StatusOK, response.Code)
		var payload struct {
			Data map[string]any `json:"data"`
		}
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
		return payload.Data
	}

	status := read(GetStatus, "/api/status")
	topup := read(GetTopUpInfo, "/api/user/topup/info")
	assert.Equal(t, status["user_funding_mode"], topup["user_funding_mode"])
	assert.Equal(t, status["user_funding_capabilities"], topup["user_funding_capabilities"])
}

func TestSelfUseSetupInitializesUserFundingDisabled(t *testing.T) {
	db := paymentWebhookTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.Setup{}))
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", 1).Update("aff_code", "fixture-aff").Error)
	common.OptionMapRWMutex.Lock()
	originalOptionMap := common.OptionMap
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = originalOptionMap
		common.OptionMapRWMutex.Unlock()
	})
	originalSetup := constant.Setup
	constant.Setup = false
	t.Cleanup(func() { constant.Setup = originalSetup })

	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/setup", strings.NewReader(`{
        "username":"root-wp3",
        "password":"password123",
        "confirmPassword":"password123",
        "SelfUseModeEnabled":true,
        "DemoSiteEnabled":false
    }`))
	context.Request.Header.Set("Content-Type", "application/json")
	PostSetup(context)

	require.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, operation_setting.UserFundingModeDisabled, operation_setting.GetUserFundingMode())
	var option model.Option
	require.NoError(t, db.Where("key = ?", "user_funding_setting.mode").First(&option).Error)
	assert.Equal(t, "disabled", option.Value)
}

func TestRetirementStripeCallbackSettlesPendingAndAcknowledgesUnknownDuplicate(t *testing.T) {
	db := paymentWebhookTestDB(t)
	stripeWebhookFixture(t)
	tradeNo := "stripe-retirement"
	require.NoError(t, db.Create(&model.TopUp{
		UserId: 1, TradeNo: tradeNo, Amount: 1, Money: 1,
		PaymentProvider: model.PaymentProviderStripe,
		PaymentMethod:   model.PaymentMethodStripe,
		Status:          common.TopUpStatusPending,
	}).Error)
	setUserFundingModeForTest(t, operation_setting.UserFundingModeRetirement)

	assert.Equal(t, http.StatusOK, deliverStripeFixture(t, "checkout.session.completed", tradeNo, "complete", "whsec_fixture"))
	assert.Equal(t, http.StatusOK, deliverStripeFixture(t, "checkout.session.completed", tradeNo, "complete", "whsec_fixture"))
	assert.Equal(t, http.StatusOK, deliverStripeFixture(t, "checkout.session.completed", "stripe-retirement-unknown", "complete", "whsec_fixture"))

	var user model.User
	require.NoError(t, db.First(&user, 1).Error)
	assert.Equal(t, 100+int(common.QuotaPerUnit), user.Quota)
}

func TestRetirementWaffoPancakeCallbackSettlesPendingAndAcknowledgesUnknownDuplicate(t *testing.T) {
	db := paymentWebhookTestDB(t)
	tradeNo := "WAFFO_PANCAKE-retirement"
	require.NoError(t, db.Create(&model.TopUp{
		UserId: 1, TradeNo: tradeNo, Amount: 1,
		PaymentProvider: model.PaymentProviderWaffoPancake,
		Status:          common.TopUpStatusPending,
	}).Error)
	setUserFundingModeForTest(t, operation_setting.UserFundingModeRetirement)
	event := &service.WaffoPancakeWebhookEvent{
		EventType: "order.completed",
		Mode:      "test",
		Data: service.WaffoPancakeWebhookData{
			OrderMerchantExternalID:       tradeNo,
			MerchantProvidedBuyerIdentity: "my-api-user-1",
		},
	}

	assert.Equal(t, http.StatusOK, deliverVerifiedPancakeFixture(t, event))
	var user model.User
	require.NoError(t, db.First(&user, 1).Error)
	assert.Equal(t, 100+int(common.QuotaPerUnit), user.Quota)

	assert.Equal(t, http.StatusOK, deliverVerifiedPancakeFixture(t, event))
	require.NoError(t, db.First(&user, 1).Error)
	assert.Equal(t, 100+int(common.QuotaPerUnit), user.Quota)

	unknown := *event
	unknown.Data.OrderMerchantExternalID = "WAFFO_PANCAKE-unknown"
	assert.Equal(t, http.StatusOK, deliverVerifiedPancakeFixture(t, &unknown))
	require.NoError(t, db.First(&user, 1).Error)
	assert.Equal(t, 100+int(common.QuotaPerUnit), user.Quota)
}

func TestDisabledWaffoPancakeCallbackAcknowledgesWithoutCredit(t *testing.T) {
	db := paymentWebhookTestDB(t)
	setUserFundingModeForTest(t, operation_setting.UserFundingModeDisabled)
	tradeNo := "WAFFO_PANCAKE-disabled"
	require.NoError(t, db.Create(&model.TopUp{
		UserId: 1, TradeNo: tradeNo, Amount: 1,
		PaymentProvider: model.PaymentProviderWaffoPancake,
		Status:          common.TopUpStatusPending,
	}).Error)
	event := &service.WaffoPancakeWebhookEvent{
		EventType: "order.completed",
		Data: service.WaffoPancakeWebhookData{
			OrderMerchantExternalID:       tradeNo,
			MerchantProvidedBuyerIdentity: "my-api-user-1",
		},
	}

	assert.Equal(t, http.StatusOK, deliverVerifiedPancakeFixture(t, event))
	var user model.User
	require.NoError(t, db.First(&user, 1).Error)
	assert.Equal(t, 100, user.Quota)
	assert.Equal(t, common.TopUpStatusPending, model.GetTopUpByTradeNo(tradeNo).Status)
}

func TestRetirementSubscriptionCallbackSettlesExistingPendingExactlyOnce(t *testing.T) {
	db := paymentWebhookTestDB(t)
	subscriptionWebhookSettings(t)
	order := createSubscriptionWebhookOrder(
		t,
		db,
		model.PaymentProviderStripe,
		model.PaymentMethodStripe,
		"subscription-retirement-stripe",
	)
	setUserFundingModeForTest(t, operation_setting.UserFundingModeRetirement)

	assert.Equal(t, http.StatusOK, deliverStripeFixture(t, "checkout.session.completed", order.TradeNo, "complete", setting.StripeWebhookSecret))
	assert.Equal(t, http.StatusOK, deliverStripeFixture(t, "checkout.session.completed", order.TradeNo, "complete", setting.StripeWebhookSecret))
	assert.EqualValues(t, 1, subscriptionEntitlementCount(t, db))
	assert.Equal(t, common.TopUpStatusSuccess, model.GetSubscriptionOrderByTradeNo(order.TradeNo).Status)
}

func TestDisabledSignedStripeCallbackAcknowledgesWithoutCredit(t *testing.T) {
	db := paymentWebhookTestDB(t)
	stripeWebhookFixture(t)
	setUserFundingModeForTest(t, operation_setting.UserFundingModeDisabled)
	order := model.TopUp{
		UserId: 1, TradeNo: "stripe-disabled", Amount: 1, Money: 1,
		PaymentProvider: model.PaymentProviderStripe,
		PaymentMethod:   model.PaymentMethodStripe,
		Status:          common.TopUpStatusPending,
	}
	require.NoError(t, db.Create(&order).Error)

	assert.Equal(t, http.StatusOK, deliverStripeFixture(t, "checkout.session.completed", order.TradeNo, "complete", setting.StripeWebhookSecret))
	var user model.User
	require.NoError(t, db.First(&user, 1).Error)
	assert.Equal(t, 100, user.Quota)
	require.NoError(t, db.First(&order, order.Id).Error)
	assert.Equal(t, common.TopUpStatusPending, order.Status)
}

func TestUnavailableUserSubscriptionCatalogIsEmpty(t *testing.T) {
	paymentWebhookTestDB(t)
	setUserFundingModeForTest(t, operation_setting.UserFundingModeRetirement)
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/subscription/plans", nil)
	GetSubscriptionPlans(context)

	require.Equal(t, http.StatusOK, response.Code)
	var payload struct {
		Data []SubscriptionPlanDTO `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	assert.Empty(t, payload.Data)
}

func TestSetupFailureRollsBackRootAndFundingOptions(t *testing.T) {
	db := paymentWebhookTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Setup{}))
	originalSetup := constant.Setup
	constant.Setup = false
	t.Cleanup(func() { constant.Setup = originalSetup })
	injected := errors.New("injected setup write failure")
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:setup-failure", func(tx *gorm.DB) {
		if tx.Statement.Table == "setups" {
			tx.AddError(injected)
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Create().Remove("test:setup-failure") })

	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/setup", strings.NewReader(`{
        "username":"rollback-root",
        "password":"password123",
        "confirmPassword":"password123",
        "SelfUseModeEnabled":true,
        "DemoSiteEnabled":true
    }`))
	context.Request.Header.Set("Content-Type", "application/json")
	PostSetup(context)

	assert.Equal(t, http.StatusOK, response.Code)
	var rootCount int64
	require.NoError(t, db.Model(&model.User{}).Where("role = ?", common.RoleRootUser).Count(&rootCount).Error)
	assert.Zero(t, rootCount)
	var setupCount int64
	require.NoError(t, db.Model(&model.Setup{}).Count(&setupCount).Error)
	assert.Zero(t, setupCount)
	state, err := model.GetUserFundingStateSnapshot()
	require.NoError(t, err)
	assert.Equal(t, operation_setting.UserFundingModeEnabled, state.Mode)
	assert.EqualValues(t, 1, state.Epoch)
	assert.False(t, constant.Setup)
}

func TestDisabledSignedNonSuccessEpayCallbacksAcknowledge(t *testing.T) {
	paymentWebhookTestDB(t)
	setUserFundingModeForTest(t, operation_setting.UserFundingModeDisabled)
	oldAddress, oldID, oldKey := operation_setting.PayAddress, operation_setting.EpayId, operation_setting.EpayKey
	operation_setting.PayAddress, operation_setting.EpayId, operation_setting.EpayKey = "https://pay.example.test", "merchant", "secret"
	t.Cleanup(func() {
		operation_setting.PayAddress, operation_setting.EpayId, operation_setting.EpayKey = oldAddress, oldID, oldKey
	})
	params := map[string]string{
		"type":         "alipay",
		"trade_no":     "provider-order",
		"out_trade_no": "disabled-non-success",
		"name":         "fixture",
		"money":        "1.00",
		"trade_status": "TRADE_CLOSED",
	}
	signed := epay.GenerateParams(params, operation_setting.EpayKey)
	form := url.Values{}
	for key, value := range signed {
		form.Set(key, value)
	}
	for _, handler := range []gin.HandlerFunc{EpayNotify, SubscriptionEpayNotify} {
		response := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(response)
		context.Request = httptest.NewRequest(http.MethodPost, "/api/epay/notify", strings.NewReader(form.Encode()))
		context.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		handler(context)
		assert.Equal(t, "success", response.Body.String())
	}
}

func TestUserFundingGateUsesBackendI18nMessage(t *testing.T) {
	paymentWebhookTestDB(t)
	setUserFundingModeForTest(t, operation_setting.UserFundingModeDisabled)
	require.NoError(t, i18n.Init())

	response := httptest.NewRecorder()
	router := gin.New()
	router.POST("/api/user/topup", UserFundingMutationGate, func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	request := httptest.NewRequest(http.MethodPost, "/api/user/topup", strings.NewReader(`{}`))
	request.Header.Set("Accept-Language", i18n.LangEn)
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusForbidden, response.Code)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	assert.Equal(t, service.UserFundingUnavailableCode, payload["code"])
	assert.Equal(t, i18n.Translate(i18n.LangEn, i18n.MsgPaymentComplianceRequired), payload["message"])
}
