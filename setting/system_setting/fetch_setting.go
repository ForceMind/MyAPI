package system_setting

import (
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/setting/config"
)

type FetchSetting struct {
	EnableSSRFProtection   bool     `json:"enable_ssrf_protection"` // 是否启用SSRF防护
	AllowPrivateIp         bool     `json:"allow_private_ip"`
	DomainFilterMode       bool     `json:"domain_filter_mode"`         // 域名过滤模式，true: 白名单模式，false: 黑名单模式
	IpFilterMode           bool     `json:"ip_filter_mode"`             // IP过滤模式，true: 白名单模式，false: 黑名单模式
	DomainList             []string `json:"domain_list"`                // domain format, e.g. example.com, *.example.com
	IpList                 []string `json:"ip_list"`                    // CIDR format
	AllowedPorts           []string `json:"allowed_ports"`              // port range format, e.g. 80, 443, 8000-9000
	ApplyIPFilterForDomain bool     `json:"apply_ip_filter_for_domain"` // 对域名启用IP过滤（实验性）
}

var defaultFetchSetting = FetchSetting{
	EnableSSRFProtection:   true, // 默认开启SSRF防护
	AllowPrivateIp:         false,
	DomainFilterMode:       false,
	IpFilterMode:           false,
	DomainList:             []string{},
	IpList:                 []string{},
	AllowedPorts:           []string{"80", "443", "8080", "8443"},
	ApplyIPFilterForDomain: true,
}

// fetchSettingGeneration is immutable after publication. Fetch policy lists
// are persisted as separate option keys, but readers must not observe fields
// or slice backing arrays while a configuration update is in progress.
type fetchSettingGeneration struct {
	setting FetchSetting
}

// managedFetchSetting owns the synchronized runtime snapshot registered with
// the generic config manager.
type managedFetchSetting struct {
	writeMutex sync.Mutex
	current    atomic.Pointer[fetchSettingGeneration]
}

func cloneFetchSetting(value FetchSetting) FetchSetting {
	value.DomainList = cloneFetchSettingStrings(value.DomainList)
	value.IpList = cloneFetchSettingStrings(value.IpList)
	value.AllowedPorts = cloneFetchSettingStrings(value.AllowedPorts)
	return value
}

func cloneFetchSettingStrings(values []string) []string {
	if values == nil {
		return nil
	}
	return append([]string{}, values...)
}

func newManagedFetchSetting(initial FetchSetting) *managedFetchSetting {
	state := &managedFetchSetting{}
	state.current.Store(&fetchSettingGeneration{setting: cloneFetchSetting(initial)})
	return state
}

func (s *managedFetchSetting) snapshot() FetchSetting {
	if s != nil {
		if current := s.current.Load(); current != nil {
			return current.setting
		}
	}
	return defaultFetchSetting
}

func (s *managedFetchSetting) detachedSnapshot() *FetchSetting {
	setting := cloneFetchSetting(s.snapshot())
	return &setting
}

func (s *managedFetchSetting) candidate(values map[string]string) (FetchSetting, error) {
	candidate := s.snapshot()
	if err := config.UpdateConfigFromMap(&candidate, values); err != nil {
		return FetchSetting{}, err
	}
	return candidate, nil
}

func (s *managedFetchSetting) ExportConfigMap() (map[string]string, error) {
	setting := s.snapshot()
	domainList, err := common.Marshal(setting.DomainList)
	if err != nil {
		return nil, err
	}
	ipList, err := common.Marshal(setting.IpList)
	if err != nil {
		return nil, err
	}
	allowedPorts, err := common.Marshal(setting.AllowedPorts)
	if err != nil {
		return nil, err
	}
	return map[string]string{
		"enable_ssrf_protection":     strconv.FormatBool(setting.EnableSSRFProtection),
		"allow_private_ip":           strconv.FormatBool(setting.AllowPrivateIp),
		"domain_filter_mode":         strconv.FormatBool(setting.DomainFilterMode),
		"ip_filter_mode":             strconv.FormatBool(setting.IpFilterMode),
		"domain_list":                string(domainList),
		"ip_list":                    string(ipList),
		"allowed_ports":              string(allowedPorts),
		"apply_ip_filter_for_domain": strconv.FormatBool(setting.ApplyIPFilterForDomain),
	}, nil
}

func (s *managedFetchSetting) ValidateConfigMap(values map[string]string) error {
	_, err := s.candidate(values)
	return err
}

func (s *managedFetchSetting) UpdateConfigMap(values map[string]string) error {
	s.writeMutex.Lock()
	defer s.writeMutex.Unlock()

	candidate, err := s.candidate(values)
	if err != nil {
		return err
	}
	s.current.Store(&fetchSettingGeneration{setting: candidate})
	return nil
}

var fetchSettingState = newManagedFetchSetting(defaultFetchSetting)

var _ config.ValidatingMapConfig = (*managedFetchSetting)(nil)

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("fetch_setting", fetchSettingState)
}

// GetFetchSetting returns a detached copy of one immutable runtime
// generation. Mutating the returned value never updates the live setting.
func GetFetchSetting() *FetchSetting {
	return fetchSettingState.detachedSnapshot()
}
