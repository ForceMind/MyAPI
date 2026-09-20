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
	AccountQuotaRefundFactKindAuthoritative      = "authoritative"
	AccountQuotaRefundFactKindLegacyWallet       = "legacy_wallet"
	AccountQuotaRefundFactKindLegacySubscription = "legacy_subscription"

	AccountQuotaRefundFactPending   = "pending"
	AccountQuotaRefundFactClaimed   = "claimed"
	AccountQuotaRefundFactRetryable = "retryable"
	AccountQuotaRefundFactApplied   = "applied"
	AccountQuotaRefundFactManual    = "manual"

	accountQuotaRefundFactLeaseSeconds = 30
	quotaWorkCursorRefundFact          = "account_refund_fact_v1"
)

var (
	ErrAccountQuotaRefundPending        = errors.New("account quota refund is durably pending")
	ErrAccountQuotaRefundFactUnknown    = errors.New("account quota refund fact outcome is unknown")
	ErrAccountQuotaRefundManualRequired = errors.New("account quota refund requires manual verification")

	accountQuotaRefundFactAfterCreateHook   func(string) error
	accountQuotaRefundFactReadbackHook      func() error
	accountQuotaLegacyRefundAfterCommitHook func(string) error
)

type AccountQuotaRefundFact struct {
	ID                 int64  `json:"id" gorm:"primaryKey"`
	EventKey           string `json:"event_key" gorm:"type:varchar(128);not null;uniqueIndex"`
	Kind               string `json:"kind" gorm:"type:varchar(32);not null;index"`
	RequestID          string `json:"request_id" gorm:"type:varchar(64);not null;index"`
	ReserveReceiptID   int64  `json:"reserve_receipt_id" gorm:"type:bigint;not null;default:0;index"`
	AuditKey           string `json:"audit_key" gorm:"type:varchar(96);not null;default:''"`
	WriterEpoch        int64  `json:"writer_epoch" gorm:"type:bigint;not null"`
	UserID             int    `json:"user_id" gorm:"not null;index"`
	TokenID            int    `json:"token_id" gorm:"not null;index"`
	SubscriptionID     int    `json:"subscription_id" gorm:"not null;default:0;index"`
	WalletQuota        int64  `json:"wallet_quota" gorm:"type:bigint;not null"`
	SubscriptionQuota  int64  `json:"subscription_quota" gorm:"type:bigint;not null;default:0"`
	TokenQuota         int64  `json:"token_quota" gorm:"type:bigint;not null"`
	RequestFingerprint string `json:"request_fingerprint" gorm:"type:char(64);not null"`
	State              string `json:"state" gorm:"type:varchar(16);not null;index"`
	TerminalReceiptID  int64  `json:"terminal_receipt_id" gorm:"type:bigint;not null;default:0"`
	Attempts           int    `json:"attempts" gorm:"not null;default:0"`
	LastError          string `json:"last_error" gorm:"type:text;not null"`
	NextAttemptAt      int64  `json:"next_attempt_at" gorm:"type:bigint;not null;default:0;index"`
	LeaseOwner         string `json:"lease_owner" gorm:"type:varchar(128);not null;default:''"`
	LeaseUntil         int64  `json:"lease_until" gorm:"type:bigint;not null;default:0;index"`
	LockVersion        int64  `json:"lock_version" gorm:"type:bigint;not null;default:1"`
	CreatedAt          int64  `json:"created_at" gorm:"type:bigint;not null"`
	UpdatedAt          int64  `json:"updated_at" gorm:"type:bigint;not null"`
}

func (AccountQuotaRefundFact) TableName() string { return "account_quota_refund_facts" }

type AccountQuotaRefundFactInput struct {
	EventKey          string
	Kind              string
	RequestID         string
	ReserveReceiptID  int64
	AuditKey          string
	WriterEpoch       int64
	UserID            int
	TokenID           int
	SubscriptionID    int
	WalletQuota       int64
	SubscriptionQuota int64
	TokenQuota        int64
}

