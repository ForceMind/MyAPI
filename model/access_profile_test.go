package model

import (
	"fmt"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveAccessProfileKeepsLegacyGroupsReadable(t *testing.T) {
	tests := []struct {
		legacy, id, kind, label string
	}{
		{legacy: "", id: "standard", kind: "standard", label: "Standard access"},
		{legacy: "default", id: "standard", kind: "standard", label: "Standard access"},
		{legacy: "vip", id: "priority", kind: "priority", label: "Priority access"},
		{legacy: "auto", id: "automatic", kind: "automatic", label: "Automatic routing"},
		{legacy: "team-a", id: "team-a", kind: "custom", label: "team-a"},
	}
	for _, testCase := range tests {
		profile := ResolveAccessProfile(testCase.legacy, "configured")
		if profile.ID != testCase.id || profile.Kind != testCase.kind || profile.Label != testCase.label {
			t.Fatalf("ResolveAccessProfile(%q) = %#v", testCase.legacy, profile)
		}
	}
	if got := ResolveAccessProfile("team-a", "configured").Description; got != "configured" {
		t.Fatalf("custom profile description = %q", got)
	}
}

func TestAccessProfilePolicyDefinitionsAreValidatedAndApplied(t *testing.T) {
	raw := `{"standard":{"label":"Team standard","description":"Shared pool","route_groups":["default"],"model_allowlist":["gpt-5"],"fallback_profiles":["priority"],"enabled":true},"priority":{"label":"Team priority","description":"Priority pool","enabled":true}}`
	require.NoError(t, setting.ValidateAccessProfileDefinitionsJSON(raw))
	require.NoError(t, setting.UpdateAccessProfileDefinitionsByJSONString(raw))
	t.Cleanup(func() {
		_ = setting.UpdateAccessProfileDefinitionsByJSONString(`{"standard":{"label":"Standard access","description":"Uses the standard channel pool and billing rules.","enabled":true},"priority":{"label":"Priority access","description":"Uses the priority channel pool when your account allows it.","enabled":true},"automatic":{"label":"Automatic routing","description":"Tries eligible channel groups in order and can fail over when enabled.","enabled":true}}`)
	})
	profile := ResolveAccessProfile("default", "")
	require.Equal(t, "Team standard", profile.Label)
	require.Equal(t, []string{"default"}, profile.RouteGroups)
	require.Equal(t, []string{"gpt-5"}, profile.ModelAllowlist)
	require.Error(t, setting.ValidateAccessProfileDefinitionsJSON(`{"":{"label":"invalid"}}`))
}

func TestAccessProfileFallbackReferencesAreBounded(t *testing.T) {
	require.ErrorContains(t,
		setting.ValidateAccessProfileDefinitionsJSON(`{"standard":{"label":"Standard","fallback_profiles":["missing"]}}`),
		"does not exist",
	)
	require.ErrorContains(t,
		setting.ValidateAccessProfileDefinitionsJSON(`{"standard":{"label":"Standard","fallback_profiles":["priority"]},"priority":{"label":"Priority","fallback_profiles":["standard"]}}`),
		"cycle detected",
	)
	require.ErrorContains(t,
		setting.ValidateAccessProfileDefinitionsJSON(`{" standard ":{"label":"Standard"},"standard":{"label":"Duplicate"}}`),
		"unique after trimming",
	)
}

func TestResolveAccountTierIsSeparateFromAccessProfile(t *testing.T) {
	tier := ResolveAccountTier("vip", "ignored for built-in tier")
	if tier.ID != "priority" || tier.Kind != "priority" || tier.Label != "Priority account" {
		t.Fatalf("vip account tier = %#v", tier)
	}
	profile := ResolveAccessProfile("vip", "")
	if profile.Label == tier.Label || profile.Description == tier.Description {
		t.Fatalf("account tier and access profile unexpectedly share presentation semantics: tier=%#v profile=%#v", tier, profile)
	}
}

func TestResolveAccountTierIDPreservesStableAndCustomIdentity(t *testing.T) {
	standard := ResolveAccountTierID("standard", "")
	require.Equal(t, "standard", standard.ID)
	require.Equal(t, "standard", standard.Kind)

	priority := ResolveAccountTierID("priority", "")
	require.Equal(t, "priority", priority.ID)
	require.Equal(t, "priority", priority.Kind)

	custom := ResolveAccountTierID("team-enterprise", "Custom team tier")
	require.Equal(t, "team-enterprise", custom.ID)
	require.Equal(t, "custom", custom.Kind)
	require.Equal(t, "Custom team tier", custom.Description)
}

func TestResolveAccountTierAppliesPolicyDefinition(t *testing.T) {
	original, err := common.Marshal(setting.GetAccessProfileSetting().AccountTiers)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, setting.UpdateAccountTierDefinitionsByJSONString(string(original))) })
	require.NoError(t, setting.UpdateAccountTierDefinitionsByJSONString(`{
		"standard":{"label":"Standard policy"},
		"team":{"label":"Team policy","description":"Restricted","route_groups":[],"model_allowlist":["gpt-5"],"enabled":false}
	}`))

	standard := ResolveAccountTierID("standard", "")
	assert.True(t, standard.Known)
	assert.True(t, standard.Enabled)
	assert.Nil(t, standard.RouteGroups)
	assert.Nil(t, standard.ModelAllowlist)

	team := ResolveAccountTierID("team", "")
	assert.True(t, team.Known)
	assert.False(t, team.Enabled)
	assert.Equal(t, "Team policy", team.Label)
	assert.Equal(t, "Restricted", team.Description)
	assert.NotNil(t, team.RouteGroups)
	assert.Empty(t, team.RouteGroups)
	assert.Equal(t, []string{"gpt-5"}, team.ModelAllowlist)

	unknown := ResolveAccountTierID("unknown", "")
	assert.False(t, unknown.Known)
	assert.True(t, unknown.Enabled)
}

