package operation_setting

import (
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/ForceMind/MyAPI/setting/config"
)

type QuotaSetting struct {
	EnableFreeModelPreConsume bool `json:"enable_free_model_pre_consume"` // 是否对免费模型启用预消耗
}

var defaultQuotaSetting = QuotaSetting{
	EnableFreeModelPreConsume: true,
}

// quotaSettingGeneration is immutable after publication. A quota decision
// must read its complete configuration from one generation.
type quotaSettingGeneration struct {
	setting QuotaSetting
}

// managedQuotaSetting owns the runtime snapshot registered with the generic
// config manager. Readers receive detached copies through the legacy getter.
type managedQuotaSetting struct {
	writeMutex sync.Mutex
	current    atomic.Pointer[quotaSettingGeneration]
}

func newManagedQuotaSetting(initial QuotaSetting) *managedQuotaSetting {
	state := &managedQuotaSetting{}
	state.current.Store(&quotaSettingGeneration{setting: initial})
	return state
}

func (s *managedQuotaSetting) snapshot() QuotaSetting {
	if s != nil {
		if current := s.current.Load(); current != nil {
			return current.setting
		}
	}
	return defaultQuotaSetting
}

func (s *managedQuotaSetting) candidate(values map[string]string) (QuotaSetting, error) {
	candidate := s.snapshot()
	if err := config.UpdateConfigFromMap(&candidate, values); err != nil {
		return QuotaSetting{}, err
	}
	return candidate, nil
}

func (s *managedQuotaSetting) ExportConfigMap() (map[string]string, error) {
	setting := s.snapshot()
	return map[string]string{
		"enable_free_model_pre_consume": strconv.FormatBool(setting.EnableFreeModelPreConsume),
	}, nil
}

func (s *managedQuotaSetting) ValidateConfigMap(values map[string]string) error {
	_, err := s.candidate(values)
	return err
}

func (s *managedQuotaSetting) UpdateConfigMap(values map[string]string) error {
	s.writeMutex.Lock()
	defer s.writeMutex.Unlock()

	candidate, err := s.candidate(values)
	if err != nil {
		return err
	}
	s.current.Store(&quotaSettingGeneration{setting: candidate})
	return nil
}

var quotaSettingState = newManagedQuotaSetting(defaultQuotaSetting)

var _ config.ValidatingMapConfig = (*managedQuotaSetting)(nil)

func init() {
	config.GlobalConfig.Register("quota_setting", quotaSettingState)
}

func GetQuotaSetting() *QuotaSetting {
	setting := quotaSettingState.snapshot()
	return &setting
}
