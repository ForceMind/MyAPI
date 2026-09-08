package promptlearning

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdmitObservedLiveTurnAcceptsOnlyOneCompleteServerMarkedUserText(t *testing.T) {
	kernel := newTestKernel(t, "v1", nil)
	observed := testObservedLiveTurn(testScope("a"), "turn-1", "request-observation-1", "build an API")

	decision := admitObservedLiveTurn(kernel, observed)

	require.Equal(t, ConfidenceTrusted, decision.confidence)
	assert.Equal(t, ReasonAcceptedLiveUserTurn, decision.reason)
	assert.Equal(t, "build an API", decision.text)
	assert.True(t, decision.reviewable())
	assert.True(t, decision.countingEligible())
	require.Len(t, decision.fingerprints.occurrence, 1)
	require.Len(t, decision.fingerprints.observation, 1)
	require.Len(t, decision.fingerprints.transport, 1)
	require.Len(t, decision.fingerprints.semantic, 1)
}

func TestAdmitObservedLiveTurnExtractsOneNewTextFromExplicitlyExcludedSegments(t *testing.T) {
	kernel := newTestKernel(t, "v1", nil)
	base := testObservedLiveTurn(testScope("a"), "turn-1", "observation-1", "build an API")
	withExcluded := cloneObservedLiveTurn(base)
	withExcluded.segments = []observedTurnSegment{
		{kind: observedTurnSegmentHistory, serverMarked: true, text: "old user request"},
		{kind: observedTurnSegmentAssistant, serverMarked: true, text: "old assistant response"},
		{kind: observedTurnSegmentTool, serverMarked: true, text: "tool output"},
		{kind: observedTurnSegmentSystem, serverMarked: true, text: "system instruction"},
		{kind: observedTurnSegmentDeveloper, serverMarked: true, text: "developer instruction"},
		base.segments[0],
		{kind: observedTurnSegmentAttachment, serverMarked: true, text: "attachment text"},
		{kind: observedTurnSegmentResponse, serverMarked: true, text: "response body"},
		{kind: observedTurnSegmentInternal, serverMarked: true, text: "internal task"},
		{kind: observedTurnSegmentAutoAnalysis, serverMarked: true, text: "automatic analysis"},
	}

	decision := admitObservedLiveTurn(kernel, withExcluded)
	baseline := admitObservedLiveTurn(kernel, base)

	require.Equal(t, ConfidenceTrusted, decision.confidence)
	assert.Equal(t, "build an API", decision.text)
	assert.Equal(t, baseline.fingerprints, decision.fingerprints)
	for _, excludedText := range []string{
		"old user request",
		"old assistant response",
		"tool output",
		"system instruction",
		"developer instruction",
		"attachment text",
		"response body",
		"internal task",
		"automatic analysis",
	} {
		assert.NotContains(t, decision.text, excludedText)
	}
}

func TestAdmitObservedLiveTurnRejectsIncompleteFullContentStyleProjection(t *testing.T) {
	kernel := newTestKernel(t, "v1", nil)
	observed := serverObservedLiveTurn{
		segments: []observedTurnSegment{{
			kind:         observedTurnSegmentNewUserText,
			serverMarked: true,
			text:         "client body that resembles a stored full-content entry",
		}},
	}

	assertRejectedObservedTurn(t, admitObservedLiveTurn(kernel, observed), ReasonInvalidProvenance, observed.segments[0].text)
}

