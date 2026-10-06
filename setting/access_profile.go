package setting

import (
	"errors"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

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
	RouteGroups      []string `json:"route_groups"`
	ModelAllowlist   []string `json:"model_allowlist"`
	FallbackProfiles []string `json:"fallback_profiles,omitempty"`
	Enabled          *bool    `json:"enabled,omitempty"`
}

// AccountTierDefinition is the account-level half of the D04 entitlement
// contract. Nil route/model lists inherit the key profile's constraint, while
// an explicitly empty list denies every value in that dimension.
type AccountTierDefinition struct {
	Label          string   `json:"label"`
	Description    string   `json:"description"`
	RouteGroups    []string `json:"route_groups"`
	ModelAllowlist []string `json:"model_allowlist"`
	Enabled        *bool    `json:"enabled,omitempty"`
}

type AccessProfileSetting struct {
	Profiles     map[string]AccessProfileDefinition `json:"profiles"`
	AccountTiers map[string]AccountTierDefinition   `json:"account_tiers"`
}

func boolPtr(value bool) *bool { return &value }

var accessProfileSetting = AccessProfileSetting{
	Profiles: map[string]AccessProfileDefinition{
		"standard":  {Label: "Standard access", Description: "Uses the standard channel pool and billing rules.", Enabled: boolPtr(true)},
		"priority":  {Label: "Priority access", Description: "Uses the priority channel pool when your account allows it.", Enabled: boolPtr(true)},
		"automatic": {Label: "Automatic routing", Description: "Tries eligible channel groups in order and can fail over when enabled.", Enabled: boolPtr(true)},
	},
	AccountTiers: map[string]AccountTierDefinition{
		"standard": {Label: "Standard account", Description: "Controls account quota, channel eligibility, and available features.", Enabled: boolPtr(true)},
		"priority": {Label: "Priority account", Description: "Uses the priority account quota, channel eligibility, and feature rules.", Enabled: boolPtr(true)},
	},
}

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
	profiles, err := common.Marshal(snapshot.Profiles)
	if err != nil {
		return nil, err
	}
	tiers, err := common.Marshal(snapshot.AccountTiers)
	if err != nil {
		return nil, err
	}
	return map[string]string{
		"profiles":      string(profiles),
		"account_tiers": string(tiers),
	}, nil
}

func (accessProfileConfig) UpdateConfigMap(values map[string]string) error {
	var profiles map[string]AccessProfileDefinition
	var tiers map[string]AccountTierDefinition
	var updateProfiles, updateTiers bool
	var err error
	if raw, ok := values["profiles"]; ok {
		profiles, err = parseAccessProfileDefinitions(raw)
		if err != nil {
			return err
		}
		updateProfiles = true
	}
	if raw, ok := values["account_tiers"]; ok {
		tiers, err = parseAccountTierDefinitions(raw)
		if err != nil {
			return err
		}
		updateTiers = true
	}
	if !updateProfiles && !updateTiers {
		return nil
	}
	accessProfileMutex.Lock()
	defer accessProfileMutex.Unlock()
	if updateProfiles {
		accessProfileSetting.Profiles = profiles
	}
	if updateTiers {
		accessProfileSetting.AccountTiers = tiers
	}
	return nil
}

func (accessProfileConfig) ValidateConfigMap(values map[string]string) error {
	if raw, ok := values["profiles"]; ok {
		if _, err := parseAccessProfileDefinitions(raw); err != nil {
			return err
		}
	}
	if raw, ok := values["account_tiers"]; ok {
		if _, err := parseAccountTierDefinitions(raw); err != nil {
			return err
		}
	}
	return nil
}

func (profile AccessProfileDefinition) clone() AccessProfileDefinition {
	// Keep nil versus explicitly empty distinct: an empty configured list is
	// a "deny all" statement, not an absent field.
	profile.RouteGroups = cloneStringList(profile.RouteGroups)
	profile.ModelAllowlist = cloneStringList(profile.ModelAllowlist)
	profile.FallbackProfiles = cloneStringList(profile.FallbackProfiles)
	if profile.Enabled != nil {
		profile.Enabled = boolPtr(*profile.Enabled)
	}
	return profile
}

func (tier AccountTierDefinition) clone() AccountTierDefinition {
	tier.RouteGroups = cloneStringList(tier.RouteGroups)
	tier.ModelAllowlist = cloneStringList(tier.ModelAllowlist)
	if tier.Enabled != nil {
		tier.Enabled = boolPtr(*tier.Enabled)
	}
	return tier
}

func cloneStringList(values []string) []string {
	if values == nil {
		return nil
	}
	return append([]string{}, values...)
}

func GetAccessProfileSetting() *AccessProfileSetting {
	accessProfileMutex.RLock()
	defer accessProfileMutex.RUnlock()
	snapshot := &AccessProfileSetting{
		Profiles:     make(map[string]AccessProfileDefinition, len(accessProfileSetting.Profiles)),
		AccountTiers: make(map[string]AccountTierDefinition, len(accessProfileSetting.AccountTiers)),
	}
	for id, profile := range accessProfileSetting.Profiles {
		snapshot.Profiles[id] = profile.clone()
	}
	for id, tier := range accessProfileSetting.AccountTiers {
		snapshot.AccountTiers[id] = tier.clone()
	}
	return snapshot
}

func GetAccessProfileDefinition(id string) (AccessProfileDefinition, bool) {
	accessProfileMutex.RLock()
	defer accessProfileMutex.RUnlock()
	profile, ok := accessProfileSetting.Profiles[strings.TrimSpace(id)]
	return profile.clone(), ok
}

