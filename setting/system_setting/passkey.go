package system_setting

import (
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/setting/config"
)

type PasskeySettings struct {
	Enabled              bool   `json:"enabled"`
	RPDisplayName        string `json:"rp_display_name"`
	RPID                 string `json:"rp_id"`
	Origins              string `json:"origins"`
	AllowInsecureOrigin  bool   `json:"allow_insecure_origin"`
	UserVerification     string `json:"user_verification"`
	AttachmentPreference string `json:"attachment_preference"`
}

var defaultPasskeySettings = PasskeySettings{
	Enabled:              false,
	RPDisplayName:        common.SystemName,
	RPID:                 "",
	Origins:              "",
	AllowInsecureOrigin:  false,
	UserVerification:     "preferred",
	AttachmentPreference: "",
}

// runtimeSnapshot is the single published generation for values whose
// effective Passkey view must stay consistent. The Passkey fields here are
// always the raw persisted values; address-derived fallbacks are applied only
// to a reader-owned copy.
type runtimeSnapshot struct {
	serverAddress  string
	passkeySettings PasskeySettings
}

type runtimeSettings struct {
	current    atomic.Pointer[runtimeSnapshot]
	writeMutex sync.Mutex
}

func newRuntimeSettings(serverAddress string, passkeySettings PasskeySettings) *runtimeSettings {
	settings := &runtimeSettings{}
	settings.current.Store(&runtimeSnapshot{
		serverAddress:  serverAddress,
		passkeySettings: passkeySettings,
	})
	return settings
}

var systemRuntimeSettings = newRuntimeSettings("http://localhost:3000", defaultPasskeySettings)

// managedPasskeySettings keeps the registered config surface private while
// preserving PasskeySettings as the public DTO returned to callers.
type managedPasskeySettings struct {
	runtime *runtimeSettings
}

var passkeySettingState = &managedPasskeySettings{runtime: systemRuntimeSettings}

var _ config.ValidatingMapConfig = (*managedPasskeySettings)(nil)

func init() {
	config.GlobalConfig.Register("passkey", passkeySettingState)
}

func GetPasskeySettings() *PasskeySettings {
	snapshot := systemRuntimeSettings.current.Load()
	return effectivePasskeySettings(snapshot)
}

func effectivePasskeySettings(snapshot *runtimeSnapshot) *PasskeySettings {
	if snapshot == nil {
		settings := defaultPasskeySettings
		return &settings
	}
	settings := snapshot.passkeySettings
	if strings.TrimSpace(settings.RPID) == "" && snapshot.serverAddress != "" {
		// 从ServerAddress提取域名作为RPID
		// ServerAddress 可能是 "https://myapi.example.com" 这种格式
		serverAddr := strings.TrimSpace(snapshot.serverAddress)
		if parsed, err := url.Parse(serverAddr); err == nil && parsed.Host != "" {
			settings.RPID = parsed.Host
		} else {
			settings.RPID = serverAddr
		}
	}
	trimmedOrigins := strings.TrimSpace(settings.Origins)
	if trimmedOrigins == "" || trimmedOrigins == "[]" {
		settings.Origins = snapshot.serverAddress
	}
	return &settings
}

func GetServerAddress() string {
	if snapshot := systemRuntimeSettings.current.Load(); snapshot != nil {
		return snapshot.serverAddress
	}
	return ""
}

func SetServerAddress(serverAddress string) {
	systemRuntimeSettings.setServerAddress(serverAddress)
}

func (s *runtimeSettings) setServerAddress(serverAddress string) {
	s.writeMutex.Lock()
	defer s.writeMutex.Unlock()

	candidate := runtimeSnapshot{serverAddress: serverAddress}
	if current := s.current.Load(); current != nil {
		candidate = *current
		candidate.serverAddress = serverAddress
	}
	s.current.Store(&candidate)
}

func (s *managedPasskeySettings) ExportConfigMap() (map[string]string, error) {
	settings := defaultPasskeySettings
	if snapshot := s.runtime.current.Load(); snapshot != nil {
		settings = snapshot.passkeySettings
	}
	return map[string]string{
		"enabled":               strconv.FormatBool(settings.Enabled),
		"rp_display_name":       settings.RPDisplayName,
		"rp_id":                 settings.RPID,
		"origins":               settings.Origins,
		"allow_insecure_origin": strconv.FormatBool(settings.AllowInsecureOrigin),
		"user_verification":     settings.UserVerification,
		"attachment_preference": settings.AttachmentPreference,
	}, nil
}

func (s *managedPasskeySettings) ValidateConfigMap(values map[string]string) error {
	_, err := s.buildCandidate(s.runtime.current.Load(), values)
	return err
}

func (s *managedPasskeySettings) UpdateConfigMap(values map[string]string) error {
	s.runtime.writeMutex.Lock()
	defer s.runtime.writeMutex.Unlock()

	candidate, err := s.buildCandidate(s.runtime.current.Load(), values)
	if err != nil {
		return err
	}
	s.runtime.current.Store(candidate)
	return nil
}

func (s *managedPasskeySettings) buildCandidate(current *runtimeSnapshot, values map[string]string) (*runtimeSnapshot, error) {
	candidate := runtimeSnapshot{passkeySettings: defaultPasskeySettings}
	if current != nil {
		candidate = *current
	}
	if err := config.UpdateConfigFromMap(&candidate.passkeySettings, values); err != nil {
		return nil, err
	}
	return &candidate, nil
}
