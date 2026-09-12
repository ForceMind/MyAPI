package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	taskdto "github.com/ForceMind/MyAPI/dto"
	"github.com/ForceMind/MyAPI/model"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupBridgeTestDB(t *testing.T) *gorm.DB {
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
		&model.Log{},
	)
	require.NoError(t, err)

	ch := model.Channel{
		Id:   101,
		Type: 1,
		Name: "test-channel",
		Key:  "test-key",
	}
	require.NoError(t, db.Create(&ch).Error)

	origDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = origDB })

	return db
}

type bridgeTestFixture struct {
	User           model.User
	Token          model.Token
	Operation      model.TaskSubmissionOperation
	Attempt        model.TaskSubmissionAttempt
	Task           model.Task
	ReserveReceipt *model.QuotaMutationReceipt
}

func durableEvidenceForTest(task *model.Task, hashCharacter string) DurableTerminalEvidence {
	if task == nil || model.DB == nil {
		return DurableTerminalEvidence{}
	}
	operation, err := model.GetTaskSubmissionOperationByPublicID(model.DB, task.TaskID)
	if err != nil || operation == nil {
		return DurableTerminalEvidence{}
	}
	attempt, err := model.FindAttemptByOperationID(model.DB, operation.ID)
	if err != nil || attempt == nil {
		return DurableTerminalEvidence{}
	}
	return DurableTerminalEvidence{ID: attempt.ProviderOperationID, Hash: strings.Repeat(hashCharacter, 64), Version: 1}
}

func settleDurableForTest(ctx context.Context, task *model.Task, actual int, reason string, clamps ...*common.QuotaClamp) (bool, error) {
	return DurableSettleTaskOnCompleteWithEvidence(ctx, task, actual, reason, durableEvidenceForTest(task, "a"), clamps...)
}

func releaseDurableForTest(ctx context.Context, task *model.Task, reason string) (bool, error) {
	return DurableReleaseTaskOnFailureWithEvidence(ctx, task, reason, durableEvidenceForTest(task, "b"))
}

