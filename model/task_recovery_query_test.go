package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func createTestOperationWithState(t *testing.T, db *gorm.DB, label string, status TaskSubmissionOperationStatus, dispatchStartedAt, createdAt int64) *TaskSubmissionOperation {
	t.Helper()
	t.Setenv("TASK_RECOVERY_IDEMPOTENCY_SECRET", strings.Repeat("1a", 32))
	require.NoError(t, db.AutoMigrate(b2SubmissionModels()...))
	ensureB2SubmissionOwner(t, db, 21)
	op := newB2SubmissionOperation(t, 21, "POST", TaskSubmissionOperationKindVideoCreate, label, fmt.Sprintf(`{"label": "%s"}`, label))
	intent, err := CreateOrLoadTaskSubmissionIntent(db, op, &TaskSubmissionAttempt{
		AttemptNo: 1, ChannelID: 61, Provider: "fixture", RequestClass: "video",
	})
	require.NoError(t, err)

	currentOp := intent.Operation
	currentAttempt := intent.Attempt

	switch status {
	case TaskSubmissionOperationStatusPrepared:
		// already prepared

	case TaskSubmissionOperationStatusReserved:
		won, err := TransitionTaskSubmissionOperation(db, currentOp.ID, TaskSubmissionOperationTransition{
			From: TaskSubmissionOperationStatusPrepared, To: TaskSubmissionOperationStatusReserved,
			ExpectedVersion: currentOp.LockVersion,
		})
		require.NoError(t, err)
		require.True(t, won)

	case TaskSubmissionOperationStatusDispatching:
		won, err := TransitionTaskSubmissionOperation(db, currentOp.ID, TaskSubmissionOperationTransition{
			From: TaskSubmissionOperationStatusPrepared, To: TaskSubmissionOperationStatusReserved,
			ExpectedVersion: currentOp.LockVersion,
		})
		require.NoError(t, err)
		require.True(t, won)
		require.NoError(t, db.First(currentOp, currentOp.ID).Error)
		require.NoError(t, db.First(currentAttempt, currentAttempt.ID).Error)
		won, err = StartTaskSubmissionDispatch(db, currentOp.ID, TaskSubmissionDispatchTransition{
			ExpectedOperationVersion: currentOp.LockVersion,
			ExpectedAttemptVersion:   currentAttempt.LockVersion,
		})
		require.NoError(t, err)
		require.True(t, won)

	case TaskSubmissionOperationStatusSubmissionUnknown:
		won, err := TransitionTaskSubmissionOperation(db, currentOp.ID, TaskSubmissionOperationTransition{
			From: TaskSubmissionOperationStatusPrepared, To: TaskSubmissionOperationStatusReserved,
			ExpectedVersion: currentOp.LockVersion,
		})
		require.NoError(t, err)
		require.True(t, won)
		require.NoError(t, db.First(currentOp, currentOp.ID).Error)
		require.NoError(t, db.First(currentAttempt, currentAttempt.ID).Error)
		won, err = StartTaskSubmissionDispatch(db, currentOp.ID, TaskSubmissionDispatchTransition{
			ExpectedOperationVersion: currentOp.LockVersion,
			ExpectedAttemptVersion:   currentAttempt.LockVersion,
		})
		require.NoError(t, err)
		require.True(t, won)
		require.NoError(t, db.First(currentOp, currentOp.ID).Error)
		won, err = TransitionTaskSubmissionOperation(db, currentOp.ID, TaskSubmissionOperationTransition{
			From: TaskSubmissionOperationStatusDispatching, To: TaskSubmissionOperationStatusSubmissionUnknown,
			ExpectedVersion: currentOp.LockVersion,
		})
		require.NoError(t, err)
		require.True(t, won)

	case TaskSubmissionOperationStatusOutcomeUnknown:
		// Direct test fixture setup for outcome_unknown
		require.NoError(t, taskRecoveryControlledWrite(db).Table("task_submission_operations").Where("id = ?", currentOp.ID).Update("status", TaskSubmissionOperationStatusOutcomeUnknown).Error)

	case TaskSubmissionOperationStatusAccepted:
		require.NoError(t, taskRecoveryControlledWrite(db).Table("task_submission_operations").Where("id = ?", currentOp.ID).Update("status", TaskSubmissionOperationStatusAccepted).Error)
	}

	// Apply timestamps if requested
	updates := make(map[string]interface{})
	if dispatchStartedAt > 0 {
		updates["dispatch_started_at"] = dispatchStartedAt
	}
	if createdAt > 0 {
		updates["created_at"] = createdAt
	}
	if len(updates) > 0 {
		require.NoError(t, taskRecoveryControlledWrite(db).Table("task_submission_operations").Where("id = ?", currentOp.ID).Updates(updates).Error)
	}
	require.NoError(t, db.First(currentOp, currentOp.ID).Error)
	return currentOp
}

