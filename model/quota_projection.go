package model

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/logger"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	quotaProjectionObligationSchemaVersion = 2
	quotaProjectionDefaultLeaseSeconds     = 30
	quotaProjectionMaxAttempts             = 10
	quotaProjectionRetryBaseSeconds        = 5
	quotaProjectionRetryMaxSeconds         = 300
)

type QuotaProjectionObligationState string

const (
	QuotaProjectionObligationStatePending   QuotaProjectionObligationState = "pending"
	QuotaProjectionObligationStateClaimed   QuotaProjectionObligationState = "claimed"
	QuotaProjectionObligationStateRetryable QuotaProjectionObligationState = "retryable"
	QuotaProjectionObligationStateApplied   QuotaProjectionObligationState = "applied"
	QuotaProjectionObligationStateManual    QuotaProjectionObligationState = "manual"
)

const (
	quotaProjectionReceiptKindTask    = "task"
	quotaProjectionReceiptKindUser    = "user"
	quotaProjectionReceiptKindAccount = "account"
)

var (
	ErrQuotaProjectionManual        = errors.New("quota projection obligation requires manual repair")
	ErrQuotaProjectionNotDue        = errors.New("quota projection obligation is not due")
	ErrQuotaProjectionClaimed       = errors.New("quota projection obligation is already claimed")
	ErrQuotaProjectionLeaseLost     = errors.New("quota projection obligation lease lost")
	ErrQuotaProjectionRepairInvalid = errors.New("invalid quota projection repair command")
	ErrQuotaProjectionRepairCASLost = errors.New("quota projection repair compare-and-swap lost")
)

// QuotaProjectionObligation is committed in the same main-database transaction
// as its immutable receipt. Redis is only a replayable projection.
type QuotaProjectionObligation struct {
	ID                   int64  `json:"id" gorm:"primaryKey"`
	SchemaVersion        int    `json:"schema_version" gorm:"not null"`
	ReceiptKind          string `json:"receipt_kind" gorm:"type:varchar(16);not null;uniqueIndex:uidx_quota_projection_receipt,priority:1;uniqueIndex:uidx_quota_projection_event,priority:1"`
	ReceiptID            int64  `json:"receipt_id" gorm:"type:bigint;not null;uniqueIndex:uidx_quota_projection_receipt,priority:2"`
	EventKey             string `json:"event_key" gorm:"type:varchar(128);not null;uniqueIndex:uidx_quota_projection_event,priority:2"`
	WriterEpoch          int64  `json:"writer_epoch" gorm:"type:bigint"`
	UserID               int    `json:"user_id" gorm:"not null;index"`
	TokenID              int    `json:"token_id" gorm:"not null;index"`
	ExpectedUserVersion  int64  `json:"expected_user_version" gorm:"type:bigint;not null"`
	ExpectedTokenVersion int64  `json:"expected_token_version" gorm:"type:bigint;not null"`
	State                string `json:"state" gorm:"type:varchar(16);not null;index:idx_quota_projection_claim,priority:1"`
	LeaseOwner           string `json:"lease_owner" gorm:"type:varchar(96);not null"`
	LeaseUntil           int64  `json:"lease_until" gorm:"type:bigint;not null;index:idx_quota_projection_claim,priority:2"`
	NextAttemptAt        int64  `json:"next_attempt_at" gorm:"type:bigint;index:idx_quota_projection_claim,priority:3"`
	Fence                int64  `json:"fence" gorm:"type:bigint;not null"`
	LockVersion          int64  `json:"lock_version" gorm:"type:bigint"`
	Attempts             int    `json:"attempts" gorm:"not null"`
	LastError            string `json:"last_error" gorm:"type:text;not null"`
	RepairAuditCommandID string `json:"repair_audit_command_id" gorm:"type:varchar(96)"`
	RepairReason         string `json:"repair_reason" gorm:"type:varchar(191)"`
	RepairedAt           int64  `json:"repaired_at" gorm:"type:bigint"`
	CreatedAt            int64  `json:"created_at" gorm:"type:bigint;not null"`
	UpdatedAt            int64  `json:"updated_at" gorm:"type:bigint;not null"`
}