func newBridgeTestFixture(t *testing.T, db *gorm.DB, label string, userQuota, tokenQuota, reservedQuota int, billingContexts ...model.TaskBillingContext) bridgeTestFixture {
	t.Helper()
	billingContext := model.TaskBillingContext{Version: model.TaskBillingContextVersion, Complete: true, ModelPrice: 1, ModelRatio: 1, GroupRatio: 1, OriginModelName: "test-model"}
	if len(billingContexts) > 0 {
		billingContext = billingContexts[0]
	}
	digest := sha256.Sum256([]byte(t.Name() + ":" + label))
	name := fmt.Sprintf("brg-%x", digest[:8])

	user := model.User{
		Username: name,
		AffCode:  name,
		Password: "fixture-password",
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
	fingerprint := model.FingerprintTaskSubmissionRequest([]byte(`{"prompt":"bridge-test"}`))

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

	// T1 Reserve
	reserveReceipt, err := model.ReserveTaskQuota(db, model.TaskQuotaReservationInput{
		OperationID:              intent.Operation.ID,
		UserID:                   user.Id,
		TokenID:                  token.Id,
		ChannelID:                intent.Attempt.ChannelID,
		ExpectedOperationVersion: intent.Operation.LockVersion,
		Quota:                    int64(reservedQuota),
		ApplyStatistics:          true,
		BillingSource:            "wallet",
		BillingContext:           billingContext,
	})
	require.NoError(t, err)

	// T2 Outbound
	won, err := model.StartTaskSubmissionDispatch(db, intent.Operation.ID, model.TaskSubmissionDispatchTransition{
		ExpectedOperationVersion: reserveReceipt.OperationVersionAfter,
		ExpectedAttemptVersion:   intent.Attempt.LockVersion,
	})
	require.NoError(t, err)
	require.True(t, won)

	var op model.TaskSubmissionOperation
	require.NoError(t, db.Where("id = ?", intent.Operation.ID).First(&op).Error)
	var att model.TaskSubmissionAttempt
	require.NoError(t, db.Where("id = ?", intent.Attempt.ID).First(&att).Error)

	// Create formal task
	task := model.Task{
		TaskID:    op.PublicID,
		UserId:    user.Id,
		ChannelId: att.ChannelID,
		Status:    model.TaskStatusSubmitted,
		Quota:     reservedQuota,
		PrivateData: model.TaskPrivateData{
			TokenId:        token.Id,
			BillingContext: &billingContext,
		},
	}
	require.NoError(t, db.Create(&task).Error)

	// T3 Outcome: Transition to Accepted
	won, err = model.TransitionTaskSubmissionAttempt(db, att.ID, model.TaskSubmissionAttemptTransition{
		From:                model.TaskSubmissionAttemptStatusDispatching,
		To:                  model.TaskSubmissionAttemptStatusAccepted,
		ExpectedVersion:     att.LockVersion,
		ProviderOperationID: "prov-" + op.PublicID,
	})
	require.NoError(t, err)
	require.True(t, won)

	won, err = model.TransitionTaskSubmissionOperation(db, op.ID, model.TaskSubmissionOperationTransition{
		From:            model.TaskSubmissionOperationStatusDispatching,
		To:              model.TaskSubmissionOperationStatusAccepted,
		ExpectedVersion: op.LockVersion,
		TaskID:          &task.ID,
	})
	require.NoError(t, err)
	require.True(t, won)

	require.NoError(t, db.Where("id = ?", op.ID).First(&op).Error)
	require.NoError(t, db.Where("id = ?", att.ID).First(&att).Error)

	return bridgeTestFixture{
		User:           user,
		Token:          token,
		Operation:      op,
		Attempt:        att,
		Task:           task,
		ReserveReceipt: reserveReceipt,
	}
}

func TestDurableSettleTaskOnComplete_SurplusRefund(t *testing.T) {
	db := setupBridgeTestDB(t)
	ctx := context.Background()

	// Initial user quota: 1000, reserved: 200. After reserve: 800.
	fixture := newBridgeTestFixture(t, db, "surplus", 1000, 1000, 200)

	// Settle with actual quota = 150 (surplus refund: 50)
	handled, err := settleDurableForTest(ctx, &fixture.Task, 150, "test settlement surplus")
	require.NoError(t, err)
	assert.True(t, handled)

	// Verify task quota updated
	var updatedTask model.Task
	require.NoError(t, db.Where("id = ?", fixture.Task.ID).First(&updatedTask).Error)
	assert.Equal(t, 150, updatedTask.Quota)

	// Verify operation status transitioned to succeeded
	var updatedOp model.TaskSubmissionOperation
	require.NoError(t, db.Where("id = ?", fixture.Operation.ID).First(&updatedOp).Error)
	assert.Equal(t, model.TaskSubmissionOperationStatusSucceeded, updatedOp.Status)

	// Verify user balance refunded surplus: 800 + 50 = 850
	var updatedUser model.User
	require.NoError(t, db.Where("id = ?", fixture.User.Id).First(&updatedUser).Error)
	assert.Equal(t, 850, updatedUser.Quota)
	assert.Equal(t, 150, updatedUser.UsedQuota)
	assert.Equal(t, 1, updatedUser.RequestCount)
	var updatedChannel model.Channel
	require.NoError(t, db.First(&updatedChannel, fixture.Task.ChannelId).Error)
	assert.Equal(t, int64(150), updatedChannel.UsedQuota)
	var observation model.TaskTerminalObservation
	require.NoError(t, db.Where("operation_id = ?", fixture.Operation.ID).First(&observation).Error)
	assert.Equal(t, model.TaskTerminalObservationApplied, observation.State)
	assert.True(t, fixture.ReserveReceipt.StatisticsApplied)
	assert.Equal(t, 1, fixture.ReserveReceipt.StatisticsVersion)

	// Verify outbox record created
	var outboxes []model.TaskBillingLogOutbox
	require.NoError(t, db.Find(&outboxes).Error)
	assert.Len(t, outboxes, 2)
	assert.Equal(t, model.TaskBillingLogOutboxStatePending, outboxes[0].State)
}

func TestDurableSettleTaskOnComplete_DeficitCharge(t *testing.T) {
	db := setupBridgeTestDB(t)
	ctx := context.Background()

	// Initial user quota: 1000, reserved: 200. After reserve: 800.
	fixture := newBridgeTestFixture(t, db, "deficit", 1000, 1000, 200)

	// Settle with actual quota = 250 (deficit charge: 50)
	handled, err := settleDurableForTest(ctx, &fixture.Task, 250, "test settlement deficit")
	require.NoError(t, err)
	assert.True(t, handled)

	// Verify task quota updated
	var updatedTask model.Task
	require.NoError(t, db.Where("id = ?", fixture.Task.ID).First(&updatedTask).Error)
	assert.Equal(t, 250, updatedTask.Quota)

	// Verify operation status transitioned to succeeded
	var updatedOp model.TaskSubmissionOperation
	require.NoError(t, db.Where("id = ?", fixture.Operation.ID).First(&updatedOp).Error)
	assert.Equal(t, model.TaskSubmissionOperationStatusSucceeded, updatedOp.Status)

	// Verify user balance charged deficit: 800 - 50 = 750
	var updatedUser model.User
	require.NoError(t, db.Where("id = ?", fixture.User.Id).First(&updatedUser).Error)
	assert.Equal(t, 750, updatedUser.Quota)
}

func TestDurableReleaseTaskOnFailure_Success(t *testing.T) {
	db := setupBridgeTestDB(t)
	ctx := context.Background()

	// Initial user quota: 1000, reserved: 200. After reserve: 800.
	fixture := newBridgeTestFixture(t, db, "release", 1000, 1000, 200)

	handled, err := releaseDurableForTest(ctx, &fixture.Task, "provider execution failed")
	require.NoError(t, err)
	assert.True(t, handled)

	// Verify task quota cleared
	var updatedTask model.Task
	require.NoError(t, db.Where("id = ?", fixture.Task.ID).First(&updatedTask).Error)
	assert.Equal(t, 0, updatedTask.Quota)

	// Verify operation status transitioned to failed
	var updatedOp model.TaskSubmissionOperation
	require.NoError(t, db.Where("id = ?", fixture.Operation.ID).First(&updatedOp).Error)
	assert.Equal(t, model.TaskSubmissionOperationStatusFailed, updatedOp.Status)

	// Verify user balance fully refunded: 800 + 200 = 1000
	var updatedUser model.User
	require.NoError(t, db.Where("id = ?", fixture.User.Id).First(&updatedUser).Error)
	assert.Equal(t, 1000, updatedUser.Quota)

	// Verify outbox record created
	var outboxes []model.TaskBillingLogOutbox
	require.NoError(t, db.Find(&outboxes).Error)
	assert.Len(t, outboxes, 2)
}

func TestDurablePollingBridge_NotDurableTask(t *testing.T) {
	_ = setupBridgeTestDB(t)
	ctx := context.Background()

	// Normal task not created through durable submission pipeline
	legacyTask := model.Task{
		TaskID:    "legacy-task-xyz-12345",
		UserId:    1,
		ChannelId: 1,
		Quota:     100,
	}

	settleHandled, err := DurableSettleTaskOnComplete(ctx, &legacyTask, 100, "reason")
	require.NoError(t, err)
	assert.False(t, settleHandled)

	releaseHandled, err := DurableReleaseTaskOnFailure(ctx, &legacyTask, "reason")
	require.NoError(t, err)
	assert.False(t, releaseHandled)
}

func TestDurablePollingBridge_Idempotency(t *testing.T) {
	db := setupBridgeTestDB(t)
	ctx := context.Background()

	fixture := newBridgeTestFixture(t, db, "idempotent", 1000, 1000, 200)

	originalTask := fixture.Task
	// First settlement
	handled1, err1 := settleDurableForTest(ctx, &fixture.Task, 180, "settle 1")
	require.NoError(t, err1)
	assert.True(t, handled1)

	// Second settlement should be idempotent no-op
	handled2, err2 := settleDurableForTest(ctx, &originalTask, 180, "settle 2")
	require.NoError(t, err2)
	assert.True(t, handled2)

	// User balance should reflect single settlement (800 + 20 = 820)
	var updatedUser model.User
	require.NoError(t, db.Where("id = ?", fixture.User.Id).First(&updatedUser).Error)
	assert.Equal(t, 820, updatedUser.Quota)
}

func TestDurablePollingBridge_MutualExclusion(t *testing.T) {
	db := setupBridgeTestDB(t)
	ctx := context.Background()

	fixture := newBridgeTestFixture(t, db, "mutex", 1000, 1000, 200)

	// Settle first
	handled, err := settleDurableForTest(ctx, &fixture.Task, 200, "settle mutex")
	require.NoError(t, err)
	assert.True(t, handled)

	// Releasing an already settled task must be rejected
	handledRelease, errRelease := releaseDurableForTest(ctx, &fixture.Task, "try refund after settle")
	assert.True(t, handledRelease)
	assert.ErrorIs(t, errRelease, model.ErrTaskTerminalObservationConflict)
	var observation model.TaskTerminalObservation
	require.NoError(t, db.Where("operation_id = ?", fixture.Operation.ID).First(&observation).Error)
	assert.Equal(t, model.TaskTerminalObservationApplied, observation.State)
}

func TestTaskPolling_RefundHookIntegration(t *testing.T) {
	db := setupBridgeTestDB(t)
	ctx := context.Background()

	fixture := newBridgeTestFixture(t, db, "refund-hook", 1000, 1000, 200)

	// Call central RefundTaskQuota, which now delegates to DurableReleaseTaskOnFailure
	ok := RefundTaskQuota(ctx, &fixture.Task, "upstream timeout refund hook")
	assert.False(t, ok)

	// Verify task quota is zeroed
	var updatedTask model.Task
	require.NoError(t, db.Where("id = ?", fixture.Task.ID).First(&updatedTask).Error)
	assert.Equal(t, 200, updatedTask.Quota)

	// Verify full refund to user
	var updatedUser model.User
	require.NoError(t, db.Where("id = ?", fixture.User.Id).First(&updatedUser).Error)
	assert.Equal(t, 800, updatedUser.Quota)

	// Verify operation status is failed
	var updatedOp model.TaskSubmissionOperation
	require.NoError(t, db.Where("id = ?", fixture.Operation.ID).First(&updatedOp).Error)
	assert.Equal(t, model.TaskSubmissionOperationStatusAccepted, updatedOp.Status)
	var observation model.TaskTerminalObservation
	require.NoError(t, db.Where("operation_id = ?", fixture.Operation.ID).First(&observation).Error)
	assert.Equal(t, model.TaskTerminalObservationManualReview, observation.State)
}

func TestTaskPolling_SettleHookIntegration(t *testing.T) {
	db := setupBridgeTestDB(t)
	ctx := context.Background()

	fixture := newBridgeTestFixture(t, db, "settle-hook", 1000, 1000, 200)

	// Call settleTaskBillingOnComplete with an adaptor returning 160
	mockAdaptor := &mockBridgeAdaptor{adjustQuota: 160}
	taskResult := &relaycommon.TaskInfo{
		TaskID: fixture.Task.TaskID,
		Status: string(model.TaskStatusSuccess),
	}

	settleTaskBillingOnComplete(ctx, mockAdaptor, &fixture.Task, taskResult)

	// Verify task quota is 160
	var updatedTask model.Task
	require.NoError(t, db.Where("id = ?", fixture.Task.ID).First(&updatedTask).Error)
	assert.Equal(t, 200, updatedTask.Quota)

	// Verify surplus refund: 800 + 40 = 840
	var updatedUser model.User
	require.NoError(t, db.Where("id = ?", fixture.User.Id).First(&updatedUser).Error)
	assert.Equal(t, 800, updatedUser.Quota)

	// Verify operation status is succeeded
	var updatedOp model.TaskSubmissionOperation
	require.NoError(t, db.Where("id = ?", fixture.Operation.ID).First(&updatedOp).Error)
	assert.Equal(t, model.TaskSubmissionOperationStatusAccepted, updatedOp.Status)
	var observation model.TaskTerminalObservation
	require.NoError(t, db.Where("operation_id = ?", fixture.Operation.ID).First(&observation).Error)
	assert.Equal(t, model.TaskTerminalObservationManualReview, observation.State)
}

type mockBridgeAdaptor struct {
	adjustQuota int
}

func (m *mockBridgeAdaptor) Init(info *relaycommon.RelayInfo) {}
func (m *mockBridgeAdaptor) FetchTask(baseURL string, key string, body map[string]any, proxy string) (*http.Response, error) {
	return nil, nil
}
func (m *mockBridgeAdaptor) ParseTaskResult(body []byte) (*relaycommon.TaskInfo, error) {
	return nil, nil
}
func (m *mockBridgeAdaptor) AdjustBillingOnComplete(task *model.Task, taskResult *relaycommon.TaskInfo) int {
	return m.adjustQuota
}

func TestDurablePollingBridge_QuotaSaturationAudit(t *testing.T) {
	db := setupBridgeTestDB(t)
	ctx := context.Background()

	fixture := newBridgeTestFixture(t, db, "clamp-audit", 1000, 1000, 200)

	clamp := &common.QuotaClamp{
		Op:       "test_conversion",
		Kind:     "upper",
		Original: 1e12,
		Clamped:  common.MaxQuota,
	}

	handled, err := settleDurableForTest(ctx, &fixture.Task, 200, "settle with clamp", clamp)
	require.ErrorIs(t, err, model.ErrTaskTerminalObservationManualReview)
	assert.True(t, handled)
	var observation model.TaskTerminalObservation
	require.NoError(t, db.Where("operation_id = ?", fixture.Operation.ID).First(&observation).Error)
	assert.Equal(t, model.TaskTerminalObservationManualReview, observation.State)
	var count int64
	require.NoError(t, db.Model(&model.TaskBillingLogOutbox{}).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestDurableTerminalObservationSanitizesReasonURLAndFreezesRequestID(t *testing.T) {
	db := setupBridgeTestDB(t)
	fixture := newBridgeTestFixture(t, db, "sanitize-terminal", 1000, 1000, 100)
	fixture.Task.PrivateData.ResultURL = "https://media.example.com/result/video.mp4?signature=secret#fragment"
	handled, err := settleDurableForTest(context.Background(), &fixture.Task, 100, "Provider raw text: signed secret")
	require.NoError(t, err)
	require.True(t, handled)
	var observation model.TaskTerminalObservation
	require.NoError(t, db.Where("operation_id = ?", fixture.Operation.ID).First(&observation).Error)
	assert.Equal(t, "task_poll_settle", observation.ReasonCode)
	assert.Equal(t, "https://media.example.com/result/video.mp4", observation.ResultURL)
	assert.Equal(t, fixture.Operation.RequestID, observation.RequestID)
	assert.Equal(t, observation.CreatedAt+model.TaskSubmissionTerminalRetentionSeconds, observation.RetentionUntil)
	var receipts []model.QuotaMutationReceipt
	require.NoError(t, db.Where("operation_id = ?", fixture.Operation.ID).Order("id").Find(&receipts).Error)
	require.Len(t, receipts, 2)
	for _, receipt := range receipts {
		assert.NotEmpty(t, receipt.RequestID)
		assert.Equal(t, fixture.Operation.RequestID, receipt.RequestID)
		var event model.TaskBillingEvent
		require.NoError(t, db.Where("event_id = ?", receipt.BillingEventID).First(&event).Error)
		assert.Equal(t, receipt.RequestID, event.RequestID)
		assert.True(t, receipt.StatisticsApplied)
		assert.Equal(t, 1, receipt.StatisticsVersion)
		assert.True(t, event.StatisticsApplied)
		assert.Equal(t, 1, event.StatisticsVersion)
		var outbox model.TaskBillingLogOutbox
		require.NoError(t, db.Where("billing_event_id = ?", event.EventID).First(&outbox).Error)
		assert.Equal(t, event.RequestID, outbox.Payload.RequestID)
		assert.NotContains(t, outbox.Payload.Content, "signed secret")
	}
}

func TestDurableMutationOutboxContractReserveExactAndRefund(t *testing.T) {
	t.Run("actual equals reserve emits consume and zero terminal", func(t *testing.T) {
		db := setupBridgeTestDB(t)
		fixture := newBridgeTestFixture(t, db, "outbox-exact", 1000, 1000, 100)
		_, err := settleDurableForTest(context.Background(), &fixture.Task, 100, "provider_terminal")
		require.NoError(t, err)
		var outboxes []model.TaskBillingLogOutbox
		require.NoError(t, db.Order("id").Find(&outboxes).Error)
		require.Len(t, outboxes, 2)
		assert.Equal(t, model.LogTypeConsume, outboxes[0].Payload.Type)
		assert.Equal(t, 100, outboxes[0].Payload.Quota)
		assert.Equal(t, model.LogTypeSystem, outboxes[1].Payload.Type)
		assert.Zero(t, outboxes[1].Payload.Quota)
	})
	t.Run("failure emits reserve consume and refund", func(t *testing.T) {
		db := setupBridgeTestDB(t)
		fixture := newBridgeTestFixture(t, db, "outbox-refund", 1000, 1000, 100)
		_, err := releaseDurableForTest(context.Background(), &fixture.Task, "provider_terminal")
		require.NoError(t, err)
		var outboxes []model.TaskBillingLogOutbox
		require.NoError(t, db.Order("id").Find(&outboxes).Error)
		require.Len(t, outboxes, 2)
		assert.Equal(t, model.LogTypeConsume, outboxes[0].Payload.Type)
		assert.Equal(t, model.LogTypeRefund, outboxes[1].Payload.Type)
		assert.Equal(t, 100, outboxes[1].Payload.Quota)
	})
}

func TestDurablePendingObservationPollingWorkerCompetition(t *testing.T) {
	db := setupBridgeTestDB(t)
	fixture := newBridgeTestFixture(t, db, "poll-worker-race", 1000, 1000, 100)
	_, err := model.CreateOrLoadTaskTerminalObservation(db, model.TaskTerminalObservationInput{OperationID: fixture.Operation.ID, TaskID: fixture.Task.ID, Outcome: "succeeded", ActualQuota: 80, ReasonCode: "provider_terminal", RequestID: fixture.Operation.RequestID, ResolutionSource: model.TaskSubmissionResolutionSourceProviderVerified, EvidenceID: fixture.Attempt.ProviderOperationID, EvidenceHash: strings.Repeat("a", 64), EvidenceVersion: 1})
	require.NoError(t, err)
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		_, err := settleDurableForTest(context.Background(), &fixture.Task, 80, "provider_terminal")
		errs <- err
	}()
	go func() {
		defer wg.Done()
		<-start
		_, err := NewTaskRecoveryWorker("race-worker").RecoverTerminalObservations(context.Background(), db)
		errs <- err
	}()
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	var user model.User
	var channel model.Channel
	var observation model.TaskTerminalObservation
	require.NoError(t, db.First(&user, fixture.User.Id).Error)
	require.NoError(t, db.First(&channel, fixture.Task.ChannelId).Error)
	require.NoError(t, db.Where("operation_id = ?", fixture.Operation.ID).First(&observation).Error)
	assert.Equal(t, 80, user.UsedQuota)
	assert.Equal(t, 1, user.RequestCount)
	assert.Equal(t, int64(80), channel.UsedQuota)
	assert.Equal(t, model.TaskTerminalObservationApplied, observation.State)
	var receiptCount, outboxCount int64
	require.NoError(t, db.Model(&model.QuotaMutationReceipt{}).Where("operation_id = ?", fixture.Operation.ID).Count(&receiptCount).Error)
	require.NoError(t, db.Model(&model.TaskBillingLogOutbox{}).Count(&outboxCount).Error)
	assert.Equal(t, int64(2), receiptCount)
	assert.Equal(t, int64(2), outboxCount)
}

func TestTerminalSanitizers(t *testing.T) {
	for _, tc := range []struct{ name, input, want string }{
		{"strip_query_fragment", "https://cdn.example.com/a/video.mp4?token=secret#part", "https://cdn.example.com/a/video.mp4"},
		{"reject_userinfo", "https://user:pass@cdn.example.com/a.mp4", ""},
		{"reject_non_http", "javascript:alert(1)", ""},
		{"reject_relative", "/private/result.mp4", ""},
	} {
		t.Run(tc.name, func(t *testing.T) { assert.Equal(t, tc.want, sanitizeTerminalResultURL(tc.input)) })
	}
	assert.Equal(t, "provider_terminal", sanitizeReasonCode("Provider returned raw secret text", "provider_terminal"))
	assert.Equal(t, "provider.failed", sanitizeReasonCode("PROVIDER.FAILED", "fallback"))
}

type terminalProjectionPollingAdaptor struct {
	body  []byte
	info  *relaycommon.TaskInfo
	quota int
}

func (a *terminalProjectionPollingAdaptor) Init(*relaycommon.RelayInfo) {}
func (a *terminalProjectionPollingAdaptor) FetchTask(string, string, map[string]any, string) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(a.body))}, nil
}
func (a *terminalProjectionPollingAdaptor) ParseTaskResult([]byte) (*relaycommon.TaskInfo, error) {
	copyInfo := *a.info
	return &copyInfo, nil
}
func (a *terminalProjectionPollingAdaptor) AdjustBillingOnComplete(*model.Task, *relaycommon.TaskInfo) int {
	return a.quota
}

