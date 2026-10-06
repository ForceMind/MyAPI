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

func TestUserCacheQuotaProjectionAntiRollback(t *testing.T) {
	truncateTables(t)
	server := useUserCacheMiniRedis(t)
	ctx := context.Background()

	const (
		userID       = 9101
		initialEpoch = int64(10)
	)
	key := getUserCacheKey(userID)

	// 1. Pre-populate a complete live user hash and the matching global epoch.
	initialHash := map[string]string{
		"Id":               strconv.Itoa(userID),
		"Group":            "default",
		"AccountTierID":    "standard",
		"Email":            "user@example.com",
		"Status":           strconv.Itoa(common.UserStatusEnabled),
		"Role":             strconv.Itoa(common.RoleCommonUser),
		"Username":         "tester",
		"Setting":          "{}",
		"AuthVersion":      "1",
		"CacheSchema":      strconv.Itoa(userCacheSchemaVersion),
		"Quota":            "1000",
		"QuotaVersion":     "10",
		"QuotaWriterEpoch": strconv.FormatInt(initialEpoch, 10),
	}
	require.NoError(t, common.RDB.HSet(ctx, key, initialHash).Err())
	require.NoError(t, common.RDB.Set(ctx, quotaWriterEpochRedisKey, initialEpoch, 0).Err())
	require.NoError(t, common.RDB.Expire(ctx, key, time.Minute).Err())

	// 2. A lower epoch is rejected even with a higher QuotaVersion.
	require.NoError(t, hydrateUserQuotaCacheRedisAtEpoch(userID, 800, 99, initialEpoch-1))
	assert.Equal(t, "1000", server.HGet(key, "Quota"), "lower epoch must not roll back the cache")
	assert.Equal(t, "10", server.HGet(key, "QuotaVersion"))
	assert.Equal(t, "10", server.HGet(key, "QuotaWriterEpoch"))

	// 3. A higher epoch wins even with a lower QuotaVersion.
	const currentEpoch = initialEpoch + 1
	require.NoError(t, hydrateUserQuotaCacheRedisAtEpoch(userID, 900, 1, currentEpoch))
	assert.Equal(t, "900", server.HGet(key, "Quota"))
	assert.Equal(t, "1", server.HGet(key, "QuotaVersion"))
	assert.Equal(t, strconv.FormatInt(currentEpoch, 10), server.HGet(key, "QuotaWriterEpoch"))
	globalEpoch, err := server.Get(quotaWriterEpochRedisKey)
	require.NoError(t, err)
	assert.Equal(t, strconv.FormatInt(currentEpoch, 10), globalEpoch)

	// 4. Within one epoch, only a strictly higher QuotaVersion is accepted.
	require.NoError(t, hydrateUserQuotaCacheRedisAtEpoch(userID, 1200, 2, currentEpoch))
	assert.Equal(t, "1200", server.HGet(key, "Quota"))
	assert.Equal(t, "2", server.HGet(key, "QuotaVersion"))

	// 5. Equal epoch and QuotaVersion must not overwrite a live projection.
	require.NoError(t, hydrateUserQuotaCacheRedisAtEpoch(userID, 1100, 2, currentEpoch))
	assert.Equal(t, "1200", server.HGet(key, "Quota"), "same version must not overwrite live quota")
	assert.Equal(t, "2", server.HGet(key, "QuotaVersion"))

	// 6. An old snapshot arriving after the advance cannot roll back the cache.
	require.NoError(t, hydrateUserQuotaCacheRedisAtEpoch(userID, 700, 100, initialEpoch))
	assert.Equal(t, "1200", server.HGet(key, "Quota"), "old snapshot must not roll back the cache")
	assert.Equal(t, "2", server.HGet(key, "QuotaVersion"))
	assert.Equal(t, strconv.FormatInt(currentEpoch, 10), server.HGet(key, "QuotaWriterEpoch"))

	// 7. Hydration on a missing cache must not create an incomplete hash.
	require.NoError(t, common.RDB.Del(ctx, key).Err())
	require.NoError(t, hydrateUserQuotaCacheRedisAtEpoch(userID, 1500, 3, currentEpoch))
	assert.False(t, server.Exists(key), "hydration on missing cache must not create a partial hash")
}

