package model

import (
	"context"
	"crypto/sha256"
	"database/sql/driver"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"gorm.io/gorm"
)

const (
	AccountQuotaMutationReceiptVersion  = 1
	AccountQuotaFingerprintVersion      = 2
	accountQuotaMutationCreateSetting   = "account-quota-mutation:receipt-create"
	accountQuotaMutationMaxContextBytes = 16 * 1024
)

const (
	AccountQuotaPhaseReserve = "reserve"
	AccountQuotaPhaseAdjust  = "reserve_adjust"
	AccountQuotaPhaseSettle  = "settle"
	AccountQuotaPhaseRefund  = "refund"
)

var (
	ErrAccountQuotaMutationInvalidInput = errors.New("invalid account quota mutation input")
	ErrAccountQuotaMutationConflict     = errors.New("account quota mutation conflicts with existing receipt")
	ErrAccountQuotaMutationNotFound     = errors.New("account quota reserve receipt not found")
	ErrAccountQuotaMutationTerminal     = errors.New("account quota mutation is already terminal")
	ErrAccountQuotaMutationIneligible   = errors.New("account quota mutation subject is not eligible")
	ErrAccountQuotaMutationInsufficient = errors.New("insufficient account quota")
	ErrAccountQuotaMutationCASLost      = errors.New("account quota mutation compare-and-swap lost")
	ErrAccountQuotaMutationStaleReceipt = errors.New("account quota mutation receipt is not in the current reservation chain")
	ErrAccountQuotaReceiptImmutable     = errors.New("account quota mutation receipt is immutable")
)

type accountQuotaMutationCreateMarkerType struct{ value byte }

var accountQuotaMutationCreateMarker = &accountQuotaMutationCreateMarkerType{}

// AccountBillingContext is the immutable pricing and routing context used for
// a synchronous relay reservation. It intentionally contains no credentials.
type AccountBillingContext struct {
	Version           int               `json:"version"`
	ModelName         string            `json:"model_name"`
	OriginModelName   string            `json:"origin_model_name"`
	UsingGroup        string            `json:"using_group"`
	BillingPreference string            `json:"billing_preference"`
	ChannelID         int               `json:"channel_id"`
	ForcePreConsume   bool              `json:"force_pre_consume"`
	Playground        bool              `json:"playground"`
	FreeModel         bool              `json:"free_model"`
	Metadata          map[string]string `json:"metadata,omitempty"`
}

func (snapshot *AccountBillingContext) Scan(value interface{}) error {
	*snapshot = AccountBillingContext{}
	data, err := taskRecoveryTextValue(value)
	if err != nil || len(data) == 0 {
		return err
	}
	return common.Unmarshal(data, snapshot)
}

func (snapshot AccountBillingContext) Value() (driver.Value, error) {
	data, err := common.Marshal(snapshot)
	if err != nil {
		return nil, err
	}
	return string(data), nil
}

// AccountQuotaDeltas records both the caller's requested mutation and the
// delta actually committed after trust/free handling and source selection.
type AccountQuotaDeltas struct {
	UserQuota              int64 `json:"user_quota"`
	TokenRemainQuota       int64 `json:"token_remain_quota"`
	TokenUsedQuota         int64 `json:"token_used_quota"`
	SubscriptionAmountUsed int64 `json:"subscription_amount_used"`
}

func (deltas *AccountQuotaDeltas) Scan(value interface{}) error {
	*deltas = AccountQuotaDeltas{}
	data, err := taskRecoveryTextValue(value)
	if err != nil || len(data) == 0 {
		return err
	}
	return common.Unmarshal(data, deltas)
}

func (deltas AccountQuotaDeltas) Value() (driver.Value, error) {
	data, err := common.Marshal(deltas)
	if err != nil {
		return nil, err
	}
	return string(data), nil
}

// AccountQuotaMutationReceipt is one immutable reserve or terminal receipt.
// MutationSlot is unique so settle and refund share one terminal slot.
type AccountQuotaMutationReceipt struct {
	ID                 int64                        `json:"id" gorm:"primaryKey"`
	ReceiptVersion     int                          `json:"receipt_version" gorm:"not null;<-:create"`
	FingerprintVersion int                          `json:"fingerprint_version" gorm:"not null;default:0;<-:create"`
	WriterEpoch        int64                        `json:"writer_epoch" gorm:"type:bigint;not null;<-:create"`
	RequestID          string                       `json:"request_id" gorm:"type:varchar(64);not null;index:idx_account_quota_request,priority:1;<-:create"`
	Phase              string                       `json:"phase" gorm:"type:varchar(16);not null;index:idx_account_quota_request,priority:2;<-:create"`
	EventKey           string                       `json:"event_key" gorm:"type:varchar(128);not null;uniqueIndex:uidx_account_quota_event;<-:create"`
	MutationSlot       string                       `json:"mutation_slot" gorm:"type:varchar(128);not null;uniqueIndex:uidx_account_quota_slot;<-:create"`
	ParentReceiptID    int64                        `json:"parent_receipt_id" gorm:"type:bigint;not null;default:0;index;<-:create"`
	AuditKey           string                       `json:"audit_key,omitempty" gorm:"type:varchar(96);not null;default:'';<-:create"`
	ReserveFingerprint string                       `json:"reserve_fingerprint" gorm:"type:char(64);not null;default:'';<-:create"`
	RequestFingerprint string                       `json:"request_fingerprint" gorm:"type:char(64);not null;<-:create"`
	BillingSource      string                       `json:"billing_source" gorm:"type:varchar(32);not null;<-:create"`
	BillingPreference  string                       `json:"billing_preference" gorm:"type:varchar(32);not null;<-:create"`
	UserID             int                          `json:"user_id" gorm:"not null;index;<-:create"`
	TokenID            int                          `json:"token_id" gorm:"not null;index;<-:create"`
	SubscriptionID     int                          `json:"subscription_id" gorm:"not null;default:0;index;<-:create"`
	RequestedQuota     int64                        `json:"requested_quota" gorm:"type:bigint;not null;<-:create"`
	AppliedQuota       int64                        `json:"applied_quota" gorm:"type:bigint;not null;<-:create"`
	RequestedDeltas    AccountQuotaDeltas           `json:"requested_deltas" gorm:"type:text;<-:create"`
	AppliedDeltas      AccountQuotaDeltas           `json:"applied_deltas" gorm:"type:text;<-:create"`
	Before             QuotaMutationAccountSnapshot `json:"before" gorm:"type:text;<-:create"`
	After              QuotaMutationAccountSnapshot `json:"after" gorm:"type:text;<-:create"`
	BillingContext     AccountBillingContext        `json:"billing_context" gorm:"type:text;<-:create"`
	CreatedAt          int64                        `json:"created_at" gorm:"type:bigint;not null;index;<-:create"`
}

func (AccountQuotaMutationReceipt) TableName() string { return "account_quota_mutation_receipts" }

// AccountQuotaReservationHead is the single mutable serialization point for a
// synchronous request. Account rows are always locked before this row:
// User -> Token -> Subscription -> ReservationHead -> current Receipt.
type AccountQuotaReservationHead struct {
	ID                 int64  `json:"id" gorm:"primaryKey"`
	RequestID          string `json:"request_id" gorm:"type:varchar(64);not null;uniqueIndex"`
	WriterEpoch        int64  `json:"writer_epoch" gorm:"type:bigint;not null"`
	RootReceiptID      int64  `json:"root_receipt_id" gorm:"type:bigint;not null;index"`
	CurrentReceiptID   int64  `json:"current_receipt_id" gorm:"type:bigint;not null;index"`
	TerminalReceiptID  int64  `json:"terminal_receipt_id" gorm:"type:bigint;not null;default:0;index"`
	ReserveFingerprint string `json:"reserve_fingerprint" gorm:"type:char(64);not null;default:''"`
	UserID             int    `json:"user_id" gorm:"not null;index"`
	TokenID            int    `json:"token_id" gorm:"not null;index"`
	SubscriptionID     int    `json:"subscription_id" gorm:"not null;default:0;index"`
	BillingSource      string `json:"billing_source" gorm:"type:varchar(32);not null"`
	AppliedQuota       int64  `json:"applied_quota" gorm:"type:bigint;not null"`
	LockVersion        int64  `json:"lock_version" gorm:"type:bigint;not null;default:1"`
	CreatedAt          int64  `json:"created_at" gorm:"type:bigint;not null"`
	UpdatedAt          int64  `json:"updated_at" gorm:"type:bigint;not null"`
}

func (AccountQuotaReservationHead) TableName() string { return "account_quota_reservation_heads" }

func accountQuotaReceiptCreateDB(tx *gorm.DB) *gorm.DB {
	return tx.Session(&gorm.Session{NewDB: true}).Set(accountQuotaMutationCreateSetting, accountQuotaMutationCreateMarker)
}

