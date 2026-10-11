package model

import (
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestTokenRevocationRejectsLateSnapshotAfterCommittedCacheFailure(t *testing.T) {
	_, fixture, server := openLegacyBalanceBatchTestWithRedis(t, "late-snapshot-revocation")
	oldSnapshot := fixture.Token
	count, published := 0, 0
	var publishErr error
	common.RDB.AddHook(rejectNthTokenCacheEvalHook{
		count: &count, failAt: 3, err: errors.New("synthetic post-commit invalidation failure"),
		beforeFailure: func() {
			// A reader began before revocation and publishes its snapshot only
			// after the last in-transaction observation window has expired.
			server.FastForward(time.Duration(tokenCacheFenceSeconds+1) * time.Second)
			published, publishErr = cacheInitToken(oldSnapshot)
		},
	})
	fixture.Token.Status = common.TokenStatusDisabled
	err := fixture.Token.Update()
	require.ErrorIs(t, err, ErrTokenMetadataMutationCommittedCacheFailure)
	require.NoError(t, publishErr)
	require.Zero(t, published)
	require.False(t, server.Exists(getTokenCacheKey(fixture.Token.Key)), "a delayed reader cannot publish accounting state")
	var stored Token
	require.NoError(t, DB.First(&stored, fixture.Token.Id).Error)
	require.Equal(t, common.TokenStatusDisabled, stored.Status)
	require.Equal(t, oldSnapshot.RemainQuota, stored.RemainQuota)
	require.Equal(t, oldSnapshot.UsedQuota, stored.UsedQuota)
	require.Equal(t, oldSnapshot.QuotaVersion, stored.QuotaVersion)
	_, err = ValidateUserToken(fixture.Token.Key)
	require.ErrorIs(t, err, ErrTokenInvalid)
}

func TestTokenCachedReadUsesCurrentAuthorizationAndPreservesPendingBalance(t *testing.T) {
	_, fixture, _ := openLegacyBalanceBatchTestWithRedis(t, "current-auth-pending-balance")
	require.NoError(t, DecreaseTokenQuota(fixture.Token.Id, fixture.Token.Key, 50))
	cached, err := cacheGetTokenByKey(fixture.Token.Key)
	require.NoError(t, err)
	require.Equal(t, int64(450), cached.RemainQuota)
	require.Equal(t, int64(50), cached.UsedQuota)
	allowIPs := "127.0.0.1"
	require.NoError(t, DB.Model(&Token{}).Where("id = ?", fixture.Token.Id).Updates(map[string]any{
		"name": "current-name", "status": common.TokenStatusDisabled,
		"expired_time": int64(2000000000), "unlimited_quota": true,
		"model_limits_enabled": true, "model_limits": "model-a",
		"allow_ips": allowIPs, "group": "auto", "access_profile_id": "restricted-profile",
		"cross_group_retry": true, "auto_groups": `["restricted"]`,
	}).Error)
	loaded, err := GetTokenByKey(fixture.Token.Key, false)
	require.NoError(t, err)
	require.Equal(t, "current-name", loaded.Name)
	require.Equal(t, common.TokenStatusDisabled, loaded.Status)
	require.Equal(t, int64(2000000000), loaded.ExpiredTime)
	require.True(t, loaded.UnlimitedQuota)
	require.True(t, loaded.ModelLimitsEnabled)
	require.Equal(t, "model-a", loaded.ModelLimits)
	require.NotNil(t, loaded.AllowIps)
	require.Equal(t, allowIPs, *loaded.AllowIps)
	require.Equal(t, "auto", loaded.Group)
	require.Equal(t, "restricted-profile", loaded.AccessProfileID)
	require.True(t, loaded.CrossGroupRetry)
	require.Equal(t, `["restricted"]`, loaded.AutoGroups)
	require.Equal(t, cached.RemainQuota, loaded.RemainQuota)
	require.Equal(t, cached.UsedQuota, loaded.UsedQuota)
	require.Equal(t, cached.QuotaVersion, loaded.QuotaVersion)
	require.Equal(t, cached.AccessedTime, loaded.AccessedTime)
	var stored Token
	require.NoError(t, DB.First(&stored, fixture.Token.Id).Error)
	require.Equal(t, int64(500), stored.RemainQuota)
	require.Zero(t, stored.UsedQuota)
	stillCached, err := cacheGetTokenByKey(fixture.Token.Key)
	require.NoError(t, err)
	require.Equal(t, cached, stillCached, "an authorization read must not overwrite live accounting state")
}

func TestTokenCachedReadPreservesNewBatchDeltasAfterDatabaseVersionAdvances(t *testing.T) {
	_, fixture, _ := openLegacyBalanceBatchTestWithRedis(t, "current-auth-drained-version")
	require.NoError(t, DecreaseTokenQuota(fixture.Token.Id, fixture.Token.Key, 50))
	flushQuotaBalanceBatchGenerations()
	var stored Token
	require.NoError(t, DB.First(&stored, fixture.Token.Id).Error)
	require.Equal(t, int64(450), stored.RemainQuota)
	require.Equal(t, int64(50), stored.UsedQuota)
	cached, err := cacheGetTokenByKey(fixture.Token.Key)
	require.NoError(t, err)
	require.Greater(t, stored.QuotaVersion, cached.QuotaVersion)
	require.NoError(t, DecreaseTokenQuota(fixture.Token.Id, fixture.Token.Key, 25))
	loaded, err := ValidateUserToken(fixture.Token.Key)
	require.NoError(t, err)
	require.Equal(t, int64(425), loaded.RemainQuota)
	require.Equal(t, int64(75), loaded.UsedQuota)
	require.Equal(t, cached.QuotaVersion, loaded.QuotaVersion)
	var after Token
	require.NoError(t, DB.First(&after, fixture.Token.Id).Error)
	require.Equal(t, stored, after, "lookup must not drain pending accounting state")
}

func TestTokenCachedReadRejectsDatabaseFailure(t *testing.T) {
	_, fixture, _ := openLegacyBalanceBatchTestWithRedis(t, "current-auth-database-error")
	callbackName := "fixture_fail_current_token_authorization_query"
	databaseErr := errors.New("synthetic token authorization database failure")
	require.NoError(t, DB.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == "tokens" {
			tx.AddError(databaseErr)
		}
	}))
	t.Cleanup(func() { _ = DB.Callback().Query().Remove(callbackName) })
	loaded, err := GetTokenByKey(fixture.Token.Key, false)
	require.Nil(t, loaded)
	require.ErrorIs(t, err, databaseErr)
	loaded, err = ValidateUserToken(fixture.Token.Key)
	require.Nil(t, loaded)
	require.ErrorIs(t, err, ErrDatabase)
}

