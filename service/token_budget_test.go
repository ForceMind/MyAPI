package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/model"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	relayconstant "github.com/ForceMind/MyAPI/relay/constant"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	hosttypes "github.com/ForceMind/MyAPI/types"
	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func strictTokenBudgetFixture(t *testing.T, mode model.QuotaWriterMode) (*gorm.DB, *gin.Context, *relaycommon.RelayInfo, int) {
	t.Helper()
	// The notification limiter owns one intentionally long-lived worker.
	cleanupOnce.Do(startCleanupTask)
	backgroundWorkers := gopool.WorkerCount()
	db := setupPostConsumeModeDB(t, mode)
	require.NoError(t, db.AutoMigrate(&model.TokenBudget{}, &model.TokenBudgetReservation{}, &model.TokenBudgetPolicyChange{}, &model.UsageReviewDecision{}, &model.Log{}, &model.BillingLogProjectionIdentity{}))
	oldLog, oldEnabled := model.LOG_DB, common.LogConsumeEnabled
	model.LOG_DB, common.LogConsumeEnabled = db, true
	t.Cleanup(func() { model.LOG_DB, common.LogConsumeEnabled = oldLog, oldEnabled })
	user, token := seedAuthoritativeBilling(t, db, "strict-budget", 1000, 1000, false)
	root := &model.User{Username: "strict-budget-root", AffCode: "strict-budget-root", Role: common.RoleRootUser, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(root).Error)
	_, err := model.ConfigureTokenBudget(context.Background(), db, root.Id, model.TokenBudgetPolicyInput{ID: strings.Repeat("a", 64), TokenID: token.Id, Enabled: true, Limit: 100})
	require.NoError(t, err)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	ctx.Set(common.RequestIdKey, "strict-budget-request")
	info := authoritativeRelay(user, token, "strict-budget-request")
	info.StrictTokenBudget, info.RelayMode, info.StartTime = true, relayconstant.RelayModeResponses, time.Now()
	info.ChannelMeta = &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenAI, ChannelId: 7, UpstreamModelName: "budget-fixture"}
	info.PriceData = hosttypes.PriceData{ModelRatio: 1, CompletionRatio: 2, CacheRatio: 1, GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1}}
	info.SetEstimatePromptTokens(100)
	session, apiErr := NewBillingSession(ctx, info, 100)
	require.Nil(t, apiErr)
	info.Billing = session
	t.Cleanup(func() {
		// PostTextConsumeQuota schedules metrics/notification work. Drain it
		// before the fixture restores process-wide DB/Redis globals.
		require.Eventually(t, func() bool { return gopool.WorkerCount() <= backgroundWorkers }, 5*time.Second, time.Millisecond)
	})
	return db, ctx, info, root.Id
}

func TestStrictTokenBudgetNormalAndUnknownSettlementBothWriters(t *testing.T) {
	for _, mode := range []model.QuotaWriterMode{model.QuotaWriterModeLegacy, model.QuotaWriterModeAuthoritative} {
		for _, unknown := range []bool{false, true} {
			name := string(mode) + "-reported"
			if unknown {
				name = string(mode) + "-unknown"
			}
			t.Run(name, func(t *testing.T) {
				db, ctx, info, rootID := strictTokenBudgetFixture(t, mode)
				client := tokenBudgetCountTestClient(t, func(w http.ResponseWriter, r *http.Request) {
					_, _ = io.WriteString(w, `{"object":"response.input_tokens","input_tokens":10}`)
				})
				request, _ := tokenBudgetOutboundFixture(t, `{"model":"budget-fixture","input":"hello","max_output_tokens":20}`)
				require.NoError(t, PrepareTokenBudgetDispatch(ctx, client, request, info))
				var reservation model.TokenBudgetReservation
				require.NoError(t, db.First(&reservation, "request_id = ?", info.RequestId).Error)
				assert.Equal(t, model.TokenBudgetSent, reservation.State)
				assert.Contains(t, reservation.PricingEvidence, `"strict_token_budget":true`)
				var usage *dto.Usage
				if !unknown {
					usage = &dto.Usage{BillingUsage: dto.NewOpenAIResponsesBillingUsage(&dto.Usage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15,
						InputTokensDetails: &dto.InputTokenDetails{CachedTokens: 6}, OutputTokensDetails: &dto.OutputTokenDetails{ReasoningTokens: 2}})}
				}
				PostTextConsumeQuota(ctx, info, usage, nil)
				assert.Equal(t, unknown, FinalizeTokenBudgetDispatch(ctx, info))
				var budget model.TokenBudget
				require.NoError(t, db.First(&budget, "token_id = ?", info.TokenId).Error)
				if !unknown {
					assert.EqualValues(t, 15, budget.Used, "cache/reasoning are included, not extra token usage")
					assert.Zero(t, budget.Reserved)
					assert.Empty(t, budget.PendingRequestID)
					return
				}
				assert.Zero(t, budget.Used)
				assert.EqualValues(t, 30, budget.Reserved)
				assert.Equal(t, info.RequestId, budget.PendingRequestID)
				assert.False(t, info.Billing.NeedsRefund())
				view, err := model.GetUsageReview(context.Background(), db, rootID, info.RequestId)
				require.NoError(t, err)
				require.NotNil(t, view.TokenBudget)
				_, err = model.ReconcileUsageReview(context.Background(), db, rootID, info.RequestId, 20, "verified fixture evidence")
				require.ErrorIs(t, err, model.ErrTokenBudgetInvalid, "old quota-only recovery cannot release a strict token budget")
				counts := model.UsageReviewTokenCounts{Input: 10, Output: 5}
				view, err = model.ReconcileUsageReview(context.Background(), db, rootID, info.RequestId, 20, "verified fixture evidence", counts)
				require.NoError(t, err)
				require.NotNil(t, view.Decision)
				require.NoError(t, model.ProjectUsageReviewDecision(context.Background(), db, db, view.Decision.ID))
				_, err = model.ReconcileUsageReview(context.Background(), db, rootID, info.RequestId, 20, "verified fixture evidence", counts)
				require.NoError(t, err)
				require.NoError(t, model.ProjectUsageReviewDecision(context.Background(), db, db, view.Decision.ID))
				require.NoError(t, db.First(&budget, "token_id = ?", info.TokenId).Error)
				assert.EqualValues(t, 15, budget.Used)
				assert.Zero(t, budget.Reserved)
				var logs []model.Log
				require.NoError(t, db.Where("request_id = ? AND type = ?", info.RequestId, model.LogTypeConsume).Find(&logs).Error)
				require.Len(t, logs, 1)
				assert.Equal(t, 10, logs[0].PromptTokens)
				assert.Equal(t, 5, logs[0].CompletionTokens)
				assert.Contains(t, logs[0].Other, `"token_counts_confirmed":true`)
			})
		}
	}
}

