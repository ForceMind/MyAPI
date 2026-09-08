package model_setting

import (
	"reflect"
	"sync"
	"testing"

	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManagedGlobalSettingsExportsDefaultsAndPreservesPartialUpdates(t *testing.T) {
	state := newManagedGlobalSettings(defaultOpenaiSettings)
	exported, err := state.ExportConfigMap()
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"pass_through_request_enabled":         "false",
		"thinking_model_blacklist":             `["moonshotai/kimi-k2-thinking","kimi-k2-thinking"]`,
		"chat_completions_to_responses_policy": `{"enabled":false,"all_channels":true}`,
	}, exported)

	require.NoError(t, state.UpdateConfigMap(map[string]string{
		"pass_through_request_enabled": "true",
		"unknown_legacy_key":           "ignored",
	}))
	snapshot := state.snapshot()
	assert.True(t, snapshot.PassThroughRequestEnabled)
	assert.Equal(t, defaultOpenaiSettings.ThinkingModelBlacklist, snapshot.ThinkingModelBlacklist)
	assert.Equal(t, defaultOpenaiSettings.ChatCompletionsToResponsesPolicy, snapshot.ChatCompletionsToResponsesPolicy)
}

func TestManagedGlobalSettingsPreservesNilEmptyAndRejectsInvalidCandidates(t *testing.T) {
	state := newManagedGlobalSettings(defaultOpenaiSettings)
	require.NoError(t, state.UpdateConfigMap(map[string]string{
		"thinking_model_blacklist":             "null",
		"chat_completions_to_responses_policy": `{"channel_ids":null,"channel_types":[],"model_patterns":[]}`,
	}))
	snapshot := state.snapshot()
	assert.Nil(t, snapshot.ThinkingModelBlacklist)
	assert.False(t, snapshot.ChatCompletionsToResponsesPolicy.Enabled)
	assert.True(t, snapshot.ChatCompletionsToResponsesPolicy.AllChannels)
	assert.Nil(t, snapshot.ChatCompletionsToResponsesPolicy.ChannelIDs)
	assert.NotNil(t, snapshot.ChatCompletionsToResponsesPolicy.ChannelTypes)
	assert.Empty(t, snapshot.ChatCompletionsToResponsesPolicy.ChannelTypes)
	assert.NotNil(t, snapshot.ChatCompletionsToResponsesPolicy.ModelPatterns)
	assert.Empty(t, snapshot.ChatCompletionsToResponsesPolicy.ModelPatterns)
	policyBeforeNull := snapshot.ChatCompletionsToResponsesPolicy
	require.NoError(t, state.UpdateConfigMap(map[string]string{
		"chat_completions_to_responses_policy": "null",
	}))
	assert.Equal(t, policyBeforeNull, state.snapshot().ChatCompletionsToResponsesPolicy)

	before := state.current.Load()
	for _, update := range []map[string]string{
		{"pass_through_request_enabled": "not-a-bool"},
		{"thinking_model_blacklist": "not-json"},
		{"thinking_model_blacklist": "true"},
		{"chat_completions_to_responses_policy": "not-json"},
		{"chat_completions_to_responses_policy": "true"},
	} {
		require.Error(t, state.ValidateConfigMap(update))
		assert.Same(t, before, state.current.Load())
		require.Error(t, state.UpdateConfigMap(update))
		assert.Same(t, before, state.current.Load())
	}
}

func TestManagedGlobalSettingsCopiesInitialAndDetachedSnapshots(t *testing.T) {
	initial := GlobalSettings{
		ThinkingModelBlacklist: []string{"thinking-before"},
		ChatCompletionsToResponsesPolicy: ChatCompletionsToResponsesPolicy{
			ChannelIDs:    []int{1},
			ChannelTypes:  []int{2},
			ModelPatterns: []string{"model-before"},
		},
	}
	state := newManagedGlobalSettings(initial)
	initial.ThinkingModelBlacklist[0] = "caller-initial-mutation"
	initial.ChatCompletionsToResponsesPolicy.ChannelIDs[0] = 10
	initial.ChatCompletionsToResponsesPolicy.ChannelTypes[0] = 20
	initial.ChatCompletionsToResponsesPolicy.ModelPatterns[0] = "caller-initial-mutation"

	snapshot := state.snapshot()
	snapshot.ThinkingModelBlacklist[0] = "caller-snapshot-mutation"
	snapshot.ChatCompletionsToResponsesPolicy.ChannelIDs[0] = 11
	snapshot.ChatCompletionsToResponsesPolicy.ChannelTypes[0] = 21
	snapshot.ChatCompletionsToResponsesPolicy.ModelPatterns[0] = "caller-snapshot-mutation"
	assert.Equal(t, []string{"thinking-before"}, state.snapshot().ThinkingModelBlacklist)
	assert.Equal(t, []int{1}, state.snapshot().ChatCompletionsToResponsesPolicy.ChannelIDs)
	assert.Equal(t, []int{2}, state.snapshot().ChatCompletionsToResponsesPolicy.ChannelTypes)
	assert.Equal(t, []string{"model-before"}, state.snapshot().ChatCompletionsToResponsesPolicy.ModelPatterns)
}

