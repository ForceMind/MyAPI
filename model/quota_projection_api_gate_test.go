package model

import (
	"context"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/stretchr/testify/require"
)

func TestPublicQuotaProjectionAPIsRequireProjectionWriterMode(t *testing.T) {
	for _, mode := range []QuotaWriterMode{QuotaWriterModeLegacy, QuotaWriterModeBridge, QuotaWriterModeAuthoritative} {
		t.Run(string(mode), func(t *testing.T) {
			_, fixture, server := openLegacyBalanceBatchTestWithRedis(t, "projection-api-"+string(mode))
			require.NoError(t, DB.Model(&QuotaWriterEpoch{}).Where("id = ?", 1).Update("mode", string(mode)).Error)
			userKey, tokenKey := getUserCacheKey(fixture.User.Id), getTokenCacheKey(fixture.Token.Key)
			userBefore, userTTL := quotaCacheHashSnapshot(t, userKey)
			tokenBefore, tokenTTL := quotaCacheHashSnapshot(t, tokenKey)
			userErr := HydrateUserQuotaCache(fixture.User.Id, 800, fixture.User.QuotaVersion+1)
			tokenErr := HydrateTokenQuotaCache(fixture.Token.Key, fixture.Token.Id, 700, 300, fixture.Token.QuotaVersion+1)
			if mode == QuotaWriterModeLegacy {
				require.ErrorIs(t, userErr, ErrQuotaProjectionModeDisabled)
				require.ErrorIs(t, tokenErr, ErrQuotaProjectionModeDisabled)
				requireQuotaCacheHashUnchanged(t, userKey, userBefore, userTTL)
				requireQuotaCacheHashUnchanged(t, tokenKey, tokenBefore, tokenTTL)
				return
			}
			require.NoError(t, userErr)
			require.NoError(t, tokenErr)
			require.Equal(t, "800", server.HGet(userKey, "Quota"))
			require.Equal(t, "700", server.HGet(tokenKey, "RemainQuota"))
			require.Equal(t, "300", server.HGet(tokenKey, "UsedQuota"))
			require.NoError(t, common.RDB.Del(context.Background(), userKey, tokenKey).Err())
			require.NoError(t, HydrateUserQuotaCache(fixture.User.Id, 900, fixture.User.QuotaVersion+2))
			require.NoError(t, HydrateTokenQuotaCache(fixture.Token.Key, fixture.Token.Id, 600, 400, fixture.Token.QuotaVersion+2))
			require.False(t, server.Exists(userKey), "a projection cannot create an incomplete cold user hash")
			require.False(t, server.Exists(tokenKey), "a projection cannot create an incomplete cold token hash")
		})
	}
}
