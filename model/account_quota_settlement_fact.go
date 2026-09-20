package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"gorm.io/gorm"
)

const (
	AccountQuotaSettlementKindAuthoritative      = "authoritative"
	AccountQuotaSettlementKindLegacyWallet       = "legacy_wallet"
	AccountQuotaSettlementKindLegacySubscription = "legacy_subscription"

	AccountQuotaSettlementPending      = "pending"
	AccountQuotaSettlementMaterialized = "materialized"
	AccountQuotaSettlementClaimed      = "claimed"
	AccountQuotaSettlementRetryable    = "retryable"
	AccountQuotaSettlementApplied      = "applied"
	AccountQuotaSettlementManual       = "manual"

	accountQuotaSettlementLeaseSeconds = 30
	quotaWorkCursorSettlementFact      = "account_settlement_fact_v1"
	quotaWorkCursorSettlementIntent    = "account_settlement_intent_v1"
)

var (
	ErrAccountQuotaSettlementPending        = errors.New("account quota settlement is durably pending")
	ErrAccountQuotaSettlementFactUnknown    = errors.New("account quota settlement fact outcome is unknown")
	ErrAccountQuotaSettlementManualRequired = errors.New("account quota settlement requires manual verification")

	accountQuotaSettlementAfterCreateHook         func(string) error
	accountQuotaSettlementReadbackHook            func() error
	accountQuotaSettlementAfterFundingCommitHook  func(string) error
	accountQuotaSettlementBeforeTokenHook         func(*AccountQuotaSettlementFact) error
	accountQuotaSettlementAfterTokenCommitHook    func(string) error
	accountQuotaSettlementAfterTerminalCommitHook func(string) error
	accountQuotaSettlementIntentAfterCreateHook   func(string) error
	accountQuotaSettlementIntentReadbackHook      func() error
	accountQuotaSettlementIntentSchemaReady       atomic.Bool
)

func RefreshAccountQuotaSettlementIntentSchemaCapability(db *gorm.DB) bool {
	ready := db != nil && db.Migrator().HasTable(&AccountQuotaSettlementIntent{})
	accountQuotaSettlementIntentSchemaReady.Store(ready)
	return ready
}

func AccountQuotaSettlementIntentSchemaReady() bool {
	return accountQuotaSettlementIntentSchemaReady.Load()
}

// AccountQuotaSettlementIntent is the independent durable inbox for one
// settlement. A singleton wake-up task never carries accounting identity; it
// only asks workers to scan these rows.
type AccountQuotaSettlementIntent struct {
	ID                 int64  `json:"id" gorm:"primaryKey"`
	EventKey           string `json:"event_key" gorm:"type:varchar(128);not null;uniqueIndex;<-:create"`
	RequestID          string `json:"request_id" gorm:"type:varchar(64);not null;index;<-:create"`
	Kind               string `json:"kind" gorm:"type:varchar(32);not null;index;<-:create"`
	UserID             int    `json:"user_id" gorm:"not null;index;<-:create"`
	TokenID            int    `json:"token_id" gorm:"not null;index;<-:create"`
	SubscriptionID     int    `json:"subscription_id" gorm:"not null;default:0;index;<-:create"`
	ReserveReceiptID   int64  `json:"reserve_receipt_id" gorm:"type:bigint;not null;default:0;index;<-:create"`
	WriterEpoch        int64  `json:"writer_epoch" gorm:"type:bigint;not null;default:0;<-:create"`
	ActualQuota        int64  `json:"actual_quota" gorm:"type:bigint;not null;default:0;<-:create"`
	Delta              int64  `json:"delta" gorm:"type:bigint;not null;<-:create"`
	ApplyToken         bool   `json:"apply_token" gorm:"not null;<-:create"`
	RequestFingerprint string `json:"request_fingerprint" gorm:"type:char(64);not null;<-:create"`
	State              string `json:"state" gorm:"type:varchar(16);not null;index"`
	FactID             int64  `json:"fact_id" gorm:"type:bigint;not null;default:0;index"`
	TerminalReceiptID  int64  `json:"terminal_receipt_id" gorm:"type:bigint;not null;default:0;index"`
	Attempts           int    `json:"attempts" gorm:"not null;default:0"`
	LastError          string `json:"last_error" gorm:"type:text;not null"`
	LockVersion        int64  `json:"lock_version" gorm:"type:bigint;not null;default:1"`
	CreatedAt          int64  `json:"created_at" gorm:"type:bigint;not null"`
	UpdatedAt          int64  `json:"updated_at" gorm:"type:bigint;not null"`
}

func (AccountQuotaSettlementIntent) TableName() string { return "account_quota_settlement_intents" }

type AccountQuotaSettlementFact struct {
	ID                 int64  `json:"id" gorm:"primaryKey"`
	EventKey           string `json:"event_key" gorm:"type:varchar(128);not null;uniqueIndex"`
	RequestID          string `json:"request_id" gorm:"type:varchar(64);not null;index"`
	Kind               string `json:"kind" gorm:"type:varchar(32);not null;index"`
	UserID             int    `json:"user_id" gorm:"not null;index"`
	TokenID            int    `json:"token_id" gorm:"not null;index"`
	SubscriptionID     int    `json:"subscription_id" gorm:"not null;default:0;index"`
	ReserveReceiptID   int64  `json:"reserve_receipt_id" gorm:"type:bigint;not null;default:0;index"`
	WriterEpoch        int64  `json:"writer_epoch" gorm:"type:bigint;not null;default:0"`
	ActualQuota        int64  `json:"actual_quota" gorm:"type:bigint;not null;default:0"`
	Delta              int64  `json:"delta" gorm:"type:bigint;not null"`
	ApplyToken         bool   `json:"apply_token" gorm:"not null"`
	FundingApplied     bool   `json:"funding_applied" gorm:"not null"`
	TokenApplied       bool   `json:"token_applied" gorm:"not null"`
	TerminalReceiptID  int64  `json:"terminal_receipt_id" gorm:"type:bigint;not null;default:0;index"`
	RequestFingerprint string `json:"request_fingerprint" gorm:"type:char(64);not null"`
	State              string `json:"state" gorm:"type:varchar(16);not null;index"`
	Attempts           int    `json:"attempts" gorm:"not null;default:0"`
	LastError          string `json:"last_error" gorm:"type:text;not null"`
	NextAttemptAt      int64  `json:"next_attempt_at" gorm:"type:bigint;not null;default:0;index"`
	LeaseOwner         string `json:"lease_owner" gorm:"type:varchar(128);not null;default:''"`
	LeaseUntil         int64  `json:"lease_until" gorm:"type:bigint;not null;default:0;index"`
	LockVersion        int64  `json:"lock_version" gorm:"type:bigint;not null;default:1"`
	CreatedAt          int64  `json:"created_at" gorm:"type:bigint;not null"`
	UpdatedAt          int64  `json:"updated_at" gorm:"type:bigint;not null"`
}

