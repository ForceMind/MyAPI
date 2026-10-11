package model

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type rejectDirectTokenCacheDeleteHook struct {
	err error
}

func (hook rejectDirectTokenCacheDeleteHook) BeforeProcess(ctx context.Context, cmd redis.Cmder) (context.Context, error) {
	if cmd.Name() == "del" {
		return ctx, hook.err
	}
	return ctx, nil
}

func (rejectDirectTokenCacheDeleteHook) AfterProcess(context.Context, redis.Cmder) error {
	return nil
}

func (rejectDirectTokenCacheDeleteHook) BeforeProcessPipeline(ctx context.Context, _ []redis.Cmder) (context.Context, error) {
	return ctx, nil
}

func (rejectDirectTokenCacheDeleteHook) AfterProcessPipeline(context.Context, []redis.Cmder) error {
	return nil
}

type rejectNthTokenCacheEvalHook struct {
	count         *int
	failAt        int
	err           error
	beforeFailure func()
}

func (hook rejectNthTokenCacheEvalHook) BeforeProcess(ctx context.Context, cmd redis.Cmder) (context.Context, error) {
	if cmd.Name() != "eval" {
		return ctx, nil
	}
	*hook.count++
	if *hook.count == hook.failAt {
		if hook.beforeFailure != nil {
			hook.beforeFailure()
		}
		return ctx, hook.err
	}
	return ctx, nil
}

func (rejectNthTokenCacheEvalHook) AfterProcess(context.Context, redis.Cmder) error {
	return nil
}

func (rejectNthTokenCacheEvalHook) BeforeProcessPipeline(ctx context.Context, _ []redis.Cmder) (context.Context, error) {
	return ctx, nil
}

func (rejectNthTokenCacheEvalHook) AfterProcessPipeline(context.Context, []redis.Cmder) error {
	return nil
}

func TestTokenQuotaMetadataFingerprintCoversWritableMetadataOnly(t *testing.T) {
	emptyAllowIPs := ""
	base := Token{
		Id: 11, UserId: 12, Key: "secret", Name: "name", Status: common.TokenStatusEnabled,
		CreatedTime: 13, AccessedTime: 14, ExpiredTime: 1<<53 + 1,
		RemainQuota: 15, QuotaVersion: 16, UnlimitedQuota: true,
		ModelLimitsEnabled: true, ModelLimits: "model-a", AllowIps: nil,
		UsedQuota: 17, Group: "auto", AccessProfileID: "profile-a",
		CrossGroupRetry: true, AutoGroups: `["vip","default"]`,
	}
	baseFingerprint, err := base.QuotaMetadataFingerprint()
	require.NoError(t, err)
	require.Len(t, baseFingerprint, sha256.Size*2)

	nilNormalized := base
	nilNormalized.AllowIps = &emptyAllowIPs
	normalizedFingerprint, err := nilNormalized.QuotaMetadataFingerprint()
	require.NoError(t, err)
	require.Equal(t, baseFingerprint, normalizedFingerprint)

	for _, tc := range []struct {
		name   string
		mutate func(*Token)
	}{
		{"id", func(token *Token) { token.Id++ }},
		{"user id", func(token *Token) { token.UserId++ }},
		{"name", func(token *Token) { token.Name += "-changed" }},
		{"status", func(token *Token) { token.Status = common.TokenStatusDisabled }},
		{"expiry", func(token *Token) { token.ExpiredTime++ }},
		{"unlimited", func(token *Token) { token.UnlimitedQuota = false }},
		{"model limits enabled", func(token *Token) { token.ModelLimitsEnabled = false }},
		{"model limits", func(token *Token) { token.ModelLimits += ",model-b" }},
		{"allow ips", func(token *Token) { value := "127.0.0.1"; token.AllowIps = &value }},
		{"group", func(token *Token) { token.Group = "default" }},
		{"access profile", func(token *Token) { token.AccessProfileID = "profile-b" }},
		{"cross group retry", func(token *Token) { token.CrossGroupRetry = false }},
		{"auto groups", func(token *Token) { token.AutoGroups = `["default"]` }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := base
			tc.mutate(&changed)
			fingerprint, err := changed.QuotaMetadataFingerprint()
			require.NoError(t, err)
			require.NotEqual(t, baseFingerprint, fingerprint)
		})
	}

	excluded := base
	excluded.Key = "different-secret"
	excluded.CreatedTime++
	excluded.AccessedTime++
	excluded.RemainQuota++
	excluded.UsedQuota++
	excluded.QuotaVersion++
	excludedFingerprint, err := excluded.QuotaMetadataFingerprint()
	require.NoError(t, err)
	require.Equal(t, baseFingerprint, excludedFingerprint)
}

