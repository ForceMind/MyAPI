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

func newUserCacheColdPublicationFixture(t *testing.T) User {
	t.Helper()
	db := openQuotaProjectionTestDB(t)
	useQuotaProjectionModelDB(t, db)
	useUserCacheMiniRedis(t)

	state, err := GetQuotaWriterEpochState(db)
	require.NoError(t, err)
	require.NoError(t, common.RDB.Set(t.Context(), quotaWriterEpochRedisKey, state.Epoch, 0).Err())

	user := User{
		Username:      "cold-publication-user",
		Password:      "fixture-password",
		Status:        common.UserStatusEnabled,
		Role:          common.RoleCommonUser,
		Group:         "default",
		AccountTierID: "standard",
		Quota:         1000,
		QuotaVersion:  4,
		AuthVersion:   1,
	}
	require.NoError(t, db.Create(&user).Error)
	return user
}

func TestUserCacheColdPublicationRejectsLateUnownedSnapshot(t *testing.T) {
	user := newUserCacheColdPublicationFixture(t)
	stale := *user.ToBaseUser()
	cacheKey := getUserCacheKey(user.Id)

	lock, err := acquireQuotaBalanceSubjectLock(BatchUpdateTypeUserQuota, user.Id)
	require.NoError(t, err)
	t.Cleanup(func() { releaseQuotaBalanceSubjectLock(lock) })

	target, err := common.CheckedQuotaBalance(user.Quota, -125)
	require.NoError(t, err)
	result := DB.Model(&User{}).
		Where("id = ? AND quota = ? AND quota_version = ?", user.Id, user.Quota, user.QuotaVersion).
		Updates(map[string]any{"quota": target, "quota_version": user.QuotaVersion + 1})
	require.NoError(t, result.Error)
	require.EqualValues(t, 1, result.RowsAffected)
	require.NoError(t, invalidateQuotaBalanceSubjectCache(lock, cacheKey, ""))

	err = writeUserCache(&stale, true)
	assert.ErrorIs(t, err, ErrQuotaBalanceMutationUnknown)
	exists, err := common.RDB.Exists(t.Context(), cacheKey).Result()
	require.NoError(t, err)
	assert.Zero(t, exists)
	floorExists, err := common.RDB.Exists(t.Context(), getUserAuthVersionKey(user.Id)).Result()
	require.NoError(t, err)
	assert.Zero(t, floorExists, "an owner mismatch must reject before publishing metadata floors")

	require.NoError(t, releaseQuotaBalanceSubjectLockContext(context.Background(), lock))
	require.NoError(t, writeUserCache(&stale, true))
	exists, err = common.RDB.Exists(t.Context(), cacheKey).Result()
	require.NoError(t, err)
	assert.Zero(t, exists, "a delayed database snapshot must not republish a cold wallet without its subject owner")
	committed, err := common.RDB.Get(t.Context(), getUserAuthVersionKey(user.Id)).Result()
	require.NoError(t, err)
	assert.Equal(t, strconv.FormatInt(stale.AuthVersion, 10), committed)

	var stored User
	require.NoError(t, DB.First(&stored, user.Id).Error)
	assert.Equal(t, target, stored.Quota)
	assert.Equal(t, user.QuotaVersion+1, stored.QuotaVersion)
}

func TestUserCacheNonOwnerRefreshPreservesLiveWalletProjection(t *testing.T) {
	user := newUserCacheColdPublicationFixture(t)
	cacheKey := getUserCacheKey(user.Id)

	lock, err := acquireQuotaBalanceSubjectLock(BatchUpdateTypeUserQuota, user.Id)
	require.NoError(t, err)
	require.NoError(t, writeUserCacheWithQuotaBalanceOwner(user.ToBaseUser(), true, lock))
	require.NoError(t, releaseQuotaBalanceSubjectLockContext(context.Background(), lock))

	before, err := common.RDB.HMGet(t.Context(), cacheKey, "Quota", "QuotaVersion", "QuotaWriterEpoch").Result()
	require.NoError(t, err)
	refresh := *user.ToBaseUser()
	refresh.Group = "vip"
	refresh.Username = "metadata-refreshed"
	refresh.AuthVersion++
	refresh.Quota = 6000
	refresh.QuotaVersion += 100
	require.NoError(t, writeUserCache(&refresh, true))

	after, err := common.RDB.HMGet(t.Context(), cacheKey, "Quota", "QuotaVersion", "QuotaWriterEpoch").Result()
	require.NoError(t, err)
	assert.Equal(t, before, after)
	metadata, err := common.RDB.HMGet(t.Context(), cacheKey, "Group", "Username", "AuthVersion").Result()
	require.NoError(t, err)
	assert.Equal(t, []interface{}{"vip", "metadata-refreshed", strconv.FormatInt(refresh.AuthVersion, 10)}, metadata)
	ttl, err := common.RDB.TTL(t.Context(), cacheKey).Result()
	require.NoError(t, err)
	assert.Greater(t, ttl, time.Duration(0))
}

