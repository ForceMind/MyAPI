package model

import (
	"crypto/sha256"
	"database/sql/driver"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"

	"github.com/ForceMind/MyAPI/common"
	"gorm.io/gorm"
)

const (
	quotaMutationReceiptVersion             = 1
	quotaMutationFingerprintVersion         = 1
	quotaMutationMaxOtherRatios             = 64
	quotaMutationMaxRatioNameLength         = 64
	quotaMutationMaxBillingJSONLength       = 16 * 1024
	quotaMutationMaxOriginModelLength       = 191
	quotaMutationMaxInt64             int64 = 1<<63 - 1
	quotaMutationReceiptCreateSetting       = "quota-mutation:receipt-create"
)

var (
	ErrTaskQuotaReservationInvalidInput      = errors.New("invalid task quota reservation input")
	ErrTaskQuotaReservationConflict          = errors.New("task quota reservation conflicts with the applied receipt")
	ErrTaskQuotaReservationCASLost           = errors.New("task quota reservation compare-and-swap lost")
	ErrTaskQuotaReservationIneligible        = errors.New("task quota reservation subject is not eligible")
	ErrTaskQuotaReservationInsufficientQuota = errors.New("insufficient quota for task reservation")
	ErrTaskQuotaReservationNotFound          = errors.New("task quota reservation receipt not found")
	ErrQuotaMutationReceiptImmutable         = errors.New("quota mutation receipt is immutable")
)

type quotaMutationReceiptCreateMarkerType struct {
	value byte
}

var quotaMutationReceiptCreateMarker = &quotaMutationReceiptCreateMarkerType{}

// TaskQuotaReservationInput is the complete gate-off T1 authorization. Quota is
// the positive amount to reserve; the authoritative TaskBillingEvent delta is
// derived internally as -Quota, so callers cannot supply an arbitrary credit.
type TaskQuotaReservationInput struct {
	OperationID              int64
	UserID                   int
	TokenID                  int
	ChannelID                int
	ExpectedOperationVersion int64
	Quota                    int64
	BillingSource            string
	SubscriptionID           int
	BillingContext           TaskBillingContext
}

// TaskQuotaBillingContext is the immutable, JSON-backed copy of the task price
// snapshot stored in a receipt. It deliberately has the same fields as the
// submission TaskBillingContext while owning its database encoding contract.
type TaskQuotaBillingContext TaskBillingContext

func (snapshot *TaskQuotaBillingContext) Scan(value interface{}) error {
	*snapshot = TaskQuotaBillingContext{}
	data, err := taskRecoveryTextValue(value)
	if err != nil || len(data) == 0 {
		return err
	}
	return common.Unmarshal(data, snapshot)
}

func (snapshot TaskQuotaBillingContext) Value() (driver.Value, error) {
	data, err := common.Marshal(snapshot)
	if err != nil {
		return nil, err
	}
	return string(data), nil
}

// QuotaMutationUserSnapshot excludes credentials and personal profile data. It
// contains the complete account state needed to audit or project this quota
// mutation without consulting a mutable cache.
type QuotaMutationUserSnapshot struct {
	ID            int    `json:"id"`
	Status        int    `json:"status"`
	Role          int    `json:"role"`
	Quota         int    `json:"quota"`
	UsedQuota     int    `json:"used_quota"`
	RequestCount  int    `json:"request_count"`
	QuotaVersion  int64  `json:"quota_version"`
	Group         string `json:"group"`
	AccountTierID string `json:"account_tier_id"`
}

// QuotaMutationTokenSnapshot omits Key, AutoGroups and IP restrictions. Those
// values are credentials or routing policy inputs, not quota mutation state.
type QuotaMutationTokenSnapshot struct {
	ID                 int    `json:"id"`
	UserID             int    `json:"user_id"`
	Status             int    `json:"status"`
	CreatedTime        int64  `json:"created_time"`
	AccessedTime       int64  `json:"accessed_time"`
	ExpiredTime        int64  `json:"expired_time"`
	RemainQuota        int    `json:"remain_quota"`
	QuotaVersion       int64  `json:"quota_version"`
	UnlimitedQuota     bool   `json:"unlimited_quota"`
	UsedQuota          int    `json:"used_quota"`
	Group              string `json:"group"`
	AccessProfileID    string `json:"access_profile_id"`
	ModelLimitsEnabled bool   `json:"model_limits_enabled"`
	ModelLimits        string `json:"model_limits"`
	CrossGroupRetry    bool   `json:"cross_group_retry"`
}

type QuotaMutationSubscriptionSnapshot struct {
	ID                  int    `json:"id"`
	UserID              int    `json:"user_id"`
	PlanID              int    `json:"plan_id"`
	AmountTotal         int64  `json:"amount_total"`
	AmountUsed          int64  `json:"amount_used"`
	QuotaVersion        int64  `json:"quota_version"`
	StartTime           int64  `json:"start_time"`
	EndTime             int64  `json:"end_time"`
	Status              string `json:"status"`
	Source              string `json:"source"`
	LastResetTime       int64  `json:"last_reset_time"`
	NextResetTime       int64  `json:"next_reset_time"`
	UpgradeGroup        string `json:"upgrade_group"`
	PrevUserGroup       string `json:"prev_user_group"`
	DowngradeGroup      string `json:"downgrade_group"`
	AllowWalletOverflow bool   `json:"allow_wallet_overflow"`
	CreatedAt           int64  `json:"created_at"`
	UpdatedAt           int64  `json:"updated_at"`
}

type QuotaMutationAccountSnapshot struct {
	User         QuotaMutationUserSnapshot          `json:"user"`
	Token        QuotaMutationTokenSnapshot         `json:"token"`
	Subscription *QuotaMutationSubscriptionSnapshot `json:"subscription,omitempty"`
}

func (snapshot *QuotaMutationAccountSnapshot) Scan(value interface{}) error {
	*snapshot = QuotaMutationAccountSnapshot{}
	data, err := taskRecoveryTextValue(value)
	if err != nil || len(data) == 0 {
		return err
	}
	return common.Unmarshal(data, snapshot)
}

func (snapshot QuotaMutationAccountSnapshot) Value() (driver.Value, error) {
	data, err := common.Marshal(snapshot)
	if err != nil {
		return nil, err
	}
	return string(data), nil
}

