package accesspolicy

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvaluatePreservesLegacyOutcomeForPreEnforcementModes(t *testing.T) {
	legacy := LegacyOutcome{
		Allow:      true,
		UsingGroup: "legacy-group",
		Price: LegacyPriceOutcome{
			ModelRatio: "1.25",
			GroupRatio: "0.80",
		},
	}
	for _, mode := range []PolicyMode{PolicyModeOff, PolicyModeAudit, PolicyModeEnforce} {
		t.Run(string(mode), func(t *testing.T) {
			decision, err := Evaluate(EvaluationInput{
				Snapshot: AccessPolicySnapshot{
					Mode: mode,
					AllowedGroups: StringList{
						Presence: ListPresenceValues,
						Values:   []string{"new-group"},
					},
				},
				Legacy: legacy,
			})
			require.NoError(t, err)
			assert.False(t, decision.Applied)
			assert.Equal(t, legacy, decision.Legacy)
			assert.Contains(t, findingCodes(decision.Findings), "legacy_group_difference")
			if mode == PolicyModeEnforce {
				assert.Contains(t, findingCodes(decision.Findings), "enforcement_not_available")
			}
		})
	}
}

func TestEvaluateReportsReferenceAndDanglingFindingsWithoutChangingLegacyOutcome(t *testing.T) {
	decision, err := Evaluate(EvaluationInput{
		Snapshot: AccessPolicySnapshot{
			Mode:          PolicyModeAudit,
			AccountTier:   Reference{State: ReferenceStateKnownDisabled, ID: "tier-a"},
			AccessProfile: Reference{State: ReferenceStateUnknown, ID: "profile-a"},
			AllowedModels: StringList{Presence: ListPresenceValues, Values: []string{"gpt-6", "gpt-5"}},
			AllowedRoutes: StringList{Presence: ListPresenceValues, Values: []string{"route-missing"}},
		},
		Legacy:          LegacyOutcome{Allow: false, UsingGroup: "legacy"},
		AvailableModels: StringList{Presence: ListPresenceValues, Values: []string{"gpt-5"}},
		AvailableRoutes: StringList{Presence: ListPresenceExplicitEmpty},
	})
	require.NoError(t, err)
	assert.False(t, decision.Applied)
	assert.False(t, decision.Legacy.Allow)
	assert.Equal(t, "legacy", decision.Legacy.UsingGroup)
	assert.Equal(t, []string{
		"dangling_model_reference",
		"dangling_route_reference",
		"disabled_reference",
		"unknown_reference",
	}, findingCodes(decision.Findings))
}

func TestNormalizeSnapshotRetainsListPresenceAndCanonicalizesSets(t *testing.T) {
	snapshot, err := NormalizeSnapshot(AccessPolicySnapshot{
		AllowedGroups: StringList{Presence: ListPresenceAbsent},
		AllowedModels: StringList{Presence: ListPresenceExplicitEmpty},
		AllowedRoutes: StringList{Values: []string{"route-b", "route-a", "route-a"}},
	})
	require.NoError(t, err)
	assert.Equal(t, ListPresenceAbsent, snapshot.AllowedGroups.Presence)
	assert.Nil(t, snapshot.AllowedGroups.Values)
	assert.Equal(t, ListPresenceExplicitEmpty, snapshot.AllowedModels.Presence)
	assert.Empty(t, snapshot.AllowedModels.Values)
	assert.Equal(t, ListPresenceValues, snapshot.AllowedRoutes.Presence)
	assert.Equal(t, []string{"route-a", "route-b"}, snapshot.AllowedRoutes.Values)
}

func TestNormalizeSnapshotCanonicalDigestIsStableForSetOrderAndDifferentForPresence(t *testing.T) {
	first, err := NormalizeSnapshot(AccessPolicySnapshot{
		Mode:             PolicyModeAudit,
		RegistryRevision: 42,
		AccountTier:      Reference{State: ReferenceStateKnownEnabled, ID: "tier-a"},
		AccessProfile:    Reference{State: ReferenceStateKnownEnabled, ID: "profile-a"},
		AllowedGroups:    StringList{Values: []string{"vip", "default"}},
	})
	require.NoError(t, err)
	second, err := NormalizeSnapshot(AccessPolicySnapshot{
		Mode:             PolicyModeAudit,
		RegistryRevision: 42,
		AccountTier:      Reference{State: ReferenceStateKnownEnabled, ID: "tier-a"},
		AccessProfile:    Reference{State: ReferenceStateKnownEnabled, ID: "profile-a"},
		AllowedGroups:    StringList{Values: []string{"default", "vip", "vip"}},
		ContentDigest:    "untrusted-input-digest",
	})
	require.NoError(t, err)
	absent, err := NormalizeSnapshot(AccessPolicySnapshot{})
	require.NoError(t, err)
	empty, err := NormalizeSnapshot(AccessPolicySnapshot{AllowedGroups: StringList{Presence: ListPresenceExplicitEmpty}})
	require.NoError(t, err)
	assert.Equal(t, first.ContentDigest, second.ContentDigest)
	assert.NotEqual(t, absent.ContentDigest, empty.ContentDigest)
}

func TestSnapshotDigestKnownAnswer(t *testing.T) {
	snapshot, err := NormalizeSnapshot(AccessPolicySnapshot{
		SchemaVersion:    CurrentSchemaVersion,
		Mode:             PolicyModeAudit,
		RegistryRevision: 42,
		AccountTier:      Reference{State: ReferenceStateKnownEnabled, ID: "tier-a"},
		AccessProfile:    Reference{State: ReferenceStateUnknown, ID: "profile-b"},
		AllowedGroups:    StringList{Presence: ListPresenceAbsent},
		AllowedModels:    StringList{Presence: ListPresenceExplicitEmpty},
		AllowedRoutes:    StringList{Presence: ListPresenceValues, Values: []string{"route-b", "route-a"}},
	})
	require.NoError(t, err)
	assert.Equal(t, "5bde8f5003a042b663565386135f861f538f4516084fd80f7c89d02ad7cf2066", snapshot.ContentDigest)
}

