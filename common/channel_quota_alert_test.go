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
}

func TestInitChannelQuotaAlertSettingsDefaultsToDisabled(t *testing.T) {
	originalEnabled := ChannelQuotaAlertEnabled
	originalWarning := ChannelQuotaAlertWarningPercent
	originalCritical := ChannelQuotaAlertCriticalPercent
	defer func() {
		ChannelQuotaAlertEnabled = originalEnabled
		ChannelQuotaAlertWarningPercent = originalWarning
		ChannelQuotaAlertCriticalPercent = originalCritical
	}()
	t.Setenv("CHANNEL_QUOTA_ALERT_ENABLED", "")
	t.Setenv("CHANNEL_QUOTA_ALERT_WARNING_PERCENT", "")
	t.Setenv("CHANNEL_QUOTA_ALERT_CRITICAL_PERCENT", "")
	initChannelQuotaAlertSettings()
	if ChannelQuotaAlertEnabled {
		t.Fatal("quota alerts must be disabled by default")
	}
	if ChannelQuotaAlertWarningPercent != DefaultChannelQuotaAlertWarningPercent || ChannelQuotaAlertCriticalPercent != DefaultChannelQuotaAlertCriticalPercent {
		t.Fatalf("unexpected defaults: warning=%v critical=%v", ChannelQuotaAlertWarningPercent, ChannelQuotaAlertCriticalPercent)
	}
}

func TestInitChannelQuotaAlertSettingsAcceptsValidThresholds(t *testing.T) {
	originalEnabled := ChannelQuotaAlertEnabled
	originalWarning := ChannelQuotaAlertWarningPercent
	originalCritical := ChannelQuotaAlertCriticalPercent
	defer func() {
		ChannelQuotaAlertEnabled = originalEnabled
		ChannelQuotaAlertWarningPercent = originalWarning
		ChannelQuotaAlertCriticalPercent = originalCritical
	}()
	t.Setenv("CHANNEL_QUOTA_ALERT_ENABLED", "true")
	t.Setenv("CHANNEL_QUOTA_ALERT_WARNING_PERCENT", "25.5")
	t.Setenv("CHANNEL_QUOTA_ALERT_CRITICAL_PERCENT", "5")
	initChannelQuotaAlertSettings()
	if !ChannelQuotaAlertEnabled || ChannelQuotaAlertWarningPercent != 25.5 || ChannelQuotaAlertCriticalPercent != 5 {
		t.Fatalf("unexpected configured values: enabled=%v warning=%v critical=%v", ChannelQuotaAlertEnabled, ChannelQuotaAlertWarningPercent, ChannelQuotaAlertCriticalPercent)
	}
}

func TestInitChannelQuotaAlertSettingsRejectsInvalidThresholds(t *testing.T) {
	originalEnabled := ChannelQuotaAlertEnabled
	originalWarning := ChannelQuotaAlertWarningPercent
	originalCritical := ChannelQuotaAlertCriticalPercent
	defer func() {
		ChannelQuotaAlertEnabled = originalEnabled
		ChannelQuotaAlertWarningPercent = originalWarning
		ChannelQuotaAlertCriticalPercent = originalCritical
	}()
	t.Setenv("CHANNEL_QUOTA_ALERT_WARNING_PERCENT", "5")
	t.Setenv("CHANNEL_QUOTA_ALERT_CRITICAL_PERCENT", "10")
	initChannelQuotaAlertSettings()
	if ChannelQuotaAlertWarningPercent != DefaultChannelQuotaAlertWarningPercent || ChannelQuotaAlertCriticalPercent != DefaultChannelQuotaAlertCriticalPercent {
		t.Fatalf("invalid thresholds were not reset: warning=%v critical=%v", ChannelQuotaAlertWarningPercent, ChannelQuotaAlertCriticalPercent)
	}
}
