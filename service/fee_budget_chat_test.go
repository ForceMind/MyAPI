package service

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	relayconstant "github.com/ForceMind/MyAPI/relay/constant"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestStrictChatFeeFreezesBothProfilesAndContextBound(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/fee-chat.db"), &gorm.Config{})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	snapshot := feePriceFixtureForModel(t, db, model.TokenBudgetOpenAIChatModel)
	row := &model.TokenBudgetReservation{BoundSource: model.TokenBudgetBoundOpenAIChat, ModelName: model.TokenBudgetOpenAIChatModel, InputTokens: model.TokenBudgetOpenAIChatContext, MaxOutputTokens: model.TokenBudgetOpenAIChatMaxOutput, RequestServiceTier: "default"}
	info := &relaycommon.RelayInfo{TieredBillingSnapshot: snapshot}
	require.NoError(t, freezeFeeBudgetPrice(context.Background(), db, info, row))
	assert.Equal(t, "6.53", row.FeeReservedUSD)
	assert.Contains(t, row.FeePriceEvidence, `"version":2`)
	for _, tc := range []struct {
		input, output, read, write, reason int
		want                               string
	}{
		{100, 10, 40, 30, 4, "0.000239"},
		{272000, 10, 0, 0, 4, "0.5441"},
		{272001, 10, 0, 0, 4, "1.088154"},
		{0, 0, 0, 0, 0, "0"},
	} {
		usage := chatBudgetUsage(tc.input, tc.output)
		evidence := usage.BillingUsage.ChatTextEvidence
		evidence.CacheRead, evidence.CacheWrite, evidence.Reasoning = common.GetPointer(tc.read), common.GetPointer(tc.write), common.GetPointer(tc.reason)
		usage.BillingUsage.OpenAIUsage.PromptTokensDetails = dto.InputTokenDetails{CachedTokens: tc.read, CacheWriteTokens: tc.write}
		usage.BillingUsage.OpenAIUsage.CompletionTokenDetails.ReasoningTokens = tc.reason
		cost, err := calculateFeeBudgetUSD(row, usage)
		require.NoError(t, err)
		assert.Equal(t, tc.want, cost)
	}
	// A later live price or rollback cannot replace the receipt frozen on row.
	snapshot.ExprString = "p * 999"
	cost, err := calculateFeeBudgetUSD(row, chatBudgetUsage(100, 10))
	require.NoError(t, err)
	assert.Equal(t, "0.0003", cost)
	for _, change := range []func(*dto.BillingUsage){
		func(b *dto.BillingUsage) { b.ChatTextEvidence.CacheRead = nil },
		func(b *dto.BillingUsage) { b.ChatTextEvidence.CacheWrite = nil },
		func(b *dto.BillingUsage) { b.ChatTextEvidence.Model = "public-alias" },
		func(b *dto.BillingUsage) { b.ChatTextEvidence.ServiceTier = "auto" },
		func(b *dto.BillingUsage) { b.OpenAIUsage.PromptTokensDetails.CachedTokens = 1 },
		func(b *dto.BillingUsage) { b.ChatTextEvidence.Reasoning = common.GetPointer(11) },
		func(b *dto.BillingUsage) { b.OpenAIUsage.PromptTokensDetails.AudioTokens = 1 },
		func(b *dto.BillingUsage) {
			b.ChatTextEvidence.CacheRead = common.GetPointer(80)
			b.ChatTextEvidence.CacheWrite = common.GetPointer(30)
		},
		func(b *dto.BillingUsage) { b.Estimated = true },
		func(b *dto.BillingUsage) { b.Incomplete = true },
	} {
		usage := chatBudgetUsage(100, 10)
		change(usage.BillingUsage)
		_, err := calculateFeeBudgetUSD(row, usage)
		require.Error(t, err)
	}
	for _, value := range []string{"", "auto", "standard", "priority"} {
		copy := *row
		copy.RequestServiceTier = value
		_, err := calculateFeeBudgetUSD(&copy, chatBudgetUsage(100, 10))
		require.Error(t, err)
	}
	copy := *row
	copy.FeePriceEvidence = strings.ReplaceAll(copy.FeePriceEvidence, `"short_context_max_input_tokens":272000`, `"short_context_max_input_tokens":999999`)
	_, err = calculateFeeBudgetUSD(&copy, chatBudgetUsage(100, 10))
	require.Error(t, err)
}

