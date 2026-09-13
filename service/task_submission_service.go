package service

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"gorm.io/gorm"
)

const (
	TaskProviderDispatchStatusAccepted = "accepted"
	TaskProviderDispatchStatusRejected = "rejected"
	TaskProviderDispatchStatusUnknown  = "unknown"
)

var (
	ErrTaskSubmissionInvalidInput    = errors.New("invalid task submission pipeline input")
	ErrTaskSubmissionDispatcherNil   = errors.New("task provider dispatcher is required")
	ErrTaskSubmissionDispatchCASLost = errors.New("task submission dispatch compare-and-swap lost")
	ErrTaskSubmissionOutcomeCASLost  = errors.New("task submission outcome compare-and-swap lost")
)

// TaskProviderDispatchResult is the upstream provider dispatch result.
type TaskProviderDispatchResult struct {
	Status         string // "accepted", "rejected", "unknown"
	ProviderTaskID string
	TaskCandidate  *model.Task
	ErrorCode      string
	ErrorMessage   string
}

// TaskProviderDispatcher defines the function signature for dispatching an operation attempt to a provider.
// This function must execute outside of any database transaction.
type TaskProviderDispatcher func(ctx context.Context, op *model.TaskSubmissionOperation, attempt *model.TaskSubmissionAttempt) (*TaskProviderDispatchResult, error)

// TaskSubmissionPipelineInput encapsulates all inputs required to run the submission pipeline.
type TaskSubmissionPipelineInput struct {
	DB             *gorm.DB
	OperationID    int64
	AttemptID      int64
	UserID         int
	TokenID        int
	ChannelID      int
	Quota          int64
	FreeModel      bool
	BillingSource  string
	SubscriptionID int
	BillingContext model.TaskBillingContext
	Dispatcher     TaskProviderDispatcher
}

// TaskSubmissionPipelineResult contains the final execution result of the submission pipeline.
type TaskSubmissionPipelineResult struct {
	Operation      *model.TaskSubmissionOperation
	Attempt        *model.TaskSubmissionAttempt
	DispatchResult *TaskProviderDispatchResult
	ReserveReceipt *model.QuotaMutationReceipt
	ReleaseReceipt *model.QuotaMutationReceipt
}

// TaskSubmissionService provides task submission dispatch and lifecycle execution.
type TaskSubmissionService struct {
	DB *gorm.DB
}

// NewTaskSubmissionService creates a new TaskSubmissionService instance.
func NewTaskSubmissionService(db *gorm.DB) *TaskSubmissionService {
	return &TaskSubmissionService{DB: db}
}

// Execute runs the complete task submission pipeline.
func (s *TaskSubmissionService) Execute(ctx context.Context, input TaskSubmissionPipelineInput) (*TaskSubmissionPipelineResult, error) {
	if input.DB == nil {
		input.DB = s.DB
	}
	return ExecuteTaskSubmissionPipeline(ctx, input)
}

