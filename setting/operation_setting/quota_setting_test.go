package operation_setting

import (
	"sync"
	"testing"

	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManagedQuotaSettingExportsDefaultsAndPreservesPartialUpdates(t *testing.T) {
	state := newManagedQuotaSetting(defaultQuotaSetting)

	exported, err := state.ExportConfigMap()
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"enable_free_model_pre_consume": "true"}, exported)

	require.NoError(t, state.UpdateConfigMap(map[string]string{
		"enable_free_model_pre_consume": "false",
		"unknown_legacy_key":            "ignored",
	}))
	require.NoError(t, state.UpdateConfigMap(map[string]string{"unknown_legacy_key": "still ignored"}))
	assert.Equal(t, QuotaSetting{EnableFreeModelPreConsume: false}, state.snapshot())
}

func TestManagedQuotaSettingInvalidCandidatesDoNotPublish(t *testing.T) {
	state := newManagedQuotaSetting(defaultQuotaSetting)
	before := state.current.Load()

	for _, update := range []map[string]string{
		{"enable_free_model_pre_consume": "not-a-bool"},
		{"enable_free_model_pre_consume": ""},
	} {
		require.Error(t, state.ValidateConfigMap(update))
		assert.Same(t, before, state.current.Load())
		require.Error(t, state.UpdateConfigMap(update))
		assert.Same(t, before, state.current.Load())
	}
}

func TestGetQuotaSettingReturnsDetachedSnapshot(t *testing.T) {
	registered := config.GlobalConfig.Get("quota_setting")
	require.NotNil(t, registered)
	baseline, err := config.ConfigToMap(registered)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, config.UpdateConfigFromMap(registered, baseline))
	})

	require.NoError(t, config.UpdateConfigFromMap(registered, map[string]string{
		"enable_free_model_pre_consume": "false",
	}))
	detached := GetQuotaSetting()
	detached.EnableFreeModelPreConsume = true
	assert.False(t, quotaSettingState.snapshot().EnableFreeModelPreConsume)
}

func TestManagedQuotaSettingPublishesWholeGeneration(t *testing.T) {
	state := newManagedQuotaSetting(QuotaSetting{EnableFreeModelPreConsume: false})
	first := QuotaSetting{EnableFreeModelPreConsume: false}
	second := QuotaSetting{EnableFreeModelPreConsume: true}
	updates := []map[string]string{
		{"enable_free_model_pre_consume": "false"},
		{"enable_free_model_pre_consume": "true"},
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
		assert.True(t, snapshot == first || snapshot == second, "observed unexpected generation: %#v", snapshot)
	}
	writers.Wait()
	select {
	case err := <-writerErr:
		require.NoError(t, err)
	default:
	}
}

func TestManagedQuotaSettingConfigManagerLoadAndExport(t *testing.T) {
	manager := config.NewConfigManager()
	state := newManagedQuotaSetting(defaultQuotaSetting)
	manager.Register("quota_setting", state)

	require.NoError(t, manager.LoadFromDB(map[string]string{
		"quota_setting.enable_free_model_pre_consume": "false",
		"quota_setting.unknown":                       "ignored",
	}))
	assert.Equal(t, QuotaSetting{EnableFreeModelPreConsume: false}, state.snapshot())

	saved := make(map[string]string)
	require.NoError(t, manager.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	assert.Equal(t, map[string]string{"quota_setting.enable_free_model_pre_consume": "false"}, saved)
}
