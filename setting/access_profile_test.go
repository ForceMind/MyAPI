package setting

import (
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func restoreAccessProfiles(t *testing.T) {
	t.Helper()
	original, err := common.Marshal(GetAccessProfileSetting().Profiles)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, UpdateAccessProfileDefinitionsByJSONString(string(original))) })
}

func restoreAccountTiers(t *testing.T) {
	t.Helper()
	original, err := common.Marshal(GetAccessProfileSetting().AccountTiers)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, UpdateAccountTierDefinitionsByJSONString(string(original))) })
}

func TestAccessProfileNormalizationAndExport(t *testing.T) {
	restoreAccessProfiles(t)
	require.NoError(t, UpdateAccessProfileDefinitionsByJSONString(`{" standard ":{"label":"Standard","fallback_profiles":[" priority "]}," priority ":{"label":"Priority"}}`))
	profile, ok := GetAccessProfileDefinition("standard")
	require.True(t, ok)
	assert.Equal(t, []string{"priority"}, profile.FallbackProfiles)
	var exported map[string]AccessProfileDefinition
	require.NoError(t, common.UnmarshalJsonStr(config.GlobalConfig.ExportAllConfigs()["access_profile_setting.profiles"], &exported))
	assert.Contains(t, exported, "standard")
	assert.NotContains(t, exported, " standard ")
	assert.Equal(t, []string{"priority"}, exported["standard"].FallbackProfiles)
}

func TestAccessProfileReadersReturnDetachedSnapshots(t *testing.T) {
	restoreAccessProfiles(t)
	require.NoError(t, UpdateAccessProfileDefinitionsByJSONString(`{"standard":{"label":"Standard","route_groups":["default"],"model_allowlist":["gpt"],"fallback_profiles":["priority"],"enabled":true},"priority":{"label":"Priority"}}`))
	profile, ok := GetAccessProfileDefinition("standard")
	require.True(t, ok)
	profile.RouteGroups[0] = "modified"
	profile.ModelAllowlist[0] = "modified"
	profile.FallbackProfiles[0] = "modified"
	*profile.Enabled = false
	snapshot := GetAccessProfileSetting()
	delete(snapshot.Profiles, "priority")
	actual, ok := GetAccessProfileDefinition("standard")
	require.True(t, ok)
	assert.Equal(t, []string{"default"}, actual.RouteGroups)
	assert.Equal(t, []string{"gpt"}, actual.ModelAllowlist)
	assert.Equal(t, []string{"priority"}, actual.FallbackProfiles)
	assert.True(t, *actual.Enabled)
	_, ok = GetAccessProfileDefinition("priority")
	assert.True(t, ok)
}

func TestAccessProfileExportPreservesExplicitEmptyPolicyLists(t *testing.T) {
	restoreAccessProfiles(t)
	require.NoError(t, UpdateAccessProfileDefinitionsByJSONString(`{
		"inherit":{"label":"Inherit"},
		"deny":{"label":"Deny","route_groups":[],"model_allowlist":[]}
	}`))
	var exported map[string]AccessProfileDefinition
	require.NoError(t, common.UnmarshalJsonStr(config.GlobalConfig.ExportAllConfigs()["access_profile_setting.profiles"], &exported))
	assert.Nil(t, exported["inherit"].RouteGroups)
	assert.Nil(t, exported["inherit"].ModelAllowlist)
	assert.NotNil(t, exported["deny"].RouteGroups)
	assert.NotNil(t, exported["deny"].ModelAllowlist)
}

func TestAccessProfileConfigImportCannotBypassValidation(t *testing.T) {
	restoreAccessProfiles(t)
	require.NoError(t, UpdateAccessProfileDefinitionsByJSONString(`{"standard":{"label":"Before"}}`))
	err := config.UpdateConfigFromMap(config.GlobalConfig.Get("access_profile_setting"), map[string]string{"profiles": `{"standard":{"label":""}}`})
	assert.Error(t, err)
	profile, ok := GetAccessProfileDefinition("standard")
	require.True(t, ok)
	assert.Equal(t, "Before", profile.Label)
}

func TestAccessProfileConfigManagerLoadsAndSavesCanonicalSnapshots(t *testing.T) {
	restoreAccessProfiles(t)
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"access_profile_setting.profiles": `{" standard ":{"label":"Imported"}}`}))
	profile, ok := GetAccessProfileDefinition("standard")
	require.True(t, ok)
	assert.Equal(t, "Imported", profile.Label)
	var saved string
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		if key == "access_profile_setting.profiles" {
			saved = value
		}
		return nil
	}))
	var profiles map[string]AccessProfileDefinition
	require.NoError(t, common.UnmarshalJsonStr(saved, &profiles))
	assert.Equal(t, profile, profiles["standard"])
	assert.NotContains(t, profiles, " standard ")
	// ConfigManager logs invalid stored options and keeps the last valid value.
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"access_profile_setting.profiles": `{"standard":{"label":"","fallback_profiles":["missing"]}}`}))
	actual, ok := GetAccessProfileDefinition("standard")
	require.True(t, ok)
	assert.Equal(t, profile, actual)
}

