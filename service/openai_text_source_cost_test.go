package service

import (
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func textCostFixture() (*OpenAIOfficialPriceSnapshot, OpenAITextCostUsage) {
	rates := OpenAIOfficialTokenRates{Input: "2.00", CachedInput: common.GetPointer("0.20"), CacheWrite: common.GetPointer("2.50"), Output: "10.00"}
	return &OpenAIOfficialPriceSnapshot{SourceURL: openAIOfficialPricingURL, ContentSHA256: strings.Repeat("a", 64), Currency: "USD", UnitTokens: 1_000_000,
			ServiceTier: "standard", Scope: "text-token-price-source-not-published", Models: []OpenAIOfficialModelPrice{{Model: "fixture-model", ShortContext: rates}}},
		OpenAITextCostUsage{InputTokens: common.GetPointer(100), OutputTokens: common.GetPointer(10), CacheReadTokens: common.GetPointer(40), CacheWriteTokens: common.GetPointer(30),
			ReasoningTokens: common.GetPointer(4), Reported: true, TextOnly: true, ContextQualified: true}
}

func TestOfficialTextCostPartitionsInputAndDoesNotChargeReasoningTwice(t *testing.T) {
	snapshot, usage := textCostFixture()
	cost, err := CalculateOpenAITextSourceCost(snapshot, "fixture-model", "short", usage)
	require.NoError(t, err)
	assert.Equal(t, 30, cost.UncachedInputTokens)
	assert.Equal(t, "0.00006", cost.UncachedInputUSD)
	assert.Equal(t, "0.000008", cost.CacheReadUSD)
	assert.Equal(t, "0.000075", cost.CacheWriteUSD)
	assert.Equal(t, "0.0001", cost.OutputUSD)
	assert.Equal(t, "0.000243", cost.TotalUSD)
	assert.Equal(t, 40, cost.CacheHitNumerator)
	assert.Equal(t, 100, cost.CacheHitDenominator)
	assert.Equal(t, snapshot.ContentSHA256, cost.SourceSHA256)
	assert.Equal(t, "source-cost-calculation-not-applied", cost.Scope)
	*snapshot.Models[0].ShortContext.CacheWrite = "999"
	assert.Equal(t, "0.000243", cost.TotalUSD, "computed result must not reprice when source object changes")
}

func TestOfficialTextCostPreservesTinyPricesAndKnownZero(t *testing.T) {
	snapshot, usage := textCostFixture()
	snapshot.Models[0].ShortContext.Input = "0.00000000000000000012"
	usage.InputTokens = common.GetPointer(3)
	usage.OutputTokens, usage.CacheReadTokens, usage.CacheWriteTokens, usage.ReasoningTokens = common.GetPointer(0), common.GetPointer(0), common.GetPointer(0), nil
	cost, err := CalculateOpenAITextSourceCost(snapshot, "fixture-model", "short", usage)
	require.NoError(t, err)
	assert.Equal(t, "0.00000000000000000000000036", cost.TotalUSD)
	usage.InputTokens = common.GetPointer(0)
	cost, err = CalculateOpenAITextSourceCost(snapshot, "fixture-model", "short", usage)
	require.NoError(t, err)
	assert.Equal(t, "0", cost.TotalUSD)
	assert.Zero(t, cost.CacheHitDenominator, "zero input has no fabricated hit rate")
}

func TestOfficialTextCostRejectsUnqualifiedOrIncompleteData(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*OpenAIOfficialPriceSnapshot, *OpenAITextCostUsage)
	}{
		{"unknown", func(_ *OpenAIOfficialPriceSnapshot, u *OpenAITextCostUsage) { u.Reported = false }},
		{"multimodal", func(_ *OpenAIOfficialPriceSnapshot, u *OpenAITextCostUsage) { u.TextOnly = false }},
		{"context eligibility unknown", func(_ *OpenAIOfficialPriceSnapshot, u *OpenAITextCostUsage) { u.ContextQualified = false }},
		{"missing output", func(_ *OpenAIOfficialPriceSnapshot, u *OpenAITextCostUsage) { u.OutputTokens = nil }},
		{"negative", func(_ *OpenAIOfficialPriceSnapshot, u *OpenAITextCostUsage) {
			u.CacheWriteTokens = common.GetPointer(-1)
		}},
		{"overlapping prefix counters", func(_ *OpenAIOfficialPriceSnapshot, u *OpenAITextCostUsage) {
			u.CacheWriteTokens = common.GetPointer(70)
		}},
		{"missing consumed price", func(s *OpenAIOfficialPriceSnapshot, _ *OpenAITextCostUsage) {
			s.Models[0].ShortContext.CachedInput = nil
		}},
		{"invalid price", func(s *OpenAIOfficialPriceSnapshot, _ *OpenAITextCostUsage) { s.Models[0].ShortContext.Input = "NaN" }},
		{"wrong currency", func(s *OpenAIOfficialPriceSnapshot, _ *OpenAITextCostUsage) { s.Currency = "CNY" }},
		{"wrong unit", func(s *OpenAIOfficialPriceSnapshot, _ *OpenAITextCostUsage) { s.UnitTokens = 1000 }},
		{"wrong tier", func(s *OpenAIOfficialPriceSnapshot, _ *OpenAITextCostUsage) { s.ServiceTier = "fast" }},
		{"wrong digest", func(s *OpenAIOfficialPriceSnapshot, _ *OpenAITextCostUsage) {
			s.ContentSHA256 = strings.Repeat("z", 64)
		}},
		{"duplicate model", func(s *OpenAIOfficialPriceSnapshot, _ *OpenAITextCostUsage) { s.Models = append(s.Models, s.Models[0]) }},
		{"reasoning above output", func(_ *OpenAIOfficialPriceSnapshot, u *OpenAITextCostUsage) {
			u.ReasoningTokens = common.GetPointer(11)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			snapshot, usage := textCostFixture()
			test.mutate(snapshot, &usage)
			cost, err := CalculateOpenAITextSourceCost(snapshot, "fixture-model", "short", usage)
			require.Error(t, err)
			assert.Nil(t, cost)
		})
	}
	snapshot, usage := textCostFixture()
	_, err := CalculateOpenAITextSourceCost(snapshot, "fixture-model", "long", usage)
	require.Error(t, err)
	_, err = CalculateOpenAITextSourceCost(snapshot, "different-model", "short", usage)
	require.Error(t, err)
}

