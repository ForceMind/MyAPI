package dto

import (
	"bytes"
	"encoding/json"
	"math"

	kitutil "github.com/ForceMind/MyAPI/relaykit/relayconvert/kitutil"
)

// Optional wire fields cannot establish a zero count when absent or null.
// Existing programmatically constructed compatibility values are unchanged.
func (u *RealtimeUsage) UnmarshalJSON(data []byte) error {
	type value RealtimeUsage
	var decoded value
	if err := kitutil.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var fields struct {
		Input         *int            `json:"input_tokens"`
		Output        *int            `json:"output_tokens"`
		Total         *int            `json:"total_tokens"`
		InputDetails  json.RawMessage `json:"input_token_details"`
		OutputDetails json.RawMessage `json:"output_token_details"`
	}
	if err := kitutil.Unmarshal(data, &fields); err != nil {
		return err
	}
	*u = RealtimeUsage(decoded)
	u.RawUsageObserved = true
	u.UsageIncomplete = fields.Input == nil || fields.Output == nil || fields.Total == nil
	for _, count := range []int{u.InputTokens, u.OutputTokens, u.TotalTokens} {
		if count < 0 || count > math.MaxInt32 {
			u.UsageIncomplete = true
		}
	}
	if int64(u.InputTokens)+int64(u.OutputTokens) != int64(u.TotalTokens) {
		u.UsageIncomplete = true
	}
	// A complete category sum is necessary for existing text/audio rates. Missing
	// optional zero categories need not be invented or added to the parent total.
	for _, group := range []struct {
		raw    json.RawMessage
		total  int
		values []int
	}{
		{fields.InputDetails, u.InputTokens, []int{u.InputTokenDetails.TextTokens, u.InputTokenDetails.AudioTokens, u.InputTokenDetails.ImageTokens}},
		{fields.OutputDetails, u.OutputTokens, []int{u.OutputTokenDetails.TextTokens, u.OutputTokenDetails.AudioTokens, u.OutputTokenDetails.ImageTokens}},
	} {
		sum := int64(0)
		for _, count := range group.values {
			if count < 0 || count > math.MaxInt32 {
				u.UsageIncomplete = true
			}
			sum += int64(count)
		}
		if sum != int64(group.total) {
			u.UsageIncomplete = true
		}
		if len(group.raw) == 0 {
			continue
		}
		if bytes.Equal(bytes.TrimSpace(group.raw), []byte("null")) {
			u.UsageIncomplete = true
			continue
		}
		var parts map[string]json.RawMessage
		if err := kitutil.Unmarshal(group.raw, &parts); err != nil {
			return err
		}
		for _, name := range []string{"text_tokens", "audio_tokens", "image_tokens", "cached_tokens", "reasoning_tokens"} {
			if raw, exists := parts[name]; exists && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
				u.UsageIncomplete = true
			}
		}
	}
	var input struct {
		Cached *int `json:"cached_tokens"`
	}
	if len(fields.InputDetails) != 0 {
		if err := kitutil.Unmarshal(fields.InputDetails, &input); err != nil {
			return err
		}
	}
	u.CachedTokensReported = input.Cached != nil
	if u.InputTokenDetails.CachedTokens < 0 || u.InputTokenDetails.CachedTokens > u.InputTokens ||
		u.OutputTokenDetails.ReasoningTokens < 0 || u.OutputTokenDetails.ReasoningTokens > u.OutputTokens ||
		u.InputTokenDetails.CachedCreationTokens != 0 || u.InputTokenDetails.CacheWriteTokens != 0 {
		u.UsageIncomplete = true
	}
	if cached := u.InputTokenDetails.CachedTokensDetails; cached != nil {
		sum, present := int64(0), 0
		for _, part := range []struct {
			count  *int
			parent int
		}{
			{cached.TextTokens, u.InputTokenDetails.TextTokens}, {cached.AudioTokens, u.InputTokenDetails.AudioTokens}, {cached.ImageTokens, u.InputTokenDetails.ImageTokens},
		} {
			if part.count == nil {
				continue
			}
			present++
			if *part.count < 0 || *part.count > part.parent {
				u.UsageIncomplete = true
			}
			sum += int64(*part.count)
		}
		if !u.CachedTokensReported || sum > int64(u.InputTokenDetails.CachedTokens) || present == 3 && sum != int64(u.InputTokenDetails.CachedTokens) {
			u.UsageIncomplete = true
		}
	}
	return nil
}