func (QuotaProjectionObligation) TableName() string { return "quota_projection_obligations" }

type quotaProjectionReceiptDescriptor struct {
	Kind                 string
	ID                   int64
	EventKey             string
	RequestID            string
	CorrelationKey       string
	WriterEpoch          int64
	UserID               int
	TokenID              int
	ExpectedUserVersion  int64
	ExpectedTokenVersion int64
}

type quotaProjectionReceipt interface {
	quotaProjectionDescriptor() quotaProjectionReceiptDescriptor
}

func (receipt *QuotaMutationReceipt) quotaProjectionDescriptor() quotaProjectionReceiptDescriptor {
	if receipt == nil {
		return quotaProjectionReceiptDescriptor{}
	}
	return quotaProjectionReceiptDescriptor{
		Kind: quotaProjectionReceiptKindTask, ID: receipt.ID, EventKey: receipt.MutationKey,
		RequestID: receipt.RequestID, CorrelationKey: receipt.MutationKey, WriterEpoch: receipt.WriterEpoch,
		UserID: receipt.UserID, TokenID: receipt.TokenID,
		ExpectedUserVersion: receipt.After.User.QuotaVersion, ExpectedTokenVersion: receipt.After.Token.QuotaVersion,
	}
}

func (receipt *UserQuotaMutationReceipt) quotaProjectionDescriptor() quotaProjectionReceiptDescriptor {
	if receipt == nil {
		return quotaProjectionReceiptDescriptor{}
	}
	return quotaProjectionReceiptDescriptor{
		Kind: quotaProjectionReceiptKindUser, ID: receipt.ID, EventKey: receipt.BusinessEventKey,
		CorrelationKey: receipt.BusinessEventKey, WriterEpoch: receipt.WriterEpoch, UserID: receipt.UserID,
		ExpectedUserVersion: receipt.QuotaVersionAfter,
	}
}

func ensureQuotaProjectionObligation(tx *gorm.DB, receipt quotaProjectionReceipt) (*QuotaProjectionObligation, error) {
	if tx == nil || receipt == nil {
		return nil, gorm.ErrInvalidDB
	}
	descriptor := receipt.quotaProjectionDescriptor()
	if descriptor.ID <= 0 || descriptor.EventKey == "" || descriptor.UserID <= 0 || descriptor.ExpectedUserVersion < 0 || descriptor.ExpectedTokenVersion < 0 {
		return nil, fmt.Errorf("invalid quota projection receipt descriptor")
	}
	now, err := taskRecoveryDBTimestamp(tx)
	if err != nil {
		return nil, err
	}
	state := QuotaProjectionObligationStatePending
	lastError := ""
	if descriptor.WriterEpoch <= 0 {
		state = QuotaProjectionObligationStateManual
		lastError = "immutable receipt does not contain a valid writer epoch"
	}
	candidate := &QuotaProjectionObligation{
		SchemaVersion: quotaProjectionObligationSchemaVersion,
		ReceiptKind:   descriptor.Kind, ReceiptID: descriptor.ID, EventKey: descriptor.EventKey,
		WriterEpoch: descriptor.WriterEpoch, UserID: descriptor.UserID, TokenID: descriptor.TokenID,
		ExpectedUserVersion: descriptor.ExpectedUserVersion, ExpectedTokenVersion: descriptor.ExpectedTokenVersion,
		State: string(state), LockVersion: 1, LastError: lastError, CreatedAt: now, UpdatedAt: now,
	}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(candidate).Error; err != nil {
		return nil, err
	}
	var stored QuotaProjectionObligation
	if err := tx.Where("receipt_kind = ? AND receipt_id = ?", descriptor.Kind, descriptor.ID).First(&stored).Error; err != nil {
		return nil, err
	}
	if stored.EventKey != descriptor.EventKey || stored.UserID != descriptor.UserID || stored.TokenID != descriptor.TokenID ||
		stored.WriterEpoch != descriptor.WriterEpoch || stored.ExpectedUserVersion != descriptor.ExpectedUserVersion || stored.ExpectedTokenVersion != descriptor.ExpectedTokenVersion {
		return nil, fmt.Errorf("quota projection obligation conflicts with receipt")
	}
	return &stored, nil
}

