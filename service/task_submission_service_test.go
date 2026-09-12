package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	relaykitdto "github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupTaskSubmissionTestDB(t *testing.T) *gorm.DB {
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
		&model.SubscriptionPlan{},
		&model.UserSubscription{},
		&model.Channel{},
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
	require.NoError(t, db.Create(&model.Channel{Id: 101, Name: "submission-test"}).Error)
	return db
}

type taskSubmissionTestFixture struct {
	User      model.User
	Token     model.Token
	Operation model.TaskSubmissionOperation
	Attempt   model.TaskSubmissionAttempt
}

func newTaskSubmissionTestFixture(t *testing.T, db *gorm.DB, label string, userQuota, tokenQuota int) taskSubmissionTestFixture {
	t.Helper()
	digest := sha256.Sum256([]byte(t.Name() + ":" + label))
	name := fmt.Sprintf("sub-%x", digest[:8])

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
	fingerprint := model.FingerprintTaskSubmissionRequest([]byte(`{"prompt":"test"}`))

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

	return taskSubmissionTestFixture{
		User:      user,
		Token:     token,
		Operation: *intent.Operation,
		Attempt:   *intent.Attempt,
	}
}

func acceptedTaskCandidateForTest(op *model.TaskSubmissionOperation, attempt *model.TaskSubmissionAttempt, providerTaskID string, quota int, billingContext model.TaskBillingContext) *model.Task {
	_ = quota
	contextCopy := billingContext
	return &model.Task{
		TaskID:     op.PublicID,
		Platform:   "test",
		UserId:     op.UserID,
		Group:      "default",
		ChannelId:  attempt.ChannelID,
		Quota:      TaskInitialQuota(op.BillingVersion, op.EstimatedQuota, op.ReservedQuota),
		Action:     "video.create",
		Status:     model.TaskStatusNotStart,
		SubmitTime: common.GetTimestamp(),
		Progress:   "0%",
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID:    providerTaskID,
			BillingPreference: op.BillingPreference,
			BillingSource:     op.BillingSource,
			FreeModel:         op.FreeModel,
			SubscriptionId:    op.SubscriptionID,
			TokenId:           op.TokenID,
			NodeName:          "test-node",
			BillingContext:    &contextCopy,
		},
	}
}

func TestTaskSubmissionPipeline_Accepted(t *testing.T) {
	db := setupTaskSubmissionTestDB(t)
	fixture := newTaskSubmissionTestFixture(t, db, "accepted", 1000, 500)
	billingContext := model.TaskBillingContext{
		Version:         model.TaskBillingContextVersion,
		Complete:        true,
		ModelPrice:      1,
		ModelRatio:      1,
		GroupRatio:      1,
		OriginModelName: "test-model",
		PerCallBilling:  true,
	}

	dispatcherCalled := false
	dispatcher := func(ctx context.Context, op *model.TaskSubmissionOperation, attempt *model.TaskSubmissionAttempt) (*TaskProviderDispatchResult, error) {
		dispatcherCalled = true
		assert.Equal(t, model.TaskSubmissionOperationStatusDispatching, op.Status)
		assert.Equal(t, model.TaskSubmissionAttemptStatusDispatching, attempt.Status)
		return &TaskProviderDispatchResult{
			Status:         TaskProviderDispatchStatusAccepted,
			ProviderTaskID: "provider-upstream-task-999",
			TaskCandidate:  acceptedTaskCandidateForTest(op, attempt, "provider-upstream-task-999", 100, billingContext),
		}, nil
	}

	input := TaskSubmissionPipelineInput{
		DB:             db,
		OperationID:    fixture.Operation.ID,
		AttemptID:      fixture.Attempt.ID,
		UserID:         fixture.User.Id,
		TokenID:        fixture.Token.Id,
		ChannelID:      fixture.Attempt.ChannelID,
		Quota:          100,
		BillingSource:  "wallet",
		BillingContext: billingContext,
		Dispatcher:     dispatcher,
	}

	result, err := ExecuteTaskSubmissionPipeline(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, dispatcherCalled)

	// Verify receipts
	require.NotNil(t, result.ReserveReceipt)
	assert.Equal(t, string(model.TaskBillingEventTypeReserve), result.ReserveReceipt.MutationType)
	assert.Nil(t, result.ReleaseReceipt)

	// Verify balances remained deducted
	var user model.User
	var token model.Token
	require.NoError(t, db.First(&user, fixture.User.Id).Error)
	require.NoError(t, db.First(&token, fixture.Token.Id).Error)
	assert.Equal(t, 900, user.Quota)
	assert.Equal(t, 400, token.RemainQuota)
	assert.Equal(t, 100, token.UsedQuota)

	// Verify Operation and Attempt status
	assert.Equal(t, model.TaskSubmissionOperationStatusAccepted, result.Operation.Status)
	require.NotNil(t, result.Operation.TaskID)
	assert.Equal(t, model.TaskSubmissionAttemptStatusAccepted, result.Attempt.Status)
	assert.Equal(t, "provider-upstream-task-999", result.Attempt.ProviderOperationID)

	// Verify formal task
	var formalTask model.Task
	require.NoError(t, db.First(&formalTask, *result.Operation.TaskID).Error)
	assert.Equal(t, fixture.Operation.PublicID, formalTask.TaskID)
	assert.Equal(t, fixture.User.Id, formalTask.UserId)
	assert.Equal(t, model.TaskStatusNotStart, formalTask.Status)
}

