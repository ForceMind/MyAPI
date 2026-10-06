package service

import (
	"crypto/sha256"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/ForceMind/MyAPI/pkg/billingexpr"
)

// A candidate is only a proposed administrator tariff. Constructing it does
// not publish prices, change options, or establish an actual upstream bill.
type OpenAIPricePublicationCandidate struct {
	Model                      string `json:"model"`
	SourceSHA256               string `json:"source_sha256"`
	Expression                 string `json:"expression"`
	ExpressionSHA256           string `json:"expression_sha256"`
	ShortContextMaxInputTokens int    `json:"short_context_max_input_tokens"`
	ServiceTier                string `json:"service_tier"`
	Scope                      string `json:"scope"`
}

// BuildOpenAIPricePublicationCandidate deliberately supports only the current
// explicitly documented context contract and bare, fully quoted text models.
// New source layouts/qualifiers require reviewed support, never inference from
// a column heading or client-posted price. Applicability must also be enforced
// at admission and settlement before any candidate becomes an effective price.
func BuildOpenAIPricePublicationCandidate(source *OpenAIOfficialPriceSnapshot, modelName string) (*OpenAIPricePublicationCandidate, error) {
	if source == nil || source.SourceURL != openAIOfficialPricingURL || source.Currency != "USD" || source.UnitTokens != 1_000_000 ||
		source.ServiceTier != "standard" || source.Scope != "text-token-price-source-not-published" || len(source.sourceDocument) == 0 || len(source.sourceDocument) > maxOpenAIPriceDocumentBytes {
		return nil, fmt.Errorf("unsupported official publication source")
	}
	if fmt.Sprintf("%x", sha256.Sum256([]byte(source.sourceDocument))) != source.ContentSHA256 {
		return nil, fmt.Errorf("official publication source digest mismatch")
	}
	if len(modelName) > 191 || !openAIPriceModelPattern.MatchString(modelName) {
		return nil, fmt.Errorf("invalid publication model")
	}
	const contextEvidence = "Short context: ≤272K input tokens. Long context: >272K input tokens."
	boundaries := 0
	for _, line := range strings.Split(source.sourceDocument, "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "Short context:") || strings.Contains(line, "Long context:") {
			if line != contextEvidence {
				return nil, fmt.Errorf("unsupported or conflicting context qualification")
			}
			boundaries++
		}
	}
	if boundaries != 1 {
		return nil, fmt.Errorf("publication requires one explicit context qualification")
	}
	models, err := parseOpenAIStandardTextPrices([]byte(source.sourceDocument))
	if err != nil {
		return nil, err
	}
	var selected *OpenAIOfficialModelPrice
	for index := range models {
		if models[index].Model == modelName {
			selected = &models[index]
		}
	}
	if selected == nil || selected.SourceLabel != modelName || selected.LongContext == nil {
		return nil, fmt.Errorf("model requires unambiguous short and long context prices")
	}
	profiles := []OpenAIOfficialTokenRates{selected.ShortContext, *selected.LongContext}
	terms := make([]string, 2)
	for index, rates := range profiles {
		if rates.CachedInput == nil || rates.CacheWrite == nil {
			return nil, fmt.Errorf("publication requires quoted cache categories")
		}
		for _, rate := range []string{rates.Input, rates.Output, *rates.CachedInput, *rates.CacheWrite} {
			// The existing expression engine uses float64. Never silently turn
			// an unrepresentable positive coefficient into zero or infinity.
			value, err := strconv.ParseFloat(rate, 64)
			if len(rate) > 64 || err != nil || math.IsInf(value, 0) || math.IsNaN(value) || value < 0 || (value == 0 && strings.Trim(rate, "0.") != "") {
				return nil, fmt.Errorf("price cannot be represented by the billing expression engine")
			}
		}
		terms[index] = fmt.Sprintf("p * %s + c * %s + cr * %s + cc * %s", rates.Input, rates.Output, *rates.CachedInput, *rates.CacheWrite)
	}
	expression := fmt.Sprintf("v1:len <= 272000 ? tier(\"short\", %s) : tier(\"long\", %s)", terms[0], terms[1])
	if _, _, err := billingexpr.RunExpr(expression, billingexpr.TokenParams{P: 1, C: 1, Len: 1, CR: 1, CC: 1}); err != nil {
		return nil, err
	}
	return &OpenAIPricePublicationCandidate{Model: modelName, SourceSHA256: source.ContentSHA256, Expression: expression,
		ExpressionSHA256: billingexpr.ExprHashString(expression), ShortContextMaxInputTokens: 272000,
		ServiceTier: "standard", Scope: "standard-text-publication-candidate-not-applied"}, nil
}
