package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"gorm.io/gorm"
)

const (
	AccountQuotaTerminalRecoveryOpen          = "open"
	AccountQuotaTerminalRecoveryRefundPending = "refund_pending"
	AccountQuotaTerminalRecoveryPending       = AccountQuotaTerminalRecoveryRefundPending
	AccountQuotaTerminalRecoveryClaimed       = "claimed"
	AccountQuotaTerminalRecoveryRetryable     = "retryable"
	AccountQuotaTerminalRecoveryApplied       = "applied"
	AccountQuotaTerminalRecoveryManual        = "manual"
	accountQuotaTerminalRecoveryLegacyPending = "pending"

	accountQuotaTerminalRecoveryLeaseSeconds = 30
)

var (
	ErrAccountQuotaTerminalRecoveryConflict = errors.New("account quota terminal recovery obligation conflicts with persisted identity")
	ErrAccountQuotaRefundIntentUnknown      = errors.New("account quota refund intent outcome is unknown")

	accountQuotaRefundIntentAfterCommitHook             func(string) error
	accountQuotaRefundIntentReadbackHook                func() error
	accountQuotaRecoveryManualUpdateHook                func(*AccountQuotaTerminalRecoveryObligation) error
	accountQuotaSettlementManualEvidenceAfterCommitHook func(string) error
	accountQuotaSettlementManualEvidenceReadbackHook    func() error
)

type AccountQuotaTerminalRecoveryObligation struct {
	ID                 int64  `json:"id" gorm:"primaryKey"`
	RequestID          string `json:"request_id" gorm:"type:varchar(64);not null;uniqueIndex"`
	ReserveReceiptID   int64  `json:"reserve_receipt_id" gorm:"type:bigint;not null;index"`
	Phase              string `json:"phase" gorm:"type:varchar(16);not null"`
	AuditKey           string `json:"audit_key" gorm:"type:varchar(96);not null"`
	ActualQuota        int64  `json:"actual_quota" gorm:"type:bigint;not null;default:0"`
	WriterEpoch        int64  `json:"writer_epoch" gorm:"type:bigint;not null;index"`
	RequestFingerprint string `json:"request_fingerprint" gorm:"type:char(64);not null"`
	State              string `json:"state" gorm:"type:varchar(16);not null;index"`
	TerminalReceiptID  int64  `json:"terminal_receipt_id" gorm:"type:bigint;not null;default:0;index"`
	Attempts           int    `json:"attempts" gorm:"not null;default:0"`
	LastError          string `json:"last_error" gorm:"type:text;not null"`
	NextAttemptAt      int64  `json:"next_attempt_at" gorm:"type:bigint;not null;default:0;index"`
	LeaseOwner         string `json:"lease_owner" gorm:"type:varchar(128);not null;default:''"`
	LeaseUntil         int64  `json:"lease_until" gorm:"type:bigint;not null;default:0;index"`
	LockVersion        int64  `json:"lock_version" gorm:"type:bigint;not null;default:1"`
	CreatedAt          int64  `json:"created_at" gorm:"type:bigint;not null"`
	UpdatedAt          int64  `json:"updated_at" gorm:"type:bigint;not null"`
}

func (AccountQuotaTerminalRecoveryObligation) TableName() string {
	return "account_quota_terminal_recovery_obligations"
}

type accountQuotaTerminalRecoveryFingerprintPayload struct {
	Version          int    `json:"version"`
	RequestID        string `json:"request_id"`
	ReserveReceiptID int64  `json:"reserve_receipt_id"`
	Phase            string `json:"phase"`
	AuditKey         string `json:"audit_key"`
	WriterEpoch      int64  `json:"writer_epoch"`
}