func TestStrictTokenBudgetDispatchFailureKeepsBothReservations(t *testing.T) {
	db, ctx, info, _ := strictTokenBudgetFixture(t, model.QuotaWriterModeLegacy)
	client := tokenBudgetCountTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"object":"response.input_tokens","input_tokens":10}`)
	})
	request, _ := tokenBudgetOutboundFixture(t, `{"model":"budget-fixture","input":"hello","max_output_tokens":20}`)
	require.NoError(t, PrepareTokenBudgetDispatch(ctx, client, request, info))
	assert.True(t, FinalizeTokenBudgetDispatch(ctx, info), "a timeout after dispatch cannot be interpreted as no usage")
	assert.False(t, info.Billing.NeedsRefund())
	var budget model.TokenBudget
	require.NoError(t, db.First(&budget, "token_id = ?", info.TokenId).Error)
	assert.EqualValues(t, 30, budget.Reserved)
	legacy, err := model.FindLegacyUsageReservation(context.Background(), db, info.RequestId)
	require.NoError(t, err)
	assert.Equal(t, model.LegacyUsageUnknown, legacy.State)
	assert.EqualValues(t, 100, legacy.ReservedQuota)
	assert.Nil(t, legacy.ActualQuota)
}

func TestStrictTokenBudgetRootCancelsUnsentAndRecoversLostHold(t *testing.T) {
	for _, mode := range []model.QuotaWriterMode{model.QuotaWriterModeLegacy, model.QuotaWriterModeAuthoritative} {
		for _, dispatched := range []bool{false, true} {
			name := string(mode) + "-not-sent"
			if dispatched {
				name = string(mode) + "-lost-hold"
			}
			t.Run(name, func(t *testing.T) {
				db, ctx, info, rootID := strictTokenBudgetFixture(t, mode)
				session := info.Billing.(*BillingSession)
				t.Cleanup(session.finishInflight)
				evidence, err := usageReviewPricingEvidence(info, calculateTextQuotaSummary(ctx, info, nil))
				require.NoError(t, err)
				require.NoError(t, model.ReserveTokenBudget(context.Background(), db, model.TokenBudgetReservation{RequestID: info.RequestId, UserID: info.UserId, TokenID: info.TokenId, ChannelID: 7, ModelName: "budget-fixture", InputTokens: 10, MaxOutputTokens: 20, PayloadSHA256: strings.Repeat("b", 64), BoundSource: model.TokenBudgetBoundOpenAIResponses, PricingEvidence: string(evidence)}))
				if dispatched {
					_, err = model.MutateTokenBudgetRequest(context.Background(), db, model.TokenBudgetMutation{TokenID: info.TokenId, RequestID: info.RequestId, Action: "send"})
					require.NoError(t, err)
					info.Billing = nil // Recovery must use durable evidence, not the old session.
					view, err := model.PrepareTokenBudgetUsageReview(context.Background(), db, rootID, info.TokenId, info.RequestId)
					require.NoError(t, err)
					require.NotNil(t, view.TokenBudget)
					_, err = model.ReconcileUsageReview(context.Background(), db, rootID, info.RequestId, 20, "verified terminal provider evidence", model.UsageReviewTokenCounts{Input: 10, Output: 5})
					require.NoError(t, err)
				} else {
					_, err = model.CancelTokenBudgetBeforeSend(context.Background(), db, 0, info.TokenId, info.RequestId, "no send")
					require.Error(t, err)
					for range 2 {
						_, err = model.CancelTokenBudgetBeforeSend(context.Background(), db, rootID, info.TokenId, info.RequestId, "verified no dispatch")
						require.NoError(t, err)
					}
				}
				var budget model.TokenBudget
				require.NoError(t, db.First(&budget, "token_id = ?", info.TokenId).Error)
				assert.Zero(t, budget.Reserved)
				assert.Empty(t, budget.PendingRequestID)
				var user model.User
				require.NoError(t, db.First(&user, info.UserId).Error)
				if dispatched {
					assert.EqualValues(t, 15, budget.Used)
					assert.Equal(t, 980, user.Quota)
				} else {
					assert.Zero(t, budget.Used)
					assert.Equal(t, 1000, user.Quota)
				}
			})
		}
	}
}
