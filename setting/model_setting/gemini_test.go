package model_setting

import (
	"math"
	"reflect"
	"sync"
	"testing"

	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManagedGeminiSettingsExportsDefaultsAndPreservesPartialUpdates(t *testing.T) {
	state := newManagedGeminiSettings(defaultGeminiSettings)
	exported, err := state.ExportConfigMap()
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"safety_settings":                           `{"default":"OFF"}`,
		"version_settings":                          `{"default":"v1beta","gemini-1.0-pro":"v1"}`,
		"supported_imagine_models":                  `["gemini-2.0-flash-exp-image-generation","gemini-2.0-flash-exp","gemini-3-pro-image-preview","gemini-3-pro-image","gemini-2.5-flash-image","gemini-3.1-flash-image","gemini-3.1-flash-image-preview"]`,
		"thinking_adapter_enabled":                  "false",
		"thinking_adapter_budget_tokens_percentage": "0.6",
		"function_call_thought_signature_enabled":   "true",
		"remove_function_response_id_enabled":       "true",
	}, exported)

	require.NoError(t, state.UpdateConfigMap(map[string]string{
		"safety_settings":          `{"default":"BLOCK_ONLY_HIGH"}`,
		"thinking_adapter_enabled": "true",
		"unknown_legacy_key":       "ignored",
	}))
	snapshot := state.snapshot()
	assert.Equal(t, map[string]string{"default": "BLOCK_ONLY_HIGH"}, snapshot.SafetySettings)
	assert.True(t, snapshot.ThinkingAdapterEnabled)
	assert.Equal(t, defaultGeminiSettings.VersionSettings, snapshot.VersionSettings)
	assert.Equal(t, defaultGeminiSettings.SupportedImagineModels, snapshot.SupportedImagineModels)
}

func TestManagedGeminiSettingsPreservesNullAndRejectsInvalidCandidates(t *testing.T) {
	state := newManagedGeminiSettings(defaultGeminiSettings)
	require.NoError(t, state.UpdateConfigMap(map[string]string{
		"safety_settings":          "null",
		"version_settings":         "null",
		"supported_imagine_models": "null",
	}))
	snapshot := state.snapshot()
	assert.Nil(t, snapshot.SafetySettings)
	assert.Nil(t, snapshot.VersionSettings)
	assert.Nil(t, snapshot.SupportedImagineModels)

	require.NoError(t, state.UpdateConfigMap(map[string]string{
		"safety_settings":          `{}`,
		"version_settings":         `{}`,
		"supported_imagine_models": `[]`,
	}))
	snapshot = state.snapshot()
	assert.NotNil(t, snapshot.SafetySettings)
	assert.Empty(t, snapshot.SafetySettings)
	assert.NotNil(t, snapshot.VersionSettings)
	assert.Empty(t, snapshot.VersionSettings)
	assert.NotNil(t, snapshot.SupportedImagineModels)
	assert.Empty(t, snapshot.SupportedImagineModels)

	before := state.current.Load()
	for _, update := range []map[string]string{
		{"safety_settings": "not-json"},
		{"version_settings": "true"},
		{"supported_imagine_models": "true"},
		{"thinking_adapter_enabled": "not-a-bool"},
		{"thinking_adapter_budget_tokens_percentage": "NaN"},
	} {
		require.Error(t, state.ValidateConfigMap(update))
		assert.Same(t, before, state.current.Load())
		require.Error(t, state.UpdateConfigMap(update))
		assert.Same(t, before, state.current.Load())
	}
}