func accountQuotaTerminalRecoveryFingerprint(input AccountQuotaTerminalInput, writerEpoch int64) (string, error) {
	data, err := common.Marshal(accountQuotaTerminalRecoveryFingerprintPayload{
		Version: AccountQuotaFingerprintVersion, RequestID: input.RequestID, ReserveReceiptID: input.ReserveReceiptID,
		Phase: AccountQuotaPhaseRefund, AuditKey: input.AuditKey, WriterEpoch: writerEpoch,
	})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

func validateAccountQuotaTerminalRecoveryIdentity(expected, stored *AccountQuotaTerminalRecoveryObligation) error {
	if expected == nil || stored == nil || expected.RequestID != stored.RequestID ||
		expected.ReserveReceiptID != stored.ReserveReceiptID || expected.Phase != stored.Phase ||
		expected.AuditKey != stored.AuditKey || expected.WriterEpoch != stored.WriterEpoch ||
		expected.RequestFingerprint != stored.RequestFingerprint {
		return ErrAccountQuotaTerminalRecoveryConflict
	}
	return nil
}

func validateAccountQuotaTerminalRecoveryRecord(db *gorm.DB, obligation *AccountQuotaTerminalRecoveryObligation) error {
	if db == nil || obligation == nil || obligation.Phase != AccountQuotaPhaseRefund || obligation.LockVersion <= 0 {
		return ErrAccountQuotaTerminalRecoveryConflict
	}
	head, err := findAccountQuotaReservationHead(db, obligation.RequestID)
	if err != nil {
		return err
	}
	if head.WriterEpoch != obligation.WriterEpoch {
		return ErrAccountQuotaTerminalRecoveryConflict
	}
	inChain, err := accountQuotaReceiptInChain(db, head, obligation.ReserveReceiptID)
	if err != nil {
		return err
	}
	if !inChain {
		return ErrAccountQuotaTerminalRecoveryConflict
	}
	var current AccountQuotaMutationReceipt
	if err := db.Where("id = ?", head.CurrentReceiptID).First(&current).Error; err != nil {
		return err
	}
	if err := validateAccountQuotaHeadReceipt(head, &current); err != nil {
		return err
	}
	input := AccountQuotaTerminalInput{RequestID: obligation.RequestID, ReserveReceiptID: obligation.ReserveReceiptID, AuditKey: obligation.AuditKey}
	recoveryFingerprint, err := accountQuotaTerminalRecoveryFingerprint(input, obligation.WriterEpoch)
	if err != nil || recoveryFingerprint != obligation.RequestFingerprint {
		return ErrAccountQuotaTerminalRecoveryConflict
	}
	return nil
}

func createAccountQuotaLifecycleObligation(tx *gorm.DB, head *AccountQuotaReservationHead, reserve *AccountQuotaMutationReceipt) error {
	if tx == nil || head == nil || reserve == nil || head.RequestID != reserve.RequestID || head.WriterEpoch != reserve.WriterEpoch ||
		head.CurrentReceiptID != reserve.ID || head.ReserveFingerprint != reserve.ReserveFingerprint {
		return ErrAccountQuotaTerminalRecoveryConflict
	}
	now := reserve.CreatedAt
	if now <= 0 {
		var err error
		now, err = taskRecoveryDBTimestamp(tx)
		if err != nil {
			return err
		}
	}
	obligation := &AccountQuotaTerminalRecoveryObligation{
		RequestID: reserve.RequestID, ReserveReceiptID: reserve.ID, WriterEpoch: reserve.WriterEpoch,
		RequestFingerprint: reserve.ReserveFingerprint, State: AccountQuotaTerminalRecoveryOpen,
		LockVersion: 1, CreatedAt: now, UpdatedAt: now,
	}
	return tx.Create(obligation).Error
}

func updateAccountQuotaLifecycleReservation(tx *gorm.DB, head *AccountQuotaReservationHead, receipt *AccountQuotaMutationReceipt, now int64) error {
	var obligation AccountQuotaTerminalRecoveryObligation
	if err := lockForUpdate(tx).Where("request_id = ?", head.RequestID).First(&obligation).Error; err != nil {
		return err
	}
	if obligation.State != AccountQuotaTerminalRecoveryOpen || obligation.WriterEpoch != head.WriterEpoch ||
		obligation.RequestFingerprint != head.ReserveFingerprint || obligation.LockVersion <= 0 {
		return ErrAccountQuotaMutationTerminal
	}
	result := tx.Model(&AccountQuotaTerminalRecoveryObligation{}).
		Where("id = ? AND state = ? AND lock_version = ?", obligation.ID, AccountQuotaTerminalRecoveryOpen, obligation.LockVersion).
		Updates(map[string]interface{}{"reserve_receipt_id": receipt.ID, "lock_version": obligation.LockVersion + 1, "updated_at": now})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrAccountQuotaMutationCASLost
	}
	return nil
}

