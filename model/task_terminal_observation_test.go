package model

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type terminalObservationFixture struct {
	Input     TaskQuotaReservationInput
	Operation TaskSubmissionOperation
	Attempt   TaskSubmissionAttempt
	Task      Task
	Reserve   *QuotaMutationReceipt
}

func newTerminalObservationFixture(t *testing.T, db *gorm.DB, label string, reserved int64) terminalObservationFixture {
	t.Helper()
	t.Setenv("TASK_RECOVERY_IDEMPOTENCY_SECRET", "1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a")
	input := newSettleTestFixture(t, db, label, reserved)
	input.ApplyStatistics = true
	reserve, err := ReserveTaskQuota(db, input)
	require.NoError(t, err)
	var operation TaskSubmissionOperation
	require.NoError(t, db.First(&operation, input.OperationID).Error)
	var attempt TaskSubmissionAttempt
	require.NoError(t, db.Where("operation_id = ?", operation.ID).First(&attempt).Error)
	won, err := StartTaskSubmissionDispatch(db, operation.ID, TaskSubmissionDispatchTransition{ExpectedOperationVersion: reserve.OperationVersionAfter, ExpectedAttemptVersion: attempt.LockVersion})
	require.NoError(t, err)
	require.True(t, won)
	require.NoError(t, db.First(&operation, operation.ID).Error)
	require.NoError(t, db.First(&attempt, attempt.ID).Error)
	bc := TaskBillingContext(reserve.BillingContext)
	task := Task{TaskID: operation.PublicID, Platform: "test", UserId: operation.UserID, Group: reserve.Before.User.Group, ChannelId: reserve.ChannelID, Quota: int(reserve.EstimatedQuota), Action: "video.create", Status: TaskStatusSubmitted, Progress: "0%", PrivateData: TaskPrivateData{TokenId: operation.TokenID, BillingSource: reserve.BillingSource, SubscriptionId: reserve.SubscriptionID, BillingContext: &bc}}
	require.NoError(t, db.Create(&task).Error)
	won, err = TransitionTaskSubmissionAttempt(db, attempt.ID, TaskSubmissionAttemptTransition{From: TaskSubmissionAttemptStatusDispatching, To: TaskSubmissionAttemptStatusAccepted, ProviderOperationID: "provider-" + label, ExpectedVersion: attempt.LockVersion})
	require.NoError(t, err)
	require.True(t, won)
	won, err = TransitionTaskSubmissionOperation(db, operation.ID, TaskSubmissionOperationTransition{From: TaskSubmissionOperationStatusDispatching, To: TaskSubmissionOperationStatusAccepted, TaskID: &task.ID, ExpectedVersion: operation.LockVersion})
	require.NoError(t, err)
	require.True(t, won)
	require.NoError(t, db.First(&operation, operation.ID).Error)
	require.NoError(t, db.First(&attempt, attempt.ID).Error)
	return terminalObservationFixture{Input: input, Operation: operation, Attempt: attempt, Task: task, Reserve: reserve}
}

func newTerminalObservation(t *testing.T, db *gorm.DB, fixture terminalObservationFixture, actual int64, outcome string) *TaskTerminalObservation {
	t.Helper()
	observation, err := CreateOrLoadTaskTerminalObservation(db, TaskTerminalObservationInput{OperationID: fixture.Operation.ID, TaskID: fixture.Task.ID, Outcome: outcome, ActualQuota: actual, ReasonCode: "provider_terminal", RequestID: fixture.Operation.RequestID, ResolutionSource: TaskSubmissionResolutionSourceProviderVerified, EvidenceID: fixture.Attempt.ProviderOperationID, EvidenceVersion: 1})
	require.NoError(t, err)
	return observation
}

func TestTaskTerminalObservationConcurrentApplyIsEconomicallyOnce(t *testing.T) {
	db := openB2SubmissionSQLite(t)
	migrateB2SubmissionFixture(t, db)
	fixture := newTerminalObservationFixture(t, db, "concurrent-apply", 200)
	observation := newTerminalObservation(t, db, fixture, 150, "succeeded")
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := ApplyTaskTerminalObservation(db.Session(&gorm.Session{NewDB: true}), observation.ID)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	var user User
	var channel Channel
	var task Task
	var stored TaskTerminalObservation
	require.NoError(t, db.First(&user, fixture.Input.UserID).Error)
	require.NoError(t, db.First(&channel, fixture.Input.ChannelID).Error)
	require.NoError(t, db.First(&task, fixture.Task.ID).Error)
	require.NoError(t, db.First(&stored, observation.ID).Error)
	assert.Equal(t, 150, user.UsedQuota)
	assert.Equal(t, 1, user.RequestCount)
	assert.Equal(t, int64(150), channel.UsedQuota)
	assert.Equal(t, 150, task.Quota)
	assert.Equal(t, TaskTerminalObservationApplied, stored.State)
	var receiptCount, outboxCount int64
	require.NoError(t, db.Model(&QuotaMutationReceipt{}).Where("operation_id = ?", fixture.Operation.ID).Count(&receiptCount).Error)
	require.NoError(t, db.Model(&TaskBillingLogOutbox{}).Count(&outboxCount).Error)
	assert.Equal(t, int64(2), receiptCount)
	assert.Equal(t, int64(2), outboxCount)
}