func (AccountQuotaSettlementFact) TableName() string { return "account_quota_settlement_facts" }

type AccountQuotaSettlementFactInput struct {
	EventKey         string
	RequestID        string
	Kind             string
	UserID           int
	TokenID          int
	SubscriptionID   int
	ReserveReceiptID int64
	WriterEpoch      int64
	ActualQuota      int64
	Delta            int64
	ApplyToken       bool
}

func normalizeAccountQuotaSettlementInput(input AccountQuotaSettlementFactInput) (AccountQuotaSettlementFactInput, string, error) {
	input.EventKey = strings.TrimSpace(input.EventKey)
	input.RequestID = strings.TrimSpace(input.RequestID)
	if input.EventKey == "" || len(input.EventKey) > 128 || input.RequestID == "" || len(input.RequestID) > 64 ||
		input.UserID <= 0 || (input.ApplyToken && input.TokenID <= 0) || input.TokenID < 0 || input.SubscriptionID < 0 {
		return input, "", ErrAccountQuotaMutationInvalidInput
	}
	switch input.Kind {
	case AccountQuotaSettlementKindAuthoritative:
		if input.TokenID <= 0 || input.ReserveReceiptID <= 0 || input.WriterEpoch <= 0 ||
			validateAccountQuotaValue(input.ActualQuota) != nil || input.Delta != 0 || input.ApplyToken {
			return input, "", ErrAccountQuotaMutationInvalidInput
		}
	case AccountQuotaSettlementKindLegacyWallet:
		if input.SubscriptionID != 0 || input.ReserveReceiptID != 0 || input.WriterEpoch != 0 || input.ActualQuota != 0 ||
			input.Delta == 0 || input.Delta < -int64(common.MaxQuota) || input.Delta > int64(common.MaxQuota) {
			return input, "", ErrAccountQuotaMutationInvalidInput
		}
	case AccountQuotaSettlementKindLegacySubscription:
		if input.SubscriptionID <= 0 || input.ReserveReceiptID != 0 || input.WriterEpoch != 0 || input.ActualQuota != 0 ||
			input.Delta == 0 || input.Delta < -int64(common.MaxQuota) || input.Delta > int64(common.MaxQuota) {
			return input, "", ErrAccountQuotaMutationInvalidInput
		}
	default:
		return input, "", ErrAccountQuotaMutationInvalidInput
	}
	if input.Kind == AccountQuotaSettlementKindAuthoritative {
		data, err := common.Marshal(struct {
			Version          int    `json:"version"`
			EventKey         string `json:"event_key"`
			RequestID        string `json:"request_id"`
			Kind             string `json:"kind"`
			UserID           int    `json:"user_id"`
			TokenID          int    `json:"token_id"`
			SubscriptionID   int    `json:"subscription_id"`
			ReserveReceiptID int64  `json:"reserve_receipt_id"`
			WriterEpoch      int64  `json:"writer_epoch"`
			ActualQuota      int64  `json:"actual_quota"`
		}{2, input.EventKey, input.RequestID, input.Kind, input.UserID, input.TokenID, input.SubscriptionID,
			input.ReserveReceiptID, input.WriterEpoch, input.ActualQuota})
		if err != nil {
			return input, "", err
		}
		digest := sha256.Sum256(data)
		return input, hex.EncodeToString(digest[:]), nil
	}
	data, err := common.Marshal(struct {
		Version        int    `json:"version"`
		EventKey       string `json:"event_key"`
		RequestID      string `json:"request_id"`
		Kind           string `json:"kind"`
		UserID         int    `json:"user_id"`
		TokenID        int    `json:"token_id"`
		SubscriptionID int    `json:"subscription_id"`
		Delta          int64  `json:"delta"`
		ApplyToken     bool   `json:"apply_token"`
	}{1, input.EventKey, input.RequestID, input.Kind, input.UserID, input.TokenID, input.SubscriptionID, input.Delta, input.ApplyToken})
	if err != nil {
		return input, "", err
	}
	digest := sha256.Sum256(data)
	return input, hex.EncodeToString(digest[:]), nil
}

// NormalizeAccountQuotaSettlementFactInput returns the canonical input and
// stable fingerprint without performing database I/O.
func NormalizeAccountQuotaSettlementFactInput(input AccountQuotaSettlementFactInput) (AccountQuotaSettlementFactInput, string, error) {
	return normalizeAccountQuotaSettlementInput(input)
}

func validateAccountQuotaSettlementIdentity(fact *AccountQuotaSettlementFact, input AccountQuotaSettlementFactInput, fingerprint string) error {
	if fact == nil || fact.EventKey != input.EventKey || fact.RequestID != input.RequestID || fact.Kind != input.Kind ||
		fact.UserID != input.UserID || fact.TokenID != input.TokenID || fact.SubscriptionID != input.SubscriptionID ||
		fact.ReserveReceiptID != input.ReserveReceiptID || fact.WriterEpoch != input.WriterEpoch || fact.ActualQuota != input.ActualQuota ||
		fact.Delta != input.Delta || fact.ApplyToken != input.ApplyToken || fact.RequestFingerprint != fingerprint {
		return ErrAccountQuotaTerminalRecoveryConflict
	}
	return nil
}

