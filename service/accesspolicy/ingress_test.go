package accesspolicy

import (
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// applyProfileDefinitionsForTest installs operator profile definitions into
// the shared registry and restores the previous registry on cleanup. The
// registry is process-global, so tests using it must not run in parallel.
func applyProfileDefinitionsForTest(t *testing.T, raw string) {
	t.Helper()
	original, err := common.Marshal(setting.GetAccessProfileSetting().Profiles)
	require.NoError(t, err)
	require.NoError(t, setting.UpdateAccessProfileDefinitionsByJSONString(raw))
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateAccessProfileDefinitionsByJSONString(string(original)))
	})
}

func applyTierDefinitionsForTest(t *testing.T, raw string) {
	t.Helper()
	original, err := common.Marshal(setting.GetAccessProfileSetting().AccountTiers)
	require.NoError(t, err)
	require.NoError(t, setting.UpdateAccountTierDefinitionsByJSONString(raw))
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateAccountTierDefinitionsByJSONString(string(original)))
	})
}

func TestBuildRequestSnapshotResolvesStableIdentities(t *testing.T) {
	snapshot, err := BuildRequestSnapshot(SnapshotSource{
		AccountTierID:    "priority",
		AccessProfileID:  "standard",
		LegacyTokenGroup: "default",
		UsingGroup:       "default",
		LegacyAllowed:    true,
	})
	require.NoError(t, err)
	assert.Equal(t, CurrentSchemaVersion, snapshot.SchemaVersion)
	assert.Equal(t, PolicyModeOff, snapshot.Mode)
	assert.Equal(t, Reference{State: ReferenceStateKnownEnabled, ID: "priority"}, snapshot.AccountTier)
	assert.Equal(t, Reference{State: ReferenceStateKnownEnabled, ID: "standard"}, snapshot.AccessProfile)
	// The built-in standard profile configures no lists: they stay absent.
	assert.Equal(t, ListPresenceAbsent, snapshot.AllowedGroups.Presence)
	assert.NotEmpty(t, snapshot.ContentDigest)
}

func TestBuildRequestSnapshotFallsBackToLegacyGroup(t *testing.T) {
	snapshot, err := BuildRequestSnapshot(SnapshotSource{
		AccessProfileID:  "",
		LegacyTokenGroup: "vip",
		UsingGroup:       "vip",
		LegacyAllowed:    true,
	})
	require.NoError(t, err)
	assert.Equal(t, "priority", snapshot.AccessProfile.ID)
}

func TestBuildRequestSnapshotPreservesListPresence(t *testing.T) {
	applyProfileDefinitionsForTest(t, `{"custom-profile":{"label":"Custom","route_groups":[],"model_allowlist":["gpt-x"]}}`)

	snapshot, err := BuildRequestSnapshot(SnapshotSource{
		AccessProfileID: "custom-profile",
		UsingGroup:      "legacy-group",
		LegacyAllowed:   true,
	})
	require.NoError(t, err)
	assert.Equal(t, ListPresenceExplicitEmpty, snapshot.AllowedGroups.Presence)
	assert.Equal(t, ListPresenceValues, snapshot.AllowedModels.Presence)
	assert.Equal(t, []string{"gpt-x"}, snapshot.AllowedModels.Values)
}

func TestBuildRequestSnapshotIntersectsTierAndProfileEntitlements(t *testing.T) {
	applyTierDefinitionsForTest(t, `{
		"team":{"label":"Team","route_groups":["shared","tier-only"],"model_allowlist":["gpt-shared","gpt-tier"]}
	}`)
	applyProfileDefinitionsForTest(t, `{
		"key":{"label":"Key","route_groups":["profile-only","shared"],"model_allowlist":["gpt-profile","gpt-shared"]}
	}`)

	snapshot, err := BuildRequestSnapshot(SnapshotSource{
		AccountTierID:   "team",
		AccessProfileID: "key",
	})
	require.NoError(t, err)
	assert.Equal(t, Reference{State: ReferenceStateKnownEnabled, ID: "team"}, snapshot.AccountTier)
	assert.Equal(t, Reference{State: ReferenceStateKnownEnabled, ID: "key"}, snapshot.AccessProfile)
	assert.Equal(t, StringList{Presence: ListPresenceValues, Values: []string{"shared"}}, snapshot.AllowedGroups)
	assert.Equal(t, snapshot.AllowedGroups, snapshot.AllowedRoutes)
	assert.Equal(t, StringList{Presence: ListPresenceValues, Values: []string{"gpt-shared"}}, snapshot.AllowedModels)
}

