package service

import (
	"context"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskEngineRunner_RunOnce_StaleDispatching_FailClosed(t *testing.T) {
	db := setupRecoveryTestDB(t)
	fixture := newRecoveryTestFixture(t, db, "runner-stale-dispatch", 1000, 500)

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

	// Simulate stale dispatch: set dispatch_started_at to 600s ago (> 5m threshold)
	staleTime := time.Now().Unix() - 600
	require.NoError(t, db.Exec("UPDATE task_submission_operations SET dispatch_started_at = ? WHERE id = ?", staleTime, fixture.Operation.ID).Error)

	runner := NewTaskEngineRunner(TaskEngineConfig{
		WorkerID:               "test-engine-dispatch",
		StaleDispatchThreshold: 5 * time.Minute,
		LogDB:                  db,
	}, db)

	report := runner.RunOnce(context.Background())

	// Validate report metrics
	assert.Equal(t, 1, report.StaleDispatchingRecovered)
	assert.Equal(t, 0, report.StaleUnfinishedRecovered)
	assert.Equal(t, 0, report.ExpiredBillingReclaimed)
	assert.Equal(t, 0, report.OutboxDelivered)
	assert.False(t, report.HasErrors())
	assert.Empty(t, report.Errors)
	assert.Greater(t, report.Duration, time.Duration(0))

	// Assert operation transitioned to submission_unknown with reason dispatch_timeout_fail_closed
	var op model.TaskSubmissionOperation
	require.NoError(t, db.First(&op, fixture.Operation.ID).Error)
	assert.Equal(t, model.TaskSubmissionOperationStatusSubmissionUnknown, op.Status)
	assert.Equal(t, "dispatch_timeout_fail_closed", op.ReasonCode)

	// Assert attempt transitioned to submission_unknown
	var att model.TaskSubmissionAttempt
	require.NoError(t, db.First(&att, fixture.Attempt.ID).Error)
	assert.Equal(t, model.TaskSubmissionAttemptStatusSubmissionUnknown, att.Status)
	assert.Equal(t, "dispatch_timeout_fail_closed", att.OutcomeCode)

	// Fail-closed invariant: User & token quota must NOT be refunded!
	var u model.User
	require.NoError(t, db.First(&u, fixture.User.Id).Error)
	assert.Equal(t, 900, u.Quota, "User quota must NOT be refunded on dispatch timeout")

	var tok model.Token
	require.NoError(t, db.First(&tok, fixture.Token.Id).Error)
	assert.Equal(t, 400, tok.RemainQuota, "Token quota must NOT be refunded on dispatch timeout")
	assert.Equal(t, 100, tok.UsedQuota)
}

func TestTaskEngineRunner_RunOnce_StaleReserved_SafeCancel(t *testing.T) {
	db := setupRecoveryTestDB(t)
	fixture := newRecoveryTestFixture(t, db, "runner-stale-reserved", 1000, 500)

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

	// Verify reserved state
	var op model.TaskSubmissionOperation
	require.NoError(t, db.First(&op, fixture.Operation.ID).Error)
	assert.Equal(t, model.TaskSubmissionOperationStatusReserved, op.Status)

	// Simulate stale reservation: created_at 700s ago (> 10m threshold)
	staleTime := time.Now().Unix() - 700
	require.NoError(t, db.Exec("UPDATE task_submission_operations SET created_at = ? WHERE id = ?", staleTime, fixture.Operation.ID).Error)

	runner := NewTaskEngineRunner(TaskEngineConfig{
		WorkerID:                  "test-engine-reserved",
		StaleReservationThreshold: 10 * time.Minute,
		LogDB:                     db,
	}, db)

	report := runner.RunOnce(context.Background())

	// Validate report metrics
	assert.Equal(t, 0, report.StaleDispatchingRecovered)
	assert.Equal(t, 1, report.StaleUnfinishedRecovered)
	assert.Equal(t, 0, report.ExpiredBillingReclaimed)
	assert.Equal(t, 0, report.OutboxDelivered)
	assert.False(t, report.HasErrors())
	assert.Empty(t, report.Errors)
	assert.Greater(t, report.Duration, time.Duration(0))

	// Assert operation is canceled
	require.NoError(t, db.First(&op, fixture.Operation.ID).Error)
	assert.Equal(t, model.TaskSubmissionOperationStatusCanceled, op.Status)
	assert.Equal(t, "stale_reservation_timeout", op.ReasonCode)

	// Refund invariant: Quota must be safely released back!
	var u model.User
	require.NoError(t, db.First(&u, fixture.User.Id).Error)
	assert.Equal(t, 1000, u.Quota, "User quota must be refunded on safe cancellation")

	var tok model.Token
	require.NoError(t, db.First(&tok, fixture.Token.Id).Error)
	assert.Equal(t, 500, tok.RemainQuota, "Token remain quota must be refunded on safe cancellation")
	assert.Equal(t, 0, tok.UsedQuota)
}

func TestTaskEngineRunner_RunOnce_PendingOutbox_Delivered(t *testing.T) {
	db := setupRecoveryTestDB(t)
	event, outbox := createTestOutbox(t, db, "runner-pending-outbox")

	require.Equal(t, model.TaskBillingLogOutboxStatePending, outbox.State)

	runner := NewTaskEngineRunner(TaskEngineConfig{
		WorkerID: "test-engine-outbox",
		LogDB:    db,
	}, db)

	report := runner.RunOnce(context.Background())

	// Validate report metrics
	assert.Equal(t, 1, report.OutboxDelivered)
	assert.Equal(t, 0, report.StaleDispatchingRecovered)
	assert.Equal(t, 0, report.StaleUnfinishedRecovered)
	assert.Equal(t, 0, report.ExpiredBillingReclaimed)
	assert.False(t, report.HasErrors())
	assert.Empty(t, report.Errors)
	assert.Greater(t, report.Duration, time.Duration(0))

	// Verify log entry created in logs table
	var logRecord model.Log
	err := db.Where("billing_event_id = ?", event.EventID).First(&logRecord).Error
	require.NoError(t, err)
	assert.Equal(t, event.UserID, logRecord.UserId)
	assert.Equal(t, 100, logRecord.Quota)
	assert.Equal(t, event.EventID, logRecord.BillingEventID)

	// Verify outbox entry transitioned to delivered
	var reloaded model.TaskBillingLogOutbox
	require.NoError(t, db.First(&reloaded, outbox.ID).Error)
	assert.Equal(t, model.TaskBillingLogOutboxStateDelivered, reloaded.State)
	require.NotNil(t, reloaded.DeliveredAt)
	assert.Greater(t, *reloaded.DeliveredAt, int64(0))
}

func TestTaskEngineRunner_RunOnce_ComprehensiveMetricsAndReport(t *testing.T) {
	db := setupRecoveryTestDB(t)

	// 1. Construct stale dispatching operation
	dispFixture := newRecoveryTestFixture(t, db, "comp-dispatch", 1000, 500)
	dispReserve, err := model.ReserveTaskQuota(db, model.TaskQuotaReservationInput{
		OperationID:              dispFixture.Operation.ID,
		UserID:                   dispFixture.User.Id,
		TokenID:                  dispFixture.Token.Id,
		ChannelID:                dispFixture.Attempt.ChannelID,
		ExpectedOperationVersion: dispFixture.Operation.LockVersion,
		Quota:                    100,
		BillingSource:            "wallet",
		BillingContext: model.TaskBillingContext{
			Version:         model.TaskBillingContextVersion,
			Complete:        true,
			ModelPrice:      1,
			ModelRatio:      1,
			GroupRatio:      1,
			OriginModelName: "comp-disp-model",
			PerCallBilling:  true,
		},
	})
	require.NoError(t, err)
	dispWon, err := model.StartTaskSubmissionDispatch(db, dispFixture.Operation.ID, model.TaskSubmissionDispatchTransition{
		ExpectedOperationVersion: dispReserve.OperationVersionAfter,
		ExpectedAttemptVersion:   dispFixture.Attempt.LockVersion,
	})
	require.NoError(t, err)
	require.True(t, dispWon)
	staleDispTime := time.Now().Unix() - 600
	require.NoError(t, db.Exec("UPDATE task_submission_operations SET dispatch_started_at = ? WHERE id = ?", staleDispTime, dispFixture.Operation.ID).Error)

	// 2. Construct stale unfinished reserved operation
	resFixture := newRecoveryTestFixture(t, db, "comp-reserved", 1000, 500)
	_, err = model.ReserveTaskQuota(db, model.TaskQuotaReservationInput{
		OperationID:              resFixture.Operation.ID,
		UserID:                   resFixture.User.Id,
		TokenID:                  resFixture.Token.Id,
		ChannelID:                resFixture.Attempt.ChannelID,
		ExpectedOperationVersion: resFixture.Operation.LockVersion,
		Quota:                    100,
		BillingSource:            "wallet",
		BillingContext: model.TaskBillingContext{
			Version:         model.TaskBillingContextVersion,
			Complete:        true,
			ModelPrice:      1,
			ModelRatio:      1,
			GroupRatio:      1,
			OriginModelName: "comp-res-model",
			PerCallBilling:  true,
		},
	})
	require.NoError(t, err)
	staleResTime := time.Now().Unix() - 700
	require.NoError(t, db.Exec("UPDATE task_submission_operations SET created_at = ? WHERE id = ?", staleResTime, resFixture.Operation.ID).Error)

	// 3. Construct expired claimed billing event
	expFixture := newRecoveryTestFixture(t, db, "comp-expired-event", 1000, 500)
	opID := expFixture.Operation.ID
	createdEvent, err := model.CreateOrLoadTaskBillingEvent(db, &model.TaskBillingEvent{
		OperationID:   &opID,
		EventType:     model.TaskBillingEventTypeReserve,
		UserID:        expFixture.User.Id,
		TokenID:       expFixture.Token.Id,
		ChannelID:     expFixture.Attempt.ChannelID,
		BillingSource: "wallet",
		QuotaDelta:    -100,
		ReasonCode:    "test_reserve",
	})
	require.NoError(t, err)
	claimWon, err := model.TransitionTaskBillingEvent(db, createdEvent.ID, model.TaskBillingEventTransition{
		From: model.TaskBillingEventStatePending,
		To:   model.TaskBillingEventStateClaimed,
		Lease: model.TaskRecoveryProcessingLease{
			WorkerID:     "old-worker",
			LeaseSeconds: 10,
		},
		ExpectedVersion: createdEvent.LockVersion,
	})
	require.NoError(t, err)
	require.True(t, claimWon)
	expiredUntil := time.Now().Unix() - 30
	require.NoError(t, db.Exec("UPDATE task_billing_events SET claimed_until = ? WHERE id = ?", expiredUntil, createdEvent.ID).Error)

	// 4. Construct pending outbox
	_, _ = createTestOutbox(t, db, "comp-outbox")

	runner := NewTaskEngineRunner(TaskEngineConfig{
		WorkerID:                  "test-comprehensive",
		StaleDispatchThreshold:    5 * time.Minute,
		StaleReservationThreshold: 10 * time.Minute,
		LogDB:                     db,
	}, db)

	// Execute RunOnce
	report := runner.RunOnce(context.Background())

	// Verify all metric counters
	assert.Equal(t, 1, report.StaleDispatchingRecovered)
	assert.Equal(t, 1, report.StaleUnfinishedRecovered)
	assert.Equal(t, 1, report.ExpiredBillingReclaimed)
	assert.Equal(t, 1, report.OutboxDelivered)
	assert.Greater(t, report.Duration, time.Duration(0))
	assert.False(t, report.HasErrors())
	assert.Empty(t, report.Errors)

	// Test ToJSON and common.Unmarshal
	jsonStr, err := report.ToJSON()
	require.NoError(t, err)
	assert.Contains(t, jsonStr, `"stale_dispatching_recovered":1`)
	assert.Contains(t, jsonStr, `"stale_unfinished_recovered":1`)
	assert.Contains(t, jsonStr, `"expired_billing_reclaimed":1`)
	assert.Contains(t, jsonStr, `"outbox_delivered":1`)

	var unmarshaled TaskEngineReport
	err = common.Unmarshal([]byte(jsonStr), &unmarshaled)
	require.NoError(t, err)
	assert.Equal(t, report.StaleDispatchingRecovered, unmarshaled.StaleDispatchingRecovered)
	assert.Equal(t, report.StaleUnfinishedRecovered, unmarshaled.StaleUnfinishedRecovered)
	assert.Equal(t, report.ExpiredBillingReclaimed, unmarshaled.ExpiredBillingReclaimed)
	assert.Equal(t, report.OutboxDelivered, unmarshaled.OutboxDelivered)
	assert.Equal(t, report.Duration, unmarshaled.Duration)
	assert.False(t, unmarshaled.HasErrors())

	// Idempotency verification: a second RunOnce recovers 0 items
	report2 := runner.RunOnce(context.Background())
	assert.Equal(t, 0, report2.StaleDispatchingRecovered)
	assert.Equal(t, 0, report2.StaleUnfinishedRecovered)
	assert.Equal(t, 0, report2.ExpiredBillingReclaimed)
	assert.Equal(t, 0, report2.OutboxDelivered)
	assert.False(t, report2.HasErrors())
}

func TestTaskEngineReport_HasErrorsAndErrorHandling(t *testing.T) {
	// 1. HasErrors on empty/nil errors
	r1 := TaskEngineReport{}
	assert.False(t, r1.HasErrors())

	// 2. HasErrors with errors
	r2 := TaskEngineReport{
		Errors: []string{"some execution error"},
	}
	assert.True(t, r2.HasErrors())

	// 3. Context cancellation handling in RunOnce
	db := setupRecoveryTestDB(t)
	runner := NewTaskEngineRunner(TaskEngineConfig{
		WorkerID: "test-canceled",
		LogDB:    db,
	}, db)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	report := runner.RunOnce(ctx)
	assert.True(t, report.HasErrors())
	assert.NotEmpty(t, report.Errors)
	assert.Contains(t, report.Errors[0], "context canceled")

	// 4. RunRecoveryPass with canceled context
	d, u, b, errs := runner.RunRecoveryPass(ctx)
	assert.Equal(t, 0, d)
	assert.Equal(t, 0, u)
	assert.Equal(t, 0, b)
	require.NotEmpty(t, errs)
	assert.ErrorIs(t, errs[0], context.Canceled)

	// 5. RunOutboxPass with canceled context
	delivered, err := runner.RunOutboxPass(ctx)
	assert.Equal(t, 0, delivered)
	assert.ErrorIs(t, err, context.Canceled)

	// 6. Nil runner safety
	var nilRunner *TaskEngineRunner
	dNil, uNil, bNil, errsNil := nilRunner.RunRecoveryPass(context.Background())
	assert.Equal(t, 0, dNil)
	assert.Equal(t, 0, uNil)
	assert.Equal(t, 0, bNil)
	assert.NotEmpty(t, errsNil)

	deliveredNil, errNil := nilRunner.RunOutboxPass(context.Background())
	assert.Equal(t, 0, deliveredNil)
	assert.Error(t, errNil)

	reportNil := nilRunner.RunOnce(context.Background())
	assert.True(t, reportNil.HasErrors())
}

func TestTaskEngineRunner_PassMethodsAndGetters(t *testing.T) {
	db := setupRecoveryTestDB(t)
	cfg := TaskEngineConfig{
		WorkerID:                  "getter-worker",
		StaleDispatchThreshold:    3 * time.Minute,
		StaleReservationThreshold: 8 * time.Minute,
		BatchSize:                 25,
		LeaseDuration:             45 * time.Second,
		OutboxMaxAttempts:         3,
		LogDB:                     db,
	}
	runner := NewTaskEngineRunner(cfg, db)

	// Verify getters
	assert.Equal(t, "getter-worker", runner.Config().WorkerID)
	assert.Equal(t, 3*time.Minute, runner.Config().StaleDispatchThreshold)
	assert.Equal(t, 8*time.Minute, runner.Config().StaleReservationThreshold)
	assert.Equal(t, 25, runner.Config().BatchSize)
	assert.Equal(t, 45*time.Second, runner.Config().LeaseDuration)
	assert.Equal(t, 3, runner.Config().OutboxMaxAttempts)
	assert.Equal(t, db, runner.DB())
	require.NotNil(t, runner.RecoveryWorker())
	require.NotNil(t, runner.OutboxService())
	assert.Equal(t, "getter-worker", runner.RecoveryWorker().WorkerID)
	assert.Equal(t, "getter-worker", runner.OutboxService().WorkerID)

	// Direct call to RunRecoveryPass
	disp, unfin, expired, errs := runner.RunRecoveryPass(context.Background())
	assert.Equal(t, 0, disp)
	assert.Equal(t, 0, unfin)
	assert.Equal(t, 0, expired)
	assert.Empty(t, errs)

	// Direct call to RunOutboxPass
	deliv, err := runner.RunOutboxPass(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, deliv)
}
