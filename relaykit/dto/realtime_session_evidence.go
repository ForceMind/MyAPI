package dto

import (
	"bytes"
	"encoding/json"

	kitutil "github.com/ForceMind/MyAPI/relaykit/relayconvert/kitutil"
)

// Both beta and GA represent automatic response creation in turn detection.
// Missing settings are unknown, never proof that automatic generation is off.
func (s *RealtimeSession) UnmarshalJSON(data []byte) error {
	type value RealtimeSession
	var decoded value
	if err := kitutil.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var fields struct {
		TurnDetection json.RawMessage `json:"turn_detection"`
		Audio         *struct {
			Input *struct {
				TurnDetection json.RawMessage `json:"turn_detection"`
			} `json:"input"`
		} `json:"audio"`
	}
	if err := kitutil.Unmarshal(data, &fields); err != nil {
		return err
	}
	*s = RealtimeSession(decoded)
	candidates := []json.RawMessage{fields.TurnDetection}
	if fields.Audio != nil && fields.Audio.Input != nil {
		candidates = append(candidates, fields.Audio.Input.TurnDetection)
	}
	seen, disabled := false, true
	for _, raw := range candidates {
		if len(raw) == 0 {
			continue
		}
		seen = true
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			continue
		}
		var detection struct {
			CreateResponse *bool `json:"create_response"`
		}
		if err := kitutil.Unmarshal(raw, &detection); err != nil {
			return err
		}
		if detection.CreateResponse == nil || *detection.CreateResponse {
			disabled = false
		}
	}
	s.AutomaticResponseDisabled = seen && disabled
	return nil
}