func TestUpdateVideoSingleTaskDurableProjectionPreservesOperationalURL(t *testing.T) {
	db := setupBridgeTestDB(t)
	fixture := newBridgeTestFixture(t, db, "video-terminal-projection", 1000, 1000, 100)
	signedURL := "https://media.example.test/video.mp4?signature=secret-value&expires=999999"
	startTime := time.Now().Add(-time.Minute).Unix()
	fixture.Task.Status = model.TaskStatusInProgress
	fixture.Task.StartTime = startTime
	fixture.Task.Action = constant.TaskActionGenerate
	fixture.Task.PrivateData.UpstreamTaskID = fixture.Attempt.ProviderOperationID
	require.NoError(t, db.Model(&model.Task{}).Where("id = ?", fixture.Task.ID).Updates(map[string]interface{}{
		"status": fixture.Task.Status, "start_time": startTime, "action": fixture.Task.Action, "private_data": fixture.Task.PrivateData,
	}).Error)
	body, err := common.Marshal(map[string]interface{}{
		"status": "SUCCESS",
		"response": map[string]interface{}{
			"state": "completed", "prompt": "private prompt", "authorization": "Bearer private-token",
		},
	})
	require.NoError(t, err)
	adaptor := &terminalProjectionPollingAdaptor{
		body:  body,
		info:  &relaycommon.TaskInfo{TaskID: fixture.Attempt.ProviderOperationID, Status: string(model.TaskStatusSuccess), Reason: "provider_terminal", Url: signedURL, Progress: "100%"},
		quota: 80,
	}
	var channel model.Channel
	require.NoError(t, db.First(&channel, fixture.Attempt.ChannelID).Error)
	require.NoError(t, updateVideoSingleTask(context.Background(), adaptor, &channel, fixture.Attempt.ProviderOperationID, map[string]*model.Task{fixture.Attempt.ProviderOperationID: &fixture.Task}))

	var task model.Task
	var observation model.TaskTerminalObservation
	require.NoError(t, db.First(&task, fixture.Task.ID).Error)
	require.NoError(t, db.Where("operation_id = ?", fixture.Operation.ID).First(&observation).Error)
	assert.Equal(t, model.TaskStatus(model.TaskStatusSuccess), task.Status)
	assert.Equal(t, 80, task.Quota)
	assert.Equal(t, startTime, task.StartTime)
	assert.Positive(t, task.FinishTime)
	assert.Equal(t, signedURL, task.PrivateData.ResultURL)
	assert.Equal(t, signedURL, task.GetResultURL())
	assert.Contains(t, string(task.Data), `"state":"completed"`)
	assert.NotContains(t, string(task.Data), "private prompt")
	assert.NotContains(t, string(task.Data), "private-token")
	assert.Equal(t, "https://media.example.test/video.mp4", observation.ResultURL)
	assert.Equal(t, signedURL, observation.OperationalResultURL)
	assert.Equal(t, startTime, observation.TaskStartTime)
	assert.Positive(t, observation.TaskFinishTime)
	assert.Equal(t, model.TaskTerminalObservationApplied, observation.State)
	var outboxes []model.TaskBillingLogOutbox
	require.NoError(t, db.Order("id").Find(&outboxes).Error)
	require.Len(t, outboxes, 2)
	for _, outbox := range outboxes {
		encoded, marshalErr := common.Marshal(outbox.Payload)
		require.NoError(t, marshalErr)
		assert.NotContains(t, string(encoded), "secret-value")
		assert.NotContains(t, string(encoded), "private prompt")
	}
}

