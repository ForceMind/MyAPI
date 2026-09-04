package model

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestUpdateChannelCredentialIfUnchanged(t *testing.T) {
	previousDB := DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}))
	DB = db
	t.Cleanup(func() { DB = previousDB })

	channel := &Channel{Name: "fixture", Type: 57, Key: "old-secret"}
	require.NoError(t, db.Create(channel).Error)

	updated, err := UpdateChannelCredentialIfUnchanged(context.Background(), channel.Id, channel.Type, "old-secret", "new-secret")
	require.NoError(t, err)
	assert.True(t, updated)

	var stored Channel
	require.NoError(t, db.First(&stored, channel.Id).Error)
	assert.Equal(t, "new-secret", stored.Key)

	updated, err = UpdateChannelCredentialIfUnchanged(context.Background(), channel.Id, 1, "new-secret", "wrong-type-secret")
	require.NoError(t, err)
	assert.False(t, updated)

	updated, err = UpdateChannelCredentialIfUnchanged(context.Background(), channel.Id, channel.Type, "old-secret", "stale-secret")
	require.NoError(t, err)
	assert.False(t, updated)
	require.NoError(t, db.First(&stored, channel.Id).Error)
	assert.Equal(t, "new-secret", stored.Key)
}

func TestUpdateChannelCredentialIfUnchangedConfirmsIdempotentZeroRows(t *testing.T) {
	previousDB := DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}))
	DB = db
	t.Cleanup(func() { DB = previousDB })

	channel := &Channel{Name: "idempotent-fixture", Type: 57, Key: "same-secret"}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Callback().Update().After("gorm:update").Register("test:force_zero_affected_rows", func(tx *gorm.DB) {
		tx.RowsAffected = 0
	}))

	updated, err := UpdateChannelCredentialIfUnchanged(context.Background(), channel.Id, channel.Type, channel.Key, channel.Key)

	require.NoError(t, err)
	assert.True(t, updated)
}

func TestUpdateChannelCredentialIfUnchangedSameTargetDetectsConcurrentK0ToK1(t *testing.T) {
	previousDB := DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}))
	DB = db
	t.Cleanup(func() { DB = previousDB })

	channel := &Channel{Name: "same-target-conflict", Type: 57, Key: "K0"}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Model(&Channel{}).Where("id = ?", channel.Id).Update("key", "K1").Error)

	updated, err := UpdateChannelCredentialIfUnchanged(context.Background(), channel.Id, channel.Type, "K0", "K0")

	require.NoError(t, err)
	assert.False(t, updated)
}

func TestChannelCredentialGateWaitHonorsDeadline(t *testing.T) {
	previousDB := DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}))
	DB = db
	t.Cleanup(func() { DB = previousDB })

	channel := &Channel{Name: "gate-deadline", Type: 57, Key: "old"}
	require.NoError(t, db.Create(channel).Error)
	gateHeld := make(chan struct{})
	releaseGate := make(chan struct{})
	holderDone := make(chan error, 1)
	go func() {
		holderDone <- WithChannelCredentialUpdateGate(context.Background(), channel.Id, func(context.Context) error {
			close(gateHeld)
			<-releaseGate
			return nil
		})
	}()
	<-gateHeld
	deadlineCtx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	updated, err := UpdateChannelCredentialIfUnchanged(deadlineCtx, channel.Id, channel.Type, "old", "new")

	assert.False(t, updated)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	close(releaseGate)
	require.NoError(t, <-holderDone)
}

func TestUpdateChannelCredentialIfUnchangedRejectsMissingDatabase(t *testing.T) {
	previousDB := DB
	DB = nil
	t.Cleanup(func() { DB = previousDB })

	updated, err := UpdateChannelCredentialIfUnchanged(context.Background(), 1, 57, "old", "new")

	assert.False(t, updated)
	assert.ErrorIs(t, err, gorm.ErrInvalidDB)
}

