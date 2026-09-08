package system_setting

import (
	"net/url"
	"strconv"
	"strings"

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

// managedPasskeySettings keeps persisted Passkey fields behind an immutable
// runtime generation. Address-derived fallbacks are deliberately applied only
// to a reader-owned copy so they are never written back to the database.
type managedPasskeySettings struct {
	runtime *runtimeSettings
}

var passkeySettingState = &managedPasskeySettings{runtime: systemRuntimeSettings}

var _ config.ValidatingMapConfig = (*managedPasskeySettings)(nil)

func init() {
	config.GlobalConfig.Register("passkey", passkeySettingState)
}

// GetPasskeySettings returns a detached effective snapshot. Modifying the
// returned value does not persist or publish a configuration change; writers
// must use the registered Passkey configuration update path instead.
func GetPasskeySettings() *PasskeySettings {
	return effectivePasskeySettings(systemRuntimeSettings.current.Load())
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

func (s *managedPasskeySettings) DiagnosticSchema() any {
	return &PasskeySettings{}
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
	return s.runtime.updatePasskeyAndServerAddress(nil, values)
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
