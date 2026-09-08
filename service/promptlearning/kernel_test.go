package promptlearning

import (
	"bytes"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKernelAdjudicatesOnlyEligibleUserContent(t *testing.T) {
	kernel := newTestKernel(t, "v1", nil)
	scope := testScope("a")
	assert.False(t, (decision{}).reviewable())
	tests := []struct {
		name       string
		candidate  candidate
		confidence Confidence
		reason     Reason
		accepted   bool
	}{
		{
			name:       "server observed new user turn",
			candidate:  newLiveUserTurn("build an API", testLiveIdentity(scope, "turn-1", "request-observation-1")),
			confidence: ConfidenceTrusted,
			reason:     ReasonAcceptedLiveUserTurn,
			accepted:   true,
		},
		{
			name:       "trusted explicit import",
			candidate:  newTrustedImportedUserTurn("build an API", testImportIdentity(scope, "event-1")),
			confidence: ConfidenceMedium,
			reason:     ReasonAcceptedTrustedImport,
			accepted:   true,
		},
		{
			name:       "legacy candidate has low confidence",
			candidate:  newLegacyUserCandidate("build an API", scope),
			confidence: ConfidenceLow,
			reason:     ReasonAcceptedLegacyCandidate,
			accepted:   true,
		},
		{
			name:       "untrusted text cannot become trusted",
			candidate:  newUntrustedClientCandidate("build an API", scope, ""),
			confidence: ConfidenceLow,
			reason:     ReasonAcceptedUntrustedClaim,
			accepted:   true,
		},
		{
			name:       "client claimed internal origin",
			candidate:  newUntrustedClientCandidate("build an API", scope, "internal_prompt_learning"),
			confidence: ConfidenceRejected,
			reason:     ReasonUntrustedOriginClaim,
		},
		{
			name:       "history",
			candidate:  newExcludedCandidate("old user message", ContentHistory),
			confidence: ConfidenceRejected,
			reason:     ReasonHistoricalContent,
		},
		{
			name:       "system instruction",
			candidate:  newExcludedCandidate("system instruction", ContentSystem),
			confidence: ConfidenceRejected,
			reason:     ReasonSystemContent,
		},
		{
			name:       "developer instruction",
			candidate:  newExcludedCandidate("developer instruction", ContentDeveloper),
			confidence: ConfidenceRejected,
			reason:     ReasonDeveloperContent,
		},
		{
			name:       "assistant output",
			candidate:  newExcludedCandidate("assistant answer", ContentAssistant),
			confidence: ConfidenceRejected,
			reason:     ReasonAssistantContent,
		},
		{
			name:       "tool result",
			candidate:  newExcludedCandidate("tool result", ContentTool),
			confidence: ConfidenceRejected,
			reason:     ReasonToolContent,
		},
		{
			name:       "attachment",
			candidate:  newExcludedCandidate("attachment text", ContentAttachment),
			confidence: ConfidenceRejected,
			reason:     ReasonAttachmentContent,
		},
		{
			name:       "response body",
			candidate:  newExcludedCandidate("response body", ContentResponse),
			confidence: ConfidenceRejected,
			reason:     ReasonResponseContent,
		},
		{
			name:       "internal task",
			candidate:  newExcludedCandidate("analyze samples", ContentInternalTask),
			confidence: ConfidenceRejected,
			reason:     ReasonInternalContent,
		},
		{
			name:       "automatic analysis",
			candidate:  newExcludedCandidate("model analysis", ContentAutoAnalysis),
			confidence: ConfidenceRejected,
			reason:     ReasonInternalContent,
		},
		{
			name:       "zero value cannot bypass constructors",
			candidate:  candidate{},
			confidence: ConfidenceRejected,
			reason:     ReasonInvalidCandidate,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision := kernel.evaluate(test.candidate)
			assert.Equal(t, test.confidence, decision.confidence)
			assert.Equal(t, test.reason, decision.reason)
			assert.Equal(t, test.accepted, decision.reviewable())
			assert.Equal(t, test.confidence == ConfidenceTrusted, decision.countingEligible())
			if test.accepted {
				assert.NotEmpty(t, decision.text)
				assert.NotEmpty(t, decision.fingerprints.transport)
				assert.NotEmpty(t, decision.fingerprints.semantic)
				assert.Equal(t, test.confidence == ConfidenceTrusted, len(decision.fingerprints.occurrence) > 0)
				assert.Equal(t, test.confidence == ConfidenceTrusted, len(decision.fingerprints.observation) > 0)
			} else {
				assert.Empty(t, decision.text)
				assert.Empty(t, decision.redactions)
				assert.Empty(t, decision.fingerprints.occurrence)
				assert.Empty(t, decision.fingerprints.observation)
				assert.Empty(t, decision.fingerprints.transport)
				assert.Empty(t, decision.fingerprints.semantic)
			}
		})
	}
}

