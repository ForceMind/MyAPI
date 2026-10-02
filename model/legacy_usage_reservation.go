package model

import (
	"context"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/ForceMind/MyAPI/common"
	"gorm.io/gorm"
)

const (
	LegacyUsagePreparing       = "preparing"
	LegacyUsageAdmissionFailed = "admission_failed"
	LegacyUsagePrepared        = "prepared"
	LegacyUsageUnknown         = "usage_unknown"
	LegacyUsageSettled         = "settled"
	LegacyUsageRefunded        = "refunded"
	LegacyUsageReviewPending   = "review_pending"
)

// Claim the request before any legacy reserve. The unique request key prevents
// two sessions with the same identity from each debiting before journal insert.
func ClaimLegacyUsageRequest(ctx context.Context, db *gorm.DB, requestID string, userID, tokenID int) error {
	if db == nil {
		return gorm.ErrInvalidDB
	}
	id, err := normalizeAccountRequestID(requestID)
	if err != nil || id != requestID || userID <= 0 || tokenID < 0 {
		return ErrAccountQuotaMutationInvalidInput
	}
	if ctx == nil {
		ctx = context.Background()
	}
	now, err := taskRecoveryDBTimestamp(db.WithContext(ctx))
	if err != nil {
		return err
	}
	row := LegacyUsageReservation{RequestID: id, UserID: userID, TokenID: tokenID, FundingSource: "pending", State: LegacyUsagePreparing, LockVersion: 1, CreatedAt: now, UpdatedAt: now}
	return db.WithContext(ctx).Create(&row).Error
}

func BindLegacyUsageReservation(ctx context.Context, db *gorm.DB, input LegacyUsageReservation) error {
	if db == nil {
		return gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if input.FundingSource != "wallet" && input.FundingSource != "subscription" && input.FundingSource != "free" {
		return ErrAccountQuotaMutationInvalidInput
	}
	if (input.FundingSource == "subscription") != (input.SubscriptionID > 0) || validateAccountQuotaValue(input.ReservedQuota) != nil || validateAccountQuotaValue(input.TokenReservedQuota) != nil {
		return ErrAccountQuotaMutationInvalidInput
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row LegacyUsageReservation
		if err := lockForUpdate(tx).Where("request_id = ?", input.RequestID).First(&row).Error; err != nil {
			return err
		}
		if row.RequestID != input.RequestID || row.UserID != input.UserID || row.TokenID != input.TokenID || row.State != LegacyUsagePreparing {
			return ErrAccountQuotaMutationConflict
		}
		now, err := taskRecoveryDBTimestamp(tx)
		if err != nil {
			return err
		}
		result := tx.Model(&LegacyUsageReservation{}).Where("id = ? AND state = ? AND lock_version = ?", row.ID, LegacyUsagePreparing, row.LockVersion).
			Updates(map[string]interface{}{"funding_source": input.FundingSource, "subscription_id": input.SubscriptionID, "channel_id": input.ChannelID, "model_name": input.ModelName, "reserved_quota": input.ReservedQuota, "token_reserved_quota": input.TokenReservedQuota, "state": LegacyUsagePrepared, "updated_at": now, "lock_version": row.LockVersion + 1})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrAccountQuotaMutationCASLost
		}
		return nil
	})
}

