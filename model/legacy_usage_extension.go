package model

import (
	"context"
	"errors"
	"math"
	"time"

	"gorm.io/gorm"
)

// ExtendLegacyUsageReservation atomically raises an existing cumulative
// reservation. Its journal is also the idempotency key and the lock shared
// with manual usage recovery. No actual consumption is asserted here.
func ExtendLegacyUsageReservation(ctx context.Context, db *gorm.DB, requestID string, userID, tokenID int, target int64, applyToken bool) (*LegacyUsageReservation, error) {
	if db == nil || userID <= 0 || tokenID < 0 || applyToken && tokenID == 0 || validateAccountQuotaValue(target) != nil {
		return nil, ErrAccountQuotaMutationInvalidInput
	}
	ctx, cancel := accountQuotaOperationContext(ctx, 5*time.Second)
	defer cancel()
	mode, err := currentQuotaWriterMode(db.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	if mode != QuotaWriterModeLegacy {
		return nil, ErrLegacyQuotaWriterModeDisabled
	}
	identity, err := FindLegacyUsageReservation(ctx, db, requestID)
	if err != nil {
		return nil, err
	}
	if identity.RequestID != requestID || identity.UserID != userID || identity.TokenID != tokenID {
		return nil, ErrAccountQuotaMutationConflict
	}
	var userLock, tokenLock *quotaBalanceSubjectLock
	if identity.FundingSource == "wallet" {
		userLock, err = prepareLegacySettlementSubject(ctx, db, BatchUpdateTypeUserQuota, userID, "")
		if err != nil {
			return nil, err
		}
		defer releaseQuotaBalanceSubjectLock(userLock)
	}
	var token Token
	if applyToken {
		if err := db.WithContext(ctx).Where("id = ? AND user_id = ?", tokenID, userID).First(&token).Error; err != nil {
			return nil, err
		}
		tokenLock, err = prepareLegacySettlementSubject(ctx, db, BatchUpdateTypeTokenQuota, tokenID, token.Key)
		if err != nil {
			return nil, err
		}
		defer releaseQuotaBalanceSubjectLock(tokenLock)
	}
	var row LegacyUsageReservation
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		mode, err := currentQuotaWriterMode(tx)
		if err != nil {
			return err
		}
		if mode != QuotaWriterModeLegacy {
			return ErrLegacyQuotaWriterModeDisabled
		}
		if err := lockForUpdate(tx).Where("request_id = ?", requestID).First(&row).Error; err != nil {
			return err
		}
		if row.RequestID != requestID || row.UserID != userID || row.TokenID != tokenID || row.FundingSource != identity.FundingSource || row.SubscriptionID != identity.SubscriptionID || row.LockVersion <= 0 {
			return ErrAccountQuotaMutationConflict
		}
		if row.State != LegacyUsagePrepared || row.ActualQuota != nil {
			return ErrAccountQuotaUsageUnresolved
		}
		if validateAccountQuotaValue(row.ReservedQuota) != nil || validateAccountQuotaValue(row.TokenReservedQuota) != nil {
			return ErrAccountQuotaMutationInvalidInput
		}
		pending, err := TextDispatchPending(row.ReviewMetadata)
		realtime, realtimeErr := RealtimeDispatchPending(row.ReviewMetadata)
		if err != nil || realtimeErr != nil || pending && !realtime {
			return ErrAccountQuotaUsageUnresolved
		}
		// A durable terminal intent owns its captured amounts even while
		// the journal still says prepared. Do not change its reservation.
		for _, entity := range []any{&AccountQuotaRefundFact{}, &AccountQuotaSettlementIntent{}, &AccountQuotaSettlementFact{}} {
			var pending struct{ ID int64 }
			result := tx.Model(entity).Select("id").Where("request_id = ?", requestID).Limit(1).Find(&pending)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected > 0 {
				return ErrAccountQuotaUsageUnresolved
			}
		}
		if target <= row.ReservedQuota {
			return nil
		}
		delta := target - row.ReservedQuota
		tokenTarget := row.TokenReservedQuota
		if applyToken {
			tokenTarget += delta
		}
		if validateAccountQuotaValue(tokenTarget) != nil {
			return ErrAccountQuotaMutationInvalidInput
		}
		now, err := taskRecoveryDBTimestamp(tx)
		if err != nil {
			return err
		}
		switch row.FundingSource {
		case "wallet":
			var user User
			if err := lockForUpdate(tx).First(&user, userID).Error; err != nil {
				return err
			}
			// Preserve the existing legacy extension contract: known work may
			// exceed wallet balance, but never integer bounds or Key limits.
			balance, err := checkedLegacyBalanceTarget(user.Quota, -int(delta))
			if err != nil {
				return err
			}
			result := tx.Model(&User{}).Where("id = ? AND quota = ? AND quota_version = ?", user.Id, user.Quota, user.QuotaVersion).
				Updates(map[string]any{"quota": balance, "quota_version": user.QuotaVersion + 1})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return ErrAccountQuotaMutationCASLost
			}
		case "subscription":
			var record SubscriptionPreConsumeRecord
			if err := lockForUpdate(tx).Where("request_id = ?", requestID).First(&record).Error; err != nil {
				return err
			}
			if record.RequestId != requestID || record.UserId != userID || record.UserSubscriptionId != row.SubscriptionID || record.Status != "consumed" || record.PreConsumed != row.ReservedQuota {
				return ErrAccountQuotaMutationConflict
			}
			var sub UserSubscription
			if err := lockForUpdate(tx).Where("id = ? AND user_id = ?", row.SubscriptionID, userID).First(&sub).Error; err != nil {
				return err
			}
			if sub.AmountUsed < 0 || sub.AmountUsed > math.MaxInt64-delta {
				return ErrAccountQuotaMutationInvalidInput
			}
			if err := postConsumeUserSubscriptionDeltaTx(tx, sub.Id, delta); err != nil {
				return err
			}
			result := tx.Model(&SubscriptionPreConsumeRecord{}).Where("id = ? AND status = ? AND pre_consumed = ?", record.Id, "consumed", record.PreConsumed).
				Updates(map[string]any{"pre_consumed": target, "updated_at": now})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return ErrAccountQuotaMutationCASLost
			}
		case BillingSourceSelfUse:
			if row.UsagePolicyRevision <= 0 {
				return ErrAccountQuotaMutationConflict
			}
		default:
			return ErrAccountQuotaMutationIneligible
		}
		if applyToken {
			if err := lockForUpdate(tx).Where("id = ? AND user_id = ?", tokenID, userID).First(&token).Error; err != nil {
				return err
			}
			remain, err := checkedLegacyBalanceTarget(token.RemainQuota, -int(delta))
			if err != nil {
				return err
			}
			if !token.UnlimitedQuota && remain < 0 {
				return ErrAccountQuotaMutationInsufficient
			}
			used, err := checkedLegacyBalanceTarget(token.UsedQuota, int(delta))
			if err != nil || used < 0 {
				return ErrAccountQuotaMutationInvalidInput
			}
			result := tx.Model(&Token{}).Where("id = ? AND remain_quota = ? AND used_quota = ? AND quota_version = ?", token.Id, token.RemainQuota, token.UsedQuota, token.QuotaVersion).
				Updates(map[string]any{"remain_quota": remain, "used_quota": used, "quota_version": token.QuotaVersion + 1, "accessed_time": now})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return ErrAccountQuotaMutationCASLost
			}
		}
		result := tx.Model(&LegacyUsageReservation{}).Where("id = ? AND state = ? AND lock_version = ?", row.ID, LegacyUsagePrepared, row.LockVersion).
			Updates(map[string]any{"reserved_quota": target, "token_reserved_quota": tokenTarget, "lock_version": row.LockVersion + 1, "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrAccountQuotaMutationCASLost
		}
		row.ReservedQuota, row.TokenReservedQuota = target, tokenTarget
		row.LockVersion++
		return nil
	})
	// A commit error is not proof of rollback. Read the same cumulative
	// identity before returning, never compensate or repeat a debit blindly.
	readCtx, readCancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer readCancel()
	if err != nil {
		stored, readErr := FindLegacyUsageReservation(readCtx, db, requestID)
		if readErr != nil {
			return nil, errors.Join(err, readErr, ErrAccountQuotaUsageUnresolved)
		}
		if stored.State != LegacyUsagePrepared {
			return stored, errors.Join(err, ErrAccountQuotaUsageUnresolved)
		}
		if stored.ReservedQuota != target || row.ReservedQuota != target || stored.TokenReservedQuota != row.TokenReservedQuota {
			return stored, err
		}
		row = *stored
	}
	if userLock != nil {
		var user User
		if db.WithContext(readCtx).First(&user, userID).Error == nil {
			_ = writeUserCacheWithQuotaBalanceOwner(user.ToBaseUser(), true, userLock)
		}
	}
	if tokenLock != nil && db.WithContext(readCtx).First(&token, tokenID).Error == nil {
		_, _ = cacheInitTokenWithQuotaBalanceOwner(token, tokenLock)
	}
	return &row, nil
}