func TestEvaluateProducesDetachedResults(t *testing.T) {
	input := EvaluationInput{
		Snapshot: AccessPolicySnapshot{AllowedGroups: StringList{Values: []string{"default"}}},
		Legacy: LegacyOutcome{Price: LegacyPriceOutcome{
			ModelRatio: "1.5",
			GroupRatio: "2",
		}},
	}
	decision, err := Evaluate(input)
	require.NoError(t, err)
	decision.Snapshot.AllowedGroups.Values[0] = "changed"
	decision.Legacy.Price.ModelRatio = "99"
	assert.Equal(t, "default", input.Snapshot.AllowedGroups.Values[0])
	assert.Equal(t, "1.5", input.Legacy.Price.ModelRatio)
}

func TestNormalizeSnapshotRejectsInvalidPresenceCombinations(t *testing.T) {
	cases := []StringList{
		{Presence: ListPresenceAbsent, Values: []string{}},
		{Presence: ListPresenceExplicitEmpty, Values: []string{"default"}},
		{Presence: ListPresenceValues},
		{Presence: ListPresence("unknown")},
	}
	for _, list := range cases {
		_, err := NormalizeSnapshot(AccessPolicySnapshot{AllowedGroups: list})
		require.ErrorIs(t, err, ErrInvalidList)
	}
}

func TestRejectsExactBoundariesAndUnsafeUnicode(t *testing.T) {
	validID := strings.Repeat("a", MaxStableIDLength)
	_, err := NormalizeSnapshot(AccessPolicySnapshot{
		AccountTier:   Reference{State: ReferenceStateKnownEnabled, ID: validID},
		AllowedModels: StringList{Values: []string{strings.Repeat("m", MaxModelLength)}},
	})
	require.NoError(t, err)

	_, err = NormalizeSnapshot(AccessPolicySnapshot{
		AccountTier: Reference{State: ReferenceStateKnownEnabled, ID: strings.Repeat("a", MaxStableIDLength+1)},
	})
	require.ErrorIs(t, err, ErrInvalidReference)
	_, err = NormalizeSnapshot(AccessPolicySnapshot{
		AllowedGroups: StringList{Values: []string{strings.Repeat("g", MaxGroupLength+1)}},
	})
	require.ErrorIs(t, err, ErrInvalidList)

	invalidValues := []string{
		strings.Repeat("m", MaxModelLength+1),
		string([]byte{'a', 0x80}),
		"model\x00name",
		"model\u0085name",
		"model\u202ename",
	}
	for _, value := range invalidValues {
		_, err := NormalizeSnapshot(AccessPolicySnapshot{
			AllowedModels: StringList{Values: []string{value}},
		})
		require.ErrorIs(t, err, ErrInvalidList)
	}
}

func TestLegacyOutcomePreservesAcceptedBytesAndRejectsUnrepresentableFields(t *testing.T) {
	legacy := LegacyOutcome{
		Allow:      true,
		UsingGroup: "legacy-group",
		Price: LegacyPriceOutcome{
			ModelRatio: "10.250",
			GroupRatio: "0.5",
		},
	}
	decision, err := Evaluate(EvaluationInput{Legacy: legacy})
	require.NoError(t, err)
	assert.Equal(t, legacy, decision.Legacy)

	cases := []struct {
		name   string
		legacy LegacyOutcome
		canary string
	}{
		{name: "group", legacy: LegacyOutcome{UsingGroup: " legacy-group"}, canary: " legacy-group"},
		{name: "model ratio", legacy: LegacyOutcome{Price: LegacyPriceOutcome{ModelRatio: "1e3"}}, canary: "1e3"},
		{name: "group ratio", legacy: LegacyOutcome{Price: LegacyPriceOutcome{GroupRatio: " 1"}}, canary: " 1"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := Evaluate(EvaluationInput{Legacy: testCase.legacy})
			require.ErrorIs(t, err, ErrInvalidLegacyOutcome)
			assert.NotContains(t, err.Error(), testCase.canary)
			assert.False(t, errors.Is(err, ErrInvalidList))
		})
	}
}

func TestLegacyPriceOutcomeHasOnlyFixedPublicStringFields(t *testing.T) {
	priceType := reflect.TypeOf(LegacyPriceOutcome{})
	require.Equal(t, 2, priceType.NumField())
	for index, fieldName := range []string{"ModelRatio", "GroupRatio"} {
		field := priceType.Field(index)
		assert.Equal(t, fieldName, field.Name)
		assert.Equal(t, reflect.TypeOf(""), field.Type)
		assert.False(t, field.Anonymous)
	}
}

func TestRawTotalLimitAppliesBeforeDeduplication(t *testing.T) {
	_, err := NormalizeSnapshot(AccessPolicySnapshot{
		AllowedGroups: StringList{Values: repeatedValues("group", MaxListValues)},
		AllowedModels: StringList{Values: repeatedValues("model", MaxListValues)},
		AllowedRoutes: StringList{Values: repeatedValues("route", MaxListValues)},
	})
	require.ErrorIs(t, err, ErrInvalidList)
}

func repeatedValues(value string, count int) []string {
	values := make([]string, count)
	for index := range values {
		values[index] = value
	}
	return values
}

func findingCodes(findings []Finding) []string {
	codes := make([]string, 0, len(findings))
	for _, finding := range findings {
		codes = append(codes, finding.Code)
	}
	return codes
}
