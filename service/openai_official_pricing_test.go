package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/ForceMind/MyAPI/model"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const officialPriceFixtureHeader = `Prices per 1M tokens.

### Standard pricing data
| Model | Short context input | Short context cached input | Short context cache writes | Short context output | Long context input | Long context cached input | Long context cache writes | Long context output |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
`

const officialPriceFixtureRow = "| fixture-model (<272K context length) | $2.00 | $0.10 | $2.50 | $10.00 | $4.00 | $0.20 | $5.00 | $15.00 |\n"

type officialPriceRoundTripper func(*http.Request) (*http.Response, error)

func (f officialPriceRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestOfficialPricingParserKeepsContextProfilesAndMissingRates(t *testing.T) {
	body := []byte("# Pricing\n\n" + officialPriceFixtureHeader + officialPriceFixtureRow +
		"| fixture-no-cache | $0.005 | - | - | $0.25 | - | - | - | - |\n\n### Batch pricing data\n" +
		"| fixture-model | $999 | $999 | $999 | $999 | $999 | $999 | $999 | $999 |\n")
	models, err := parseOpenAIStandardTextPrices(body)
	require.NoError(t, err)
	require.Len(t, models, 2)
	assert.Equal(t, "fixture-model", models[0].Model)
	assert.Equal(t, "fixture-model (<272K context length)", models[0].SourceLabel)
	assert.Equal(t, "2.00", models[0].ShortContext.Input)
	require.NotNil(t, models[0].ShortContext.CachedInput)
	assert.Equal(t, "0.10", *models[0].ShortContext.CachedInput)
	require.NotNil(t, models[0].LongContext)
	assert.Equal(t, "4.00", models[0].LongContext.Input)
	assert.Equal(t, "15.00", models[0].LongContext.Output)
	assert.Equal(t, "0.005", models[1].ShortContext.Input)
	assert.Nil(t, models[1].ShortContext.CachedInput)
	assert.Nil(t, models[1].ShortContext.CacheWrite)
	assert.Nil(t, models[1].LongContext)
}

func TestOfficialPricingParserRejectsAmbiguousOrInvalidDocuments(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"no standard table", "### Batch pricing data\n" + officialPriceFixtureRow},
		{"changed header", strings.ReplaceAll(officialPriceFixtureHeader, "Short context input", "Unknown price") + officialPriceFixtureRow},
		{"duplicate model", officialPriceFixtureHeader + officialPriceFixtureRow + officialPriceFixtureRow},
		{"missing output", officialPriceFixtureHeader + strings.Replace(officialPriceFixtureRow, "$10.00", "-", 1)},
		{"partial long profile", officialPriceFixtureHeader + strings.Replace(officialPriceFixtureRow, "$15.00", "-", 1)},
		{"nonfinite price", officialPriceFixtureHeader + strings.Replace(officialPriceFixtureRow, "$2.00", "$NaN", 1)},
		{"negative price", officialPriceFixtureHeader + strings.Replace(officialPriceFixtureRow, "$2.00", "$-1", 1)},
		{"changed unit", strings.ReplaceAll(officialPriceFixtureHeader, "Prices per 1M tokens.", "Prices per 1K tokens.") + officialPriceFixtureRow},
	} {
		t.Run(tc.name, func(t *testing.T) {
			models, err := parseOpenAIStandardTextPrices([]byte(tc.body))
			require.Error(t, err)
			assert.Nil(t, models)
		})
	}
}

func TestOfficialPricingFetchProducesEvidenceWithoutCredentials(t *testing.T) {
	oldClient := openAIOfficialPricingClient
	t.Cleanup(func() { openAIOfficialPricingClient = oldClient })
	body := officialPriceFixtureHeader + officialPriceFixtureRow
	openAIOfficialPricingClient = &http.Client{Transport: officialPriceRoundTripper(func(req *http.Request) (*http.Response, error) {
		assert.Equal(t, openAIOfficialPricingURL, req.URL.String())
		assert.Empty(t, req.Header.Get("Authorization"))
		assert.Empty(t, req.Header.Get("Cookie"))
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/markdown"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	snapshot, err := FetchOpenAIOfficialPricing(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "USD", snapshot.Currency)
	assert.Equal(t, 1_000_000, snapshot.UnitTokens)
	assert.Equal(t, "standard", snapshot.ServiceTier)
	assert.Equal(t, openAIOfficialPricingURL, snapshot.SourceURL)
	digest := sha256.Sum256([]byte(body))
	assert.Equal(t, hex.EncodeToString(digest[:]), snapshot.ContentSHA256)
	assert.Positive(t, snapshot.FetchedAt)
}

func TestOfficialPricingFetchRejectsHTTPFailureOversizeAndCancellation(t *testing.T) {
	oldClient := openAIOfficialPricingClient
	t.Cleanup(func() { openAIOfficialPricingClient = oldClient })
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"HTTP failure", http.StatusServiceUnavailable, officialPriceFixtureHeader + officialPriceFixtureRow},
		{"oversize", http.StatusOK, strings.Repeat("x", maxOpenAIPriceDocumentBytes+1)},
		{"HTML not a pricing table", http.StatusOK, "<html>Unavailable</html>"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			openAIOfficialPricingClient = &http.Client{Transport: officialPriceRoundTripper(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})}
			snapshot, err := FetchOpenAIOfficialPricing(context.Background())
			require.Error(t, err)
			assert.Nil(t, snapshot)
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	openAIOfficialPricingClient = &http.Client{Transport: officialPriceRoundTripper(func(req *http.Request) (*http.Response, error) {
		return nil, req.Context().Err()
	})}
	snapshot, err := FetchOpenAIOfficialPricing(ctx)
	require.Error(t, err)
	assert.True(t, errors.Is(err, context.Canceled))
	assert.Nil(t, snapshot)
}

func TestOfficialPricingFetchDoesNotFollowRedirect(t *testing.T) {
	oldClient := openAIOfficialPricingClient
	t.Cleanup(func() { openAIOfficialPricingClient = oldClient })
	client := *oldClient
	calls := 0
	client.Transport = officialPriceRoundTripper(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: http.StatusFound, Header: http.Header{"Location": []string{"https://example.invalid/pricing.md"}}, Body: io.NopCloser(strings.NewReader("redirect"))}, nil
	})
	openAIOfficialPricingClient = &client
	snapshot, err := FetchOpenAIOfficialPricing(context.Background())
	require.Error(t, err)
	assert.Nil(t, snapshot)
	assert.Equal(t, 1, calls)
}

// Explicit opt-in: this tests only the public documentation schema, never an
// account/API credential or a billable model call. It cannot publish prices.
func TestOfficialPricingLiveSourceContract(t *testing.T) {
	if os.Getenv("MYAPI_VERIFY_OFFICIAL_PRICING") != "1" {
		t.Skip("public official source verification not enabled")
	}
	snapshot, err := FetchOpenAIOfficialPricing(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "standard", snapshot.ServiceTier)
	assert.Equal(t, "USD", snapshot.Currency)
	assert.Equal(t, 1_000_000, snapshot.UnitTokens)
	require.NotEmpty(t, snapshot.Models)
	assert.Len(t, snapshot.ContentSHA256, 64)
	for _, model := range snapshot.Models {
		assert.NotEmpty(t, model.Model)
		assert.NotEmpty(t, model.ShortContext.Input)
		assert.NotEmpty(t, model.ShortContext.Output)
	}
	candidate, err := BuildOpenAIPricePublicationCandidate(snapshot, model.TokenBudgetOpenAIChatModel)
	require.NoError(t, err, "the additional strict Chat model must pass the same saved-source publication qualification")
	assert.Equal(t, 272000, candidate.ShortContextMaxInputTokens)
	t.Logf("public source models=%d sha256=%s", len(snapshot.Models), snapshot.ContentSHA256)
}
