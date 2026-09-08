package model_setting

import (
	"slices"
	"sync"
	"testing"

	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManagedQwenSettingsExportsDefaultsAndPreservesPartialUpdates(t *testing.T) {
	state := newManagedQwenSettings(defaultQwenSettings)

	exported, err := state.ExportConfigMap()
	require.NoError(t, err)
	assert.Equal(t, `["z-image","qwen-image","wan2.6","wan2.7","qwen-image-edit","qwen-image-edit-max","qwen-image-edit-max-2026-01-16","qwen-image-edit-plus","qwen-image-edit-plus-2025-12-15","qwen-image-edit-plus-2025-10-30"]`, exported["sync_image_models"])

	require.NoError(t, state.UpdateConfigMap(map[string]string{
		"sync_image_models":  `["custom-image","wan2.7"]`,
		"unknown_legacy_key": "ignored",
	}))
	assert.Equal(t, []string{"custom-image", "wan2.7"}, state.snapshot().SyncImageModels)
}

func TestManagedQwenSettingsPreservesNullAndRejectsInvalidCandidates(t *testing.T) {
	state := newManagedQwenSettings(defaultQwenSettings)
	require.NoError(t, state.UpdateConfigMap(map[string]string{"sync_image_models": "null"}))
	assert.Nil(t, state.snapshot().SyncImageModels)
	require.NoError(t, state.UpdateConfigMap(map[string]string{"sync_image_models": "[]"}))
	assert.NotNil(t, state.snapshot().SyncImageModels)
	assert.Empty(t, state.snapshot().SyncImageModels)

	before := state.current.Load()
	for _, update := range []map[string]string{
		{"sync_image_models": "not-json"},
		{"sync_image_models": "true"},
	} {
		require.Error(t, state.ValidateConfigMap(update))
		assert.Same(t, before, state.current.Load())
		require.Error(t, state.UpdateConfigMap(update))
		assert.Same(t, before, state.current.Load())
	}
}

func TestManagedQwenSettingsCopiesInitialAndDetachedSnapshots(t *testing.T) {
	initial := QwenSettings{SyncImageModels: []string{"before"}}
	state := newManagedQwenSettings(initial)
	initial.SyncImageModels[0] = "caller-initial-mutation"
	assert.Equal(t, []string{"before"}, state.snapshot().SyncImageModels)

	snapshot := state.snapshot()
	snapshot.SyncImageModels[0] = "caller-snapshot-mutation"
	snapshot.SyncImageModels = append(snapshot.SyncImageModels, "caller-append")
	assert.Equal(t, []string{"before"}, state.snapshot().SyncImageModels)
}

func TestGetQwenSettingsReturnsDetachedSlice(t *testing.T) {
	registered := config.GlobalConfig.Get("qwen")
	require.NotNil(t, registered)
	baseline, err := config.ConfigToMap(registered)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, config.UpdateConfigFromMap(registered, baseline))
	})

	require.NoError(t, config.UpdateConfigFromMap(registered, map[string]string{
		"sync_image_models": `["one","two"]`,
	}))
	detached := GetQwenSettings()
	detached.SyncImageModels[0] = "caller-mutation"
	detached.SyncImageModels = append(detached.SyncImageModels, "caller-append")
	assert.Equal(t, []string{"one", "two"}, qwenSettingsState.snapshot().SyncImageModels)
}

func TestManagedQwenSettingsPublishesWholeGeneration(t *testing.T) {
	state := newManagedQwenSettings(QwenSettings{SyncImageModels: []string{"first", "first-tail"}})
	first := []string{"first", "first-tail"}
	second := []string{"second", "second-tail", "second-last"}
	updates := []map[string]string{
		{"sync_image_models": `["first","first-tail"]`},
		{"sync_image_models": `["second","second-tail","second-last"]`},
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
		models := state.snapshot().SyncImageModels
		assert.True(t, slices.Equal(models, first) || slices.Equal(models, second), "observed mixed generation: %#v", models)
	}
	writers.Wait()
	select {
	case err := <-writerErr:
		require.NoError(t, err)
	default:
	}
}

func TestManagedQwenSettingsConfigManagerLoadAndExport(t *testing.T) {
	manager := config.NewConfigManager()
	state := newManagedQwenSettings(defaultQwenSettings)
	manager.Register("qwen", state)

	require.NoError(t, manager.LoadFromDB(map[string]string{
		"qwen.sync_image_models": `["managed-image"]`,
		"qwen.unknown":           "ignored",
	}))
	assert.Equal(t, []string{"managed-image"}, state.snapshot().SyncImageModels)

	saved := make(map[string]string)
	require.NoError(t, manager.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	assert.Equal(t, map[string]string{
		"qwen.sync_image_models": `["managed-image"]`,
	}, saved)
}

func TestIsSyncImageModelPreservesContainsAndEmptyPatternBehavior(t *testing.T) {
	registered := config.GlobalConfig.Get("qwen")
	require.NotNil(t, registered)
	baseline, err := config.ConfigToMap(registered)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, config.UpdateConfigFromMap(registered, baseline))
	})

	require.NoError(t, config.UpdateConfigFromMap(registered, map[string]string{
		"sync_image_models": `["image"]`,
	}))
	assert.True(t, IsSyncImageModel("qwen-image-3.0"))
	assert.False(t, IsSyncImageModel("wan2.7"))

	require.NoError(t, config.UpdateConfigFromMap(registered, map[string]string{
		"sync_image_models": `[""]`,
	}))
	assert.True(t, IsSyncImageModel("unrelated-model"))
}
