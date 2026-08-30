package common

import "testing"

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
