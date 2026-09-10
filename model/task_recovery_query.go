package model

import (
	"errors"
	"fmt"

	"gorm.io/gorm"
)

const (
	TaskRecoveryDefaultScanLimit = 50
	TaskRecoveryMaxScanLimit     = 100
)

func clampScanLimit(limit int) int {
	if limit <= 0 {
		return TaskRecoveryDefaultScanLimit
	}
	if limit > TaskRecoveryMaxScanLimit {
		return TaskRecoveryMaxScanLimit
	}
	return limit
}

// ListStaleDispatchingOperations returns operations stuck in 'dispatching' whose
// dispatch_started_at is at or before cutoffTime.
func ListStaleDispatchingOperations(tx *gorm.DB, cutoffTime int64, limit int) ([]*TaskSubmissionOperation, error) {
	if tx == nil {
		return nil, gorm.ErrInvalidDB
	}
	if cutoffTime < 0 {
		return nil, fmt.Errorf("%w: cutoff time cannot be negative", ErrTaskRecoveryInvalidRecord)
	}
	var results []*TaskSubmissionOperation
	err := tx.Where(
		"status = ? AND dispatch_started_at IS NOT NULL AND dispatch_started_at <= ?",
		TaskSubmissionOperationStatusDispatching, cutoffTime,
	).Order("dispatch_started_at ASC, id ASC").Limit(clampScanLimit(limit)).Find(&results).Error
	if err != nil {
		return nil, err
	}
	return results, nil
}

// ListUnfinishedPreparedOrReservedOperations returns operations in 'prepared' or 'reserved'
// created at or before cutoffTime.
func ListUnfinishedPreparedOrReservedOperations(tx *gorm.DB, cutoffTime int64, limit int) ([]*TaskSubmissionOperation, error) {
	if tx == nil {
		return nil, gorm.ErrInvalidDB
	}
	if cutoffTime < 0 {
		return nil, fmt.Errorf("%w: cutoff time cannot be negative", ErrTaskRecoveryInvalidRecord)
	}
	var results []*TaskSubmissionOperation
	err := tx.Where(
		"status IN (?, ?) AND created_at <= ?",
		TaskSubmissionOperationStatusPrepared, TaskSubmissionOperationStatusReserved, cutoffTime,
	).Order("created_at ASC, id ASC").Limit(clampScanLimit(limit)).Find(&results).Error
	if err != nil {
		return nil, err
	}
	return results, nil
}

// ListUnknownTaskSubmissionOperations returns operations in 'submission_unknown' or 'outcome_unknown'
// that await provider inquiry or manual resolution.
func ListUnknownTaskSubmissionOperations(tx *gorm.DB, limit int) ([]*TaskSubmissionOperation, error) {
	if tx == nil {
		return nil, gorm.ErrInvalidDB
	}
	var results []*TaskSubmissionOperation
	err := tx.Where(
		"status IN (?, ?)",
		TaskSubmissionOperationStatusSubmissionUnknown, TaskSubmissionOperationStatusOutcomeUnknown,
	).Order("updated_at ASC, id ASC").Limit(clampScanLimit(limit)).Find(&results).Error
	if err != nil {
		return nil, err
	}
	return results, nil
}

// ListClaimableTaskBillingLogOutboxes returns outbox entries that are eligible for worker processing:
// 1. state == 'pending'
// 2. state == 'retryable' AND next_attempt_at <= now
// 3. state == 'claimed' AND claimed_until > 0 AND claimed_until <= now (lease expired)
func ListClaimableTaskBillingLogOutboxes(tx *gorm.DB, now int64, limit int) ([]*TaskBillingLogOutbox, error) {
	if tx == nil {
		return nil, gorm.ErrInvalidDB
	}
	if now < 0 {
		return nil, fmt.Errorf("%w: now timestamp cannot be negative", ErrTaskRecoveryInvalidRecord)
	}
	var results []*TaskBillingLogOutbox
	err := tx.Where(
		"state = ? OR (state = ? AND next_attempt_at <= ?) OR (state = ? AND claimed_until > 0 AND claimed_until <= ?)",
		TaskBillingLogOutboxStatePending,
		TaskBillingLogOutboxStateRetryable, now,
		TaskBillingLogOutboxStateClaimed, now,
	).Order("id ASC").Limit(clampScanLimit(limit)).Find(&results).Error
	if err != nil {
		return nil, err
	}
	return results, nil
}

// ListStaleClaimedTaskBillingEvents returns billing events that have been in 'claimed'
// state past their lease expiry.
func ListStaleClaimedTaskBillingEvents(tx *gorm.DB, now int64, limit int) ([]*TaskBillingEvent, error) {
	if tx == nil {
		return nil, gorm.ErrInvalidDB
	}
	if now < 0 {
		return nil, fmt.Errorf("%w: now timestamp cannot be negative", ErrTaskRecoveryInvalidRecord)
	}
	var results []*TaskBillingEvent
	err := tx.Where(
		"state = ? AND claimed_until > 0 AND claimed_until <= ?",
		TaskBillingEventStateClaimed, now,
	).Order("id ASC").Limit(clampScanLimit(limit)).Find(&results).Error
	if err != nil {
		return nil, err
	}
	return results, nil
}

// FindAttemptByOperationID returns the TaskSubmissionAttempt for an operation.
func FindAttemptByOperationID(tx *gorm.DB, operationID int64) (*TaskSubmissionAttempt, error) {
	if tx == nil {
		return nil, gorm.ErrInvalidDB
	}
	if operationID <= 0 {
		return nil, ErrTaskRecoveryInvalidRecord
	}
	var attempt TaskSubmissionAttempt
	err := tx.Where("operation_id = ?", operationID).First(&attempt).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &attempt, nil
}