func validateAuthoritativeSettlementFactSubject(ctx context.Context, db *gorm.DB, input AccountQuotaSettlementFactInput) error {
	if input.Kind != AccountQuotaSettlementKindAuthoritative {
		return nil
	}
	var reserve AccountQuotaMutationReceipt
	if err := db.WithContext(ctx).Where("id = ?", input.ReserveReceiptID).First(&reserve).Error; err != nil {
		return err
	}
	if reserve.RequestID != input.RequestID || reserve.UserID != input.UserID || reserve.TokenID != input.TokenID ||
		reserve.SubscriptionID != input.SubscriptionID || reserve.WriterEpoch != input.WriterEpoch ||
		(reserve.Phase != AccountQuotaPhaseReserve && reserve.Phase != AccountQuotaPhaseAdjust) {
		return ErrAccountQuotaTerminalRecoveryConflict
	}
	return nil
}

func settlementFactInputFromIntent(intent *AccountQuotaSettlementIntent) AccountQuotaSettlementFactInput {
	if intent == nil {
		return AccountQuotaSettlementFactInput{}
	}
	return AccountQuotaSettlementFactInput{
		EventKey: intent.EventKey, RequestID: intent.RequestID, Kind: intent.Kind,
		UserID: intent.UserID, TokenID: intent.TokenID, SubscriptionID: intent.SubscriptionID,
		ReserveReceiptID: intent.ReserveReceiptID, WriterEpoch: intent.WriterEpoch, ActualQuota: intent.ActualQuota,
		Delta: intent.Delta, ApplyToken: intent.ApplyToken,
	}
}

func validateAccountQuotaSettlementIntent(intent *AccountQuotaSettlementIntent, input AccountQuotaSettlementFactInput, fingerprint string) error {
	if intent == nil || intent.EventKey != input.EventKey || intent.RequestID != input.RequestID || intent.Kind != input.Kind ||
		intent.UserID != input.UserID || intent.TokenID != input.TokenID || intent.SubscriptionID != input.SubscriptionID ||
		intent.ReserveReceiptID != input.ReserveReceiptID || intent.WriterEpoch != input.WriterEpoch || intent.ActualQuota != input.ActualQuota ||
		intent.Delta != input.Delta || intent.ApplyToken != input.ApplyToken || intent.RequestFingerprint != fingerprint || intent.LockVersion <= 0 {
		return ErrAccountQuotaTerminalRecoveryConflict
	}
	return nil
}

func readAccountQuotaSettlementIntentDetached(ctx context.Context, db *gorm.DB, input AccountQuotaSettlementFactInput, fingerprint string) (*AccountQuotaSettlementIntent, error) {
	readCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	if accountQuotaSettlementIntentReadbackHook != nil {
		if err := accountQuotaSettlementIntentReadbackHook(); err != nil {
			return nil, err
		}
	}
	var stored AccountQuotaSettlementIntent
	if err := db.WithContext(readCtx).Where("event_key = ?", input.EventKey).First(&stored).Error; err != nil {
		return nil, err
	}
	if err := validateAccountQuotaSettlementIntent(&stored, input, fingerprint); err != nil {
		return nil, err
	}
	return &stored, nil
}

// EnsureAccountQuotaSettlementIntent persists exact settlement identity before
// the recoverable fact is materialized. This is the only source consumed by
// newly scheduled settlement recovery scans.
func EnsureAccountQuotaSettlementIntent(ctx context.Context, db *gorm.DB, input AccountQuotaSettlementFactInput) (*AccountQuotaSettlementIntent, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	normalized, fingerprint, err := normalizeAccountQuotaSettlementInput(input)
	if err != nil {
		return nil, err
	}
	if err := validateAuthoritativeSettlementFactSubject(ctx, db, normalized); err != nil {
		return nil, err
	}
	var stored AccountQuotaSettlementIntent
	result := db.WithContext(ctx).Where("event_key = ?", normalized.EventKey).Limit(1).Find(&stored)
	if result.Error != nil {
		return nil, errors.Join(result.Error, ErrAccountQuotaSettlementFactUnknown)
	}
	if result.RowsAffected > 0 {
		return &stored, validateAccountQuotaSettlementIntent(&stored, normalized, fingerprint)
	}
	now, err := taskRecoveryDBTimestamp(db.WithContext(ctx))
	if err != nil {
		return nil, errors.Join(err, ErrAccountQuotaSettlementFactUnknown)
	}
	candidate := &AccountQuotaSettlementIntent{
		EventKey: normalized.EventKey, RequestID: normalized.RequestID, Kind: normalized.Kind,
		UserID: normalized.UserID, TokenID: normalized.TokenID, SubscriptionID: normalized.SubscriptionID,
		ReserveReceiptID: normalized.ReserveReceiptID, WriterEpoch: normalized.WriterEpoch, ActualQuota: normalized.ActualQuota,
		Delta: normalized.Delta, ApplyToken: normalized.ApplyToken, RequestFingerprint: fingerprint,
		State: AccountQuotaSettlementPending, LockVersion: 1, CreatedAt: now, UpdatedAt: now,
	}
	createErr := db.WithContext(ctx).Create(candidate).Error
	if createErr == nil && accountQuotaSettlementIntentAfterCreateHook != nil {
		createErr = accountQuotaSettlementIntentAfterCreateHook(normalized.EventKey)
	}
	if createErr == nil {
		return candidate, nil
	}
	storedIntent, readErr := readAccountQuotaSettlementIntentDetached(ctx, db, normalized, fingerprint)
	if readErr == nil {
		return storedIntent, nil
	}
	return nil, errors.Join(createErr, fmt.Errorf("%w: settlement intent readback: %v", ErrAccountQuotaSettlementFactUnknown, readErr))
}

// FindAccountQuotaSettlementFactByEventKey retrieves a settlement fact by its
// unique event key. Returns (nil, nil) when no fact exists yet.
func FindAccountQuotaSettlementFactByEventKey(db *gorm.DB, eventKey string) (*AccountQuotaSettlementFact, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	key := strings.TrimSpace(eventKey)
	if key == "" {
		return nil, ErrAccountQuotaMutationInvalidInput
	}
	var fact AccountQuotaSettlementFact
	err := db.Where("event_key = ?", key).First(&fact).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &fact, nil
}

