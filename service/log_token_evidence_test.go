package service

import (
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogTokenEvidencePreservesCachePresenceAndReportedZero(t *testing.T) {
	zero := 0
	for _, item := range []struct {
		name          string
		cache         *int
		input, output int
	}{
		{"missing-cache-is-unknown", nil, 20, 5},
		{"explicit-cache-zero", &zero, 20, 5},
		{"explicit-usage-zero", &zero, 0, 0},
	} {
		t.Run(item.name, func(t *testing.T) {
			usage := &dto.Usage{InputTokens: item.input, OutputTokens: item.output}
			usage.BillingUsage = &dto.BillingUsage{Source: dto.BillingUsageSourceOAIResponses, Semantic: dto.BillingUsageSemanticOpenAI,
				OpenAIUsage:           &dto.Usage{InputTokens: item.input, OutputTokens: item.output},
				ResponsesTextEvidence: &dto.ResponsesTextEvidence{CacheRead: item.cache}}
			before := dto.CloneBillingUsage(usage.BillingUsage)
			other := map[string]interface{}{}
			appendUsageBillingPathForLog(other, false, usage)
			require.Equal(t, "reported", other["usage_accuracy"])
			evidence, ok := other["token_usage_evidence"].(*model.LogTokenUsageEvidence)
			require.True(t, ok)
			require.NotNil(t, evidence.InputTokens)
			require.NotNil(t, evidence.OutputTokens)
			assert.EqualValues(t, item.input, *evidence.InputTokens)
			assert.EqualValues(t, item.output, *evidence.OutputTokens)
			if item.cache == nil {
				assert.Nil(t, evidence.CacheReadTokens)
			} else {
				require.NotNil(t, evidence.CacheReadTokens)
				assert.Zero(t, *evidence.CacheReadTokens)
			}
			assert.Equal(t, before, usage.BillingUsage, "statistics do not mutate provider or billing evidence")
		})
	}
}

func TestLogTokenEvidenceNeverTurnsEstimatesOrMissingUsageIntoKnownCounters(t *testing.T) {
	for _, usage := range []*dto.Usage{
		nil,
		{TotalTokens: 20},
		{PromptTokens: 10, CompletionTokens: 5, BillingUsage: &dto.BillingUsage{Source: dto.BillingUsageSourceOAIChat, OpenAIUsage: &dto.Usage{PromptTokens: 10, CompletionTokens: 5}, Incomplete: true}},
		{PromptTokens: 10, CompletionTokens: 5, BillingUsage: &dto.BillingUsage{Source: dto.BillingUsageSourceOAIChat, OpenAIUsage: &dto.Usage{PromptTokens: 10, CompletionTokens: 5}, Estimated: true}},
	} {
		other := map[string]interface{}{}
		appendUsageBillingPathForLog(other, false, usage)
		assert.NotContains(t, other, "token_usage_evidence")
	}
	other := map[string]interface{}{}
	appendUsageBillingPathForLog(other, true, &dto.Usage{PromptTokens: 10, CompletionTokens: 5})
	assert.Equal(t, "estimated", other["usage_accuracy"])
	assert.NotContains(t, other, "token_usage_evidence")
}

func TestLogTokenEvidenceNormalizesNativeCacheAndReasoningOnce(t *testing.T) {
	claude := &dto.ClaudeUsage{InputTokens: 10, OutputTokens: 5, CacheReadInputTokens: 100, CacheCreationInputTokens: 20}
	gemini := &dto.GeminiUsageMetadata{PromptTokenCount: 100, ToolUsePromptTokenCount: 20, CandidatesTokenCount: 10, ThoughtsTokenCount: 5, TotalTokenCount: 135, CachedContentTokenCount: 30}
	for _, item := range []struct {
		name                  string
		billing               *dto.BillingUsage
		input, output, cached int64
	}{
		{"anthropic", dto.NewClaudeMessagesBillingUsage(claude), 130, 5, 100},
		{"gemini", dto.NewGeminiChatBillingUsage(gemini), 120, 15, 30},
	} {
		t.Run(item.name, func(t *testing.T) {
			other := map[string]interface{}{}
			usage := &dto.Usage{BillingUsage: item.billing}
			appendUsageBillingPathForLog(other, false, usage)
			evidence, ok := other["token_usage_evidence"].(*model.LogTokenUsageEvidence)
			require.True(t, ok)
			assert.Equal(t, item.input, *evidence.InputTokens)
			assert.Equal(t, item.output, *evidence.OutputTokens)
			require.NotNil(t, evidence.CacheReadTokens)
			assert.Equal(t, item.cached, *evidence.CacheReadTokens)
		})
	}
}

func TestLogTokenEvidenceKeepsNativeZeroAndRejectsOutOfRangeCounters(t *testing.T) {
	zero := 0
	usage := &dto.Usage{BillingUsage: &dto.BillingUsage{Source: dto.BillingUsageSourceOAIResponses, Semantic: dto.BillingUsageSemanticOpenAI,
		OpenAIUsage: &dto.Usage{InputTokens: 0, OutputTokens: 0, PromptTokens: 99}, ResponsesTextEvidence: &dto.ResponsesTextEvidence{CacheRead: &zero}}}
	other := map[string]interface{}{}
	appendUsageBillingPathForLog(other, false, usage)
	evidence, ok := other["token_usage_evidence"].(*model.LogTokenUsageEvidence)
	require.True(t, ok)
	assert.Zero(t, *evidence.InputTokens, "compatibility counters do not replace a reported native zero")
	outside := int64(common.MaxQuota)
	outside++
	usage.BillingUsage.OpenAIUsage.InputTokens = int(outside)
	other = map[string]interface{}{}
	appendUsageBillingPathForLog(other, false, usage)
	assert.NotContains(t, other, "token_usage_evidence")
}