// This state proves no upstream request was sent; existing refund facts still
// own compensation. It never asserts that the debit has already been refunded.
func FailLegacyUsageAdmission(ctx context.Context, db *gorm.DB, requestID string) error {
	if db == nil {
		return gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return db.WithContext(ctx).Model(&LegacyUsageReservation{}).Where("request_id = ? AND state IN ?", requestID, []string{LegacyUsagePreparing, LegacyUsagePrepared}).Update("state", LegacyUsageAdmissionFailed).Error
}

// LegacyUsageReservation preserves request identity before an upstream send.
// It is not a replacement balance ledger. All balance writes continue through
// existing settlement/refund facts. ReservedQuota is NOT a confirmed charge;
// ActualQuota remains NULL until a known settlement completes.
type LegacyUsageReservation struct {
	ID                 int64  `json:"id" gorm:"primaryKey"`
	RequestID          string `json:"request_id" gorm:"type:varchar(64);not null;uniqueIndex"`
	UserID             int    `json:"user_id" gorm:"not null;index"`
	TokenID            int    `json:"token_id" gorm:"not null;index"`
	ChannelID          int    `json:"channel_id" gorm:"not null;default:0"`
	ModelName          string `json:"model_name" gorm:"size:512"`
	FundingSource      string `json:"funding_source" gorm:"type:varchar(32);not null"`
	SubscriptionID     int    `json:"subscription_id" gorm:"not null;default:0"`
	ReservedQuota      int64  `json:"reserved_quota" gorm:"type:bigint;not null"`
	TokenReservedQuota int64  `json:"token_reserved_quota" gorm:"type:bigint;not null"`
	ActualQuota        *int64 `json:"actual_quota" gorm:"type:bigint"`
	State              string `json:"state" gorm:"type:varchar(16);not null;index"`
	Reason             string `json:"reason" gorm:"type:varchar(32);not null"`
	ReviewMetadata     string `json:"review_metadata,omitempty" gorm:"type:text"`
	ReviewedBy         int    `json:"reviewed_by" gorm:"not null;default:0"`
	EvidenceDigest     string `json:"evidence_digest" gorm:"type:varchar(64);not null;default:''"`
	LockVersion        int64  `json:"lock_version" gorm:"type:bigint;not null"`
	CreatedAt          int64  `json:"created_at" gorm:"type:bigint;not null"`
	UpdatedAt          int64  `json:"updated_at" gorm:"type:bigint;not null"`
}

func (LegacyUsageReservation) TableName() string { return "legacy_usage_reservations" }

func PrepareLegacyUsageReservation(ctx context.Context, db *gorm.DB, input LegacyUsageReservation) (*LegacyUsageReservation, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	requestID, err := normalizeAccountRequestID(input.RequestID)
	if err != nil || input.UserID <= 0 || input.TokenID < 0 || input.SubscriptionID < 0 ||
		validateAccountQuotaValue(input.ReservedQuota) != nil || validateAccountQuotaValue(input.TokenReservedQuota) != nil {
		return nil, ErrAccountQuotaMutationInvalidInput
	}
	if input.FundingSource != "wallet" && input.FundingSource != "subscription" && input.FundingSource != "free" {
		return nil, ErrAccountQuotaMutationInvalidInput
	}
	if (input.FundingSource == "subscription") != (input.SubscriptionID > 0) {
		return nil, ErrAccountQuotaMutationInvalidInput
	}
	if ctx == nil {
		ctx = context.Background()
	}
	input.RequestID = requestID
	input.ID, input.ActualQuota = 0, nil
	input.State, input.Reason, input.LockVersion = LegacyUsagePrepared, "", 1
	now, err := taskRecoveryDBTimestamp(db.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	input.CreatedAt, input.UpdatedAt = now, now
	if err := db.WithContext(ctx).Create(&input).Error; err != nil {
		// Do not replay a fresh pre-consume against an existing request. Its
		// reservation might already be held; a second debit is not equivalent.
		return nil, err
	}
	return &input, nil
}

func FindLegacyUsageReservation(ctx context.Context, db *gorm.DB, requestID string) (*LegacyUsageReservation, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var row LegacyUsageReservation
	if err := db.WithContext(ctx).Where("request_id = ?", strings.TrimSpace(requestID)).First(&row).Error; err != nil {
		return nil, err
	}
	if row.RequestID != strings.TrimSpace(requestID) {
		return nil, gorm.ErrRecordNotFound
	}
	return &row, nil
}

// UpdateLegacyUsageReservation uses the same session's current reservation
// totals. It never performs a quota mutation or manufactures an actual amount.
func UpdateLegacyUsageReservation(ctx context.Context, db *gorm.DB, requestID string, reserved, tokenReserved int64, state, reason string, actual *int64, metadata ...string) error {
	if db == nil {
		return gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if len(metadata) > 1 || (len(metadata) == 1 && len(metadata[0]) > 16384) {
		return ErrAccountQuotaMutationInvalidInput
	}
	if reserved < 0 || reserved > int64(common.MaxQuota) || tokenReserved < 0 || tokenReserved > int64(common.MaxQuota) {
		return ErrAccountQuotaMutationInvalidInput
	}
	switch state {
	case LegacyUsagePrepared, LegacyUsageUnknown, LegacyUsageRefunded:
		if actual != nil {
			return ErrAccountQuotaMutationInvalidInput
		}
	case LegacyUsageSettled:
		if actual == nil || validateAccountQuotaValue(*actual) != nil {
			return ErrAccountQuotaMutationInvalidInput
		}
	default:
		return ErrAccountQuotaMutationInvalidInput
	}
	if state == LegacyUsageUnknown {
		switch reason {
		case "missing", "estimated", "invalid", "partial", "reservation_update":
		default:
			return ErrAccountQuotaMutationInvalidInput
		}
	} else if reason != "" {
		return ErrAccountQuotaMutationInvalidInput
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row LegacyUsageReservation
		if err := lockForUpdate(tx).Where("request_id = ?", requestID).First(&row).Error; err != nil {
			return err
		}
		if row.RequestID != requestID {
			return ErrAccountQuotaMutationConflict
		}
		if row.State == LegacyUsageUnknown {
			if state == LegacyUsageUnknown {
				return nil
			}
			return ErrAccountQuotaUsageUnresolved
		}
		if row.State != LegacyUsagePrepared {
			if row.State == state && ((actual == nil && row.ActualQuota == nil) || (actual != nil && row.ActualQuota != nil && *actual == *row.ActualQuota)) {
				return nil
			}
			return ErrAccountQuotaMutationConflict
		}
		if reserved < row.ReservedQuota || tokenReserved < row.TokenReservedQuota {
			return ErrAccountQuotaMutationConflict
		}
		now, err := taskRecoveryDBTimestamp(tx)
		if err != nil {
			return err
		}
		updates := map[string]interface{}{"reserved_quota": reserved, "token_reserved_quota": tokenReserved, "state": state, "reason": reason, "actual_quota": actual, "updated_at": now, "lock_version": row.LockVersion + 1}
		if len(metadata) == 1 {
			updates["review_metadata"] = metadata[0]
		}
		result := tx.Model(&LegacyUsageReservation{}).Where("id = ? AND lock_version = ? AND state = ?", row.ID, row.LockVersion, LegacyUsagePrepared).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrAccountQuotaMutationCASLost
		}
		return nil
	})
}

func LegacyUsageNeedsReview(ctx context.Context, db *gorm.DB, tokenID int) (bool, error) {
	if db == nil {
		return false, gorm.ErrInvalidDB
	}
	if tokenID <= 0 {
		return false, ErrAccountQuotaMutationInvalidInput
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var row LegacyUsageReservation
	err := db.WithContext(ctx).Where("token_id = ? AND state = ?", tokenID, LegacyUsageUnknown).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	return err == nil, err
}

func validateLegacyUsageSettlement(tx *gorm.DB, input AccountQuotaSettlementFactInput) error {
	if input.Kind == AccountQuotaSettlementKindAuthoritative {
		return nil
	}
	var row LegacyUsageReservation
	err := lockForUpdate(tx).Where("request_id = ?", input.RequestID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	} // Pre-upgrade request.
	if err != nil {
		return err
	}
	if row.RequestID != input.RequestID || row.UserID != input.UserID || row.TokenID != input.TokenID || row.SubscriptionID != input.SubscriptionID {
		return ErrAccountQuotaMutationConflict
	}
	switch row.State {
	case LegacyUsagePrepared:
		return nil
	case LegacyUsageReviewPending, LegacyUsageSettled:
		if row.ActualQuota != nil && input.Delta == *row.ActualQuota-row.ReservedQuota {
			return nil
		}
		return ErrAccountQuotaMutationConflict
	default:
		return ErrAccountQuotaUsageUnresolved
	}
}

func validateLegacyUsageRefund(tx *gorm.DB, requestID string) error {
	var row LegacyUsageReservation
	err := lockForUpdate(tx).Where("request_id = ?", requestID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if row.RequestID != requestID {
		return ErrAccountQuotaMutationConflict
	}
	if row.State != LegacyUsagePrepared && row.State != LegacyUsageRefunded && row.State != LegacyUsagePreparing && row.State != LegacyUsageAdmissionFailed {
		return ErrAccountQuotaUsageUnresolved
	}
	return nil
}

// ResolveLegacyUnknownUsage persists the operator's exact decision before
// invoking the existing settlement-intent recovery path. A restart reuses that
// intent and never invents a new delta. It does not switch the quota writer.
func ResolveLegacyUnknownUsage(ctx context.Context, db *gorm.DB, actorID int, requestID string, actual int64, evidenceDigest string) (*LegacyUsageReservation, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	digest, err := hex.DecodeString(evidenceDigest)
	if actorID <= 0 || err != nil || len(digest) != 32 || strings.ToLower(evidenceDigest) != evidenceDigest || validateAccountQuotaValue(actual) != nil {
		return nil, ErrAccountQuotaMutationInvalidInput
	}
	var record LegacyUsageReservation
	var intent *AccountQuotaSettlementIntent
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := authorizeUsageReviewer(tx, actorID); err != nil {
			return err
		}
		if err := lockForUpdate(tx).Where("request_id = ?", requestID).First(&record).Error; err != nil {
			return err
		}
		if record.RequestID != requestID {
			return ErrAccountQuotaMutationConflict
		}
		if err := validateReviewedQuotaObligations(record.ReviewMetadata, actual); err != nil {
			return err
		}
		if record.State == LegacyUsageSettled || record.State == LegacyUsageReviewPending {
			if record.ActualQuota == nil || *record.ActualQuota != actual || record.ReviewedBy != actorID || record.EvidenceDigest != evidenceDigest {
				return ErrAccountQuotaMutationConflict
			}
			if record.State == LegacyUsageSettled {
				return nil
			}
		} else {
			if record.State != LegacyUsageUnknown {
				return ErrAccountQuotaUsageUnresolved
			}
			now, err := taskRecoveryDBTimestamp(tx)
			if err != nil {
				return err
			}
			result := tx.Model(&LegacyUsageReservation{}).Where("id = ? AND state = ? AND lock_version = ?", record.ID, LegacyUsageUnknown, record.LockVersion).
				Updates(map[string]interface{}{"state": LegacyUsageReviewPending, "actual_quota": actual, "reviewed_by": actorID, "evidence_digest": evidenceDigest, "lock_version": record.LockVersion + 1, "updated_at": now})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return ErrAccountQuotaMutationCASLost
			}
			record.State, record.ActualQuota, record.ReviewedBy, record.EvidenceDigest = LegacyUsageReviewPending, &actual, actorID, evidenceDigest
		}
		if actual == record.ReservedQuota {
			return tx.Model(&LegacyUsageReservation{}).Where("id = ? AND state = ?", record.ID, LegacyUsageReviewPending).Update("state", LegacyUsageSettled).Error
		}
		kind := AccountQuotaSettlementKindLegacyWallet
		if record.FundingSource == "subscription" {
			kind = AccountQuotaSettlementKindLegacySubscription
		}
		if record.FundingSource == "free" {
			return ErrAccountQuotaMutationIneligible
		}
		intent, err = EnsureAccountQuotaSettlementIntent(ctx, tx, AccountQuotaSettlementFactInput{
			EventKey: "billing-settlement:" + requestID + ":v1", RequestID: requestID, Kind: kind, UserID: record.UserID, TokenID: record.TokenID,
			SubscriptionID: record.SubscriptionID, Delta: actual - record.ReservedQuota, ApplyToken: record.TokenID > 0,
		})
		return err
	})
	if err != nil {
		return nil, err
	}
	if intent != nil {
		stored, _, err := RecoverAccountQuotaSettlementIntent(ctx, db, intent, "usage-review:"+requestID)
		if err != nil {
			return &record, err
		}
		if stored == nil || stored.State != AccountQuotaSettlementApplied {
			return &record, ErrAccountQuotaSettlementPending
		}
		if err := db.WithContext(ctx).Model(&LegacyUsageReservation{}).Where("id = ? AND state = ? AND evidence_digest = ?", record.ID, LegacyUsageReviewPending, evidenceDigest).Update("state", LegacyUsageSettled).Error; err != nil {
			return &record, err
		}
	}
	return FindLegacyUsageReservation(ctx, db, requestID)
}
