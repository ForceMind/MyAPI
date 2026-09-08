// Package promptlearning contains the pure admission, sanitization, and
// fingerprinting boundary for prompt-learning samples. It deliberately has no
// persistence, transport, worker, relay, credential, or logging dependencies.
package promptlearning

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

const (
	DefaultMaxTextBytes = 64 * 1024
	MaxTextBytesLimit   = 1024 * 1024
	minimumHMACKeyBytes = 32
	maximumRetiredKeys  = 8
	maxProvenanceBytes  = 256

	TextPolicyVersion        = "nfkc-lf-tab-cf-reject-v4"
	RedactionPolicyVersion   = "redaction-context-state-machine-v6"
	FingerprintPolicyVersion = "hmac-sha256-v2"
	OccurrencePolicyVersion  = "occurrence-v2"
	observationPolicyVersion = "observation-v1"
)

// Confidence determines how a caller may use a reviewed sample. Trusted means
// only that a future persistence layer may attempt a unique occurrence insert;
// the kernel never declares an occurrence counted.
type Confidence string

const (
	ConfidenceTrusted  Confidence = "trusted"
	ConfidenceMedium   Confidence = "medium"
	ConfidenceLow      Confidence = "low"
	ConfidenceRejected Confidence = "rejected"
)

// Reason is deliberately machine-readable and never includes submitted text.
type Reason string

const (
	ReasonAcceptedLiveUserTurn    Reason = "accepted_live_user_turn"
	ReasonAcceptedTrustedImport   Reason = "accepted_trusted_import"
	ReasonAcceptedLegacyCandidate Reason = "accepted_legacy_candidate"
	ReasonAcceptedUntrustedClaim  Reason = "accepted_untrusted_candidate"
	ReasonUntrustedOriginClaim    Reason = "untrusted_origin_claim"
	ReasonHistoricalContent       Reason = "historical_content"
	ReasonSystemContent           Reason = "system_content"
	ReasonDeveloperContent        Reason = "developer_content"
	ReasonAssistantContent        Reason = "assistant_content"
	ReasonToolContent             Reason = "tool_content"
	ReasonAttachmentContent       Reason = "attachment_content"
	ReasonResponseContent         Reason = "response_content"
	ReasonInternalContent         Reason = "internal_content"
	ReasonInvalidCandidate        Reason = "invalid_candidate"
	ReasonInvalidProvenance       Reason = "invalid_provenance"
	ReasonTextTooLarge            Reason = "text_too_large"
	ReasonInvalidUTF8             Reason = "invalid_utf8"
	ReasonUnsupportedControl      Reason = "unsupported_control_character"
	ReasonUnsafeUnicodeFormat     Reason = "unsafe_unicode_format_character"
	ReasonEmptyText               Reason = "empty_text"
	ReasonRedactedTextTooLarge    Reason = "redacted_text_too_large"
	ReasonRedactionFailed         Reason = "redaction_failed"
)

// ExcludedContent identifies material that must never become a learning sample.
type ExcludedContent string

const (
	ContentHistory      ExcludedContent = "history"
	ContentSystem       ExcludedContent = "system"
	ContentDeveloper    ExcludedContent = "developer"
	ContentAssistant    ExcludedContent = "assistant"
	ContentTool         ExcludedContent = "tool"
	ContentAttachment   ExcludedContent = "attachment"
	ContentResponse     ExcludedContent = "response"
	ContentInternalTask ExcludedContent = "internal_task"
	ContentAutoAnalysis ExcludedContent = "automatic_analysis"
)

// scopeID is an opaque, server-constructed isolation boundary. Its receipts
// identify the policy generation, owning principal, and subject scope. The
// subject scope receipt may represent a key, user, project, or another
// server-owned subject, but must never contain the credential or expose a scope
// kind to an external client.
type scopeID struct {
	policyReceipt       string
	ownerReceipt        string
	subjectScopeReceipt string
}

func newScopeID(policyReceipt, ownerReceipt, subjectScopeReceipt string) scopeID {
	return scopeID{
		policyReceipt:       policyReceipt,
		ownerReceipt:        ownerReceipt,
		subjectScopeReceipt: subjectScopeReceipt,
	}
}