func accountQuotaRefundFactFingerprint(input AccountQuotaRefundFactInput) (string, error) {
	data, err := common.Marshal(struct {
		Version           int    `json:"version"`
		EventKey          string `json:"event_key"`
		Kind              string `json:"kind"`
		RequestID         string `json:"request_id"`
		ReserveReceiptID  int64  `json:"reserve_receipt_id"`
		AuditKey          string `json:"audit_key"`
		WriterEpoch       int64  `json:"writer_epoch"`
		UserID            int    `json:"user_id"`
		TokenID           int    `json:"token_id"`
		SubscriptionID    int    `json:"subscription_id"`
		WalletQuota       int64  `json:"wallet_quota"`
		SubscriptionQuota int64  `json:"subscription_quota"`
		TokenQuota        int64  `json:"token_quota"`
	}{2, input.EventKey, input.Kind, input.RequestID, input.ReserveReceiptID, input.AuditKey, input.WriterEpoch, input.UserID, input.TokenID, input.SubscriptionID, input.WalletQuota, input.SubscriptionQuota, input.TokenQuota})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

func normalizeAccountQuotaRefundFactInput(input AccountQuotaRefundFactInput) (AccountQuotaRefundFactInput, string, error) {
	input.EventKey = strings.TrimSpace(input.EventKey)
	input.RequestID = strings.TrimSpace(input.RequestID)
	input.AuditKey = strings.TrimSpace(input.AuditKey)
	if input.EventKey == "" || len(input.EventKey) > 128 || input.RequestID == "" || len(input.RequestID) > 64 || len(input.AuditKey) > 96 ||
		input.UserID <= 0 || input.TokenID <= 0 || input.WalletQuota < 0 || input.SubscriptionQuota < 0 || input.TokenQuota < 0 ||
		input.WalletQuota > int64(common.MaxQuota) || input.SubscriptionQuota > int64(common.MaxQuota) || input.TokenQuota > int64(common.MaxQuota) {
		return input, "", ErrAccountQuotaMutationInvalidInput
	}
	switch input.Kind {
	case AccountQuotaRefundFactKindAuthoritative:
		if input.ReserveReceiptID <= 0 || input.WriterEpoch <= 0 || input.AuditKey == "" || input.SubscriptionID != 0 || input.WalletQuota != 0 || input.SubscriptionQuota != 0 || input.TokenQuota != 0 {
			return input, "", ErrAccountQuotaMutationInvalidInput
		}
	case AccountQuotaRefundFactKindLegacyWallet:
		if input.SubscriptionID != 0 || input.SubscriptionQuota != 0 || input.WalletQuota == 0 && input.TokenQuota == 0 {
			return input, "", ErrAccountQuotaMutationInvalidInput
		}
	case AccountQuotaRefundFactKindLegacySubscription:
		if input.SubscriptionID <= 0 || input.SubscriptionQuota <= 0 || input.WalletQuota != 0 {
			return input, "", ErrAccountQuotaMutationInvalidInput
		}
	default:
		return input, "", ErrAccountQuotaMutationInvalidInput
	}
	fingerprint, err := accountQuotaRefundFactFingerprint(input)
	return input, fingerprint, err
}

func validateAccountQuotaRefundFactIdentity(stored *AccountQuotaRefundFact, input AccountQuotaRefundFactInput, fingerprint string) error {
	if stored == nil || stored.EventKey != input.EventKey || stored.Kind != input.Kind || stored.RequestID != input.RequestID ||
		stored.ReserveReceiptID != input.ReserveReceiptID || stored.AuditKey != input.AuditKey || stored.WriterEpoch != input.WriterEpoch ||
		stored.UserID != input.UserID || stored.TokenID != input.TokenID || stored.SubscriptionID != input.SubscriptionID ||
		stored.WalletQuota != input.WalletQuota || stored.SubscriptionQuota != input.SubscriptionQuota || stored.TokenQuota != input.TokenQuota ||
		stored.RequestFingerprint != fingerprint {
		return ErrAccountQuotaTerminalRecoveryConflict
	}
	return nil
}

func readAccountQuotaRefundFactDetached(ctx context.Context, db *gorm.DB, input AccountQuotaRefundFactInput, fingerprint string) (*AccountQuotaRefundFact, error) {
	baseCtx := context.Background()
	if ctx != nil {
		baseCtx = context.WithoutCancel(ctx)
	}
	readCtx, cancel := context.WithTimeout(baseCtx, 2*time.Second)
	defer cancel()
	if accountQuotaRefundFactReadbackHook != nil {
		if err := accountQuotaRefundFactReadbackHook(); err != nil {
			return nil, err
		}
	}
	var stored AccountQuotaRefundFact
	if err := db.WithContext(readCtx).Where("event_key = ?", input.EventKey).First(&stored).Error; err != nil {
		return nil, err
	}
	if err := validateAccountQuotaRefundFactIdentity(&stored, input, fingerprint); err != nil {
		return nil, err
	}
	return &stored, nil
}

func EnsureAccountQuotaRefundFact(ctx context.Context, db *gorm.DB, input AccountQuotaRefundFactInput) (*AccountQuotaRefundFact, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	normalized, fingerprint, err := normalizeAccountQuotaRefundFactInput(input)
	if err != nil {
		return nil, err
	}
	var existing AccountQuotaRefundFact
	result := db.WithContext(ctx).Where("event_key = ?", normalized.EventKey).Limit(1).Find(&existing)
	if result.Error != nil {
		return nil, errors.Join(result.Error, ErrAccountQuotaRefundFactUnknown, ErrAccountQuotaRefundManualRequired)
	}
	if result.RowsAffected > 0 {
		if err := validateAccountQuotaRefundFactIdentity(&existing, normalized, fingerprint); err != nil {
			return nil, err
		}
		return &existing, nil
	}
	now, err := taskRecoveryDBTimestamp(db.WithContext(ctx))
	if err != nil {
		return nil, errors.Join(err, ErrAccountQuotaRefundFactUnknown, ErrAccountQuotaRefundManualRequired)
	}
	candidate := &AccountQuotaRefundFact{
		EventKey: normalized.EventKey, Kind: normalized.Kind, RequestID: normalized.RequestID,
		ReserveReceiptID: normalized.ReserveReceiptID, AuditKey: normalized.AuditKey, WriterEpoch: normalized.WriterEpoch,
		UserID: normalized.UserID, TokenID: normalized.TokenID, SubscriptionID: normalized.SubscriptionID,
		WalletQuota: normalized.WalletQuota, SubscriptionQuota: normalized.SubscriptionQuota, TokenQuota: normalized.TokenQuota,
		RequestFingerprint: fingerprint, State: AccountQuotaRefundFactPending, LockVersion: 1, CreatedAt: now, UpdatedAt: now,
	}
	createErr := db.WithContext(ctx).Create(candidate).Error
	if createErr == nil && accountQuotaRefundFactAfterCreateHook != nil {
		createErr = accountQuotaRefundFactAfterCreateHook(normalized.EventKey)
	}
	if createErr == nil {
		return candidate, nil
	}
	stored, readErr := readAccountQuotaRefundFactDetached(ctx, db, normalized, fingerprint)
	if readErr == nil {
		return stored, nil
	}
	return nil, errors.Join(createErr, fmt.Errorf("%w: event_key=%s: %v", ErrAccountQuotaRefundFactUnknown, normalized.EventKey, readErr), ErrAccountQuotaRefundManualRequired)
}

func claimAccountQuotaRefundFact(ctx context.Context, db *gorm.DB, fact *AccountQuotaRefundFact, workerID string, now int64) (*AccountQuotaRefundFact, bool, error) {
	forceRetry := strings.HasPrefix(workerID, "billing-session:")
	claimable := fact.State == AccountQuotaRefundFactPending ||
		(fact.State == AccountQuotaRefundFactRetryable && (fact.NextAttemptAt <= now || forceRetry)) ||
		(fact.State == AccountQuotaRefundFactClaimed && fact.LeaseUntil <= now)
	if !claimable {
		return fact, false, nil
	}
	result := db.WithContext(ctx).Model(&AccountQuotaRefundFact{}).
		Where("id = ? AND state = ? AND lock_version = ? AND lease_until = ? AND next_attempt_at = ?", fact.ID, fact.State, fact.LockVersion, fact.LeaseUntil, fact.NextAttemptAt).
		Updates(map[string]interface{}{
			"state": AccountQuotaRefundFactClaimed, "lease_owner": workerID, "lease_until": now + accountQuotaRefundFactLeaseSeconds,
			"next_attempt_at": int64(0), "attempts": fact.Attempts + 1, "lock_version": fact.LockVersion + 1, "updated_at": now,
		})
	if result.Error != nil || result.RowsAffected != 1 {
		return fact, false, result.Error
	}
	var claimed AccountQuotaRefundFact
	if err := db.WithContext(ctx).First(&claimed, fact.ID).Error; err != nil {
		return nil, false, err
	}
	return &claimed, true, nil
}

func finishAccountQuotaRefundFact(ctx context.Context, db *gorm.DB, claimed *AccountQuotaRefundFact, state string, terminalReceiptID int64, runErr error) error {
	now, err := taskRecoveryDBTimestamp(db.WithContext(ctx))
	if err != nil {
		return err
	}
	message := ""
	if runErr != nil {
		message = runErr.Error()
		if len(message) > 4096 {
			message = message[:4096]
		}
	}
	updates := map[string]interface{}{
		"state": state, "terminal_receipt_id": terminalReceiptID, "last_error": message,
		"lease_owner": "", "lease_until": int64(0), "lock_version": claimed.LockVersion + 1, "updated_at": now,
	}
	if state == AccountQuotaRefundFactRetryable {
		updates["next_attempt_at"] = now
	} else {
		updates["next_attempt_at"] = int64(0)
	}
	result := db.WithContext(ctx).Model(&AccountQuotaRefundFact{}).
		Where("id = ? AND state = ? AND lease_owner = ? AND lock_version = ?", claimed.ID, AccountQuotaRefundFactClaimed, claimed.LeaseOwner, claimed.LockVersion).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		var stored AccountQuotaRefundFact
		if err := db.WithContext(ctx).First(&stored, claimed.ID).Error; err == nil && stored.State == AccountQuotaRefundFactApplied {
			return nil
		}
		return ErrAccountQuotaTerminalRecoveryConflict
	}
	return nil
}