func GetAccountTierDefinition(id string) (AccountTierDefinition, bool) {
	accessProfileMutex.RLock()
	defer accessProfileMutex.RUnlock()
	tier, ok := accessProfileSetting.AccountTiers[strings.TrimSpace(id)]
	return tier.clone(), ok
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

func UpdateAccountTierDefinitionsByJSONString(raw string) error {
	tiers, err := parseAccountTierDefinitions(raw)
	if err != nil {
		return err
	}
	accessProfileMutex.Lock()
	accessProfileSetting.AccountTiers = tiers
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

func ValidateAccountTierDefinitionsJSON(raw string) error {
	_, err := parseAccountTierDefinitions(raw)
	return err
}

func NormalizeAccountTierDefinitionsJSON(raw string) (string, error) {
	tiers, err := parseAccountTierDefinitions(raw)
	if err != nil {
		return "", err
	}
	data, err := common.Marshal(tiers)
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
	normalized := make(map[string]AccessProfileDefinition, len(profiles))
	for rawID, profile := range profiles {
		id := strings.TrimSpace(rawID)
		if !validPolicyID(id) {
			return nil, errors.New("access profile id is invalid")
		}
		if _, exists := normalized[id]; exists {
			return nil, errors.New("access profile ids must be unique after trimming: " + id)
		}
		profile.Label = strings.TrimSpace(profile.Label)
		profile.Description = strings.TrimSpace(profile.Description)
		if profile.Label == "" {
			return nil, errors.New("access profile label must not be empty: " + id)
		}
		var err error
		if profile.RouteGroups, err = normalizePolicyIDList(profile.RouteGroups); err != nil {
			return nil, errors.New("access profile route_groups are invalid: " + id)
		}
		if profile.ModelAllowlist, err = normalizePolicyTextList(profile.ModelAllowlist); err != nil {
			return nil, errors.New("access profile model_allowlist is invalid: " + id)
		}
		if profile.FallbackProfiles, err = normalizePolicyIDList(profile.FallbackProfiles); err != nil {
			return nil, errors.New("access profile fallback_profiles are invalid: " + id)
		}
		if 2*len(profile.RouteGroups)+len(profile.ModelAllowlist) > 256 {
			return nil, errors.New("access profile policy lists are too large: " + id)
		}
		normalized[id] = profile
	}
	graph := make(map[string][]string, len(normalized))
	for id, profile := range normalized {
		for _, fallback := range profile.FallbackProfiles {
			if _, exists := normalized[fallback]; !exists {
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
	for id := range normalized {
		if err := visit(id); err != nil {
			return nil, err
		}
	}
	return normalized, nil
}

func parseAccountTierDefinitions(raw string) (map[string]AccountTierDefinition, error) {
	var tiers map[string]AccountTierDefinition
	if err := common.UnmarshalJsonStr(raw, &tiers); err != nil {
		return nil, err
	}
	if tiers == nil {
		return nil, errors.New("account tier definitions must be a JSON object")
	}
	normalized := make(map[string]AccountTierDefinition, len(tiers))
	for rawID, tier := range tiers {
		id := strings.TrimSpace(rawID)
		if !validPolicyID(id) {
			return nil, errors.New("account tier id is invalid")
		}
		if _, exists := normalized[id]; exists {
			return nil, errors.New("account tier ids must be unique after trimming: " + id)
		}
		tier.Label = strings.TrimSpace(tier.Label)
		tier.Description = strings.TrimSpace(tier.Description)
		if tier.Label == "" {
			return nil, errors.New("account tier label must not be empty: " + id)
		}
		var err error
		if tier.RouteGroups, err = normalizePolicyIDList(tier.RouteGroups); err != nil {
			return nil, errors.New("account tier route_groups are invalid: " + id)
		}
		if tier.ModelAllowlist, err = normalizePolicyTextList(tier.ModelAllowlist); err != nil {
			return nil, errors.New("account tier model_allowlist is invalid: " + id)
		}
		if 2*len(tier.RouteGroups)+len(tier.ModelAllowlist) > 256 {
			return nil, errors.New("account tier policy lists are too large: " + id)
		}
		normalized[id] = tier
	}
	return normalized, nil
}

func normalizePolicyIDList(values []string) ([]string, error) {
	if values == nil {
		return nil, nil
	}
	if len(values) > 128 {
		return nil, errors.New("too many values")
	}
	normalized := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if !validPolicyID(value) {
			return nil, errors.New("invalid id")
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	return normalized, nil
}

func normalizePolicyTextList(values []string) ([]string, error) {
	if values == nil {
		return nil, nil
	}
	if len(values) > 128 {
		return nil, errors.New("too many values")
	}
	normalized := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if !validPolicyText(value, 255) {
			return nil, errors.New("invalid value")
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	return normalized, nil
}

func validPolicyID(value string) bool {
	if !validPolicyText(value, 64) {
		return false
	}
	for _, char := range value {
		if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || strings.ContainsRune("._:-/", char)) {
			return false
		}
	}
	return true
}

func validPolicyText(value string, maxLength int) bool {
	if value == "" || len(value) > maxLength || !utf8.ValidString(value) {
		return false
	}
	for _, char := range value {
		if char <= 0x1f || (char >= 0x7f && char <= 0x9f) || unicode.Is(unicode.Cf, char) {
			return false
		}
	}
	return true
}
