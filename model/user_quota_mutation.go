package model

import (
	"context"
	"crypto/sha256"
	"database/sql"
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
	UserQuotaMutationReceiptVersion       = 1
	userQuotaMutationReceiptCreateSetting = "user-quota-mutation:receipt-create"
	userQuotaMutationMaxKeyLength         = 128
	userQuotaMutationMaxTypeLength        = 32
	userQuotaMutationMaxReasonLength      = 64
	userQuotaMutationMaxMetadataLength    = 16 * 1024
)

var (
	ErrUserQuotaMutationReceiptImmutable  = errors.New("user quota mutation receipt is immutable")
	ErrUserQuotaMutationConflict          = errors.New("user quota mutation conflicts with existing receipt")
	ErrUserQuotaMutationInvalidInput      = errors.New("invalid user quota mutation input")
	ErrUserQuotaCASLost                   = errors.New("user quota compare-and-swap lost")
	ErrInsufficientUserQuota              = errors.New("insufficient user quota")
	ErrUserQuotaUpperBoundExceeded        = errors.New("user quota upper bound exceeded")
	ErrUserQuotaBusinessSchemaUnavailable = errors.New("user quota business writer schema capability is unavailable")

	userQuotaBusinessSchemaCapability  atomic.Pointer[userQuotaBusinessSchemaSnapshot]
	userQuotaBusinessSchemaBinding     atomic.Pointer[userQuotaBusinessDBBinding]
	userQuotaBusinessDBGeneration      atomic.Uint64
	userQuotaBusinessSnapshotSequence  atomic.Uint64
	userQuotaBusinessBeforePublishHook func(*userQuotaBusinessPublishExpectation)
)

const (
	userQuotaBusinessSchemaUnknown = iota
	userQuotaBusinessSchemaLegacy
	userQuotaBusinessSchemaReady
)

type userQuotaBusinessSchemaSnapshot struct {
	pool             *sql.DB
	dbGeneration     uint64
	snapshotSequence uint64
	state            int
}

type userQuotaBusinessDBBinding struct {
	pool       *sql.DB
	generation uint64
}

type userQuotaBusinessPublishExpectation struct {
	pool             *sql.DB
	binding          *userQuotaBusinessDBBinding
	previousSnapshot *userQuotaBusinessSchemaSnapshot
}

var userQuotaBusinessSchemaCatalog = func(db *gorm.DB) ([]string, error) {
	return db.Migrator().GetTables()
}

func currentUserQuotaBusinessMainPool() (*sql.DB, error) {
	if DB == nil {
		return nil, ErrUserQuotaBusinessSchemaUnavailable
	}
	pool, err := DB.DB()
	if err != nil {
		return nil, err
	}
	return pool, nil
}

func bindUserQuotaBusinessMainDB(pool *sql.DB) (*userQuotaBusinessDBBinding, error) {
	for {
		mainPool, err := currentUserQuotaBusinessMainPool()
		if err != nil || mainPool != pool {
			return nil, ErrUserQuotaBusinessSchemaUnavailable
		}
		current := userQuotaBusinessSchemaBinding.Load()
		if current != nil && current.pool == pool {
			return current, nil
		}
		next := &userQuotaBusinessDBBinding{pool: pool, generation: userQuotaBusinessDBGeneration.Add(1)}
		mainPool, err = currentUserQuotaBusinessMainPool()
		if err != nil || mainPool != pool {
			return nil, ErrUserQuotaBusinessSchemaUnavailable
		}
		if userQuotaBusinessSchemaBinding.CompareAndSwap(current, next) {
			mainPool, err = currentUserQuotaBusinessMainPool()
			if err == nil && mainPool == pool {
				return next, nil
			}
			userQuotaBusinessSchemaBinding.CompareAndSwap(next, current)
			return nil, ErrUserQuotaBusinessSchemaUnavailable
		}
	}
}

func userQuotaBusinessPublishStillCurrent(pool *sql.DB, binding *userQuotaBusinessDBBinding) bool {
	if binding == nil || userQuotaBusinessSchemaBinding.Load() != binding || DB == nil {
		return false
	}
	mainPool, err := DB.DB()
	return err == nil && mainPool == pool
}

