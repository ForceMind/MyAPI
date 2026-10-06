package service

import (
	"net/http"
	"time"

	"github.com/ForceMind/MyAPI/model"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/gin-gonic/gin"
)

func realtimeUsageDispatchSession(c *gin.Context, info *relaycommon.RelayInfo) *BillingSession {
	if c == nil || c.Request == nil || c.Request.URL == nil || c.Request.Method != http.MethodGet || c.Request.URL.Path != "/v1/realtime" ||
		info == nil || info.RelayFormat != types.RelayFormatOpenAIRealtime || info.StrictTokenBudget || info.PriceData.UsePrice || info.IsChannelTest {
		return nil
	}
	session, _ := info.Billing.(*BillingSession)
	return session
}

// The caller has an accepted upstream WebSocket, but has not started frame
// forwarding. Persist first, including when cancellation races the upgrade.
// No timeout or crash may subsequently be mistaken for proof of zero usage.
func PrepareRealtimeUsageDispatch(c *gin.Context, info *relaycommon.RelayInfo) error {
	session := realtimeUsageDispatchSession(c, info)
	if session == nil {
		return nil
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
	session.textDispatchTracked, session.textDispatchPossible, session.realtimeDispatchTracked = true, true, true
	reserveID := int64(0)
	if session.reserveReceipt != nil {
		reserveID = session.reserveReceipt.ID
	}
	ctx, cancel := billingOperationContext(c.Request.Context(), 5*time.Second)
	defer cancel()
	return model.SetRealtimeDispatchEvidence(ctx, model.DB, info.RequestId, info.UserId, info.TokenId, reserveID, string(metadata))
}

func FinalizeRealtimeUsageDispatch(c *gin.Context, info *relaycommon.RelayInfo) bool {
	return finalizeUsageDispatch(c, info, realtimeUsageDispatchSession(c, info))
}