func TestUpdateChannelCredentialIfUnchangedKeepsCacheConsistentAfterRequestCancellation(t *testing.T) {
	previousDB := DB
	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}))
	DB = db
	common.MemoryCacheEnabled = true

	channel := &Channel{Name: "cache-fixture", Type: 57, Key: "old-secret"}
	require.NoError(t, db.Create(channel).Error)
	channelSyncLock.Lock()
	previousChannelsIDM := channelsIDM
	previousGeneration := channelCredentialCacheGeneration
	channelsIDM = map[int]*Channel{channel.Id: channel}
	channelSyncLock.Unlock()
	t.Cleanup(func() {
		channelSyncLock.Lock()
		channelsIDM = previousChannelsIDM
		channelCredentialCacheGeneration = previousGeneration
		channelSyncLock.Unlock()
		DB = previousDB
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
	})

	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, db.Callback().Update().After("gorm:update").Register("test:cancel_after_update", func(*gorm.DB) {
		cancel()
	}))

	updated, err := UpdateChannelCredentialIfUnchanged(ctx, channel.Id, channel.Type, "old-secret", "new-secret")

	require.NoError(t, err)
	assert.True(t, updated)
	assert.ErrorIs(t, ctx.Err(), context.Canceled)
	cached, err := CacheGetChannel(channel.Id)
	require.NoError(t, err)
	assert.Equal(t, "new-secret", cached.Key)
	assert.Empty(t, cached.Keys)
	assert.Equal(t, "old-secret", channel.Key, "cache update must not mutate an in-flight channel snapshot")
}

func TestInitChannelCacheRetriesSnapshotAfterCredentialGenerationChanges(t *testing.T) {
	previousDB := DB
	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	db, err := gorm.Open(sqlite.Open("file:channel-cache-generation?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}))
	DB = db
	common.MemoryCacheEnabled = true
	channel := &Channel{Name: "generation-fixture", Type: 57, Key: "old-secret", Models: "gpt-5", Group: "default", Status: common.ChannelStatusEnabled}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, channel.AddAbilities(db))

	channelSyncLock.Lock()
	previousChannelsIDM := channelsIDM
	previousGroups := group2model2channels
	previousAdvancedConfigs := channel2advancedCustomConfig
	previousGeneration := channelCredentialCacheGeneration
	channelsIDM = map[int]*Channel{channel.Id: channel}
	channelCredentialCacheGeneration = 0
	channelSyncLock.Unlock()
	t.Cleanup(func() {
		channelSyncLock.Lock()
		channelsIDM = previousChannelsIDM
		group2model2channels = previousGroups
		channel2advancedCustomConfig = previousAdvancedConfigs
		channelCredentialCacheGeneration = previousGeneration
		channelSyncLock.Unlock()
		DB = previousDB
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
		require.NoError(t, sqlDB.Close())
	})

	firstSnapshotRead := make(chan struct{})
	releaseSnapshot := make(chan struct{})
	var channelReads atomic.Int32
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("test:block_first_channel_cache_snapshot", func(tx *gorm.DB) {
		if tx.Statement.Table == "channels" && channelReads.Add(1) == 1 {
			close(firstSnapshotRead)
			<-releaseSnapshot
		}
	}))
	initDone := make(chan struct{})
	go func() {
		InitChannelCache()
		close(initDone)
	}()
	<-firstSnapshotRead

	updated, err := UpdateChannelCredentialIfUnchanged(context.Background(), channel.Id, channel.Type, "old-secret", "new-secret")
	require.NoError(t, err)
	require.True(t, updated)
	close(releaseSnapshot)
	<-initDone

	assert.GreaterOrEqual(t, channelReads.Load(), int32(2))
	cached, err := CacheGetChannel(channel.Id)
	require.NoError(t, err)
	assert.Equal(t, "new-secret", cached.Key)
}

func TestInsertChannelWithAbilitiesIsAtomic(t *testing.T) {
	t.Run("creates channel and abilities", func(t *testing.T) {
		previousDB := DB
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		require.NoError(t, err)
		require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}))
		DB = db
		t.Cleanup(func() { DB = previousDB })

		channel := &Channel{Name: "fixture", Key: "credential-secret", Models: "gpt-5", Group: "default", Status: 1}
		require.NoError(t, InsertChannelWithAbilities(context.Background(), channel))
		assert.NotZero(t, channel.Id)
		var channelCount int64
		var abilityCount int64
		require.NoError(t, db.Model(&Channel{}).Where("id = ?", channel.Id).Count(&channelCount).Error)
		require.NoError(t, db.Model(&Ability{}).Where("channel_id = ?", channel.Id).Count(&abilityCount).Error)
		assert.Equal(t, int64(1), channelCount)
		assert.Equal(t, int64(1), abilityCount)
	})

	t.Run("rolls back channel when abilities fail", func(t *testing.T) {
		previousDB := DB
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		require.NoError(t, err)
		require.NoError(t, db.AutoMigrate(&Channel{}))
		DB = db
		t.Cleanup(func() { DB = previousDB })

		channel := &Channel{Name: "rollback-fixture", Key: "credential-secret", Models: "gpt-5", Group: "default", Status: 1}
		require.Error(t, InsertChannelWithAbilities(context.Background(), channel))
		var count int64
		require.NoError(t, db.Model(&Channel{}).Where("name = ?", channel.Name).Count(&count).Error)
		assert.Zero(t, count)
	})
}
