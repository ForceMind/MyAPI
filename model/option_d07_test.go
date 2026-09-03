package model

import (
	"errors"
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/setting"
	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/ForceMind/MyAPI/setting/operation_setting"
	"github.com/ForceMind/MyAPI/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestValidateOptionValueRejectsMalformedD07Settings(t *testing.T) {
	for _, testCase := range []struct {
		key   string
		value string
	}{
		{key: "ModelRequestRateLimitEnabled", value: "not-a-bool"},
		{key: "ModelRequestRateLimitDurationMinutes", value: "-1"},
		{key: "ModelRequestRateLimitCount", value: "-1"},
		{key: "ModelRequestRateLimitSuccessCount", value: "0"},
		{key: "ModelRequestRateLimitGroup", value: `{"vip":[10,8],"wrong":"not-a-pair"}`},
		{key: "ModelRatio", value: `{"model":"wrong"}`},
		{key: "ModelPrice", value: `{"model":"wrong"}`},
		{key: "CompletionRatio", value: `{"model":"wrong"}`},
		{key: "CacheRatio", value: `{"model":"wrong"}`},
		{key: "CreateCacheRatio", value: `{"model":"wrong"}`},
		{key: "ImageRatio", value: `{"model":"wrong"}`},
		{key: "AudioRatio", value: `{"model":"wrong"}`},
		{key: "AudioCompletionRatio", value: `{"model":"wrong"}`},
		{key: "GroupRatio", value: `{"vip":-0.1}`},
		{key: "GroupGroupRatio", value: `{"vip":{"default":-0.1}}`},
		{key: "group_ratio_setting.group_ratio", value: `{"vip":-0.1}`},
		{key: "group_ratio_setting.group_group_ratio", value: `{"vip":{"default":-0.1}}`},
		{key: "group_ratio_setting.group_special_usable_group", value: `{"vip":null}`},
		{key: "gemini.safety_settings", value: `{"default":"BLOCK_SOME"}`},
		{key: "gemini.thinking_adapter_budget_tokens_percentage", value: "0.001"},
		{key: "claude.default_max_tokens", value: `{"default":2147483647}`},
		{key: "claude.thinking_adapter_budget_tokens_percentage", value: "0"},
		{key: "AutoGroups", value: `["default",1]`},
		{key: "TopupGroupRatio", value: `{"default":-1}`},
		{key: "PayMethods", value: `[{"type":"ok"},{"type":1}]`},
		{key: "AutomaticDisableStatusCodes", value: "99"},
		{key: "AutomaticRetryStatusCodes", value: "invalid"},
		{key: "Chats", value: `{"not":"an-array"}`},
		{key: "UserUsableGroups", value: `["not","an","object"]`},
	} {
		t.Run(testCase.key, func(t *testing.T) {
			require.Error(t, validateOptionValue(testCase.key, testCase.value))
		})
	}
}