func TestResolveAccessProfileReportsRegistryResolution(t *testing.T) {
	original, err := common.Marshal(setting.GetAccessProfileSetting().Profiles)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, setting.UpdateAccessProfileDefinitionsByJSONString(string(original))) })
	require.NoError(t, setting.UpdateAccessProfileDefinitionsByJSONString(`{
		"known":{"label":"Known"},
		"disabled":{"label":"Disabled","enabled":false}
	}`))

	known := ResolveAccessProfileID("known", "default", "")
	assert.True(t, known.Known)
	assert.True(t, known.Enabled)
	disabled := ResolveAccessProfileID("disabled", "default", "")
	assert.True(t, disabled.Known)
	assert.False(t, disabled.Enabled)
	unknown := ResolveAccessProfileID("unknown", "default", "")
	assert.False(t, unknown.Known)
}

func TestResolveAccessProfileIDUsesExplicitStableIdentity(t *testing.T) {
	profile := ResolveAccessProfileID("standard", "vip", "")
	require.Equal(t, "standard", profile.ID)
	require.Equal(t, "Standard access", profile.Label)

	custom := ResolveAccessProfileID("team-enterprise", "vip", "Configured custom profile")
	require.Equal(t, "team-enterprise", custom.ID)
	require.Equal(t, "Configured custom profile", custom.Description)
}

func TestMigrateAccessProfileIdentifiersBackfillsLegacyRows(t *testing.T) {
	require.NotNil(t, DB)
	require.NoError(t, DB.AutoMigrate(&User{}, &Token{}))
	suffix := time.Now().UnixNano()
	user := &User{
		Username: fmt.Sprintf("profile-migration-%d", suffix),
		Password: "migration-test-password",
		Group:    "vip",
	}
	require.NoError(t, DB.Create(user).Error)
	token := &Token{
		UserId: user.Id,
		Key:    fmt.Sprintf("migration-test-key-%d", suffix),
		Group:  "auto",
	}
	require.NoError(t, DB.Create(token).Error)
	t.Cleanup(func() {
		_ = DB.Unscoped().Delete(&Token{}, token.Id).Error
		_ = DB.Unscoped().Delete(&User{}, user.Id).Error
	})

	// Simulate a row written by a pre-identity version after AutoMigrate added
	// the columns with their default values.
	require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Update("account_tier_id", "").Error)
	require.NoError(t, DB.Model(&Token{}).Where("id = ?", token.Id).Update("access_profile_id", "").Error)
	require.NoError(t, MigrateAccessProfileIdentifiers())

	var migratedUser User
	var migratedToken Token
	require.NoError(t, DB.First(&migratedUser, user.Id).Error)
	require.NoError(t, DB.First(&migratedToken, token.Id).Error)
	require.Equal(t, "priority", migratedUser.AccountTierID)
	require.Equal(t, "automatic", migratedToken.AccessProfileID)
}

func TestTokenUpdatePreservesExplicitAccessProfileID(t *testing.T) {
	require.NotNil(t, DB)
	require.NoError(t, DB.AutoMigrate(&Token{}))
	suffix := time.Now().UnixNano()
	token := &Token{
		UserId: 7,
		Key:    fmt.Sprintf("explicit-profile-key-%d", suffix),
		Name:   "explicit-profile",
		Group:  "vip",
	}
	require.NoError(t, token.Insert())
	t.Cleanup(func() { _ = DB.Unscoped().Delete(&Token{}, token.Id).Error })

	token.AccessProfileID = "team-priority"
	token.Name = "updated-profile"
	require.NoError(t, token.Update())

	var got Token
	require.NoError(t, DB.First(&got, token.Id).Error)
	require.Equal(t, "team-priority", got.AccessProfileID)
}

func TestUserEditPreservesExplicitAccountTierID(t *testing.T) {
	require.NotNil(t, DB)
	require.NoError(t, DB.AutoMigrate(&User{}))
	suffix := time.Now().UnixNano()
	user := &User{
		Username:      fmt.Sprintf("explicit-tier-%d", suffix),
		Password:      "explicit-tier-password",
		Group:         "default",
		AccountTierID: "standard",
	}
	require.NoError(t, DB.Create(user).Error)
	t.Cleanup(func() { _ = DB.Unscoped().Delete(&User{}, user.Id).Error })

	edit := &User{
		Id:            user.Id,
		Username:      user.Username,
		DisplayName:   "Explicit tier",
		Group:         user.Group,
		AccountTierID: "team-enterprise",
	}
	require.NoError(t, edit.Edit(false))

	var got User
	require.NoError(t, DB.First(&got, user.Id).Error)
	require.Equal(t, "team-enterprise", got.AccountTierID)
	assert.Greater(t, got.AuthVersion, user.AuthVersion)
}