// liveTurnIdentity is opaque server-observed provenance. A future caller must
// obtain the conversation and turn receipts from a trusted server ingress or
// session system, never client JSON. The physical node and epoch are retained
// and validated only as audit provenance; they do not define a logical turn or
// participate in its logical fingerprints.
type liveTurnIdentity struct {
	scope                scopeID
	sourceNodeID         string
	sourceEpoch          string
	conversationReceipt  string
	turnReceipt          string
	requestObservationID string
}

// newLiveTurnIdentity retains requestObservationID only for a separate physical
// observation binding. Logical occurrence and transport payload fingerprints
// deliberately exclude it so retries observed under different HTTP request IDs
// still collapse.
func newLiveTurnIdentity(
	scope scopeID,
	sourceNodeID string,
	sourceEpoch string,
	conversationReceipt string,
	turnReceipt string,
	requestObservationID string,
) liveTurnIdentity {
	return liveTurnIdentity{
		scope:                scope,
		sourceNodeID:         sourceNodeID,
		sourceEpoch:          sourceEpoch,
		conversationReceipt:  conversationReceipt,
		turnReceipt:          turnReceipt,
		requestObservationID: requestObservationID,
	}
}

// importIdentity identifies an event from an explicitly trusted server import
// path. Imported records remain medium confidence and do not count as live
// turns.
type importIdentity struct {
	scope        scopeID
	sourceNodeID string
	sourceEpoch  string
	eventReceipt string
}

func newImportIdentity(scope scopeID, sourceNodeID, sourceEpoch, eventReceipt string) importIdentity {
	return importIdentity{
		scope:        scope,
		sourceNodeID: sourceNodeID,
		sourceEpoch:  sourceEpoch,
		eventReceipt: eventReceipt,
	}
}

type candidateSource uint8

const (
	sourceInvalid candidateSource = iota
	sourceLiveUserTurn
	sourceTrustedImport
	sourceLegacyCandidate
	sourceUntrustedClient
	sourceExcluded
)

// candidate is package-private and every constructor is package-private, so
// an external caller cannot mint trusted provenance. A future trusted ingress
// facade must live in this package and construct candidates only from records it
// observed and authenticated on the server. This pure kernel does not itself
// prove that a future ingress integration is trustworthy.
type candidate struct {
	text          string
	source        candidateSource
	scope         scopeID
	liveIdentity  liveTurnIdentity
	importID      importIdentity
	claimedOrigin string
	excluded      ExcludedContent
}

func newLiveUserTurn(text string, identity liveTurnIdentity) candidate {
	return candidate{text: text, source: sourceLiveUserTurn, scope: identity.scope, liveIdentity: identity}
}

func newTrustedImportedUserTurn(text string, identity importIdentity) candidate {
	return candidate{text: text, source: sourceTrustedImport, scope: identity.scope, importID: identity}
}

func newLegacyUserCandidate(text string, scope scopeID) candidate {
	return candidate{text: text, source: sourceLegacyCandidate, scope: scope}
}

// newUntrustedClientCandidate cannot create a persistence-eligible candidate.
// A non-empty claimedOrigin is rejected instead of interpreting a
// client-supplied internal or server origin string.
func newUntrustedClientCandidate(text string, scope scopeID, claimedOrigin string) candidate {
	return candidate{text: text, source: sourceUntrustedClient, scope: scope, claimedOrigin: claimedOrigin}
}

func newExcludedCandidate(text string, content ExcludedContent) candidate {
	return candidate{text: text, source: sourceExcluded, excluded: content}
}

// HMACKeyVersion is one version of two domain-specific fingerprint keys. Key
// material is copied by NewKernel and is never returned by this package.
type HMACKeyVersion struct {
	Version      string
	TransportKey []byte
	SemanticKey  []byte
}

type Config struct {
	ActiveKey    HMACKeyVersion
	RetiredKeys  []HMACKeyVersion
	MaxTextBytes int
}

type keyVersion struct {
	version      string
	transportKey []byte
	semanticKey  []byte
}

// Kernel is immutable after construction and safe for concurrent use.
type Kernel struct {
	keys         []keyVersion
	maxTextBytes int
}

var ErrInvalidConfig = errors.New("invalid prompt-learning kernel configuration")