func TestAccountTierDefinitionsPreserveInheritanceAndExplicitDeny(t *testing.T) {
	restoreAccountTiers(t)
	require.NoError(t, UpdateAccountTierDefinitionsByJSONString(`{
		" inherited ":{"label":"Inherited"},
		"deny":{"label":"Deny","route_groups":[],"model_allowlist":[],"enabled":false},
		"limited":{"label":"Limited","route_groups":[" vip ","vip"],"model_allowlist":["gpt-5","gpt-5"]}
	}`))

	inherited, ok := GetAccountTierDefinition("inherited")
	require.True(t, ok)
	assert.Nil(t, inherited.RouteGroups)
	assert.Nil(t, inherited.ModelAllowlist)
	assert.Nil(t, inherited.Enabled)

	deny, ok := GetAccountTierDefinition("deny")
	require.True(t, ok)
	assert.NotNil(t, deny.RouteGroups)
	assert.Empty(t, deny.RouteGroups)
	assert.NotNil(t, deny.ModelAllowlist)
	assert.Empty(t, deny.ModelAllowlist)
	require.NotNil(t, deny.Enabled)
	assert.False(t, *deny.Enabled)

	limited, ok := GetAccountTierDefinition("limited")
	require.True(t, ok)
	assert.Equal(t, []string{"vip"}, limited.RouteGroups)
	assert.Equal(t, []string{"gpt-5"}, limited.ModelAllowlist)

	var exported map[string]AccountTierDefinition
	require.NoError(t, common.UnmarshalJsonStr(config.GlobalConfig.ExportAllConfigs()["access_profile_setting.account_tiers"], &exported))
	assert.Nil(t, exported["inherited"].RouteGroups)
	assert.NotNil(t, exported["deny"].RouteGroups)
	assert.Empty(t, exported["deny"].RouteGroups)
	assert.NotNil(t, exported["deny"].ModelAllowlist)
	assert.Empty(t, exported["deny"].ModelAllowlist)
}

func TestAccessPolicyRegistryUpdateIsAtomicAndDetached(t *testing.T) {
	restoreAccessProfiles(t)
	restoreAccountTiers(t)
	require.NoError(t, UpdateAccessProfileDefinitionsByJSONString(`{"before":{"label":"Before"}}`))
	require.NoError(t, UpdateAccountTierDefinitionsByJSONString(`{"before":{"label":"Before"}}`))

	cfg := config.GlobalConfig.Get("access_profile_setting")
	err := config.UpdateConfigFromMap(cfg, map[string]string{
		"profiles":      `{"after":{"label":"After"}}`,
		"account_tiers": `{"broken":{"label":""}}`,
	})
	require.Error(t, err)
	_, profileBefore := GetAccessProfileDefinition("before")
	_, tierBefore := GetAccountTierDefinition("before")
	assert.True(t, profileBefore)
	assert.True(t, tierBefore)

	require.NoError(t, config.UpdateConfigFromMap(cfg, map[string]string{
		"profiles":      `{"after":{"label":"After","route_groups":["default"]}}`,
		"account_tiers": `{"after":{"label":"After","model_allowlist":["gpt-5"]}}`,
	}))
	snapshot := GetAccessProfileSetting()
	snapshot.Profiles["after"] = AccessProfileDefinition{}
	snapshot.AccountTiers["after"] = AccountTierDefinition{}
	profile, ok := GetAccessProfileDefinition("after")
	require.True(t, ok)
	assert.Equal(t, []string{"default"}, profile.RouteGroups)
	tier, ok := GetAccountTierDefinition("after")
	require.True(t, ok)
	assert.Equal(t, []string{"gpt-5"}, tier.ModelAllowlist)
}

func TestAccountTierConfigIsValidatedAndExported(t *testing.T) {
	restoreAccountTiers(t)
	cfg := config.GlobalConfig.Get("access_profile_setting")
	require.Error(t, config.ValidateConfigFromMap(cfg, map[string]string{
		"account_tiers": `{"bad id":{"label":"Bad"}}`,
	}))
	require.NoError(t, config.UpdateConfigFromMap(cfg, map[string]string{
		"account_tiers": `{" team ":{"label":"Team","route_groups":["vip"]}}`,
	}))

	var exported map[string]AccountTierDefinition
	require.NoError(t, common.UnmarshalJsonStr(config.GlobalConfig.ExportAllConfigs()["access_profile_setting.account_tiers"], &exported))
	assert.Contains(t, exported, "team")
	assert.NotContains(t, exported, " team ")
	assert.Equal(t, []string{"vip"}, exported["team"].RouteGroups)
}
