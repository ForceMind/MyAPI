package service

import (
	"fmt"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/pkg/billingexpr"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/shopspring/decimal"
)

// RecordRealtimeTieredResponse runs only for provider-reported response.done
// usage. Local estimates never establish a confirmed per-response quote.
func RecordRealtimeTieredResponse(info *relaycommon.RelayInfo, usage *dto.RealtimeUsage) error {
	snap := info.TieredBillingSnapshot
	if snap == nil || snap.BillingMode != "tiered_expr" {
		return nil
	}
	if info.RealtimeTieredPricing == nil {
		info.RealtimeTieredPricing = &relaycommon.RealtimeTieredPricing{ExprHash: snap.ExprHash, QuotaPerUnit: snap.QuotaPerUnit, GroupRatio: snap.GroupRatio}
	}
	state := info.RealtimeTieredPricing
	if usage == nil || snap.ExprHash != billingexpr.ExprHashString(snap.ExprString) ||
		state.ExprHash != snap.ExprHash || state.QuotaPerUnit != snap.QuotaPerUnit || state.GroupRatio != snap.GroupRatio {
		state.Incomplete = true
		return fmt.Errorf("realtime tiered quote lacks stable usage or expression")
	}
	projected := &dto.Usage{PromptTokens: usage.InputTokens, CompletionTokens: usage.OutputTokens,
		PromptTokensDetails: usage.InputTokenDetails, CompletionTokenDetails: usage.OutputTokenDetails}
	request := billingexpr.RequestInput{}
	if info.BillingRequestInput != nil {
		request = *info.BillingRequestInput
	}
	result, err := billingexpr.ComputeTieredQuotaWithRequest(snap,
		BuildTieredTokenParams(projected, false, billingexpr.UsedVars(snap.ExprString)), request)
	if err != nil {
		state.Incomplete = true
		return err
	}
	noteQuotaClamp(info, result.Clamp)
	if result.Clamp != nil {
		state.Incomplete = true
		return result.Clamp
	}
	// Validate all totals before committing any of them; no bare integer sums
	// may wrap into a small reservation or refund.
	values := []struct{ old, delta int }{
		{state.Quota, result.ActualQuotaAfterGroup}, {state.Responses, 1},
		{state.InputTokens, usage.InputTokens}, {state.OutputTokens, usage.OutputTokens}, {state.TotalTokens, usage.TotalTokens},
	}
	updated := make([]int, len(values))
	for index, value := range values {
		if value.delta < 0 {
			state.Incomplete = true
			return fmt.Errorf("realtime reported token counts must be non-negative")
		}
		var clamp *common.QuotaClamp
		updated[index], clamp = common.QuotaFromDecimalChecked(decimal.NewFromInt(int64(value.old)).Add(decimal.NewFromInt(int64(value.delta))))
		noteQuotaClamp(info, clamp)
		if clamp != nil {
			state.Incomplete = true
			return clamp
		}
	}
	state.Quota, state.Responses, state.InputTokens, state.OutputTokens, state.TotalTokens = updated[0], updated[1], updated[2], updated[3], updated[4]
	return nil
}
