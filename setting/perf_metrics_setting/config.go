package perf_metrics_setting

import (
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/ForceMind/MyAPI/setting/config"
)

type PerfMetricsSetting struct {
	Enabled       bool   `json:"enabled"`
	FlushInterval int    `json:"flush_interval"`
	BucketTime    string `json:"bucket_time"`
	RetentionDays int    `json:"retention_days"`
}

var defaultPerfMetricsSetting = PerfMetricsSetting{
	Enabled:       true,
	FlushInterval: 5,
	BucketTime:    "hour",
	RetentionDays: 0,
}

// perfMetricsGeneration is immutable after publication. Each helper takes one
// snapshot so it cannot observe a partially updated configuration struct.
type perfMetricsGeneration struct {
	setting PerfMetricsSetting
}

type managedPerfMetricsSetting struct {
	writeMutex sync.Mutex
	current    atomic.Pointer[perfMetricsGeneration]
}

func newManagedPerfMetricsSetting(initial PerfMetricsSetting) *managedPerfMetricsSetting {
	setting := &managedPerfMetricsSetting{}
	setting.current.Store(&perfMetricsGeneration{setting: initial})
	return setting
}

func (s *managedPerfMetricsSetting) snapshot() PerfMetricsSetting {
	if s != nil {
		if current := s.current.Load(); current != nil {
			return current.setting
		}
	}
	return defaultPerfMetricsSetting
}

func (s *managedPerfMetricsSetting) candidate(values map[string]string) (PerfMetricsSetting, error) {
	candidate := s.snapshot()
	if err := config.UpdateConfigFromMap(&candidate, values); err != nil {
		return PerfMetricsSetting{}, err
	}
	return candidate, nil
}

func (s *managedPerfMetricsSetting) ExportConfigMap() (map[string]string, error) {
	setting := s.snapshot()
	return map[string]string{
		"enabled":        strconv.FormatBool(setting.Enabled),
		"flush_interval": strconv.Itoa(setting.FlushInterval),
		"bucket_time":    setting.BucketTime,
		"retention_days": strconv.Itoa(setting.RetentionDays),
	}, nil
}

// ValidateConfigMap uses the existing generic scalar parser against an isolated
// complete candidate, without publishing it.
func (s *managedPerfMetricsSetting) ValidateConfigMap(values map[string]string) error {
	_, err := s.candidate(values)
	return err
}

func (s *managedPerfMetricsSetting) UpdateConfigMap(values map[string]string) error {
	s.writeMutex.Lock()
	defer s.writeMutex.Unlock()
	candidate, err := s.candidate(values)
	if err != nil {
		return err
	}
	s.current.Store(&perfMetricsGeneration{setting: candidate})
	return nil
}

var perfMetricsSetting = newManagedPerfMetricsSetting(defaultPerfMetricsSetting)

var _ config.ValidatingMapConfig = (*managedPerfMetricsSetting)(nil)

func init() {
	config.GlobalConfig.Register("perf_metrics_setting", perfMetricsSetting)
}

func GetSetting() PerfMetricsSetting {
	return perfMetricsSetting.snapshot()
}

func GetBucketSeconds() int64 {
	setting := perfMetricsSetting.snapshot()
	return bucketSeconds(setting)
}

func bucketSeconds(setting PerfMetricsSetting) int64 {
	switch setting.BucketTime {
	case "minute":
		return 60
	case "5min":
		return 300
	case "hour":
		return 3600
	default:
		return 3600
	}
}

func GetFlushIntervalMinutes() int {
	setting := perfMetricsSetting.snapshot()
	return flushIntervalMinutes(setting)
}

func flushIntervalMinutes(setting PerfMetricsSetting) int {
	if setting.FlushInterval < 1 {
		return 1
	}
	return setting.FlushInterval
}
