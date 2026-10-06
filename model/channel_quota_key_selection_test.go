package model

import (
	"path/filepath"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestCodexQuotaKeySelectionSkipsOnlyExcludedCredentials(t *testing.T) {
	single := &Channel{Key: "single-oauth-key"}
	key, index, apiErr := single.GetNextEnabledKeyExcluding(map[int]bool{0: true})
	require.NotNil(t, apiErr)
	assert.Empty(t, key)
	assert.Zero(t, index)
	key, index, apiErr = single.GetNextEnabledKeyExcluding(nil)
	require.Nil(t, apiErr)
	assert.Equal(t, "single-oauth-key", key)
	assert.Zero(t, index)

	multi := &Channel{Key: "key-a\nkey-b\nkey-c", ChannelInfo: ChannelInfo{
		IsMultiKey: true, MultiKeyMode: constant.MultiKeyModeRandom,
		MultiKeyStatusList: map[int]int{2: common.ChannelStatusAutoDisabled},
	}}
	key, index, apiErr = multi.GetNextEnabledKeyExcluding(map[int]bool{0: true})
	require.Nil(t, apiErr)
	assert.Equal(t, "key-b", key)
	assert.Equal(t, 1, index)
	key, _, apiErr = multi.GetNextEnabledKeyExcluding(map[int]bool{0: true, 1: true})
	require.NotNil(t, apiErr)
	assert.Empty(t, key)
}

func TestCodexQuotaPollingNeverSelectsAnExcludedCredential(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "quota-polling.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	previousDB, previousCache := DB, common.MemoryCacheEnabled
	DB, common.MemoryCacheEnabled = db, false
	t.Cleanup(func() {
		DB, common.MemoryCacheEnabled = previousDB, previousCache
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.AutoMigrate(&Channel{}))
	channel := &Channel{
		Name: "quota-polling", Key: "account-a\naccount-b", Status: common.ChannelStatusEnabled,
		ChannelInfo: ChannelInfo{IsMultiKey: true, MultiKeySize: 2, MultiKeyMode: constant.MultiKeyModePolling},
	}
	require.NoError(t, db.Create(channel).Error)

	key, index, apiErr := channel.GetNextEnabledKeyExcluding(map[int]bool{0: true})
	require.Nil(t, apiErr)
	assert.Equal(t, "account-b", key)
	assert.Equal(t, 1, index)
	key, index, apiErr = channel.GetNextEnabledKeyExcluding(map[int]bool{1: true})
	require.Nil(t, apiErr)
	assert.Equal(t, "account-a", key)
	assert.Zero(t, index)
	key, _, apiErr = channel.GetNextEnabledKeyExcluding(map[int]bool{0: true, 1: true})
	require.NotNil(t, apiErr)
	assert.Empty(t, key)
}