func TestTaskSubmissionPipeline_Rejected(t *testing.T) {
	db := setupTaskSubmissionTestDB(t)
	fixture := newTaskSubmissionTestFixture(t, db, "rejected", 1000, 500)

	dispatcherCalled := false
	dispatcher := func(ctx context.Context, op *model.TaskSubmissionOperation, attempt *model.TaskSubmissionAttempt) (*TaskProviderDispatchResult, error) {
		dispatcherCalled = true
		assert.Equal(t, model.TaskSubmissionOperationStatusDispatching, op.Status)
		assert.Equal(t, model.TaskSubmissionAttemptStatusDispatching, attempt.Status)
		return &TaskProviderDispatchResult{
			Status:       TaskProviderDispatchStatusRejected,
			ErrorCode:    "UPSTREAM_POLICY_REJECT",
			ErrorMessage: "prompt contains restricted keywords",
		}, nil
	}

	input := TaskSubmissionPipelineInput{
		DB:            db,
		OperationID:   fixture.Operation.ID,
		AttemptID:     fixture.Attempt.ID,
		UserID:        fixture.User.Id,
		TokenID:       fixture.Token.Id,
		ChannelID:     fixture.Attempt.ChannelID,
		Quota:         100,
		BillingSource: "wallet",
		BillingContext: model.TaskBillingContext{
			Version:         model.TaskBillingContextVersion,
			Complete:        true,
			ModelPrice:      1,
			ModelRatio:      1,
			GroupRatio:      1,
			OriginModelName: "test-model",
			PerCallBilling:  true,
		},
		Dispatcher: dispatcher,
	}

	result, err := ExecuteTaskSubmissionPipeline(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, dispatcherCalled)

	// Verify receipts: reserve receipt exists AND release refund receipt exists
	require.NotNil(t, result.ReserveReceipt)
	require.NotNil(t, result.ReleaseReceipt)
	assert.Equal(t, string(model.TaskBillingEventTypeRefund), result.ReleaseReceipt.MutationType)
	assert.Equal(t, int64(100), result.ReleaseReceipt.Quota)
	assert.Equal(t, "local.dispatch_rejected", result.ReleaseReceipt.EvidenceID)
	assert.Equal(t, 1, result.ReleaseReceipt.EvidenceVersion)

	// Verify balances were fully refunded
	var user model.User
	var token model.Token
	require.NoError(t, db.First(&user, fixture.User.Id).Error)
	require.NoError(t, db.First(&token, fixture.Token.Id).Error)
	assert.Equal(t, 1000, user.Quota)
	assert.Equal(t, 500, token.RemainQuota)
	assert.Equal(t, 0, token.UsedQuota)
	assert.Equal(t, 0, user.UsedQuota)
	assert.Equal(t, 1, user.RequestCount)
	var channel model.Channel
	require.NoError(t, db.First(&channel, fixture.Attempt.ChannelID).Error)
	assert.Zero(t, channel.UsedQuota)
	var outboxCount int64
	require.NoError(t, db.Model(&model.TaskBillingLogOutbox{}).Count(&outboxCount).Error)
	assert.Equal(t, int64(2), outboxCount)

	// Verify Operation and Attempt status
	assert.Equal(t, model.TaskSubmissionOperationStatusRejected, result.Operation.Status)
	assert.Equal(t, model.TaskSubmissionAttemptStatusRejected, result.Attempt.Status)
	assert.Equal(t, "upstream_policy_reject", result.Attempt.OutcomeCode)
	assert.Equal(t, "upstream_policy_reject", result.Operation.ReasonCode)
}

func TestTaskSubmissionPipeline_Unknown_ExplicitStatus(t *testing.T) {
	db := setupTaskSubmissionTestDB(t)
	fixture := newTaskSubmissionTestFixture(t, db, "unknown-status", 1000, 500)

	dispatcherCalled := false
	dispatcher := func(ctx context.Context, op *model.TaskSubmissionOperation, attempt *model.TaskSubmissionAttempt) (*TaskProviderDispatchResult, error) {
		dispatcherCalled = true
		return &TaskProviderDispatchResult{
			Status:         TaskProviderDispatchStatusUnknown,
			ProviderTaskID: "pending-or-indeterminate-id",
			ErrorCode:      "STATUS_POLL_REQUIRED",
			ErrorMessage:   "upstream responded with gateway timeout",
		}, nil
	}

	input := TaskSubmissionPipelineInput{
		DB:            db,
		OperationID:   fixture.Operation.ID,
		AttemptID:     fixture.Attempt.ID,
		UserID:        fixture.User.Id,
		TokenID:       fixture.Token.Id,
		ChannelID:     fixture.Attempt.ChannelID,
		Quota:         100,
		BillingSource: "wallet",
		BillingContext: model.TaskBillingContext{
			Version:         model.TaskBillingContextVersion,
			Complete:        true,
			ModelPrice:      1,
			ModelRatio:      1,
			GroupRatio:      1,
			OriginModelName: "test-model",
			PerCallBilling:  true,
		},
		Dispatcher: dispatcher,
	}

	result, err := ExecuteTaskSubmissionPipeline(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, dispatcherCalled)

	// Verify receipts: reserve receipt exists, release receipt is NIL (fail-closed, quota NOT refunded)
	require.NotNil(t, result.ReserveReceipt)
	assert.Nil(t, result.ReleaseReceipt)

	// Verify balances remained deducted (not refunded)
	var user model.User
	var token model.Token
	require.NoError(t, db.First(&user, fixture.User.Id).Error)
	require.NoError(t, db.First(&token, fixture.Token.Id).Error)
	assert.Equal(t, 900, user.Quota)
	assert.Equal(t, 400, token.RemainQuota)

	// Verify Operation and Attempt status
	assert.Equal(t, model.TaskSubmissionOperationStatusSubmissionUnknown, result.Operation.Status)
	assert.Equal(t, model.TaskSubmissionAttemptStatusSubmissionUnknown, result.Attempt.Status)
	assert.Equal(t, "pending-or-indeterminate-id", result.Attempt.ProviderOperationID)
	assert.Equal(t, "status_poll_required", result.Attempt.OutcomeCode)
	assert.Equal(t, "status_poll_required", result.Operation.ReasonCode)
}