func captureUserQuotaBusinessPublishExpectation(db *gorm.DB) (*userQuotaBusinessPublishExpectation, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	pool, err := db.DB()
	if err != nil {
		return nil, err
	}
	binding, err := bindUserQuotaBusinessMainDB(pool)
	if err != nil {
		return nil, err
	}
	return &userQuotaBusinessPublishExpectation{
		pool: pool, binding: binding, previousSnapshot: userQuotaBusinessSchemaCapability.Load(),
	}, nil
}

func publishUserQuotaBusinessSchemaCapability(expected *userQuotaBusinessPublishExpectation, state int) error {
	if expected == nil || !userQuotaBusinessPublishStillCurrent(expected.pool, expected.binding) {
		return ErrUserQuotaBusinessSchemaUnavailable
	}
	if userQuotaBusinessBeforePublishHook != nil {
		userQuotaBusinessBeforePublishHook(expected)
	}
	if !userQuotaBusinessPublishStillCurrent(expected.pool, expected.binding) {
		return ErrUserQuotaBusinessSchemaUnavailable
	}
	next := &userQuotaBusinessSchemaSnapshot{
		pool: expected.pool, dbGeneration: expected.binding.generation,
		snapshotSequence: userQuotaBusinessSnapshotSequence.Add(1), state: state,
	}
	if !userQuotaBusinessSchemaCapability.CompareAndSwap(expected.previousSnapshot, next) {
		return ErrUserQuotaBusinessSchemaUnavailable
	}
	if userQuotaBusinessPublishStillCurrent(expected.pool, expected.binding) {
		return nil
	}
	currentBinding := userQuotaBusinessSchemaBinding.Load()
	currentPool, err := currentUserQuotaBusinessMainPool()
	if err == nil && currentBinding != nil && currentBinding.pool == currentPool {
		unknown := &userQuotaBusinessSchemaSnapshot{
			pool: currentPool, dbGeneration: currentBinding.generation,
			snapshotSequence: userQuotaBusinessSnapshotSequence.Add(1), state: userQuotaBusinessSchemaUnknown,
		}
		userQuotaBusinessSchemaCapability.CompareAndSwap(next, unknown)
	} else {
		userQuotaBusinessSchemaCapability.CompareAndSwap(next, nil)
	}
	return ErrUserQuotaBusinessSchemaUnavailable
}

func RefreshUserQuotaBusinessSchemaCapability(db *gorm.DB) error {
	expected, err := captureUserQuotaBusinessPublishExpectation(db)
	if err != nil {
		return err
	}
	tables, err := userQuotaBusinessSchemaCatalog(db)
	if err != nil {
		if publishErr := publishUserQuotaBusinessSchemaCapability(expected, userQuotaBusinessSchemaUnknown); publishErr != nil {
			return publishErr
		}
		return fmt.Errorf("%w: %v", ErrUserQuotaBusinessSchemaUnavailable, err)
	}
	tableSet := make(map[string]struct{}, len(tables))
	for _, table := range tables {
		tableSet[strings.ToLower(table)] = struct{}{}
	}
	if _, ok := tableSet["quota_writer_epochs"]; ok {
		return publishUserQuotaBusinessSchemaCapability(expected, userQuotaBusinessSchemaReady)
	}
	// Git history shows system_tasks/system_task_locks already existed in
	// 0d7af39 (2026-08-28), while the C03b receipt/projection generation began
	// in 1d5bc44 and the epoch/work-cursor generation in 522bce4. Only tables
	// introduced by those durable-accounting generations make an epoch-less
	// schema partial; older shared infrastructure remains a valid legacy DB.
	for _, durableTable := range []string{
		"user_quota_mutation_receipts", "account_quota_mutation_receipts", "account_quota_reservation_heads",
		"account_quota_terminal_recovery_obligations", "account_quota_refund_facts", "account_quota_settlement_intents",
		"account_quota_settlement_facts", "quota_projection_obligations", "quota_work_cursors", "quota_balance_batch_drains",
		"quota_balance_batch_subjects", "quota_mutation_receipts",
	} {
		if _, ok := tableSet[durableTable]; ok {
			if publishErr := publishUserQuotaBusinessSchemaCapability(expected, userQuotaBusinessSchemaUnknown); publishErr != nil {
				return publishErr
			}
			return fmt.Errorf("%w: durable table %s exists without quota_writer_epochs", ErrUserQuotaBusinessSchemaUnavailable, durableTable)
		}
	}
	return publishUserQuotaBusinessSchemaCapability(expected, userQuotaBusinessSchemaLegacy)
}