func TestKernelRejectsInvalidProvenance(t *testing.T) {
	kernel := newTestKernel(t, "v1", nil)
	scope := testScope("a")
	tests := []candidate{
		newLiveUserTurn("text", newLiveTurnIdentity(scope, "", "epoch", "conversation", "turn", "observation")),
		newLiveUserTurn("text", newLiveTurnIdentity(scope, "node", "", "conversation", "turn", "observation")),
		newLiveUserTurn("text", newLiveTurnIdentity(scope, "node", "epoch", "", "turn", "observation")),
		newLiveUserTurn("text", newLiveTurnIdentity(scope, "node", "epoch", "conversation", "", "observation")),
		newLiveUserTurn("text", newLiveTurnIdentity(scope, "node", "epoch", "conversation", "turn", "")),
		newLiveUserTurn("text", newLiveTurnIdentity(scope, "node id", "epoch", "conversation", "turn", "observation")),
		newLiveUserTurn("text", newLiveTurnIdentity(scope, "node\nid", "epoch", "conversation", "turn", "observation")),
		newLiveUserTurn("text", newLiveTurnIdentity(scope, "node\tid", "epoch", "conversation", "turn", "observation")),
		newLiveUserTurn("text", newLiveTurnIdentity(scope, "node\u00a0id", "epoch", "conversation", "turn", "observation")),
		newLiveUserTurn("text", newLiveTurnIdentity(scope, "node\u200bid", "epoch", "conversation", "turn", "observation")),
		newLiveUserTurn("text", newLiveTurnIdentity(scope, strings.Repeat("r", maxProvenanceBytes+1), "epoch", "conversation", "turn", "observation")),
		newTrustedImportedUserTurn("text", newImportIdentity(scope, "node", "epoch", "")),
		newTrustedImportedUserTurn("text", newImportIdentity(scopeID{}, "node", "epoch", "event")),
		newTrustedImportedUserTurn("text", newImportIdentity(newScopeID("policy\nreceipt", "owner", "token"), "node", "epoch", "event")),
		newLegacyUserCandidate("text", scopeID{}),
		newUntrustedClientCandidate("text", scopeID{}, ""),
	}

	for _, candidate := range tests {
		decision := kernel.evaluate(candidate)
		assert.Equal(t, ConfidenceRejected, decision.confidence)
		assert.Equal(t, ReasonInvalidProvenance, decision.reason)
	}
}

func TestKernelNormalizesAndBoundsTextDeterministically(t *testing.T) {
	baseConfig := testConfig("v1", nil)
	tests := []struct {
		name       string
		maxBytes   int
		text       string
		confidence Confidence
		reason     Reason
		wantText   string
	}{
		{
			name:       "NFKC LF and outer whitespace",
			maxBytes:   128,
			text:       "  Ｈｅｌｌｏ\nWorld  ",
			confidence: ConfidenceTrusted,
			reason:     ReasonAcceptedLiveUserTurn,
			wantText:   "Hello\nWorld",
		},
		{
			name:       "raw byte bound",
			maxBytes:   4,
			text:       "12345",
			confidence: ConfidenceRejected,
			reason:     ReasonTextTooLarge,
		},
		{
			name:       "normalization expansion bound",
			maxBytes:   3,
			text:       "㌀",
			confidence: ConfidenceRejected,
			reason:     ReasonTextTooLarge,
		},
		{
			name:       "invalid UTF-8",
			maxBytes:   128,
			text:       string([]byte{0xff, 0xfe}),
			confidence: ConfidenceRejected,
			reason:     ReasonInvalidUTF8,
		},
		{
			name:       "unsupported control",
			maxBytes:   128,
			text:       "visible\x00hidden",
			confidence: ConfidenceRejected,
			reason:     ReasonUnsupportedControl,
		},
		{
			name:       "carriage return is rejected",
			maxBytes:   128,
			text:       "line one\r\nline two",
			confidence: ConfidenceRejected,
			reason:     ReasonUnsupportedControl,
		},
		{
			name:       "next line control is rejected",
			maxBytes:   128,
			text:       "visible\u0085hidden",
			confidence: ConfidenceRejected,
			reason:     ReasonUnsupportedControl,
		},
		{
			name:       "control sequence introducer is rejected",
			maxBytes:   128,
			text:       "visible\u009bhidden",
			confidence: ConfidenceRejected,
			reason:     ReasonUnsupportedControl,
		},
		{
			name:       "zero width format is rejected",
			maxBytes:   128,
			text:       "visible\u200bhidden",
			confidence: ConfidenceRejected,
			reason:     ReasonUnsafeUnicodeFormat,
		},
		{
			name:       "bidirectional override is rejected",
			maxBytes:   128,
			text:       "visible\u202ehidden",
			confidence: ConfidenceRejected,
			reason:     ReasonUnsafeUnicodeFormat,
		},
		{
			name:       "redaction expansion bound",
			maxBytes:   10,
			text:       "a@b.co",
			confidence: ConfidenceRejected,
			reason:     ReasonRedactedTextTooLarge,
		},
		{
			name:       "empty after normalization",
			maxBytes:   128,
			text:       " \n\t ",
			confidence: ConfidenceRejected,
			reason:     ReasonEmptyText,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := baseConfig
			config.MaxTextBytes = test.maxBytes
			kernel, err := NewKernel(config)
			require.NoError(t, err)
			decision := kernel.evaluate(newLiveUserTurn(test.text, testLiveIdentity(testScope("a"), "turn", "observation")))
			assert.Equal(t, test.confidence, decision.confidence)
			assert.Equal(t, test.reason, decision.reason)
			assert.Equal(t, test.wantText, decision.text)
			if !decision.reviewable() {
				assert.NotContains(t, string(decision.reason), test.text)
			}
		})
	}
}