func TestGetGlobalSettingsReturnsDetachedSnapshot(t *testing.T) {
	registered := config.GlobalConfig.Get("global")
	require.NotNil(t, registered)
	baseline, err := config.ConfigToMap(registered)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, config.UpdateConfigFromMap(registered, baseline)) })

	require.NoError(t, config.UpdateConfigFromMap(registered, map[string]string{
		"thinking_model_blacklist":             `["thinking-one"]`,
		"chat_completions_to_responses_policy": `{"channel_ids":[1],"channel_types":[2],"model_patterns":["model-one"]}`,
	}))
	detached := GetGlobalSettings()
	detached.ThinkingModelBlacklist[0] = "caller-mutation"
	detached.ChatCompletionsToResponsesPolicy.ChannelIDs[0] = 10
	detached.ChatCompletionsToResponsesPolicy.ChannelTypes[0] = 20
	detached.ChatCompletionsToResponsesPolicy.ModelPatterns[0] = "caller-mutation"

	snapshot := globalSettingsState.snapshot()
	assert.Equal(t, []string{"thinking-one"}, snapshot.ThinkingModelBlacklist)
	assert.Equal(t, []int{1}, snapshot.ChatCompletionsToResponsesPolicy.ChannelIDs)
	assert.Equal(t, []int{2}, snapshot.ChatCompletionsToResponsesPolicy.ChannelTypes)
	assert.Equal(t, []string{"model-one"}, snapshot.ChatCompletionsToResponsesPolicy.ModelPatterns)
}

func TestManagedGlobalSettingsPublishesWholeGeneration(t *testing.T) {
	state := newManagedGlobalSettings(GlobalSettings{
		ThinkingModelBlacklist: []string{"first"},
		ChatCompletionsToResponsesPolicy: ChatCompletionsToResponsesPolicy{
			Enabled: true, ChannelIDs: []int{1}, ChannelTypes: []int{2}, ModelPatterns: []string{"first"},
		},
	})
	first := state.snapshot()
	second := GlobalSettings{PassThroughRequestEnabled: true, ThinkingModelBlacklist: []string{"second"}, ChatCompletionsToResponsesPolicy: ChatCompletionsToResponsesPolicy{AllChannels: true, ChannelIDs: []int{3}, ChannelTypes: []int{4}, ModelPatterns: []string{"second"}}}
	updates := []map[string]string{
		{"pass_through_request_enabled": "false", "thinking_model_blacklist": `["first"]`, "chat_completions_to_responses_policy": `{"enabled":true,"channel_ids":[1],"channel_types":[2],"model_patterns":["first"]}`},
		{"pass_through_request_enabled": "true", "thinking_model_blacklist": `["second"]`, "chat_completions_to_responses_policy": `{"enabled":false,"all_channels":true,"channel_ids":[3],"channel_types":[4],"model_patterns":["second"]}`},
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
		assert.True(t, reflect.DeepEqual(snapshot, first) || reflect.DeepEqual(snapshot, second), "observed mixed generation: %#v", snapshot)
	}
	writers.Wait()
	select {
	case err := <-writerErr:
		require.NoError(t, err)
	default:
	}
}

func TestManagedGlobalSettingsConfigManagerLoadAndHelpers(t *testing.T) {
	manager := config.NewConfigManager()
	state := newManagedGlobalSettings(defaultOpenaiSettings)
	manager.Register("global", state)
	require.NoError(t, manager.LoadFromDB(map[string]string{
		"global.pass_through_request_enabled":         "true",
		"global.thinking_model_blacklist":             `[" thinking-model "]`,
		"global.chat_completions_to_responses_policy": `{"enabled":true,"all_channels":false,"channel_ids":[1],"channel_types":[2],"model_patterns":["model"]}`,
		"global.unknown":                              "ignored",
	}))
	snapshot := state.snapshot()
	assert.True(t, snapshot.PassThroughRequestEnabled)
	assert.True(t, snapshot.ShouldPreserveThinkingSuffix("thinking-model"))
	assert.False(t, snapshot.ShouldPreserveThinkingSuffix("thinking-model-extra"))
	assert.True(t, snapshot.ChatCompletionsToResponsesPolicy.IsChannelEnabled(1, 0))
	assert.True(t, snapshot.ChatCompletionsToResponsesPolicy.IsChannelEnabled(0, 2))
	assert.False(t, snapshot.ChatCompletionsToResponsesPolicy.IsChannelEnabled(3, 4))

	saved := make(map[string]string)
	require.NoError(t, manager.SaveToDB(func(key, value string) error { saved[key] = value; return nil }))
	assert.Equal(t, map[string]string{
		"global.pass_through_request_enabled":         "true",
		"global.thinking_model_blacklist":             `[" thinking-model "]`,
		"global.chat_completions_to_responses_policy": `{"enabled":true,"all_channels":false,"channel_ids":[1],"channel_types":[2],"model_patterns":["model"]}`,
	}, saved)
}