// InitializeQuotaProjectionObligationsWithDB safely initializes fields added by
// schema version 2. It does not claim, apply, or requeue any obligation.
func InitializeQuotaProjectionObligationsWithDB(db *gorm.DB) error {
	if db == nil {
		return gorm.ErrInvalidDB
	}
	if err := db.Model(&QuotaProjectionObligation{}).Where("lock_version IS NULL OR lock_version < ?", 1).Update("lock_version", 1).Error; err != nil {
		return err
	}
	return db.Model(&QuotaProjectionObligation{}).Where("schema_version IS NULL OR schema_version < ?", quotaProjectionObligationSchemaVersion).
		Update("schema_version", quotaProjectionObligationSchemaVersion).Error
}

func findQuotaProjectionObligation(db *gorm.DB, descriptor quotaProjectionReceiptDescriptor) (*QuotaProjectionObligation, error) {
	var obligation QuotaProjectionObligation
	if err := db.Where("receipt_kind = ? AND receipt_id = ?", descriptor.Kind, descriptor.ID).First(&obligation).Error; err != nil {
		return nil, err
	}
	return &obligation, nil
}

func claimQuotaProjectionObligation(db *gorm.DB, id int64, owner string, leaseSeconds int) (*QuotaProjectionObligation, bool, error) {
	var claimed *QuotaProjectionObligation
	var won bool
	var err error
	for attempt := 0; attempt < 4; attempt++ {
		claimed, won, err = claimQuotaProjectionObligationOnce(db, id, owner, leaseSeconds)
		if err == nil || !strings.Contains(strings.ToLower(err.Error()), "database is locked") {
			return claimed, won, err
		}
		time.Sleep(time.Duration(attempt+1) * time.Millisecond)
	}
	return claimed, won, err
}