func TestKernelDoesNotTrustPublicRedactionMarkers(t *testing.T) {
	kernel := newTestKernel(t, "v1", nil)
	text := strings.Join([]string{
		"api key [REDACTED:API_KEY]shortsecret",
		"authorization: Bearer shortsecret",
		`password "correct horse battery staple"`,
		`authorization Bearer "short secret"`,
		"password=person@example.com/hunter2",
		"token=sk-abcdefghijklmnopqrstuvwxyz123456",
	}, "\n")

	decision := kernel.evaluate(newLiveUserTurn(text, testLiveIdentity(testScope("a"), "turn", "observation")))
	require.True(t, decision.reviewable())
	assert.NotContains(t, decision.text, "shortsecret")
	assert.NotContains(t, decision.text, "correct horse battery staple")
	assert.NotContains(t, decision.text, "short secret")
	assert.NotContains(t, decision.text, "person@example.com")
	assert.NotContains(t, decision.text, "hunter2")
	assert.NotContains(t, decision.text, "sk-abcdefghijklmnopqrstuvwxyz123456")
	// The first forged public marker begins with a composite delimiter, so the
	// context scanner intentionally fails closed through EOF. Structured scans
	// still record their earlier matches, but only the fail-closed marker remains.
	assert.GreaterOrEqual(t, decision.redactions[RedactionSecret], 1)
	assert.Equal(t, 1, decision.redactions[RedactionAPIKey])
	assert.Contains(t, decision.text, "[REDACTED:SECRET]")
	assert.NotContains(t, decision.text, internalMarkerPrefix)
	assert.NotContains(t, decision.text, "\x00")
}

func TestContextSecretScannerRejectsQuotedAndMarkerSplitBypasses(t *testing.T) {
	kernel := newTestKernel(t, "v1", nil)
	tests := []struct {
		name    string
		input   string
		secrets []string
		want    string
	}{
		{
			name:    "password with whitespace separator and quoted spaces",
			input:   `password "correct horse battery staple"`,
			secrets: []string{"correct horse battery staple"},
			want:    `password "[REDACTED:SECRET]"`,
		},
		{
			name:    "bearer with whitespace separator and quoted credential",
			input:   `authorization Bearer "short secret"`,
			secrets: []string{"short secret"},
			want:    `authorization Bearer "[REDACTED:SECRET]"`,
		},
		{
			name:    "structured email marker cannot split password suffix",
			input:   "password=person@example.com/hunter2",
			secrets: []string{"person@example.com", "hunter2"},
			want:    "password=[REDACTED:SECRET]",
		},
		{
			name:    "public marker cannot split quoted password",
			input:   `password [REDACTED:SECRET] "still secret"`,
			secrets: []string{"still secret"},
			want:    "password [REDACTED:SECRET]",
		},
		{
			name:    "public marker cannot split quoted bearer",
			input:   `authorization Bearer [REDACTED:SECRET] "still bearer secret"`,
			secrets: []string{"still bearer secret"},
			want:    "authorization Bearer [REDACTED:SECRET]",
		},
		{
			name:    "public API key marker cannot terminate unquoted value",
			input:   `api key [REDACTED:API_KEY]shortsecret`,
			secrets: []string{"shortsecret", "]shortsecret"},
			want:    "api key [REDACTED:SECRET]",
		},
		{
			name:    "quoted password may cross line",
			input:   "password=\"first line\nsecond line\"",
			secrets: []string{"first line", "second line"},
			want:    `password="[REDACTED:SECRET]"`,
		},
		{
			name:    "escaped quote does not terminate quoted password",
			input:   `password="prefix \"still secret\" suffix"`,
			secrets: []string{"prefix", "still secret", "suffix"},
			want:    `password="[REDACTED:SECRET]"`,
		},
		{
			name:    "JSON quoted password key does not consume sibling field",
			input:   `{"password":"correct horse","other":"visible"}`,
			secrets: []string{"correct horse"},
			want:    `{"password":"[REDACTED:SECRET]","other":"visible"}`,
		},
		{
			name:    "JSON quoted authorization key does not consume sibling field",
			input:   `{"authorization":"Bearer short secret","other":"visible"}`,
			secrets: []string{"Bearer short secret"},
			want:    `{"authorization":"[REDACTED:SECRET]","other":"visible"}`,
		},
		{
			name:    "YAML quoted api key does not consume sibling field",
			input:   `{'api_key': 'shortsecret', 'other': 'visible'}`,
			secrets: []string{"shortsecret"},
			want:    `{'api_key': '[REDACTED:SECRET]', 'other': 'visible'}`,
		},
		{
			name:  "empty YAML value does not consume next line",
			input: "password:\nother: visible",
			want:  "password:\nother: visible",
		},
		{
			name:    "YAML indentless token sequence fails closed",
			input:   "token:\n- shortsecret\nother: visible",
			secrets: []string{"shortsecret", "visible"},
			want:    "[REDACTED:SECRET]",
		},
		{
			name:    "YAML indentless authorization sequence fails closed",
			input:   "authorization:\n- Bearer short secret\nother: visible",
			secrets: []string{"short secret", "visible"},
			want:    "[REDACTED:SECRET]",
		},
		{
			name:    "single line YAML scalar preserves sibling",
			input:   "password: shortsecret\nother: visible",
			secrets: []string{"shortsecret"},
			want:    "password: [REDACTED:SECRET]\nother: visible",
		},
		{
			name:    "pretty JSON key colon and value across records fail closed",
			input:   "\"password\"\n:\n\"correct horse\"\n\"other\":\"visible\"",
			secrets: []string{"correct horse", "visible"},
			want:    "[REDACTED:SECRET]",
		},
		{
			name:    "YAML literal block fails closed",
			input:   "password: |\n  first secret\nother: visible",
			secrets: []string{"first secret", "visible"},
			want:    "password: [REDACTED:SECRET]",
		},
		{
			name:    "YAML folded block fails closed",
			input:   "password: >-\n  folded secret\nother: visible",
			secrets: []string{"folded secret", "visible"},
			want:    "password: [REDACTED:SECRET]",
		},
		{
			name:    "YAML indented value after colon fails closed",
			input:   "password:\n  indented secret\nother: visible",
			secrets: []string{"indented secret", "visible"},
			want:    "[REDACTED:SECRET]",
		},
		{
			name:    "YAML unquoted continuation fails closed",
			input:   "password: first secret\n  continued secret\nother: visible",
			secrets: []string{"first secret", "continued secret", "visible"},
			want:    "password: [REDACTED:SECRET]",
		},
		{
			name:    "YAML delimiter before indented continuation fails closed",
			input:   "password: first secret,\n  continued secret\nother: visible",
			secrets: []string{"first secret", "continued secret", "visible"},
			want:    "password: [REDACTED:SECRET]",
		},
		{
			name:    "unparseable quoted suffix fails closed",
			input:   "password: \"first secret\" trailing secret\nother: visible",
			secrets: []string{"first secret", "trailing secret", "visible"},
			want:    "password: [REDACTED:SECRET]",
		},
		{
			name:    "nested JSON array fails closed",
			input:   `{"password":["first secret",{"nested":"second secret"}],"other":"visible"}`,
			secrets: []string{"first secret", "second secret", "visible"},
			want:    `{"password":[REDACTED:SECRET]`,
		},
		{
			name:    "nested JSON object fails closed",
			input:   `{"password":{"nested":"object secret"},"other":"visible"}`,
			secrets: []string{"object secret", "visible"},
			want:    `{"password":[REDACTED:SECRET]`,
		},
		{
			name:    "Unicode line separator bounds sibling",
			input:   "password: shortsecret\u2028other: visible",
			secrets: []string{"shortsecret"},
			want:    "password: [REDACTED:SECRET]\u2028other: visible",
		},
		{
			name:    "Unicode paragraph separator bounds sibling",
			input:   "password: shortsecret\u2029other: visible",
			secrets: []string{"shortsecret"},
			want:    "password: [REDACTED:SECRET]\u2029other: visible",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision := kernel.evaluate(newLiveUserTurn(test.input, testLiveIdentity(testScope("a"), "turn", "observation")))
			require.True(t, decision.reviewable())
			want := test.want
			if want == "" {
				want = "[REDACTED:SECRET]"
			}
			assert.Equal(t, want, decision.text)
			for _, secret := range test.secrets {
				assert.NotContains(t, decision.text, secret)
			}
			assert.NotContains(t, decision.text, internalMarkerPrefix)
			assert.NotContains(t, decision.text, "\x00")
		})
	}
}

