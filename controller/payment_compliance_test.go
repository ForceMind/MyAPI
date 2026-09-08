package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/ForceMind/MyAPI/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func paymentComplianceControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Option{}))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	previousDB := model.DB
	model.DB = db
	common.OptionMapRWMutex.Lock()
	previousOptions := common.OptionMap
	common.OptionMap = map[string]string{"fixture": "before"}
	common.OptionMapRWMutex.Unlock()
	paymentSetting := config.GlobalConfig.Get("payment_setting")
	require.NotNil(t, paymentSetting)
	previousPaymentSetting, err := config.ConfigToMap(paymentSetting)
	require.NoError(t, err)
	require.NoError(t, config.UpdateConfigFromMap(paymentSetting, map[string]string{
		"amount_options":           `null`,
		"amount_discount":          `null`,
		"compliance_confirmed":     "false",
		"compliance_terms_version": "",
		"compliance_confirmed_at":  "0",
		"compliance_confirmed_by":  "0",
		"compliance_confirmed_ip":  "",
	}))
	t.Cleanup(func() {
		model.DB = previousDB
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptions
		common.OptionMapRWMutex.Unlock()
		require.NoError(t, config.UpdateConfigFromMap(paymentSetting, previousPaymentSetting))
		require.NoError(t, sqlDB.Close())
	})
	return db
}

func confirmPaymentComplianceRequest(t *testing.T) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	router := gin.New()
	router.POST("/api/option/payment_compliance", func(context *gin.Context) {
		context.Set("id", 42)
		ConfirmPaymentCompliance(context)
	})
	request := httptest.NewRequest(http.MethodPost, "/api/option/payment_compliance", strings.NewReader(`{"confirmed":true}`))
	request.RemoteAddr = "192.0.2.17:12345"
	router.ServeHTTP(response, request)
	return response
}

func paymentComplianceOldOptionValues() map[string]string {
	return map[string]string{
		"payment_setting.compliance_confirmed":     "false",
		"payment_setting.compliance_terms_version": "v0",
		"payment_setting.compliance_confirmed_at":  "1600000000",
		"payment_setting.compliance_confirmed_by":  "7",
		"payment_setting.compliance_confirmed_ip":  "198.51.100.7",
	}
}

func seedPaymentComplianceOldState(t *testing.T, db *gorm.DB) operation_setting.PaymentSetting {
	t.Helper()
	oldOptions := paymentComplianceOldOptionValues()
	for key, value := range oldOptions {
		require.NoError(t, db.Create(&model.Option{Key: key, Value: value}).Error)
	}
	common.OptionMapRWMutex.Lock()
	for key, value := range oldOptions {
		common.OptionMap[key] = value
	}
	common.OptionMapRWMutex.Unlock()
	oldPaymentSetting := operation_setting.PaymentSetting{
		ComplianceTermsVersion: oldOptions["payment_setting.compliance_terms_version"],
		ComplianceConfirmedAt:  1600000000,
		ComplianceConfirmedBy:  7,
		ComplianceConfirmedIP:  oldOptions["payment_setting.compliance_confirmed_ip"],
	}
	registered := config.GlobalConfig.Get("payment_setting")
	require.NotNil(t, registered)
	require.NoError(t, config.UpdateConfigFromMap(registered, map[string]string{
		"amount_options":           `null`,
		"amount_discount":          `null`,
		"compliance_confirmed":     "false",
		"compliance_terms_version": oldPaymentSetting.ComplianceTermsVersion,
		"compliance_confirmed_at":  "1600000000",
		"compliance_confirmed_by":  "7",
		"compliance_confirmed_ip":  oldPaymentSetting.ComplianceConfirmedIP,
	}))
	return oldPaymentSetting
}

