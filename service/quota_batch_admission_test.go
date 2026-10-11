package service

import (
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	relaytypes "github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type quotaAdmissionSnapshot struct {
	userQuota                  int64
	userUsedQuota              int64
	tokenRemainQuota           int64
	tokenUsedQuota             int64
	legacyReservations         int64
	settlementFacts            int64
	settlementIntents          int64
	accountMutationReceipts    int64
	userQuotaMutationReceipts  int64
	accountReservationHeadRows int64
}

func loadQuotaAdmissionSnapshot(t *testing.T, db *gorm.DB, userID, tokenID int) quotaAdmissionSnapshot {
	t.Helper()
	var user model.User
	require.NoError(t, db.First(&user, userID).Error)
	var token model.Token
	require.NoError(t, db.First(&token, tokenID).Error)

	snapshot := quotaAdmissionSnapshot{
		userQuota:        user.Quota,
		userUsedQuota:    user.UsedQuota,
		tokenRemainQuota: token.RemainQuota,
		tokenUsedQuota:   token.UsedQuota,
	}
	require.NoError(t, db.Model(&model.LegacyUsageReservation{}).Count(&snapshot.legacyReservations).Error)
	require.NoError(t, db.Model(&model.AccountQuotaSettlementFact{}).Count(&snapshot.settlementFacts).Error)
	require.NoError(t, db.Model(&model.AccountQuotaSettlementIntent{}).Count(&snapshot.settlementIntents).Error)
	require.NoError(t, db.Model(&model.AccountQuotaMutationReceipt{}).Count(&snapshot.accountMutationReceipts).Error)
	require.NoError(t, db.Model(&model.UserQuotaMutationReceipt{}).Count(&snapshot.userQuotaMutationReceipts).Error)
	require.NoError(t, db.Model(&model.AccountQuotaReservationHead{}).Count(&snapshot.accountReservationHeadRows).Error)
	return snapshot
}

func TestQuotaBatchAdmissionRejectsCacheDependentWritersWithoutRedis(t *testing.T) {
	for _, mode := range []model.QuotaWriterMode{model.QuotaWriterModeLegacy, model.QuotaWriterModeBridge} {
		for _, redisEnabled := range []bool{false, true} {
			for _, forcePreConsume := range []bool{false, true} {
				admissionKind := "trusted"
				if forcePreConsume {
					admissionKind = "forced"
				}
				label := fmt.Sprintf("%s-r%v-%s", mode, redisEnabled, admissionKind)
				name := fmt.Sprintf("%s/redis-enabled-%v-rdb-nil/%s", mode, redisEnabled, admissionKind)
				t.Run(name, func(t *testing.T) {
					db := setupPostConsumeModeDB(t, mode)
					common.BatchUpdateEnabled = true
					common.RedisEnabled = redisEnabled
					common.RDB = nil

					seedQuota := common.GetTrustQuota() + 1000
					user, token := seedAuthoritativeBilling(t, db, "batch-admission-"+label, seedQuota, seedQuota, false)
					ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
					ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
					ctx.Set("token_quota", token.RemainQuota)
					info := authoritativeRelay(user, token, "batch-admission-"+label)
					info.ChannelMeta = &relaycommon.ChannelMeta{}
					info.ForcePreConsume = forcePreConsume
					before := loadQuotaAdmissionSnapshot(t, db, user.Id, token.Id)

					session, apiErr := NewBillingSession(ctx, info, 100)

					require.Nil(t, session)
					require.NotNil(t, apiErr)
					require.ErrorIs(t, apiErr, model.ErrBatchQuotaCacheUnavailable)
					require.Equal(t, relaytypes.ErrorCodeUpdateDataError, apiErr.GetErrorCode())
					require.True(t, relaytypes.IsSkipRetryError(apiErr))
					require.Equal(t, before, loadQuotaAdmissionSnapshot(t, db, user.Id, token.Id))
				})
			}
		}
	}
}

func TestQuotaBatchAdmissionKeepsLegacyWithoutBatching(t *testing.T) {
	db := setupPostConsumeModeDB(t, model.QuotaWriterModeLegacy)
	common.BatchUpdateEnabled = false
	common.RedisEnabled = false
	common.RDB = nil
	user, token := seedAuthoritativeBilling(t, db, "legacy-no-batch", 1000, 500, false)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	ctx.Set("token_quota", token.RemainQuota)
	info := authoritativeRelay(user, token, "legacy-no-batch-request")
	info.ChannelMeta = &relaycommon.ChannelMeta{}
	info.ForcePreConsume = true

	session, apiErr := NewBillingSession(ctx, info, 100)
	require.Nil(t, apiErr)
	require.NotNil(t, session)
	require.NoError(t, session.Settle(60))
	userQuota, tokenQuota, tokenUsed := loadPostConsumeBalances(t, db, user, token)
	require.Equal(t, int64(940), userQuota)
	require.Equal(t, int64(440), tokenQuota)
	require.Equal(t, int64(60), tokenUsed)
}

func TestQuotaBatchAdmissionKeepsAuthoritativeWithoutRedis(t *testing.T) {
	db := setupPostConsumeModeDB(t, model.QuotaWriterModeAuthoritative)
	common.BatchUpdateEnabled = true
	common.RedisEnabled = false
	common.RDB = nil
	user, token := seedAuthoritativeBilling(t, db, "authoritative-no-redis", 1000, 500, false)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	ctx.Set("token_quota", token.RemainQuota)
	info := authoritativeRelay(user, token, "authoritative-no-redis-request")
	info.ChannelMeta = &relaycommon.ChannelMeta{}
	info.ForcePreConsume = true

	session, apiErr := NewBillingSession(ctx, info, 100)
	require.Nil(t, apiErr)
	require.NotNil(t, session)
	require.NoError(t, session.Settle(60))
	userQuota, tokenQuota, tokenUsed := loadPostConsumeBalances(t, db, user, token)
	require.Equal(t, int64(940), userQuota)
	require.Equal(t, int64(440), tokenQuota)
	require.Equal(t, int64(60), tokenUsed)
}
