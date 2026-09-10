package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

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

	reasonCode := strings.TrimSpace(input.ReasonCode)
	if reasonCode == "" {
		reasonCode = "provider_verified_rejected"
	}
	if len(reasonCode) > 64 {
		reasonCode = reasonCode[:64]
	}

	var formalTask *model.Task
	var releaseReceipt *model.QuotaMutationReceipt

	err = db.Transaction(func(tx *gorm.DB) error {
		if op.Status == model.TaskSubmissionOperationStatusSubmissionUnknown {
			if providerStatus == TaskProviderDispatchStatusAccepted {
				var task model.Task
				err := tx.Where("task_id = ? AND user_id = ?", op.PublicID, op.UserID).First(&task).Error
				if errors.Is(err, gorm.ErrRecordNotFound) {
					task = model.Task{
						TaskID:    op.PublicID,
						UserId:    op.UserID,
						ChannelId: attempt.ChannelID,
						Status:    model.TaskStatusSubmitted,
					}
					if err := tx.Create(&task).Error; err != nil {
						return fmt.Errorf("create formal task failed: %w", err)
					}
				} else if err != nil {
					return fmt.Errorf("query formal task failed: %w", err)
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
					ExpectedVersion:     attempt.LockVersion,
				})
				if err != nil {
					return fmt.Errorf("transition attempt to accepted failed: %w", err)
				}
				if !attemptWon {
					return fmt.Errorf("%w: attempt accepted CAS lost", ErrTaskResolutionCASLost)
				}

			} else {
				billingContext := input.BillingContext
				if !billingContext.Complete || billingContext.OriginModelName == "" {
					if reserveReceipt, rErr := model.FindTaskQuotaReceipt(db, op.ID, string(model.TaskBillingEventTypeReserve), op.UserID, op.TokenID); rErr == nil && reserveReceipt != nil {
						billingContext = model.TaskBillingContext(reserveReceipt.BillingContext)
					}
				}
				if !billingContext.Complete || billingContext.OriginModelName == "" {
					billingContext = model.TaskBillingContext{
						Version:         model.TaskBillingContextVersion,
						Complete:        true,
						ModelPrice:      1,
						ModelRatio:      1,
						GroupRatio:      1,
						OriginModelName: "provider_verified_resolution",
						PerCallBilling:  true,
					}
				}
				channelID := attempt.ChannelID
				if channelID <= 0 {
					channelID = 1
				}
				var relErr error
				releaseReceipt, relErr = model.ReleaseTaskQuotaReservation(tx, model.TaskQuotaReleaseInput{
					OperationID:              op.ID,
					UserID:                   op.UserID,
					TokenID:                  op.TokenID,
					ChannelID:                channelID,
					ExpectedOperationVersion: op.LockVersion,
					ReasonCode:               reasonCode,
					BillingContext:           billingContext,
					TargetOperationStatus:    "",
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
		} else if op.Status == model.TaskSubmissionOperationStatusOutcomeUnknown {
			targetStatus := model.TaskSubmissionOperationStatusSucceeded
			if providerStatus == TaskProviderDispatchStatusRejected {
				targetStatus = model.TaskSubmissionOperationStatusFailed
			}
			opWon, err := model.TransitionTaskSubmissionOperation(tx, op.ID, model.TaskSubmissionOperationTransition{
				From:             model.TaskSubmissionOperationStatusOutcomeUnknown,
				To:               targetStatus,
				ResolutionSource: model.TaskSubmissionResolutionSourceProviderVerified,
				ReasonCode:       reasonCode,
				ExpectedVersion:  op.LockVersion,
			})
			if err != nil {
				return fmt.Errorf("transition operation from outcome_unknown failed: %w", err)
			}
			if !opWon {
				return fmt.Errorf("%w: operation outcome CAS lost", ErrTaskResolutionCASLost)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
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

	reasonCode := strings.TrimSpace(input.ReasonCode)
	if reasonCode == "" || len(reasonCode) > 64 {
		return nil, fmt.Errorf("%w: reason code must be non-empty and at most 64 characters", ErrTaskResolutionInvalidInput)
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

	billingContext := input.BillingContext
	channelID := 0
	if attempt != nil && attempt.ChannelID > 0 {
		channelID = attempt.ChannelID
	}
	if op.Status == model.TaskSubmissionOperationStatusReserved || op.Status == model.TaskSubmissionOperationStatusSubmissionUnknown {
		if !billingContext.Complete || billingContext.OriginModelName == "" {
			if reserveReceipt, rErr := model.FindTaskQuotaReceipt(db, op.ID, string(model.TaskBillingEventTypeReserve), op.UserID, op.TokenID); rErr == nil && reserveReceipt != nil {
				billingContext = model.TaskBillingContext(reserveReceipt.BillingContext)
				if channelID == 0 && reserveReceipt.ChannelID > 0 {
					channelID = reserveReceipt.ChannelID
				}
			}
		}
		if !billingContext.Complete || billingContext.OriginModelName == "" {
			billingContext = model.TaskBillingContext{
				Version:         model.TaskBillingContextVersion,
				Complete:        true,
				ModelPrice:      1,
				ModelRatio:      1,
				GroupRatio:      1,
				OriginModelName: "manual_audit_resolution",
				PerCallBilling:  true,
			}
		}
		if channelID <= 0 {
			channelID = 1
		}
	}

	err = db.Transaction(func(tx *gorm.DB) error {
		if op.Status == model.TaskSubmissionOperationStatusReserved || op.Status == model.TaskSubmissionOperationStatusSubmissionUnknown {
			var relErr error
			releaseReceipt, relErr = model.ReleaseTaskQuotaReservation(tx, model.TaskQuotaReleaseInput{
				OperationID:              op.ID,
				UserID:                   op.UserID,
				TokenID:                  op.TokenID,
				ChannelID:                channelID,
				ExpectedOperationVersion: op.LockVersion,
				ReasonCode:               reasonCode,
				BillingContext:           billingContext,
				TargetOperationStatus:    "",
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
		}
		var createErr error
		auditEvent, createErr = model.CreateOrLoadTaskBillingEvent(tx, candidate)
		if createErr != nil {
			if errors.Is(createErr, model.ErrTaskBillingEventConflict) {
				return ErrTaskResolutionConflict
			}
			return fmt.Errorf("create manual resolution event failed: %w", createErr)
		}

		return nil
	})
	if err != nil {
		var existingEvent model.TaskBillingEvent
		if queryErr := db.Where("event_key = ?", canonicalEventKey).First(&existingEvent).Error; queryErr == nil {
			if existingEvent.AuditCommandID == auditCmd && existingEvent.ResolutionSource == model.TaskSubmissionResolutionSourceManualAudit {
				_ = db.Where("id = ?", op.ID).First(&op)
				if attempt != nil {
					_ = db.Where("id = ?", attempt.ID).First(attempt)
				}
				existingReceipt, _ := model.FindTaskQuotaReceipt(db, op.ID, string(model.TaskBillingEventTypeRefund), op.UserID, op.TokenID)
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
