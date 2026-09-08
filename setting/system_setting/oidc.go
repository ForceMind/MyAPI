package system_setting

import (
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/ForceMind/MyAPI/setting/config"
)

type OIDCSettings struct {
	Enabled               bool   `json:"enabled"`
	DisplayName           string `json:"display_name"`
	ClientId              string `json:"client_id"`
	ClientSecret          string `json:"client_secret"`
	WellKnown             string `json:"well_known"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserInfoEndpoint      string `json:"user_info_endpoint"`
}

// 默认配置
var defaultOIDCSettings = OIDCSettings{}

// oidcSettingsGeneration is immutable after publication. OAuth settings are
// persisted as independent option keys, but readers must not observe a
// partially rewritten struct during an option load or update.
type oidcSettingsGeneration struct {
	settings OIDCSettings
}

// managedOIDCSettings owns the synchronized runtime settings registered with
// the generic config manager.
type managedOIDCSettings struct {
	writeMutex sync.Mutex
	current    atomic.Pointer[oidcSettingsGeneration]
}

func newManagedOIDCSettings(initial OIDCSettings) *managedOIDCSettings {
	settings := &managedOIDCSettings{}
	settings.current.Store(&oidcSettingsGeneration{settings: initial})
	return settings
}

func (s *managedOIDCSettings) snapshot() OIDCSettings {
	if s != nil {
		if current := s.current.Load(); current != nil {
			return current.settings
		}
	}
	return defaultOIDCSettings
}

func (s *managedOIDCSettings) detachedSnapshot() *OIDCSettings {
	settings := s.snapshot()
	return &settings
}

func (s *managedOIDCSettings) candidate(values map[string]string) (OIDCSettings, error) {
	candidate := s.snapshot()
	if err := config.UpdateConfigFromMap(&candidate, values); err != nil {
		return OIDCSettings{}, err
	}
	return candidate, nil
}

func (s *managedOIDCSettings) ExportConfigMap() (map[string]string, error) {
	settings := s.snapshot()
	return map[string]string{
		"enabled":                strconv.FormatBool(settings.Enabled),
		"display_name":           settings.DisplayName,
		"client_id":              settings.ClientId,
		"client_secret":          settings.ClientSecret,
		"well_known":             settings.WellKnown,
		"authorization_endpoint": settings.AuthorizationEndpoint,
		"token_endpoint":         settings.TokenEndpoint,
		"user_info_endpoint":     settings.UserInfoEndpoint,
	}, nil
}

func (s *managedOIDCSettings) ValidateConfigMap(values map[string]string) error {
	_, err := s.candidate(values)
	return err
}

func (s *managedOIDCSettings) UpdateConfigMap(values map[string]string) error {
	s.writeMutex.Lock()
	defer s.writeMutex.Unlock()

	candidate, err := s.candidate(values)
	if err != nil {
		return err
	}
	s.current.Store(&oidcSettingsGeneration{settings: candidate})
	return nil
}

var oidcSettingsState = newManagedOIDCSettings(defaultOIDCSettings)

var _ config.ValidatingMapConfig = (*managedOIDCSettings)(nil)

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("oidc", oidcSettingsState)
}

// GetOIDCSettings returns a detached copy of one immutable runtime
// generation. Mutating the returned value never updates the live setting.
func GetOIDCSettings() *OIDCSettings {
	return oidcSettingsState.detachedSnapshot()
}

// GetEffectiveDisplayName returns the admin-configured display name, or the
// literal "OIDC" when none has been set. Centralizing this fallback keeps the
// default in one place for both the OAuth provider name and the public
// status payload.
func (s *OIDCSettings) GetEffectiveDisplayName() string {
	if trimmed := strings.TrimSpace(s.DisplayName); trimmed != "" {
		return trimmed
	}
	return "OIDC"
}
