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
	// The first strict slice is native, stateless Responses with ordinary
	// per-token quota billing. Other billing paths stay unchanged and cannot
	// silently bypass this opt-in policy.
	if info.ChannelMeta == nil || info.ChannelType != constant.ChannelTypeOpenAI || info.RelayMode != relayconstant.RelayModeResponses ||
		info.Billing == nil || info.PriceData.FreeModel || info.PriceData.UsePrice {
		return ErrTokenBudgetUnsupported
	}
	return nil
}

func PrepareTokenBudgetDispatch(c *gin.Context, client *http.Client, req *http.Request, info *relaycommon.RelayInfo) error {
	if info == nil || !info.StrictTokenBudget {
		return nil
	}
	if c == nil || c.Request == nil {
		return ErrTokenBudgetUnsupported
	}
	if err := ValidateTokenBudgetSelectedChannel(info); err != nil {
		return err
	}
	bound, err := CountTokenBudgetBound(c.Request.Context(), client, req, info)
	if err != nil {
		return err
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
	if usage == nil || usage.BillingUsage == nil || usage.BillingUsage.Source != dto.BillingUsageSourceOAIResponses || usage.BillingUsage.OpenAIUsage == nil {
		return model.ErrTokenBudgetPending
	}
	actual := usage.BillingUsage.OpenAIUsage
	operation, cancel := billingOperationContext(ctx.Request.Context(), 5*time.Second)
	defer cancel()
	_, err := model.MutateTokenBudgetRequest(operation, model.DB, model.TokenBudgetMutation{TokenID: info.TokenId, RequestID: info.RequestId, Action: "settle", Input: int64(actual.InputTokens), Output: int64(actual.OutputTokens)})
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