func prepareLegacyRefundUser(ctx context.Context, db *gorm.DB, fact *AccountQuotaRefundFact) (*quotaBalanceSubjectLock, error) {
	if fact.WalletQuota <= 0 || !common.RedisEnabled || common.RDB == nil {
		return nil, nil
	}
	lock, err := acquireQuotaBalanceSubjectLockContext(ctx, BatchUpdateTypeUserQuota, fact.UserID)
	if err != nil {
		return nil, err
	}
	if err := recoverQuotaBalanceSubjectPreparationsWithDB(ctx, db, BatchUpdateTypeUserQuota, fact.UserID, 0, lock); err != nil {
		releaseQuotaBalanceSubjectLock(lock)
		return nil, err
	}
	if err := drainQuotaBalanceGenerationsForSubjectWithDB(db.WithContext(ctx), BatchUpdateTypeUserQuota, fact.UserID); err != nil {
		releaseQuotaBalanceSubjectLock(lock)
		return nil, err
	}
	if err := invalidateQuotaBalanceSubjectCache(lock, getUserCacheKey(fact.UserID), ""); err != nil {
		releaseQuotaBalanceSubjectLock(lock)
		return nil, err
	}
	return lock, nil
}

func prepareLegacyRefundToken(ctx context.Context, db *gorm.DB, fact *AccountQuotaRefundFact) (*quotaBalanceSubjectLock, string, error) {
	if fact.TokenQuota <= 0 || !common.RedisEnabled || common.RDB == nil {
		return nil, "", nil
	}
	lock, err := acquireQuotaBalanceSubjectLockContext(ctx, BatchUpdateTypeTokenQuota, fact.TokenID)
	if err != nil {
		return nil, "", err
	}
	if err := recoverQuotaBalanceSubjectPreparationsWithDB(ctx, db, BatchUpdateTypeTokenQuota, fact.TokenID, 0, lock); err != nil {
		releaseQuotaBalanceSubjectLock(lock)
		return nil, "", err
	}
	if err := drainQuotaBalanceGenerationsForSubjectWithDB(db.WithContext(ctx), BatchUpdateTypeTokenQuota, fact.TokenID); err != nil {
		releaseQuotaBalanceSubjectLock(lock)
		return nil, "", err
	}
	var token Token
	if err := db.WithContext(ctx).Select("id", "key").Where("id = ?", fact.TokenID).First(&token).Error; err != nil {
		releaseQuotaBalanceSubjectLock(lock)
		return nil, "", err
	}
	if err := invalidateQuotaBalanceSubjectCache(lock, getTokenCacheKey(token.Key), getTokenCacheFenceKey(token.Key)); err != nil {
		releaseQuotaBalanceSubjectLock(lock)
		return nil, "", err
	}
	return lock, token.Key, nil
}

