package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/ForceMind/MyAPI/common"
	"gorm.io/gorm"
)

const (
	AccountQuotaExecutionGateTableName        = "account_quota_execution_gates"
	AccountQuotaExecutionPermitTableName      = "account_quota_execution_permits"
	AccountQuotaExecutionGateSingletonID      = 1
	AccountQuotaExecutionGateSchemaVersion    = 1
	AccountQuotaExecutionGateStateEnforcing   = "enforcing"
	AccountQuotaExecutionPermitBindingVersion = 1

	AccountQuotaExecutionPermitSourceRuntimeV1          = "runtime_v1"
	AccountQuotaExecutionPermitSourceHistoricalReviewV1 = "historical_review_v1"
	AccountQuotaExecutionPermitStateActive              = "active"
	AccountQuotaExecutionPermitStateRevoked             = "revoked"
)

var (
	ErrAccountQuotaExecutionGateUninitialized = errors.New("account quota execution gate is not initialized")
	ErrAccountQuotaExecutionGateInvalid       = errors.New("account quota execution gate is invalid")
	ErrAccountQuotaExecutionFrozen            = errors.New("account quota settlement is frozen without a valid execution permit")
)

type AccountQuotaExecutionGate struct {
	ID            int    `json:"id" gorm:"primaryKey;autoIncrement:false"`
	SchemaVersion int    `json:"schema_version" gorm:"not null"`
	State         string `json:"state" gorm:"type:varchar(16);not null"`
	CreatedAt     int64  `json:"created_at" gorm:"type:bigint;not null"`
	UpdatedAt     int64  `json:"updated_at" gorm:"type:bigint;not null"`
}

func (AccountQuotaExecutionGate) TableName() string { return AccountQuotaExecutionGateTableName }

// AccountQuotaExecutionPermit is the immutable authorization boundary for one
// settlement identity. FactID is populated exactly once when an intent first
// materializes its fact.
type AccountQuotaExecutionPermit struct {
	ID                    int64  `json:"id" gorm:"primaryKey"`
	EventKey              string `json:"event_key" gorm:"type:varchar(128);not null;uniqueIndex"`
	IntentID              int64  `json:"intent_id" gorm:"type:bigint;not null;default:0;index"`
	FactID                int64  `json:"fact_id" gorm:"type:bigint;not null;default:0;index"`
	Source                string `json:"source" gorm:"type:varchar(32);not null;index"`
	BindingVersion        int    `json:"binding_version" gorm:"not null"`
	BindingDigest         string `json:"binding_digest" gorm:"type:char(64);not null"`
	RequestID             string `json:"request_id" gorm:"type:varchar(64);not null;index"`
	RequestFingerprint    string `json:"request_fingerprint" gorm:"type:char(64);not null"`
	Kind                  string `json:"kind" gorm:"type:varchar(32);not null;index"`
	UserID                int    `json:"user_id" gorm:"not null;index"`
	TokenID               int    `json:"token_id" gorm:"not null;index"`
	SubscriptionID        int    `json:"subscription_id" gorm:"not null;default:0;index"`
	ReserveReceiptID      int64  `json:"reserve_receipt_id" gorm:"type:bigint;not null;default:0;index"`
	WriterEpoch           int64  `json:"writer_epoch" gorm:"type:bigint;not null;default:0"`
	ActualQuota           int64  `json:"actual_quota" gorm:"type:bigint;not null;default:0"`
	Delta                 int64  `json:"delta" gorm:"type:bigint;not null"`
	ApplyToken            bool   `json:"apply_token" gorm:"not null"`
	InitialFundingApplied bool   `json:"initial_funding_applied" gorm:"not null"`
	InitialTokenApplied   bool   `json:"initial_token_applied" gorm:"not null"`
	AllowFunding          bool   `json:"allow_funding" gorm:"not null"`
	AllowToken            bool   `json:"allow_token" gorm:"not null"`
	State                 string `json:"state" gorm:"type:varchar(16);not null;index"`
	ApprovalID            string `json:"approval_id" gorm:"type:varchar(128);not null"`
	EvidenceDigest        string `json:"evidence_digest" gorm:"type:char(64);not null"`
	CreatedAt             int64  `json:"created_at" gorm:"type:bigint;not null"`
}

