package service

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/model"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	relayconstant "github.com/ForceMind/MyAPI/relay/constant"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func TokenBudgetRelayError(c *gin.Context, err error) *types.NewAPIError {
	status, code, message := http.StatusServiceUnavailable, "token_budget_unavailable", i18n.MsgTokenBudgetUnavailable
	switch {
	case errors.Is(err, ErrTokenBudgetUnsupported):
		status, code, message = http.StatusBadRequest, "token_budget_unsupported_request", i18n.MsgTokenBudgetUnsupported
	case errors.Is(err, model.ErrFeeBudgetExceeded):
		status, code, message = http.StatusForbidden, "token_budget_fee_exceeded", i18n.MsgFeeBudgetExceeded // gitleaks:allow public error-code identifier, not a credential
	case errors.Is(err, ErrFeeBudgetEvidence):
		code, message = "token_budget_fee_evidence", i18n.MsgFeeBudgetEvidence // gitleaks:allow public error-code identifier, not a credential
	case errors.Is(err, model.ErrTokenBudgetExceeded):
		status, code, message = http.StatusForbidden, "token_budget_exceeded", i18n.MsgTokenBudgetExceeded
	case errors.Is(err, model.ErrTokenBudgetPending), errors.Is(err, model.ErrTokenBudgetConflict), errors.Is(err, model.ErrTokenBudgetDuplicate):
		status, code, message = http.StatusConflict, "token_budget_pending", i18n.MsgTokenBudgetPending
	case errors.Is(err, ErrTokenBudgetCountUnavailable):
		code, message = "token_budget_count_unavailable", i18n.MsgTokenBudgetCountUnavailable
	}
	return types.NewErrorWithStatusCode(errors.New(common.TranslateMessage(c, message)), types.ErrorCode(code), status, types.ErrOptionWithSkipRetry())
}

func ValidateTokenBudgetSelectedChannel(info *relaycommon.RelayInfo) error {
	if info == nil || !info.StrictTokenBudget {
		return nil
	}
	// Native Responses and the reviewed Chat text slice require ordinary
	// per-token quota billing. Other billing paths stay unchanged and cannot
	// silently bypass this opt-in policy.
	if info.ChannelMeta == nil || info.ChannelType != constant.ChannelTypeOpenAI || (info.RelayMode != relayconstant.RelayModeResponses && info.RelayMode != relayconstant.RelayModeChatCompletions) ||
		info.Billing == nil || info.PriceData.FreeModel || info.PriceData.UsePrice {
		return ErrTokenBudgetUnsupported
	}
	return nil
}

func PrepareTokenBudgetDispatch(c *gin.Context, client *http.Client, req *http.Request, info *relaycommon.RelayInfo) error {
	if info == nil || !info.StrictTokenBudget {
		return nil
	}
	if c == nil || c.Request == nil || c.Request.URL == nil || req == nil || req.URL == nil || c.Request.URL.EscapedPath() != req.URL.EscapedPath() {
		return ErrTokenBudgetUnsupported
	}
	if err := ValidateTokenBudgetSelectedChannel(info); err != nil {
		return err
	}
	bound, err := CountTokenBudgetBound(c.Request.Context(), client, req, info)
	if err != nil {
		return err
	}
	policy, err := model.LookupTokenBudget(c.Request.Context(), model.DB, info.TokenId)
	if err != nil {
		return err
	}
	if policy == nil {
		return model.ErrTokenBudgetConflict
	}
	if policy.FeeEnabled {
		if err := freezeFeeBudgetPrice(c.Request.Context(), model.DB, info, bound); err != nil {
			return err
		}
	}
	evidence, err := usageReviewPricingEvidence(info, calculateTextQuotaSummary(c, info, nil))
	if err != nil {
		return err
	}
	bound.PricingEvidence = string(evidence)
	ctx, cancel := billingOperationContext(c.Request.Context(), 5*time.Second)
	defer cancel()
	if err := model.ReserveTokenBudget(ctx, model.DB, *bound); err != nil {
		return err
	}
	total := bound.InputTokens + bound.MaxOutputTokens
	if bound.BoundSource == model.TokenBudgetBoundOpenAIChat {
		total = model.TokenBudgetOpenAIChatContext
	}
	info.TokenBudgetAudit = map[string]interface{}{"bound_source": bound.BoundSource, "input_tokens_bound": bound.InputTokens,
		"max_output_tokens": bound.MaxOutputTokens, "reserved": total, "fee_reserved_usd": "0"}
	if bound.FeeEnabled {
		info.TokenBudgetAudit["fee_reserved_usd"] = bound.FeeReservedUSD
	}
	_, err = model.MutateTokenBudgetRequest(ctx, model.DB, model.TokenBudgetMutation{TokenID: info.TokenId, RequestID: info.RequestId, Action: "send"})
	return err
}

func HoldTokenBudgetUsage(ctx context.Context, info *relaycommon.RelayInfo, reason string) error {
	if info == nil || !info.StrictTokenBudget {
		return nil
	}
	operation, cancel := billingOperationContext(ctx, 5*time.Second)
	defer cancel()
	_, err := model.MutateTokenBudgetRequest(operation, model.DB, model.TokenBudgetMutation{TokenID: info.TokenId, RequestID: info.RequestId, Action: "hold", Reason: reason})
	return err
}

