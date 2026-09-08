package model_setting

import (
	"fmt"
	"math"
	"sync"
	"sync/atomic"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/setting/config"
)

const defaultGeminiSafetySetting = "OFF"

var validGeminiSafetySettings = map[string]struct{}{
	"OFF":                              {},
	"BLOCK_NONE":                       {},
	"BLOCK_ONLY_HIGH":                  {},
	"BLOCK_MEDIUM_AND_ABOVE":           {},
	"BLOCK_LOW_AND_ABOVE":              {},
	"HARM_BLOCK_THRESHOLD_UNSPECIFIED": {},
}

// GeminiSettings defines Gemini model configuration. 注意bool要以enabled结尾才可以生效编辑
type GeminiSettings struct {
	SafetySettings                        map[string]string `json:"safety_settings"`
	VersionSettings                       map[string]string `json:"version_settings"`
	SupportedImagineModels                []string          `json:"supported_imagine_models"`
	ThinkingAdapterEnabled                bool              `json:"thinking_adapter_enabled"`
	ThinkingAdapterBudgetTokensPercentage float64           `json:"thinking_adapter_budget_tokens_percentage"`
	FunctionCallThoughtSignatureEnabled   bool              `json:"function_call_thought_signature_enabled"`
	RemoveFunctionResponseIdEnabled       bool              `json:"remove_function_response_id_enabled"`
}

// 默认配置
var defaultGeminiSettings = GeminiSettings{
	SafetySettings: map[string]string{
		"default": defaultGeminiSafetySetting,
	},
	VersionSettings: map[string]string{
		"default":        "v1beta",
		"gemini-1.0-pro": "v1",
	},
	SupportedImagineModels: []string{
		"gemini-2.0-flash-exp-image-generation",
		"gemini-2.0-flash-exp",
		"gemini-3-pro-image-preview",
		"gemini-3-pro-image",
		"gemini-2.5-flash-image",
		"gemini-3.1-flash-image",
		"gemini-3.1-flash-image-preview",
	},
	ThinkingAdapterEnabled:                false,
	ThinkingAdapterBudgetTokensPercentage: 0.6,
	FunctionCallThoughtSignatureEnabled:   true,
	RemoveFunctionResponseIdEnabled:       true,
}

// geminiSettingsGeneration is immutable after publication. Gemini settings
// include maps and a slice, so readers must not observe an in-place update.
type geminiSettingsGeneration struct {
	settings GeminiSettings
}

// managedGeminiSettings owns the synchronized runtime snapshot registered
// with the generic config manager.
type managedGeminiSettings struct {
	writeMutex sync.Mutex
	current    atomic.Pointer[geminiSettingsGeneration]
}

func cloneGeminiSettings(settings GeminiSettings) GeminiSettings {
	clone := GeminiSettings{
		ThinkingAdapterEnabled:                settings.ThinkingAdapterEnabled,
		ThinkingAdapterBudgetTokensPercentage: settings.ThinkingAdapterBudgetTokensPercentage,
		FunctionCallThoughtSignatureEnabled:   settings.FunctionCallThoughtSignatureEnabled,
		RemoveFunctionResponseIdEnabled:       settings.RemoveFunctionResponseIdEnabled,
	}
	if settings.SafetySettings != nil {
		clone.SafetySettings = make(map[string]string, len(settings.SafetySettings))
		for key, value := range settings.SafetySettings {
			clone.SafetySettings[key] = value
		}
	}
	if settings.VersionSettings != nil {
		clone.VersionSettings = make(map[string]string, len(settings.VersionSettings))
		for key, value := range settings.VersionSettings {
			clone.VersionSettings[key] = value
		}
	}
	if settings.SupportedImagineModels != nil {
		clone.SupportedImagineModels = append([]string{}, settings.SupportedImagineModels...)
	}
	return clone
}

func newManagedGeminiSettings(initial GeminiSettings) *managedGeminiSettings {
	state := &managedGeminiSettings{}
	state.current.Store(&geminiSettingsGeneration{settings: cloneGeminiSettings(initial)})
	return state
}

