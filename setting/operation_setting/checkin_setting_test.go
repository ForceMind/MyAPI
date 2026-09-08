package operation_setting

import (
	"sync"
	"testing"

	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManagedCheckinSettingExportsDefaultsAndPreservesPartialUpdates(t *testing.T) {
	state := newManagedCheckinSetting(defaultCheckinSetting)

	exported, err := state.ExportConfigMap()
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"enabled":   "false",
		"min_quota": "1000",
		"max_quota": "10000",
	}, exported)

	require.NoError(t, state.UpdateConfigMap(map[string]string{
		"enabled":   "true",
		"min_quota": "300",
		"unknown":   "preserved-as-an-option-only",
	}))
	require.NoError(t, state.UpdateConfigMap(map[string]string{
		"max_quota": "900",
	}))
	assert.Equal(t, CheckinSetting{
		Enabled:  true,
		MinQuota: 300,
		MaxQuota: 900,
	}, state.snapshot())
}

func TestManagedCheckinSettingValidationDoesNotPublishInvalidCandidates(t *testing.T) {
	state := newManagedCheckinSetting(defaultCheckinSetting)
	before := state.current.Load()

	for _, update := range []map[string]string{
		{"enabled": "not-a-bool"},
		{"min_quota": "1.5"},
		{"max_quota": "9223372036854775808"},
		{"min_quota": "null"},
	} {
		require.Error(t, state.ValidateConfigMap(update))
		assert.Same(t, before, state.current.Load())
		require.Error(t, state.UpdateConfigMap(update))
		assert.Same(t, before, state.current.Load())
	}
}

func TestGetCheckinSettingReturnsDetachedSnapshot(t *testing.T) {
	registered := config.GlobalConfig.Get("checkin_setting")
	require.NotNil(t, registered)
	baseline, err := config.ConfigToMap(registered)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, config.UpdateConfigFromMap(registered, baseline))
	})

	require.NoError(t, config.UpdateConfigFromMap(registered, map[string]string{
		"enabled":   "true",
		"min_quota": "321",
		"max_quota": "654",
	}))
	detached := GetCheckinSetting()
	detached.Enabled = false
	detached.MinQuota = -1
	detached.MaxQuota = -2

	assert.Equal(t, CheckinSetting{
		Enabled:  true,
		MinQuota: 321,
		MaxQuota: 654,
	}, checkinSettingState.snapshot())
}

func TestManagedCheckinSettingConcurrentPartialUpdatesRetainAllFields(t *testing.T) {
	state := newManagedCheckinSetting(defaultCheckinSetting)
	start := make(chan struct{})
	errs := make(chan error, 2)
	var workers sync.WaitGroup

	for _, update := range []map[string]string{
		{"enabled": "true", "min_quota": "200"},
		{"max_quota": "800"},
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

	assert.Equal(t, CheckinSetting{
		Enabled:  true,
		MinQuota: 200,
		MaxQuota: 800,
	}, state.snapshot())
}

func TestManagedCheckinSettingPublishesWholeGeneration(t *testing.T) {
	state := newManagedCheckinSetting(CheckinSetting{Enabled: false, MinQuota: 1, MaxQuota: 2})
	first := CheckinSetting{Enabled: false, MinQuota: 1, MaxQuota: 2}
	second := CheckinSetting{Enabled: true, MinQuota: 30, MaxQuota: 40}
	updates := []map[string]string{
		{"enabled": "false", "min_quota": "1", "max_quota": "2"},
		{"enabled": "true", "min_quota": "30", "max_quota": "40"},
	}

	var writers sync.WaitGroup
	writerErr := make(chan error, 1)
	started := make(chan struct{})
	writers.Add(1)
	go func() {
		defer writers.Done()
		for i := 0; i < 100; i++ {
			if err := state.UpdateConfigMap(updates[i%len(updates)]); err != nil {
				writerErr <- err
				return
			}
			if i == 0 {
				close(started)
			}
		}
	}()
	<-started
	for i := 0; i < 1000; i++ {
		snapshot := state.snapshot()
		assert.True(t, snapshot == first || snapshot == second, "observed mixed generation: %#v", snapshot)
	}
	writers.Wait()
	select {
	case err := <-writerErr:
		require.NoError(t, err)
	default:
	}
}
