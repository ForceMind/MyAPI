package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStrictFeeBudgetReportedAndUnknownBothWriters(t *testing.T) {
	for _, mode := range []model.QuotaWriterMode{model.QuotaWriterModeLegacy, model.QuotaWriterModeAuthoritative} {
		for _, unknown := range []bool{false, true} {
			name := string(mode) + "-reported"
			if unknown {
				name = string(mode) + "-unknown-cache"
			}
			t.Run(name, func(t *testing.T) {
				db, ctx, info, rootID := strictTokenBudgetFixture(t, mode)
				_, err := model.ConfigureTokenBudget(context.Background(), db, rootID, model.TokenBudgetPolicyInput{ID: strings.Repeat("e", 64), TokenID: info.TokenId, ExpectedRevision: 1, Enabled: true, Limit: 100, Fee: &model.FeeBudgetPolicyInput{Enabled: true, LimitUSD: "0.001"}})
				require.NoError(t, err)
				info.TieredBillingSnapshot = feePriceFixture(t, db)
				info.OriginModelName, info.UpstreamModelName = "fixture-model", "fixture-model"
				client := tokenBudgetCountTestClient(t, func(w http.ResponseWriter, r *http.Request) {
					_, _ = io.WriteString(w, `{"object":"response.input_tokens","input_tokens":10}`)
				})
				request, _ := tokenBudgetOutboundFixture(t, `{"model":"fixture-model","input":"hello","max_output_tokens":20,"service_tier":"default"}`)
				require.NoError(t, PrepareTokenBudgetDispatch(ctx, client, request, info))
				usage := feeUsageFixture()
				usage.BillingUsage.OpenAIUsage = &dto.Usage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15, InputTokensDetails: &dto.InputTokenDetails{CachedTokens: 6}, OutputTokensDetails: &dto.OutputTokenDetails{ReasoningTokens: 2}}
				usage.BillingUsage.ResponsesTextEvidence.CacheRead = common.GetPointer(6)
				usage.BillingUsage.ResponsesTextEvidence.CacheWrite = common.GetPointer(0)
				if unknown {
					usage.BillingUsage.ResponsesTextEvidence.CacheWrite = nil
				}
				PostTextConsumeQuota(ctx, info, usage, nil)
				assert.Equal(t, unknown, FinalizeTokenBudgetDispatch(ctx, info))
				policy, err := model.LookupTokenBudget(context.Background(), db, info.TokenId)
				require.NoError(t, err)
				if unknown {
					assert.Zero(t, policy.Used)
					assert.Equal(t, "0", policy.FeeUsedUSD)
					assert.Equal(t, "0.000225", policy.FeeReservedUSD)
					counts := model.UsageReviewTokenCounts{Input: 10, Output: 5}
					_, err = model.ReconcileUsageReview(context.Background(), db, rootID, info.RequestId, 20, "verified exact cost evidence", counts)
					require.ErrorIs(t, err, model.ErrFeeBudgetInvalid)
					counts.FeeUSD = common.GetPointer("0.0000586")
					for range 2 {
						view, err := model.ReconcileUsageReview(context.Background(), db, rootID, info.RequestId, 20, "verified exact cost evidence", counts)
						require.NoError(t, err)
						require.NoError(t, model.ProjectUsageReviewDecision(context.Background(), db, db, view.Decision.ID))
					}
				}
				policy, err = model.LookupTokenBudget(context.Background(), db, info.TokenId)
				require.NoError(t, err)
				assert.Equal(t, "0.0000586", policy.FeeUsedUSD)
				assert.Equal(t, "0", policy.FeeReservedUSD)
				assert.Empty(t, policy.PendingRequestID)
				assert.EqualValues(t, 15, policy.Used)
				var logs []model.Log
				require.NoError(t, db.Where("request_id = ? AND type = ?", info.RequestId, model.LogTypeConsume).Find(&logs).Error)
				require.Len(t, logs, 1)
				assert.Contains(t, logs[0].Other, "0.0000586")
				assert.Contains(t, logs[0].Other, "not_invoice")
			})
		}
	}
}
