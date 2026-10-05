package openai

import (
	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relaykit/dto"
)

// A present Chat usage object, including an explicit all-zero one, is upstream
// evidence. Missing counters remain partial; they are never replaced with a
// local estimate. Ignore any upstream-supplied internal billing envelope.
func captureChatUsageEvidence(usage *dto.Usage, body []byte) bool {
	var envelope struct {
		Usage *struct {
			Input  *int `json:"prompt_tokens"`
			Output *int `json:"completion_tokens"`
			Total  *int `json:"total_tokens"`
		} `json:"usage"`
	}
	if usage == nil || common.Unmarshal(body, &envelope) != nil || envelope.Usage == nil {
		return false
	}
	evidence := envelope.Usage
	complete := evidence.Input != nil && evidence.Output != nil
	if complete {
		complete = *evidence.Input >= 0 && *evidence.Input <= common.MaxQuota &&
			*evidence.Output >= 0 && *evidence.Output <= common.MaxQuota &&
			int64(*evidence.Input)+int64(*evidence.Output) <= int64(common.MaxQuota)
		if complete && evidence.Total != nil {
			complete = complete && int64(*evidence.Total) == int64(*evidence.Input)+int64(*evidence.Output)
		}
	}
	usage.BillingUsage = dto.CloneBillingUsage(&dto.BillingUsage{
		ChatTextEvidence: captureChatTextEvidence(body),
		Source:           dto.BillingUsageSourceOAIChat, Semantic: dto.BillingUsageSemanticOpenAI,
		Incomplete: !complete, OpenAIUsage: usage,
	})
	return true
}

// Preserve JSON field presence at the native Responses boundary. The shared
// Usage counters are values for compatibility; decoding a missing/null field
// into zero is not evidence of an actual zero-token response.
func markResponsesUsageEvidence(usage *dto.Usage, body []byte, stream bool) {
	type counters struct {
		Input  *int `json:"input_tokens"`
		Output *int `json:"output_tokens"`
		Total  *int `json:"total_tokens"`
	}
	var envelope struct {
		Usage    *counters `json:"usage"`
		Response *struct {
			Usage *counters `json:"usage"`
		} `json:"response"`
	}
	if usage == nil || usage.BillingUsage == nil {
		return
	}
	usage.BillingUsage.Incomplete = true
	captureResponsesTextEvidence(usage, body, stream)
	if err := common.Unmarshal(body, &envelope); err != nil {
		return
	}
	evidence := envelope.Usage
	if stream {
		if envelope.Response == nil {
			return
		}
		evidence = envelope.Response.Usage
	}
	if evidence == nil || evidence.Input == nil || evidence.Output == nil {
		return
	}
	if *evidence.Input < 0 || *evidence.Input > common.MaxQuota || *evidence.Output < 0 || *evidence.Output > common.MaxQuota {
		return
	}
	if evidence.Total != nil && int64(*evidence.Total) != int64(*evidence.Input)+int64(*evidence.Output) {
		return
	}
	usage.BillingUsage.Incomplete = false
}

// Fee qualification uses the native provider boundary, not client-supplied
// billing fields or zero defaults in compatibility counters. Ambiguous JSON
// leaves fee evidence absent without changing the existing Token-only contract.
func captureResponsesTextEvidence(usage *dto.Usage, body []byte, stream bool) {
	usage.BillingUsage.ResponsesTextEvidence = nil
	if _, err := common.CanonicalJSONObjectDigest(body); err != nil {
		return
	}
	type response struct {
		Model       string `json:"model"`
		ServiceTier string `json:"service_tier"`
		Usage       *struct {
			Details *struct {
				CacheRead  *int `json:"cached_tokens"`
				CacheWrite *int `json:"cache_write_tokens"`
			} `json:"input_tokens_details"`
		} `json:"usage"`
	}
	var outer struct {
		response
		Response *response `json:"response"`
	}
	if common.Unmarshal(body, &outer) != nil {
		return
	}
	evidence := &outer.response
	if stream {
		evidence = outer.Response
	}
	if evidence == nil || evidence.Usage == nil || evidence.Usage.Details == nil {
		return
	}
	usage.BillingUsage.ResponsesTextEvidence = &dto.ResponsesTextEvidence{
		Model: evidence.Model, ServiceTier: evidence.ServiceTier,
		CacheRead: evidence.Usage.Details.CacheRead, CacheWrite: evidence.Usage.Details.CacheWrite,
	}
}

// responsesUsageForBilling keeps the provider's original counters separate
// from the Chat-style projection used by settlement. Presence is meaningful:
// an explicitly returned zero usage must not be replaced with an estimate.
func responsesUsageForBilling(raw *dto.Usage) dto.Usage {
	billing := dto.CloneBillingUsage(&dto.BillingUsage{
		Source:      dto.BillingUsageSourceOAIResponses,
		Semantic:    dto.BillingUsageSemanticOpenAI,
		OpenAIUsage: raw,
	})
	usage := *dto.CloneBillingUsage(billing).OpenAIUsage
	usage.PromptTokens = usage.InputTokens
	usage.CompletionTokens = usage.OutputTokens
	if usage.InputTokensDetails != nil {
		usage.PromptTokensDetails = dto.CloneInputTokenDetails(*usage.InputTokensDetails)
	}
	if usage.OutputTokensDetails != nil {
		usage.CompletionTokenDetails = *usage.OutputTokensDetails
	}
	usage.BillingUsage = billing
	usage.UsageSource = billing.Source
	usage.UsageSemantic = billing.Semantic
	return usage
}

