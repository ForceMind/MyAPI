package model

import (
	"errors"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupChannelStatusTest(t *testing.T) {
	t.Helper()
	truncateTables(t)
	require.NoError(t, DB.Exec("DELETE FROM abilities").Error)
	require.NoError(t, DB.Exec("DELETE FROM channels").Error)

	memoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		common.MemoryCacheEnabled = memoryCacheEnabled
	})
}

func TestUpdateChannelStatusPersistsMultiKeyState(t *testing.T) {
	setupChannelStatusTest(t)

	channel := Channel{
		Name:   "multi-key-status",
		Key:    "key-a\nkey-b",
		Status: common.ChannelStatusEnabled,
		ChannelInfo: ChannelInfo{
			IsMultiKey:           true,
			MultiKeySize:         2,
			MultiKeyMode:         constant.MultiKeyModePolling,
			MultiKeyPollingIndex: 1,
		},
	}
	require.NoError(t, DB.Create(&channel).Error)

	changed := UpdateChannelStatus(channel.Id, "key-a", common.ChannelStatusAutoDisabled, "provider rejected key")
	require.True(t, changed)

	var stored Channel
	require.NoError(t, DB.First(&stored, channel.Id).Error)
	assert.Equal(t, common.ChannelStatusEnabled, stored.Status)
	assert.Equal(t, common.ChannelStatusAutoDisabled, stored.ChannelInfo.MultiKeyStatusList[0])
	assert.Equal(t, "provider rejected key", stored.ChannelInfo.MultiKeyDisabledReason[0])
	assert.NotZero(t, stored.ChannelInfo.MultiKeyDisabledTime[0])
	assert.Equal(t, 1, stored.ChannelInfo.MultiKeyPollingIndex)
}

func TestSaveStatusStateFromSingleKeySnapshotPreservesUnownedColumns(t *testing.T) {
	setupChannelStatusTest(t)

	channel := Channel{
		Name:        "single-key-status",
		Key:         "original-key",
		Status:      common.ChannelStatusEnabled,
		Models:      "original-model",
		Group:       "default",
		UsedQuota:   100,
		ChannelInfo: ChannelInfo{},
	}
	require.NoError(t, DB.Create(&channel).Error)

	stale, err := GetChannelById(channel.Id, true)
	require.NoError(t, err)

	concurrentChannelInfo := ChannelInfo{
		IsMultiKey:           true,
		MultiKeySize:         2,
		MultiKeyMode:         constant.MultiKeyModePolling,
		MultiKeyPollingIndex: 1,
	}
	require.NoError(t, DB.Model(&Channel{}).Where("id = ?", channel.Id).Updates(map[string]any{
		"key":          "rotated-key",
		"used_quota":   gorm.Expr("used_quota + ?", 250),
		"models":       "concurrent-model",
		"channel_info": concurrentChannelInfo,
	}).Error)

	stale.Status = common.ChannelStatusManuallyDisabled
	stale.SetOtherInfo(map[string]interface{}{
		"status_reason": "manual operation",
		"status_time":   int64(1234),
	})
	require.NoError(t, stale.saveStatusState())

	var stored Channel
	require.NoError(t, DB.First(&stored, channel.Id).Error)
	assert.Equal(t, common.ChannelStatusManuallyDisabled, stored.Status)
	assert.Equal(t, "rotated-key", stored.Key)
	assert.Equal(t, int64(350), stored.UsedQuota)
	assert.Equal(t, "concurrent-model", stored.Models)
	assert.Equal(t, concurrentChannelInfo, stored.ChannelInfo)

	otherInfo := stored.GetOtherInfo()
	assert.Equal(t, "manual operation", otherInfo["status_reason"])
	assert.Equal(t, float64(1234), otherInfo["status_time"])
}

func setupChannelStatusCacheTest(t *testing.T) {
	t.Helper()
	truncateTables(t)
	previousMemoryCache := common.MemoryCacheEnabled
	channelSyncLock.Lock()
	previousChannels := channelsIDM
	previousGroups := group2model2channels
	previousCandidates := group2model2routingCandidates
	previousConfigs := channel2advancedCustomConfig
	previousDataGeneration := channelCacheDataGeneration
	previousPublishGeneration := channelCachePublishGeneration
	previousObservedEpoch := channelCacheObservedCommittedEpoch
	previousPublishedEpoch := channelCachePublishedEpoch
	channelCacheObservedCommittedEpoch = 0
	channelCachePublishedEpoch = 0
	channelCacheDataGeneration = 0
	channelCachePublishGeneration = 0
	channelSyncLock.Unlock()
	common.MemoryCacheEnabled = true
	t.Cleanup(func() {
		channelSyncLock.Lock()
		channelsIDM = previousChannels
		group2model2channels = previousGroups
		group2model2routingCandidates = previousCandidates
		channel2advancedCustomConfig = previousConfigs
		channelCacheDataGeneration = previousDataGeneration
		channelCachePublishGeneration = previousPublishGeneration
		channelCacheObservedCommittedEpoch = previousObservedEpoch
		channelCachePublishedEpoch = previousPublishedEpoch
		channelSyncLock.Unlock()
		common.MemoryCacheEnabled = previousMemoryCache
	})
	require.NoError(t, InitChannelCache())
}

