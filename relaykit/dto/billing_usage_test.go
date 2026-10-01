package dto

import (
	"testing"

	kitutil "github.com/ForceMind/MyAPI/relaykit/relayconvert/kitutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewGeminiChatBillingUsageRequiresTokenContent(t *testing.T) {
	require.Nil(t, NewGeminiChatBillingUsage(nil))
	require.Nil(t, NewGeminiChatBillingUsage(&GeminiUsageMetadata{}))

	billingUsage := NewGeminiChatBillingUsage(&GeminiUsageMetadata{PromptTokenCount: 1})
	require.NotNil(t, billingUsage)
	require.NotNil(t, billingUsage.GeminiUsageMetadata)
	assert.Equal(t, BillingUsageSourceGeminiChat, billingUsage.Source)
	assert.Equal(t, BillingUsageSemanticGemini, billingUsage.Semantic)
	assert.False(t, billingUsage.Estimated)
}

func TestNewClaudeMessagesBillingUsageRequiresTokenContent(t *testing.T) {
	require.Nil(t, NewClaudeMessagesBillingUsage(nil))
	require.Nil(t, NewClaudeMessagesBillingUsage(&ClaudeUsage{}))
	require.Nil(t, NewClaudeMessagesBillingUsage(&ClaudeUsage{CacheCreation: &ClaudeCacheCreationUsage{}}))

	billingUsage := NewClaudeMessagesBillingUsage(&ClaudeUsage{InputTokens: 1})
	require.NotNil(t, billingUsage)
	require.NotNil(t, billingUsage.ClaudeUsage)
	assert.Equal(t, BillingUsageSourceClaudeMessages, billingUsage.Source)
	assert.Equal(t, BillingUsageSemanticAnthropic, billingUsage.Semantic)

	cacheOnly := NewClaudeMessagesBillingUsage(&ClaudeUsage{
		CacheCreation: &ClaudeCacheCreationUsage{Ephemeral5mInputTokens: 4},
	})
	require.NotNil(t, cacheOnly)
}

func TestNewOpenAIChatBillingUsageRequiresTokenContent(t *testing.T) {
	require.Nil(t, NewOpenAIChatBillingUsage(nil))
	require.Nil(t, NewOpenAIChatBillingUsage(&Usage{}))

	billingUsage := NewOpenAIChatBillingUsage(&Usage{PromptTokens: 1})
	require.NotNil(t, billingUsage)
	require.NotNil(t, billingUsage.OpenAIUsage)
	assert.Equal(t, BillingUsageSourceOAIChat, billingUsage.Source)
	assert.Equal(t, BillingUsageSemanticOpenAI, billingUsage.Semantic)
	assert.Equal(t, 1, billingUsage.OpenAIUsage.PromptTokens)
}

func TestNewEstimatedGeminiChatBillingUsage(t *testing.T) {
	billingUsage := NewEstimatedGeminiChatBillingUsage(&Usage{
		PromptTokens:     11,
		CompletionTokens: 7,
	})

	require.NotNil(t, billingUsage)
	require.NotNil(t, billingUsage.GeminiUsageMetadata)
	assert.True(t, billingUsage.Estimated)
	assert.Equal(t, 11, billingUsage.GeminiUsageMetadata.PromptTokenCount)
	assert.Equal(t, 7, billingUsage.GeminiUsageMetadata.CandidatesTokenCount)
	assert.Equal(t, 18, billingUsage.GeminiUsageMetadata.TotalTokenCount)
}

func TestResponsesOutputDetailsSurviveBillingSnapshot(t *testing.T) {
	var usage Usage
	require.NoError(t, kitutil.Unmarshal([]byte(`{"input_tokens":100,"output_tokens":10,"total_tokens":110,"output_tokens_details":{"reasoning_tokens":4}}`), &usage))
	require.NotNil(t, usage.OutputTokensDetails)
	assert.Equal(t, 4, usage.OutputTokensDetails.ReasoningTokens)
	billing := NewOpenAIResponsesBillingUsage(&usage)
	require.NotNil(t, billing)
	usage.OutputTokensDetails.ReasoningTokens = 9
	assert.Equal(t, 4, billing.OpenAIUsage.OutputTokensDetails.ReasoningTokens)
	clone := CloneBillingUsage(billing)
	billing.OpenAIUsage.OutputTokensDetails.ReasoningTokens = 7
	assert.Equal(t, 4, clone.OpenAIUsage.OutputTokensDetails.ReasoningTokens)
	assert.Equal(t, 10, clone.OpenAIUsage.OutputTokens)
	data, err := kitutil.Marshal(clone.OpenAIUsage)
	require.NoError(t, err)
	var roundTrip Usage
	require.NoError(t, kitutil.Unmarshal(data, &roundTrip))
	require.NotNil(t, roundTrip.OutputTokensDetails)
	assert.Equal(t, 4, roundTrip.OutputTokensDetails.ReasoningTokens)
}