func TestContextSecretScannerHandlesManyShortRecordsDeterministically(t *testing.T) {
	const recordCount = 512
	var input strings.Builder
	var expected strings.Builder
	for index := 0; index < recordCount; index++ {
		if index > 0 {
			input.WriteByte('\n')
			expected.WriteByte('\n')
		}
		input.WriteString("password: value-")
		input.WriteString(strconv.Itoa(index))
		expected.WriteString("password: [REDACTED:SECRET]")
	}

	kernel := newTestKernel(t, "v1", nil)
	decision := kernel.evaluate(newLiveUserTurn(input.String(), testLiveIdentity(testScope("a"), "turn", "observation")))
	require.True(t, decision.reviewable())
	assert.Equal(t, expected.String(), decision.text)
	assert.Equal(t, recordCount, decision.redactions[RedactionSecret])
	assert.NotContains(t, decision.text, internalMarkerPrefix)
	assert.NotContains(t, decision.text, "\x00")
}

func TestContextSecretScannerUsesUnicodeWordBoundaries(t *testing.T) {
	kernel := newTestKernel(t, "v1", nil)
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "ASCII label suffix inside Chinese prose is not a context label",
			input: "我的token 是普通说明",
			want:  "我的token 是普通说明",
		},
		{
			name:  "ASCII label prefix inside Arabic prose is not a context label",
			input: "tokenملاحظة عادية",
			want:  "tokenملاحظة عادية",
		},
		{
			name:  "Unicode punctuation still bounds a credential label",
			input: "说明：token=shortsecret, visible",
			want:  "说明:token=[REDACTED:SECRET], visible",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision := kernel.evaluate(newLiveUserTurn(test.input, testLiveIdentity(testScope("a"), "turn", "observation")))
			require.True(t, decision.reviewable())
			assert.Equal(t, test.want, decision.text)
			assert.NotContains(t, decision.text, internalMarkerPrefix)
			assert.NotContains(t, decision.text, "\x00")
		})
	}
}

func TestContextSecretScannerPreservesExactInternalMarkerKind(t *testing.T) {
	kernel := newTestKernel(t, "v1", nil)
	tests := []struct {
		name string
		text string
		want string
		kind RedactionKind
	}{
		{
			name: "labelled API key",
			text: "token=sk-abcdefghijklmnopqrstuvwxyz123456",
			want: "token=[REDACTED:API_KEY]",
			kind: RedactionAPIKey,
		},
		{
			name: "bearer API key",
			text: "authorization: Bearer sk-abcdefghijklmnopqrstuvwxyz123456",
			want: "authorization: Bearer [REDACTED:API_KEY]",
			kind: RedactionAPIKey,
		},
		{
			name: "quoted JSON email",
			text: `{"password":"person@example.com","other":"visible"}`,
			want: `{"password":"[REDACTED:EMAIL]","other":"visible"}`,
			kind: RedactionEmail,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision := kernel.evaluate(newLiveUserTurn(test.text, testLiveIdentity(testScope("a"), "turn", "observation")))
			require.True(t, decision.reviewable())
			assert.Equal(t, test.want, decision.text)
			assert.Equal(t, 1, decision.redactions[test.kind])
			assert.Zero(t, decision.redactions[RedactionSecret])
			assert.NotContains(t, decision.text, internalMarkerPrefix)
			assert.NotContains(t, decision.text, "\x00")
		})
	}
}

