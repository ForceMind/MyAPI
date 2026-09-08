package model_setting

import (
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/ForceMind/MyAPI/setting/config"
)

type ChatCompletionsToResponsesPolicy struct {
	Enabled       bool     `json:"enabled"`
	AllChannels   bool     `json:"all_channels"`
	ChannelIDs    []int    `json:"channel_ids,omitempty"`
	ChannelTypes  []int    `json:"channel_types,omitempty"`
	ModelPatterns []string `json:"model_patterns,omitempty"`
}

func (p ChatCompletionsToResponsesPolicy) IsChannelEnabled(channelID int, channelType int) bool {
	if !p.Enabled {
		return false
	}
	if p.AllChannels {
		return true
	}

	if channelID > 0 && len(p.ChannelIDs) > 0 && slices.Contains(p.ChannelIDs, channelID) {
		return true
	}
	if channelType > 0 && len(p.ChannelTypes) > 0 && slices.Contains(p.ChannelTypes, channelType) {
		return true
	}
	return false
}

type GlobalSettings struct {
	PassThroughRequestEnabled        bool                             `json:"pass_through_request_enabled"`
	ThinkingModelBlacklist           []string                         `json:"thinking_model_blacklist"`
	ChatCompletionsToResponsesPolicy ChatCompletionsToResponsesPolicy `json:"chat_completions_to_responses_policy"`
}

// 默认配置
var defaultOpenaiSettings = GlobalSettings{
	PassThroughRequestEnabled: false,
	ThinkingModelBlacklist: []string{
		"moonshotai/kimi-k2-thinking",
		"kimi-k2-thinking",
	},
	ChatCompletionsToResponsesPolicy: ChatCompletionsToResponsesPolicy{
		Enabled:     false,
		AllChannels: true,
	},
}

// globalSettingsGeneration is immutable after publication. Global settings
// contain several slices, so readers must not observe an in-place update.
type globalSettingsGeneration struct {
	settings GlobalSettings
}

// managedGlobalSettings owns the synchronized runtime snapshot registered
// with the generic config manager.
type managedGlobalSettings struct {
	writeMutex sync.Mutex
	current    atomic.Pointer[globalSettingsGeneration]
}

func cloneGlobalSettings(settings GlobalSettings) GlobalSettings {
	clone := GlobalSettings{
		PassThroughRequestEnabled: settings.PassThroughRequestEnabled,
		ChatCompletionsToResponsesPolicy: ChatCompletionsToResponsesPolicy{
			Enabled:     settings.ChatCompletionsToResponsesPolicy.Enabled,
			AllChannels: settings.ChatCompletionsToResponsesPolicy.AllChannels,
		},
	}
	if settings.ThinkingModelBlacklist != nil {
		clone.ThinkingModelBlacklist = append([]string{}, settings.ThinkingModelBlacklist...)
	}
	if settings.ChatCompletionsToResponsesPolicy.ChannelIDs != nil {
		clone.ChatCompletionsToResponsesPolicy.ChannelIDs = append([]int{}, settings.ChatCompletionsToResponsesPolicy.ChannelIDs...)
	}
	if settings.ChatCompletionsToResponsesPolicy.ChannelTypes != nil {
		clone.ChatCompletionsToResponsesPolicy.ChannelTypes = append([]int{}, settings.ChatCompletionsToResponsesPolicy.ChannelTypes...)
	}
	if settings.ChatCompletionsToResponsesPolicy.ModelPatterns != nil {
		clone.ChatCompletionsToResponsesPolicy.ModelPatterns = append([]string{}, settings.ChatCompletionsToResponsesPolicy.ModelPatterns...)
	}
	return clone
}

func newManagedGlobalSettings(initial GlobalSettings) *managedGlobalSettings {
	state := &managedGlobalSettings{}
	state.current.Store(&globalSettingsGeneration{settings: cloneGlobalSettings(initial)})
	return state
}

func (s *managedGlobalSettings) snapshot() GlobalSettings {
	if s != nil {
		if current := s.current.Load(); current != nil {
			return cloneGlobalSettings(current.settings)
		}
	}
	return cloneGlobalSettings(defaultOpenaiSettings)
}

func (s *managedGlobalSettings) candidate(values map[string]string) (GlobalSettings, error) {
	candidate := s.snapshot()
	if err := config.UpdateConfigFromMap(&candidate, values); err != nil {
		return GlobalSettings{}, err
	}
	return candidate, nil
}

func (s *managedGlobalSettings) DiagnosticSchema() any {
	return &GlobalSettings{}
}

func (s *managedGlobalSettings) ExportConfigMap() (map[string]string, error) {
	settings := s.snapshot()
	return config.ConfigToMap(&settings)
}

func (s *managedGlobalSettings) ValidateConfigMap(values map[string]string) error {
	_, err := s.candidate(values)
	return err
}

func (s *managedGlobalSettings) UpdateConfigMap(values map[string]string) error {
	s.writeMutex.Lock()
	defer s.writeMutex.Unlock()

	candidate, err := s.candidate(values)
	if err != nil {
		return err
	}
	s.current.Store(&globalSettingsGeneration{settings: cloneGlobalSettings(candidate)})
	return nil
}

var globalSettingsState = newManagedGlobalSettings(defaultOpenaiSettings)

var _ config.ValidatingMapConfig = (*managedGlobalSettings)(nil)

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("global", globalSettingsState)
}

func GetGlobalSettings() *GlobalSettings {
	settings := globalSettingsState.snapshot()
	return &settings
}

// ShouldPreserveThinkingSuffix 判断模型是否配置为保留 thinking/-nothinking/-low/-high/-medium 后缀
func ShouldPreserveThinkingSuffix(modelName string) bool {
	return GetGlobalSettings().ShouldPreserveThinkingSuffix(modelName)
}

// ShouldPreserveThinkingSuffix reports whether this settings snapshot keeps
// the model's thinking suffix using the existing exact-match rule.
func (settings GlobalSettings) ShouldPreserveThinkingSuffix(modelName string) bool {
	target := strings.TrimSpace(modelName)
	if target == "" {
		return false
	}

	for _, entry := range settings.ThinkingModelBlacklist {
		if strings.TrimSpace(entry) == target {
			return true
		}
	}
	return false
}
