package common

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelQuotaAlertSettingsJSONRoundTrip(t *testing.T) {
	settings := ChannelQuotaAlertSettings{Enabled: true, WarningPercent: 25.5, CriticalPercent: 5}
	raw, err := MarshalChannelQuotaAlertSettings(settings)
	if err != nil {
		t.Fatalf("marshal settings: %v", err)
	}
	parsed, err := ParseChannelQuotaAlertSettings(raw)
	if err != nil {
		t.Fatalf("parse settings: %v", err)
	}
	if parsed != settings {
		t.Fatalf("round-trip mismatch: got %#v want %#v", parsed, settings)
	}
}

func TestValidateChannelQuotaAlertSettingsRejectsUnknownOrInvertedValues(t *testing.T) {
	if _, err := ParseChannelQuotaAlertSettings(`{"enabled":true,"warning_percent":20,"critical_percent":10,"notify_url":"https://example.invalid"}`); err == nil {
		t.Fatal("unknown notification fields must be rejected")
	}
	if _, err := ParseChannelQuotaAlertSettings(`{"enabled":true,"warning_percent":10,"critical_percent":10}`); err == nil {
		t.Fatal("critical threshold must be lower than warning threshold")
	}
	if err := ValidateChannelQuotaAlertOptionValue("ChannelQuotaAlertEnabled", "not-a-bool"); err == nil {
		t.Fatal("invalid enabled value must be rejected")
	}
	if err := ValidateChannelQuotaAlertOptionValue(ChannelQuotaAlertSettingsOptionKey, `{"enabled":true,"warning_percent":20,"critical_percent":10,"cooldown_seconds":604801}`); err == nil {
		t.Fatal("cooldown must be bounded")
	}
}

func TestEvaluateChannelQuotaAlertTransitionDeduplicatesRepeatedStatus(t *testing.T) {
	settings := ChannelQuotaAlertSettings{Enabled: true, WarningPercent: 20, CriticalPercent: 10, CooldownSeconds: 3600}
	first := EvaluateChannelQuotaAlertTransition("channel-7", "healthy", "warning", 1000, 0, settings)
	if first.Kind != "threshold" || first.Suppressed || first.DedupKey != "channel-7:warning" {
		t.Fatalf("unexpected first event: %#v", first)
	}
	repeated := EvaluateChannelQuotaAlertTransition("channel-7", "warning", "warning", 1200, 1000, settings)
	if !repeated.Suppressed || repeated.Kind != "" || repeated.NextEligible != 4600 {
		t.Fatalf("repeated event should be suppressed: %#v", repeated)
	}
	reminder := EvaluateChannelQuotaAlertTransition("channel-7", "warning", "warning", 4600, 1000, settings)
	if reminder.Kind != "reminder" || reminder.Suppressed {
		t.Fatalf("cooldown reminder should be eligible: %#v", reminder)
	}
}

func TestEvaluateChannelQuotaAlertTransitionRecoveryIsOptIn(t *testing.T) {
	settings := ChannelQuotaAlertSettings{Enabled: true, WarningPercent: 20, CriticalPercent: 10, CooldownSeconds: 60}
	withoutRecovery := EvaluateChannelQuotaAlertTransition("channel-8", "critical", "healthy", 1000, 900, settings)
	if withoutRecovery.Kind != "" || withoutRecovery.Suppressed {
		t.Fatalf("recovery should be disabled by default: %#v", withoutRecovery)
	}
	settings.NotifyOnRecovery = true
	withRecovery := EvaluateChannelQuotaAlertTransition("channel-8", "critical", "healthy", 1000, 900, settings)
	if withRecovery.Kind != "recovery" || withRecovery.DedupKey != "channel-8:healthy" {
		t.Fatalf("recovery event should be emitted when enabled: %#v", withRecovery)
	}
}

func TestEvaluateChannelQuotaAlertTransitionRequiresSubject(t *testing.T) {
	settings := ChannelQuotaAlertSettings{Enabled: true, WarningPercent: 20, CriticalPercent: 10}
	event := EvaluateChannelQuotaAlertTransition("  ", "healthy", "critical", 1000, 0, settings)
	if event.Kind != "" || event.DedupKey != "" {
		t.Fatalf("blank subject must not produce an event: %#v", event)
	}
}

