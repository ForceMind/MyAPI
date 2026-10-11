package model

import (
	"math"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type historicalRedisHashSnapshot struct {
	values    map[string]string
	expiresAt int64
}

func runHistoricalAmountRedisContract(t *testing.T, client *redis.Client) {
	t.Helper()

	t.Run("wide-arithmetic-and-rejection-atomicity", func(t *testing.T) {
		useHistoricalAmountRedisFixture(t, client, 101)
		ctx := t.Context()

		userKey := "myapi:s2c:historical:wide:user:9301"
		userEpochKey := userKey + ":writer-epoch"
		require.NoError(t, client.HSet(ctx, userKey, map[string]string{
			"Id": "9301", "CacheSchema": strconv.Itoa(userCacheSchemaVersion),
			"Quota": strconv.FormatInt(common.MaxQuotaBalance, 10), "QuotaWriterEpoch": "101",
		}).Err())
		require.NoError(t, client.Set(ctx, userEpochKey, "101", 0).Err())
		require.NoError(t, client.Expire(ctx, userKey, time.Minute).Err())
		userExpiry := historicalRedisExpiry(t, client, userKey)
		result, err := client.Eval(ctx, userQuotaDeltaScript,
			[]string{userKey, userEpochKey}, -1, 9301, userCacheSchemaVersion, 101).Int()
		require.NoError(t, err)
		require.Equal(t, 1, result)
		require.Equal(t, strconv.FormatInt(common.MaxQuotaBalance-1, 10), client.HGet(ctx, userKey, "Quota").Val())
		require.Equal(t, userExpiry, historicalRedisExpiry(t, client, userKey))

		tokenKey := "myapi:s2c:historical:wide:token:9302"
		tokenEpochKey := tokenKey + ":writer-epoch"
		require.NoError(t, client.HSet(ctx, tokenKey, map[string]string{
			"Id": "9302", "CacheSchema": strconv.Itoa(tokenCacheSchemaVersion),
			"RemainQuota":  strconv.FormatInt(common.MaxQuotaBalance-1, 10),
			"UsedQuota":    strconv.FormatInt(common.MaxQuotaBalance, 10),
			"AccessedTime": "123", "QuotaWriterEpoch": "101",
		}).Err())
		require.NoError(t, client.Set(ctx, tokenEpochKey, "101", 0).Err())
		require.NoError(t, client.Expire(ctx, tokenKey, time.Minute).Err())
		tokenExpiry := historicalRedisExpiry(t, client, tokenKey)
		result, err = client.Eval(ctx, tokenQuotaDeltaScript,
			[]string{tokenKey, tokenEpochKey}, 1, 9302, 456, 101).Int()
		require.NoError(t, err)
		require.Equal(t, 1, result)
		require.Equal(t, []interface{}{
			strconv.FormatInt(common.MaxQuotaBalance, 10),
			strconv.FormatInt(common.MaxQuotaBalance-1, 10), "456",
		}, client.HMGet(ctx, tokenKey, "RemainQuota", "UsedQuota", "AccessedTime").Val())
		require.Equal(t, tokenExpiry, historicalRedisExpiry(t, client, tokenKey))

		for _, tc := range []struct {
			name   string
			key    string
			script string
			amount interface{}
			fields map[string]string
		}{
			{
				name: "reserve-used-overflow", key: "myapi:s2c:historical:reject:9303", script: tokenQuotaReserveScript, amount: 1,
				fields: map[string]string{"Id": "9303", "CacheSchema": strconv.Itoa(tokenCacheSchemaVersion), "RemainQuota": "100", "UsedQuota": strconv.FormatInt(common.MaxQuotaBalance, 10), "AccessedTime": "123", "QuotaWriterEpoch": "101"},
			},
			{
				name: "delta-used-underflow", key: "myapi:s2c:historical:reject:9304", script: tokenQuotaDeltaScript, amount: 1,
				fields: map[string]string{"Id": "9304", "CacheSchema": strconv.Itoa(tokenCacheSchemaVersion), "RemainQuota": strconv.FormatInt(common.MaxQuotaBalance-1, 10), "UsedQuota": "0", "AccessedTime": "123", "QuotaWriterEpoch": "101"},
			},
			{
				name: "delta-unrepresentable-inverse", key: "myapi:s2c:historical:reject:9305", script: tokenQuotaDeltaScript, amount: common.MinQuota,
				fields: map[string]string{"Id": "9305", "CacheSchema": strconv.Itoa(tokenCacheSchemaVersion), "RemainQuota": "5000000000", "UsedQuota": "5000000000", "AccessedTime": "123", "QuotaWriterEpoch": "101"},
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				epochKey := tc.key + ":writer-epoch"
				require.NoError(t, client.HSet(ctx, tc.key, tc.fields).Err())
				require.NoError(t, client.Expire(ctx, tc.key, time.Minute).Err())
				before := historicalRedisSnapshot(t, client, tc.key)
				result, evalErr := client.Eval(ctx, tc.script,
					[]string{tc.key, epochKey}, tc.amount, tc.fields["Id"], 456, 101).Int()
				status, quotaErr := quotaResultFromLua(result, evalErr)
				require.ErrorIs(t, quotaErr, errInvalidQuotaCacheOperation)
				require.NotEqual(t, cacheQuotaOK, status)
				requireHistoricalRedisSnapshot(t, client, tc.key, before)
				exists, existsErr := client.Exists(ctx, epochKey).Result()
				require.NoError(t, existsErr)
				require.Zero(t, exists, "target validation must finish before the first Redis write")
			})
		}
	})

	t.Run("exact-version-ordering", func(t *testing.T) {
		useHistoricalAmountRedisFixture(t, client, 102)
		ctx := t.Context()
		const current = int64(1 << 53)
		const next = current + 1

		writerUserID := 9311
		writerKey := getUserCacheKey(writerUserID)
		require.NoError(t, client.HSet(ctx, writerKey, map[string]string{
			"Id": strconv.Itoa(writerUserID), "CacheSchema": strconv.Itoa(userCacheSchemaVersion),
			"Quota": "100", "QuotaVersion": "99", "QuotaWriterEpoch": strconv.FormatInt(current, 10),
		}).Err())
		require.NoError(t, client.Set(ctx, quotaWriterEpochRedisKey, current, 0).Err())
		require.NoError(t, client.Expire(ctx, writerKey, time.Minute).Err())
		require.NoError(t, hydrateUserQuotaCacheRedisAtEpoch(writerUserID, 200, 1, next))
		require.Equal(t, []interface{}{"200", "1", strconv.FormatInt(next, 10)},
			client.HMGet(ctx, writerKey, "Quota", "QuotaVersion", "QuotaWriterEpoch").Val())
		require.Equal(t, strconv.FormatInt(next, 10), client.Get(ctx, quotaWriterEpochRedisKey).Val())

		userID := 9312
		userKey := getUserCacheKey(userID)
		require.NoError(t, client.HSet(ctx, userKey, map[string]string{
			"Id": strconv.Itoa(userID), "CacheSchema": strconv.Itoa(userCacheSchemaVersion),
			"Quota": "100", "QuotaVersion": strconv.FormatInt(current, 10), "QuotaWriterEpoch": strconv.FormatInt(next, 10),
		}).Err())
		require.NoError(t, client.Expire(ctx, userKey, time.Minute).Err())
		require.NoError(t, hydrateUserQuotaCacheRedisAtEpoch(userID, 200, next, next))
		require.Equal(t, []interface{}{"200", strconv.FormatInt(next, 10)}, client.HMGet(ctx, userKey, "Quota", "QuotaVersion").Val())

		tokenID := 9313
		tokenName := "s2c-historical-version-token-9313"
		tokenKey := getTokenCacheKey(tokenName)
		require.NoError(t, client.HSet(ctx, tokenKey, map[string]string{
			"Id": strconv.Itoa(tokenID), "CacheSchema": strconv.Itoa(tokenCacheSchemaVersion),
			"RemainQuota": "100", "UsedQuota": "10", "QuotaVersion": strconv.FormatInt(current, 10), "QuotaWriterEpoch": strconv.FormatInt(next, 10),
		}).Err())
		require.NoError(t, client.Expire(ctx, tokenKey, time.Minute).Err())
		require.NoError(t, hydrateTokenQuotaCacheRedisAtEpoch(tokenName, tokenID, 90, 20, next, next))
		require.Equal(t, []interface{}{"90", "20", strconv.FormatInt(next, 10)},
			client.HMGet(ctx, tokenKey, "RemainQuota", "UsedQuota", "QuotaVersion").Val())

		authKey := getUserAuthFenceKey(9314)
		require.NoError(t, client.Set(ctx, authKey, current, 0).Err())
		require.NoError(t, SetUserAuthVersionFence(9314, next))
		require.Equal(t, strconv.FormatInt(next, 10), client.Get(ctx, authKey).Val())

		maxTokenID := 9315
		maxTokenName := "s2c-historical-max-version-token-9315"
		maxTokenKey := getTokenCacheKey(maxTokenName)
		require.NoError(t, client.HSet(ctx, maxTokenKey, map[string]string{
			"Id": strconv.Itoa(maxTokenID), "CacheSchema": strconv.Itoa(tokenCacheSchemaVersion),
			"RemainQuota": "300", "UsedQuota": "30", "QuotaVersion": strconv.FormatInt(math.MaxInt64-1, 10), "QuotaWriterEpoch": strconv.FormatInt(next, 10),
		}).Err())
		require.NoError(t, client.Expire(ctx, maxTokenKey, time.Minute).Err())
		require.NoError(t, hydrateTokenQuotaCacheRedisAtEpoch(maxTokenName, maxTokenID, 301, 29, math.MaxInt64, next))
		require.Equal(t, []interface{}{"301", "29", strconv.FormatInt(math.MaxInt64, 10)},
			client.HMGet(ctx, maxTokenKey, "RemainQuota", "UsedQuota", "QuotaVersion").Val())
		require.NoError(t, hydrateTokenQuotaCacheRedisAtEpoch(maxTokenName, maxTokenID, 1, 1, next, next))
		require.Equal(t, []interface{}{"301", "29", strconv.FormatInt(math.MaxInt64, 10)},
			client.HMGet(ctx, maxTokenKey, "RemainQuota", "UsedQuota", "QuotaVersion").Val())
		require.Positive(t, historicalRedisExpiry(t, client, maxTokenKey))

		maxAuthKey := getUserAuthFenceKey(9316)
		require.NoError(t, client.Set(ctx, maxAuthKey, int64(math.MaxInt64-1), 0).Err())
		require.NoError(t, SetUserAuthVersionFence(9316, math.MaxInt64))
		require.NoError(t, SetUserAuthVersionFence(9316, next))
		require.Equal(t, strconv.FormatInt(math.MaxInt64, 10), client.Get(ctx, maxAuthKey).Val())

		maxEpochUserID := 9317
		maxEpochKey := getUserCacheKey(maxEpochUserID)
		require.NoError(t, client.HSet(ctx, maxEpochKey, map[string]string{
			"Id": strconv.Itoa(maxEpochUserID), "CacheSchema": strconv.Itoa(userCacheSchemaVersion),
			"Quota": "400", "QuotaVersion": "8", "QuotaWriterEpoch": strconv.FormatInt(math.MaxInt64-1, 10),
		}).Err())
		require.NoError(t, client.Set(ctx, quotaWriterEpochRedisKey, int64(math.MaxInt64-1), 0).Err())
		require.NoError(t, client.Expire(ctx, maxEpochKey, time.Minute).Err())
		require.NoError(t, hydrateUserQuotaCacheRedisAtEpoch(maxEpochUserID, 401, 9, math.MaxInt64))
		require.Equal(t, []interface{}{"401", "9", strconv.FormatInt(math.MaxInt64, 10)},
			client.HMGet(ctx, maxEpochKey, "Quota", "QuotaVersion", "QuotaWriterEpoch").Val())
		require.Equal(t, strconv.FormatInt(math.MaxInt64, 10), client.Get(ctx, quotaWriterEpochRedisKey).Val())
		before := historicalRedisSnapshot(t, client, maxEpochKey)
		require.NoError(t, hydrateUserQuotaCacheRedisAtEpoch(maxEpochUserID, 1, math.MaxInt64, next))
		requireHistoricalRedisSnapshot(t, client, maxEpochKey, before)
		require.Equal(t, strconv.FormatInt(math.MaxInt64, 10), client.Get(ctx, quotaWriterEpochRedisKey).Val())
	})

	t.Run("cold-publication-and-trust", func(t *testing.T) {
		db := useHistoricalAmountRedisFixture(t, client, 103)
		ctx := t.Context()
		const userID = 9321
		const tokenID = 9322
		const tokenName = "s2c-historical-cold-token-9322"
		allowIPs := ""
		user := User{
			Id: userID, Username: "s2c-historical-cold-user", Password: "fixture-password",
			Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Group: "default", AccountTierID: "standard",
			Quota: common.MaxQuotaBalance, QuotaVersion: (1 << 53) + 1, AuthVersion: (1 << 53) + 1,
		}
		token := Token{
			Id: tokenID, UserId: userID, Key: tokenName, Name: "historical-cold-token",
			Status: common.TokenStatusEnabled, ExpiredTime: -1, Group: "default", AccessProfileID: "standard", AllowIps: &allowIPs,
			RemainQuota: common.MaxQuotaBalance, UsedQuota: 25, QuotaVersion: math.MaxInt64,
		}
		require.NoError(t, db.Create(&user).Error)
		require.NoError(t, db.Create(&token).Error)

		require.NoError(t, writeUserCache(user.ToBaseUser(), true))
		published, err := cacheInitToken(token)
		require.NoError(t, err)
		require.Zero(t, published)
		loaded, err := GetTokenByKey(tokenName, false)
		require.NoError(t, err)
		require.Equal(t, tokenID, loaded.Id)
		for _, key := range []string{getUserCacheKey(userID), getTokenCacheKey(tokenName)} {
			exists, existsErr := client.Exists(ctx, key).Result()
			require.NoError(t, existsErr)
			require.Zero(t, exists, "an unowned cold read must not publish accounting state")
		}

		seedUserQuotaCacheForTest(t, user.ToBaseUser())
		seedTokenQuotaCacheForTest(t, token)
		userKey, tokenKey := getUserCacheKey(userID), getTokenCacheKey(tokenName)
		require.Equal(t, []interface{}{
			strconv.FormatInt(common.MaxQuotaBalance, 10), strconv.FormatInt(user.QuotaVersion, 10), "103",
		}, client.HMGet(ctx, userKey, "Quota", "QuotaVersion", "QuotaWriterEpoch").Val())
		require.Equal(t, []interface{}{
			strconv.FormatInt(common.MaxQuotaBalance, 10), "25", strconv.FormatInt(math.MaxInt64, 10), "103",
		}, client.HMGet(ctx, tokenKey, "RemainQuota", "UsedQuota", "QuotaVersion", "QuotaWriterEpoch").Val())

		beforeUser := historicalRedisSnapshot(t, client, userKey)
		beforeToken := historicalRedisSnapshot(t, client, tokenKey)
		beforeKeys := historicalRedisKeys(t, client)
		eligible, err := LegacyBatchQuotaTrustEligible(userID, tokenID, tokenName, false, 103, 100)
		require.NoError(t, err)
		require.True(t, eligible)
		requireHistoricalRedisSnapshot(t, client, userKey, beforeUser)
		requireHistoricalRedisSnapshot(t, client, tokenKey, beforeToken)
		require.Equal(t, beforeKeys, historicalRedisKeys(t, client))

		require.NoError(t, client.HSet(ctx, tokenKey, "RemainQuota", strconv.FormatInt(common.MaxQuotaBalance+1, 10)).Err())
		boundaryUser := historicalRedisSnapshot(t, client, userKey)
		boundaryToken := historicalRedisSnapshot(t, client, tokenKey)
		boundaryKeys := historicalRedisKeys(t, client)
		eligible, err = LegacyBatchQuotaTrustEligible(userID, tokenID, tokenName, false, 103, 100)
		require.NoError(t, err)
		require.False(t, eligible)
		requireHistoricalRedisSnapshot(t, client, userKey, boundaryUser)
		requireHistoricalRedisSnapshot(t, client, tokenKey, boundaryToken)
		require.Equal(t, boundaryKeys, historicalRedisKeys(t, client))
		require.NoError(t, client.HSet(ctx, tokenKey, "RemainQuota", strconv.FormatInt(common.MaxQuotaBalance, 10)).Err())

		sentinel := "myapi:s2c:historical:cold:sentinel:9321"
		require.NoError(t, client.Set(ctx, sentinel, "keep", 0).Err())
		require.NoError(t, client.HSet(ctx, userKey, "CacheSchema", userCacheSchemaVersion-1).Err())
		refresh := *user.ToBaseUser()
		refresh.Username = "metadata-refresh-must-stay-cold"
		require.NoError(t, writeUserCache(&refresh, true))
		require.Zero(t, historicalRedisExists(t, client, userKey))
		require.Equal(t, "keep", client.Get(ctx, sentinel).Val())
		require.EqualValues(t, 1, historicalRedisExists(t, client, tokenKey))

		require.NoError(t, client.HSet(ctx, tokenKey, "CacheSchema", tokenCacheSchemaVersion-1).Err())
		published, err = cacheInitToken(token)
		require.NoError(t, err)
		require.Zero(t, published)
		require.Zero(t, historicalRedisExists(t, client, tokenKey))
		require.Equal(t, "keep", client.Get(ctx, sentinel).Val())
	})
}