func applyLegacyRefundTokenTx(tx *gorm.DB, fact *AccountQuotaRefundFact, now int64) error {
	if fact.TokenQuota <= 0 {
		return nil
	}
	var token Token
	if err := lockForUpdate(tx).Where("id = ?", fact.TokenID).First(&token).Error; err != nil {
		return err
	}
	remain, err := checkedLegacyBalanceTarget(token.RemainQuota, int(fact.TokenQuota))
	if err != nil {
		return err
	}
	used, err := checkedLegacyBalanceTarget(token.UsedQuota, -int(fact.TokenQuota))
	if err != nil || used < 0 {
		return ErrAccountQuotaMutationIneligible
	}
	result := tx.Table("tokens").Where("id = ? AND remain_quota = ? AND used_quota = ? AND quota_version = ?", token.Id, token.RemainQuota, token.UsedQuota, token.QuotaVersion).
		Updates(map[string]interface{}{"remain_quota": remain, "used_quota": used, "quota_version": token.QuotaVersion + 1, "accessed_time": now})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrAccountQuotaMutationCASLost
	}
	return nil
}

func markLegacyRefundFactAppliedTx(tx *gorm.DB, fact *AccountQuotaRefundFact, now int64) error {
	result := tx.Model(&AccountQuotaRefundFact{}).
		Where("id = ? AND state = ? AND lease_owner = ? AND lock_version = ?", fact.ID, AccountQuotaRefundFactClaimed, fact.LeaseOwner, fact.LockVersion).
		Updates(map[string]interface{}{
			"state": AccountQuotaRefundFactApplied, "lease_owner": "", "lease_until": int64(0), "last_error": "",
			"next_attempt_at": int64(0), "lock_version": fact.LockVersion + 1, "updated_at": now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrAccountQuotaMutationCASLost
	}
	return nil
}

func refreshLegacyRefundCaches(ctx context.Context, db *gorm.DB, fact *AccountQuotaRefundFact, userLock, tokenLock *quotaBalanceSubjectLock) {
	if userLock != nil {
		var user User
		if err := db.WithContext(ctx).Where("id = ?", fact.UserID).First(&user).Error; err == nil {
			_ = writeUserCacheWithQuotaBalanceOwner(user.ToBaseUser(), true, userLock)
		}
	}
	if tokenLock != nil {
		var token Token
		if err := db.WithContext(ctx).Where("id = ?", fact.TokenID).First(&token).Error; err == nil {
			_, _ = cacheInitTokenWithQuotaBalanceOwner(token, tokenLock)
		}
	}
}

func applyLegacyWalletRefundFact(ctx context.Context, db *gorm.DB, claimed *AccountQuotaRefundFact) error {
	userLock, err := prepareLegacyRefundUser(ctx, db, claimed)
	if err != nil {
		return err
	}
	defer releaseQuotaBalanceSubjectLock(userLock)
	tokenLock, _, err := prepareLegacyRefundToken(ctx, db, claimed)
	if err != nil {
		return err
	}
	defer releaseQuotaBalanceSubjectLock(tokenLock)
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var fact AccountQuotaRefundFact
		if err := lockForUpdate(tx).Where("id = ?", claimed.ID).First(&fact).Error; err != nil {
			return err
		}
		if fact.State == AccountQuotaRefundFactApplied {
			return nil
		}
		if fact.State != AccountQuotaRefundFactClaimed || fact.LeaseOwner != claimed.LeaseOwner || fact.LockVersion != claimed.LockVersion {
			return ErrAccountQuotaTerminalRecoveryConflict
		}
		if fact.WalletQuota > 0 {
			var user User
			if err := lockForUpdate(tx).Where("id = ?", fact.UserID).First(&user).Error; err != nil {
				return err
			}
			userQuota, err := checkedLegacyBalanceTarget(user.Quota, int(fact.WalletQuota))
			if err != nil {
				return err
			}
			result := tx.Table("users").Where("id = ? AND quota = ? AND quota_version = ?", user.Id, user.Quota, user.QuotaVersion).
				Updates(map[string]interface{}{"quota": userQuota, "quota_version": user.QuotaVersion + 1})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return ErrAccountQuotaMutationCASLost
			}
		}
		now, err := taskRecoveryDBTimestamp(tx)
		if err != nil {
			return err
		}
		if err := applyLegacyRefundTokenTx(tx, &fact, now); err != nil {
			return err
		}
		return markLegacyRefundFactAppliedTx(tx, &fact, now)
	})
	if err == nil && accountQuotaLegacyRefundAfterCommitHook != nil {
		err = accountQuotaLegacyRefundAfterCommitHook(claimed.EventKey)
	}
	if err != nil {
		var stored AccountQuotaRefundFact
		readCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		readErr := db.WithContext(readCtx).First(&stored, claimed.ID).Error
		cancel()
		if readErr == nil && stored.State == AccountQuotaRefundFactApplied {
			err = nil
		} else {
			return errors.Join(err, readErr)
		}
	}
	refreshLegacyRefundCaches(ctx, db, claimed, userLock, tokenLock)
	return nil
}

