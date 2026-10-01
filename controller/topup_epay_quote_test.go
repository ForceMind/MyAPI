package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/setting"
	"github.com/ForceMind/MyAPI/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupEpayQuoteFixture(t *testing.T) *gorm.DB {
	t.Helper()
	db := paymentWebhookTestDB(t)
	model.InitColumnNamesForTest()
	config := setting.CapturePaymentConfig()
	methods, err := common.Marshal(operation_setting.GetPayMethods())
	require.NoError(t, err)
	discounts, err := common.Marshal(operation_setting.GetPaymentSetting().AmountDiscount)
	require.NoError(t, err)
	saved := map[string]string{"QuotaPerUnit": strconv.FormatFloat(common.QuotaPerUnit, 'g', -1, 64), "Price": strconv.FormatFloat(config.Price(), 'g', -1, 64),
		"MinTopUp": strconv.Itoa(config.MinTopUp()), "PayAddress": config.PayAddress(), "EpayId": config.EpayId(), "EpayKey": config.EpayKey(),
		"PayMethods": string(methods), "TopupGroupRatio": common.TopupGroupRatio2JSONString(), "payment_setting.amount_discount": string(discounts),
		"general_setting.quota_display_type": operation_setting.GetQuotaDisplayType()}
	previousPurchase := epayPurchase
	t.Cleanup(func() {
		epayPurchase = previousPurchase
		require.NoError(t, model.UpdateOptionsBulk(saved))
		model.InitOptionMap()
	})
	require.NoError(t, model.UpdateOptionsBulk(map[string]string{"QuotaPerUnit": "100", "Price": "2", "MinTopUp": "1", "PayAddress": "https://epay-fixture.invalid", "EpayId": "synthetic-shop", "EpayKey": "synthetic-key",
		"PayMethods": `[{"type":"alipay","name":"Alipay"}]`, "TopupGroupRatio": `{"default":2}`, "payment_setting.amount_discount": `{"250":0.5}`,
		"general_setting.quota_display_type": "TOKENS"}))
	model.InitOptionMap()
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", 1).Update("group", "default").Error)
	return db
}

func requestEpayQuoteFixture() *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/user/pay", strings.NewReader(`{"amount":250,"payment_method":"alipay"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("id", 1)
	c.Set(userFundingEpochContextKey, operation_setting.GetUserFundingSetting().Epoch)
	RequestEpay(c)
	return recorder
}