func TestAdmitObservedLiveTurnRejectsClientClaimsAndInvalidOrMixedNewText(t *testing.T) {
	kernel := newTestKernel(t, "v1", nil)
	base := testObservedLiveTurn(testScope("a"), "turn-1", "request-observation-1", "sensitive new text")

	tests := []struct {
		name   string
		mutate func(*serverObservedLiveTurn)
		reason Reason
	}{
		{
			name: "client role claim",
			mutate: func(observed *serverObservedLiveTurn) {
				observed.segments[0].clientClaimedRole = "user"
			},
			reason: ReasonUntrustedOriginClaim,
		},
		{
			name: "client origin claim",
			mutate: func(observed *serverObservedLiveTurn) {
				observed.segments[0].clientClaimedOrigin = "server_observed"
			},
			reason: ReasonUntrustedOriginClaim,
		},
		{
			name: "not server marked",
			mutate: func(observed *serverObservedLiveTurn) {
				observed.segments[0].serverMarked = false
			},
			reason: ReasonUntrustedOriginClaim,
		},
		{
			name: "no segment",
			mutate: func(observed *serverObservedLiveTurn) {
				observed.segments = nil
			},
			reason: ReasonInvalidCandidate,
		},
		{
			name: "two new user texts",
			mutate: func(observed *serverObservedLiveTurn) {
				observed.segments = append(observed.segments, observedTurnSegment{
					kind:         observedTurnSegmentNewUserText,
					serverMarked: true,
					text:         "second independent text",
				})
			},
			reason: ReasonInvalidCandidate,
		},
		{
			name: "mixed segment is not server marked",
			mutate: func(observed *serverObservedLiveTurn) {
				observed.segments = append(observed.segments, observedTurnSegment{
					kind: observedTurnSegmentAssistant,
					text: "assistant output",
				})
			},
			reason: ReasonUntrustedOriginClaim,
		},
		{
			name: "mixed segment carries client role claim",
			mutate: func(observed *serverObservedLiveTurn) {
				observed.segments = append(observed.segments, observedTurnSegment{
					kind:              observedTurnSegmentHistory,
					serverMarked:      true,
					text:              "old input",
					clientClaimedRole: "user",
				})
			},
			reason: ReasonUntrustedOriginClaim,
		},
		{
			name: "zero new user texts",
			mutate: func(observed *serverObservedLiveTurn) {
				observed.segments = []observedTurnSegment{
					{kind: observedTurnSegmentHistory, serverMarked: true, text: "old input"},
					{kind: observedTurnSegmentAssistant, serverMarked: true, text: "old output"},
				}
			},
			reason: ReasonInvalidCandidate,
		},
		{
			name: "too many segments",
			mutate: func(observed *serverObservedLiveTurn) {
				observed.segments = make([]observedTurnSegment, maxObservedTurnSegments+1)
			},
			reason: ReasonInvalidCandidate,
		},
		{
			name: "invalid kind",
			mutate: func(observed *serverObservedLiveTurn) {
				observed.segments[0].kind = observedTurnSegmentInvalid
			},
			reason: ReasonInvalidCandidate,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			observed := cloneObservedLiveTurn(base)
			test.mutate(&observed)
			assertRejectedObservedTurn(t, admitObservedLiveTurn(kernel, observed), test.reason, "sensitive new text")
		})
	}
}

func TestAdmitObservedLiveTurnRejectsExcludedContent(t *testing.T) {
	kernel := newTestKernel(t, "v1", nil)
	tests := []struct {
		name   string
		kind   observedTurnSegmentKind
		reason Reason
	}{
		{name: "history", kind: observedTurnSegmentHistory, reason: ReasonHistoricalContent},
		{name: "system", kind: observedTurnSegmentSystem, reason: ReasonSystemContent},
		{name: "developer", kind: observedTurnSegmentDeveloper, reason: ReasonDeveloperContent},
		{name: "assistant", kind: observedTurnSegmentAssistant, reason: ReasonAssistantContent},
		{name: "tool", kind: observedTurnSegmentTool, reason: ReasonToolContent},
		{name: "attachment", kind: observedTurnSegmentAttachment, reason: ReasonAttachmentContent},
		{name: "response", kind: observedTurnSegmentResponse, reason: ReasonResponseContent},
		{name: "internal", kind: observedTurnSegmentInternal, reason: ReasonInternalContent},
		{name: "auto analysis", kind: observedTurnSegmentAutoAnalysis, reason: ReasonInternalContent},
	}

	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			observed := testObservedLiveTurn(testScope("a"), "turn-"+strconv.Itoa(index), "observation-1", "excluded secret text")
			observed.segments[0].kind = test.kind
			assertRejectedObservedTurn(t, admitObservedLiveTurn(kernel, observed), test.reason, "excluded secret text")
		})
	}
}