func TestTaskTerminalObservationManualUpgradeConflictAndGuards(t *testing.T) {
	db := openB2SubmissionSQLite(t)
	migrateB2SubmissionFixture(t, db)
	fixture := newTerminalObservationFixture(t, db, "manual-guard", 100)
	observation := newTerminalObservation(t, db, fixture, 100, "succeeded")
	clamp := &common.QuotaClamp{Op: "QuotaRound", Kind: common.QuotaClampOverflow, Original: 1e20, Clamped: common.MaxQuota}
	upgraded, err := CreateOrLoadTaskTerminalObservation(db, TaskTerminalObservationInput{OperationID: fixture.Operation.ID, TaskID: fixture.Task.ID, Outcome: "succeeded", ActualQuota: 100, ReasonCode: "provider_terminal", RequestID: fixture.Operation.RequestID, ResolutionSource: TaskSubmissionResolutionSourceProviderVerified, EvidenceID: fixture.Attempt.ProviderOperationID, EvidenceVersion: 1, ManualReview: true, QuotaClamp: clamp})
	require.NoError(t, err)
	require.NoError(t, db.First(&upgraded, observation.ID).Error)
	assert.Equal(t, TaskTerminalObservationManualReview, upgraded.State)
	assert.Equal(t, "QuotaRound", upgraded.ClampOp)
	assert.Equal(t, string(common.QuotaClampOverflow), upgraded.ClampKind)
	assert.Equal(t, fmt.Sprint(clamp.Original), upgraded.ClampOriginal)
	assert.Equal(t, common.MaxQuota, upgraded.ClampClamped)
	assert.ErrorIs(t, db.Model(&TaskTerminalObservation{}).Where("id = ?", observation.ID).Update("state", TaskTerminalObservationApplied).Error, ErrTaskRecoveryInvalidRecord)
	assert.ErrorIs(t, db.Table("task_terminal_observations").Where("id = ?", observation.ID).Update("state", TaskTerminalObservationApplied).Error, ErrTaskRecoveryInvalidRecord)
	assert.ErrorIs(t, db.Table("task_terminal_observations AS o").Where("o.id = ?", observation.ID).Update("state", TaskTerminalObservationApplied).Error, ErrTaskRecoveryInvalidRecord)
	assert.ErrorIs(t, db.Table("(?) AS o", db.Table("task_terminal_observations")).Where("o.id = ?", observation.ID).Update("state", TaskTerminalObservationApplied).Error, ErrTaskRecoveryInvalidRecord)
	assert.ErrorIs(t, taskRecoveryControlledWrite(db).Table("task_terminal_observations").Where("id = ? AND state = ? AND lock_version = ?", observation.ID, observation.State, observation.LockVersion).Update("actual_quota", 99).Error, ErrTaskRecoveryInvalidRecord)
	assert.ErrorIs(t, db.Delete(&TaskTerminalObservation{}, observation.ID).Error, ErrTaskRecoveryInvalidRecord)
	assert.ErrorIs(t, db.Table("task_terminal_observations AS o").Where("o.id = ?", observation.ID).Delete(nil).Error, ErrTaskRecoveryInvalidRecord)
	assert.ErrorIs(t, db.Table("(?) AS o", db.Table("task_terminal_observations")).Where("o.id = ?", observation.ID).Delete(nil).Error, ErrTaskRecoveryInvalidRecord)

	fixture2 := newTerminalObservationFixture(t, db, "applied-conflict", 100)
	appliedObs := newTerminalObservation(t, db, fixture2, 100, "succeeded")
	_, err = ApplyTaskTerminalObservation(db, appliedObs.ID)
	require.NoError(t, err)
	_, err = CreateOrLoadTaskTerminalObservation(db, TaskTerminalObservationInput{OperationID: fixture2.Operation.ID, TaskID: fixture2.Task.ID, Outcome: "succeeded", ActualQuota: 100, ReasonCode: "provider_terminal", RequestID: fixture2.Operation.RequestID, ResolutionSource: TaskSubmissionResolutionSourceProviderVerified, EvidenceID: fixture2.Attempt.ProviderOperationID, EvidenceVersion: 1, ManualReview: true})
	require.NoError(t, err)
	_, err = CreateOrLoadTaskTerminalObservation(db, TaskTerminalObservationInput{OperationID: fixture2.Operation.ID, TaskID: fixture2.Task.ID, Outcome: "failed", ActualQuota: 0, ReasonCode: "provider_terminal", RequestID: fixture2.Operation.RequestID, ResolutionSource: TaskSubmissionResolutionSourceProviderVerified, EvidenceID: fixture2.Attempt.ProviderOperationID, EvidenceVersion: 1})
	require.ErrorIs(t, err, ErrTaskTerminalObservationConflict)
	var stored TaskTerminalObservation
	require.NoError(t, db.First(&stored, appliedObs.ID).Error)
	assert.Equal(t, TaskTerminalObservationApplied, stored.State)
	assert.Equal(t, 1, stored.ConflictCount)
	assert.NotEmpty(t, stored.LastConflictFingerprint)
}