func TestTaskSubmissionPipeline_Unknown_NetworkError(t *testing.T) {
	db := setupTaskSubmissionTestDB(t)
	fixture := newTaskSubmissionTestFixture(t, db, "unknown-neterr", 1000, 500)

	dispatcherCalled := false
	dispatcher := func(ctx context.Context, op *model.TaskSubmissionOperation, attempt *model.TaskSubmissionAttempt) (*TaskProviderDispatchResult, error) {
		dispatcherCalled = true
		return nil, errors.New("connection reset by peer")
	}

	input := TaskSubmissionPipelineInput{
		DB:            db,
		OperationID:   fixture.Operation.ID,
		AttemptID:     fixture.Attempt.ID,
		UserID:        fixture.User.Id,
		TokenID:       fixture.Token.Id,
		ChannelID:     fixture.Attempt.ChannelID,
		Quota:         100,
		BillingSource: "wallet",
		BillingContext: model.TaskBillingContext{
			Version:         model.TaskBillingContextVersion,
			Complete:        true,
			ModelPrice:      1,
			ModelRatio:      1,
			GroupRatio:      1,
			OriginModelName: "test-model",
			PerCallBilling:  true,
		},
		Dispatcher: dispatcher,
	}

	result, err := ExecuteTaskSubmissionPipeline(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, dispatcherCalled)

	// Verify receipts: quota retained (fail-closed)
	require.NotNil(t, result.ReserveReceipt)
	assert.Nil(t, result.ReleaseReceipt)

	var user model.User
	require.NoError(t, db.First(&user, fixture.User.Id).Error)
	assert.Equal(t, 900, user.Quota)

	assert.Equal(t, model.TaskSubmissionOperationStatusSubmissionUnknown, result.Operation.Status)
	assert.Equal(t, model.TaskSubmissionAttemptStatusSubmissionUnknown, result.Attempt.Status)
	assert.Equal(t, "task_dispatch_unknown", result.Operation.ReasonCode)
}

func TestTaskSubmissionPipeline_ReserveFailure_InsufficientQuota(t *testing.T) {
	db := setupTaskSubmissionTestDB(t)
	// User only has 50 quota, trying to reserve 100
	fixture := newTaskSubmissionTestFixture(t, db, "insufficient", 50, 500)

	dispatcherCalled := false
	dispatcher := func(ctx context.Context, op *model.TaskSubmissionOperation, attempt *model.TaskSubmissionAttempt) (*TaskProviderDispatchResult, error) {
		dispatcherCalled = true
		return &TaskProviderDispatchResult{Status: TaskProviderDispatchStatusAccepted}, nil
	}

	input := TaskSubmissionPipelineInput{
		DB:            db,
		OperationID:   fixture.Operation.ID,
		AttemptID:     fixture.Attempt.ID,
		UserID:        fixture.User.Id,
		TokenID:       fixture.Token.Id,
		ChannelID:     fixture.Attempt.ChannelID,
		Quota:         100,
		BillingSource: "wallet",
		BillingContext: model.TaskBillingContext{
			Version:         model.TaskBillingContextVersion,
			Complete:        true,
			ModelPrice:      1,
			ModelRatio:      1,
			GroupRatio:      1,
			OriginModelName: "test-model",
			PerCallBilling:  true,
		},
		Dispatcher: dispatcher,
	}

	result, err := ExecuteTaskSubmissionPipeline(context.Background(), input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "task quota reservation failed")
	assert.Nil(t, result)
	assert.False(t, dispatcherCalled)

	// Operation and Attempt should remain in Prepared status
	var op model.TaskSubmissionOperation
	require.NoError(t, db.First(&op, fixture.Operation.ID).Error)
	assert.Equal(t, model.TaskSubmissionOperationStatusPrepared, op.Status)

	var attempt model.TaskSubmissionAttempt
	require.NoError(t, db.First(&attempt, fixture.Attempt.ID).Error)
	assert.Equal(t, model.TaskSubmissionAttemptStatusPrepared, attempt.Status)
}

