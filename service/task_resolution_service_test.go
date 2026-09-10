package service

import (
	"context"
	"crypto/sha256"
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

func setupResolutionTestDB(t *testing.T) *gorm.DB {
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

type resolutionTestFixture struct {
	User      model.User
	Token     model.Token
	Operation model.TaskSubmissionOperation
	Attempt   model.TaskSubmissionAttempt
}

func newResolutionTestFixture(t *testing.T, db *gorm.DB, label string, userQuota, tokenQuota int) resolutionTestFixture {
	t.Helper()
	digest := sha256.Sum256([]byte(t.Name() + ":" + label))
	name := fmt.Sprintf("res-%x", digest[:8])

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
	fingerprint := model.FingerprintTaskSubmissionRequest([]byte(`{"prompt":"resolution-test"}`))

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

	return resolutionTestFixture{
		User:      user,
		Token:     token,
		Operation: *intent.Operation,
		Attempt:   *intent.Attempt,
	}
}

// advanceToSubmissionUnknown advances fixture operation & attempt to submission_unknown with reserved quota
func advanceToSubmissionUnknown(t *testing.T, db *gorm.DB, fixture resolutionTestFixture, quota int64) (model.TaskSubmissionOperation, model.TaskSubmissionAttempt) {
	t.Helper()
	reserveReceipt, err := model.ReserveTaskQuota(db, model.TaskQuotaReservationInput{
		OperationID:              fixture.Operation.ID,
		UserID:                   fixture.User.Id,
		TokenID:                  fixture.Token.Id,
		ChannelID:                fixture.Attempt.ChannelID,
		ExpectedOperationVersion: fixture.Operation.LockVersion,
		Quota:                    quota,
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
		ExpectedOperationVersion: reserveReceipt.OperationVersionAfter,
		ExpectedAttemptVersion:   fixture.Attempt.LockVersion,
	})
	require.NoError(t, err)
	require.True(t, won)

	var op model.TaskSubmissionOperation
	require.NoError(t, db.First(&op, fixture.Operation.ID).Error)
	var att model.TaskSubmissionAttempt
	require.NoError(t, db.First(&att, fixture.Attempt.ID).Error)

	won, err = model.TransitionTaskSubmissionOperation(db, op.ID, model.TaskSubmissionOperationTransition{
		From:            model.TaskSubmissionOperationStatusDispatching,
		To:              model.TaskSubmissionOperationStatusSubmissionUnknown,
		ReasonCode:      "dispatch_timeout_fail_closed",
		ExpectedVersion: op.LockVersion,
	})
	require.NoError(t, err)
	require.True(t, won)

	won, err = model.TransitionTaskSubmissionAttempt(db, att.ID, model.TaskSubmissionAttemptTransition{
		From:            model.TaskSubmissionAttemptStatusDispatching,
		To:              model.TaskSubmissionAttemptStatusSubmissionUnknown,
		OutcomeCode:     "dispatch_timeout_fail_closed",
		ExpectedVersion: att.LockVersion,
	})
	require.NoError(t, err)
	require.True(t, won)

	require.NoError(t, db.First(&op, fixture.Operation.ID).Error)
	require.NoError(t, db.First(&att, fixture.Attempt.ID).Error)
	return op, att
}

func TestTaskResolutionService_ProviderVerified_Accepted(t *testing.T) {
	db := setupResolutionTestDB(t)
	fixture := newResolutionTestFixture(t, db, "pv-accepted", 1000, 500)
	op, att := advanceToSubmissionUnknown(t, db, fixture, 100)

	svc := NewTaskResolutionService(db)
	result, err := svc.ResolveOperationProviderVerified(context.Background(), ProviderVerifiedResolutionInput{
		OperationID:         op.ID,
		ProviderStatus:      TaskProviderDispatchStatusAccepted,
		ProviderOperationID: "provider-verified-task-888",
		UpstreamRequestID:   "req-upstream-xyz",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, model.TaskSubmissionOperationStatusAccepted, result.Operation.Status)
	assert.Equal(t, model.TaskSubmissionAttemptStatusAccepted, result.Attempt.Status)
	assert.Equal(t, "provider-verified-task-888", result.Attempt.ProviderOperationID)
	assert.Equal(t, "req-upstream-xyz", result.Attempt.UpstreamRequestID)
	assert.Equal(t, model.TaskSubmissionResolutionSourceProviderVerified, result.Operation.ResolutionSource)

	// Verify formal task was created and linked
	require.NotNil(t, result.Task)
	assert.Equal(t, op.PublicID, result.Task.TaskID)
	assert.Equal(t, fixture.User.Id, result.Task.UserId)
	assert.Equal(t, att.ChannelID, result.Task.ChannelId)
	require.NotNil(t, result.Operation.TaskID)
	assert.Equal(t, result.Task.ID, *result.Operation.TaskID)

	// User quota was NOT refunded on accept (stayed reserved)
	var u model.User
	require.NoError(t, db.First(&u, fixture.User.Id).Error)
	assert.Equal(t, 900, u.Quota)
}

func TestTaskResolutionService_ProviderVerified_Rejected(t *testing.T) {
	db := setupResolutionTestDB(t)
	fixture := newResolutionTestFixture(t, db, "pv-rejected", 1000, 500)
	op, _ := advanceToSubmissionUnknown(t, db, fixture, 100)

	svc := NewTaskResolutionService(db)
	result, err := svc.ResolveOperationProviderVerified(context.Background(), ProviderVerifiedResolutionInput{
		OperationID:    op.ID,
		ProviderStatus: TaskProviderDispatchStatusRejected,
		ReasonCode:     "upstream_not_found",
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
	require.NotNil(t, result)
	assert.Equal(t, model.TaskSubmissionOperationStatusRejected, result.Operation.Status)
	assert.Equal(t, model.TaskSubmissionAttemptStatusRejected, result.Attempt.Status)
	assert.Equal(t, "upstream_not_found", result.Attempt.OutcomeCode)
	assert.Equal(t, model.TaskSubmissionResolutionSourceProviderVerified, result.Operation.ResolutionSource)

	// Verify quota was fully released back!
	var u model.User
	require.NoError(t, db.First(&u, fixture.User.Id).Error)
	assert.Equal(t, 1000, u.Quota)

	var tok model.Token
	require.NoError(t, db.First(&tok, fixture.Token.Id).Error)
	assert.Equal(t, 500, tok.RemainQuota)
	assert.Equal(t, 0, tok.UsedQuota)

	require.NotNil(t, result.ReleaseReceipt)
	assert.Equal(t, int64(100), result.ReleaseReceipt.Quota)
}

func TestTaskResolutionService_ProviderVerified_OutcomeUnknown(t *testing.T) {
	db := setupResolutionTestDB(t)
	fixture := newResolutionTestFixture(t, db, "pv-outcome-unknown", 1000, 500)

	// Setup operation in outcome_unknown
	formalTask := model.Task{
		TaskID:    fixture.Operation.PublicID,
		UserId:    fixture.User.Id,
		ChannelId: fixture.Attempt.ChannelID,
		Status:    model.TaskStatusSubmitted,
	}
	require.NoError(t, db.Create(&formalTask).Error)

	// Reserve and start dispatch
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

	var op model.TaskSubmissionOperation
	require.NoError(t, db.First(&op, fixture.Operation.ID).Error)
	var att model.TaskSubmissionAttempt
	require.NoError(t, db.First(&att, fixture.Attempt.ID).Error)

	won, err = model.TransitionTaskSubmissionAttempt(db, att.ID, model.TaskSubmissionAttemptTransition{
		From:                model.TaskSubmissionAttemptStatusDispatching,
		To:                  model.TaskSubmissionAttemptStatusAccepted,
		ProviderOperationID: "prov-op-1",
		ExpectedVersion:     att.LockVersion,
	})
	require.NoError(t, err)
	require.True(t, won)

	won, err = model.TransitionTaskSubmissionOperation(db, op.ID, model.TaskSubmissionOperationTransition{
		From:            model.TaskSubmissionOperationStatusDispatching,
		To:              model.TaskSubmissionOperationStatusAccepted,
		TaskID:          &formalTask.ID,
		ExpectedVersion: op.LockVersion,
	})
	require.NoError(t, err)
	require.True(t, won)

	require.NoError(t, db.First(&op, op.ID).Error)
	won, err = model.TransitionTaskSubmissionOperation(db, op.ID, model.TaskSubmissionOperationTransition{
		From:            model.TaskSubmissionOperationStatusAccepted,
		To:              model.TaskSubmissionOperationStatusOutcomeUnknown,
		ExpectedVersion: op.LockVersion,
	})
	require.NoError(t, err)
	require.True(t, won)

	// Resolve outcome_unknown with provider status accepted -> succeeded
	svc := NewTaskResolutionService(db)
	result, err := svc.ResolveOperationProviderVerified(context.Background(), ProviderVerifiedResolutionInput{
		OperationID:    op.ID,
		ProviderStatus: TaskProviderDispatchStatusAccepted,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, model.TaskSubmissionOperationStatusSucceeded, result.Operation.Status)
	assert.Equal(t, model.TaskSubmissionResolutionSourceProviderVerified, result.Operation.ResolutionSource)
}

func TestTaskResolutionService_ManualAudit_FullFlow_Reserved(t *testing.T) {
	db := setupResolutionTestDB(t)
	fixture := newResolutionTestFixture(t, db, "audit-reserved", 1000, 500)

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

	svc := NewTaskResolutionService(db)
	result, err := svc.ResolveOperationManualAudit(context.Background(), ManualAuditResolutionInput{
		OperationID:    op.ID,
		AuditCommandID: "audit-cmd-001",
		OperatorUserID: 99,
		ReasonCode:     "manual_audit_cancel",
		TargetStatus:   model.TaskSubmissionOperationStatusCanceled,
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
	require.NotNil(t, result)
	assert.Equal(t, model.TaskSubmissionOperationStatusCanceled, result.Operation.Status)

	// Quota refunded
	var u model.User
	require.NoError(t, db.First(&u, fixture.User.Id).Error)
	assert.Equal(t, 1000, u.Quota)

	var tok model.Token
	require.NoError(t, db.First(&tok, fixture.Token.Id).Error)
	assert.Equal(t, 500, tok.RemainQuota)
	assert.Equal(t, 0, tok.UsedQuota)

	// Assert immutable audit TaskBillingEvent recorded
	require.NotNil(t, result.BillingEvent)
	assert.Equal(t, model.TaskBillingEventTypeManualResolution, result.BillingEvent.EventType)
	assert.Equal(t, "audit-cmd-001", result.BillingEvent.AuditCommandID)
	assert.Equal(t, model.TaskSubmissionResolutionSourceManualAudit, result.BillingEvent.ResolutionSource)
	assert.Equal(t, "manual_audit_cancel", result.BillingEvent.ReasonCode)
}

func TestTaskResolutionService_ManualAudit_FullFlow_SubmissionUnknown(t *testing.T) {
	db := setupResolutionTestDB(t)
	fixture := newResolutionTestFixture(t, db, "audit-unknown", 1000, 500)
	op, _ := advanceToSubmissionUnknown(t, db, fixture, 100)

	svc := NewTaskResolutionService(db)
	result, err := svc.ResolveOperationManualAudit(context.Background(), ManualAuditResolutionInput{
		OperationID:    op.ID,
		AuditCommandID: "audit-cmd-sub-unknown-1",
		OperatorUserID: 1,
		ReasonCode:     "admin_manual_reject",
		TargetStatus:   model.TaskSubmissionOperationStatusRejected,
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
	require.NotNil(t, result)
	assert.Equal(t, model.TaskSubmissionOperationStatusRejected, result.Operation.Status)
	assert.Equal(t, model.TaskSubmissionAttemptStatusRejected, result.Attempt.Status)

	// Quota refunded
	var u model.User
	require.NoError(t, db.First(&u, fixture.User.Id).Error)
	assert.Equal(t, 1000, u.Quota)

	require.NotNil(t, result.BillingEvent)
	assert.Equal(t, "audit-cmd-sub-unknown-1", result.BillingEvent.AuditCommandID)
	assert.Equal(t, model.TaskSubmissionResolutionSourceManualAudit, result.BillingEvent.ResolutionSource)
}

func TestTaskResolutionService_ManualAudit_IdempotentReplay(t *testing.T) {
	db := setupResolutionTestDB(t)
	fixture := newResolutionTestFixture(t, db, "audit-replay", 1000, 500)
	op, _ := advanceToSubmissionUnknown(t, db, fixture, 100)

	svc := NewTaskResolutionService(db)
	input := ManualAuditResolutionInput{
		OperationID:    op.ID,
		AuditCommandID: "cmd-replay-100",
		OperatorUserID: 1,
		ReasonCode:     "manual_reject",
		TargetStatus:   model.TaskSubmissionOperationStatusRejected,
	}

	firstResult, err := svc.ResolveOperationManualAudit(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, firstResult)
	assert.Equal(t, model.TaskSubmissionOperationStatusRejected, firstResult.Operation.Status)

	// Repeat exact same call
	secondResult, err := svc.ResolveOperationManualAudit(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, secondResult)
	assert.Equal(t, firstResult.Operation.ID, secondResult.Operation.ID)
	assert.Equal(t, firstResult.BillingEvent.EventKey, secondResult.BillingEvent.EventKey)

	// Verify user quota was refunded once, not twice
	var u model.User
	require.NoError(t, db.First(&u, fixture.User.Id).Error)
	assert.Equal(t, 1000, u.Quota)
}

func TestTaskResolutionService_ManualAudit_ConflictRejected(t *testing.T) {
	db := setupResolutionTestDB(t)
	fixture := newResolutionTestFixture(t, db, "audit-conflict", 1000, 500)
	op, _ := advanceToSubmissionUnknown(t, db, fixture, 100)

	svc := NewTaskResolutionService(db)
	input := ManualAuditResolutionInput{
		OperationID:    op.ID,
		AuditCommandID: "cmd-conflict-test",
		OperatorUserID: 1,
		ReasonCode:     "initial_reason",
		TargetStatus:   model.TaskSubmissionOperationStatusRejected,
	}

	_, err := svc.ResolveOperationManualAudit(context.Background(), input)
	require.NoError(t, err)

	// Conflicting reason code
	conflictingInput := input
	conflictingInput.ReasonCode = "conflicting_different_reason"
	_, err = svc.ResolveOperationManualAudit(context.Background(), conflictingInput)
	assert.ErrorIs(t, err, ErrTaskResolutionConflict)

	// Conflicting target status
	conflictingStatusInput := input
	conflictingStatusInput.TargetStatus = model.TaskSubmissionOperationStatusCanceled
	_, err = svc.ResolveOperationManualAudit(context.Background(), conflictingStatusInput)
	assert.Error(t, err)
}

func TestTaskResolutionService_ManualAudit_ValidationErrors(t *testing.T) {
	db := setupResolutionTestDB(t)
	fixture := newRecoveryTestFixture(t, db, "audit-validation", 1000, 500)
	svc := NewTaskResolutionService(db)

	testCases := []struct {
		name        string
		input       ManualAuditResolutionInput
		expectedErr error
	}{
		{
			name: "empty audit command id",
			input: ManualAuditResolutionInput{
				OperationID:    fixture.Operation.ID,
				AuditCommandID: "",
				OperatorUserID: 1,
				ReasonCode:     "reason",
				TargetStatus:   model.TaskSubmissionOperationStatusCanceled,
			},
			expectedErr: ErrTaskResolutionInvalidInput,
		},
		{
			name: "audit command id with invalid uppercase/symbols",
			input: ManualAuditResolutionInput{
				OperationID:    fixture.Operation.ID,
				AuditCommandID: "INVALID!CMD@#$",
				OperatorUserID: 1,
				ReasonCode:     "reason",
				TargetStatus:   model.TaskSubmissionOperationStatusCanceled,
			},
			expectedErr: ErrTaskResolutionInvalidInput,
		},
		{
			name: "audit command id too long",
			input: ManualAuditResolutionInput{
				OperationID:    fixture.Operation.ID,
				AuditCommandID: strings.Repeat("a", 49),
				OperatorUserID: 1,
				ReasonCode:     "reason",
				TargetStatus:   model.TaskSubmissionOperationStatusCanceled,
			},
			expectedErr: ErrTaskResolutionInvalidInput,
		},
		{
			name: "zero operator user id",
			input: ManualAuditResolutionInput{
				OperationID:    fixture.Operation.ID,
				AuditCommandID: "valid-cmd-1",
				OperatorUserID: 0,
				ReasonCode:     "reason",
				TargetStatus:   model.TaskSubmissionOperationStatusCanceled,
			},
			expectedErr: ErrTaskResolutionInvalidInput,
		},
		{
			name: "empty reason code",
			input: ManualAuditResolutionInput{
				OperationID:    fixture.Operation.ID,
				AuditCommandID: "valid-cmd-1",
				OperatorUserID: 1,
				ReasonCode:     "",
				TargetStatus:   model.TaskSubmissionOperationStatusCanceled,
			},
			expectedErr: ErrTaskResolutionInvalidInput,
		},
		{
			name: "unsupported target status",
			input: ManualAuditResolutionInput{
				OperationID:    fixture.Operation.ID,
				AuditCommandID: "valid-cmd-1",
				OperatorUserID: 1,
				ReasonCode:     "reason",
				TargetStatus:   model.TaskSubmissionOperationStatusAccepted,
			},
			expectedErr: ErrTaskResolutionInvalidInput,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.ResolveOperationManualAudit(context.Background(), tc.input)
			assert.ErrorIs(t, err, tc.expectedErr)
		})
	}
}