func TestTokenQuotaUpdateRejectsMetadataChangedAfterFormOpened(t *testing.T) {
	_, fixture, _ := openLegacyBalanceBatchTestWithRedis(t, "token-quota-metadata-first")

	opened, err := GetTokenById(fixture.Token.Id)
	require.NoError(t, err)
	openedFingerprint, err := opened.QuotaMetadataFingerprint()
	require.NoError(t, err)
	metadataUpdate := *opened
	metadataUpdate.Status = common.TokenStatusDisabled
	require.NoError(t, metadataUpdate.Update())

	opened.RemainQuota--
	_, _, err = opened.UpdateWithQuota(newTokenQuotaAdjustmentCommandForTest(t, opened, opened.QuotaVersion, openedFingerprint))
	require.ErrorIs(t, err, ErrTokenQuotaVersionConflict)

	var stored Token
	require.NoError(t, DB.First(&stored, fixture.Token.Id).Error)
	require.Equal(t, common.TokenStatusDisabled, stored.Status)
	require.Equal(t, fixture.Token.RemainQuota, stored.RemainQuota)
	require.Equal(t, fixture.Token.QuotaVersion, stored.QuotaVersion)
}

func TestTokenQuotaUpdateDoesNotPublishMetadataSnapshotAfterCommit(t *testing.T) {
	_, fixture, server := openLegacyBalanceBatchTestWithRedis(t, "token-quota-before-metadata")
	requested, err := GetTokenById(fixture.Token.Id)
	require.NoError(t, err)
	openedFingerprint, err := requested.QuotaMetadataFingerprint()
	require.NoError(t, err)
	requested.RemainQuota--

	callbackName := "token-quota-metadata-after-snapshot"
	injected := false
	require.NoError(t, DB.Callback().Query().After("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		current, ok := tx.Statement.Dest.(*Token)
		if injected || !ok || current.Id != fixture.Token.Id || current.QuotaVersion != fixture.Token.QuotaVersion+1 {
			return
		}
		injected = true
		server.FastForward(time.Duration(tokenCacheFenceSeconds+1) * time.Second)
		result := tx.Session(&gorm.Session{NewDB: true, SkipHooks: true}).
			Model(&Token{}).
			Where("id = ?", fixture.Token.Id).
			Updates(map[string]any{"status": common.TokenStatusDisabled, "name": "metadata-after-quota"})
		if result.Error != nil {
			tx.AddError(result.Error)
		}
	}))
	t.Cleanup(func() { _ = DB.Callback().Query().Remove(callbackName) })

	_, replayed, err := requested.UpdateWithQuota(newTokenQuotaAdjustmentCommandForTest(t, requested, requested.QuotaVersion, openedFingerprint))
	require.NoError(t, err)
	require.False(t, replayed)
	require.True(t, injected)
	require.True(t, server.Exists(getTokenCacheFenceKey(fixture.Token.Key)))
	require.False(t, server.Exists(getTokenCacheKey(fixture.Token.Key)))

	loaded, err := GetTokenByKey(fixture.Token.Key, false)
	require.NoError(t, err)
	require.Equal(t, common.TokenStatusDisabled, loaded.Status)
	require.Equal(t, "metadata-after-quota", loaded.Name)
}