func TestUserCacheOwnerCanRecoverColdWalletProjection(t *testing.T) {
	user := newUserCacheColdPublicationFixture(t)
	cacheKey := getUserCacheKey(user.Id)

	lock, err := acquireQuotaBalanceSubjectLock(BatchUpdateTypeUserQuota, user.Id)
	require.NoError(t, err)
	t.Cleanup(func() { releaseQuotaBalanceSubjectLock(lock) })
	require.NoError(t, writeUserCacheWithQuotaBalanceOwner(user.ToBaseUser(), true, lock))

	cached, err := cacheGetUserBase(user.Id)
	require.NoError(t, err)
	assert.Equal(t, user.Quota, cached.Quota)
	assert.Equal(t, user.QuotaVersion, cached.QuotaVersion)

	equalVersion := *user.ToBaseUser()
	equalVersion.Quota = user.Quota + 200
	require.NoError(t, writeUserCacheWithQuotaBalanceOwner(&equalVersion, true, lock))
	quota, err := common.RDB.HGet(t.Context(), cacheKey, "Quota").Result()
	require.NoError(t, err)
	assert.Equal(t, strconv.FormatInt(user.Quota, 10), quota)

	advanced := *user.ToBaseUser()
	advanced.Quota = user.Quota + 300
	advanced.QuotaVersion++
	require.NoError(t, writeUserCacheWithQuotaBalanceOwner(&advanced, true, lock))
	advancedState, err := common.RDB.HMGet(t.Context(), cacheKey, "Quota", "QuotaVersion").Result()
	require.NoError(t, err)
	assert.Equal(t, []interface{}{strconv.FormatInt(advanced.Quota, 10), strconv.FormatInt(advanced.QuotaVersion, 10)}, advancedState)
}

func TestUserCacheNonOwnerInvalidatesOldSchemaWithoutRecreating(t *testing.T) {
	user := newUserCacheColdPublicationFixture(t)
	cacheKey := getUserCacheKey(user.Id)
	sentinelKey := cacheKey + ":unrelated"
	require.NoError(t, common.RDB.Set(t.Context(), sentinelKey, "keep", 0).Err())
	require.NoError(t, common.RDB.HSet(t.Context(), cacheKey, map[string]string{
		"Id":               strconv.Itoa(user.Id),
		"AuthVersion":      strconv.FormatInt(user.AuthVersion, 10),
		"CacheSchema":      strconv.Itoa(userCacheSchemaVersion - 1),
		"Quota":            strconv.FormatInt(user.Quota, 10),
		"QuotaVersion":     strconv.FormatInt(user.QuotaVersion, 10),
		"QuotaWriterEpoch": "1",
	}).Err())

	refresh := *user.ToBaseUser()
	refresh.Group = "late-metadata"
	require.NoError(t, writeUserCache(&refresh, true))
	exists, err := common.RDB.Exists(t.Context(), cacheKey).Result()
	require.NoError(t, err)
	assert.Zero(t, exists, "an unowned old-schema hash may be invalidated but must not be recreated")
	sentinel, err := common.RDB.Get(t.Context(), sentinelKey).Result()
	require.NoError(t, err)
	assert.Equal(t, "keep", sentinel)
}

func TestUserCacheAuthFieldFenceKeepsExactVersionBoundary(t *testing.T) {
	user := newUserCacheColdPublicationFixture(t)
	cacheKey := getUserCacheKey(user.Id)
	const currentVersion = int64(1 << 53)
	const nextVersion = currentVersion + 1

	stale := *user.ToBaseUser()
	stale.AuthVersion = currentVersion
	require.NoError(t, SetUserAuthVersionFence(user.Id, nextVersion))
	err := writeUserCache(&stale, true)
	assert.ErrorIs(t, err, ErrUserAuthCachePending)
	exists, readErr := common.RDB.Exists(t.Context(), cacheKey).Result()
	require.NoError(t, readErr)
	assert.Zero(t, exists)
	committedExists, readErr := common.RDB.Exists(t.Context(), getUserAuthVersionKey(user.Id)).Result()
	require.NoError(t, readErr)
	assert.Zero(t, committedExists)

	require.NoError(t, publishCommittedUserAuthVersion(user.Id, nextVersion))
	current := stale
	current.AuthVersion = nextVersion
	lock, err := acquireQuotaBalanceSubjectLock(BatchUpdateTypeUserQuota, user.Id)
	require.NoError(t, err)
	require.NoError(t, writeUserCacheWithQuotaBalanceOwner(&current, true, lock))
	require.NoError(t, releaseQuotaBalanceSubjectLockContext(context.Background(), lock))
	quotaState, err := common.RDB.HMGet(t.Context(), cacheKey, "Quota", "QuotaVersion", "QuotaWriterEpoch").Result()
	require.NoError(t, err)

	require.NoError(t, updateUserCacheFieldAtVersion(user.Id, "Group", "exact", nextVersion))
	group, err := common.RDB.HGet(t.Context(), cacheKey, "Group").Result()
	require.NoError(t, err)
	assert.Equal(t, "exact", group)
	err = updateUserCacheFieldAtVersion(user.Id, "Group", "rounded-stale", currentVersion)
	assert.ErrorIs(t, err, ErrUserAuthCachePending)
	group, err = common.RDB.HGet(t.Context(), cacheKey, "Group").Result()
	require.NoError(t, err)
	assert.Equal(t, "exact", group)
	after, err := common.RDB.HMGet(t.Context(), cacheKey, "Quota", "QuotaVersion", "QuotaWriterEpoch").Result()
	require.NoError(t, err)
	assert.Equal(t, quotaState, after)
}