func TestOfficialTextCostRequiresPricesOnlyForConsumedCategories(t *testing.T) {
	snapshot, usage := textCostFixture()
	snapshot.Models[0].ShortContext.CachedInput = nil
	snapshot.Models[0].ShortContext.CacheWrite = nil
	usage.CacheReadTokens, usage.CacheWriteTokens = common.GetPointer(0), common.GetPointer(0)
	cost, err := CalculateOpenAITextSourceCost(snapshot, "fixture-model", "short", usage)
	require.NoError(t, err)
	assert.Equal(t, "0.0003", cost.TotalUSD)
	assert.Nil(t, snapshot.Models[0].ShortContext.CachedInput, "absence remains absence, not a free-price rewrite")
	usage.CacheReadTokens = common.GetPointer(1)
	_, err = CalculateOpenAITextSourceCost(snapshot, "fixture-model", "short", usage)
	require.Error(t, err)
}

func TestOfficialTextCostUsesExplicitLongProfileAndTokenBounds(t *testing.T) {
	snapshot, usage := textCostFixture()
	snapshot.Models[0].LongContext = &OpenAIOfficialTokenRates{Input: "4", CachedInput: common.GetPointer("0.4"), CacheWrite: common.GetPointer("5"), Output: "15"}
	cost, err := CalculateOpenAITextSourceCost(snapshot, "fixture-model", "long", usage)
	require.NoError(t, err)
	assert.Equal(t, "0.000436", cost.TotalUSD)
	assert.Equal(t, "long", cost.ContextProfile)
	_, err = CalculateOpenAITextSourceCost(snapshot, "fixture-model", "auto", usage)
	require.Error(t, err, "no guessed context threshold")
	usage.InputTokens = common.GetPointer(common.MaxQuota + 1)
	_, err = CalculateOpenAITextSourceCost(snapshot, "fixture-model", "short", usage)
	require.Error(t, err)
}
