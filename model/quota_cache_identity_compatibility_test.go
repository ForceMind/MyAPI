package model

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func quotaCacheHashSnapshot(t *testing.T, key string) (map[string]string, time.Duration) {
	t.Helper()
	values, err := common.RDB.HGetAll(context.Background(), key).Result()
	require.NoError(t, err)
	ttl, err := common.RDB.TTL(context.Background(), key).Result()
	require.NoError(t, err)
	return values, ttl
}

func requireQuotaCacheHashUnchanged(t *testing.T, key string, before map[string]string, ttl time.Duration) {
	t.Helper()
	after, err := common.RDB.HGetAll(context.Background(), key).Result()
	require.NoError(t, err)
	assert.Equal(t, before, after)
	afterTTL, err := common.RDB.TTL(context.Background(), key).Result()
	require.NoError(t, err)
	assert.Equal(t, ttl, afterTTL)
}

func TestUserCacheMetadataRejectsMismatchedIdentityWithoutWrites(t *testing.T) {
	_, fixture, _ := openLegacyBalanceBatchTestWithRedis(t, "user-cache-identity")
	ctx := context.Background()
	key := getUserCacheKey(fixture.User.Id)
	require.NoError(t, common.RDB.HSet(ctx, key, "Id", fixture.User.Id+1000).Err())
	before, ttl := quotaCacheHashSnapshot(t, key)
	committedBefore, err := common.RDB.Get(ctx, getUserAuthVersionKey(fixture.User.Id)).Result()
	require.NoError(t, err)

	updated := fixture.User
	updated.Group = "identity-must-not-change"
	updated.AuthVersion++
	err = writeUserCache(updated.ToBaseUser(), false)
	require.ErrorIs(t, err, ErrQuotaCacheIdentityMismatch)
	requireQuotaCacheHashUnchanged(t, key, before, ttl)
	committedAfter, err := common.RDB.Get(ctx, getUserAuthVersionKey(fixture.User.Id)).Result()
	require.NoError(t, err)
	assert.Equal(t, committedBefore, committedAfter)
}

func TestUserCacheMetadataRejectsIncompleteQuotaStateWithoutWrites(t *testing.T) {
	_, fixture, _ := openLegacyBalanceBatchTestWithRedis(t, "user-cache-incomplete")
	ctx := context.Background()
	key := getUserCacheKey(fixture.User.Id)
	require.NoError(t, common.RDB.HDel(ctx, key, "QuotaVersion", "QuotaWriterEpoch").Err())
	before, ttl := quotaCacheHashSnapshot(t, key)
	committedBefore, err := common.RDB.Get(ctx, getUserAuthVersionKey(fixture.User.Id)).Result()
	require.NoError(t, err)

	updated := fixture.User
	updated.Group = "incomplete-must-not-change"
	updated.AuthVersion++
	err = writeUserCache(updated.ToBaseUser(), false)
	require.Error(t, err)
	requireQuotaCacheHashUnchanged(t, key, before, ttl)
	committedAfter, err := common.RDB.Get(ctx, getUserAuthVersionKey(fixture.User.Id)).Result()
	require.NoError(t, err)
	assert.Equal(t, committedBefore, committedAfter)
}

func TestTokenCacheInitRejectsMismatchedIdentityWithoutWrites(t *testing.T) {
	_, fixture, _ := openLegacyBalanceBatchTestWithRedis(t, "token-cache-identity")
	ctx := context.Background()
	key := getTokenCacheKey(fixture.Token.Key)
	require.NoError(t, common.RDB.HSet(ctx, key, "Id", fixture.Token.Id+1000).Err())
	before, ttl := quotaCacheHashSnapshot(t, key)

	_, err := cacheInitToken(fixture.Token)
	require.ErrorIs(t, err, ErrQuotaCacheIdentityMismatch)
	requireQuotaCacheHashUnchanged(t, key, before, ttl)
}