type sunoTerminalBatchAdaptor struct {
	code   string
	items  []taskdto.SunoDataResponse
	adjust int
}

func (a *sunoTerminalBatchAdaptor) Init(*relaycommon.RelayInfo) {}
func (a *sunoTerminalBatchAdaptor) FetchTask(string, string, map[string]any, string) (*http.Response, error) {
	code := a.code
	if code == "" {
		code = taskdto.TaskSuccessCode
	}
	body, err := common.Marshal(taskdto.TaskResponse[[]taskdto.SunoDataResponse]{Code: code, Data: a.items})
	if err != nil {
		return nil, err
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(body))}, nil
}
func (a *sunoTerminalBatchAdaptor) ParseTaskResult([]byte) (*relaycommon.TaskInfo, error) {
	return nil, nil
}
func (a *sunoTerminalBatchAdaptor) AdjustBillingOnComplete(*model.Task, *relaycommon.TaskInfo) int {
	return a.adjust
}

func prepareDurablePollingFixture(t *testing.T, db *gorm.DB, fixture *bridgeTestFixture, platform constant.TaskPlatform) {
	t.Helper()
	fixture.Task.Platform = platform
	fixture.Task.Action = constant.TaskActionGenerate
	fixture.Task.Status = model.TaskStatusInProgress
	fixture.Task.Progress = "50%"
	fixture.Task.SubmitTime = time.Now().Unix()
	fixture.Task.StartTime = time.Now().Add(-time.Minute).Unix()
	fixture.Task.PrivateData.UpstreamTaskID = fixture.Attempt.ProviderOperationID
	require.NoError(t, db.Model(&model.Task{}).Where("id = ?", fixture.Task.ID).Updates(map[string]interface{}{
		"platform": fixture.Task.Platform, "action": fixture.Task.Action, "status": fixture.Task.Status,
		"progress": fixture.Task.Progress, "submit_time": fixture.Task.SubmitTime, "start_time": fixture.Task.StartTime,
		"private_data": fixture.Task.PrivateData,
	}).Error)
}