func (receipt *AccountQuotaMutationReceipt) BeforeCreate(tx *gorm.DB) error {
	marker, ok := tx.Get(accountQuotaMutationCreateSetting)
	if !ok || marker != accountQuotaMutationCreateMarker {
		return ErrAccountQuotaReceiptImmutable
	}
	if receipt == nil || receipt.ID != 0 || receipt.CreatedAt != 0 || receipt.ReceiptVersion != AccountQuotaMutationReceiptVersion ||
		receipt.FingerprintVersion != AccountQuotaFingerprintVersion ||
		receipt.WriterEpoch <= 0 || receipt.RequestID == "" || receipt.EventKey == "" || receipt.MutationSlot == "" ||
		len(receipt.ReserveFingerprint) != 64 || len(receipt.RequestFingerprint) != 64 || receipt.UserID <= 0 || receipt.TokenID <= 0 {
		return ErrAccountQuotaMutationInvalidInput
	}
	now, err := taskRecoveryDBTimestamp(tx)
	if err != nil {
		return err
	}
	receipt.CreatedAt = now
	return nil
}

func (*AccountQuotaMutationReceipt) BeforeUpdate(*gorm.DB) error {
	return ErrAccountQuotaReceiptImmutable
}
func (*AccountQuotaMutationReceipt) BeforeDelete(*gorm.DB) error {
	return ErrAccountQuotaReceiptImmutable
}

type AccountQuotaReserveInput struct {
	RequestID         string
	UserID            int
	TokenID           int
	RequestedQuota    int64
	BillingPreference string
	TrustQuota        int64
	AllowTrust        bool
	Playground        bool
	BillingContext    AccountBillingContext
}

type AccountQuotaTerminalInput struct {
	RequestID        string
	ReserveReceiptID int64
	ActualQuota      int64
	BillingContext   AccountBillingContext
	AuditKey         string
}

type accountQuotaFingerprintPayload struct {
	Version            int                   `json:"version"`
	RequestID          string                `json:"request_id"`
	Phase              string                `json:"phase"`
	ReserveReceiptID   int64                 `json:"reserve_receipt_id"`
	ReserveFingerprint string                `json:"reserve_fingerprint"`
	UserID             int                   `json:"user_id"`
	TokenID            int                   `json:"token_id"`
	RequestedQuota     int64                 `json:"requested_quota"`
	ActualQuota        int64                 `json:"actual_quota"`
	Preference         string                `json:"preference"`
	Playground         bool                  `json:"playground"`
	AuditKey           string                `json:"audit_key"`
	BillingContext     AccountBillingContext `json:"billing_context"`
}

func accountQuotaReceiptFingerprint(receipt *AccountQuotaMutationReceipt) (string, error) {
	if receipt == nil {
		return "", ErrAccountQuotaMutationInvalidInput
	}
	payload := accountQuotaFingerprintPayload{
		Version: receipt.FingerprintVersion, RequestID: receipt.RequestID, Phase: receipt.Phase,
		ReserveReceiptID: receipt.ParentReceiptID, ReserveFingerprint: receipt.ReserveFingerprint,
		UserID: receipt.UserID, TokenID: receipt.TokenID, Preference: receipt.BillingPreference,
		Playground: receipt.BillingContext.Playground, AuditKey: receipt.AuditKey, BillingContext: receipt.BillingContext,
	}
	switch receipt.Phase {
	case AccountQuotaPhaseReserve:
		payload.ReserveReceiptID = 0
		payload.ReserveFingerprint = ""
		payload.RequestedQuota = receipt.RequestedQuota
	case AccountQuotaPhaseAdjust:
		// Adjust identity is stable for a target even if another adjustment wins
		// between retries. The immutable ParentReceiptID still records the chain.
		payload.ReserveReceiptID = 0
		payload.RequestedQuota = receipt.RequestedQuota
	case AccountQuotaPhaseSettle, AccountQuotaPhaseRefund:
		payload.ActualQuota = receipt.RequestedQuota
	case "direct_token":
		payload.ReserveReceiptID = 0
		payload.ReserveFingerprint = ""
		payload.Preference = ""
		payload.Playground = false
		payload.BillingContext = AccountBillingContext{}
		payload.ActualQuota = receipt.RequestedQuota
	default:
		payload.ActualQuota = receipt.RequestedQuota
	}
	return accountQuotaFingerprint(payload)
}

// RecomputeAccountQuotaReceiptFingerprint verifies that a stored receipt has
// all durable inputs required to reproduce its request identity.
func RecomputeAccountQuotaReceiptFingerprint(receipt *AccountQuotaMutationReceipt) (string, error) {
	return accountQuotaReceiptFingerprint(receipt)
}

func accountQuotaFingerprint(payload accountQuotaFingerprintPayload) (string, error) {
	data, err := common.Marshal(payload)
	if err != nil {
		return "", err
	}
	if len(data) > accountQuotaMutationMaxContextBytes {
		return "", ErrAccountQuotaMutationInvalidInput
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

func normalizeAccountRequestID(requestID string) (string, error) {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" || len(requestID) > 64 {
		return "", ErrAccountQuotaMutationInvalidInput
	}
	return requestID, nil
}

func normalizeAccountBillingPreference(preference string) (string, error) {
	preference = common.NormalizeBillingPreference(preference)
	switch preference {
	case "wallet_only", "subscription_only", "wallet_first", "subscription_first":
		return preference, nil
	default:
		return "", ErrAccountQuotaMutationInvalidInput
	}
}

func accountQuotaEventKey(requestID, phase string) string {
	return "request:" + requestID + ":" + phase + ":v1"
}

func accountQuotaMutationSlot(requestID, phase string) string {
	if phase == AccountQuotaPhaseReserve {
		return accountQuotaEventKey(requestID, phase)
	}
	return "request:" + requestID + ":terminal:v1"
}

func findAccountQuotaReceiptByEvent(db *gorm.DB, eventKey string) (*AccountQuotaMutationReceipt, error) {
	var receipt AccountQuotaMutationReceipt
	result := db.Where("event_key = ?", eventKey).Limit(1).Find(&receipt)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &receipt, nil
}

func FindAccountQuotaReserveReceipt(db *gorm.DB, requestID string) (*AccountQuotaMutationReceipt, error) {
	requestID, err := normalizeAccountRequestID(requestID)
	if err != nil {
		return nil, err
	}
	return findAccountQuotaReceiptByEvent(db, accountQuotaEventKey(requestID, AccountQuotaPhaseReserve))
}

func FindAccountQuotaTerminalReceipt(db *gorm.DB, requestID string) (*AccountQuotaMutationReceipt, error) {
	requestID, err := normalizeAccountRequestID(requestID)
	if err != nil {
		return nil, err
	}
	var receipt AccountQuotaMutationReceipt
	result := db.Where("mutation_slot = ?", accountQuotaMutationSlot(requestID, AccountQuotaPhaseSettle)).Limit(1).Find(&receipt)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &receipt, nil
}

func validateAccountQuotaValue(value int64) error {
	if value < 0 || value > int64(common.MaxQuota) {
		return ErrAccountQuotaMutationInvalidInput
	}
	return nil
}

func accountQuotaRequestedDeltas(source string, quota int64, playground bool) AccountQuotaDeltas {
	delta := AccountQuotaDeltas{}
	if source == "free" {
		return delta
	}
	if source == "wallet" {
		delta.UserQuota = -quota
	} else if source == "subscription" {
		delta.SubscriptionAmountUsed = quota
	}
	if !playground {
		delta.TokenRemainQuota = -quota
		delta.TokenUsedQuota = quota
	}
	return delta
}

func accountQuotaAfter(before QuotaMutationAccountSnapshot, deltas AccountQuotaDeltas, databaseNow int64) (QuotaMutationAccountSnapshot, error) {
	after := before
	if before.Subscription != nil {
		copySubscription := *before.Subscription
		after.Subscription = &copySubscription
	}
	userQuota := int64(before.User.Quota) + deltas.UserQuota
	if userQuota < int64(common.MinQuota) || userQuota > int64(common.MaxQuota) {
		return QuotaMutationAccountSnapshot{}, ErrAccountQuotaMutationInsufficient
	}
	remainQuota := int64(before.Token.RemainQuota) + deltas.TokenRemainQuota
	usedQuota := int64(before.Token.UsedQuota) + deltas.TokenUsedQuota
	if remainQuota < int64(common.MinQuota) || remainQuota > int64(common.MaxQuota) ||
		usedQuota < int64(common.MinQuota) || usedQuota > int64(common.MaxQuota) {
		return QuotaMutationAccountSnapshot{}, ErrAccountQuotaMutationIneligible
	}
	if deltas.SubscriptionAmountUsed != 0 {
		if after.Subscription == nil {
			return QuotaMutationAccountSnapshot{}, ErrAccountQuotaMutationInvalidInput
		}
		if deltas.SubscriptionAmountUsed > 0 && after.Subscription.AmountUsed > quotaMutationMaxInt64-deltas.SubscriptionAmountUsed {
			return QuotaMutationAccountSnapshot{}, ErrAccountQuotaMutationIneligible
		}
		if deltas.SubscriptionAmountUsed < 0 && after.Subscription.AmountUsed < -deltas.SubscriptionAmountUsed {
			return QuotaMutationAccountSnapshot{}, ErrAccountQuotaMutationIneligible
		}
		newUsed := after.Subscription.AmountUsed + deltas.SubscriptionAmountUsed
		if newUsed < 0 || newUsed > int64(common.MaxQuota) {
			return QuotaMutationAccountSnapshot{}, ErrAccountQuotaMutationInsufficient
		}
		after.Subscription.AmountUsed = newUsed
		after.Subscription.QuotaVersion++
		after.Subscription.UpdatedAt = databaseNow
	}
	if deltas.UserQuota != 0 {
		after.User.Quota = int(userQuota)
		after.User.QuotaVersion++
	}
	if deltas.TokenRemainQuota != 0 || deltas.TokenUsedQuota != 0 {
		if usedQuota < 0 {
			return QuotaMutationAccountSnapshot{}, ErrAccountQuotaMutationIneligible
		}
		after.Token.RemainQuota = int(remainQuota)
		after.Token.UsedQuota = int(usedQuota)
		after.Token.AccessedTime = databaseNow
		after.Token.QuotaVersion++
	}
	return after, nil
}

func applyAccountQuotaBalances(tx *gorm.DB, before, after QuotaMutationAccountSnapshot, deltas AccountQuotaDeltas) error {
	if deltas.UserQuota != 0 {
		result := tx.Table("users").Where("id = ? AND quota = ? AND quota_version = ?", before.User.ID, before.User.Quota, before.User.QuotaVersion).
			Updates(map[string]interface{}{"quota": after.User.Quota, "quota_version": after.User.QuotaVersion})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrAccountQuotaMutationCASLost
		}
	}
	if deltas.TokenRemainQuota != 0 || deltas.TokenUsedQuota != 0 {
		result := tx.Table("tokens").Where("id = ? AND user_id = ? AND remain_quota = ? AND used_quota = ? AND quota_version = ?",
			before.Token.ID, before.Token.UserID, before.Token.RemainQuota, before.Token.UsedQuota, before.Token.QuotaVersion).
			Updates(map[string]interface{}{"remain_quota": after.Token.RemainQuota, "used_quota": after.Token.UsedQuota,
				"accessed_time": after.Token.AccessedTime, "quota_version": after.Token.QuotaVersion})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrAccountQuotaMutationCASLost
		}
	}
	if deltas.SubscriptionAmountUsed != 0 {
		if before.Subscription == nil || after.Subscription == nil {
			return ErrAccountQuotaMutationInvalidInput
		}
		result := tx.Table("user_subscriptions").Where("id = ? AND user_id = ? AND amount_used = ? AND quota_version = ?",
			before.Subscription.ID, before.Subscription.UserID, before.Subscription.AmountUsed, before.Subscription.QuotaVersion).
			Updates(map[string]interface{}{"amount_used": after.Subscription.AmountUsed, "quota_version": after.Subscription.QuotaVersion, "updated_at": after.Subscription.UpdatedAt})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrAccountQuotaMutationCASLost
		}
	}
	return nil
}

