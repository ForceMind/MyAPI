package operation_setting

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/ForceMind/MyAPI/setting/config"
)

type MonitorSetting struct {
	AutoTestChannelEnabled bool    `json:"auto_test_channel_enabled"`
	AutoTestChannelMinutes float64 `json:"auto_test_channel_minutes"`
	ChannelTestMode        string  `json:"channel_test_mode"`
	ChannelTestConcurrency int     `json:"channel_test_concurrency"`
}

const (
	ChannelTestModeScheduledAll    = "scheduled_all"
	ChannelTestModeAutoBanOnly     = "auto_ban_only"
	ChannelTestModePassiveRecovery = "passive_recovery"

	ChannelTestConcurrencyOptionKey = "monitor_setting.channel_test_concurrency"
	DefaultChannelTestConcurrency   = 1
	MaxChannelTestConcurrency       = 32
)

var defaultMonitorSetting = MonitorSetting{
	AutoTestChannelEnabled: false,
	AutoTestChannelMinutes: 10,
	ChannelTestMode:        ChannelTestModeScheduledAll,
	ChannelTestConcurrency: DefaultChannelTestConcurrency,
}

// managedMonitorSetting keeps the persisted configuration as immutable
// generations. Readers never observe an in-progress config import, and the
// effective environment-derived view is intentionally kept out of this state.
type managedMonitorSetting struct {
	writeMutex sync.Mutex
	current    atomic.Pointer[MonitorSetting]
}

func newManagedMonitorSetting(initial MonitorSetting) *managedMonitorSetting {
	setting := &managedMonitorSetting{}
	setting.current.Store(&initial)
	return setting
}

// monitorSetting is registered as a managed configuration instead of exposing
// a mutable struct to the reflection-based config loader.
var monitorSetting = newManagedMonitorSetting(defaultMonitorSetting)

func init() {
	config.GlobalConfig.Register("monitor_setting", monitorSetting)
}

func (s *managedMonitorSetting) snapshot() MonitorSetting {
	if current := s.current.Load(); current != nil {
		return *current
	}
	return defaultMonitorSetting
}

func (s *managedMonitorSetting) publish(setting MonitorSetting) {
	s.writeMutex.Lock()
	s.current.Store(&setting)
	s.writeMutex.Unlock()
}

func (s *managedMonitorSetting) ExportConfigMap() (map[string]string, error) {
	setting := s.snapshot()
	return map[string]string{
		"auto_test_channel_enabled": strconv.FormatBool(setting.AutoTestChannelEnabled),
		"auto_test_channel_minutes": strconv.FormatFloat(setting.AutoTestChannelMinutes, 'f', -1, 64),
		"channel_test_mode":         setting.ChannelTestMode,
		"channel_test_concurrency":  strconv.Itoa(setting.ChannelTestConcurrency),
	}, nil
}

// ValidateConfigMap validates an entire pending generation without publishing
// it. ConfigManager detects this method through its validating MapConfig
// contract when available.
func (s *managedMonitorSetting) ValidateConfigMap(values map[string]string) error {
	_, err := s.candidate(values)
	return err
}

func (s *managedMonitorSetting) UpdateConfigMap(values map[string]string) error {
	s.writeMutex.Lock()
	defer s.writeMutex.Unlock()
	candidate, err := s.candidate(values)
	if err != nil {
		return err
	}
	s.current.Store(&candidate)
	return nil
}

// candidate applies only recognized persisted fields to one locally copied
// generation. It deliberately leaves mode fallback and concurrency
// normalization to GetMonitorSetting so exporting the persisted setting never
// changes an operator's raw DB value.
func (s *managedMonitorSetting) candidate(values map[string]string) (MonitorSetting, error) {
	candidate := s.snapshot()
	if raw, ok := values["auto_test_channel_enabled"]; ok {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return MonitorSetting{}, fmt.Errorf("config field %q: %w", "auto_test_channel_enabled", err)
		}
		candidate.AutoTestChannelEnabled = value
	}
	if raw, ok := values["auto_test_channel_minutes"]; ok {
		value, err := strconv.ParseFloat(raw, 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
			if err == nil {
				err = fmt.Errorf("value must be finite")
			}
			return MonitorSetting{}, fmt.Errorf("config field %q: %w", "auto_test_channel_minutes", err)
		}
		candidate.AutoTestChannelMinutes = value
	}
	if raw, ok := values["channel_test_mode"]; ok {
		candidate.ChannelTestMode = raw
	}
	if raw, ok := values["channel_test_concurrency"]; ok {
		value, err := parseMonitorSettingInt(raw)
		if err != nil {
			return MonitorSetting{}, fmt.Errorf("config field %q: %w", "channel_test_concurrency", err)
		}
		candidate.ChannelTestConcurrency = value
	}
	return candidate, nil
}

func parseMonitorSettingInt(raw string) (int, error) {
	if value, err := strconv.Atoi(raw); err == nil {
		return value, nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	minValue := -math.Ldexp(1, strconv.IntSize-1)
	maxValue := math.Ldexp(1, strconv.IntSize-1)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || math.Trunc(value) != value || value < minValue || value >= maxValue {
		return 0, fmt.Errorf("invalid integer")
	}
	return int(value), nil
}

func GetMonitorSetting() *MonitorSetting {
	// Start with a local copy of one persisted generation. Do not apply runtime
	// overrides to the managed state: removing an environment override must
	// immediately reveal the stored DB value again.
	setting := monitorSetting.snapshot()
	if frequencyValue := os.Getenv("CHANNEL_TEST_FREQUENCY"); frequencyValue != "" {
		frequency, err := strconv.Atoi(frequencyValue)
		if err == nil && frequency > 0 {
			setting.AutoTestChannelEnabled = true
			setting.AutoTestChannelMinutes = float64(frequency)
			setting.ChannelTestMode = ChannelTestModeScheduledAll
		}
	}
	if enabled, ok := os.LookupEnv("CHANNEL_TEST_ENABLED"); ok {
		parsed, err := strconv.ParseBool(enabled)
		if err == nil {
			setting.AutoTestChannelEnabled = parsed
		}
	}
	switch setting.ChannelTestMode {
	case ChannelTestModeAutoBanOnly, ChannelTestModePassiveRecovery:
	default:
		setting.ChannelTestMode = ChannelTestModeScheduledAll
	}
	setting.ChannelTestConcurrency = NormalizeChannelTestConcurrency(setting.ChannelTestConcurrency)
	return &setting
}

func NormalizeChannelTestConcurrency(concurrency int) int {
	if concurrency < 1 {
		return DefaultChannelTestConcurrency
	}
	if concurrency > MaxChannelTestConcurrency {
		return MaxChannelTestConcurrency
	}
	return concurrency
}

func ValidateChannelTestConcurrency(value string) error {
	concurrency, err := strconv.Atoi(value)
	if err != nil || concurrency < 1 || concurrency > MaxChannelTestConcurrency {
		return fmt.Errorf("channel test concurrency must be between 1 and %d", MaxChannelTestConcurrency)
	}
	return nil
}
