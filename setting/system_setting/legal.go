package system_setting

import (
	"sync"
	"sync/atomic"

	"github.com/ForceMind/MyAPI/setting/config"
)

type LegalSettings struct {
	UserAgreement string `json:"user_agreement"`
	PrivacyPolicy string `json:"privacy_policy"`
}

var defaultLegalSettings = LegalSettings{
	UserAgreement: "",
	PrivacyPolicy: "",
}

// legalSettingsGeneration is immutable after publication. Legal text is
// persisted as two independent option keys, but readers must not observe a
// caller-mutated live struct while an option load or update is in progress.
type legalSettingsGeneration struct {
	settings LegalSettings
}

// managedLegalSettings owns the synchronized runtime view registered with the
// generic config manager. Its exported snapshots are copies, so callers cannot
// modify the live generation through GetLegalSettings.
type managedLegalSettings struct {
	writeMutex sync.Mutex
	current    atomic.Pointer[legalSettingsGeneration]
}

func newManagedLegalSettings(initial LegalSettings) *managedLegalSettings {
	setting := &managedLegalSettings{}
	setting.current.Store(&legalSettingsGeneration{settings: initial})
	return setting
}

func (s *managedLegalSettings) snapshot() LegalSettings {
	if s != nil {
		if current := s.current.Load(); current != nil {
			return current.settings
		}
	}
	return defaultLegalSettings
}

func (s *managedLegalSettings) detachedSnapshot() *LegalSettings {
	settings := s.snapshot()
	return &settings
}

func (s *managedLegalSettings) candidate(values map[string]string) LegalSettings {
	candidate := s.snapshot()
	if value, ok := values["user_agreement"]; ok {
		candidate.UserAgreement = value
	}
	if value, ok := values["privacy_policy"]; ok {
		candidate.PrivacyPolicy = value
	}
	return candidate
}

func (s *managedLegalSettings) ExportConfigMap() (map[string]string, error) {
	settings := s.snapshot()
	return map[string]string{
		"user_agreement": settings.UserAgreement,
		"privacy_policy": settings.PrivacyPolicy,
	}, nil
}

// ValidateConfigMap constructs the same complete candidate as UpdateConfigMap
// without publishing it. Legal text intentionally has no additional content
// restrictions: existing installations may store arbitrary valid text.
func (s *managedLegalSettings) ValidateConfigMap(values map[string]string) error {
	_ = s.candidate(values)
	return nil
}

func (s *managedLegalSettings) UpdateConfigMap(values map[string]string) error {
	s.writeMutex.Lock()
	defer s.writeMutex.Unlock()
	s.current.Store(&legalSettingsGeneration{settings: s.candidate(values)})
	return nil
}

var legalSettingState = newManagedLegalSettings(defaultLegalSettings)

var _ config.ValidatingMapConfig = (*managedLegalSettings)(nil)

func init() {
	config.GlobalConfig.Register("legal", legalSettingState)
}

// GetLegalSettings returns a detached copy of one immutable runtime
// generation. It preserves the legacy pointer return type for controllers and
// other callers, but mutations to the returned value are not configuration
// writes.
func GetLegalSettings() *LegalSettings {
	return legalSettingState.detachedSnapshot()
}
