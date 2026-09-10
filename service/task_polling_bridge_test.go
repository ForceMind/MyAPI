package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
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

func newBridgeTestFixture(t *testing.T, db *gorm.DB, label string, userQuota, tokenQuota, reservedQuota int) bridgeTestFixture {
	t.Helper()
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
		BillingSource:            "wallet",
		BillingContext: model.TaskBillingContext{
			Version:         model.TaskBillingContextVersion,
			Complete:        true,
			ModelPrice:      1,
			ModelRatio:      1,
			GroupRatio:      1,
			OriginModelName: "test-model",
		},
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
			TokenId: token.Id,
			BillingContext: &model.TaskBillingContext{
				Version:         model.TaskBillingContextVersion,
				Complete:        true,
				ModelPrice:      1,
				ModelRatio:      1,
				GroupRatio:      1,
				OriginModelName: "test-model",
			},
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
	handled, err := DurableSettleTaskOnComplete(ctx, &fixture.Task, 150, "test settlement surplus")
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

	// Verify outbox record created
	var outboxes []model.TaskBillingLogOutbox
	require.NoError(t, db.Find(&outboxes).Error)
	assert.Len(t, outboxes, 1)
	assert.Equal(t, model.TaskBillingLogOutboxStatePending, outboxes[0].State)
}

func TestDurableSettleTaskOnComplete_DeficitCharge(t *testing.T) {
	db := setupBridgeTestDB(t)
	ctx := context.Background()

	// Initial user quota: 1000, reserved: 200. After reserve: 800.
	fixture := newBridgeTestFixture(t, db, "deficit", 1000, 1000, 200)

	// Settle with actual quota = 250 (deficit charge: 50)
	handled, err := DurableSettleTaskOnComplete(ctx, &fixture.Task, 250, "test settlement deficit")
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

	handled, err := DurableReleaseTaskOnFailure(ctx, &fixture.Task, "provider execution failed")
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
	assert.Len(t, outboxes, 1)
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

	// First settlement
	handled1, err1 := DurableSettleTaskOnComplete(ctx, &fixture.Task, 180, "settle 1")
	require.NoError(t, err1)
	assert.True(t, handled1)

	// Second settlement should be idempotent no-op
	handled2, err2 := DurableSettleTaskOnComplete(ctx, &fixture.Task, 180, "settle 2")
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
	handled, err := DurableSettleTaskOnComplete(ctx, &fixture.Task, 200, "settle mutex")
	require.NoError(t, err)
	assert.True(t, handled)

	// Releasing an already settled task must be rejected
	handledRelease, errRelease := DurableReleaseTaskOnFailure(ctx, &fixture.Task, "try refund after settle")
	assert.True(t, handledRelease)
	assert.ErrorIs(t, errRelease, model.ErrTaskQuotaAlreadySettled)
}

func TestTaskPolling_RefundHookIntegration(t *testing.T) {
	db := setupBridgeTestDB(t)
	ctx := context.Background()

	fixture := newBridgeTestFixture(t, db, "refund-hook", 1000, 1000, 200)

	// Call central RefundTaskQuota, which now delegates to DurableReleaseTaskOnFailure
	ok := RefundTaskQuota(ctx, &fixture.Task, "upstream timeout refund hook")
	assert.True(t, ok)

	// Verify task quota is zeroed
	var updatedTask model.Task
	require.NoError(t, db.Where("id = ?", fixture.Task.ID).First(&updatedTask).Error)
	assert.Equal(t, 0, updatedTask.Quota)

	// Verify full refund to user
	var updatedUser model.User
	require.NoError(t, db.Where("id = ?", fixture.User.Id).First(&updatedUser).Error)
	assert.Equal(t, 1000, updatedUser.Quota)

	// Verify operation status is failed
	var updatedOp model.TaskSubmissionOperation
	require.NoError(t, db.Where("id = ?", fixture.Operation.ID).First(&updatedOp).Error)
	assert.Equal(t, model.TaskSubmissionOperationStatusFailed, updatedOp.Status)
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
	assert.Equal(t, 160, updatedTask.Quota)

	// Verify surplus refund: 800 + 40 = 840
	var updatedUser model.User
	require.NoError(t, db.Where("id = ?", fixture.User.Id).First(&updatedUser).Error)
	assert.Equal(t, 840, updatedUser.Quota)

	// Verify operation status is succeeded
	var updatedOp model.TaskSubmissionOperation
	require.NoError(t, db.Where("id = ?", fixture.Operation.ID).First(&updatedOp).Error)
	assert.Equal(t, model.TaskSubmissionOperationStatusSucceeded, updatedOp.Status)
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

	handled, err := DurableSettleTaskOnComplete(ctx, &fixture.Task, 200, "settle with clamp", clamp)
	require.NoError(t, err)
	assert.True(t, handled)

	// Verify outbox record contains admin_info.quota_saturation
	var outbox model.TaskBillingLogOutbox
	require.NoError(t, db.First(&outbox).Error)
	assert.Contains(t, outbox.Payload.Other, "quota_saturation")
	assert.Contains(t, outbox.Payload.Other, "test_conversion")
}