func TestListStaleDispatchingOperations(t *testing.T) {
	db := openB2SubmissionSQLite(t)

	// 1. Stale dispatching
	stale1 := createTestOperationWithState(t, db, "stale1", TaskSubmissionOperationStatusDispatching, 800, 700)
	stale2 := createTestOperationWithState(t, db, "stale2", TaskSubmissionOperationStatusDispatching, 600, 500)
	// 2. Fresh dispatching
	_ = createTestOperationWithState(t, db, "fresh", TaskSubmissionOperationStatusDispatching, 950, 900)
	// 3. Stale timestamp, but already accepted
	_ = createTestOperationWithState(t, db, "accepted", TaskSubmissionOperationStatusAccepted, 600, 500)

	// Scan cutoff at 900
	results, err := ListStaleDispatchingOperations(db, 900, 10)
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, stale2.ID, results[0].ID) // ordered by dispatch_started_at ASC
	assert.Equal(t, stale1.ID, results[1].ID)

	// Boundary: negative cutoff
	_, err = ListStaleDispatchingOperations(db, -1, 10)
	require.Error(t, err)

	// Nil DB
	_, err = ListStaleDispatchingOperations(nil, 900, 10)
	require.ErrorIs(t, err, gorm.ErrInvalidDB)
}

func TestListUnfinishedPreparedOrReservedOperations(t *testing.T) {
	db := openB2SubmissionSQLite(t)

	// 1. Prepared older than cutoff
	prep := createTestOperationWithState(t, db, "prep", TaskSubmissionOperationStatusPrepared, 0, 500)
	// 2. Reserved older than cutoff
	res := createTestOperationWithState(t, db, "res", TaskSubmissionOperationStatusReserved, 0, 600)
	// 3. Reserved newer than cutoff
	_ = createTestOperationWithState(t, db, "res_new", TaskSubmissionOperationStatusReserved, 0, 950)
	// 4. Dispatching older than cutoff (must not appear)
	_ = createTestOperationWithState(t, db, "disp", TaskSubmissionOperationStatusDispatching, 500, 500)

	results, err := ListUnfinishedPreparedOrReservedOperations(db, 800, 10)
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, prep.ID, results[0].ID)
	assert.Equal(t, res.ID, results[1].ID)
}

func TestListUnknownTaskSubmissionOperations(t *testing.T) {
	db := openB2SubmissionSQLite(t)

	subUnknown := createTestOperationWithState(t, db, "sub_unknown", TaskSubmissionOperationStatusSubmissionUnknown, 500, 500)
	outUnknown := createTestOperationWithState(t, db, "out_unknown", TaskSubmissionOperationStatusOutcomeUnknown, 500, 500)
	_ = createTestOperationWithState(t, db, "accepted", TaskSubmissionOperationStatusAccepted, 500, 500)

	results, err := ListUnknownTaskSubmissionOperations(db, 10)
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, subUnknown.ID, results[0].ID)
	assert.Equal(t, outUnknown.ID, results[1].ID)
}

