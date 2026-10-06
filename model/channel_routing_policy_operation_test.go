package model

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupChannelOperationTest(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB := DB
	previousMemoryCache := common.MemoryCacheEnabled
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(8)
	require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}))
	DB = db
	common.MemoryCacheEnabled = false
	resetChannelRoutingSchemaStateForTest(db)
	require.NoError(t, ensureChannelRoutingSchema(db))
	channelSyncLock.Lock()
	previousObserved := channelCacheObservedCommittedEpoch
	previousPublished := channelCachePublishedEpoch
	channelCacheObservedCommittedEpoch = 0
	channelCachePublishedEpoch = 0
	channelSyncLock.Unlock()
	t.Cleanup(func() {
		channelSyncLock.Lock()
		channelCacheObservedCommittedEpoch = previousObserved
		channelCachePublishedEpoch = previousPublished
		channelSyncLock.Unlock()
		resetChannelRoutingSchemaStateForTest(db)
		DB = previousDB
		common.MemoryCacheEnabled = previousMemoryCache
		require.NoError(t, sqlDB.Close())
	})
	return db
}

func operationFixtureChannel(name string) Channel {
	priority := int64(10)
	weight := uint(1)
	return Channel{
		Name: name, Type: constant.ChannelTypeOpenAI, Key: "secret-" + name,
		Status: common.ChannelStatusEnabled, Group: "default", Models: "operation-model",
		Priority: &priority, Weight: &weight,
	}
}

func TestChannelCreateOperationSerialReplayAndConflict(t *testing.T) {
	db := setupChannelOperationTest(t)
	channels := []Channel{operationFixtureChannel("serial")}
	fingerprint, err := FingerprintChannelOperation(channels)
	require.NoError(t, err)

	beforeEpoch, err := GetCommittedChannelRoutingEpoch(db)
	require.NoError(t, err)
	first, replayed, err := BatchInsertChannelsWithOperation(context.Background(), channels, "serial-operation-key", fingerprint)
	require.NoError(t, err)
	assert.False(t, replayed)
	require.Len(t, first.IDs, 1)
	afterFirstEpoch, err := GetCommittedChannelRoutingEpoch(db)
	require.NoError(t, err)
	assert.Equal(t, beforeEpoch+1, afterFirstEpoch)

	second, replayed, err := BatchInsertChannelsWithOperation(context.Background(), channels, "serial-operation-key", fingerprint)
	require.NoError(t, err)
	assert.True(t, replayed)
	assert.Equal(t, first, second)
	afterReplayEpoch, err := GetCommittedChannelRoutingEpoch(db)
	require.NoError(t, err)
	assert.Equal(t, afterFirstEpoch, afterReplayEpoch)

	var channelCount int64
	require.NoError(t, db.Model(&Channel{}).Count(&channelCount).Error)
	assert.Equal(t, int64(1), channelCount)

	conflictingFingerprint, err := FingerprintChannelOperation([]Channel{operationFixtureChannel("different")})
	require.NoError(t, err)
	_, _, err = BatchInsertChannelsWithOperation(context.Background(), []Channel{operationFixtureChannel("different")}, "serial-operation-key", conflictingFingerprint)
	require.ErrorIs(t, err, ErrChannelOperationConflict)
}

func TestChannelCreateOperationConcurrentReplayCreatesOneBatch(t *testing.T) {
	db := setupChannelOperationTest(t)
	channels := []Channel{operationFixtureChannel("concurrent-a"), operationFixtureChannel("concurrent-b")}
	fingerprint, err := FingerprintChannelOperation(channels)
	require.NoError(t, err)

	type outcome struct {
		result   ChannelOperationResult
		replayed bool
		err      error
	}
	start := make(chan struct{})
	outcomes := make(chan outcome, 2)
	var workers sync.WaitGroup
	for range 2 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			result, replayed, operationErr := BatchInsertChannelsWithOperation(context.Background(), channels, "concurrent-operation-key", fingerprint)
			outcomes <- outcome{result: result, replayed: replayed, err: operationErr}
		}()
	}
	close(start)
	workers.Wait()
	close(outcomes)

	results := make([]outcome, 0, 2)
	for result := range outcomes {
		require.NoError(t, result.err)
		results = append(results, result)
	}
	require.Len(t, results, 2)
	assert.Equal(t, results[0].result.IDs, results[1].result.IDs)
	assert.NotEqual(t, results[0].replayed, results[1].replayed)
	var channelCount int64
	require.NoError(t, db.Model(&Channel{}).Count(&channelCount).Error)
	assert.Equal(t, int64(2), channelCount)
}

