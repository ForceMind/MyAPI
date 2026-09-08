package model

import (
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestChannelRoutingValuesCanNormalizeLegacyOversizedRows(t *testing.T) {
	previousDB := DB
	previousMemory := common.MemoryCacheEnabled
	db, err := gorm.Open(sqlite.Open("file:channel-routing-values?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.Migrator().DropTable(&Ability{}, &Channel{}))
	require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}))
	DB = db
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		DB = previousDB
		common.MemoryCacheEnabled = previousMemory
		sqlDB, sqlErr := db.DB()
		require.NoError(t, sqlErr)
		require.NoError(t, sqlDB.Close())
	})

	legacyPriority := MaxChannelRoutingPriority + 42
	legacyWeight := MaxChannelRoutingWeight + 42
	channel := Channel{
		Type: 1, Key: "test-key", Status: common.ChannelStatusEnabled, Name: "legacy",
		Models: "routing-model", Group: "default", Priority: &legacyPriority, Weight: &legacyWeight,
	}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, channel.AddAbilities(nil))
	boundedPriority := MaxChannelRoutingPriority
	boundedWeight := uint(1)
	peer := Channel{
		Type: 1, Key: "peer-key", Status: common.ChannelStatusEnabled, Name: "bounded-peer",
		Models: "routing-model", Group: "default", Priority: &boundedPriority, Weight: &boundedWeight,
	}
	require.NoError(t, db.Create(&peer).Error)
	require.NoError(t, peer.AddAbilities(nil))

	management, err := ListChannelRoutingManagementChannels()
	require.NoError(t, err)
	require.Len(t, management, 2)
	assert.Equal(t, MaxChannelRoutingPriority, management[0].Priority)
	assert.Equal(t, MaxChannelRoutingWeight, management[0].Weight)
	assert.Equal(t, "1000042", management[0].LegacyPriority)
	assert.Equal(t, "1000042", management[0].LegacyWeight)
	candidates, err := ListEligibleChannelRoutingCandidates("default", "routing-model", "/v1/responses")
	require.NoError(t, err)
	require.Len(t, candidates, 2)
	assert.Equal(t, legacyPriority, candidates[0].Priority)
	assert.Equal(t, MaxChannelRoutingPriority, candidates[1].Priority)

	_, err = UpdateChannelRoutingValuesCAS(channel.Id, 12, 7, MaxChannelRoutingPriority, MaxChannelRoutingWeight)
	require.ErrorIs(t, err, ErrChannelRoutingConflict, "the bounded display value alone must not bypass raw legacy CAS")
	concurrentPriority := legacyPriority + 1
	concurrentWeight := legacyWeight + 1
	require.NoError(t, db.Model(&Channel{}).Where("id = ?", channel.Id).Updates(map[string]interface{}{
		"priority": concurrentPriority, "weight": concurrentWeight,
	}).Error)
	_, err = UpdateChannelRoutingValuesCAS(channel.Id, 12, 7, legacyPriority, concurrentWeight)
	require.ErrorIs(t, err, ErrChannelRoutingConflict, "an exact stale legacy token must detect a concurrent raw value change")
	_, err = UpdateChannelRoutingValuesCAS(channel.Id, 12, 7, concurrentPriority, legacyWeight)
	require.ErrorIs(t, err, ErrChannelRoutingConflict, "an exact stale legacy weight token must detect a concurrent raw value change")
	updated, err := UpdateChannelRoutingValuesCAS(channel.Id, 12, 7, concurrentPriority, concurrentWeight)
	require.NoError(t, err)
	assert.Equal(t, int64(12), updated.GetPriority())
	assert.Equal(t, 7, updated.GetWeight())

	var stored Channel
	require.NoError(t, db.First(&stored, channel.Id).Error)
	assert.Equal(t, int64(12), stored.GetPriority())
	assert.Equal(t, 7, stored.GetWeight())
}
