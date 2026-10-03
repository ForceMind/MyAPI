package model

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"gorm.io/gorm"
)

var ErrAccountQuotaUsageUnresolved = errors.New("account quota usage requires reconciliation")

// HoldAccountQuotaUnknownUsage freezes the existing reservation lifecycle. It
// records no actual charge and creates no refund or settlement intent. The
// reservation head serializes this operation with terminal mutations; ordinary
// settle, refund, extension and background refund recovery already require an
// open lifecycle and therefore cannot clear this hold.
func HoldAccountQuotaUnknownUsage(parent context.Context, db *gorm.DB, requestID string, reserveReceiptID int64, reason string, metadata ...string) (*AccountQuotaTerminalRecoveryObligation, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	requestID, err := normalizeAccountRequestID(requestID)
	if err != nil || reserveReceiptID <= 0 {
		return nil, ErrAccountQuotaMutationInvalidInput
	}
	reviewMetadata := ""
	if len(metadata) > 1 {
		return nil, ErrAccountQuotaMutationInvalidInput
	}
	if len(metadata) == 1 {
		reviewMetadata = metadata[0]
	}
	if len(reviewMetadata) > 16384 {
		return nil, ErrAccountQuotaMutationInvalidInput
	}
	switch reason {
	case "missing", "estimated", "invalid", "partial":
	default:
		return nil, ErrAccountQuotaMutationInvalidInput
	}
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), 5*time.Second)
	defer cancel()
	var stored AccountQuotaTerminalRecoveryObligation
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var head AccountQuotaReservationHead
		if err := lockForUpdate(tx).Where("request_id = ?", requestID).First(&head).Error; err != nil {
			return err
		}
		if head.RequestID != requestID {
			return ErrAccountQuotaMutationConflict
		}
		if head.TerminalReceiptID != 0 {
			return ErrAccountQuotaMutationTerminal
		}
		inChain, err := accountQuotaReceiptInChain(tx, &head, reserveReceiptID)
		if err != nil {
			return err
		}
		if !inChain {
			return ErrAccountQuotaMutationStaleReceipt
		}
		var current AccountQuotaMutationReceipt
		if err := tx.First(&current, head.CurrentReceiptID).Error; err != nil {
			return err
		}
		if err := validateAccountQuotaHeadReceipt(&head, &current); err != nil {
			return err
		}
		if err := lockForUpdate(tx).Where("request_id = ?", requestID).First(&stored).Error; err != nil {
			return err
		}
		if stored.WriterEpoch != head.WriterEpoch || stored.LockVersion <= 0 ||
			stored.RequestFingerprint != head.ReserveFingerprint || stored.ReserveReceiptID != current.ID {
			return ErrAccountQuotaTerminalRecoveryConflict
		}
		if stored.State == AccountQuotaTerminalRecoveryUsageUnknown {
			// A replay does not replace the first observation or its timestamp.
			return nil
		}
		if stored.State != AccountQuotaTerminalRecoveryOpen {
			return ErrAccountQuotaMutationTerminal
		}
		now, err := taskRecoveryDBTimestamp(tx)
		if err != nil {
			return err
		}
		result := tx.Model(&AccountQuotaTerminalRecoveryObligation{}).
			Where("id = ? AND state = ? AND lock_version = ?", stored.ID, AccountQuotaTerminalRecoveryOpen, stored.LockVersion).
			Updates(map[string]interface{}{
				"state": AccountQuotaTerminalRecoveryUsageUnknown, "last_error": "usage_" + reason, "review_metadata": reviewMetadata,
				"lock_version": stored.LockVersion + 1, "updated_at": now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrAccountQuotaMutationCASLost
		}
		stored.State = AccountQuotaTerminalRecoveryUsageUnknown
		stored.LastError = "usage_" + reason
		stored.ReviewMetadata = reviewMetadata
		stored.LockVersion++
		stored.UpdatedAt = now
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &stored, nil
}

