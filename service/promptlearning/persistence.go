package promptlearning

import (
	"context"
	"errors"

	"github.com/ForceMind/MyAPI/model"
)

// persistedObservedLiveTurn is deliberately package-private. Only a future
// authenticated server ingress in this package may construct it from its own
// verified session, conversation and turn records; controller payloads, Full
// Content logs and client-supplied roles cannot enter this boundary.
type persistedObservedLiveTurn struct {
	userID           int
	scopeRef         string
	policyGeneration int64
	observed         serverObservedLiveTurn
}

// persistObservedLiveTurn admits one server-observed turn through the pure
// kernel and stores only its redacted text plus HMAC-derived identities. A
// rejected/non-counting candidate causes no database write. The model layer
// rechecks the enabled policy generation inside its transaction, so a revoke
// racing the admission cannot leave a newly authorized sample behind.
func persistObservedLiveTurn(ctx context.Context, kernel *Kernel, input persistedObservedLiveTurn) (*model.PromptLearningSample, bool, decision, error) {
	if kernel == nil || input.userID <= 0 || input.scopeRef == "" || input.policyGeneration <= 0 {
		return nil, false, rejected(ReasonInvalidCandidate), model.ErrPromptLearningInvalidInput
	}
	decision := admitObservedLiveTurn(kernel, input.observed)
	if !decision.countingEligible() {
		return nil, false, decision, nil
	}
	if len(decision.fingerprints.occurrence) == 0 || len(decision.fingerprints.semantic) == 0 || len(decision.fingerprints.observation) == 0 {
		return nil, false, rejected(ReasonInvalidCandidate), errors.New("trusted prompt learning decision is missing fingerprints")
	}
	sample, created, err := model.RecordPromptLearningSample(ctx, model.PromptLearningSampleInput{
		UserID:                input.userID,
		ScopeRef:              input.scopeRef,
		OccurrenceFingerprint: decision.fingerprints.occurrence[0].digest,
		SemanticFingerprint:   decision.fingerprints.semantic[0].digest,
		SourceReceipt:         decision.fingerprints.observation[0].digest,
		RedactedText:          decision.text,
		Confidence:            string(decision.confidence),
		PolicyGeneration:      input.policyGeneration,
	})
	return sample, created, decision, err
}