func closeAccountQuotaLifecycle(tx *gorm.DB, head *AccountQuotaReservationHead, input AccountQuotaTerminalInput, phase string, terminal *AccountQuotaMutationReceipt, now int64) error {
	var obligation AccountQuotaTerminalRecoveryObligation
	if err := lockForUpdate(tx).Where("request_id = ?", head.RequestID).First(&obligation).Error; err != nil {
		return err
	}
	if obligation.WriterEpoch != head.WriterEpoch || obligation.LockVersion <= 0 {
		return ErrAccountQuotaTerminalRecoveryConflict
	}
	switch phase {
	case AccountQuotaPhaseSettle:
		if obligation.State != AccountQuotaTerminalRecoveryOpen || obligation.RequestFingerprint != head.ReserveFingerprint {
			return ErrAccountQuotaMutationTerminal
		}
	case AccountQuotaPhaseRefund:
		if obligation.State == AccountQuotaTerminalRecoveryOpen {
			if obligation.RequestFingerprint != head.ReserveFingerprint {
				return ErrAccountQuotaTerminalRecoveryConflict
			}
		} else if obligation.State == AccountQuotaTerminalRecoveryRefundPending || obligation.State == AccountQuotaTerminalRecoveryRetryable || obligation.State == AccountQuotaTerminalRecoveryClaimed {
			expectedFingerprint, err := accountQuotaTerminalRecoveryFingerprint(input, head.WriterEpoch)
			if err != nil || obligation.Phase != AccountQuotaPhaseRefund || obligation.AuditKey != input.AuditKey ||
				obligation.ReserveReceiptID != input.ReserveReceiptID || obligation.RequestFingerprint != expectedFingerprint {
				return ErrAccountQuotaTerminalRecoveryConflict
			}
		} else {
			return ErrAccountQuotaMutationTerminal
		}
	default:
		return ErrAccountQuotaMutationInvalidInput
	}
	result := tx.Model(&AccountQuotaTerminalRecoveryObligation{}).
		Where("id = ? AND lock_version = ?", obligation.ID, obligation.LockVersion).
		Updates(map[string]interface{}{
			"phase": phase, "audit_key": input.AuditKey, "actual_quota": input.ActualQuota, "state": AccountQuotaTerminalRecoveryApplied,
			"terminal_receipt_id": terminal.ID, "last_error": "", "next_attempt_at": int64(0),
			"lease_owner": "", "lease_until": int64(0), "lock_version": obligation.LockVersion + 1, "updated_at": now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrAccountQuotaMutationCASLost
	}
	return nil
}

func validateAccountQuotaSettlementManualEvidenceIdentity(stored *AccountQuotaTerminalRecoveryObligation, input AccountQuotaTerminalInput, writerEpoch int64, fingerprint string) error {
	if stored == nil || stored.RequestID != input.RequestID || stored.ReserveReceiptID != input.ReserveReceiptID ||
		stored.Phase != AccountQuotaPhaseSettle || stored.ActualQuota != input.ActualQuota || stored.WriterEpoch != writerEpoch ||
		stored.RequestFingerprint != fingerprint || stored.State != AccountQuotaTerminalRecoveryManual {
		return ErrAccountQuotaTerminalRecoveryConflict
	}
	return nil
}

// PersistAccountQuotaSettlementManualEvidence records exact settlement input
// on the lifecycle row that was committed with the reserve. It is the final
// fail-closed fallback when neither the settlement inbox nor fact can be
// created; it never applies quota automatically.
func PersistAccountQuotaSettlementManualEvidence(ctx context.Context, db *gorm.DB, input AccountQuotaTerminalInput, cause error) (*AccountQuotaTerminalRecoveryObligation, error) {
	if db == nil || cause == nil {
		return nil, ErrAccountQuotaMutationInvalidInput
	}
	normalized, err := normalizeAccountTerminalInput(input, AccountQuotaPhaseSettle)
	if err != nil {
		return nil, err
	}
	baseCtx := context.Background()
	if ctx != nil {
		baseCtx = context.WithoutCancel(ctx)
	}
	writeCtx, cancel := context.WithTimeout(baseCtx, 2*time.Second)
	defer cancel()
	message := cause.Error()
	if len(message) > 4096 {
		message = message[:4096]
	}
	var stored AccountQuotaTerminalRecoveryObligation
	var expectedWriterEpoch int64
	var expectedFingerprint string
	err = db.WithContext(writeCtx).Transaction(func(tx *gorm.DB) error {
		head, err := findAccountQuotaReservationHead(tx, normalized.RequestID)
		if err != nil {
			return err
		}
		var lockedHead AccountQuotaReservationHead
		if err := lockForUpdate(tx).Where("id = ?", head.ID).First(&lockedHead).Error; err != nil {
			return err
		}
		if lockedHead.TerminalReceiptID > 0 {
			return ErrAccountQuotaMutationTerminal
		}
		inChain, err := accountQuotaReceiptInChain(tx, &lockedHead, normalized.ReserveReceiptID)
		if err != nil || !inChain {
			if err != nil {
				return err
			}
			return ErrAccountQuotaMutationStaleReceipt
		}
		var current AccountQuotaMutationReceipt
		if err := lockForUpdate(tx).Where("id = ?", lockedHead.CurrentReceiptID).First(&current).Error; err != nil {
			return err
		}
		fingerprint, err := accountQuotaTerminalFingerprint(normalized, AccountQuotaPhaseSettle, &lockedHead, &current)
		if err != nil {
			return err
		}
		expectedWriterEpoch = lockedHead.WriterEpoch
		expectedFingerprint = fingerprint
		if err := lockForUpdate(tx).Where("request_id = ?", normalized.RequestID).First(&stored).Error; err != nil {
			return err
		}
		if stored.State == AccountQuotaTerminalRecoveryManual {
			if validateAccountQuotaSettlementManualEvidenceIdentity(&stored, normalized, expectedWriterEpoch, expectedFingerprint) == nil {
				return nil
			}
			return ErrAccountQuotaTerminalRecoveryConflict
		}
		if stored.State != AccountQuotaTerminalRecoveryOpen || stored.RequestFingerprint != lockedHead.ReserveFingerprint {
			return ErrAccountQuotaTerminalRecoveryConflict
		}
		now, err := taskRecoveryDBTimestamp(tx)
		if err != nil {
			return err
		}
		result := tx.Model(&AccountQuotaTerminalRecoveryObligation{}).
			Where("id = ? AND state = ? AND lock_version = ?", stored.ID, AccountQuotaTerminalRecoveryOpen, stored.LockVersion).
			Updates(map[string]interface{}{
				"reserve_receipt_id": normalized.ReserveReceiptID, "phase": AccountQuotaPhaseSettle,
				"actual_quota": normalized.ActualQuota, "request_fingerprint": fingerprint,
				"state": AccountQuotaTerminalRecoveryManual, "last_error": message,
				"lock_version": stored.LockVersion + 1, "updated_at": now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrAccountQuotaMutationCASLost
		}
		return tx.First(&stored, stored.ID).Error
	})
	if err == nil && accountQuotaSettlementManualEvidenceAfterCommitHook != nil {
		err = accountQuotaSettlementManualEvidenceAfterCommitHook(normalized.RequestID)
	}
	if err == nil {
		return &stored, nil
	}
	if expectedWriterEpoch <= 0 || expectedFingerprint == "" {
		return nil, err
	}
	readCtx, readCancel := context.WithTimeout(context.WithoutCancel(writeCtx), 2*time.Second)
	defer readCancel()
	if accountQuotaSettlementManualEvidenceReadbackHook != nil {
		if readErr := accountQuotaSettlementManualEvidenceReadbackHook(); readErr != nil {
			return nil, errors.Join(err, readErr)
		}
	}
	var readback AccountQuotaTerminalRecoveryObligation
	readErr := db.WithContext(readCtx).Where("request_id = ?", normalized.RequestID).First(&readback).Error
	if readErr != nil {
		return nil, errors.Join(err, readErr)
	}
	identityErr := validateAccountQuotaSettlementManualEvidenceIdentity(&readback, normalized, expectedWriterEpoch, expectedFingerprint)
	if identityErr == nil {
		return &readback, nil
	}
	return nil, errors.Join(err, identityErr)
}

// EnsureAccountQuotaRefundRecovery persists the refund intent before any refund
// attempt. The lifecycle row is created atomically with the authoritative
// reserve; this method only claims that existing obligation or resolves an
// already committed terminal receipt.
func EnsureAccountQuotaRefundRecovery(ctx context.Context, db *gorm.DB, input AccountQuotaTerminalInput, cause error) (*AccountQuotaTerminalRecoveryObligation, *AccountQuotaMutationReceipt, error) {
	if db == nil {
		return nil, nil, gorm.ErrInvalidDB
	}
	normalized, err := normalizeAccountTerminalInput(input, AccountQuotaPhaseRefund)
	if err != nil {
		return nil, nil, err
	}
	normalized.ActualQuota = 0
	baseCtx := context.Background()
	if ctx != nil {
		baseCtx = context.WithoutCancel(ctx)
	}
	detached, cancel := context.WithTimeout(baseCtx, 2*time.Second)
	defer cancel()

	var obligation *AccountQuotaTerminalRecoveryObligation
	var terminal *AccountQuotaMutationReceipt
	err = db.WithContext(detached).Transaction(func(tx *gorm.DB) error {
		head, err := findAccountQuotaReservationHead(tx, normalized.RequestID)
		if err != nil {
			return err
		}
		var lockedHead AccountQuotaReservationHead
		if err := lockForUpdate(tx).Where("id = ?", head.ID).First(&lockedHead).Error; err != nil {
			return err
		}
		inChain, err := accountQuotaReceiptInChain(tx, &lockedHead, normalized.ReserveReceiptID)
		if err != nil || !inChain {
			if err != nil {
				return err
			}
			return ErrAccountQuotaMutationStaleReceipt
		}
		if lockedHead.TerminalReceiptID > 0 {
			var receipt AccountQuotaMutationReceipt
			if err := tx.First(&receipt, lockedHead.TerminalReceiptID).Error; err != nil {
				return err
			}
			if receipt.Phase != AccountQuotaPhaseRefund {
				return ErrAccountQuotaMutationTerminal
			}
			terminal = &receipt
		}
		var current AccountQuotaMutationReceipt
		if err := tx.Where("id = ?", lockedHead.CurrentReceiptID).First(&current).Error; err != nil {
			return err
		}
		if err := validateAccountQuotaHeadReceipt(&lockedHead, &current); err != nil {
			return err
		}
		var stored AccountQuotaTerminalRecoveryObligation
		if err := lockForUpdate(tx).Where("request_id = ?", normalized.RequestID).First(&stored).Error; err != nil {
			return err
		}
		if stored.WriterEpoch != lockedHead.WriterEpoch || stored.LockVersion <= 0 {
			return ErrAccountQuotaTerminalRecoveryConflict
		}
		if terminal != nil {
			if stored.State != AccountQuotaTerminalRecoveryApplied || stored.TerminalReceiptID != terminal.ID {
				return ErrAccountQuotaTerminalRecoveryConflict
			}
			obligation = &stored
			return nil
		}
		recoveryFingerprint, err := accountQuotaTerminalRecoveryFingerprint(normalized, lockedHead.WriterEpoch)
		if err != nil {
			return err
		}
		lastError := ""
		if cause != nil {
			lastError = cause.Error()
			if len(lastError) > 4096 {
				lastError = lastError[:4096]
			}
		}
		switch stored.State {
		case AccountQuotaTerminalRecoveryOpen:
			now, err := taskRecoveryDBTimestamp(tx)
			if err != nil {
				return err
			}
			result := tx.Model(&AccountQuotaTerminalRecoveryObligation{}).
				Where("id = ? AND state = ? AND lock_version = ?", stored.ID, AccountQuotaTerminalRecoveryOpen, stored.LockVersion).
				Updates(map[string]interface{}{
					"reserve_receipt_id": normalized.ReserveReceiptID, "phase": AccountQuotaPhaseRefund,
					"audit_key": normalized.AuditKey, "request_fingerprint": recoveryFingerprint,
					"state": AccountQuotaTerminalRecoveryRefundPending, "last_error": lastError,
					"next_attempt_at": int64(0), "lease_owner": "", "lease_until": int64(0),
					"lock_version": stored.LockVersion + 1, "updated_at": now,
				})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return ErrAccountQuotaTerminalRecoveryConflict
			}
			if err := tx.First(&stored, stored.ID).Error; err != nil {
				return err
			}
		case AccountQuotaTerminalRecoveryRefundPending, AccountQuotaTerminalRecoveryRetryable, AccountQuotaTerminalRecoveryClaimed:
			expected := &AccountQuotaTerminalRecoveryObligation{
				RequestID: normalized.RequestID, ReserveReceiptID: normalized.ReserveReceiptID, Phase: AccountQuotaPhaseRefund,
				AuditKey: normalized.AuditKey, WriterEpoch: lockedHead.WriterEpoch, RequestFingerprint: recoveryFingerprint,
			}
			if err := validateAccountQuotaTerminalRecoveryIdentity(expected, &stored); err != nil {
				return err
			}
			if cause != nil && stored.State != AccountQuotaTerminalRecoveryClaimed {
				now, err := taskRecoveryDBTimestamp(tx)
				if err != nil {
					return err
				}
				result := tx.Model(&AccountQuotaTerminalRecoveryObligation{}).
					Where("id = ? AND state = ? AND lock_version = ?", stored.ID, stored.State, stored.LockVersion).
					Updates(map[string]interface{}{"last_error": lastError, "lock_version": stored.LockVersion + 1, "updated_at": now})
				if result.Error != nil || result.RowsAffected != 1 {
					if result.Error != nil {
						return result.Error
					}
					return ErrAccountQuotaTerminalRecoveryConflict
				}
				if err := tx.First(&stored, stored.ID).Error; err != nil {
					return err
				}
			}
		case AccountQuotaTerminalRecoveryApplied:
			return ErrAccountQuotaMutationTerminal
		default:
			return ErrAccountQuotaTerminalRecoveryConflict
		}
		obligation = &stored
		return nil
	})
	if err == nil && accountQuotaRefundIntentAfterCommitHook != nil {
		err = accountQuotaRefundIntentAfterCommitHook(normalized.RequestID)
	}
	if err == nil {
		return obligation, terminal, nil
	}
	readbackCtx, readbackCancel := context.WithTimeout(context.WithoutCancel(detached), 2*time.Second)
	defer readbackCancel()
	if accountQuotaRefundIntentReadbackHook != nil {
		if readbackErr := accountQuotaRefundIntentReadbackHook(); readbackErr != nil {
			return nil, nil, errors.Join(err, fmt.Errorf("%w: %w", ErrAccountQuotaRefundIntentUnknown, readbackErr))
		}
	}
	var stored AccountQuotaTerminalRecoveryObligation
	if readbackErr := db.WithContext(readbackCtx).Where("request_id = ?", normalized.RequestID).First(&stored).Error; readbackErr != nil {
		return nil, nil, errors.Join(err, fmt.Errorf("%w: %w", ErrAccountQuotaRefundIntentUnknown, readbackErr))
	}
	if stored.State == AccountQuotaTerminalRecoveryApplied && stored.TerminalReceiptID > 0 && stored.Phase == AccountQuotaPhaseRefund {
		var receipt AccountQuotaMutationReceipt
		if readbackErr := db.WithContext(readbackCtx).First(&receipt, stored.TerminalReceiptID).Error; readbackErr != nil {
			return nil, nil, errors.Join(err, fmt.Errorf("%w: terminal readback: %v", ErrAccountQuotaRefundIntentUnknown, readbackErr))
		}
		return &stored, &receipt, nil
	}
	if stored.State == AccountQuotaTerminalRecoveryRefundPending || stored.State == AccountQuotaTerminalRecoveryRetryable || stored.State == AccountQuotaTerminalRecoveryClaimed {
		expectedFingerprint, fingerprintErr := accountQuotaTerminalRecoveryFingerprint(normalized, stored.WriterEpoch)
		expected := &AccountQuotaTerminalRecoveryObligation{
			RequestID: normalized.RequestID, ReserveReceiptID: normalized.ReserveReceiptID, Phase: AccountQuotaPhaseRefund,
			AuditKey: normalized.AuditKey, WriterEpoch: stored.WriterEpoch, RequestFingerprint: expectedFingerprint,
		}
		if fingerprintErr == nil && validateAccountQuotaTerminalRecoveryIdentity(expected, &stored) == nil {
			return &stored, nil, nil
		}
	}
	return nil, nil, err
}

func claimAccountQuotaTerminalRecovery(db *gorm.DB, candidate *AccountQuotaTerminalRecoveryObligation, workerID string, now int64) (*AccountQuotaTerminalRecoveryObligation, bool, error) {
	if candidate == nil || strings.TrimSpace(workerID) == "" {
		return nil, false, ErrAccountQuotaTerminalRecoveryConflict
	}
	claimable := candidate.State == AccountQuotaTerminalRecoveryRefundPending ||
		(candidate.State == AccountQuotaTerminalRecoveryRetryable && candidate.NextAttemptAt <= now) ||
		(candidate.State == AccountQuotaTerminalRecoveryClaimed && candidate.LeaseUntil <= now)
	if !claimable {
		return candidate, false, nil
	}
	result := db.Model(&AccountQuotaTerminalRecoveryObligation{}).
		Where("id = ? AND state = ? AND lock_version = ? AND lease_until = ? AND next_attempt_at = ?",
			candidate.ID, candidate.State, candidate.LockVersion, candidate.LeaseUntil, candidate.NextAttemptAt).
		Updates(map[string]interface{}{
			"state": AccountQuotaTerminalRecoveryClaimed, "lease_owner": workerID,
			"lease_until": now + accountQuotaTerminalRecoveryLeaseSeconds, "next_attempt_at": 0,
			"attempts": candidate.Attempts + 1, "lock_version": candidate.LockVersion + 1, "updated_at": now,
		})
	if result.Error != nil || result.RowsAffected != 1 {
		return candidate, false, result.Error
	}
	var claimed AccountQuotaTerminalRecoveryObligation
	if err := db.First(&claimed, candidate.ID).Error; err != nil {
		return nil, false, err
	}
	return &claimed, true, nil
}

func finishAccountQuotaTerminalRecovery(db *gorm.DB, claimed *AccountQuotaTerminalRecoveryObligation, receipt *AccountQuotaMutationReceipt, runErr error) error {
	if runErr == nil && receipt != nil {
		var stored AccountQuotaTerminalRecoveryObligation
		if err := db.First(&stored, claimed.ID).Error; err != nil {
			return err
		}
		if stored.State == AccountQuotaTerminalRecoveryApplied && stored.TerminalReceiptID == receipt.ID {
			return nil
		}
		return ErrAccountQuotaTerminalRecoveryConflict
	}
	now, err := taskRecoveryDBTimestamp(db)
	if err != nil {
		return err
	}
	updates := map[string]interface{}{
		"lease_owner": "", "lease_until": 0, "lock_version": claimed.LockVersion + 1, "updated_at": now,
	}
	{
		message := "refund recovery failed"
		if runErr != nil {
			message = runErr.Error()
		}
		if len(message) > 4096 {
			message = message[:4096]
		}
		updates["state"] = AccountQuotaTerminalRecoveryRetryable
		updates["last_error"] = message
		updates["next_attempt_at"] = now + int64(min(claimed.Attempts+1, 30))
	}
	result := db.Model(&AccountQuotaTerminalRecoveryObligation{}).
		Where("id = ? AND state = ? AND lease_owner = ? AND lock_version = ?", claimed.ID, AccountQuotaTerminalRecoveryClaimed, claimed.LeaseOwner, claimed.LockVersion).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrAccountQuotaTerminalRecoveryConflict
	}
	return nil
}