func TestTaskSubmissionPipeline_ValidationErrors(t *testing.T) {
	db := setupTaskSubmissionTestDB(t)
	fixture := newTaskSubmissionTestFixture(t, db, "validation", 1000, 500)

	// Missing dispatcher
	_, err := ExecuteTaskSubmissionPipeline(context.Background(), TaskSubmissionPipelineInput{
		DB:          db,
		OperationID: fixture.Operation.ID,
		UserID:      fixture.User.Id,
		TokenID:     fixture.Token.Id,
		ChannelID:   fixture.Attempt.ChannelID,
		Quota:       100,
	})
	require.ErrorIs(t, err, ErrTaskSubmissionDispatcherNil)

	// Invalid input IDs
	dummyDispatcher := func(ctx context.Context, op *model.TaskSubmissionOperation, attempt *model.TaskSubmissionAttempt) (*TaskProviderDispatchResult, error) {
		return nil, nil
	}
	_, err = ExecuteTaskSubmissionPipeline(context.Background(), TaskSubmissionPipelineInput{
		DB:          db,
		OperationID: -1,
		Dispatcher:  dummyDispatcher,
	})
	require.ErrorIs(t, err, ErrTaskSubmissionInvalidInput)

	// Ownership mismatch
	_, err = ExecuteTaskSubmissionPipeline(context.Background(), TaskSubmissionPipelineInput{
		DB:          db,
		OperationID: fixture.Operation.ID,
		UserID:      fixture.User.Id + 999, // wrong user ID
		TokenID:     fixture.Token.Id,
		ChannelID:   fixture.Attempt.ChannelID,
		Quota:       100,
		Dispatcher:  dummyDispatcher,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ownership or channel mismatch")
}

func TestTaskSubmissionService_ExecuteWrapper(t *testing.T) {
	db := setupTaskSubmissionTestDB(t)
	fixture := newTaskSubmissionTestFixture(t, db, "service-wrapper", 1000, 500)

	svc := NewTaskSubmissionService(db)
	billingContext := model.TaskBillingContext{
		Version: model.TaskBillingContextVersion, Complete: true, ModelPrice: 1,
		ModelRatio: 1, GroupRatio: 1, OriginModelName: "test-model", PerCallBilling: true,
	}
	result, err := svc.Execute(context.Background(), TaskSubmissionPipelineInput{
		OperationID:    fixture.Operation.ID,
		AttemptID:      fixture.Attempt.ID,
		UserID:         fixture.User.Id,
		TokenID:        fixture.Token.Id,
		ChannelID:      fixture.Attempt.ChannelID,
		Quota:          100,
		BillingSource:  "wallet",
		BillingContext: billingContext,
		Dispatcher: func(ctx context.Context, op *model.TaskSubmissionOperation, attempt *model.TaskSubmissionAttempt) (*TaskProviderDispatchResult, error) {
			return &TaskProviderDispatchResult{
				Status:         TaskProviderDispatchStatusAccepted,
				ProviderTaskID: "wrapper-task-id",
				TaskCandidate:  acceptedTaskCandidateForTest(op, attempt, "wrapper-task-id", 100, billingContext),
			}, nil
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, model.TaskSubmissionOperationStatusAccepted, result.Operation.Status)
}

func TestTaskSubmissionPipeline_ContextTimeoutFailClosed(t *testing.T) {
	db := setupTaskSubmissionTestDB(t)
	fixture := newTaskSubmissionTestFixture(t, db, "timeout-cancel", 1000, 500)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	dispatcher := func(ctx context.Context, op *model.TaskSubmissionOperation, attempt *model.TaskSubmissionAttempt) (*TaskProviderDispatchResult, error) {
		// Even if dispatcher returns "rejected", canceled context MUST force "unknown"
		return &TaskProviderDispatchResult{
			Status: TaskProviderDispatchStatusRejected,
		}, nil
	}

	result, err := ExecuteTaskSubmissionPipeline(ctx, TaskSubmissionPipelineInput{
		DB:            db,
		OperationID:   fixture.Operation.ID,
		AttemptID:     fixture.Attempt.ID,
		UserID:        fixture.User.Id,
		TokenID:       fixture.Token.Id,
		ChannelID:     fixture.Attempt.ChannelID,
		Quota:         100,
		BillingSource: "wallet",
		BillingContext: model.TaskBillingContext{
			Version:         model.TaskBillingContextVersion,
			Complete:        true,
			ModelPrice:      1,
			ModelRatio:      1,
			GroupRatio:      1,
			OriginModelName: "test-model",
			PerCallBilling:  true,
		},
		Dispatcher: dispatcher,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	// Must fail-closed to submission_unknown without releasing quota
	assert.Equal(t, model.TaskSubmissionOperationStatusSubmissionUnknown, result.Operation.Status)
	assert.Nil(t, result.ReleaseReceipt, "must not refund on canceled context")
}

func TestTaskSubmissionPipeline_RejectsOpenTransaction(t *testing.T) {
	db := setupTaskSubmissionTestDB(t)
	fixture := newTaskSubmissionTestFixture(t, db, "open-tx", 1000, 500)

	tx := db.Begin()
	defer tx.Rollback()

	_, err := ExecuteTaskSubmissionPipeline(context.Background(), TaskSubmissionPipelineInput{
		DB:          tx,
		OperationID: fixture.Operation.ID,
		UserID:      fixture.User.Id,
		TokenID:     fixture.Token.Id,
		ChannelID:   fixture.Attempt.ChannelID,
		Quota:       100,
		Dispatcher: func(ctx context.Context, op *model.TaskSubmissionOperation, attempt *model.TaskSubmissionAttempt) (*TaskProviderDispatchResult, error) {
			return nil, nil
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pipeline must not run within an existing database transaction")
}

func TestTaskSubmissionPipeline_MismatchedAttemptID(t *testing.T) {
	db := setupTaskSubmissionTestDB(t)
	fixture1 := newTaskSubmissionTestFixture(t, db, "mismatch-1", 1000, 500)
	fixture2 := newTaskSubmissionTestFixture(t, db, "mismatch-2", 1000, 500)

	_, err := ExecuteTaskSubmissionPipeline(context.Background(), TaskSubmissionPipelineInput{
		DB:          db,
		OperationID: fixture1.Operation.ID,
		AttemptID:   fixture2.Attempt.ID, // belongs to fixture2!
		UserID:      fixture1.User.Id,
		TokenID:     fixture1.Token.Id,
		ChannelID:   fixture1.Attempt.ChannelID,
		Quota:       100,
		Dispatcher: func(ctx context.Context, op *model.TaskSubmissionOperation, attempt *model.TaskSubmissionAttempt) (*TaskProviderDispatchResult, error) {
			return nil, nil
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "attempt does not belong to operation")
}

func TestTaskSubmissionPipeline_ZeroQuota(t *testing.T) {
	db := setupTaskSubmissionTestDB(t)
	fixture := newTaskSubmissionTestFixture(t, db, "zero-quota-pipe", 1000, 500)

	billingContext := model.TaskBillingContext{
		Version: model.TaskBillingContextVersion, Complete: true, ModelPrice: 0,
		ModelRatio: 1, GroupRatio: 1, OriginModelName: "free-model", PerCallBilling: true,
	}
	result, err := ExecuteTaskSubmissionPipeline(context.Background(), TaskSubmissionPipelineInput{
		DB:             db,
		OperationID:    fixture.Operation.ID,
		AttemptID:      fixture.Attempt.ID,
		UserID:         fixture.User.Id,
		TokenID:        fixture.Token.Id,
		ChannelID:      fixture.Attempt.ChannelID,
		Quota:          0, // Free model
		FreeModel:      true,
		BillingSource:  "wallet",
		BillingContext: billingContext,
		Dispatcher: func(ctx context.Context, op *model.TaskSubmissionOperation, attempt *model.TaskSubmissionAttempt) (*TaskProviderDispatchResult, error) {
			return &TaskProviderDispatchResult{
				Status:         TaskProviderDispatchStatusAccepted,
				ProviderTaskID: "free-task-1",
				TaskCandidate:  acceptedTaskCandidateForTest(op, attempt, "free-task-1", 0, billingContext),
			}, nil
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, model.TaskSubmissionOperationStatusAccepted, result.Operation.Status)
	assert.Equal(t, int64(0), result.ReserveReceipt.Quota)
}

func TestTaskSubmissionPipeline_Subscription(t *testing.T) {
	db := setupTaskSubmissionTestDB(t)
	fixture := newTaskSubmissionTestFixture(t, db, "sub-pipe", 1000, 500)

	sub := model.UserSubscription{
		UserId: fixture.User.Id, PlanId: 1, AmountTotal: 1000, AmountUsed: 100,
		Status: "active", StartTime: 1, EndTime: 1<<31 - 1,
	}
	require.NoError(t, db.Create(&sub).Error)

	// Pipeline with subscription rejected -> verifies full refund
	result, err := ExecuteTaskSubmissionPipeline(context.Background(), TaskSubmissionPipelineInput{
		DB:             db,
		OperationID:    fixture.Operation.ID,
		AttemptID:      fixture.Attempt.ID,
		UserID:         fixture.User.Id,
		TokenID:        fixture.Token.Id,
		ChannelID:      fixture.Attempt.ChannelID,
		Quota:          100,
		BillingSource:  "subscription",
		SubscriptionID: sub.Id,
		BillingContext: model.TaskBillingContext{
			Version:         model.TaskBillingContextVersion,
			Complete:        true,
			ModelPrice:      1,
			ModelRatio:      1,
			GroupRatio:      1,
			OriginModelName: "test-model",
			PerCallBilling:  true,
		},
		Dispatcher: func(ctx context.Context, op *model.TaskSubmissionOperation, attempt *model.TaskSubmissionAttempt) (*TaskProviderDispatchResult, error) {
			return &TaskProviderDispatchResult{
				Status:    TaskProviderDispatchStatusRejected,
				ErrorCode: "content_policy_violation",
			}, nil
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, model.TaskSubmissionOperationStatusRejected, result.Operation.Status)
	require.NotNil(t, result.ReleaseReceipt)
	assert.Equal(t, int64(100), result.ReleaseReceipt.Quota)

	var checkSub model.UserSubscription
	require.NoError(t, db.First(&checkSub, sub.Id).Error)
	assert.Equal(t, int64(100), checkSub.AmountUsed, "subscription amount_used should be restored to initial 100")
}

func TestTaskSubmissionPipeline_AcceptedWithoutCompleteTaskCandidateBecomesUnknown(t *testing.T) {
	db := setupTaskSubmissionTestDB(t)
	fixture := newTaskSubmissionTestFixture(t, db, "accepted-missing-task", 1000, 500)
	billingContext := model.TaskBillingContext{
		Version: model.TaskBillingContextVersion, Complete: true, ModelPrice: 1,
		ModelRatio: 1, GroupRatio: 1, OriginModelName: "test-model", PerCallBilling: true,
	}

	result, err := ExecuteTaskSubmissionPipeline(context.Background(), TaskSubmissionPipelineInput{
		DB: db, OperationID: fixture.Operation.ID, AttemptID: fixture.Attempt.ID,
		UserID: fixture.User.Id, TokenID: fixture.Token.Id, ChannelID: fixture.Attempt.ChannelID,
		Quota: 100, BillingSource: "wallet", BillingContext: billingContext,
		Dispatcher: func(context.Context, *model.TaskSubmissionOperation, *model.TaskSubmissionAttempt) (*TaskProviderDispatchResult, error) {
			return &TaskProviderDispatchResult{
				Status: TaskProviderDispatchStatusAccepted, ProviderTaskID: "missing-candidate-upstream",
			}, nil
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, TaskProviderDispatchStatusUnknown, result.DispatchResult.Status)
	assert.Equal(t, "invalid_task_candidate", result.DispatchResult.ErrorCode)
	assert.Equal(t, model.TaskSubmissionOperationStatusSubmissionUnknown, result.Operation.Status)
	assert.Equal(t, model.TaskSubmissionAttemptStatusSubmissionUnknown, result.Attempt.Status)
	assert.Nil(t, result.Operation.TaskID)

	var count int64
	require.NoError(t, db.Model(&model.Task{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestTaskSubmissionPipeline_TaskInsertFailureBecomesUnknown(t *testing.T) {
	db := setupTaskSubmissionTestDB(t)
	fixture := newTaskSubmissionTestFixture(t, db, "accepted-task-insert-failure", 1000, 500)
	billingContext := model.TaskBillingContext{
		Version: model.TaskBillingContextVersion, Complete: true, ModelPrice: 1,
		ModelRatio: 1, GroupRatio: 1, OriginModelName: "test-model", PerCallBilling: true,
	}
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:reject-formal-task-create", func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Schema != nil && tx.Statement.Schema.Table == "tasks" {
			tx.AddError(errors.New("injected task insert failure"))
		}
	}))

	result, err := ExecuteTaskSubmissionPipeline(context.Background(), TaskSubmissionPipelineInput{
		DB: db, OperationID: fixture.Operation.ID, AttemptID: fixture.Attempt.ID,
		UserID: fixture.User.Id, TokenID: fixture.Token.Id, ChannelID: fixture.Attempt.ChannelID,
		Quota: 100, BillingSource: "wallet", BillingContext: billingContext,
		Dispatcher: func(_ context.Context, op *model.TaskSubmissionOperation, attempt *model.TaskSubmissionAttempt) (*TaskProviderDispatchResult, error) {
			return &TaskProviderDispatchResult{
				Status: TaskProviderDispatchStatusAccepted, ProviderTaskID: "insert-failure-upstream",
				TaskCandidate: acceptedTaskCandidateForTest(op, attempt, "insert-failure-upstream", 100, billingContext),
			}, nil
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, TaskProviderDispatchStatusUnknown, result.DispatchResult.Status)
	assert.Equal(t, "task_create_failed", result.DispatchResult.ErrorCode)
	assert.Equal(t, model.TaskSubmissionOperationStatusSubmissionUnknown, result.Operation.Status)
	assert.Equal(t, model.TaskSubmissionAttemptStatusSubmissionUnknown, result.Attempt.Status)
	assert.Nil(t, result.Operation.TaskID)

	var count int64
	require.NoError(t, db.Model(&model.Task{}).Count(&count).Error)
	assert.Zero(t, count)
	assert.Equal(t, "test", result.Attempt.TaskPlatform)
	assert.Equal(t, "video.create", result.Attempt.TaskAction)

	require.NoError(t, db.Callback().Create().Remove("test:reject-formal-task-create"))
	recovered, err := ResolveOperationProviderVerified(context.Background(), ProviderVerifiedResolutionInput{
		DB: db, OperationID: result.Operation.ID, ProviderStatus: TaskProviderDispatchStatusAccepted,
		ProviderOperationID: "insert-failure-upstream",
	})
	require.NoError(t, err)
	require.NotNil(t, recovered.Task)
	assert.Equal(t, "insert-failure-upstream", recovered.Task.PrivateData.UpstreamTaskID)
	assert.Equal(t, "video.create", recovered.Task.Action)
	require.NotNil(t, recovered.Task.PrivateData.BillingContext)
	_, err = model.SettleTaskQuotaReservation(db, model.TaskQuotaSettlementInput{
		OperationID: recovered.Operation.ID, UserID: fixture.User.Id, TokenID: fixture.Token.Id, ChannelID: fixture.Attempt.ChannelID,
		ExpectedOperationVersion: recovered.Operation.LockVersion, ActualQuota: 0, ReasonCode: "recovered-complete",
		BillingContext: *recovered.Task.PrivateData.BillingContext, TargetOperationStatus: model.TaskSubmissionOperationStatusSucceeded,
	})
	require.NoError(t, err)
}

func TestTaskSubmissionPipeline_AutomaticSubscriptionEligibilityEdges(t *testing.T) {
	t.Run("due reset restores quota before selection", func(t *testing.T) {
		db := setupTaskSubmissionTestDB(t)
		fixture := newTaskSubmissionTestFixture(t, db, "reset-enough", 0, 100)
		user := fixture.User
		user.SetSetting(relaykitdto.UserSetting{BillingPreference: "subscription_only"})
		require.NoError(t, db.Model(&user).Update("setting", user.Setting).Error)
		plan := model.SubscriptionPlan{Title: "daily", Enabled: true, TotalAmount: 100, DurationUnit: "month", DurationValue: 1, QuotaResetPeriod: model.SubscriptionResetDaily}
		require.NoError(t, db.Create(&plan).Error)
		sub := model.UserSubscription{UserId: user.Id, PlanId: plan.Id, AmountTotal: 100, AmountUsed: 100, Status: "active", EndTime: 1<<31 - 1, LastResetTime: 1, NextResetTime: 1, AllowWalletOverflow: true}
		require.NoError(t, db.Create(&sub).Error)
		result, err := ExecuteTaskSubmissionPipeline(context.Background(), TaskSubmissionPipelineInput{
			DB: db, OperationID: fixture.Operation.ID, AttemptID: fixture.Attempt.ID, UserID: user.Id, TokenID: fixture.Token.Id,
			ChannelID: fixture.Attempt.ChannelID, Quota: 50, BillingContext: ingressBillingContext(50),
			Dispatcher: func(context.Context, *model.TaskSubmissionOperation, *model.TaskSubmissionAttempt) (*TaskProviderDispatchResult, error) {
				return &TaskProviderDispatchResult{Status: TaskProviderDispatchStatusUnknown}, nil
			},
		})
		require.NoError(t, err)
		assert.Equal(t, "subscription", result.ReserveReceipt.BillingSource)
		require.NoError(t, db.First(&sub, sub.Id).Error)
		assert.Equal(t, int64(50), sub.AmountUsed)
	})

	t.Run("due reset remains insufficient", func(t *testing.T) {
		db := setupTaskSubmissionTestDB(t)
		fixture := newTaskSubmissionTestFixture(t, db, "reset-insufficient", 0, 100)
		user := fixture.User
		user.SetSetting(relaykitdto.UserSetting{BillingPreference: "subscription_only"})
		require.NoError(t, db.Model(&user).Update("setting", user.Setting).Error)
		plan := model.SubscriptionPlan{Title: "daily-small", Enabled: true, TotalAmount: 10, DurationUnit: "month", DurationValue: 1, QuotaResetPeriod: model.SubscriptionResetDaily}
		require.NoError(t, db.Create(&plan).Error)
		sub := model.UserSubscription{UserId: user.Id, PlanId: plan.Id, AmountTotal: 10, AmountUsed: 10, Status: "active", EndTime: 1<<31 - 1, LastResetTime: 1, NextResetTime: 1, AllowWalletOverflow: true}
		require.NoError(t, db.Create(&sub).Error)
		dispatched := false
		_, err := ExecuteTaskSubmissionPipeline(context.Background(), TaskSubmissionPipelineInput{
			DB: db, OperationID: fixture.Operation.ID, AttemptID: fixture.Attempt.ID, UserID: user.Id, TokenID: fixture.Token.Id,
			ChannelID: fixture.Attempt.ChannelID, Quota: 50, BillingContext: ingressBillingContext(50),
			Dispatcher: func(context.Context, *model.TaskSubmissionOperation, *model.TaskSubmissionAttempt) (*TaskProviderDispatchResult, error) {
				dispatched = true
				return nil, nil
			},
		})
		require.ErrorIs(t, err, model.ErrTaskQuotaReservationInsufficientQuota)
		assert.False(t, dispatched)
		require.NoError(t, db.First(&sub, sub.Id).Error)
		assert.Equal(t, int64(10), sub.AmountUsed, "failed reservation rolls back the reset with the transaction")
	})

	for _, tc := range []struct {
		name          string
		createSub     bool
		allowOverflow bool
		wallet        int
		wantSource    string
		wantErr       bool
	}{
		{name: "no subscription falls back to wallet", wallet: 100, wantSource: "wallet"},
		{name: "insufficient subscription allows wallet overflow", createSub: true, allowOverflow: true, wallet: 100, wantSource: "wallet"},
		{name: "insufficient subscription blocks wallet overflow", createSub: true, allowOverflow: false, wallet: 100, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupTaskSubmissionTestDB(t)
			fixture := newTaskSubmissionTestFixture(t, db, tc.name, tc.wallet, 100)
			user := fixture.User
			user.SetSetting(relaykitdto.UserSetting{BillingPreference: "subscription_first"})
			require.NoError(t, db.Model(&user).Update("setting", user.Setting).Error)
			if tc.createSub {
				plan := model.SubscriptionPlan{Title: "small", Enabled: true, TotalAmount: 10, DurationUnit: "month", DurationValue: 1}
				require.NoError(t, db.Create(&plan).Error)
				require.NoError(t, db.Create(&model.UserSubscription{UserId: user.Id, PlanId: plan.Id, AmountTotal: 10, Status: "active", EndTime: 1<<31 - 1, AllowWalletOverflow: tc.allowOverflow}).Error)
			}
			dispatched := false
			result, err := ExecuteTaskSubmissionPipeline(context.Background(), TaskSubmissionPipelineInput{
				DB: db, OperationID: fixture.Operation.ID, AttemptID: fixture.Attempt.ID, UserID: user.Id, TokenID: fixture.Token.Id,
				ChannelID: fixture.Attempt.ChannelID, Quota: 50, BillingContext: ingressBillingContext(50),
				Dispatcher: func(context.Context, *model.TaskSubmissionOperation, *model.TaskSubmissionAttempt) (*TaskProviderDispatchResult, error) {
					dispatched = true
					return &TaskProviderDispatchResult{Status: TaskProviderDispatchStatusUnknown}, nil
				},
			})
			if tc.wantErr {
				require.ErrorIs(t, err, model.ErrTaskQuotaReservationInsufficientQuota)
				assert.False(t, dispatched)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantSource, result.ReserveReceipt.BillingSource)
		})
	}
}

func TestTaskSubmissionPipeline_T3CASFailureRollsBackTaskInsert(t *testing.T) {
	for _, table := range []string{"task_submission_attempts", "task_submission_operations"} {
		t.Run(table, func(t *testing.T) {
			db := setupTaskSubmissionTestDB(t)
			fixture := newTaskSubmissionTestFixture(t, db, "t3-rollback-"+table, 1000, 500)
			billingContext := ingressBillingContext(100)
			_, err := ExecuteTaskSubmissionPipeline(context.Background(), TaskSubmissionPipelineInput{
				DB: db, OperationID: fixture.Operation.ID, AttemptID: fixture.Attempt.ID, UserID: fixture.User.Id, TokenID: fixture.Token.Id,
				ChannelID: fixture.Attempt.ChannelID, Quota: 100, BillingSource: "wallet", BillingContext: billingContext,
				Dispatcher: func(_ context.Context, op *model.TaskSubmissionOperation, attempt *model.TaskSubmissionAttempt) (*TaskProviderDispatchResult, error) {
					identifier := attempt.ID
					if table == "task_submission_operations" {
						identifier = op.ID
					}
					require.NoError(t, db.Exec("UPDATE "+table+" SET lock_version = lock_version + 1 WHERE id = ?", identifier).Error)
					return &TaskProviderDispatchResult{Status: TaskProviderDispatchStatusAccepted, ProviderTaskID: "t3-upstream", TaskCandidate: acceptedTaskCandidateForTest(op, attempt, "t3-upstream", 100, billingContext)}, nil
				},
			})
			require.Error(t, err)
			var count int64
			require.NoError(t, db.Model(&model.Task{}).Count(&count).Error)
			assert.Zero(t, count)
		})
	}
}

func TestTaskSubmissionPipeline_TrueFreeBypassesFundingEligibility(t *testing.T) {
	for _, brokenSubscription := range []bool{false, true} {
		name := "no_subscription"
		if brokenSubscription {
			name = "exhausted_missing_plan_pending_reset"
		}
		t.Run(name, func(t *testing.T) {
			db := setupTaskSubmissionTestDB(t)
			fixture := newTaskSubmissionTestFixture(t, db, "free-bypass-"+name, 0, 0)
			user := fixture.User
			user.SetSetting(relaykitdto.UserSetting{BillingPreference: "subscription_only"})
			require.NoError(t, db.Model(&user).Update("setting", user.Setting).Error)
			var sub model.UserSubscription
			if brokenSubscription {
				sub = model.UserSubscription{UserId: user.Id, PlanId: 999999, AmountTotal: 10, AmountUsed: 10, Status: "active", EndTime: 1<<31 - 1, NextResetTime: 1, AllowWalletOverflow: false}
				require.NoError(t, db.Create(&sub).Error)
			}
			result, err := ExecuteTaskSubmissionPipeline(context.Background(), TaskSubmissionPipelineInput{
				DB: db, OperationID: fixture.Operation.ID, AttemptID: fixture.Attempt.ID, UserID: user.Id, TokenID: fixture.Token.Id,
				ChannelID: fixture.Attempt.ChannelID, Quota: 0, FreeModel: true, BillingContext: ingressBillingContext(0),
				Dispatcher: func(context.Context, *model.TaskSubmissionOperation, *model.TaskSubmissionAttempt) (*TaskProviderDispatchResult, error) {
					return &TaskProviderDispatchResult{Status: TaskProviderDispatchStatusUnknown}, nil
				},
			})
			require.NoError(t, err)
			assert.Equal(t, int64(0), result.ReserveReceipt.Quota)
			assert.Equal(t, "wallet_only", result.ReserveReceipt.BillingPreference)
			assert.Equal(t, "wallet", result.ReserveReceipt.BillingSource)
			assert.Zero(t, result.ReserveReceipt.SubscriptionID)
			var token model.Token
			require.NoError(t, db.First(&token, fixture.Token.Id).Error)
			assert.Zero(t, token.RemainQuota)
			if brokenSubscription {
				var unchanged model.UserSubscription
				require.NoError(t, db.First(&unchanged, sub.Id).Error)
				assert.Equal(t, int64(10), unchanged.AmountUsed)
				assert.Equal(t, int64(1), unchanged.NextResetTime)
			}
		})
	}
}
