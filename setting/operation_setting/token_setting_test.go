package operation_setting

import (
	"sync"
	"testing"

	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManagedTokenSettingPreservesConfigContract(t *testing.T) {
	state := newManagedTokenSetting(defaultTokenSetting)

	exported, err := state.ExportConfigMap()
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"max_user_tokens": "1000",
	}, exported)

	before := state.current.Load()
	require.NoError(t, state.ValidateConfigMap(map[string]string{
		"max_user_tokens": "500",
	}))
	assert.Same(t, before, state.current.Load())

	require.NoError(t, state.UpdateConfigMap(map[string]string{
		"max_user_tokens": "0",
		"unknown":         "option-only",
	}))
	assert.Equal(t, TokenSetting{MaxUserTokens: 0}, state.snapshot())

	require.NoError(t, state.UpdateConfigMap(map[string]string{
		"unknown": "still-option-only",
	}))
	assert.Equal(t, TokenSetting{MaxUserTokens: 0}, state.snapshot())
}

func TestManagedTokenSettingRejectsInvalidScalarsWithoutPublishing(t *testing.T) {
	state := newManagedTokenSetting(defaultTokenSetting)
	before := state.current.Load()

	for _, update := range []map[string]string{
		{"max_user_tokens": "1.5"},
		{"max_user_tokens": "9223372036854775808"},
		{"max_user_tokens": "null"},
	} {
		require.Error(t, state.ValidateConfigMap(update))
		assert.Same(t, before, state.current.Load())
		require.Error(t, state.UpdateConfigMap(update))
		assert.Same(t, before, state.current.Load())
	}
}

func TestGetTokenSettingReturnsDetachedSnapshot(t *testing.T) {
	registered := config.GlobalConfig.Get("token_setting")
	require.NotNil(t, registered)
	baseline, err := config.ConfigToMap(registered)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, config.UpdateConfigFromMap(registered, baseline))
	})

	require.NoError(t, config.UpdateConfigFromMap(registered, map[string]string{
		"max_user_tokens": "321",
	}))
	detached := GetTokenSetting()
	detached.MaxUserTokens = -1

	assert.Equal(t, 321, GetMaxUserTokens())
	assert.Equal(t, TokenSetting{MaxUserTokens: 321}, tokenSettingState.snapshot())
}

func TestManagedTokenSettingConcurrentReadWritePublishesWholeGenerations(t *testing.T) {
	state := newManagedTokenSetting(TokenSetting{MaxUserTokens: 1})
	first := TokenSetting{MaxUserTokens: 1}
	second := TokenSetting{MaxUserTokens: 2}
	updates := []map[string]string{
		{"max_user_tokens": "1"},
		{"max_user_tokens": "2"},
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

func TestManagedTokenSettingConfigManagerIntegration(t *testing.T) {
	manager := config.NewConfigManager()
	state := newManagedTokenSetting(defaultTokenSetting)
	manager.Register("token_setting", state)

	require.NoError(t, manager.LoadFromDB(map[string]string{
		"token_setting.max_user_tokens": "654",
		"token_setting.unknown":         "ignored-by-struct",
	}))
	assert.Equal(t, TokenSetting{MaxUserTokens: 654}, state.snapshot())

	exported, err := config.ConfigToMap(state)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"max_user_tokens": "654"}, exported)
}