func publishUserQuotaBusinessSchemaReady(expected *userQuotaBusinessPublishExpectation) error {
	return publishUserQuotaBusinessSchemaCapability(expected, userQuotaBusinessSchemaReady)
}

type userQuotaMutationReceiptCreateMarkerType struct {
	value byte
}

var userQuotaMutationReceiptCreateMarker = &userQuotaMutationReceiptCreateMarkerType{}

// UserQuotaMutationReceipt represents an immutable, auditable mutation receipt for a user's quota.
// BusinessEventKey enforces exact replay deduplication and conflict detection.
type UserQuotaMutationReceipt struct {
	ID                 int64                     `json:"id" gorm:"primaryKey"`
	ReceiptVersion     int                       `json:"receipt_version" gorm:"not null;<-:create"`
	WriterEpoch        int64                     `json:"writer_epoch" gorm:"type:bigint;<-:create"`
	MutationType       string                    `json:"mutation_type" gorm:"type:varchar(32);not null;index:idx_user_quota_mutation_lookup,priority:2;<-:create"`
	BusinessEventKey   string                    `json:"business_event_key" gorm:"type:varchar(128);not null;uniqueIndex:uidx_user_quota_mutation_key;<-:create"`
	RequestFingerprint string                    `json:"request_fingerprint" gorm:"type:char(64);not null;<-:create"`
	UserID             int                       `json:"user_id" gorm:"not null;index:idx_user_quota_mutation_lookup,priority:1;<-:create"`
	Delta              int64                     `json:"delta" gorm:"type:bigint;not null;<-:create"`
	QuotaBefore        int                       `json:"quota_before" gorm:"not null;<-:create"`
	QuotaAfter         int                       `json:"quota_after" gorm:"not null;<-:create"`
	QuotaVersionBefore int64                     `json:"quota_version_before" gorm:"type:bigint;not null;<-:create"`
	QuotaVersionAfter  int64                     `json:"quota_version_after" gorm:"type:bigint;not null;<-:create"`
	ReasonCode         string                    `json:"reason_code" gorm:"type:varchar(64);not null;<-:create"`
	OperatorUserID     int                       `json:"operator_user_id" gorm:"not null;default:0;<-:create"`
	Metadata           string                    `json:"metadata" gorm:"type:text;not null;<-:create"`
	After              QuotaMutationUserSnapshot `json:"after" gorm:"type:text;<-:create"`
	CreatedAt          int64                     `json:"created_at" gorm:"type:bigint;not null;index;<-:create"`
}

func (UserQuotaMutationReceipt) TableName() string {
	return "user_quota_mutation_receipts"
}

func userQuotaMutationReceiptCreateAllowed(tx *gorm.DB) bool {
	if tx == nil {
		return false
	}
	val, ok := tx.Get(userQuotaMutationReceiptCreateSetting)
	return ok && val == userQuotaMutationReceiptCreateMarker
}

func userQuotaMutationReceiptCreateDB(tx *gorm.DB) *gorm.DB {
	return tx.Session(&gorm.Session{NewDB: true}).Set(userQuotaMutationReceiptCreateSetting, userQuotaMutationReceiptCreateMarker)
}