func (s *managedGeminiSettings) snapshot() GeminiSettings {
	if s != nil {
		if current := s.current.Load(); current != nil {
			return cloneGeminiSettings(current.settings)
		}
	}
	return cloneGeminiSettings(defaultGeminiSettings)
}

func (s *managedGeminiSettings) candidate(values map[string]string) (GeminiSettings, error) {
	candidate := s.snapshot()
	if err := config.UpdateConfigFromMap(&candidate, values); err != nil {
		return GeminiSettings{}, err
	}
	return candidate, nil
}

func (s *managedGeminiSettings) ExportConfigMap() (map[string]string, error) {
	settings := s.snapshot()
	return config.ConfigToMap(&settings)
}

func (s *managedGeminiSettings) ValidateConfigMap(values map[string]string) error {
	_, err := s.candidate(values)
	return err
}

func (s *managedGeminiSettings) UpdateConfigMap(values map[string]string) error {
	s.writeMutex.Lock()
	defer s.writeMutex.Unlock()

	candidate, err := s.candidate(values)
	if err != nil {
		return err
	}
	s.current.Store(&geminiSettingsGeneration{settings: cloneGeminiSettings(candidate)})
	return nil
}

var geminiSettingsState = newManagedGeminiSettings(defaultGeminiSettings)

var _ config.ValidatingMapConfig = (*managedGeminiSettings)(nil)

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("gemini", geminiSettingsState)
}

// GetGeminiSettings 获取Gemini配置
func GetGeminiSettings() *GeminiSettings {
	settings := geminiSettingsState.snapshot()
	return &settings
}

// GetGeminiSafetySetting 获取安全设置
func GetGeminiSafetySetting(key string) string {
	return geminiSettingsState.snapshot().SafetySetting(key)
}

// SafetySetting returns the effective safety threshold from this settings
// snapshot. Empty exact/default values retain the legacy fallback to OFF.
func (settings GeminiSettings) SafetySetting(key string) string {
	if value := settings.SafetySettings[key]; value != "" {
		return value
	}
	if value := settings.SafetySettings["default"]; value != "" {
		return value
	}
	return defaultGeminiSafetySetting
}

// ValidateGeminiSafetySettings validates the JSON persisted by the option API.
// Empty values remain valid because read-time fallback returns the default.
func ValidateGeminiSafetySettings(value string) error {
	var settings map[string]string
	if err := common.UnmarshalJsonStr(value, &settings); err != nil {
		return fmt.Errorf("Gemini safety settings must be a JSON string map: %w", err)
	}
	if settings == nil {
		return fmt.Errorf("Gemini safety settings must be a JSON string map")
	}
	for category, threshold := range settings {
		if threshold == "" {
			continue
		}
		if _, ok := validGeminiSafetySettings[threshold]; !ok {
			return fmt.Errorf("invalid Gemini safety threshold %q for %q", threshold, category)
		}
	}
	return nil
}

// ValidateGeminiThinkingAdapterBudgetTokensPercentage keeps the derived
// thinking budget finite and within the range accepted by the settings UI.
func ValidateGeminiThinkingAdapterBudgetTokensPercentage(value float64) error {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0.002 || value > 1 {
		return fmt.Errorf("Gemini thinking adapter budget percentage must be finite and in [0.002,1], got %v", value)
	}
	return nil
}

// GetGeminiVersionSetting 获取版本设置
func GetGeminiVersionSetting(key string) string {
	settings := geminiSettingsState.snapshot()
	if value, ok := settings.VersionSettings[key]; ok {
		return value
	}
	return settings.VersionSettings["default"]
}

func IsGeminiModelSupportImagine(model string) bool {
	return geminiSettingsState.snapshot().SupportsImagine(model)
}

// SupportsImagine reports whether this settings snapshot enables the model's
// image capability using the legacy exact-match rule.
func (settings GeminiSettings) SupportsImagine(model string) bool {
	for _, v := range settings.SupportedImagineModels {
		if v == model {
			return true
		}
	}
	return false
}
