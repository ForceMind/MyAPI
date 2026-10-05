package openai

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/relaykit/dto"
)

// Strict financial evidence is extracted from exact native field names. Go's
// case-insensitive struct matching and compatible-provider fallbacks cannot
// supply it. Unknown usage categories require a future qualification review.
func captureChatTextEvidence(body []byte) *dto.ChatTextEvidence {
	fields, ok := exactChatResponseFields(body)
	if !ok {
		return nil
	}
	var model, tier, object string
	if common.Unmarshal(fields["model"], &model) != nil || common.Unmarshal(fields["object"], &object) != nil {
		return nil
	}
	if value, exists := fields["service_tier"]; exists && common.Unmarshal(value, &tier) != nil {
		return nil
	}
	stream := object == "chat.completion.chunk"
	if object != "chat.completion" && !stream {
		return nil
	}
	if !chatTextChoices(fields["choices"], stream, true) {
		return nil
	}
	var usage map[string]json.RawMessage
	if common.Unmarshal(fields["usage"], &usage) != nil || usage == nil {
		return nil
	}
	for key := range usage {
		switch key {
		case "prompt_tokens", "completion_tokens", "total_tokens", "prompt_tokens_details", "completion_tokens_details":
		default:
			return nil
		}
	}
	var counters struct {
		Input  *int `json:"prompt_tokens"`
		Output *int `json:"completion_tokens"`
		Total  *int `json:"total_tokens"`
	}
	if common.Unmarshal(fields["usage"], &counters) != nil || counters.Input == nil || counters.Output == nil || counters.Total == nil ||
		*counters.Input < 0 || *counters.Input > common.MaxQuota || *counters.Output < 0 || *counters.Output > common.MaxQuota ||
		int64(*counters.Input)+int64(*counters.Output) != int64(*counters.Total) {
		return nil
	}
	evidence := &dto.ChatTextEvidence{Model: model, ServiceTier: tier, PromptTokens: *counters.Input, CompletionTokens: *counters.Output, TotalTokens: *counters.Total, Stream: stream}
	for _, name := range []string{"prompt_tokens_details", "completion_tokens_details"} {
		raw, exists := usage[name]
		if !exists || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			continue
		}
		var details map[string]json.RawMessage
		if common.Unmarshal(raw, &details) != nil || details == nil {
			return nil
		}
		for key, value := range details {
			var count *int
			if common.Unmarshal(value, &count) != nil {
				return nil
			}
			if count != nil && (*count < 0 || *count > common.MaxQuota) {
				return nil
			}
			switch {
			case name == "prompt_tokens_details" && key == "cached_tokens":
				evidence.CacheRead = count
			case name == "prompt_tokens_details" && key == "cache_write_tokens":
				evidence.CacheWrite = count
			case name == "completion_tokens_details" && key == "reasoning_tokens":
				evidence.Reasoning = count
			case key == "audio_tokens" || name == "completion_tokens_details" && (key == "accepted_prediction_tokens" || key == "rejected_prediction_tokens"):
				// Explicit non-text or prediction usage cannot release a text reserve.
				if count == nil || *count != 0 {
					return nil
				}
			default:
				return nil
			}
		}
	}
	read, write := int64(0), int64(0)
	if evidence.CacheRead != nil {
		read = int64(*evidence.CacheRead)
	}
	if evidence.CacheWrite != nil {
		write = int64(*evidence.CacheWrite)
	}
	if read+write > int64(evidence.PromptTokens) || evidence.Reasoning != nil && *evidence.Reasoning > evidence.CompletionTokens {
		return nil
	}
	return evidence
}

func exactChatResponseFields(body []byte) (map[string]json.RawMessage, bool) {
	if _, err := common.CanonicalJSONObjectDigest(body); err != nil {
		return nil, false
	}
	var fields map[string]json.RawMessage
	if common.Unmarshal(body, &fields) != nil || fields == nil {
		return nil, false
	}
	for key := range fields {
		switch strings.ToLower(key) {
		case "model", "service_tier", "object", "usage", "choices", "error":
			if key != strings.ToLower(key) {
				return nil, false
			}
		}
	}
	if value, exists := fields["error"]; exists && !bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
		return nil, false
	}
	return fields, true
}

// Only native plain-text completion/delta shapes are qualified. A usage chunk
// must have an empty choices array; ordinary completion has one index-zero choice.
func chatTextChoices(raw json.RawMessage, stream, usage bool) bool {
	var choices []map[string]json.RawMessage
	if common.Unmarshal(raw, &choices) != nil || choices == nil {
		return false
	}
	if stream && usage {
		return len(choices) == 0
	}
	if len(choices) != 1 {
		return false
	}
	choice := choices[0]
	for key := range choice {
		switch key {
		case "index", "message", "delta", "finish_reason", "logprobs":
		default:
			return false
		}
	}
	var index *int
	if common.Unmarshal(choice["index"], &index) != nil || index == nil || *index != 0 {
		return false
	}
	var finish *string
	if common.Unmarshal(choice["finish_reason"], &finish) != nil || !stream && finish == nil {
		return false
	}
	if finish != nil && *finish != "stop" && *finish != "length" && *finish != "content_filter" {
		return false
	}
	key := "message"
	if stream {
		key = "delta"
	}
	var message map[string]json.RawMessage
	if common.Unmarshal(choice[key], &message) != nil || message == nil {
		return false
	}
	for name, value := range message {
		var text *string
		if name != "role" && name != "content" && name != "refusal" {
			return false
		}
		if common.Unmarshal(value, &text) != nil {
			return false
		}
		if name == "role" && (text == nil || *text != "assistant") {
			return false
		}
	}
	if !stream {
		var role string
		if common.Unmarshal(message["role"], &role) != nil || role != "assistant" {
			return false
		}
	}
	return true
}

type strictChatStreamEvidence struct {
	seenUsage, invalid bool
	tier               string
}

func (s *strictChatStreamEvidence) observe(body []byte, model string) {
	if s.invalid {
		return
	}
	fields, ok := exactChatResponseFields(body)
	var reported, object string
	if !ok || common.Unmarshal(fields["model"], &reported) != nil || reported != model ||
		common.Unmarshal(fields["object"], &object) != nil || object != "chat.completion.chunk" || s.seenUsage {
		s.invalid = true
		return
	}
	if value, exists := fields["service_tier"]; exists {
		var tier string
		if common.Unmarshal(value, &tier) != nil || tier != "" && tier != "default" || s.tier != "" && tier != "" && s.tier != tier {
			s.invalid = true
			return
		}
		if tier != "" {
			s.tier = tier
		}
	}
	usage, hasUsage := fields["usage"]
	hasUsage = hasUsage && !bytes.Equal(bytes.TrimSpace(usage), []byte("null"))
	if hasUsage {
		s.seenUsage = true
		s.invalid = captureChatTextEvidence(body) == nil
	} else {
		s.invalid = !chatTextChoices(fields["choices"], true, false)
	}
}