func TestTokenQuotaUpdateReceiptSurvivesPostCommitCacheFailure(t *testing.T) {
	_, fixture, server := openLegacyBalanceBatchTestWithRedis(t, "token-quota-receipt-post-cache")
	requested, err := GetTokenById(fixture.Token.Id)
	require.NoError(t, err)
	fingerprint, err := requested.QuotaMetadataFingerprint()
	require.NoError(t, err)
	requested.RemainQuota--
	command := newTokenQuotaAdjustmentCommandForTest(t, requested, requested.QuotaVersion, fingerprint)

	callbackName := "token-quota-fail-cache-after-receipt"
	callbackRegistered := true
	require.NoError(t, DB.Callback().Create().After("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		adjustment, ok := tx.Statement.Dest.(*TokenQuotaAdjustment)
		if ok && adjustment.RequestID == command.RequestID {
			server.SetError("ERR synthetic post-commit token quota cache failure")
		}
	}))
	t.Cleanup(func() {
		server.SetError("")
		if callbackRegistered {
			_ = DB.Callback().Create().Remove(callbackName)
		}
	})

	result, replayed, err := requested.UpdateWithQuota(command)
	require.NoError(t, err)
	require.False(t, replayed)
	require.NotNil(t, result)
	server.SetError("")
	require.NoError(t, DB.Callback().Create().Remove(callbackName))
	callbackRegistered = false

	retry := fixture.Token
	replayedResult, replayed, err := retry.UpdateWithQuota(command)
	require.NoError(t, err)
	require.True(t, replayed)
	require.Equal(t, result, replayedResult)
	require.Equal(t, requested.RemainQuota, retry.RemainQuota)
	require.Equal(t, requested.QuotaVersion, retry.QuotaVersion)
	var count int64
	require.NoError(t, DB.Model(&TokenQuotaAdjustment{}).Where("request_id = ?", command.RequestID).Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestTokenQuotaReplayConfirmsReceiptWithoutReadingLeadingCache(t *testing.T) {
	_, fixture, server := openLegacyBalanceBatchTestWithRedis(t, "token-quota-replay-leading-cache")
	requested, err := GetTokenById(fixture.Token.Id)
	require.NoError(t, err)
	fingerprint, err := requested.QuotaMetadataFingerprint()
	require.NoError(t, err)
	requested.RemainQuota--
	command := newTokenQuotaAdjustmentCommandForTest(t, requested, requested.QuotaVersion, fingerprint)
	result, replayed, err := requested.UpdateWithQuota(command)
	require.NoError(t, err)
	require.False(t, replayed)

	databaseRemain := requested.RemainQuota - 7
	databaseUsed := requested.UsedQuota + 7
	databaseVersion := requested.QuotaVersion + 1
	require.NoError(t, DB.Model(&Token{}).Where("id = ?", requested.Id).Updates(map[string]any{
		"name": "database-current", "remain_quota": databaseRemain, "used_quota": databaseUsed, "quota_version": databaseVersion,
	}).Error)
	cacheKey := getTokenCacheKey(requested.Key)
	fenceKey := getTokenCacheFenceKey(requested.Key)
	ctx := context.Background()
	require.NoError(t, common.RDB.HSet(ctx, cacheKey, map[string]any{
		"Id": requested.Id, "CacheSchema": tokenCacheSchemaVersion,
		"RemainQuota": databaseRemain - 11, "UsedQuota": databaseUsed + 11, "QuotaVersion": databaseVersion + 1,
	}).Err())
	require.NoError(t, common.RDB.Set(ctx, fenceKey, "pending", time.Minute).Err())
	cacheBefore, err := common.RDB.HGetAll(ctx, cacheKey).Result()
	require.NoError(t, err)
	fenceBefore, err := common.RDB.Get(ctx, fenceKey).Result()
	require.NoError(t, err)

	server.SetError("ERR synthetic unavailable cache during receipt replay")
	t.Cleanup(func() { server.SetError("") })
	retry := fixture.Token
	replayedResult, replayed, err := retry.UpdateWithQuota(command)
	require.NoError(t, err)
	require.True(t, replayed)
	require.Equal(t, result, replayedResult)
	require.Equal(t, "database-current", retry.Name)
	require.Equal(t, databaseRemain, retry.RemainQuota)
	require.Equal(t, databaseUsed, retry.UsedQuota)
	require.Equal(t, databaseVersion, retry.QuotaVersion)
	server.SetError("")

	cacheAfter, err := common.RDB.HGetAll(ctx, cacheKey).Result()
	require.NoError(t, err)
	fenceAfter, err := common.RDB.Get(ctx, fenceKey).Result()
	require.NoError(t, err)
	require.Equal(t, cacheBefore, cacheAfter)
	require.Equal(t, fenceBefore, fenceAfter)
	var receiptCount int64
	require.NoError(t, DB.Model(&TokenQuotaAdjustment{}).Where("request_id = ?", command.RequestID).Count(&receiptCount).Error)
	require.Equal(t, int64(1), receiptCount)
}

func TestTokenQuotaWriterModeProbeErrorFailsClosed(t *testing.T) {
	_, fixture, _ := openLegacyBalanceBatchTestWithRedis(t, "token-quota-probe-error")
	requested, err := GetTokenById(fixture.Token.Id)
	require.NoError(t, err)
	fingerprint, err := requested.QuotaMetadataFingerprint()
	require.NoError(t, err)
	requested.RemainQuota--
	require.NoError(t, DB.Where("id = ?", quotaWriterEpochSingletonID).Delete(&QuotaWriterEpoch{}).Error)

	probeErr := errors.New("synthetic table probe failure")
	callbackName := "token-quota-table-probe-error"
	require.NoError(t, DB.Callback().Row().Before("gorm:row").Register(callbackName, func(tx *gorm.DB) {
		tx.AddError(probeErr)
	}))
	t.Cleanup(func() { _ = DB.Callback().Row().Remove(callbackName) })

	_, _, err = requested.UpdateWithQuota(newTokenQuotaAdjustmentCommandForTest(t, requested, requested.QuotaVersion, fingerprint))
	require.ErrorIs(t, err, ErrQuotaWriterEpochUnavailable)
	require.NotContains(t, err.Error(), probeErr.Error())
	var stored Token
	require.NoError(t, DB.First(&stored, fixture.Token.Id).Error)
	require.Equal(t, fixture.Token.RemainQuota, stored.RemainQuota)
	require.Equal(t, fixture.Token.QuotaVersion, stored.QuotaVersion)
}

func TestTokenCacheInvalidationIsAtomicAgainstDirectDeleteFailure(t *testing.T) {
	_, fixture, server := openLegacyBalanceBatchTestWithRedis(t, "token-cache-atomic-invalidation")
	require.True(t, server.Exists(getTokenCacheKey(fixture.Token.Key)))

	common.RDB.AddHook(rejectDirectTokenCacheDeleteHook{err: errors.New("synthetic direct DEL failure")})
	require.NoError(t, invalidateTokenCacheForMutation(fixture.Token.Key))
	require.True(t, server.Exists(getTokenCacheFenceKey(fixture.Token.Key)))
	require.False(t, server.Exists(getTokenCacheKey(fixture.Token.Key)))
}

func TestTokenCacheReadRejectsHashWhileMutationFenceExists(t *testing.T) {
	_, fixture, server := openLegacyBalanceBatchTestWithRedis(t, "token-cache-fenced-read")
	require.True(t, server.Exists(getTokenCacheKey(fixture.Token.Key)))
	require.NoError(t, DB.Model(&Token{}).Where("id = ?", fixture.Token.Id).Update("status", common.TokenStatusDisabled).Error)
	require.NoError(t, common.RDB.Set(context.Background(), getTokenCacheFenceKey(fixture.Token.Key), 1, time.Minute).Err())

	_, err := cacheGetTokenByKey(fixture.Token.Key)
	require.Error(t, err)
	loaded, err := GetTokenByKey(fixture.Token.Key, false)
	require.NoError(t, err)
	require.Equal(t, common.TokenStatusDisabled, loaded.Status)
}

func TestTokenMetadataMutationsRejectDatabaseWriteWhenCacheInvalidationFails(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Token) error
		verify func(*testing.T, Token)
	}{
		{
			name: "update",
			mutate: func(token *Token) error {
				token.Name = "must-not-be-written"
				return token.Update()
			},
			verify: func(t *testing.T, stored Token) {
				require.NotEqual(t, "must-not-be-written", stored.Name)
			},
		},
		{
			name: "select update",
			mutate: func(token *Token) error {
				token.Status = common.TokenStatusDisabled
				token.AccessedTime++
				return token.SelectUpdate()
			},
			verify: func(t *testing.T, stored Token) {
				require.Equal(t, common.TokenStatusEnabled, stored.Status)
				require.Equal(t, int64(0), stored.AccessedTime)
			},
		},
		{
			name: "delete",
			mutate: func(token *Token) error {
				return token.Delete()
			},
			verify: func(t *testing.T, stored Token) {
				require.False(t, stored.DeletedAt.Valid)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, fixture, server := openLegacyBalanceBatchTestWithRedis(t, "token-cache-fail-closed-"+test.name)
			server.SetError("ERR synthetic token cache invalidation failure")
			err := test.mutate(&fixture.Token)
			server.SetError("")
			require.Error(t, err)

			var stored Token
			require.NoError(t, DB.Unscoped().First(&stored, fixture.Token.Id).Error)
			test.verify(t, stored)
		})
	}
}

