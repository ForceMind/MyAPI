// Package accesspolicy defines the detached, side-effect-free S3 policy
// envelope used before policy enforcement is enabled. It deliberately has no
// dependency on storage, cache, HTTP, runtime settings, or legacy routing.
package accesspolicy

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	// CurrentSchemaVersion is the only snapshot schema this preparatory core
	// understands. A caller must not infer an unknown schema as an enforceable
	// policy.
	CurrentSchemaVersion uint16 = 1

	MaxStableIDLength   = 64
	MaxGroupLength      = 64
	MaxModelLength      = 255
	MaxListValues       = 128
	MaxTotalListValues  = 256
	MaxPriceRatioLength = 32

	canonicalDigestDomain = "myapi/access-policy-snapshot/v1"
)

// Stable errors let callers distinguish invalid input from an unsupported
// schema without risking an error that includes caller-controlled data.
var (
	ErrInvalidReference     = errors.New("invalid access policy reference")
	ErrInvalidList          = errors.New("invalid access policy list")
	ErrInvalidPolicyMode    = errors.New("invalid access policy mode")
	ErrUnsupportedSchema    = errors.New("unsupported access policy schema")
	ErrInvalidLegacyOutcome = errors.New("invalid legacy access policy outcome")
)

// ReferenceState records whether a stable policy reference was absent, known,
// disabled, or not resolvable. Unknown and disabled are diagnostic states, not
// permission decisions.
type ReferenceState string

const (
	ReferenceStateEmpty         ReferenceState = "empty"
	ReferenceStateKnownEnabled  ReferenceState = "known_enabled"
	ReferenceStateKnownDisabled ReferenceState = "known_disabled"
	ReferenceStateUnknown       ReferenceState = "unknown"
)

// ListPresence distinguishes a missing field from an explicitly empty list.
// This distinction is retained in the digest and in all detached outputs.
type ListPresence string

const (
	ListPresenceAbsent        ListPresence = "absent"
	ListPresenceExplicitEmpty ListPresence = "explicit_empty"
	ListPresenceValues        ListPresence = "values"
)

// PolicyMode is intentionally separated from application state. PRE1 only
// reports policy differences; it never changes legacy authorization, route,
// group, or price behavior.
type PolicyMode string

const (
	PolicyModeOff     PolicyMode = "off"
	PolicyModeAudit   PolicyMode = "audit"
	PolicyModeEnforce PolicyMode = "enforce"
)

// Reference is a bounded, non-secret stable identifier and its resolution
// state. Empty references carry no ID.
type Reference struct {
	State ReferenceState `json:"state"`
	ID    string         `json:"id,omitempty"`
}

// StringList is a canonical detached set. Values are sorted and de-duplicated
// whenever Presence is ListPresenceValues.
type StringList struct {
	Presence ListPresence `json:"presence"`
	Values   []string     `json:"values,omitempty"`
}

// AccessPolicySnapshot contains only policy metadata safe for diagnostics and
// a canonical content digest. RegistryRevision is a fixed numeric field; this
// package deliberately offers no arbitrary metadata, map, or opaque payload.
type AccessPolicySnapshot struct {
	SchemaVersion    uint16     `json:"schema_version"`
	Mode             PolicyMode `json:"mode"`
	RegistryRevision uint64     `json:"registry_revision"`

	AccountTier   Reference  `json:"account_tier"`
	AccessProfile Reference  `json:"access_profile"`
	AllowedGroups StringList `json:"allowed_groups"`
	AllowedModels StringList `json:"allowed_models"`
	AllowedRoutes StringList `json:"allowed_routes"`

	ContentDigest string `json:"content_digest"`
}

// LegacyPriceOutcome is the only accepted price result shape in PRE1. Ratios
// are canonical non-negative decimal strings. Accepted strings are copied
// byte-for-byte; callers must reject or adapt other legacy result shapes at
// their own boundary rather than placing opaque fields in this envelope.
type LegacyPriceOutcome struct {
	ModelRatio string `json:"model_ratio,omitempty"`
	GroupRatio string `json:"group_ratio,omitempty"`
}

// LegacyOutcome is the complete legacy outcome that this preparatory core
// preserves. Accepted allow/group/price values are not trimmed, sorted, or
// otherwise rewritten. UsingModel carries the model the legacy path resolved;
// it participates in policy comparison but is not a billing input.
type LegacyOutcome struct {
	Allow      bool               `json:"allow"`
	UsingGroup string             `json:"using_group,omitempty"`
	UsingModel string             `json:"using_model,omitempty"`
	Price      LegacyPriceOutcome `json:"price"`
}