func selectAccountBillingSource(tx *gorm.DB, user *User, preference string, walletQuota, subscriptionQuota int64, now int64) (string, *UserSubscription, error) {
	walletAvailable := int64(user.Quota) >= walletQuota
	if preference == "wallet_only" || (preference == "wallet_first" && walletAvailable) {
		if walletAvailable {
			return "wallet", nil, nil
		}
		if preference == "wallet_only" {
			return "", nil, ErrAccountQuotaMutationInsufficient
		}
	}
	subscription, hasActive, allowWalletOverflow, err := selectActiveUserSubscriptionForPreConsumeTx(tx, user.Id, subscriptionQuota, now)
	if err != nil {
		return "", nil, err
	}
	subscriptionAvailable := subscription != nil
	switch preference {
	case "subscription_only":
		if subscriptionAvailable {
			return "subscription", subscription, nil
		}
	case "wallet_first":
		if subscriptionAvailable {
			return "subscription", subscription, nil
		}
	case "subscription_first":
		if subscriptionAvailable {
			return "subscription", subscription, nil
		}
		if walletAvailable && (!hasActive || allowWalletOverflow) {
			return "wallet", nil, nil
		}
	}
	return "", nil, ErrAccountQuotaMutationInsufficient
}

func normalizeAccountReserveInput(input AccountQuotaReserveInput) (AccountQuotaReserveInput, string, error) {
	var err error
	input.RequestID, err = normalizeAccountRequestID(input.RequestID)
	if err != nil || input.UserID <= 0 || input.TokenID <= 0 || validateAccountQuotaValue(input.RequestedQuota) != nil || validateAccountQuotaValue(input.TrustQuota) != nil {
		return input, "", ErrAccountQuotaMutationInvalidInput
	}
	input.BillingPreference, err = normalizeAccountBillingPreference(input.BillingPreference)
	if err != nil {
		return input, "", err
	}
	input.BillingContext.BillingPreference = input.BillingPreference
	input.BillingContext.Playground = input.Playground
	fingerprint, err := accountQuotaFingerprint(accountQuotaFingerprintPayload{
		Version: AccountQuotaFingerprintVersion, RequestID: input.RequestID, Phase: AccountQuotaPhaseReserve,
		UserID: input.UserID, TokenID: input.TokenID, RequestedQuota: input.RequestedQuota, Preference: input.BillingPreference,
		Playground: input.Playground, BillingContext: input.BillingContext,
	})
	return input, fingerprint, err
}

func accountQuotaReplay(db *gorm.DB, eventKey, fingerprint string) (*AccountQuotaMutationReceipt, error) {
	receipt, err := findAccountQuotaReceiptByEvent(db, eventKey)
	if err != nil || receipt == nil {
		return receipt, err
	}
	if receipt.RequestFingerprint != fingerprint {
		return nil, ErrAccountQuotaMutationConflict
	}
	recomputed, recomputeErr := accountQuotaReceiptFingerprint(receipt)
	if recomputeErr != nil || recomputed != receipt.RequestFingerprint {
		return nil, ErrAccountQuotaMutationConflict
	}
	return receipt, nil
}

const accountQuotaSQLiteTransactionAttempts = 20

var accountQuotaTransactionAfterCommitHook func(string) error

func accountQuotaOperationContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if ctx == nil {
		return context.WithTimeout(context.Background(), timeout)
	}
	return ctx, func() {}
}

func projectAccountQuotaReceiptDetached(ctx context.Context, db *gorm.DB, receipt *AccountQuotaMutationReceipt) {
	if receipt == nil || db == nil {
		return
	}
	baseCtx := context.Background()
	if ctx != nil {
		baseCtx = context.WithoutCancel(ctx)
	}
	projectionCtx, cancel := context.WithTimeout(baseCtx, 2*time.Second)
	defer cancel()
	_ = ProjectQuotaMutationReceipt(projectionCtx, db.WithContext(projectionCtx), receipt)
}

func accountQuotaSQLiteRetryable(db *gorm.DB, err error) bool {
	if db == nil || err == nil || db.Dialector == nil || db.Dialector.Name() != "sqlite" {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "sqlite_busy") || strings.Contains(message, "sqlite_locked") ||
		strings.Contains(message, "database is locked") || strings.Contains(message, "database table is locked")
}

func accountQuotaTransaction(ctx context.Context, db *gorm.DB, eventKey string, fingerprint *string, fn func(*gorm.DB) (*AccountQuotaMutationReceipt, error)) (*AccountQuotaMutationReceipt, error) {
	ctx, cancel := accountQuotaOperationContext(ctx, 5*time.Second)
	defer cancel()
	var lastErr error
	for attempt := 0; attempt < accountQuotaSQLiteTransactionAttempts; attempt++ {
		var committed *AccountQuotaMutationReceipt
		err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var err error
			committed, err = fn(tx)
			return err
		})
		if err == nil && accountQuotaTransactionAfterCommitHook != nil {
			err = accountQuotaTransactionAfterCommitHook(eventKey)
		}
		if err == nil {
			return committed, nil
		}
		lastErr = err
		// Every ambiguous/busy result is resolved against the same durable event
		// identity before any retry. A conflicting row is never accepted.
		if fingerprint != nil && *fingerprint != "" {
			readbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
			replay, replayErr := accountQuotaReplay(db.WithContext(readbackCtx), eventKey, *fingerprint)
			cancel()
			if replayErr != nil && !accountQuotaSQLiteRetryable(db, replayErr) {
				return nil, replayErr
			}
			if replay != nil {
				return replay, nil
			}
		}
		if !accountQuotaSQLiteRetryable(db, err) && !errors.Is(err, ErrAccountQuotaMutationCASLost) {
			return nil, err
		}
		if attempt+1 == accountQuotaSQLiteTransactionAttempts {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(min(attempt+1, 10)) * 5 * time.Millisecond):
		}
	}
	return nil, lastErr
}

