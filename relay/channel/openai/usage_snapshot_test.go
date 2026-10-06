package openai

import (
	"testing"

	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponsesProjectionCannotChangeFrozenCacheModalities(t *testing.T) {
	raw := &dto.Usage{InputTokens: 100, OutputTokens: 10,
		InputTokensDetails: &dto.InputTokenDetails{CachedTokens: 40,
			CachedTokensDetails: dto.NewCachedTokenDetails(25, 10, 5)}}
	projected := responsesUsageForBilling(raw)
	require.NotNil(t, projected.BillingUsage)
	*projected.PromptTokensDetails.CachedTokensDetails.TextTokens = 88
	projected.InputTokensDetails.CachedTokens = 99
	assert.Equal(t, 25, *projected.BillingUsage.OpenAIUsage.InputTokensDetails.CachedTokensDetails.TextTokens)
	assert.Equal(t, 40, projected.BillingUsage.OpenAIUsage.InputTokensDetails.CachedTokens)
	assert.Equal(t, 25, *raw.InputTokensDetails.CachedTokensDetails.TextTokens)
	assert.Equal(t, 40, raw.InputTokensDetails.CachedTokens)
}