func TestEpayOrderAndSignedParametersUseOneSnapshotAcrossConfigurationChange(t *testing.T) {
	db := setupEpayQuoteFixture(t)
	original := epayPurchase
	called := false
	epayPurchase = func(client *epay.Client, args *epay.PurchaseArgs) (string, map[string]string, error) {
		called = true
		var stored model.TopUp
		require.NoError(t, db.Where("trade_no = ?", args.ServiceTradeNo).First(&stored).Error)
		assert.Equal(t, "100", stored.QuotaPerUnitSnapshot)
		assert.EqualValues(t, 2, stored.Amount)
		assert.Equal(t, 5.0, stored.Money)
		assert.Equal(t, "5.00", args.Money)
		require.NoError(t, model.UpdateOptionsBulk(map[string]string{"QuotaPerUnit": "200", "Price": "7", "TopupGroupRatio": `{"default":1}`, "payment_setting.amount_discount": `{"250":1}`, "general_setting.quota_display_type": "USD", "EpayId": "changed-shop", "EpayKey": "changed-key"}))
		return original(client, args)
	}
	response := requestEpayQuoteFixture()
	require.True(t, called)
	assert.Equal(t, http.StatusOK, response.Code)
	var result struct {
		Message string            `json:"message"`
		Data    map[string]string `json:"data"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
	assert.Equal(t, "success", result.Message)
	assert.Equal(t, "5.00", result.Data["money"])
	assert.Equal(t, "synthetic-shop", result.Data["pid"])
	_, err := model.RechargeEpayTrusted(result.Data["out_trade_no"], "alipay", "127.0.0.1")
	require.NoError(t, err)
	var user model.User
	require.NoError(t, db.First(&user, 1).Error)
	assert.Equal(t, 300, user.Quota)
}

func TestEpayInsertFailureDoesNotGeneratePaymentParameters(t *testing.T) {
	db := setupEpayQuoteFixture(t)
	called := false
	epayPurchase = func(*epay.Client, *epay.PurchaseArgs) (string, map[string]string, error) {
		called = true
		return "", nil, errors.New("unexpected SDK call")
	}
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("fixture_epay_insert_failure", func(tx *gorm.DB) {
		if tx.Statement.Table == "top_ups" {
			tx.AddError(errors.New("synthetic insertion failure"))
		}
	}))
	response := requestEpayQuoteFixture()
	assert.False(t, called)
	assert.Contains(t, response.Body.String(), "创建订单失败")
	var count int64
	require.NoError(t, db.Model(&model.TopUp{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestEpayParameterFailureLeavesRecoverableFailedOrder(t *testing.T) {
	db := setupEpayQuoteFixture(t)
	epayPurchase = func(*epay.Client, *epay.PurchaseArgs) (string, map[string]string, error) {
		return "", nil, errors.New("synthetic parameter failure")
	}
	response := requestEpayQuoteFixture()
	assert.Contains(t, response.Body.String(), "拉起支付失败")
	var order model.TopUp
	require.NoError(t, db.First(&order).Error)
	assert.Equal(t, common.TopUpStatusFailed, order.Status)
	assert.Equal(t, "100", order.QuotaPerUnitSnapshot)
	var user model.User
	require.NoError(t, db.First(&user, 1).Error)
	assert.Equal(t, 100, user.Quota)
}

func TestEpayUnavailablePricingDoesNotCreateOrderOrParameters(t *testing.T) {
	db := setupEpayQuoteFixture(t)
	require.NoError(t, db.Model(&model.Option{}).Where("key = ?", "QuotaPerUnit").Update("value", "NaN").Error)
	model.InitOptionMap()
	assert.False(t, model.PricingRuntimeReady())
	called := false
	epayPurchase = func(*epay.Client, *epay.PurchaseArgs) (string, map[string]string, error) {
		called = true
		return "", nil, nil
	}
	response := requestEpayQuoteFixture()
	assert.Equal(t, http.StatusServiceUnavailable, response.Code)
	assert.Contains(t, response.Body.String(), "PRICING_RUNTIME_UNAVAILABLE")
	assert.False(t, called)
	var count int64
	require.NoError(t, db.Model(&model.TopUp{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestEpaySignedNotificationUsesStoredUnitAfterPricingChange(t *testing.T) {
	db := setupEpayQuoteFixture(t)
	response := requestEpayQuoteFixture()
	var result struct {
		Message string            `json:"message"`
		Data    map[string]string `json:"data"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
	require.Equal(t, "success", result.Message)
	require.NoError(t, model.UpdateOptionsBulk(map[string]string{"QuotaPerUnit": "200", "Price": "7", "general_setting.quota_display_type": "USD"}))
	notification := epay.GenerateParams(map[string]string{"out_trade_no": result.Data["out_trade_no"], "pid": "synthetic-shop", "type": "alipay", "trade_status": epay.StatusTradeSuccess, "money": "5.00", "sign_type": "MD5"}, "synthetic-key")
	values := url.Values{}
	for key, value := range notification {
		values.Set(key, value)
	}
	for attempt := 0; attempt < 2; attempt++ {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/user/epay/notify", strings.NewReader(values.Encode()))
		c.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		EpayNotify(c)
		require.Equal(t, "success", recorder.Body.String())
	}
	var user model.User
	require.NoError(t, db.First(&user, 1).Error)
	assert.Equal(t, 300, user.Quota)
	var order model.TopUp
	require.NoError(t, db.Where("trade_no = ?", result.Data["out_trade_no"]).First(&order).Error)
	assert.Equal(t, "100", order.QuotaPerUnitSnapshot)
	assert.Equal(t, common.TopUpStatusSuccess, order.Status)
}