func TestCacheModalitiesSurviveIndependentBillingSnapshots(t *testing.T) {
	var raw Usage
	require.NoError(t, kitutil.Unmarshal([]byte(`{"input_tokens":100,"output_tokens":10,"input_tokens_details":{"cached_tokens":40,"cached_tokens_details":{"text_tokens":25,"audio_tokens":10,"image_tokens":5}}}`), &raw))
	require.NotNil(t, raw.InputTokensDetails)
	require.NotNil(t, raw.InputTokensDetails.CachedTokensDetails)
	billing := NewOpenAIResponsesBillingUsage(&raw)
	require.NotNil(t, billing)
	*raw.InputTokensDetails.CachedTokensDetails.AudioTokens = 99
	assert.Equal(t, 10, *billing.OpenAIUsage.InputTokensDetails.CachedTokensDetails.AudioTokens)
	clone := CloneBillingUsage(billing)
	*billing.OpenAIUsage.InputTokensDetails.CachedTokensDetails.TextTokens = 88
	assert.Equal(t, 25, *clone.OpenAIUsage.InputTokensDetails.CachedTokensDetails.TextTokens)
	assert.Equal(t, 5, *clone.OpenAIUsage.InputTokensDetails.CachedTokensDetails.ImageTokens)
	data, err := kitutil.Marshal(clone.OpenAIUsage)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"cached_tokens_details"`)
	assert.NotContains(t, string(data), `"audio_tokens":99`)
	var zero InputTokenDetails
	data, err = kitutil.Marshal(zero)
	require.NoError(t, err)
	assert.NotContains(t, string(data), `"cached_tokens_details"`)
	zero.CachedTokensDetails = NewCachedTokenDetails(0, 0, 0)
	data, err = kitutil.Marshal(zero)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"cached_tokens_details":{"text_tokens":0,"audio_tokens":0,"image_tokens":0}`)
	// Chat-style prompt details must also be isolated, not only Responses.
	raw.PromptTokensDetails = CloneInputTokenDetails(*clone.OpenAIUsage.InputTokensDetails)
	chat := NewOpenAIChatBillingUsage(&raw)
	require.NotNil(t, chat)
	*raw.PromptTokensDetails.CachedTokensDetails.ImageTokens = 77
	assert.Equal(t, 5, *chat.OpenAIUsage.PromptTokensDetails.CachedTokensDetails.ImageTokens)
}

func TestBillingUsageJSONUsesProtocolNamedFields(t *testing.T) {
	billingUsage := &BillingUsage{
		OpenAIUsage:         &Usage{PromptTokens: 1, BillingUsage: NewClaudeMessagesBillingUsage(&ClaudeUsage{InputTokens: 9})},
		ClaudeUsage:         &ClaudeUsage{InputTokens: 2, BillingUsage: NewOpenAIChatBillingUsage(&Usage{PromptTokens: 8})},
		GeminiUsageMetadata: &GeminiUsageMetadata{PromptTokenCount: 3, BillingUsage: NewOpenAIChatBillingUsage(&Usage{PromptTokens: 7})},
	}

	data, err := kitutil.Marshal(billingUsage)
	require.NoError(t, err)

	assert.Contains(t, string(data), `"openai_usage"`)
	assert.Contains(t, string(data), `"claude_usage"`)
	assert.Contains(t, string(data), `"gemini_usage_metadata"`)
	assert.NotContains(t, string(data), `"usage":`)
	assert.NotContains(t, string(data), `"usage_metadata"`)

	clone := CloneBillingUsage(billingUsage)
	require.NotNil(t, clone.OpenAIUsage)
	require.NotNil(t, clone.ClaudeUsage)
	require.NotNil(t, clone.GeminiUsageMetadata)
	assert.Nil(t, clone.OpenAIUsage.BillingUsage)
	assert.Nil(t, clone.ClaudeUsage.BillingUsage)
	assert.Nil(t, clone.GeminiUsageMetadata.BillingUsage)
}
