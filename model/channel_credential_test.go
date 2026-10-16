package model

import (
	"context"
	"strings"
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
	require.NoError(t, db.Create(&Channel{Id: 7, Type: 57, Key: "credential-a"}).Error)

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

func TestLookupTrustedChannelCredentialSubjectUsesHistoricalProviderEvidence(t *testing.T) {
	previousDB := DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&Channel{},
		&ChannelQuotaIdentityKeyRegistry{}, &ChannelQuotaIdentityKeyVersion{}, &ChannelQuotaIdentityAlias{},
		&ChannelQuotaSamplingTarget{}, &ChannelQuotaSnapshot{},
	))
	DB = db
	t.Cleanup(func() { DB = previousDB })
	keyring := quotaIdentityKeyring(t, "credential-guard", 'g')
	require.NoError(t, EnsureChannelQuotaIdentityKeyring(t.Context(), db, keyring))
	require.NoError(t, db.Create(&Channel{Id: 7, Type: 57, Key: "credential-a"}).Error)
	credential, err := ResolveChannelQuotaIdentity(t.Context(), db, keyring, "channel_type_57", common.ChannelQuotaIdentityKindCredential, []byte("credential-a"))
	require.NoError(t, err)
	account, err := ResolveChannelQuotaIdentity(t.Context(), db, keyring, "channel_type_57", common.ChannelQuotaIdentityKindProviderAccount, []byte("account-a"))
	require.NoError(t, err)
	now := time.Now().Unix()
	require.NoError(t, db.Create(&ChannelQuotaSamplingTarget{
		ChannelID: 7, SubjectRef: credential.SubjectRef, IdentityQuality: credential.Quality,
		ConfirmedSubjectRef: account.SubjectRef, Active: true, LastResult: "sampled", EvidenceVersion: 3, CreatedAt: now, UpdatedAt: now,
	}).Error)
	snapshot := &ChannelQuotaSnapshot{
		ChannelId: 7, SubjectRef: account.SubjectRef, IdentityQuality: account.Quality,
		CodexObservationQualified: true, ObservedAt: now - SubscriptionShareDefaultMaxAge - 1, MetricType: "codex_rate_limit", WindowType: "five_hour",
		WindowSeconds: 18000, Source: codexQuotaWindowPrimarySource, Status: "success", Unit: "percent",
	}
	require.NoError(t, db.Create(snapshot).Error)
	require.NoError(t, db.Model(&ChannelQuotaSamplingTarget{}).
		Where("channel_id = ? AND subject_ref = ?", 7, credential.SubjectRef).
		Update("verified_snapshot_id", snapshot.Id).Error)

	trusted, err := LookupTrustedChannelCredentialSubject(t.Context(), 7, credential.SubjectRef)
	require.NoError(t, err)
	assert.Equal(t, account.SubjectRef, trusted)
	require.NoError(t, db.Model(&ChannelQuotaSamplingTarget{}).Where("channel_id = ? AND subject_ref = ?", 7, credential.SubjectRef).Update("last_result", "error").Error)
	rotated, err := ResolveChannelQuotaIdentity(t.Context(), db, keyring, "channel_type_57", common.ChannelQuotaIdentityKindCredential, []byte("credential-b"))
	require.NoError(t, err)
	updated, err := UpdateChannelCredentialAndSubjectIfUnchanged(
		t.Context(), 7, 57, "credential-a", "credential-b", credential.SubjectRef, rotated.SubjectRef, account.SubjectRef,
	)
	require.NoError(t, err)
	require.True(t, updated)
	var stored Channel
	require.NoError(t, db.First(&stored, 7).Error)
	assert.Equal(t, "credential-b", stored.Key)
	var movedTarget ChannelQuotaSamplingTarget
	require.NoError(t, db.Where("channel_id = ? AND subject_ref = ?", 7, rotated.SubjectRef).Take(&movedTarget).Error)
	assert.Equal(t, int64(4), movedTarget.EvidenceVersion)
	assert.Zero(t, movedTarget.VerifiedSnapshotID)
	assert.Equal(t, account.SubjectRef, movedTarget.ConfirmedSubjectRef)
	trusted, err = LookupTrustedChannelCredentialSubject(t.Context(), 7, rotated.SubjectRef)
	require.NoError(t, err)
	assert.Equal(t, account.SubjectRef, trusted)
	blocker := &ChannelQuotaSnapshot{
		ChannelId: 7, SubjectRef: account.SubjectRef, IdentityQuality: account.Quality,
		ObservedAt: now, MetricType: "codex_rate_limit", WindowType: "five_hour",
		WindowSeconds: 18000, Source: codexQuotaWindowPrimarySource, Status: "error", ErrorCode: "upstream_timeout", Unit: "percent",
	}
	require.NoError(t, db.Create(blocker).Error)
	trusted, err = LookupTrustedChannelCredentialSubject(t.Context(), 7, rotated.SubjectRef)
	require.NoError(t, err)
	assert.Equal(t, account.SubjectRef, trusted)

	require.NoError(t, db.Delete(snapshot).Error)
	_, err = LookupTrustedChannelCredentialSubject(t.Context(), 7, rotated.SubjectRef)
	assert.ErrorIs(t, err, ErrChannelCredentialSubjectUnknown)
}