func applyLegacySubscriptionRefundFact(ctx context.Context, db *gorm.DB, claimed *AccountQuotaRefundFact) error {
	tokenLock, _, err := prepareLegacyRefundToken(ctx, db, claimed)
	if err != nil {
		return err
	}
	defer releaseQuotaBalanceSubjectLock(tokenLock)
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var fact AccountQuotaRefundFact
		if err := lockForUpdate(tx).Where("id = ?", claimed.ID).First(&fact).Error; err != nil {
			return err
		}
		if fact.State == AccountQuotaRefundFactApplied {
			return nil
		}
		if fact.State != AccountQuotaRefundFactClaimed || fact.LeaseOwner != claimed.LeaseOwner || fact.LockVersion != claimed.LockVersion {
			return ErrAccountQuotaTerminalRecoveryConflict
		}
		var record SubscriptionPreConsumeRecord
		if err := lockForUpdate(tx).Where("request_id = ?", fact.RequestID).First(&record).Error; err != nil {
			return err
		}
		if record.UserId != fact.UserID || record.UserSubscriptionId != fact.SubscriptionID || record.PreConsumed != fact.SubscriptionQuota {
			return ErrAccountQuotaTerminalRecoveryConflict
		}
		switch record.Status {
		case "consumed":
			var subscription UserSubscription
			if err := lockForUpdate(tx).Where("id = ?", fact.SubscriptionID).First(&subscription).Error; err != nil {
				return err
			}
			if subscription.UserId != fact.UserID || subscription.AmountUsed < fact.SubscriptionQuota {
				return ErrAccountQuotaMutationIneligible
			}
			result := tx.Model(&UserSubscription{}).
				Where("id = ? AND amount_used = ? AND quota_version = ?", subscription.Id, subscription.AmountUsed, subscription.QuotaVersion).
				Updates(map[string]interface{}{"amount_used": subscription.AmountUsed - fact.SubscriptionQuota, "quota_version": subscription.QuotaVersion + 1})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return ErrAccountQuotaMutationCASLost
			}
			result = tx.Model(&SubscriptionPreConsumeRecord{}).Where("id = ? AND status = ?", record.Id, "consumed").Update("status", "refunded")
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return ErrAccountQuotaMutationCASLost
			}
		case "refunded":
			// Without an applied fact, a pre-existing subscription refund cannot
			// prove whether the token side was also refunded. Fail to manual review.
			return ErrAccountQuotaTerminalRecoveryConflict
		default:
			return ErrAccountQuotaTerminalRecoveryConflict
		}
		now, err := taskRecoveryDBTimestamp(tx)
		if err != nil {
			return err
		}
		if err := applyLegacyRefundTokenTx(tx, &fact, now); err != nil {
			return err
		}
		return markLegacyRefundFactAppliedTx(tx, &fact, now)
	})
	if err == nil && accountQuotaLegacyRefundAfterCommitHook != nil {
		err = accountQuotaLegacyRefundAfterCommitHook(claimed.EventKey)
	}
	if err != nil {
		var stored AccountQuotaRefundFact
		readCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		readErr := db.WithContext(readCtx).First(&stored, claimed.ID).Error
		cancel()
		if readErr == nil && stored.State == AccountQuotaRefundFactApplied {
			err = nil
		} else {
			return errors.Join(err, readErr)
		}
	}
	refreshLegacyRefundCaches(ctx, db, claimed, nil, tokenLock)
	return nil
}