func NewKernel(config Config) (*Kernel, error) {
	maxTextBytes := config.MaxTextBytes
	if maxTextBytes == 0 {
		maxTextBytes = DefaultMaxTextBytes
	}
	if maxTextBytes < 1 || maxTextBytes > MaxTextBytesLimit || len(config.RetiredKeys) > maximumRetiredKeys {
		return nil, ErrInvalidConfig
	}

	configured := make([]HMACKeyVersion, 0, 1+len(config.RetiredKeys))
	configured = append(configured, config.ActiveKey)
	configured = append(configured, config.RetiredKeys...)
	keys := make([]keyVersion, 0, len(configured))
	versions := make(map[string]struct{}, len(configured))
	for _, item := range configured {
		if !validKeyVersion(item.Version) || len(item.TransportKey) < minimumHMACKeyBytes || len(item.SemanticKey) < minimumHMACKeyBytes {
			return nil, ErrInvalidConfig
		}
		if hmac.Equal(item.TransportKey, item.SemanticKey) {
			return nil, ErrInvalidConfig
		}
		if _, exists := versions[item.Version]; exists {
			return nil, ErrInvalidConfig
		}
		versions[item.Version] = struct{}{}
		keys = append(keys, keyVersion{
			version:      item.Version,
			transportKey: append([]byte(nil), item.TransportKey...),
			semanticKey:  append([]byte(nil), item.SemanticKey...),
		})
	}

	return &Kernel{keys: keys, maxTextBytes: maxTextBytes}, nil
}

func validKeyVersion(version string) bool {
	if len(version) == 0 || len(version) > 64 {
		return false
	}
	for _, char := range version {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '.' || char == '_' || char == '-' {
			continue
		}
		return false
	}
	return true
}

type versionedDigest struct {
	keyVersion string
	digest     string
}

type fingerprints struct {
	// Active-key digest is first. Retired-key aliases allow dedupe to continue
	// across bounded key rotation without making an old key active again.
	// Transport binds normalized pre-redaction payload under HMAC so two payloads
	// that redact to the same preview can still be detected as a conflict.
	// Observation separately binds the trusted physical provenance to that
	// normalized payload without changing logical occurrence or retry semantics.
	policyVersion      string
	occurrenceVersion  string
	observationVersion string
	occurrence         []versionedDigest
	observation        []versionedDigest
	transport          []versionedDigest
	semantic           []versionedDigest
}

func (fingerprints fingerprints) matchesTransport(stored versionedDigest) bool {
	return matchesDigest(fingerprints.transport, stored)
}

func (fingerprints fingerprints) matchesOccurrence(stored versionedDigest) bool {
	return matchesDigest(fingerprints.occurrence, stored)
}

func (fingerprints fingerprints) matchesObservation(stored versionedDigest) bool {
	return matchesDigest(fingerprints.observation, stored)
}

func (fingerprints fingerprints) matchesSemantic(stored versionedDigest) bool {
	return matchesDigest(fingerprints.semantic, stored)
}

func matchesDigest(candidates []versionedDigest, stored versionedDigest) bool {
	for _, candidate := range candidates {
		if candidate.keyVersion == stored.keyVersion && hmac.Equal([]byte(candidate.digest), []byte(stored.digest)) {
			return true
		}
	}
	return false
}

type decision struct {
	confidence   Confidence
	reason       Reason
	text         string
	redactions   RedactionSummary
	fingerprints fingerprints
}

// reviewable means text is a sanitized preview candidate. It grants no
// permission to persist, count, export, or submit the sample.
func (decision decision) reviewable() bool {
	switch decision.confidence {
	case ConfidenceTrusted, ConfidenceMedium, ConfidenceLow:
		return true
	default:
		return false
	}
}

// countingEligible is only a structural classification inside the pure kernel;
// it is not a persistence authorization. It does not say the request has been
// counted: only a future authorized database uniqueness decision can distinguish
// a new occurrence, an exact replay, and a conflicting payload.
func (decision decision) countingEligible() bool {
	return decision.confidence == ConfidenceTrusted
}

