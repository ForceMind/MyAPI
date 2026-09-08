package console_setting

import (
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/ForceMind/MyAPI/setting/config"
)

type ConsoleSetting struct {
	ApiInfo              string `json:"api_info"`              // 控制台 API 信息 (JSON 数组字符串)
	UptimeKumaGroups     string `json:"uptime_kuma_groups"`    // Uptime Kuma 分组配置 (JSON 数组字符串)
	Announcements        string `json:"announcements"`         // 系统公告 (JSON 数组字符串)
	FAQ                  string `json:"faq"`                   // 常见问题 (JSON 数组字符串)
	ApiInfoEnabled       bool   `json:"api_info_enabled"`      // 是否启用 API 信息面板
	UptimeKumaEnabled    bool   `json:"uptime_kuma_enabled"`   // 是否启用 Uptime Kuma 面板
	AnnouncementsEnabled bool   `json:"announcements_enabled"` // 是否启用系统公告面板
	FAQEnabled           bool   `json:"faq_enabled"`           // 是否启用常见问答面板
}

// 默认配置
var defaultConsoleSetting = ConsoleSetting{
	ApiInfo:              "",
	UptimeKumaGroups:     "",
	Announcements:        "",
	FAQ:                  "",
	ApiInfoEnabled:       true,
	UptimeKumaEnabled:    true,
	AnnouncementsEnabled: true,
	FAQEnabled:           true,
}

// consoleSettingGeneration is immutable after publication. Console content is
// configured as independent option keys, but readers must not observe fields
// from a struct being rewritten by the generic reflection-based config loader.
type consoleSettingGeneration struct {
	setting ConsoleSetting
}

// managedConsoleSetting owns the synchronized runtime snapshot registered
// with the generic config manager.
type managedConsoleSetting struct {
	writeMutex sync.Mutex
	current    atomic.Pointer[consoleSettingGeneration]
}

func newManagedConsoleSetting(initial ConsoleSetting) *managedConsoleSetting {
	setting := &managedConsoleSetting{}
	setting.current.Store(&consoleSettingGeneration{setting: initial})
	return setting
}

func (s *managedConsoleSetting) snapshot() ConsoleSetting {
	if s != nil {
		if current := s.current.Load(); current != nil {
			return current.setting
		}
	}
	return defaultConsoleSetting
}

func (s *managedConsoleSetting) detachedSnapshot() *ConsoleSetting {
	setting := s.snapshot()
	return &setting
}

func (s *managedConsoleSetting) candidate(values map[string]string) (ConsoleSetting, error) {
	candidate := s.snapshot()
	if err := config.UpdateConfigFromMap(&candidate, values); err != nil {
		return ConsoleSetting{}, err
	}
	return candidate, nil
}

func (s *managedConsoleSetting) ExportConfigMap() (map[string]string, error) {
	setting := s.snapshot()
	return map[string]string{
		"api_info":              setting.ApiInfo,
		"uptime_kuma_groups":    setting.UptimeKumaGroups,
		"announcements":         setting.Announcements,
		"faq":                   setting.FAQ,
		"api_info_enabled":      strconv.FormatBool(setting.ApiInfoEnabled),
		"uptime_kuma_enabled":   strconv.FormatBool(setting.UptimeKumaEnabled),
		"announcements_enabled": strconv.FormatBool(setting.AnnouncementsEnabled),
		"faq_enabled":           strconv.FormatBool(setting.FAQEnabled),
	}, nil
}

func (s *managedConsoleSetting) ValidateConfigMap(values map[string]string) error {
	_, err := s.candidate(values)
	return err
}

func (s *managedConsoleSetting) UpdateConfigMap(values map[string]string) error {
	s.writeMutex.Lock()
	defer s.writeMutex.Unlock()

	candidate, err := s.candidate(values)
	if err != nil {
		return err
	}
	s.current.Store(&consoleSettingGeneration{setting: candidate})
	return nil
}

var consoleSettingState = newManagedConsoleSetting(defaultConsoleSetting)

var _ config.ValidatingMapConfig = (*managedConsoleSetting)(nil)

func init() {
	// 注册到全局配置管理器，键名为 console_setting
	config.GlobalConfig.Register("console_setting", consoleSettingState)
}

// GetConsoleSetting returns a detached copy of one immutable runtime
// generation. Mutating the returned value never updates the live setting.
func GetConsoleSetting() *ConsoleSetting {
	return consoleSettingState.detachedSnapshot()
}