// QuotaMutationReceipt is the immutable, auditable application receipt for one
// canonical Task reserve event. MutationKey is the TaskBillingEvent EventKey;
// RequestFingerprint covers every TaskQuotaReservationInput field, while
// OperationRequestFingerprint binds the receipt to the durable T0 request.
type QuotaMutationReceipt struct {
	ID                          int64                        `json:"id" gorm:"primaryKey"`
	ReceiptVersion              int                          `json:"receipt_version" gorm:"not null;<-:create"`
	MutationType                string                       `json:"mutation_type" gorm:"type:varchar(32);not null;default:'reserve';uniqueIndex:uidx_quota_mutation_receipt_op_type,priority:2;<-:create"`
	MutationKey                 string                       `json:"mutation_key" gorm:"type:varchar(128);not null;uniqueIndex:uidx_quota_mutation_receipt_key;<-:create"`
	RequestFingerprint          string                       `json:"request_fingerprint" gorm:"type:char(64);not null;<-:create"`
	OperationRequestFingerprint string                       `json:"operation_request_fingerprint" gorm:"type:char(64);not null;<-:create"`
	OperationID                 int64                        `json:"operation_id" gorm:"not null;uniqueIndex:uidx_quota_mutation_receipt_op_type,priority:1;<-:create"`
	OperationPublicID           string                       `json:"operation_public_id" gorm:"type:varchar(48);not null;<-:create"`
	ExpectedOperationVersion    int64                        `json:"expected_operation_version" gorm:"type:bigint;not null;<-:create"`
	OperationVersionBefore      int64                        `json:"operation_version_before" gorm:"type:bigint;not null;<-:create"`
	OperationVersionAfter       int64                        `json:"operation_version_after" gorm:"type:bigint;not null;<-:create"`
	BillingEventID              string                       `json:"billing_event_id" gorm:"type:varchar(64);not null;uniqueIndex:uidx_quota_mutation_receipt_event;<-:create"`
	BillingEventKey             string                       `json:"billing_event_key" gorm:"type:varchar(128);not null;<-:create"`
	BillingEventVersion         int64                        `json:"billing_event_version" gorm:"type:bigint;not null;<-:create"`
	UserID                      int                          `json:"user_id" gorm:"not null;index:idx_quota_mutation_receipt_lookup,priority:1;<-:create"`
	TokenID                     int                          `json:"token_id" gorm:"not null;index:idx_quota_mutation_receipt_lookup,priority:2;<-:create"`
	ChannelID                   int                          `json:"channel_id" gorm:"not null;<-:create"`
	BillingSource               string                       `json:"billing_source" gorm:"type:varchar(32);not null;<-:create"`
	SubscriptionID              int                          `json:"subscription_id,omitempty" gorm:"index;<-:create"`
	Quota                       int64                        `json:"quota" gorm:"type:bigint;not null;<-:create"`
	BillingContext              TaskQuotaBillingContext      `json:"billing_context" gorm:"type:text;not null;<-:create"`
	Before                      QuotaMutationAccountSnapshot `json:"before" gorm:"column:before_snapshot;type:text;not null;<-:create"`
	After                       QuotaMutationAccountSnapshot `json:"after" gorm:"column:after_snapshot;type:text;not null;<-:create"`
	CreatedAt                   int64                        `json:"created_at" gorm:"type:bigint;not null;index;<-:create"`
}

func (QuotaMutationReceipt) TableName() string {
	return "quota_mutation_receipts"
}

func (receipt *QuotaMutationReceipt) BeforeCreate(tx *gorm.DB) error {
	if tx == nil {
		return gorm.ErrInvalidDB
	}
	if !quotaMutationReceiptCreateAllowed(tx) {
		return fmt.Errorf("%w: receipts may only be created by the atomic quota mutation boundary", ErrQuotaMutationReceiptImmutable)
	}
	if receipt == nil || receipt.ID != 0 || receipt.CreatedAt != 0 {
		return fmt.Errorf("%w: new receipt carries database-managed fields", ErrTaskQuotaReservationInvalidInput)
	}
	if err := validateQuotaMutationReceiptShape(receipt); err != nil {
		return err
	}
	createdAt, err := taskRecoveryDBTimestamp(tx)
	if err != nil {
		return err
	}
	receipt.CreatedAt = createdAt
	return nil
}

func (*QuotaMutationReceipt) BeforeUpdate(_ *gorm.DB) error {
	return ErrQuotaMutationReceiptImmutable
}

func (*QuotaMutationReceipt) BeforeDelete(_ *gorm.DB) error {
	return ErrQuotaMutationReceiptImmutable
}

func quotaMutationReceiptCreateAllowed(tx *gorm.DB) bool {
	if tx == nil {
		return false
	}
	value, ok := tx.Get(quotaMutationReceiptCreateSetting)
	return ok && value == quotaMutationReceiptCreateMarker
}

func quotaMutationReceiptCreateDB(tx *gorm.DB) *gorm.DB {
	return tx.Session(&gorm.Session{NewDB: true}).Set(quotaMutationReceiptCreateSetting, quotaMutationReceiptCreateMarker)
}

// FindTaskQuotaReservation performs only a main-database read. Absence is an
// explicit error so a caller resolving an unknown commit cannot treat an
// unreadable or missing obligation as authorization to refund or retry blindly.
func FindTaskQuotaReservation(tx *gorm.DB, operationID int64, userID, tokenID int) (*QuotaMutationReceipt, error) {
	receipt, err := findTaskQuotaReservation(tx, operationID, userID, tokenID)
	if err != nil {
		return nil, err
	}
	if receipt == nil {
		return nil, ErrTaskQuotaReservationNotFound
	}
	return receipt, nil
}

func findTaskQuotaReservation(tx *gorm.DB, operationID int64, userID, tokenID int) (*QuotaMutationReceipt, error) {
	return findTaskQuotaReceiptByType(tx, operationID, string(TaskBillingEventTypeReserve), userID, tokenID)
}

// FindTaskQuotaReceipt performs only a main-database read for a specific receipt type.
func FindTaskQuotaReceipt(tx *gorm.DB, operationID int64, mutationType string, userID, tokenID int) (*QuotaMutationReceipt, error) {
	receipt, err := findTaskQuotaReceiptByType(tx, operationID, mutationType, userID, tokenID)
	if err != nil {
		return nil, err
	}
	if receipt == nil {
		return nil, ErrTaskQuotaReservationNotFound
	}
	return receipt, nil
}

