package openai

import (
	"fmt"

	"github.com/ForceMind/MyAPI/relaykit/dto"
)

// Bound memory by concurrent work, not the number of responses in the session.
const maxRealtimePendingResponses = 128
const maxRealtimeResponseIDBytes = 512

type realtimeResponseLifecycle struct {
	pending                int
	active                 map[string]struct{}
	sawCreated             bool
	audioSent              bool
	automaticAudioPossible bool
	automaticDisabled      bool
	unverified             bool
}

// The handler's state mutex serializes both socket readers and this tracker.
func (s *realtimeResponseLifecycle) observeClient(event *dto.RealtimeEvent) error {
	switch event.Type {
	case dto.RealtimeEventInputAudioBufferAppend:
		if event.Audio != "" {
			s.audioSent = true
			if !s.automaticDisabled {
				s.automaticAudioPossible = true
			}
			if s.automaticAudioPossible && s.pending > 0 {
				s.unverified = true
			}
		}
	case dto.RealtimeEventTypeResponseCreate:
		if s.pending >= maxRealtimePendingResponses {
			s.unverified = true
			return fmt.Errorf("too many unconfirmed realtime response requests")
		}
		s.pending++
		if s.automaticAudioPossible {
			s.unverified = true
		}
	}
	return nil
}

func (s *realtimeResponseLifecycle) observeTarget(event *dto.RealtimeEvent) error {
	switch event.Type {
	case dto.RealtimeEventTypeSessionCreated, dto.RealtimeEventTypeSessionUpdated:
		s.automaticDisabled = event.Session != nil && event.Session.AutomaticResponseDisabled
		if s.audioSent && !s.automaticDisabled {
			s.automaticAudioPossible = true
		}
		if s.automaticAudioPossible && s.pending > 0 {
			s.unverified = true
		}
	case dto.RealtimeEventTypeResponseCreated:
		if event.Response == nil || event.Response.ID == "" || len(event.Response.ID) > maxRealtimeResponseIDBytes ||
			event.Response.Status != "" && event.Response.Status != "in_progress" {
			s.unverified = true
			return fmt.Errorf("realtime response creation lacks a valid identity")
		}
		id := event.Response.ID
		if _, duplicate := s.active[id]; duplicate || len(s.active) >= maxRealtimePendingResponses {
			s.unverified = true
			return fmt.Errorf("realtime response creation conflicts with pending work")
		}
		if s.active == nil {
			s.active = make(map[string]struct{})
		}
		s.active[id] = struct{}{}
		s.sawCreated = true
		if s.pending > 0 {
			if s.automaticAudioPossible {
				// An automatic response is not evidence that a manually dispatched
				// request was accepted. Do not cancel unrelated pending work.
				s.unverified = true
			} else {
				s.pending--
			}
		}
	case dto.RealtimeEventTypeResponseDone:
		if event.Response == nil {
			s.unverified = true
			return fmt.Errorf("realtime response completion lacks a response")
		}
		switch event.Response.Status {
		case "", "completed", "cancelled", "failed", "incomplete":
		default:
			s.unverified = true
			return fmt.Errorf("realtime response completion has a nonterminal status")
		}
		if s.sawCreated {
			if _, exists := s.active[event.Response.ID]; !exists {
				s.unverified = true
				return fmt.Errorf("realtime response completion has no matching pending identity")
			}
			delete(s.active, event.Response.ID)
		}
		// Missing/invalid usage is handled by the existing raw-evidence path.
		// A cancel request or an error event alone never proves zero usage.
	}
	return nil
}

func (s *realtimeResponseLifecycle) needsReview() bool {
	return s.unverified || s.pending > 0 || len(s.active) > 0
}
