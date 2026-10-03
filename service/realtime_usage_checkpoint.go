package service

import (
	"fmt"
	"math"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	hosttypes "github.com/ForceMind/MyAPI/types"
	"github.com/gin-gonic/gin"
)

// Called only after a complete reported response has passed existing pricing
// validation, and before that response is forwarded to the client. Estimates
// never enter this cumulative checkpoint or its manual-recovery lower bound.
func RecordRealtimeUsageCheckpoint(c *gin.Context, info *relaycommon.RelayInfo, usage *dto.RealtimeUsage) error {
	session := realtimeUsageDispatchSession(c, info)
	if session == nil {
		return nil
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	if !session.realtimeDispatchTracked || !session.textDispatchPossible || session.usageUnknown || session.settled || session.refunded || session.settlementPending || session.refundRecoveryScheduled || session.settlementInput != nil {
		return model.ErrAccountQuotaUsageUnresolved
	}
	if usage == nil || !usage.RawUsageObserved || usage.UsageIncomplete || info.PriceData.QuotedQuotaUnit(0) <= 0 {
		return model.ErrAccountQuotaUsageUnresolved
	}
	next := hosttypes.RealtimeUsageCheckpoint{}
	if info.RealtimeCheckpoint != nil {
		next = *info.RealtimeCheckpoint
	}
	for _, value := range []struct {
		target *int
		delta  int
	}{
		{&next.Responses, 1}, {&next.Input, usage.InputTokens}, {&next.Output, usage.OutputTokens}, {&next.Total, usage.TotalTokens},
		{&next.InputText, usage.InputTokenDetails.TextTokens}, {&next.InputAudio, usage.InputTokenDetails.AudioTokens}, {&next.InputImage, usage.InputTokenDetails.ImageTokens},
		{&next.OutputText, usage.OutputTokenDetails.TextTokens}, {&next.OutputAudio, usage.OutputTokenDetails.AudioTokens}, {&next.OutputImage, usage.OutputTokenDetails.ImageTokens},
	} {
		if *value.target < 0 || *value.target > common.MaxQuota || value.delta < 0 || value.delta > common.MaxQuota-*value.target {
			return fmt.Errorf("realtime checkpoint exceeds supported counts")
		}
		*value.target += value.delta
	}
	if snap := info.TieredBillingSnapshot; snap != nil && snap.BillingMode == "tiered_expr" {
		pricing := info.RealtimeTieredPricing
		if pricing == nil || pricing.Incomplete || pricing.Responses != next.Responses || pricing.InputTokens != next.Input || pricing.OutputTokens != next.Output || pricing.TotalTokens != next.Total {
			return model.ErrAccountQuotaUsageUnresolved
		}
		next.Quota = pricing.Quota // Already computed independently per response.
	} else {
		if next.InputImage != 0 || next.OutputImage != 0 {
			// The legacy audio quote has no image rate. Do not manufacture one.
			return model.ErrAccountQuotaUsageUnresolved
		}
		completion, audio, audioCompletion := requestAudioRatios(info.PriceData, info.OriginModelName)
		for _, ratio := range []float64{info.PriceData.ModelRatio, info.PriceData.GroupRatioInfo.GroupRatio, completion, audio, audioCompletion} {
			if ratio < 0 || math.IsNaN(ratio) || math.IsInf(ratio, 0) {
				return model.ErrAccountQuotaUsageUnresolved
			}
		}
		quota, clamp := calculateAudioQuota(QuotaInfo{
			InputDetails: TokenDetails{TextTokens: next.InputText, AudioTokens: next.InputAudio}, OutputDetails: TokenDetails{TextTokens: next.OutputText, AudioTokens: next.OutputAudio},
			ModelName: info.OriginModelName, ModelRatio: info.PriceData.ModelRatio, GroupRatio: info.PriceData.GroupRatioInfo.GroupRatio, priceData: info.PriceData,
		})
		noteQuotaClamp(info, clamp)
		if clamp != nil {
			return clamp
		}
		next.Quota = quota
	}
	if next.Total == 0 {
		next.Quota = 0 // Preserve the existing explicit-zero settlement contract.
	}
	if info.RealtimeCheckpoint != nil && next.Quota < info.RealtimeCheckpoint.Quota {
		return model.ErrAccountQuotaMutationConflict
	}
	// Retain this known evidence in memory even if persistence is uncertain;
	// ordinary hold retry can then preserve it without guessing or refunding.
	info.RealtimeCheckpoint = &next
	reserveID := int64(0)
	if session.reserveReceipt != nil {
		reserveID = session.reserveReceipt.ID
	}
	ctx, cancel := billingOperationContext(c.Request.Context(), 5*time.Second)
	defer cancel()
	return model.RecordRealtimeUsageCheckpoint(ctx, model.DB, info.RequestId, info.UserId, info.TokenId, reserveID, next)
}
