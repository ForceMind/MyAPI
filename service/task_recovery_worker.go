package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ForceMind/MyAPI/logger"
	"github.com/ForceMind/MyAPI/model"
	"gorm.io/gorm"
)

const (
	DefaultStaleDispatchThreshold    = 5 * time.Minute
	DefaultStaleReservationThreshold = 10 * time.Minute
	DefaultRecoveryBatchSize         = 50
	DefaultRecoveryLeaseDuration     = 60 * time.Second
	MaxRecoveryLeaseDuration         = 300 * time.Second
)

var (
	ErrTaskRecoveryWorkerInvalidInput = errors.New("invalid task recovery worker input")
)

// TaskRecoveryWorker manages background recovery inspection scans for stale task submissions
// and expired billing event processing leases.
type TaskRecoveryWorker struct {
	WorkerID                  string
	StaleDispatchThreshold    time.Duration
	StaleReservationThreshold time.Duration
	BatchSize                 int
	LeaseDuration             time.Duration
}

// NewTaskRecoveryWorker constructs a TaskRecoveryWorker with production-safe defaults.
func NewTaskRecoveryWorker(workerID string) *TaskRecoveryWorker {
	return &TaskRecoveryWorker{
		WorkerID:                  strings.TrimSpace(workerID),
		StaleDispatchThreshold:    DefaultStaleDispatchThreshold,
		StaleReservationThreshold: DefaultStaleReservationThreshold,
		BatchSize:                 DefaultRecoveryBatchSize,
		LeaseDuration:             DefaultRecoveryLeaseDuration,
	}
}

func (w *TaskRecoveryWorker) resolveConfig() (string, time.Duration, time.Duration, int, time.Duration) {
	workerID := strings.TrimSpace(w.WorkerID)
	if workerID == "" {
		workerID = "task_recovery_worker"
	}
	dispatchThreshold := w.StaleDispatchThreshold
	if dispatchThreshold <= 0 {
		dispatchThreshold = DefaultStaleDispatchThreshold
	}
	reservationThreshold := w.StaleReservationThreshold
	if reservationThreshold <= 0 {
		reservationThreshold = DefaultStaleReservationThreshold
	}
	batchSize := w.BatchSize
	if batchSize <= 0 {
		batchSize = DefaultRecoveryBatchSize
	}
	leaseDuration := w.LeaseDuration
	if leaseDuration <= 0 {
		leaseDuration = DefaultRecoveryLeaseDuration
	}
	if leaseDuration > MaxRecoveryLeaseDuration {
		leaseDuration = MaxRecoveryLeaseDuration
	}
	return workerID, dispatchThreshold, reservationThreshold, batchSize, leaseDuration
}

