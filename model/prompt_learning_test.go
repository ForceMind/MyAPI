package model

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupPromptLearningTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	previousDB := DB
	DB = db
	t.Cleanup(func() {
		DB = previousDB
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.AutoMigrate(&PromptLearningPolicy{}, &PromptLearningSample{}, &PromptLearningRun{}, &PromptInstructionVersion{}, &PromptInstructionApplication{}))
	return db
}

func promptLearningFingerprint(character string) string {
	return strings.Repeat(character, 64)
}

func promptLearningSampleInput(policy *PromptLearningPolicy) PromptLearningSampleInput {
	return PromptLearningSampleInput{
		UserID: policy.UserID, ScopeRef: policy.ScopeRef, PolicyGeneration: policy.Generation,
		OccurrenceFingerprint: promptLearningFingerprint("a"), SemanticFingerprint: promptLearningFingerprint("b"),
		SourceReceipt: "server-observation-1", RedactedText: "Draft a rollback plan for the deployment.", Confidence: "trusted",
	}
}

func TestPromptLearningPolicyDefaultsDisabledAndRevocationStopsNewSamples(t *testing.T) {
	setupPromptLearningTestDB(t)
	input := PromptLearningPolicyInput{UserID: 17, ScopeRef: "project:alpha", Enabled: false}
	policy, err := UpsertPromptLearningPolicy(context.Background(), input)
	require.NoError(t, err)
	assert.False(t, policy.Enabled)
	assert.Equal(t, int64(1), policy.Generation)
	assert.Positive(t, policy.RevokedAt)

	_, _, err = RecordPromptLearningSample(context.Background(), promptLearningSampleInput(policy))
	require.ErrorIs(t, err, ErrPromptLearningPolicyDisabled)

	input.Enabled = true
	policy, err = UpsertPromptLearningPolicy(context.Background(), input)
	require.NoError(t, err)
	assert.True(t, policy.Enabled)
	assert.Equal(t, int64(2), policy.Generation)
	assert.Zero(t, policy.RevokedAt)
	createdSample, created, err := RecordPromptLearningSample(context.Background(), promptLearningSampleInput(policy))
	require.NoError(t, err)
	assert.True(t, created)
	assert.Equal(t, policy.ID, createdSample.PolicyID)

	input.Enabled = false
	policy, err = UpsertPromptLearningPolicy(context.Background(), input)
	require.NoError(t, err)
	assert.False(t, policy.Enabled)
	assert.Equal(t, int64(3), policy.Generation)
	_, _, err = RecordPromptLearningSample(context.Background(), promptLearningSampleInput(policy))
	require.ErrorIs(t, err, ErrPromptLearningPolicyDisabled)
}

func TestPromptLearningSamplesAreIdempotentAndScopeIsolated(t *testing.T) {
	setupPromptLearningTestDB(t)
	policy, err := UpsertPromptLearningPolicy(context.Background(), PromptLearningPolicyInput{UserID: 17, ScopeRef: "project:alpha", Enabled: true})
	require.NoError(t, err)
	input := promptLearningSampleInput(policy)
	first, created, err := RecordPromptLearningSample(context.Background(), input)
	require.NoError(t, err)
	assert.True(t, created)
	replayed, created, err := RecordPromptLearningSample(context.Background(), input)
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, first.ID, replayed.ID)

	input.RedactedText = "Different redacted content under one occurrence."
	_, _, err = RecordPromptLearningSample(context.Background(), input)
	require.ErrorIs(t, err, ErrPromptLearningSampleConflict)

	items, total, err := ListPromptLearningSamples(context.Background(), 17, "project:alpha", 0, 20)
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, items, 1)
	assert.Equal(t, first.RedactedText, items[0].RedactedText)

	otherScope, err := UpsertPromptLearningPolicy(context.Background(), PromptLearningPolicyInput{UserID: 17, ScopeRef: "project:beta", Enabled: true})
	require.NoError(t, err)
	otherInput := promptLearningSampleInput(otherScope)
	otherInput.OccurrenceFingerprint = promptLearningFingerprint("c")
	otherInput.SemanticFingerprint = promptLearningFingerprint("d")
	_, _, err = RecordPromptLearningSample(context.Background(), otherInput)
	require.NoError(t, err)

	items, total, err = ListPromptLearningSamples(context.Background(), 18, "project:alpha", 0, 20)
	require.NoError(t, err)
	assert.Zero(t, total)
	assert.Empty(t, items)
	items, total, err = ListPromptLearningSamples(context.Background(), 17, "project:alpha", 0, 20)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Len(t, items, 1)
}