func TestIntersectStringListsPreservesD04PresenceSemantics(t *testing.T) {
	absent := StringList{Presence: ListPresenceAbsent}
	empty := StringList{Presence: ListPresenceExplicitEmpty, Values: []string{}}
	left := StringList{Presence: ListPresenceValues, Values: []string{"a", "shared"}}
	right := StringList{Presence: ListPresenceValues, Values: []string{"shared", "b"}}
	disjoint := StringList{Presence: ListPresenceValues, Values: []string{"other"}}

	assert.Equal(t, absent, intersectStringLists(absent, absent))
	assert.Equal(t, left, intersectStringLists(absent, left))
	assert.Equal(t, right, intersectStringLists(right, absent))
	assert.Equal(t, empty, intersectStringLists(empty, left))
	assert.Equal(t, empty, intersectStringLists(left, empty))
	assert.Equal(t, StringList{Presence: ListPresenceValues, Values: []string{"shared"}}, intersectStringLists(left, right))
	assert.Equal(t, empty, intersectStringLists(left, disjoint))
}

func TestBuildRequestSnapshotUnknownTierCannotExpandProfile(t *testing.T) {
	applyTierDefinitionsForTest(t, `{"known":{"label":"Known"}}`)
	applyProfileDefinitionsForTest(t, `{
		"key":{"label":"Key","route_groups":["default"],"model_allowlist":["gpt-5"]}
	}`)

	decision, err := EvaluateRequest(PolicyModeEnforce, SnapshotSource{
		AccountTierID:   "missing",
		AccessProfileID: "key",
		UsingGroup:      "default",
		UsingModel:      "gpt-5",
		LegacyAllowed:   true,
	}, 0)
	require.NoError(t, err)
	assert.Equal(t, Reference{State: ReferenceStateUnknown, ID: "missing"}, decision.Snapshot.AccountTier)
	assert.Equal(t, ListPresenceExplicitEmpty, decision.Snapshot.AllowedGroups.Presence)
	assert.Equal(t, ListPresenceExplicitEmpty, decision.Snapshot.AllowedModels.Presence)
	assert.ElementsMatch(t,
		[]string{"legacy_group_difference", "legacy_model_difference", "unknown_reference"},
		findingCodes(BlockingFindings(decision)),
	)
}

func TestBuildRequestSnapshotDisabledProfileCannotInheritTierAccess(t *testing.T) {
	applyTierDefinitionsForTest(t, `{
		"team":{"label":"Team","route_groups":["default"],"model_allowlist":["gpt-5"]}
	}`)
	applyProfileDefinitionsForTest(t, `{
		"disabled":{"label":"Disabled","enabled":false}
	}`)

	decision, err := EvaluateRequest(PolicyModeEnforce, SnapshotSource{
		AccountTierID:   "team",
		AccessProfileID: "disabled",
		UsingGroup:      "default",
		UsingModel:      "gpt-5",
		LegacyAllowed:   true,
	}, 0)
	require.NoError(t, err)
	assert.Equal(t, Reference{State: ReferenceStateKnownDisabled, ID: "disabled"}, decision.Snapshot.AccessProfile)
	assert.Equal(t, ListPresenceExplicitEmpty, decision.Snapshot.AllowedGroups.Presence)
	assert.Equal(t, ListPresenceExplicitEmpty, decision.Snapshot.AllowedModels.Presence)
	assert.Contains(t, findingCodes(BlockingFindings(decision)), "disabled_reference")
}

func TestBuildRequestSnapshotAbsentTierListsInheritProfileAndExplicitEmptyDenies(t *testing.T) {
	applyTierDefinitionsForTest(t, `{
		"inherit":{"label":"Inherit"},
		"deny-models":{"label":"Deny models","model_allowlist":[]}
	}`)
	applyProfileDefinitionsForTest(t, `{
		"key":{"label":"Key","route_groups":["default"],"model_allowlist":["gpt-5"]}
	}`)

	inherited, err := BuildRequestSnapshot(SnapshotSource{AccountTierID: "inherit", AccessProfileID: "key"})
	require.NoError(t, err)
	assert.Equal(t, StringList{Presence: ListPresenceValues, Values: []string{"default"}}, inherited.AllowedGroups)
	assert.Equal(t, StringList{Presence: ListPresenceValues, Values: []string{"gpt-5"}}, inherited.AllowedModels)

	denied, err := BuildRequestSnapshot(SnapshotSource{AccountTierID: "deny-models", AccessProfileID: "key"})
	require.NoError(t, err)
	assert.Equal(t, StringList{Presence: ListPresenceValues, Values: []string{"default"}}, denied.AllowedGroups)
	assert.Equal(t, ListPresenceExplicitEmpty, denied.AllowedModels.Presence)
}