func claimQuotaProjectionObligationOnce(db *gorm.DB, id int64, owner string, leaseSeconds int) (*QuotaProjectionObligation, bool, error) {
	if db == nil || id <= 0 || strings.TrimSpace(owner) == "" {
		return nil, false, gorm.ErrInvalidDB
	}
	if leaseSeconds <= 0 {
		leaseSeconds = quotaProjectionDefaultLeaseSeconds
	}
	var claimed QuotaProjectionObligation
	won := false
	err := db.Transaction(func(tx *gorm.DB) error {
		now, err := taskRecoveryDBTimestamp(tx)
		if err != nil {
			return err
		}
		var current QuotaProjectionObligation
		if err := lockForUpdate(tx).Where("id = ?", id).First(&current).Error; err != nil {
			return err
		}
		state := QuotaProjectionObligationState(current.State)
		if state == QuotaProjectionObligationStateApplied || state == QuotaProjectionObligationStateManual {
			claimed = current
			return nil
		}
		if state == QuotaProjectionObligationStateRetryable && current.NextAttemptAt > now {
			claimed = current
			return nil
		}
		if state == QuotaProjectionObligationStateClaimed && current.LeaseUntil > now {
			claimed = current
			return nil
		}
		if state != QuotaProjectionObligationStatePending && state != QuotaProjectionObligationStateRetryable && state != QuotaProjectionObligationStateClaimed {
			return fmt.Errorf("invalid quota projection obligation state %q", current.State)
		}
		result := tx.Model(&QuotaProjectionObligation{}).
			Where("id = ? AND fence = ? AND lock_version = ? AND state = ? AND lease_until = ? AND next_attempt_at = ?",
				current.ID, current.Fence, current.LockVersion, current.State, current.LeaseUntil, current.NextAttemptAt).
			Updates(map[string]interface{}{
				"state": string(QuotaProjectionObligationStateClaimed), "lease_owner": owner,
				"lease_until": now + int64(leaseSeconds), "next_attempt_at": 0,
				"fence": current.Fence + 1, "lock_version": current.LockVersion + 1,
				"attempts": current.Attempts + 1, "updated_at": now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return nil
		}
		if err := tx.Where("id = ?", id).First(&claimed).Error; err != nil {
			return err
		}
		won = true
		return nil
	})
	return &claimed, won, err
}

func quotaProjectionRetryDelaySeconds(attempts int) int64 {
	if attempts <= 0 {
		return quotaProjectionRetryBaseSeconds
	}
	delay := int64(quotaProjectionRetryBaseSeconds)
	for i := 1; i < attempts && delay < quotaProjectionRetryMaxSeconds; i++ {
		delay *= 2
		if delay > quotaProjectionRetryMaxSeconds {
			delay = quotaProjectionRetryMaxSeconds
		}
	}
	return delay
}

func finishQuotaProjectionObligation(db *gorm.DB, claimed *QuotaProjectionObligation, projectionErr error) error {
	if db == nil || claimed == nil || claimed.ID <= 0 {
		return gorm.ErrInvalidDB
	}
	now, err := taskRecoveryDBTimestamp(db)
	if err != nil {
		return err
	}
	state := QuotaProjectionObligationStateApplied
	lastError := ""
	nextAttemptAt := int64(0)
	if projectionErr != nil {
		state = QuotaProjectionObligationStateRetryable
		lastError = projectionErr.Error()
		if len(lastError) > 4096 {
			lastError = lastError[:4096]
		}
		if claimed.Attempts >= quotaProjectionMaxAttempts {
			state = QuotaProjectionObligationStateManual
		} else {
			nextAttemptAt = now + quotaProjectionRetryDelaySeconds(claimed.Attempts)
		}
	}
	result := db.Model(&QuotaProjectionObligation{}).
		Where("id = ? AND state = ? AND lease_owner = ? AND fence = ? AND lock_version = ?",
			claimed.ID, string(QuotaProjectionObligationStateClaimed), claimed.LeaseOwner, claimed.Fence, claimed.LockVersion).
		Updates(map[string]interface{}{
			"state": string(state), "lease_owner": "", "lease_until": 0, "next_attempt_at": nextAttemptAt,
			"last_error": lastError, "updated_at": now, "lock_version": claimed.LockVersion + 1,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrQuotaProjectionLeaseLost
	}
	return nil
}

func loadQuotaProjectionSnapshot(db *gorm.DB, obligation *QuotaProjectionObligation) (quotaProjectionReceiptDescriptor, QuotaMutationAccountSnapshot, error) {
	if obligation.ReceiptKind == quotaProjectionReceiptKindTask {
		var receipt QuotaMutationReceipt
		if err := db.Where("id = ?", obligation.ReceiptID).First(&receipt).Error; err != nil {
			return quotaProjectionReceiptDescriptor{}, QuotaMutationAccountSnapshot{}, err
		}
		descriptor := receipt.quotaProjectionDescriptor()
		if receipt.After.User.ID != receipt.UserID || receipt.After.Token.ID != receipt.TokenID ||
			receipt.After.User.QuotaVersion != obligation.ExpectedUserVersion || receipt.After.Token.QuotaVersion != obligation.ExpectedTokenVersion {
			return descriptor, QuotaMutationAccountSnapshot{}, fmt.Errorf("task receipt after snapshot does not match projection identity")
		}
		return descriptor, receipt.After, nil
	}
	if obligation.ReceiptKind == quotaProjectionReceiptKindAccount {
		var receipt AccountQuotaMutationReceipt
		if err := db.Where("id = ?", obligation.ReceiptID).First(&receipt).Error; err != nil {
			return quotaProjectionReceiptDescriptor{}, QuotaMutationAccountSnapshot{}, err
		}
		descriptor := receipt.quotaProjectionDescriptor()
		if receipt.After.User.ID != receipt.UserID || receipt.After.Token.ID != receipt.TokenID ||
			receipt.After.User.QuotaVersion != obligation.ExpectedUserVersion || receipt.After.Token.QuotaVersion != obligation.ExpectedTokenVersion {
			return descriptor, QuotaMutationAccountSnapshot{}, fmt.Errorf("account receipt after snapshot does not match projection identity")
		}
		return descriptor, receipt.After, nil
	}
	if obligation.ReceiptKind == quotaProjectionReceiptKindUser {
		var receipt UserQuotaMutationReceipt
		if err := db.Where("id = ?", obligation.ReceiptID).First(&receipt).Error; err != nil {
			return quotaProjectionReceiptDescriptor{}, QuotaMutationAccountSnapshot{}, err
		}
		descriptor := receipt.quotaProjectionDescriptor()
		if receipt.After.ID != receipt.UserID || receipt.After.Quota != receipt.QuotaAfter || receipt.After.QuotaVersion != receipt.QuotaVersionAfter {
			return descriptor, QuotaMutationAccountSnapshot{}, fmt.Errorf("user receipt after snapshot is missing or inconsistent")
		}
		return descriptor, QuotaMutationAccountSnapshot{User: receipt.After}, nil
	}
	return quotaProjectionReceiptDescriptor{}, QuotaMutationAccountSnapshot{}, fmt.Errorf("unknown quota projection receipt kind %q", obligation.ReceiptKind)
}

func invalidateQuotaProjectionCaches(userID int, tokenKey string) {
	if userID > 0 {
		_ = InvalidateUserQuotaCache(userID)
	}
	if tokenKey != "" {
		_ = InvalidateTokenQuotaCache(tokenKey)
	}
}

func applyQuotaProjectionObligation(ctx context.Context, db *gorm.DB, obligation *QuotaProjectionObligation) (quotaProjectionReceiptDescriptor, error) {
	descriptor, snapshot, err := loadQuotaProjectionSnapshot(db, obligation)
	if err != nil {
		return descriptor, err
	}
	if descriptor.WriterEpoch != obligation.WriterEpoch || descriptor.UserID != obligation.UserID || descriptor.TokenID != obligation.TokenID ||
		descriptor.ExpectedUserVersion != obligation.ExpectedUserVersion || descriptor.ExpectedTokenVersion != obligation.ExpectedTokenVersion {
		return descriptor, fmt.Errorf("quota projection receipt no longer matches obligation")
	}
	if _, err := requireQuotaProjectionEpoch(db, obligation.WriterEpoch); err != nil {
		return descriptor, err
	}
	if !common.RedisEnabled || common.RDB == nil {
		return descriptor, fmt.Errorf("redis projection unavailable")
	}

	tokenKey := ""
	if obligation.TokenID > 0 {
		var token Token
		if err := db.Select("id", "key").Where("id = ?", obligation.TokenID).First(&token).Error; err != nil {
			invalidateQuotaProjectionCaches(obligation.UserID, "")
			return descriptor, fmt.Errorf("load token projection key: %w", err)
		}
		tokenKey = token.Key
	}
	if err := hydrateUserQuotaCacheRedisAtEpoch(snapshot.User.ID, snapshot.User.Quota, snapshot.User.QuotaVersion, obligation.WriterEpoch); err != nil {
		invalidateQuotaProjectionCaches(obligation.UserID, tokenKey)
		return descriptor, fmt.Errorf("hydrate user quota projection: %w", err)
	}
	if obligation.TokenID > 0 {
		if err := hydrateTokenQuotaCacheRedisAtEpoch(tokenKey, snapshot.Token.ID, snapshot.Token.RemainQuota, snapshot.Token.UsedQuota, snapshot.Token.QuotaVersion, obligation.WriterEpoch); err != nil {
			invalidateQuotaProjectionCaches(obligation.UserID, tokenKey)
			return descriptor, fmt.Errorf("hydrate token quota projection: %w", err)
		}
	}
	return descriptor, nil
}

func quotaProjectionWarningCorrelation(descriptor quotaProjectionReceiptDescriptor, eventKey string) string {
	if strings.TrimSpace(descriptor.RequestID) != "" {
		return "request_id=" + descriptor.RequestID
	}
	correlationKey := strings.TrimSpace(descriptor.CorrelationKey)
	if correlationKey == "" {
		correlationKey = strings.TrimSpace(eventKey)
	}
	return "correlation_key=" + correlationKey
}

func quotaProjectionObligationUnavailableWarning(descriptor quotaProjectionReceiptDescriptor, err error) string {
	return fmt.Sprintf("quota projection obligation unavailable: %s event=%s receipt=%s/%d error=%v",
		quotaProjectionWarningCorrelation(descriptor, descriptor.EventKey), descriptor.EventKey, descriptor.Kind, descriptor.ID, err)
}

func projectClaimedQuotaObligation(ctx context.Context, db *gorm.DB, claimed *QuotaProjectionObligation) error {
	descriptor, projectionErr := applyQuotaProjectionObligation(ctx, db, claimed)
	finishErr := finishQuotaProjectionObligation(db, claimed, projectionErr)
	correlation := quotaProjectionWarningCorrelation(descriptor, claimed.EventKey)
	if projectionErr != nil {
		logger.LogWarn(ctx, fmt.Sprintf("quota projection failed: %s event=%s receipt=%s/%d obligation=%d error=%v", correlation, claimed.EventKey, claimed.ReceiptKind, claimed.ReceiptID, claimed.ID, projectionErr))
	}
	if finishErr != nil {
		logger.LogWarn(ctx, fmt.Sprintf("quota projection obligation update failed: %s event=%s receipt=%s/%d obligation=%d error=%v", correlation, claimed.EventKey, claimed.ReceiptKind, claimed.ReceiptID, claimed.ID, finishErr))
		return errors.Join(projectionErr, finishErr)
	}
	return projectionErr
}

// ProjectQuotaMutationReceipt is post-commit only. Projection failure never
// changes the acknowledged main-ledger result.
func ProjectQuotaMutationReceipt(ctx context.Context, db *gorm.DB, receipt quotaProjectionReceipt) error {
	if db == nil || receipt == nil {
		return gorm.ErrInvalidDB
	}
	descriptor := receipt.quotaProjectionDescriptor()
	obligation, err := findQuotaProjectionObligation(db, descriptor)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = db.Transaction(func(tx *gorm.DB) error {
			_, ensureErr := ensureQuotaProjectionObligation(tx, receipt)
			return ensureErr
		})
		if err == nil {
			obligation, err = findQuotaProjectionObligation(db, descriptor)
		}
	}
	if err != nil {
		logger.LogWarn(ctx, quotaProjectionObligationUnavailableWarning(descriptor, err))
		return err
	}
	switch QuotaProjectionObligationState(obligation.State) {
	case QuotaProjectionObligationStateApplied:
		return nil
	case QuotaProjectionObligationStateManual:
		return ErrQuotaProjectionManual
	case QuotaProjectionObligationStateRetryable:
		now, nowErr := taskRecoveryDBTimestamp(db)
		if nowErr != nil {
			return nowErr
		}
		if obligation.NextAttemptAt > now {
			return ErrQuotaProjectionNotDue
		}
	case QuotaProjectionObligationStateClaimed:
		now, nowErr := taskRecoveryDBTimestamp(db)
		if nowErr != nil {
			return nowErr
		}
		if obligation.LeaseUntil > now {
			return ErrQuotaProjectionClaimed
		}
	}
	if _, err := requireQuotaProjectionEpoch(db, obligation.WriterEpoch); err != nil {
		return err
	}
	owner := fmt.Sprintf("direct-%d-%d", time.Now().UnixNano(), obligation.ID)
	claimed, won, err := claimQuotaProjectionObligation(db, obligation.ID, owner, quotaProjectionDefaultLeaseSeconds)
	if err != nil {
		return err
	}
	if !won {
		return ErrQuotaProjectionNotDue
	}
	return projectClaimedQuotaObligation(ctx, db, claimed)
}

// RunQuotaProjectionObligations replays due persisted obligations. The existing
// distributed task-recovery obligation gate defaults off.
func RunQuotaProjectionObligations(ctx context.Context, db *gorm.DB, workerID string, limit int) (int, error) {
	if !QuotaProjectionObligationRecoveryEnabled() {
		return 0, nil
	}
	if db == nil || strings.TrimSpace(workerID) == "" {
		return 0, gorm.ErrInvalidDB
	}
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	now, err := taskRecoveryDBTimestamp(db)
	if err != nil {
		return 0, err
	}
	var candidates []QuotaProjectionObligation
	if err := db.Where("state = ? OR (state = ? AND next_attempt_at <= ?) OR (state = ? AND lease_until <= ?)",
		string(QuotaProjectionObligationStatePending), string(QuotaProjectionObligationStateRetryable), now,
		string(QuotaProjectionObligationStateClaimed), now).
		Order("id ASC").Limit(limit).Find(&candidates).Error; err != nil {
		return 0, err
	}
	processed := 0
	var passErrors []error
	for i := range candidates {
		if err := ctx.Err(); err != nil {
			return processed, errors.Join(append(passErrors, err)...)
		}
		if _, err := requireQuotaProjectionEpoch(db, candidates[i].WriterEpoch); err != nil {
			passErrors = append(passErrors, fmt.Errorf("obligation %d mode/epoch: %w", candidates[i].ID, err))
			continue
		}
		claimed, won, claimErr := claimQuotaProjectionObligation(db, candidates[i].ID, workerID, quotaProjectionDefaultLeaseSeconds)
		if claimErr != nil {
			passErrors = append(passErrors, claimErr)
			continue
		}
		if !won {
			continue
		}
		processed++
		if err := projectClaimedQuotaObligation(ctx, db, claimed); err != nil {
			passErrors = append(passErrors, err)
		}
	}
	return processed, errors.Join(passErrors...)
}

type QuotaProjectionRepairInput struct {
	ObligationID   int64
	AuditCommandID string
	Reason         string
}

func validQuotaProjectionAuditText(value string, max int) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > max {
		return false
	}
	for _, c := range value {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '.' || c == '_' || c == '-' || c == ' ' || c == ':' {
			continue
		}
		return false
	}
	return true
}

