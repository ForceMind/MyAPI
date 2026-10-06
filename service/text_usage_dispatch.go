package service

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/ForceMind/MyAPI/model"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/gin-gonic/gin"
)

func textUsageDispatchSession(c *gin.Context, info *relaycommon.RelayInfo) *BillingSession {
	if c == nil || c.Request == nil || c.Request.URL == nil || c.Request.Method != http.MethodPost || info == nil || info.StrictTokenBudget || info.IsChannelTest {
		return nil
	}
	if info.PriceData.UsePrice && RelayFailoverFromContext(c.Request.Context()) == nil {
		return nil
	}
	// Query parameters cannot bypass a guard for the same billable route.
	switch c.Request.URL.Path {
	case "/v1/chat/completions", "/v1/completions", "/v1/messages", "/v1/responses", "/v1/responses/compact":
	default:
		// Existing Gemini wildcard routes select generation from the validated
		// request, and the adaptor constructs the upstream action itself. Checking
		// just a URL action suffix would leave alternate accepted spellings unguarded.
		generation, ok := info.Request.(*dto.GeminiChatRequest)
		if !ok || generation == nil || (!strings.HasPrefix(c.Request.URL.Path, "/v1beta/models/") && !strings.HasPrefix(c.Request.URL.Path, "/v1/models/")) {
			return nil
		}
	}
	session, _ := info.Billing.(*BillingSession)
	return session
}

// Ordinary text dispatch preserves evidence on existing writer rows before
// sending. The separate strict Token/USD journal is unchanged. Neither path
// treats a timeout or process crash as evidence of zero consumption.
func PrepareTextUsageDispatch(c *gin.Context, request *http.Request, info *relaycommon.RelayInfo) error {
	session := textUsageDispatchSession(c, info)
	if session == nil {
		return nil
	}
	if request == nil {
		return model.ErrAccountQuotaMutationInvalidInput
	}
	if err := c.Request.Context().Err(); err != nil {
		return err
	}
	if err := request.Context().Err(); err != nil {
		return err
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.settled || session.refunded || session.usageUnknown || session.settlementPending || session.refundRecoveryScheduled || session.settlementInput != nil || session.fundingSettled || session.textDispatchPossible {
		return model.ErrAccountQuotaUsageUnresolved
	}
	metadata, err := usageReviewPricingEvidence(info, calculateTextQuotaSummary(c, info, nil))
	if err != nil {
		return err
	}
	session.textDispatchTracked, session.textDispatchPossible = true, true
	reserveID := int64(0)
	if session.reserveReceipt != nil {
		reserveID = session.reserveReceipt.ID
	}
	ctx, cancel := billingOperationContext(c.Request.Context(), 5*time.Second)
	defer cancel()
	if err := model.SetTextDispatchEvidence(ctx, model.DB, info.RequestId, info.UserId, info.TokenId, reserveID, string(metadata), true); err != nil {
		return err // Outcome may be uncertain; finalization retains and reviews it.
	}
	request.GetBody = nil // Do not transparently replay an ambiguous paid POST.
	return nil
}

// Preserve existing explicit request/auth/validation/rate-limit refusals.
// Timeouts, conflicts, redirects, server failures and accepted responses are
// not evidence of zero consumption. Use the raw transport status, not a mapped
// client status. Strict budgets deliberately do not use these exceptions.
func ObserveTextUsageDispatchResponse(info *relaycommon.RelayInfo, status int) {
	if info == nil || info.StrictTokenBudget {
		return
	}
	session, ok := info.Billing.(*BillingSession)
	if !ok {
		return
	}
	switch status {
	case 400, 401, 403, 404, 405, 413, 414, 415, 422, 429:
	default:
		return
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.realtimeDispatchTracked {
		return
	}
	if session.textDispatchTracked && !session.usageUnknown {
		reserveID := int64(0)
		if session.reserveReceipt != nil {
			reserveID = session.reserveReceipt.ID
		}
		ctx, cancel := billingOperationContext(context.Background(), 5*time.Second)
		defer cancel()
		if err := model.SetTextDispatchEvidence(ctx, model.DB, info.RequestId, info.UserId, info.TokenId, reserveID, "", false); err != nil {
			return
		}
		session.textDispatchPossible = false
	}
}

func TextUsageDispatchNeedsReview(info *relaycommon.RelayInfo) bool {
	if info == nil || info.StrictTokenBudget {
		return false
	}
	session, ok := info.Billing.(*BillingSession)
	if !ok {
		return false
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	return session.textDispatchTracked && !session.settled && !session.refunded && (session.textDispatchPossible || session.usageUnknown)
}

// Call before retry decisions and on the outer relay exit. Persist uncertainty
// through the existing writer and review UI, rather than refunding a parse or
// transport failure. A failed hold is retried on the next exit check while the
// in-memory reservation remains protected from settlement and refund.
func FinalizeTextUsageDispatch(c *gin.Context, info *relaycommon.RelayInfo) bool {
	session := textUsageDispatchSession(c, info)
	return finalizeUsageDispatch(c, info, session)
}

func finalizeUsageDispatch(c *gin.Context, info *relaycommon.RelayInfo, session *BillingSession) bool {
	if session == nil {
		return false
	}
	session.mu.Lock()
	if !session.textDispatchTracked || session.settled || session.refunded || !session.textDispatchPossible && !session.usageUnknown {
		session.mu.Unlock()
		return false
	}
	if session.textDispatchFinalizing || session.usageUnknown && session.usageHoldPersisted || !session.usageUnknown && (session.settlementInput != nil || session.settlementPending || session.fundingSettled) {
		session.mu.Unlock()
		return true
	}
	session.textDispatchFinalizing = true
	session.usageUnknown, session.settlementPending = true, true
	session.mu.Unlock()
	defer func() { session.mu.Lock(); session.textDispatchFinalizing = false; session.mu.Unlock() }()
	holdUnverifiedTextUsage(c, info, "missing", calculateTextQuotaSummary(c, info, nil))
	return true
}