func findTaskQuotaReceiptByType(tx *gorm.DB, operationID int64, mutationType string, userID, tokenID int) (*QuotaMutationReceipt, error) {
	if tx == nil {
		return nil, gorm.ErrInvalidDB
	}
	if operationID <= 0 || userID <= 0 || tokenID <= 0 || mutationType == "" {
		return nil, ErrTaskQuotaReservationInvalidInput
	}
	var receipt QuotaMutationReceipt
	err := tx.Session(&gorm.Session{NewDB: true}).Where("operation_id = ? AND mutation_type = ?", operationID, mutationType).First(&receipt).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if receipt.UserID != userID || receipt.TokenID != tokenID {
		return nil, ErrTaskQuotaReservationConflict
	}
	if err := validateStoredQuotaMutationReceipt(tx, &receipt); err != nil {
		return nil, err
	}
	return &receipt, nil
}

// ReserveTaskQuota applies gate-off T1 atomically. It has no cache, LOG_DB,
// network or global DB side effects and intentionally has no production caller.
func ReserveTaskQuota(tx *gorm.DB, input TaskQuotaReservationInput) (*QuotaMutationReceipt, error) {
	if tx == nil {
		return nil, gorm.ErrInvalidDB
	}
	normalized, fingerprint, err := normalizeTaskQuotaReservationInput(input)
	if err != nil {
		return nil, err
	}
	if existing, err := findTaskQuotaReservation(tx, normalized.OperationID, normalized.UserID, normalized.TokenID); err != nil {
		return nil, err
	} else if existing != nil {
		if existing.RequestFingerprint != fingerprint {
			return nil, ErrTaskQuotaReservationConflict
		}
		return existing, nil
	}

	var result *QuotaMutationReceipt
	err = taskRecoveryAtomicTransaction(tx, "task_quota_reserve", func(writeDB *gorm.DB) error {
		databaseNow, err := taskRecoveryDBTimestamp(writeDB)
		if err != nil {
			return err
		}

		var user User
		if err := lockForUpdate(writeDB).Where("id = ?", normalized.UserID).First(&user).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("%w: user does not exist", ErrTaskQuotaReservationIneligible)
			}
			return err
		}
		if user.Status != common.UserStatusEnabled || !quotaMutationVersionValid(user.QuotaVersion) {
			return fmt.Errorf("%w: user is disabled or has an invalid quota version", ErrTaskQuotaReservationIneligible)
		}
		if !quotaMutationInt32Value(user.Quota) || !quotaMutationInt32Value(user.UsedQuota) {
			return fmt.Errorf("%w: user quota state exceeds its database boundary", ErrTaskQuotaReservationIneligible)
		}

		var token Token
		if err := lockForUpdate(writeDB).Where("id = ?", normalized.TokenID).First(&token).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("%w: token does not exist", ErrTaskQuotaReservationIneligible)
			}
			return err
		}
		if token.UserId != normalized.UserID || token.Status != common.TokenStatusEnabled ||
			(token.ExpiredTime != -1 && token.ExpiredTime <= databaseNow) || !quotaMutationVersionValid(token.QuotaVersion) {
			return fmt.Errorf("%w: token owner, status, expiry, or quota version is invalid", ErrTaskQuotaReservationIneligible)
		}
		if !quotaMutationInt32Value(token.RemainQuota) || !quotaMutationInt32Value(token.UsedQuota) {
			return fmt.Errorf("%w: token quota state exceeds its database boundary", ErrTaskQuotaReservationIneligible)
		}

		var subscription *UserSubscription
		if normalized.BillingSource == "subscription" {
			var locked UserSubscription
			if err := lockForUpdate(writeDB).Where("id = ?", normalized.SubscriptionID).First(&locked).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return fmt.Errorf("%w: subscription does not exist", ErrTaskQuotaReservationIneligible)
				}
				return err
			}
			if locked.UserId != normalized.UserID || locked.Status != "active" || locked.StartTime > databaseNow ||
				locked.EndTime <= databaseNow || !quotaMutationVersionValid(locked.QuotaVersion) ||
				locked.AmountTotal < 0 || locked.AmountUsed < 0 ||
				(locked.NextResetTime > 0 && locked.NextResetTime <= databaseNow) {
				return fmt.Errorf("%w: subscription owner, lifecycle, reset boundary, or quota version is invalid", ErrTaskQuotaReservationIneligible)
			}
			if locked.AmountTotal > 0 && locked.AmountUsed > locked.AmountTotal {
				return fmt.Errorf("%w: subscription usage exceeds its finite total", ErrTaskQuotaReservationIneligible)
			}
			subscription = &locked
		}

		var operation TaskSubmissionOperation
		if err := lockForUpdate(writeDB).Where("id = ?", normalized.OperationID).First(&operation).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("%w: operation does not exist", ErrTaskQuotaReservationIneligible)
			}
			return err
		}
		if existing, err := findTaskQuotaReservation(writeDB, normalized.OperationID, normalized.UserID, normalized.TokenID); err != nil {
			return err
		} else if existing != nil {
			if existing.RequestFingerprint != fingerprint {
				return ErrTaskQuotaReservationConflict
			}
			result = existing
			return nil
		}
		if err := validateStoredTaskSubmissionOperation(writeDB, &operation); err != nil {
			return err
		}
		if operation.UserID != normalized.UserID || operation.TokenID != normalized.TokenID ||
			operation.Status != TaskSubmissionOperationStatusPrepared || operation.TaskID != nil ||
			operation.LockVersion != normalized.ExpectedOperationVersion || operation.LockVersion == quotaMutationMaxInt64 {
			return ErrTaskQuotaReservationCASLost
		}

		var attempt TaskSubmissionAttempt
		if err := lockForUpdate(writeDB).Where("operation_id = ?", operation.ID).First(&attempt).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("%w: operation has no submission attempt", ErrTaskQuotaReservationIneligible)
			}
			return err
		}
		if err := validateStoredTaskSubmissionAttempt(writeDB, &attempt); err != nil {
			return err
		}
		if attempt.ChannelID != normalized.ChannelID || attempt.Status != TaskSubmissionAttemptStatusPrepared {
			return fmt.Errorf("%w: submission attempt is not the requested prepared channel", ErrTaskQuotaReservationIneligible)
		}

		eventKey := "task:" + operation.PublicID + ":" + string(TaskBillingEventTypeReserve) + ":v1"
		if existingEvent, err := loadTaskBillingEventByEventKeyForReplay(writeDB, eventKey); err != nil {
			return err
		} else if existingEvent != nil {
			return fmt.Errorf("%w: reserve event exists without its receipt", ErrTaskQuotaReservationConflict)
		}

		before := quotaMutationSnapshot(&user, &token, subscription)
		after, err := quotaMutationReservationAfter(before, normalized, databaseNow)
		if err != nil {
			return err
		}
		if err := applyTaskQuotaReservationBalances(writeDB, before, after, normalized); err != nil {
			return err
		}

		operationID := operation.ID
		event, err := CreateOrLoadTaskBillingEvent(writeDB, &TaskBillingEvent{
			OperationID:    &operationID,
			EventType:      TaskBillingEventTypeReserve,
			UserID:         normalized.UserID,
			TokenID:        normalized.TokenID,
			ChannelID:      normalized.ChannelID,
			BillingSource:  normalized.BillingSource,
			SubscriptionID: normalized.SubscriptionID,
			QuotaDelta:     -normalized.Quota,
			ReasonCode:     "task_t1_reserve",
		})
		if err != nil {
			return err
		}
		applied, err := applySynchronousTaskQuotaReserveEvent(writeDB, event)
		if err != nil {
			return err
		}
		if !applied {
			return ErrTaskQuotaReservationCASLost
		}
		if err := writeDB.Where("id = ?", event.ID).First(event).Error; err != nil {
			return err
		}
		if event.State != TaskBillingEventStateApplied || event.AppliedAt == nil || event.LockVersion != 2 {
			return fmt.Errorf("%w: reserve event was not durably applied", ErrTaskQuotaReservationCASLost)
		}

		receipt := &QuotaMutationReceipt{
			ReceiptVersion:              quotaMutationReceiptVersion,
			MutationType:                string(TaskBillingEventTypeReserve),
			MutationKey:                 event.EventKey,
			RequestFingerprint:          fingerprint,
			OperationRequestFingerprint: operation.RequestFingerprint,
			OperationID:                 operation.ID,
			OperationPublicID:           operation.PublicID,
			ExpectedOperationVersion:    normalized.ExpectedOperationVersion,
			OperationVersionBefore:      operation.LockVersion,
			OperationVersionAfter:       operation.LockVersion + 1,
			BillingEventID:              event.EventID,
			BillingEventKey:             event.EventKey,
			BillingEventVersion:         event.LockVersion,
			UserID:                      normalized.UserID,
			TokenID:                     normalized.TokenID,
			ChannelID:                   normalized.ChannelID,
			BillingSource:               normalized.BillingSource,
			SubscriptionID:              normalized.SubscriptionID,
			Quota:                       normalized.Quota,
			BillingContext:              TaskQuotaBillingContext(normalized.BillingContext),
			Before:                      before,
			After:                       after,
		}
		if err := quotaMutationReceiptCreateDB(writeDB).Create(receipt).Error; err != nil {
			return err
		}

		won, err := TransitionTaskSubmissionOperation(writeDB, operation.ID, TaskSubmissionOperationTransition{
			From:            TaskSubmissionOperationStatusPrepared,
			To:              TaskSubmissionOperationStatusReserved,
			ExpectedVersion: operation.LockVersion,
		})
		if err != nil {
			return err
		}
		if !won {
			return ErrTaskQuotaReservationCASLost
		}
		result = receipt
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// applySynchronousTaskQuotaReserveEvent is the dedicated T1 state CAS. The
// balance mutation has already been applied in this same savepoint, so a worker
// lease would invent an asynchronous owner that never existed. attempt_count=1
// records the single synchronous application attempt and satisfies the durable
// applied-state invariant.
func applySynchronousTaskQuotaReserveEvent(tx *gorm.DB, event *TaskBillingEvent) (bool, error) {
	if tx == nil {
		return false, gorm.ErrInvalidDB
	}
	if event == nil || event.ID <= 0 || event.State != TaskBillingEventStatePending || event.LockVersion <= 0 || event.LockVersion == quotaMutationMaxInt64 {
		return false, ErrTaskQuotaReservationInvalidInput
	}
	appliedAt, err := taskRecoveryDBTimestamp(tx)
	if err != nil {
		return false, err
	}
	updated := taskRecoveryControlledWrite(tx).Table("task_billing_events").Where(
		"id = ? AND state = ? AND lock_version = ? AND updated_at <= ?",
		event.ID, TaskBillingEventStatePending, event.LockVersion, appliedAt,
	).Updates(map[string]interface{}{
		"state":         TaskBillingEventStateApplied,
		"attempt_count": 1,
		"applied_at":    appliedAt,
		"updated_at":    appliedAt,
		"lock_version":  gorm.Expr("lock_version + ?", 1),
	})
	return updated.RowsAffected == 1, updated.Error
}