func EnsureAccountQuotaSettlementFact(ctx context.Context, db *gorm.DB, input AccountQuotaSettlementFactInput) (*AccountQuotaSettlementFact, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	normalized, fingerprint, err := normalizeAccountQuotaSettlementInput(input)
	if err != nil {
		return nil, err
	}
	if err := validateAuthoritativeSettlementFactSubject(ctx, db, normalized); err != nil {
		return nil, err
	}
	var stored AccountQuotaSettlementFact
	result := db.WithContext(ctx).Where("event_key = ?", normalized.EventKey).Limit(1).Find(&stored)
	if result.Error != nil {
		return nil, errors.Join(result.Error, ErrAccountQuotaSettlementFactUnknown, ErrAccountQuotaSettlementManualRequired)
	}
	if result.RowsAffected > 0 {
		return &stored, validateAccountQuotaSettlementIdentity(&stored, normalized, fingerprint)
	}
	now, err := taskRecoveryDBTimestamp(db.WithContext(ctx))
	if err != nil {
		return nil, errors.Join(err, ErrAccountQuotaSettlementFactUnknown, ErrAccountQuotaSettlementManualRequired)
	}
	candidate := &AccountQuotaSettlementFact{
		EventKey: normalized.EventKey, RequestID: normalized.RequestID, Kind: normalized.Kind,
		UserID: normalized.UserID, TokenID: normalized.TokenID, SubscriptionID: normalized.SubscriptionID,
		ReserveReceiptID: normalized.ReserveReceiptID, WriterEpoch: normalized.WriterEpoch, ActualQuota: normalized.ActualQuota,
		Delta: normalized.Delta, ApplyToken: normalized.ApplyToken, FundingApplied: false, TokenApplied: false, RequestFingerprint: fingerprint,
		State: AccountQuotaSettlementPending, LockVersion: 1, CreatedAt: now, UpdatedAt: now,
	}
	createErr := db.WithContext(ctx).Create(candidate).Error
	if createErr == nil && accountQuotaSettlementAfterCreateHook != nil {
		createErr = accountQuotaSettlementAfterCreateHook(normalized.EventKey)
	}
	if createErr == nil {
		return candidate, nil
	}
	readCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	if accountQuotaSettlementReadbackHook != nil {
		if err := accountQuotaSettlementReadbackHook(); err != nil {
			return nil, errors.Join(createErr, err, ErrAccountQuotaSettlementFactUnknown, ErrAccountQuotaSettlementManualRequired)
		}
	}
	if err := db.WithContext(readCtx).Where("event_key = ?", normalized.EventKey).First(&stored).Error; err != nil {
		return nil, errors.Join(createErr, err, ErrAccountQuotaSettlementFactUnknown, ErrAccountQuotaSettlementManualRequired)
	}
	if err := validateAccountQuotaSettlementIdentity(&stored, normalized, fingerprint); err != nil {
		return nil, err
	}
	return &stored, nil
}

func claimAccountQuotaSettlementFact(ctx context.Context, db *gorm.DB, fact *AccountQuotaSettlementFact, workerID string, now int64) (*AccountQuotaSettlementFact, bool, error) {
	claimable := fact.State == AccountQuotaSettlementPending ||
		(fact.State == AccountQuotaSettlementRetryable && fact.NextAttemptAt <= now) ||
		(fact.State == AccountQuotaSettlementClaimed && fact.LeaseUntil <= now)
	if !claimable {
		return fact, false, nil
	}
	result := db.WithContext(ctx).Model(&AccountQuotaSettlementFact{}).
		Where("id = ? AND state = ? AND lock_version = ? AND lease_until = ? AND next_attempt_at = ?", fact.ID, fact.State, fact.LockVersion, fact.LeaseUntil, fact.NextAttemptAt).
		Updates(map[string]any{
			"state": AccountQuotaSettlementClaimed, "lease_owner": workerID, "lease_until": now + accountQuotaSettlementLeaseSeconds,
			"next_attempt_at": int64(0), "attempts": fact.Attempts + 1, "lock_version": fact.LockVersion + 1, "updated_at": now,
		})
	if result.Error != nil || result.RowsAffected != 1 {
		return fact, false, result.Error
	}
	var claimed AccountQuotaSettlementFact
	if err := db.WithContext(ctx).First(&claimed, fact.ID).Error; err != nil {
		return nil, false, err
	}
	return &claimed, true, nil
}

func prepareLegacySettlementSubject(ctx context.Context, db *gorm.DB, kind, id int, tokenKey string) (*quotaBalanceSubjectLock, error) {
	if !common.RedisEnabled || common.RDB == nil {
		if common.BatchUpdateEnabled {
			return nil, ErrBatchQuotaCacheUnavailable
		}
		return nil, nil
	}
	lock, err := acquireQuotaBalanceSubjectLockContext(ctx, kind, id)
	if err != nil {
		return nil, err
	}
	if err := recoverQuotaBalanceSubjectPreparationsWithDB(ctx, db, kind, id, 0, lock); err != nil {
		releaseQuotaBalanceSubjectLock(lock)
		return nil, err
	}
	if err := drainQuotaBalanceGenerationsForSubjectWithDB(db.WithContext(ctx), kind, id); err != nil {
		releaseQuotaBalanceSubjectLock(lock)
		return nil, err
	}
	cacheKey := getUserCacheKey(id)
	fenceKey := ""
	if kind == BatchUpdateTypeTokenQuota {
		cacheKey = getTokenCacheKey(tokenKey)
		fenceKey = getTokenCacheFenceKey(tokenKey)
	}
	if err := invalidateQuotaBalanceSubjectCache(lock, cacheKey, fenceKey); err != nil {
		releaseQuotaBalanceSubjectLock(lock)
		return nil, err
	}
	return lock, nil
}