// Finding is a stable, non-secret diagnostic observation. It does not imply a
// new authorization or pricing result.
type Finding struct {
	Code    string         `json:"code"`
	Subject string         `json:"subject,omitempty"`
	State   ReferenceState `json:"state,omitempty"`
}

// PolicyDecision is a detached envelope returned by Evaluate. Applied remains
// false in PRE1, including for an input that requests enforce, because D04
// has not authorized the core to change legacy behavior.
type PolicyDecision struct {
	Snapshot AccessPolicySnapshot `json:"snapshot"`
	Applied  bool                 `json:"applied"`
	Legacy   LegacyOutcome        `json:"legacy"`
	Findings []Finding            `json:"findings"`
}

// EvaluationInput supplies a candidate policy and the pre-existing result.
// AvailableModels and AvailableRoutes are optional inventories. Their absent
// state means dangling references cannot be determined by this invocation.
type EvaluationInput struct {
	Snapshot        AccessPolicySnapshot
	Legacy          LegacyOutcome
	AvailableModels StringList
	AvailableRoutes StringList
}

// NormalizeSnapshot validates, bounds, copies, and canonically digests a
// candidate snapshot. The input ContentDigest is never trusted or preserved.
func NormalizeSnapshot(input AccessPolicySnapshot) (AccessPolicySnapshot, error) {
	if rawListValueCount(input.AllowedGroups, input.AllowedModels, input.AllowedRoutes) > MaxTotalListValues {
		return AccessPolicySnapshot{}, ErrInvalidList
	}
	output := AccessPolicySnapshot{
		SchemaVersion:    input.SchemaVersion,
		Mode:             input.Mode,
		RegistryRevision: input.RegistryRevision,
	}
	if output.SchemaVersion == 0 {
		output.SchemaVersion = CurrentSchemaVersion
	}
	if output.SchemaVersion != CurrentSchemaVersion {
		return AccessPolicySnapshot{}, ErrUnsupportedSchema
	}
	if output.Mode == "" {
		output.Mode = PolicyModeOff
	}
	if output.Mode != PolicyModeOff && output.Mode != PolicyModeAudit && output.Mode != PolicyModeEnforce {
		return AccessPolicySnapshot{}, ErrInvalidPolicyMode
	}

	var err error
	if output.AccountTier, err = normalizeReference(input.AccountTier); err != nil {
		return AccessPolicySnapshot{}, err
	}
	if output.AccessProfile, err = normalizeReference(input.AccessProfile); err != nil {
		return AccessPolicySnapshot{}, err
	}
	if output.AllowedGroups, err = normalizeStringList(input.AllowedGroups, MaxGroupLength, true); err != nil {
		return AccessPolicySnapshot{}, err
	}
	if output.AllowedModels, err = normalizeStringList(input.AllowedModels, MaxModelLength, false); err != nil {
		return AccessPolicySnapshot{}, err
	}
	if output.AllowedRoutes, err = normalizeStringList(input.AllowedRoutes, MaxStableIDLength, true); err != nil {
		return AccessPolicySnapshot{}, err
	}
	if totalListValues(output.AllowedGroups, output.AllowedModels, output.AllowedRoutes) > MaxTotalListValues {
		return AccessPolicySnapshot{}, ErrInvalidList
	}

	output.ContentDigest = snapshotDigest(output)
	return output, nil
}

// Evaluate returns a detached diagnostic envelope. It intentionally retains
// the legacy allow, selected group, and price outcome for every mode. Off and
// audit are explicitly non-applied; enforce is also non-applied until the S3
// enforcement contract and migration gate are approved.
func Evaluate(input EvaluationInput) (PolicyDecision, error) {
	snapshot, err := NormalizeSnapshot(input.Snapshot)
	if err != nil {
		return PolicyDecision{}, err
	}
	legacy, err := validateAndCopyLegacyOutcome(input.Legacy)
	if err != nil {
		return PolicyDecision{}, err
	}
	availableModels, err := normalizeStringList(input.AvailableModels, MaxModelLength, false)
	if err != nil {
		return PolicyDecision{}, err
	}
	availableRoutes, err := normalizeStringList(input.AvailableRoutes, MaxStableIDLength, true)
	if err != nil {
		return PolicyDecision{}, err
	}

	findings := make([]Finding, 0, 8)
	findings = appendReferenceFinding(findings, "account_tier", snapshot.AccountTier)
	findings = appendReferenceFinding(findings, "access_profile", snapshot.AccessProfile)
	findings = appendDanglingFindings(findings, "model", snapshot.AllowedModels, availableModels)
	findings = appendDanglingFindings(findings, "route", snapshot.AllowedRoutes, availableRoutes)
	findings = appendPolicyDifferenceFinding(findings, snapshot.AllowedGroups, legacy.UsingGroup, "legacy_group_difference")
	findings = appendPolicyDifferenceFinding(findings, snapshot.AllowedModels, legacy.UsingModel, "legacy_model_difference")
	if snapshot.Mode == PolicyModeEnforce {
		findings = append(findings, Finding{Code: "enforcement_not_available"})
	}
	findings = canonicalFindings(findings)

	return PolicyDecision{
		Snapshot: snapshot,
		Applied:  false,
		Legacy:   legacy,
		Findings: findings,
	}, nil
}