func TestListClaimableTaskBillingLogOutboxes(t *testing.T) {
	db := openB2SubmissionSQLite(t)
	now := int64(1000)

	createEventAndOutbox := func(label string) (*TaskBillingEvent, *TaskBillingLogOutbox) {
		op := createTestOperationWithState(t, db, "op_"+label, TaskSubmissionOperationStatusReserved, 0, 500)
		event, err := CreateOrLoadTaskBillingEvent(db, &TaskBillingEvent{
			OperationID:   &op.ID,
			EventType:     TaskBillingEventTypeReserve,
			UserID:        op.UserID,
			TokenID:       op.TokenID,
			ChannelID:     61,
			BillingSource: "wallet",
			QuotaDelta:    -10,
		})
		require.NoError(t, err)

		candidate, err := NewTaskBillingLogOutbox(event, TaskBillingLogPayload{
			Content:   "outbox test " + label,
			ModelName: "test-model",
			Group:     "default",
		})
		require.NoError(t, err)

		outbox, err := CreateOrLoadTaskBillingLogOutbox(db, candidate)
		require.NoError(t, err)
		return event, outbox
	}

	// 1. Pending (claimable)
	_, p := createEventAndOutbox("pending")

	// 2. Retryable past (claimable)
	_, rPast := createEventAndOutbox("r_past")
	won, err := TransitionTaskBillingLogOutbox(db, rPast.ID, TaskBillingLogOutboxTransition{
		From:            TaskBillingLogOutboxStatePending,
		To:              TaskBillingLogOutboxStateClaimed,
		ExpectedVersion: rPast.LockVersion,
		Lease: TaskRecoveryProcessingLease{
			WorkerID:     "worker-1",
			LeaseSeconds: 60,
		},
	})
	require.NoError(t, err)
	require.True(t, won)
	var rPastReloaded TaskBillingLogOutbox
	require.NoError(t, db.First(&rPastReloaded, rPast.ID).Error)
	won, err = TransitionTaskBillingLogOutbox(db, rPast.ID, TaskBillingLogOutboxTransition{
		From:              TaskBillingLogOutboxStateClaimed,
		To:                TaskBillingLogOutboxStateRetryable,
		WorkerID:          "worker-1",
		ExpectedVersion:   rPastReloaded.LockVersion,
		LastErrorCode:     "network_timeout",
		RetryDelaySeconds: 10,
	})
	require.NoError(t, err)
	require.True(t, won)
	require.NoError(t, taskRecoveryControlledWrite(db).Table("task_billing_log_outboxes").Where("id = ?", rPast.ID).Update("next_attempt_at", 900).Error)

	// 3. Retryable future (NOT claimable)
	_, rFuture := createEventAndOutbox("r_future")
	won, err = TransitionTaskBillingLogOutbox(db, rFuture.ID, TaskBillingLogOutboxTransition{
		From:            TaskBillingLogOutboxStatePending,
		To:              TaskBillingLogOutboxStateClaimed,
		ExpectedVersion: rFuture.LockVersion,
		Lease: TaskRecoveryProcessingLease{
			WorkerID:     "worker-1",
			LeaseSeconds: 60,
		},
	})
	require.NoError(t, err)
	require.True(t, won)
	var rFutureReloaded TaskBillingLogOutbox
	require.NoError(t, db.First(&rFutureReloaded, rFuture.ID).Error)
	won, err = TransitionTaskBillingLogOutbox(db, rFuture.ID, TaskBillingLogOutboxTransition{
		From:              TaskBillingLogOutboxStateClaimed,
		To:                TaskBillingLogOutboxStateRetryable,
		WorkerID:          "worker-1",
		ExpectedVersion:   rFutureReloaded.LockVersion,
		LastErrorCode:     "network_timeout",
		RetryDelaySeconds: 200,
	})
	require.NoError(t, err)
	require.True(t, won)
	require.NoError(t, taskRecoveryControlledWrite(db).Table("task_billing_log_outboxes").Where("id = ?", rFuture.ID).Update("next_attempt_at", 1200).Error)

	// 4. Claimed expired (claimable via reclaim)
	_, cExpired := createEventAndOutbox("c_exp")
	won, err = TransitionTaskBillingLogOutbox(db, cExpired.ID, TaskBillingLogOutboxTransition{
		From:            TaskBillingLogOutboxStatePending,
		To:              TaskBillingLogOutboxStateClaimed,
		ExpectedVersion: cExpired.LockVersion,
		Lease: TaskRecoveryProcessingLease{
			WorkerID:     "worker-1",
			LeaseSeconds: 60,
		},
	})
	require.NoError(t, err)
	require.True(t, won)
	require.NoError(t, taskRecoveryControlledWrite(db).Table("task_billing_log_outboxes").Where("id = ?", cExpired.ID).Update("claimed_until", 900).Error)

	// 5. Delivered (NOT claimable)
	_, del := createEventAndOutbox("delivered")
	won, err = TransitionTaskBillingLogOutbox(db, del.ID, TaskBillingLogOutboxTransition{
		From:            TaskBillingLogOutboxStatePending,
		To:              TaskBillingLogOutboxStateClaimed,
		ExpectedVersion: del.LockVersion,
		Lease: TaskRecoveryProcessingLease{
			WorkerID:     "worker-1",
			LeaseSeconds: 60,
		},
	})
	require.NoError(t, err)
	require.True(t, won)
	var delReloaded TaskBillingLogOutbox
	require.NoError(t, db.First(&delReloaded, del.ID).Error)
	won, err = TransitionTaskBillingLogOutbox(db, del.ID, TaskBillingLogOutboxTransition{
		From:            TaskBillingLogOutboxStateClaimed,
		To:              TaskBillingLogOutboxStateDelivered,
		WorkerID:        "worker-1",
		ExpectedVersion: delReloaded.LockVersion,
	})
	require.NoError(t, err)
	require.True(t, won)

	results, err := ListClaimableTaskBillingLogOutboxes(db, now, 10)
	require.NoError(t, err)
	require.Len(t, results, 3)
	ids := []int64{results[0].ID, results[1].ID, results[2].ID}
	assert.Contains(t, ids, p.ID)
	assert.Contains(t, ids, rPast.ID)
	assert.Contains(t, ids, cExpired.ID)
}