func TestUpdateChannelCredentialElementIfUnchangedPreservesOtherKeys(t *testing.T) {
	previousDB := DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}))
	DB = db
	t.Cleanup(func() { DB = previousDB })

	newline := &Channel{Type: 48, Key: "old\nother", Status: common.ChannelStatusEnabled, ChannelInfo: ChannelInfo{IsMultiKey: true}}
	require.NoError(t, db.Create(newline).Error)
	updated, err := UpdateChannelCredentialElementIfUnchanged(context.Background(), newline.Id, newline.Type, "old", "new")
	require.NoError(t, err)
	require.True(t, updated)
	require.NoError(t, db.First(newline, newline.Id).Error)
	require.Equal(t, "new\nother", newline.Key)

	oldJSON := `{"access_token":"old","account_id":"account"}`
	newJSON := `{"access_token":"new","account_id":"account"}`
	jsonChannel := &Channel{Type: 48, Key: "[" + oldJSON + `,{"access_token":"other","account_id":"other"}]`, Status: common.ChannelStatusEnabled, ChannelInfo: ChannelInfo{IsMultiKey: true}}
	require.NoError(t, db.Create(jsonChannel).Error)
	updated, err = UpdateChannelCredentialElementIfUnchanged(context.Background(), jsonChannel.Id, jsonChannel.Type, oldJSON, newJSON)
	require.NoError(t, err)
	require.True(t, updated)
	require.NoError(t, db.First(jsonChannel, jsonChannel.Id).Error)
	require.Contains(t, jsonChannel.Key, `"access_token":"new"`)
	require.Contains(t, jsonChannel.Key, `"access_token":"other"`)
	require.NotContains(t, jsonChannel.Key, `"access_token":"old"`)
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
	previousRoutingCandidates := group2model2routingCandidates
	previousAdvancedConfigs := channel2advancedCustomConfig
	previousGeneration := channelCredentialCacheGeneration
	previousDataGeneration := channelCacheDataGeneration
	previousPublishGeneration := channelCachePublishGeneration
	previousObservedEpoch := channelCacheObservedCommittedEpoch
	previousPublishedEpoch := channelCachePublishedEpoch
	channelsIDM = map[int]*Channel{channel.Id: channel}
	channelCredentialCacheGeneration = 0
	channelCacheDataGeneration = 0
	channelCachePublishGeneration = 0
	channelCacheObservedCommittedEpoch = 0
	channelCachePublishedEpoch = 0
	channelSyncLock.Unlock()
	t.Cleanup(func() {
		channelSyncLock.Lock()
		channelsIDM = previousChannelsIDM
		group2model2channels = previousGroups
		group2model2routingCandidates = previousRoutingCandidates
		channel2advancedCustomConfig = previousAdvancedConfigs
		channelCredentialCacheGeneration = previousGeneration
		channelCacheDataGeneration = previousDataGeneration
		channelCachePublishGeneration = previousPublishGeneration
		channelCacheObservedCommittedEpoch = previousObservedEpoch
		channelCachePublishedEpoch = previousPublishedEpoch
		channelSyncLock.Unlock()
		DB = previousDB
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
		require.NoError(t, sqlDB.Close())
	})

	firstSnapshotRead := make(chan struct{})
	releaseSnapshot := make(chan struct{})
	var channelReads atomic.Int32
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("test:block_first_channel_cache_snapshot", func(tx *gorm.DB) {
		if strings.Contains(tx.Statement.SQL.String(), "JOIN abilities") && channelReads.Add(1) == 1 {
			close(firstSnapshotRead)
			<-releaseSnapshot
		}
	}))
	initDone := make(chan error, 1)
	go func() {
		initDone <- InitChannelCache()
	}()
	<-firstSnapshotRead

	updated, err := UpdateChannelCredentialIfUnchanged(context.Background(), channel.Id, channel.Type, "old-secret", "new-secret")
	require.NoError(t, err)
	require.True(t, updated)
	close(releaseSnapshot)
	require.NoError(t, <-initDone)

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
