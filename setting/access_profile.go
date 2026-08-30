package setting

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"

	"github.com/ForceMind/MyAPI/setting/config"
)

// AccessProfileDefinition is the operator-facing policy metadata for a key
// access profile. The legacy group remains the routing identifier; these
// fields make the user-facing contract explicit and can be adopted by future
// routing policy migrations without changing the persisted Token.Group field.
type AccessProfileDefinition struct {
	Label            string   `json:"label"`
	Description      string   `json:"description"`
	RouteGroups      []string `json:"route_groups,omitempty"`
	ModelAllowlist   []string `json:"model_allowlist,omitempty"`
	FallbackProfiles []string `json:"fallback_profiles,omitempty"`
	Enabled          *bool    `json:"enabled,omitempty"`
}

type AccessProfileSetting struct {
	Profiles map[string]AccessProfileDefinition `json:"profiles"`
}

func boolPtr(value bool) *bool { return &value }

var accessProfileSetting = AccessProfileSetting{Profiles: map[string]AccessProfileDefinition{
	"standard":  {Label: "Standard access", Description: "Uses the standard channel pool and billing rules.", Enabled: boolPtr(true)},
	"priority":  {Label: "Priority access", Description: "Uses the priority channel pool when your account allows it.", Enabled: boolPtr(true)},
	"automatic": {Label: "Automatic routing", Description: "Tries eligible channel groups in order and can fail over when enabled.", Enabled: boolPtr(true)},
}}

var accessProfileMutex sync.RWMutex

func init() {
	config.GlobalConfig.Register("access_profile_setting", &accessProfileSetting)
}

func GetAccessProfileSetting() *AccessProfileSetting {
	return &accessProfileSetting
}

func GetAccessProfileDefinition(id string) (AccessProfileDefinition, bool) {
	accessProfileMutex.RLock()
	defer accessProfileMutex.RUnlock()
	profile, ok := accessProfileSetting.Profiles[strings.TrimSpace(id)]
	return profile, ok
}

func UpdateAccessProfileDefinitionsByJSONString(raw string) error {
	if err := ValidateAccessProfileDefinitionsJSON(raw); err != nil {
		return err
	}
	var profiles map[string]AccessProfileDefinition
	if err := json.Unmarshal([]byte(raw), &profiles); err != nil {
		return err
	}
	accessProfileMutex.Lock()
	accessProfileSetting.Profiles = profiles
	accessProfileMutex.Unlock()
	return nil
}

// ValidateAccessProfileDefinitionsJSON validates the independently editable
// profile registry before it is persisted by the generic option endpoint.
func ValidateAccessProfileDefinitionsJSON(raw string) error {
	var profiles map[string]AccessProfileDefinition
	if err := json.Unmarshal([]byte(raw), &profiles); err != nil {
		return err
	}
	if profiles == nil {
		return errors.New("access profile definitions must be a JSON object")
	}
	for id, profile := range profiles {
		id = strings.TrimSpace(id)
		if id == "" {
			return errors.New("access profile id must not be empty")
		}
		if strings.TrimSpace(profile.Label) == "" {
			return errors.New("access profile label must not be empty: " + id)
		}
		for _, fallback := range profile.FallbackProfiles {
			if strings.TrimSpace(fallback) == id {
				return errors.New("access profile cannot fall back to itself: " + id)
			}
		}
	}
	return nil
}