func applyUsagePostProcessing(info *relaycommon.RelayInfo, usage *dto.Usage, responseBody []byte) {
	if info == nil || usage == nil {
		return
	}

	switch info.ChannelType {
	case constant.ChannelTypeDeepSeek:
		if usage.PromptTokensDetails.CachedTokens == 0 && usage.PromptCacheHitTokens != 0 {
			usage.PromptTokensDetails.CachedTokens = usage.PromptCacheHitTokens
		}
	case constant.ChannelTypeZhipu_v4:
		// 智普的cached_tokens在标准位置: usage.prompt_tokens_details.cached_tokens
		if usage.PromptTokensDetails.CachedTokens == 0 {
			if usage.InputTokensDetails != nil && usage.InputTokensDetails.CachedTokens > 0 {
				usage.PromptTokensDetails.CachedTokens = usage.InputTokensDetails.CachedTokens
			} else if cachedTokens, ok := extractCachedTokensFromBody(responseBody); ok {
				usage.PromptTokensDetails.CachedTokens = cachedTokens
			} else if usage.PromptCacheHitTokens > 0 {
				usage.PromptTokensDetails.CachedTokens = usage.PromptCacheHitTokens
			}
		}
	case constant.ChannelTypeMoonshot:
		// Moonshot的cached_tokens在非标准位置: choices[].usage.cached_tokens
		if usage.PromptTokensDetails.CachedTokens == 0 {
			if usage.InputTokensDetails != nil && usage.InputTokensDetails.CachedTokens > 0 {
				usage.PromptTokensDetails.CachedTokens = usage.InputTokensDetails.CachedTokens
			} else if cachedTokens, ok := extractMoonshotCachedTokensFromBody(responseBody); ok {
				usage.PromptTokensDetails.CachedTokens = cachedTokens
			} else if cachedTokens, ok := extractCachedTokensFromBody(responseBody); ok {
				usage.PromptTokensDetails.CachedTokens = cachedTokens
			} else if usage.PromptCacheHitTokens > 0 {
				usage.PromptTokensDetails.CachedTokens = usage.PromptCacheHitTokens
			}
		}
	case constant.ChannelTypeOpenAI:
		if usage.PromptTokensDetails.CachedTokens == 0 {
			if cachedTokens, ok := extractLlamaCachedTokensFromBody(responseBody); ok {
				usage.PromptTokensDetails.CachedTokens = cachedTokens
			}
		}
	}
}

func extractCachedTokensFromBody(body []byte) (int, bool) {
	if len(body) == 0 {
		return 0, false
	}

	var payload struct {
		Usage struct {
			PromptTokensDetails struct {
				CachedTokens *int `json:"cached_tokens"`
			} `json:"prompt_tokens_details"`
			CachedTokens         *int `json:"cached_tokens"`
			PromptCacheHitTokens *int `json:"prompt_cache_hit_tokens"`
		} `json:"usage"`
	}

	if err := common.Unmarshal(body, &payload); err != nil {
		return 0, false
	}

	if payload.Usage.PromptTokensDetails.CachedTokens != nil {
		return *payload.Usage.PromptTokensDetails.CachedTokens, true
	}
	if payload.Usage.CachedTokens != nil {
		return *payload.Usage.CachedTokens, true
	}
	if payload.Usage.PromptCacheHitTokens != nil {
		return *payload.Usage.PromptCacheHitTokens, true
	}
	return 0, false
}

// extractMoonshotCachedTokensFromBody 从Moonshot的非标准位置提取cached_tokens
// Moonshot的流式响应格式: {"choices":[{"usage":{"cached_tokens":111}}]}
func extractMoonshotCachedTokensFromBody(body []byte) (int, bool) {
	if len(body) == 0 {
		return 0, false
	}

	var payload struct {
		Choices []struct {
			Usage struct {
				CachedTokens *int `json:"cached_tokens"`
			} `json:"usage"`
		} `json:"choices"`
	}

	if err := common.Unmarshal(body, &payload); err != nil {
		return 0, false
	}

	// 遍历choices查找cached_tokens
	for _, choice := range payload.Choices {
		if choice.Usage.CachedTokens != nil && *choice.Usage.CachedTokens > 0 {
			return *choice.Usage.CachedTokens, true
		}
	}

	return 0, false
}

// extractLlamaCachedTokensFromBody 从llama.cpp的非标准位置提取cache_n
func extractLlamaCachedTokensFromBody(body []byte) (int, bool) {
	if len(body) == 0 {
		return 0, false
	}

	var payload struct {
		Timings struct {
			CachedTokens *int `json:"cache_n"`
		} `json:"timings"`
	}

	if err := common.Unmarshal(body, &payload); err != nil {
		return 0, false
	}

	if payload.Timings.CachedTokens == nil {
		return 0, false
	}
	return *payload.Timings.CachedTokens, true
}