func TestPromptLearningRejectsUnredactedOrStaleInput(t *testing.T) {
	setupPromptLearningTestDB(t)
	policy, err := UpsertPromptLearningPolicy(context.Background(), PromptLearningPolicyInput{UserID: 17, ScopeRef: "project:alpha", Enabled: true})
	require.NoError(t, err)
	input := promptLearningSampleInput(policy)
	input.RedactedText = "has\ncontrol"
	_, _, err = RecordPromptLearningSample(context.Background(), input)
	require.ErrorIs(t, err, ErrPromptLearningInvalidInput)

	input = promptLearningSampleInput(policy)
	input.PolicyGeneration++
	_, _, err = RecordPromptLearningSample(context.Background(), input)
	require.ErrorIs(t, err, ErrPromptLearningPolicyStale)
}

func TestPromptInstructionVersionsAreImmutableAndCommandIdempotent(t *testing.T) {
	setupPromptLearningTestDB(t)
	firstInput := PromptInstructionVersionInput{
		UserID: 17, ScopeRef: "project:alpha", CommandID: "manual-edit-1", Content: "# Instructions\r\n\r\nUse staged rollouts.\r\n", Source: promptInstructionVersionSourceManual, CreatedBy: 17,
	}
	first, created, err := CreatePromptInstructionVersion(context.Background(), firstInput)
	require.NoError(t, err)
	require.True(t, created)
	assert.Equal(t, "# Instructions\n\nUse staged rollouts.\n", first.Content)
	assert.Len(t, first.ContentHash, 64)

	replayed, created, err := CreatePromptInstructionVersion(context.Background(), firstInput)
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, first.ID, replayed.ID)

	conflicting := firstInput
	conflicting.Content = "# Different"
	_, _, err = CreatePromptInstructionVersion(context.Background(), conflicting)
	require.ErrorIs(t, err, ErrPromptInstructionConflict)

	second, created, err := CreatePromptInstructionVersion(context.Background(), PromptInstructionVersionInput{
		UserID: 17, ScopeRef: "project:alpha", CommandID: "manual-edit-2", ParentID: &first.ID, Content: "# Instructions\n\nUse canary rollouts.\n", Source: promptInstructionVersionSourceManual, CreatedBy: 17,
	})
	require.NoError(t, err)
	require.True(t, created)
	require.NotNil(t, second.ParentID)
	assert.Equal(t, first.ID, *second.ParentID)

	err = DB.Model(&PromptInstructionVersion{}).Where("id = ?", first.ID).Update("content", "mutated").Error
	require.ErrorIs(t, err, ErrPromptInstructionImmutable)

	versions, total, err := ListPromptInstructionVersions(context.Background(), 17, "project:alpha", 0, 20)
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	require.Len(t, versions, 2)
	assert.Equal(t, second.ID, versions[0].ID)
	assert.Equal(t, first.ID, versions[1].ID)
}

