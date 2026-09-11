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

	const userID = 9101
	key := getUserCacheKey(userID)

	// 1. Pre-populate a live user hash in Redis with QuotaVersion=10 and Quota=1000
	initialHash := map[string]string{
		"Id":            strconv.Itoa(userID),
		"Group":         "default",
		"AccountTierID": "standard",
		"Email":         "user@example.com",
		"Status":        strconv.Itoa(common.UserStatusEnabled),
		"Role":          strconv.Itoa(common.RoleCommonUser),
		"Username":      "tester",
		"Setting":       "{}",
		"AuthVersion":   "1",
		"CacheSchema":   strconv.Itoa(userCacheSchemaVersion),
		"Quota":         "1000",
		"QuotaVersion":  "10",
	}
	require.NoError(t, common.RDB.HSet(ctx, key, initialHash).Err())
	require.NoError(t, common.RDB.Expire(ctx, key, time.Minute).Err())

	// 2. Out-of-order hydration with older QuotaVersion=8 and Quota=800 must be dropped (anti-rollback)
	err := HydrateUserQuotaCache(userID, 800, 8)
	require.NoError(t, err)

	quotaVal, err := common.RDB.HGet(ctx, key, "Quota").Result()
	require.NoError(t, err)
	assert.Equal(t, "1000", quotaVal, "stale quota must not roll back the cache")

	qvVal, err := common.RDB.HGet(ctx, key, "QuotaVersion").Result()
	require.NoError(t, err)
	assert.Equal(t, "10", qvVal, "stale quota version must not roll back the cache")

	// 2b. Equal QuotaVersion=10 with different Quota=900 must also be dropped (anti-rollback against live decrements)
	err = HydrateUserQuotaCache(userID, 900, 10)
	require.NoError(t, err)

	quotaVal, err = common.RDB.HGet(ctx, key, "Quota").Result()
	require.NoError(t, err)
	assert.Equal(t, "1000", quotaVal, "equal quota version must not overwrite live quota")

	// 3. Newer hydration with QuotaVersion=11 and Quota=1200 must advance the cache projection
	err = HydrateUserQuotaCache(userID, 1200, 11)
	require.NoError(t, err)

	quotaVal, err = common.RDB.HGet(ctx, key, "Quota").Result()
	require.NoError(t, err)
	assert.Equal(t, "1200", quotaVal, "newer quota projection must be applied")

	qvVal, err = common.RDB.HGet(ctx, key, "QuotaVersion").Result()
	require.NoError(t, err)
	assert.Equal(t, "11", qvVal, "newer quota version must advance")

	// 4. InvalidateUserQuotaCache removes the cache key
	err = InvalidateUserQuotaCache(userID)
	require.NoError(t, err)
	assert.False(t, server.Exists(key), "invalidation must clear cache")

	// 5. Hydration on cold/missing cache must not create an incomplete or partial hash
	err = HydrateUserQuotaCache(userID, 1500, 12)
	require.NoError(t, err)
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