func insertChannelStatusRoutingFixture(t *testing.T, id int, name string) *Channel {
	t.Helper()
	priority := int64(10)
	weight := uint(2)
	channel := &Channel{
		Id: id, Type: constant.ChannelTypeOpenAI, Key: "key-" + name,
		Status: common.ChannelStatusEnabled, Name: name,
		Group: "default", Models: "status-model", Priority: &priority, Weight: &weight,
	}
	require.NoError(t, DB.Create(channel).Error)
	require.NoError(t, DB.Create(&Ability{
		Group: "default", Model: "status-model", ChannelId: channel.Id,
		Enabled: true, Priority: &priority, Weight: weight,
	}).Error)
	return channel
}

func TestUpdateChannelStatusRoundTripCommitsAbilityThenRebuildsCache(t *testing.T) {
	setupChannelStatusCacheTest(t)
	channel := insertChannelStatusRoutingFixture(t, 401, "round-trip")
	require.NoError(t, InitChannelCache())

	changed, err := UpdateChannelStatusWithError(channel.Id, "", common.ChannelStatusManuallyDisabled, "manual")
	require.NoError(t, err)
	require.True(t, changed)
	var disabledChannel Channel
	require.NoError(t, DB.First(&disabledChannel, channel.Id).Error)
	assert.Equal(t, common.ChannelStatusManuallyDisabled, disabledChannel.Status)
	var disabledAbility Ability
	require.NoError(t, DB.First(&disabledAbility, "channel_id = ?", channel.Id).Error)
	assert.False(t, disabledAbility.Enabled)
	assert.Empty(t, routingPolicyFromCacheForTest("default", "status-model", "").Tiers)

	changed, err = UpdateChannelStatusWithError(channel.Id, "", common.ChannelStatusEnabled, "manual")
	require.NoError(t, err)
	require.True(t, changed)
	var enabledChannel Channel
	require.NoError(t, DB.First(&enabledChannel, channel.Id).Error)
	assert.Equal(t, common.ChannelStatusEnabled, enabledChannel.Status)
	var enabledAbility Ability
	require.NoError(t, DB.First(&enabledAbility, "channel_id = ?", channel.Id).Error)
	assert.True(t, enabledAbility.Enabled)
	policy := routingPolicyFromCacheForTest("default", "status-model", "")
	require.Len(t, policy.Tiers, 1)
	require.Len(t, policy.Tiers[0].Candidates, 1)
	assert.Equal(t, channel.Id, policy.Tiers[0].Candidates[0].ChannelID)
	channelSyncLock.RLock()
	assert.Equal(t, []int{channel.Id}, group2model2channels["default"]["status-model"])
	channelSyncLock.RUnlock()
}

func TestUpdateChannelStatusWriteFailureRollsBackWithoutChangingCache(t *testing.T) {
	setupChannelStatusCacheTest(t)
	channel := insertChannelStatusRoutingFixture(t, 402, "rollback")
	require.NoError(t, InitChannelCache())
	beforePolicy := routingPolicyFromCacheForTest("default", "status-model", "")

	forcedError := errors.New("forced ability status failure")
	require.NoError(t, DB.Callback().Update().Before("gorm:update").Register("test:fail_ability_status_update", func(tx *gorm.DB) {
		if tx.Statement.Table == "abilities" {
			tx.AddError(forcedError)
		}
	}))
	t.Cleanup(func() {
		require.NoError(t, DB.Callback().Update().Remove("test:fail_ability_status_update"))
	})

	changed, err := UpdateChannelStatusWithError(channel.Id, "", common.ChannelStatusManuallyDisabled, "manual")
	require.ErrorIs(t, err, forcedError)
	assert.False(t, changed)
	var storedChannel Channel
	require.NoError(t, DB.First(&storedChannel, channel.Id).Error)
	assert.Equal(t, common.ChannelStatusEnabled, storedChannel.Status)
	var storedAbility Ability
	require.NoError(t, DB.First(&storedAbility, "channel_id = ?", channel.Id).Error)
	assert.True(t, storedAbility.Enabled)
	assert.Equal(t, beforePolicy, routingPolicyFromCacheForTest("default", "status-model", ""))
}