func ReserveAccountQuota(ctx context.Context, db *gorm.DB, input AccountQuotaReserveInput) (*AccountQuotaMutationReceipt, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	ctx, cancel := accountQuotaOperationContext(ctx, 5*time.Second)
	defer cancel()
	db = db.WithContext(ctx)
	normalized, fingerprint, err := normalizeAccountReserveInput(input)
	if err != nil {
		return nil, err
	}
	initialState, err := requireDurableQuotaWriterEpoch(db)
	if err != nil {
		return nil, err
	}
	eventKey := accountQuotaEventKey(normalized.RequestID, AccountQuotaPhaseReserve)
	if existing, replayErr := accountQuotaReplay(db, eventKey, fingerprint); replayErr != nil || existing != nil {
		if existing != nil && existing.WriterEpoch != initialState.Epoch {
			return nil, ErrQuotaWriterEpochMismatch
		}
		return existing, replayErr
	}

	receipt, err := accountQuotaTransaction(ctx, db, eventKey, &fingerprint, func(tx *gorm.DB) (*AccountQuotaMutationReceipt, error) {
		state, err := requireDurableQuotaWriterEpoch(tx)
		if err != nil {
			return nil, err
		}
		if state.Epoch != initialState.Epoch {
			return nil, ErrDurableQuotaWriterModeDisabled
		}
		if existing, replayErr := accountQuotaReplay(tx, eventKey, fingerprint); replayErr != nil || existing != nil {
			return existing, replayErr
		}
		now, err := taskRecoveryDBTimestamp(tx)
		if err != nil {
			return nil, err
		}
		var user User
		if err := lockForUpdate(tx).Where("id = ?", normalized.UserID).First(&user).Error; err != nil {
			return nil, err
		}
		if user.Status != common.UserStatusEnabled || !quotaMutationVersionValid(user.QuotaVersion) {
			return nil, ErrAccountQuotaMutationIneligible
		}
		var token Token
		if err := lockForUpdate(tx).Where("id = ?", normalized.TokenID).First(&token).Error; err != nil {
			return nil, err
		}
		if token.UserId != user.Id || token.Status != common.TokenStatusEnabled || (token.ExpiredTime != -1 && token.ExpiredTime <= now) || !quotaMutationVersionValid(token.QuotaVersion) {
			return nil, ErrAccountQuotaMutationIneligible
		}

		source := "free"
		appliedQuota := int64(0)
		var subscription *UserSubscription
		if !normalized.BillingContext.FreeModel {
			minimumReserve := normalized.RequestedQuota
			if minimumReserve == 0 {
				minimumReserve = 1
			}
			source, subscription, err = selectAccountBillingSource(tx, &user, normalized.BillingPreference, minimumReserve, minimumReserve, now)
			if err != nil {
				return nil, err
			}
			appliedQuota = minimumReserve
			if normalized.RequestedQuota > 0 && normalized.AllowTrust && source == "wallet" && normalized.TrustQuota > 0 &&
				int64(user.Quota) > normalized.TrustQuota && (token.UnlimitedQuota || int64(token.RemainQuota) > normalized.TrustQuota) {
				appliedQuota = 0
			}
			if !normalized.Playground && !token.UnlimitedQuota && int64(token.RemainQuota) < appliedQuota {
				return nil, ErrAccountQuotaMutationInsufficient
			}
		}

		requestedDeltas := accountQuotaRequestedDeltas(source, normalized.RequestedQuota, normalized.Playground)
		appliedDeltas := accountQuotaRequestedDeltas(source, appliedQuota, normalized.Playground)
		before := quotaMutationSnapshot(&user, &token, subscription)
		after, err := accountQuotaAfter(before, appliedDeltas, now)
		if err != nil {
			return nil, err
		}
		if _, err := requireQuotaProjectionEpoch(tx, state.Epoch); err != nil {
			return nil, err
		}
		if err := applyAccountQuotaBalances(tx, before, after, appliedDeltas); err != nil {
			return nil, err
		}
		receipt := &AccountQuotaMutationReceipt{
			ReceiptVersion: AccountQuotaMutationReceiptVersion, FingerprintVersion: AccountQuotaFingerprintVersion, WriterEpoch: state.Epoch, RequestID: normalized.RequestID,
			Phase: AccountQuotaPhaseReserve, EventKey: eventKey, MutationSlot: accountQuotaMutationSlot(normalized.RequestID, AccountQuotaPhaseReserve),
			ReserveFingerprint: fingerprint, RequestFingerprint: fingerprint, BillingSource: source, BillingPreference: normalized.BillingPreference,
			UserID: user.Id, TokenID: token.Id, RequestedQuota: normalized.RequestedQuota, AppliedQuota: appliedQuota,
			RequestedDeltas: requestedDeltas, AppliedDeltas: appliedDeltas, Before: before, After: after, BillingContext: normalized.BillingContext,
		}
		if subscription != nil {
			receipt.SubscriptionID = subscription.Id
		}
		if err := accountQuotaReceiptCreateDB(tx).Create(receipt).Error; err != nil {
			return nil, err
		}
		head := &AccountQuotaReservationHead{
			RequestID: normalized.RequestID, WriterEpoch: state.Epoch, RootReceiptID: receipt.ID, CurrentReceiptID: receipt.ID,
			ReserveFingerprint: fingerprint, UserID: user.Id, TokenID: token.Id, SubscriptionID: receipt.SubscriptionID,
			BillingSource: source, AppliedQuota: appliedQuota, LockVersion: 1, CreatedAt: now, UpdatedAt: now,
		}
		if err := tx.Create(head).Error; err != nil {
			return nil, err
		}
		if err := createAccountQuotaLifecycleObligation(tx, head, receipt); err != nil {
			return nil, err
		}
		if _, err := ensureQuotaProjectionObligation(tx, receipt); err != nil {
			return nil, err
		}
		return receipt, nil
	})
	if err != nil {
		return nil, err
	}
	if receipt != nil {
		projectAccountQuotaReceiptDetached(ctx, db, receipt)
	}
	return receipt, nil
}

func normalizeAccountTerminalInput(input AccountQuotaTerminalInput, phase string) (AccountQuotaTerminalInput, error) {
	var err error
	input.RequestID, err = normalizeAccountRequestID(input.RequestID)
	input.AuditKey = strings.TrimSpace(input.AuditKey)
	if err != nil || input.ReserveReceiptID <= 0 || validateAccountQuotaValue(input.ActualQuota) != nil || len(input.AuditKey) > 96 {
		return input, ErrAccountQuotaMutationInvalidInput
	}
	if phase == AccountQuotaPhaseRefund && input.AuditKey == "" {
		return input, ErrAccountQuotaMutationInvalidInput
	}
	return input, nil
}

func findAccountQuotaReservationHead(db *gorm.DB, requestID string) (*AccountQuotaReservationHead, error) {
	var head AccountQuotaReservationHead
	result := db.Where("request_id = ?", requestID).Limit(1).Find(&head)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, ErrAccountQuotaMutationNotFound
	}
	return &head, nil
}

func accountQuotaReceiptInChain(db *gorm.DB, head *AccountQuotaReservationHead, receiptID int64) (bool, error) {
	if head == nil || receiptID <= 0 {
		return false, ErrAccountQuotaMutationInvalidInput
	}
	currentID := head.CurrentReceiptID
	for depth := 0; depth < 1024 && currentID > 0; depth++ {
		var receipt AccountQuotaMutationReceipt
		if err := db.Where("id = ?", currentID).First(&receipt).Error; err != nil {
			return false, err
		}
		recomputed, err := accountQuotaReceiptFingerprint(&receipt)
		if err != nil || receipt.RequestID != head.RequestID || receipt.ReserveFingerprint != head.ReserveFingerprint || recomputed != receipt.RequestFingerprint {
			return false, ErrAccountQuotaMutationStaleReceipt
		}
		if currentID == receiptID {
			return true, nil
		}
		currentID = receipt.ParentReceiptID
	}
	return false, nil
}

func validateAccountQuotaHeadReceipt(head *AccountQuotaReservationHead, receipt *AccountQuotaMutationReceipt) error {
	if head == nil || receipt == nil || receipt.ID != head.CurrentReceiptID || receipt.RequestID != head.RequestID ||
		receipt.WriterEpoch != head.WriterEpoch || receipt.UserID != head.UserID || receipt.TokenID != head.TokenID ||
		receipt.SubscriptionID != head.SubscriptionID || receipt.BillingSource != head.BillingSource ||
		receipt.ReserveFingerprint != head.ReserveFingerprint || receipt.AppliedQuota != head.AppliedQuota {
		return ErrAccountQuotaMutationConflict
	}
	recomputed, err := accountQuotaReceiptFingerprint(receipt)
	if err != nil || recomputed != receipt.RequestFingerprint {
		return ErrAccountQuotaMutationConflict
	}
	return nil
}