func TestTokenCachedReadRejectsMismatchedSubject(t *testing.T) {
	for _, field := range []string{"Id", "UserId"} {
		t.Run(field, func(t *testing.T) {
			_, fixture, server := openLegacyBalanceBatchTestWithRedis(t, "current-auth-wrong-"+field)
			server.HSet(getTokenCacheKey(fixture.Token.Key), field, strconv.Itoa(fixture.Token.Id+fixture.User.Id+1))
			loaded, err := GetTokenByKey(fixture.Token.Key, false)
			require.Nil(t, loaded)
			require.ErrorIs(t, err, ErrQuotaCacheIdentityMismatch)
			loaded, err = ValidateUserToken(fixture.Token.Key)
			require.Nil(t, loaded)
			require.ErrorIs(t, err, ErrTokenInvalid)
			var stored Token
			require.NoError(t, DB.First(&stored, fixture.Token.Id).Error)
			require.Equal(t, fixture.Token.RemainQuota, stored.RemainQuota)
			require.Equal(t, fixture.Token.UsedQuota, stored.UsedQuota)
			require.Equal(t, fixture.Token.QuotaVersion, stored.QuotaVersion)
		})
	}
}

func TestTokenLookupRetainsDatabaseFallbackOnCacheMissOrRedisFailure(t *testing.T) {
	for _, scenario := range []string{"miss", "redis failure"} {
		t.Run(scenario, func(t *testing.T) {
			_, fixture, server := openLegacyBalanceBatchTestWithRedis(t, "current-auth-fallback-"+scenario)
			if scenario == "miss" {
				server.Del(getTokenCacheKey(fixture.Token.Key))
			} else {
				server.SetError("ERR synthetic Redis read failure")
			}
			loaded, err := ValidateUserToken(fixture.Token.Key)
			require.NoError(t, err)
			require.Equal(t, fixture.Token.Id, loaded.Id)
			require.Equal(t, fixture.Token.UserId, loaded.UserId)
			require.Equal(t, fixture.Token.RemainQuota, loaded.RemainQuota)
			require.Equal(t, fixture.Token.UsedQuota, loaded.UsedQuota)
			server.SetError("")
		})
	}
}

func TestTokenBatchDeletionRejectsSnapshotPublishedDuringSlowDelete(t *testing.T) {
	_, fixture, server := openLegacyBalanceBatchTestWithRedis(t, "late-snapshot-batch-delete")
	oldSnapshot := fixture.Token
	callbackName := "fixture_publish_snapshot_before_batch_delete"
	var publishErr error
	published := 0
	require.NoError(t, DB.Callback().Delete().Before("gorm:delete").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table != "tokens" {
			return
		}
		server.FastForward(time.Duration(tokenCacheFenceSeconds+1) * time.Second)
		published, publishErr = cacheInitToken(oldSnapshot)
	}))
	t.Cleanup(func() { _ = DB.Callback().Delete().Remove(callbackName) })
	deleted, err := BatchDeleteTokens([]int{fixture.Token.Id}, fixture.User.Id)
	require.NoError(t, err)
	require.Equal(t, 1, deleted)
	require.NoError(t, publishErr)
	require.Zero(t, published)
	require.False(t, server.Exists(getTokenCacheKey(fixture.Token.Key)), "a delayed reader cannot publish accounting state")
	var stored Token
	require.NoError(t, DB.Unscoped().First(&stored, fixture.Token.Id).Error)
	require.True(t, stored.DeletedAt.Valid)
	require.Equal(t, oldSnapshot.RemainQuota, stored.RemainQuota)
	require.Equal(t, oldSnapshot.UsedQuota, stored.UsedQuota)
	require.Equal(t, oldSnapshot.QuotaVersion, stored.QuotaVersion)
	_, err = ValidateUserToken(fixture.Token.Key)
	require.ErrorIs(t, err, ErrTokenInvalid)
}