func TestRedactionRestoreIsBoundedAndRejectsInvalidMarkers(t *testing.T) {
	state := newRedactionState()
	state.marker(RedactionSecret, "[REDACTED:SECRET]")
	text, status := state.restore("prefix\x00R999\x00suffix", 128)
	assert.Equal(t, redactionRestoreInvalidMarker, status)
	assert.Empty(t, text)
	text, status = state.restore("prefix\x00R-1\x00suffix", 128)
	assert.Equal(t, redactionRestoreInvalidMarker, status)
	assert.Empty(t, text)

	config := testConfig("v1", nil)
	config.MaxTextBytes = 1024
	kernel, err := NewKernel(config)
	require.NoError(t, err)
	manyShortMatches := strings.TrimSpace(strings.Repeat("a@b.co ", 120))
	decision := kernel.evaluate(newLiveUserTurn(manyShortMatches, testLiveIdentity(testScope("a"), "turn", "observation")))
	assert.Equal(t, ConfidenceRejected, decision.confidence)
	assert.Equal(t, ReasonRedactedTextTooLarge, decision.reason)
	assert.Empty(t, decision.text)
}

func TestKernelRedactsSensitiveTextInTwoLayers(t *testing.T) {
	kernel := newTestKernel(t, "v1", nil)
	secrets := []string{
		"dbuser:dbpass",
		"eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk",
		"sk-abcdefghijklmnopqrstuvwxyz123456",
		"person@example.com",
		"+1 (415) 555-2671",
		"hunter2",
		"Q7vN2mL9xP4cR8tY1uK6wA3z",
	}
	text := strings.Join([]string{
		"database=postgres://dbuser:dbpass@db.example/app",
		"jwt=" + secrets[1],
		"api key " + secrets[2],
		"email " + secrets[3],
		"phone " + secrets[4],
		"Password=" + secrets[5] + ";",
		"opaque " + secrets[6],
	}, "\n")

	decision := kernel.evaluate(newLiveUserTurn(text, testLiveIdentity(testScope("a"), "turn", "observation")))
	require.True(t, decision.reviewable())
	for _, secret := range secrets {
		assert.NotContains(t, decision.text, secret)
	}
	assert.Contains(t, decision.text, "postgres://[REDACTED:URL_CREDENTIAL]@db.example/app")
	assert.Contains(t, decision.text, "[REDACTED:JWT]")
	assert.Contains(t, decision.text, "[REDACTED:API_KEY]")
	assert.Contains(t, decision.text, "[REDACTED:EMAIL]")
	assert.Contains(t, decision.text, "[REDACTED:PHONE]")
	assert.Contains(t, decision.text, "[REDACTED:SECRET]")
	assert.Contains(t, decision.text, "[REDACTED:HIGH_ENTROPY]")
	for _, kind := range []RedactionKind{
		RedactionURLCredential,
		RedactionJWT,
		RedactionAPIKey,
		RedactionEmail,
		RedactionPhone,
		RedactionSecret,
		RedactionHighEntropy,
	} {
		assert.GreaterOrEqual(t, decision.redactions[kind], 1, "missing redaction kind %s", kind)
	}
}

func TestFingerprintSeparatesRetryAndSemanticDomains(t *testing.T) {
	kernel := newTestKernel(t, "v1", nil)
	scope := testScope("a")
	first := kernel.evaluate(newLiveUserTurn("  BUILD   an API  ", testLiveIdentity(scope, "turn-1", "request-observation-1")))
	retryWithNewRequestObservation := kernel.evaluate(newLiveUserTurn("  BUILD   an API  ", testLiveIdentity(scope, "turn-1", "request-observation-2")))
	retryAfterPhysicalMove := kernel.evaluate(newLiveUserTurn("  BUILD   an API  ", newLiveTurnIdentity(
		scope,
		"node-2",
		"epoch-after-restart",
		"conversation-1",
		"turn-1",
		"request-observation-3",
	)))
	independentRepeat := kernel.evaluate(newLiveUserTurn("build an api", testLiveIdentity(scope, "turn-2", "request-observation-4")))

	require.Len(t, first.fingerprints.transport, 1)
	require.Len(t, first.fingerprints.semantic, 1)
	require.Len(t, first.fingerprints.occurrence, 1)
	require.Len(t, first.fingerprints.observation, 1)
	assert.Len(t, first.fingerprints.transport[0].digest, 64)
	assert.Equal(t, FingerprintPolicyVersion, first.fingerprints.policyVersion)
	assert.Equal(t, OccurrencePolicyVersion, first.fingerprints.occurrenceVersion)
	assert.Equal(t, observationPolicyVersion, first.fingerprints.observationVersion)
	assert.Equal(t, first.fingerprints.occurrence, retryWithNewRequestObservation.fingerprints.occurrence)
	assert.Equal(t, first.fingerprints.occurrence, retryAfterPhysicalMove.fingerprints.occurrence)
	assert.Equal(t, first.fingerprints.transport, retryWithNewRequestObservation.fingerprints.transport)
	assert.Equal(t, first.fingerprints.transport, retryAfterPhysicalMove.fingerprints.transport)
	assert.NotEqual(t, first.fingerprints.observation, retryWithNewRequestObservation.fingerprints.observation)
	assert.NotEqual(t, first.fingerprints.observation, retryAfterPhysicalMove.fingerprints.observation)
	assert.Equal(t, first.fingerprints.semantic, retryWithNewRequestObservation.fingerprints.semantic)
	assert.Equal(t, first.fingerprints.semantic, retryAfterPhysicalMove.fingerprints.semantic)
	assert.NotEqual(t, first.fingerprints.occurrence, independentRepeat.fingerprints.occurrence)
	assert.NotEqual(t, first.fingerprints.observation, independentRepeat.fingerprints.observation)
	assert.NotEqual(t, first.fingerprints.transport, independentRepeat.fingerprints.transport)
	assert.Equal(t, first.fingerprints.semantic, independentRepeat.fingerprints.semantic)
	assert.NotEqual(t, first.fingerprints.transport[0].digest, first.fingerprints.semantic[0].digest)
}