func TestTaskTerminalApplyFailureMatrixRollsBack(t *testing.T) {
	cases := []struct {
		name   string
		actual int64
		manual bool
		inject func(*testing.T, *gorm.DB, terminalObservationFixture, *TaskTerminalObservation)
	}{
		{"terminal_receipt", 80, false, func(t *testing.T, db *gorm.DB, _ terminalObservationFixture, _ *TaskTerminalObservation) {
			require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:terminal-receipt", func(tx *gorm.DB) {
				if tx.Statement != nil && tx.Statement.Schema != nil && tx.Statement.Schema.Table == "quota_mutation_receipts" {
					tx.AddError(errors.New("receipt failure"))
				}
			}))
		}},
		{"event_cas", 80, false, func(t *testing.T, db *gorm.DB, _ terminalObservationFixture, _ *TaskTerminalObservation) {
			require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:event-cas", func(tx *gorm.DB) {
				if tx.Statement != nil && tx.Statement.Table == "task_billing_events" {
					tx.AddError(errors.New("event cas failure"))
				}
			}))
		}},
		{"task_update", 80, false, func(t *testing.T, db *gorm.DB, _ terminalObservationFixture, _ *TaskTerminalObservation) {
			require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:task-update", func(tx *gorm.DB) {
				if tx.Statement != nil && tx.Statement.Table == "tasks" {
					tx.AddError(errors.New("task update failure"))
				}
			}))
		}},
		{"outbox_insert", 80, false, func(t *testing.T, db *gorm.DB, _ terminalObservationFixture, _ *TaskTerminalObservation) {
			require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:outbox", func(tx *gorm.DB) {
				if tx.Statement != nil && tx.Statement.Schema != nil && tx.Statement.Schema.Table == "task_billing_log_outboxes" {
					tx.AddError(errors.New("outbox failure"))
				}
			}))
		}},
		{"observation_cas", 80, false, func(t *testing.T, db *gorm.DB, _ terminalObservationFixture, o *TaskTerminalObservation) {
			require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:observation-cas", func(tx *gorm.DB) {
				if tx.Statement != nil && tx.Statement.Table == "tasks" {
					_ = tx.Exec("UPDATE task_terminal_observations SET lock_version = lock_version + 1 WHERE id = ?", o.ID).Error
				}
			}))
		}},
		{"user_stats", 101, true, func(t *testing.T, db *gorm.DB, fixture terminalObservationFixture, _ *TaskTerminalObservation) {
			require.NoError(t, db.Model(&User{}).Where("id = ?", fixture.Input.UserID).Update("used_quota", common.MaxQuota).Error)
		}},
		{"channel_missing", 80, true, func(t *testing.T, db *gorm.DB, fixture terminalObservationFixture, _ *TaskTerminalObservation) {
			require.NoError(t, db.Delete(&Channel{}, fixture.Input.ChannelID).Error)
		}},
		{"channel_overflow", 101, true, func(t *testing.T, db *gorm.DB, fixture terminalObservationFixture, _ *TaskTerminalObservation) {
			require.NoError(t, db.Model(&Channel{}).Where("id = ?", fixture.Input.ChannelID).Update("used_quota", int64(math.MaxInt64)).Error)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := openB2SubmissionSQLite(t)
			migrateB2SubmissionFixture(t, db)
			fixture := newTerminalObservationFixture(t, db, tc.name, 100)
			observation := newTerminalObservation(t, db, fixture, tc.actual, "succeeded")
			tc.inject(t, db, fixture, observation)
			_, err := ApplyTaskTerminalObservation(db, observation.ID)
			require.Error(t, err)
			var op TaskSubmissionOperation
			var task Task
			var stored TaskTerminalObservation
			var user User
			var token Token
			require.NoError(t, db.First(&op, fixture.Operation.ID).Error)
			require.NoError(t, db.First(&task, fixture.Task.ID).Error)
			require.NoError(t, db.First(&stored, observation.ID).Error)
			require.NoError(t, db.First(&user, fixture.Input.UserID).Error)
			require.NoError(t, db.First(&token, fixture.Input.TokenID).Error)
			assert.Equal(t, TaskSubmissionOperationStatusAccepted, op.Status)
			assert.Equal(t, TaskStatus(TaskStatusSubmitted), task.Status)
			if tc.manual {
				assert.Equal(t, TaskTerminalObservationManualReview, stored.State)
			} else {
				assert.Equal(t, TaskTerminalObservationPending, stored.State)
			}
			assert.Equal(t, 900, user.Quota)
			assert.Equal(t, 400, token.RemainQuota)
			assert.Equal(t, 100, token.UsedQuota)
			var terminalReceipts int64
			require.NoError(t, db.Model(&QuotaMutationReceipt{}).Where("operation_id = ? AND mutation_type = ?", fixture.Operation.ID, TaskBillingEventTypeTerminalSettlement).Count(&terminalReceipts).Error)
			assert.Zero(t, terminalReceipts)
		})
	}
}