func normalizeTaskQuotaReservationInput(input TaskQuotaReservationInput) (TaskQuotaReservationInput, string, error) {
	input.BillingSource = strings.ToLower(strings.TrimSpace(input.BillingSource))
	input.BillingContext.OriginModelName = strings.TrimSpace(input.BillingContext.OriginModelName)
	if len(input.BillingContext.OtherRatios) == 0 {
		input.BillingContext.OtherRatios = nil
	} else {
		otherRatios := make(map[string]float64, len(input.BillingContext.OtherRatios))
		for key, value := range input.BillingContext.OtherRatios {
			otherRatios[key] = value
		}
		input.BillingContext.OtherRatios = otherRatios
	}
	if input.OperationID <= 0 || input.UserID <= 0 || input.TokenID <= 0 || input.ChannelID <= 0 ||
		input.ExpectedOperationVersion <= 0 || input.ExpectedOperationVersion == quotaMutationMaxInt64 ||
		input.Quota < 0 || input.Quota > int64(common.MaxQuota) || input.SubscriptionID < 0 ||
		int64(input.UserID) > int64(common.MaxQuota) || int64(input.TokenID) > int64(common.MaxQuota) ||
		int64(input.ChannelID) > int64(common.MaxQuota) || int64(input.SubscriptionID) > int64(common.MaxQuota) {
		return input, "", ErrTaskQuotaReservationInvalidInput
	}
	if input.BillingSource != "wallet" && input.BillingSource != "subscription" {
		return input, "", fmt.Errorf("%w: unsupported billing source", ErrTaskQuotaReservationInvalidInput)
	}
	if (input.BillingSource == "wallet" && input.SubscriptionID != 0) ||
		(input.BillingSource == "subscription" && input.SubscriptionID <= 0) {
		return input, "", fmt.Errorf("%w: subscription does not match billing source", ErrTaskQuotaReservationInvalidInput)
	}
	if err := validateTaskQuotaBillingContext(input.BillingContext); err != nil {
		return input, "", err
	}
	fingerprintPayload := struct {
		Version                  int                `json:"version"`
		OperationID              int64              `json:"operation_id"`
		UserID                   int                `json:"user_id"`
		TokenID                  int                `json:"token_id"`
		ChannelID                int                `json:"channel_id"`
		ExpectedOperationVersion int64              `json:"expected_operation_version"`
		Quota                    int64              `json:"quota"`
		BillingSource            string             `json:"billing_source"`
		SubscriptionID           int                `json:"subscription_id"`
		BillingContext           TaskBillingContext `json:"billing_context"`
	}{
		Version: quotaMutationFingerprintVersion, OperationID: input.OperationID,
		UserID: input.UserID, TokenID: input.TokenID, ChannelID: input.ChannelID,
		ExpectedOperationVersion: input.ExpectedOperationVersion, Quota: input.Quota,
		BillingSource: input.BillingSource, SubscriptionID: input.SubscriptionID,
		BillingContext: input.BillingContext,
	}
	data, err := common.Marshal(fingerprintPayload)
	if err != nil {
		return input, "", fmt.Errorf("%w: encode reservation fingerprint: %v", ErrTaskQuotaReservationInvalidInput, err)
	}
	digest := sha256.Sum256(data)
	return input, hex.EncodeToString(digest[:]), nil
}

