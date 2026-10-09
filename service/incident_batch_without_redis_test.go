package service

import (
	"context"
	"fmt"
	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"testing"
	"time"
)

// Synthetic accounting regressions. No production data or upstream calls.
func TestIncidentSyntheticZeroReserveBatchWithoutRedis(t *testing.T) {
	for _, batch := range []bool{false, true} {
		for _, actual := range []int{0, 60} {
			t.Run(fmt.Sprintf("batch-%v/actual-%d", batch, actual), func(t *testing.T) {
				db := setupPostConsumeModeDB(t, model.QuotaWriterModeLegacy)
				common.BatchUpdateEnabled = batch
				require.NoError(t, db.AutoMigrate(&model.Log{}))
				trust := common.GetTrustQuota()
				user, token := seedAuthoritativeBilling(t, db, "incident-synthetic", trust+1000, trust+1000, false)
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
				c.Set("token_quota", token.RemainQuota)
				info := authoritativeRelay(user, token, "incident-synthetic-request")
				info.ChannelMeta = &relaycommon.ChannelMeta{}
				info.StartTime = time.Now()
				c.Set(common.RequestIdKey, info.RequestId)
				session, apiErr := NewBillingSession(c, info, 100)
				require.Nil(t, apiErr, "trusted request must be admitted despite missing Redis")
				info.Billing = session
				require.Zero(t, info.FinalPreConsumedQuota)
				require.True(t, session.trusted)
				err := SettleBilling(c, info, actual)
				expectedCharge := actual
				if batch && actual > 0 {
					expectedCharge = 0
					require.ErrorIs(t, err, model.ErrBatchQuotaCacheUnavailable)
					require.ErrorIs(t, err, model.ErrAccountQuotaSettlementPending)
					var fact model.AccountQuotaSettlementFact
					require.NoError(t, db.Where("request_id = ?", info.RequestId).First(&fact).Error)
					require.Equal(t, model.AccountQuotaSettlementRetryable, fact.State)
					require.EqualValues(t, actual, fact.Delta)
					require.False(t, fact.FundingApplied)
					require.False(t, fact.TokenApplied)
					require.Contains(t, fact.LastError, "batch quota cache is unavailable")
					row, readErr := model.FindLegacyUsageReservation(context.Background(), db, info.RequestId)
					require.NoError(t, readErr)
					require.Equal(t, model.LegacyUsagePrepared, row.State)
					require.Nil(t, row.ActualQuota)
					t.Logf("reserved=0 actual=%d fact_state=%s funding=%v token=%v journal=%s last_error=%s", actual, fact.State, fact.FundingApplied, fact.TokenApplied, row.State, fact.LastError)
				} else {
					require.NoError(t, err)
				}
				uq, tq, used := loadPostConsumeBalances(t, db, user, token)
				require.Equal(t, trust+1000-expectedCharge, uq)
				require.Equal(t, trust+1000-expectedCharge, tq)
				require.Equal(t, expectedCharge, used)
			})
		}
	}
}

func TestIncidentSyntheticTextUsageLogBatchWithoutRedis(t *testing.T) {
	db := setupPostConsumeModeDB(t, model.QuotaWriterModeLegacy)
	common.BatchUpdateEnabled = true
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	oldLogDB := model.LOG_DB
	model.LOG_DB = db
	t.Cleanup(func() { model.LOG_DB = oldLogDB })
	trust := common.GetTrustQuota()
	user, token := seedAuthoritativeBilling(t, db, "incident-text", trust+1000, trust+1000, false)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	c.Set("token_quota", token.RemainQuota)
	info := authoritativeRelay(user, token, "incident-text-request")
	info.ChannelMeta = &relaycommon.ChannelMeta{}
	info.StartTime = time.Now()
	info.PriceData.ModelRatio = 1
	info.PriceData.CompletionRatio = 1
	info.PriceData.GroupRatioInfo.GroupRatio = 1
	c.Set(common.RequestIdKey, info.RequestId)
	session, apiErr := NewBillingSession(c, info, 100)
	require.Nil(t, apiErr)
	info.Billing = session
	require.Zero(t, info.FinalPreConsumedQuota)
	require.NoError(t, PrepareTextUsageDispatch(c, httptest.NewRequest("POST", "https://synthetic.invalid/never-sent", nil), info))
	usage := &dto.Usage{PromptTokens: 40, CompletionTokens: 20, TotalTokens: 60}
	require.Equal(t, "", textUsageReviewReason(c, usage), "synthetic usage is complete and valid")
	require.Equal(t, 60, calculateTextQuotaSummary(c, info, usage).Quota)
	PostTextConsumeQuota(c, info, usage, nil)
	var log model.Log
	require.NoError(t, db.Where("request_id = ?", info.RequestId).First(&log).Error)
	require.Equal(t, "usage_settlement_pending", log.Content)
	var other map[string]interface{}
	require.NoError(t, common.UnmarshalJsonStr(log.Other, &other))
	require.Equal(t, "pending_review", other["settlement_status"])
	require.Nil(t, other["actual_quota"])
	require.Equal(t, float64(0), other["reserved_quota"])
	var fact model.AccountQuotaSettlementFact
	require.NoError(t, db.Where("request_id = ?", info.RequestId).First(&fact).Error)
	require.Equal(t, model.AccountQuotaSettlementRetryable, fact.State)
	require.EqualValues(t, 60, fact.Delta)
	require.False(t, fact.FundingApplied)
	require.False(t, fact.TokenApplied)
	t.Logf("valid upstream-style usage=40+20 log=%s other=%s fact_error=%s", log.Content, log.Other, fact.LastError)
}

func TestIncidentSyntheticAuthoritativeRedisAbsenceControl(t *testing.T) {
	db := setupPostConsumeModeDB(t, model.QuotaWriterModeAuthoritative)
	common.BatchUpdateEnabled = true
	trust := common.GetTrustQuota()
	user, token := seedAuthoritativeBilling(t, db, "incident-authoritative", trust+1000, trust+1000, false)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	c.Set("token_quota", token.RemainQuota)
	info := authoritativeRelay(user, token, "incident-authoritative-request")
	info.ChannelMeta = &relaycommon.ChannelMeta{}
	info.StartTime = time.Now()
	session, apiErr := NewBillingSession(c, info, 100)
	require.Nil(t, apiErr)
	info.Billing = session
	require.Zero(t, info.FinalPreConsumedQuota)
	require.NoError(t, SettleBilling(c, info, 60))
	uq, tq, used := loadPostConsumeBalances(t, db, user, token)
	require.Equal(t, trust+940, uq)
	require.Equal(t, trust+940, tq)
	require.Equal(t, 60, used)
	var fact model.AccountQuotaSettlementFact
	require.NoError(t, db.Where("request_id = ?", info.RequestId).First(&fact).Error)
	require.Equal(t, model.AccountQuotaSettlementApplied, fact.State)
	require.True(t, fact.FundingApplied)
	require.True(t, fact.TokenApplied)
	t.Log("authoritative writer settles60 with batch=true and Redis disabled; cache projection is separate")
}