func TestTaskStatisticsChannelInt64Contract(t *testing.T) {
	t.Setenv("TASK_RECOVERY_IDEMPOTENCY_SECRET", "1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a")
	db := openB2SubmissionSQLite(t)
	migrateB2SubmissionFixture(t, db)
	input := newTaskQuotaReservationFixture(t, db, "channel-bigint")
	require.NoError(t, db.Model(&Channel{}).Where("id = ?", input.ChannelID).Update("used_quota", int64(math.MaxInt32)+100).Error)
	_, err := ReserveTaskQuota(db, input)
	require.NoError(t, err)
	var channel Channel
	require.NoError(t, db.First(&channel, input.ChannelID).Error)
	assert.Equal(t, int64(math.MaxInt32)+200, channel.UsedQuota)

	input2 := newTaskQuotaReservationFixture(t, db, "channel-overflow")
	require.NoError(t, db.Model(&Channel{}).Where("id = ?", input2.ChannelID).Update("used_quota", int64(math.MaxInt64)).Error)
	_, err = ReserveTaskQuota(db, input2)
	require.ErrorIs(t, err, ErrTaskTerminalObservationManualReview)
	var operation TaskSubmissionOperation
	require.NoError(t, db.First(&operation, input2.OperationID).Error)
	assert.Equal(t, TaskSubmissionOperationStatusPrepared, operation.Status)
}

func TestTaskReserveFailureMatrixRollsBack(t *testing.T) {
	cases := []struct {
		name   string
		inject func(*testing.T, *gorm.DB, TaskQuotaReservationInput)
	}{
		{"user_stats", func(t *testing.T, db *gorm.DB, input TaskQuotaReservationInput) {
			require.NoError(t, db.Model(&User{}).Where("id = ?", input.UserID).Update("used_quota", common.MaxQuota).Error)
		}},
		{"request_count_overflow", func(t *testing.T, db *gorm.DB, input TaskQuotaReservationInput) {
			require.NoError(t, db.Model(&User{}).Where("id = ?", input.UserID).Update("request_count", common.MaxQuota).Error)
		}},
		{"channel_missing", func(t *testing.T, db *gorm.DB, input TaskQuotaReservationInput) {
			require.NoError(t, db.Delete(&Channel{}, input.ChannelID).Error)
		}},
		{"channel_int64_overflow", func(t *testing.T, db *gorm.DB, input TaskQuotaReservationInput) {
			require.NoError(t, db.Model(&Channel{}).Where("id = ?", input.ChannelID).Update("used_quota", int64(math.MaxInt64)).Error)
		}},
		{"event_cas", func(t *testing.T, db *gorm.DB, _ TaskQuotaReservationInput) {
			require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:reserve-event", func(tx *gorm.DB) {
				if tx.Statement != nil && tx.Statement.Table == "task_billing_events" {
					tx.AddError(errors.New("event cas"))
				}
			}))
		}},
		{"receipt_insert", func(t *testing.T, db *gorm.DB, _ TaskQuotaReservationInput) {
			require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:reserve-receipt", func(tx *gorm.DB) {
				if tx.Statement != nil && tx.Statement.Schema != nil && tx.Statement.Schema.Table == "quota_mutation_receipts" {
					tx.AddError(errors.New("receipt insert"))
				}
			}))
		}},
		{"outbox_insert", func(t *testing.T, db *gorm.DB, _ TaskQuotaReservationInput) {
			require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:reserve-outbox", func(tx *gorm.DB) {
				if tx.Statement != nil && tx.Statement.Schema != nil && tx.Statement.Schema.Table == "task_billing_log_outboxes" {
					tx.AddError(errors.New("outbox insert"))
				}
			}))
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("TASK_RECOVERY_IDEMPOTENCY_SECRET", "1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a")
			db := openB2SubmissionSQLite(t)
			migrateB2SubmissionFixture(t, db)
			input := newTaskQuotaReservationFixture(t, db, tc.name)
			tc.inject(t, db, input)
			_, err := ReserveTaskQuota(db, input)
			require.Error(t, err)
			var user User
			var token Token
			var operation TaskSubmissionOperation
			require.NoError(t, db.First(&user, input.UserID).Error)
			require.NoError(t, db.First(&token, input.TokenID).Error)
			require.NoError(t, db.First(&operation, input.OperationID).Error)
			assert.Equal(t, 1000, user.Quota)
			assert.Equal(t, 500, token.RemainQuota)
			assert.Equal(t, TaskSubmissionOperationStatusPrepared, operation.Status)
			var receiptCount, eventCount, outboxCount int64
			require.NoError(t, db.Model(&QuotaMutationReceipt{}).Where("operation_id = ?", input.OperationID).Count(&receiptCount).Error)
			require.NoError(t, db.Model(&TaskBillingEvent{}).Where("operation_id = ?", input.OperationID).Count(&eventCount).Error)
			require.NoError(t, db.Model(&TaskBillingLogOutbox{}).Count(&outboxCount).Error)
			assert.Zero(t, receiptCount)
			assert.Zero(t, eventCount)
			assert.Zero(t, outboxCount)
		})
	}
}