func TestEvaluateChannelQuotaAlertOccurrenceV2SeparatesOccurrences(t *testing.T) {
	settings := ChannelQuotaAlertSettings{
		Enabled:          true,
		WarningPercent:   20,
		CriticalPercent:  10,
		CooldownSeconds:  60,
		NotifyOnRecovery: true,
	}
	base := ChannelQuotaAlertOccurrenceInput{
		SubjectRef:        "channel:7",
		SourceSnapshotRef: "snapshot:100",
		PreviousStatus:    "healthy",
		CurrentStatus:     "critical",
		ObservedAt:        1000,
		SourceTrusted:     true,
		HasProviderTotal:  true,
	}

	critical := EvaluateChannelQuotaAlertOccurrenceV2(base, settings)
	replayed := EvaluateChannelQuotaAlertOccurrenceV2(base, settings)
	recovery := EvaluateChannelQuotaAlertOccurrenceV2(ChannelQuotaAlertOccurrenceInput{
		SubjectRef:        "channel:7",
		SourceSnapshotRef: "snapshot:101",
		PreviousStatus:    "critical",
		CurrentStatus:     "healthy",
		ObservedAt:        1060,
		LastDeliveredAt:   1000,
		SourceTrusted:     true,
		HasProviderTotal:  true,
	}, settings)
	secondCritical := EvaluateChannelQuotaAlertOccurrenceV2(ChannelQuotaAlertOccurrenceInput{
		SubjectRef:        "channel:7",
		SourceSnapshotRef: "snapshot:102",
		PreviousStatus:    "healthy",
		CurrentStatus:     "critical",
		ObservedAt:        1120,
		LastDeliveredAt:   1000,
		SourceTrusted:     true,
		HasProviderTotal:  true,
	}, settings)

	require.Equal(t, "threshold", critical.Kind)
	require.NotEmpty(t, critical.EventKey)
	assert.Equal(t, critical.EventKey, replayed.EventKey, "same trusted source replay must converge")
	require.Equal(t, "recovery", recovery.Kind)
	require.Equal(t, "threshold", secondCritical.Kind)
	assert.NotEqual(t, critical.EventKey, recovery.EventKey)
	assert.NotEqual(t, critical.EventKey, secondCritical.EventKey)
	assert.True(t, strings.HasPrefix(critical.EventKey, channelQuotaAlertOccurrenceVersion+":"))
	assert.NotContains(t, critical.EventKey, "channel:7")
	assert.NotContains(t, critical.EventKey, "snapshot:100")
}

func TestEvaluateChannelQuotaAlertOccurrenceV2UsesDistinctReminderCyclesAndSuppressesCooldown(t *testing.T) {
	settings := ChannelQuotaAlertSettings{Enabled: true, WarningPercent: 20, CriticalPercent: 10, CooldownSeconds: 60}
	first := ChannelQuotaAlertOccurrenceInput{
		SubjectRef:        "channel:8",
		SourceSnapshotRef: "snapshot:200",
		PreviousStatus:    "warning",
		CurrentStatus:     "warning",
		ObservedAt:        2000,
		LastDeliveredAt:   1900,
		SourceTrusted:     true,
		HasProviderTotal:  true,
	}
	secondCycle := first
	secondCycle.LastDeliveredAt = 1960
	secondCycle.ObservedAt = 2020
	secondSource := first
	secondSource.SourceSnapshotRef = "snapshot:201"

	firstOutcome := EvaluateChannelQuotaAlertOccurrenceV2(first, settings)
	secondCycleOutcome := EvaluateChannelQuotaAlertOccurrenceV2(secondCycle, settings)
	secondSourceOutcome := EvaluateChannelQuotaAlertOccurrenceV2(secondSource, settings)
	suppressed := EvaluateChannelQuotaAlertOccurrenceV2(ChannelQuotaAlertOccurrenceInput{
		SubjectRef:        "channel:8",
		SourceSnapshotRef: "snapshot:202",
		PreviousStatus:    "warning",
		CurrentStatus:     "warning",
		ObservedAt:        1959,
		LastDeliveredAt:   1900,
		SourceTrusted:     true,
		HasProviderTotal:  true,
	}, settings)

	require.Equal(t, "reminder", firstOutcome.Kind)
	require.Equal(t, "reminder", secondCycleOutcome.Kind)
	require.Equal(t, "reminder", secondSourceOutcome.Kind)
	assert.NotEqual(t, firstOutcome.EventKey, secondCycleOutcome.EventKey)
	assert.NotEqual(t, firstOutcome.EventKey, secondSourceOutcome.EventKey)
	assert.True(t, suppressed.Suppressed)
	assert.Empty(t, suppressed.Kind)
	assert.Empty(t, suppressed.EventKey)
	assert.Equal(t, int64(1960), suppressed.NextEligible)
}

