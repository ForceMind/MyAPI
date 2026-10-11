package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTokenQuotaEditRejectsLateUnownedBalanceSnapshot(t *testing.T) {
	_, fixture, server := openLegacyBalanceBatchTestWithRedis(t, "late-unowned-token-balance")
	oldSnapshot := fixture.Token
	requested := fixture.Token
	fingerprint, err := requested.QuotaMetadataFingerprint()
	require.NoError(t, err)
	requested.RemainQuota = 2
	require.NoError(t, requested.UpdateWithQuota(requested.QuotaVersion, fingerprint))
	var stored Token
	require.NoError(t, DB.First(&stored, fixture.Token.Id).Error)
	require.Equal(t, int64(2), stored.RemainQuota)
	require.Equal(t, oldSnapshot.QuotaVersion+1, stored.QuotaVersion)

	server.FastForward(time.Duration(tokenCacheFenceSeconds+1) * time.Second)
	published, err := cacheInitToken(oldSnapshot)
	require.NoError(t, err)
	require.Zero(t, published, "an unowned late reader must not publish the old spending limit")
	require.False(t, server.Exists(getTokenCacheKey(fixture.Token.Key)))

	loaded, err := ValidateUserToken(fixture.Token.Key)
	require.NoError(t, err)
	require.Equal(t, int64(2), loaded.RemainQuota)
	reserved, err := TryReserveTokenQuota(fixture.Token.Id, fixture.Token.Key, 3, false)
	require.NoError(t, err)
	require.False(t, reserved, "owner recovery must enforce the edited limit")
	var after Token
	require.NoError(t, DB.First(&after, fixture.Token.Id).Error)
	require.Equal(t, stored, after, "a denied reserve must not change the token balance or version")
}

func TestTokenColdAuthorizationReadDoesNotPublishAccountingSnapshot(t *testing.T) {
	_, fixture, server := openLegacyBalanceBatchTestWithRedis(t, "cold-token-auth-read")
	server.Del(getTokenCacheKey(fixture.Token.Key))
	loaded, err := GetTokenByKey(fixture.Token.Key, false)
	require.NoError(t, err)
	require.Equal(t, fixture.Token.Id, loaded.Id)
	require.Equal(t, fixture.Token.RemainQuota, loaded.RemainQuota)
	require.False(t, server.Exists(getTokenCacheKey(fixture.Token.Key)), "only the accounting owner may rebuild a cold amount projection")
}