func TestListStaleClaimedTaskBillingEvents(t *testing.T) {
	db := openB2SubmissionSQLite(t)
	now := int64(1000)

	createEvent := func(label string) *TaskBillingEvent {
		op := createTestOperationWithState(t, db, "ev_op_"+label, TaskSubmissionOperationStatusReserved, 0, 500)
		event, err := CreateOrLoadTaskBillingEvent(db, &TaskBillingEvent{
			OperationID:   &op.ID,
			EventType:     TaskBillingEventTypeReserve,
			UserID:        op.UserID,
			TokenID:       op.TokenID,
			ChannelID:     61,
			BillingSource: "wallet",
			QuotaDelta:    -10,
		})
		require.NoError(t, err)
		return event
	}

	eExpired := createEvent("expired")
	won, err := TransitionTaskBillingEvent(db, eExpired.ID, TaskBillingEventTransition{
		From:            TaskBillingEventStatePending,
		To:              TaskBillingEventStateClaimed,
		ExpectedVersion: eExpired.LockVersion,
		Lease: TaskRecoveryProcessingLease{
			WorkerID:     "worker-1",
			LeaseSeconds: 60,
		},
	})
	require.NoError(t, err)
	require.True(t, won)
	require.NoError(t, taskRecoveryControlledWrite(db).Table("task_billing_events").Where("id = ?", eExpired.ID).Update("claimed_until", 900).Error)

	results, err := ListStaleClaimedTaskBillingEvents(db, now, 10)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, eExpired.ID, results[0].ID)
}

func TestFindAttemptByOperationID(t *testing.T) {
	db := openB2SubmissionSQLite(t)

	op := createTestOperationWithState(t, db, "op_attempt", TaskSubmissionOperationStatusPrepared, 0, 500)
	var attempt TaskSubmissionAttempt
	require.NoError(t, db.Where("operation_id = ?", op.ID).First(&attempt).Error)

	found, err := FindAttemptByOperationID(db, op.ID)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, attempt.ID, found.ID)

	notFound, err := FindAttemptByOperationID(db, 999999)
	require.NoError(t, err)
	assert.Nil(t, notFound)

	_, err = FindAttemptByOperationID(db, 0)
	require.ErrorIs(t, err, ErrTaskRecoveryInvalidRecord)
}