func TestEvaluateChannelQuotaAlertOccurrenceV2FailsClosedForUnsafeInputs(t *testing.T) {
	settings := ChannelQuotaAlertSettings{Enabled: true, WarningPercent: 20, CriticalPercent: 10, CooldownSeconds: 60}
	valid := ChannelQuotaAlertOccurrenceInput{
		SubjectRef:        "channel:9",
		SourceSnapshotRef: "snapshot:300",
		PreviousStatus:    "healthy",
		CurrentStatus:     "critical",
		ObservedAt:        3000,
		SourceTrusted:     true,
		HasProviderTotal:  true,
	}
	for _, test := range []struct {
		name     string
		input    ChannelQuotaAlertOccurrenceInput
		settings ChannelQuotaAlertSettings
	}{
		{name: "disabled", input: valid, settings: ChannelQuotaAlertSettings{WarningPercent: 20, CriticalPercent: 10, CooldownSeconds: 60}},
		{name: "missing provider total", input: func() ChannelQuotaAlertOccurrenceInput { value := valid; value.HasProviderTotal = false; return value }(), settings: settings},
		{name: "untrusted source", input: func() ChannelQuotaAlertOccurrenceInput { value := valid; value.SourceTrusted = false; return value }(), settings: settings},
		{name: "unknown status", input: func() ChannelQuotaAlertOccurrenceInput { value := valid; value.CurrentStatus = "unknown"; return value }(), settings: settings},
		{name: "raw URL reference", input: func() ChannelQuotaAlertOccurrenceInput {
			value := valid
			value.SubjectRef = "channel:https://example.invalid/token=secret"
			return value
		}(), settings: settings},
		{name: "known API key reference", input: func() ChannelQuotaAlertOccurrenceInput {
			value := valid
			value.SubjectRef = "sk-proj-secret"
			return value
		}(), settings: settings},
		{name: "arbitrary opaque source reference", input: func() ChannelQuotaAlertOccurrenceInput {
			value := valid
			value.SourceSnapshotRef = "snapshot:key=secret"
			return value
		}(), settings: settings},
		{name: "zero identifier", input: func() ChannelQuotaAlertOccurrenceInput { value := valid; value.SubjectRef = "channel:0"; return value }(), settings: settings},
		{name: "noncanonical identifier", input: func() ChannelQuotaAlertOccurrenceInput {
			value := valid
			value.SourceSnapshotRef = "snapshot:0300"
			return value
		}(), settings: settings},
		{name: "identifier overflow", input: func() ChannelQuotaAlertOccurrenceInput {
			value := valid
			value.SubjectRef = "channel:9223372036854775808"
			return value
		}(), settings: settings},
		{name: "future delivery", input: func() ChannelQuotaAlertOccurrenceInput {
			value := valid
			value.LastDeliveredAt = value.ObservedAt + 1
			return value
		}(), settings: settings},
		{name: "next eligible timestamp overflow", input: func() ChannelQuotaAlertOccurrenceInput {
			value := valid
			value.PreviousStatus = "warning"
			value.CurrentStatus = "warning"
			value.ObservedAt = maxChannelQuotaAlertTimestamp
			value.LastDeliveredAt = maxChannelQuotaAlertTimestamp - 30
			return value
		}(), settings: settings},
	} {
		t.Run(test.name, func(t *testing.T) {
			outcome := EvaluateChannelQuotaAlertOccurrenceV2(test.input, test.settings)
			assert.Equal(t, ChannelQuotaAlertOccurrenceOutcome{}, outcome)
			assert.NotContains(t, outcome.EventKey, "secret")
			assert.NotContains(t, outcome.EventKey, "https")
		})
	}
}

func TestInitChannelQuotaAlertSettingsDefaultsToDisabled(t *testing.T) {
	originalEnabled := ChannelQuotaAlertEnabled
	originalWarning := ChannelQuotaAlertWarningPercent
	originalCritical := ChannelQuotaAlertCriticalPercent
	originalCooldown := ChannelQuotaAlertCooldownSeconds
	originalRecovery := ChannelQuotaAlertNotifyOnRecovery
	defer func() {
		ChannelQuotaAlertEnabled = originalEnabled
		ChannelQuotaAlertWarningPercent = originalWarning
		ChannelQuotaAlertCriticalPercent = originalCritical
		ChannelQuotaAlertCooldownSeconds = originalCooldown
		ChannelQuotaAlertNotifyOnRecovery = originalRecovery
	}()
	t.Setenv("CHANNEL_QUOTA_ALERT_ENABLED", "")
	t.Setenv("CHANNEL_QUOTA_ALERT_WARNING_PERCENT", "")
	t.Setenv("CHANNEL_QUOTA_ALERT_CRITICAL_PERCENT", "")
	t.Setenv("CHANNEL_QUOTA_ALERT_COOLDOWN_SECONDS", "")
	t.Setenv("CHANNEL_QUOTA_ALERT_NOTIFY_ON_RECOVERY", "")
	initChannelQuotaAlertSettings()
	if ChannelQuotaAlertEnabled {
		t.Fatal("quota alerts must be disabled by default")
	}
	if ChannelQuotaAlertWarningPercent != DefaultChannelQuotaAlertWarningPercent || ChannelQuotaAlertCriticalPercent != DefaultChannelQuotaAlertCriticalPercent {
		t.Fatalf("unexpected defaults: warning=%v critical=%v", ChannelQuotaAlertWarningPercent, ChannelQuotaAlertCriticalPercent)
	}
	if ChannelQuotaAlertCooldownSeconds != DefaultChannelQuotaAlertCooldownSeconds || ChannelQuotaAlertNotifyOnRecovery {
		t.Fatalf("unexpected default policy: cooldown=%v recovery=%v", ChannelQuotaAlertCooldownSeconds, ChannelQuotaAlertNotifyOnRecovery)
	}
}

