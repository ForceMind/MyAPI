package common

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// ChannelQuotaAlertSettingsOptionKey is persisted through the existing,
// root-only /api/option/ settings API. Keeping the three values in one JSON
// option makes threshold updates atomic and avoids transient warning/critical
// inversions when a process reloads settings from the database.
const (
	ChannelQuotaAlertSettingsOptionKey = "ChannelQuotaAlertSettings"
	// Legacy scalar option keys remain readable for one-time migrations.
	ChannelQuotaAlertEnabledOptionKey         = "ChannelQuotaAlertEnabled"
	ChannelQuotaAlertWarningPercentOptionKey  = "ChannelQuotaAlertWarningPercent"
	ChannelQuotaAlertCriticalPercentOptionKey = "ChannelQuotaAlertCriticalPercent"
)

type ChannelQuotaAlertSettings struct {
	Enabled         bool    `json:"enabled"`
	WarningPercent  float64 `json:"warning_percent"`
	CriticalPercent float64 `json:"critical_percent"`
	// CooldownSeconds controls how often a repeated alert for the same
	// channel/status may be emitted by a future notifier. A zero value is
	// accepted for backwards-compatible JSON options and uses the safe default
	// during policy evaluation.
	CooldownSeconds int64 `json:"cooldown_seconds"`
	// NotifyOnRecovery opts in to a single healthy transition after a warning
	// or critical state. It is metadata only until an outbound notifier is
	// explicitly configured; this package never sends network requests.
	NotifyOnRecovery bool `json:"notify_on_recovery"`
}

func DefaultChannelQuotaAlertSettings() ChannelQuotaAlertSettings {
	return ChannelQuotaAlertSettings{
		Enabled:          DefaultChannelQuotaAlertEnabled,
		WarningPercent:   DefaultChannelQuotaAlertWarningPercent,
		CriticalPercent:  DefaultChannelQuotaAlertCriticalPercent,
		CooldownSeconds:  DefaultChannelQuotaAlertCooldownSeconds,
		NotifyOnRecovery: DefaultChannelQuotaAlertNotifyOnRecovery,
	}
}

// ValidateChannelQuotaAlertSettings enforces finite percentage thresholds and
// the ordering needed by the read-only alert derivation. It does not trigger
// notifications, provider calls, routing changes, or channel state changes.
func ValidateChannelQuotaAlertSettings(settings ChannelQuotaAlertSettings) error {
	if !finiteChannelQuotaAlertPercent(settings.WarningPercent) ||
		settings.WarningPercent <= 0 || settings.WarningPercent > 100 {
		return errors.New("channel quota warning_percent must be finite and between 0 and 100")
	}
	if !finiteChannelQuotaAlertPercent(settings.CriticalPercent) ||
		settings.CriticalPercent < 0 || settings.CriticalPercent >= settings.WarningPercent {
		return errors.New("channel quota critical_percent must be finite, non-negative, and lower than warning_percent")
	}
	if settings.CooldownSeconds < 0 || settings.CooldownSeconds > 7*24*60*60 {
		return errors.New("channel quota alert cooldown_seconds must be between 0 and 604800")
	}
	return nil
}

func ParseChannelQuotaAlertSettings(raw string) (ChannelQuotaAlertSettings, error) {
	var settings ChannelQuotaAlertSettings
	if err := DecodeJsonStrict(strings.NewReader(raw), &settings); err != nil {
		return ChannelQuotaAlertSettings{}, fmt.Errorf("invalid channel quota alert settings: %w", err)
	}
	if err := ValidateChannelQuotaAlertSettings(settings); err != nil {
		return ChannelQuotaAlertSettings{}, err
	}
	return settings, nil
}

func MarshalChannelQuotaAlertSettings(settings ChannelQuotaAlertSettings) (string, error) {
	if err := ValidateChannelQuotaAlertSettings(settings); err != nil {
		return "", err
	}
	encoded, err := Marshal(settings)
	if err != nil {
		return "", fmt.Errorf("marshal channel quota alert settings: %w", err)
	}
	return string(encoded), nil
}

