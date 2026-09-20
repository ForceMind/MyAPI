package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupRecoveryTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	t.Setenv("TASK_RECOVERY_IDEMPOTENCY_SECRET", strings.Repeat("1a", 32))
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	err = db.AutoMigrate(
		&model.User{},
		&model.Token{},
		&model.Channel{},
		&model.UserSubscription{},
		&model.Task{},
		&model.TaskRecoveryIdentity{},
		&model.TaskSubmissionOperation{},
		&model.TaskSubmissionAttempt{},
		&model.TaskTerminalObservation{},
		&model.TaskBillingEvent{},
		&model.TaskBillingLogOutbox{},
		&model.QuotaMutationReceipt{},
		&model.UserQuotaMutationReceipt{},
		&model.AccountQuotaMutationReceipt{},
		&model.AccountQuotaReservationHead{},
		&model.AccountQuotaTerminalRecoveryObligation{},
		&model.AccountQuotaRefundFact{},
		&model.QuotaWorkCursor{},
		&model.QuotaWriterEpoch{},
		&model.QuotaProjectionObligation{},
		&model.Log{},
	)
	require.NoError(t, err)
	require.NoError(t, model.EnsureQuotaWriterEpochStateWithDB(db))
	setServiceQuotaWriterMode(t, db, model.QuotaWriterModeAuthoritative, 1)
	require.NoError(t, db.Create(&model.Channel{Id: 101, Name: "recovery-test"}).Error)
	return db
}

type recoveryTestFixture struct {
	User      model.User
	Token     model.Token
	Operation model.TaskSubmissionOperation
	Attempt   model.TaskSubmissionAttempt
}

func newRecoveryTestFixture(t *testing.T, db *gorm.DB, label string, userQuota, tokenQuota int) recoveryTestFixture {
	t.Helper()
	digest := sha256.Sum256([]byte(t.Name() + ":" + label))
	name := fmt.Sprintf("rec-%x", digest[:8])

	user := model.User{
		Username: name,
		AffCode:  name,
		Password: "test-fixture-password",
		Status:   common.UserStatusEnabled,
		Quota:    userQuota,
	}
	require.NoError(t, db.Create(&user).Error)

	token := model.Token{
		UserId:      user.Id,
		Key:         name,
		Status:      common.TokenStatusEnabled,
		RemainQuota: tokenQuota,
		ExpiredTime: -1,
	}
	require.NoError(t, db.Create(&token).Error)

	keyHash, err := model.HashTaskSubmissionIdempotencyKey(name)
	require.NoError(t, err)
	fingerprint := model.FingerprintTaskSubmissionRequest([]byte(`{"prompt":"recovery-test"}`))

	opCandidate := &model.TaskSubmissionOperation{
		UserID:             user.Id,
		TokenID:            token.Id,
		HTTPMethod:         "POST",
		OperationKind:      model.TaskSubmissionOperationKindVideoCreate,
		IdempotencyKeyHash: keyHash,
		RequestFingerprint: fingerprint,
	}

	attemptCandidate := &model.TaskSubmissionAttempt{
		AttemptNo:    1,
		ChannelID:    101,
		Provider:     "test-provider",
		RequestClass: "video",
	}

	intent, err := model.CreateOrLoadTaskSubmissionIntent(db, opCandidate, attemptCandidate)
	require.NoError(t, err)
	require.NotNil(t, intent)

	return recoveryTestFixture{
		User:      user,
		Token:     token,
		Operation: *intent.Operation,
		Attempt:   *intent.Attempt,
	}
}

