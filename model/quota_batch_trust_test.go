package model

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/stretchr/testify/require"
)

func TestLegacyBatchQuotaTrustRequiresLiveCoherentProjections(t *testing.T) {
	tests := []struct {
		name      string
		key       string
		field     string
		value     string
		operation string
		unlimited bool
		eligible  bool
	}{
		{name: "live balances", eligible: true},
		{name: "missing wallet", key: "user", operation: "delete"},
		{name: "missing token", key: "token", operation: "delete"},
		{name: "persistent wallet", key: "user", operation: "persist"},
		{name: "persistent token", key: "token", operation: "persist"},
		{name: "wrong user", key: "user", field: "Id", value: "999999"},
		{name: "wrong token", key: "token", field: "Id", value: "999999"},
		{name: "wrong token owner", key: "token", field: "UserId", value: "999999"},
		{name: "old user schema", key: "user", field: "CacheSchema", value: "0"},
		{name: "old token schema", key: "token", field: "CacheSchema", value: "0"},
		{name: "wrong user epoch", key: "user", field: "QuotaWriterEpoch", value: "1"},
		{name: "wrong token epoch", key: "token", field: "QuotaWriterEpoch", value: "1"},
		{name: "wrong global epoch", key: "epoch", value: "1", operation: "set"},
		{name: "malformed user version", key: "user", field: "QuotaVersion", value: "01"},
		{name: "malformed token version", key: "token", field: "QuotaVersion", value: "-1"},
		{name: "large exact version", key: "token", field: "QuotaVersion", value: "9223372036854775807", eligible: true},
		{name: "wallet at threshold", key: "user", field: "Quota", value: "100"},
		{name: "token at threshold", key: "token", field: "RemainQuota", value: "100"},
		{name: "wide exact balance", key: "token", field: "RemainQuota", value: "9007199254740991", eligible: true},
		{name: "out of range balance", key: "token", field: "RemainQuota", value: "9007199254740992"},
		{name: "malformed wallet balance", key: "user", field: "Quota", value: "1e3"},
		{name: "missing wallet balance", key: "user", field: "Quota", operation: "hdelete"},
		{name: "negative cumulative", key: "token", field: "UsedQuota", value: "-1"},
		{name: "unlimited retains trust rule", key: "token", field: "RemainQuota", value: "-1", unlimited: true, eligible: true},
		{name: "wallet owner busy", key: "user-lock", value: "busy", operation: "set"},
		{name: "token owner busy", key: "token-lock", value: "busy", operation: "set"},
		{name: "token mutation pending", key: "token-fence", value: "1", operation: "set"},
		{name: "user authentication pending", key: "auth-fence", value: "2", operation: "set"},
		{name: "user authentication revoked", key: "auth-floor", value: "2", operation: "set"},
		{name: "malformed authentication version", key: "user", field: "AuthVersion", value: "0"},
		{name: "malformed authentication floor", key: "auth-floor", value: "bad", operation: "set"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, fixture, server := openLegacyBalanceBatchTestWithRedis(t, "batch-trust-"+tc.name)
			userKey, tokenKey := getUserCacheKey(fixture.User.Id), getTokenCacheKey(fixture.Token.Key)
			userLock, err := quotaBalanceSubjectLockKey(BatchUpdateTypeUserQuota, fixture.User.Id)
			require.NoError(t, err)
			tokenLock, err := quotaBalanceSubjectLockKey(BatchUpdateTypeTokenQuota, fixture.Token.Id)
			require.NoError(t, err)
			keys := map[string]string{"user": userKey, "token": tokenKey, "epoch": quotaWriterEpochRedisKey,
				"user-lock": userLock, "token-lock": tokenLock, "token-fence": getTokenCacheFenceKey(fixture.Token.Key),
				"auth-fence": getUserAuthFenceKey(fixture.User.Id), "auth-floor": getUserAuthVersionKey(fixture.User.Id)}
			ctx := context.Background()
			switch tc.operation {
			case "delete":
				server.Del(keys[tc.key])
			case "persist":
				require.NoError(t, common.RDB.Persist(ctx, keys[tc.key]).Err())
			case "hdelete":
				require.NoError(t, common.RDB.HDel(ctx, keys[tc.key], tc.field).Err())
			case "set":
				require.NoError(t, common.RDB.Set(ctx, keys[tc.key], tc.value, time.Minute).Err())
			default:
				if tc.field != "" {
					server.HSet(keys[tc.key], tc.field, tc.value)
				}
			}
			state, err := GetQuotaWriterEpochState(DB)
			require.NoError(t, err)
			beforeUser, userTTL := quotaCacheHashSnapshot(t, userKey)
			beforeToken, tokenTTL := quotaCacheHashSnapshot(t, tokenKey)
			beforeKeys := server.Keys()

			eligible, err := LegacyBatchQuotaTrustEligible(fixture.User.Id, fixture.Token.Id, fixture.Token.Key, tc.unlimited, state.Epoch, 100)

			require.NoError(t, err)
			require.Equal(t, tc.eligible, eligible)
			requireQuotaCacheHashUnchanged(t, userKey, beforeUser, userTTL)
			requireQuotaCacheHashUnchanged(t, tokenKey, beforeToken, tokenTTL)
			require.Equal(t, beforeKeys, server.Keys(), "trust checking must not publish or delete cache keys")
		})
	}
}