func TestManagedGeminiSettingsCopiesInitialAndDetachedSnapshots(t *testing.T) {
	initial := GeminiSettings{
		SafetySettings:         map[string]string{"safety": "before"},
		VersionSettings:        map[string]string{"version": "before"},
		SupportedImagineModels: []string{"before"},
	}
	state := newManagedGeminiSettings(initial)
	initial.SafetySettings["safety"] = "caller-initial-mutation"
	initial.VersionSettings["version"] = "caller-initial-mutation"
	initial.SupportedImagineModels[0] = "caller-initial-mutation"

	snapshot := state.snapshot()
	assert.Equal(t, "before", snapshot.SafetySettings["safety"])
	assert.Equal(t, "before", snapshot.VersionSettings["version"])
	assert.Equal(t, []string{"before"}, snapshot.SupportedImagineModels)

	snapshot.SafetySettings["safety"] = "caller-snapshot-mutation"
	snapshot.VersionSettings["version"] = "caller-snapshot-mutation"
	snapshot.SupportedImagineModels[0] = "caller-snapshot-mutation"
	snapshot.SupportedImagineModels = append(snapshot.SupportedImagineModels, "caller-append")
	assert.Equal(t, "before", state.snapshot().SafetySettings["safety"])
	assert.Equal(t, "before", state.snapshot().VersionSettings["version"])
	assert.Equal(t, []string{"before"}, state.snapshot().SupportedImagineModels)
}

func TestGetGeminiSettingsReturnsDetachedSnapshot(t *testing.T) {
	registered := config.GlobalConfig.Get("gemini")
	require.NotNil(t, registered)
	baseline, err := config.ConfigToMap(registered)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, config.UpdateConfigFromMap(registered, baseline))
	})

	require.NoError(t, config.UpdateConfigFromMap(registered, map[string]string{
		"safety_settings":          `{"default":"BLOCK_ONLY_HIGH"}`,
		"version_settings":         `{"default":"v1"}`,
		"supported_imagine_models": `["one"]`,
	}))
	detached := GetGeminiSettings()
	detached.SafetySettings["default"] = "caller-mutation"
	detached.VersionSettings["default"] = "caller-mutation"
	detached.SupportedImagineModels[0] = "caller-mutation"
	detached.SupportedImagineModels = append(detached.SupportedImagineModels, "caller-append")

	snapshot := geminiSettingsState.snapshot()
	assert.Equal(t, "BLOCK_ONLY_HIGH", snapshot.SafetySettings["default"])
	assert.Equal(t, "v1", snapshot.VersionSettings["default"])
	assert.Equal(t, []string{"one"}, snapshot.SupportedImagineModels)
}