func TestOccurrenceFingerprintUsesOnlyStableLogicalTurnIdentity(t *testing.T) {
	config := testConfig("v1", nil)
	kernel, err := NewKernel(config)
	require.NoError(t, err)
	scope := newScopeID("policy-stable", "owner-stable", "project-scope-receipt")
	identity := newLiveTurnIdentity(
		scope,
		"physical-node",
		"process-epoch",
		"conversation-receipt",
		"turn-receipt",
		"http-observation",
	)

	decision := kernel.evaluate(newLiveUserTurn("payload", identity))
	require.Len(t, decision.fingerprints.occurrence, 1)
	require.Len(t, decision.fingerprints.observation, 1)
	require.Len(t, decision.fingerprints.transport, 1)
	assert.Equal(t, "hmac-sha256-v2", decision.fingerprints.policyVersion)
	assert.Equal(t, "occurrence-v2", decision.fingerprints.occurrenceVersion)
	assert.Equal(t, "observation-v1", decision.fingerprints.observationVersion)
	wantOccurrenceMaterial := frame(
		FingerprintPolicyVersion,
		OccurrencePolicyVersion,
		scopeMaterial(scope),
		frame(identity.conversationReceipt, identity.turnReceipt),
	)
	assert.Equal(t, fingerprintHMAC(config.ActiveKey.TransportKey, wantOccurrenceMaterial), decision.fingerprints.occurrence[0].digest)
	wantObservationMaterial := frame(
		FingerprintPolicyVersion,
		observationPolicyVersion,
		TextPolicyVersion,
		"observation",
		scopeMaterial(scope),
		identity.sourceNodeID,
		identity.sourceEpoch,
		identity.conversationReceipt,
		identity.turnReceipt,
		identity.requestObservationID,
		"payload",
	)
	assert.Equal(t, fingerprintHMAC(config.ActiveKey.TransportKey, wantObservationMaterial), decision.fingerprints.observation[0].digest)
	wantTransportMaterial := frame(
		FingerprintPolicyVersion,
		TextPolicyVersion,
		"transport",
		scopeMaterial(scope),
		frame("live", identity.conversationReceipt, identity.turnReceipt),
		"payload",
	)
	assert.Equal(t, fingerprintHMAC(config.ActiveKey.TransportKey, wantTransportMaterial), decision.fingerprints.transport[0].digest)
}

func TestOccurrenceReceiptExposesReplayAndConflictInputs(t *testing.T) {
	kernel := newTestKernel(t, "v1", nil)
	identity := testLiveIdentity(testScope("a"), "turn-1", "request-observation-1")
	first := kernel.evaluate(newLiveUserTurn("first payload", identity))
	replay := kernel.evaluate(newLiveUserTurn("first payload", identity))
	conflict := kernel.evaluate(newLiveUserTurn("different payload", identity))

	require.True(t, first.countingEligible())
	assert.Equal(t, first.fingerprints.occurrence, replay.fingerprints.occurrence)
	assert.Equal(t, first.fingerprints.transport, replay.fingerprints.transport)
	assert.Equal(t, first.fingerprints.observation, replay.fingerprints.observation)
	assert.Equal(t, first.fingerprints.occurrence, conflict.fingerprints.occurrence)
	assert.NotEqual(t, first.fingerprints.transport, conflict.fingerprints.transport)
	assert.NotEqual(t, first.fingerprints.observation, conflict.fingerprints.observation)
	assert.True(t, conflict.fingerprints.matchesOccurrence(first.fingerprints.occurrence[0]))
	assert.True(t, replay.fingerprints.matchesObservation(first.fingerprints.observation[0]))

	secretOne := kernel.evaluate(newLiveUserTurn("token=sk-aaaaaaaaaaaaaaaaaaaaaaaa", identity))
	secretTwo := kernel.evaluate(newLiveUserTurn("token=sk-bbbbbbbbbbbbbbbbbbbbbbbb", identity))
	require.Equal(t, secretOne.text, secretTwo.text)
	assert.Equal(t, secretOne.fingerprints.occurrence, secretTwo.fingerprints.occurrence)
	assert.NotEqual(t, secretOne.fingerprints.transport, secretTwo.fingerprints.transport)
	assert.NotEqual(t, secretOne.fingerprints.observation, secretTwo.fingerprints.observation)
	assert.Equal(t, secretOne.fingerprints.semantic, secretTwo.fingerprints.semantic)
}

