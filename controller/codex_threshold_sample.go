package controller

import (
	"encoding/json"
	"math"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
)

// A complete sample is new evidence, not an upgrade of old value-only DTOs.
// Both window slots must be explicit (null proves absence), and every present
// window must carry its reported percentage, duration and reset boundary.
func qualifyCodexThresholdSnapshots(snapshots []model.ChannelQuotaSnapshot, status int, body []byte, nativeSource bool) {
	if !nativeSource || status < 200 || status >= 300 || len(snapshots) == 0 {
		return
	}
	if _, err := common.CanonicalJSONObjectDigest(body); err != nil {
		return
	}
	var outer struct {
		RateLimit map[string]json.RawMessage `json:"rate_limit"`
	}
	if common.Unmarshal(body, &outer) != nil || outer.RateLimit == nil {
		return
	}
	for key := range outer.RateLimit {
		switch key {
		case "allowed", "limit_reached", "primary_window", "secondary_window", "plan_type":
		default:
			return
		}
	}
	var flags struct {
		Allowed *bool `json:"allowed"`
		Reached *bool `json:"limit_reached"`
	}
	encoded, err := common.Marshal(outer.RateLimit)
	if err != nil || common.Unmarshal(encoded, &flags) != nil || flags.Allowed == nil || flags.Reached == nil {
		return
	}
	count := 0
	hasExhaustedWindow := false
	for _, key := range []string{"primary_window", "secondary_window"} {
		raw, exists := outer.RateLimit[key]
		if !exists {
			return
		}
		var window *struct {
			Used    *float64 `json:"used_percent"`
			Seconds *int64   `json:"limit_window_seconds"`
			Reset   *int64   `json:"reset_at"`
		}
		if common.Unmarshal(raw, &window) != nil {
			return
		}
		if window == nil {
			continue
		}
		if window.Used == nil || math.IsNaN(*window.Used) || math.IsInf(*window.Used, 0) || *window.Used < 0 || *window.Used > 100 || window.Seconds == nil || *window.Seconds <= 0 || window.Reset == nil || *window.Reset <= 0 {
			return
		}
		if *window.Used == 100 {
			hasExhaustedWindow = true
		}
		count++
	}
	if count == 0 || count != len(snapshots) {
		return
	}
	for _, row := range snapshots {
		if row.Status != "success" {
			return
		}
	}
	for index := range snapshots {
		// Keep the existing healthy/admission contract unchanged. An event
		// observation can also prove exhaustion, but contradictory flags cannot
		// manufacture recovery or an exhausted account window.
		snapshots[index].CodexThresholdQualified = *flags.Allowed && !*flags.Reached
		snapshots[index].CodexObservationQualified = (*flags.Allowed && !*flags.Reached && !hasExhaustedWindow) || (!*flags.Allowed && *flags.Reached && hasExhaustedWindow)
	}
}