func TestTaskReleaseOutboxFailureRollsBack(t *testing.T) {
	t.Setenv("TASK_RECOVERY_IDEMPOTENCY_SECRET", "1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a")
	db := openB2SubmissionSQLite(t)
	migrateB2SubmissionFixture(t, db)
	input := newSettleTestFixture(t, db, "release-outbox-rollback", 100)
	reserve, err := ReserveTaskQuota(db, input)
	require.NoError(t, err)
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:release-outbox-failure", func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Schema != nil && tx.Statement.Schema.Table == "task_billing_log_outboxes" {
			tx.AddError(errors.New("release outbox failure"))
		}
	}))
	_, err = ReleaseTaskQuotaReservation(db, TaskQuotaReleaseInput{OperationID: input.OperationID, UserID: input.UserID, TokenID: input.TokenID, ChannelID: input.ChannelID, ExpectedOperationVersion: reserve.OperationVersionAfter, ReasonCode: "provider_rejected", BillingContext: input.BillingContext, RequireStatisticsEvidence: true})
	require.Error(t, err)
	var user User
	var token Token
	require.NoError(t, db.First(&user, input.UserID).Error)
	require.NoError(t, db.First(&token, input.TokenID).Error)
	assert.Equal(t, 900, user.Quota)
	assert.Equal(t, 100, user.UsedQuota)
	assert.Equal(t, 400, token.RemainQuota)
	var refundCount, outboxCount int64
	require.NoError(t, db.Model(&QuotaMutationReceipt{}).Where("operation_id = ? AND mutation_type = ?", input.OperationID, TaskBillingEventTypeRefund).Count(&refundCount).Error)
	require.NoError(t, db.Model(&TaskBillingLogOutbox{}).Count(&outboxCount).Error)
	assert.Zero(t, refundCount)
	assert.Equal(t, int64(1), outboxCount)
}

func TestTaskTerminalManualReviewCASErrorsPropagate(t *testing.T) {
	db := openB2SubmissionSQLite(t)
	migrateB2SubmissionFixture(t, db)
	fixture := newTerminalObservationFixture(t, db, "manual-cas-error", 100)
	observation := newTerminalObservation(t, db, fixture, 80, "succeeded")
	require.NoError(t, db.Delete(&Task{}, fixture.Task.ID).Error)
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:manual-cas-error", func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Table == "task_terminal_observations" {
			tx.AddError(errors.New("manual cas persistence failed"))
		}
	}))
	_, err := ApplyTaskTerminalObservation(db, observation.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mark manual review")
	assert.Contains(t, err.Error(), "manual cas persistence failed")
	var stored TaskTerminalObservation
	require.NoError(t, db.First(&stored, observation.ID).Error)
	assert.Equal(t, TaskTerminalObservationPending, stored.State)
}

func TestTaskTerminalObservationCreateGuardRejectsForgedState(t *testing.T) {
	db := openB2SubmissionSQLite(t)
	migrateB2SubmissionFixture(t, db)
	forged := TaskTerminalObservation{OperationID: 1, TaskID: 1, Outcome: "succeeded", ActualQuota: 1, TaskStatus: TaskStatusSuccess, ReasonCode: "provider_terminal", RequestID: "request", Fingerprint: strings.Repeat("a", 64), State: TaskTerminalObservationApplied, CreatedAt: 1, UpdatedAt: 1, LockVersion: 1, RetentionUntil: 1 + TaskSubmissionTerminalRetentionSeconds}
	require.ErrorIs(t, db.Create(&forged).Error, ErrTaskRecoveryInvalidRecord)
}