func (receipt *UserQuotaMutationReceipt) BeforeCreate(tx *gorm.DB) error {
	if tx == nil {
		return gorm.ErrInvalidDB
	}
	if !userQuotaMutationReceiptCreateAllowed(tx) {
		return fmt.Errorf("%w: user quota receipts may only be created by authoritative mutation kernel", ErrUserQuotaMutationReceiptImmutable)
	}
	if receipt == nil || receipt.ID != 0 || receipt.CreatedAt != 0 {
		return fmt.Errorf("%w: new user quota receipt carries database-managed fields", ErrUserQuotaMutationInvalidInput)
	}
	if receipt.ReceiptVersion != UserQuotaMutationReceiptVersion || receipt.MutationType == "" ||
		receipt.BusinessEventKey == "" || len(receipt.RequestFingerprint) != 64 || receipt.UserID <= 0 {
		return fmt.Errorf("%w: user quota receipt shape is invalid", ErrUserQuotaMutationInvalidInput)
	}
	createdAt, err := taskRecoveryDBTimestamp(tx)
	if err != nil {
		return err
	}
	receipt.CreatedAt = createdAt
	return nil
}

func (*UserQuotaMutationReceipt) BeforeUpdate(_ *gorm.DB) error {
	return ErrUserQuotaMutationReceiptImmutable
}

func (*UserQuotaMutationReceipt) BeforeDelete(_ *gorm.DB) error {
	return ErrUserQuotaMutationReceiptImmutable
}

// UserQuotaMutationInput contains the parameters for an authoritative quota mutation.
type UserQuotaMutationInput struct {
	UserID            int
	Delta             int64
	MutationType      string
	BusinessEventKey  string
	ReasonCode        string
	OperatorUserID    int
	Metadata          map[string]interface{}
	MaxQuotaExclusive int64
}

type userQuotaMutationFingerprintPayload struct {
	Version           int                    `json:"version"`
	UserID            int                    `json:"user_id"`
	Delta             int64                  `json:"delta"`
	MutationType      string                 `json:"mutation_type"`
	BusinessEventKey  string                 `json:"business_event_key"`
	ReasonCode        string                 `json:"reason_code"`
	OperatorUserID    int                    `json:"operator_user_id"`
	Metadata          map[string]interface{} `json:"metadata,omitempty"`
	MaxQuotaExclusive int64                  `json:"max_quota_exclusive,omitempty"`
}

func normalizeUserQuotaMutationInput(input UserQuotaMutationInput) (UserQuotaMutationInput, string, string, error) {
	normalized := input
	normalized.MutationType = strings.ToLower(strings.TrimSpace(input.MutationType))
	normalized.BusinessEventKey = strings.TrimSpace(input.BusinessEventKey)
	normalized.ReasonCode = strings.TrimSpace(input.ReasonCode)

	if normalized.UserID <= 0 {
		return normalized, "", "", fmt.Errorf("%w: user id must be positive", ErrUserQuotaMutationInvalidInput)
	}
	if normalized.MutationType == "" || len(normalized.MutationType) > userQuotaMutationMaxTypeLength {
		return normalized, "", "", fmt.Errorf("%w: mutation type is required and must not exceed %d chars", ErrUserQuotaMutationInvalidInput, userQuotaMutationMaxTypeLength)
	}
	if normalized.BusinessEventKey == "" || len(normalized.BusinessEventKey) > userQuotaMutationMaxKeyLength {
		return normalized, "", "", fmt.Errorf("%w: business event key is required and must not exceed %d chars", ErrUserQuotaMutationInvalidInput, userQuotaMutationMaxKeyLength)
	}
	if len(normalized.ReasonCode) > userQuotaMutationMaxReasonLength {
		return normalized, "", "", fmt.Errorf("%w: reason code exceeds %d chars", ErrUserQuotaMutationInvalidInput, userQuotaMutationMaxReasonLength)
	}
	if normalized.Delta == 0 {
		return normalized, "", "", fmt.Errorf("%w: delta cannot be zero", ErrUserQuotaMutationInvalidInput)
	}
	if normalized.Delta < int64(common.MinQuota) || normalized.Delta > int64(common.MaxQuota) {
		return normalized, "", "", fmt.Errorf("%w: delta out of allowed quota bounds", ErrUserQuotaMutationInvalidInput)
	}
	if normalized.MaxQuotaExclusive < 0 || normalized.MaxQuotaExclusive > int64(common.MaxQuota) {
		return normalized, "", "", fmt.Errorf("%w: quota upper bound is invalid", ErrUserQuotaMutationInvalidInput)
	}

	metadataStr := ""
	if len(normalized.Metadata) > 0 {
		data, err := common.Marshal(normalized.Metadata)
		if err != nil {
			return normalized, "", "", fmt.Errorf("%w: failed to serialize metadata: %v", ErrUserQuotaMutationInvalidInput, err)
		}
		if len(data) > userQuotaMutationMaxMetadataLength {
			return normalized, "", "", fmt.Errorf("%w: metadata exceeds %d bytes", ErrUserQuotaMutationInvalidInput, userQuotaMutationMaxMetadataLength)
		}
		metadataStr = string(data)
	}

	payload := userQuotaMutationFingerprintPayload{
		Version:           UserQuotaMutationReceiptVersion,
		UserID:            normalized.UserID,
		Delta:             normalized.Delta,
		MutationType:      normalized.MutationType,
		BusinessEventKey:  normalized.BusinessEventKey,
		ReasonCode:        normalized.ReasonCode,
		OperatorUserID:    normalized.OperatorUserID,
		Metadata:          normalized.Metadata,
		MaxQuotaExclusive: normalized.MaxQuotaExclusive,
	}
	payloadBytes, err := common.Marshal(payload)
	if err != nil {
		return normalized, "", "", fmt.Errorf("%w: failed to serialize fingerprint payload: %v", ErrUserQuotaMutationInvalidInput, err)
	}
	digest := sha256.Sum256(payloadBytes)
	fingerprint := hex.EncodeToString(digest[:])

	return normalized, fingerprint, metadataStr, nil
}