func TestBatchInsertChannelsPanicRollsBackOperationAndEpoch(t *testing.T) {
	db := setupChannelOperationTest(t)
	channels := []Channel{operationFixtureChannel("panic")}
	fingerprint, err := FingerprintChannelOperation(channels)
	require.NoError(t, err)
	beforeEpoch, err := GetCommittedChannelRoutingEpoch(db)
	require.NoError(t, err)

	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:panic_channel_create", func(tx *gorm.DB) {
		if tx.Statement.Table == "channels" {
			panic("forced channel create panic")
		}
	}))
	t.Cleanup(func() {
		require.NoError(t, db.Callback().Create().Remove("test:panic_channel_create"))
	})

	require.Panics(t, func() {
		_, _, _ = BatchInsertChannelsWithOperation(context.Background(), channels, "panic-operation-key", fingerprint)
	})
	var channelCount int64
	var operationCount int64
	require.NoError(t, db.Model(&Channel{}).Count(&channelCount).Error)
	require.NoError(t, db.Model(&ChannelRoutingOperation{}).Count(&operationCount).Error)
	assert.Zero(t, channelCount)
	assert.Zero(t, operationCount)
	afterEpoch, err := GetCommittedChannelRoutingEpoch(db)
	require.NoError(t, err)
	assert.Equal(t, beforeEpoch, afterEpoch)
}

func TestUpdateAbilitiesPanicRollsBackAndDoesNotAdvanceEpoch(t *testing.T) {
	db := setupChannelOperationTest(t)
	channel := operationFixtureChannel("ability-panic")
	require.NoError(t, channel.Insert())
	beforeEpoch, err := GetCommittedChannelRoutingEpoch(db)
	require.NoError(t, err)
	var beforeCount int64
	require.NoError(t, db.Model(&Ability{}).Where("channel_id = ?", channel.Id).Count(&beforeCount).Error)

	forced := errors.New("forced ability delete panic")
	require.NoError(t, db.Callback().Delete().Before("gorm:delete").Register("test:panic_ability_delete", func(tx *gorm.DB) {
		if tx.Statement.Table == "abilities" {
			panic(forced)
		}
	}))
	t.Cleanup(func() {
		require.NoError(t, db.Callback().Delete().Remove("test:panic_ability_delete"))
	})

	require.PanicsWithValue(t, forced, func() {
		_ = channel.UpdateAbilities(nil)
	})
	var afterCount int64
	require.NoError(t, db.Model(&Ability{}).Where("channel_id = ?", channel.Id).Count(&afterCount).Error)
	assert.Equal(t, beforeCount, afterCount)
	afterEpoch, err := GetCommittedChannelRoutingEpoch(db)
	require.NoError(t, err)
	assert.Equal(t, beforeEpoch, afterEpoch)
}

