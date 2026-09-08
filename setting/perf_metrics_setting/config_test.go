package perf_metrics_setting

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManagedPerfMetricsSettingExportsDefaultsAndPartialUpdatesPreserveTheCandidate(t *testing.T) {
	state := newManagedPerfMetricsSetting(defaultPerfMetricsSetting)

	exported, err := state.ExportConfigMap()
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"enabled":        "true",
		"flush_interval": "5",
		"bucket_time":    "hour",
		"retention_days": "0",
	}, exported)

	require.NoError(t, state.UpdateConfigMap(map[string]string{
		"enabled":        "false",
		"bucket_time":    "minute",
		"retention_days": "7",
	}))
	require.NoError(t, state.UpdateConfigMap(map[string]string{
		"flush_interval": "9",
	}))
	assert.Equal(t, PerfMetricsSetting{
		Enabled:       false,
		FlushInterval: 9,
		BucketTime:    "minute",
		RetentionDays: 7,
	}, state.snapshot())
}

func TestManagedPerfMetricsSettingValidationDoesNotPublishInvalidCandidates(t *testing.T) {
	state := newManagedPerfMetricsSetting(defaultPerfMetricsSetting)
	before := state.current.Load()

	require.NoError(t, state.UpdateConfigMap(map[string]string{
		"flush_interval": "5.000000",
		"retention_days": "7.000000",
	}))
	assert.Equal(t, PerfMetricsSetting{
		Enabled:       true,
		FlushInterval: 5,
		BucketTime:    "hour",
		RetentionDays: 7,
	}, state.snapshot())

	before = state.current.Load()
	require.Error(t, state.ValidateConfigMap(map[string]string{"flush_interval": "not-an-int"}))
	assert.Same(t, before, state.current.Load())
	for _, update := range []map[string]string{
		{"retention_days": "1.5"},
		{"retention_days": "9223372036854775808"},
		{"retention_days": "NaN"},
		{"retention_days": "Inf"},
		{"retention_days": "-Inf"},
	} {
		require.Error(t, state.UpdateConfigMap(update))
		assert.Same(t, before, state.current.Load())
	}
}

func TestManagedPerfMetricsSettingDetachedSnapshotAndLegacyFallbacks(t *testing.T) {
	state := newManagedPerfMetricsSetting(PerfMetricsSetting{
		Enabled:       true,
		FlushInterval: 0,
		BucketTime:    "unknown",
		RetentionDays: 3,
	})

	snapshot := state.snapshot()
	snapshot.BucketTime = "minute"
	assert.Equal(t, "unknown", state.snapshot().BucketTime)
	assert.Equal(t, int64(3600), bucketSeconds(state.snapshot()))
	assert.Equal(t, 1, flushIntervalMinutes(state.snapshot()))
}

func TestManagedPerfMetricsSettingConcurrentPartialUpdatesRetainAllFields(t *testing.T) {
	state := newManagedPerfMetricsSetting(defaultPerfMetricsSetting)
	start := make(chan struct{})
	errs := make(chan error, 2)
	var workers sync.WaitGroup

	for _, update := range []map[string]string{
		{"enabled": "false", "flush_interval": "9"},
		{"bucket_time": "5min", "retention_days": "7"},
	} {
		update := update
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			errs <- state.UpdateConfigMap(update)
		}()
	}

	close(start)
	workers.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	assert.Equal(t, PerfMetricsSetting{
		Enabled:       false,
		FlushInterval: 9,
		BucketTime:    "5min",
		RetentionDays: 7,
	}, state.snapshot())
}