// FindUserQuotaMutationReceiptByEventKey retrieves a receipt by its unique business event key within a transaction.
func FindUserQuotaMutationReceiptByEventKey(tx *gorm.DB, businessEventKey string) (*UserQuotaMutationReceipt, error) {
	if tx == nil {
		return nil, gorm.ErrInvalidDB
	}
	key := strings.TrimSpace(businessEventKey)
	if key == "" {
		return nil, ErrUserQuotaMutationInvalidInput
	}
	var receipt UserQuotaMutationReceipt
	err := tx.Session(&gorm.Session{NewDB: true}).Where("business_event_key = ?", key).First(&receipt).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &receipt, nil
}

// MutateUserQuotaAuthoritative executes an atomic, authoritative quota modification within the caller's transaction tx.
// It enforces:
// 1. Exact replay idempotency on businessEventKey;
// 2. Conflict detection on key mismatch;
// 3. Row lock via lockForUpdate on the User;
// 4. Balance bounds and non-negative invariants;
// 5. Optimistic CAS on (quota, quota_version);
// 6. Insertion of immutable UserQuotaMutationReceipt.
func mutateUserQuotaAuthoritative(tx *gorm.DB, input UserQuotaMutationInput) (*UserQuotaMutationReceipt, bool, error) {
	if tx == nil {
		return nil, false, gorm.ErrInvalidDB
	}

	normalized, fingerprint, metadataStr, err := normalizeUserQuotaMutationInput(input)
	if err != nil {
		return nil, false, err
	}

	// 1. Idempotency pre-check: if receipt already exists
	existing, err := FindUserQuotaMutationReceiptByEventKey(tx, normalized.BusinessEventKey)
	if err != nil {
		return nil, false, err
	}
	if existing != nil {
		if existing.RequestFingerprint != fingerprint {
			return nil, false, ErrUserQuotaMutationConflict
		}
		if _, err := ensureQuotaProjectionObligation(tx, existing); err != nil {
			return nil, false, err
		}
		return existing, true, nil
	}

	// 2. Lock User row
	var user User
	if err := lockForUpdate(tx).Where("id = ?", normalized.UserID).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, fmt.Errorf("%w: user does not exist", ErrUserQuotaMutationInvalidInput)
		}
		return nil, false, err
	}

	// Double-check receipt existence after acquiring user lock (handles concurrent idempotent requests)
	existing, err = FindUserQuotaMutationReceiptByEventKey(tx, normalized.BusinessEventKey)
	if err != nil {
		return nil, false, err
	}
	if existing != nil {
		if existing.RequestFingerprint != fingerprint {
			return nil, false, ErrUserQuotaMutationConflict
		}
		if _, err := ensureQuotaProjectionObligation(tx, existing); err != nil {
			return nil, false, err
		}
		return existing, true, nil
	}

	writerState, err := requireDurableQuotaWriterEpoch(tx)
	if err != nil {
		return nil, false, err
	}

	if user.Status != common.UserStatusEnabled || !quotaMutationVersionValid(user.QuotaVersion) {
		return nil, false, fmt.Errorf("%w: user is disabled or has invalid quota version", ErrUserQuotaMutationInvalidInput)
	}

	// 3. Balance and bounds check
	targetQuota := int64(user.Quota) + normalized.Delta
	if normalized.Delta < 0 && targetQuota < 0 {
		return nil, false, ErrInsufficientUserQuota
	}
	if targetQuota < int64(common.MinQuota) || targetQuota > int64(common.MaxQuota) {
		return nil, false, fmt.Errorf("%w: target quota exceeds system bounds", ErrUserQuotaMutationInvalidInput)
	}
	if normalized.MaxQuotaExclusive > 0 && targetQuota >= normalized.MaxQuotaExclusive {
		return nil, false, ErrUserQuotaUpperBoundExceeded
	}

	newQuota := int(targetQuota)
	newQuotaVersion := user.QuotaVersion + 1

	// 4. Optimistic CAS update on User
	res := tx.Model(&User{}).
		Where("id = ? AND quota = ? AND quota_version = ?", user.Id, user.Quota, user.QuotaVersion).
		Updates(map[string]interface{}{
			"quota":         newQuota,
			"quota_version": newQuotaVersion,
		})
	if res.Error != nil {
		return nil, false, res.Error
	}
	if res.RowsAffected != 1 {
		return nil, false, ErrUserQuotaCASLost
	}

	// 5. Create immutable receipt
	receipt := &UserQuotaMutationReceipt{
		ReceiptVersion:     UserQuotaMutationReceiptVersion,
		WriterEpoch:        writerState.Epoch,
		MutationType:       normalized.MutationType,
		BusinessEventKey:   normalized.BusinessEventKey,
		RequestFingerprint: fingerprint,
		UserID:             user.Id,
		Delta:              normalized.Delta,
		QuotaBefore:        user.Quota,
		QuotaAfter:         newQuota,
		QuotaVersionBefore: user.QuotaVersion,
		QuotaVersionAfter:  newQuotaVersion,
		ReasonCode:         normalized.ReasonCode,
		OperatorUserID:     normalized.OperatorUserID,
		Metadata:           metadataStr,
		After: func() QuotaMutationUserSnapshot {
			after := user
			after.Quota = newQuota
			after.QuotaVersion = newQuotaVersion
			return quotaMutationUserSnapshot(&after)
		}(),
	}

	if err := userQuotaMutationReceiptCreateDB(tx).Create(receipt).Error; err != nil {
		return nil, false, err
	}
	if _, err := ensureQuotaProjectionObligation(tx, receipt); err != nil {
		return nil, false, err
	}

	return receipt, false, nil
}

