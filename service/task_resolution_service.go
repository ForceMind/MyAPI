package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/logger"
	"github.com/ForceMind/MyAPI/model"
	"gorm.io/gorm"
)

var (
	ErrTaskResolutionInvalidInput      = errors.New("invalid task resolution input")
	ErrTaskResolutionConflict          = errors.New("task resolution audit command conflict")
	ErrTaskResolutionCASLost           = errors.New("task resolution compare-and-swap lost")
	ErrTaskResolutionInvalidTransition = errors.New("invalid task resolution state transition")
)

// ProviderVerifiedResolutionInput defines the parameters for upstream-verified task resolution.
type ProviderVerifiedResolutionInput struct {
	DB                  *gorm.DB
	OperationID         int64
	ProviderStatus      string // "accepted" or "rejected"
	ProviderOperationID string
	UpstreamRequestID   string
	ReasonCode          string
	BillingContext      model.TaskBillingContext
	TaskPlatform        string
	TaskAction          string
	ActualQuota         *int64
	EvidenceID          string
	EvidenceHash        string
	EvidenceVersion     int
}

// ProviderVerifiedResolutionResult holds the result of a provider-verified resolution.
type ProviderVerifiedResolutionResult struct {
	Operation      *model.TaskSubmissionOperation
	Attempt        *model.TaskSubmissionAttempt
	Task           *model.Task
	ReleaseReceipt *model.QuotaMutationReceipt
}

// ManualAuditResolutionInput defines the parameters for administrator manual audit resolution.
type ManualAuditResolutionInput struct {
	DB             *gorm.DB
	OperationID    int64
	AuditCommandID string // 1-48 characters
	OperatorUserID int
	ReasonCode     string
	TargetStatus   model.TaskSubmissionOperationStatus // "rejected" or "canceled"
	BillingContext model.TaskBillingContext
}

// ManualAuditResolutionResult holds the result of a manual audit resolution.
type ManualAuditResolutionResult struct {
	Operation      *model.TaskSubmissionOperation
	Attempt        *model.TaskSubmissionAttempt
	BillingEvent   *model.TaskBillingEvent
	ReleaseReceipt *model.QuotaMutationReceipt
}

// TaskResolutionService provides deterministic resolution for unknown task submission operations.
type TaskResolutionService struct {
	DB *gorm.DB
}

// NewTaskResolutionService creates a new TaskResolutionService.
func NewTaskResolutionService(db *gorm.DB) *TaskResolutionService {
	return &TaskResolutionService{DB: db}
}

// ResolveOperationProviderVerified resolves submission_unknown or outcome_unknown operations
// based on upstream provider confirmation:
// - Accepted: associates a formal Task, transitions attempt & operation CAS to accepted.
// - Rejected: releases reserved quota via model.ReleaseTaskQuotaReservation and transitions attempt & operation CAS to rejected.
func (s *TaskResolutionService) ResolveOperationProviderVerified(ctx context.Context, input ProviderVerifiedResolutionInput) (*ProviderVerifiedResolutionResult, error) {
	if input.DB == nil {
		input.DB = s.DB
	}
	return ResolveOperationProviderVerified(ctx, input)
}