func useHistoricalAmountRedisFixture(t *testing.T, client *redis.Client, writerEpoch int64) *gorm.DB {
	t.Helper()
	db := openQuotaProjectionTestDB(t)
	useQuotaProjectionModelDB(t, db)
	setQuotaWriterStateForTest(t, db, QuotaWriterModeLegacy, writerEpoch)
	oldRedisEnabled, oldRDB, oldSyncFrequency, oldBatchUpdate := common.RedisEnabled, common.RDB, common.SyncFrequency, common.BatchUpdateEnabled
	common.RedisEnabled, common.RDB, common.SyncFrequency, common.BatchUpdateEnabled = true, client, 60, false
	t.Cleanup(func() {
		common.RedisEnabled, common.RDB, common.SyncFrequency, common.BatchUpdateEnabled = oldRedisEnabled, oldRDB, oldSyncFrequency, oldBatchUpdate
	})
	require.NoError(t, client.Set(t.Context(), quotaWriterEpochRedisKey, writerEpoch, 0).Err())
	return db
}

func historicalRedisSnapshot(t *testing.T, client *redis.Client, key string) historicalRedisHashSnapshot {
	t.Helper()
	values, err := client.HGetAll(t.Context(), key).Result()
	require.NoError(t, err)
	return historicalRedisHashSnapshot{values: values, expiresAt: historicalRedisExpiry(t, client, key)}
}

func requireHistoricalRedisSnapshot(t *testing.T, client *redis.Client, key string, before historicalRedisHashSnapshot) {
	t.Helper()
	after := historicalRedisSnapshot(t, client, key)
	require.Equal(t, before, after)
}

func historicalRedisExpiry(t *testing.T, client *redis.Client, key string) int64 {
	t.Helper()
	expiresAt, err := client.Do(t.Context(), "PEXPIRETIME", key).Int64()
	require.NoError(t, err)
	return expiresAt
}

func historicalRedisExists(t *testing.T, client *redis.Client, key string) int64 {
	t.Helper()
	exists, err := client.Exists(t.Context(), key).Result()
	require.NoError(t, err)
	return exists
}

func historicalRedisKeys(t *testing.T, client *redis.Client) []string {
	t.Helper()
	keys, err := client.Keys(t.Context(), "*").Result()
	require.NoError(t, err)
	sort.Strings(keys)
	return keys
}