func TestAdmitObservedLiveTurnRequiresEveryOpaqueProvenanceReceipt(t *testing.T) {
	kernel := newTestKernel(t, "v1", nil)
	base := testObservedLiveTurn(testScope("a"), "turn-1", "request-observation-1", "new text")

	tests := []struct {
		name   string
		mutate func(*serverObservedLiveTurn)
	}{
		{name: "invalid scope", mutate: func(observed *serverObservedLiveTurn) { observed.scope = scopeID{} }},
		{name: "node", mutate: func(observed *serverObservedLiveTurn) { observed.sourceNodeID = "" }},
		{name: "epoch", mutate: func(observed *serverObservedLiveTurn) { observed.sourceEpoch = "epoch with space" }},
		{name: "conversation", mutate: func(observed *serverObservedLiveTurn) { observed.conversationReceipt = "" }},
		{name: "turn", mutate: func(observed *serverObservedLiveTurn) { observed.turnReceipt = "turn\nreceipt" }},
		{name: "request observation", mutate: func(observed *serverObservedLiveTurn) {
			observed.requestObservationID = strings.Repeat("r", maxProvenanceBytes+1)
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			observed := cloneObservedLiveTurn(base)
			test.mutate(&observed)
			assertRejectedObservedTurn(t, admitObservedLiveTurn(kernel, observed), ReasonInvalidProvenance, "new text")
		})
	}
}

func TestAdmitObservedLiveTurnUsesLogicalOccurrenceAcrossTransportMetadata(t *testing.T) {
	kernel := newTestKernel(t, "v1", nil)
	scope := testScope("a")
	first := admitObservedLiveTurn(kernel, testObservedLiveTurn(scope, "turn-1", "observation-1", "first payload"))
	recomputed := admitObservedLiveTurn(kernel, testObservedLiveTurn(scope, "turn-1", "observation-1", "first payload"))
	physicalReplays := []serverObservedLiveTurn{
		testObservedLiveTurn(scope, "turn-1", "observation-2", "first payload"),
		testObservedLiveTurn(scope, "turn-1", "observation-1", "first payload"),
		testObservedLiveTurn(scope, "turn-1", "observation-1", "first payload"),
	}
	physicalReplays[1].sourceNodeID = "node-2"
	physicalReplays[2].sourceEpoch = "epoch-after-restart"
	conflict := admitObservedLiveTurn(kernel, testObservedLiveTurn(scope, "turn-1", "observation-1", "different payload"))

	require.True(t, first.countingEligible())
	require.True(t, recomputed.countingEligible())
	require.True(t, conflict.countingEligible())
	assert.Equal(t, first.fingerprints.observation, recomputed.fingerprints.observation)
	for _, replay := range physicalReplays {
		decision := admitObservedLiveTurn(kernel, replay)
		require.True(t, decision.countingEligible())
		assert.Equal(t, first.fingerprints.occurrence, decision.fingerprints.occurrence)
		assert.Equal(t, first.fingerprints.transport, decision.fingerprints.transport)
		assert.NotEqual(t, first.fingerprints.observation, decision.fingerprints.observation)
	}
	assert.Equal(t, first.fingerprints.occurrence, conflict.fingerprints.occurrence)
	assert.NotEqual(t, first.fingerprints.transport, conflict.fingerprints.transport)
	assert.NotEqual(t, first.fingerprints.observation, conflict.fingerprints.observation)
}