// ResolveOperationProviderVerified is the package-level handler for upstream-verified resolution.
func ResolveOperationProviderVerified(ctx context.Context, input ProviderVerifiedResolutionInput) (*ProviderVerifiedResolutionResult, error) {
	db := input.DB
	if db == nil {
		db = model.DB
	}
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	if input.OperationID <= 0 {
		return nil, fmt.Errorf("%w: invalid operation id", ErrTaskResolutionInvalidInput)
	}

	providerStatus := strings.ToLower(strings.TrimSpace(input.ProviderStatus))
	if providerStatus != TaskProviderDispatchStatusAccepted && providerStatus != TaskProviderDispatchStatusRejected {
		return nil, fmt.Errorf("%w: provider status must be accepted or rejected", ErrTaskResolutionInvalidInput)
	}

	var op model.TaskSubmissionOperation
	if err := db.Where("id = ?", input.OperationID).First(&op).Error; err != nil {
		return nil, fmt.Errorf("load operation failed: %w", err)
	}

	attempt, err := model.FindAttemptByOperationID(db, op.ID)
	if err != nil {
		return nil, fmt.Errorf("find attempt failed: %w", err)
	}
	if attempt == nil {
		return nil, fmt.Errorf("%w: attempt not found for operation %d", ErrTaskResolutionInvalidInput, op.ID)
	}

	if op.Status != model.TaskSubmissionOperationStatusSubmissionUnknown &&
		op.Status != model.TaskSubmissionOperationStatusOutcomeUnknown {
		return nil, fmt.Errorf("%w: operation status is %s, expected submission_unknown or outcome_unknown", ErrTaskResolutionInvalidTransition, op.Status)
	}

	if op.Status == model.TaskSubmissionOperationStatusSubmissionUnknown &&
		providerStatus == TaskProviderDispatchStatusAccepted &&
		strings.TrimSpace(input.ProviderOperationID) == "" {
		return nil, fmt.Errorf("%w: accepted provider resolution requires provider operation id", ErrTaskResolutionInvalidInput)
	}
	resolvedPlatform := attempt.TaskPlatform
	resolvedAction := attempt.TaskAction
	if resolvedPlatform == "" && resolvedAction == "" {
		resolvedPlatform = strings.ToLower(strings.TrimSpace(input.TaskPlatform))
		resolvedAction = strings.TrimSpace(input.TaskAction)
	} else if (input.TaskPlatform != "" && strings.ToLower(strings.TrimSpace(input.TaskPlatform)) != resolvedPlatform) ||
		(input.TaskAction != "" && strings.TrimSpace(input.TaskAction) != resolvedAction) {
		return nil, fmt.Errorf("%w: recovery classification conflicts with durable attempt", ErrTaskResolutionConflict)
	}
	if providerStatus == TaskProviderDispatchStatusAccepted && op.Status == model.TaskSubmissionOperationStatusSubmissionUnknown {
		if !validTaskRecoveryClassification(resolvedPlatform, resolvedAction) {
			return nil, fmt.Errorf("%w: accepted recovery requires a valid task platform and action", ErrTaskResolutionInvalidInput)
		}
	}

	reasonCode := sanitizeReasonCode(input.ReasonCode, "provider_verified_terminal")
	if strings.TrimSpace(input.ReasonCode) != "" && reasonCode == "provider_verified_terminal" && strings.ToLower(strings.TrimSpace(input.ReasonCode)) != reasonCode {
		logger.LogWarn(ctx, "unsafe provider resolution reason replaced with provider_verified_terminal")
	}
	if op.Status == model.TaskSubmissionOperationStatusSubmissionUnknown && providerStatus == TaskProviderDispatchStatusRejected && !validProviderTerminalEvidence(input, &op, attempt) {
		_, observationErr := model.CreateOrLoadTaskTerminalObservation(db, model.TaskTerminalObservationInput{OperationID: op.ID, TaskID: 0, Outcome: "failed", ActualQuota: 0, ReasonCode: reasonCode, RequestID: op.RequestID, ManualReview: true, EvidenceID: strings.TrimSpace(input.EvidenceID), EvidenceHash: strings.ToLower(strings.TrimSpace(input.EvidenceHash)), EvidenceVersion: input.EvidenceVersion})
		if observationErr != nil {
			return nil, observationErr
		}
		return nil, model.ErrTaskTerminalObservationManualReview
	}
	if op.Status == model.TaskSubmissionOperationStatusOutcomeUnknown {
		if op.TaskID == nil || *op.TaskID <= 0 {
			return nil, fmt.Errorf("%w: outcome_unknown operation has no task", ErrTaskResolutionInvalidInput)
		}
		var task model.Task
		if err := db.First(&task, *op.TaskID).Error; err != nil {
			return nil, err
		}
		outcome := "succeeded"
		actualQuota := int64(0)
		validEvidence := validProviderTerminalEvidence(input, &op, attempt)
		if providerStatus == TaskProviderDispatchStatusAccepted {
			validEvidence = validEvidence && input.ActualQuota != nil && *input.ActualQuota >= 0 && *input.ActualQuota <= int64(common.MaxQuota)
			if validEvidence {
				actualQuota = *input.ActualQuota
			}
		} else {
			outcome = "failed"
			if input.ActualQuota != nil && *input.ActualQuota != 0 {
				return nil, fmt.Errorf("%w: rejected terminal outcome requires zero actual quota", ErrTaskResolutionInvalidInput)
			}
		}
		evidenceID := strings.TrimSpace(input.EvidenceID)
		evidenceHash := strings.ToLower(strings.TrimSpace(input.EvidenceHash))
		observation, err := model.CreateOrLoadTaskTerminalObservation(db, model.TaskTerminalObservationInput{OperationID: op.ID, TaskID: task.ID, Outcome: outcome, ActualQuota: actualQuota, ReasonCode: reasonCode, ResolutionSource: model.TaskSubmissionResolutionSourceProviderVerified, EvidenceID: evidenceID, EvidenceHash: evidenceHash, EvidenceVersion: input.EvidenceVersion, ManualReview: !validEvidence, RequestID: op.RequestID, TaskData: terminalTaskDataProjection(task.Data), TaskStartTime: task.StartTime, TaskFinishTime: task.FinishTime, OperationalResultURL: terminalOperationalResultURL(task.PrivateData.ResultURL), TaskUpstreamID: strings.TrimSpace(task.PrivateData.UpstreamTaskID), ResultURL: sanitizeTerminalResultURL(task.PrivateData.ResultURL)})
		if err != nil {
			return nil, err
		}
		if !validEvidence {
			return nil, model.ErrTaskTerminalObservationManualReview
		}
		applied, err := model.ApplyTaskTerminalObservation(db, observation.ID)
		if err != nil {
			return nil, err
		}
		if applied != nil && applied.Receipt != nil {
			_ = model.ProjectQuotaMutationReceipt(ctx, db, applied.Receipt)
		}
		if err := db.First(&op, op.ID).Error; err != nil {
			return nil, err
		}
		if err := db.First(&task, task.ID).Error; err != nil {
			return nil, err
		}
		return &ProviderVerifiedResolutionResult{Operation: &op, Attempt: attempt, Task: &task, ReleaseReceipt: func() *model.QuotaMutationReceipt {
			if outcome == "failed" {
				return applied.Receipt
			}
			return nil
		}()}, nil
	}

	var formalTask *model.Task
	var releaseReceipt *model.QuotaMutationReceipt

	err = db.Transaction(func(tx *gorm.DB) error {
		if op.Status == model.TaskSubmissionOperationStatusSubmissionUnknown {
			if providerStatus == TaskProviderDispatchStatusAccepted {
				reserveReceipt, err := model.FindTaskQuotaReservation(tx, op.ID, op.UserID, op.TokenID)
				if err != nil {
					return fmt.Errorf("load task reservation for accepted recovery failed: %w", err)
				}
				billingContext := model.TaskBillingContext(reserveReceipt.BillingContext)
				billingContextCopy := billingContext
				task := model.Task{
					TaskID: op.PublicID, Platform: constant.TaskPlatform(resolvedPlatform), UserId: op.UserID,
					Group: reserveReceipt.Before.User.Group, ChannelId: attempt.ChannelID, Quota: TaskInitialQuota(reserveReceipt.RequestFingerprintVersion, reserveReceipt.EstimatedQuota, reserveReceipt.Quota),
					Action: resolvedAction, Status: model.TaskStatusNotStart, SubmitTime: op.CreatedAt, Progress: "0%",
					Properties: model.Properties{OriginModelName: billingContext.OriginModelName},
					PrivateData: model.TaskPrivateData{
						UpstreamTaskID: strings.TrimSpace(input.ProviderOperationID), BillingPreference: reserveReceipt.BillingPreference,
						BillingSource: reserveReceipt.BillingSource, SubscriptionId: reserveReceipt.SubscriptionID,
						FreeModel: reserveReceipt.FreeModel, TokenId: op.TokenID, BillingContext: &billingContextCopy,
					},
				}
				if err := tx.Create(&task).Error; err != nil {
					return fmt.Errorf("create complete formal task failed: %w", err)
				}
				formalTask = &task

				opWon, err := model.TransitionTaskSubmissionOperation(tx, op.ID, model.TaskSubmissionOperationTransition{
					From:             model.TaskSubmissionOperationStatusSubmissionUnknown,
					To:               model.TaskSubmissionOperationStatusAccepted,
					TaskID:           &task.ID,
					ResolutionSource: model.TaskSubmissionResolutionSourceProviderVerified,
					ExpectedVersion:  op.LockVersion,
				})
				if err != nil {
					return fmt.Errorf("transition operation to accepted failed: %w", err)
				}
				if !opWon {
					return fmt.Errorf("%w: operation accepted CAS lost", ErrTaskResolutionCASLost)
				}

				attemptWon, err := model.TransitionTaskSubmissionAttempt(tx, attempt.ID, model.TaskSubmissionAttemptTransition{
					From:                model.TaskSubmissionAttemptStatusSubmissionUnknown,
					To:                  model.TaskSubmissionAttemptStatusAccepted,
					ProviderOperationID: strings.TrimSpace(input.ProviderOperationID),
					UpstreamRequestID:   strings.TrimSpace(input.UpstreamRequestID),
					TaskPlatform:        resolvedPlatform,
					TaskAction:          resolvedAction,
					ExpectedVersion:     attempt.LockVersion,
				})
				if err != nil {
					return fmt.Errorf("transition attempt to accepted failed: %w", err)
				}
				if !attemptWon {
					return fmt.Errorf("%w: attempt accepted CAS lost", ErrTaskResolutionCASLost)
				}

			} else {
				reserveEvidence, evidenceErr := model.FindTaskQuotaReceipt(tx, op.ID, string(model.TaskBillingEventTypeReserve), op.UserID, op.TokenID)
				if evidenceErr != nil || reserveEvidence == nil {
					return model.ErrTaskTerminalObservationManualReview
				}
				billingContext := model.TaskBillingContext(reserveEvidence.BillingContext)
				channelID := reserveEvidence.ChannelID
				var relErr error
				releaseReceipt, relErr = model.ReleaseTaskQuotaReservation(tx, model.TaskQuotaReleaseInput{
					OperationID:               op.ID,
					UserID:                    op.UserID,
					TokenID:                   op.TokenID,
					ChannelID:                 channelID,
					ExpectedOperationVersion:  op.LockVersion,
					ReasonCode:                reasonCode,
					BillingContext:            billingContext,
					TargetOperationStatus:     "",
					ResolutionSource:          model.TaskSubmissionResolutionSourceProviderVerified,
					RequireStatisticsEvidence: true,
					EvidenceID:                strings.TrimSpace(input.EvidenceID), EvidenceHash: strings.ToLower(strings.TrimSpace(input.EvidenceHash)), EvidenceVersion: input.EvidenceVersion,
				})
				if relErr != nil {
					return fmt.Errorf("release quota reservation failed: %w", relErr)
				}

				opWon, err := model.TransitionTaskSubmissionOperation(tx, op.ID, model.TaskSubmissionOperationTransition{
					From:             model.TaskSubmissionOperationStatusSubmissionUnknown,
					To:               model.TaskSubmissionOperationStatusRejected,
					ResolutionSource: model.TaskSubmissionResolutionSourceProviderVerified,
					ReasonCode:       reasonCode,
					ExpectedVersion:  op.LockVersion,
				})
				if err != nil {
					return fmt.Errorf("transition operation to rejected failed: %w", err)
				}
				if !opWon {
					return fmt.Errorf("%w: operation rejected CAS lost", ErrTaskResolutionCASLost)
				}

				attemptWon, err := model.TransitionTaskSubmissionAttempt(tx, attempt.ID, model.TaskSubmissionAttemptTransition{
					From:            model.TaskSubmissionAttemptStatusSubmissionUnknown,
					To:              model.TaskSubmissionAttemptStatusRejected,
					OutcomeCode:     reasonCode,
					ExpectedVersion: attempt.LockVersion,
				})
				if err != nil {
					return fmt.Errorf("transition attempt to rejected failed: %w", err)
				}
				if !attemptWon {
					return fmt.Errorf("%w: attempt rejected CAS lost", ErrTaskResolutionCASLost)
				}
			}
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, model.ErrTaskTerminalObservationManualReview) {
			_, obsErr := model.CreateOrLoadTaskTerminalObservation(db, model.TaskTerminalObservationInput{OperationID: op.ID, TaskID: 0, Outcome: "failed", ActualQuota: 0, ReasonCode: "provider_resolution_evidence_missing", RequestID: op.RequestID, ManualReview: true, ResolutionSource: model.TaskSubmissionResolutionSourceProviderVerified, EvidenceID: strings.TrimSpace(input.EvidenceID), EvidenceHash: strings.ToLower(strings.TrimSpace(input.EvidenceHash)), EvidenceVersion: input.EvidenceVersion})
			if obsErr != nil {
				return nil, obsErr
			}
			return nil, model.ErrTaskTerminalObservationManualReview
		}
		return nil, err
	}

	if releaseReceipt != nil {
		_ = model.ProjectQuotaMutationReceipt(ctx, db, releaseReceipt)
	}
	if err := db.Where("id = ?", op.ID).First(&op).Error; err != nil {
		return nil, fmt.Errorf("reload final operation failed: %w", err)
	}
	if attempt != nil {
		if err := db.Where("id = ?", attempt.ID).First(attempt).Error; err != nil {
			return nil, fmt.Errorf("reload final attempt failed: %w", err)
		}
	}

	return &ProviderVerifiedResolutionResult{
		Operation:      &op,
		Attempt:        attempt,
		Task:           formalTask,
		ReleaseReceipt: releaseReceipt,
	}, nil
}

func validProviderTerminalEvidence(input ProviderVerifiedResolutionInput, operation *model.TaskSubmissionOperation, attempt *model.TaskSubmissionAttempt) bool {
	if operation == nil || attempt == nil || input.EvidenceVersion != 1 {
		return false
	}
	evidenceID := strings.TrimSpace(input.EvidenceID)
	return evidenceID != "" && len(evidenceID) <= 191 && (evidenceID == attempt.UpstreamRequestID || evidenceID == attempt.ProviderOperationID)
}

func validTaskRecoveryClassification(platform, action string) bool {
	if platform == "" || action == "" || len(platform) > 30 || len(action) > 40 {
		return false
	}
	for _, value := range []string{platform, action} {
		for _, character := range value {
			if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '.' && character != '_' && character != '-' {
				return false
			}
		}
	}
	return true
}

// ResolveOperationManualAudit applies administrator manual audit resolution.
// Parameters: AuditCommandID (1-48 chars), OperatorUserID, ReasonCode, TargetStatus (rejected/canceled).
// If target is rejected/canceled, reserved quota is refunded via ReleaseTaskQuotaReservation,
// status is transitioned to rejected/canceled, and an immutable TaskBillingEvent is recorded
// with resolution_source="manual_audit" and AuditCommandID.
// Idempotency: Repeating the same AuditCommandID returns the original result. Conflicting parameters are rejected.
func (s *TaskResolutionService) ResolveOperationManualAudit(ctx context.Context, input ManualAuditResolutionInput) (*ManualAuditResolutionResult, error) {
	if input.DB == nil {
		input.DB = s.DB
	}
	return ResolveOperationManualAudit(ctx, input)
}

// ResolveOperationManualAudit is the package-level handler for manual audit resolution.
func ResolveOperationManualAudit(ctx context.Context, input ManualAuditResolutionInput) (*ManualAuditResolutionResult, error) {
	db := input.DB
	if db == nil {
		db = model.DB
	}
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	if input.OperationID <= 0 || input.OperatorUserID <= 0 {
		return nil, fmt.Errorf("%w: operation_id and operator_user_id must be positive", ErrTaskResolutionInvalidInput)
	}

	auditCmd := strings.ToLower(strings.TrimSpace(input.AuditCommandID))
	if !validAuditCommandID(auditCmd) {
		return nil, fmt.Errorf("%w: audit command id must be 1-48 characters containing only lowercase letters, digits, '.', '-', '_'", ErrTaskResolutionInvalidInput)
	}

	reasonCode := strings.ToLower(strings.TrimSpace(input.ReasonCode))
	if reasonCode == "" || reasonCode != sanitizeReasonCode(reasonCode, "") {
		return nil, fmt.Errorf("%w: reason code must use lowercase internal-code characters", ErrTaskResolutionInvalidInput)
	}

	targetStatus := input.TargetStatus
	if targetStatus != model.TaskSubmissionOperationStatusRejected && targetStatus != model.TaskSubmissionOperationStatusCanceled {
		return nil, fmt.Errorf("%w: target status must be rejected or canceled", ErrTaskResolutionInvalidInput)
	}

	var op model.TaskSubmissionOperation
	if err := db.Where("id = ?", input.OperationID).First(&op).Error; err != nil {
		return nil, fmt.Errorf("load operation failed: %w", err)
	}

	attempt, err := model.FindAttemptByOperationID(db, op.ID)
	if err != nil {
		return nil, fmt.Errorf("find attempt failed: %w", err)
	}

	canonicalEventKey := "task:" + op.PublicID + ":" + string(model.TaskBillingEventTypeManualResolution) + ":" + auditCmd + ":v1"

	var existingEvent model.TaskBillingEvent
	err = db.Where("event_key = ?", canonicalEventKey).First(&existingEvent).Error
	if err == nil {
		if existingEvent.ReasonCode != reasonCode ||
			existingEvent.UserID != op.UserID ||
			existingEvent.TokenID != op.TokenID ||
			(attempt != nil && existingEvent.ChannelID != attempt.ChannelID) {
			return nil, ErrTaskResolutionConflict
		}
		if op.Status != targetStatus && op.Status.Terminal() {
			return nil, ErrTaskResolutionConflict
		}
		return &ManualAuditResolutionResult{
			Operation:    &op,
			Attempt:      attempt,
			BillingEvent: &existingEvent,
		}, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("check existing audit command event failed: %w", err)
	}

	if op.Status.Terminal() {
		return nil, fmt.Errorf("%w: cannot audit operation already in terminal status %s", ErrTaskResolutionInvalidTransition, op.Status)
	}

	if targetStatus == model.TaskSubmissionOperationStatusCanceled {
		if op.Status != model.TaskSubmissionOperationStatusPrepared && op.Status != model.TaskSubmissionOperationStatusReserved {
			return nil, fmt.Errorf("%w: canceled target status only valid for prepared or reserved operations", ErrTaskResolutionInvalidTransition)
		}
	} else if targetStatus == model.TaskSubmissionOperationStatusRejected {
		if op.Status != model.TaskSubmissionOperationStatusPrepared &&
			op.Status != model.TaskSubmissionOperationStatusReserved &&
			op.Status != model.TaskSubmissionOperationStatusSubmissionUnknown {
			return nil, fmt.Errorf("%w: rejected target status not valid for operation status %s", ErrTaskResolutionInvalidTransition, op.Status)
		}
	}

	var releaseReceipt *model.QuotaMutationReceipt
	var auditEvent *model.TaskBillingEvent

	var billingContext model.TaskBillingContext
	channelID := 0
	if op.Status == model.TaskSubmissionOperationStatusReserved || op.Status == model.TaskSubmissionOperationStatusSubmissionUnknown {
		reserveEvidence, evidenceErr := model.FindTaskQuotaReceipt(db, op.ID, string(model.TaskBillingEventTypeReserve), op.UserID, op.TokenID)
		if evidenceErr != nil || reserveEvidence == nil {
			return nil, model.ErrTaskTerminalObservationManualReview
		}
		billingContext = model.TaskBillingContext(reserveEvidence.BillingContext)
		channelID = reserveEvidence.ChannelID
	}

	err = db.Transaction(func(tx *gorm.DB) error {
		if op.Status == model.TaskSubmissionOperationStatusReserved || op.Status == model.TaskSubmissionOperationStatusSubmissionUnknown {
			var relErr error
			releaseReceipt, relErr = model.ReleaseTaskQuotaReservation(tx, model.TaskQuotaReleaseInput{
				OperationID:               op.ID,
				UserID:                    op.UserID,
				TokenID:                   op.TokenID,
				ChannelID:                 channelID,
				ExpectedOperationVersion:  op.LockVersion,
				ReasonCode:                reasonCode,
				BillingContext:            billingContext,
				TargetOperationStatus:     "",
				ResolutionSource:          model.TaskSubmissionResolutionSourceManualAudit,
				RequireStatisticsEvidence: true,
				EvidenceID:                "audit." + auditCmd, EvidenceVersion: 1,
			})
			if relErr != nil {
				return fmt.Errorf("release quota reservation failed: %w", relErr)
			}
		}

		if op.Status == model.TaskSubmissionOperationStatusSubmissionUnknown {
			opWon, err := model.TransitionTaskSubmissionOperation(tx, op.ID, model.TaskSubmissionOperationTransition{
				From:             model.TaskSubmissionOperationStatusSubmissionUnknown,
				To:               model.TaskSubmissionOperationStatusRejected,
				ResolutionSource: model.TaskSubmissionResolutionSourceManualAudit,
				AuditCommandID:   input.AuditCommandID,
				ReasonCode:       reasonCode,
				ExpectedVersion:  op.LockVersion,
			})
			if err != nil {
				return fmt.Errorf("transition operation to rejected failed: %w", err)
			}
			if !opWon {
				return fmt.Errorf("%w: operation rejected CAS lost", ErrTaskResolutionCASLost)
			}

			if attempt != nil && attempt.Status == model.TaskSubmissionAttemptStatusSubmissionUnknown {
				attemptWon, err := model.TransitionTaskSubmissionAttempt(tx, attempt.ID, model.TaskSubmissionAttemptTransition{
					From:            model.TaskSubmissionAttemptStatusSubmissionUnknown,
					To:              model.TaskSubmissionAttemptStatusRejected,
					OutcomeCode:     reasonCode,
					ExpectedVersion: attempt.LockVersion,
				})
				if err != nil {
					return fmt.Errorf("transition attempt to rejected failed: %w", err)
				}
				if !attemptWon {
					return fmt.Errorf("%w: attempt rejected CAS lost", ErrTaskResolutionCASLost)
				}
			}

		} else if op.Status == model.TaskSubmissionOperationStatusReserved {
			opWon, err := model.TransitionTaskSubmissionOperation(tx, op.ID, model.TaskSubmissionOperationTransition{
				From:             model.TaskSubmissionOperationStatusReserved,
				To:               targetStatus,
				ResolutionSource: model.TaskSubmissionResolutionSourceManualAudit,
				AuditCommandID:   input.AuditCommandID,
				ReasonCode:       reasonCode,
				ExpectedVersion:  op.LockVersion,
			})
			if err != nil {
				return fmt.Errorf("transition reserved operation to %s failed: %w", targetStatus, err)
			}
			if !opWon {
				return fmt.Errorf("%w: operation transition CAS lost", ErrTaskResolutionCASLost)
			}

		} else if op.Status == model.TaskSubmissionOperationStatusPrepared {
			opWon, err := model.TransitionTaskSubmissionOperation(tx, op.ID, model.TaskSubmissionOperationTransition{
				From:            model.TaskSubmissionOperationStatusPrepared,
				To:              targetStatus,
				ReasonCode:      reasonCode,
				ExpectedVersion: op.LockVersion,
			})
			if err != nil {
				return fmt.Errorf("transition prepared operation to %s failed: %w", targetStatus, err)
			}
			if !opWon {
				return fmt.Errorf("%w: operation transition CAS lost", ErrTaskResolutionCASLost)
			}
		}

		if channelID <= 0 && attempt != nil && attempt.ChannelID > 0 {
			channelID = attempt.ChannelID
		}
		billingSource := "wallet"
		subscriptionID := 0
		if releaseReceipt != nil {
			billingSource = releaseReceipt.BillingSource
			subscriptionID = releaseReceipt.SubscriptionID
		}
		candidate := &model.TaskBillingEvent{
			OperationID:      &op.ID,
			EventType:        model.TaskBillingEventTypeManualResolution,
			UserID:           op.UserID,
			TokenID:          op.TokenID,
			ChannelID:        channelID,
			BillingSource:    billingSource,
			SubscriptionID:   subscriptionID,
			QuotaDelta:       0,
			ReasonCode:       reasonCode,
			ResolutionSource: model.TaskSubmissionResolutionSourceManualAudit,
			AuditCommandID:   auditCmd,
			EvidenceID:       "audit." + auditCmd,
			EvidenceVersion:  1,
			RequestID: func() string {
				if op.RequestID != "" {
					return op.RequestID
				}
				return "manual_" + auditCmd
			}(),
		}
		var createErr error
		auditEvent, createErr = model.CreateOrLoadTaskBillingEvent(tx, candidate)
		if createErr != nil {
			if errors.Is(createErr, model.ErrTaskBillingEventConflict) {
				return ErrTaskResolutionConflict
			}
			return fmt.Errorf("create manual resolution event failed: %w", createErr)
		}
		other, marshalErr := common.Marshal(map[string]interface{}{"operation_id": op.PublicID, "audit_command_id": auditCmd, "reason": reasonCode})
		if marshalErr != nil {
			return fmt.Errorf("encode manual resolution outbox: %w", marshalErr)
		}
		outboxCandidate, outboxErr := model.NewTaskBillingLogOutbox(auditEvent, model.TaskBillingLogPayload{Content: "manual_resolution", ModelName: "manual_resolution", Group: "system", Other: string(other)})
		if outboxErr != nil {
			return fmt.Errorf("build manual resolution outbox: %w", outboxErr)
		}
		if _, outboxErr = model.CreateOrLoadTaskBillingLogOutbox(tx, outboxCandidate); outboxErr != nil {
			return fmt.Errorf("create manual resolution outbox: %w", outboxErr)
		}

		return nil
	})
	if err != nil {
		if errors.Is(err, model.ErrTaskTerminalObservationManualReview) {
			_, obsErr := model.CreateOrLoadTaskTerminalObservation(db, model.TaskTerminalObservationInput{OperationID: op.ID, TaskID: 0, Outcome: "failed", ActualQuota: 0, ReasonCode: "manual_resolution_evidence_missing", RequestID: op.RequestID, ManualReview: true, ResolutionSource: model.TaskSubmissionResolutionSourceManualAudit, EvidenceID: "audit." + auditCmd, EvidenceVersion: 1})
			if obsErr != nil {
				return nil, obsErr
			}
			return nil, model.ErrTaskTerminalObservationManualReview
		}
		var existingEvent model.TaskBillingEvent
		if queryErr := db.Where("event_key = ?", canonicalEventKey).First(&existingEvent).Error; queryErr == nil {
			if existingEvent.AuditCommandID == auditCmd && existingEvent.ResolutionSource == model.TaskSubmissionResolutionSourceManualAudit {
				_ = db.Where("id = ?", op.ID).First(&op)
				if attempt != nil {
					_ = db.Where("id = ?", attempt.ID).First(attempt)
				}
				existingReceipt, _ := model.FindTaskQuotaReceipt(db, op.ID, string(model.TaskBillingEventTypeRefund), op.UserID, op.TokenID)
				if existingReceipt != nil {
					_ = model.ProjectQuotaMutationReceipt(ctx, db, existingReceipt)
				}
				return &ManualAuditResolutionResult{
					Operation:      &op,
					Attempt:        attempt,
					BillingEvent:   &existingEvent,
					ReleaseReceipt: existingReceipt,
				}, nil
			}
		}
		return nil, err
	}

	if releaseReceipt != nil {
		_ = model.ProjectQuotaMutationReceipt(ctx, db, releaseReceipt)
	}
	if err := db.Where("id = ?", op.ID).First(&op).Error; err != nil {
		return nil, fmt.Errorf("reload final operation failed: %w", err)
	}
	if attempt != nil {
		if err := db.Where("id = ?", attempt.ID).First(attempt).Error; err != nil {
			return nil, fmt.Errorf("reload final attempt failed: %w", err)
		}
	}

	return &ManualAuditResolutionResult{
		Operation:      &op,
		Attempt:        attempt,
		BillingEvent:   auditEvent,
		ReleaseReceipt: releaseReceipt,
	}, nil
}

func validAuditCommandID(value string) bool {
	if value == "" || len(value) > 48 {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '-' || character == '_' || character == '.' {
			continue
		}
		return false
	}
	return true
}
