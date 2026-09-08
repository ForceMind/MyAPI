package operation_setting

import (
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/ForceMind/MyAPI/setting/config"
)

// CheckinSetting 签到功能配置
type CheckinSetting struct {
	Enabled  bool `json:"enabled"`   // 是否启用签到功能
	MinQuota int  `json:"min_quota"` // 签到最小额度奖励
	MaxQuota int  `json:"max_quota"` // 签到最大额度奖励
}

// 默认配置
var defaultCheckinSetting = CheckinSetting{
	Enabled:  false, // 默认关闭
	MinQuota: 1000,  // 默认最小额度 1000 (约 0.002 USD)
	MaxQuota: 10000, // 默认最大额度 10000 (约 0.02 USD)
}

// checkinSettingGeneration is immutable after publication. A check-in must
// read one complete generation so enablement and its quota range cannot be
// observed from different configuration updates.
type checkinSettingGeneration struct {
	setting CheckinSetting
}

// managedCheckinSetting owns the runtime snapshot registered with the generic
// config manager. GetCheckinSetting keeps its legacy pointer API but returns a
// detached copy, so callers cannot mutate the published generation.
type managedCheckinSetting struct {
	writeMutex sync.Mutex
	current    atomic.Pointer[checkinSettingGeneration]
}

func newManagedCheckinSetting(initial CheckinSetting) *managedCheckinSetting {
	setting := &managedCheckinSetting{}
	setting.current.Store(&checkinSettingGeneration{setting: initial})
	return setting
}

func (s *managedCheckinSetting) snapshot() CheckinSetting {
	if s != nil {
		if current := s.current.Load(); current != nil {
			return current.setting
		}
	}
	return defaultCheckinSetting
}

func (s *managedCheckinSetting) candidate(values map[string]string) (CheckinSetting, error) {
	candidate := s.snapshot()
	if err := config.UpdateConfigFromMap(&candidate, values); err != nil {
		return CheckinSetting{}, err
	}
	return candidate, nil
}

func (s *managedCheckinSetting) ExportConfigMap() (map[string]string, error) {
	setting := s.snapshot()
	return map[string]string{
		"enabled":   strconv.FormatBool(setting.Enabled),
		"min_quota": strconv.Itoa(setting.MinQuota),
		"max_quota": strconv.Itoa(setting.MaxQuota),
	}, nil
}

func (s *managedCheckinSetting) ValidateConfigMap(values map[string]string) error {
	_, err := s.candidate(values)
	return err
}

func (s *managedCheckinSetting) UpdateConfigMap(values map[string]string) error {
	s.writeMutex.Lock()
	defer s.writeMutex.Unlock()

	candidate, err := s.candidate(values)
	if err != nil {
		return err
	}
	s.current.Store(&checkinSettingGeneration{setting: candidate})
	return nil
}

var checkinSettingState = newManagedCheckinSetting(defaultCheckinSetting)

var _ config.ValidatingMapConfig = (*managedCheckinSetting)(nil)

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("checkin_setting", checkinSettingState)
}

// GetCheckinSetting 获取签到配置
func GetCheckinSetting() *CheckinSetting {
	setting := checkinSettingState.snapshot()
	return &setting
}

// IsCheckinEnabled 是否启用签到功能
func IsCheckinEnabled() bool {
	setting := checkinSettingState.snapshot()
	return setting.Enabled
}

// GetCheckinQuotaRange 获取签到额度范围
func GetCheckinQuotaRange() (min, max int) {
	setting := checkinSettingState.snapshot()
	return setting.MinQuota, setting.MaxQuota
}
