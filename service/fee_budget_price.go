package service

import (
	"context"
	"encoding/hex"
	"errors"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/pkg/billingexpr"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

var ErrFeeBudgetEvidence = errors.New("strict USD budget requires complete usage and applicable frozen published prices")

const feeBudgetPriceScope = "official_standard_text_usage_cost_not_invoice"

type feeBudgetPriceEvidence struct {
	LongRates                  *OpenAIOfficialTokenRates `json:"long_rates,omitempty"`
	ShortContextMaxInputTokens int64                     `json:"short_context_max_input_tokens,omitempty"`
	Version                    int                       `json:"version"`
	Currency                   string                    `json:"currency"`
	Scope                      string                    `json:"scope"`
	Model                      string                    `json:"model"`
	Profile                    string                    `json:"profile"`
	PublicationID              string                    `json:"publication_id"`
	SourceSHA256               string                    `json:"source_sha256"`
	ExpressionSHA256           string                    `json:"expression_sha256"`
	Rates                      OpenAIOfficialTokenRates  `json:"rates"`
}

// Explicit service_tier=default is required: omission is auto and can select
// project-level pricing. See https://developers.openai.com/api/reference/cli/resources/responses/methods/create
// Price provenance was captured with the billing expression before counting.
// Re-read immutable source/receipt evidence, never today's mutable tariff.
func freezeFeeBudgetPrice(ctx context.Context, db *gorm.DB, info *relaycommon.RelayInfo, bound *model.TokenBudgetReservation) error {
	if info == nil || bound == nil || bound.RequestServiceTier != "default" || info.TieredBillingSnapshot == nil {
		return ErrFeeBudgetEvidence
	}
	snapshot := info.TieredBillingSnapshot
	if snapshot.BillingMode != "tiered_expr" || snapshot.ExprVersion != 1 || snapshot.ModelName != bound.ModelName || snapshot.ExprHash != billingexpr.ExprHashString(snapshot.ExprString) || snapshot.OfficialPricePublicationID == "" {
		return ErrFeeBudgetEvidence
	}
	source, err := LoadFrozenOpenAIPriceSource(ctx, db, snapshot.OfficialPriceSourceSHA256)
	if err != nil {
		return err
	}
	candidate, err := BuildOpenAIPricePublicationCandidate(source, bound.ModelName)
	if err != nil || candidate.Expression != snapshot.ExprString {
		return ErrFeeBudgetEvidence
	}
	var receipt model.PricePublication
	if err := db.WithContext(ctx).First(&receipt, "id = ?", snapshot.OfficialPricePublicationID).Error; err != nil {
		return err
	}
	if receipt.ID != snapshot.OfficialPricePublicationID {
		return ErrFeeBudgetEvidence
	}
	var published model.PricePublicationSnapshot
	if common.UnmarshalJsonStr(receipt.AfterJSON, &published) != nil {
		return ErrFeeBudgetEvidence
	}
	binding, ok := published.State.Models[bound.ModelName]
	if !ok || binding.PublicationID != receipt.ID || binding.SourceSHA256 != source.ContentSHA256 || binding.ExpressionSHA256 != snapshot.ExprHash || published.Expressions[bound.ModelName] != snapshot.ExprString || published.Modes[bound.ModelName] != "tiered_expr" {
		return ErrFeeBudgetEvidence
	}
	if bound.BoundSource == model.TokenBudgetBoundOpenAIChat {
		return freezeChatFeeBudgetPrice(source, receipt.ID, snapshot.ExprHash, bound)
	}
	var rates OpenAIOfficialTokenRates
	profile := "short"
	for _, price := range source.Models {
		if price.Model != bound.ModelName {
			continue
		}
		rates = price.ShortContext
		if bound.InputTokens > int64(candidate.ShortContextMaxInputTokens) {
			if price.LongContext == nil {
				return ErrFeeBudgetEvidence
			}
			profile = "long"
			rates = *price.LongContext
		}
	}
	if rates.CachedInput == nil || rates.CacheWrite == nil {
		return ErrFeeBudgetEvidence
	}
	maximum := decimal.Zero
	for _, value := range []string{rates.Input, *rates.CachedInput, *rates.CacheWrite} {
		rate, err := decimal.NewFromString(value)
		if err != nil || rate.IsNegative() {
			return ErrFeeBudgetEvidence
		}
		if rate.GreaterThan(maximum) {
			maximum = rate
		}
	}
	outputRate, err := decimal.NewFromString(rates.Output)
	if err != nil || outputRate.IsNegative() {
		return ErrFeeBudgetEvidence
	}
	boundUSD := maximum.Mul(decimal.NewFromInt(bound.InputTokens)).Add(outputRate.Mul(decimal.NewFromInt(bound.MaxOutputTokens))).Shift(-6)
	bound.FeeReservedUSD, err = model.NormalizeFeeBudgetUSD(boundUSD.String())
	if err != nil {
		return err
	}
	encoded, err := common.Marshal(feeBudgetPriceEvidence{Version: 1, Currency: "USD", Scope: feeBudgetPriceScope, Model: bound.ModelName, Profile: profile,
		PublicationID: receipt.ID, SourceSHA256: source.ContentSHA256, ExpressionSHA256: snapshot.ExprHash, Rates: rates})
	if err != nil {
		return err
	}
	bound.FeeEnabled, bound.FeePriceEvidence = true, string(encoded)
	return nil
}

// Actual fee arithmetic reuses the existing exact source-cost calculator.
// Presence, provider/model/tier, and the persisted publication are separate
// qualification gates; no missing cache category is guessed as zero.
func calculateFeeBudgetUSD(row *model.TokenBudgetReservation, usage *dto.Usage) (string, error) {
	if row != nil && row.BoundSource == model.TokenBudgetBoundOpenAIChat {
		return calculateChatFeeBudgetUSD(row, usage)
	}
	if row == nil || !row.FeeEnabled || row.RequestServiceTier != "default" || usage == nil || usage.BillingUsage == nil {
		return "", ErrFeeBudgetEvidence
	}
	billing := usage.BillingUsage
	if billing.Estimated || billing.Incomplete || billing.Source != dto.BillingUsageSourceOAIResponses || billing.OpenAIUsage == nil || billing.ResponsesTextEvidence == nil {
		return "", ErrFeeBudgetEvidence
	}
	var price feeBudgetPriceEvidence
	if common.UnmarshalJsonStr(row.FeePriceEvidence, &price) != nil || price.Version != 1 || price.Currency != "USD" || price.Scope != feeBudgetPriceScope || price.Model != row.ModelName {
		return "", ErrFeeBudgetEvidence
	}
	for _, digest := range []string{price.PublicationID, price.SourceSHA256, price.ExpressionSHA256} {
		decoded, err := hex.DecodeString(digest)
		if err != nil || len(decoded) != 32 || hex.EncodeToString(decoded) != digest {
			return "", ErrFeeBudgetEvidence
		}
	}
	if price.Rates.CachedInput == nil || price.Rates.CacheWrite == nil {
		return "", ErrFeeBudgetEvidence
	}
	for _, rate := range []string{price.Rates.Input, price.Rates.Output, *price.Rates.CachedInput, *price.Rates.CacheWrite} {
		if len(rate) > 64 || !openAIPriceAmountPattern.MatchString("$"+rate) {
			return "", ErrFeeBudgetEvidence
		}
	}
	evidence, raw := billing.ResponsesTextEvidence, billing.OpenAIUsage
	if evidence.Model != price.Model || evidence.ServiceTier != "default" || evidence.CacheRead == nil || evidence.CacheWrite == nil || raw.InputTokensDetails == nil {
		return "", ErrFeeBudgetEvidence
	}
	details := raw.InputTokensDetails
	if *evidence.CacheRead != details.CachedTokens || *evidence.CacheWrite != details.CacheWriteTokens || details.CachedCreationTokens != 0 || details.ImageTokens != 0 || details.AudioTokens != 0 {
		return "", ErrFeeBudgetEvidence
	}
	var reasoning *int
	if raw.OutputTokensDetails != nil {
		if raw.OutputTokensDetails.ImageTokens != 0 || raw.OutputTokensDetails.AudioTokens != 0 {
			return "", ErrFeeBudgetEvidence
		}
		reasoning = &raw.OutputTokensDetails.ReasoningTokens
	}
	if details.CachedTokensDetails != nil {
		if text := details.CachedTokensDetails.TextTokens; text != nil && *text != *evidence.CacheRead {
			return "", ErrFeeBudgetEvidence
		}
		for _, count := range []*int{details.CachedTokensDetails.ImageTokens, details.CachedTokensDetails.AudioTokens} {
			if count != nil && *count != 0 {
				return "", ErrFeeBudgetEvidence
			}
		}
	}
	profile := "short"
	if raw.InputTokens > 272000 {
		profile = "long"
	}
	if price.Profile != profile {
		return "", ErrFeeBudgetEvidence
	}
	source := &OpenAIOfficialPriceSnapshot{SourceURL: openAIOfficialPricingURL, ContentSHA256: price.SourceSHA256, Currency: "USD", UnitTokens: 1_000_000, ServiceTier: "standard", Scope: "text-token-price-source-not-published",
		Models: []OpenAIOfficialModelPrice{{Model: price.Model, ShortContext: price.Rates, LongContext: &price.Rates}}}
	cost, err := CalculateOpenAITextSourceCost(source, price.Model, price.Profile, OpenAITextCostUsage{InputTokens: &raw.InputTokens, OutputTokens: &raw.OutputTokens, CacheReadTokens: evidence.CacheRead, CacheWriteTokens: evidence.CacheWrite, ReasoningTokens: reasoning, Reported: true, TextOnly: true, ContextQualified: true})
	if err != nil {
		return "", err
	}
	return model.NormalizeFeeBudgetUSD(cost.TotalUSD)
}