func TestFingerprintIsolatesScopes(t *testing.T) {
	kernel := newTestKernel(t, "v1", nil)
	baselineScope := newScopeID("policy-a", "owner-a", "key-scope-receipt-a")
	baseline := kernel.evaluate(newLiveUserTurn("build an API", testLiveIdentity(baselineScope, "turn", "observation-1")))
	require.True(t, baseline.reviewable())

	tests := []struct {
		name  string
		scope scopeID
	}{
		{name: "policy boundary", scope: newScopeID("policy-b", "owner-a", "key-scope-receipt-a")},
		{name: "owner boundary", scope: newScopeID("policy-a", "owner-b", "key-scope-receipt-a")},
		{name: "key-like subject boundary", scope: newScopeID("policy-a", "owner-a", "key-scope-receipt-b")},
		{name: "user-like subject boundary", scope: newScopeID("policy-a", "owner-a", "user-scope-receipt-a")},
		{name: "project-like subject boundary", scope: newScopeID("policy-a", "owner-a", "project-scope-receipt-a")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			otherScope := kernel.evaluate(newLiveUserTurn("build an API", testLiveIdentity(test.scope, "turn", "observation-2")))
			require.True(t, otherScope.reviewable())
			assert.NotEqual(t, baseline.fingerprints.transport, otherScope.fingerprints.transport)
			assert.NotEqual(t, baseline.fingerprints.semantic, otherScope.fingerprints.semantic)
			assert.NotEqual(t, baseline.fingerprints.occurrence, otherScope.fingerprints.occurrence)
			assert.NotEqual(t, baseline.fingerprints.observation, otherScope.fingerprints.observation)
			assert.False(t, otherScope.fingerprints.matchesTransport(baseline.fingerprints.transport[0]))
			assert.False(t, otherScope.fingerprints.matchesSemantic(baseline.fingerprints.semantic[0]))
			assert.False(t, otherScope.fingerprints.matchesObservation(baseline.fingerprints.observation[0]))
		})
	}
}

func TestFingerprintRetiredKeyAliasesSupportRotation(t *testing.T) {
	oldConfig := testConfig("v1", nil)
	oldKernel, err := NewKernel(oldConfig)
	require.NoError(t, err)
	rotatedKernel := newTestKernel(t, "v2", []HMACKeyVersion{oldConfig.ActiveKey})
	candidate := newLiveUserTurn("build an API", testLiveIdentity(testScope("a"), "turn", "observation"))

	oldDecision := oldKernel.evaluate(candidate)
	rotatedDecision := rotatedKernel.evaluate(candidate)
	require.Len(t, rotatedDecision.fingerprints.occurrence, 2)
	require.Len(t, rotatedDecision.fingerprints.observation, 2)
	require.Len(t, rotatedDecision.fingerprints.transport, 2)
	require.Len(t, rotatedDecision.fingerprints.semantic, 2)
	assert.Equal(t, "v2", rotatedDecision.fingerprints.transport[0].keyVersion)
	assert.Equal(t, oldDecision.fingerprints.occurrence[0], rotatedDecision.fingerprints.occurrence[1])
	assert.Equal(t, oldDecision.fingerprints.observation[0], rotatedDecision.fingerprints.observation[1])
	assert.Equal(t, oldDecision.fingerprints.transport[0], rotatedDecision.fingerprints.transport[1])
	assert.Equal(t, oldDecision.fingerprints.semantic[0], rotatedDecision.fingerprints.semantic[1])
	assert.True(t, rotatedDecision.fingerprints.matchesTransport(oldDecision.fingerprints.transport[0]))
	assert.True(t, rotatedDecision.fingerprints.matchesOccurrence(oldDecision.fingerprints.occurrence[0]))
	assert.True(t, rotatedDecision.fingerprints.matchesObservation(oldDecision.fingerprints.observation[0]))
	assert.True(t, rotatedDecision.fingerprints.matchesSemantic(oldDecision.fingerprints.semantic[0]))
	assert.False(t, rotatedDecision.fingerprints.matchesTransport(versionedDigest{keyVersion: "unknown", digest: strings.Repeat("0", 64)}))
}

func TestNewKernelRejectsUnsafeConfigurationWithoutEchoingKeys(t *testing.T) {
	valid := testConfig("v1", nil)
	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{name: "missing version", mutate: func(config *Config) { config.ActiveKey.Version = "" }},
		{name: "unsafe version", mutate: func(config *Config) { config.ActiveKey.Version = "v1:unsafe" }},
		{name: "short transport key", mutate: func(config *Config) { config.ActiveKey.TransportKey = []byte("visible-secret") }},
		{name: "short semantic key", mutate: func(config *Config) { config.ActiveKey.SemanticKey = []byte("visible-secret") }},
		{name: "shared domain key", mutate: func(config *Config) { config.ActiveKey.SemanticKey = config.ActiveKey.TransportKey }},
		{name: "duplicate version", mutate: func(config *Config) { config.RetiredKeys = []HMACKeyVersion{config.ActiveKey} }},
		{name: "invalid max bytes", mutate: func(config *Config) { config.MaxTextBytes = MaxTextBytesLimit + 1 }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := valid
			test.mutate(&config)
			kernel, err := NewKernel(config)
			require.ErrorIs(t, err, ErrInvalidConfig)
			assert.Nil(t, kernel)
			assert.Equal(t, "invalid prompt-learning kernel configuration", err.Error())
			assert.NotContains(t, err.Error(), "visible-secret")
		})
	}
}

func TestFingerprintHMACMatchesKnownAnswer(t *testing.T) {
	key := bytes.Repeat([]byte{0x0b}, 20)
	assert.Equal(
		t,
		"b0344c61d8db38535ca8afceaf0bf12b881dc200c9833da726e9376c2e32cff7",
		fingerprintHMAC(key, "Hi There"),
	)
}