// ExecuteTaskSubmissionPipeline executes the end-to-end task submission pipeline:
// 1. T1 Reserve: atomically reserves quota via model.ReserveTaskQuota in a DB transaction.
// 2. T2 Outbound: atomically starts dispatch via model.StartTaskSubmissionDispatch in a DB transaction.
// 3. Dispatch: calls dispatcher(ctx, op, attempt) OUTSIDE of any database transaction.
// 4. T3 Outcome: handles the provider outcome (accepted, rejected, unknown) in a DB transaction:
//   - accepted: transitions attempt & operation to accepted, records ProviderTaskID, ensures formal task exists.
//   - rejected: releases quota reservation (full refund) and transitions attempt & operation to rejected.
//   - unknown: transitions attempt & operation to submission_unknown; retains reserved quota (fail-closed).
func ExecuteTaskSubmissionPipeline(ctx context.Context, input TaskSubmissionPipelineInput) (*TaskSubmissionPipelineResult, error) {
	db := input.DB
	if db == nil {
		db = model.DB
	}
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	if db.Statement != nil && db.Statement.ConnPool != nil {
		if _, ok := db.Statement.ConnPool.(gorm.TxCommitter); ok {
			return nil, errors.New("pipeline must not run within an existing database transaction")
		}
	}
	if input.Dispatcher == nil {
		return nil, ErrTaskSubmissionDispatcherNil
	}
	if input.OperationID <= 0 || input.UserID <= 0 || input.TokenID <= 0 || input.ChannelID <= 0 ||
		input.Quota < 0 || input.Quota > int64(common.MaxQuota) {
		return nil, ErrTaskSubmissionInvalidInput
	}

	var op model.TaskSubmissionOperation
	if err := db.Where("id = ?", input.OperationID).First(&op).Error; err != nil {
		return nil, fmt.Errorf("load task submission operation failed: %w", err)
	}

	var attempt model.TaskSubmissionAttempt
	if input.AttemptID > 0 {
		if err := db.Where("id = ?", input.AttemptID).First(&attempt).Error; err != nil {
			return nil, fmt.Errorf("load task submission attempt failed: %w", err)
		}
	} else {
		if err := db.Where("operation_id = ?", op.ID).First(&attempt).Error; err != nil {
			return nil, fmt.Errorf("load task submission attempt by operation failed: %w", err)
		}
	}

	if attempt.OperationID != op.ID {
		return nil, fmt.Errorf("%w: attempt does not belong to operation", ErrTaskSubmissionInvalidInput)
	}
	if op.UserID != input.UserID || op.TokenID != input.TokenID || attempt.ChannelID != input.ChannelID {
		return nil, fmt.Errorf("%w: operation/attempt ownership or channel mismatch", ErrTaskSubmissionInvalidInput)
	}
	if op.Status != model.TaskSubmissionOperationStatusPrepared || attempt.Status != model.TaskSubmissionAttemptStatusPrepared {
		return nil, fmt.Errorf("%w: operation or attempt not in prepared status", ErrTaskSubmissionInvalidInput)
	}

	// 1. T1 Reserve: execute in a DB transaction
	var reserveReceipt *model.QuotaMutationReceipt
	err := db.Transaction(func(tx *gorm.DB) error {
		var err error
		reserveReceipt, err = model.ReserveTaskQuota(tx, model.TaskQuotaReservationInput{
			OperationID:              op.ID,
			UserID:                   input.UserID,
			TokenID:                  input.TokenID,
			ChannelID:                input.ChannelID,
			ExpectedOperationVersion: op.LockVersion,
			Quota:                    input.Quota,
			FreeModel:                input.FreeModel,
			ApplyStatistics:          true,
			RequestID:                op.RequestID,
			BillingSource:            input.BillingSource,
			SubscriptionID:           input.SubscriptionID,
			SelectBillingSource:      input.BillingSource == "",
			BillingContext:           input.BillingContext,
		})
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("task quota reservation failed: %w", err)
	}
	_ = model.ProjectQuotaMutationReceipt(ctx, db, reserveReceipt)

	// 2. T2 Outbound: atomically transition operation and attempt to dispatching in a DB transaction
	err = db.Transaction(func(tx *gorm.DB) error {
		won, err := model.StartTaskSubmissionDispatch(tx, op.ID, model.TaskSubmissionDispatchTransition{
			ExpectedOperationVersion: reserveReceipt.OperationVersionAfter,
			ExpectedAttemptVersion:   attempt.LockVersion,
		})
		if err != nil {
			return err
		}
		if !won {
			return ErrTaskSubmissionDispatchCASLost
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("start task submission dispatch failed: %w", err)
	}

	// Refresh local state after T2 transition
	if err := db.Where("id = ?", op.ID).First(&op).Error; err != nil {
		return nil, fmt.Errorf("reload operation after dispatch failed: %w", err)
	}
	if err := db.Where("id = ?", attempt.ID).First(&attempt).Error; err != nil {
		return nil, fmt.Errorf("reload attempt after dispatch failed: %w", err)
	}

	// 3. Dispatch: execute OUTSIDE of any database transaction
	dispatchResult, dispatchErr := input.Dispatcher(ctx, &op, &attempt)
	if dispatchErr != nil {
		if dispatchResult == nil {
			dispatchResult = &TaskProviderDispatchResult{
				Status:       TaskProviderDispatchStatusUnknown,
				ErrorMessage: dispatchErr.Error(),
			}
		} else if dispatchResult.Status == "" {
			dispatchResult.Status = TaskProviderDispatchStatusUnknown
		}
	} else if dispatchResult == nil {
		dispatchResult = &TaskProviderDispatchResult{
			Status: TaskProviderDispatchStatusUnknown,
		}
	}

	if ctx.Err() != nil {
		dispatchResult.Status = TaskProviderDispatchStatusUnknown
		if dispatchResult.ErrorMessage == "" {
			dispatchResult.ErrorMessage = ctx.Err().Error()
		}
	}

	status := strings.ToLower(strings.TrimSpace(dispatchResult.Status))
	if status != TaskProviderDispatchStatusAccepted && status != TaskProviderDispatchStatusRejected {
		status = TaskProviderDispatchStatusUnknown
	}
	dispatchResult.Status = status
	if status == TaskProviderDispatchStatusAccepted {
		if err := validateTaskProviderCandidate(dispatchResult, &op, &attempt, input, reserveReceipt); err != nil {
			dispatchResult.Status = TaskProviderDispatchStatusUnknown
			dispatchResult.ErrorCode = "invalid_task_candidate"
			dispatchResult.ErrorMessage = err.Error()
			status = TaskProviderDispatchStatusUnknown
		}
	}

	// 4. T3 Outcome: handle outcome in a DB transaction
	var releaseReceipt *model.QuotaMutationReceipt
	var taskCreateErr error
	err = db.Transaction(func(tx *gorm.DB) error {
		switch status {
		case TaskProviderDispatchStatusAccepted:
			formalTask := *dispatchResult.TaskCandidate
			if err := tx.Create(&formalTask).Error; err != nil {
				taskCreateErr = err
				return fmt.Errorf("create formal task for accepted operation failed: %w", err)
			}

			// Transition attempt to accepted
			attemptWon, err := model.TransitionTaskSubmissionAttempt(tx, attempt.ID, model.TaskSubmissionAttemptTransition{
				From:                model.TaskSubmissionAttemptStatusDispatching,
				To:                  model.TaskSubmissionAttemptStatusAccepted,
				ProviderOperationID: strings.TrimSpace(dispatchResult.ProviderTaskID),
				ExpectedVersion:     attempt.LockVersion,
			})
			if err != nil {
				return fmt.Errorf("transition attempt to accepted failed: %w", err)
			}
			if !attemptWon {
				return fmt.Errorf("%w: attempt accepted CAS failed", ErrTaskSubmissionOutcomeCASLost)
			}

			// Transition operation to accepted
			opWon, err := model.TransitionTaskSubmissionOperation(tx, op.ID, model.TaskSubmissionOperationTransition{
				From:            model.TaskSubmissionOperationStatusDispatching,
				To:              model.TaskSubmissionOperationStatusAccepted,
				TaskID:          &formalTask.ID,
				ExpectedVersion: op.LockVersion,
			})
			if err != nil {
				return fmt.Errorf("transition operation to accepted failed: %w", err)
			}
			if !opWon {
				return fmt.Errorf("%w: operation accepted CAS failed", ErrTaskSubmissionOutcomeCASLost)
			}

		case TaskProviderDispatchStatusRejected:
			outcomeCode := sanitizeReasonCode(dispatchResult.ErrorCode, "task_dispatch_rejected")
			reasonCode := "task_dispatch_rejected"
			if outcomeCode != "" {
				reasonCode = outcomeCode
			}

			// Release reserved quota first to strictly preserve User -> Token -> Sub -> Op -> Attempt lock hierarchy
			var relErr error
			releaseReceipt, relErr = model.ReleaseTaskQuotaReservation(tx, model.TaskQuotaReleaseInput{
				OperationID:               op.ID,
				UserID:                    input.UserID,
				TokenID:                   input.TokenID,
				ChannelID:                 input.ChannelID,
				ExpectedOperationVersion:  op.LockVersion,
				ReasonCode:                reasonCode,
				BillingContext:            input.BillingContext,
				TargetOperationStatus:     model.TaskSubmissionOperationStatusRejected,
				RequireStatisticsEvidence: true,
				EvidenceID:                "local.dispatch_rejected", EvidenceVersion: 1,
			})
			if relErr != nil {
				return fmt.Errorf("release quota reservation failed: %w", relErr)
			}

			// Transition attempt to rejected after operation
			attemptWon, err := model.TransitionTaskSubmissionAttempt(tx, attempt.ID, model.TaskSubmissionAttemptTransition{
				From:            model.TaskSubmissionAttemptStatusDispatching,
				To:              model.TaskSubmissionAttemptStatusRejected,
				OutcomeCode:     outcomeCode,
				ExpectedVersion: attempt.LockVersion,
			})
			if err != nil {
				return fmt.Errorf("transition attempt to rejected failed: %w", err)
			}
			if !attemptWon {
				return fmt.Errorf("%w: attempt rejected CAS failed", ErrTaskSubmissionOutcomeCASLost)
			}

		case TaskProviderDispatchStatusUnknown:
			// Fail-closed: do not release quota
			outcomeCode := sanitizeReasonCode(dispatchResult.ErrorCode, "task_dispatch_unknown")
			attemptWon, err := model.TransitionTaskSubmissionAttempt(tx, attempt.ID, model.TaskSubmissionAttemptTransition{
				From:                model.TaskSubmissionAttemptStatusDispatching,
				To:                  model.TaskSubmissionAttemptStatusSubmissionUnknown,
				ProviderOperationID: strings.TrimSpace(dispatchResult.ProviderTaskID),
				OutcomeCode:         outcomeCode,
				ExpectedVersion:     attempt.LockVersion,
			})
			if err != nil {
				return fmt.Errorf("transition attempt to unknown failed: %w", err)
			}
			if !attemptWon {
				return fmt.Errorf("%w: attempt unknown CAS failed", ErrTaskSubmissionOutcomeCASLost)
			}

			reasonCode := "task_dispatch_unknown"
			if outcomeCode != "" {
				reasonCode = outcomeCode
			}
			opWon, err := model.TransitionTaskSubmissionOperation(tx, op.ID, model.TaskSubmissionOperationTransition{
				From:            model.TaskSubmissionOperationStatusDispatching,
				To:              model.TaskSubmissionOperationStatusSubmissionUnknown,
				ReasonCode:      reasonCode,
				ExpectedVersion: op.LockVersion,
			})
			if err != nil {
				return fmt.Errorf("transition operation to unknown failed: %w", err)
			}
			if !opWon {
				return fmt.Errorf("%w: operation unknown CAS failed", ErrTaskSubmissionOutcomeCASLost)
			}
		}
		return nil
	})
	if err != nil && taskCreateErr != nil {
		dispatchResult.Status = TaskProviderDispatchStatusUnknown
		dispatchResult.ErrorCode = "task_create_failed"
		dispatchResult.ErrorMessage = "accepted provider result could not be persisted"
		if unknownErr := transitionTaskSubmissionToUnknown(db, &op, &attempt, dispatchResult); unknownErr != nil {
			return nil, fmt.Errorf("task submission outcome processing failed: %v; mark task creation failure unknown: %w", err, unknownErr)
		}
		err = nil
	}
	if err != nil {
		return nil, fmt.Errorf("task submission outcome processing failed: %w", err)
	}
	if releaseReceipt != nil {
		_ = model.ProjectQuotaMutationReceipt(ctx, db, releaseReceipt)
	}

	// Reload latest operation and attempt state
	if err := db.Where("id = ?", op.ID).First(&op).Error; err != nil {
		return nil, fmt.Errorf("reload final operation failed: %w", err)
	}
	if err := db.Where("id = ?", attempt.ID).First(&attempt).Error; err != nil {
		return nil, fmt.Errorf("reload final attempt failed: %w", err)
	}

	return &TaskSubmissionPipelineResult{
		Operation:      &op,
		Attempt:        &attempt,
		DispatchResult: dispatchResult,
		ReserveReceipt: reserveReceipt,
		ReleaseReceipt: releaseReceipt,
	}, nil
}

func validateTaskProviderCandidate(result *TaskProviderDispatchResult, operation *model.TaskSubmissionOperation, attempt *model.TaskSubmissionAttempt, input TaskSubmissionPipelineInput, receipt *model.QuotaMutationReceipt) error {
	if result == nil || operation == nil || attempt == nil || receipt == nil || result.TaskCandidate == nil {
		return errors.New("accepted provider result is missing a task candidate")
	}
	providerTaskID := strings.TrimSpace(result.ProviderTaskID)
	candidate := result.TaskCandidate
	if providerTaskID == "" || candidate.ID != 0 || candidate.TaskID != operation.PublicID || candidate.UserId != input.UserID ||
		candidate.ChannelId != input.ChannelID || candidate.Platform == "" || strings.TrimSpace(candidate.Action) == "" ||
		candidate.Quota != TaskInitialQuota(receipt.RequestFingerprintVersion, receipt.EstimatedQuota, receipt.Quota) || candidate.PrivateData.UpstreamTaskID != providerTaskID || candidate.PrivateData.FreeModel != receipt.FreeModel ||
		candidate.PrivateData.TokenId != input.TokenID || candidate.PrivateData.BillingPreference != operation.BillingPreference ||
		candidate.PrivateData.BillingSource != operation.BillingSource || candidate.PrivateData.SubscriptionId != operation.SubscriptionID ||
		candidate.PrivateData.BillingSource != receipt.BillingSource || candidate.PrivateData.SubscriptionId != receipt.SubscriptionID ||
		candidate.PrivateData.BillingPreference != receipt.BillingPreference || candidate.PrivateData.BillingContext == nil ||
		!reflect.DeepEqual(*candidate.PrivateData.BillingContext, input.BillingContext) {
		return errors.New("accepted provider task candidate does not match the frozen submission contract")
	}
	return nil
}

// TaskInitialQuota returns the quota persisted on a formal Task. V2 receipts
// separate the estimated initial charge from the actual reservation; historical
// receipts use the reservation amount for both meanings.
func TaskInitialQuota(version int, estimatedQuota, reservedQuota int64) int {
	if version >= 2 {
		return int(estimatedQuota)
	}
	return int(reservedQuota)
}

func transitionTaskSubmissionToUnknown(db *gorm.DB, operation *model.TaskSubmissionOperation, attempt *model.TaskSubmissionAttempt, result *TaskProviderDispatchResult) error {
	return db.Transaction(func(tx *gorm.DB) error {
		outcomeCode := sanitizeReasonCode(result.ErrorCode, "task_dispatch_unknown")
		attemptWon, err := model.TransitionTaskSubmissionAttempt(tx, attempt.ID, model.TaskSubmissionAttemptTransition{
			From: model.TaskSubmissionAttemptStatusDispatching, To: model.TaskSubmissionAttemptStatusSubmissionUnknown,
			ProviderOperationID: strings.TrimSpace(result.ProviderTaskID), OutcomeCode: outcomeCode, ExpectedVersion: attempt.LockVersion,
			TaskPlatform: func() string {
				if result.TaskCandidate != nil {
					return string(result.TaskCandidate.Platform)
				}
				return ""
			}(),
			TaskAction: func() string {
				if result.TaskCandidate != nil {
					return result.TaskCandidate.Action
				}
				return ""
			}(),
		})
		if err != nil {
			return err
		}
		if !attemptWon {
			return ErrTaskSubmissionOutcomeCASLost
		}
		reasonCode := outcomeCode
		if reasonCode == "" {
			reasonCode = "task_dispatch_unknown"
		}
		opWon, err := model.TransitionTaskSubmissionOperation(tx, operation.ID, model.TaskSubmissionOperationTransition{
			From: model.TaskSubmissionOperationStatusDispatching, To: model.TaskSubmissionOperationStatusSubmissionUnknown,
			ReasonCode: reasonCode, ExpectedVersion: operation.LockVersion,
		})
		if err != nil {
			return err
		}
		if !opWon {
			return ErrTaskSubmissionOutcomeCASLost
		}
		return nil
	})
}
