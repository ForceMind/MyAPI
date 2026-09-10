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

func TestTaskSubmissionPipeline_Accepted(t *testing.T) {
	db := setupTaskSubmissionTestDB(t)
	fixture := newTaskSubmissionTestFixture(t, db, "accepted", 1000, 500)

	dispatcherCalled := false
	dispatcher := func(ctx context.Context, op *model.TaskSubmissionOperation, attempt *model.TaskSubmissionAttempt) (*TaskProviderDispatchResult, error) {
		dispatcherCalled = true
		assert.Equal(t, model.TaskSubmissionOperationStatusDispatching, op.Status)
		assert.Equal(t, model.TaskSubmissionAttemptStatusDispatching, attempt.Status)
		return &TaskProviderDispatchResult{
			Status:         TaskProviderDispatchStatusAccepted,
			ProviderTaskID: "provider-upstream-task-999",
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
		BillingContext: model.TaskBillingContext{
			Version:         model.TaskBillingContextVersion,
			Complete:        true,
			ModelPrice:      1,
			ModelRatio:      1,
			GroupRatio:      1,
			OriginModelName: "test-model",
			PerCallBilling:  true,
		},
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
	assert.Equal(t, model.TaskStatus(model.TaskStatusSubmitted), formalTask.Status)
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
		DB:             db,
		OperationID:    fixture.Operation.ID,
		AttemptID:      fixture.Attempt.ID,
		UserID:         fixture.User.Id,
		TokenID:        fixture.Token.Id,
		ChannelID:      fixture.Attempt.ChannelID,
		Quota:          100,
		BillingSource:  "wallet",
		BillingContext: model.TaskBillingContext{
			Version:         model.TaskBillingContextVersion,
			Complete:        true,
			ModelPrice:      1,
			ModelRatio:      1,
			GroupRatio:      1,
			OriginModelName: "test-model",
			PerCallBilling:  true,
		},
		Dispatcher:     dispatcher,
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

	// Verify balances were fully refunded
	var user model.User
	var token model.Token
	require.NoError(t, db.First(&user, fixture.User.Id).Error)
	require.NoError(t, db.First(&token, fixture.Token.Id).Error)
	assert.Equal(t, 1000, user.Quota)
	assert.Equal(t, 500, token.RemainQuota)
	assert.Equal(t, 0, token.UsedQuota)

	// Verify Operation and Attempt status
	assert.Equal(t, model.TaskSubmissionOperationStatusRejected, result.Operation.Status)
	assert.Equal(t, model.TaskSubmissionAttemptStatusRejected, result.Attempt.Status)
	assert.Equal(t, "UPSTREAM_POLICY_REJECT", result.Attempt.OutcomeCode)
	assert.Equal(t, "UPSTREAM_POLICY_REJECT", result.Operation.ReasonCode)
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
		DB:             db,
		OperationID:    fixture.Operation.ID,
		AttemptID:      fixture.Attempt.ID,
		UserID:         fixture.User.Id,
		TokenID:        fixture.Token.Id,
		ChannelID:      fixture.Attempt.ChannelID,
		Quota:          100,
		BillingSource:  "wallet",
		BillingContext: model.TaskBillingContext{
			Version:         model.TaskBillingContextVersion,
			Complete:        true,
			ModelPrice:      1,
			ModelRatio:      1,
			GroupRatio:      1,
			OriginModelName: "test-model",
			PerCallBilling:  true,
		},
		Dispatcher:     dispatcher,
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
	assert.Equal(t, "STATUS_POLL_REQUIRED", result.Attempt.OutcomeCode)
	assert.Equal(t, "STATUS_POLL_REQUIRED", result.Operation.ReasonCode)
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
		DB:             db,
		OperationID:    fixture.Operation.ID,
		AttemptID:      fixture.Attempt.ID,
		UserID:         fixture.User.Id,
		TokenID:        fixture.Token.Id,
		ChannelID:      fixture.Attempt.ChannelID,
		Quota:          100,
		BillingSource:  "wallet",
		BillingContext: model.TaskBillingContext{
			Version:         model.TaskBillingContextVersion,
			Complete:        true,
			ModelPrice:      1,
			ModelRatio:      1,
			GroupRatio:      1,
			OriginModelName: "test-model",
			PerCallBilling:  true,
		},
		Dispatcher:     dispatcher,
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
		DB:             db,
		OperationID:    fixture.Operation.ID,
		AttemptID:      fixture.Attempt.ID,
		UserID:         fixture.User.Id,
		TokenID:        fixture.Token.Id,
		ChannelID:      fixture.Attempt.ChannelID,
		Quota:          100,
		BillingSource:  "wallet",
		BillingContext: model.TaskBillingContext{
			Version:         model.TaskBillingContextVersion,
			Complete:        true,
			ModelPrice:      1,
			ModelRatio:      1,
			GroupRatio:      1,
			OriginModelName: "test-model",
			PerCallBilling:  true,
		},
		Dispatcher:     dispatcher,
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
	result, err := svc.Execute(context.Background(), TaskSubmissionPipelineInput{
		OperationID:    fixture.Operation.ID,
		AttemptID:      fixture.Attempt.ID,
		UserID:         fixture.User.Id,
		TokenID:        fixture.Token.Id,
		ChannelID:      fixture.Attempt.ChannelID,
		Quota:          100,
		BillingSource:  "wallet",
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
				Status:         TaskProviderDispatchStatusAccepted,
				ProviderTaskID: "wrapper-task-id",
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
		DB:             db,
		OperationID:    fixture.Operation.ID,
		AttemptID:      fixture.Attempt.ID,
		UserID:         fixture.User.Id,
		TokenID:        fixture.Token.Id,
		ChannelID:      fixture.Attempt.ChannelID,
		Quota:          100,
		BillingSource:  "wallet",
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

	result, err := ExecuteTaskSubmissionPipeline(context.Background(), TaskSubmissionPipelineInput{
		DB:             db,
		OperationID:    fixture.Operation.ID,
		AttemptID:      fixture.Attempt.ID,
		UserID:         fixture.User.Id,
		TokenID:        fixture.Token.Id,
		ChannelID:      fixture.Attempt.ChannelID,
		Quota:          0, // Free model
		BillingSource:  "wallet",
		BillingContext: model.TaskBillingContext{
			Version:         model.TaskBillingContextVersion,
			Complete:        true,
			ModelPrice:      0,
			ModelRatio:      1,
			GroupRatio:      1,
			OriginModelName: "free-model",
			PerCallBilling:  true,
		},
		Dispatcher: func(ctx context.Context, op *model.TaskSubmissionOperation, attempt *model.TaskSubmissionAttempt) (*TaskProviderDispatchResult, error) {
			return &TaskProviderDispatchResult{
				Status:         TaskProviderDispatchStatusAccepted,
				ProviderTaskID: "free-task-1",
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