func TestTaskTerminalObservationPendingManualUpgradeCASErrorsPropagate(t *testing.T) {
	db := openB2SubmissionSQLite(t)
	migrateB2SubmissionFixture(t, db)
	fixture := newTerminalObservationFixture(t, db, "manual-upgrade-cas", 100)
	observation := newTerminalObservation(t, db, fixture, 80, "succeeded")
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:manual-upgrade-cas", func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Table == "task_terminal_observations" {
			tx.AddError(errors.New("manual upgrade cas failed"))
		}
	}))
	_, err := CreateOrLoadTaskTerminalObservation(db, TaskTerminalObservationInput{OperationID: fixture.Operation.ID, TaskID: fixture.Task.ID, Outcome: "succeeded", ActualQuota: 80, ReasonCode: "provider_terminal", RequestID: fixture.Operation.RequestID, ManualReview: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "manual upgrade cas failed")
	var stored TaskTerminalObservation
	require.NoError(t, db.First(&stored, observation.ID).Error)
	assert.Equal(t, TaskTerminalObservationPending, stored.State)
}

func TestV2ReservationWithoutStatisticsEvidenceCannotReleaseOrSettle(t *testing.T) {
	t.Setenv("TASK_RECOVERY_IDEMPOTENCY_SECRET", "1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a")
	db := openB2SubmissionSQLite(t)
	migrateB2SubmissionFixture(t, db)
	input := newSettleTestFixture(t, db, "missing-stats-proof", 100)
	input.ApplyStatistics = false
	reserve, err := ReserveTaskQuota(db, input)
	require.NoError(t, err)
	assert.False(t, reserve.StatisticsApplied)
	assert.Zero(t, reserve.StatisticsVersion)
	_, err = ReleaseTaskQuotaReservation(db, TaskQuotaReleaseInput{OperationID: input.OperationID, UserID: input.UserID, TokenID: input.TokenID, ChannelID: input.ChannelID, ExpectedOperationVersion: reserve.OperationVersionAfter, ReasonCode: "provider_rejected", BillingContext: input.BillingContext})
	require.ErrorIs(t, err, ErrTaskTerminalObservationManualReview)
	_, err = SettleTaskQuotaReservation(db, TaskQuotaSettlementInput{OperationID: input.OperationID, UserID: input.UserID, TokenID: input.TokenID, ChannelID: input.ChannelID, ExpectedOperationVersion: reserve.OperationVersionAfter, ActualQuota: 100, ReasonCode: "provider_terminal", BillingContext: input.BillingContext})
	require.ErrorIs(t, err, ErrTaskTerminalObservationManualReview)
	var user User
	require.NoError(t, db.First(&user, input.UserID).Error)
	assert.Equal(t, 900, user.Quota)
	assert.Zero(t, user.UsedQuota)
	assert.Zero(t, user.RequestCount)
}

func TestTaskStatisticsEvidenceTamperingIsRejected(t *testing.T) {
	for _, target := range []string{"receipt", "event"} {
		t.Run(target, func(t *testing.T) {
			t.Setenv("TASK_RECOVERY_IDEMPOTENCY_SECRET", "1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a")
			db := openB2SubmissionSQLite(t)
			migrateB2SubmissionFixture(t, db)
			input := newTaskQuotaReservationFixture(t, db, "stats-tamper-"+target)
			receipt, err := ReserveTaskQuota(db, input)
			require.NoError(t, err)
			if target == "receipt" {
				require.NoError(t, db.Exec("UPDATE quota_mutation_receipts SET statistics_applied = ? WHERE id = ?", false, receipt.ID).Error)
			} else {
				require.NoError(t, db.Exec("UPDATE task_billing_events SET statistics_applied = ? WHERE event_id = ?", false, receipt.BillingEventID).Error)
			}
			_, err = FindTaskQuotaReservation(db, input.OperationID, input.UserID, input.TokenID)
			require.Error(t, err)
		})
	}
}

func TestTaskTerminalEconomicClampFailsWithoutPartialReceipt(t *testing.T) {
	for _, tc := range []struct {
		name         string
		subscription bool
		mutate       func(*testing.T, *gorm.DB, TaskQuotaReservationInput)
	}{
		{"wallet_max", false, func(t *testing.T, db *gorm.DB, input TaskQuotaReservationInput) {
			require.NoError(t, db.Model(&User{}).Where("id = ?", input.UserID).Update("quota", common.MaxQuota).Error)
		}},
		{"token_used_underflow", false, func(t *testing.T, db *gorm.DB, input TaskQuotaReservationInput) {
			require.NoError(t, db.Model(&Token{}).Where("id = ?", input.TokenID).Update("used_quota", 0).Error)
		}},
		{"subscription_used_underflow", true, func(t *testing.T, db *gorm.DB, input TaskQuotaReservationInput) {
			require.NoError(t, db.Model(&UserSubscription{}).Where("id = ?", input.SubscriptionID).Update("amount_used", int64(50)).Error)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("TASK_RECOVERY_IDEMPOTENCY_SECRET", "1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a")
			db := openB2SubmissionSQLite(t)
			migrateB2SubmissionFixture(t, db)
			var input TaskQuotaReservationInput
			if tc.subscription {
				input = newSettleSubscriptionFixture(t, db, tc.name, 100)
			} else {
				input = newSettleTestFixture(t, db, tc.name, 100)
			}
			input.ApplyStatistics = true
			reserve, err := ReserveTaskQuota(db, input)
			require.NoError(t, err)
			tc.mutate(t, db, input)
			_, err = ReleaseTaskQuotaReservation(db, TaskQuotaReleaseInput{OperationID: input.OperationID, UserID: input.UserID, TokenID: input.TokenID, ChannelID: input.ChannelID, ExpectedOperationVersion: reserve.OperationVersionAfter, ReasonCode: "provider_rejected", BillingContext: input.BillingContext, RequireStatisticsEvidence: true})
			require.ErrorIs(t, err, ErrTaskTerminalObservationManualReview)
			var count int64
			require.NoError(t, db.Model(&QuotaMutationReceipt{}).Where("operation_id = ? AND mutation_type = ?", input.OperationID, TaskBillingEventTypeRefund).Count(&count).Error)
			assert.Zero(t, count)
		})
	}
}

