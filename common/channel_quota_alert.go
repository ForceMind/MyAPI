package common

import (
	"crypto/sha256"
	"encoding/hex"
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

const (
	channelQuotaAlertOccurrenceVersion = "quota-alert-occurrence-v2"
	maxChannelQuotaAlertTimestamp      = int64(^uint64(0) >> 1)
)

// ChannelQuotaAlertOccurrenceInput is the bounded, redacted input required to
// derive an event identity for a future persistent alert pipeline. SubjectRef
// must be the canonical channel:<positive decimal id> form. SourceSnapshotRef
// must be the canonical snapshot:<positive decimal id> form; that snapshot ID
// identifies one immutable normalized observation. Neither accepts names,
// URLs, provider responses, credentials, request bodies, or arbitrary opaque
// identifiers. The future persistence boundary must verify that both IDs
// belong to the same authorized scope before calling this pure helper.
type ChannelQuotaAlertOccurrenceInput struct {
	SubjectRef        string
	SourceSnapshotRef string
	PreviousStatus    string
	CurrentStatus     string
	ObservedAt        int64
	LastDeliveredAt   int64
	SourceTrusted     bool
	HasProviderTotal  bool
}

// ChannelQuotaAlertOccurrenceOutcome is a notifier-neutral event candidate.
// EventKey is a SHA-256 digest over bounded redacted identifiers and the
// occurrence identity, so it does not expose the input references. A blank
// EventKey means no event may be persisted or delivered.
type ChannelQuotaAlertOccurrenceOutcome struct {
	Status       string `json:"status"`
	Kind         string `json:"kind"`
	EventKey     string `json:"event_key"`
	Suppressed   bool   `json:"suppressed"`
	NextEligible int64  `json:"next_eligible_at,omitempty"`
}

// EvaluateChannelQuotaAlertOccurrenceV2 derives a versioned event identity
// without DB, network, notifier, routing, or configuration side effects.
//
// Unlike the legacy EvaluateChannelQuotaAlertTransition helper, EventKey is
// tied to an immutable source snapshot and occurrence kind. A later
// healthy->critical transition or a later reminder therefore cannot be
// permanently suppressed by a historical <subject>:<status> delivery key.
// Invalid, untrusted, disabled, total-less, or unknown inputs fail closed.
func EvaluateChannelQuotaAlertOccurrenceV2(input ChannelQuotaAlertOccurrenceInput, settings ChannelQuotaAlertSettings) ChannelQuotaAlertOccurrenceOutcome {
	if !settings.Enabled || ValidateChannelQuotaAlertSettings(settings) != nil ||
		!validChannelQuotaAlertOccurrenceInput(input) {
		return ChannelQuotaAlertOccurrenceOutcome{}
	}
	if settings.CooldownSeconds <= 0 {
		settings.CooldownSeconds = DefaultChannelQuotaAlertCooldownSeconds
	}
	// A saturated timestamp must never wrap into a negative next_eligible_at.
	// Failing closed is safer than creating an event whose cooldown cannot be
	// represented or persisted consistently by a future delivery pipeline.
	if input.LastDeliveredAt > maxChannelQuotaAlertTimestamp-settings.CooldownSeconds {
		return ChannelQuotaAlertOccurrenceOutcome{}
	}

	outcome := ChannelQuotaAlertOccurrenceOutcome{Status: input.CurrentStatus}
	switch input.CurrentStatus {
	case "healthy":
		if !settings.NotifyOnRecovery || (input.PreviousStatus != "warning" && input.PreviousStatus != "critical") {
			return outcome
		}
		outcome.Kind = "recovery"
	case "warning", "critical":
		if input.PreviousStatus != input.CurrentStatus {
			outcome.Kind = "threshold"
			break
		}
		if input.LastDeliveredAt > 0 && input.ObservedAt-input.LastDeliveredAt < settings.CooldownSeconds {
			outcome.Suppressed = true
			outcome.NextEligible = input.LastDeliveredAt + settings.CooldownSeconds
			return outcome
		}
		outcome.Kind = "reminder"
	default:
		return ChannelQuotaAlertOccurrenceOutcome{}
	}

	outcome.EventKey = channelQuotaAlertOccurrenceKey(input, outcome.Kind)
	return outcome
}

func validChannelQuotaAlertOccurrenceInput(input ChannelQuotaAlertOccurrenceInput) bool {
	return input.SourceTrusted && input.HasProviderTotal && input.ObservedAt > 0 &&
		input.LastDeliveredAt >= 0 && input.LastDeliveredAt <= input.ObservedAt &&
		validChannelQuotaAlertOccurrenceReference(input.SubjectRef, "channel:") &&
		validChannelQuotaAlertOccurrenceReference(input.SourceSnapshotRef, "snapshot:") &&
		validChannelQuotaAlertStatus(input.PreviousStatus) && validChannelQuotaAlertStatus(input.CurrentStatus)
}

func validChannelQuotaAlertOccurrenceReference(value, prefix string) bool {
	identifier := strings.TrimPrefix(value, prefix)
	if identifier == value || identifier == "" || identifier == "0" ||
		(len(identifier) > 1 && identifier[0] == '0') {
		return false
	}
	for index := 0; index < len(identifier); index++ {
		if identifier[index] < '0' || identifier[index] > '9' {
			return false
		}
	}
	parsed, err := strconv.ParseInt(identifier, 10, 64)
	return err == nil && parsed > 0
}

func validChannelQuotaAlertStatus(status string) bool {
	return status == "healthy" || status == "warning" || status == "critical"
}

func channelQuotaAlertOccurrenceKey(input ChannelQuotaAlertOccurrenceInput, kind string) string {
	cycle := "transition"
	if kind == "reminder" {
		cycle = strconv.FormatInt(input.LastDeliveredAt, 10)
	}
	parts := []string{
		channelQuotaAlertOccurrenceVersion,
		input.SubjectRef,
		input.SourceSnapshotRef,
		input.CurrentStatus,
		kind,
		cycle,
	}
	var builder strings.Builder
	for _, part := range parts {
		fmt.Fprintf(&builder, "%d:%s", len(part), part)
	}
	digest := sha256.Sum256([]byte(builder.String()))
	return channelQuotaAlertOccurrenceVersion + ":" + hex.EncodeToString(digest[:])
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
