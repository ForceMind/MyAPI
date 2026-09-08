package system_setting

import (
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/ForceMind/MyAPI/setting/config"
)

type DiscordSettings struct {
	Enabled      bool   `json:"enabled"`
	ClientId     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

// 默认配置
var defaultDiscordSettings = DiscordSettings{}

// discordSettingsGeneration is immutable after publication. OAuth settings
// are persisted as independent option keys, but readers must not observe a
// partially rewritten struct during an option load or update.
type discordSettingsGeneration struct {
	settings DiscordSettings
}

// managedDiscordSettings owns the synchronized runtime settings registered
// with the generic config manager.
type managedDiscordSettings struct {
	writeMutex sync.Mutex
	current    atomic.Pointer[discordSettingsGeneration]
}

func newManagedDiscordSettings(initial DiscordSettings) *managedDiscordSettings {
	settings := &managedDiscordSettings{}
	settings.current.Store(&discordSettingsGeneration{settings: initial})
	return settings
}

func (s *managedDiscordSettings) snapshot() DiscordSettings {
	if s != nil {
		if current := s.current.Load(); current != nil {
			return current.settings
		}
	}
	return defaultDiscordSettings
}

func (s *managedDiscordSettings) detachedSnapshot() *DiscordSettings {
	settings := s.snapshot()
	return &settings
}

func (s *managedDiscordSettings) candidate(values map[string]string) (DiscordSettings, error) {
	candidate := s.snapshot()
	if err := config.UpdateConfigFromMap(&candidate, values); err != nil {
		return DiscordSettings{}, err
	}
	return candidate, nil
}

func (s *managedDiscordSettings) ExportConfigMap() (map[string]string, error) {
	settings := s.snapshot()
	return map[string]string{
		"enabled":       strconv.FormatBool(settings.Enabled),
		"client_id":     settings.ClientId,
		"client_secret": settings.ClientSecret,
	}, nil
}

func (s *managedDiscordSettings) ValidateConfigMap(values map[string]string) error {
	_, err := s.candidate(values)
	return err
}

func (s *managedDiscordSettings) UpdateConfigMap(values map[string]string) error {
	s.writeMutex.Lock()
	defer s.writeMutex.Unlock()

	candidate, err := s.candidate(values)
	if err != nil {
		return err
	}
	s.current.Store(&discordSettingsGeneration{settings: candidate})
	return nil
}

var discordSettingsState = newManagedDiscordSettings(defaultDiscordSettings)

var _ config.ValidatingMapConfig = (*managedDiscordSettings)(nil)

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("discord", discordSettingsState)
}

// GetDiscordSettings returns a detached copy of one immutable runtime
// generation. Mutating the returned value never updates the live setting.
func GetDiscordSettings() *DiscordSettings {
	return discordSettingsState.detachedSnapshot()
}
