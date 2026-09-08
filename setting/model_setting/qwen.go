package model_setting

import (
	"strings"
	"sync"
	"sync/atomic"

	"github.com/ForceMind/MyAPI/setting/config"
)

// QwenSettings defines Qwen model configuration. 注意bool要以enabled结尾才可以生效编辑
type QwenSettings struct {
	SyncImageModels []string `json:"sync_image_models"`
}

// 默认配置
var defaultQwenSettings = QwenSettings{
	SyncImageModels: []string{
		"z-image",
		"qwen-image",
		"wan2.6",
		"wan2.7",
		"qwen-image-edit",
		"qwen-image-edit-max",
		"qwen-image-edit-max-2026-01-16",
		"qwen-image-edit-plus",
		"qwen-image-edit-plus-2025-12-15",
		"qwen-image-edit-plus-2025-10-30",
	},
}

// qwenSettingsGeneration is immutable after publication. The configured model
// patterns determine whether Ali image requests use the synchronous or
// asynchronous upstream protocol, so readers must see one complete list.
type qwenSettingsGeneration struct {
	settings QwenSettings
}

// managedQwenSettings owns the synchronized runtime snapshot registered with
// the generic config manager.
type managedQwenSettings struct {
	writeMutex sync.Mutex
	current    atomic.Pointer[qwenSettingsGeneration]
}

func cloneQwenSettings(settings QwenSettings) QwenSettings {
	if settings.SyncImageModels == nil {
		return QwenSettings{}
	}
	return QwenSettings{
		SyncImageModels: append([]string{}, settings.SyncImageModels...),
	}
}

func newManagedQwenSettings(initial QwenSettings) *managedQwenSettings {
	state := &managedQwenSettings{}
	state.current.Store(&qwenSettingsGeneration{settings: cloneQwenSettings(initial)})
	return state
}

func (s *managedQwenSettings) snapshot() QwenSettings {
	if s != nil {
		if current := s.current.Load(); current != nil {
			return cloneQwenSettings(current.settings)
		}
	}
	return cloneQwenSettings(defaultQwenSettings)
}

func (s *managedQwenSettings) candidate(values map[string]string) (QwenSettings, error) {
	candidate := s.snapshot()
	if err := config.UpdateConfigFromMap(&candidate, values); err != nil {
		return QwenSettings{}, err
	}
	return candidate, nil
}

func (s *managedQwenSettings) ExportConfigMap() (map[string]string, error) {
	settings := s.snapshot()
	return config.ConfigToMap(&settings)
}

func (s *managedQwenSettings) ValidateConfigMap(values map[string]string) error {
	_, err := s.candidate(values)
	return err
}

func (s *managedQwenSettings) UpdateConfigMap(values map[string]string) error {
	s.writeMutex.Lock()
	defer s.writeMutex.Unlock()

	candidate, err := s.candidate(values)
	if err != nil {
		return err
	}
	s.current.Store(&qwenSettingsGeneration{settings: cloneQwenSettings(candidate)})
	return nil
}

var qwenSettingsState = newManagedQwenSettings(defaultQwenSettings)

var _ config.ValidatingMapConfig = (*managedQwenSettings)(nil)

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("qwen", qwenSettingsState)
}

// GetQwenSettings
func GetQwenSettings() *QwenSettings {
	settings := qwenSettingsState.snapshot()
	return &settings
}

// IsSyncImageModel
func IsSyncImageModel(model string) bool {
	settings := qwenSettingsState.snapshot()
	for _, m := range settings.SyncImageModels {
		if strings.Contains(model, m) {
			return true
		}
	}
	return false
}
