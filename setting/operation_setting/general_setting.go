package operation_setting

import (
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/ForceMind/MyAPI/setting/config"
)

// 额度展示类型
const (
	QuotaDisplayTypeUSD    = "USD"
	QuotaDisplayTypeCNY    = "CNY"
	QuotaDisplayTypeTokens = "TOKENS"
	QuotaDisplayTypeCustom = "CUSTOM"
)

type GeneralSetting struct {
	DocsLink            string `json:"docs_link"`
	PingIntervalEnabled bool   `json:"ping_interval_enabled"`
	PingIntervalSeconds int    `json:"ping_interval_seconds"`
	// 当前站点额度展示类型：USD / CNY / TOKENS
	QuotaDisplayType string `json:"quota_display_type"`
	// 自定义货币符号，用于 CUSTOM 展示类型
	CustomCurrencySymbol string `json:"custom_currency_symbol"`
	// 自定义货币与美元汇率（1 USD = X Custom）
	CustomCurrencyExchangeRate float64 `json:"custom_currency_exchange_rate"`
}

var defaultGeneralSetting = GeneralSetting{
	DocsLink:                   "https://github.com/ForceMind/MyAPI#readme",
	PingIntervalEnabled:        false,
	PingIntervalSeconds:        60,
	QuotaDisplayType:           QuotaDisplayTypeUSD,
	CustomCurrencySymbol:       "¤",
	CustomCurrencyExchangeRate: 1.0,
}

// generalSettingGeneration is immutable after publication. The persisted
// fields are deliberately kept unconstrained here so existing reader-side
// fallbacks and legacy option values retain their current behavior.
type generalSettingGeneration struct {
	setting GeneralSetting
}

// managedGeneralSetting owns the runtime snapshot registered with the generic
// config manager. Readers receive copied settings and can never mutate the
// published generation through the legacy pointer-returning getter.
type managedGeneralSetting struct {
	writeMutex sync.Mutex
	current    atomic.Pointer[generalSettingGeneration]
}

func newManagedGeneralSetting(initial GeneralSetting) *managedGeneralSetting {
	setting := &managedGeneralSetting{}
	setting.current.Store(&generalSettingGeneration{setting: initial})
	return setting
}

func (s *managedGeneralSetting) snapshot() GeneralSetting {
	if s != nil {
		if current := s.current.Load(); current != nil {
			return current.setting
		}
	}
	return defaultGeneralSetting
}

func (s *managedGeneralSetting) candidate(values map[string]string) (GeneralSetting, error) {
	candidate := s.snapshot()
	if err := config.UpdateConfigFromMap(&candidate, values); err != nil {
		return GeneralSetting{}, err
	}
	return candidate, nil
}

func (s *managedGeneralSetting) ExportConfigMap() (map[string]string, error) {
	setting := s.snapshot()
	return map[string]string{
		"docs_link":                     setting.DocsLink,
		"ping_interval_enabled":         strconv.FormatBool(setting.PingIntervalEnabled),
		"ping_interval_seconds":         strconv.Itoa(setting.PingIntervalSeconds),
		"quota_display_type":            setting.QuotaDisplayType,
		"custom_currency_symbol":        setting.CustomCurrencySymbol,
		"custom_currency_exchange_rate": strconv.FormatFloat(setting.CustomCurrencyExchangeRate, 'f', -1, 64),
	}, nil
}

func (s *managedGeneralSetting) ValidateConfigMap(values map[string]string) error {
	_, err := s.candidate(values)
	return err
}

func (s *managedGeneralSetting) UpdateConfigMap(values map[string]string) error {
	s.writeMutex.Lock()
	defer s.writeMutex.Unlock()

	candidate, err := s.candidate(values)
	if err != nil {
		return err
	}
	s.current.Store(&generalSettingGeneration{setting: candidate})
	return nil
}

var generalSettingState = newManagedGeneralSetting(defaultGeneralSetting)

var _ config.ValidatingMapConfig = (*managedGeneralSetting)(nil)

func init() {
	config.GlobalConfig.Register("general_setting", generalSettingState)
}

func GetGeneralSetting() *GeneralSetting {
	setting := generalSettingState.snapshot()
	return &setting
}

// GetQuotaDisplayType 返回额度展示类型
func GetQuotaDisplayType() string {
	setting := generalSettingState.snapshot()
	return setting.QuotaDisplayType
}