// RecoverStaleDispatching scans for operations stuck in 'dispatching' beyond StaleDispatchThreshold.
// It strictly enforces fail-closed semantics: atomically moves both operation and attempt to
// 'submission_unknown' with reason "dispatch_timeout_fail_closed". It NEVER automatically resends or refunds.
func (w *TaskRecoveryWorker) RecoverStaleDispatching(ctx context.Context, db *gorm.DB) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if db == nil {
		db = model.DB
	}
	if db == nil {
		return 0, gorm.ErrInvalidDB
	}
	_, dispatchThreshold, _, batchSize, _ := w.resolveConfig()
	now, _ := getDBTimestamp(db)
	cutoffTime := now - int64(dispatchThreshold.Seconds())
	if cutoffTime < 0 {
		cutoffTime = 0
	}

	ops, err := model.ListStaleDispatchingOperations(db, cutoffTime, batchSize)
	if err != nil {
		return 0, fmt.Errorf("list stale dispatching operations failed: %w", err)
	}

	recoveredCount := 0
	for _, op := range ops {
		if err := ctx.Err(); err != nil {
			return recoveredCount, err
		}

		attempt, err := model.FindAttemptByOperationID(db, op.ID)
		if err != nil {
			return recoveredCount, fmt.Errorf("find attempt for operation %d failed: %w", op.ID, err)
		}
		if attempt == nil {
			continue
		}

		// Strict fail-closed: atomics CAS op & attempt to submission_unknown. No quota refund!
		// Lock order: Operation -> Attempt.
		txErr := db.Transaction(func(tx *gorm.DB) error {
			opWon, err := model.TransitionTaskSubmissionOperation(tx, op.ID, model.TaskSubmissionOperationTransition{
				From:            model.TaskSubmissionOperationStatusDispatching,
				To:              model.TaskSubmissionOperationStatusSubmissionUnknown,
				ReasonCode:      "dispatch_timeout_fail_closed",
				ExpectedVersion: op.LockVersion,
			})
			if err != nil {
				return err
			}
			if !opWon {
				return model.ErrTaskRecoveryInvalidTransition
			}

			attemptWon, err := model.TransitionTaskSubmissionAttempt(tx, attempt.ID, model.TaskSubmissionAttemptTransition{
				From:            model.TaskSubmissionAttemptStatusDispatching,
				To:              model.TaskSubmissionAttemptStatusSubmissionUnknown,
				OutcomeCode:     "dispatch_timeout_fail_closed",
				ExpectedVersion: attempt.LockVersion,
			})
			if err != nil {
				return err
			}
			if !attemptWon {
				return model.ErrTaskRecoveryInvalidTransition
			}

			return nil
		})

		if txErr == nil {
			recoveredCount++
		} else if errors.Is(txErr, model.ErrTaskRecoveryInvalidTransition) {
			// CAS lost due to concurrent state change, safely skip
			continue
		} else {
			return recoveredCount, fmt.Errorf("recover stale dispatching op %d failed: %w", op.ID, txErr)
		}
	}

	return recoveredCount, nil
}

