package dto

import (
	"bytes"
	"encoding/json"
	"math"

	kitutil "github.com/ForceMind/MyAPI/relaykit/relayconvert/kitutil"
)

// Preserve explicit zero separately from absent/null counts at the wire boundary.
// Existing programmatically constructed DTOs retain their compatibility behavior.
func (u *GeminiUsageMetadata) UnmarshalJSON(data []byte) error {
	type value GeminiUsageMetadata
	var decoded value
	if err := kitutil.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var fields struct {
		Prompt     *int `json:"promptTokenCount"`
		Candidates *int `json:"candidatesTokenCount"`
		Total      *int `json:"totalTokenCount"`
	}
	if err := kitutil.Unmarshal(data, &fields); err != nil {
		return err
	}
	*u = GeminiUsageMetadata(decoded)
	u.BillingUsage = nil
	u.RawUsageObserved = true
	u.PromptTokensReported = fields.Prompt != nil
	u.CandidatesTokensReported = fields.Candidates != nil
	u.TotalTokensReported = fields.Total != nil
	var rawFields map[string]json.RawMessage
	if err := kitutil.Unmarshal(data, &rawFields); err != nil {
		return err
	}
	for _, name := range []string{"toolUsePromptTokenCount", "thoughtsTokenCount", "cachedContentTokenCount"} {
		if raw, exists := rawFields[name]; exists && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			u.InvalidTokenEvidence = true
		}
	}
	// A listed modality with no numeric count is not a reported zero category.
	for _, name := range []string{"promptTokensDetails", "toolUsePromptTokensDetails", "candidatesTokensDetails"} {
		if raw, exists := rawFields[name]; exists && !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			var details []struct {
				Count *int `json:"tokenCount"`
			}
			if err := kitutil.Unmarshal(raw, &details); err != nil {
				return err
			}
			for _, detail := range details {
				if detail.Count == nil {
					u.InvalidTokenEvidence = true
				}
			}
		}
	}
	sum := 0
	for _, count := range []int{u.PromptTokenCount, u.ToolUsePromptTokenCount, u.CandidatesTokenCount, u.ThoughtsTokenCount} {
		if count < 0 || count > math.MaxInt-sum {
			u.InvalidTokenEvidence = true
			break
		}
		sum += count
	}
	if u.TotalTokenCount < 0 || u.CachedContentTokenCount < 0 || u.CachedContentTokenCount > u.PromptTokenCount ||
		u.TotalTokensReported && sum != u.TotalTokenCount {
		u.InvalidTokenEvidence = true
	}
	for _, group := range []struct {
		details []GeminiPromptTokensDetails
		limit   int
	}{
		{u.PromptTokensDetails, u.PromptTokenCount}, {u.ToolUsePromptTokensDetails, u.ToolUsePromptTokenCount}, {u.CandidatesTokensDetails, u.CandidatesTokenCount},
	} {
		total := 0
		for _, detail := range group.details {
			if detail.TokenCount < 0 || detail.TokenCount > math.MaxInt-total {
				u.InvalidTokenEvidence = true
				break
			}
			total += detail.TokenCount
		}
		if total > group.limit {
			u.InvalidTokenEvidence = true
		}
	}
	return nil
}

// Raw empty/all-zero metadata remains evidence to classify, not an absent object.
func HasGeminiUsageEvidence(metadata *GeminiUsageMetadata) bool {
	return metadata != nil && (metadata.RawUsageObserved || HasGeminiUsageMetadataTokens(metadata))
}