func (AccountQuotaExecutionPermit) TableName() string { return AccountQuotaExecutionPermitTableName }

type HistoricalAccountQuotaExecutionPermitInput struct {
	FactID                int64
	Settlement            AccountQuotaSettlementFactInput
	RequestFingerprint    string
	InitialFundingApplied bool
	InitialTokenApplied   bool
	AllowFunding          bool
	AllowToken            bool
	ApprovalID            string
	EvidenceDigest        string
}

func ValidateAccountQuotaExecutionGate(ctx context.Context, db *gorm.DB) error {
	if db == nil {
		return gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if !db.Migrator().HasTable(&AccountQuotaExecutionGate{}) || !db.Migrator().HasTable(&AccountQuotaExecutionPermit{}) {
		return ErrAccountQuotaExecutionGateUninitialized
	}
	var gates []AccountQuotaExecutionGate
	if err := db.WithContext(ctx).Limit(2).Find(&gates).Error; err != nil {
		return errors.Join(ErrAccountQuotaExecutionGateInvalid, err)
	}
	if len(gates) != 1 || gates[0].ID != AccountQuotaExecutionGateSingletonID ||
		gates[0].SchemaVersion != AccountQuotaExecutionGateSchemaVersion || gates[0].State != AccountQuotaExecutionGateStateEnforcing {
		return ErrAccountQuotaExecutionGateInvalid
	}
	return nil
}

// EnsureFreshAccountQuotaExecutionGate initializes only an empty, already
// migrated database. It never creates schema and never upgrades existing rows.
func EnsureFreshAccountQuotaExecutionGate(ctx context.Context, db *gorm.DB) error {
	if db == nil {
		return gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if !db.Migrator().HasTable(&AccountQuotaExecutionGate{}) || !db.Migrator().HasTable(&AccountQuotaExecutionPermit{}) {
		return ErrAccountQuotaExecutionGateUninitialized
	}
	var gates []AccountQuotaExecutionGate
	result := db.WithContext(ctx).Limit(2).Find(&gates)
	if result.Error != nil {
		return errors.Join(ErrAccountQuotaExecutionGateInvalid, result.Error)
	}
	if result.RowsAffected > 0 {
		return ValidateAccountQuotaExecutionGate(ctx, db)
	}
	for _, model := range []any{&AccountQuotaExecutionPermit{}, &AccountQuotaSettlementIntent{}, &AccountQuotaSettlementFact{}} {
		if !db.Migrator().HasTable(model) {
			continue
		}
		var row struct {
			ID int64 `gorm:"column:id"`
		}
		result := db.WithContext(ctx).Model(model).Select("id").Limit(1).Find(&row)
		if result.Error != nil {
			return errors.Join(ErrAccountQuotaExecutionGateInvalid, result.Error)
		}
		if result.RowsAffected != 0 {
			return ErrAccountQuotaExecutionGateUninitialized
		}
	}
	now, err := taskRecoveryDBTimestamp(db.WithContext(ctx))
	if err != nil {
		return errors.Join(ErrAccountQuotaExecutionGateInvalid, err)
	}
	gate := AccountQuotaExecutionGate{ID: AccountQuotaExecutionGateSingletonID, SchemaVersion: AccountQuotaExecutionGateSchemaVersion,
		State: AccountQuotaExecutionGateStateEnforcing, CreatedAt: now, UpdatedAt: now}
	if err := db.WithContext(ctx).Create(&gate).Error; err != nil {
		return errors.Join(ErrAccountQuotaExecutionGateInvalid, err)
	}
	return ValidateAccountQuotaExecutionGate(ctx, db)
}

func accountQuotaExecutionPermitInput(permit *AccountQuotaExecutionPermit) AccountQuotaSettlementFactInput {
	if permit == nil {
		return AccountQuotaSettlementFactInput{}
	}
	return AccountQuotaSettlementFactInput{
		EventKey: permit.EventKey, RequestID: permit.RequestID, Kind: permit.Kind, UserID: permit.UserID,
		TokenID: permit.TokenID, SubscriptionID: permit.SubscriptionID, ReserveReceiptID: permit.ReserveReceiptID,
		WriterEpoch: permit.WriterEpoch, ActualQuota: permit.ActualQuota, Delta: permit.Delta, ApplyToken: permit.ApplyToken,
	}
}

func accountQuotaExecutionPermitDigest(permit *AccountQuotaExecutionPermit) (string, error) {
	if permit == nil {
		return "", ErrAccountQuotaExecutionFrozen
	}
	payload := struct {
		BindingVersion        string `json:"binding_version"`
		Source                string `json:"source"`
		ApprovalID            string `json:"approval_id"`
		HistoricalEvidence    string `json:"historical_evidence"`
		EventKey              string `json:"event_key"`
		IntentID              string `json:"intent_id"`
		FactID                string `json:"fact_id"`
		RequestID             string `json:"request_id"`
		RequestFingerprint    string `json:"request_fingerprint"`
		Kind                  string `json:"kind"`
		UserID                string `json:"user_id"`
		TokenID               string `json:"token_id"`
		SubscriptionID        string `json:"subscription_id"`
		ReserveReceiptID      string `json:"reserve_receipt_id"`
		WriterEpoch           string `json:"writer_epoch"`
		ActualQuota           string `json:"actual_quota"`
		Delta                 string `json:"delta"`
		ApplyToken            bool   `json:"apply_token"`
		InitialFundingApplied bool   `json:"initial_funding_applied"`
		InitialTokenApplied   bool   `json:"initial_token_applied"`
		AllowFunding          bool   `json:"allow_funding"`
		AllowToken            bool   `json:"allow_token"`
	}{
		strconv.Itoa(permit.BindingVersion), permit.Source, permit.ApprovalID,
		func() string {
			if permit.Source == AccountQuotaExecutionPermitSourceHistoricalReviewV1 {
				return permit.EvidenceDigest
			}
			return ""
		}(), permit.EventKey,
		strconv.FormatInt(permit.IntentID, 10), strconv.FormatInt(permit.FactID, 10),
		permit.RequestID, permit.RequestFingerprint, permit.Kind, strconv.Itoa(permit.UserID), strconv.Itoa(permit.TokenID),
		strconv.Itoa(permit.SubscriptionID), strconv.FormatInt(permit.ReserveReceiptID, 10), strconv.FormatInt(permit.WriterEpoch, 10),
		strconv.FormatInt(permit.ActualQuota, 10), strconv.FormatInt(permit.Delta, 10), permit.ApplyToken,
		permit.InitialFundingApplied, permit.InitialTokenApplied, permit.AllowFunding, permit.AllowToken,
	}
	data, err := common.Marshal(payload)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

func validateAccountQuotaExecutionPermitBinding(permit *AccountQuotaExecutionPermit, intent *AccountQuotaSettlementIntent, fact *AccountQuotaSettlementFact) error {
	if permit == nil || permit.ID <= 0 || permit.EventKey == "" || permit.BindingVersion != AccountQuotaExecutionPermitBindingVersion ||
		(permit.State != AccountQuotaExecutionPermitStateActive && permit.State != AccountQuotaExecutionPermitStateRevoked) ||
		(permit.Source != AccountQuotaExecutionPermitSourceRuntimeV1 && permit.Source != AccountQuotaExecutionPermitSourceHistoricalReviewV1) ||
		(permit.IntentID <= 0 && permit.FactID <= 0) || (permit.IntentID < 0 || permit.FactID < 0) || permit.InitialTokenApplied && !permit.InitialFundingApplied ||
		permit.CreatedAt <= 0 || strings.TrimSpace(permit.ApprovalID) == "" || strings.TrimSpace(permit.ApprovalID) != permit.ApprovalID || len(permit.ApprovalID) > 128 {
		return ErrAccountQuotaExecutionFrozen
	}
	evidence, evidenceErr := hex.DecodeString(permit.EvidenceDigest)
	if evidenceErr != nil || len(evidence) != 32 || strings.ToLower(permit.EvidenceDigest) != permit.EvidenceDigest ||
		permit.Source == AccountQuotaExecutionPermitSourceRuntimeV1 && (permit.ApprovalID != permit.Source || permit.EvidenceDigest != permit.BindingDigest) {
		return ErrAccountQuotaExecutionFrozen
	}
	normalized, fingerprint, err := normalizeAccountQuotaSettlementInput(accountQuotaExecutionPermitInput(permit))
	if err != nil || fingerprint != permit.RequestFingerprint || normalized.EventKey != permit.EventKey {
		return errors.Join(ErrAccountQuotaExecutionFrozen, err)
	}
	digest, err := accountQuotaExecutionPermitDigest(permit)
	if err != nil || digest != permit.BindingDigest {
		return errors.Join(ErrAccountQuotaExecutionFrozen, err)
	}
	if permit.IntentID > 0 {
		if intent == nil || intent.ID != permit.IntentID || validateAccountQuotaSettlementIntent(intent, normalized, fingerprint) != nil {
			return ErrAccountQuotaExecutionFrozen
		}
		if permit.FactID > 0 && intent.FactID > 0 && intent.FactID != permit.FactID {
			return ErrAccountQuotaExecutionFrozen
		}
	} else if intent != nil {
		return ErrAccountQuotaExecutionFrozen
	}
	if permit.FactID > 0 {
		if fact == nil || fact.ID != permit.FactID || validateAccountQuotaSettlementIdentity(fact, normalized, fingerprint) != nil {
			return ErrAccountQuotaExecutionFrozen
		}
		if permit.InitialFundingApplied && !fact.FundingApplied || permit.InitialTokenApplied && !fact.TokenApplied ||
			fact.FundingApplied && !permit.InitialFundingApplied && !permit.AllowFunding ||
			fact.TokenApplied && permit.ApplyToken && !permit.InitialTokenApplied && !permit.AllowToken || fact.TokenApplied && !fact.FundingApplied {
			return ErrAccountQuotaExecutionFrozen
		}
	} else if fact != nil {
		return ErrAccountQuotaExecutionFrozen
	}
	return nil
}

func newAccountQuotaExecutionPermit(intent *AccountQuotaSettlementIntent, fact *AccountQuotaSettlementFact, input AccountQuotaSettlementFactInput, fingerprint, source string) (*AccountQuotaExecutionPermit, error) {
	permit := &AccountQuotaExecutionPermit{
		EventKey: input.EventKey, Source: source, BindingVersion: AccountQuotaExecutionPermitBindingVersion,
		RequestID: input.RequestID, RequestFingerprint: fingerprint, Kind: input.Kind, UserID: input.UserID,
		TokenID: input.TokenID, SubscriptionID: input.SubscriptionID, ReserveReceiptID: input.ReserveReceiptID,
		WriterEpoch: input.WriterEpoch, ActualQuota: input.ActualQuota, Delta: input.Delta, ApplyToken: input.ApplyToken,
		AllowFunding: true, AllowToken: true, State: AccountQuotaExecutionPermitStateActive, ApprovalID: source,
	}
	if intent != nil {
		permit.IntentID = intent.ID
	}
	if fact != nil {
		permit.FactID = fact.ID
		permit.InitialFundingApplied = fact.FundingApplied
		permit.InitialTokenApplied = fact.TokenApplied
	}
	digest, err := accountQuotaExecutionPermitDigest(permit)
	if err != nil {
		return nil, err
	}
	permit.BindingDigest = digest
	permit.EvidenceDigest = digest
	return permit, nil
}

func createRuntimeAccountQuotaExecutionPermit(tx *gorm.DB, intent *AccountQuotaSettlementIntent, fact *AccountQuotaSettlementFact, input AccountQuotaSettlementFactInput, fingerprint string) (*AccountQuotaExecutionPermit, error) {
	if err := ValidateAccountQuotaExecutionGate(tx.Statement.Context, tx); err != nil {
		return nil, err
	}
	permit, err := newAccountQuotaExecutionPermit(intent, fact, input, fingerprint, AccountQuotaExecutionPermitSourceRuntimeV1)
	if err != nil {
		return nil, err
	}
	now, err := taskRecoveryDBTimestamp(tx)
	if err != nil {
		return nil, err
	}
	permit.CreatedAt = now
	if err := tx.Create(permit).Error; err != nil {
		return nil, err
	}
	return permit, validateAccountQuotaExecutionPermitBinding(permit, intent, fact)
}

func bindRuntimeAccountQuotaExecutionPermitFact(tx *gorm.DB, permit *AccountQuotaExecutionPermit, intent *AccountQuotaSettlementIntent, fact *AccountQuotaSettlementFact) error {
	if tx == nil || permit == nil || intent == nil || fact == nil || permit.Source != AccountQuotaExecutionPermitSourceRuntimeV1 ||
		permit.State != AccountQuotaExecutionPermitStateActive || permit.IntentID != intent.ID || permit.FactID != 0 {
		return ErrAccountQuotaExecutionFrozen
	}
	previousDigest := permit.BindingDigest
	permit.FactID = fact.ID
	digest, err := accountQuotaExecutionPermitDigest(permit)
	if err != nil {
		return err
	}
	result := tx.Model(&AccountQuotaExecutionPermit{}).
		Where("id = ? AND fact_id = ? AND binding_digest = ? AND state = ?", permit.ID, 0, previousDigest, AccountQuotaExecutionPermitStateActive).
		Updates(map[string]any{"fact_id": fact.ID, "binding_digest": digest, "evidence_digest": digest})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrAccountQuotaExecutionFrozen
	}
	permit.BindingDigest = digest
	permit.EvidenceDigest = digest
	return validateAccountQuotaExecutionPermitBinding(permit, intent, fact)
}

func loadAccountQuotaExecutionPermitForFact(ctx context.Context, db *gorm.DB, fact *AccountQuotaSettlementFact, lock bool) (*AccountQuotaExecutionPermit, error) {
	if fact == nil {
		return nil, ErrAccountQuotaExecutionFrozen
	}
	query := db.WithContext(ctx)
	if lock {
		query = lockForUpdate(query)
	}
	var permit AccountQuotaExecutionPermit
	if err := query.Where("event_key = ? AND fact_id = ?", fact.EventKey, fact.ID).First(&permit).Error; err != nil {
		return nil, errors.Join(ErrAccountQuotaExecutionFrozen, err)
	}
	var intent *AccountQuotaSettlementIntent
	if permit.IntentID > 0 {
		var stored AccountQuotaSettlementIntent
		intentQuery := db.WithContext(ctx)
		if lock {
			intentQuery = lockForUpdate(intentQuery)
		}
		if err := intentQuery.First(&stored, permit.IntentID).Error; err != nil {
			return nil, errors.Join(ErrAccountQuotaExecutionFrozen, err)
		}
		intent = &stored
	}
	if err := validateAccountQuotaExecutionPermitBinding(&permit, intent, fact); err != nil {
		return nil, err
	}
	if permit.State != AccountQuotaExecutionPermitStateActive {
		return nil, ErrAccountQuotaExecutionFrozen
	}
	return &permit, nil
}

func loadAccountQuotaExecutionPermitForIntent(ctx context.Context, db *gorm.DB, intent *AccountQuotaSettlementIntent, lock bool) (*AccountQuotaExecutionPermit, error) {
	if intent == nil {
		return nil, ErrAccountQuotaExecutionFrozen
	}
	query := db.WithContext(ctx)
	if lock {
		query = lockForUpdate(query)
	}
	var permit AccountQuotaExecutionPermit
	if err := query.Where("event_key = ? AND intent_id = ?", intent.EventKey, intent.ID).First(&permit).Error; err != nil {
		return nil, errors.Join(ErrAccountQuotaExecutionFrozen, err)
	}
	var fact *AccountQuotaSettlementFact
	if permit.FactID > 0 {
		var stored AccountQuotaSettlementFact
		factQuery := db.WithContext(ctx)
		if lock {
			factQuery = lockForUpdate(factQuery)
		}
		if err := factQuery.First(&stored, permit.FactID).Error; err != nil {
			return nil, errors.Join(ErrAccountQuotaExecutionFrozen, err)
		}
		fact = &stored
	}
	if err := validateAccountQuotaExecutionPermitBinding(&permit, intent, fact); err != nil {
		return nil, err
	}
	if permit.State != AccountQuotaExecutionPermitStateActive {
		return nil, ErrAccountQuotaExecutionFrozen
	}
	return &permit, nil
}

// PermitHistoricalAccountQuotaSettlement is an explicit, exact-binding API for
// controlled manifest import. It does not discover or infer approvals.
func PermitHistoricalAccountQuotaSettlement(ctx context.Context, db *gorm.DB, input HistoricalAccountQuotaExecutionPermitInput) (*AccountQuotaExecutionPermit, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	input.ApprovalID = strings.TrimSpace(input.ApprovalID)
	input.EvidenceDigest = strings.TrimSpace(input.EvidenceDigest)
	evidence, evidenceErr := hex.DecodeString(input.EvidenceDigest)
	if input.FactID <= 0 || input.ApprovalID == "" || len(input.ApprovalID) > 128 || evidenceErr != nil || len(evidence) != 32 || strings.ToLower(input.EvidenceDigest) != input.EvidenceDigest ||
		input.InitialTokenApplied && !input.InitialFundingApplied {
		return nil, ErrAccountQuotaMutationInvalidInput
	}
	normalized, fingerprint, err := normalizeAccountQuotaSettlementInput(input.Settlement)
	if err != nil || fingerprint != input.RequestFingerprint {
		return nil, errors.Join(ErrAccountQuotaMutationInvalidInput, err)
	}
	var created AccountQuotaExecutionPermit
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ValidateAccountQuotaExecutionGate(ctx, tx); err != nil {
			return err
		}
		var fact AccountQuotaSettlementFact
		if err := lockForUpdate(tx).First(&fact, input.FactID).Error; err != nil {
			return err
		}
		if err := validateAccountQuotaSettlementIdentity(&fact, normalized, fingerprint); err != nil {
			return ErrAccountQuotaTerminalRecoveryConflict
		}
		var intent *AccountQuotaSettlementIntent
		var storedIntent AccountQuotaSettlementIntent
		result := lockForUpdate(tx).Where("event_key = ?", fact.EventKey).Limit(1).Find(&storedIntent)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected > 0 {
			if err := validateAccountQuotaSettlementIntent(&storedIntent, normalized, fingerprint); err != nil || storedIntent.FactID > 0 && storedIntent.FactID != fact.ID {
				return ErrAccountQuotaTerminalRecoveryConflict
			}
			intent = &storedIntent
		}
		var existing AccountQuotaExecutionPermit
		result = lockForUpdate(tx).Where("event_key = ?", fact.EventKey).Limit(1).Find(&existing)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected > 0 {
			if existing.Source != AccountQuotaExecutionPermitSourceHistoricalReviewV1 || existing.FactID != fact.ID ||
				existing.InitialFundingApplied != input.InitialFundingApplied || existing.InitialTokenApplied != input.InitialTokenApplied ||
				existing.AllowFunding != input.AllowFunding || existing.AllowToken != input.AllowToken ||
				existing.ApprovalID != input.ApprovalID || existing.EvidenceDigest != input.EvidenceDigest {
				return ErrAccountQuotaTerminalRecoveryConflict
			}
			if err := validateAccountQuotaExecutionPermitBinding(&existing, intent, &fact); err != nil {
				return err
			}
			created = existing
			return nil
		}
		if fact.FundingApplied != input.InitialFundingApplied || fact.TokenApplied != input.InitialTokenApplied {
			return ErrAccountQuotaTerminalRecoveryConflict
		}
		permit, err := newAccountQuotaExecutionPermit(intent, &fact, normalized, fingerprint, AccountQuotaExecutionPermitSourceHistoricalReviewV1)
		if err != nil {
			return err
		}
		permit.InitialFundingApplied = input.InitialFundingApplied
		permit.InitialTokenApplied = input.InitialTokenApplied
		permit.AllowFunding = input.AllowFunding
		permit.AllowToken = input.AllowToken
		permit.ApprovalID = input.ApprovalID
		permit.EvidenceDigest = input.EvidenceDigest
		permit.BindingDigest, err = accountQuotaExecutionPermitDigest(permit)
		if err != nil {
			return err
		}
		permit.CreatedAt, err = taskRecoveryDBTimestamp(tx)
		if err != nil {
			return err
		}
		if err := tx.Create(permit).Error; err != nil {
			return err
		}
		if err := validateAccountQuotaExecutionPermitBinding(permit, intent, &fact); err != nil {
			return err
		}
		created = *permit
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("historical settlement permit: %w", err)
	}
	return &created, nil
}