func TestAdmitObservedLiveTurnFingerprintsNormalizedEligiblePayload(t *testing.T) {
	kernel := newTestKernel(t, "v1", nil)
	scope := testScope("a")
	unnormalized := admitObservedLiveTurn(kernel, testObservedLiveTurn(
		scope,
		"turn-1",
		"observation-1",
		"  ＡＰＩ  ",
	))
	canonical := admitObservedLiveTurn(kernel, testObservedLiveTurn(
		scope,
		"turn-1",
		"observation-1",
		"API",
	))

	require.True(t, unnormalized.countingEligible())
	require.True(t, canonical.countingEligible())
	assert.Equal(t, "API", unnormalized.text)
	assert.Equal(t, canonical.text, unnormalized.text)
	assert.Equal(t, canonical.fingerprints.occurrence, unnormalized.fingerprints.occurrence)
	assert.Equal(t, canonical.fingerprints.observation, unnormalized.fingerprints.observation)
	assert.Equal(t, canonical.fingerprints.transport, unnormalized.fingerprints.transport)
	assert.Equal(t, canonical.fingerprints.semantic, unnormalized.fingerprints.semantic)
}

func TestAdmitObservedLiveTurnIsolatesScopes(t *testing.T) {
	kernel := newTestKernel(t, "v1", nil)
	first := admitObservedLiveTurn(kernel, testObservedLiveTurn(testScope("a"), "turn-1", "observation-1", "build an API"))
	second := admitObservedLiveTurn(kernel, testObservedLiveTurn(testScope("b"), "turn-1", "observation-1", "build an API"))

	require.True(t, first.countingEligible())
	require.True(t, second.countingEligible())
	assert.NotEqual(t, first.fingerprints.occurrence, second.fingerprints.occurrence)
	assert.NotEqual(t, first.fingerprints.observation, second.fingerprints.observation)
	assert.NotEqual(t, first.fingerprints.transport, second.fingerprints.transport)
	assert.NotEqual(t, first.fingerprints.semantic, second.fingerprints.semantic)
}

func TestAdmitObservedLiveTurnRejectsEmptyAndUnsafeTextWithoutLeakage(t *testing.T) {
	kernel := newTestKernel(t, "v1", nil)
	tests := []struct {
		name   string
		text   string
		reason Reason
	}{
		{name: "empty", text: " \t ", reason: ReasonEmptyText},
		{name: "invalid UTF-8", text: string([]byte{0xff}), reason: ReasonInvalidUTF8},
		{name: "control", text: "new\x00text", reason: ReasonUnsupportedControl},
	}

	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			observed := testObservedLiveTurn(testScope("a"), "turn-"+strconv.Itoa(index), "observation-1", test.text)
			assertRejectedObservedTurn(t, admitObservedLiveTurn(kernel, observed), test.reason, test.text)
		})
	}
}

func testObservedLiveTurn(scope scopeID, turnReceipt, requestObservationID, text string) serverObservedLiveTurn {
	return serverObservedLiveTurn{
		scope:                scope,
		sourceNodeID:         "node-1",
		sourceEpoch:          "epoch-1",
		conversationReceipt:  "conversation-1",
		turnReceipt:          turnReceipt,
		requestObservationID: requestObservationID,
		segments: []observedTurnSegment{{
			kind:         observedTurnSegmentNewUserText,
			serverMarked: true,
			text:         text,
		}},
	}
}

func cloneObservedLiveTurn(observed serverObservedLiveTurn) serverObservedLiveTurn {
	clone := observed
	clone.segments = append([]observedTurnSegment(nil), observed.segments...)
	return clone
}

func assertRejectedObservedTurn(t *testing.T, decision decision, reason Reason, submittedText string) {
	t.Helper()
	assert.Equal(t, ConfidenceRejected, decision.confidence)
	assert.Equal(t, reason, decision.reason)
	assert.False(t, decision.reviewable())
	assert.False(t, decision.countingEligible())
	assert.Empty(t, decision.text)
	assert.Empty(t, decision.redactions)
	assert.Empty(t, decision.fingerprints.occurrence)
	assert.Empty(t, decision.fingerprints.observation)
	assert.Empty(t, decision.fingerprints.transport)
	assert.Empty(t, decision.fingerprints.semantic)
	assert.NotContains(t, decision.text, submittedText)
}
