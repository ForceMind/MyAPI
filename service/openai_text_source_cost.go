package service

import (
	"encoding/hex"
	"fmt"

	"github.com/ForceMind/MyAPI/common"
	"github.com/shopspring/decimal"
)

// All mandatory counters are optional at the type boundary so a missing
// report is not converted into an explicit zero. Callers must also establish
// text-only usage and the applicability of the selected context profile.
type OpenAITextCostUsage struct {
	InputTokens      *int
	OutputTokens     *int
	CacheReadTokens  *int
	CacheWriteTokens *int
	ReasoningTokens  *int
	Reported         bool
	TextOnly         bool
	ContextQualified bool
}

type OpenAITextSourceCost struct {
	Model               string `json:"model"`
	ContextProfile      string `json:"context_profile"`
	SourceSHA256        string `json:"source_sha256"`
	Currency            string `json:"currency"`
	Scope               string `json:"scope"`
	UncachedInputTokens int    `json:"uncached_input_tokens"`
	CacheReadTokens     int    `json:"cache_read_tokens"`
	CacheWriteTokens    int    `json:"cache_write_tokens"`
	OutputTokens        int    `json:"output_tokens"`
	UncachedInputUSD    string `json:"uncached_input_usd"`
	CacheReadUSD        string `json:"cache_read_usd"`
	CacheWriteUSD       string `json:"cache_write_usd"`
	OutputUSD           string `json:"output_usd"`
	TotalUSD            string `json:"total_usd"`
	CacheHitNumerator   int    `json:"cache_hit_numerator"`
	CacheHitDenominator int    `json:"cache_hit_denominator"`
}

// CalculateOpenAITextSourceCost computes source USD, not wallet quota or an
// effective administrator tariff. It never publishes/applies this source.
func CalculateOpenAITextSourceCost(snapshot *OpenAIOfficialPriceSnapshot, modelName, profile string, usage OpenAITextCostUsage) (*OpenAITextSourceCost, error) {
	if snapshot == nil || snapshot.SourceURL != openAIOfficialPricingURL || snapshot.Currency != "USD" || snapshot.UnitTokens != 1_000_000 ||
		snapshot.ServiceTier != "standard" || snapshot.Scope != "text-token-price-source-not-published" || len(snapshot.ContentSHA256) != 64 {
		return nil, fmt.Errorf("unsupported official text price source")
	}
	if _, err := hex.DecodeString(snapshot.ContentSHA256); err != nil {
		return nil, fmt.Errorf("invalid official price source digest")
	}
	if !usage.Reported || !usage.TextOnly || !usage.ContextQualified {
		return nil, fmt.Errorf("cost requires reported text usage and a qualified context profile")
	}
	for _, count := range []*int{usage.InputTokens, usage.OutputTokens, usage.CacheReadTokens, usage.CacheWriteTokens} {
		if count == nil || *count < 0 || *count > common.MaxQuota {
			return nil, fmt.Errorf("missing or out-of-range text token count")
		}
	}
	if usage.ReasoningTokens != nil && (*usage.ReasoningTokens < 0 || *usage.ReasoningTokens > *usage.OutputTokens) {
		return nil, fmt.Errorf("reasoning tokens must be a subset of output tokens")
	}
	remaining := *usage.InputTokens
	if *usage.CacheReadTokens > remaining {
		return nil, fmt.Errorf("cached input exceeds total input")
	}
	remaining -= *usage.CacheReadTokens
	if *usage.CacheWriteTokens > remaining {
		return nil, fmt.Errorf("cache reads and writes exceed total input")
	}
	remaining -= *usage.CacheWriteTokens
	var selected *OpenAIOfficialModelPrice
	for index := range snapshot.Models {
		if snapshot.Models[index].Model != modelName {
			continue
		}
		if selected != nil {
			return nil, fmt.Errorf("ambiguous model price source")
		}
		selected = &snapshot.Models[index]
	}
	if selected == nil {
		return nil, fmt.Errorf("model is absent from official text price source")
	}
	var rates OpenAIOfficialTokenRates
	switch profile {
	case "short":
		rates = selected.ShortContext
	case "long":
		if selected.LongContext == nil {
			return nil, fmt.Errorf("long-context price is not quoted")
		}
		rates = *selected.LongContext
	default:
		return nil, fmt.Errorf("context profile must be explicitly selected")
	}
	inputs := []struct {
		rate  *string
		count int
	}{
		{&rates.Input, remaining}, {rates.CachedInput, *usage.CacheReadTokens},
		{rates.CacheWrite, *usage.CacheWriteTokens}, {&rates.Output, *usage.OutputTokens},
	}
	var amounts [4]decimal.Decimal
	total := decimal.Zero
	for index, input := range inputs {
		if input.rate == nil {
			if input.count != 0 {
				return nil, fmt.Errorf("consumed token category has no quoted price")
			}
			amounts[index] = decimal.Zero
			continue
		}
		if len(*input.rate) > maxOpenAIPriceDocumentBytes || !openAIPriceAmountPattern.MatchString("$"+*input.rate) {
			return nil, fmt.Errorf("invalid decimal token price")
		}
		price, err := decimal.NewFromString(*input.rate)
		if err != nil {
			return nil, err
		}
		// Shift is exact. Div would use global division precision and silently
		// round sufficiently small but legitimate prices to zero.
		amounts[index] = price.Mul(decimal.NewFromInt(int64(input.count))).Shift(-6)
		total = total.Add(amounts[index])
	}
	return &OpenAITextSourceCost{Model: modelName, ContextProfile: profile, SourceSHA256: snapshot.ContentSHA256,
		Currency: "USD", Scope: "source-cost-calculation-not-applied", UncachedInputTokens: remaining,
		CacheReadTokens: *usage.CacheReadTokens, CacheWriteTokens: *usage.CacheWriteTokens, OutputTokens: *usage.OutputTokens,
		UncachedInputUSD: amounts[0].String(), CacheReadUSD: amounts[1].String(), CacheWriteUSD: amounts[2].String(), OutputUSD: amounts[3].String(), TotalUSD: total.String(),
		CacheHitNumerator: *usage.CacheReadTokens, CacheHitDenominator: *usage.InputTokens}, nil
}
