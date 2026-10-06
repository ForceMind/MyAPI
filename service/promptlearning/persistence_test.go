package promptlearning

import (
	"context"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupPromptLearningPersistenceDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	previousDB := model.DB
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.AutoMigrate(&model.PromptLearningPolicy{}, &model.PromptLearningSample{}))
}

func persistenceKernel(t *testing.T) *Kernel {
	t.Helper()
	kernel, err := NewKernel(Config{ActiveKey: HMACKeyVersion{
		Version: "persist-v1", TransportKey: []byte(strings.Repeat("a", 32)), SemanticKey: []byte(strings.Repeat("b", 32)),
	}})
	require.NoError(t, err)
	return kernel
}

func persistenceObservedTurn() serverObservedLiveTurn {
	return serverObservedLiveTurn{
		scope:        newScopeID("policy-receipt", "owner-receipt", "scope-receipt"),
		sourceNodeID: "node-a", sourceEpoch: "epoch-1", conversationReceipt: "conversation-1", turnReceipt: "turn-1", requestObservationID: "request-1",
		segments: []observedTurnSegment{{kind: observedTurnSegmentNewUserText, serverMarked: true, text: "Plan a release rollout. token=sk_secret_should_not_persist"}},
	}
}

func TestPersistObservedLiveTurnRequiresEnabledGenerationAndStoresOnlyRedactedData(t *testing.T) {
	setupPromptLearningPersistenceDB(t)
	policy, err := model.UpsertPromptLearningPolicy(context.Background(), model.PromptLearningPolicyInput{UserID: 17, ScopeRef: "scope:alpha", Enabled: false})
	require.NoError(t, err)
	input := persistedObservedLiveTurn{userID: 17, scopeRef: "scope:alpha", policyGeneration: policy.Generation, observed: persistenceObservedTurn()}

	sample, created, decision, err := persistObservedLiveTurn(context.Background(), persistenceKernel(t), input)
	require.ErrorIs(t, err, model.ErrPromptLearningPolicyDisabled)
	assert.Nil(t, sample)
	assert.False(t, created)
	assert.Equal(t, ConfidenceTrusted, decision.confidence)

	policy, err = model.UpsertPromptLearningPolicy(context.Background(), model.PromptLearningPolicyInput{UserID: 17, ScopeRef: "scope:alpha", Enabled: true})
	require.NoError(t, err)
	input.policyGeneration = policy.Generation
	sample, created, decision, err = persistObservedLiveTurn(context.Background(), persistenceKernel(t), input)
	require.NoError(t, err)
	require.True(t, created)
	assert.Equal(t, ConfidenceTrusted, decision.confidence)
	assert.NotContains(t, sample.RedactedText, "sk_secret_should_not_persist")
	assert.NotContains(t, sample.SourceReceipt, "conversation-1")

	replayed, created, _, err := persistObservedLiveTurn(context.Background(), persistenceKernel(t), input)
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, sample.ID, replayed.ID)
}

func TestPersistObservedLiveTurnRejectsUntrustedCandidateWithoutWriting(t *testing.T) {
	setupPromptLearningPersistenceDB(t)
	policy, err := model.UpsertPromptLearningPolicy(context.Background(), model.PromptLearningPolicyInput{UserID: 17, ScopeRef: "scope:alpha", Enabled: true})
	require.NoError(t, err)
	observed := persistenceObservedTurn()
	observed.segments[0].clientClaimedOrigin = "server"

	sample, created, decision, err := persistObservedLiveTurn(context.Background(), persistenceKernel(t), persistedObservedLiveTurn{
		userID: 17, scopeRef: "scope:alpha", policyGeneration: policy.Generation, observed: observed,
	})

	require.NoError(t, err)
	assert.Nil(t, sample)
	assert.False(t, created)
	assert.Equal(t, ConfidenceRejected, decision.confidence)
	items, total, err := model.ListPromptLearningSamples(context.Background(), 17, "scope:alpha", 0, 20)
	require.NoError(t, err)
	assert.Zero(t, total)
	assert.Empty(t, items)
}