// RecoverStaleUnfinished scans for operations in 'prepared' or 'reserved' older than StaleReservationThreshold.
// It verifies that no attempt ever entered dispatching (attempt is prepared without dispatch started).
// If the operation is 'reserved', it releases the reserved quota back to the user/token/subscription
// via model.ReleaseTaskQuotaReservation and transitions the operation to 'canceled'.
// If the operation is 'prepared', no reservation exists, so it transitions the operation directly to 'canceled'.
func (w *TaskRecoveryWorker) RecoverStaleUnfinished(ctx context.Context, db *gorm.DB) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if db == nil {
		db = model.DB
	}
	if db == nil {
		return 0, gorm.ErrInvalidDB
	}
	_, _, reservationThreshold, batchSize, _ := w.resolveConfig()
	now, _ := getDBTimestamp(db)
	cutoffTime := now - int64(reservationThreshold.Seconds())
	if cutoffTime < 0 {
		cutoffTime = 0
	}

	ops, err := model.ListUnfinishedPreparedOrReservedOperations(db, cutoffTime, batchSize)
	if err != nil {
		return 0, fmt.Errorf("list unfinished prepared or reserved operations failed: %w", err)
	}

	recoveredCount := 0
	for _, op := range ops {
		if err := ctx.Err(); err != nil {
			return recoveredCount, err
		}

		attempt, err := model.FindAttemptByOperationID(db, op.ID)
		if err != nil {
			return recoveredCount, fmt.Errorf("find attempt for operation %d failed: %w", op.ID, err)
		}

		// Ensure attempt has NEVER entered dispatching
		if attempt != nil {
			if attempt.Status != model.TaskSubmissionAttemptStatusPrepared || attempt.StartedAt != nil {
				// Attempt already dispatched or attempted dispatch; skip
				continue
			}
		} else if op.Status == model.TaskSubmissionOperationStatusReserved {
			logger.LogWarn(ctx, fmt.Sprintf("stale reserved operation %d has no attempt, skip safe cancellation", op.ID))
			continue
		}

		switch op.Status {
		case model.TaskSubmissionOperationStatusReserved:
			channelID := 0
			if attempt != nil && attempt.ChannelID > 0 {
				channelID = attempt.ChannelID
			}
			var billingContext model.TaskBillingContext
			if reserveReceipt, rErr := model.FindTaskQuotaReceipt(db, op.ID, string(model.TaskBillingEventTypeReserve), op.UserID, op.TokenID); rErr == nil && reserveReceipt != nil {
				billingContext = model.TaskBillingContext(reserveReceipt.BillingContext)
				if channelID == 0 && reserveReceipt.ChannelID > 0 {
					channelID = reserveReceipt.ChannelID
				}
			}
			if !billingContext.Complete || billingContext.OriginModelName == "" {
				billingContext = model.TaskBillingContext{
					Version:         model.TaskBillingContextVersion,
					Complete:        true,
					ModelPrice:      1,
					ModelRatio:      1,
					GroupRatio:      1,
					OriginModelName: "stale_recovery",
					PerCallBilling:  true,
				}
			}
			if channelID <= 0 {
				channelID = 1
			}
			receipt, relErr := model.ReleaseTaskQuotaReservation(db, model.TaskQuotaReleaseInput{
				OperationID:              op.ID,
				UserID:                   op.UserID,
				TokenID:                  op.TokenID,
				ChannelID:                channelID,
				ExpectedOperationVersion: op.LockVersion,
				ReasonCode:               "stale_reservation_timeout",
				BillingContext:           billingContext,
				TargetOperationStatus:    model.TaskSubmissionOperationStatusCanceled,
			})
			if relErr != nil {
				if errors.Is(relErr, model.ErrTaskQuotaReservationCASLost) ||
					errors.Is(relErr, model.ErrTaskRecoveryInvalidTransition) {
					continue
				}
				logger.LogError(ctx, fmt.Sprintf("release stale reserved quota op %d failed: %v", op.ID, relErr))
				continue
			}
			if receipt != nil {
				recoveredCount++
			}

		case model.TaskSubmissionOperationStatusPrepared:
			won, transErr := model.TransitionTaskSubmissionOperation(db, op.ID, model.TaskSubmissionOperationTransition{
				From:            model.TaskSubmissionOperationStatusPrepared,
				To:              model.TaskSubmissionOperationStatusCanceled,
				ReasonCode:      "stale_prepared_timeout",
				ExpectedVersion: op.LockVersion,
			})
			if transErr != nil {
				if errors.Is(transErr, model.ErrTaskRecoveryInvalidTransition) {
					continue
				}
				return recoveredCount, fmt.Errorf("transition stale prepared op %d to canceled failed: %w", op.ID, transErr)
			}
			if won {
				recoveredCount++
			}
		}
	}

	return recoveredCount, nil
}

// RecoverExpiredBillingEvents scans for TaskBillingEvent records stuck in 'claimed' beyond their lease expiry.
// It reclaims the lease using model.ReclaimExpiredTaskBillingEvent.
func (w *TaskRecoveryWorker) RecoverExpiredBillingEvents(ctx context.Context, db *gorm.DB) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if db == nil {
		db = model.DB
	}
	if db == nil {
		return 0, gorm.ErrInvalidDB
	}
	workerID, _, _, batchSize, leaseDuration := w.resolveConfig()
	now, _ := getDBTimestamp(db)

	events, err := model.ListStaleClaimedTaskBillingEvents(db, now, batchSize)
	if err != nil {
		return 0, fmt.Errorf("list stale claimed billing events failed: %w", err)
	}

	recoveredCount := 0
	for _, event := range events {
		if err := ctx.Err(); err != nil {
			return recoveredCount, err
		}

		lease := model.TaskRecoveryProcessingLease{
			WorkerID:     workerID,
			LeaseSeconds: int64(leaseDuration.Seconds()),
		}

		won, reclaimErr := model.ReclaimExpiredTaskBillingEvent(db, event.ID, lease, event.LockVersion, 0)
		if reclaimErr != nil {
			if errors.Is(reclaimErr, model.ErrTaskRecoveryInvalidTransition) {
				continue
			}
			return recoveredCount, fmt.Errorf("reclaim expired billing event %d failed: %w", event.ID, reclaimErr)
		}
		if won {
			recoveredCount++
		}
	}

	return recoveredCount, nil
}