func normalizeReference(input Reference) (Reference, error) {
	state := input.State
	if state == "" && input.ID == "" {
		state = ReferenceStateEmpty
	}
	if state != ReferenceStateEmpty && state != ReferenceStateKnownEnabled && state != ReferenceStateKnownDisabled && state != ReferenceStateUnknown {
		return Reference{}, ErrInvalidReference
	}
	if state == ReferenceStateEmpty {
		if input.ID != "" {
			return Reference{}, ErrInvalidReference
		}
		return Reference{State: ReferenceStateEmpty}, nil
	}
	if !isStableID(input.ID, MaxStableIDLength) {
		return Reference{}, ErrInvalidReference
	}
	return Reference{State: state, ID: input.ID}, nil
}

func normalizeStringList(input StringList, maxLength int, stableID bool) (StringList, error) {
	presence := input.Presence
	if presence == "" {
		if input.Values == nil {
			presence = ListPresenceAbsent
		} else if len(input.Values) == 0 {
			presence = ListPresenceExplicitEmpty
		} else {
			presence = ListPresenceValues
		}
	}
	switch presence {
	case ListPresenceAbsent:
		if input.Values != nil {
			return StringList{}, ErrInvalidList
		}
		return StringList{Presence: ListPresenceAbsent}, nil
	case ListPresenceExplicitEmpty:
		if len(input.Values) != 0 {
			return StringList{}, ErrInvalidList
		}
		return StringList{Presence: ListPresenceExplicitEmpty, Values: []string{}}, nil
	case ListPresenceValues:
		if len(input.Values) == 0 || len(input.Values) > MaxListValues {
			return StringList{}, ErrInvalidList
		}
	default:
		return StringList{}, ErrInvalidList
	}

	values := make([]string, 0, len(input.Values))
	seen := make(map[string]struct{}, len(input.Values))
	for _, value := range input.Values {
		valid := isSafePolicyText(value, maxLength)
		if stableID {
			valid = isStableID(value, maxLength)
		}
		if !valid {
			return StringList{}, ErrInvalidList
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		values = append(values, value)
	}
	sort.Strings(values)
	return StringList{Presence: ListPresenceValues, Values: values}, nil
}

func validateAndCopyLegacyOutcome(input LegacyOutcome) (LegacyOutcome, error) {
	if input.UsingGroup != "" && !isStableID(input.UsingGroup, MaxGroupLength) {
		return LegacyOutcome{}, ErrInvalidLegacyOutcome
	}
	if input.UsingModel != "" && !isSafePolicyText(input.UsingModel, MaxModelLength) {
		return LegacyOutcome{}, ErrInvalidLegacyOutcome
	}
	if !isCanonicalRatio(input.Price.ModelRatio) || !isCanonicalRatio(input.Price.GroupRatio) {
		return LegacyOutcome{}, ErrInvalidLegacyOutcome
	}
	return input, nil
}

func isStableID(value string, maxLength int) bool {
	if !isSafePolicyText(value, maxLength) {
		return false
	}
	for _, char := range value {
		if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || strings.ContainsRune("._:-/", char)) {
			return false
		}
	}
	return true
}