func validateTaskQuotaBillingContext(context TaskBillingContext) error {
	if context.Version != TaskBillingContextVersion || !context.Complete || context.OriginModelName == "" ||
		len(context.OriginModelName) > quotaMutationMaxOriginModelLength {
		return fmt.Errorf("%w: billing snapshot is incomplete", ErrTaskQuotaReservationInvalidInput)
	}
	if !quotaMutationRateInRange(context.ModelPrice, true) || !quotaMutationRateInRange(context.ModelRatio, false) ||
		!quotaMutationRateInRange(context.GroupRatio, false) {
		return fmt.Errorf("%w: billing snapshot contains an invalid primary rate", ErrTaskQuotaReservationInvalidInput)
	}
	if len(context.OtherRatios) > quotaMutationMaxOtherRatios {
		return fmt.Errorf("%w: billing snapshot has too many other ratios", ErrTaskQuotaReservationInvalidInput)
	}
	for name, rate := range context.OtherRatios {
		if name == "" || name != strings.TrimSpace(name) || len(name) > quotaMutationMaxRatioNameLength ||
			rate <= 0 || rate > float64(common.MaxQuota) || math.IsNaN(rate) || math.IsInf(rate, 0) {
			return fmt.Errorf("%w: billing snapshot contains an invalid other ratio", ErrTaskQuotaReservationInvalidInput)
		}
	}
	data, err := common.Marshal(context)
	if err != nil || len(data) > quotaMutationMaxBillingJSONLength {
		return fmt.Errorf("%w: billing snapshot exceeds its encoded boundary", ErrTaskQuotaReservationInvalidInput)
	}
	return nil
}

func quotaMutationRateInRange(value float64, allowTokenSentinel bool) bool {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return false
	}
	if allowTokenSentinel && value == -1 {
		return true
	}
	return value >= 0 && value <= float64(common.MaxQuota)
}

func quotaMutationVersionValid(version int64) bool {
	return version >= 0 && version < quotaMutationMaxInt64
}

func quotaMutationInt32Value(value int) bool {
	return int64(value) >= int64(common.MinQuota) && int64(value) <= int64(common.MaxQuota)
}

func quotaMutationSnapshot(user *User, token *Token, subscription *UserSubscription) QuotaMutationAccountSnapshot {
	snapshot := QuotaMutationAccountSnapshot{
		User: QuotaMutationUserSnapshot{
			ID: user.Id, Status: user.Status, Role: user.Role, Quota: user.Quota,
			UsedQuota: user.UsedQuota, RequestCount: user.RequestCount, QuotaVersion: user.QuotaVersion,
			Group: user.Group, AccountTierID: user.AccountTierID,
		},
		Token: QuotaMutationTokenSnapshot{
			ID: token.Id, UserID: token.UserId, Status: token.Status, CreatedTime: token.CreatedTime,
			AccessedTime: token.AccessedTime, ExpiredTime: token.ExpiredTime, RemainQuota: token.RemainQuota,
			QuotaVersion: token.QuotaVersion, UnlimitedQuota: token.UnlimitedQuota, UsedQuota: token.UsedQuota,
			Group: token.Group, AccessProfileID: token.AccessProfileID, ModelLimitsEnabled: token.ModelLimitsEnabled,
			ModelLimits: token.ModelLimits, CrossGroupRetry: token.CrossGroupRetry,
		},
	}
	if subscription != nil {
		snapshot.Subscription = &QuotaMutationSubscriptionSnapshot{
			ID: subscription.Id, UserID: subscription.UserId, PlanID: subscription.PlanId,
			AmountTotal: subscription.AmountTotal, AmountUsed: subscription.AmountUsed, QuotaVersion: subscription.QuotaVersion,
			StartTime: subscription.StartTime, EndTime: subscription.EndTime, Status: subscription.Status,
			Source: subscription.Source, LastResetTime: subscription.LastResetTime, NextResetTime: subscription.NextResetTime,
			UpgradeGroup: subscription.UpgradeGroup, PrevUserGroup: subscription.PrevUserGroup,
			DowngradeGroup: subscription.DowngradeGroup, AllowWalletOverflow: subscription.AllowWalletOverflow,
			CreatedAt: subscription.CreatedAt, UpdatedAt: subscription.UpdatedAt,
		}
	}
	return snapshot
}