// MutateUserQuotaAuthoritative executes an atomic, authoritative quota modification within the caller's transaction tx.
// It enforces:
// 1. Exact replay idempotency on businessEventKey;
// 2. Conflict detection on key mismatch;
// 3. Row lock via lockForUpdate on the User;
// 4. Balance bounds and non-negative invariants;
// 5. Optimistic CAS on (quota, quota_version);
// 6. Insertion of immutable UserQuotaMutationReceipt.
func MutateUserQuotaAuthoritative(tx *gorm.DB, input UserQuotaMutationInput) (*UserQuotaMutationReceipt, error) {
	receipt, _, err := mutateUserQuotaAuthoritative(tx, input)
	return receipt, err
}

// mutateBusinessUserQuotaIfAuthoritative lets persisted business actions keep
// their legacy/bridge transaction while switching only authoritative mode to
// immutable user-quota receipts.
func businessUserQuotaWriterMode(tx *gorm.DB) (QuotaWriterMode, error) {
	if tx == nil {
		return "", gorm.ErrInvalidDB
	}
	pool, err := tx.DB()
	if err != nil || DB == nil {
		return "", ErrUserQuotaBusinessSchemaUnavailable
	}
	mainPool, mainErr := DB.DB()
	if mainErr != nil || pool != mainPool {
		return "", ErrUserQuotaBusinessSchemaUnavailable
	}
	snapshot := userQuotaBusinessSchemaCapability.Load()
	binding := userQuotaBusinessSchemaBinding.Load()
	if snapshot == nil || binding == nil || snapshot.pool != pool || binding.pool != pool ||
		snapshot.dbGeneration == 0 || snapshot.dbGeneration != binding.generation {
		return "", ErrUserQuotaBusinessSchemaUnavailable
	}
	switch snapshot.state {
	case userQuotaBusinessSchemaLegacy:
		return QuotaWriterModeLegacy, nil
	case userQuotaBusinessSchemaReady:
		// Continue with the persisted mode below.
	default:
		return "", ErrUserQuotaBusinessSchemaUnavailable
	}
	state, err := GetQuotaWriterEpochState(tx)
	if err != nil {
		return "", err
	}
	mode := QuotaWriterMode(state.Mode)
	if mode != QuotaWriterModeLegacy && mode != QuotaWriterModeBridge && mode != QuotaWriterModeAuthoritative {
		return "", ErrDurableQuotaWriterModeDisabled
	}
	return mode, nil
}