// evaluate admits only constructor-produced user candidates. Rejections never
// return submitted text, partial text, or text-derived fingerprints.
func (kernel *Kernel) evaluate(candidate candidate) decision {
	confidence, reason, scopeIdentity, transportIdentity, occurrenceIdentity := adjudicate(candidate)
	if confidence == ConfidenceRejected {
		return rejected(reason)
	}
	if len(candidate.text) > kernel.maxTextBytes {
		return rejected(ReasonTextTooLarge)
	}
	if !utf8.ValidString(candidate.text) {
		return rejected(ReasonInvalidUTF8)
	}
	if containsUnsupportedControl(candidate.text) {
		return rejected(ReasonUnsupportedControl)
	}
	if containsUnicodeFormat(candidate.text) {
		return rejected(ReasonUnsafeUnicodeFormat)
	}

	normalized := normalizeText(candidate.text)
	if len(normalized) > kernel.maxTextBytes {
		return rejected(ReasonTextTooLarge)
	}
	if containsUnsupportedControl(normalized) {
		return rejected(ReasonUnsupportedControl)
	}
	if containsUnicodeFormat(normalized) {
		return rejected(ReasonUnsafeUnicodeFormat)
	}
	if normalized == "" {
		return rejected(ReasonEmptyText)
	}

	redacted, summary, restoreStatus := redactText(normalized, kernel.maxTextBytes)
	if restoreStatus == redactionRestoreInvalidMarker {
		return rejected(ReasonRedactionFailed)
	}
	if restoreStatus == redactionRestoreTooLarge || len(redacted) > kernel.maxTextBytes {
		return rejected(ReasonRedactedTextTooLarge)
	}
	transportMaterial := frame(FingerprintPolicyVersion, TextPolicyVersion, "transport", scopeIdentity, transportIdentity, normalized)
	semanticMaterial := frame(FingerprintPolicyVersion, TextPolicyVersion, RedactionPolicyVersion, "semantic", scopeIdentity, canonicalSemanticText(redacted))
	fingerprints := fingerprints{
		policyVersion:      FingerprintPolicyVersion,
		occurrenceVersion:  OccurrencePolicyVersion,
		observationVersion: observationPolicyVersion,
		transport:          make([]versionedDigest, 0, len(kernel.keys)),
		semantic:           make([]versionedDigest, 0, len(kernel.keys)),
	}
	if confidence == ConfidenceTrusted {
		// A logical occurrence is stable across physical-node routing and normal
		// server restarts. Only the isolated scope and trusted server-issued
		// conversation/turn receipts participate in its identity.
		occurrenceMaterial := frame(FingerprintPolicyVersion, OccurrencePolicyVersion, scopeIdentity, occurrenceIdentity)
		identity := candidate.liveIdentity
		observationMaterial := frame(
			FingerprintPolicyVersion,
			observationPolicyVersion,
			TextPolicyVersion,
			"observation",
			scopeIdentity,
			identity.sourceNodeID,
			identity.sourceEpoch,
			identity.conversationReceipt,
			identity.turnReceipt,
			identity.requestObservationID,
			normalized,
		)
		fingerprints.occurrence = make([]versionedDigest, 0, len(kernel.keys))
		fingerprints.observation = make([]versionedDigest, 0, len(kernel.keys))
		for _, key := range kernel.keys {
			fingerprints.occurrence = append(fingerprints.occurrence, versionedDigest{
				keyVersion: key.version,
				digest:     fingerprintHMAC(key.transportKey, occurrenceMaterial),
			})
			fingerprints.observation = append(fingerprints.observation, versionedDigest{
				keyVersion: key.version,
				digest:     fingerprintHMAC(key.transportKey, observationMaterial),
			})
		}
	}
	for _, key := range kernel.keys {
		fingerprints.transport = append(fingerprints.transport, versionedDigest{
			keyVersion: key.version,
			digest:     fingerprintHMAC(key.transportKey, transportMaterial),
		})
		fingerprints.semantic = append(fingerprints.semantic, versionedDigest{
			keyVersion: key.version,
			digest:     fingerprintHMAC(key.semanticKey, semanticMaterial),
		})
	}

	return decision{
		confidence:   confidence,
		reason:       reason,
		text:         redacted,
		redactions:   summary,
		fingerprints: fingerprints,
	}
}