func TestPromptInstructionVersionRejectsCrossScopeParentAndInvisibleReads(t *testing.T) {
	setupPromptLearningTestDB(t)
	parent, _, err := CreatePromptInstructionVersion(context.Background(), PromptInstructionVersionInput{
		UserID: 17, ScopeRef: "project:alpha", CommandID: "alpha-1", Content: "Alpha", Source: promptInstructionVersionSourceManual, CreatedBy: 17,
	})
	require.NoError(t, err)
	_, _, err = CreatePromptInstructionVersion(context.Background(), PromptInstructionVersionInput{
		UserID: 17, ScopeRef: "project:beta", CommandID: "beta-1", ParentID: &parent.ID, Content: "Beta", Source: promptInstructionVersionSourceManual, CreatedBy: 17,
	})
	require.ErrorIs(t, err, ErrPromptInstructionNotFound)
	_, err = GetPromptInstructionVersion(context.Background(), 18, "project:alpha", parent.ID)
	require.ErrorIs(t, err, ErrPromptInstructionNotFound)
	_, _, err = CreatePromptInstructionVersion(context.Background(), PromptInstructionVersionInput{
		UserID: 17, ScopeRef: "project:alpha", CommandID: "unsafe", Content: "has\x00nul", Source: promptInstructionVersionSourceManual, CreatedBy: 17,
	})
	require.ErrorIs(t, err, ErrPromptLearningInvalidInput)
}

func TestPromptInstructionApplicationRequiresCurrentAuthorizationAndUsesCAS(t *testing.T) {
	setupPromptLearningTestDB(t)
	policy, err := UpsertPromptLearningPolicy(context.Background(), PromptLearningPolicyInput{UserID: 17, ScopeRef: "project:alpha", Enabled: true})
	require.NoError(t, err)
	version, _, err := CreatePromptInstructionVersion(context.Background(), PromptInstructionVersionInput{
		UserID: 17, ScopeRef: "project:alpha", CommandID: "version-1", Content: "Instructions", Source: promptInstructionVersionSourceManual, CreatedBy: 17,
	})
	require.NoError(t, err)
	input := PromptInstructionApplicationInput{
		UserID: 17, ScopeRef: "project:alpha", CommandID: "apply-1", VersionID: version.ID, TargetReceipt: promptLearningFingerprint("c"),
		AuthorizationGeneration: policy.Generation, AuthorizationExpiresAt: time.Now().Add(time.Hour).Unix(), CreatedBy: 17,
	}
	application, created, err := CreatePromptInstructionApplication(context.Background(), input)
	require.NoError(t, err)
	require.True(t, created)
	assert.Equal(t, promptInstructionApplicationStatePending, application.State)
	replayed, created, err := CreatePromptInstructionApplication(context.Background(), input)
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, application.ID, replayed.ID)

	won, err := TransitionPromptInstructionApplication(context.Background(), application.ID, application.LockVersion, promptInstructionApplicationStateApplied, promptLearningFingerprint("d"), version.ContentHash)
	require.NoError(t, err)
	assert.True(t, won)
	won, err = TransitionPromptInstructionApplication(context.Background(), application.ID, application.LockVersion, promptInstructionApplicationStateRollbackPending, "", "")
	require.ErrorIs(t, err, ErrPromptInstructionApplication)
	assert.False(t, won)

	var applied PromptInstructionApplication
	require.NoError(t, DB.First(&applied, application.ID).Error)
	won, err = TransitionPromptInstructionApplication(context.Background(), applied.ID, applied.LockVersion, promptInstructionApplicationStateRollbackPending, "", "")
	require.NoError(t, err)
	assert.True(t, won)
	var rollbackPending PromptInstructionApplication
	require.NoError(t, DB.First(&rollbackPending, application.ID).Error)
	won, err = TransitionPromptInstructionApplication(context.Background(), rollbackPending.ID, rollbackPending.LockVersion, promptInstructionApplicationStateRolledBack, promptLearningFingerprint("e"), "")
	require.NoError(t, err)
	assert.True(t, won)
}