func TestAuxiliaryOptionValidationRejectsMixedBulkBeforePersistence(t *testing.T) {
	db := accessProfileTestDB(t)
	require.NoError(t, db.AutoMigrate(&Option{}))
	previousAutoGroups := setting.AutoGroups2JsonString()
	previousPayMethods := operation_setting.PayMethods2JsonString()
	previousTopupRatios := common.TopupGroupRatio2JSONString()
	previousDisableCodes := operation_setting.AutomaticDisableStatusCodesToString()
	previousRetryCodes := operation_setting.AutomaticRetryStatusCodesToString()
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["default"]`))
	require.NoError(t, operation_setting.UpdatePayMethodsByJsonString(`[{"type":"existing"}]`))
	require.NoError(t, common.UpdateTopupGroupRatioByJSONString(`{"default":1}`))
	require.NoError(t, operation_setting.AutomaticDisableStatusCodesFromString("401"))
	require.NoError(t, operation_setting.AutomaticRetryStatusCodesFromString("500-503"))
	common.OptionMapRWMutex.Lock()
	previousOptions := common.OptionMap
	common.OptionMap = map[string]string{"Notice": "before"}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(previousAutoGroups))
		require.NoError(t, operation_setting.UpdatePayMethodsByJsonString(previousPayMethods))
		require.NoError(t, common.UpdateTopupGroupRatioByJSONString(previousTopupRatios))
		require.NoError(t, operation_setting.AutomaticDisableStatusCodesFromString(previousDisableCodes))
		require.NoError(t, operation_setting.AutomaticRetryStatusCodesFromString(previousRetryCodes))
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptions
		common.OptionMapRWMutex.Unlock()
	})

	for key, value := range map[string]string{
		"AutoGroups":                  `["next",1]`,
		"TopupGroupRatio":             `{"default":-1}`,
		"PayMethods":                  `[{"type":"ok"},{"type":1}]`,
		"AutomaticDisableStatusCodes": "99",
		"AutomaticRetryStatusCodes":   "invalid",
	} {
		require.Error(t, UpdateOptionsBulk(map[string]string{"Notice": "after", key: value}), key)
	}

	var persisted int64
	require.NoError(t, db.Model(&Option{}).Count(&persisted).Error)
	assert.Zero(t, persisted)
	assert.JSONEq(t, `["default"]`, setting.AutoGroups2JsonString())
	assert.JSONEq(t, `[{"type":"existing"}]`, operation_setting.PayMethods2JsonString())
	assert.JSONEq(t, `{"default":1}`, common.TopupGroupRatio2JSONString())
	assert.Equal(t, "401", operation_setting.AutomaticDisableStatusCodesToString())
	assert.Equal(t, "500-503", operation_setting.AutomaticRetryStatusCodesToString())
	common.OptionMapRWMutex.RLock()
	assert.Equal(t, "before", common.OptionMap["Notice"])
	common.OptionMapRWMutex.RUnlock()
}

func TestNullCollectionOptionsArePersistedAndPublishedCanonically(t *testing.T) {
	db := accessProfileTestDB(t)
	require.NoError(t, db.AutoMigrate(&Option{}))
	previousChats := setting.Chats2JsonString()
	previousUserGroups := setting.UserUsableGroups2JSONString()
	previousAutoGroups := setting.AutoGroups2JsonString()
	previousPayMethods := operation_setting.PayMethods2JsonString()
	common.OptionMapRWMutex.Lock()
	previousOptions := common.OptionMap
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateChatsByJsonString(previousChats))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(previousUserGroups))
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(previousAutoGroups))
		require.NoError(t, operation_setting.UpdatePayMethodsByJsonString(previousPayMethods))
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptions
		common.OptionMapRWMutex.Unlock()
	})

	require.NoError(t, UpdateOptionsBulk(map[string]string{
		"Chats":            " \n null \t",
		"UserUsableGroups": "null",
		"AutoGroups":       " null ",
		"PayMethods":       "null",
	}))
	expected := map[string]string{
		"Chats":            "[]",
		"UserUsableGroups": "{}",
		"AutoGroups":       "[]",
		"PayMethods":       "[]",
	}
	for key, value := range expected {
		var option Option
		require.NoError(t, db.First(&option, "`key` = ?", key).Error)
		assert.Equal(t, value, option.Value)
	}
	common.OptionMapRWMutex.RLock()
	for key, value := range expected {
		assert.Equal(t, value, common.OptionMap[key])
	}
	common.OptionMapRWMutex.RUnlock()
	assert.JSONEq(t, `[]`, setting.Chats2JsonString())
	assert.JSONEq(t, `{}`, setting.UserUsableGroups2JSONString())
	assert.JSONEq(t, `[]`, setting.AutoGroups2JsonString())
	assert.JSONEq(t, `[]`, operation_setting.PayMethods2JsonString())
}

func TestGenericConfigValidationRejectsMalformedValueBeforePersistence(t *testing.T) {
	db := accessProfileTestDB(t)
	require.NoError(t, db.AutoMigrate(&Option{}))

	claudeConfig := config.GlobalConfig.Get("claude")
	require.NotNil(t, claudeConfig)
	beforeConfig, err := config.ConfigToMap(claudeConfig)
	require.NoError(t, err)

	const key = "claude.thinking_adapter_enabled"
	common.OptionMapRWMutex.Lock()
	previousOptions := common.OptionMap
	common.OptionMap = map[string]string{key: "true"}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptions
		common.OptionMapRWMutex.Unlock()
	})

	require.Error(t, UpdateOption(key, "not-a-bool"))
	var persisted int64
	require.NoError(t, db.Model(&Option{}).Count(&persisted).Error)
	assert.Zero(t, persisted)
	afterConfig, err := config.ConfigToMap(claudeConfig)
	require.NoError(t, err)
	assert.Equal(t, beforeConfig, afterConfig)
	common.OptionMapRWMutex.RLock()
	assert.Equal(t, "true", common.OptionMap[key])
	common.OptionMapRWMutex.RUnlock()
}

func TestModelSemanticConfigValidationRejectsInvalidValuesBeforePersistence(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		configName string
		key        string
		value      string
	}{
		{name: "Gemini safety", configName: "gemini", key: "gemini.safety_settings", value: `{"default":"BLOCK_SOME"}`},
		{name: "Gemini budget", configName: "gemini", key: "gemini.thinking_adapter_budget_tokens_percentage", value: "0.001"},
		{name: "Claude max tokens", configName: "claude", key: "claude.default_max_tokens", value: `{"default":2147483647}`},
		{name: "Claude budget", configName: "claude", key: "claude.thinking_adapter_budget_tokens_percentage", value: "0"},
		{name: "Claude budget whitespace", configName: "claude", key: "claude.thinking_adapter_budget_tokens_percentage", value: " 0.5 "},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			db := accessProfileTestDB(t)
			require.NoError(t, db.AutoMigrate(&Option{}))
			registeredConfig := config.GlobalConfig.Get(testCase.configName)
			require.NotNil(t, registeredConfig)
			beforeConfig, err := config.ConfigToMap(registeredConfig)
			require.NoError(t, err)

			common.OptionMapRWMutex.Lock()
			previousOptions := common.OptionMap
			common.OptionMap = map[string]string{testCase.key: "unchanged"}
			common.OptionMapRWMutex.Unlock()
			t.Cleanup(func() {
				common.OptionMapRWMutex.Lock()
				common.OptionMap = previousOptions
				common.OptionMapRWMutex.Unlock()
			})

			require.Error(t, UpdateOption(testCase.key, testCase.value))
			var persisted int64
			require.NoError(t, db.Model(&Option{}).Count(&persisted).Error)
			assert.Zero(t, persisted)
			afterConfig, err := config.ConfigToMap(registeredConfig)
			require.NoError(t, err)
			assert.Equal(t, beforeConfig, afterConfig)
			common.OptionMapRWMutex.RLock()
			assert.Equal(t, "unchanged", common.OptionMap[testCase.key])
			common.OptionMapRWMutex.RUnlock()
		})
	}
}

func TestMalformedD07OptionsDoNotReachDatabaseOrRuntime(t *testing.T) {
	db := accessProfileTestDB(t)
	require.NoError(t, db.AutoMigrate(&Option{}))

	previousRateLimit := setting.GetModelRequestRateLimitConfig()
	baselineRateLimit := setting.ModelRequestRateLimitConfig{
		Enabled:         true,
		DurationMinutes: 4,
		Total:           40,
		Success:         30,
		Group:           map[string][2]int{"vip": {20, 10}},
	}
	require.NoError(t, setting.ApplyModelRequestRateLimitConfig(baselineRateLimit))
	previousImageRatio := ratio_setting.ImageRatio2JSONString()
	const baselineImageRatio = `{"baseline-image":2}`
	require.NoError(t, ratio_setting.UpdateImageRatioByJSONString(baselineImageRatio))
	common.OptionMapRWMutex.Lock()
	previousOptions := common.OptionMap
	common.OptionMap = map[string]string{
		"ImageRatio":                           baselineImageRatio,
		"ModelRequestRateLimitCount":           "40",
		"ModelRequestRateLimitDurationMinutes": "4",
		"ModelRequestRateLimitSuccessCount":    "30",
		"ModelRequestRateLimitGroup":           `{"vip":[20,10]}`,
		"ModelRequestRateLimitEnabled":         "true",
	}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		require.NoError(t, setting.ApplyModelRequestRateLimitConfig(previousRateLimit))
		require.NoError(t, ratio_setting.UpdateImageRatioByJSONString(previousImageRatio))
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptions
		common.OptionMapRWMutex.Unlock()
	})

	require.Error(t, UpdateOption("ImageRatio", `{"new-image":3,"wrong":"not-a-number"}`))
	require.Error(t, UpdateOptionsBulk(map[string]string{
		"ModelRequestRateLimitCount": "25",
		"ModelRequestRateLimitGroup": `{"new":[12,8],"wrong":"not-a-pair"}`,
	}))

	var persisted int64
	require.NoError(t, db.Model(&Option{}).Count(&persisted).Error)
	assert.Zero(t, persisted)
	assert.Equal(t, baselineRateLimit, setting.GetModelRequestRateLimitConfig())
	assert.JSONEq(t, baselineImageRatio, ratio_setting.ImageRatio2JSONString())
	common.OptionMapRWMutex.RLock()
	assert.Equal(t, baselineImageRatio, common.OptionMap["ImageRatio"])
	assert.Equal(t, "40", common.OptionMap["ModelRequestRateLimitCount"])
	assert.Equal(t, `{"vip":[20,10]}`, common.OptionMap["ModelRequestRateLimitGroup"])
	common.OptionMapRWMutex.RUnlock()
}

func TestUpdateOptionsBulkPublishesRateLimitConfigOnceComplete(t *testing.T) {
	db := accessProfileTestDB(t)
	require.NoError(t, db.AutoMigrate(&Option{}))
	previousRateLimit := setting.GetModelRequestRateLimitConfig()
	common.OptionMapRWMutex.Lock()
	previousOptions := common.OptionMap
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		require.NoError(t, setting.ApplyModelRequestRateLimitConfig(previousRateLimit))
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptions
		common.OptionMapRWMutex.Unlock()
	})

	values := map[string]string{
		"ModelRequestRateLimitEnabled":         "true",
		"ModelRequestRateLimitDurationMinutes": "6",
		"ModelRequestRateLimitCount":           "60",
		"ModelRequestRateLimitSuccessCount":    "50",
		"ModelRequestRateLimitGroup":           `{"vip":[30,20]}`,
	}
	require.NoError(t, UpdateOptionsBulk(values))
	assert.Equal(t, setting.ModelRequestRateLimitConfig{
		Enabled:         true,
		DurationMinutes: 6,
		Total:           60,
		Success:         50,
		Group:           map[string][2]int{"vip": {30, 20}},
	}, setting.GetModelRequestRateLimitConfig())
	common.OptionMapRWMutex.RLock()
	for key, value := range values {
		assert.Equal(t, value, common.OptionMap[key])
	}
	common.OptionMapRWMutex.RUnlock()
	var persisted int64
	require.NoError(t, db.Model(&Option{}).Count(&persisted).Error)
	assert.Equal(t, int64(len(values)), persisted)
}

func TestUpdateOptionsBulkRejectsCombinedRateLimitOverflowBeforePersistence(t *testing.T) {
	db := accessProfileTestDB(t)
	require.NoError(t, db.AutoMigrate(&Option{}))
	previousRateLimit := setting.GetModelRequestRateLimitConfig()
	baseline := setting.ModelRequestRateLimitConfig{
		DurationMinutes: 1,
		Total:           1,
		Success:         1,
		Group:           map[string][2]int{},
	}
	require.NoError(t, setting.ApplyModelRequestRateLimitConfig(baseline))
	common.OptionMapRWMutex.Lock()
	previousOptions := common.OptionMap
	common.OptionMap = map[string]string{
		"ModelRequestRateLimitDurationMinutes": "1",
		"ModelRequestRateLimitCount":           "1",
	}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		require.NoError(t, setting.ApplyModelRequestRateLimitConfig(previousRateLimit))
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptions
		common.OptionMapRWMutex.Unlock()
	})

	maxDurationMinutes := int64(math.MaxInt64) / int64(time.Minute)
	err := UpdateOptionsBulk(map[string]string{
		"ModelRequestRateLimitDurationMinutes": strconv.FormatInt(maxDurationMinutes, 10),
		"ModelRequestRateLimitCount":           strconv.Itoa(math.MaxInt32),
	})
	require.Error(t, err)

	var persisted int64
	require.NoError(t, db.Model(&Option{}).Count(&persisted).Error)
	assert.Zero(t, persisted)
	assert.Equal(t, baseline, setting.GetModelRequestRateLimitConfig())
	common.OptionMapRWMutex.RLock()
	assert.Equal(t, "1", common.OptionMap["ModelRequestRateLimitDurationMinutes"])
	assert.Equal(t, "1", common.OptionMap["ModelRequestRateLimitCount"])
	common.OptionMapRWMutex.RUnlock()
}

func TestLoadOptionsRejectsCombinedRateLimitOverflowAsOneSnapshot(t *testing.T) {
	db := accessProfileTestDB(t)
	require.NoError(t, db.AutoMigrate(&Option{}))
	previousRateLimit := setting.GetModelRequestRateLimitConfig()
	baseline := setting.ModelRequestRateLimitConfig{
		DurationMinutes: 1,
		Total:           1,
		Success:         1,
		Group:           map[string][2]int{},
	}
	require.NoError(t, setting.ApplyModelRequestRateLimitConfig(baseline))
	common.OptionMapRWMutex.Lock()
	previousOptions := common.OptionMap
	common.OptionMap = map[string]string{
		"ModelRequestRateLimitDurationMinutes": "1",
		"ModelRequestRateLimitCount":           "1",
	}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		require.NoError(t, setting.ApplyModelRequestRateLimitConfig(previousRateLimit))
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptions
		common.OptionMapRWMutex.Unlock()
	})

	maxDurationMinutes := int64(math.MaxInt64) / int64(time.Minute)
	require.NoError(t, db.Create(&[]Option{
		{Key: "ModelRequestRateLimitDurationMinutes", Value: strconv.FormatInt(maxDurationMinutes, 10)},
		{Key: "ModelRequestRateLimitCount", Value: strconv.Itoa(math.MaxInt32)},
	}).Error)
	loadOptionsFromDatabase()

	assert.Equal(t, baseline, setting.GetModelRequestRateLimitConfig())
	common.OptionMapRWMutex.RLock()
	assert.Equal(t, "1", common.OptionMap["ModelRequestRateLimitDurationMinutes"])
	assert.Equal(t, "1", common.OptionMap["ModelRequestRateLimitCount"])
	common.OptionMapRWMutex.RUnlock()
}

func TestRateLimitPersistenceFailureDoesNotPublishPreparedConfig(t *testing.T) {
	db := accessProfileTestDB(t)
	require.NoError(t, db.AutoMigrate(&Option{}))
	previousRateLimit := setting.GetModelRequestRateLimitConfig()
	baseline := setting.ModelRequestRateLimitConfig{
		DurationMinutes: 2,
		Total:           20,
		Success:         10,
		Group:           map[string][2]int{"vip": {8, 6}},
	}
	require.NoError(t, setting.ApplyModelRequestRateLimitConfig(baseline))
	common.OptionMapRWMutex.Lock()
	previousOptions := common.OptionMap
	common.OptionMap = map[string]string{
		"ModelRequestRateLimitCount": "20",
		"ModelRequestRateLimitGroup": `{"vip":[8,6]}`,
	}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		require.NoError(t, setting.ApplyModelRequestRateLimitConfig(previousRateLimit))
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptions
		common.OptionMapRWMutex.Unlock()
	})
	writeErr := errors.New("injected rate limit persistence failure")
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:fail-rate-limit-option-create", func(tx *gorm.DB) {
		tx.AddError(writeErr)
	}))

	err := UpdateOptionsBulk(map[string]string{
		"ModelRequestRateLimitCount": "30",
		"ModelRequestRateLimitGroup": `{"vip":[12,9]}`,
	})
	require.ErrorIs(t, err, writeErr)
	assert.Equal(t, baseline, setting.GetModelRequestRateLimitConfig())
	common.OptionMapRWMutex.RLock()
	assert.Equal(t, "20", common.OptionMap["ModelRequestRateLimitCount"])
	assert.Equal(t, `{"vip":[8,6]}`, common.OptionMap["ModelRequestRateLimitGroup"])
	common.OptionMapRWMutex.RUnlock()
	var persisted int64
	require.NoError(t, db.Model(&Option{}).Count(&persisted).Error)
	assert.Zero(t, persisted)
}
