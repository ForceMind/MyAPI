package service

import (
	"context"
	"fmt"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/logger"
	"github.com/ForceMind/MyAPI/model"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/gin-gonic/gin"
)

// HoldUnknownUsage prevents this session from turning an estimate into a
// terminal charge/refund. The durable writer freezes its existing lifecycle.
// Legacy sessions retain their pre-dispatch reservation journal. Failure to
// write the hold never falls back to a guessed charge or refund.
func (s *BillingSession) HoldUnknownUsage(parent context.Context, reason string, metadata ...string) error {
	defer s.finishInflight()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.settled || s.refunded || s.refundRecoveryScheduled || s.settlementInput != nil {
		return model.ErrAccountQuotaMutationTerminal
	}
	s.usageUnknown = true
	s.settlementPending = true
	if s.reserveReceipt == nil {
		if !s.legacyUsageJournal {
			return model.ErrAccountQuotaUsageUnresolved
		}
		ctx, cancel := billingOperationContext(parent, 5*time.Second)
		defer cancel()
		return model.UpdateLegacyUsageReservation(ctx, model.DB, s.relayInfo.RequestId,
			int64(s.preConsumedQuota), int64(s.tokenConsumed), model.LegacyUsageUnknown, reason, nil, metadata...)
	}
	_, err := model.HoldAccountQuotaUnknownUsage(parent, model.DB, s.reserveReceipt.RequestID, s.reserveReceipt.ID, reason, metadata...)
	return err
}

func textUsageReviewReason(ctx *gin.Context, usage *dto.Usage) string {
	if usage == nil {
		return "missing"
	}
	if usage.BillingUsage != nil && usage.BillingUsage.Estimated {
		return "estimated"
	}
	if usage.BillingUsage != nil && usage.BillingUsage.Incomplete {
		return "partial"
	}
	effective, hasSource := usageFromBillingUsage(usage)
	if usage.BillingUsage != nil && usage.BillingUsage.OpenAIUsage != nil {
		if details := usage.BillingUsage.OpenAIUsage.InputTokensDetails; details != nil {
			for _, value := range []int{details.CachedTokens, details.CachedCreationTokens, details.CacheWriteTokens, details.TextTokens, details.AudioTokens, details.ImageTokens} {
				if value < 0 || int64(value) > int64(common.MaxQuota) {
					return "invalid"
				}
			}
		}
	}
	if !hasSource {
		if common.GetContextKeyBool(ctx, constant.ContextKeyLocalCountTokens) {
			return "estimated"
		}
		if usage.BillingUsage != nil {
			return "invalid"
		}
		effective = usage
		if !dto.HasOpenAIUsageTokens(usage) {
			return "missing"
		}
	}
	for _, value := range []int{effective.PromptTokens, effective.CompletionTokens, effective.TotalTokens, effective.InputTokens, effective.OutputTokens} {
		if value < 0 || int64(value) > int64(common.MaxQuota) {
			return "invalid"
		}
	}
	for _, value := range []int{effective.PromptTokensDetails.CachedTokens, effective.PromptTokensDetails.CachedCreationTokens,
		effective.PromptTokensDetails.CacheWriteTokens, effective.PromptTokensDetails.TextTokens,
		effective.PromptTokensDetails.AudioTokens, effective.PromptTokensDetails.ImageTokens,
		effective.CompletionTokenDetails.ReasoningTokens, effective.CompletionTokenDetails.TextTokens,
		effective.CompletionTokenDetails.AudioTokens, effective.CompletionTokenDetails.ImageTokens} {
		if value < 0 || int64(value) > int64(common.MaxQuota) {
			return "invalid"
		}
	}
	if effective.CompletionTokenDetails.ReasoningTokens > effective.CompletionTokens {
		return "invalid"
	}
	if hasSource && effective.UsageSemantic == dto.BillingUsageSemanticOpenAI {
		input, output := int64(effective.PromptTokens), int64(effective.CompletionTokens)
		if input+output > int64(common.MaxQuota) || (effective.TotalTokens > 0 && int64(effective.TotalTokens) != input+output) {
			return "invalid"
		}
		cached := int64(effective.PromptTokensDetails.CachedTokens)
		written := int64(effective.PromptTokensDetails.CacheCreationTokensTotal())
		if cached+written > input {
			return "invalid"
		}
	}
	return ""
}