func assertDurableAccountingState(t *testing.T, db *gorm.DB, fixture bridgeTestFixture, taskStatus model.TaskStatus, operationStatus model.TaskSubmissionOperationStatus, taskQuota, userQuota, usedQuota int, channelQuota int64, outboxCount int64) {
	t.Helper()
	var task model.Task
	var operation model.TaskSubmissionOperation
	var user model.User
	var channel model.Channel
	require.NoError(t, db.First(&task, fixture.Task.ID).Error)
	require.NoError(t, db.First(&operation, fixture.Operation.ID).Error)
	require.NoError(t, db.First(&user, fixture.User.Id).Error)
	require.NoError(t, db.First(&channel, fixture.Attempt.ChannelID).Error)
	assert.Equal(t, taskStatus, task.Status)
	assert.Equal(t, operationStatus, operation.Status)
	assert.Equal(t, taskQuota, task.Quota)
	assert.Equal(t, userQuota, user.Quota)
	assert.Equal(t, usedQuota, user.UsedQuota)
	assert.Equal(t, 1, user.RequestCount)
	assert.Equal(t, channelQuota, channel.UsedQuota)
	var count int64
	require.NoError(t, db.Model(&model.TaskBillingLogOutbox{}).Count(&count).Error)
	assert.Equal(t, outboxCount, count)
}

