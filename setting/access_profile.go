package setting

import (
	"errors"
	"strings"
	"sync"

	"github.com/ForceMind/MyAPI/common"
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
	config.GlobalConfig.Register("access_profile_setting", accessProfileConfig{})
}

// The registry exposes only synchronized import/export operations, never the
// mutable setting pointer. All three ConfigManager load/save/export paths use
// these operations through config.MapConfig.
type accessProfileConfig struct{}

func (accessProfileConfig) ExportConfigMap() (map[string]string, error) {
	snapshot := GetAccessProfileSetting()
	raw, err := common.Marshal(snapshot.Profiles)
	if err != nil {
		return nil, err
	}
	return map[string]string{"profiles": string(raw)}, nil
}

func (accessProfileConfig) UpdateConfigMap(values map[string]string) error {
	if raw, ok := values["profiles"]; ok {
		return UpdateAccessProfileDefinitionsByJSONString(raw)
	}
	return nil
}

func (profile AccessProfileDefinition) clone() AccessProfileDefinition {
	profile.RouteGroups = append([]string(nil), profile.RouteGroups...)
	profile.ModelAllowlist = append([]string(nil), profile.ModelAllowlist...)
	profile.FallbackProfiles = append([]string(nil), profile.FallbackProfiles...)
	if profile.Enabled != nil {
		profile.Enabled = boolPtr(*profile.Enabled)
	}
	return profile
}

func GetAccessProfileSetting() *AccessProfileSetting {
	accessProfileMutex.RLock()
	defer accessProfileMutex.RUnlock()
	snapshot := &AccessProfileSetting{Profiles: make(map[string]AccessProfileDefinition, len(accessProfileSetting.Profiles))}
	for id, profile := range accessProfileSetting.Profiles {
		snapshot.Profiles[id] = profile.clone()
	}
	return snapshot
}

func GetAccessProfileDefinition(id string) (AccessProfileDefinition, bool) {
	accessProfileMutex.RLock()
	defer accessProfileMutex.RUnlock()
	profile, ok := accessProfileSetting.Profiles[strings.TrimSpace(id)]
	return profile.clone(), ok
}

func UpdateAccessProfileDefinitionsByJSONString(raw string) error {
	profiles, err := parseAccessProfileDefinitions(raw)
	if err != nil {
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
	_, err := parseAccessProfileDefinitions(raw)
	return err
}

// NormalizeAccessProfileDefinitionsJSON gives persistence and OptionMap the
// same canonical IDs/fallback references used by the live profile registry.
func NormalizeAccessProfileDefinitionsJSON(raw string) (string, error) {
	profiles, err := parseAccessProfileDefinitions(raw)
	if err != nil {
		return "", err
	}
	data, err := common.Marshal(profiles)
	return string(data), err
}

func parseAccessProfileDefinitions(raw string) (map[string]AccessProfileDefinition, error) {
	var profiles map[string]AccessProfileDefinition
	if err := common.UnmarshalJsonStr(raw, &profiles); err != nil {
		return nil, err
	}
	if profiles == nil {
		return nil, errors.New("access profile definitions must be a JSON object")
	}
	normalizedIDs := make(map[string]struct{}, len(profiles))
	for id, profile := range profiles {
		id = strings.TrimSpace(id)
		if id == "" {
			return nil, errors.New("access profile id must not be empty")
		}
		if _, exists := normalizedIDs[id]; exists {
			return nil, errors.New("access profile ids must be unique after trimming: " + id)
		}
		normalizedIDs[id] = struct{}{}
		if strings.TrimSpace(profile.Label) == "" {
			return nil, errors.New("access profile label must not be empty: " + id)
		}
	}
	graph := make(map[string][]string, len(profiles))
	for rawID, profile := range profiles {
		id := strings.TrimSpace(rawID)
		for _, rawFallback := range profile.FallbackProfiles {
			fallback := strings.TrimSpace(rawFallback)
			if fallback == "" {
				return nil, errors.New("access profile fallback id must not be empty: " + id)
			}
			if _, exists := normalizedIDs[fallback]; !exists {
				return nil, errors.New("access profile fallback does not exist: " + id + " -> " + fallback)
			}
			if fallback == id {
				return nil, errors.New("access profile cannot fall back to itself: " + id)
			}
			graph[id] = append(graph[id], fallback)
		}
	}
	state := make(map[string]uint8, len(graph))
	var visit func(string) error
	visit = func(id string) error {
		if state[id] == 1 {
			return errors.New("access profile fallback cycle detected at: " + id)
		}
		if state[id] == 2 {
			return nil
		}
		state[id] = 1
		for _, fallback := range graph[id] {
			if err := visit(fallback); err != nil {
				return err
			}
		}
		state[id] = 2
		return nil
	}
	for id := range normalizedIDs {
		if err := visit(id); err != nil {
			return nil, err
		}
	}
	normalized := make(map[string]AccessProfileDefinition, len(profiles))
	for id, profile := range profiles {
		for i, fallback := range profile.FallbackProfiles {
			profile.FallbackProfiles[i] = strings.TrimSpace(fallback)
		}
		normalized[strings.TrimSpace(id)] = profile
	}
	return normalized, nil
}