func accountQuotaRecoveryStructuralError(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, ErrAccountQuotaMutationNotFound) ||
		errors.Is(err, ErrAccountQuotaTerminalRecoveryConflict) || errors.Is(err, ErrAccountQuotaMutationStaleReceipt) ||
		errors.Is(err, ErrAccountQuotaMutationInvalidInput)
}

func markAccountQuotaTerminalRecoveryManual(db *gorm.DB, candidate *AccountQuotaTerminalRecoveryObligation, cause error) error {
	if db == nil || candidate == nil || cause == nil {
		return ErrAccountQuotaTerminalRecoveryConflict
	}
	now, err := taskRecoveryDBTimestamp(db)
	if err != nil {
		return err
	}
	message := cause.Error()
	if len(message) > 4096 {
		message = message[:4096]
	}
	if accountQuotaRecoveryManualUpdateHook != nil {
		if err := accountQuotaRecoveryManualUpdateHook(candidate); err != nil {
			return err
		}
	}
	result := db.Model(&AccountQuotaTerminalRecoveryObligation{}).
		Where("id = ? AND state = ? AND lock_version = ?", candidate.ID, candidate.State, candidate.LockVersion).
		Updates(map[string]interface{}{
			"state": AccountQuotaTerminalRecoveryManual, "last_error": message, "lease_owner": "", "lease_until": int64(0),
			"lock_version": candidate.LockVersion + 1, "updated_at": now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		var stored AccountQuotaTerminalRecoveryObligation
		if err := db.First(&stored, candidate.ID).Error; err == nil && stored.State == AccountQuotaTerminalRecoveryManual {
			return nil
		}
		return ErrAccountQuotaTerminalRecoveryConflict
	}
	return nil
}

func RunAccountQuotaTerminalRecoveryObligations(ctx context.Context, db *gorm.DB, workerID string, limit int) (int, error) {
	if !QuotaProjectionObligationRecoveryEnabled() {
		return 0, nil
	}
	if db == nil || strings.TrimSpace(workerID) == "" {
		return 0, gorm.ErrInvalidDB
	}
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
	}
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	db = db.WithContext(ctx)
	now, err := taskRecoveryDBTimestamp(db)
	if err != nil {
		return 0, err
	}
	workCursor, err := loadQuotaWorkCursor(ctx, db, quotaWorkCursorRefundRecovery)
	if err != nil {
		return 0, err
	}
	if err := beginQuotaWorkCycle(ctx, db, workCursor, &AccountQuotaTerminalRecoveryObligation{}); err != nil {
		return 0, err
	}
	if workCursor.HighWatermark == 0 {
		return 0, nil
	}
	processed := 0
	scanned := 0
	var runErrors []error
	for scanned < limit {
		pageSize := min(25, limit-scanned)
		var candidates []AccountQuotaTerminalRecoveryObligation
		if err := db.Where("id > ? AND id <= ? AND (state = ? OR (state = ? AND next_attempt_at <= ?) OR (state = ? AND lease_until <= ?))",
			workCursor.LastID, workCursor.HighWatermark, AccountQuotaTerminalRecoveryRefundPending, AccountQuotaTerminalRecoveryRetryable, now,
			AccountQuotaTerminalRecoveryClaimed, now).Order("id ASC").Limit(pageSize).Find(&candidates).Error; err != nil {
			return processed, errors.Join(append(runErrors, err)...)
		}
		if len(candidates) == 0 {
			if err := finishQuotaWorkCycle(ctx, db, workCursor); err != nil {
				return processed, errors.Join(append(runErrors, err)...)
			}
			return processed, errors.Join(runErrors...)
		}
		for index := range candidates {
			candidate := &candidates[index]
			workCursor.LastID = candidate.ID
			scanned++
			if err := ctx.Err(); err != nil {
				return processed, errors.Join(append(runErrors, err)...)
			}
			if validationErr := validateAccountQuotaTerminalRecoveryRecord(db, candidate); validationErr != nil {
				if accountQuotaRecoveryStructuralError(validationErr) {
					if manualErr := markAccountQuotaTerminalRecoveryManual(db, candidate, validationErr); manualErr != nil {
						runErrors = append(runErrors, errors.Join(validationErr, manualErr))
					} else {
						runErrors = append(runErrors, validationErr)
					}
				} else {
					runErrors = append(runErrors, validationErr)
				}
				continue
			}
			claimed, won, claimErr := claimAccountQuotaTerminalRecovery(db, candidate, workerID, now)
			if claimErr != nil {
				runErrors = append(runErrors, claimErr)
				continue
			}
			if !won {
				continue
			}
			processed++
			attemptCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
			receipt, refundErr := RefundAccountQuota(attemptCtx, db, AccountQuotaTerminalInput{
				RequestID: claimed.RequestID, ReserveReceiptID: claimed.ReserveReceiptID, AuditKey: claimed.AuditKey,
			})
			cancel()
			if finishErr := finishAccountQuotaTerminalRecovery(db, claimed, receipt, refundErr); finishErr != nil {
				runErrors = append(runErrors, finishErr)
				continue
			}
			if refundErr != nil {
				runErrors = append(runErrors, refundErr)
			}
		}
		if err := saveQuotaWorkCursor(ctx, db, workCursor); err != nil {
			return processed, errors.Join(append(runErrors, err)...)
		}
	}
	if err := saveQuotaWorkCursor(ctx, db, workCursor); err != nil {
		return processed, errors.Join(append(runErrors, err)...)
	}
	return processed, errors.Join(runErrors...)
}