func accountQuotaTerminalFingerprint(input AccountQuotaTerminalInput, phase string, head *AccountQuotaReservationHead, current *AccountQuotaMutationReceipt) (string, error) {
	if head == nil || current == nil || current.ID != head.CurrentReceiptID || current.ReserveFingerprint != head.ReserveFingerprint {
		return "", ErrAccountQuotaMutationConflict
	}
	return accountQuotaFingerprint(accountQuotaFingerprintPayload{
		Version: AccountQuotaFingerprintVersion, RequestID: input.RequestID, Phase: phase,
		ReserveReceiptID: current.ID, ReserveFingerprint: head.ReserveFingerprint,
		UserID: head.UserID, TokenID: head.TokenID, ActualQuota: input.ActualQuota,
		Preference: current.BillingPreference, Playground: current.BillingContext.Playground,
		AuditKey: input.AuditKey, BillingContext: current.BillingContext,
	})
}

func terminalAccountQuotaDeltas(reserve *AccountQuotaMutationReceipt, actualQuota int64, phase string) (AccountQuotaDeltas, int64, error) {
	if phase == AccountQuotaPhaseRefund {
		deltas := accountQuotaRequestedDeltas(reserve.BillingSource, reserve.AppliedQuota, reserve.BillingContext.Playground)
		return AccountQuotaDeltas{
			UserQuota: -deltas.UserQuota, TokenRemainQuota: -deltas.TokenRemainQuota,
			TokenUsedQuota: -deltas.TokenUsedQuota, SubscriptionAmountUsed: -deltas.SubscriptionAmountUsed,
		}, 0, nil
	}
	delta := actualQuota - reserve.AppliedQuota
	return accountQuotaRequestedDeltas(reserve.BillingSource, delta, reserve.BillingContext.Playground), actualQuota, nil
}

func applyAccountQuotaTerminal(ctx context.Context, db *gorm.DB, input AccountQuotaTerminalInput, phase string) (*AccountQuotaMutationReceipt, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	ctx, cancel := accountQuotaOperationContext(ctx, 5*time.Second)
	defer cancel()
	db = db.WithContext(ctx)
	normalized, err := normalizeAccountTerminalInput(input, phase)
	if err != nil {
		return nil, err
	}
	initialState, err := GetQuotaWriterEpochState(db)
	if err != nil {
		return nil, err
	}
	initialMode := QuotaWriterMode(initialState.Mode)
	if initialMode != QuotaWriterModeAuthoritative && initialMode != QuotaWriterModeBridge {
		return nil, ErrQuotaProjectionModeDisabled
	}
	if initialMode == QuotaWriterModeAuthoritative {
		if _, err := requireDurableQuotaWriterEpoch(db); err != nil {
			return nil, err
		}
	}
	eventKey := accountQuotaEventKey(normalized.RequestID, phase)
	head, err := findAccountQuotaReservationHead(db, normalized.RequestID)
	if err != nil {
		return nil, err
	}
	var current AccountQuotaMutationReceipt
	if err := db.Where("id = ?", head.CurrentReceiptID).First(&current).Error; err != nil {
		return nil, err
	}
	if err := validateAccountQuotaHeadReceipt(head, &current); err != nil {
		return nil, err
	}
	fingerprint, err := accountQuotaTerminalFingerprint(normalized, phase, head, &current)
	if err != nil {
		return nil, err
	}
	if existing, replayErr := accountQuotaReplay(db, eventKey, fingerprint); replayErr != nil || existing != nil {
		if existing != nil && existing.WriterEpoch != initialState.Epoch {
			return nil, ErrQuotaWriterEpochMismatch
		}
		return existing, replayErr
	}

	receipt, err := accountQuotaTransaction(ctx, db, eventKey, &fingerprint, func(tx *gorm.DB) (*AccountQuotaMutationReceipt, error) {
		state, err := GetQuotaWriterEpochState(tx)
		if err != nil {
			return nil, err
		}
		mode := QuotaWriterMode(state.Mode)
		if mode != QuotaWriterModeAuthoritative && mode != QuotaWriterModeBridge {
			return nil, ErrQuotaProjectionModeDisabled
		}
		if mode == QuotaWriterModeAuthoritative {
			if _, err := requireDurableQuotaWriterEpoch(tx); err != nil {
				return nil, err
			}
		}
		observedHead, err := findAccountQuotaReservationHead(tx, normalized.RequestID)
		if err != nil {
			return nil, err
		}
		var user User
		if err := lockForUpdate(tx).Where("id = ?", observedHead.UserID).First(&user).Error; err != nil {
			return nil, err
		}
		var token Token
		if err := lockForUpdate(tx).Where("id = ?", observedHead.TokenID).First(&token).Error; err != nil {
			return nil, err
		}
		var subscription *UserSubscription
		if observedHead.SubscriptionID > 0 {
			var locked UserSubscription
			if err := lockForUpdate(tx).Where("id = ?", observedHead.SubscriptionID).First(&locked).Error; err != nil {
				return nil, err
			}
			subscription = &locked
		}
		var head AccountQuotaReservationHead
		if err := lockForUpdate(tx).Where("id = ?", observedHead.ID).First(&head).Error; err != nil {
			return nil, err
		}
		if head.WriterEpoch != state.Epoch {
			return nil, ErrQuotaWriterEpochMismatch
		}
		if head.UserID != user.Id || token.UserId != user.Id ||
			head.TokenID != token.Id || head.LockVersion <= 0 || !quotaMutationVersionValid(user.QuotaVersion) ||
			!quotaMutationVersionValid(token.QuotaVersion) ||
			(subscription != nil && (subscription.UserId != user.Id || subscription.Id != head.SubscriptionID || !quotaMutationVersionValid(subscription.QuotaVersion))) {
			return nil, ErrAccountQuotaMutationIneligible
		}
		inChain, err := accountQuotaReceiptInChain(tx, &head, normalized.ReserveReceiptID)
		if err != nil {
			return nil, err
		}
		if !inChain {
			return nil, ErrAccountQuotaMutationStaleReceipt
		}
		var current AccountQuotaMutationReceipt
		if err := lockForUpdate(tx).Where("id = ?", head.CurrentReceiptID).First(&current).Error; err != nil {
			return nil, err
		}
		if err := validateAccountQuotaHeadReceipt(&head, &current); err != nil {
			return nil, err
		}
		fingerprint, err = accountQuotaTerminalFingerprint(normalized, phase, &head, &current)
		if err != nil {
			return nil, err
		}
		if existing, replayErr := accountQuotaReplay(tx, eventKey, fingerprint); replayErr != nil || existing != nil {
			return existing, replayErr
		}
		if head.TerminalReceiptID > 0 {
			var terminal AccountQuotaMutationReceipt
			if err := tx.Where("id = ?", head.TerminalReceiptID).First(&terminal).Error; err != nil {
				return nil, err
			}
			if terminal.EventKey == eventKey && terminal.RequestFingerprint == fingerprint {
				return &terminal, nil
			}
			return nil, ErrAccountQuotaMutationTerminal
		}
		now, err := taskRecoveryDBTimestamp(tx)
		if err != nil {
			return nil, err
		}
		deltas, appliedQuota, err := terminalAccountQuotaDeltas(&current, normalized.ActualQuota, phase)
		if err != nil {
			return nil, err
		}
		before := quotaMutationSnapshot(&user, &token, subscription)
		after, err := accountQuotaAfter(before, deltas, now)
		if err != nil {
			return nil, err
		}
		if _, err := requireQuotaProjectionEpoch(tx, current.WriterEpoch); err != nil {
			return nil, err
		}
		if err := applyAccountQuotaBalances(tx, before, after, deltas); err != nil {
			return nil, err
		}
		receipt := &AccountQuotaMutationReceipt{
			ReceiptVersion: AccountQuotaMutationReceiptVersion, FingerprintVersion: AccountQuotaFingerprintVersion, WriterEpoch: current.WriterEpoch, RequestID: normalized.RequestID,
			Phase: phase, EventKey: eventKey, MutationSlot: accountQuotaMutationSlot(normalized.RequestID, phase), ParentReceiptID: current.ID,
			AuditKey: normalized.AuditKey, ReserveFingerprint: head.ReserveFingerprint, RequestFingerprint: fingerprint,
			BillingSource: current.BillingSource, BillingPreference: current.BillingPreference,
			UserID: current.UserID, TokenID: current.TokenID, SubscriptionID: current.SubscriptionID,
			RequestedQuota: normalized.ActualQuota, AppliedQuota: appliedQuota, RequestedDeltas: deltas, AppliedDeltas: deltas,
			Before: before, After: after, BillingContext: current.BillingContext,
		}
		if err := accountQuotaReceiptCreateDB(tx).Create(receipt).Error; err != nil {
			return nil, err
		}
		result := tx.Model(&AccountQuotaReservationHead{}).
			Where("id = ? AND lock_version = ? AND current_receipt_id = ? AND terminal_receipt_id = 0", head.ID, head.LockVersion, head.CurrentReceiptID).
			Updates(map[string]interface{}{"terminal_receipt_id": receipt.ID, "lock_version": head.LockVersion + 1, "updated_at": now})
		if result.Error != nil {
			return nil, result.Error
		}
		if result.RowsAffected != 1 {
			return nil, ErrAccountQuotaMutationCASLost
		}
		if err := closeAccountQuotaLifecycle(tx, &head, normalized, phase, receipt, now); err != nil {
			return nil, err
		}
		if _, err := ensureQuotaProjectionObligation(tx, receipt); err != nil {
			return nil, err
		}
		return receipt, nil
	})
	if err != nil {
		if terminal, terminalErr := FindAccountQuotaTerminalReceipt(db, normalized.RequestID); terminalErr == nil && terminal != nil {
			if terminal.EventKey == eventKey && fingerprint != "" && terminal.RequestFingerprint == fingerprint {
				return terminal, nil
			}
			return nil, ErrAccountQuotaMutationTerminal
		}
		return nil, err
	}
	if receipt != nil {
		projectAccountQuotaReceiptDetached(ctx, db, receipt)
	}
	return receipt, nil
}