func SettleTokenBudgetUsage(ctx *gin.Context, info *relaycommon.RelayInfo, usage *dto.Usage) error {
	if info == nil || !info.StrictTokenBudget {
		return nil
	}
	if reason := textUsageReviewReason(ctx, usage); reason != "" {
		return model.ErrTokenBudgetPending
	}
	if usage == nil || usage.BillingUsage == nil || usage.BillingUsage.OpenAIUsage == nil {
		return model.ErrTokenBudgetPending
	}
	operation, cancel := billingOperationContext(ctx.Request.Context(), 5*time.Second)
	defer cancel()
	var row model.TokenBudgetReservation
	if err := model.DB.WithContext(operation).First(&row, "request_id = ?", info.RequestId).Error; err != nil {
		return err
	}
	if row.RequestID != info.RequestId || row.TokenID != info.TokenId || row.UserID != info.UserId {
		return model.ErrTokenBudgetConflict
	}
	actual := usage.BillingUsage.OpenAIUsage
	input, output := int64(actual.InputTokens), int64(actual.OutputTokens)
	switch row.BoundSource {
	case model.TokenBudgetBoundOpenAIResponses:
		if usage.BillingUsage.Source != dto.BillingUsageSourceOAIResponses {
			return model.ErrTokenBudgetPending
		}
	case model.TokenBudgetBoundOpenAIChat:
		evidence, err := qualifiedChatBudgetUsage(&row, usage)
		if err != nil {
			return err
		}
		if evidence.Stream != info.IsStream || info.IsStream &&
			(info.StreamStatus == nil || info.StreamStatus.EndReason != relaycommon.StreamEndReasonDone || info.StreamStatus.EndError != nil || info.StreamStatus.HasErrors()) {
			return model.ErrTokenBudgetPending
		}
		// The adapter records early cancellation and downstream write failures.
		// A client may cancel after receiving the complete terminal event; that
		// cannot invalidate already-qualified upstream usage. Persist through
		// the bounded, cancellation-detached operation context created above.
		if info.TieredBillingSnapshot != nil && info.TieredBillingSnapshot.OfficialPricePublicationID != "" && (evidence.CacheRead == nil || evidence.CacheWrite == nil || evidence.ServiceTier != "default") {
			return model.ErrTokenBudgetPending
		}
		input, output = int64(evidence.PromptTokens), int64(evidence.CompletionTokens)
	default:
		return model.ErrTokenBudgetConflict
	}
	var feeUSD *string
	if row.FeeEnabled {
		amount, err := calculateFeeBudgetUSD(&row, usage)
		if err != nil {
			return err
		}
		feeUSD = &amount
	}
	_, err := model.MutateTokenBudgetRequest(operation, model.DB, model.TokenBudgetMutation{TokenID: info.TokenId, RequestID: info.RequestId, Action: "settle", Input: input, Output: output, FeeUSD: feeUSD})
	if err == nil {
		if info.TokenBudgetAudit == nil {
			info.TokenBudgetAudit = map[string]interface{}{}
		}
		info.TokenBudgetAudit["actual_input"], info.TokenBudgetAudit["actual_output"] = input, output
		if feeUSD != nil {
			info.TokenBudgetAudit["actual_fee_usd"] = *feeUSD
		}
	}
	if err == nil && feeUSD != nil {
		var price feeBudgetPriceEvidence
		if err := common.UnmarshalJsonStr(row.FeePriceEvidence, &price); err != nil {
			return err
		}
		info.ConfirmedAPIUsageCost = map[string]string{"amount_usd": *feeUSD, "currency": "USD", "scope": feeBudgetPriceScope, "publication_id": price.PublicationID, "source_sha256": price.SourceSHA256}
	}
	return err
}

// Every exit from the strict relay checks the durable pre-send record. Only a
// proven pre-dispatch cancellation permits the old quota refund path. Storage
// errors are conservative: never reinterpret an uncertain dispatch as unused.
func FinalizeTokenBudgetDispatch(c *gin.Context, info *relaycommon.RelayInfo) bool {
	if info == nil || !info.StrictTokenBudget {
		return false
	}
	ctx, cancel := billingOperationContext(c.Request.Context(), 5*time.Second)
	defer cancel()
	var row model.TokenBudgetReservation
	err := model.DB.WithContext(ctx).First(&row, "request_id = ?", info.RequestId).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false
	}
	if err == nil && row.RequestID == info.RequestId && row.TokenID == info.TokenId && row.UserID == info.UserId {
		switch row.State {
		case model.TokenBudgetSettled, model.TokenBudgetCancelled:
			return false
		case model.TokenBudgetPrepared:
			_, err = model.MutateTokenBudgetRequest(ctx, model.DB, model.TokenBudgetMutation{TokenID: info.TokenId, RequestID: info.RequestId, Action: "cancel"})
			if err == nil {
				return false
			}
		case model.TokenBudgetUnknown:
			return true
		}
	}
	holdUnverifiedTextUsage(c, info, "missing", calculateTextQuotaSummary(c, info, nil))
	return true
}