func applyAccountQuotaSettlementFunding(ctx context.Context, db *gorm.DB, fact *AccountQuotaSettlementFact) error {
	var lock *quotaBalanceSubjectLock
	var err error
	if fact.Kind == AccountQuotaSettlementKindLegacyWallet {
		lock, err = prepareLegacySettlementSubject(ctx, db, BatchUpdateTypeUserQuota, fact.UserID, "")
		if err != nil {
			return err
		}
		defer releaseQuotaBalanceSubjectLock(lock)
	}
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current AccountQuotaSettlementFact
		if err := lockForUpdate(tx).First(&current, fact.ID).Error; err != nil {
			return err
		}
		if current.FundingApplied {
			return nil
		}
		if current.State != AccountQuotaSettlementClaimed || current.LeaseOwner != fact.LeaseOwner || current.LockVersion != fact.LockVersion {
			return ErrAccountQuotaTerminalRecoveryConflict
		}
		switch current.Kind {
		case AccountQuotaSettlementKindLegacyWallet:
			var user User
			if err := lockForUpdate(tx).Where("id = ?", current.UserID).First(&user).Error; err != nil {
				return err
			}
			target, err := checkedLegacyBalanceTarget(user.Quota, -int(current.Delta))
			if err != nil {
				return err
			}
			result := tx.Model(&User{}).Where("id = ? AND quota = ? AND quota_version = ?", user.Id, user.Quota, user.QuotaVersion).
				Updates(map[string]any{"quota": target, "quota_version": user.QuotaVersion + 1})
			if result.Error != nil || result.RowsAffected != 1 {
				if result.Error != nil {
					return result.Error
				}
				return ErrAccountQuotaMutationCASLost
			}
		case AccountQuotaSettlementKindLegacySubscription:
			var subscription UserSubscription
			if err := lockForUpdate(tx).Where("id = ?", current.SubscriptionID).First(&subscription).Error; err != nil {
				return err
			}
			if subscription.UserId != current.UserID {
				return ErrAccountQuotaTerminalRecoveryConflict
			}
			newUsed := subscription.AmountUsed + current.Delta
			if newUsed < 0 {
				newUsed = 0
			}
			if subscription.AmountTotal > 0 && newUsed > subscription.AmountTotal {
				return ErrAccountQuotaMutationIneligible
			}
			result := tx.Model(&UserSubscription{}).Where("id = ? AND amount_used = ? AND quota_version = ?", subscription.Id, subscription.AmountUsed, subscription.QuotaVersion).
				Updates(map[string]any{"amount_used": newUsed, "quota_version": subscription.QuotaVersion + 1})
			if result.Error != nil || result.RowsAffected != 1 {
				if result.Error != nil {
					return result.Error
				}
				return ErrAccountQuotaMutationCASLost
			}
		default:
			return ErrAccountQuotaMutationInvalidInput
		}
		now, err := taskRecoveryDBTimestamp(tx)
		if err != nil {
			return err
		}
		updates := map[string]any{"funding_applied": true, "lock_version": current.LockVersion + 1, "updated_at": now}
		if !current.ApplyToken {
			updates["token_applied"] = true
			updates["state"] = AccountQuotaSettlementApplied
			updates["lease_owner"] = ""
			updates["lease_until"] = int64(0)
		}
		result := tx.Model(&AccountQuotaSettlementFact{}).
			Where("id = ? AND state = ? AND lease_owner = ? AND lock_version = ?", current.ID, AccountQuotaSettlementClaimed, current.LeaseOwner, current.LockVersion).
			Updates(updates)
		if result.Error != nil || result.RowsAffected != 1 {
			if result.Error != nil {
				return result.Error
			}
			return ErrAccountQuotaMutationCASLost
		}
		return nil
	})
	if err == nil && accountQuotaSettlementAfterFundingCommitHook != nil {
		err = accountQuotaSettlementAfterFundingCommitHook(fact.EventKey)
	}
	var stored AccountQuotaSettlementFact
	readCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	readErr := db.WithContext(readCtx).First(&stored, fact.ID).Error
	cancel()
	if err != nil && (readErr != nil || !stored.FundingApplied) {
		return errors.Join(err, readErr)
	}
	if lock != nil {
		var user User
		if readErr := db.WithContext(ctx).First(&user, fact.UserID).Error; readErr == nil {
			_ = writeUserCacheWithQuotaBalanceOwner(user.ToBaseUser(), true, lock)
		}
	}
	return nil
}