func TestPromptInstructionApplicationRejectsRevokedOrCrossScopeReferences(t *testing.T) {
	setupPromptLearningTestDB(t)
	policy, err := UpsertPromptLearningPolicy(context.Background(), PromptLearningPolicyInput{UserID: 17, ScopeRef: "project:alpha", Enabled: true})
	require.NoError(t, err)
	version, _, err := CreatePromptInstructionVersion(context.Background(), PromptInstructionVersionInput{
		UserID: 17, ScopeRef: "project:alpha", CommandID: "version-1", Content: "Instructions", Source: promptInstructionVersionSourceManual, CreatedBy: 17,
	})
	require.NoError(t, err)
	policy, err = UpsertPromptLearningPolicy(context.Background(), PromptLearningPolicyInput{UserID: 17, ScopeRef: "project:alpha", Enabled: false})
	require.NoError(t, err)
	_, _, err = CreatePromptInstructionApplication(context.Background(), PromptInstructionApplicationInput{
		UserID: 17, ScopeRef: "project:alpha", CommandID: "apply-revoked", VersionID: version.ID, TargetReceipt: promptLearningFingerprint("c"),
		AuthorizationGeneration: policy.Generation, AuthorizationExpiresAt: time.Now().Add(time.Hour).Unix(), CreatedBy: 17,
	})
	require.ErrorIs(t, err, ErrPromptLearningPolicyDisabled)

	otherPolicy, err := UpsertPromptLearningPolicy(context.Background(), PromptLearningPolicyInput{UserID: 17, ScopeRef: "project:beta", Enabled: true})
	require.NoError(t, err)
	_, _, err = CreatePromptInstructionApplication(context.Background(), PromptInstructionApplicationInput{
		UserID: 17, ScopeRef: "project:beta", CommandID: "apply-wrong-version", VersionID: version.ID, TargetReceipt: promptLearningFingerprint("c"),
		AuthorizationGeneration: otherPolicy.Generation, AuthorizationExpiresAt: time.Now().Add(time.Hour).Unix(), CreatedBy: 17,
	})
	require.ErrorIs(t, err, ErrPromptInstructionNotFound)
}

func promptLearningRunInput(policy *PromptLearningPolicy) PromptLearningRunInput {
	return PromptLearningRunInput{
		UserID: policy.UserID, ScopeRef: policy.ScopeRef, CommandID: "analysis-1", PolicyGeneration: policy.Generation,
		SampleUpperID: 24, SampleCount: 3, ModelRef: "gpt-6", TemplateVersion: "prompt-learning-v1",
		InputTokenLimit: 8_000, OutputTokenLimit: 2_000, MaximumAttempts: 2,
	}
}

func TestPromptLearningRunIsIdempotentScopedAndFenced(t *testing.T) {
	setupPromptLearningTestDB(t)
	policy, err := UpsertPromptLearningPolicy(context.Background(), PromptLearningPolicyInput{UserID: 17, ScopeRef: "project:alpha", Enabled: true})
	require.NoError(t, err)
	baseline, _, err := CreatePromptInstructionVersion(context.Background(), PromptInstructionVersionInput{
		UserID: 17, ScopeRef: "project:alpha", CommandID: "baseline", Content: "Preserve rollback instructions.", Source: promptInstructionVersionSourceManual, CreatedBy: 17,
	})
	require.NoError(t, err)
	input := promptLearningRunInput(policy)
	input.BaselineVersionID = &baseline.ID

	run, created, err := CreatePromptLearningRun(context.Background(), input)
	require.NoError(t, err)
	require.True(t, created)
	assert.Equal(t, promptLearningRunStatePending, run.State)
	replayed, created, err := CreatePromptLearningRun(context.Background(), input)
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, run.ID, replayed.ID)
	conflict := input
	conflict.ModelRef = "different-model"
	_, _, err = CreatePromptLearningRun(context.Background(), conflict)
	require.ErrorIs(t, err, ErrPromptInstructionConflict)

	claimed, won, err := ClaimPromptLearningRun(context.Background(), 17, "project:alpha", run.ID, "worker-a", time.Now().Add(time.Minute).Unix())
	require.NoError(t, err)
	require.True(t, won)
	require.Equal(t, promptLearningRunStateLeased, claimed.State)
	assert.Greater(t, claimed.LeaseFence, run.LeaseFence)

	updated, err := TransitionPromptLearningRun(context.Background(), 17, "project:alpha", run.ID, claimed.LeaseFence, promptLearningRunStatePreparing)
	require.NoError(t, err)
	assert.True(t, updated)
	updated, err = TransitionPromptLearningRun(context.Background(), 17, "project:alpha", run.ID, claimed.LeaseFence, promptLearningRunStateSubmitted)
	require.NoError(t, err)
	assert.True(t, updated)
	updated, err = TransitionPromptLearningRun(context.Background(), 17, "project:alpha", run.ID, claimed.LeaseFence, promptLearningRunStateSubmitUnknown)
	require.NoError(t, err)
	assert.True(t, updated)
	updated, err = TransitionPromptLearningRun(context.Background(), 17, "project:alpha", run.ID, claimed.LeaseFence, promptLearningRunStateOutcomeUnknown)
	require.NoError(t, err)
	assert.True(t, updated)
	updated, err = TransitionPromptLearningRun(context.Background(), 17, "project:alpha", run.ID, claimed.LeaseFence-1, promptLearningRunStateManuallyResolved)
	require.ErrorIs(t, err, ErrPromptLearningRun)
	assert.False(t, updated)
}