func TestTokenMetadataUpdateRefreshesFenceAfterSlowDatabaseMutation(t *testing.T) {
	_, fixture, server := openLegacyBalanceBatchTestWithRedis(t, "token-cache-slow-metadata-update")
	cacheKey := getTokenCacheKey(fixture.Token.Key)
	staleHash, err := common.RDB.HGetAll(context.Background(), cacheKey).Result()
	require.NoError(t, err)
	require.NotEmpty(t, staleHash)
	fixture.Token.Name = "metadata-after-slow-update"

	callbackName := "token-cache-expire-fence-during-metadata-update"
	injected := false
	require.NoError(t, DB.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if injected || tx.Statement.Table != "tokens" {
			return
		}
		injected = true
		server.FastForward(time.Duration(tokenCacheFenceSeconds+1) * time.Second)
		if err := common.RDB.HSet(context.Background(), cacheKey, staleHash).Err(); err != nil {
			tx.AddError(err)
			return
		}
		if err := common.RDB.Expire(context.Background(), cacheKey, time.Minute).Err(); err != nil {
			tx.AddError(err)
		}
	}))
	t.Cleanup(func() { _ = DB.Callback().Update().Remove(callbackName) })

	require.NoError(t, fixture.Token.Update())
	require.True(t, injected)
	require.True(t, server.Exists(getTokenCacheFenceKey(fixture.Token.Key)))
	require.False(t, server.Exists(getTokenCacheKey(fixture.Token.Key)))

	server.FastForward(time.Duration(tokenCacheFenceSeconds+1) * time.Second)
	loaded, err := GetTokenByKey(fixture.Token.Key, false)
	require.NoError(t, err)
	require.Equal(t, "metadata-after-slow-update", loaded.Name)
}

func TestTokenMetadataUpdateReportsPostCommitInvalidationFailure(t *testing.T) {
	_, fixture, server := openLegacyBalanceBatchTestWithRedis(t, "token-cache-post-commit-failure")
	count := 0
	common.RDB.AddHook(rejectNthTokenCacheEvalHook{
		count: &count, failAt: 3, err: errors.New("synthetic post-commit invalidation failure"),
	})
	fixture.Token.Name = "metadata-was-committed"

	err := fixture.Token.Update()
	require.ErrorIs(t, err, ErrTokenMetadataMutationCommittedCacheFailure)
	require.ErrorIs(t, err, ErrBatchQuotaCacheUnavailable)
	require.Equal(t, 3, count)

	var stored Token
	require.NoError(t, DB.First(&stored, fixture.Token.Id).Error)
	require.Equal(t, "metadata-was-committed", stored.Name)
	require.True(t, server.Exists(getTokenCacheFenceKey(fixture.Token.Key)))
	require.False(t, server.Exists(getTokenCacheKey(fixture.Token.Key)))
}