func TestManagedGeminiSettingsPublishesWholeGeneration(t *testing.T) {
	state := newManagedGeminiSettings(GeminiSettings{
		SafetySettings:         map[string]string{"default": "first"},
		VersionSettings:        map[string]string{"default": "first"},
		SupportedImagineModels: []string{"first"},
		ThinkingAdapterEnabled: true,
	})
	first := state.snapshot()
	second := GeminiSettings{
		SafetySettings:         map[string]string{"default": "second"},
		VersionSettings:        map[string]string{"default": "second"},
		SupportedImagineModels: []string{"second", "second-tail"},
	}
	updates := []map[string]string{
		{"safety_settings": `{"default":"first"}`, "version_settings": `{"default":"first"}`, "supported_imagine_models": `["first"]`, "thinking_adapter_enabled": "true"},
		{"safety_settings": `{"default":"second"}`, "version_settings": `{"default":"second"}`, "supported_imagine_models": `["second","second-tail"]`, "thinking_adapter_enabled": "false"},
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

func TestManagedGeminiSettingsConfigManagerLoadAndExport(t *testing.T) {
	manager := config.NewConfigManager()
	state := newManagedGeminiSettings(defaultGeminiSettings)
	manager.Register("gemini", state)

	require.NoError(t, manager.LoadFromDB(map[string]string{
		"gemini.safety_settings":                           `{"default":"BLOCK_ONLY_HIGH"}`,
		"gemini.version_settings":                          `{"default":"v1"}`,
		"gemini.supported_imagine_models":                  `["managed-image"]`,
		"gemini.thinking_adapter_enabled":                  "true",
		"gemini.thinking_adapter_budget_tokens_percentage": "0.4",
		"gemini.function_call_thought_signature_enabled":   "false",
		"gemini.remove_function_response_id_enabled":       "false",
		"gemini.unknown":                                   "ignored",
	}))
	assert.Equal(t, GeminiSettings{
		SafetySettings:                        map[string]string{"default": "BLOCK_ONLY_HIGH"},
		VersionSettings:                       map[string]string{"default": "v1"},
		SupportedImagineModels:                []string{"managed-image"},
		ThinkingAdapterEnabled:                true,
		ThinkingAdapterBudgetTokensPercentage: 0.4,
		FunctionCallThoughtSignatureEnabled:   false,
		RemoveFunctionResponseIdEnabled:       false,
	}, state.snapshot())

	saved := make(map[string]string)
	require.NoError(t, manager.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	assert.Equal(t, map[string]string{
		"gemini.safety_settings":                           `{"default":"BLOCK_ONLY_HIGH"}`,
		"gemini.version_settings":                          `{"default":"v1"}`,
		"gemini.supported_imagine_models":                  `["managed-image"]`,
		"gemini.thinking_adapter_enabled":                  "true",
		"gemini.thinking_adapter_budget_tokens_percentage": "0.4",
		"gemini.function_call_thought_signature_enabled":   "false",
		"gemini.remove_function_response_id_enabled":       "false",
	}, saved)
}

func TestGeminiHelpersPreserveExistingReadSemantics(t *testing.T) {
	registered := config.GlobalConfig.Get("gemini")
	require.NotNil(t, registered)
	baseline, err := config.ConfigToMap(registered)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, config.UpdateConfigFromMap(registered, baseline))
	})

	require.NoError(t, config.UpdateConfigFromMap(registered, map[string]string{
		"safety_settings":          `{"default":"BLOCK_SOME","HARM_CATEGORY_DANGEROUS_CONTENT":"BLOCK_ONLY_HIGH","HARM_CATEGORY_HATE_SPEECH":""}`,
		"version_settings":         `{"default":"v1beta","gemini-1.0-pro":""}`,
		"supported_imagine_models": `["image"]`,
	}))
	assert.Equal(t, "BLOCK_ONLY_HIGH", GetGeminiSafetySetting("HARM_CATEGORY_DANGEROUS_CONTENT"))
	assert.Equal(t, "BLOCK_SOME", GetGeminiSafetySetting("HARM_CATEGORY_HATE_SPEECH"))
	assert.Equal(t, "BLOCK_SOME", GetGeminiSafetySetting("other"))
	assert.Equal(t, "", GetGeminiVersionSetting("gemini-1.0-pro"))
	assert.Equal(t, "v1beta", GetGeminiVersionSetting("other"))
	assert.True(t, IsGeminiModelSupportImagine("image"))
	assert.False(t, IsGeminiModelSupportImagine("image-extra"))

	require.NoError(t, config.UpdateConfigFromMap(registered, map[string]string{
		"safety_settings":  "null",
		"version_settings": "null",
	}))
	assert.Equal(t, defaultGeminiSafetySetting, GetGeminiSafetySetting("other"))
	assert.Equal(t, "", GetGeminiVersionSetting("other"))
}

func TestValidateGeminiSafetySettings(t *testing.T) {
	for _, value := range []string{`{}`, `{"default":""}`, `{"HARM_CATEGORY_HATE_SPEECH":""}`, `{"default":"OFF"}`, `{"default":"BLOCK_NONE"}`, `{"default":"BLOCK_ONLY_HIGH"}`, `{"default":"BLOCK_MEDIUM_AND_ABOVE"}`, `{"default":"BLOCK_LOW_AND_ABOVE"}`, `{"default":"HARM_BLOCK_THRESHOLD_UNSPECIFIED"}`} {
		require.NoError(t, ValidateGeminiSafetySettings(value), value)
	}
	for _, value := range []string{`null`, `[]`, `{"default":1}`, `{"default":"BLOCK_SOME"}`, `{"default":" off "}`, `{"default":`} {
		assert.Error(t, ValidateGeminiSafetySettings(value), value)
	}
}

func TestValidateGeminiThinkingAdapterBudgetTokensPercentage(t *testing.T) {
	for _, value := range []float64{-1, 0, 0.001, 1.01, math.NaN(), math.Inf(1)} {
		require.Error(t, ValidateGeminiThinkingAdapterBudgetTokensPercentage(value))
	}
	for _, value := range []float64{0.002, 0.6, 1} {
		require.NoError(t, ValidateGeminiThinkingAdapterBudgetTokensPercentage(value))
	}
}