func TestDurablePollingTimeoutAndMissingUpstreamRemainManual(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		db := setupBridgeTestDB(t)
		fixture := newBridgeTestFixture(t, db, "durable-timeout", 1000, 1000, 100)
		prepareDurablePollingFixture(t, db, &fixture, constant.TaskPlatform("kling"))
		fixture.Task.SubmitTime = time.Now().Add(-2 * time.Minute).Unix()
		require.NoError(t, db.Model(&model.Task{}).Where("id = ?", fixture.Task.ID).Update("submit_time", fixture.Task.SubmitTime).Error)
		previousTimeout := constant.TaskTimeoutMinutes
		constant.TaskTimeoutMinutes = 1
		t.Cleanup(func() { constant.TaskTimeoutMinutes = previousTimeout })
		sweepTimedOutTasks(context.Background())
		var observation model.TaskTerminalObservation
		require.NoError(t, db.Where("operation_id = ?", fixture.Operation.ID).First(&observation).Error)
		assert.Equal(t, model.TaskTerminalObservationManualReview, observation.State)
		assert.Equal(t, "task_timeout_unverified", observation.ReasonCode)
		assertDurableAccountingState(t, db, fixture, model.TaskStatusInProgress, model.TaskSubmissionOperationStatusAccepted, 100, 900, 100, 100, 1)
	})

	t.Run("missing_upstream", func(t *testing.T) {
		db := setupBridgeTestDB(t)
		fixture := newBridgeTestFixture(t, db, "durable-null-upstream", 1000, 1000, 100)
		prepareDurablePollingFixture(t, db, &fixture, constant.TaskPlatform("kling"))
		fixture.Task.PrivateData.UpstreamTaskID = ""
		require.NoError(t, db.Model(&model.Task{}).Where("id = ?", fixture.Task.ID).Update("private_data", fixture.Task.PrivateData).Error)
		previousTimeout := constant.TaskTimeoutMinutes
		constant.TaskTimeoutMinutes = 0
		t.Cleanup(func() { constant.TaskTimeoutMinutes = previousTimeout })
		previousQueryLimit := constant.TaskQueryLimit
		constant.TaskQueryLimit = 100
		t.Cleanup(func() { constant.TaskQueryLimit = previousQueryLimit })
		previousFactory := GetTaskAdaptorFunc
		GetTaskAdaptorFunc = func(constant.TaskPlatform) TaskPollingAdaptor { return &terminalProjectionPollingAdaptor{} }
		t.Cleanup(func() { GetTaskAdaptorFunc = previousFactory })
		var unfinishedCount int64
		require.NoError(t, db.Model(&model.Task{}).Where("progress != ? AND status NOT IN ?", "100%", []model.TaskStatus{model.TaskStatusFailure, model.TaskStatusSuccess}).Count(&unfinishedCount).Error)
		require.Equal(t, int64(1), unfinishedCount)
		summary := RunTaskPollingOnce(context.Background(), nil)
		assert.Equal(t, 1, summary.NullTasksFailed)
		var observation model.TaskTerminalObservation
		require.NoError(t, db.Where("operation_id = ?", fixture.Operation.ID).First(&observation).Error)
		assert.Equal(t, model.TaskTerminalObservationManualReview, observation.State)
		assert.Equal(t, "missing_upstream_task_id", observation.ReasonCode)
		assertDurableAccountingState(t, db, fixture, model.TaskStatusInProgress, model.TaskSubmissionOperationStatusAccepted, 100, 900, 100, 100, 1)
	})
}