func mutateBusinessUserQuotaIfAuthoritative(tx *gorm.DB, input UserQuotaMutationInput) (*UserQuotaMutationReceipt, bool, bool, error) {
	mode, err := businessUserQuotaWriterMode(tx)
	if err != nil {
		return nil, false, false, err
	}
	switch mode {
	case QuotaWriterModeAuthoritative:
		receipt, replayed, err := mutateUserQuotaAuthoritative(tx, input)
		return receipt, replayed, true, err
	case QuotaWriterModeLegacy, QuotaWriterModeBridge:
		return nil, false, false, nil
	default:
		return nil, false, false, ErrDurableQuotaWriterModeDisabled
	}
}

func replayBusinessUserQuotaMutationAuthoritative(tx *gorm.DB, input UserQuotaMutationInput) (*UserQuotaMutationReceipt, error) {
	normalized, fingerprint, _, err := normalizeUserQuotaMutationInput(input)
	if err != nil {
		return nil, err
	}
	receipt, err := FindUserQuotaMutationReceiptByEventKey(tx, normalized.BusinessEventKey)
	if err != nil {
		return nil, err
	}
	if receipt == nil || receipt.RequestFingerprint != fingerprint {
		return nil, ErrUserQuotaMutationConflict
	}
	return receipt, nil
}

func projectBusinessUserQuotaReceipt(db *gorm.DB, receipt *UserQuotaMutationReceipt) {
	if db == nil || receipt == nil {
		return
	}
	baseCtx := context.Background()
	if db.Statement != nil && db.Statement.Context != nil {
		baseCtx = db.Statement.Context
	}
	projectionCtx, cancel := context.WithTimeout(context.WithoutCancel(baseCtx), 2*time.Second)
	defer cancel()
	_ = ProjectQuotaMutationReceipt(projectionCtx, db.WithContext(projectionCtx), receipt)
}

// MutateUserQuota runs MutateUserQuotaAuthoritative inside a managed database transaction.
// Upon successful commit, it safely hydrates the Redis user quota projection (skipping replay).
func MutateUserQuota(db *gorm.DB, input UserQuotaMutationInput) (*UserQuotaMutationReceipt, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}

	var receipt *UserQuotaMutationReceipt
	err := db.Transaction(func(tx *gorm.DB) error {
		var err error
		receipt, _, err = mutateUserQuotaAuthoritative(tx, input)
		return err
	})
	if err != nil {
		return nil, err
	}

	// Redis/network work is strictly post-commit. A failure is recorded on the
	// durable obligation and never changes the acknowledged main-ledger result.
	projectBusinessUserQuotaReceipt(db, receipt)
	return receipt, nil
}