func quotaMutationReservationAfter(before QuotaMutationAccountSnapshot, input TaskQuotaReservationInput, databaseNow int64) (QuotaMutationAccountSnapshot, error) {
	after := before
	if before.Subscription != nil {
		copySubscription := *before.Subscription
		after.Subscription = &copySubscription
	}
	if input.BillingSource == "wallet" {
		if int64(before.User.Quota) < input.Quota {
			return QuotaMutationAccountSnapshot{}, ErrTaskQuotaReservationInsufficientQuota
		}
	} else {
		if after.Subscription == nil {
			return QuotaMutationAccountSnapshot{}, ErrTaskQuotaReservationInvalidInput
		}
		if after.Subscription.AmountUsed > quotaMutationMaxInt64-input.Quota {
			return QuotaMutationAccountSnapshot{}, fmt.Errorf("%w: subscription usage would overflow", ErrTaskQuotaReservationIneligible)
		}
		if after.Subscription.AmountTotal > 0 && after.Subscription.AmountUsed+input.Quota > after.Subscription.AmountTotal {
			return QuotaMutationAccountSnapshot{}, ErrTaskQuotaReservationInsufficientQuota
		}
	}
	if !before.Token.UnlimitedQuota && int64(before.Token.RemainQuota) < input.Quota {
		return QuotaMutationAccountSnapshot{}, ErrTaskQuotaReservationInsufficientQuota
	}
	if input.Quota == 0 {
		return after, nil
	}
	if input.BillingSource == "wallet" {
		after.User.Quota = int(int64(before.User.Quota) - input.Quota)
		after.User.QuotaVersion++
	} else {
		after.Subscription.AmountUsed += input.Quota
		after.Subscription.QuotaVersion++
		after.Subscription.UpdatedAt = databaseNow
	}
	remainAfter := int64(before.Token.RemainQuota) - input.Quota
	usedAfter := int64(before.Token.UsedQuota) + input.Quota
	if remainAfter < int64(common.MinQuota) || remainAfter > int64(common.MaxQuota) ||
		usedAfter < int64(common.MinQuota) || usedAfter > int64(common.MaxQuota) {
		return QuotaMutationAccountSnapshot{}, fmt.Errorf("%w: token quota result exceeds its int32 boundary", ErrTaskQuotaReservationIneligible)
	}
	after.Token.RemainQuota = int(remainAfter)
	after.Token.UsedQuota = int(usedAfter)
	after.Token.AccessedTime = databaseNow
	after.Token.QuotaVersion++
	return after, nil
}

func applyTaskQuotaReservationBalances(tx *gorm.DB, before, after QuotaMutationAccountSnapshot, input TaskQuotaReservationInput) error {
	if input.Quota == 0 {
		return nil
	}
	if input.BillingSource == "wallet" {
		updated := tx.Table("users").Where(
			"id = ? AND quota = ? AND quota_version = ?", before.User.ID, before.User.Quota, before.User.QuotaVersion,
		).Updates(map[string]interface{}{
			"quota": after.User.Quota, "quota_version": after.User.QuotaVersion,
		})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return ErrTaskQuotaReservationCASLost
		}
	}

	updatedToken := tx.Table("tokens").Where(
		"id = ? AND user_id = ? AND remain_quota = ? AND used_quota = ? AND quota_version = ?",
		before.Token.ID, before.Token.UserID, before.Token.RemainQuota, before.Token.UsedQuota, before.Token.QuotaVersion,
	).Updates(map[string]interface{}{
		"remain_quota": after.Token.RemainQuota, "used_quota": after.Token.UsedQuota,
		"accessed_time": after.Token.AccessedTime, "quota_version": after.Token.QuotaVersion,
	})
	if updatedToken.Error != nil {
		return updatedToken.Error
	}
	if updatedToken.RowsAffected != 1 {
		return ErrTaskQuotaReservationCASLost
	}

	if input.BillingSource == "subscription" {
		if before.Subscription == nil || after.Subscription == nil {
			return ErrTaskQuotaReservationInvalidInput
		}
		updatedSubscription := tx.Table("user_subscriptions").Where(
			"id = ? AND user_id = ? AND amount_used = ? AND quota_version = ?",
			before.Subscription.ID, before.Subscription.UserID, before.Subscription.AmountUsed, before.Subscription.QuotaVersion,
		).Updates(map[string]interface{}{
			"amount_used": after.Subscription.AmountUsed, "quota_version": after.Subscription.QuotaVersion,
			"updated_at": after.Subscription.UpdatedAt,
		})
		if updatedSubscription.Error != nil {
			return updatedSubscription.Error
		}
		if updatedSubscription.RowsAffected != 1 {
			return ErrTaskQuotaReservationCASLost
		}
	}
	return nil
}

func validateQuotaMutationReceiptShape(receipt *QuotaMutationReceipt) error {
	if receipt == nil || receipt.ReceiptVersion != quotaMutationReceiptVersion || receipt.MutationKey == "" ||
		receipt.MutationKey != receipt.BillingEventKey || len(receipt.MutationKey) > 128 ||
		!validTaskRecoveryDigest(receipt.RequestFingerprint) || !validTaskRecoveryDigest(receipt.OperationRequestFingerprint) ||
		receipt.OperationID <= 0 || !validTaskSubmissionPublicID(receipt.OperationPublicID) ||
		receipt.ExpectedOperationVersion <= 0 || receipt.OperationVersionBefore != receipt.ExpectedOperationVersion ||
		receipt.OperationVersionBefore == quotaMutationMaxInt64 ||
		(receipt.OperationVersionAfter != receipt.OperationVersionBefore && receipt.OperationVersionAfter != receipt.OperationVersionBefore+1) ||
		!validTaskBillingEventID(receipt.BillingEventID) || receipt.BillingEventVersion <= 0 ||
		receipt.UserID <= 0 || receipt.TokenID <= 0 || receipt.ChannelID <= 0 || receipt.SubscriptionID < 0 ||
		receipt.Quota < 0 || receipt.Quota > int64(common.MaxQuota) {
		return fmt.Errorf("%w: receipt identity or version fields are invalid", ErrTaskQuotaReservationInvalidInput)
	}
	if receipt.MutationType == "" {
		receipt.MutationType = string(TaskBillingEventTypeReserve)
	}
	if receipt.MutationType != string(TaskBillingEventTypeReserve) &&
		receipt.MutationType != string(TaskBillingEventTypeRefund) &&
		receipt.MutationType != string(TaskBillingEventTypeTerminalSettlement) {
		return fmt.Errorf("%w: receipt mutation type is invalid", ErrTaskQuotaReservationInvalidInput)
	}
	if receipt.MutationType == string(TaskBillingEventTypeReserve) && receipt.OperationVersionAfter != receipt.OperationVersionBefore+1 {
		return fmt.Errorf("%w: reserve receipt must advance operation version", ErrTaskQuotaReservationInvalidInput)
	}
	if receipt.BillingSource != "wallet" && receipt.BillingSource != "subscription" ||
		(receipt.BillingSource == "wallet" && receipt.SubscriptionID != 0) ||
		(receipt.BillingSource == "subscription" && receipt.SubscriptionID <= 0) {
		return fmt.Errorf("%w: receipt funding source is invalid", ErrTaskQuotaReservationInvalidInput)
	}
	if err := validateTaskQuotaBillingContext(TaskBillingContext(receipt.BillingContext)); err != nil {
		return err
	}
	switch receipt.MutationType {
	case string(TaskBillingEventTypeReserve):
		if err := validateQuotaMutationSnapshotPair(receipt); err != nil {
			return err
		}
		_, fingerprint, err := normalizeTaskQuotaReservationInput(TaskQuotaReservationInput{
			OperationID: receipt.OperationID, UserID: receipt.UserID, TokenID: receipt.TokenID,
			ChannelID: receipt.ChannelID, ExpectedOperationVersion: receipt.ExpectedOperationVersion,
			Quota: receipt.Quota, BillingSource: receipt.BillingSource, SubscriptionID: receipt.SubscriptionID,
			BillingContext: TaskBillingContext(receipt.BillingContext),
		})
		if err != nil || fingerprint != receipt.RequestFingerprint {
			return fmt.Errorf("%w: receipt request fingerprint does not match its immutable payload", ErrTaskQuotaReservationInvalidInput)
		}
	case string(TaskBillingEventTypeRefund):
		if err := validateQuotaMutationRefundShape(receipt); err != nil {
			return err
		}
	case string(TaskBillingEventTypeTerminalSettlement):
		if err := validateQuotaMutationSettlementShape(receipt); err != nil {
			return err
		}
	}
	return nil
}