func SettleAccountQuota(ctx context.Context, db *gorm.DB, input AccountQuotaTerminalInput) (*AccountQuotaMutationReceipt, error) {
	return applyAccountQuotaTerminal(ctx, db, input, AccountQuotaPhaseSettle)
}

func RefundAccountQuota(ctx context.Context, db *gorm.DB, input AccountQuotaTerminalInput) (*AccountQuotaMutationReceipt, error) {
	input.ActualQuota = 0
	return applyAccountQuotaTerminal(ctx, db, input, AccountQuotaPhaseRefund)
}

// InspectAccountQuotaRecovery is read-only. It never infers an upstream result.
func InspectAccountQuotaRecovery(db *gorm.DB, requestID string) (*AccountQuotaMutationReceipt, *AccountQuotaMutationReceipt, error) {
	requestID, err := normalizeAccountRequestID(requestID)
	if err != nil {
		return nil, nil, err
	}
	head, err := findAccountQuotaReservationHead(db, requestID)
	if errors.Is(err, ErrAccountQuotaMutationNotFound) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	var reserve AccountQuotaMutationReceipt
	if err := db.Where("id = ?", head.CurrentReceiptID).First(&reserve).Error; err != nil {
		return nil, nil, err
	}
	terminal, err := FindAccountQuotaTerminalReceipt(db, requestID)
	return &reserve, terminal, err
}

func (receipt *AccountQuotaMutationReceipt) quotaProjectionDescriptor() quotaProjectionReceiptDescriptor {
	if receipt == nil {
		return quotaProjectionReceiptDescriptor{}
	}
	return quotaProjectionReceiptDescriptor{
		Kind: quotaProjectionReceiptKindAccount, ID: receipt.ID, EventKey: receipt.EventKey,
		RequestID: receipt.RequestID, CorrelationKey: receipt.EventKey, WriterEpoch: receipt.WriterEpoch,
		UserID: receipt.UserID, TokenID: receipt.TokenID,
		ExpectedUserVersion: receipt.After.User.QuotaVersion, ExpectedTokenVersion: receipt.After.Token.QuotaVersion,
	}
}

// ExtendAccountQuotaReservation raises an existing reservation to targetQuota.
// The returned receipt carries the new cumulative AppliedQuota while its
// AppliedDeltas contain only the additional atomic mutation.
func ExtendAccountQuotaReservation(ctx context.Context, db *gorm.DB, reserveReceiptID int64, targetQuota int64) (*AccountQuotaMutationReceipt, error) {
	if db == nil || reserveReceiptID <= 0 || validateAccountQuotaValue(targetQuota) != nil {
		return nil, ErrAccountQuotaMutationInvalidInput
	}
	ctx, cancel := accountQuotaOperationContext(ctx, 5*time.Second)
	defer cancel()
	db = db.WithContext(ctx)
	var supplied AccountQuotaMutationReceipt
	if err := db.Where("id = ?", reserveReceiptID).First(&supplied).Error; err != nil {
		return nil, err
	}
	if supplied.Phase != AccountQuotaPhaseReserve && supplied.Phase != AccountQuotaPhaseAdjust {
		return nil, ErrAccountQuotaMutationInvalidInput
	}
	head, err := findAccountQuotaReservationHead(db, supplied.RequestID)
	if err != nil {
		return nil, err
	}
	initialState, err := requireDurableQuotaWriterEpoch(db)
	if err != nil {
		return nil, err
	}
	if initialState.Epoch != head.WriterEpoch {
		return nil, ErrQuotaWriterEpochMismatch
	}
	var current AccountQuotaMutationReceipt
	if err := db.Where("id = ?", head.CurrentReceiptID).First(&current).Error; err != nil {
		return nil, err
	}
	if targetQuota <= head.AppliedQuota {
		inChain, chainErr := accountQuotaReceiptInChain(db, head, reserveReceiptID)
		if chainErr != nil {
			return nil, chainErr
		}
		if !inChain {
			return nil, ErrAccountQuotaMutationStaleReceipt
		}
		return &current, nil
	}
	eventKey := fmt.Sprintf("request:%s:reserve-adjust-%d:v1", head.RequestID, targetQuota)
	fingerprint, err := accountQuotaFingerprint(accountQuotaFingerprintPayload{
		Version: AccountQuotaFingerprintVersion, RequestID: head.RequestID, Phase: AccountQuotaPhaseAdjust,
		ReserveFingerprint: head.ReserveFingerprint, UserID: head.UserID, TokenID: head.TokenID, RequestedQuota: targetQuota,
		Preference: current.BillingPreference, Playground: current.BillingContext.Playground, BillingContext: current.BillingContext,
	})
	if err != nil {
		return nil, err
	}
	if existing, replayErr := accountQuotaReplay(db, eventKey, fingerprint); replayErr != nil || existing != nil {
		return existing, replayErr
	}

	receipt, err := accountQuotaTransaction(ctx, db, eventKey, &fingerprint, func(tx *gorm.DB) (*AccountQuotaMutationReceipt, error) {
		state, err := GetQuotaWriterEpochState(tx)
		if err != nil {
			return nil, err
		}
		if QuotaWriterMode(state.Mode) != QuotaWriterModeAuthoritative || state.Epoch != head.WriterEpoch {
			return nil, ErrDurableQuotaWriterModeDisabled
		}
		observedHead, err := findAccountQuotaReservationHead(tx, head.RequestID)
		if err != nil {
			return nil, err
		}
		var user User
		if err := lockForUpdate(tx).Where("id = ?", observedHead.UserID).First(&user).Error; err != nil {
			return nil, err
		}
		var token Token
		if err := lockForUpdate(tx).Where("id = ?", observedHead.TokenID).First(&token).Error; err != nil {
			return nil, err
		}
		var subscription *UserSubscription
		if observedHead.SubscriptionID > 0 {
			var locked UserSubscription
			if err := lockForUpdate(tx).Where("id = ?", observedHead.SubscriptionID).First(&locked).Error; err != nil {
				return nil, err
			}
			subscription = &locked
		}
		var lockedHead AccountQuotaReservationHead
		if err := lockForUpdate(tx).Where("id = ?", observedHead.ID).First(&lockedHead).Error; err != nil {
			return nil, err
		}
		if lockedHead.TerminalReceiptID > 0 {
			return nil, ErrAccountQuotaMutationTerminal
		}
		inChain, err := accountQuotaReceiptInChain(tx, &lockedHead, reserveReceiptID)
		if err != nil {
			return nil, err
		}
		if !inChain {
			return nil, ErrAccountQuotaMutationStaleReceipt
		}
		var current AccountQuotaMutationReceipt
		if err := lockForUpdate(tx).Where("id = ?", lockedHead.CurrentReceiptID).First(&current).Error; err != nil {
			return nil, err
		}
		if err := validateAccountQuotaHeadReceipt(&lockedHead, &current); err != nil {
			return nil, err
		}
		if existing, replayErr := accountQuotaReplay(tx, eventKey, fingerprint); replayErr != nil || existing != nil {
			return existing, replayErr
		}
		if targetQuota <= lockedHead.AppliedQuota {
			return &current, nil
		}
		delta := targetQuota - lockedHead.AppliedQuota
		if !current.BillingContext.Playground && !token.UnlimitedQuota && int64(token.RemainQuota) < delta {
			return nil, ErrAccountQuotaMutationInsufficient
		}
		switch lockedHead.BillingSource {
		case "wallet":
			if int64(user.Quota) < delta {
				return nil, ErrAccountQuotaMutationInsufficient
			}
		case "subscription":
			if subscription == nil || (subscription.AmountTotal > 0 && subscription.AmountUsed > subscription.AmountTotal-delta) {
				return nil, ErrAccountQuotaMutationInsufficient
			}
		case "free":
			return nil, ErrAccountQuotaMutationInvalidInput
		default:
			return nil, ErrAccountQuotaMutationInvalidInput
		}
		now, err := taskRecoveryDBTimestamp(tx)
		if err != nil {
			return nil, err
		}
		deltas := accountQuotaRequestedDeltas(lockedHead.BillingSource, delta, current.BillingContext.Playground)
		before := quotaMutationSnapshot(&user, &token, subscription)
		after, err := accountQuotaAfter(before, deltas, now)
		if err != nil {
			return nil, err
		}
		if _, err := requireQuotaProjectionEpoch(tx, lockedHead.WriterEpoch); err != nil {
			return nil, err
		}
		if err := applyAccountQuotaBalances(tx, before, after, deltas); err != nil {
			return nil, err
		}
		receipt := &AccountQuotaMutationReceipt{
			ReceiptVersion: AccountQuotaMutationReceiptVersion, FingerprintVersion: AccountQuotaFingerprintVersion, WriterEpoch: lockedHead.WriterEpoch,
			RequestID: lockedHead.RequestID, Phase: AccountQuotaPhaseAdjust, EventKey: eventKey, MutationSlot: eventKey,
			ParentReceiptID: current.ID, ReserveFingerprint: lockedHead.ReserveFingerprint, RequestFingerprint: fingerprint,
			BillingSource: current.BillingSource, BillingPreference: current.BillingPreference,
			UserID: current.UserID, TokenID: current.TokenID, SubscriptionID: current.SubscriptionID,
			RequestedQuota: targetQuota, AppliedQuota: targetQuota, RequestedDeltas: deltas, AppliedDeltas: deltas,
			Before: before, After: after, BillingContext: current.BillingContext,
		}
		if err := accountQuotaReceiptCreateDB(tx).Create(receipt).Error; err != nil {
			return nil, err
		}
		result := tx.Model(&AccountQuotaReservationHead{}).
			Where("id = ? AND lock_version = ? AND current_receipt_id = ? AND terminal_receipt_id = 0", lockedHead.ID, lockedHead.LockVersion, lockedHead.CurrentReceiptID).
			Updates(map[string]interface{}{"current_receipt_id": receipt.ID, "applied_quota": targetQuota, "lock_version": lockedHead.LockVersion + 1, "updated_at": now})
		if result.Error != nil {
			return nil, result.Error
		}
		if result.RowsAffected != 1 {
			return nil, ErrAccountQuotaMutationCASLost
		}
		if err := updateAccountQuotaLifecycleReservation(tx, &lockedHead, receipt, now); err != nil {
			return nil, err
		}
		if _, err := ensureQuotaProjectionObligation(tx, receipt); err != nil {
			return nil, err
		}
		return receipt, nil
	})
	if err != nil {
		return nil, err
	}
	if receipt != nil {
		projectAccountQuotaReceiptDetached(ctx, db, receipt)
	}
	return receipt, nil
}