func TestFingerprintKeysAreIndependentAndCopied(t *testing.T) {
	candidate := newLiveUserTurn("payload", testLiveIdentity(testScope("a"), "turn", "observation"))
	firstConfig := testConfig("v1", nil)
	firstKernel, err := NewKernel(firstConfig)
	require.NoError(t, err)
	beforeMutation := firstKernel.evaluate(candidate)

	firstConfig.ActiveKey.TransportKey[0] ^= 0xff
	firstConfig.ActiveKey.SemanticKey[0] ^= 0xff
	afterMutation := firstKernel.evaluate(candidate)
	assert.Equal(t, beforeMutation.fingerprints, afterMutation.fingerprints)

	secondConfig := testConfig("v1", nil)
	secondConfig.ActiveKey.TransportKey = []byte(strings.Repeat("x", minimumHMACKeyBytes))
	secondConfig.ActiveKey.SemanticKey = []byte(strings.Repeat("y", minimumHMACKeyBytes))
	secondKernel, err := NewKernel(secondConfig)
	require.NoError(t, err)
	withDifferentKeys := secondKernel.evaluate(candidate)
	assert.NotEqual(t, beforeMutation.fingerprints.occurrence, withDifferentKeys.fingerprints.occurrence)
	assert.NotEqual(t, beforeMutation.fingerprints.observation, withDifferentKeys.fingerprints.observation)
	assert.NotEqual(t, beforeMutation.fingerprints.transport, withDifferentKeys.fingerprints.transport)
	assert.NotEqual(t, beforeMutation.fingerprints.semantic, withDifferentKeys.fingerprints.semantic)
}

func TestHMACKeyLengthAndRetiredKeyBoundaries(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{name: "31 byte active transport rejected", config: configWithKeyLength(31, 32), wantErr: true},
		{name: "31 byte active semantic rejected", config: configWithKeyLength(32, 31), wantErr: true},
		{name: "32 byte active keys accepted", config: configWithKeyLength(32, 32)},
		{name: "maximum retired keys accepted", config: testConfig("active", retiredKeyVersions(maximumRetiredKeys))},
		{name: "too many retired keys rejected", config: testConfig("active", retiredKeyVersions(maximumRetiredKeys+1)), wantErr: true},
		{
			name: "31 byte retired key rejected",
			config: testConfig("active", []HMACKeyVersion{{
				Version:      "retired",
				TransportKey: []byte(strings.Repeat("t", minimumHMACKeyBytes-1)),
				SemanticKey:  []byte(strings.Repeat("s", minimumHMACKeyBytes)),
			}}),
			wantErr: true,
		},
		{
			name: "31 byte retired semantic key rejected",
			config: testConfig("active", []HMACKeyVersion{{
				Version:      "retired",
				TransportKey: []byte(strings.Repeat("t", minimumHMACKeyBytes)),
				SemanticKey:  []byte(strings.Repeat("s", minimumHMACKeyBytes-1)),
			}}),
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			kernel, err := NewKernel(test.config)
			if test.wantErr {
				require.ErrorIs(t, err, ErrInvalidConfig)
				assert.Nil(t, kernel)
				return
			}
			require.NoError(t, err)
			assert.NotNil(t, kernel)
		})
	}
}

func newTestKernel(t *testing.T, version string, retired []HMACKeyVersion) *Kernel {
	t.Helper()
	kernel, err := NewKernel(testConfig(version, retired))
	require.NoError(t, err)
	return kernel
}

func testConfig(version string, retired []HMACKeyVersion) Config {
	transportSeed := byte('t')
	semanticSeed := byte('s')
	if version == "v2" {
		transportSeed = 'u'
		semanticSeed = 'r'
	}
	return Config{
		ActiveKey: HMACKeyVersion{
			Version:      version,
			TransportKey: []byte(strings.Repeat(string(transportSeed), minimumHMACKeyBytes)),
			SemanticKey:  []byte(strings.Repeat(string(semanticSeed), minimumHMACKeyBytes)),
		},
		RetiredKeys:  retired,
		MaxTextBytes: DefaultMaxTextBytes,
	}
}

func configWithKeyLength(transportLength, semanticLength int) Config {
	return Config{
		ActiveKey: HMACKeyVersion{
			Version:      "v1",
			TransportKey: []byte(strings.Repeat("t", transportLength)),
			SemanticKey:  []byte(strings.Repeat("s", semanticLength)),
		},
		MaxTextBytes: DefaultMaxTextBytes,
	}
}

func retiredKeyVersions(count int) []HMACKeyVersion {
	versions := make([]HMACKeyVersion, 0, count)
	for index := 0; index < count; index++ {
		version := "retired-" + strconv.Itoa(index)
		versions = append(versions, HMACKeyVersion{
			Version:      version,
			TransportKey: []byte(strings.Repeat("transport-"+version, minimumHMACKeyBytes)),
			SemanticKey:  []byte(strings.Repeat("semantic-"+version, minimumHMACKeyBytes)),
		})
	}
	return versions
}

func testScope(suffix string) scopeID {
	return newScopeID("policy-"+suffix, "owner-"+suffix, "subject-scope-"+suffix)
}

func testLiveIdentity(scope scopeID, turnReceipt, requestObservationID string) liveTurnIdentity {
	return newLiveTurnIdentity(scope, "node-1", "epoch-1", "conversation-1", turnReceipt, requestObservationID)
}

func testImportIdentity(scope scopeID, eventReceipt string) importIdentity {
	return newImportIdentity(scope, "node-1", "epoch-1", eventReceipt)
}