func TestEvaluateRequestAuditFlagsGroupDifferenceWithoutBlockingLegacy(t *testing.T) {
	applyProfileDefinitionsForTest(t, `{"narrow":{"label":"Narrow","route_groups":["allowed-group"]}}`)

	decision, err := EvaluateRequest(PolicyModeAudit, SnapshotSource{
		AccessProfileID: "narrow",
		UsingGroup:      "legacy-group",
		LegacyAllowed:   true,
		GroupRatio:      "1.5",
	}, 7)
	require.NoError(t, err)
	assert.False(t, decision.Applied)
	assert.True(t, decision.Legacy.Allow)
	assert.Equal(t, "legacy-group", decision.Legacy.UsingGroup)
	assert.Equal(t, "1.5", decision.Legacy.Price.GroupRatio)
	assert.Equal(t, uint64(7), decision.Snapshot.RegistryRevision)
	assert.Equal(t, PolicyModeAudit, decision.Snapshot.Mode)

	blocking := BlockingFindings(decision)
	require.Len(t, blocking, 1)
	assert.Equal(t, "legacy_group_difference", blocking[0].Code)
	assert.Equal(t, "legacy-group", blocking[0].Subject)
}

func TestBlockingFindingsExcludesDiagnosticOnlyCodes(t *testing.T) {
	decision, err := EvaluateRequest(PolicyModeEnforce, SnapshotSource{
		UsingGroup:    "default",
		LegacyAllowed: true,
	}, 0)
	require.NoError(t, err)
	// The empty tier reference and the enforcement notice are diagnostics.
	assert.Empty(t, BlockingFindings(decision))
	assert.Contains(t, findingCodes(decision.Findings), "empty_reference")
	assert.Contains(t, findingCodes(decision.Findings), "enforcement_not_available")
}

func TestEvaluateRequestFlagsModelOutsideAllowlist(t *testing.T) {
	applyProfileDefinitionsForTest(t, `{"model-gated":{"label":"Model gated","model_allowlist":["gpt-allowed"]}}`)

	decision, err := EvaluateRequest(PolicyModeAudit, SnapshotSource{
		AccessProfileID: "model-gated",
		UsingGroup:      "default",
		UsingModel:      "gpt-blocked",
		LegacyAllowed:   true,
	}, 0)
	require.NoError(t, err)
	assert.False(t, decision.Applied)
	assert.True(t, decision.Legacy.Allow)
	assert.Equal(t, "gpt-blocked", decision.Legacy.UsingModel)

	blocking := BlockingFindings(decision)
	require.Len(t, blocking, 1)
	assert.Equal(t, "legacy_model_difference", blocking[0].Code)
	assert.Equal(t, "gpt-blocked", blocking[0].Subject)
}

func TestEvaluateRequestModelInsideAllowlistProducesNoModelFinding(t *testing.T) {
	applyProfileDefinitionsForTest(t, `{"model-gated":{"label":"Model gated","model_allowlist":["gpt-allowed"]}}`)

	decision, err := EvaluateRequest(PolicyModeAudit, SnapshotSource{
		AccessProfileID: "model-gated",
		UsingGroup:      "default",
		UsingModel:      "gpt-allowed",
		LegacyAllowed:   true,
	}, 0)
	require.NoError(t, err)
	assert.NotContains(t, findingCodes(decision.Findings), "legacy_model_difference")
	assert.Empty(t, BlockingFindings(decision))
}

func TestEvaluateRequestAbsentModelAllowlistDoesNotRestrict(t *testing.T) {
	// The built-in standard profile has no model allowlist: any model passes.
	decision, err := EvaluateRequest(PolicyModeAudit, SnapshotSource{
		AccessProfileID: "standard",
		UsingGroup:      "default",
		UsingModel:      "any-model",
		LegacyAllowed:   true,
	}, 0)
	require.NoError(t, err)
	assert.NotContains(t, findingCodes(decision.Findings), "legacy_model_difference")
	assert.Empty(t, BlockingFindings(decision))
}

func TestEvaluateRequestExplicitEmptyModelAllowlistDeniesAllModels(t *testing.T) {
	applyProfileDefinitionsForTest(t, `{"no-models":{"label":"No models","model_allowlist":[]}}`)

	decision, err := EvaluateRequest(PolicyModeEnforce, SnapshotSource{
		AccessProfileID: "no-models",
		UsingGroup:      "default",
		UsingModel:      "any-model",
		LegacyAllowed:   true,
	}, 0)
	require.NoError(t, err)
	blocking := BlockingFindings(decision)
	require.Len(t, blocking, 1)
	assert.Equal(t, "legacy_model_difference", blocking[0].Code)
}

func TestValidateLegacyOutcomeRejectsInvalidUsingModel(t *testing.T) {
	_, err := Evaluate(EvaluationInput{
		Snapshot: AccessPolicySnapshot{Mode: PolicyModeAudit},
		Legacy: LegacyOutcome{
			Allow:      true,
			UsingGroup: "default",
			UsingModel: "bad\nmodel",
		},
	})
	require.ErrorIs(t, err, ErrInvalidLegacyOutcome)
}