func TestLegacyBatchQuotaTrustErrorsDoNotAuthorize(t *testing.T) {
	_, fixture, server := openLegacyBalanceBatchTestWithRedis(t, "batch-trust-error")
	state, err := GetQuotaWriterEpochState(DB)
	require.NoError(t, err)
	server.SetError("ERR synthetic trust cache failure")
	eligible, err := LegacyBatchQuotaTrustEligible(fixture.User.Id, fixture.Token.Id, fixture.Token.Key, false, state.Epoch, 100)
	require.Error(t, err)
	require.False(t, eligible)
	server.SetError("")
	server.FastForward(time.Duration(tokenCacheTTLSeconds()+userCacheTTLSeconds()) * time.Second)
	eligible, err = LegacyBatchQuotaTrustEligible(fixture.User.Id, fixture.Token.Id, fixture.Token.Key, false, state.Epoch, 100)
	require.NoError(t, err)
	require.False(t, eligible)
	require.False(t, server.Exists(getTokenCacheKey(fixture.Token.Key)))
	require.False(t, server.Exists(getUserCacheKey(fixture.User.Id)))
}

func TestUnownedRefreshCannotMakePersistentQuotaProjectionTrusted(t *testing.T) {
	for _, subject := range []string{"user", "token"} {
		t.Run(subject, func(t *testing.T) {
			_, fixture, server := openLegacyBalanceBatchTestWithRedis(t, "persistent-trust-"+subject)
			state, err := GetQuotaWriterEpochState(DB)
			require.NoError(t, err)
			key := getUserCacheKey(fixture.User.Id)
			if subject == "token" {
				key = getTokenCacheKey(fixture.Token.Key)
			}
			require.NoError(t, common.RDB.Persist(context.Background(), key).Err())
			eligible, err := LegacyBatchQuotaTrustEligible(fixture.User.Id, fixture.Token.Id, fixture.Token.Key, false, state.Epoch, 100)
			require.NoError(t, err)
			require.False(t, eligible)
			if subject == "user" {
				fixture.User.AuthVersion++
				require.NoError(t, DB.Model(&User{}).Where("id = ?", fixture.User.Id).Update("auth_version", fixture.User.AuthVersion).Error)
				require.NoError(t, common.RDB.Set(context.Background(), getUserAuthFenceKey(fixture.User.Id), fixture.User.AuthVersion, time.Minute).Err())
				require.NoError(t, updateUserCache(fixture.User))
				floor, err := server.Get(getUserAuthVersionKey(fixture.User.Id))
				require.NoError(t, err)
				require.Equal(t, strconv.FormatInt(fixture.User.AuthVersion, 10), floor)
				require.False(t, server.Exists(getUserAuthFenceKey(fixture.User.Id)))
			} else {
				_, err = cacheInitToken(fixture.Token)
				require.NoError(t, err)
			}
			eligible, err = LegacyBatchQuotaTrustEligible(fixture.User.Id, fixture.Token.Id, fixture.Token.Key, false, state.Epoch, 100)
			require.NoError(t, err)
			require.False(t, eligible, "an unowned refresh cannot confirm persistent accounting state")
			require.False(t, server.Exists(key), "uncertain accounting state must remain cold for owner recovery")
			var user User
			require.NoError(t, DB.First(&user, fixture.User.Id).Error)
			require.Equal(t, fixture.User.Quota, user.Quota)
			require.Equal(t, fixture.User.QuotaVersion, user.QuotaVersion)
			var token Token
			require.NoError(t, DB.First(&token, fixture.Token.Id).Error)
			require.Equal(t, fixture.Token.RemainQuota, token.RemainQuota)
			require.Equal(t, fixture.Token.UsedQuota, token.UsedQuota)
			require.Equal(t, fixture.Token.QuotaVersion, token.QuotaVersion)
		})
	}
}