func isSafePolicyText(value string, maxLength int) bool {
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

func isCanonicalRatio(value string) bool {
	if value == "" {
		return true
	}
	if len(value) > MaxPriceRatioLength {
		return false
	}
	dotSeen := false
	digitSeen := false
	for index, char := range value {
		if char >= '0' && char <= '9' {
			digitSeen = true
			continue
		}
		if char == '.' && !dotSeen && index != 0 && index != len(value)-1 {
			dotSeen = true
			continue
		}
		return false
	}
	return digitSeen
}

func totalListValues(lists ...StringList) int {
	total := 0
	for _, list := range lists {
		total += len(list.Values)
	}
	return total
}

func rawListValueCount(lists ...StringList) int {
	total := 0
	for _, list := range lists {
		total += len(list.Values)
	}
	return total
}

func appendReferenceFinding(findings []Finding, subject string, reference Reference) []Finding {
	switch reference.State {
	case ReferenceStateEmpty:
		return append(findings, Finding{Code: "empty_reference", Subject: subject, State: reference.State})
	case ReferenceStateKnownDisabled:
		return append(findings, Finding{Code: "disabled_reference", Subject: subject, State: reference.State})
	case ReferenceStateUnknown:
		return append(findings, Finding{Code: "unknown_reference", Subject: subject, State: reference.State})
	default:
		return findings
	}
}

func appendDanglingFindings(findings []Finding, subject string, requested, available StringList) []Finding {
	if requested.Presence != ListPresenceValues || available.Presence == ListPresenceAbsent {
		return findings
	}
	availableValues := make(map[string]struct{}, len(available.Values))
	for _, value := range available.Values {
		availableValues[value] = struct{}{}
	}
	for _, value := range requested.Values {
		if _, exists := availableValues[value]; !exists {
			findings = append(findings, Finding{Code: "dangling_" + subject + "_reference", Subject: value, State: ReferenceStateUnknown})
		}
	}
	return findings
}

func appendPolicyDifferenceFinding(findings []Finding, allowed StringList, legacyValue, code string) []Finding {
	if legacyValue == "" || allowed.Presence == ListPresenceAbsent {
		return findings
	}
	for _, value := range allowed.Values {
		if value == legacyValue {
			return findings
		}
	}
	return append(findings, Finding{Code: code, Subject: legacyValue})
}

func canonicalFindings(input []Finding) []Finding {
	seen := make(map[string]struct{}, len(input))
	findings := make([]Finding, 0, len(input))
	for _, finding := range input {
		key := finding.Code + "\x00" + finding.Subject + "\x00" + string(finding.State)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		findings = append(findings, finding)
	}
	sort.Slice(findings, func(left, right int) bool {
		if findings[left].Code != findings[right].Code {
			return findings[left].Code < findings[right].Code
		}
		if findings[left].Subject != findings[right].Subject {
			return findings[left].Subject < findings[right].Subject
		}
		return findings[left].State < findings[right].State
	})
	return findings
}

func snapshotDigest(snapshot AccessPolicySnapshot) string {
	hash := sha256.New()
	writeCanonicalString(hash, canonicalDigestDomain)
	var schema [2]byte
	binary.BigEndian.PutUint16(schema[:], snapshot.SchemaVersion)
	_, _ = hash.Write(schema[:])
	writeCanonicalString(hash, string(snapshot.Mode))
	var revision [8]byte
	binary.BigEndian.PutUint64(revision[:], snapshot.RegistryRevision)
	_, _ = hash.Write(revision[:])
	writeCanonicalReference(hash, snapshot.AccountTier)
	writeCanonicalReference(hash, snapshot.AccessProfile)
	writeCanonicalList(hash, snapshot.AllowedGroups)
	writeCanonicalList(hash, snapshot.AllowedModels)
	writeCanonicalList(hash, snapshot.AllowedRoutes)
	return stringHex(hash.Sum(nil))
}

func writeCanonicalReference(hash interface{ Write([]byte) (int, error) }, reference Reference) {
	writeCanonicalString(hash, string(reference.State))
	writeCanonicalString(hash, reference.ID)
}

func writeCanonicalList(hash interface{ Write([]byte) (int, error) }, list StringList) {
	writeCanonicalString(hash, string(list.Presence))
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(list.Values)))
	_, _ = hash.Write(length[:])
	for _, value := range list.Values {
		writeCanonicalString(hash, value)
	}
}

func writeCanonicalString(hash interface{ Write([]byte) (int, error) }, value string) {
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(value)))
	_, _ = hash.Write(length[:])
	_, _ = hash.Write([]byte(value))
}

func stringHex(value []byte) string {
	const digits = "0123456789abcdef"
	output := make([]byte, len(value)*2)
	for index, byteValue := range value {
		output[index*2] = digits[byteValue>>4]
		output[index*2+1] = digits[byteValue&0x0f]
	}
	return string(output)
}
