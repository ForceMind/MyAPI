package service

import (
	"context"
	"crypto/sha256"
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
		&model.UserSubscription{},
		&model.Task{},
		&model.TaskRecoveryIdentity{},
		&model.TaskSubmissionOperation{},
		&model.TaskSubmissionAttempt{},
		&model.TaskBillingEvent{},
		&model.TaskBillingLogOutbox{},
		&model.QuotaMutationReceipt{},
		&model.Log{},
	)
	require.NoError(t, err)
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

	var tok model.Token
	require.NoError(t, db.First(&tok, fixture.Token.Id).Error)
	assert.Equal(t, 500, tok.RemainQuota)
	assert.Equal(t, 0, tok.UsedQuota)

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