func nowUnix() int64 { return time.Now().Unix() }

func RecoverAccountQuotaRefundFact(ctx context.Context, db *gorm.DB, fact *AccountQuotaRefundFact, workerID string) (*AccountQuotaRefundFact, *AccountQuotaMutationReceipt, error) {
	if db == nil || fact == nil || strings.TrimSpace(workerID) == "" {
		return nil, nil, gorm.ErrInvalidDB
	}
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
	}
	now, err := taskRecoveryDBTimestamp(db.WithContext(ctx))
	if err != nil {
		return nil, nil, err
	}
	claimed, won, err := claimAccountQuotaRefundFact(ctx, db, fact, workerID, now)
	if err != nil || !won {
		return claimed, nil, err
	}
	var terminal *AccountQuotaMutationReceipt
	switch claimed.Kind {
	case AccountQuotaRefundFactKindAuthoritative:
		input := AccountQuotaTerminalInput{RequestID: claimed.RequestID, ReserveReceiptID: claimed.ReserveReceiptID, AuditKey: claimed.AuditKey}
		if _, recovered, ensureErr := EnsureAccountQuotaRefundRecovery(ctx, db, input, nil); ensureErr != nil {
			err = ensureErr
		} else if recovered != nil {
			terminal = recovered
		} else {
			terminal, err = RefundAccountQuota(ctx, db, input)
		}
	case AccountQuotaRefundFactKindLegacyWallet:
		err = applyLegacyWalletRefundFact(ctx, db, claimed)
	case AccountQuotaRefundFactKindLegacySubscription:
		err = applyLegacySubscriptionRefundFact(ctx, db, claimed)
	default:
		err = ErrAccountQuotaMutationInvalidInput
	}
	legacyFact := claimed.Kind == AccountQuotaRefundFactKindLegacyWallet || claimed.Kind == AccountQuotaRefundFactKindLegacySubscription
	if legacyFact && err == nil {
		var stored AccountQuotaRefundFact
		if readErr := db.WithContext(ctx).First(&stored, claimed.ID).Error; readErr != nil {
			return claimed, nil, readErr
		}
		return &stored, nil, nil
	}
	if err == nil {
		terminalID := int64(0)
		if terminal != nil {
			terminalID = terminal.ID
		}
		if finishErr := finishAccountQuotaRefundFact(ctx, db, claimed, AccountQuotaRefundFactApplied, terminalID, nil); finishErr != nil {
			return claimed, terminal, finishErr
		}
	} else {
		state := AccountQuotaRefundFactRetryable
		resultErr := ErrAccountQuotaRefundPending
		if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, ErrAccountQuotaMutationInvalidInput) ||
			errors.Is(err, ErrAccountQuotaMutationIneligible) || errors.Is(err, ErrAccountQuotaTerminalRecoveryConflict) {
			state = AccountQuotaRefundFactManual
			resultErr = ErrAccountQuotaRefundManualRequired
		}
		if finishErr := finishAccountQuotaRefundFact(ctx, db, claimed, state, 0, err); finishErr != nil {
			return claimed, nil, errors.Join(err, finishErr)
		}
		return claimed, nil, errors.Join(resultErr, err)
	}
	var stored AccountQuotaRefundFact
	if readErr := db.WithContext(ctx).First(&stored, claimed.ID).Error; readErr != nil {
		return claimed, terminal, readErr
	}
	return &stored, terminal, nil
}