func validateQuotaMutationRefundShape(receipt *QuotaMutationReceipt) error {
	before, after := receipt.Before, receipt.After
	if before.User.ID != receipt.UserID || after.User.ID != receipt.UserID ||
		before.Token.ID != receipt.TokenID || after.Token.ID != receipt.TokenID ||
		before.Token.UserID != receipt.UserID || after.Token.UserID != receipt.UserID ||
		before.User.QuotaVersion < 0 || before.Token.QuotaVersion < 0 ||
		after.User.QuotaVersion < before.User.QuotaVersion || after.Token.QuotaVersion < before.Token.QuotaVersion {
		return fmt.Errorf("%w: refund receipt account snapshots do not match their subjects", ErrTaskQuotaReservationInvalidInput)
	}
	if receipt.Quota == 0 {
		if !reflect.DeepEqual(before, after) {
			return fmt.Errorf("%w: zero-quota refund receipt changed account state", ErrTaskQuotaReservationInvalidInput)
		}
		return nil
	}
	if after.Token.QuotaVersion != before.Token.QuotaVersion+1 {
		return fmt.Errorf("%w: token quota version did not advance on refund", ErrTaskQuotaReservationInvalidInput)
	}
	if receipt.BillingSource == "wallet" {
		if before.Subscription != nil || after.Subscription != nil ||
			after.User.QuotaVersion != before.User.QuotaVersion+1 ||
			int64(after.User.Quota)-int64(before.User.Quota) != receipt.Quota {
			return fmt.Errorf("%w: wallet refund snapshot delta is invalid", ErrTaskQuotaReservationInvalidInput)
		}
	} else {
		if before.Subscription == nil || after.Subscription == nil || before.Subscription.ID != receipt.SubscriptionID ||
			after.Subscription.ID != receipt.SubscriptionID || before.Subscription.UserID != receipt.UserID ||
			after.Subscription.UserID != receipt.UserID || after.Subscription.QuotaVersion != before.Subscription.QuotaVersion+1 ||
			before.Subscription.AmountUsed-after.Subscription.AmountUsed != receipt.Quota ||
			!reflect.DeepEqual(before.User, after.User) {
			return fmt.Errorf("%w: subscription refund snapshot delta is invalid", ErrTaskQuotaReservationInvalidInput)
		}
	}
	if int64(after.Token.RemainQuota)-int64(before.Token.RemainQuota) != receipt.Quota ||
		int64(before.Token.UsedQuota)-int64(after.Token.UsedQuota) != receipt.Quota {
		return fmt.Errorf("%w: token refund snapshot delta is invalid", ErrTaskQuotaReservationInvalidInput)
	}
	return nil
}

func validateQuotaMutationSettlementShape(receipt *QuotaMutationReceipt) error {
	before, after := receipt.Before, receipt.After
	if before.User.ID != receipt.UserID || after.User.ID != receipt.UserID ||
		before.Token.ID != receipt.TokenID || after.Token.ID != receipt.TokenID ||
		before.Token.UserID != receipt.UserID || after.Token.UserID != receipt.UserID ||
		before.User.QuotaVersion < 0 || before.Token.QuotaVersion < 0 ||
		after.User.QuotaVersion < before.User.QuotaVersion || after.Token.QuotaVersion < before.Token.QuotaVersion {
		return fmt.Errorf("%w: settlement receipt account snapshots do not match their subjects", ErrTaskQuotaReservationInvalidInput)
	}
	return nil
}

func validateQuotaMutationSettlementSnapshots(before, after QuotaMutationAccountSnapshot, billingSource string, delta int64) error {
	if delta == 0 {
		if !reflect.DeepEqual(before, after) {
			return fmt.Errorf("%w: zero-delta settlement changed account state", ErrTaskQuotaReservationInvalidInput)
		}
		return nil
	}
	if after.Token.QuotaVersion != before.Token.QuotaVersion+1 {
		return fmt.Errorf("%w: token quota version did not advance on settlement", ErrTaskQuotaReservationInvalidInput)
	}
	if billingSource == "wallet" {
		if after.User.QuotaVersion != before.User.QuotaVersion+1 {
			return fmt.Errorf("%w: user quota version did not advance on wallet settlement", ErrTaskQuotaReservationInvalidInput)
		}
		userDiff := int64(after.User.Quota) - int64(before.User.Quota)
		if userDiff != delta && !(after.User.Quota == common.MaxQuota || after.User.Quota == common.MinQuota) {
			return fmt.Errorf("%w: wallet settlement delta mismatch", ErrTaskQuotaReservationInvalidInput)
		}
	} else {
		if before.Subscription == nil || after.Subscription == nil {
			return fmt.Errorf("%w: missing subscription snapshot on settlement", ErrTaskQuotaReservationInvalidInput)
		}
		if after.Subscription.QuotaVersion != before.Subscription.QuotaVersion+1 {
			return fmt.Errorf("%w: subscription quota version did not advance on settlement", ErrTaskQuotaReservationInvalidInput)
		}
		subDiff := before.Subscription.AmountUsed - after.Subscription.AmountUsed
		if subDiff != delta && after.Subscription.AmountUsed != 0 {
			return fmt.Errorf("%w: subscription settlement delta mismatch", ErrTaskQuotaReservationInvalidInput)
		}
	}
	tokenRemainDiff := int64(after.Token.RemainQuota) - int64(before.Token.RemainQuota)
	if tokenRemainDiff != delta && !(after.Token.RemainQuota == common.MaxQuota || after.Token.RemainQuota == common.MinQuota) {
		return fmt.Errorf("%w: token remain delta mismatch on settlement", ErrTaskQuotaReservationInvalidInput)
	}
	return nil
}