func TestConfirmPaymentCompliancePersistsAndPublishesAllOptions(t *testing.T) {
	db := paymentComplianceControllerTestDB(t)

	response := confirmPaymentComplianceRequest(t)
	assert.Equal(t, http.StatusOK, response.Code)
	var payload struct {
		Success bool `json:"success"`
		Data    struct {
			Confirmed    bool   `json:"confirmed"`
			TermsVersion string `json:"terms_version"`
			ConfirmedAt  int64  `json:"confirmed_at"`
			ConfirmedBy  int    `json:"confirmed_by"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	require.True(t, payload.Data.Confirmed)
	require.Equal(t, operation_setting.CurrentComplianceTermsVersion, payload.Data.TermsVersion)
	require.NotZero(t, payload.Data.ConfirmedAt)
	require.Equal(t, 42, payload.Data.ConfirmedBy)

	expectedOptions := map[string]string{
		"payment_setting.compliance_confirmed":     strconv.FormatBool(payload.Data.Confirmed),
		"payment_setting.compliance_terms_version": payload.Data.TermsVersion,
		"payment_setting.compliance_confirmed_at":  strconv.FormatInt(payload.Data.ConfirmedAt, 10),
		"payment_setting.compliance_confirmed_by":  strconv.Itoa(payload.Data.ConfirmedBy),
		"payment_setting.compliance_confirmed_ip":  "192.0.2.17",
	}
	var storedOptions []model.Option
	require.NoError(t, db.Find(&storedOptions).Error)
	persisted := make(map[string]string, len(storedOptions))
	for _, option := range storedOptions {
		persisted[option.Key] = option.Value
	}
	assert.Equal(t, expectedOptions, persisted)

	common.OptionMapRWMutex.RLock()
	published := make(map[string]string, len(common.OptionMap))
	for key, value := range common.OptionMap {
		published[key] = value
	}
	common.OptionMapRWMutex.RUnlock()
	expectedPublished := map[string]string{"fixture": "before"}
	for key, value := range expectedOptions {
		expectedPublished[key] = value
	}
	assert.Equal(t, expectedPublished, published)
	assert.Equal(t, operation_setting.PaymentSetting{
		ComplianceConfirmed:    payload.Data.Confirmed,
		ComplianceTermsVersion: payload.Data.TermsVersion,
		ComplianceConfirmedAt:  payload.Data.ConfirmedAt,
		ComplianceConfirmedBy:  payload.Data.ConfirmedBy,
		ComplianceConfirmedIP:  expectedOptions["payment_setting.compliance_confirmed_ip"],
	}, *operation_setting.GetPaymentSetting())
}

func TestConfirmPaymentComplianceRollsBackAllOptionsOnPersistenceFailure(t *testing.T) {
	db := paymentComplianceControllerTestDB(t)
	oldOptions := paymentComplianceOldOptionValues()
	oldPaymentSetting := seedPaymentComplianceOldState(t, db)
	injected := errors.New("injected payment compliance persistence failure")
	updates := 0
	const callbackName = "test:payment-compliance-bulk-failure"
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		updates++
		if updates == 2 {
			tx.AddError(injected)
		}
	}))
	t.Cleanup(func() { require.NoError(t, db.Callback().Update().Remove(callbackName)) })

	response := confirmPaymentComplianceRequest(t)
	assert.Equal(t, http.StatusOK, response.Code)
	var payload struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &payload))
	assert.False(t, payload.Success)
	assert.Contains(t, payload.Message, injected.Error())
	assert.Equal(t, 2, updates)

	var options []model.Option
	require.NoError(t, db.Find(&options).Error)
	persisted := make(map[string]string, len(options))
	for _, option := range options {
		persisted[option.Key] = option.Value
	}
	assert.Equal(t, oldOptions, persisted)
	common.OptionMapRWMutex.RLock()
	expectedPublished := map[string]string{"fixture": "before"}
	for key, value := range oldOptions {
		expectedPublished[key] = value
	}
	assert.Equal(t, expectedPublished, common.OptionMap)
	common.OptionMapRWMutex.RUnlock()
	assert.Equal(t, oldPaymentSetting, *operation_setting.GetPaymentSetting())
}
