package openai

import (
	"encoding/json"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// This boundary accepts compatible wire shapes without promoting them to
// native fee evidence. original remains the only source of that evidence.
type responsesChatUsage struct {
	original   []byte
	normalized []byte
	cached     json.RawMessage
	present    bool
	stream     bool
}

func normalizeResponsesChatUsage(body []byte, stream bool) ([]byte, responsesChatUsage, error) {
	observed := responsesChatUsage{original: body, stream: stream}
	var outer map[string]json.RawMessage
	if err := common.Unmarshal(body, &outer); err != nil {
		return nil, observed, err
	}
	usagePath := "usage"
	raw := outer["usage"]
	if stream {
		var eventType string
		if value := outer["type"]; len(value) != 0 {
			if err := common.Unmarshal(value, &eventType); err != nil {
				return nil, observed, err
			}
		}
		switch eventType {
		case "response.completed", "response.done", "response.incomplete":
		default:
			return body, observed, nil
		}
		var response map[string]json.RawMessage
		if value := outer["response"]; len(value) != 0 && string(value) != "null" {
			if err := common.Unmarshal(value, &response); err != nil {
				return nil, observed, err
			}
		}
		// An explicitly null canonical usage is not replaced by a root alias.
		if value, exists := response["usage"]; exists {
			raw = value
		}
		usagePath = "response.usage"
	}
	if len(raw) == 0 || string(raw) == "null" {
		return body, observed, nil
	}
	var fields map[string]json.RawMessage
	if err := common.Unmarshal(raw, &fields); err != nil {
		return nil, observed, err
	}
	if fields == nil {
		return body, observed, nil
	}
	observed.present = true
	details, canonical := fields["input_tokens_details"]
	if !canonical {
		details = fields["prompt_tokens_details"]
		if len(details) != 0 {
			var err error
			raw, err = sjson.SetRawBytes(raw, "input_tokens_details", details)
			if err != nil {
				return nil, observed, err
			}
		}
	}
	if len(details) != 0 && string(details) != "null" {
		var counts map[string]json.RawMessage
		if err := common.Unmarshal(details, &counts); err != nil {
			return nil, observed, err
		}
		observed.cached = counts["cached_tokens"]
	}
	// Internal source claims are never accepted from upstream JSON.
	// Once selected, keep only Responses details in the accounting DTO. Leaving
	// a conflicting Chat alias here would let downstream compatibility fallback
	// revive it over a canonical explicit zero.
	for _, key := range []string{"billing_usage", "usage_source", "usage_semantic", "prompt_tokens_details"} {
		var err error
		raw, err = sjson.DeleteBytes(raw, key)
		if err != nil {
			return nil, observed, err
		}
	}
	normalized, err := sjson.SetRawBytes(body, usagePath, raw)
	if err != nil {
		return nil, observed, err
	}
	observed.normalized = normalized
	return normalized, observed, nil
}

// Project only the cache field at this adapter boundary. The shared value DTO
// and other provider adapters keep their existing contracts.
func (observed responsesChatUsage) clientBody(value any, chat bool) ([]byte, error) {
	body, err := common.Marshal(value)
	if err != nil || !chat || !gjson.GetBytes(body, "usage").IsObject() {
		return body, err
	}
	body, err = sjson.DeleteBytes(body, "usage.billing_usage")
	if err != nil {
		return nil, err
	}
	for _, path := range []string{"usage.prompt_tokens_details.cached_tokens", "usage.input_tokens_details.cached_tokens"} {
		if path == "usage.input_tokens_details.cached_tokens" && !gjson.GetBytes(body, "usage.input_tokens_details").IsObject() {
			continue
		}
		if len(observed.cached) == 0 {
			body, err = sjson.DeleteBytes(body, path)
		} else {
			body, err = sjson.SetRawBytes(body, path, observed.cached)
		}
		if err != nil {
			return nil, err
		}
	}
	return body, nil
}

func (observed responsesChatUsage) captureEvidence(usage *dto.Usage) {
	if usage == nil || usage.BillingUsage == nil || !observed.present {
		return
	}
	markResponsesUsageEvidence(usage, observed.normalized, observed.stream)
	// Capture again from original bytes: aliases must not acquire fee proof.
	captureResponsesTextEvidence(usage, observed.original, observed.stream)
	if _, err := common.CanonicalJSONObjectDigest(observed.original); err != nil {
		usage.BillingUsage.Incomplete = true
	}
	if usage.BillingUsage.OpenAIUsage == nil {
		usage.BillingUsage.Incomplete = true
		return
	}
	if details := usage.BillingUsage.OpenAIUsage.InputTokensDetails; details != nil {
		for _, n := range []int{details.CachedTokens, details.CachedCreationTokens, details.CacheWriteTokens, details.TextTokens, details.ImageTokens, details.AudioTokens} {
			if n < 0 || n > common.MaxQuota || n > usage.InputTokens {
				usage.BillingUsage.Incomplete = true
			}
		}
	}
}
