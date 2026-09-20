package setting

import (
	"errors"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/setting/config"
)

// Access policy enforcement follows the D04 contract: off -> audit -> scoped
// enforce. The legacy routing path remains the only authority while the mode
// is off or audit; enforce applies only to requests whose selected legacy
// group is listed in EnforceGroups, so an operator can scope enforcement to a
// single migrated group before enabling it globally.
const (
	AccessPolicyModeOff     = "off"
	AccessPolicyModeAudit   = "audit"
	AccessPolicyModeEnforce = "enforce"
)

type AccessPolicyModeSetting struct {
	// Mode is one of off / audit / enforce. Unknown values fail validation.
	Mode string `json:"mode"`
	// EnforceGroups lists legacy group names where enforce mode actually
	// rejects. An empty list means enforce behaves like audit everywhere,
	// which keeps the default enforce scope empty and safe.
	EnforceGroups []string `json:"enforce_groups,omitempty"`
}

var defaultAccessPolicyModeSetting = AccessPolicyModeSetting{Mode: AccessPolicyModeOff}

// MaxEnforceGroups bounds the operator-configured enforcement scope.
const MaxEnforceGroups = 128

type accessPolicyModeGeneration struct {
	setting AccessPolicyModeSetting
	scope   map[string]struct{}
}

type managedAccessPolicyModeSetting struct {
	writeMutex sync.Mutex
	current    atomic.Pointer[accessPolicyModeGeneration]
}

func newManagedAccessPolicyModeSetting(initial AccessPolicyModeSetting) *managedAccessPolicyModeSetting {
	state := &managedAccessPolicyModeSetting{}
	generation := &accessPolicyModeGeneration{setting: initial, scope: map[string]struct{}{}}
	for _, group := range initial.EnforceGroups {
		generation.scope[group] = struct{}{}
	}
	state.current.Store(generation)
	return state
}

func (s *managedAccessPolicyModeSetting) generation() *accessPolicyModeGeneration {
	if s != nil {
		if current := s.current.Load(); current != nil {
			return current
		}
	}
	return &accessPolicyModeGeneration{setting: defaultAccessPolicyModeSetting, scope: map[string]struct{}{}}
}

func (s *managedAccessPolicyModeSetting) candidate(values map[string]string) (AccessPolicyModeSetting, error) {
	candidate := s.generation().setting
	// UpdateConfigFromMap expects a JSON array for slice fields, while the
	// flat option map carries enforce_groups as a comma-separated string.
	// Parse that field explicitly and drop it before the generic pass.
	remaining := make(map[string]string, len(values))
	for key, value := range values {
		remaining[key] = value
	}
	if raw, ok := remaining["enforce_groups"]; ok {
		groups, err := parseAccessPolicyEnforceGroups(raw)
		if err != nil {
			return AccessPolicyModeSetting{}, err
		}
		for _, group := range groups {
			if strings.TrimSpace(group) != group || group == "" {
				return AccessPolicyModeSetting{}, errors.New("access policy enforce scope contains an invalid group name")
			}
		}
		candidate.EnforceGroups = groups
		delete(remaining, "enforce_groups")
	}
	if err := config.UpdateConfigFromMap(&candidate, remaining); err != nil {
		return AccessPolicyModeSetting{}, err
	}
	candidate.Mode = strings.TrimSpace(candidate.Mode)
	if candidate.Mode == "" {
		candidate.Mode = AccessPolicyModeOff
	}
	switch candidate.Mode {
	case AccessPolicyModeOff, AccessPolicyModeAudit, AccessPolicyModeEnforce:
	default:
		return AccessPolicyModeSetting{}, errors.New("access policy mode must be one of: off, audit, enforce")
	}
	if len(candidate.EnforceGroups) > MaxEnforceGroups {
		return AccessPolicyModeSetting{}, errors.New("access policy enforce scope exceeds the group limit")
	}
	normalized := make([]string, 0, len(candidate.EnforceGroups))
	seen := make(map[string]struct{}, len(candidate.EnforceGroups))
	for _, group := range candidate.EnforceGroups {
		group = strings.TrimSpace(group)
		if group == "" || len(group) > 64 || strings.ContainsAny(group, " \t\r\n") {
			return AccessPolicyModeSetting{}, errors.New("access policy enforce scope contains an invalid group name")
		}
		if _, exists := seen[group]; exists {
			continue
		}
		seen[group] = struct{}{}
		normalized = append(normalized, group)
	}
	candidate.EnforceGroups = normalized
	return candidate, nil
}

func (s *managedAccessPolicyModeSetting) ExportConfigMap() (map[string]string, error) {
	setting := s.generation().setting
	return map[string]string{
		"mode":           setting.Mode,
		"enforce_groups": strings.Join(setting.EnforceGroups, ","),
	}, nil
}

func (s *managedAccessPolicyModeSetting) ValidateConfigMap(values map[string]string) error {
	_, err := s.candidate(values)
	return err
}

func (s *managedAccessPolicyModeSetting) UpdateConfigMap(values map[string]string) error {
	s.writeMutex.Lock()
	defer s.writeMutex.Unlock()

	candidate, err := s.candidate(values)
	if err != nil {
		return err
	}
	generation := &accessPolicyModeGeneration{setting: candidate, scope: make(map[string]struct{}, len(candidate.EnforceGroups))}
	for _, group := range candidate.EnforceGroups {
		generation.scope[group] = struct{}{}
	}
	s.current.Store(generation)
	return nil
}

var accessPolicyModeState = newManagedAccessPolicyModeSetting(defaultAccessPolicyModeSetting)

var _ config.ValidatingMapConfig = (*managedAccessPolicyModeSetting)(nil)

func init() {
	config.GlobalConfig.Register("access_policy_mode_setting", accessPolicyModeState)
}

// GetAccessPolicyModeSetting returns a detached copy of the current mode.
func GetAccessPolicyModeSetting() AccessPolicyModeSetting {
	return accessPolicyModeState.generation().setting
}

// GetAccessPolicyMode returns the configured mode; it never returns an empty
// or unknown value because validation rejects those at update time.
func GetAccessPolicyMode() string {
	return accessPolicyModeState.generation().setting.Mode
}

// AccessPolicyEnforcedForGroup reports whether enforce mode actually rejects
// requests for the given legacy group. Audit mode never enforces; enforce
// mode with an empty scope list enforces nowhere.
func AccessPolicyEnforcedForGroup(group string) bool {
	generation := accessPolicyModeState.generation()
	if generation.setting.Mode != AccessPolicyModeEnforce {
		return false
	}
	_, ok := generation.scope[strings.TrimSpace(group)]
	return ok
}

// parseAccessPolicyEnforceGroups converts the flat comma-separated option
// value into the typed slice, accepting either "a,b" or a JSON array so both
// the legacy option editor and structured callers work.
func parseAccessPolicyEnforceGroups(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if strings.HasPrefix(raw, "[") {
		var groups []string
		if err := common.UnmarshalJsonStr(raw, &groups); err != nil {
			return nil, errors.New("access policy enforce scope must be a comma-separated list or a JSON array")
		}
		for _, group := range groups {
			if strings.TrimSpace(group) != group || group == "" {
				return nil, errors.New("access policy enforce scope contains an invalid group name")
			}
		}
		return groups, nil
	}
	parts := strings.Split(raw, ",")
	groups := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			// Bare separators and whitespace-only segments are ignored:
			// operators legitimately write "a, b" with spaces after commas.
			continue
		}
		groups = append(groups, trimmed)
	}
	return groups, nil
}