func validateQuotaMutationSnapshotPair(receipt *QuotaMutationReceipt) error {
	before, after := receipt.Before, receipt.After
	if before.User.ID != receipt.UserID || after.User.ID != receipt.UserID ||
		before.Token.ID != receipt.TokenID || after.Token.ID != receipt.TokenID ||
		before.Token.UserID != receipt.UserID || after.Token.UserID != receipt.UserID ||
		before.User.QuotaVersion < 0 || before.Token.QuotaVersion < 0 ||
		after.User.QuotaVersion < before.User.QuotaVersion || after.Token.QuotaVersion < before.Token.QuotaVersion {
		return fmt.Errorf("%w: receipt account snapshots do not match their subjects", ErrTaskQuotaReservationInvalidInput)
	}
	if receipt.Quota == 0 {
		if !reflect.DeepEqual(before, after) {
			return fmt.Errorf("%w: zero-quota receipt changed account state", ErrTaskQuotaReservationInvalidInput)
		}
		return nil
	}
	if after.Token.QuotaVersion != before.Token.QuotaVersion+1 {
		return fmt.Errorf("%w: token quota version did not advance", ErrTaskQuotaReservationInvalidInput)
	}
	if receipt.BillingSource == "wallet" {
		if before.Subscription != nil || after.Subscription != nil ||
			after.User.QuotaVersion != before.User.QuotaVersion+1 ||
			int64(before.User.Quota)-int64(after.User.Quota) != receipt.Quota {
			return fmt.Errorf("%w: wallet snapshot delta is invalid", ErrTaskQuotaReservationInvalidInput)
		}
	} else {
		if before.Subscription == nil || after.Subscription == nil || before.Subscription.ID != receipt.SubscriptionID ||
			after.Subscription.ID != receipt.SubscriptionID || before.Subscription.UserID != receipt.UserID ||
			after.Subscription.UserID != receipt.UserID || after.Subscription.QuotaVersion != before.Subscription.QuotaVersion+1 ||
			after.Subscription.AmountUsed-before.Subscription.AmountUsed != receipt.Quota ||
			!reflect.DeepEqual(before.User, after.User) {
			return fmt.Errorf("%w: subscription snapshot delta is invalid", ErrTaskQuotaReservationInvalidInput)
		}
	}
	if int64(before.Token.RemainQuota)-int64(after.Token.RemainQuota) != receipt.Quota ||
		int64(after.Token.UsedQuota)-int64(before.Token.UsedQuota) != receipt.Quota {
		return fmt.Errorf("%w: token snapshot delta is invalid", ErrTaskQuotaReservationInvalidInput)
	}
	return nil
}

func validateStoredQuotaMutationReceipt(tx *gorm.DB, receipt *QuotaMutationReceipt) error {
	if tx == nil {
		return gorm.ErrInvalidDB
	}
	if receipt == nil || receipt.ID <= 0 || receipt.CreatedAt <= 0 {
		return fmt.Errorf("%w: stored receipt is missing database identity", ErrTaskQuotaReservationInvalidInput)
	}
	if err := validateQuotaMutationReceiptShape(receipt); err != nil {
		return err
	}
	var operation TaskSubmissionOperation
	if err := tx.Session(&gorm.Session{NewDB: true}).Where("id = ?", receipt.OperationID).First(&operation).Error; err != nil {
		return err
	}
	if err := validateStoredTaskSubmissionOperation(tx, &operation); err != nil {
		return err
	}
	if operation.PublicID != receipt.OperationPublicID || operation.UserID != receipt.UserID ||
		operation.TokenID != receipt.TokenID || operation.RequestFingerprint != receipt.OperationRequestFingerprint ||
		operation.LockVersion < receipt.OperationVersionAfter {
		return fmt.Errorf("%w: receipt does not match its durable operation", ErrTaskQuotaReservationConflict)
	}
	event, err := GetTaskBillingEventByEventID(tx, receipt.BillingEventID)
	if err != nil {
		return err
	}
	if event == nil {
		return fmt.Errorf("%w: receipt billing event is missing", ErrTaskQuotaReservationConflict)
	}
	if err := validateStoredTaskBillingEvent(tx, event); err != nil {
		return err
	}
	if event.OperationID == nil || *event.OperationID != receipt.OperationID || event.EventKey != receipt.BillingEventKey ||
		string(event.EventType) != receipt.MutationType || event.State != TaskBillingEventStateApplied ||
		event.UserID != receipt.UserID || event.TokenID != receipt.TokenID || event.ChannelID != receipt.ChannelID ||
		event.BillingSource != receipt.BillingSource || event.SubscriptionID != receipt.SubscriptionID ||
		event.LockVersion != receipt.BillingEventVersion {
		return fmt.Errorf("%w: receipt does not match its authoritative billing event", ErrTaskQuotaReservationConflict)
	}
	switch receipt.MutationType {
	case string(TaskBillingEventTypeReserve):
		if event.QuotaDelta != -receipt.Quota {
			return fmt.Errorf("%w: reserve receipt does not match its reserve quota delta", ErrTaskQuotaReservationConflict)
		}
	case string(TaskBillingEventTypeRefund):
		if event.QuotaDelta != receipt.Quota {
			return fmt.Errorf("%w: refund receipt does not match its refund quota delta", ErrTaskQuotaReservationConflict)
		}
	case string(TaskBillingEventTypeTerminalSettlement):
		reserveReceipt, err := findTaskQuotaReservation(tx, receipt.OperationID, receipt.UserID, receipt.TokenID)
		if err != nil {
			return err
		}
		if reserveReceipt == nil {
			return fmt.Errorf("%w: settlement receipt is missing its prerequisite reserve receipt", ErrTaskQuotaReservationConflict)
		}
		expectedDelta := reserveReceipt.Quota - receipt.Quota
		if event.QuotaDelta != expectedDelta {
			return fmt.Errorf("%w: settlement receipt does not match its calculated quota delta", ErrTaskQuotaReservationConflict)
		}
		if err := validateQuotaMutationSettlementSnapshots(receipt.Before, receipt.After, receipt.BillingSource, expectedDelta); err != nil {
			return err
		}
	}
	return nil
}