func TestTaskTerminalObservationRequiresAttemptBoundProviderEvidence(t *testing.T) {
	cases := []struct {
		name         string
		outcome      string
		source       string
		evidenceID   string
		evidenceHash string
		version      int
		wantError    bool
	}{
		{name: "missing_source", outcome: "succeeded", evidenceID: "bound", version: 1, wantError: true},
		{name: "missing_id", outcome: "succeeded", source: TaskSubmissionResolutionSourceProviderVerified, version: 1, wantError: true},
		{name: "hash_only", outcome: "succeeded", source: TaskSubmissionResolutionSourceProviderVerified, evidenceHash: strings.Repeat("a", 64), version: 1, wantError: true},
		{name: "wrong_id", outcome: "succeeded", source: TaskSubmissionResolutionSourceProviderVerified, evidenceID: "wrong", version: 1, wantError: true},
		{name: "bound_success", outcome: "succeeded", source: TaskSubmissionResolutionSourceProviderVerified, evidenceID: "fixture", version: 1},
		{name: "bound_failure", outcome: "failed", source: TaskSubmissionResolutionSourceProviderVerified, evidenceID: "fixture", version: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := openB2SubmissionSQLite(t)
			migrateB2SubmissionFixture(t, db)
			fixture := newTerminalObservationFixture(t, db, "evidence-"+tc.name, 100)
			evidenceID := tc.evidenceID
			if evidenceID == "fixture" {
				evidenceID = fixture.Attempt.ProviderOperationID
			}
			observation, err := CreateOrLoadTaskTerminalObservation(db, TaskTerminalObservationInput{
				OperationID: fixture.Operation.ID, TaskID: fixture.Task.ID, Outcome: tc.outcome, ActualQuota: func() int64 {
					if tc.outcome == "succeeded" {
						return 80
					}
					return 0
				}(), ReasonCode: "provider_terminal", RequestID: fixture.Operation.RequestID,
				ResolutionSource: tc.source, EvidenceID: evidenceID, EvidenceHash: tc.evidenceHash, EvidenceVersion: tc.version,
			})
			if tc.wantError {
				require.ErrorIs(t, err, ErrTaskRecoveryInvalidRecord)
				if observation != nil {
					assert.Zero(t, observation.ID)
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, evidenceID, observation.EvidenceID)
		})
	}
}

func TestTaskTerminalObservationApplyRechecksAttemptEvidence(t *testing.T) {
	db := openB2SubmissionSQLite(t)
	migrateB2SubmissionFixture(t, db)
	fixture := newTerminalObservationFixture(t, db, "evidence-apply-recheck", 100)
	observation := newTerminalObservation(t, db, fixture, 80, "succeeded")
	require.NoError(t, db.Exec("UPDATE task_submission_attempts SET provider_operation_id = ? WHERE id = ?", "changed-provider-id", fixture.Attempt.ID).Error)
	_, err := ApplyTaskTerminalObservation(db, observation.ID)
	require.ErrorIs(t, err, ErrTaskTerminalObservationManualReview)
	var stored TaskTerminalObservation
	var task Task
	var user User
	require.NoError(t, db.First(&stored, observation.ID).Error)
	require.NoError(t, db.First(&task, fixture.Task.ID).Error)
	require.NoError(t, db.First(&user, fixture.Input.UserID).Error)
	assert.Equal(t, TaskTerminalObservationManualReview, stored.State)
	assert.Equal(t, TaskStatus(TaskStatusSubmitted), task.Status)
	assert.Equal(t, 900, user.Quota)
	assert.Equal(t, 100, user.UsedQuota)
}

