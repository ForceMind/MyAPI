package model

import (
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestChannelQuotaAlertSettingsOptionPersistsAndAppliesAtomically(t *testing.T) {
	previousDB := DB
	previousMap := common.OptionMap
	previousEnabled := common.ChannelQuotaAlertEnabled
	previousWarning := common.ChannelQuotaAlertWarningPercent
	previousCritical := common.ChannelQuotaAlertCriticalPercent
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Option{}))
	DB = db
	common.OptionMap = map[string]string{}
	t.Cleanup(func() {
		DB = previousDB
		common.OptionMap = previousMap
		common.ChannelQuotaAlertEnabled = previousEnabled
		common.ChannelQuotaAlertWarningPercent = previousWarning
		common.ChannelQuotaAlertCriticalPercent = previousCritical
	})

	raw := `{"enabled":true,"warning_percent":25.5,"critical_percent":5}`
	require.NoError(t, UpdateOption(common.ChannelQuotaAlertSettingsOptionKey, raw))
	var saved Option
	require.NoError(t, db.First(&saved, "key = ?", common.ChannelQuotaAlertSettingsOptionKey).Error)
	require.JSONEq(t, raw, saved.Value)
	require.Equal(t, raw, common.OptionMap[common.ChannelQuotaAlertSettingsOptionKey])
	require.True(t, common.ChannelQuotaAlertEnabled)
	require.Equal(t, 25.5, common.ChannelQuotaAlertWarningPercent)
	require.Equal(t, 5.0, common.ChannelQuotaAlertCriticalPercent)
}

func TestChannelQuotaAlertSettingsOptionRejectsInvalidBeforeDatabaseWrite(t *testing.T) {
	previousDB := DB
	previousMap := common.OptionMap
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Option{}))
	DB = db
	common.OptionMap = map[string]string{}
	t.Cleanup(func() {
		DB = previousDB
		common.OptionMap = previousMap
	})

	err = UpdateOption(common.ChannelQuotaAlertSettingsOptionKey, `{"enabled":true,"warning_percent":10,"critical_percent":10}`)
	require.Error(t, err)
	var count int64
	require.NoError(t, db.Model(&Option{}).Where("key = ?", common.ChannelQuotaAlertSettingsOptionKey).Count(&count).Error)
	require.Zero(t, count)
}
