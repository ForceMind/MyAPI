package service

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/model"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	hosttypes "github.com/ForceMind/MyAPI/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBillingUnknownUsagePersistsAndBlocksSessionTerminals(t *testing.T) {
	db := setupAuthoritativeBillingDB(t)
	user, token := seedAuthoritativeBilling(t, db, "usage-review", 1000, 1000, false)
	session, apiErr := NewBillingSession(nil, authoritativeRelay(user, token, "usage-review-session"), 100)
	require.Nil(t, apiErr)
	require.NoError(t, session.HoldUnknownUsage(context.Background(), "estimated"))
	assert.False(t, session.NeedsRefund())
	require.ErrorIs(t, session.Settle(120), model.ErrAccountQuotaUsageUnresolved)
	require.ErrorIs(t, session.Refund(nil), model.ErrAccountQuotaUsageUnresolved)
	require.ErrorIs(t, session.Reserve(150), model.ErrAccountQuotaUsageUnresolved)
	blocked, err := model.AccountQuotaUsageNeedsReview(context.Background(), db, token.Id)
	require.NoError(t, err)
	assert.True(t, blocked)
	// A new model call does not inherit process-local session flags.
	_, err = model.RefundAccountQuota(context.Background(), db, model.AccountQuotaTerminalInput{
		RequestID: "usage-review-session", ReserveReceiptID: session.reserveReceipt.ID, AuditKey: "fresh-worker-refund",
	})
	require.Error(t, err)
	require.NoError(t, db.First(user, user.Id).Error)
	require.NoError(t, db.First(token, token.Id).Error)
	assert.Equal(t, 900, user.Quota)
	assert.Equal(t, 900, token.RemainQuota)
}

func TestTextUnknownUsagePersistsAcrossBothWriterModes(t *testing.T) {
	for _, mode := range []model.QuotaWriterMode{model.QuotaWriterModeLegacy, model.QuotaWriterModeAuthoritative} {
		for _, estimated := range []bool{false, true} {
			t.Run(string(mode)+map[bool]string{false: "-missing", true: "-estimated"}[estimated], func(t *testing.T) {
				db := setupPostConsumeModeDB(t, mode)
				user, token := seedAuthoritativeBilling(t, db, "text-hold", 1000, 1000, false)
				info := authoritativeRelay(user, token, "text-hold-request")
				info.StartTime = time.Now()
				info.ChannelMeta = &relaycommon.ChannelMeta{ChannelId: 77}
				info.PriceData = hosttypes.PriceData{ModelRatio: 1, CompletionRatio: 2, GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1}}
				info.SetEstimatePromptTokens(100)
				ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
				ctx.Request = httptest.NewRequest("POST", "/v1/responses", nil)
				ctx.Set(common.RequestIdKey, info.RequestId)
				session, apiErr := NewBillingSession(ctx, info, 100)
				require.Nil(t, apiErr)
				info.Billing = session
				var usage *dto.Usage
				if estimated {
					billing := dto.NewOpenAIResponsesBillingUsage(&dto.Usage{InputTokens: 100, OutputTokens: 10, TotalTokens: 110})
					billing.Estimated = true
					usage = &dto.Usage{BillingUsage: billing}
				}
				PostTextConsumeQuota(ctx, info, usage, nil)
				// A later call with reported usage cannot bypass a held session
				// or manufacture confirmed statistics before audited reconciliation.
				PostTextConsumeQuota(ctx, info, &dto.Usage{BillingUsage: dto.NewOpenAIResponsesBillingUsage(&dto.Usage{InputTokens: 100, OutputTokens: 10, TotalTokens: 110})}, nil)
				require.ErrorIs(t, session.Settle(120), model.ErrAccountQuotaUsageUnresolved)
				require.ErrorIs(t, session.Refund(ctx), model.ErrAccountQuotaUsageUnresolved)
				require.NoError(t, db.First(user, user.Id).Error)
				require.NoError(t, db.First(token, token.Id).Error)
				assert.Equal(t, 900, user.Quota)
				assert.Zero(t, user.UsedQuota)
				assert.Zero(t, user.RequestCount)
				assert.Equal(t, 900, token.RemainQuota)
				if mode == model.QuotaWriterModeLegacy {
					record, err := model.FindLegacyUsageReservation(context.Background(), db, info.RequestId)
					require.NoError(t, err)
					assert.Equal(t, model.LegacyUsageUnknown, record.State)
					assert.Nil(t, record.ActualQuota)
					assert.EqualValues(t, 100, record.ReservedQuota)
					assert.Contains(t, record.ReviewMetadata, "quota_unit")
				} else {
					var record model.AccountQuotaTerminalRecoveryObligation
					require.NoError(t, db.Where("request_id = ?", info.RequestId).First(&record).Error)
					assert.Equal(t, model.AccountQuotaTerminalRecoveryUsageUnknown, record.State)
					assert.Zero(t, record.TerminalReceiptID)
					assert.Contains(t, record.ReviewMetadata, "quota_unit")
				}
			})
		}
	}
}