// RequeueQuotaProjectionObligation is an explicit, audited manual repair. It
// validates the immutable receipt before changing only the obligation state.
func RequeueQuotaProjectionObligation(db *gorm.DB, input QuotaProjectionRepairInput) (*QuotaProjectionObligation, error) {
	input.AuditCommandID = strings.TrimSpace(input.AuditCommandID)
	input.Reason = strings.TrimSpace(input.Reason)
	if db == nil || input.ObligationID <= 0 || !validQuotaProjectionAuditText(input.AuditCommandID, 96) || !validQuotaProjectionAuditText(input.Reason, 191) {
		return nil, ErrQuotaProjectionRepairInvalid
	}
	var repaired QuotaProjectionObligation
	err := db.Transaction(func(tx *gorm.DB) error {
		now, err := taskRecoveryDBTimestamp(tx)
		if err != nil {
			return err
		}
		var current QuotaProjectionObligation
		if err := lockForUpdate(tx).Where("id = ?", input.ObligationID).First(&current).Error; err != nil {
			return err
		}
		if QuotaProjectionObligationState(current.State) != QuotaProjectionObligationStateManual {
			return ErrQuotaProjectionRepairInvalid
		}
		descriptor, _, err := loadQuotaProjectionSnapshot(tx, &current)
		if err != nil {
			return err
		}
		if descriptor.WriterEpoch != current.WriterEpoch || descriptor.UserID != current.UserID || descriptor.TokenID != current.TokenID ||
			descriptor.ExpectedUserVersion != current.ExpectedUserVersion || descriptor.ExpectedTokenVersion != current.ExpectedTokenVersion {
			return ErrQuotaProjectionRepairInvalid
		}
		if _, err := requireQuotaProjectionEpoch(tx, current.WriterEpoch); err != nil {
			return err
		}
		result := tx.Model(&QuotaProjectionObligation{}).
			Where("id = ? AND state = ? AND fence = ? AND lock_version = ?", current.ID, string(QuotaProjectionObligationStateManual), current.Fence, current.LockVersion).
			Updates(map[string]interface{}{
				"state": string(QuotaProjectionObligationStateRetryable), "lease_owner": "", "lease_until": 0,
				"next_attempt_at": now, "fence": current.Fence + 1, "lock_version": current.LockVersion + 1,
				"attempts": 0, "last_error": "", "repair_audit_command_id": input.AuditCommandID,
				"repair_reason": input.Reason, "repaired_at": now, "updated_at": now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrQuotaProjectionRepairCASLost
		}
		return tx.First(&repaired, current.ID).Error
	})
	return &repaired, err
}
