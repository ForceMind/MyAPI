package service

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func createTestOutbox(t *testing.T, db *gorm.DB, label string) (*model.TaskBillingEvent, *model.TaskBillingLogOutbox) {
	t.Helper()
	fixture := newTaskSubmissionTestFixture(t, db, label, 1000, 500)
	op := fixture.Operation

	won, err := model.TransitionTaskSubmissionOperation(db, op.ID, model.TaskSubmissionOperationTransition{
		From:            model.TaskSubmissionOperationStatusPrepared,
		To:              model.TaskSubmissionOperationStatusReserved,
		ExpectedVersion: op.LockVersion,
	})
	require.NoError(t, err)
	require.True(t, won)

	event, err := model.CreateOrLoadTaskBillingEvent(db, &model.TaskBillingEvent{
		OperationID:   &op.ID,
		EventType:     model.TaskBillingEventTypeReserve,
		UserID:        op.UserID,
		TokenID:       op.TokenID,
		ChannelID:     fixture.Attempt.ChannelID,
		BillingSource: "wallet",
		QuotaDelta:    -100,
	})
	require.NoError(t, err)

	candidate, err := model.NewTaskBillingLogOutbox(event, model.TaskBillingLogPayload{
		Content:          "task billing log " + label,
		ModelName:        "test-model",
		Group:            "default",
		PromptTokens:     10,
		CompletionTokens: 20,
	})
	require.NoError(t, err)

	outbox, err := model.CreateOrLoadTaskBillingLogOutbox(db, candidate)
	require.NoError(t, err)
	return event, outbox
}

func TestTaskBillingOutboxService_NormalDelivery(t *testing.T) {
	db := setupTaskSubmissionTestDB(t)
	event, outbox := createTestOutbox(t, db, "normal")

	svc := NewTaskBillingOutboxService("worker-normal")
	svc.LogDB = db

	delivered, err := svc.ProcessClaimableBatch(context.Background(), db)
	require.NoError(t, err)
	assert.Equal(t, 1, delivered)

	// Verify log entry created in logs table
	var logRecord model.Log
	err = db.Where("billing_event_id = ?", event.EventID).First(&logRecord).Error
	require.NoError(t, err)
	assert.Equal(t, event.UserID, logRecord.UserId)
	assert.Equal(t, 100, logRecord.Quota)
	assert.Equal(t, "test-model", logRecord.ModelName)
	assert.Equal(t, "task billing log normal", logRecord.Content)
	assert.Equal(t, event.EventID, logRecord.BillingEventID)

	// Verify outbox entry transitioned to delivered
	var reloaded model.TaskBillingLogOutbox
	require.NoError(t, db.First(&reloaded, outbox.ID).Error)
	assert.Equal(t, model.TaskBillingLogOutboxStateDelivered, reloaded.State)
	require.NotNil(t, reloaded.DeliveredAt)
	assert.Greater(t, *reloaded.DeliveredAt, int64(0))
	assert.Empty(t, reloaded.ClaimedBy)
	assert.Zero(t, reloaded.ClaimedUntil)
	assert.Equal(t, 1, reloaded.AttemptCount)
}

func TestTaskBillingOutboxService_IdempotentDeduplication(t *testing.T) {
	db := setupTaskSubmissionTestDB(t)
	event, outbox := createTestOutbox(t, db, "idempotent")

	// Pre-insert log entry to simulate at-least-once replay
	preLog := model.Log{
		UserId:         event.UserID,
		BillingEventID: event.EventID,
		Content:        "pre-existing log entry",
		ModelName:      "test-model",
		RequestId:      "req-pre-existing",
		CreatedAt:      time.Now().Unix(),
	}
	require.NoError(t, db.Create(&preLog).Error)

	svc := NewTaskBillingOutboxService("worker-idempotent")
	svc.LogDB = db

	delivered, err := svc.ProcessClaimableBatch(context.Background(), db)
	require.NoError(t, err)
	assert.Equal(t, 1, delivered)

	// Verify only 1 log entry exists (no duplicate inserted)
	var logCount int64
	require.NoError(t, db.Model(&model.Log{}).Where("billing_event_id = ?", event.EventID).Count(&logCount).Error)
	assert.Equal(t, int64(1), logCount)

	// Verify outbox entry is marked delivered
	var reloaded model.TaskBillingLogOutbox
	require.NoError(t, db.First(&reloaded, outbox.ID).Error)
	assert.Equal(t, model.TaskBillingLogOutboxStateDelivered, reloaded.State)
	require.NotNil(t, reloaded.DeliveredAt)
	assert.Greater(t, *reloaded.DeliveredAt, int64(0))
}