func adjudicate(candidate candidate) (Confidence, Reason, string, string, string) {
	switch candidate.source {
	case sourceLiveUserTurn:
		identity := candidate.liveIdentity
		if !validScopeID(candidate.scope) || candidate.scope != identity.scope ||
			!validProvenanceID(identity.sourceNodeID) ||
			!validProvenanceID(identity.sourceEpoch) ||
			!validProvenanceID(identity.conversationReceipt) ||
			!validProvenanceID(identity.turnReceipt) ||
			!validProvenanceID(identity.requestObservationID) {
			return ConfidenceRejected, ReasonInvalidProvenance, "", "", ""
		}
		return ConfidenceTrusted, ReasonAcceptedLiveUserTurn, scopeMaterial(candidate.scope), frame(
			"live",
			identity.conversationReceipt,
			identity.turnReceipt,
		), frame(identity.conversationReceipt, identity.turnReceipt)
	case sourceTrustedImport:
		identity := candidate.importID
		if !validScopeID(candidate.scope) || candidate.scope != identity.scope ||
			!validProvenanceID(identity.sourceNodeID) ||
			!validProvenanceID(identity.sourceEpoch) ||
			!validProvenanceID(identity.eventReceipt) {
			return ConfidenceRejected, ReasonInvalidProvenance, "", "", ""
		}
		return ConfidenceMedium, ReasonAcceptedTrustedImport, scopeMaterial(candidate.scope), frame(
			"import",
			identity.sourceNodeID,
			identity.sourceEpoch,
			identity.eventReceipt,
		), ""
	case sourceLegacyCandidate:
		if !validScopeID(candidate.scope) {
			return ConfidenceRejected, ReasonInvalidProvenance, "", "", ""
		}
		return ConfidenceLow, ReasonAcceptedLegacyCandidate, scopeMaterial(candidate.scope), "legacy", ""
	case sourceUntrustedClient:
		if strings.TrimSpace(candidate.claimedOrigin) != "" {
			return ConfidenceRejected, ReasonUntrustedOriginClaim, "", "", ""
		}
		if !validScopeID(candidate.scope) {
			return ConfidenceRejected, ReasonInvalidProvenance, "", "", ""
		}
		return ConfidenceLow, ReasonAcceptedUntrustedClaim, scopeMaterial(candidate.scope), "untrusted", ""
	case sourceExcluded:
		switch candidate.excluded {
		case ContentHistory:
			return ConfidenceRejected, ReasonHistoricalContent, "", "", ""
		case ContentSystem:
			return ConfidenceRejected, ReasonSystemContent, "", "", ""
		case ContentDeveloper:
			return ConfidenceRejected, ReasonDeveloperContent, "", "", ""
		case ContentAssistant:
			return ConfidenceRejected, ReasonAssistantContent, "", "", ""
		case ContentTool:
			return ConfidenceRejected, ReasonToolContent, "", "", ""
		case ContentAttachment:
			return ConfidenceRejected, ReasonAttachmentContent, "", "", ""
		case ContentResponse:
			return ConfidenceRejected, ReasonResponseContent, "", "", ""
		case ContentInternalTask, ContentAutoAnalysis:
			return ConfidenceRejected, ReasonInternalContent, "", "", ""
		default:
			return ConfidenceRejected, ReasonInvalidCandidate, "", "", ""
		}
	default:
		return ConfidenceRejected, ReasonInvalidCandidate, "", "", ""
	}
}

func validScopeID(scope scopeID) bool {
	return validProvenanceID(scope.policyReceipt) &&
		validProvenanceID(scope.ownerReceipt) &&
		validProvenanceID(scope.subjectScopeReceipt)
}

func scopeMaterial(scope scopeID) string {
	return frame(scope.policyReceipt, scope.ownerReceipt, scope.subjectScopeReceipt)
}

func validProvenanceID(value string) bool {
	if len(value) == 0 || len(value) > maxProvenanceBytes || !utf8.ValidString(value) || strings.TrimSpace(value) != value {
		return false
	}
	for _, char := range value {
		if unicode.IsSpace(char) || unicode.IsControl(char) || unicode.In(char, unicode.Cf) {
			return false
		}
	}
	return true
}

func rejected(reason Reason) decision {
	return decision{confidence: ConfidenceRejected, reason: reason}
}

func normalizeText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return strings.TrimSpace(norm.NFKC.String(text))
}

func containsUnsupportedControl(text string) bool {
	for _, char := range text {
		if unicode.IsControl(char) && char != '\n' && char != '\t' {
			return true
		}
	}
	return false
}

func containsUnicodeFormat(text string) bool {
	for _, char := range text {
		if unicode.In(char, unicode.Cf) {
			return true
		}
	}
	return false
}

func canonicalSemanticText(text string) string {
	return strings.Join(strings.Fields(cases.Fold().String(text)), " ")
}

func frame(parts ...string) string {
	var builder strings.Builder
	for _, part := range parts {
		builder.WriteString(strconv.Itoa(len(part)))
		builder.WriteByte(':')
		builder.WriteString(part)
	}
	return builder.String()
}

func fingerprintHMAC(key []byte, material string) string {
	digest := hmac.New(sha256.New, key)
	digest.Write([]byte(material))
	return hex.EncodeToString(digest.Sum(nil))
}
