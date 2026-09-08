package promptlearning

// observedTurnSegmentKind is assigned only by a future authenticated server
// ingress. It intentionally does not mirror client-supplied message roles.
type observedTurnSegmentKind uint8

const maxObservedTurnSegments = 256

const (
	observedTurnSegmentInvalid observedTurnSegmentKind = iota
	observedTurnSegmentNewUserText
	observedTurnSegmentHistory
	observedTurnSegmentSystem
	observedTurnSegmentDeveloper
	observedTurnSegmentAssistant
	observedTurnSegmentTool
	observedTurnSegmentAttachment
	observedTurnSegmentResponse
	observedTurnSegmentInternal
	observedTurnSegmentAutoAnalysis
)

// observedTurnSegment is a package-private, server-observed projection of one
// turn segment. It is deliberately not a wire or log-record type: callers
// must not construct it from Full Content JSONL or client request JSON.
//
// clientClaimedRole and clientClaimedOrigin make an attempted client-side
// provenance claim explicit so the admission boundary can reject it rather
// than treating it as trusted metadata.
type observedTurnSegment struct {
	kind                observedTurnSegmentKind
	serverMarked        bool
	text                string
	clientClaimedRole   string
	clientClaimedOrigin string
}

// serverObservedLiveTurn is an opaque, server-observed input to the live-turn
// admission boundary. All receipts must come from authenticated server state;
// requestObservationID, sourceNodeID, and sourceEpoch are validated audit
// provenance only and deliberately do not define a logical occurrence.
type serverObservedLiveTurn struct {
	scope                scopeID
	sourceNodeID         string
	sourceEpoch          string
	conversationReceipt  string
	turnReceipt          string
	requestObservationID string
	segments             []observedTurnSegment
}

// admitObservedLiveTurn reduces a bounded complete server-observed turn to its
// one eligible live-user candidate. It accepts surrounding segments only when
// each one is an explicitly excluded, server-marked content kind. Excluded
// content never participates in returned text or fingerprints. Rejections
// contain no submitted text or fingerprints because they use the kernel's
// rejected decision path.
func admitObservedLiveTurn(kernel *Kernel, observed serverObservedLiveTurn) decision {
	if kernel == nil {
		return rejected(ReasonInvalidCandidate)
	}
	if !validScopeID(observed.scope) ||
		!validProvenanceID(observed.sourceNodeID) ||
		!validProvenanceID(observed.sourceEpoch) ||
		!validProvenanceID(observed.conversationReceipt) ||
		!validProvenanceID(observed.turnReceipt) ||
		!validProvenanceID(observed.requestObservationID) {
		return rejected(ReasonInvalidProvenance)
	}
	if len(observed.segments) == 0 || len(observed.segments) > maxObservedTurnSegments {
		return rejected(ReasonInvalidCandidate)
	}

	eligibleSegment := -1
	var singleExcludedContent ExcludedContent
	for index := range observed.segments {
		segment := observed.segments[index]
		if !segment.serverMarked || segment.clientClaimedRole != "" || segment.clientClaimedOrigin != "" {
			return rejected(ReasonUntrustedOriginClaim)
		}
		if segment.kind == observedTurnSegmentNewUserText {
			if eligibleSegment >= 0 {
				return rejected(ReasonInvalidCandidate)
			}
			eligibleSegment = index
			continue
		}

		excludedContent, ok := excludedContentForObservedSegmentKind(segment.kind)
		if !ok {
			return rejected(ReasonInvalidCandidate)
		}
		if len(observed.segments) == 1 {
			singleExcludedContent = excludedContent
		}
	}
	if eligibleSegment < 0 {
		if len(observed.segments) == 1 {
			return kernel.evaluate(newExcludedCandidate(observed.segments[0].text, singleExcludedContent))
		}
		return rejected(ReasonInvalidCandidate)
	}
	segment := observed.segments[eligibleSegment]

	identity := newLiveTurnIdentity(
		observed.scope,
		observed.sourceNodeID,
		observed.sourceEpoch,
		observed.conversationReceipt,
		observed.turnReceipt,
		observed.requestObservationID,
	)
	return kernel.evaluate(newLiveUserTurn(segment.text, identity))
}

func excludedContentForObservedSegmentKind(kind observedTurnSegmentKind) (ExcludedContent, bool) {
	switch kind {
	case observedTurnSegmentHistory:
		return ContentHistory, true
	case observedTurnSegmentSystem:
		return ContentSystem, true
	case observedTurnSegmentDeveloper:
		return ContentDeveloper, true
	case observedTurnSegmentAssistant:
		return ContentAssistant, true
	case observedTurnSegmentTool:
		return ContentTool, true
	case observedTurnSegmentAttachment:
		return ContentAttachment, true
	case observedTurnSegmentResponse:
		return ContentResponse, true
	case observedTurnSegmentInternal:
		return ContentInternalTask, true
	case observedTurnSegmentAutoAnalysis:
		return ContentAutoAnalysis, true
	default:
		return "", false
	}
}