func TestChannelRoutingEpochDetectsRemoteInstanceAndPreservesSnapshotLabels(t *testing.T) {
	previousDB := DB
	previousMemoryCache := common.MemoryCacheEnabled
	dsn := filepath.Join(t.TempDir(), "routing-epoch.db")
	dbA, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	dbB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, dbA.AutoMigrate(&Channel{}, &Ability{}))
	resetChannelRoutingSchemaStateForTest(dbA)
	resetChannelRoutingSchemaStateForTest(dbB)
	require.NoError(t, ensureChannelRoutingSchema(dbA))
	require.NoError(t, ensureChannelRoutingSchema(dbB))
	DB = dbA
	common.MemoryCacheEnabled = true
	channelSyncLock.Lock()
	previousObserved := channelCacheObservedCommittedEpoch
	previousPublished := channelCachePublishedEpoch
	channelCacheObservedCommittedEpoch = 0
	channelCachePublishedEpoch = 0
	channelSyncLock.Unlock()
	t.Cleanup(func() {
		channelSyncLock.Lock()
		channelCacheObservedCommittedEpoch = previousObserved
		channelCachePublishedEpoch = previousPublished
		channelSyncLock.Unlock()
		resetChannelRoutingSchemaStateForTest(dbA)
		resetChannelRoutingSchemaStateForTest(dbB)
		DB = previousDB
		common.MemoryCacheEnabled = previousMemoryCache
	})

	channel := operationFixtureChannel("remote")
	require.NoError(t, channel.Insert())
	require.NoError(t, InitChannelCache())
	before, err := GetRuntimeChannelRoutingPolicy("default", "operation-model", "")
	require.NoError(t, err)
	require.Len(t, before.Policy.Tiers, 1)
	assert.False(t, before.CachePending)
	assert.Equal(t, before.ClusterCommittedEpoch, before.LocalPublishedEpoch)
	assert.Equal(t, uint(1), before.Policy.Tiers[0].Candidates[0].Weight)

	remoteWeight := uint(9)
	var remoteEpoch int64
	require.NoError(t, dbB.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&Ability{}).Where("channel_id = ?", channel.Id).Update("weight", remoteWeight)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errors.New("remote routing mutation changed no ability")
		}
		var advanceErr error
		remoteEpoch, advanceErr = advanceChannelRoutingEpoch(tx)
		return advanceErr
	}))

	cached, err := GetRuntimeChannelRoutingPolicy("default", "operation-model", "")
	require.NoError(t, err)
	assert.Equal(t, ChannelRoutingSourceCache, cached.Source)
	assert.True(t, cached.CachePending)
	assert.Equal(t, before.LocalPublishedEpoch, cached.LocalPublishedEpoch)
	assert.Equal(t, remoteEpoch, cached.ClusterCommittedEpoch)
	assert.Equal(t, before.LocalPublishedEpoch, cached.Generation)
	assert.Equal(t, uint(1), cached.Policy.Tiers[0].Candidates[0].Weight)

	common.MemoryCacheEnabled = false
	database, err := GetRuntimeChannelRoutingPolicy("default", "operation-model", "")
	require.NoError(t, err)
	assert.Equal(t, ChannelRoutingSourceDatabase, database.Source)
	assert.Equal(t, remoteEpoch, database.Generation)
	assert.Equal(t, remoteEpoch, database.ClusterCommittedEpoch)
	assert.Equal(t, remoteWeight, database.Policy.Tiers[0].Candidates[0].Weight)

	common.MemoryCacheEnabled = true
	require.NoError(t, InitChannelCache())
	repaired, err := GetRuntimeChannelRoutingPolicy("default", "operation-model", "")
	require.NoError(t, err)
	assert.False(t, repaired.CachePending)
	assert.Equal(t, remoteEpoch, repaired.LocalPublishedEpoch)
	assert.Equal(t, remoteWeight, repaired.Policy.Tiers[0].Candidates[0].Weight)
}

func TestChannelCopyOperationReplaysPersistedResultAndConflicts(t *testing.T) {
	db := setupChannelOperationTest(t)
	source := operationFixtureChannel("copy-source")
	require.NoError(t, source.Insert())
	request := struct {
		SourceID     int    `json:"source_id"`
		Suffix       string `json:"suffix"`
		ResetBalance bool   `json:"reset_balance"`
	}{SourceID: source.Id, Suffix: "-copy", ResetBalance: true}
	fingerprint, err := FingerprintChannelOperation(request)
	require.NoError(t, err)

	first, replayed, err := CopyChannelWithOperation(context.Background(), source.Id, request.Suffix, request.ResetBalance, "copy-operation-key", fingerprint)
	require.NoError(t, err)
	assert.False(t, replayed)
	require.Len(t, first.IDs, 1)
	second, replayed, err := CopyChannelWithOperation(context.Background(), source.Id, request.Suffix, request.ResetBalance, "copy-operation-key", fingerprint)
	require.NoError(t, err)
	assert.True(t, replayed)
	assert.Equal(t, first, second)

	var count int64
	require.NoError(t, db.Model(&Channel{}).Count(&count).Error)
	assert.Equal(t, int64(2), count)

	conflictingFingerprint, err := FingerprintChannelOperation(struct {
		SourceID     int    `json:"source_id"`
		Suffix       string `json:"suffix"`
		ResetBalance bool   `json:"reset_balance"`
	}{SourceID: source.Id, Suffix: "-different", ResetBalance: true})
	require.NoError(t, err)
	_, _, err = CopyChannelWithOperation(context.Background(), source.Id, "-different", true, "copy-operation-key", conflictingFingerprint)
	require.ErrorIs(t, err, ErrChannelOperationConflict)
}