func TestTaskRecoveryWorker_RecoverStaleDispatching_FailClosed(t *testing.T) {
	db := setupRecoveryTestDB(t)
	fixture := newRecoveryTestFixture(t, db, "stale-dispatch", 1000, 500)

	// Reserve 100 quota
	reserveReceipt, err := model.ReserveTaskQuota(db, model.TaskQuotaReservationInput{
		OperationID:              fixture.Operation.ID,
		UserID:                   fixture.User.Id,
		TokenID:                  fixture.Token.Id,
		ChannelID:                fixture.Attempt.ChannelID,
		ExpectedOperationVersion: fixture.Operation.LockVersion,
		Quota:                    100,
		BillingSource:            "wallet",
		ApplyStatistics:          true,
		BillingContext: model.TaskBillingContext{
			Version:         model.TaskBillingContextVersion,
			Complete:        true,
			ModelPrice:      1,
			ModelRatio:      1,
			GroupRatio:      1,
			OriginModelName: "test-model",
			PerCallBilling:  true,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, reserveReceipt)

	// Start dispatch -> both op and attempt move to dispatching
	won, err := model.StartTaskSubmissionDispatch(db, fixture.Operation.ID, model.TaskSubmissionDispatchTransition{
		ExpectedOperationVersion: reserveReceipt.OperationVersionAfter,
		ExpectedAttemptVersion:   fixture.Attempt.LockVersion,
	})
	require.NoError(t, err)
	require.True(t, won)

	// Verify initial dispatching state and reserved quota
	var op model.TaskSubmissionOperation
	require.NoError(t, db.First(&op, fixture.Operation.ID).Error)
	assert.Equal(t, model.TaskSubmissionOperationStatusDispatching, op.Status)
	require.NotNil(t, op.DispatchStartedAt)

	var u model.User
	require.NoError(t, db.First(&u, fixture.User.Id).Error)
	assert.Equal(t, 900, u.Quota)

	var tok model.Token
	require.NoError(t, db.First(&tok, fixture.Token.Id).Error)
	assert.Equal(t, 400, tok.RemainQuota)
	assert.Equal(t, 100, tok.UsedQuota)

	// Simulate stale dispatch: set dispatch_started_at to 600s ago (exceeding default 5m threshold)
	staleTime := time.Now().Unix() - 600
	require.NoError(t, db.Exec("UPDATE task_submission_operations SET dispatch_started_at = ? WHERE id = ?", staleTime, op.ID).Error)

	worker := NewTaskRecoveryWorker("recovery-worker-test-1")
	recoveredCount, err := worker.RecoverStaleDispatching(context.Background(), db)
	require.NoError(t, err)
	assert.Equal(t, 1, recoveredCount)

	// Assert operation transitioned to submission_unknown with reason dispatch_timeout_fail_closed
	require.NoError(t, db.First(&op, fixture.Operation.ID).Error)
	assert.Equal(t, model.TaskSubmissionOperationStatusSubmissionUnknown, op.Status)
	assert.Equal(t, "dispatch_timeout_fail_closed", op.ReasonCode)

	// Assert attempt transitioned to submission_unknown with outcome dispatch_timeout_fail_closed
	var att model.TaskSubmissionAttempt
	require.NoError(t, db.First(&att, fixture.Attempt.ID).Error)
	assert.Equal(t, model.TaskSubmissionAttemptStatusSubmissionUnknown, att.Status)
	assert.Equal(t, "dispatch_timeout_fail_closed", att.OutcomeCode)

	// CRITICAL ASSERTION: Fail-closed verification!
	// Reserved quota must NOT be refunded!
	require.NoError(t, db.First(&u, fixture.User.Id).Error)
	assert.Equal(t, 900, u.Quota, "Fail-closed invariant: User quota must NOT be refunded automatically on dispatch timeout")

	require.NoError(t, db.First(&tok, fixture.Token.Id).Error)
	assert.Equal(t, 400, tok.RemainQuota, "Fail-closed invariant: Token remain quota must NOT be refunded automatically")
	assert.Equal(t, 100, tok.UsedQuota)

	// Assert idempotency: second run recovers 0
	recoveredAgain, err := worker.RecoverStaleDispatching(context.Background(), db)
	require.NoError(t, err)
	assert.Equal(t, 0, recoveredAgain)
}

func TestTaskRecoveryWorker_RecoverStaleUnfinished_Reserved_SafeCancel(t *testing.T) {
	db := setupRecoveryTestDB(t)
	fixture := newRecoveryTestFixture(t, db, "stale-reserved", 1000, 500)

	// Reserve 100 quota without dispatching
	reserveReceipt, err := model.ReserveTaskQuota(db, model.TaskQuotaReservationInput{
		OperationID:              fixture.Operation.ID,
		UserID:                   fixture.User.Id,
		TokenID:                  fixture.Token.Id,
		ChannelID:                fixture.Attempt.ChannelID,
		ExpectedOperationVersion: fixture.Operation.LockVersion,
		Quota:                    100,
		BillingSource:            "wallet",
		ApplyStatistics:          true,
		BillingContext: model.TaskBillingContext{
			Version:         model.TaskBillingContextVersion,
			Complete:        true,
			ModelPrice:      1,
			ModelRatio:      1,
			GroupRatio:      1,
			OriginModelName: "test-model",
			PerCallBilling:  true,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, reserveReceipt)

	var op model.TaskSubmissionOperation
	require.NoError(t, db.First(&op, fixture.Operation.ID).Error)
	assert.Equal(t, model.TaskSubmissionOperationStatusReserved, op.Status)

	var u model.User
	require.NoError(t, db.First(&u, fixture.User.Id).Error)
	assert.Equal(t, 900, u.Quota)

	// Simulate stale reservation: created_at 700s ago (older than 10m threshold)
	staleTime := time.Now().Unix() - 700
	require.NoError(t, db.Exec("UPDATE task_submission_operations SET created_at = ? WHERE id = ?", staleTime, op.ID).Error)

	worker := NewTaskRecoveryWorker("recovery-worker-test-2")
	recoveredCount, err := worker.RecoverStaleUnfinished(context.Background(), db)
	require.NoError(t, err)
	assert.Equal(t, 1, recoveredCount)

	// Assert operation is canceled
	require.NoError(t, db.First(&op, fixture.Operation.ID).Error)
	assert.Equal(t, model.TaskSubmissionOperationStatusCanceled, op.Status)
	assert.Equal(t, "stale_reservation_timeout", op.ReasonCode)

	// CRITICAL ASSERTION: Reserved quota IS safely released back!
	require.NoError(t, db.First(&u, fixture.User.Id).Error)
	assert.Equal(t, 1000, u.Quota, "Quota must be fully released back to user on safe cancellation")
	assert.Zero(t, u.UsedQuota)
	assert.Equal(t, 1, u.RequestCount)

	var tok model.Token
	require.NoError(t, db.First(&tok, fixture.Token.Id).Error)
	assert.Equal(t, 500, tok.RemainQuota)
	assert.Equal(t, 0, tok.UsedQuota)
	var channel model.Channel
	require.NoError(t, db.First(&channel, fixture.Attempt.ChannelID).Error)
	assert.Zero(t, channel.UsedQuota)
	releaseReceipt, err := model.FindTaskQuotaReceipt(db, fixture.Operation.ID, string(model.TaskBillingEventTypeRefund), fixture.User.Id, fixture.Token.Id)
	require.NoError(t, err)
	assert.Equal(t, "local.stale_reservation_timeout", releaseReceipt.EvidenceID)
	var releaseProjection model.QuotaProjectionObligation
	require.NoError(t, db.Where("receipt_kind = ? AND receipt_id = ?", "task", releaseReceipt.ID).First(&releaseProjection).Error)
	assert.GreaterOrEqual(t, releaseProjection.Attempts, 1, "stale release must invoke projection after commit")

	// Assert refund receipt and billing event exist
	storedReceipt, err := model.FindTaskQuotaReceipt(db, op.ID, string(model.TaskBillingEventTypeRefund), fixture.User.Id, fixture.Token.Id)
	require.NoError(t, err)
	require.NotNil(t, storedReceipt)
	assert.Equal(t, int64(100), storedReceipt.Quota)
}

func TestTaskRecoveryWorker_RecoverStaleUnfinished_Prepared_Cancel(t *testing.T) {
	db := setupRecoveryTestDB(t)
	fixture := newRecoveryTestFixture(t, db, "stale-prepared", 1000, 500)

	// Operation stays in prepared (never reserved)
	staleTime := time.Now().Unix() - 700
	require.NoError(t, db.Exec("UPDATE task_submission_operations SET created_at = ? WHERE id = ?", staleTime, fixture.Operation.ID).Error)

	worker := NewTaskRecoveryWorker("recovery-worker-test-3")
	recoveredCount, err := worker.RecoverStaleUnfinished(context.Background(), db)
	require.NoError(t, err)
	assert.Equal(t, 1, recoveredCount)

	var op model.TaskSubmissionOperation
	require.NoError(t, db.First(&op, fixture.Operation.ID).Error)
	assert.Equal(t, model.TaskSubmissionOperationStatusCanceled, op.Status)
	assert.Equal(t, "stale_prepared_timeout", op.ReasonCode)
}

func TestTaskRecoveryWorker_RecoverStaleUnfinished_AlreadyDispatched_Skipped(t *testing.T) {
	db := setupRecoveryTestDB(t)
	fixture := newRecoveryTestFixture(t, db, "dispatched-skip", 1000, 500)

	// Reserve and dispatch
	receipt, err := model.ReserveTaskQuota(db, model.TaskQuotaReservationInput{
		OperationID:              fixture.Operation.ID,
		UserID:                   fixture.User.Id,
		TokenID:                  fixture.Token.Id,
		ChannelID:                fixture.Attempt.ChannelID,
		ExpectedOperationVersion: fixture.Operation.LockVersion,
		Quota:                    100,
		BillingSource:            "wallet",
		ApplyStatistics:          true,
		BillingContext: model.TaskBillingContext{
			Version:         model.TaskBillingContextVersion,
			Complete:        true,
			ModelPrice:      1,
			ModelRatio:      1,
			GroupRatio:      1,
			OriginModelName: "test-model",
			PerCallBilling:  true,
		},
	})
	require.NoError(t, err)

	won, err := model.StartTaskSubmissionDispatch(db, fixture.Operation.ID, model.TaskSubmissionDispatchTransition{
		ExpectedOperationVersion: receipt.OperationVersionAfter,
		ExpectedAttemptVersion:   fixture.Attempt.LockVersion,
	})
	require.NoError(t, err)
	require.True(t, won)

	// Even if created_at is old, attempt HAS entered dispatching
	staleTime := time.Now().Unix() - 700
	require.NoError(t, db.Exec("UPDATE task_submission_operations SET created_at = ? WHERE id = ?", staleTime, fixture.Operation.ID).Error)

	worker := NewTaskRecoveryWorker("recovery-worker-test-4")
	recoveredCount, err := worker.RecoverStaleUnfinished(context.Background(), db)
	require.NoError(t, err)
	assert.Equal(t, 0, recoveredCount, "Already dispatched operations must NOT be canceled by unfinished recovery")

	var op model.TaskSubmissionOperation
	require.NoError(t, db.First(&op, fixture.Operation.ID).Error)
	assert.Equal(t, model.TaskSubmissionOperationStatusDispatching, op.Status)
}

func TestTaskRecoveryWorker_RecoverExpiredBillingEvents_LeaseReclaim(t *testing.T) {
	db := setupRecoveryTestDB(t)
	fixture := newRecoveryTestFixture(t, db, "expired-event", 1000, 500)

	opID := fixture.Operation.ID
	event := &model.TaskBillingEvent{
		OperationID:   &opID,
		EventType:     model.TaskBillingEventTypeReserve,
		UserID:        fixture.User.Id,
		TokenID:       fixture.Token.Id,
		ChannelID:     fixture.Attempt.ChannelID,
		BillingSource: "wallet",
		QuotaDelta:    -100,
		ReasonCode:    "test_reserve",
	}
	createdEvent, err := model.CreateOrLoadTaskBillingEvent(db, event)
	require.NoError(t, err)
	require.NotNil(t, createdEvent)

	// Claim the event with a lease that expired in the past
	won, err := model.TransitionTaskBillingEvent(db, createdEvent.ID, model.TaskBillingEventTransition{
		From: model.TaskBillingEventStatePending,
		To:   model.TaskBillingEventStateClaimed,
		Lease: model.TaskRecoveryProcessingLease{
			WorkerID:     "expired-worker",
			LeaseSeconds: 10,
		},
		ExpectedVersion: createdEvent.LockVersion,
	})
	require.NoError(t, err)
	require.True(t, won)

	// Simulate expired lease: claimed_until in the past
	expiredUntil := time.Now().Unix() - 20
	require.NoError(t, db.Exec("UPDATE task_billing_events SET claimed_until = ? WHERE id = ?", expiredUntil, createdEvent.ID).Error)

	worker := NewTaskRecoveryWorker("reclaiming-worker")
	recoveredCount, err := worker.RecoverExpiredBillingEvents(context.Background(), db)
	require.NoError(t, err)
	assert.Equal(t, 1, recoveredCount)

	// Verify lease reclaimed by the new worker
	var updatedEvent model.TaskBillingEvent
	require.NoError(t, db.First(&updatedEvent, createdEvent.ID).Error)
	assert.Equal(t, "reclaiming-worker", updatedEvent.ClaimedBy)
	assert.Equal(t, 2, updatedEvent.AttemptCount)
	assert.Equal(t, "lease_expired", updatedEvent.LastErrorCode)
	assert.Greater(t, updatedEvent.ClaimedUntil, time.Now().Unix())
}

func TestTaskRecoveryWorker_ContextCancellation(t *testing.T) {
	db := setupRecoveryTestDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	worker := NewTaskRecoveryWorker("worker-canceled")
	_, err := worker.RecoverStaleDispatching(ctx, db)
	assert.ErrorIs(t, err, context.Canceled)

	_, err = worker.RecoverStaleUnfinished(ctx, db)
	assert.ErrorIs(t, err, context.Canceled)

	_, err = worker.RecoverExpiredBillingEvents(ctx, db)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestRecoverTerminalObservationsBadRecordDoesNotBlockLater(t *testing.T) {
	db := setupBridgeTestDB(t)
	bad := newBridgeTestFixture(t, db, "worker-bad", 1000, 1000, 100)
	good := newBridgeTestFixture(t, db, "worker-good", 1000, 1000, 100)
	badObservation, err := model.CreateOrLoadTaskTerminalObservation(db, model.TaskTerminalObservationInput{OperationID: bad.Operation.ID, TaskID: bad.Task.ID, Outcome: "succeeded", ActualQuota: 80, ReasonCode: "worker_terminal", RequestID: bad.Operation.RequestID, ResolutionSource: model.TaskSubmissionResolutionSourceProviderVerified, EvidenceID: bad.Attempt.ProviderOperationID, EvidenceVersion: 1})
	require.NoError(t, err)
	goodObservation, err := model.CreateOrLoadTaskTerminalObservation(db, model.TaskTerminalObservationInput{OperationID: good.Operation.ID, TaskID: good.Task.ID, Outcome: "succeeded", ActualQuota: 80, ReasonCode: "worker_terminal", RequestID: good.Operation.RequestID, ResolutionSource: model.TaskSubmissionResolutionSourceProviderVerified, EvidenceID: good.Attempt.ProviderOperationID, EvidenceVersion: 1})
	require.NoError(t, err)
	require.NoError(t, db.Delete(&model.Task{}, bad.Task.ID).Error)
	worker := NewTaskRecoveryWorker("worker-continue")
	processed, err := worker.RecoverTerminalObservations(context.Background(), db)
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	var storedBad, storedGood model.TaskTerminalObservation
	require.NoError(t, db.First(&storedBad, badObservation.ID).Error)
	require.NoError(t, db.First(&storedGood, goodObservation.ID).Error)
	assert.Equal(t, model.TaskTerminalObservationManualReview, storedBad.State)
	assert.Equal(t, model.TaskTerminalObservationApplied, storedGood.State)
	terminalReceipt, err := model.FindTaskQuotaReceipt(db, good.Operation.ID, string(model.TaskBillingEventTypeTerminalSettlement), good.User.Id, good.Token.Id)
	require.NoError(t, err)
	var terminalProjection model.QuotaProjectionObligation
	require.NoError(t, db.Where("receipt_kind = ? AND receipt_id = ?", "task", terminalReceipt.ID).First(&terminalProjection).Error)
	assert.GreaterOrEqual(t, terminalProjection.Attempts, 1, "terminal recovery must invoke projection after commit")
}

func TestRecoverTerminalObservationsTransientErrorContinuesAndAggregates(t *testing.T) {
	db := setupBridgeTestDB(t)
	first := newBridgeTestFixture(t, db, "worker-transient-first", 1000, 1000, 100)
	second := newBridgeTestFixture(t, db, "worker-transient-second", 1000, 1000, 100)
	firstObservation, err := model.CreateOrLoadTaskTerminalObservation(db, model.TaskTerminalObservationInput{OperationID: first.Operation.ID, TaskID: first.Task.ID, Outcome: "succeeded", ActualQuota: 90, ReasonCode: "worker_terminal", RequestID: first.Operation.RequestID, ResolutionSource: model.TaskSubmissionResolutionSourceProviderVerified, EvidenceID: first.Attempt.ProviderOperationID, EvidenceVersion: 1})
	require.NoError(t, err)
	secondObservation, err := model.CreateOrLoadTaskTerminalObservation(db, model.TaskTerminalObservationInput{OperationID: second.Operation.ID, TaskID: second.Task.ID, Outcome: "succeeded", ActualQuota: 90, ReasonCode: "worker_terminal", RequestID: second.Operation.RequestID, ResolutionSource: model.TaskSubmissionResolutionSourceProviderVerified, EvidenceID: second.Attempt.ProviderOperationID, EvidenceVersion: 1})
	require.NoError(t, err)
	failedOnce := false
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register("test:one-transient-user-read", func(tx *gorm.DB) {
		if !failedOnce && tx.Statement != nil && tx.Statement.Table == "users" {
			failedOnce = true
			tx.AddError(errors.New("serialization retry"))
		}
	}))
	processed, err := NewTaskRecoveryWorker("worker-transient").RecoverTerminalObservations(context.Background(), db)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "serialization retry")
	assert.Equal(t, 1, processed)
	var storedFirst, storedSecond model.TaskTerminalObservation
	require.NoError(t, db.First(&storedFirst, firstObservation.ID).Error)
	require.NoError(t, db.First(&storedSecond, secondObservation.ID).Error)
	assert.Equal(t, model.TaskTerminalObservationRetryable, storedFirst.State)
	assert.Equal(t, model.TaskTerminalObservationApplied, storedSecond.State)
	_, err = model.CreateOrLoadTaskTerminalObservation(db, model.TaskTerminalObservationInput{OperationID: first.Operation.ID, TaskID: first.Task.ID, Outcome: "succeeded", ActualQuota: 90, ReasonCode: "worker_terminal", RequestID: first.Operation.RequestID, ResolutionSource: model.TaskSubmissionResolutionSourceProviderVerified, EvidenceID: first.Attempt.ProviderOperationID, EvidenceVersion: 1, ManualReview: true})
	require.NoError(t, err)
	require.NoError(t, db.First(&storedFirst, firstObservation.ID).Error)
	assert.Equal(t, model.TaskTerminalObservationManualReview, storedFirst.State)
	assert.Zero(t, storedFirst.NextAttemptAt)
}

func TestRecoverTerminalObservationsRepairsMissingOutboxesForAllMutationTypes(t *testing.T) {
	db := setupBridgeTestDB(t)
	reserveOnly := newBridgeTestFixture(t, db, "repair-reserve", 1000, 1000, 100)
	settled := newBridgeTestFixture(t, db, "repair-terminal", 1000, 1000, 100)
	released := newBridgeTestFixture(t, db, "repair-refund", 1000, 1000, 100)
	_, err := settleDurableForTest(context.Background(), &settled.Task, 80, "provider_terminal")
	require.NoError(t, err)
	_, err = releaseDurableForTest(context.Background(), &released.Task, "provider_terminal")
	require.NoError(t, err)
	for _, item := range []struct {
		operationID int64
		mutation    string
	}{{reserveOnly.Operation.ID, string(model.TaskBillingEventTypeReserve)}, {settled.Operation.ID, string(model.TaskBillingEventTypeTerminalSettlement)}, {released.Operation.ID, string(model.TaskBillingEventTypeRefund)}} {
		require.NoError(t, db.Exec("DELETE FROM task_billing_log_outboxes WHERE billing_event_id IN (SELECT billing_event_id FROM quota_mutation_receipts WHERE operation_id = ? AND mutation_type = ?)", item.operationID, item.mutation).Error)
	}
	processed, err := NewTaskRecoveryWorker("repair-outboxes").RecoverTerminalObservations(context.Background(), db)
	require.NoError(t, err)
	assert.Zero(t, processed)
	var count int64
	require.NoError(t, db.Model(&model.TaskBillingLogOutbox{}).Count(&count).Error)
	assert.Equal(t, int64(5), count)
}

func TestRecoverTerminalObservationsMissingOutboxWithoutReceiptReportsAndDoesNotGuess(t *testing.T) {
	db := setupBridgeTestDB(t)
	fixture := newBridgeTestFixture(t, db, "repair-no-proof", 1000, 1000, 100)
	_, err := settleDurableForTest(context.Background(), &fixture.Task, 80, "provider_terminal")
	require.NoError(t, err)
	require.NoError(t, db.Exec("DELETE FROM task_billing_log_outboxes WHERE billing_event_id IN (SELECT billing_event_id FROM quota_mutation_receipts WHERE operation_id = ? AND mutation_type = ?)", fixture.Operation.ID, model.TaskBillingEventTypeTerminalSettlement).Error)
	require.NoError(t, db.Exec("DELETE FROM quota_mutation_receipts WHERE operation_id = ? AND mutation_type = ?", fixture.Operation.ID, model.TaskBillingEventTypeTerminalSettlement).Error)
	_, err = NewTaskRecoveryWorker("repair-no-proof").RecoverTerminalObservations(context.Background(), db)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "lacks proof")
	var count int64
	require.NoError(t, db.Model(&model.TaskBillingLogOutbox{}).Where("billing_event_id IN (SELECT event_id FROM task_billing_events WHERE operation_id = ? AND event_type = ?)", fixture.Operation.ID, model.TaskBillingEventTypeTerminalSettlement).Count(&count).Error)
	assert.Zero(t, count)
}

func TestRecoverTerminalObservationsHistoricalTaskWithoutActualEvidenceIsManual(t *testing.T) {
	db := setupBridgeTestDB(t)
	fixture := newBridgeTestFixture(t, db, "historical-no-actual", 1000, 1000, 100)
	require.NoError(t, db.Model(&model.Task{}).Where("id = ?", fixture.Task.ID).Updates(map[string]interface{}{"status": model.TaskStatusSuccess, "quota": 80, "progress": "100%"}).Error)
	processed, err := NewTaskRecoveryWorker("historical-manual").RecoverTerminalObservations(context.Background(), db)
	require.NoError(t, err)
	assert.Zero(t, processed)
	var observation model.TaskTerminalObservation
	require.NoError(t, db.Where("operation_id = ?", fixture.Operation.ID).First(&observation).Error)
	assert.Equal(t, model.TaskTerminalObservationManualReview, observation.State)
	assert.Zero(t, observation.ActualQuota)
	var operation model.TaskSubmissionOperation
	var user model.User
	var channel model.Channel
	require.NoError(t, db.First(&operation, fixture.Operation.ID).Error)
	require.NoError(t, db.First(&user, fixture.User.Id).Error)
	require.NoError(t, db.First(&channel, fixture.Task.ChannelId).Error)
	assert.Equal(t, model.TaskSubmissionOperationStatusAccepted, operation.Status)
	assert.Equal(t, 100, user.UsedQuota)
	assert.Equal(t, int64(100), channel.UsedQuota)
	var terminalCount int64
	require.NoError(t, db.Model(&model.QuotaMutationReceipt{}).Where("operation_id = ? AND mutation_type = ?", fixture.Operation.ID, model.TaskBillingEventTypeTerminalSettlement).Count(&terminalCount).Error)
	assert.Zero(t, terminalCount)
}

func TestRecoverStaleReservedWithoutStatisticsEvidenceCreatesManualReview(t *testing.T) {
	db := setupRecoveryTestDB(t)
	fixture := newRecoveryTestFixture(t, db, "stale-missing-stats", 1000, 500)
	reserve, err := model.ReserveTaskQuota(db, model.TaskQuotaReservationInput{OperationID: fixture.Operation.ID, UserID: fixture.User.Id, TokenID: fixture.Token.Id, ChannelID: fixture.Attempt.ChannelID, ExpectedOperationVersion: fixture.Operation.LockVersion, Quota: 100, BillingSource: "wallet", ApplyStatistics: false, BillingContext: ingressBillingContext(100)})
	require.NoError(t, err)
	require.False(t, reserve.StatisticsApplied)
	require.NoError(t, db.Exec("UPDATE task_submission_operations SET created_at = ? WHERE id = ?", time.Now().Unix()-700, fixture.Operation.ID).Error)
	count, err := NewTaskRecoveryWorker("stale-missing-stats").RecoverStaleUnfinished(context.Background(), db)
	require.NoError(t, err)
	assert.Zero(t, count)
	var operation model.TaskSubmissionOperation
	var observation model.TaskTerminalObservation
	var user model.User
	require.NoError(t, db.First(&operation, fixture.Operation.ID).Error)
	require.NoError(t, db.Where("operation_id = ?", fixture.Operation.ID).First(&observation).Error)
	require.NoError(t, db.First(&user, fixture.User.Id).Error)
	assert.Equal(t, model.TaskSubmissionOperationStatusReserved, operation.Status)
	assert.Equal(t, model.TaskTerminalObservationManualReview, observation.State)
	assert.Zero(t, observation.TaskID)
	assert.Equal(t, 900, user.Quota)
	assert.Zero(t, user.UsedQuota)
	assert.Zero(t, user.RequestCount)
}

func TestRecoverTerminalObservationsPendingIsNotStarvedByLowerRetryables(t *testing.T) {
	db := setupBridgeTestDB(t)
	retryableUserIDs := make(map[int]struct{}, 50)
	for index := 0; index < 50; index++ {
		fixture := newBridgeTestFixture(t, db, fmt.Sprintf("starvation-retryable-%02d", index), 1000, 1000, 100)
		observation, err := model.CreateOrLoadTaskTerminalObservation(db, model.TaskTerminalObservationInput{OperationID: fixture.Operation.ID, TaskID: fixture.Task.ID, Outcome: "succeeded", ActualQuota: 90, ReasonCode: "worker_terminal", RequestID: fixture.Operation.RequestID, ResolutionSource: model.TaskSubmissionResolutionSourceProviderVerified, EvidenceID: fixture.Attempt.ProviderOperationID, EvidenceVersion: 1})
		require.NoError(t, err)
		require.NoError(t, model.MarkTaskTerminalObservationRetryable(db, observation.ID, observation.LockVersion, 30))
		past := observation.CreatedAt - 100
		require.NoError(t, db.Exec("UPDATE task_terminal_observations SET created_at = ?, updated_at = ?, next_attempt_at = ?, retention_until = ? WHERE id = ?", past, past, past+1, past+model.TaskSubmissionTerminalRetentionSeconds, observation.ID).Error)
		retryableUserIDs[fixture.User.Id] = struct{}{}
	}
	pending := newBridgeTestFixture(t, db, "starvation-pending-51", 1000, 1000, 100)
	pendingObservation, err := model.CreateOrLoadTaskTerminalObservation(db, model.TaskTerminalObservationInput{OperationID: pending.Operation.ID, TaskID: pending.Task.ID, Outcome: "succeeded", ActualQuota: 80, ReasonCode: "worker_terminal", RequestID: pending.Operation.RequestID, ResolutionSource: model.TaskSubmissionResolutionSourceProviderVerified, EvidenceID: pending.Attempt.ProviderOperationID, EvidenceVersion: 1})
	require.NoError(t, err)

	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:retryable-users-remain-transient", func(tx *gorm.DB) {
		if tx.Statement == nil || tx.Statement.Schema == nil || tx.Statement.Schema.Table != "quota_mutation_receipts" {
			return
		}
		receipt, ok := tx.Statement.Dest.(*model.QuotaMutationReceipt)
		if !ok {
			return
		}
		if _, blocked := retryableUserIDs[receipt.UserID]; blocked {
			tx.AddError(errors.New("serialization retry"))
		}
	}))
	worker := NewTaskRecoveryWorker("starvation")
	worker.BatchSize = 50
	processed, recoverErr := worker.RecoverTerminalObservations(context.Background(), db)
	require.Error(t, recoverErr)
	assert.Equal(t, 1, processed)
	var stored model.TaskTerminalObservation
	require.NoError(t, db.First(&stored, pendingObservation.ID).Error)
	assert.Equal(t, model.TaskTerminalObservationApplied, stored.State)
}

