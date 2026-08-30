package common

import "testing"

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
