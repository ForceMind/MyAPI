package service

import (
	"context"
	"crypto/sha256"
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
		&model.QuotaWriterEpoch{},
		&model.QuotaProjectionObligation{},
		&model.Log{},
	)
	require.NoError(t, err)
	require.NoError(t, model.EnsureQuotaWriterEpochStateWithDB(db))
	setServiceQuotaWriterMode(t, db, model.QuotaWriterModeAuthoritative, 1)
	require.NoError(t, db.Create(&model.Channel{Id: 101, Name: "resolution-test"}).Error)
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
func advanceToSubmissionUnknown(t *testing.T, db *gorm.DB, fixture resolutionTestFixture, quota int64, providerOperationIDs ...string) (model.TaskSubmissionOperation, model.TaskSubmissionAttempt) {
	t.Helper()
	reserveReceipt, err := model.ReserveTaskQuota(db, model.TaskQuotaReservationInput{
		OperationID:              fixture.Operation.ID,
		UserID:                   fixture.User.Id,
		TokenID:                  fixture.Token.Id,
		ChannelID:                fixture.Attempt.ChannelID,
		ExpectedOperationVersion: fixture.Operation.LockVersion,
		Quota:                    quota,
		ApplyStatistics:          true,
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

	providerOperationID := ""
	if len(providerOperationIDs) > 0 {
		providerOperationID = providerOperationIDs[0]
	}

	won, err = model.TransitionTaskSubmissionOperation(db, op.ID, model.TaskSubmissionOperationTransition{
		From:            model.TaskSubmissionOperationStatusDispatching,
		To:              model.TaskSubmissionOperationStatusSubmissionUnknown,
		ReasonCode:      "dispatch_timeout_fail_closed",
		ExpectedVersion: op.LockVersion,
	})
	require.NoError(t, err)
	require.True(t, won)

	won, err = model.TransitionTaskSubmissionAttempt(db, att.ID, model.TaskSubmissionAttemptTransition{
		From:                model.TaskSubmissionAttemptStatusDispatching,
		To:                  model.TaskSubmissionAttemptStatusSubmissionUnknown,
		OutcomeCode:         "dispatch_timeout_fail_closed",
		ProviderOperationID: providerOperationID,
		ExpectedVersion:     att.LockVersion,
		TaskPlatform:        "test",
		TaskAction:          "video.create",
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
	assert.Equal(t, "provider-verified-task-888", result.Task.PrivateData.UpstreamTaskID)
	assert.Equal(t, "video.create", result.Task.Action)
	require.NotNil(t, result.Task.PrivateData.BillingContext)
	require.NotNil(t, result.Operation.TaskID)
	assert.Equal(t, result.Task.ID, *result.Operation.TaskID)

	// User quota was NOT refunded on accept (stayed reserved)
	var u model.User
	require.NoError(t, db.First(&u, fixture.User.Id).Error)
	assert.Equal(t, 900, u.Quota)

	settlement, err := model.SettleTaskQuotaReservation(db, model.TaskQuotaSettlementInput{
		OperationID: op.ID, UserID: fixture.User.Id, TokenID: fixture.Token.Id, ChannelID: att.ChannelID,
		ExpectedOperationVersion: result.Operation.LockVersion, ActualQuota: 0, ReasonCode: "provider_completed_free",
		BillingContext: *result.Task.PrivateData.BillingContext, TargetOperationStatus: model.TaskSubmissionOperationStatusSucceeded,
	})
	require.NoError(t, err)
	require.NotNil(t, settlement)
	require.NoError(t, db.First(&u, fixture.User.Id).Error)
	assert.Equal(t, 1000, u.Quota)
	assert.Zero(t, u.UsedQuota)
	assert.Equal(t, 1, u.RequestCount)
}

func TestTaskResolutionService_ProviderVerified_Rejected(t *testing.T) {
	db := setupResolutionTestDB(t)
	fixture := newResolutionTestFixture(t, db, "pv-rejected", 1000, 500)
	op, _ := advanceToSubmissionUnknown(t, db, fixture, 100, "provider-rejected-proof")

	svc := NewTaskResolutionService(db)
	resolutionInput := ProviderVerifiedResolutionInput{
		OperationID:     op.ID,
		ProviderStatus:  TaskProviderDispatchStatusRejected,
		EvidenceID:      "provider-rejected-proof",
		EvidenceVersion: 1,
		ReasonCode:      "upstream_not_found",
		BillingContext: model.TaskBillingContext{
			Version:         model.TaskBillingContextVersion,
			Complete:        true,
			ModelPrice:      1,
			ModelRatio:      1,
			GroupRatio:      1,
			OriginModelName: "test-model",
			PerCallBilling:  true,
		},
	}
	result, err := svc.ResolveOperationProviderVerified(context.Background(), resolutionInput)
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
	var channel model.Channel
	require.NoError(t, db.First(&channel, fixture.Attempt.ChannelID).Error)
	assert.Zero(t, channel.UsedQuota)

	require.NotNil(t, result.ReleaseReceipt)
	assert.Equal(t, int64(100), result.ReleaseReceipt.Quota)
	assert.Empty(t, result.ReleaseReceipt.EvidenceHash)
	assert.Equal(t, 1, result.ReleaseReceipt.EvidenceVersion)
	var providerOutboxes []model.TaskBillingLogOutbox
	require.NoError(t, db.Order("id").Find(&providerOutboxes).Error)
	require.Len(t, providerOutboxes, 2)
	assert.Equal(t, model.LogTypeConsume, providerOutboxes[0].Payload.Type)
	assert.Equal(t, model.LogTypeRefund, providerOutboxes[1].Payload.Type)
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
		ApplyStatistics:          true,
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
		ActualQuota:    common.GetPointer(int64(100)), EvidenceID: "prov-op-1", EvidenceVersion: 1,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, model.TaskSubmissionOperationStatusSucceeded, result.Operation.Status)
	assert.Equal(t, model.TaskSubmissionResolutionSourceProviderVerified, result.Operation.ResolutionSource)
	terminalReceipt, err := model.FindTaskQuotaReceipt(db, op.ID, string(model.TaskBillingEventTypeTerminalSettlement), fixture.User.Id, fixture.Token.Id)
	require.NoError(t, err)
	assert.Equal(t, "prov-op-1", terminalReceipt.EvidenceID)
	assert.Equal(t, 1, terminalReceipt.EvidenceVersion)
	var evidenceOutbox model.TaskBillingLogOutbox
	require.NoError(t, db.Where("billing_event_id = ?", terminalReceipt.BillingEventID).First(&evidenceOutbox).Error)
	assert.Equal(t, "prov-op-1", evidenceOutbox.Payload.UpstreamRequestID)
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
		ApplyStatistics:          true,
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
	assert.Zero(t, u.UsedQuota)
	assert.Equal(t, 1, u.RequestCount)

	var tok model.Token
	require.NoError(t, db.First(&tok, fixture.Token.Id).Error)
	assert.Equal(t, 500, tok.RemainQuota)
	var manualOutboxes []model.TaskBillingLogOutbox
	require.NoError(t, db.Order("id").Find(&manualOutboxes).Error)
	require.Len(t, manualOutboxes, 3)
	assert.Equal(t, "audit.audit-cmd-001", result.ReleaseReceipt.EvidenceID)
	for _, outbox := range manualOutboxes {
		assert.NotEmpty(t, outbox.Payload.RequestID)
	}
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

func TestTaskResolutionService_ManualAudit_ConcurrentReplay(t *testing.T) {
	db := setupResolutionTestDB(t)
	fixture := newResolutionTestFixture(t, db, "audit-concurrent", 1000, 500)

	// Reserve 100 quota
	receipt, err := model.ReserveTaskQuota(db, model.TaskQuotaReservationInput{
		OperationID:              fixture.Operation.ID,
		UserID:                   fixture.User.Id,
		TokenID:                  fixture.Token.Id,
		ChannelID:                fixture.Attempt.ChannelID,
		ExpectedOperationVersion: fixture.Operation.LockVersion,
		Quota:                    100,
		ApplyStatistics:          true,
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
	require.NotNil(t, receipt)

	svc := NewTaskResolutionService(db)
	input := ManualAuditResolutionInput{
		OperationID:    fixture.Operation.ID,
		AuditCommandID: "concurrent-cmd-1",
		OperatorUserID: 99,
		ReasonCode:     "concurrent_audit_test",
		TargetStatus:   model.TaskSubmissionOperationStatusCanceled,
	}

	const concurrency = 5
	results := make([]*ManualAuditResolutionResult, concurrency)
	errorsList := make([]error, concurrency)
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		idx := i
		go func() {
			defer wg.Done()
			results[idx], errorsList[idx] = svc.ResolveOperationManualAudit(context.Background(), input)
		}()
	}
	wg.Wait()

	// Every concurrent goroutine must succeed and return the same canonical audit event
	for i := 0; i < concurrency; i++ {
		require.NoError(t, errorsList[i])
		require.NotNil(t, results[i])
		assert.Equal(t, "concurrent-cmd-1", results[i].BillingEvent.AuditCommandID)
	}

	// Verify balance refunded exactly once (1000 - 100 + 100 = 1000)
	var updatedUser model.User
	require.NoError(t, db.First(&updatedUser, fixture.User.Id).Error)
	assert.Equal(t, 1000, updatedUser.Quota)
}

func TestProviderVerifiedResolution_HistoricalUnknownClassificationFallback(t *testing.T) {
	db := setupResolutionTestDB(t)
	fixture := newResolutionTestFixture(t, db, "historical-classification", 1000, 500)
	op, att := advanceToSubmissionUnknown(t, db, fixture, 100)
	require.NoError(t, db.Exec("UPDATE task_submission_attempts SET task_platform = '', task_action = '' WHERE id = ?", att.ID).Error)

	_, err := ResolveOperationProviderVerified(context.Background(), ProviderVerifiedResolutionInput{
		DB: db, OperationID: op.ID, ProviderStatus: TaskProviderDispatchStatusAccepted, ProviderOperationID: "historical-upstream",
	})
	require.ErrorIs(t, err, ErrTaskResolutionInvalidInput)
	var unchanged model.TaskSubmissionOperation
	require.NoError(t, db.First(&unchanged, op.ID).Error)
	assert.Equal(t, model.TaskSubmissionOperationStatusSubmissionUnknown, unchanged.Status)

	result, err := ResolveOperationProviderVerified(context.Background(), ProviderVerifiedResolutionInput{
		DB: db, OperationID: op.ID, ProviderStatus: TaskProviderDispatchStatusAccepted, ProviderOperationID: "historical-upstream",
		TaskPlatform: "test", TaskAction: "video.create",
	})
	require.NoError(t, err)
	require.NotNil(t, result.Task)
	assert.Equal(t, "test", string(result.Task.Platform))
	assert.Equal(t, "video.create", result.Task.Action)
	require.NotNil(t, result.Task.PrivateData.BillingContext)
	_, err = model.SettleTaskQuotaReservation(db, model.TaskQuotaSettlementInput{
		OperationID: result.Operation.ID, UserID: fixture.User.Id, TokenID: fixture.Token.Id, ChannelID: att.ChannelID,
		ExpectedOperationVersion: result.Operation.LockVersion, ActualQuota: 0, ReasonCode: "historical-complete",
		BillingContext: *result.Task.PrivateData.BillingContext, TargetOperationStatus: model.TaskSubmissionOperationStatusSucceeded,
	})
	require.NoError(t, err)
}

func TestProviderVerifiedOutcomeUnknownRequiresBoundEvidence(t *testing.T) {
	for _, tc := range []struct {
		name           string
		providerStatus string
		evidenceID     string
		evidenceHash   string
		actualQuota    *int64
		wantSuccess    bool
		wantInvalid    bool
	}{
		{name: "success_missing", providerStatus: TaskProviderDispatchStatusAccepted, actualQuota: common.GetPointer(int64(100))},
		{name: "success_arbitrary_id", providerStatus: TaskProviderDispatchStatusAccepted, evidenceID: "unbound-proof", actualQuota: common.GetPointer(int64(100))},
		{name: "success_hash_only", providerStatus: TaskProviderDispatchStatusAccepted, evidenceHash: strings.Repeat("a", 64), actualQuota: common.GetPointer(int64(100))},
		{name: "success_bound_id", providerStatus: TaskProviderDispatchStatusAccepted, evidenceID: "bound-provider-id", actualQuota: common.GetPointer(int64(100)), wantSuccess: true},
		{name: "failure_bound_id_nil_actual", providerStatus: TaskProviderDispatchStatusRejected, evidenceID: "bound-provider-id", wantSuccess: true},
		{name: "failure_nonzero_actual", providerStatus: TaskProviderDispatchStatusRejected, evidenceID: "bound-provider-id", actualQuota: common.GetPointer(int64(1)), wantInvalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupResolutionTestDB(t)
			fixture := newResolutionTestFixture(t, db, "evidence-"+tc.name, 1000, 500)
			formalTask := model.Task{TaskID: fixture.Operation.PublicID, UserId: fixture.User.Id, ChannelId: fixture.Attempt.ChannelID, Status: model.TaskStatusSubmitted, Quota: 100}
			require.NoError(t, db.Create(&formalTask).Error)
			receipt, err := model.ReserveTaskQuota(db, model.TaskQuotaReservationInput{OperationID: fixture.Operation.ID, UserID: fixture.User.Id, TokenID: fixture.Token.Id, ChannelID: fixture.Attempt.ChannelID, ExpectedOperationVersion: fixture.Operation.LockVersion, Quota: 100, BillingSource: "wallet", ApplyStatistics: true, BillingContext: ingressBillingContext(100)})
			require.NoError(t, err)
			won, err := model.StartTaskSubmissionDispatch(db, fixture.Operation.ID, model.TaskSubmissionDispatchTransition{ExpectedOperationVersion: receipt.OperationVersionAfter, ExpectedAttemptVersion: fixture.Attempt.LockVersion})
			require.NoError(t, err)
			require.True(t, won)
			var op model.TaskSubmissionOperation
			var attempt model.TaskSubmissionAttempt
			require.NoError(t, db.First(&op, fixture.Operation.ID).Error)
			require.NoError(t, db.First(&attempt, fixture.Attempt.ID).Error)
			won, err = model.TransitionTaskSubmissionAttempt(db, attempt.ID, model.TaskSubmissionAttemptTransition{From: model.TaskSubmissionAttemptStatusDispatching, To: model.TaskSubmissionAttemptStatusAccepted, ProviderOperationID: "bound-provider-id", ExpectedVersion: attempt.LockVersion})
			require.NoError(t, err)
			require.True(t, won)
			won, err = model.TransitionTaskSubmissionOperation(db, op.ID, model.TaskSubmissionOperationTransition{From: model.TaskSubmissionOperationStatusDispatching, To: model.TaskSubmissionOperationStatusAccepted, TaskID: &formalTask.ID, ExpectedVersion: op.LockVersion})
			require.NoError(t, err)
			require.True(t, won)
			require.NoError(t, db.First(&op, op.ID).Error)
			won, err = model.TransitionTaskSubmissionOperation(db, op.ID, model.TaskSubmissionOperationTransition{From: model.TaskSubmissionOperationStatusAccepted, To: model.TaskSubmissionOperationStatusOutcomeUnknown, ExpectedVersion: op.LockVersion})
			require.NoError(t, err)
			require.True(t, won)
			input := ProviderVerifiedResolutionInput{DB: db, OperationID: op.ID, ProviderStatus: tc.providerStatus, ActualQuota: tc.actualQuota, EvidenceID: tc.evidenceID, EvidenceHash: tc.evidenceHash, EvidenceVersion: 1}
			result, err := ResolveOperationProviderVerified(context.Background(), input)
			if tc.wantInvalid {
				require.ErrorIs(t, err, ErrTaskResolutionInvalidInput)
				return
			}
			if tc.wantSuccess {
				require.NoError(t, err)
				if tc.providerStatus == TaskProviderDispatchStatusRejected {
					assert.Equal(t, model.TaskSubmissionOperationStatusFailed, result.Operation.Status)
					assert.Equal(t, int64(100), result.ReleaseReceipt.Quota)
				} else {
					assert.Equal(t, model.TaskSubmissionOperationStatusSucceeded, result.Operation.Status)
				}
				return
			}
			require.ErrorIs(t, err, model.ErrTaskTerminalObservationManualReview)
			var unchanged model.TaskSubmissionOperation
			require.NoError(t, db.First(&unchanged, op.ID).Error)
			assert.Equal(t, model.TaskSubmissionOperationStatusOutcomeUnknown, unchanged.Status)
		})
	}
}

func TestManualResolutionOutboxFailureRollsBackReleaseAndProjection(t *testing.T) {
	db := setupResolutionTestDB(t)
	fixture := newResolutionTestFixture(t, db, "manual-outbox-rollback", 1000, 500)
	reserve, err := model.ReserveTaskQuota(db, model.TaskQuotaReservationInput{OperationID: fixture.Operation.ID, UserID: fixture.User.Id, TokenID: fixture.Token.Id, ChannelID: fixture.Attempt.ChannelID, ExpectedOperationVersion: fixture.Operation.LockVersion, Quota: 100, BillingSource: "wallet", ApplyStatistics: true, BillingContext: ingressBillingContext(100)})
	require.NoError(t, err)
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:manual-resolution-outbox", func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Schema != nil && tx.Statement.Schema.Table == "task_billing_log_outboxes" {
			if candidate, ok := tx.Statement.Dest.(*model.TaskBillingLogOutbox); ok && candidate.Payload.ModelName == "manual_resolution" {
				tx.AddError(errors.New("manual outbox failure"))
			}
		}
	}))
	_, err = ResolveOperationManualAudit(context.Background(), ManualAuditResolutionInput{DB: db, OperationID: fixture.Operation.ID, AuditCommandID: "audit-outbox-failure", OperatorUserID: 99, ReasonCode: "manual_audit_cancel", TargetStatus: model.TaskSubmissionOperationStatusCanceled, BillingContext: ingressBillingContext(100)})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "manual resolution outbox")
	var operation model.TaskSubmissionOperation
	var user model.User
	var token model.Token
	require.NoError(t, db.First(&operation, fixture.Operation.ID).Error)
	require.NoError(t, db.First(&user, fixture.User.Id).Error)
	require.NoError(t, db.First(&token, fixture.Token.Id).Error)
	assert.Equal(t, model.TaskSubmissionOperationStatusReserved, operation.Status)
	assert.Equal(t, 900, user.Quota)
	assert.Equal(t, 100, user.UsedQuota)
	assert.Equal(t, 400, token.RemainQuota)
	var refundCount int64
	require.NoError(t, db.Model(&model.QuotaMutationReceipt{}).Where("operation_id = ? AND mutation_type = ?", fixture.Operation.ID, model.TaskBillingEventTypeRefund).Count(&refundCount).Error)
	assert.Zero(t, refundCount)
	var reserveOutboxCount int64
	require.NoError(t, db.Model(&model.TaskBillingLogOutbox{}).Where("billing_event_id = ?", reserve.BillingEventID).Count(&reserveOutboxCount).Error)
	assert.Equal(t, int64(1), reserveOutboxCount)
}