func applyAccountQuotaSettlementToken(ctx context.Context, db *gorm.DB, fact *AccountQuotaSettlementFact) error {
	if !fact.ApplyToken {
		return nil
	}
	var token Token
	if err := db.WithContext(ctx).Select("id", "key").Where("id = ? AND user_id = ?", fact.TokenID, fact.UserID).First(&token).Error; err != nil {
		return err
	}
	lock, err := prepareLegacySettlementSubject(ctx, db, BatchUpdateTypeTokenQuota, fact.TokenID, token.Key)
	if err != nil {
		return err
	}
	defer releaseQuotaBalanceSubjectLock(lock)
	if accountQuotaSettlementBeforeTokenHook != nil {
		if err := accountQuotaSettlementBeforeTokenHook(fact); err != nil {
			return err
		}
	}
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current AccountQuotaSettlementFact
		if err := lockForUpdate(tx).First(&current, fact.ID).Error; err != nil {
			return err
		}
		if current.TokenApplied && current.State == AccountQuotaSettlementApplied {
			return nil
		}
		if !current.FundingApplied || current.State != AccountQuotaSettlementClaimed || current.LeaseOwner != fact.LeaseOwner {
			return ErrAccountQuotaTerminalRecoveryConflict
		}
		var currentToken Token
		if err := lockForUpdate(tx).Where("id = ? AND user_id = ?", current.TokenID, current.UserID).First(&currentToken).Error; err != nil {
			return err
		}
		remain, err := checkedLegacyBalanceTarget(currentToken.RemainQuota, -int(current.Delta))
		if err != nil {
			return err
		}
		used, err := checkedLegacyBalanceTarget(currentToken.UsedQuota, int(current.Delta))
		if err != nil || used < 0 {
			return ErrAccountQuotaMutationIneligible
		}
		now, err := taskRecoveryDBTimestamp(tx)
		if err != nil {
			return err
		}
		result := tx.Model(&Token{}).Where("id = ? AND remain_quota = ? AND used_quota = ? AND quota_version = ?", currentToken.Id, currentToken.RemainQuota, currentToken.UsedQuota, currentToken.QuotaVersion).
			Updates(map[string]any{"remain_quota": remain, "used_quota": used, "quota_version": currentToken.QuotaVersion + 1, "accessed_time": now})
		if result.Error != nil || result.RowsAffected != 1 {
			if result.Error != nil {
				return result.Error
			}
			return ErrAccountQuotaMutationCASLost
		}
		result = tx.Model(&AccountQuotaSettlementFact{}).
			Where("id = ? AND state = ? AND lease_owner = ? AND lock_version = ?", current.ID, AccountQuotaSettlementClaimed, current.LeaseOwner, current.LockVersion).
			Updates(map[string]any{
				"token_applied": true, "state": AccountQuotaSettlementApplied, "lease_owner": "", "lease_until": int64(0),
				"last_error": "", "next_attempt_at": int64(0), "lock_version": current.LockVersion + 1, "updated_at": now,
			})
		if result.Error != nil || result.RowsAffected != 1 {
			if result.Error != nil {
				return result.Error
			}
			return ErrAccountQuotaMutationCASLost
		}
		return nil
	})
	if err == nil && accountQuotaSettlementAfterTokenCommitHook != nil {
		err = accountQuotaSettlementAfterTokenCommitHook(fact.EventKey)
	}
	var stored AccountQuotaSettlementFact
	readCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	readErr := db.WithContext(readCtx).First(&stored, fact.ID).Error
	cancel()
	if err != nil && (readErr != nil || stored.State != AccountQuotaSettlementApplied) {
		return errors.Join(err, readErr)
	}
	if lock != nil {
		if readErr := db.WithContext(ctx).First(&token, fact.TokenID).Error; readErr == nil {
			_, _ = cacheInitTokenWithQuotaBalanceOwner(token, lock)
		}
	}
	return nil
}

func applyAuthoritativeAccountQuotaSettlement(ctx context.Context, db *gorm.DB, fact *AccountQuotaSettlementFact) error {
	if fact == nil || fact.Kind != AccountQuotaSettlementKindAuthoritative {
		return ErrAccountQuotaMutationInvalidInput
	}
	receipt, err := SettleAccountQuota(ctx, db, AccountQuotaTerminalInput{
		RequestID: fact.RequestID, ReserveReceiptID: fact.ReserveReceiptID, ActualQuota: fact.ActualQuota,
	})
	if err != nil {
		return err
	}
	if receipt == nil || receipt.Phase != AccountQuotaPhaseSettle || receipt.WriterEpoch != fact.WriterEpoch ||
		receipt.UserID != fact.UserID || receipt.TokenID != fact.TokenID || receipt.SubscriptionID != fact.SubscriptionID ||
		receipt.RequestedQuota != fact.ActualQuota {
		return ErrAccountQuotaTerminalRecoveryConflict
	}
	if accountQuotaSettlementAfterTerminalCommitHook != nil {
		if err := accountQuotaSettlementAfterTerminalCommitHook(fact.EventKey); err != nil {
			return err
		}
	}
	now, err := taskRecoveryDBTimestamp(db.WithContext(ctx))
	if err != nil {
		return err
	}
	result := db.WithContext(ctx).Model(&AccountQuotaSettlementFact{}).
		Where("id = ? AND state = ? AND lease_owner = ? AND lock_version = ?", fact.ID, AccountQuotaSettlementClaimed, fact.LeaseOwner, fact.LockVersion).
		Updates(map[string]any{
			"funding_applied": true, "token_applied": true, "terminal_receipt_id": receipt.ID,
			"state": AccountQuotaSettlementApplied, "last_error": "", "next_attempt_at": int64(0),
			"lease_owner": "", "lease_until": int64(0), "lock_version": fact.LockVersion + 1, "updated_at": now,
		})
	if result.Error == nil && result.RowsAffected == 1 {
		return nil
	}
	readCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	var stored AccountQuotaSettlementFact
	readErr := db.WithContext(readCtx).First(&stored, fact.ID).Error
	if readErr == nil && stored.State == AccountQuotaSettlementApplied && stored.TerminalReceiptID == receipt.ID {
		return nil
	}
	if result.Error != nil {
		return errors.Join(result.Error, readErr)
	}
	return errors.Join(ErrAccountQuotaMutationCASLost, readErr)
}