// ChannelQuotaAlertEvent is the notifier-neutral result of applying an alert
// policy to two consecutive observations. Keeping this decision pure allows
// the API/UI to preview state and lets a later notifier persist delivery state
// without coupling quota sampling to email/webhook credentials.
type ChannelQuotaAlertEvent struct {
	Status       string `json:"status"`
	Kind         string `json:"kind"`
	DedupKey     string `json:"dedup_key"`
	Suppressed   bool   `json:"suppressed"`
	NextEligible int64  `json:"next_eligible_at,omitempty"`
}

// EvaluateChannelQuotaAlertTransition decides whether a transition is
// eligible for notification. Empty/unknown statuses never emit events.
// repeated warning/critical states are deduplicated by cooldown; recovery is
// opt-in. No notification is sent by this helper.
func EvaluateChannelQuotaAlertTransition(subject, previousStatus, currentStatus string, observedAt, lastNotifiedAt int64, settings ChannelQuotaAlertSettings) ChannelQuotaAlertEvent {
	subject = strings.TrimSpace(subject)
	event := ChannelQuotaAlertEvent{Status: currentStatus}
	if subject != "" {
		event.DedupKey = subject + ":" + currentStatus
	}
	if subject == "" || !settings.Enabled || observedAt <= 0 || (currentStatus != "warning" && currentStatus != "critical" && currentStatus != "healthy") {
		return event
	}
	if settings.CooldownSeconds <= 0 {
		settings.CooldownSeconds = DefaultChannelQuotaAlertCooldownSeconds
	}
	if currentStatus == "healthy" {
		if settings.NotifyOnRecovery && (previousStatus == "warning" || previousStatus == "critical") {
			event.Kind = "recovery"
		} else {
			return event
		}
	} else if previousStatus != currentStatus {
		event.Kind = "threshold"
	} else if lastNotifiedAt <= 0 || observedAt-lastNotifiedAt >= settings.CooldownSeconds {
		event.Kind = "reminder"
	} else {
		event.Suppressed = true
		event.NextEligible = lastNotifiedAt + settings.CooldownSeconds
		return event
	}
	if lastNotifiedAt > 0 && observedAt-lastNotifiedAt < settings.CooldownSeconds && event.Kind != "recovery" && previousStatus == currentStatus {
		event.Suppressed = true
		event.NextEligible = lastNotifiedAt + settings.CooldownSeconds
	}
	return event
}

func finiteChannelQuotaAlertPercent(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

// ValidateChannelQuotaAlertOptionValue validates both the canonical JSON
// option and legacy scalar keys. Scalar keys remain accepted for migrations;
// new callers should persist ChannelQuotaAlertSettings atomically.
func ValidateChannelQuotaAlertOptionValue(key, value string) error {
	switch key {
	case ChannelQuotaAlertSettingsOptionKey:
		_, err := ParseChannelQuotaAlertSettings(value)
		return err
	case ChannelQuotaAlertEnabledOptionKey:
		if _, err := strconv.ParseBool(strings.TrimSpace(value)); err != nil {
			return fmt.Errorf("channel quota alert enabled must be boolean: %w", err)
		}
	case ChannelQuotaAlertWarningPercentOptionKey:
		percent, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil || !finiteChannelQuotaAlertPercent(percent) || percent <= 0 || percent > 100 {
			return errors.New("channel quota warning percent must be finite and between 0 and 100")
		}
		if percent <= ChannelQuotaAlertCriticalPercent {
			return errors.New("channel quota warning percent must be higher than critical percent")
		}
	case ChannelQuotaAlertCriticalPercentOptionKey:
		percent, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil || !finiteChannelQuotaAlertPercent(percent) || percent < 0 || percent > 100 {
			return errors.New("channel quota critical percent must be finite and between 0 and 100")
		}
		if percent >= ChannelQuotaAlertWarningPercent {
			return errors.New("channel quota critical percent must be lower than warning percent")
		}
	}
	return nil
}