// AccountQuotaUsageNeedsReview reads the primary DB, never a cached Token. A
// caller must still serialize strict-budget admission with its quota mutation;
// this read alone is not a concurrency-safe budget reservation.
func AccountQuotaUsageNeedsReview(ctx context.Context, db *gorm.DB, tokenID int) (bool, error) {
	if db == nil {
		return false, gorm.ErrInvalidDB
	}
	if tokenID <= 0 {
		return false, ErrAccountQuotaMutationInvalidInput
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var row AccountQuotaReservationHead
	query := db.WithContext(ctx).Model(&AccountQuotaReservationHead{}).
		Where("token_id = ? AND terminal_receipt_id = ?", tokenID, 0).
		Where("request_id IN (?)", db.Model(&AccountQuotaTerminalRecoveryObligation{}).
			Select("request_id").Where("state = ?", AccountQuotaTerminalRecoveryUsageUnknown))
	result := query.Limit(1).Find(&row)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0 && strings.TrimSpace(row.RequestID) != "", nil
}

// ResolveAccountQuotaUnknownUsage settles a held authoritative reservation using
// explicitly reviewed evidence. ActualQuota is the existing internal quota unit,
// NOT tokens or USD. The caller must derive it from verified usage and the frozen
// request price. EvidenceDigest is an audit reference, not proof by itself.
// Root identity is checked against the primary database inside the transaction.
// The hold is never observably reopened: clearing it and creating the immutable
// terminal receipt either commit together or both roll back.
func ResolveAccountQuotaUnknownUsage(ctx context.Context, db *gorm.DB, actorID int, input AccountQuotaTerminalInput, evidenceDigest string) (*AccountQuotaMutationReceipt, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	digest, err := hex.DecodeString(evidenceDigest)
	if actorID <= 0 || err != nil || len(digest) != 32 || evidenceDigest != strings.ToLower(evidenceDigest) {
		return nil, ErrAccountQuotaMutationInvalidInput
	}
	input.AuditKey = fmt.Sprintf("usage-review:%d:%s", actorID, evidenceDigest)
	input, err = normalizeAccountTerminalInput(input, AccountQuotaPhaseSettle)
	if err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return applyAccountQuotaTerminalReviewed(ctx, db, input, AccountQuotaPhaseSettle, actorID)
}

func authorizeUsageReviewer(db *gorm.DB, actorID int) error {
	var actor User
	if err := db.First(&actor, actorID).Error; err != nil {
		return err
	}
	if actor.Role != common.RoleRootUser || actor.Status != common.UserStatusEnabled {
		return ErrAccountQuotaMutationIneligible
	}
	return nil
}

// Called only within the terminal mutation transaction, after the established
// User -> Token -> Subscription -> Head -> Receipt lock sequence.
func reopenReviewedUsageTx(tx *gorm.DB, head *AccountQuotaReservationHead, actorID int, actual int64) error {
	if err := authorizeUsageReviewer(tx, actorID); err != nil {
		return err
	}
	var hold AccountQuotaTerminalRecoveryObligation
	if err := lockForUpdate(tx).Where("request_id = ?", head.RequestID).First(&hold).Error; err != nil {
		return err
	}
	if hold.State != AccountQuotaTerminalRecoveryUsageUnknown || hold.WriterEpoch != head.WriterEpoch ||
		hold.RequestFingerprint != head.ReserveFingerprint || hold.LockVersion <= 0 {
		return ErrAccountQuotaTerminalRecoveryConflict
	}
	if err := validateReviewedQuotaObligations(hold.ReviewMetadata, actual); err != nil {
		return err
	}
	result := tx.Model(&AccountQuotaTerminalRecoveryObligation{}).
		Where("id = ? AND state = ? AND lock_version = ?", hold.ID, AccountQuotaTerminalRecoveryUsageUnknown, hold.LockVersion).
		Updates(map[string]interface{}{"state": AccountQuotaTerminalRecoveryOpen, "lock_version": hold.LockVersion + 1})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrAccountQuotaMutationCASLost
	}
	return nil
}