// MutateTokenQuotaAuthoritative applies one token remain/used delta with an
// immutable account receipt. tokenRemainDelta is mirrored to used quota.
func MutateTokenQuotaAuthoritative(ctx context.Context, db *gorm.DB, eventKey string, userID, tokenID int, tokenRemainDelta int64) (*AccountQuotaMutationReceipt, error) {
	eventKey = strings.TrimSpace(eventKey)
	if db == nil || eventKey == "" || len(eventKey) > 64 || userID <= 0 || tokenID <= 0 || tokenRemainDelta == 0 ||
		tokenRemainDelta < int64(common.MinQuota) || tokenRemainDelta > int64(common.MaxQuota) {
		return nil, ErrAccountQuotaMutationInvalidInput
	}
	ctx, cancel := accountQuotaOperationContext(ctx, 5*time.Second)
	defer cancel()
	db = db.WithContext(ctx)
	fingerprint, err := accountQuotaFingerprint(accountQuotaFingerprintPayload{
		Version: AccountQuotaFingerprintVersion, RequestID: eventKey, Phase: "direct_token",
		UserID: userID, TokenID: tokenID, ActualQuota: tokenRemainDelta,
	})
	if err != nil {
		return nil, err
	}
	initialState, err := requireDurableQuotaWriterEpoch(db)
	if err != nil {
		return nil, err
	}
	if existing, err := accountQuotaReplay(db, eventKey, fingerprint); err != nil || existing != nil {
		if existing != nil && existing.WriterEpoch != initialState.Epoch {
			return nil, ErrQuotaWriterEpochMismatch
		}
		return existing, err
	}
	var receipt *AccountQuotaMutationReceipt
	err = db.Transaction(func(tx *gorm.DB) error {
		state, err := requireDurableQuotaWriterEpoch(tx)
		if err != nil {
			return err
		}
		if existing, err := accountQuotaReplay(tx, eventKey, fingerprint); err != nil || existing != nil {
			receipt = existing
			return err
		}
		now, err := taskRecoveryDBTimestamp(tx)
		if err != nil {
			return err
		}
		var user User
		if err := lockForUpdate(tx).Where("id = ?", userID).First(&user).Error; err != nil {
			return err
		}
		var token Token
		if err := lockForUpdate(tx).Where("id = ?", tokenID).First(&token).Error; err != nil {
			return err
		}
		if token.UserId != user.Id || !quotaMutationVersionValid(user.QuotaVersion) || !quotaMutationVersionValid(token.QuotaVersion) {
			return ErrAccountQuotaMutationIneligible
		}
		if tokenRemainDelta < 0 && !token.UnlimitedQuota && int64(token.RemainQuota) < -tokenRemainDelta {
			return ErrAccountQuotaMutationInsufficient
		}
		deltas := AccountQuotaDeltas{TokenRemainQuota: tokenRemainDelta, TokenUsedQuota: -tokenRemainDelta}
		before := quotaMutationSnapshot(&user, &token, nil)
		after, err := accountQuotaAfter(before, deltas, now)
		if err != nil {
			return err
		}
		if _, err := requireQuotaProjectionEpoch(tx, state.Epoch); err != nil {
			return err
		}
		if err := applyAccountQuotaBalances(tx, before, after, deltas); err != nil {
			return err
		}
		receipt = &AccountQuotaMutationReceipt{
			ReceiptVersion: AccountQuotaMutationReceiptVersion, FingerprintVersion: AccountQuotaFingerprintVersion, WriterEpoch: state.Epoch, RequestID: eventKey,
			Phase: "direct_token", EventKey: eventKey, MutationSlot: eventKey, ReserveFingerprint: fingerprint, RequestFingerprint: fingerprint,
			BillingSource: "token", BillingPreference: "direct", UserID: userID, TokenID: tokenID,
			RequestedQuota: tokenRemainDelta, AppliedQuota: tokenRemainDelta, RequestedDeltas: deltas, AppliedDeltas: deltas,
			Before: before, After: after, BillingContext: AccountBillingContext{Version: 1},
		}
		if err := accountQuotaReceiptCreateDB(tx).Create(receipt).Error; err != nil {
			return err
		}
		_, err = ensureQuotaProjectionObligation(tx, receipt)
		return err
	})
	if err != nil {
		if replay, replayErr := accountQuotaReplay(db, eventKey, fingerprint); replayErr == nil && replay != nil {
			return replay, nil
		}
		return nil, err
	}
	projectAccountQuotaReceiptDetached(ctx, db, receipt)
	return receipt, nil
}

const (
	accountQuotaMigrationBatchSize = 200
	accountQuotaMigrationRunBudget = 1000
)

var accountQuotaMigrationBatchHook func(int)