func TestDurablePollingChannelFailuresDoNotBulkFinalize(t *testing.T) {
	for _, tc := range []struct {
		name     string
		platform constant.TaskPlatform
		run      func(context.Context, int, string, map[string]*model.Task) error
	}{
		{name: "suno", platform: constant.TaskPlatformSuno, run: func(ctx context.Context, channelID int, upstreamID string, tasks map[string]*model.Task) error {
			return updateSunoTasks(ctx, channelID, []string{upstreamID}, tasks)
		}},
		{name: "video", platform: constant.TaskPlatform("kling"), run: func(ctx context.Context, channelID int, upstreamID string, tasks map[string]*model.Task) error {
			return updateVideoTasks(ctx, constant.TaskPlatform("kling"), channelID, []string{upstreamID}, tasks)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupBridgeTestDB(t)
			fixture := newBridgeTestFixture(t, db, "channel-failure-"+tc.name, 1000, 1000, 100)
			prepareDurablePollingFixture(t, db, &fixture, tc.platform)
			previousMemoryCache := common.MemoryCacheEnabled
			common.MemoryCacheEnabled = false
			t.Cleanup(func() { common.MemoryCacheEnabled = previousMemoryCache })
			require.NoError(t, db.Delete(&model.Channel{}, fixture.Attempt.ChannelID).Error)
			_ = tc.run(context.Background(), fixture.Attempt.ChannelID, fixture.Attempt.ProviderOperationID, map[string]*model.Task{fixture.Attempt.ProviderOperationID: &fixture.Task})
			var observation model.TaskTerminalObservation
			require.NoError(t, db.Where("operation_id = ?", fixture.Operation.ID).First(&observation).Error)
			assert.Equal(t, model.TaskTerminalObservationManualReview, observation.State)
			var task model.Task
			var operation model.TaskSubmissionOperation
			var user model.User
			require.NoError(t, db.First(&task, fixture.Task.ID).Error)
			require.NoError(t, db.First(&operation, fixture.Operation.ID).Error)
			require.NoError(t, db.First(&user, fixture.User.Id).Error)
			assert.Equal(t, model.TaskStatus(model.TaskStatusInProgress), task.Status)
			assert.Equal(t, model.TaskSubmissionOperationStatusAccepted, operation.Status)
			assert.Equal(t, 900, user.Quota)
			assert.Equal(t, 100, user.UsedQuota)
			assert.Equal(t, 1, user.RequestCount)
			var outboxCount int64
			require.NoError(t, db.Model(&model.TaskBillingLogOutbox{}).Count(&outboxCount).Error)
			assert.Equal(t, int64(1), outboxCount)
		})
	}
}