func TestTaskTerminalObservationExistingReceiptReplayValidation(t *testing.T) {
	t.Run("matching_success_receipt_projects_receipt_quota", func(t *testing.T) {
		db := openB2SubmissionSQLite(t)
		migrateB2SubmissionFixture(t, db)
		fixture := newTerminalObservationFixture(t, db, "receipt-replay-success", 100)
		observation := newTerminalObservation(t, db, fixture, 80, "succeeded")
		receipt, err := SettleTaskQuotaReservation(db, TaskQuotaSettlementInput{OperationID: fixture.Operation.ID, UserID: fixture.Input.UserID, TokenID: fixture.Input.TokenID, ChannelID: fixture.Input.ChannelID, ExpectedOperationVersion: fixture.Operation.LockVersion, ActualQuota: 80, ReasonCode: "provider_terminal", BillingContext: fixture.Input.BillingContext, TargetOperationStatus: TaskSubmissionOperationStatusSucceeded, ResolutionSource: TaskSubmissionResolutionSourceProviderVerified, RequireStatisticsEvidence: true, EvidenceID: fixture.Attempt.ProviderOperationID, EvidenceVersion: 1, RequestID: fixture.Operation.RequestID})
		require.NoError(t, err)
		result, err := ApplyTaskTerminalObservation(db, observation.ID)
		require.NoError(t, err)
		assert.Equal(t, receipt.ID, result.Receipt.ID)
		var task Task
		require.NoError(t, db.First(&task, fixture.Task.ID).Error)
		assert.Equal(t, int(receipt.Quota), task.Quota)
		assert.Equal(t, TaskStatus(TaskStatusSuccess), task.Status)
	})

	t.Run("mismatched_observation_actual_is_manual", func(t *testing.T) {
		db := openB2SubmissionSQLite(t)
		migrateB2SubmissionFixture(t, db)
		fixture := newTerminalObservationFixture(t, db, "receipt-replay-mismatch", 100)
		observation := newTerminalObservation(t, db, fixture, 80, "succeeded")
		_, err := SettleTaskQuotaReservation(db, TaskQuotaSettlementInput{OperationID: fixture.Operation.ID, UserID: fixture.Input.UserID, TokenID: fixture.Input.TokenID, ChannelID: fixture.Input.ChannelID, ExpectedOperationVersion: fixture.Operation.LockVersion, ActualQuota: 80, ReasonCode: "provider_terminal", BillingContext: fixture.Input.BillingContext, TargetOperationStatus: TaskSubmissionOperationStatusSucceeded, ResolutionSource: TaskSubmissionResolutionSourceProviderVerified, RequireStatisticsEvidence: true, EvidenceID: fixture.Attempt.ProviderOperationID, EvidenceVersion: 1, RequestID: fixture.Operation.RequestID})
		require.NoError(t, err)
		require.NoError(t, db.Exec("UPDATE task_terminal_observations SET actual_quota = ? WHERE id = ?", 81, observation.ID).Error)
		_, err = ApplyTaskTerminalObservation(db, observation.ID)
		require.ErrorIs(t, err, ErrTaskTerminalObservationManualReview)
		var task Task
		var stored TaskTerminalObservation
		require.NoError(t, db.First(&task, fixture.Task.ID).Error)
		require.NoError(t, db.First(&stored, observation.ID).Error)
		assert.Equal(t, TaskStatus(TaskStatusSubmitted), task.Status)
		assert.Equal(t, TaskTerminalObservationManualReview, stored.State)
	})

	t.Run("matching_failure_receipt_projects_zero", func(t *testing.T) {
		db := openB2SubmissionSQLite(t)
		migrateB2SubmissionFixture(t, db)
		fixture := newTerminalObservationFixture(t, db, "receipt-replay-failure", 100)
		observation := newTerminalObservation(t, db, fixture, 0, "failed")
		receipt, err := ReleaseTaskQuotaReservation(db, TaskQuotaReleaseInput{OperationID: fixture.Operation.ID, UserID: fixture.Input.UserID, TokenID: fixture.Input.TokenID, ChannelID: fixture.Input.ChannelID, ExpectedOperationVersion: fixture.Operation.LockVersion, ReasonCode: "provider_terminal", BillingContext: fixture.Input.BillingContext, TargetOperationStatus: TaskSubmissionOperationStatusFailed, ResolutionSource: TaskSubmissionResolutionSourceProviderVerified, RequireStatisticsEvidence: true, EvidenceID: fixture.Attempt.ProviderOperationID, EvidenceVersion: 1, RequestID: fixture.Operation.RequestID})
		require.NoError(t, err)
		result, err := ApplyTaskTerminalObservation(db, observation.ID)
		require.NoError(t, err)
		assert.Equal(t, receipt.ID, result.Receipt.ID)
		var task Task
		require.NoError(t, db.First(&task, fixture.Task.ID).Error)
		assert.Zero(t, task.Quota)
		assert.Equal(t, TaskStatus(TaskStatusFailure), task.Status)
	})
}
