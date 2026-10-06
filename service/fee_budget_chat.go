package service

import (
	"encoding/hex"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/shopspring/decimal"
)

// Only the first reviewed model needs v2: its context-sized reservation does
// not identify the actual short/long price tier. Freeze both applicable tiers.
func freezeChatFeeBudgetPrice(source *OpenAIOfficialPriceSnapshot, publication, expression string, bound *model.TokenBudgetReservation) error {
	if bound.ModelName != model.TokenBudgetOpenAIChatModel || bound.InputTokens != model.TokenBudgetOpenAIChatContext || bound.MaxOutputTokens <= 0 || bound.MaxOutputTokens > model.TokenBudgetOpenAIChatMaxOutput {
		return ErrFeeBudgetEvidence
	}
	for _, price := range source.Models {
		if price.Model != bound.ModelName {
			continue
		}
		if price.LongContext == nil {
			return ErrFeeBudgetEvidence
		}
		maxInput, maxOutput := decimal.Zero, decimal.Zero
		for _, rates := range []OpenAIOfficialTokenRates{price.ShortContext, *price.LongContext} {
			if rates.CachedInput == nil || rates.CacheWrite == nil {
				return ErrFeeBudgetEvidence
			}
			for _, value := range []string{rates.Input, *rates.CachedInput, *rates.CacheWrite} {
				rate, err := decimal.NewFromString(value)
				if err != nil || rate.IsNegative() {
					return ErrFeeBudgetEvidence
				}
				if rate.GreaterThan(maxInput) {
					maxInput = rate
				}
			}
			rate, err := decimal.NewFromString(rates.Output)
			if err != nil || rate.IsNegative() {
				return ErrFeeBudgetEvidence
			}
			if rate.GreaterThan(maxOutput) {
				maxOutput = rate
			}
		}
		premium := maxOutput.Sub(maxInput)
		if premium.IsNegative() {
			premium = decimal.Zero
		}
		amount := maxInput.Mul(decimal.NewFromInt(bound.InputTokens)).Add(premium.Mul(decimal.NewFromInt(bound.MaxOutputTokens))).Shift(-6)
		reserved, err := model.NormalizeFeeBudgetUSD(amount.String())
		if err != nil {
			return err
		}
		encoded, err := common.Marshal(feeBudgetPriceEvidence{Version: 2, Currency: "USD", Scope: feeBudgetPriceScope, Model: bound.ModelName, Profile: "actual_input",
			PublicationID: publication, SourceSHA256: source.ContentSHA256, ExpressionSHA256: expression,
			Rates: price.ShortContext, LongRates: price.LongContext, ShortContextMaxInputTokens: 272000})
		if err != nil {
			return err
		}
		bound.FeeEnabled, bound.FeeReservedUSD, bound.FeePriceEvidence = true, reserved, string(encoded)
		return nil
	}
	return ErrFeeBudgetEvidence
}

func calculateChatFeeBudgetUSD(row *model.TokenBudgetReservation, usage *dto.Usage) (string, error) {
	if row == nil || !row.FeeEnabled || row.BoundSource != model.TokenBudgetBoundOpenAIChat || row.RequestServiceTier != "default" {
		return "", ErrFeeBudgetEvidence
	}
	evidence, err := qualifiedChatBudgetUsage(row, usage)
	if err != nil || evidence.ServiceTier != "default" || evidence.CacheRead == nil || evidence.CacheWrite == nil {
		return "", ErrFeeBudgetEvidence
	}
	var price feeBudgetPriceEvidence
	if _, err := common.CanonicalJSONObjectDigest([]byte(row.FeePriceEvidence)); err != nil {
		return "", ErrFeeBudgetEvidence
	}
	if common.UnmarshalJsonStr(row.FeePriceEvidence, &price) != nil || price.Version != 2 || price.Currency != "USD" || price.Scope != feeBudgetPriceScope ||
		price.Model != row.ModelName || price.Profile != "actual_input" || price.ShortContextMaxInputTokens != 272000 || price.LongRates == nil {
		return "", ErrFeeBudgetEvidence
	}
	for _, digest := range []string{price.PublicationID, price.SourceSHA256, price.ExpressionSHA256} {
		decoded, err := hex.DecodeString(digest)
		if err != nil || len(decoded) != 32 || hex.EncodeToString(decoded) != digest {
			return "", ErrFeeBudgetEvidence
		}
	}
	for _, rates := range []OpenAIOfficialTokenRates{price.Rates, *price.LongRates} {
		if rates.CachedInput == nil || rates.CacheWrite == nil {
			return "", ErrFeeBudgetEvidence
		}
		for _, value := range []string{rates.Input, rates.Output, *rates.CachedInput, *rates.CacheWrite} {
			if len(value) > 64 || !openAIPriceAmountPattern.MatchString("$"+value) {
				return "", ErrFeeBudgetEvidence
			}
		}
	}
	profile := "short"
	if int64(evidence.PromptTokens) > price.ShortContextMaxInputTokens {
		profile = "long"
	}
	source := &OpenAIOfficialPriceSnapshot{SourceURL: openAIOfficialPricingURL, ContentSHA256: price.SourceSHA256, Currency: "USD", UnitTokens: 1000000, ServiceTier: "standard", Scope: "text-token-price-source-not-published",
		Models: []OpenAIOfficialModelPrice{{Model: price.Model, ShortContext: price.Rates, LongContext: price.LongRates}}}
	cost, err := CalculateOpenAITextSourceCost(source, price.Model, profile, OpenAITextCostUsage{InputTokens: &evidence.PromptTokens, OutputTokens: &evidence.CompletionTokens,
		CacheReadTokens: evidence.CacheRead, CacheWriteTokens: evidence.CacheWrite, ReasoningTokens: evidence.Reasoning, Reported: true, TextOnly: true, ContextQualified: true})
	if err != nil {
		return "", err
	}
	return model.NormalizeFeeBudgetUSD(cost.TotalUSD)
}
