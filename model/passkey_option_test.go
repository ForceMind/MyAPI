package model

import (
	"errors"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/ForceMind/MyAPI/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

var passkeyOptionBaseline = map[string]string{
	"enabled":               "false",
	"rp_display_name":       "Before",
	"rp_id":                 "",
	"origins":               "",
	"allow_insecure_origin": "false",
	"user_verification":     "preferred",
	"attachment_preference": "",
}

func preparePasskeyOptionTest(t *testing.T) (*gorm.DB, interface{}) {
	t.Helper()
	db := accessProfileTestDB(t)
	require.NoError(t, db.AutoMigrate(&Option{}))
	passkeyConfig := config.GlobalConfig.Get("passkey")
	require.NotNil(t, passkeyConfig)
	previousSettings, err := config.ConfigToMap(passkeyConfig)
	require.NoError(t, err)
	previousServerAddress := system_setting.GetServerAddress()
	const testServerAddress = "https://before.example.test"
	system_setting.SetServerAddress(testServerAddress)
	require.NoError(t, config.UpdateConfigFromMap(passkeyConfig, passkeyOptionBaseline))

	common.OptionMapRWMutex.Lock()
	previousOptions := common.OptionMap
	common.OptionMap = make(map[string]string, len(passkeyOptionBaseline))
	for key, value := range passkeyOptionBaseline {
		common.OptionMap[passkeyOptionPrefix+key] = value
	}
	common.OptionMap[serverAddressOptionKey] = testServerAddress
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		require.NoError(t, config.UpdateConfigFromMap(passkeyConfig, previousSettings))
		system_setting.SetServerAddress(previousServerAddress)
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptions
		common.OptionMapRWMutex.Unlock()
	})
	return db, passkeyConfig
}

func TestUpdateOptionsBulkPublishesPasskeyAndServerAddressAsOneGroup(t *testing.T) {
	db, passkeyConfig := preparePasskeyOptionTest(t)
	passkeyValues := map[string]string{
		"passkey.enabled":               "true",
		"passkey.rp_display_name":       "Updated RP",
		"passkey.rp_id":                 "passkey.example.test",
		"passkey.origins":               "https://passkey.example.test",
		"passkey.allow_insecure_origin": "false",
		"passkey.user_verification":     "required",
		"passkey.attachment_preference": "platform",
	}
	values := make(map[string]string, len(passkeyValues)+1)
	for key, value := range passkeyValues {
		values[key] = value
	}
	values[serverAddressOptionKey] = "https://dashboard.example.test/"

	require.NoError(t, UpdateOptionsBulk(values))
	exported, err := config.ConfigToMap(passkeyConfig)
	require.NoError(t, err)
	for key, value := range passkeyValues {
		configKey, ok := passkeyOptionConfigKey(key)
		require.True(t, ok)
		assert.Equal(t, value, exported[configKey])
	}
	assert.Equal(t, values[serverAddressOptionKey], system_setting.GetServerAddress())
	assert.Equal(t, "passkey.example.test", system_setting.GetPasskeySettings().RPID)

	common.OptionMapRWMutex.RLock()
	for key, value := range values {
		assert.Equal(t, value, common.OptionMap[key])
	}
	common.OptionMapRWMutex.RUnlock()

	var stored []Option
	require.NoError(t, db.Order("key").Find(&stored).Error)
	require.Len(t, stored, len(values))
	for _, option := range stored {
		assert.Equal(t, values[option.Key], option.Value)
	}
}

func TestUpdateOptionsBulkRejectsInvalidPasskeyGroupBeforePersistence(t *testing.T) {
	db, passkeyConfig := preparePasskeyOptionTest(t)
	before, err := config.ConfigToMap(passkeyConfig)
	require.NoError(t, err)
	beforeServerAddress := system_setting.GetServerAddress()

	err = UpdateOptionsBulk(map[string]string{
		serverAddressOptionKey:    "https://must-not-publish.example.test",
		"passkey.enabled":         "not-a-bool",
		"passkey.rp_display_name": "Must not publish",
	})
	require.Error(t, err)

	var count int64
	require.NoError(t, db.Model(&Option{}).Count(&count).Error)
	assert.Zero(t, count)
	after, err := config.ConfigToMap(passkeyConfig)
	require.NoError(t, err)
	assert.Equal(t, before, after)
	assert.Equal(t, beforeServerAddress, system_setting.GetServerAddress())
	common.OptionMapRWMutex.RLock()
	assert.Equal(t, "false", common.OptionMap["passkey.enabled"])
	assert.Equal(t, "Before", common.OptionMap["passkey.rp_display_name"])
	common.OptionMapRWMutex.RUnlock()
}

