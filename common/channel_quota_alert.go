package common

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
}

func DefaultChannelQuotaAlertSettings() ChannelQuotaAlertSettings {
	return ChannelQuotaAlertSettings{
		Enabled:         DefaultChannelQuotaAlertEnabled,
		WarningPercent:  DefaultChannelQuotaAlertWarningPercent,
		CriticalPercent: DefaultChannelQuotaAlertCriticalPercent,
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
	return nil
}

func ParseChannelQuotaAlertSettings(raw string) (ChannelQuotaAlertSettings, error) {
	var settings ChannelQuotaAlertSettings
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&settings); err != nil {
		return ChannelQuotaAlertSettings{}, fmt.Errorf("invalid channel quota alert settings: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return ChannelQuotaAlertSettings{}, errors.New("invalid channel quota alert settings: multiple JSON values")
		}
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
	encoded, err := json.Marshal(settings)
	if err != nil {
		return "", fmt.Errorf("marshal channel quota alert settings: %w", err)
	}
	return string(encoded), nil
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
