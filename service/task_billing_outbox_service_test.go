package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/ForceMind/MyAPI/common"
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

func testOutboxLog(event *model.TaskBillingEvent, outbox *model.TaskBillingLogOutbox) model.Log {
	return model.Log{
		UserId:            outbox.Payload.UserID,
		CreatedAt:         outbox.Payload.CreatedAt,
		Type:              outbox.Payload.Type,
		Content:           outbox.Payload.Content,
		Username:          outbox.Payload.Username,
		TokenName:         outbox.Payload.TokenName,
		ModelName:         outbox.Payload.ModelName,
		Quota:             outbox.Payload.Quota,
		PromptTokens:      outbox.Payload.PromptTokens,
		CompletionTokens:  outbox.Payload.CompletionTokens,
		UseTime:           outbox.Payload.UseTime,
		IsStream:          outbox.Payload.IsStream,
		ChannelId:         outbox.Payload.ChannelID,
		TokenId:           outbox.Payload.TokenID,
		Group:             outbox.Payload.Group,
		Ip:                outbox.Payload.IP,
		RequestId:         event.RequestID,
		UpstreamRequestId: outbox.Payload.UpstreamRequestID,
		BillingEventID:    outbox.BillingEventID,
		Other:             outbox.Payload.Other,
	}
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
	assert.NotEmpty(t, event.RequestID)
	assert.Equal(t, event.RequestID, logRecord.RequestId)

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

	// Pre-insert the exact immutable projection to simulate an acknowledged
	// log write followed by an uncertain outbox state transition.
	preLog := testOutboxLog(event, outbox)
	require.NoError(t, model.CreateLog(db, &preLog))

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

func TestTaskBillingOutboxService_RejectsConflictingExistingProjection(t *testing.T) {
	db := setupTaskSubmissionTestDB(t)
	event, outbox := createTestOutbox(t, db, "conflict")

	conflictLog := testOutboxLog(event, outbox)
	conflictLog.Content = "conflicting projection"
	conflictLog.LogRowKey = "conflicting-row-key"
	conflictLog.BillingProjectionDigest = model.ComputeBillingProjectionDigest(&conflictLog)
	require.NoError(t, db.Exec(`INSERT INTO logs (
		user_id, created_at, type, content, username, token_name, model_name,
		quota, prompt_tokens, completion_tokens, use_time, is_stream, channel_id,
		token_id, `+"`group`"+`, ip, request_id, upstream_request_id,
		billing_event_id, billing_projection_digest, log_row_key, other
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		conflictLog.UserId, conflictLog.CreatedAt, conflictLog.Type, conflictLog.Content,
		conflictLog.Username, conflictLog.TokenName, conflictLog.ModelName, conflictLog.Quota,
		conflictLog.PromptTokens, conflictLog.CompletionTokens, conflictLog.UseTime, conflictLog.IsStream,
		conflictLog.ChannelId, conflictLog.TokenId, conflictLog.Group, conflictLog.Ip,
		conflictLog.RequestId, conflictLog.UpstreamRequestId, conflictLog.BillingEventID,
		conflictLog.BillingProjectionDigest, conflictLog.LogRowKey, conflictLog.Other,
	).Error)

	svc := NewTaskBillingOutboxService("worker-conflict")
	svc.LogDB = db
	delivered, err := svc.ProcessClaimableBatch(context.Background(), db)
	require.NoError(t, err)
	assert.Zero(t, delivered)

	var reloaded model.TaskBillingLogOutbox
	require.NoError(t, db.First(&reloaded, outbox.ID).Error)
	assert.Equal(t, model.TaskBillingLogOutboxStateQuarantined, reloaded.State)
	assert.Equal(t, ErrorCodeBillingProjectionConflict, reloaded.LastErrorCode)

	var logCount int64
	require.NoError(t, db.Model(&model.Log{}).Where("billing_event_id = ?", event.EventID).Count(&logCount).Error)
	assert.EqualValues(t, 1, logCount)
	var identity model.BillingLogProjectionIdentity
	require.NoError(t, db.Where("billing_event_id = ?", event.EventID).First(&identity).Error)
	assert.Equal(t, model.BillingLogProjectionIdentityStatusQuarantined, identity.Status)
	assert.Contains(t, identity.Reason, "conflict")
	visible, total, err := model.GetAllLogs(model.LogTypeUnknown, 0, 0, "", "", "", 0, 10, 0, "", conflictLog.RequestId, "")
	require.NoError(t, err)
	assert.Zero(t, total)
	assert.Empty(t, visible)
	stat, err := model.SumUsedQuota(model.LogTypeConsume, 0, 0, "", "", "", 0, "")
	require.NoError(t, err)
	assert.Zero(t, stat.Quota)
	remaining, err := model.CountOldLog(t.Context(), conflictLog.CreatedAt+1)
	require.NoError(t, err)
	assert.Zero(t, remaining)
	cleanup, err := model.DeleteOldLogBatchDetailed(t.Context(), conflictLog.CreatedAt+1, 10)
	require.NoError(t, err)
	assert.Zero(t, cleanup.Deleted)
	require.NoError(t, db.Model(&model.Log{}).Where("billing_event_id = ?", event.EventID).Count(&logCount).Error)
	assert.EqualValues(t, 1, logCount)
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

func TestTaskBillingOutboxService_LegacyEmptyBillingRequestIDUsesDeterministicEventIdentity(t *testing.T) {
	db := setupTaskSubmissionTestDB(t)
	event, outbox := createTestOutbox(t, db, "legacy-empty-request")
	event.Payload.RequestID = ""
	eventPayload, err := common.Marshal(event.Payload)
	require.NoError(t, err)
	require.NoError(t, db.Exec("UPDATE task_billing_events SET request_id = '', payload = ? WHERE id = ?", string(eventPayload), event.ID).Error)
	outbox.Payload.RequestID = ""
	outboxPayload, err := common.Marshal(outbox.Payload)
	require.NoError(t, err)
	require.NoError(t, db.Exec("UPDATE task_billing_log_outboxes SET payload = ? WHERE id = ?", string(outboxPayload), outbox.ID).Error)
	svc := NewTaskBillingOutboxService("legacy-request-worker")
	svc.LogDB = db
	delivered, err := svc.ProcessClaimableBatch(context.Background(), db)
	require.NoError(t, err)
	assert.Equal(t, 1, delivered)
	var logRecord model.Log
	require.NoError(t, db.Where("billing_event_id = ?", event.EventID).First(&logRecord).Error)
	digest := sha256.Sum256([]byte(event.EventID))
	assert.Equal(t, "billing_"+hex.EncodeToString(digest[:])[:48], logRecord.RequestId)
}

type serviceClickHouseSQLiteDialector struct {
	gorm.Dialector
}

func (serviceClickHouseSQLiteDialector) Name() string {
	return string(common.DatabaseTypeClickHouse)
}

func openTaskBillingClickHouseSQLiteFixture(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(serviceClickHouseSQLiteDialector{Dialector: sqlite.Open("file:" + t.Name() + "?mode=memory&cache=shared")}, &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.Exec(`CREATE TABLE logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER DEFAULT 0,
		created_at INTEGER DEFAULT 0,
		type INTEGER DEFAULT 0,
		content TEXT DEFAULT '',
		username TEXT DEFAULT '',
		token_name TEXT DEFAULT '',
		model_name TEXT DEFAULT '',
		quota INTEGER DEFAULT 0,
		prompt_tokens INTEGER DEFAULT 0,
		completion_tokens INTEGER DEFAULT 0,
		use_time INTEGER DEFAULT 0,
		is_stream INTEGER DEFAULT 0,
		channel_id INTEGER DEFAULT 0,
		token_id INTEGER DEFAULT 0,
		"group" TEXT DEFAULT '',
		ip TEXT DEFAULT '',
		request_id TEXT DEFAULT '',
		upstream_request_id TEXT DEFAULT '',
		billing_event_id TEXT DEFAULT '',
		billing_projection_digest TEXT DEFAULT '',
		log_row_key TEXT DEFAULT '',
		other TEXT DEFAULT ''
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE billing_log_projection_identities (
		billing_event_id TEXT,
		digest TEXT,
		canonical_version INTEGER,
		status TEXT DEFAULT 'canonical',
		reason TEXT DEFAULT '',
		updated_at INTEGER
	)`).Error)
	return db
}

func TestTaskBillingOutboxServiceClickHouseUsesIdentityWithoutScanningLogs(t *testing.T) {
	mainDB := setupTaskSubmissionTestDB(t)
	event, outbox := createTestOutbox(t, mainDB, "clickhouse-identity")
	logDB := openTaskBillingClickHouseSQLiteFixture(t)
	candidate := testOutboxLog(event, outbox)
	require.NoError(t, model.PrepareLogProjectionIdentity(&candidate))
	require.NoError(t, logDB.Exec(
		"INSERT INTO billing_log_projection_identities (billing_event_id, digest, canonical_version, status, reason, updated_at) VALUES (?, ?, ?, 'canonical', '', ?)",
		candidate.BillingEventID, candidate.BillingProjectionDigest, 1, 1,
	).Error)

	const queryGuard = "test:clickhouse-outbox-no-log-scan"
	require.NoError(t, logDB.Callback().Query().Before("gorm:query").Register(queryGuard, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Table == "logs" {
			tx.AddError(errors.New("outbox attempted to scan ClickHouse logs"))
		}
	}))

	svc := NewTaskBillingOutboxService("clickhouse-identity-worker")
	svc.LogDB = logDB
	delivered, err := svc.ProcessClaimableBatch(t.Context(), mainDB)
	require.NoError(t, err)
	assert.Equal(t, 1, delivered)
	require.NoError(t, logDB.Callback().Query().Remove(queryGuard))

	var logCount int64
	require.NoError(t, logDB.Table("logs").Where("billing_event_id = ?", event.EventID).Count(&logCount).Error)
	assert.Equal(t, int64(1), logCount)
	var reloaded model.TaskBillingLogOutbox
	require.NoError(t, mainDB.First(&reloaded, outbox.ID).Error)
	assert.Equal(t, model.TaskBillingLogOutboxStateDelivered, reloaded.State)
}

func TestTaskBillingOutboxServiceClickHouseIdentityConflictQuarantines(t *testing.T) {
	mainDB := setupTaskSubmissionTestDB(t)
	event, outbox := createTestOutbox(t, mainDB, "clickhouse-conflict")
	logDB := openTaskBillingClickHouseSQLiteFixture(t)
	candidate := testOutboxLog(event, outbox)
	require.NoError(t, model.PrepareLogProjectionIdentity(&candidate))
	conflictingDigest := "1" + strings.Repeat("f", 64)
	require.NotEqual(t, candidate.BillingProjectionDigest, conflictingDigest)
	require.NoError(t, logDB.Exec(
		"INSERT INTO billing_log_projection_identities (billing_event_id, digest, canonical_version, status, reason, updated_at) VALUES (?, ?, ?, 'canonical', '', ?)",
		candidate.BillingEventID, conflictingDigest, 1, 1,
	).Error)

	svc := NewTaskBillingOutboxService("clickhouse-conflict-worker")
	svc.LogDB = logDB
	delivered, err := svc.ProcessClaimableBatch(t.Context(), mainDB)
	require.NoError(t, err)
	assert.Zero(t, delivered)

	var logCount int64
	require.NoError(t, logDB.Table("logs").Count(&logCount).Error)
	assert.Zero(t, logCount)
	var reloaded model.TaskBillingLogOutbox
	require.NoError(t, mainDB.First(&reloaded, outbox.ID).Error)
	assert.Equal(t, model.TaskBillingLogOutboxStateQuarantined, reloaded.State)
	assert.Equal(t, ErrorCodeBillingProjectionConflict, reloaded.LastErrorCode)
}