func TestProviderVerifiedSubmissionUnknownFailureWithoutEvidenceStaysUnknown(t *testing.T) {
	db := setupResolutionTestDB(t)
	fixture := newResolutionTestFixture(t, db, "rejected-no-evidence", 1000, 500)
	op, _ := advanceToSubmissionUnknown(t, db, fixture, 100)
	_, err := ResolveOperationProviderVerified(context.Background(), ProviderVerifiedResolutionInput{DB: db, OperationID: op.ID, ProviderStatus: TaskProviderDispatchStatusRejected, ReasonCode: "provider_rejected"})
	require.ErrorIs(t, err, model.ErrTaskTerminalObservationManualReview)
	var unchanged model.TaskSubmissionOperation
	require.NoError(t, db.First(&unchanged, op.ID).Error)
	assert.Equal(t, model.TaskSubmissionOperationStatusSubmissionUnknown, unchanged.Status)
	var user model.User
	require.NoError(t, db.First(&user, fixture.User.Id).Error)
	assert.Equal(t, 900, user.Quota)
	assert.Equal(t, 100, user.UsedQuota)
	assert.Equal(t, 1, user.RequestCount)
	var observation model.TaskTerminalObservation
	require.NoError(t, db.Where("operation_id = ?", op.ID).First(&observation).Error)
	assert.Equal(t, int64(0), observation.TaskID)
	assert.Equal(t, model.TaskTerminalObservationManualReview, observation.State)
}
