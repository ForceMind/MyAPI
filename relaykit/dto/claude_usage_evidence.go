package dto

import kitutil "github.com/ForceMind/MyAPI/relaykit/relayconvert/kitutil"

// Preserve missing/null versus explicitly reported zero at the wire boundary.
// Programmatically constructed compatibility DTOs retain their old behavior.
func (u *ClaudeUsage) UnmarshalJSON(data []byte) error {
	type value ClaudeUsage
	var decoded value
	if err := kitutil.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var fields struct {
		Input         *int `json:"input_tokens"`
		Output        *int `json:"output_tokens"`
		CacheCreation *int `json:"cache_creation_input_tokens"`
	}
	if err := kitutil.Unmarshal(data, &fields); err != nil {
		return err
	}
	*u = ClaudeUsage(decoded)
	u.BillingUsage = nil // Upstream JSON cannot supply trusted internal metadata.
	u.RawUsageObserved = true
	u.InputTokensReported = fields.Input != nil
	u.OutputTokensReported = fields.Output != nil
	for _, count := range []int{u.InputTokens, u.OutputTokens, u.CacheReadInputTokens, u.CacheCreationInputTokens, u.ClaudeCacheCreation5mTokens, u.ClaudeCacheCreation1hTokens} {
		if count < 0 {
			u.InvalidTokenEvidence = true
		}
	}
	if u.CacheCreation != nil {
		five, hour := u.CacheCreation.Ephemeral5mInputTokens, u.CacheCreation.Ephemeral1hInputTokens
		if five < 0 || hour < 0 || five+hour < five || fields.CacheCreation != nil && five+hour != *fields.CacheCreation {
			u.InvalidTokenEvidence = true
		}
	}
	return nil
}