func TestDurableSunoTerminalPaths(t *testing.T) {
	t.Run("success_per_call", func(t *testing.T) {
		db := setupBridgeTestDB(t)
		billingContext := model.TaskBillingContext{Version: model.TaskBillingContextVersion, Complete: true, ModelPrice: 1, ModelRatio: 1, GroupRatio: 1, OriginModelName: "test-model", PerCallBilling: true}
		fixture := newBridgeTestFixture(t, db, "suno-success", 1000, 1000, 100, billingContext)
		prepareDurablePollingFixture(t, db, &fixture, constant.TaskPlatformSuno)
		baseURL := "https://suno.example.test"
		require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", fixture.Attempt.ChannelID).Updates(map[string]interface{}{"type": constant.ChannelTypeSunoAPI, "base_url": baseURL}).Error)
		data, err := common.Marshal(map[string]interface{}{"songs": []interface{}{map[string]interface{}{"id": "song-1", "prompt": "private prompt", "status": "complete"}}})
		require.NoError(t, err)
		adaptor := &sunoTerminalBatchAdaptor{items: []taskdto.SunoDataResponse{{TaskID: fixture.Attempt.ProviderOperationID, Status: string(model.TaskStatusSuccess), StartTime: fixture.Task.StartTime, FinishTime: time.Now().Unix(), Data: data}}}
		previousFactory := GetTaskAdaptorFunc
		GetTaskAdaptorFunc = func(constant.TaskPlatform) TaskPollingAdaptor { return adaptor }
		t.Cleanup(func() { GetTaskAdaptorFunc = previousFactory })
		require.NoError(t, updateSunoTasks(context.Background(), fixture.Attempt.ChannelID, []string{fixture.Attempt.ProviderOperationID}, map[string]*model.Task{fixture.Attempt.ProviderOperationID: &fixture.Task}))
		assertDurableAccountingState(t, db, fixture, model.TaskStatusSuccess, model.TaskSubmissionOperationStatusSucceeded, 100, 900, 100, 100, 2)
		var task model.Task
		require.NoError(t, db.First(&task, fixture.Task.ID).Error)
		assert.NotContains(t, string(task.Data), "private prompt")
		assert.Contains(t, string(task.Data), "song-1")
	})

	t.Run("success_without_actual_is_manual", func(t *testing.T) {
		db := setupBridgeTestDB(t)
		fixture := newBridgeTestFixture(t, db, "suno-success-manual", 1000, 1000, 100)
		prepareDurablePollingFixture(t, db, &fixture, constant.TaskPlatformSuno)
		baseURL := "https://suno.example.test"
		require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", fixture.Attempt.ChannelID).Updates(map[string]interface{}{"type": constant.ChannelTypeSunoAPI, "base_url": baseURL}).Error)
		adaptor := &sunoTerminalBatchAdaptor{items: []taskdto.SunoDataResponse{{TaskID: fixture.Attempt.ProviderOperationID, Status: string(model.TaskStatusSuccess), FinishTime: time.Now().Unix()}}}
		previousFactory := GetTaskAdaptorFunc
		GetTaskAdaptorFunc = func(constant.TaskPlatform) TaskPollingAdaptor { return adaptor }
		t.Cleanup(func() { GetTaskAdaptorFunc = previousFactory })
		require.NoError(t, updateSunoTasks(context.Background(), fixture.Attempt.ChannelID, []string{fixture.Attempt.ProviderOperationID}, map[string]*model.Task{fixture.Attempt.ProviderOperationID: &fixture.Task}))
		var observation model.TaskTerminalObservation
		require.NoError(t, db.Where("operation_id = ?", fixture.Operation.ID).First(&observation).Error)
		assert.Equal(t, model.TaskTerminalObservationManualReview, observation.State)
		assert.Equal(t, "terminal_actual_evidence_missing", observation.ReasonCode)
		assertDurableAccountingState(t, db, fixture, model.TaskStatusInProgress, model.TaskSubmissionOperationStatusAccepted, 100, 900, 100, 100, 1)
	})

	t.Run("failure", func(t *testing.T) {
		db := setupBridgeTestDB(t)
		fixture := newBridgeTestFixture(t, db, "suno-failure", 1000, 1000, 100)
		prepareDurablePollingFixture(t, db, &fixture, constant.TaskPlatformSuno)
		baseURL := "https://suno.example.test"
		require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", fixture.Attempt.ChannelID).Updates(map[string]interface{}{"type": constant.ChannelTypeSunoAPI, "base_url": baseURL}).Error)
		adaptor := &sunoTerminalBatchAdaptor{items: []taskdto.SunoDataResponse{{TaskID: fixture.Attempt.ProviderOperationID, Status: string(model.TaskStatusFailure), FailReason: "Provider failure: private text", FinishTime: time.Now().Unix()}}}
		previousFactory := GetTaskAdaptorFunc
		GetTaskAdaptorFunc = func(constant.TaskPlatform) TaskPollingAdaptor { return adaptor }
		t.Cleanup(func() { GetTaskAdaptorFunc = previousFactory })
		require.NoError(t, updateSunoTasks(context.Background(), fixture.Attempt.ChannelID, []string{fixture.Attempt.ProviderOperationID}, map[string]*model.Task{fixture.Attempt.ProviderOperationID: &fixture.Task}))
		assertDurableAccountingState(t, db, fixture, model.TaskStatusFailure, model.TaskSubmissionOperationStatusFailed, 0, 1000, 0, 0, 2)
		var task model.Task
		require.NoError(t, db.First(&task, fixture.Task.ID).Error)
		assert.NotContains(t, task.FailReason, "private text")
	})

	t.Run("batch_rejection_keeps_pending", func(t *testing.T) {
		db := setupBridgeTestDB(t)
		fixture := newBridgeTestFixture(t, db, "suno-batch-rejected", 1000, 1000, 100)
		prepareDurablePollingFixture(t, db, &fixture, constant.TaskPlatformSuno)
		baseURL := "https://suno.example.test"
		require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", fixture.Attempt.ChannelID).Updates(map[string]interface{}{"type": constant.ChannelTypeSunoAPI, "base_url": baseURL}).Error)
		adaptor := &sunoTerminalBatchAdaptor{code: "error"}
		previousFactory := GetTaskAdaptorFunc
		GetTaskAdaptorFunc = func(constant.TaskPlatform) TaskPollingAdaptor { return adaptor }
		t.Cleanup(func() { GetTaskAdaptorFunc = previousFactory })
		err := updateSunoTasks(context.Background(), fixture.Attempt.ChannelID, []string{fixture.Attempt.ProviderOperationID}, map[string]*model.Task{fixture.Attempt.ProviderOperationID: &fixture.Task})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "batch response rejected")
		var observationCount int64
		require.NoError(t, db.Model(&model.TaskTerminalObservation{}).Where("operation_id = ?", fixture.Operation.ID).Count(&observationCount).Error)
		assert.Zero(t, observationCount)
		assertDurableAccountingState(t, db, fixture, model.TaskStatusInProgress, model.TaskSubmissionOperationStatusAccepted, 100, 900, 100, 100, 1)
	})
}

func TestDurableVideoFailureUsesObservationBeforeTaskProjection(t *testing.T) {
	db := setupBridgeTestDB(t)
	fixture := newBridgeTestFixture(t, db, "video-failure-terminal", 1000, 1000, 100)
	prepareDurablePollingFixture(t, db, &fixture, constant.TaskPlatform("kling"))
	body, err := common.Marshal(map[string]interface{}{"status": "FAILURE", "reason": "private provider text"})
	require.NoError(t, err)
	adaptor := &terminalProjectionPollingAdaptor{body: body, info: &relaycommon.TaskInfo{TaskID: fixture.Attempt.ProviderOperationID, Status: string(model.TaskStatusFailure), Reason: "Provider failure: private text"}}
	var channel model.Channel
	require.NoError(t, db.First(&channel, fixture.Attempt.ChannelID).Error)
	require.NoError(t, updateVideoSingleTask(context.Background(), adaptor, &channel, fixture.Attempt.ProviderOperationID, map[string]*model.Task{fixture.Attempt.ProviderOperationID: &fixture.Task}))
	assertDurableAccountingState(t, db, fixture, model.TaskStatusFailure, model.TaskSubmissionOperationStatusFailed, 0, 1000, 0, 0, 2)
	var observation model.TaskTerminalObservation
	var task model.Task
	require.NoError(t, db.Where("operation_id = ?", fixture.Operation.ID).First(&observation).Error)
	require.NoError(t, db.First(&task, fixture.Task.ID).Error)
	assert.Equal(t, model.TaskTerminalObservationApplied, observation.State)
	assert.Equal(t, "task_poll_release", observation.ReasonCode)
	assert.Equal(t, "task_poll_release", task.FailReason)
	assert.NotContains(t, string(task.Data), "private provider text")
}