func TestTextUsageEvidenceRejectsContradictoryCategories(t *testing.T) {
	for _, tc := range []struct {
		name   string
		usage  dto.Usage
		reason string
	}{
		{"included cache and reasoning", dto.Usage{InputTokens: 100, OutputTokens: 10, TotalTokens: 110, InputTokensDetails: &dto.InputTokenDetails{CachedTokens: 40}, OutputTokensDetails: &dto.OutputTokenDetails{ReasoningTokens: 4}}, ""},
		{"negative cache", dto.Usage{InputTokens: 100, OutputTokens: 10, TotalTokens: 110, InputTokensDetails: &dto.InputTokenDetails{CachedTokens: -1}}, "invalid"},
		{"overlapping cache categories", dto.Usage{InputTokens: 100, OutputTokens: 10, TotalTokens: 110, InputTokensDetails: &dto.InputTokenDetails{CachedTokens: 70, CacheWriteTokens: 40}}, "invalid"},
		{"reasoning exceeds output", dto.Usage{InputTokens: 100, OutputTokens: 10, TotalTokens: 110, OutputTokensDetails: &dto.OutputTokenDetails{ReasoningTokens: 11}}, "invalid"},
		{"inconsistent total", dto.Usage{InputTokens: 100, OutputTokens: 10, TotalTokens: 111}, "invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			usage := &dto.Usage{BillingUsage: dto.NewOpenAIResponsesBillingUsage(&tc.usage)}
			assert.Equal(t, tc.reason, textUsageReviewReason(ctx, usage))
		})
	}
}

func TestReportedUsageOverridesEarlierLocalEstimateMarker(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyLocalCountTokens, true)
	usage := &dto.Usage{BillingUsage: dto.NewOpenAIResponsesBillingUsage(&dto.Usage{InputTokens: 100, OutputTokens: 10, TotalTokens: 110})}
	assert.Empty(t, textUsageReviewReason(ctx, usage))
	usage.BillingUsage.Estimated = true
	assert.Equal(t, "estimated", textUsageReviewReason(ctx, usage))
	assert.Equal(t, "estimated", textUsageReviewReason(ctx, &dto.Usage{PromptTokens: 100, CompletionTokens: 10}))
	zero := &dto.Usage{BillingUsage: &dto.BillingUsage{Source: dto.BillingUsageSourceOAIResponses, Semantic: dto.BillingUsageSemanticOpenAI, OpenAIUsage: &dto.Usage{}}}
	assert.Empty(t, textUsageReviewReason(ctx, zero), "explicit reported zero must not become an estimate")
}

func TestAudioAndRealtimeUnverifiedUsageNeverBecomesConfirmed(t *testing.T) {
	for _, mode := range []string{"audio-missing", "realtime-missing", "realtime-estimated"} {
		t.Run(mode, func(t *testing.T) {
			truncate(t)
			seedUser(t, 92, 1000)
			seedChannel(t, 92)
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest("POST", "/v1/realtime", nil)
			settler := &textQuotaTestSettler{preConsumed: 100}
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 92}, UserId: 92, OriginModelName: "unverified-fixture", StartTime: time.Now(), Billing: settler, FinalPreConsumedQuota: 100,
				PriceData: hosttypes.PriceData{ModelRatio: 1, CompletionRatio: 2, GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1}}}
			require.NoError(t, info.PriceData.CaptureQuotaUnit(1000))
			require.NotPanics(t, func() {
				if mode == "audio-missing" {
					PostAudioConsumeQuota(ctx, info, nil, "")
					return
				}
				var usage *dto.RealtimeUsage
				if mode == "realtime-estimated" {
					info.RealtimeUsageUnverified = true
					usage = &dto.RealtimeUsage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15, InputTokenDetails: dto.InputTokenDetails{TextTokens: 10}, OutputTokenDetails: dto.OutputTokenDetails{TextTokens: 5}}
				}
				PostWssConsumeQuota(ctx, info, info.OriginModelName, usage, "")
			})
			assert.Empty(t, settler.settled)
			assert.Zero(t, settler.refund)
		})
	}
}

func TestLegacyUsageAdmissionClaimsRequestBeforeAnyDuplicateReserve(t *testing.T) {
	db := setupPostConsumeModeDB(t, model.QuotaWriterModeLegacy)
	user, token := seedAuthoritativeBilling(t, db, "claim-before-reserve", 1000, 1000, false)
	start := make(chan struct{})
	results := make(chan *BillingSession, 2)
	for range 2 {
		go func() {
			<-start
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest("POST", "/v1/responses", nil)
			info := authoritativeRelay(user, token, "duplicate-admission")
			info.ForcePreConsume = true
			session, apiErr := NewBillingSession(ctx, info, 100)
			if apiErr != nil {
				results <- nil
				return
			}
			results <- session
		}()
	}
	close(start)
	var winner *BillingSession
	winners := 0
	for range 2 {
		if session := <-results; session != nil {
			winner = session
			winners++
		}
	}
	require.Equal(t, 1, winners)
	require.NoError(t, winner.HoldUnknownUsage(context.Background(), "missing"))
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	_, apiErr := NewBillingSession(ctx, authoritativeRelay(user, token, "duplicate-admission"), 100)
	require.NotNil(t, apiErr)
	require.NoError(t, db.First(user, user.Id).Error)
	require.NoError(t, db.First(token, token.Id).Error)
	assert.Equal(t, 900, user.Quota)
	assert.Equal(t, 900, token.RemainQuota)
	assert.Equal(t, 100, token.UsedQuota)
}
