package accesspolicy

import (
	"strings"

	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/setting"
)

// This file is the single production ingress point for the S3 policy core.
// It assembles a request-scoped snapshot from already-loaded request state
// (token record, user cache, profile registry) and never performs its own
// storage, cache, or network access. Callers decide whether the resulting
// PolicyDecision is only recorded (audit) or applied (scoped enforce); the
// detached core itself still never changes the legacy outcome.

// SnapshotSource carries the request-scoped identities the policy snapshot
// needs. All fields come from state the caller already loaded for the legacy
// path, so building a snapshot adds no I/O.
type SnapshotSource struct {
	// AccountTierID is the user's persisted stable tier identity.
	AccountTierID string
	// AccessProfileID is the token's persisted stable profile identity.
	AccessProfileID string
	// LegacyTokenGroup is the token's legacy group, used only as the
	// compatibility fallback when AccessProfileID is empty.
	LegacyTokenGroup string
	// UsingGroup is the legacy group the request resolved to.
	UsingGroup string
	// UsingModel is the model the legacy path resolved for this request.
	UsingModel string
	// LegacyAllowed reports the legacy authorization outcome.
	LegacyAllowed bool
	// ModelRatio and GroupRatio are the canonical ratio strings the legacy
	// billing path produced for this request, when known.
	ModelRatio string
	GroupRatio string
}

// BuildRequestSnapshot resolves tier/profile references against their current
// registries and produces a normalized, digested snapshot. Effective route and
// model lists are the intersection of Account Tier and Key Profile policy.
// Absent lists inherit the other side, while an explicit empty list denies all.
// Unknown or disabled references are represented as deny-all constraints so a
// stale stable ID can never widen access even before the reference finding is
// applied by the caller.
func BuildRequestSnapshot(source SnapshotSource) (AccessPolicySnapshot, error) {
	registry := setting.GetAccessProfileSetting()
	tier, tierReference := resolveTierReference(source.AccountTierID, registry)
	profile := model.ResolveAccessProfileIDWithSnapshot(strings.TrimSpace(source.AccessProfileID), source.LegacyTokenGroup, "", registry)
	profileReference := Reference{
		State: policyReferenceState(profile.Known, profile.Enabled),
		ID:    profile.ID,
	}

	tierGroups, tierModels := policyListsForReference(tier.RouteGroups, tier.ModelAllowlist, tierReference.State)
	profileGroups, profileModels := policyListsForReference(profile.RouteGroups, profile.ModelAllowlist, profileReference.State)
	effectiveGroups := intersectStringLists(tierGroups, profileGroups)
	effectiveModels := intersectStringLists(tierModels, profileModels)
	snapshot := AccessPolicySnapshot{
		SchemaVersion: CurrentSchemaVersion,
		Mode:          PolicyModeOff,
		AccountTier:   tierReference,
		AccessProfile: profileReference,
		AllowedGroups: effectiveGroups,
		AllowedModels: effectiveModels,
		AllowedRoutes: cloneStringList(effectiveGroups),
	}

	return NormalizeSnapshot(snapshot)
}

// EvaluateRequest builds the snapshot and evaluates it against the legacy
// outcome in one call, stamping the configured mode and registry revision
// onto the snapshot.
func EvaluateRequest(mode PolicyMode, source SnapshotSource, registryRevision uint64) (PolicyDecision, error) {
	snapshot, err := BuildRequestSnapshot(source)
	if err != nil {
		return PolicyDecision{}, err
	}
	snapshot.Mode = mode
	snapshot.RegistryRevision = registryRevision
	return Evaluate(EvaluationInput{
		Snapshot: snapshot,
		Legacy: LegacyOutcome{
			Allow:      source.LegacyAllowed,
			UsingGroup: source.UsingGroup,
			UsingModel: source.UsingModel,
			Price: LegacyPriceOutcome{
				ModelRatio: source.ModelRatio,
				GroupRatio: source.GroupRatio,
			},
		},
	})
}

// BlockingFindings filters a decision down to the findings that justify a
// rejection under scoped enforce: the request's resolved legacy group or
// model falls outside the policy's lists, or a referenced tier/profile is
// disabled or unknown. Diagnostic-only findings (empty references, dangling
// inventories, enforcement notices) are intentionally excluded so audit and
// enforce stay explainable.
func BlockingFindings(decision PolicyDecision) []Finding {
	blocking := make([]Finding, 0, len(decision.Findings))
	for _, finding := range decision.Findings {
		switch finding.Code {
		case "legacy_group_difference", "legacy_model_difference", "disabled_reference", "unknown_reference":
			blocking = append(blocking, finding)
		}
	}
	return blocking
}

func resolveTierReference(tierID string, registry *setting.AccessProfileSetting) (model.AccountTierMetadata, Reference) {
	tierID = strings.TrimSpace(tierID)
	if tierID == "" {
		return model.AccountTierMetadata{}, Reference{State: ReferenceStateEmpty}
	}
	tier := model.ResolveAccountTierIDWithSnapshot(tierID, "", registry)
	return tier, Reference{State: policyReferenceState(tier.Known, tier.Enabled), ID: tier.ID}
}

func policyReferenceState(known, enabled bool) ReferenceState {
	if !known {
		return ReferenceStateUnknown
	}
	if !enabled {
		return ReferenceStateKnownDisabled
	}
	return ReferenceStateKnownEnabled
}

func policyListsForReference(routeGroups, modelAllowlist []string, state ReferenceState) (StringList, StringList) {
	if state == ReferenceStateKnownDisabled || state == ReferenceStateUnknown {
		denyAll := StringList{Presence: ListPresenceExplicitEmpty, Values: []string{}}
		return denyAll, cloneStringList(denyAll)
	}
	return stringListFrom(routeGroups), stringListFrom(modelAllowlist)
}

// intersectStringLists applies D04 list semantics: absent is inherit, explicit
// empty is deny-all, and two value lists retain only their shared values.
func intersectStringLists(left, right StringList) StringList {
	if left.Presence == ListPresenceExplicitEmpty || right.Presence == ListPresenceExplicitEmpty {
		return StringList{Presence: ListPresenceExplicitEmpty, Values: []string{}}
	}
	if left.Presence == ListPresenceAbsent {
		return cloneStringList(right)
	}
	if right.Presence == ListPresenceAbsent {
		return cloneStringList(left)
	}

	rightValues := make(map[string]struct{}, len(right.Values))
	for _, value := range right.Values {
		rightValues[value] = struct{}{}
	}
	values := make([]string, 0, len(left.Values))
	seen := make(map[string]struct{}, len(left.Values))
	for _, value := range left.Values {
		if _, exists := rightValues[value]; !exists {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		values = append(values, value)
	}
	if len(values) == 0 {
		return StringList{Presence: ListPresenceExplicitEmpty, Values: []string{}}
	}
	return StringList{Presence: ListPresenceValues, Values: values}
}

func cloneStringList(list StringList) StringList {
	if list.Values == nil {
		return StringList{Presence: list.Presence}
	}
	return StringList{Presence: list.Presence, Values: append([]string{}, list.Values...)}
}

// stringListFrom preserves the absent/explicit-empty/values distinction from
// the operator-configured profile definition.
func stringListFrom(values []string) StringList {
	if values == nil {
		return StringList{Presence: ListPresenceAbsent}
	}
	if len(values) == 0 {
		return StringList{Presence: ListPresenceExplicitEmpty, Values: []string{}}
	}
	return StringList{Presence: ListPresenceValues, Values: append([]string(nil), values...)}
}