func TestTokenCacheInitRejectsMalformedSameSchemaBeforeEpochWrites(t *testing.T) {
	_, fixture, _ := openLegacyBalanceBatchTestWithRedis(t, "token-cache-malformed")
	ctx := context.Background()
	key := getTokenCacheKey(fixture.Token.Key)
	state, err := GetQuotaWriterEpochState(DB)
	require.NoError(t, err)
	require.Greater(t, state.Epoch, int64(1))
	staleEpoch := state.Epoch - 1
	require.NoError(t, common.RDB.Set(ctx, quotaWriterEpochRedisKey, staleEpoch, 0).Err())
	require.NoError(t, common.RDB.HSet(ctx, key, "QuotaWriterEpoch", staleEpoch, "UsedQuota", "invalid").Err())
	before, ttl := quotaCacheHashSnapshot(t, key)

	_, err = cacheInitToken(fixture.Token)
	require.Error(t, err)
	requireQuotaCacheHashUnchanged(t, key, before, ttl)
	globalEpoch, err := common.RDB.Get(ctx, quotaWriterEpochRedisKey).Int64()
	require.NoError(t, err)
	assert.Equal(t, staleEpoch, globalEpoch)
}

func TestQuotaCacheHydrationRejectsMismatchedIdentityWithoutWrites(t *testing.T) {
	_, fixture, _ := openLegacyBalanceBatchTestWithRedis(t, "quota-cache-hydrate-identity")
	require.NoError(t, DB.Model(&QuotaWriterEpoch{}).Where("id = ?", 1).Update("mode", string(QuotaWriterModeAuthoritative)).Error)
	ctx := context.Background()

	userKey := getUserCacheKey(fixture.User.Id)
	require.NoError(t, common.RDB.HSet(ctx, userKey, "Id", fixture.User.Id+1000).Err())
	userBefore, userTTL := quotaCacheHashSnapshot(t, userKey)
	err := HydrateUserQuotaCache(fixture.User.Id, fixture.User.Quota, fixture.User.QuotaVersion+1)
	require.ErrorIs(t, err, ErrQuotaCacheIdentityMismatch)
	requireQuotaCacheHashUnchanged(t, userKey, userBefore, userTTL)

	tokenKey := getTokenCacheKey(fixture.Token.Key)
	require.NoError(t, common.RDB.HSet(ctx, tokenKey, "Id", fixture.Token.Id+1000).Err())
	tokenBefore, tokenTTL := quotaCacheHashSnapshot(t, tokenKey)
	err = HydrateTokenQuotaCache(fixture.Token.Key, fixture.Token.Id, fixture.Token.RemainQuota, fixture.Token.UsedQuota, fixture.Token.QuotaVersion+1)
	require.ErrorIs(t, err, ErrQuotaCacheIdentityMismatch)
	requireQuotaCacheHashUnchanged(t, tokenKey, tokenBefore, tokenTTL)
}

func TestUserCacheFieldUpdateRejectsMismatchedIdentityWithoutWrites(t *testing.T) {
	_, fixture, _ := openLegacyBalanceBatchTestWithRedis(t, "user-cache-field-identity")
	ctx := context.Background()
	key := getUserCacheKey(fixture.User.Id)
	require.NoError(t, common.RDB.HSet(ctx, key, "Id", fixture.User.Id+1000).Err())
	before, ttl := quotaCacheHashSnapshot(t, key)

	err := updateUserCacheFieldAtVersion(fixture.User.Id, "Group", "new-group", fixture.User.AuthVersion)
	require.ErrorIs(t, err, ErrQuotaCacheIdentityMismatch)
	requireQuotaCacheHashUnchanged(t, key, before, ttl)
}

func TestUserCacheFieldUpdateInvalidatesOnlyTouchedLegacySchema(t *testing.T) {
	_, fixture, server := openLegacyBalanceBatchTestWithRedis(t, "user-cache-field-schema")
	ctx := context.Background()
	key := getUserCacheKey(fixture.User.Id)
	const sentinel = "quota-cache-field-schema:sentinel"
	require.NoError(t, common.RDB.Set(ctx, sentinel, "keep", 0).Err())
	require.NoError(t, common.RDB.HSet(ctx, key, "CacheSchema", strconv.Itoa(userCacheSchemaVersion-1)).Err())
	committedBefore, err := common.RDB.Get(ctx, getUserAuthVersionKey(fixture.User.Id)).Result()
	require.NoError(t, err)

	require.NoError(t, updateUserCacheFieldAtVersion(fixture.User.Id, "Group", "new-group", fixture.User.AuthVersion+1))
	assert.False(t, server.Exists(key))
	value, err := server.Get(sentinel)
	require.NoError(t, err)
	assert.Equal(t, "keep", value)
	committedAfter, err := common.RDB.Get(ctx, getUserAuthVersionKey(fixture.User.Id)).Result()
	require.NoError(t, err)
	assert.Equal(t, committedBefore, committedAfter)
}
