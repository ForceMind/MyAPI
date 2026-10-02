package service

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/pkg/billingexpr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func publicationSourceFixture(document string) *OpenAIOfficialPriceSnapshot {
	return &OpenAIOfficialPriceSnapshot{sourceDocument: document, ContentSHA256: fmt.Sprintf("%x", sha256.Sum256([]byte(document))), SourceURL: openAIOfficialPricingURL,
		Currency: "USD", UnitTokens: 1_000_000, ServiceTier: "standard", Scope: "text-token-price-source-not-published"}
}

func publicationDocumentFixture() string {
	return officialPriceFixtureHeader + strings.ReplaceAll(officialPriceFixtureRow, " (<272K context length)", "") + "\nShort context: ≤272K input tokens. Long context: >272K input tokens.\n"
}

func TestOpenAIPricePublicationCandidateUsesSourceAndFullContext(t *testing.T) {
	source := publicationSourceFixture(publicationDocumentFixture())
	// Displayed client rates are not evidence. The retained document is reparsed.
	source.Models = []OpenAIOfficialModelPrice{{Model: "fixture-model", ShortContext: OpenAIOfficialTokenRates{Input: "999"}}}
	candidate, err := BuildOpenAIPricePublicationCandidate(source, "fixture-model")
	require.NoError(t, err)
	assert.Equal(t, source.ContentSHA256, candidate.SourceSHA256)
	assert.Equal(t, 272000, candidate.ShortContextMaxInputTokens)
	assert.Equal(t, billingexpr.ExprHashString(candidate.Expression), candidate.ExpressionSHA256)
	for _, test := range []struct {
		length float64
		tier   string
		want   float64
	}{
		{272000, "short", 164094}, {272001, "long", 328157},
	} {
		// Cache remains an input subset; the condition uses full len, not p.
		cost, trace, err := billingexpr.RunExpr(candidate.Expression, billingexpr.TokenParams{P: test.length - 200000, Len: test.length, CR: 199990, CC: 10, C: 7})
		require.NoError(t, err)
		assert.Equal(t, test.tier, trace.MatchedTier)
		assert.InDelta(t, test.want, cost, 0.000001)
	}
	assert.Equal(t, "text-token-price-source-not-published", source.Scope)
}

func TestOpenAIPricePublicationCandidateRejectsAmbiguousEvidence(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(string) string
	}{
		{"missing boundary", func(s string) string { return strings.Split(s, "\nShort context:")[0] }},
		{"duplicate boundary", func(s string) string {
			return s + "Short context: ≤272K input tokens. Long context: >272K input tokens.\n"
		}},
		{"different boundary", func(s string) string { return strings.ReplaceAll(s, "272K", "128K") }},
		{"qualified model", func(s string) string {
			return strings.ReplaceAll(s, "| fixture-model |", "| fixture-model (<272K context length) |")
		}},
		{"unquoted cache", func(s string) string { return strings.Replace(s, "$0.10", "-", 1) }},
		{"unquoted long cache", func(s string) string { return strings.Replace(s, "$5.00", "-", 1) }},
		{"unquoted long", func(s string) string { return strings.Replace(s, "$4.00 | $0.20 | $5.00 | $15.00", "- | - | - | -", 1) }},
		{"float overflow", func(s string) string { return strings.Replace(s, "$2.00", "$"+strings.Repeat("9", 400), 1) }},
		{"float underflow", func(s string) string { return strings.Replace(s, "$2.00", "$0."+strings.Repeat("0", 400)+"1", 1) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate, err := BuildOpenAIPricePublicationCandidate(publicationSourceFixture(test.change(publicationDocumentFixture())), "fixture-model")
			require.Error(t, err)
			assert.Nil(t, candidate)
		})
	}
	source := publicationSourceFixture(publicationDocumentFixture())
	source.sourceDocument += "tampered"
	_, err := BuildOpenAIPricePublicationCandidate(source, "fixture-model")
	require.Error(t, err)
	_, err = BuildOpenAIPricePublicationCandidate(publicationSourceFixture(publicationDocumentFixture()), "other-model")
	require.Error(t, err)
}