func TestStrictChatFeeUnknownRecoveryBothWriters(t *testing.T) {
	for _, mode := range []model.QuotaWriterMode{model.QuotaWriterModeLegacy, model.QuotaWriterModeAuthoritative} {
		for _, unknown := range []bool{false, true} {
			t.Run(string(mode)+map[bool]string{false: "-actual", true: "-missing-cache"}[unknown], func(t *testing.T) {
				db, ctx, info, root := strictTokenBudgetFixture(t, mode)
				_, err := model.ConfigureTokenBudget(context.Background(), db, root, model.TokenBudgetPolicyInput{ID: strings.Repeat("e", 64), TokenID: info.TokenId, ExpectedRevision: 1, Enabled: true, Limit: model.TokenBudgetOpenAIChatContext, Fee: &model.FeeBudgetPolicyInput{Enabled: true, LimitUSD: "10"}})
				require.NoError(t, err)
				info.RelayMode, info.UpstreamModelName, info.OriginModelName = relayconstant.RelayModeChatCompletions, model.TokenBudgetOpenAIChatModel, model.TokenBudgetOpenAIChatModel
				info.TieredBillingSnapshot = feePriceFixtureForModel(t, db, model.TokenBudgetOpenAIChatModel)
				ctx.Request.URL.Path = "/v1/chat/completions"
				request, _ := chatBudgetOutbound(t, chatBudgetBody)
				client := tokenBudgetCountTestClient(t, func(http.ResponseWriter, *http.Request) { t.Error("no count call") })
				require.NoError(t, PrepareTokenBudgetDispatch(ctx, client, request, info))
				usage := chatBudgetUsage(10, 5)
				usage.BillingUsage.ChatTextEvidence.CacheRead = common.GetPointer(6)
				usage.BillingUsage.OpenAIUsage.PromptTokensDetails.CachedTokens = 6
				if unknown {
					usage.BillingUsage.ChatTextEvidence.CacheWrite = nil
				}
				PostTextConsumeQuota(ctx, info, usage, nil)
				assert.Equal(t, unknown, FinalizeTokenBudgetDispatch(ctx, info))
				policy, err := model.LookupTokenBudget(context.Background(), db, info.TokenId)
				require.NoError(t, err)
				if unknown {
					assert.Equal(t, "5.2502", policy.FeeReservedUSD)
					assert.Equal(t, "0", policy.FeeUsedUSD)
					assert.False(t, info.Billing.NeedsRefund())
					_, err := model.PrepareTokenBudgetUsageReview(context.Background(), db, root, info.TokenId, info.RequestId, common.GetPointer("0.0000586"))
					require.NoError(t, err)
					for range 2 {
						view, err := model.ReconcileUsageReview(context.Background(), db, root, info.RequestId, 20, "synthetic Chat actual cost", model.UsageReviewTokenCounts{Input: 10, Output: 5, FeeUSD: common.GetPointer("0.0000586")})
						require.NoError(t, err)
						require.NoError(t, model.ProjectUsageReviewDecision(context.Background(), db, db, view.Decision.ID))
					}
				}
				policy, err = model.LookupTokenBudget(context.Background(), db, info.TokenId)
				require.NoError(t, err)
				assert.EqualValues(t, 15, policy.Used)
				assert.Zero(t, policy.Reserved)
				assert.Equal(t, "0.0000586", policy.FeeUsedUSD)
				assert.Equal(t, "0", policy.FeeReservedUSD)
				var logs []model.Log
				require.NoError(t, db.Where("request_id = ? AND type = ?", info.RequestId, model.LogTypeConsume).Find(&logs).Error)
				require.Len(t, logs, 1)
				assert.Contains(t, logs[0].Other, "0.0000586")
			})
		}
	}
}