func backfillAccountQuotaReceiptChain(db *gorm.DB, requestID string) (*AccountQuotaMutationReceipt, *AccountQuotaMutationReceipt, *AccountQuotaMutationReceipt, error) {
	var root *AccountQuotaMutationReceipt
	var current *AccountQuotaMutationReceipt
	var terminal *AccountQuotaMutationReceipt
	var reserveFingerprint string
	var cursor int64
	for {
		var receipts []AccountQuotaMutationReceipt
		if err := db.Where("request_id = ? AND id > ?", requestID, cursor).Order("id ASC").Limit(accountQuotaMigrationBatchSize).Find(&receipts).Error; err != nil {
			return nil, nil, nil, err
		}
		if accountQuotaMigrationBatchHook != nil {
			accountQuotaMigrationBatchHook(len(receipts))
		}
		if len(receipts) == 0 {
			break
		}
		for index := range receipts {
			receipt := receipts[index]
			cursor = receipt.ID
			migrated := receipt
			migrated.FingerprintVersion = AccountQuotaFingerprintVersion
			if receipt.Phase == AccountQuotaPhaseReserve {
				if root != nil {
					return nil, nil, nil, ErrAccountQuotaMutationConflict
				}
				migrated.ReserveFingerprint = ""
				fingerprint, err := accountQuotaReceiptFingerprint(&migrated)
				if err != nil {
					return nil, nil, nil, err
				}
				reserveFingerprint = fingerprint
				migrated.ReserveFingerprint = fingerprint
				migrated.RequestFingerprint = fingerprint
				rootCopy := migrated
				root = &rootCopy
			} else {
				if reserveFingerprint == "" {
					return nil, nil, nil, ErrAccountQuotaMutationConflict
				}
				migrated.ReserveFingerprint = reserveFingerprint
				fingerprint, err := accountQuotaReceiptFingerprint(&migrated)
				if err != nil {
					return nil, nil, nil, err
				}
				migrated.RequestFingerprint = fingerprint
			}
			if receipt.FingerprintVersion == AccountQuotaFingerprintVersion {
				if receipt.ReserveFingerprint != migrated.ReserveFingerprint || receipt.RequestFingerprint != migrated.RequestFingerprint {
					return nil, nil, nil, ErrAccountQuotaMutationConflict
				}
			} else {
				result := db.Session(&gorm.Session{SkipHooks: true}).Table((AccountQuotaMutationReceipt{}).TableName()).
					Where("id = ? AND fingerprint_version = ?", receipt.ID, receipt.FingerprintVersion).
					Updates(map[string]interface{}{
						"fingerprint_version": AccountQuotaFingerprintVersion,
						"reserve_fingerprint": migrated.ReserveFingerprint,
						"request_fingerprint": migrated.RequestFingerprint,
					})
				if result.Error != nil {
					return nil, nil, nil, result.Error
				}
				if result.RowsAffected != 1 {
					return nil, nil, nil, ErrAccountQuotaMutationCASLost
				}
			}
			switch migrated.Phase {
			case AccountQuotaPhaseReserve, AccountQuotaPhaseAdjust:
				copyReceipt := migrated
				current = &copyReceipt
			case AccountQuotaPhaseSettle, AccountQuotaPhaseRefund:
				copyReceipt := migrated
				terminal = &copyReceipt
			}
		}
	}
	if root == nil || current == nil {
		return nil, nil, nil, ErrAccountQuotaMutationConflict
	}
	return root, current, terminal, nil
}

// InitializeAccountQuotaReservationHeadsWithDB backfills receipt fingerprints,
// reservation heads, and lifecycle obligations in bounded keyset pages.
func InitializeAccountQuotaReservationHeadsWithDB(db *gorm.DB) error {
	if db == nil {
		return gorm.ErrInvalidDB
	}
	if !db.Migrator().HasTable(&AccountQuotaMutationReceipt{}) || !db.Migrator().HasTable(&AccountQuotaReservationHead{}) {
		return nil
	}
	baseCtx := context.Background()
	if db.Statement != nil && db.Statement.Context != nil {
		baseCtx = db.Statement.Context
	}
	ctx, cancel := context.WithTimeout(baseCtx, 5*time.Second)
	defer cancel()
	workCursor, err := loadQuotaWorkCursor(ctx, db, quotaWorkCursorAccountMigration)
	if err != nil {
		return err
	}
	if workCursor.Complete {
		return nil
	}
	db = db.WithContext(ctx)
	cursor := workCursor.LastID
	processed := 0
	for processed < accountQuotaMigrationRunBudget {
		var roots []AccountQuotaMutationReceipt
		remaining := accountQuotaMigrationRunBudget - processed
		batchSize := min(accountQuotaMigrationBatchSize, remaining)
		if err := db.Where("phase = ? AND id > ?", AccountQuotaPhaseReserve, cursor).
			Order("id ASC").Limit(batchSize).Find(&roots).Error; err != nil {
			return err
		}
		if accountQuotaMigrationBatchHook != nil {
			accountQuotaMigrationBatchHook(len(roots))
		}
		if len(roots) == 0 {
			workCursor.Complete = true
			workCursor.LastID = cursor
			return saveQuotaWorkCursor(ctx, db, workCursor)
		}
		for index := range roots {
			cursor = roots[index].ID
			root, current, terminal, err := backfillAccountQuotaReceiptChain(db, roots[index].RequestID)
			if err != nil {
				return err
			}
			now := current.CreatedAt
			if now <= 0 {
				now, err = taskRecoveryDBTimestamp(db)
				if err != nil {
					return err
				}
			}
			head := AccountQuotaReservationHead{
				RequestID: root.RequestID, WriterEpoch: root.WriterEpoch, RootReceiptID: root.ID, CurrentReceiptID: current.ID,
				ReserveFingerprint: root.ReserveFingerprint, UserID: root.UserID, TokenID: root.TokenID, SubscriptionID: root.SubscriptionID,
				BillingSource: root.BillingSource, AppliedQuota: current.AppliedQuota, LockVersion: 1, CreatedAt: now, UpdatedAt: now,
			}
			if terminal != nil {
				head.TerminalReceiptID = terminal.ID
			}
			var existing AccountQuotaReservationHead
			result := db.Where("request_id = ?", root.RequestID).Limit(1).Find(&existing)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				if err := db.Create(&head).Error; err != nil {
					if replayErr := db.Where("request_id = ?", root.RequestID).First(&existing).Error; replayErr != nil {
						return err
					}
				} else {
					existing = head
				}
			} else {
				updates := map[string]interface{}{}
				if existing.LockVersion < 1 {
					updates["lock_version"] = int64(1)
				}
				if existing.ReserveFingerprint != root.ReserveFingerprint {
					updates["reserve_fingerprint"] = root.ReserveFingerprint
				}
				if len(updates) > 0 {
					if err := db.Model(&AccountQuotaReservationHead{}).Where("id = ?", existing.ID).Updates(updates).Error; err != nil {
						return err
					}
				}
			}
			var obligation AccountQuotaTerminalRecoveryObligation
			obligationResult := db.Where("request_id = ?", root.RequestID).Limit(1).Find(&obligation)
			if obligationResult.Error != nil {
				return obligationResult.Error
			}
			if obligationResult.RowsAffected == 0 {
				obligation = AccountQuotaTerminalRecoveryObligation{
					RequestID: root.RequestID, ReserveReceiptID: current.ID, WriterEpoch: root.WriterEpoch,
					RequestFingerprint: root.ReserveFingerprint, State: AccountQuotaTerminalRecoveryOpen,
					LockVersion: 1, CreatedAt: now, UpdatedAt: now,
				}
				if terminal != nil {
					obligation.Phase = terminal.Phase
					obligation.AuditKey = terminal.AuditKey
					obligation.ActualQuota = terminal.RequestedQuota
					obligation.RequestFingerprint = terminal.RequestFingerprint
					obligation.State = AccountQuotaTerminalRecoveryApplied
					obligation.TerminalReceiptID = terminal.ID
				}
				if err := db.Create(&obligation).Error; err != nil {
					var replay AccountQuotaTerminalRecoveryObligation
					if replayErr := db.Where("request_id = ?", root.RequestID).First(&replay).Error; replayErr != nil {
						return err
					}
				}
			} else {
				updates := map[string]interface{}{}
				if obligation.State == accountQuotaTerminalRecoveryLegacyPending {
					updates["state"] = AccountQuotaTerminalRecoveryRefundPending
				}
				if obligation.State == accountQuotaTerminalRecoveryLegacyPending || obligation.State == AccountQuotaTerminalRecoveryRefundPending ||
					obligation.State == AccountQuotaTerminalRecoveryRetryable || obligation.State == AccountQuotaTerminalRecoveryClaimed {
					fingerprint, fingerprintErr := accountQuotaTerminalRecoveryFingerprint(AccountQuotaTerminalInput{
						RequestID: obligation.RequestID, ReserveReceiptID: obligation.ReserveReceiptID, AuditKey: obligation.AuditKey,
					}, obligation.WriterEpoch)
					if fingerprintErr != nil {
						return fingerprintErr
					}
					if obligation.RequestFingerprint != fingerprint {
						updates["request_fingerprint"] = fingerprint
					}
				}
				if obligation.State == AccountQuotaTerminalRecoveryOpen && obligation.RequestFingerprint != root.ReserveFingerprint {
					updates["request_fingerprint"] = root.ReserveFingerprint
				}
				if obligation.State == AccountQuotaTerminalRecoveryApplied && terminal != nil && obligation.RequestFingerprint != terminal.RequestFingerprint {
					updates["request_fingerprint"] = terminal.RequestFingerprint
				}
				if len(updates) > 0 {
					if err := db.Model(&AccountQuotaTerminalRecoveryObligation{}).Where("id = ?", obligation.ID).Updates(updates).Error; err != nil {
						return err
					}
				}
			}
			processed++
		}
		workCursor.LastID = cursor
		if err := saveQuotaWorkCursor(ctx, db, workCursor); err != nil {
			return err
		}
	}
	var nextRoot AccountQuotaMutationReceipt
	result := db.Select("id").Where("phase = ? AND id > ?", AccountQuotaPhaseReserve, cursor).Limit(1).Find(&nextRoot)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		workCursor.Complete = true
		return saveQuotaWorkCursor(ctx, db, workCursor)
	}
	return ErrQuotaWorkIncomplete
}
