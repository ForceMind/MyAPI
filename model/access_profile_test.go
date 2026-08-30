package model

import (
	"fmt"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/setting"
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
	raw := `{"standard":{"label":"Team standard","description":"Shared pool","route_groups":["default"],"model_allowlist":["gpt-5"],"fallback_profiles":["priority"],"enabled":true}}`
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
}