func TestPromptLearningPolicyRevokeCancelsUnsubmittedRunsButPreservesSubmittedUnknown(t *testing.T) {
	setupPromptLearningTestDB(t)
	policy, err := UpsertPromptLearningPolicy(context.Background(), PromptLearningPolicyInput{UserID: 17, ScopeRef: "project:alpha", Enabled: true})
	require.NoError(t, err)
	pending, _, err := CreatePromptLearningRun(context.Background(), promptLearningRunInput(policy))
	require.NoError(t, err)
	submittedInput := promptLearningRunInput(policy)
	submittedInput.CommandID = "analysis-submitted"
	submitted, _, err := CreatePromptLearningRun(context.Background(), submittedInput)
	require.NoError(t, err)
	claimed, won, err := ClaimPromptLearningRun(context.Background(), 17, "project:alpha", submitted.ID, "worker-a", time.Now().Add(time.Minute).Unix())
	require.NoError(t, err)
	require.True(t, won)
	for _, state := range []string{promptLearningRunStatePreparing, promptLearningRunStateSubmitted, promptLearningRunStateSubmitUnknown} {
		updated, transitionErr := TransitionPromptLearningRun(context.Background(), 17, "project:alpha", submitted.ID, claimed.LeaseFence, state)
		require.NoError(t, transitionErr)
		require.True(t, updated)
	}

	_, err = UpsertPromptLearningPolicy(context.Background(), PromptLearningPolicyInput{UserID: 17, ScopeRef: "project:alpha", Enabled: false})
	require.NoError(t, err)
	var pendingAfter PromptLearningRun
	var submittedAfter PromptLearningRun
	require.NoError(t, DB.First(&pendingAfter, pending.ID).Error)
	require.NoError(t, DB.First(&submittedAfter, submitted.ID).Error)
	assert.Equal(t, promptLearningRunStateCancelled, pendingAfter.State)
	assert.Equal(t, promptLearningRunStateSubmitUnknown, submittedAfter.State)
}

func TestPromptLearningRunUserCancellationStopsOnlyPreSubmissionWork(t *testing.T) {
	setupPromptLearningTestDB(t)
	policy, err := UpsertPromptLearningPolicy(context.Background(), PromptLearningPolicyInput{UserID: 17, ScopeRef: "project:alpha", Enabled: true})
	require.NoError(t, err)
	run, _, err := CreatePromptLearningRun(context.Background(), promptLearningRunInput(policy))
	require.NoError(t, err)
	cancelled, err := CancelPromptLearningRun(context.Background(), 17, "project:alpha", run.ID)
	require.NoError(t, err)
	assert.True(t, cancelled)
	cancelled, err = CancelPromptLearningRun(context.Background(), 17, "project:alpha", run.ID)
	require.NoError(t, err)
	assert.False(t, cancelled)
}
