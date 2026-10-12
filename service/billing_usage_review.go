package service

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/logger"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/pkg/billingexpr"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	hosttypes "github.com/ForceMind/MyAPI/types"
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
		err := model.UpdateLegacyUsageReservation(ctx, model.DB, s.relayInfo.RequestId,
			int64(s.preConsumedQuota), int64(s.tokenConsumed), model.LegacyUsageUnknown, reason, nil, metadata...)
		s.usageHoldPersisted = err == nil
		return err
	}
	_, err := model.HoldAccountQuotaUnknownUsage(parent, model.DB, s.reserveReceipt.RequestID, s.reserveReceipt.ID, reason, metadata...)
	s.usageHoldPersisted = err == nil
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
	if usage.BillingUsage != nil && usage.BillingUsage.ClaudeUsage != nil {
		raw := usage.BillingUsage.ClaudeUsage
		// Validate native TTL aliases before normalization can fold or clamp
		// them. Out-of-bound evidence is unresolved, never a certain charge.
		for _, value := range []int{raw.InputTokens, raw.OutputTokens, raw.CacheReadInputTokens, raw.CacheCreationInputTokens, raw.ClaudeCacheCreation5mTokens, raw.ClaudeCacheCreation1hTokens, raw.GetCacheCreation5mTokens(), raw.GetCacheCreation1hTokens()} {
			if value < 0 || int64(value) > int64(common.MaxQuota) {
				return "invalid"
			}
		}
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

const (
	usageReviewRequestOutcomeCompleted = "completed"
	usageReviewUsageOutcomeReported    = "reported"
	usageReviewUsageOutcomeUnknown     = "unknown"
)

type usageReviewDisposition struct {
	RequestOutcome string
	UsageOutcome   string
}

// textUsageReviewDisposition records request completion independently from
// billing evidence. Missing, partial, invalid, or locally estimated usage can
// follow a completed response, but none of them prove delivery by themselves.
func textUsageReviewDisposition(ctx *gin.Context, info *relaycommon.RelayInfo, usage *dto.Usage) usageReviewDisposition {
	reason := textUsageReviewReason(ctx, usage)
	disposition := usageReviewDisposition{UsageOutcome: usageReviewUsageOutcomeReported}
	if reason != "" {
		disposition.UsageOutcome = reason
	}
	if ctx == nil || info == nil || ctx.Writer == nil {
		return disposition
	}
	if info.IsStream {
		if !ctx.GetBool("relay_response_completed") || info.StreamStatus == nil ||
			info.StreamStatus.EndReason != relaycommon.StreamEndReasonDone ||
			info.StreamStatus.EndError != nil || info.StreamStatus.HasErrors() {
			return disposition
		}
	} else {
		if !ctx.GetBool("relay_response_completed") || ctx.Request == nil || ctx.Request.Context().Err() != nil ||
			ctx.Writer.Status() < 200 || ctx.Writer.Status() >= 300 {
			return disposition
		}
		expected, err := strconv.Atoi(ctx.Writer.Header().Get("Content-Length"))
		// PostTextConsumeQuota is reached only after a successful adapter
		// response. Require its non-empty IOCopyBytesGracefully boundary here;
		// a default 200 writer with no response is not completion evidence.
		if err != nil || expected <= 0 || ctx.Writer.Size() != expected {
			return disposition
		}
	}
	disposition.RequestOutcome = usageReviewRequestOutcomeCompleted
	return disposition
}

func recordPendingBillingSettlement(ctx *gin.Context, info *relaycommon.RelayInfo) {
	recordPendingBillingSettlementWithDisposition(ctx, info, usageReviewDisposition{})
}

func recordPendingBillingSettlementWithDisposition(ctx *gin.Context, info *relaycommon.RelayInfo, disposition usageReviewDisposition) {
	other := map[string]interface{}{
		"settlement_status": "pending_review",
		"actual_quota":      nil,
		"reserved_quota":    info.FinalPreConsumedQuota,
	}
	appendUsageReviewDisposition(other, disposition)
	model.RecordErrorLog(ctx, info.UserId, info.ChannelId, info.OriginModelName, ctx.GetString("token_name"),
		"usage_settlement_pending", info.TokenId, int(time.Now().Unix()-info.StartTime.Unix()), info.IsStream, info.UsingGroup,
		other)
}

func usageReviewPricingEvidence(info *relaycommon.RelayInfo, summary textQuotaSummary) ([]byte, error) {
	return usageReviewPricingEvidenceWithDisposition(info, summary, usageReviewDisposition{})
}

func usageReviewPricingEvidenceWithDisposition(info *relaycommon.RelayInfo, summary textQuotaSummary, disposition usageReviewDisposition) ([]byte, error) {
	// Preserve independently known tool obligations and frozen pricing inputs;
	// never serialize request headers, prompts, API keys or raw model output.
	var knownRealtimeQuota *int
	if state := info.RealtimeTieredPricing; state != nil && state.Responses > 0 {
		amount := state.Quota
		knownRealtimeQuota = &amount
	}
	if checkpoint := info.RealtimeCheckpoint; checkpoint != nil {
		amount := checkpoint.Quota
		knownRealtimeQuota = &amount
	}
	var frozenTieredPricing *billingexpr.BillingSnapshot
	if snapshot := info.TieredBillingSnapshot; snapshot != nil && snapshot.OfficialPricePublicationID != "" {
		frozenTieredPricing = snapshot
	}
	metadata, metadataErr := common.Marshal(struct {
		FixedPrice          bool                               `json:"fixed_price,omitempty"`
		ModelPrice          float64                            `json:"model_price,omitempty"`
		PriceRatios         map[string]float64                 `json:"price_ratios,omitempty"`
		UpstreamModel       string                             `json:"upstream_model,omitempty"`
		Version             int                                `json:"version"`
		Model               string                             `json:"model"`
		QuotaUnit           float64                            `json:"quota_unit"`
		ModelRatio          float64                            `json:"model_ratio"`
		CompletionRatio     float64                            `json:"completion_ratio"`
		GroupRatio          float64                            `json:"group_ratio"`
		KnownTools          []ToolSurchargeItem                `json:"known_tool_obligations,omitempty"`
		UnitCaptured        bool                               `json:"quota_unit_captured"`
		KnownRealtimeQuota  *int                               `json:"known_realtime_quota,omitempty"`
		FrozenTieredPricing *billingexpr.BillingSnapshot       `json:"frozen_tiered_pricing,omitempty"`
		StrictTokenBudget   bool                               `json:"strict_token_budget,omitempty"`
		RealtimeCheckpoint  *hosttypes.RealtimeUsageCheckpoint `json:"realtime_usage_checkpoint,omitempty"`
		RequestOutcome      string                             `json:"request_outcome,omitempty"`
		UsageOutcome        string                             `json:"usage_outcome,omitempty"`
	}{
		FixedPrice:          info.PriceData.UsePrice,
		ModelPrice:          info.PriceData.ModelPrice,
		PriceRatios:         info.PriceData.OtherRatios(),
		UpstreamModel:       info.GetUpstreamModelName(),
		Version:             1,
		Model:               info.OriginModelName,
		QuotaUnit:           requestQuotaUnit(info.PriceData),
		ModelRatio:          summary.ModelRatio,
		CompletionRatio:     summary.CompletionRatio,
		GroupRatio:          summary.GroupRatio,
		KnownTools:          summary.ToolSurchargeItems,
		UnitCaptured:        info.PriceData.QuotedQuotaUnit(0) > 0,
		KnownRealtimeQuota:  knownRealtimeQuota,
		FrozenTieredPricing: frozenTieredPricing,
		StrictTokenBudget:   info.StrictTokenBudget,
		RealtimeCheckpoint:  info.RealtimeCheckpoint,
		RequestOutcome:      disposition.RequestOutcome,
		UsageOutcome:        disposition.UsageOutcome,
	})
	if len(metadata) > 16384 {
		metadataErr = fmt.Errorf("usage review pricing metadata exceeds limit")
	}
	return metadata, metadataErr
}

func holdUnverifiedTextUsage(ctx *gin.Context, info *relaycommon.RelayInfo, reason string, summary textQuotaSummary) {
	holdUnverifiedTextUsageWithDisposition(ctx, info, reason, summary, usageReviewDisposition{UsageOutcome: usageReviewUsageOutcomeUnknown})
}

func holdUnverifiedTextUsageWithDisposition(ctx *gin.Context, info *relaycommon.RelayInfo, reason string, summary textQuotaSummary, disposition usageReviewDisposition) {
	if ctx != nil {
		ctx.Set("relay_completion_unverified", true)
	}
	if info.StrictTokenBudget {
		if err := HoldTokenBudgetUsage(ctx.Request.Context(), info, reason); err != nil {
			logger.LogError(ctx, "token budget remains unresolved: "+err.Error())
		}
	}
	if disposition.UsageOutcome == "" {
		disposition.UsageOutcome = usageReviewUsageOutcomeUnknown
	}
	metadata, metadataErr := usageReviewPricingEvidenceWithDisposition(info, summary, disposition)
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
	appendUsageReviewDisposition(other, disposition)
	if common.SensitiveRequestDiagnostics(ctx) {
		appendRequestPath(ctx, info, other)
	}
	if reason != "estimated" {
		other["usage_accuracy"] = "unknown"
	}
	InjectTieredBillingInfo(other, info, nil)
	if info.TokenBudgetAudit != nil {
		other["admin_info"] = map[string]interface{}{"token_budget": info.TokenBudgetAudit}
	}
	// Error/review records are excluded from confirmed consume aggregates. The
	// explicit NULL actual_quota is not a zero-priced consumption record.
	model.RecordErrorLog(ctx, info.UserId, info.ChannelId, info.OriginModelName, ctx.GetString("token_name"),
		"usage_pending_review", info.TokenId, int(time.Now().Unix()-info.StartTime.Unix()), info.IsStream, info.UsingGroup, other)
}

func appendUsageReviewDisposition(other map[string]interface{}, disposition usageReviewDisposition) {
	if other == nil {
		return
	}
	if disposition.RequestOutcome != "" {
		other["request_outcome"] = disposition.RequestOutcome
	}
	if disposition.UsageOutcome != "" {
		other["usage_outcome"] = disposition.UsageOutcome
	}
}

// Caller holds the session mutex. Journal completion failure does not undo an
// already committed settlement or mark it safe for a refund.
func (s *BillingSession) completeLegacyUsageJournal(parent context.Context, state string, actual *int64) error {
	ctx, cancel := billingOperationContext(parent, 5*time.Second)
	defer cancel()
	return model.UpdateLegacyUsageReservation(ctx, model.DB, s.relayInfo.RequestId,
		int64(s.preConsumedQuota), int64(s.tokenConsumed), state, "", actual)
}