func TestRecoverTerminalObservationsHistoricalScanRunsWhenObservationBatchIsFull(t *testing.T) {
	db := setupBridgeTestDB(t)
	pending := newBridgeTestFixture(t, db, "historical-full-pending", 1000, 1000, 100)
	_, err := model.CreateOrLoadTaskTerminalObservation(db, model.TaskTerminalObservationInput{OperationID: pending.Operation.ID, TaskID: pending.Task.ID, Outcome: "succeeded", ActualQuota: 80, ReasonCode: "worker_terminal", RequestID: pending.Operation.RequestID, ResolutionSource: model.TaskSubmissionResolutionSourceProviderVerified, EvidenceID: pending.Attempt.ProviderOperationID, EvidenceVersion: 1})
	require.NoError(t, err)
	historical := newBridgeTestFixture(t, db, "historical-full-scan", 1000, 1000, 100)
	require.NoError(t, db.Model(&model.Task{}).Where("id = ?", historical.Task.ID).Update("status", model.TaskStatusSuccess).Error)
	worker := NewTaskRecoveryWorker("historical-full")
	worker.BatchSize = 1
	processed, recoverErr := worker.RecoverTerminalObservations(context.Background(), db)
	require.NoError(t, recoverErr)
	assert.Equal(t, 1, processed)
	var observation model.TaskTerminalObservation
	require.NoError(t, db.Where("operation_id = ?", historical.Operation.ID).First(&observation).Error)
	assert.Equal(t, model.TaskTerminalObservationManualReview, observation.State)
	assert.Equal(t, "historical_terminal_success_unproven", observation.ReasonCode)
}
