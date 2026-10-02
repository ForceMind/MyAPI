package service

import (
	"net/http"

	"github.com/ForceMind/MyAPI/model"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/gin-gonic/gin"
)

func textUsageDispatchSession(c *gin.Context, info *relaycommon.RelayInfo) *BillingSession {
	if c == nil || c.Request == nil || c.Request.URL == nil || c.Request.Method != http.MethodPost || info == nil || info.StrictTokenBudget || info.PriceData.UsePrice || info.IsChannelTest {
		return nil
	}
	// Query parameters cannot bypass a guard for the same billable route.
	switch c.Request.URL.Path {
	case "/v1/chat/completions", "/v1/responses", "/v1/responses/compact":
	default:
		return nil
	}
	session, _ := info.Billing.(*BillingSession)
	return session
}

// This live-request guard supplements, never replaces, the durable strict
// Token/USD dispatch journal. A process crash still needs separate evidence;
// there is no timer-based release or inferred zero-use settlement here.
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
	session.textDispatchTracked, session.textDispatchPossible = true, true
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
	if session.textDispatchTracked && !session.usageUnknown {
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