func RunAccountQuotaRefundFacts(ctx context.Context, db *gorm.DB, workerID string, limit int) (int, error) {
	if db == nil || strings.TrimSpace(workerID) == "" {
		return 0, gorm.ErrInvalidDB
	}
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
	}
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	db = db.WithContext(ctx)
	cursor, err := loadQuotaWorkCursor(ctx, db, quotaWorkCursorRefundFact)
	if err != nil {
		return 0, err
	}
	if err := beginQuotaWorkCycle(ctx, db, cursor, &AccountQuotaRefundFact{}); err != nil {
		return 0, err
	}
	if cursor.HighWatermark == 0 {
		return 0, nil
	}
	now, err := taskRecoveryDBTimestamp(db)
	if err != nil {
		return 0, err
	}
	processed := 0
	scanned := 0
	var runErrors []error
	for scanned < limit {
		var facts []AccountQuotaRefundFact
		if err := db.Where("id > ? AND id <= ? AND (state = ? OR (state = ? AND next_attempt_at <= ?) OR (state = ? AND lease_until <= ?))",
			cursor.LastID, cursor.HighWatermark, AccountQuotaRefundFactPending, AccountQuotaRefundFactRetryable, now, AccountQuotaRefundFactClaimed, now).
			Order("id ASC").Limit(min(25, limit-scanned)).Find(&facts).Error; err != nil {
			return processed, errors.Join(append(runErrors, err)...)
		}
		if len(facts) == 0 {
			if err := finishQuotaWorkCycle(ctx, db, cursor); err != nil {
				return processed, errors.Join(append(runErrors, err)...)
			}
			return processed, errors.Join(runErrors...)
		}
		for index := range facts {
			cursor.LastID = facts[index].ID
			scanned++
			stored, _, runErr := RecoverAccountQuotaRefundFact(ctx, db, &facts[index], fmt.Sprintf("%s:%d", workerID, facts[index].ID))
			if stored != nil && stored.State == AccountQuotaRefundFactApplied {
				processed++
			}
			if runErr != nil {
				runErrors = append(runErrors, runErr)
			}
		}
		if err := saveQuotaWorkCursor(ctx, db, cursor); err != nil {
			return processed, errors.Join(append(runErrors, err)...)
		}
	}
	return processed, errors.Join(runErrors...)
}
