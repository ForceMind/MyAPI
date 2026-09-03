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