func TestPasskeyOptionPersistenceFailureDoesNotPublish(t *testing.T) {
	db, passkeyConfig := preparePasskeyOptionTest(t)
	before, err := config.ConfigToMap(passkeyConfig)
	require.NoError(t, err)
	beforeServerAddress := system_setting.GetServerAddress()
	writeErr := errors.New("injected passkey option write failure")
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:fail-passkey-option-create", func(tx *gorm.DB) {
		tx.AddError(writeErr)
	}))

	err = UpdateOptionsBulk(map[string]string{
		serverAddressOptionKey:    "https://must-not-publish.example.test",
		"passkey.enabled":         "true",
		"passkey.rp_display_name": "Must not publish",
	})
	require.ErrorIs(t, err, writeErr)

	after, err := config.ConfigToMap(passkeyConfig)
	require.NoError(t, err)
	assert.Equal(t, before, after)
	assert.Equal(t, beforeServerAddress, system_setting.GetServerAddress())
	common.OptionMapRWMutex.RLock()
	assert.Equal(t, "false", common.OptionMap["passkey.enabled"])
	assert.Equal(t, "Before", common.OptionMap["passkey.rp_display_name"])
	common.OptionMapRWMutex.RUnlock()
}

func TestLoadOptionsKeepsPasskeyRuntimeWhenGroupIsInvalid(t *testing.T) {
	db, passkeyConfig := preparePasskeyOptionTest(t)
	before, err := config.ConfigToMap(passkeyConfig)
	require.NoError(t, err)
	beforeServerAddress := system_setting.GetServerAddress()
	require.NoError(t, db.Create(&[]Option{
		{Key: serverAddressOptionKey, Value: "https://must-not-publish.example.test"},
		{Key: "passkey.rp_display_name", Value: "Must not publish"},
		{Key: "passkey.enabled", Value: "not-a-bool"},
	}).Error)

	loadOptionsFromDatabase()

	after, err := config.ConfigToMap(passkeyConfig)
	require.NoError(t, err)
	assert.Equal(t, before, after)
	assert.Equal(t, beforeServerAddress, system_setting.GetServerAddress())
	common.OptionMapRWMutex.RLock()
	assert.Equal(t, beforeServerAddress, common.OptionMap[serverAddressOptionKey])
	assert.Equal(t, "false", common.OptionMap["passkey.enabled"])
	assert.Equal(t, "Before", common.OptionMap["passkey.rp_display_name"])
	common.OptionMapRWMutex.RUnlock()
}

func TestLoadOptionsPublishesPasskeyAndServerAddressAsOneGroup(t *testing.T) {
	db, passkeyConfig := preparePasskeyOptionTest(t)
	values := map[string]string{
		serverAddressOptionKey:          "https://dashboard.example.test/",
		"passkey.enabled":               "true",
		"passkey.rp_display_name":       "Updated RP",
		"passkey.rp_id":                 "passkey.example.test",
		"passkey.origins":               "https://passkey.example.test",
		"passkey.allow_insecure_origin": "false",
		"passkey.user_verification":     "required",
		"passkey.attachment_preference": "platform",
	}
	options := make([]Option, 0, len(values))
	for key, value := range values {
		options = append(options, Option{Key: key, Value: value})
	}
	require.NoError(t, db.Create(&options).Error)

	loadOptionsFromDatabase()

	exported, err := config.ConfigToMap(passkeyConfig)
	require.NoError(t, err)
	assert.Equal(t, values[serverAddressOptionKey], system_setting.GetServerAddress())
	assert.Equal(t, "passkey.example.test", exported["rp_id"])
	assert.Equal(t, "https://passkey.example.test", exported["origins"])
	common.OptionMapRWMutex.RLock()
	for key, value := range values {
		assert.Equal(t, value, common.OptionMap[key])
	}
	common.OptionMapRWMutex.RUnlock()
}
