package model

import (
	"context"
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQuotaScriptsRejectPartialMutation(t *testing.T) {
	for _, script := range []struct{ name, source string }{
		{"reserve", tokenQuotaReserveScript}, {"delta", tokenQuotaDeltaScript},
	} {
		t.Run(script.name, func(t *testing.T) {
			server := useUserCacheMiniRedis(t)
			ctx := context.Background()
			key := "quota-script-fixture"
			writerEpoch := int64(7)
			before := map[string]string{"Id": "42", "RemainQuota": "100", "UsedQuota": "bad", "AccessedTime": "99", "Name": "preserve", "QuotaWriterEpoch": strconv.FormatInt(writerEpoch, 10)}
			require.NoError(t, common.RDB.HSet(ctx, key, before).Err())
			require.NoError(t, common.RDB.Set(ctx, quotaWriterEpochRedisKey, writerEpoch, 0).Err())
			require.NoError(t, common.RDB.Expire(ctx, key, time.Minute).Err())
			ttl := server.TTL(key)
			result, err := common.RDB.Eval(ctx, script.source, []string{key, quotaWriterEpochRedisKey}, 10, 42, 123, writerEpoch).Int()
			status, err := quotaResultFromLua(result, err)
			assert.NoError(t, err)
			assert.Equal(t, cacheQuotaMiss, status, "malformed cache is not insufficient balance or a successful debit")
			after, readErr := common.RDB.HGetAll(ctx, key).Result()
			require.NoError(t, readErr)
			assert.Equal(t, before, after, "a failed second increment must not leave the first one applied")
			assert.Equal(t, ttl, server.TTL(key))
		})
	}
}

func TestUserCacheMetadataDoesNotCreateQuotaLessHash(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	server := useUserCacheMiniRedis(t)
	user := createReserveTestUser(t, 123)
	require.NoError(t, updateUserCache(user))
	assert.False(t, server.Exists(getUserCacheKey(user.Id)), "a metadata-only refresh must not create an incomplete user hash")
	loaded, err := GetUserCache(user.Id)
	require.NoError(t, err)
	assert.Equal(t, 123, loaded.Quota)
	assert.Equal(t, "123", server.HGet(getUserCacheKey(user.Id), "Quota"))
}

func TestQuotaScriptsRejectInvalidAmountsWithoutMutation(t *testing.T) {
	for _, script := range []struct {
		name, source string
		token        bool
	}{
		{"user-reserve", userQuotaReserveScript, false},
		{"user-delta", userQuotaDeltaScript, false},
		{"token-reserve", tokenQuotaReserveScript, true},
		{"token-delta", tokenQuotaDeltaScript, true},
	} {
		for _, amount := range []string{"", "bad", "NaN", "Inf", "1.5", "1e1", "01", "+1", "2147483648", "-2147483649"} {
			t.Run(script.name+"/"+amount, func(t *testing.T) {
				server := useUserCacheMiniRedis(t)
				ctx := context.Background()
				key := "quota-script-fixture"
				writerEpoch := int64(7)
				before := map[string]string{"Id": "42", "CacheSchema": strconv.Itoa(userCacheSchemaVersion), "Quota": "100", "RemainQuota": "100", "UsedQuota": "20", "AccessedTime": "99", "QuotaWriterEpoch": strconv.FormatInt(writerEpoch, 10)}
				require.NoError(t, common.RDB.HSet(ctx, key, before).Err())
				require.NoError(t, common.RDB.Set(ctx, quotaWriterEpochRedisKey, writerEpoch, 0).Err())
				require.NoError(t, common.RDB.Expire(ctx, key, time.Minute).Err())
				ttl := server.TTL(key)
				third := userCacheSchemaVersion
				if script.token {
					third = 123
				}
				result, evalErr := common.RDB.Eval(ctx, script.source, []string{key, quotaWriterEpochRedisKey}, amount, 42, third, writerEpoch).Int()
				status, err := quotaResultFromLua(result, evalErr)
				assert.Error(t, err, "invalid amount must not be classified as a valid balance result")
				assert.NotEqual(t, cacheQuotaOK, status)
				after, readErr := common.RDB.HGetAll(ctx, key).Result()
				require.NoError(t, readErr)
				assert.Equal(t, before, after)
				assert.Equal(t, ttl, server.TTL(key))
			})
		}
	}
}

func TestQuotaCacheGoAmountBounds(t *testing.T) {
	for _, delta := range []int64{math.MinInt64, int64(common.MinQuota) - 1, int64(common.MaxQuota) + 1, math.MaxInt64} {
		for _, operation := range []struct {
			name string
			call func() (cacheQuotaResult, error)
		}{
			{"user-reserve", func() (cacheQuotaResult, error) { return cacheTryReserveUserQuota(42, delta) }},
			{"user-delta", func() (cacheQuotaResult, error) { return cacheApplyUserQuotaDelta(42, delta) }},
			{"token-reserve", func() (cacheQuotaResult, error) { return cacheTryReserveTokenQuota(42, "fixture", delta) }},
			{"token-delta", func() (cacheQuotaResult, error) { return cacheApplyTokenQuotaDelta(42, "fixture", delta) }},
		} {
			t.Run(operation.name+"/"+strconv.FormatInt(delta, 10), func(t *testing.T) {
				status, err := operation.call()
				assert.ErrorIs(t, err, errInvalidQuotaCacheOperation)
				assert.NotEqual(t, cacheQuotaOK, status)
			})
		}
	}
	if strconv.IntSize == 64 {
		oversized := int64(common.MaxQuota) + 1
		ok, err := TryReserveUserQuota(42, int(oversized))
		assert.False(t, ok)
		assert.ErrorIs(t, err, errInvalidQuotaCacheOperation)
		for _, unlimited := range []bool{false, true} {
			ok, err = TryReserveTokenQuota(42, "fixture", int(oversized), unlimited)
			assert.False(t, ok)
			assert.ErrorIs(t, err, errInvalidQuotaCacheOperation)
		}
	}
}

func TestQuotaCacheOverflowDoesNotFallBackToDatabase(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	server := useUserCacheMiniRedis(t)
	token := createReserveTestToken(t, 100)
	_, err := GetTokenByKey(token.Key, true)
	require.NoError(t, err)
	server.HSet(getTokenCacheKey(token.Key), "UsedQuota", strconv.Itoa(common.MaxQuota))
	reserved, err := TryReserveTokenQuota(token.Id, token.Key, 10, false)
	assert.False(t, reserved)
	assert.ErrorIs(t, err, errInvalidQuotaCacheOperation)
	stored := getTokenFromDB(t, token.Id)
	assert.Equal(t, 100, stored.RemainQuota)
	assert.Zero(t, stored.UsedQuota)
	assert.Equal(t, "100", server.HGet(getTokenCacheKey(token.Key), "RemainQuota"))
	assert.Equal(t, strconv.Itoa(common.MaxQuota), server.HGet(getTokenCacheKey(token.Key), "UsedQuota"))
}