func TestInitChannelQuotaAlertSettingsAcceptsValidThresholds(t *testing.T) {
	originalEnabled := ChannelQuotaAlertEnabled
	originalWarning := ChannelQuotaAlertWarningPercent
	originalCritical := ChannelQuotaAlertCriticalPercent
	originalCooldown := ChannelQuotaAlertCooldownSeconds
	originalRecovery := ChannelQuotaAlertNotifyOnRecovery
	defer func() {
		ChannelQuotaAlertEnabled = originalEnabled
		ChannelQuotaAlertWarningPercent = originalWarning
		ChannelQuotaAlertCriticalPercent = originalCritical
		ChannelQuotaAlertCooldownSeconds = originalCooldown
		ChannelQuotaAlertNotifyOnRecovery = originalRecovery
	}()
	t.Setenv("CHANNEL_QUOTA_ALERT_ENABLED", "true")
	t.Setenv("CHANNEL_QUOTA_ALERT_WARNING_PERCENT", "25.5")
	t.Setenv("CHANNEL_QUOTA_ALERT_CRITICAL_PERCENT", "5")
	t.Setenv("CHANNEL_QUOTA_ALERT_COOLDOWN_SECONDS", "1800")
	t.Setenv("CHANNEL_QUOTA_ALERT_NOTIFY_ON_RECOVERY", "true")
	initChannelQuotaAlertSettings()
	if !ChannelQuotaAlertEnabled || ChannelQuotaAlertWarningPercent != 25.5 || ChannelQuotaAlertCriticalPercent != 5 {
		t.Fatalf("unexpected configured values: enabled=%v warning=%v critical=%v", ChannelQuotaAlertEnabled, ChannelQuotaAlertWarningPercent, ChannelQuotaAlertCriticalPercent)
	}
	if ChannelQuotaAlertCooldownSeconds != 1800 || !ChannelQuotaAlertNotifyOnRecovery {
		t.Fatalf("unexpected configured policy: cooldown=%v recovery=%v", ChannelQuotaAlertCooldownSeconds, ChannelQuotaAlertNotifyOnRecovery)
	}
}

func TestInitChannelQuotaAlertSettingsRejectsInvalidThresholds(t *testing.T) {
	originalEnabled := ChannelQuotaAlertEnabled
	originalWarning := ChannelQuotaAlertWarningPercent
	originalCritical := ChannelQuotaAlertCriticalPercent
	originalCooldown := ChannelQuotaAlertCooldownSeconds
	originalRecovery := ChannelQuotaAlertNotifyOnRecovery
	defer func() {
		ChannelQuotaAlertEnabled = originalEnabled
		ChannelQuotaAlertWarningPercent = originalWarning
		ChannelQuotaAlertCriticalPercent = originalCritical
		ChannelQuotaAlertCooldownSeconds = originalCooldown
		ChannelQuotaAlertNotifyOnRecovery = originalRecovery
	}()
	t.Setenv("CHANNEL_QUOTA_ALERT_WARNING_PERCENT", "5")
	t.Setenv("CHANNEL_QUOTA_ALERT_CRITICAL_PERCENT", "10")
	t.Setenv("CHANNEL_QUOTA_ALERT_COOLDOWN_SECONDS", "")
	t.Setenv("CHANNEL_QUOTA_ALERT_NOTIFY_ON_RECOVERY", "")
	initChannelQuotaAlertSettings()
	if ChannelQuotaAlertWarningPercent != DefaultChannelQuotaAlertWarningPercent || ChannelQuotaAlertCriticalPercent != DefaultChannelQuotaAlertCriticalPercent {
		t.Fatalf("invalid thresholds were not reset: warning=%v critical=%v", ChannelQuotaAlertWarningPercent, ChannelQuotaAlertCriticalPercent)
	}
}