func TestTaskBillingOutboxService_FailureRetry(t *testing.T) {
	db := setupTaskSubmissionTestDB(t)
	event, outbox := createTestOutbox(t, db, "retry")

	// Broken LogDB simulates delivery failure
	brokenDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	brokenSQL, err := brokenDB.DB()
	require.NoError(t, err)
	require.NoError(t, brokenSQL.Close())

	svc := NewTaskBillingOutboxService("worker-retry")
	svc.LogDB = brokenDB

	delivered, err := svc.ProcessClaimableBatch(context.Background(), db)
	require.NoError(t, err)
	assert.Equal(t, 0, delivered)

	// Verify no log inserted into the valid db
	var logCount int64
	require.NoError(t, db.Model(&model.Log{}).Where("billing_event_id = ?", event.EventID).Count(&logCount).Error)
	assert.Equal(t, int64(0), logCount)

	// Verify outbox state transitioned to retryable with increased next_attempt_at
	var reloaded model.TaskBillingLogOutbox
	require.NoError(t, db.First(&reloaded, outbox.ID).Error)
	assert.Equal(t, model.TaskBillingLogOutboxStateRetryable, reloaded.State)
	assert.Greater(t, reloaded.NextAttemptAt, int64(0))
	assert.Equal(t, ErrorCodeLogDeliveryFailed, reloaded.LastErrorCode)
	assert.Nil(t, reloaded.DeliveredAt)
	assert.Equal(t, 1, reloaded.AttemptCount)
	assert.Empty(t, reloaded.ClaimedBy)
	assert.Zero(t, reloaded.ClaimedUntil)
}

func TestTaskBillingOutboxService_ReclaimExpiredLease(t *testing.T) {
	db := setupTaskSubmissionTestDB(t)
	event, outbox := createTestOutbox(t, db, "reclaim")

	// Worker 1 claims the outbox
	won, err := model.TransitionTaskBillingLogOutbox(db, outbox.ID, model.TaskBillingLogOutboxTransition{
		From:            model.TaskBillingLogOutboxStatePending,
		To:              model.TaskBillingLogOutboxStateClaimed,
		ExpectedVersion: outbox.LockVersion,
		Lease: model.TaskRecoveryProcessingLease{
			WorkerID:     "crashed-worker-1",
			LeaseSeconds: 60,
		},
	})
	require.NoError(t, err)
	require.True(t, won)

	// Simulate worker 1 crash and lease expiration
	require.NoError(t, db.Table("task_billing_log_outboxes").Where("id = ?", outbox.ID).Update("claimed_until", 1).Error)

	// Worker 2 reclaims and delivers
	svc := NewTaskBillingOutboxService("active-worker-2")
	svc.LogDB = db

	delivered, err := svc.ProcessClaimableBatch(context.Background(), db)
	require.NoError(t, err)
	assert.Equal(t, 1, delivered)

	// Verify log entry exists
	var logCount int64
	require.NoError(t, db.Model(&model.Log{}).Where("billing_event_id = ?", event.EventID).Count(&logCount).Error)
	assert.Equal(t, int64(1), logCount)

	// Verify delivered state and updated attempt count
	var reloaded model.TaskBillingLogOutbox
	require.NoError(t, db.First(&reloaded, outbox.ID).Error)
	assert.Equal(t, model.TaskBillingLogOutboxStateDelivered, reloaded.State)
	require.NotNil(t, reloaded.DeliveredAt)
	assert.Equal(t, 2, reloaded.AttemptCount)
}

func TestTaskBillingOutboxService_MaxAttemptsExceeded(t *testing.T) {
	db := setupTaskSubmissionTestDB(t)
	_, outbox := createTestOutbox(t, db, "max-attempts")

	// Pre-set attempt_count so the next attempt will reach max attempts
	require.NoError(t, db.Table("task_billing_log_outboxes").Where("id = ?", outbox.ID).Update("attempt_count", 2).Error)

	brokenDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	brokenSQL, err := brokenDB.DB()
	require.NoError(t, err)
	require.NoError(t, brokenSQL.Close())

	svc := &TaskBillingOutboxService{
		WorkerID:    "worker-max",
		LogDB:       brokenDB,
		MaxAttempts: 3,
	}

	delivered, err := svc.ProcessClaimableBatch(context.Background(), db)
	require.NoError(t, err)
	assert.Equal(t, 0, delivered)

	var reloaded model.TaskBillingLogOutbox
	require.NoError(t, db.First(&reloaded, outbox.ID).Error)
	assert.Equal(t, model.TaskBillingLogOutboxStateRetryable, reloaded.State)
	assert.Equal(t, ErrorCodeMaxAttemptsExceeded, reloaded.LastErrorCode)
	assert.Equal(t, 3, reloaded.AttemptCount)
}

func TestTaskBillingOutboxService_ConcurrentWorkers(t *testing.T) {
	db := setupTaskSubmissionTestDB(t)

	// Create 5 distinct outboxes
	for i := 1; i <= 5; i++ {
		createTestOutbox(t, db, fmt.Sprintf("concurrent-outbox-%d", i))
	}

	const workerCount = 3
	deliveredCounts := make([]int, workerCount)
	var wg sync.WaitGroup

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		idx := i
		go func() {
			defer wg.Done()
			svc := NewTaskBillingOutboxService(fmt.Sprintf("worker-%d", idx))
			svc.LogDB = db
			count, err := svc.ProcessClaimableBatch(context.Background(), db)
			if err == nil {
				deliveredCounts[idx] = count
			}
		}()
	}
	wg.Wait()

	totalDelivered := 0
	for _, c := range deliveredCounts {
		totalDelivered += c
	}
	assert.Equal(t, 5, totalDelivered)

	// Verify all 5 are delivered in outbox table
	var deliveredOutboxes []model.TaskBillingLogOutbox
	require.NoError(t, db.Where("state = ?", model.TaskBillingLogOutboxStateDelivered).Find(&deliveredOutboxes).Error)
	assert.Len(t, deliveredOutboxes, 5)

	// Verify exactly 5 logs exist
	var logCount int64
	require.NoError(t, db.Model(&model.Log{}).Count(&logCount).Error)
	assert.Equal(t, int64(5), logCount)
}

