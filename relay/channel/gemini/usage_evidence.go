package gemini

import (
	"strings"

	"github.com/ForceMind/MyAPI/relaykit/dto"
)

// A compatibility converter may emit its own final chunk. Only the original
// provider finish reasons and metadata can establish a final billing witness.
type geminiStreamUsageEvidence struct {
	candidates    map[int64]bool
	latest        *dto.GeminiUsageMetadata
	terminalUsage bool
	invalid       bool
}

func (s *geminiStreamUsageEvidence) observe(response *dto.GeminiChatResponse) {
	if s.candidates == nil {
		s.candidates = make(map[int64]bool)
	}
	if len(response.Candidates) == 0 && response.GetUsageMetadata() == nil && response.PromptFeedback == nil {
		s.invalid = true
	}
	frame := make(map[int64]bool)
	for _, candidate := range response.Candidates {
		if candidate.Index < 0 || frame[candidate.Index] {
			s.invalid = true
		}
		frame[candidate.Index] = true
		if s.candidates[candidate.Index] && len(candidate.Content.Parts) > 0 {
			s.invalid = true
		}
		if _, seen := s.candidates[candidate.Index]; !seen {
			s.candidates[candidate.Index] = false
			s.terminalUsage = false // Earlier totals cannot prove a newly discovered candidate.
		}
		if candidate.FinishReason != nil {
			reason := strings.TrimSpace(*candidate.FinishReason)
			if reason != "" && reason != "FINISH_REASON_UNSPECIFIED" {
				s.candidates[candidate.Index] = true
			}
		}
	}
	terminal := len(s.candidates) > 0
	for _, finished := range s.candidates {
		terminal = terminal && finished
	}
	if len(s.candidates) == 0 && response.PromptFeedback != nil && response.PromptFeedback.BlockReason != nil && strings.TrimSpace(*response.PromptFeedback.BlockReason) != "" {
		terminal = true
	}
	if metadata := response.GetUsageMetadata(); dto.HasGeminiUsageEvidence(metadata) {
		if prior := s.latest; prior != nil && (metadata.PromptTokenCount < prior.PromptTokenCount || metadata.ToolUsePromptTokenCount < prior.ToolUsePromptTokenCount ||
			metadata.CandidatesTokenCount < prior.CandidatesTokenCount || metadata.ThoughtsTokenCount < prior.ThoughtsTokenCount ||
			metadata.TotalTokenCount < prior.TotalTokenCount || metadata.CachedContentTokenCount < prior.CachedContentTokenCount) {
			s.invalid = true
		}
		copy := *metadata
		s.latest = &copy
		s.terminalUsage = terminal
	}
}

func (s *geminiStreamUsageEvidence) incomplete() bool { return s.invalid || !s.terminalUsage }
