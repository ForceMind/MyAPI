package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/model"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type quotaBatchColdTrustFixture struct {
	db     *gorm.DB
	server *miniredis.Miniredis
	user   *model.User
	token  *model.Token
}

func newQuotaBatchColdTrustFixture(t *testing.T, label string, quota int) quotaBatchColdTrustFixture {
	t.Helper()
	db := setupPostConsumeModeDB(t, model.QuotaWriterModeLegacy)
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	require.NoError(t, client.Ping(context.Background()).Err())
	common.RedisEnabled = true
	common.RDB = client
	common.BatchUpdateEnabled = true
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	user, token := seedAuthoritativeBilling(t, db, label, quota, quota, false)
	return quotaBatchColdTrustFixture{db: db, server: server, user: user, token: token}
}

func (fixture quotaBatchColdTrustFixture) billingInput(requestID string) (*gin.Context, *relaycommon.RelayInfo) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	common.SetContextKey(ctx, constant.ContextKeyTokenQuota, fixture.token.RemainQuota)
	info := authoritativeRelay(fixture.user, fixture.token, requestID)
	info.ChannelMeta = &relaycommon.ChannelMeta{}
	return ctx, info
}

func TestQuotaBatchColdCacheDoesNotTrustStaleDatabaseBalance(t *testing.T) {
	gin.SetMode(gin.TestMode)
	trustQuota := common.GetTrustQuota()
	require.Positive(t, trustQuota)
	initialQuota := trustQuota + 100
	const preConsumedQuota = 100

	t.Run("cold cache without pending balance generation reserves and settles once", func(t *testing.T) {
		fixture := newQuotaBatchColdTrustFixture(t, "cold-trust-control", initialQuota)
		ctx, info := fixture.billingInput("cold-trust-control-request")

		session, apiErr := NewBillingSession(ctx, info, preConsumedQuota)

		require.Nil(t, apiErr)
		require.NotNil(t, session)
		require.False(t, session.trusted)
		require.Equal(t, preConsumedQuota, session.GetPreConsumedQuota())
		require.NoError(t, session.Settle(60))

		userCacheKey := "user:" + strconv.Itoa(fixture.user.Id)
		tokenCacheKey := "token:" + common.GenerateHMAC(fixture.token.Key)
		readBalances := func() (int64, int64, int64) {
			userQuota, err := common.RDB.HGet(context.Background(), userCacheKey, "Quota").Int64()
			require.NoError(t, err)
			tokenQuota, err := common.RDB.HGet(context.Background(), tokenCacheKey, "RemainQuota").Int64()
			require.NoError(t, err)
			tokenUsed, err := common.RDB.HGet(context.Background(), tokenCacheKey, "UsedQuota").Int64()
			require.NoError(t, err)
			return userQuota, tokenQuota, tokenUsed
		}
		userQuota, tokenQuota, tokenUsed := readBalances()
		require.Equal(t, int64(initialQuota-60), userQuota)
		require.Equal(t, int64(initialQuota-60), tokenQuota)
		require.Equal(t, int64(60), tokenUsed)
		var generationCount int64
		require.NoError(t, fixture.db.Model(&model.QuotaBalanceBatchDrain{}).Count(&generationCount).Error)

		require.NoError(t, session.Settle(60))
		replayedUserQuota, replayedTokenQuota, replayedTokenUsed := readBalances()
		require.Equal(t, userQuota, replayedUserQuota)
		require.Equal(t, tokenQuota, replayedTokenQuota)
		require.Equal(t, tokenUsed, replayedTokenUsed)
		var replayedGenerationCount int64
		require.NoError(t, fixture.db.Model(&model.QuotaBalanceBatchDrain{}).Count(&replayedGenerationCount).Error)
		require.Equal(t, generationCount, replayedGenerationCount)
	})

	t.Run("evicted cache with pending debit rejects stale database trust", func(t *testing.T) {
		fixture := newQuotaBatchColdTrustFixture(t, "cold-trust-pending", initialQuota)
		pendingDebit := trustQuota + 50
		effectiveQuota := int64(initialQuota - pendingDebit)
		require.Less(t, effectiveQuota, int64(preConsumedQuota))

		require.NoError(t, model.DecreaseUserQuota(fixture.user.Id, pendingDebit, false))
		var pending model.QuotaBalanceBatchDrain
		require.NoError(t, fixture.db.Order("id DESC").First(&pending).Error)
		require.Equal(t, "pending", pending.State)
		require.True(t, pending.CacheApplied)
		require.Equal(t, -int64(pendingDebit), pending.Payload.UserQuota[fixture.user.Id])
		userCacheKey := "user:" + strconv.Itoa(fixture.user.Id)
		cachedQuota, err := common.RDB.HGet(context.Background(), userCacheKey, "Quota").Int64()
		require.NoError(t, err)
		require.Equal(t, effectiveQuota, cachedQuota)

		var databaseBefore model.User
		require.NoError(t, fixture.db.First(&databaseBefore, fixture.user.Id).Error)
		require.Equal(t, int64(initialQuota), databaseBefore.Quota, "pending generation must not already be reflected in the database")
		require.NoError(t, model.InvalidateUserQuotaCache(fixture.user.Id))
		require.False(t, fixture.server.Exists(userCacheKey))

		ctx, info := fixture.billingInput("cold-trust-pending-request")
		session, apiErr := NewBillingSession(ctx, info, preConsumedQuota)

		var generationAfter model.QuotaBalanceBatchDrain
		require.NoError(t, fixture.db.First(&generationAfter, pending.ID).Error)
		require.Equal(t, pending.ID, generationAfter.ID)
		require.Equal(t, pending.SchemaVersion, generationAfter.SchemaVersion)
		require.Equal(t, pending.GenerationKey, generationAfter.GenerationKey)
		require.Equal(t, pending.WriterEpoch, generationAfter.WriterEpoch)
		require.Equal(t, pending.PayloadFingerprint, generationAfter.PayloadFingerprint)
		require.Equal(t, pending.Payload, generationAfter.Payload)
		require.Equal(t, pending.SubjectKind, generationAfter.SubjectKind)
		require.Equal(t, pending.SubjectID, generationAfter.SubjectID)
		var databaseAfter model.User
		require.NoError(t, fixture.db.First(&databaseAfter, fixture.user.Id).Error)
		if generationAfter.State == "applied" {
			require.Equal(t, effectiveQuota, databaseAfter.Quota)
		} else {
			require.Equal(t, "pending", generationAfter.State)
			require.Equal(t, int64(initialQuota), databaseAfter.Quota)
		}
		var tokenAfter model.Token
		require.NoError(t, fixture.db.First(&tokenAfter, fixture.token.Id).Error)
		require.Equal(t, int64(initialQuota), tokenAfter.RemainQuota)
		require.Zero(t, tokenAfter.UsedQuota)

		if session != nil {
			assert.False(t, session.trusted, "a pending debit must prevent the stale database trust bypass")
			assert.NotZero(t, session.GetPreConsumedQuota(), "a pending debit must not produce a zero reservation")
			session.finishInflight()
		}
		require.Nil(t, session)
		require.NotNil(t, apiErr)
	})
}