func TestWriteUserCacheQuotaAntiRollback(t *testing.T) {
	truncateTables(t)
	server := useUserCacheMiniRedis(t)
	ctx := context.Background()

	const userID = 9102
	key := getUserCacheKey(userID)

	// 1. Initial complete write with QuotaVersion=5
	base := &UserBase{
		Id:            userID,
		Group:         "default",
		AccountTierID: "standard",
		Email:         "u2@example.com",
		Quota:         500,
		Status:        common.UserStatusEnabled,
		Role:          common.RoleCommonUser,
		Username:      "u2",
		Setting:       "{}",
		AuthVersion:   1,
		CacheSchema:   userCacheSchemaVersion,
		QuotaVersion:  5,
	}
	require.NoError(t, writeUserCache(base, true))

	assert.Equal(t, "500", server.HGet(key, "Quota"))
	assert.Equal(t, "5", server.HGet(key, "QuotaVersion"))

	// 2. An older snapshot with QuotaVersion=3 arriving later must NOT roll back Quota and QuotaVersion,
	// but can update metadata (AuthVersion etc)
	staleSnapshot := &UserBase{
		Id:            userID,
		Group:         "vip",
		AccountTierID: "standard",
		Email:         "u2@example.com",
		Quota:         300,
		Status:        common.UserStatusEnabled,
		Role:          common.RoleCommonUser,
		Username:      "u2-renamed",
		Setting:       "{}",
		AuthVersion:   1,
		CacheSchema:   userCacheSchemaVersion,
		QuotaVersion:  3,
	}
	require.NoError(t, writeUserCache(staleSnapshot, true))

	assert.Equal(t, "vip", server.HGet(key, "Group"), "metadata can be refreshed")
	assert.Equal(t, "500", server.HGet(key, "Quota"), "older quota must not roll back the cache")
	assert.Equal(t, "5", server.HGet(key, "QuotaVersion"), "older quota version must not roll back")

	// 3. A newer snapshot with QuotaVersion=6 must advance Quota and QuotaVersion
	newerSnapshot := &UserBase{
		Id:            userID,
		Group:         "vip",
		AccountTierID: "standard",
		Email:         "u2@example.com",
		Quota:         600,
		Status:        common.UserStatusEnabled,
		Role:          common.RoleCommonUser,
		Username:      "u2-renamed",
		Setting:       "{}",
		AuthVersion:   1,
		CacheSchema:   userCacheSchemaVersion,
		QuotaVersion:  6,
	}
	require.NoError(t, writeUserCache(newerSnapshot, true))

	assert.Equal(t, "600", server.HGet(key, "Quota"))
	assert.Equal(t, "6", server.HGet(key, "QuotaVersion"))

	// 4. Metadata-only update (includeQuota=false) does not alter Quota or QuotaVersion
	metaOnly := &UserBase{
		Id:            userID,
		Group:         "svip",
		AccountTierID: "standard",
		Email:         "u2@example.com",
		Quota:         0, // dummy
		Status:        common.UserStatusEnabled,
		Role:          common.RoleCommonUser,
		Username:      "u2-renamed",
		Setting:       "{}",
		AuthVersion:   1,
		CacheSchema:   userCacheSchemaVersion,
		QuotaVersion:  0, // dummy
	}
	require.NoError(t, writeUserCache(metaOnly, false))

	assert.Equal(t, "svip", server.HGet(key, "Group"))
	assert.Equal(t, "600", server.HGet(key, "Quota"), "metadata update must not overwrite quota")
	assert.Equal(t, "6", server.HGet(key, "QuotaVersion"), "metadata update must not overwrite quota version")
	_ = ctx
}

func TestTokenCacheQuotaProjectionAntiRollback(t *testing.T) {
	truncateTables(t)
	server := useUserCacheMiniRedis(t)
	ctx := context.Background()

	tokenKey := "sk-test-token-key-projection"
	hmacKey := getTokenCacheKey(tokenKey)

	// 1. Initial token cache initialization with QuotaVersion=5
	tok := Token{
		Id:           201,
		UserId:       9103,
		Key:          tokenKey,
		Status:       common.TokenStatusEnabled,
		Name:         "test-tok",
		RemainQuota:  500,
		UsedQuota:    100,
		QuotaVersion: 5,
	}
	res, err := cacheInitToken(tok)
	require.NoError(t, err)
	assert.Equal(t, 1, res)

	assert.Equal(t, "500", server.HGet(hmacKey, "RemainQuota"))
	assert.Equal(t, "100", server.HGet(hmacKey, "UsedQuota"))
	assert.Equal(t, "5", server.HGet(hmacKey, "QuotaVersion"))

	// 2. Out-of-order hydration with older QuotaVersion=3 must be dropped
	err = HydrateTokenQuotaCache(tokenKey, 201, 300, 50, 3)
	require.NoError(t, err)

	assert.Equal(t, "500", server.HGet(hmacKey, "RemainQuota"), "stale token quota must not roll back")
	assert.Equal(t, "100", server.HGet(hmacKey, "UsedQuota"))
	assert.Equal(t, "5", server.HGet(hmacKey, "QuotaVersion"))

	// 2b. Equal QuotaVersion=5 with different quota must also be dropped
	err = HydrateTokenQuotaCache(tokenKey, 201, 450, 150, 5)
	require.NoError(t, err)

	assert.Equal(t, "500", server.HGet(hmacKey, "RemainQuota"), "equal quota version must not overwrite token quota")
	assert.Equal(t, "100", server.HGet(hmacKey, "UsedQuota"))
	assert.Equal(t, "5", server.HGet(hmacKey, "QuotaVersion"))

	// 3. Newer hydration with QuotaVersion=7 must advance
	err = HydrateTokenQuotaCache(tokenKey, 201, 700, 150, 7)
	require.NoError(t, err)

	assert.Equal(t, "700", server.HGet(hmacKey, "RemainQuota"))
	assert.Equal(t, "150", server.HGet(hmacKey, "UsedQuota"))
	assert.Equal(t, "7", server.HGet(hmacKey, "QuotaVersion"))

	// 4. Invalidation sets fence and removes key
	err = InvalidateTokenQuotaCache(tokenKey)
	require.NoError(t, err)
	assert.False(t, server.Exists(hmacKey))

	// 5. Hydration on cold/missing token hash does not create partial hash
	err = HydrateTokenQuotaCache(tokenKey, 201, 800, 200, 8)
	require.NoError(t, err)
	assert.False(t, server.Exists(hmacKey), "hydration on absent token cache must not create partial hash")
	_ = ctx
}
