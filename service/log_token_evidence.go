package service

import (
	"strings"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/relaykit/dto"
)

// Keep statistics observational: use the existing provider evidence and never
// feed these counters back into pricing, pre-consumption, or settlement.
func appendLogTokenEvidence(other map[string]interface{}, original, effective *dto.Usage, reported bool) {
	if !reported || original == nil || effective == nil {
		return
	}
	values := []int{effective.PromptTokens, effective.CompletionTokens}
	if raw := original.BillingUsage; raw != nil {
		if v := raw.OpenAIUsage; v != nil {
			values = append(values, v.InputTokens, v.OutputTokens, v.PromptTokens, v.CompletionTokens)
		}
		if v := raw.ClaudeUsage; v != nil {
			values = append(values, v.InputTokens, v.OutputTokens, v.CacheReadInputTokens, v.CacheCreationInputTokens)
		}
		if v := raw.GeminiUsageMetadata; v != nil {
			values = append(values, v.PromptTokenCount, v.ToolUsePromptTokenCount, v.CandidatesTokenCount, v.ThoughtsTokenCount)
		}
	}
	for _, value := range values {
		if value < 0 || value > common.MaxQuota {
			return
		}
	}
	input, output := int64(effective.PromptTokens), int64(effective.CompletionTokens)
	if raw := original.BillingUsage; raw != nil && raw.OpenAIUsage != nil {
		switch strings.ToLower(strings.TrimSpace(raw.Source)) {
		case dto.BillingUsageSourceOAIResponses:
			input, output = int64(raw.OpenAIUsage.InputTokens), int64(raw.OpenAIUsage.OutputTokens)
		case dto.BillingUsageSourceOAIChat:
			input, output = int64(raw.OpenAIUsage.PromptTokens), int64(raw.OpenAIUsage.CompletionTokens)
		}
	}
	if original.BillingUsage != nil && original.BillingUsage.ClaudeUsage != nil {
		raw := original.BillingUsage.ClaudeUsage
		if raw.InputTokens < 0 || raw.OutputTokens < 0 || raw.CacheReadInputTokens < 0 || raw.CacheCreationInputTokens < 0 {
			return
		}
		input = int64(raw.InputTokens) + int64(raw.CacheReadInputTokens) + int64(raw.CacheCreationInputTokens)
		output = int64(raw.OutputTokens)
	} else if original.BillingUsage != nil && original.BillingUsage.GeminiUsageMetadata != nil {
		raw := original.BillingUsage.GeminiUsageMetadata
		if raw.PromptTokenCount < 0 || raw.ToolUsePromptTokenCount < 0 || raw.CandidatesTokenCount < 0 || raw.ThoughtsTokenCount < 0 {
			return
		}
		input = int64(raw.PromptTokenCount) + int64(raw.ToolUsePromptTokenCount)
		output = int64(raw.CandidatesTokenCount) + int64(raw.ThoughtsTokenCount)
	}
	if input < 0 || output < 0 {
		return
	}
	if input == 0 && output == 0 && original.BillingUsage == nil {
		return
	}
	evidence := &model.LogTokenUsageEvidence{Schema: 1, InputTokens: &input, OutputTokens: &output}
	// A positive compatibility counter establishes a cache read. An omitted
	// value-only counter cannot establish zero; native OpenAI evidence can.
	if effective.PromptTokensDetails.CachedTokens > 0 {
		cached := int64(effective.PromptTokensDetails.CachedTokens)
		if cached <= input {
			evidence.CacheReadTokens = &cached
		}
	}
	if raw := original.BillingUsage; raw != nil {
		var cached *int
		if raw.ChatTextEvidence != nil {
			cached = raw.ChatTextEvidence.CacheRead
		} else if raw.ResponsesTextEvidence != nil {
			cached = raw.ResponsesTextEvidence.CacheRead
		}
		if cached != nil {
			evidence.CacheReadTokens = nil
			if *cached >= 0 && int64(*cached) <= input {
				value := int64(*cached)
				evidence.CacheReadTokens = &value
			}
		}
	}
	other["token_usage_evidence"] = evidence
}