func finishAccountQuotaSettlementFailure(ctx context.Context, db *gorm.DB, claimed *AccountQuotaSettlementFact, runErr error) error {
	now, err := taskRecoveryDBTimestamp(db.WithContext(ctx))
	if err != nil {
		return err
	}
	state := AccountQuotaSettlementRetryable
	if errors.Is(runErr, gorm.ErrRecordNotFound) || errors.Is(runErr, ErrAccountQuotaMutationInvalidInput) ||
		errors.Is(runErr, ErrAccountQuotaMutationIneligible) || errors.Is(runErr, ErrAccountQuotaTerminalRecoveryConflict) ||
		errors.Is(runErr, ErrAccountQuotaMutationTerminal) || errors.Is(runErr, ErrAccountQuotaMutationNotFound) ||
		errors.Is(runErr, ErrAccountQuotaMutationStaleReceipt) || errors.Is(runErr, ErrQuotaWriterEpochMismatch) {
		state = AccountQuotaSettlementManual
	}
	message := runErr.Error()
	if len(message) > 4096 {
		message = message[:4096]
	}
	nextAttemptAt := now
	if state == AccountQuotaSettlementManual {
		nextAttemptAt = 0
	}
	result := db.WithContext(ctx).Model(&AccountQuotaSettlementFact{}).
		Where("id = ? AND state = ? AND lease_owner = ? AND lock_version = ?", claimed.ID, AccountQuotaSettlementClaimed, claimed.LeaseOwner, claimed.LockVersion).
		Updates(map[string]any{
			"state": state, "last_error": message, "next_attempt_at": nextAttemptAt, "lease_owner": "", "lease_until": int64(0),
			"lock_version": claimed.LockVersion + 1, "updated_at": now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		var stored AccountQuotaSettlementFact
		if err := db.WithContext(ctx).First(&stored, claimed.ID).Error; err == nil && stored.State == AccountQuotaSettlementApplied {
			return nil
		}
		return ErrAccountQuotaTerminalRecoveryConflict
	}
	return nil
}

func RecoverAccountQuotaSettlementFact(ctx context.Context, db *gorm.DB, fact *AccountQuotaSettlementFact, workerID string) (*AccountQuotaSettlementFact, error) {
	if db == nil || fact == nil || strings.TrimSpace(workerID) == "" {
		return nil, gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	now, err := taskRecoveryDBTimestamp(db.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	claimed, won, err := claimAccountQuotaSettlementFact(ctx, db, fact, workerID, now)
	if err != nil || !won {
		return claimed, err
	}
	if claimed.Kind == AccountQuotaSettlementKindAuthoritative {
		err = applyAuthoritativeAccountQuotaSettlement(ctx, db, claimed)
	} else if !claimed.FundingApplied {
		err = applyAccountQuotaSettlementFunding(ctx, db, claimed)
		if err == nil {
			if readErr := db.WithContext(ctx).First(claimed, claimed.ID).Error; readErr != nil {
				err = readErr
			}
		}
	}
	if err == nil && claimed.Kind != AccountQuotaSettlementKindAuthoritative && claimed.State != AccountQuotaSettlementApplied && !claimed.TokenApplied {
		err = applyAccountQuotaSettlementToken(ctx, db, claimed)
	}
	if err != nil {
		if finishErr := finishAccountQuotaSettlementFailure(ctx, db, claimed, err); finishErr != nil {
			return claimed, errors.Join(err, finishErr)
		}
		var stored AccountQuotaSettlementFact
		_ = db.WithContext(ctx).First(&stored, claimed.ID).Error
		if stored.State == AccountQuotaSettlementManual {
			return &stored, errors.Join(ErrAccountQuotaSettlementManualRequired, err)
		}
		return &stored, errors.Join(ErrAccountQuotaSettlementPending, err)
	}
	var stored AccountQuotaSettlementFact
	if err := db.WithContext(ctx).First(&stored, claimed.ID).Error; err != nil {
		return claimed, err
	}
	return &stored, nil
}

func RunAccountQuotaSettlementFacts(ctx context.Context, db *gorm.DB, workerID string, limit int) (int, error) {
	if db == nil || strings.TrimSpace(workerID) == "" {
		return 0, gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	cursor, err := loadQuotaWorkCursor(ctx, db, quotaWorkCursorSettlementFact)
	if err != nil {
		return 0, err
	}
	if err := beginQuotaWorkCycle(ctx, db, cursor, &AccountQuotaSettlementFact{}); err != nil {
		return 0, err
	}
	if cursor.HighWatermark == 0 {
		return 0, nil
	}
	now, err := taskRecoveryDBTimestamp(db.WithContext(ctx))
	if err != nil {
		return 0, err
	}
	processed := 0
	scanned := 0
	var runErrors []error
	for scanned < limit {
		var facts []AccountQuotaSettlementFact
		if err := db.WithContext(ctx).Where("id > ? AND id <= ? AND (state = ? OR (state = ? AND next_attempt_at <= ?) OR (state = ? AND lease_until <= ?))",
			cursor.LastID, cursor.HighWatermark, AccountQuotaSettlementPending, AccountQuotaSettlementRetryable, now, AccountQuotaSettlementClaimed, now).
			Order("id ASC").Limit(min(25, limit-scanned)).Find(&facts).Error; err != nil {
			return processed, err
		}
		if len(facts) == 0 {
			if err := finishQuotaWorkCycle(ctx, db, cursor); err != nil {
				return processed, err
			}
			return processed, errors.Join(runErrors...)
		}
		for index := range facts {
			cursor.LastID = facts[index].ID
			scanned++
			stored, runErr := RecoverAccountQuotaSettlementFact(ctx, db, &facts[index], fmt.Sprintf("%s:%d", workerID, facts[index].ID))
			if stored != nil && stored.State == AccountQuotaSettlementApplied {
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

func updateAccountQuotaSettlementIntent(ctx context.Context, db *gorm.DB, intent *AccountQuotaSettlementIntent, fact *AccountQuotaSettlementFact, state string, runErr error) (*AccountQuotaSettlementIntent, error) {
	if intent == nil {
		return nil, ErrAccountQuotaMutationInvalidInput
	}
	now, err := taskRecoveryDBTimestamp(db.WithContext(ctx))
	if err != nil {
		return intent, err
	}
	lastError := ""
	if runErr != nil {
		lastError = runErr.Error()
		if len(lastError) > 4096 {
			lastError = lastError[:4096]
		}
	}
	updates := map[string]any{
		"state": state, "attempts": intent.Attempts + 1, "last_error": lastError,
		"lock_version": intent.LockVersion + 1, "updated_at": now,
	}
	if fact != nil {
		updates["fact_id"] = fact.ID
		updates["terminal_receipt_id"] = fact.TerminalReceiptID
	}
	result := db.WithContext(ctx).Model(&AccountQuotaSettlementIntent{}).
		Where("id = ? AND lock_version = ?", intent.ID, intent.LockVersion).Updates(updates)
	if result.Error != nil {
		return intent, result.Error
	}
	var stored AccountQuotaSettlementIntent
	if err := db.WithContext(ctx).First(&stored, intent.ID).Error; err != nil {
		return intent, err
	}
	if result.RowsAffected != 1 && stored.State != AccountQuotaSettlementApplied && stored.State != AccountQuotaSettlementManual {
		return &stored, ErrAccountQuotaMutationCASLost
	}
	return &stored, nil
}

func accountQuotaSettlementIntentStructuralError(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, ErrAccountQuotaMutationInvalidInput) ||
		errors.Is(err, ErrAccountQuotaMutationIneligible) || errors.Is(err, ErrAccountQuotaTerminalRecoveryConflict) ||
		errors.Is(err, ErrAccountQuotaMutationTerminal) || errors.Is(err, ErrAccountQuotaMutationNotFound) ||
		errors.Is(err, ErrAccountQuotaMutationStaleReceipt) || errors.Is(err, ErrQuotaWriterEpochMismatch)
}

func accountQuotaSettlementIntentCreationStructuralError(err error) bool {
	return errors.Is(err, ErrAccountQuotaMutationInvalidInput) || errors.Is(err, ErrAccountQuotaMutationIneligible) ||
		errors.Is(err, ErrAccountQuotaTerminalRecoveryConflict) || errors.Is(err, ErrAccountQuotaMutationTerminal) ||
		errors.Is(err, ErrAccountQuotaMutationStaleReceipt) || errors.Is(err, ErrQuotaWriterEpochMismatch)
}

// RecoverAccountQuotaSettlementIntent materializes and applies one durable
// inbox row. Repeated or concurrent scans converge through fact and receipt
// fingerprints rather than through task payload ordering.
func RecoverAccountQuotaSettlementIntent(ctx context.Context, db *gorm.DB, intent *AccountQuotaSettlementIntent, workerID string) (*AccountQuotaSettlementIntent, *AccountQuotaSettlementFact, error) {
	if db == nil || intent == nil || strings.TrimSpace(workerID) == "" {
		return nil, nil, gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	input := settlementFactInputFromIntent(intent)
	_, fingerprint, err := normalizeAccountQuotaSettlementInput(input)
	if err != nil || validateAccountQuotaSettlementIntent(intent, input, fingerprint) != nil {
		if err == nil {
			err = ErrAccountQuotaTerminalRecoveryConflict
		}
		stored, updateErr := updateAccountQuotaSettlementIntent(ctx, db, intent, nil, AccountQuotaSettlementManual, err)
		return stored, nil, errors.Join(ErrAccountQuotaSettlementManualRequired, err, updateErr)
	}
	if intent.State == AccountQuotaSettlementManual {
		return intent, nil, ErrAccountQuotaSettlementManualRequired
	}
	if intent.State == AccountQuotaSettlementApplied {
		var fact AccountQuotaSettlementFact
		if intent.FactID <= 0 || db.WithContext(ctx).First(&fact, intent.FactID).Error != nil || fact.State != AccountQuotaSettlementApplied {
			return intent, nil, ErrAccountQuotaTerminalRecoveryConflict
		}
		return intent, &fact, nil
	}

	fact, ensureErr := EnsureAccountQuotaSettlementFact(ctx, db, input)
	if ensureErr != nil {
		state := AccountQuotaSettlementPending
		if accountQuotaSettlementIntentCreationStructuralError(ensureErr) {
			state = AccountQuotaSettlementManual
		}
		stored, updateErr := updateAccountQuotaSettlementIntent(ctx, db, intent, nil, state, ensureErr)
		if state == AccountQuotaSettlementManual {
			return stored, nil, errors.Join(ErrAccountQuotaSettlementManualRequired, ensureErr, updateErr)
		}
		return stored, nil, errors.Join(ErrAccountQuotaSettlementPending, ensureErr, updateErr)
	}
	storedFact, recoverErr := RecoverAccountQuotaSettlementFact(ctx, db, fact, workerID+":fact")
	if storedFact == nil {
		storedFact = fact
	}
	state := AccountQuotaSettlementMaterialized
	if storedFact.State == AccountQuotaSettlementApplied {
		state = AccountQuotaSettlementApplied
	} else if storedFact.State == AccountQuotaSettlementManual || accountQuotaSettlementIntentStructuralError(recoverErr) {
		state = AccountQuotaSettlementManual
	}
	storedIntent, updateErr := updateAccountQuotaSettlementIntent(ctx, db, intent, storedFact, state, recoverErr)
	if state == AccountQuotaSettlementApplied && updateErr == nil {
		return storedIntent, storedFact, nil
	}
	if state == AccountQuotaSettlementManual {
		return storedIntent, storedFact, errors.Join(ErrAccountQuotaSettlementManualRequired, recoverErr, updateErr)
	}
	return storedIntent, storedFact, errors.Join(ErrAccountQuotaSettlementPending, recoverErr, updateErr)
}

func RunAccountQuotaSettlementIntents(ctx context.Context, db *gorm.DB, workerID string, limit int) (int, error) {
	if db == nil || strings.TrimSpace(workerID) == "" {
		return 0, gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	cursor, err := loadQuotaWorkCursor(ctx, db, quotaWorkCursorSettlementIntent)
	if err != nil {
		return 0, err
	}
	if err := beginQuotaWorkCycle(ctx, db, cursor, &AccountQuotaSettlementIntent{}); err != nil {
		return 0, err
	}
	if cursor.HighWatermark == 0 {
		return 0, nil
	}
	processed := 0
	scanned := 0
	var runErrors []error
	for scanned < limit {
		var intents []AccountQuotaSettlementIntent
		if err := db.WithContext(ctx).Where("id > ? AND id <= ? AND state IN ?", cursor.LastID, cursor.HighWatermark,
			[]string{AccountQuotaSettlementPending, AccountQuotaSettlementMaterialized}).
			Order("id ASC").Limit(min(25, limit-scanned)).Find(&intents).Error; err != nil {
			return processed, errors.Join(append(runErrors, err)...)
		}
		if len(intents) == 0 {
			if err := finishQuotaWorkCycle(ctx, db, cursor); err != nil {
				return processed, errors.Join(append(runErrors, err)...)
			}
			return processed, errors.Join(runErrors...)
		}
		for index := range intents {
			if err := ctx.Err(); err != nil {
				return processed, errors.Join(append(runErrors, err)...)
			}
			cursor.LastID = intents[index].ID
			scanned++
			stored, _, runErr := RecoverAccountQuotaSettlementIntent(ctx, db, &intents[index], fmt.Sprintf("%s:%d", workerID, intents[index].ID))
			if stored != nil && stored.State == AccountQuotaSettlementApplied {
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