func TestBatchChannelStatusFailureRollsBackAllChannelsAndCache(t *testing.T) {
	setupChannelStatusCacheTest(t)
	first := insertChannelStatusRoutingFixture(t, 403, "batch-first")
	insertChannelStatusRoutingFixture(t, 404, "batch-second")
	require.NoError(t, InitChannelCache())
	beforePolicy := routingPolicyFromCacheForTest("default", "status-model", "")

	changedCount, err := UpdateChannelStatusesWithError(
		[]int{first.Id, 999999},
		common.ChannelStatusManuallyDisabled,
		"manual batch",
	)
	require.Error(t, err)
	assert.Zero(t, changedCount)
	var stored Channel
	require.NoError(t, DB.First(&stored, first.Id).Error)
	assert.Equal(t, common.ChannelStatusEnabled, stored.Status)
	var ability Ability
	require.NoError(t, DB.First(&ability, "channel_id = ?", first.Id).Error)
	assert.True(t, ability.Enabled)
	assert.Equal(t, beforePolicy, routingPolicyFromCacheForTest("default", "status-model", ""))
}

func TestChannelUpdateSynchronizesRoutingFieldsAtomically(t *testing.T) {
	setupChannelStatusCacheTest(t)
	channel := insertChannelStatusRoutingFixture(t, 405, "field-sync")
	require.NoError(t, InitChannelCache())

	newPriority := int64(25)
	newWeight := uint(7)
	channel.Group = "vip"
	channel.Models = "new-model"
	channel.Priority = &newPriority
	channel.Weight = &newWeight
	require.NoError(t, channel.Update())
	require.NoError(t, InitChannelCache())

	assert.Empty(t, routingPolicyFromCacheForTest("default", "status-model", "").Tiers)
	policy := routingPolicyFromCacheForTest("vip", "new-model", "")
	require.Len(t, policy.Tiers, 1)
	require.Len(t, policy.Tiers[0].Candidates, 1)
	candidate := policy.Tiers[0].Candidates[0]
	assert.Equal(t, newPriority, candidate.Priority)
	assert.Equal(t, newWeight, candidate.Weight)
	var abilities []Ability
	require.NoError(t, DB.Where("channel_id = ?", channel.Id).Find(&abilities).Error)
	require.Len(t, abilities, 1)
	assert.Equal(t, "vip", abilities[0].Group)
	assert.Equal(t, "new-model", abilities[0].Model)
	assert.Equal(t, newPriority, *abilities[0].Priority)
	assert.Equal(t, newWeight, abilities[0].Weight)
}

func TestChannelDeleteRemovesAbilitiesBeforeCacheRefresh(t *testing.T) {
	setupChannelStatusCacheTest(t)
	channel := insertChannelStatusRoutingFixture(t, 406, "delete")
	require.NoError(t, InitChannelCache())

	require.NoError(t, channel.Delete())
	require.NoError(t, InitChannelCache())

	var channelCount int64
	var abilityCount int64
	require.NoError(t, DB.Model(&Channel{}).Where("id = ?", channel.Id).Count(&channelCount).Error)
	require.NoError(t, DB.Model(&Ability{}).Where("channel_id = ?", channel.Id).Count(&abilityCount).Error)
	assert.Zero(t, channelCount)
	assert.Zero(t, abilityCount)
	assert.Empty(t, routingPolicyFromCacheForTest("default", "status-model", "").Tiers)
	_, err := CacheGetChannel(channel.Id)
	require.Error(t, err)
}

func TestUpdateChannelStatusCacheRefreshFailureKeepsLastGoodAfterCommit(t *testing.T) {
	setupChannelStatusCacheTest(t)
	channel := insertChannelStatusRoutingFixture(t, 407, "refresh-failure")
	require.NoError(t, InitChannelCache())
	beforePolicy := routingPolicyFromCacheForTest("default", "status-model", "")

	refreshError := errors.New("forced committed refresh failure")
	require.NoError(t, DB.Callback().Query().After("gorm:query").Register("test:fail_committed_status_refresh", func(tx *gorm.DB) {
		if strings.Contains(tx.Statement.SQL.String(), "JOIN abilities") {
			tx.AddError(refreshError)
		}
	}))
	t.Cleanup(func() {
		require.NoError(t, DB.Callback().Query().Remove("test:fail_committed_status_refresh"))
	})

	changed, err := UpdateChannelStatusWithError(channel.Id, "", common.ChannelStatusManuallyDisabled, "manual")
	require.True(t, changed)
	require.ErrorIs(t, err, refreshError)
	var storedChannel Channel
	require.NoError(t, DB.First(&storedChannel, channel.Id).Error)
	assert.Equal(t, common.ChannelStatusManuallyDisabled, storedChannel.Status)
	var storedAbility Ability
	require.NoError(t, DB.First(&storedAbility, "channel_id = ?", channel.Id).Error)
	assert.False(t, storedAbility.Enabled)
	assert.Equal(t, beforePolicy, routingPolicyFromCacheForTest("default", "status-model", ""))
}