func recordPendingBillingSettlement(ctx *gin.Context, info *relaycommon.RelayInfo) {
	model.RecordErrorLog(ctx, info.UserId, info.ChannelId, info.OriginModelName, ctx.GetString("token_name"),
		"usage_settlement_pending", info.TokenId, int(time.Now().Unix()-info.StartTime.Unix()), info.IsStream, info.UsingGroup,
		map[string]interface{}{"settlement_status": "pending_review", "actual_quota": nil, "reserved_quota": info.FinalPreConsumedQuota})
}

func holdUnverifiedTextUsage(ctx *gin.Context, info *relaycommon.RelayInfo, reason string, summary textQuotaSummary) {
	// Preserve independently known tool obligations and frozen pricing inputs;
	// never serialize request headers, prompts, API keys or raw model output.
	var knownRealtimeQuota *int
	if state := info.RealtimeTieredPricing; state != nil && state.Responses > 0 {
		amount := state.Quota
		knownRealtimeQuota = &amount
	}
	metadata, metadataErr := common.Marshal(struct {
		Version            int                 `json:"version"`
		Model              string              `json:"model"`
		QuotaUnit          float64             `json:"quota_unit"`
		ModelRatio         float64             `json:"model_ratio"`
		CompletionRatio    float64             `json:"completion_ratio"`
		GroupRatio         float64             `json:"group_ratio"`
		KnownTools         []ToolSurchargeItem `json:"known_tool_obligations,omitempty"`
		UnitCaptured       bool                `json:"quota_unit_captured"`
		KnownRealtimeQuota *int                `json:"known_realtime_quota,omitempty"`
	}{1, info.OriginModelName, requestQuotaUnit(info.PriceData), summary.ModelRatio, summary.CompletionRatio, summary.GroupRatio, summary.ToolSurchargeItems, info.PriceData.QuotedQuotaUnit(0) > 0, knownRealtimeQuota})
	if len(metadata) > 16384 {
		metadataErr = fmt.Errorf("usage review pricing metadata exceeds limit")
	}
	var holdErr error
	if session, ok := info.Billing.(*BillingSession); ok {
		parent := context.Background()
		if ctx != nil && ctx.Request != nil {
			parent = ctx.Request.Context()
		}
		if metadataErr == nil {
			holdErr = session.HoldUnknownUsage(parent, reason, string(metadata))
		} else {
			holdErr = session.HoldUnknownUsage(parent, reason, `{"incomplete_pricing_evidence":true}`)
		}
	} else {
		holdErr = model.ErrAccountQuotaUsageUnresolved
	}
	if metadataErr != nil || holdErr != nil {
		logger.LogError(ctx, fmt.Sprintf("usage reconciliation persistence requires attention: %v; %v", holdErr, metadataErr))
	}
	other := map[string]interface{}{"usage_accuracy": reason, "settlement_status": "pending_review", "actual_quota": nil,
		"reserved_quota": info.FinalPreConsumedQuota, "known_tool_obligations": summary.ToolSurchargeItems,
		"review_persisted": holdErr == nil}
	if reason != "estimated" {
		other["usage_accuracy"] = "unknown"
	}
	// Error/review records are excluded from confirmed consume aggregates. The
	// explicit NULL actual_quota is not a zero-priced consumption record.
	model.RecordErrorLog(ctx, info.UserId, info.ChannelId, info.OriginModelName, ctx.GetString("token_name"),
		"usage_pending_review", info.TokenId, int(time.Now().Unix()-info.StartTime.Unix()), info.IsStream, info.UsingGroup, other)
}

// Caller holds the session mutex. Journal completion failure does not undo an
// already committed settlement or mark it safe for a refund.
func (s *BillingSession) completeLegacyUsageJournal(parent context.Context, state string, actual *int64) error {
	ctx, cancel := billingOperationContext(parent, 5*time.Second)
	defer cancel()
	return model.UpdateLegacyUsageReservation(ctx, model.DB, s.relayInfo.RequestId,
		int64(s.preConsumedQuota), int64(s.tokenConsumed), state, "", actual)
}
