package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/ForceMind/MyAPI/common"
	"gorm.io/gorm"
)

// Token budgets use actual input + output counts, never internal quota units.
// The maximum keeps integer JSON values exact in the existing browser client.
const MaxTokenBudget int64 = 1<<53 - 1

const (
	TokenBudgetBoundOpenAIResponses = "openai_responses_input_tokens"
	TokenBudgetBoundOpenAIChat      = "openai_chat_context_window"
	// Exact reviewed native model contract, not a tokenizer estimate or wildcard.
	TokenBudgetOpenAIChatModel           = "gpt-6.1-sol"
	TokenBudgetOpenAIChatContext   int64 = 1050000
	TokenBudgetOpenAIChatMaxOutput int64 = 128000
)

func validTokenBudgetBoundSource(source string) bool {
	return source == TokenBudgetBoundOpenAIResponses || source == TokenBudgetBoundOpenAIChat
}

const (
	TokenBudgetPrepared  = "prepared"
	TokenBudgetSent      = "sent"
	TokenBudgetUnknown   = "usage_unknown"
	TokenBudgetSettled   = "settled"
	TokenBudgetCancelled = "not_sent"
)

var (
	ErrTokenBudgetInvalid   = errors.New("invalid token budget input")
	ErrTokenBudgetConflict  = errors.New("token budget revision or evidence conflict")
	ErrTokenBudgetPending   = errors.New("token budget has an unresolved request")
	ErrTokenBudgetExceeded  = errors.New("token budget is insufficient")
	ErrTokenBudgetBound     = errors.New("reported usage does not match the reserved bound")
	ErrTokenBudgetDuplicate = errors.New("token budget request has already been admitted")
)

func ValidateTokenBudgetSchema(db *gorm.DB) error {
	if db == nil {
		return gorm.ErrInvalidDB
	}
	for _, entity := range []any{&TokenBudget{}, &TokenBudgetReservation{}, &TokenBudgetPolicyChange{}} {
		if !db.Migrator().HasTable(entity) {
			return errors.New("token budget schema migration is required")
		}
	}
	for _, column := range []string{"fee_enabled", "fee_limit_usd", "fee_used_usd", "fee_reserved_usd", "account_threshold_enabled", "account_min_remaining_bps", "account_max_age_seconds"} {
		if !db.Migrator().HasColumn(&TokenBudget{}, column) {
			return errors.New("fee budget schema migration is required")
		}
	}
	for _, column := range []string{"bound_source", "pricing_evidence", "fee_enabled", "fee_reserved_usd", "fee_price_evidence", "actual_fee_usd", "observed_fee_usd", "request_service_tier"} {
		if !db.Migrator().HasColumn(&TokenBudgetReservation{}, column) {
			return errors.New("token budget evidence migration is required")
		}
	}
	if !db.Migrator().HasColumn(&ChannelQuotaSnapshot{}, "codex_threshold_qualified") {
		return errors.New("account threshold sampling migration is required")
	}
	for _, column := range []string{"actual_input_tokens", "actual_output_tokens", "actual_fee_usd"} {
		if !db.Migrator().HasColumn(&UsageReviewDecision{}, column) {
			return errors.New("token budget review migration is required")
		}
	}
	return nil
}

type TokenBudget struct {
	AccountThresholdEnabled bool   `json:"account_threshold_enabled" gorm:"not null;default:false"`
	AccountMinRemainingBPS  int    `json:"account_min_remaining_bps" gorm:"not null;default:0"`
	AccountMaxAgeSeconds    int64  `json:"account_max_age_seconds" gorm:"not null;default:300"`
	FeeEnabled              bool   `json:"fee_enabled" gorm:"not null;default:false"`
	FeeLimitUSD             string `json:"fee_limit_usd" gorm:"type:varchar(128);not null;default:'0'"`
	FeeUsedUSD              string `json:"fee_used_usd" gorm:"type:varchar(128);not null;default:'0'"`
	FeeReservedUSD          string `json:"fee_reserved_usd" gorm:"type:varchar(128);not null;default:'0'"`
	TokenID                 int    `json:"token_id" gorm:"primaryKey;autoIncrement:false"`
	UserID                  int    `json:"user_id" gorm:"not null;index"`
	Enabled                 bool   `json:"enabled" gorm:"not null"`
	Limit                   int64  `json:"limit" gorm:"column:token_limit;type:bigint;not null"`
	Used                    int64  `json:"used" gorm:"type:bigint;not null"`
	Reserved                int64  `json:"reserved" gorm:"type:bigint;not null"`
	PendingRequestID        string `json:"pending_request_id" gorm:"type:varchar(64);not null"`
	Revision                int64  `json:"revision" gorm:"type:bigint;not null"`
}

// A pre-dispatch record remains blocking even if persisting an unknown marker
// fails. There is intentionally no lease expiry or automatic refund of Sent.
type TokenBudgetReservation struct {
	RequestServiceTier string  `json:"request_service_tier" gorm:"type:varchar(32);not null;default:''"`
	ObservedFeeUSD     *string `json:"observed_fee_usd" gorm:"type:varchar(128)"`
	FeeEnabled         bool    `json:"fee_enabled" gorm:"not null;default:false"`
	FeeReservedUSD     string  `json:"fee_reserved_usd" gorm:"type:varchar(128);not null;default:'0'"`
	ActualFeeUSD       *string `json:"actual_fee_usd" gorm:"type:varchar(128)"`
	FeePriceEvidence   string  `json:"-" gorm:"type:text"`
	PricingEvidence    string  `json:"-" gorm:"type:text"`
	BoundSource        string  `json:"bound_source" gorm:"type:varchar(64);not null;default:''"`
	RequestID          string  `json:"request_id" gorm:"type:varchar(64);primaryKey"`
	TokenID            int     `json:"token_id" gorm:"not null;index"`
	UserID             int     `json:"user_id" gorm:"not null;index"`
	ChannelID          int     `json:"channel_id" gorm:"not null"`
	ModelName          string  `json:"model_name" gorm:"size:512;not null"`
	PayloadSHA256      string  `json:"payload_sha256" gorm:"type:varchar(64);not null"`
	InputTokens        int64   `json:"input_tokens_bound" gorm:"type:bigint;not null"`
	MaxOutputTokens    int64   `json:"max_output_tokens" gorm:"type:bigint;not null"`
	Reserved           int64   `json:"reserved" gorm:"type:bigint;not null"`
	State              string  `json:"state" gorm:"type:varchar(16);not null;index"`
	ActualInput        *int64  `json:"actual_input" gorm:"type:bigint"`
	ActualOutput       *int64  `json:"actual_output" gorm:"type:bigint"`
	ObservedInput      *int64  `json:"observed_input" gorm:"type:bigint"`
	ObservedOutput     *int64  `json:"observed_output" gorm:"type:bigint"`
	Reason             string  `json:"reason" gorm:"type:varchar(64);not null"`
	ReviewedBy         int     `json:"reviewed_by" gorm:"not null"`
	EvidenceReference  string  `json:"evidence_reference,omitempty" gorm:"size:2048;not null"`
	EvidenceDigest     string  `json:"evidence_digest" gorm:"type:varchar(64);not null"`
	CreatedAt          int64   `json:"created_at" gorm:"type:bigint;not null"`
	UpdatedAt          int64   `json:"updated_at" gorm:"type:bigint;not null"`
}

type TokenBudgetPolicyChange struct {
	ID         string `json:"id" gorm:"type:varchar(64);primaryKey"`
	Digest     string `json:"digest" gorm:"type:varchar(64);not null"`
	ActorID    int    `json:"actor_id" gorm:"not null"`
	TokenID    int    `json:"token_id" gorm:"not null;index"`
	BeforeJSON string `json:"before" gorm:"type:text;not null"`
	AfterJSON  string `json:"after" gorm:"type:text;not null"`
	CreatedAt  int64  `json:"created_at" gorm:"type:bigint;not null"`
}

func (*TokenBudgetPolicyChange) BeforeUpdate(*gorm.DB) error { return ErrAccountQuotaReceiptImmutable }
func (*TokenBudgetPolicyChange) BeforeDelete(*gorm.DB) error { return ErrAccountQuotaReceiptImmutable }

func validBudgetDigest(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && hex.EncodeToString(decoded) == value
}

func budgetDigest(value any) (string, error) {
	encoded, err := common.Marshal(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

// All budget writers take the Token row first, then its budget row. Existing
// quota transactions are not nested here and retain their own writer fences.
func lockTokenBudget(tx *gorm.DB, tokenID int, allowDeleted bool) (*TokenBudget, error) {
	var token Token
	if tokenID <= 0 {
		return nil, ErrTokenBudgetInvalid
	}
	query := lockForUpdate(tx)
	if allowDeleted {
		query = query.Unscoped()
	}
	if err := query.First(&token, tokenID).Error; err != nil {
		return nil, err
	}
	var budget TokenBudget
	err := lockForUpdate(tx).First(&budget, "token_id = ?", tokenID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &TokenBudget{TokenID: tokenID, UserID: token.UserId, FeeLimitUSD: "0", FeeUsedUSD: "0", FeeReservedUSD: "0", AccountMinRemainingBPS: 2000, AccountMaxAgeSeconds: 300}, nil
	}
	if err != nil {
		return nil, err
	}
	if budget.UserID != token.UserId {
		return nil, ErrTokenBudgetConflict
	}
	if budget.Limit < 0 || budget.Used < 0 || budget.Reserved < 0 || budget.Limit > MaxTokenBudget || budget.Used > MaxTokenBudget || budget.Reserved > MaxTokenBudget || budget.Revision <= 0 || budget.Revision >= MaxTokenBudget {
		return nil, ErrTokenBudgetConflict
	}
	if err := validateFeeBudgetState(&budget); err != nil {
		return nil, err
	}
	return &budget, nil
}

func saveTokenBudget(tx *gorm.DB, budget *TokenBudget, previous int64) error {
	if previous == 0 {
		return tx.Create(budget).Error
	}
	result := tx.Model(&TokenBudget{}).Where("token_id = ? AND revision = ?", budget.TokenID, previous).
		Updates(map[string]any{"enabled": budget.Enabled, "token_limit": budget.Limit, "used": budget.Used,
			"reserved": budget.Reserved, "pending_request_id": budget.PendingRequestID, "revision": budget.Revision, "fee_enabled": budget.FeeEnabled, "fee_limit_usd": budget.FeeLimitUSD, "fee_used_usd": budget.FeeUsedUSD, "fee_reserved_usd": budget.FeeReservedUSD, "account_threshold_enabled": budget.AccountThresholdEnabled, "account_min_remaining_bps": budget.AccountMinRemainingBPS, "account_max_age_seconds": budget.AccountMaxAgeSeconds})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrTokenBudgetConflict
	}
	return nil
}

func eligibleTokenBudgetSubject(tx *gorm.DB, tokenID, userID int) error {
	var token Token
	if err := tx.First(&token, tokenID).Error; err != nil {
		return err
	}
	var user User
	if err := tx.First(&user, userID).Error; err != nil {
		return err
	}
	if token.UserId != userID || token.Status != common.TokenStatusEnabled || user.Status != common.UserStatusEnabled || (token.ExpiredTime != -1 && token.ExpiredTime < common.GetTimestamp()) {
		return ErrAccountQuotaMutationIneligible
	}
	return nil
}

type TokenBudgetPolicyInput struct {
	AccountThreshold *AccountQuotaThresholdPolicyInput `json:"account_threshold,omitempty"`
	Fee              *FeeBudgetPolicyInput             `json:"fee,omitempty"`
	ID               string                            `json:"id"`
	TokenID          int                               `json:"token_id"`
	ExpectedRevision int64                             `json:"expected_revision"`
	Enabled          bool                              `json:"enabled"`
	Limit            int64                             `json:"limit"`
}

// LookupTokenBudget is an internal admission read. A missing row means the
// feature is unconfigured; missing schema or unavailable storage is an error.
func LookupTokenBudget(ctx context.Context, db *gorm.DB, tokenID int) (*TokenBudget, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if tokenID <= 0 {
		return nil, ErrTokenBudgetInvalid
	}
	var budget TokenBudget
	err := db.WithContext(ctx).First(&budget, "token_id = ?", tokenID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := validateFeeBudgetState(&budget); err != nil {
		return nil, err
	}
	return &budget, nil
}

type TokenBudgetView struct {
	Policy  TokenBudget             `json:"policy"`
	Pending *TokenBudgetReservation `json:"pending"`
	Review  *UsageReviewDetail      `json:"review,omitempty"`
}

func ReadTokenBudget(ctx context.Context, db *gorm.DB, actorID, tokenID int) (*TokenBudgetView, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if actorID <= 0 || tokenID <= 0 {
		return nil, ErrTokenBudgetInvalid
	}
	var actor User
	if err := db.WithContext(ctx).First(&actor, actorID).Error; err != nil {
		return nil, err
	}
	if actor.Status != common.UserStatusEnabled {
		return nil, ErrAccountQuotaMutationIneligible
	}
	var token Token
	if err := db.WithContext(ctx).Unscoped().First(&token, tokenID).Error; err != nil {
		return nil, err
	}
	if actor.Role != common.RoleRootUser && token.UserId != actorID {
		return nil, gorm.ErrRecordNotFound
	}
	budget, err := LookupTokenBudget(ctx, db, tokenID)
	if err != nil {
		return nil, err
	}
	if budget == nil {
		budget = &TokenBudget{TokenID: tokenID, UserID: token.UserId, FeeLimitUSD: "0", FeeUsedUSD: "0", FeeReservedUSD: "0", AccountMinRemainingBPS: 2000, AccountMaxAgeSeconds: 300}
	}
	if budget.UserID != token.UserId {
		return nil, ErrTokenBudgetConflict
	}
	view := &TokenBudgetView{Policy: *budget}
	if budget.PendingRequestID == "" {
		return view, nil
	}
	var pending TokenBudgetReservation
	if err := db.WithContext(ctx).First(&pending, "request_id = ?", budget.PendingRequestID).Error; err != nil {
		return nil, err
	}
	if pending.RequestID != budget.PendingRequestID || pending.UserID != token.UserId || pending.TokenID != tokenID {
		return nil, ErrTokenBudgetConflict
	}
	if actor.Role != common.RoleRootUser {
		pending.EvidenceReference = ""
	}
	view.Pending = &pending
	if pending.State == TokenBudgetPrepared {
		// An unsent token reservation cannot safely offer cancellation when an
		// automatic quota settlement already exists. Keep ordinary prepared
		// reservations independent of a usage-review journal.
		var intents []AccountQuotaSettlementIntent
		if err := db.WithContext(ctx).Select("id").Where("request_id = ?", pending.RequestID).Limit(1).Find(&intents).Error; err != nil {
			return nil, ErrAccountQuotaSettlementFactUnknown
		}
		var facts []AccountQuotaSettlementFact
		if err := db.WithContext(ctx).Select("id").Where("request_id = ?", pending.RequestID).Limit(1).Find(&facts).Error; err != nil {
			return nil, ErrAccountQuotaSettlementFactUnknown
		}
		if len(intents) == 0 && len(facts) == 0 {
			return view, nil
		}
	}
	view.Review, err = GetUsageReview(ctx, db, actorID, pending.RequestID)
	if err != nil {
		return nil, err
	}
	if pending.State == TokenBudgetPrepared {
		view.Review.SettlementStatus, view.Review.RecoveryBlockReason = "manual", "automatic_settlement_manual"
		view.Review.CanRecoverTextDispatch, view.Review.CanReconcileUsage = false, false
		return view, nil
	}
	if view.Review.TokenBudget == nil {
		view.Review.CanReconcileUsage = false
	}
	if view.Review.TokenBudget == nil && (view.Review.State == LegacyUsagePrepared || view.Review.State == AccountQuotaTerminalRecoveryOpen) {
		// The recover endpoint prepares a lost quota hold before reconciling.
		// This GET only projects that capability; attaching the independent
		// reservation inside GetUsageReview would make Prepare incorrectly
		// treat it as an already prepared hold.
		var evidence struct {
			StrictTokenBudget bool `json:"strict_token_budget"`
		}
		qualified := (pending.State == TokenBudgetSent || pending.State == TokenBudgetUnknown || pending.State == TokenBudgetSettled) &&
			validTokenBudgetBoundSource(pending.BoundSource) && common.UnmarshalJsonStr(pending.PricingEvidence, &evidence) == nil && evidence.StrictTokenBudget
		if qualified {
			view.Review.TokenBudget = view.Pending
			view.Review.CanReconcileUsage = view.Review.SettlementStatus == "none" || view.Review.Writer == "legacy" && view.Review.SettlementStatus == "applied_journal_pending"
		}
	}
	if pending.State != TokenBudgetSent && pending.State != TokenBudgetUnknown && pending.State != TokenBudgetSettled {
		view.Review.CanReconcileUsage = false
	}
	return view, nil
}

func ConfigureTokenBudget(ctx context.Context, db *gorm.DB, actorID int, input TokenBudgetPolicyInput) (*TokenBudget, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if !validBudgetDigest(input.ID) || input.TokenID <= 0 || input.ExpectedRevision < 0 || input.Limit < 0 || input.Limit > MaxTokenBudget {
		return nil, ErrTokenBudgetInvalid
	}
	if input.AccountThreshold != nil {
		copy := *input.AccountThreshold
		if !common.ValidAccountQuotaThreshold(common.AccountQuotaThreshold{MinimumRemainingBPS: copy.MinimumRemainingBPS, MaxAgeSeconds: copy.MaxAgeSeconds}) {
			return nil, ErrAccountQuotaThresholdInvalid
		}
		input.AccountThreshold = &copy
	}
	if input.Fee != nil {
		value := *input.Fee
		var err error
		value.LimitUSD, err = NormalizeFeeBudgetUSD(value.LimitUSD)
		if err != nil {
			return nil, err
		}
		input.Fee = &value
	}
	digest, err := budgetDigest(struct {
		Actor int
		Input TokenBudgetPolicyInput
	}{actorID, input})
	if err != nil {
		return nil, err
	}
	var result *TokenBudget
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := authorizeUsageReviewer(tx, actorID); err != nil {
			return err
		}
		budget, err := lockTokenBudget(tx, input.TokenID, false)
		if err != nil {
			return err
		}
		var old TokenBudgetPolicyChange
		err = tx.First(&old, "id = ?", input.ID).Error
		if err == nil {
			if old.ID != input.ID || old.Digest != digest {
				return ErrTokenBudgetConflict
			}
			result = budget // Receipt replay never re-applies an obsolete policy.
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if budget.Revision != input.ExpectedRevision {
			return ErrTokenBudgetConflict
		}
		if budget.PendingRequestID != "" {
			return ErrTokenBudgetPending
		}
		if input.Enabled && input.Limit < budget.Used {
			return ErrTokenBudgetExceeded
		}
		before, err := common.Marshal(budget)
		if err != nil {
			return err
		}
		if input.Fee != nil {
			if input.Fee.Enabled {
				if err := feeBudgetFits(input.Fee.LimitUSD, budget.FeeUsedUSD, "0"); err != nil {
					return err
				}
			}
			budget.FeeEnabled, budget.FeeLimitUSD = input.Fee.Enabled, input.Fee.LimitUSD
		}
		if input.AccountThreshold != nil {
			budget.AccountThresholdEnabled, budget.AccountMinRemainingBPS, budget.AccountMaxAgeSeconds = input.AccountThreshold.Enabled, input.AccountThreshold.MinimumRemainingBPS, input.AccountThreshold.MaxAgeSeconds
		}
		if budget.AccountThresholdEnabled && (input.Enabled || budget.FeeEnabled) {
			return ErrAccountQuotaThresholdCombination
		}
		previous := budget.Revision
		budget.Enabled, budget.Limit, budget.Revision = input.Enabled, input.Limit, previous+1
		if err := saveTokenBudget(tx, budget, previous); err != nil {
			return err
		}
		after, err := common.Marshal(budget)
		if err != nil {
			return err
		}
		now, err := taskRecoveryDBTimestamp(tx)
		if err != nil {
			return err
		}
		if err := tx.Create(&TokenBudgetPolicyChange{ID: input.ID, Digest: digest, ActorID: actorID, TokenID: input.TokenID, BeforeJSON: string(before), AfterJSON: string(after), CreatedAt: now}).Error; err != nil {
			return err
		}
		result = budget
		return nil
	})
	return result, err
}

// ReserveTokenBudget accepts only server-verified, frozen request bounds. It
// cannot be exposed directly as an HTTP endpoint accepting client estimates.
func ReserveTokenBudget(ctx context.Context, db *gorm.DB, input TokenBudgetReservation) error {
	if db == nil {
		return gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	id, err := normalizeAccountRequestID(input.RequestID)
	if err != nil || id != input.RequestID || input.UserID <= 0 || input.TokenID <= 0 || input.ChannelID <= 0 ||
		input.ModelName == "" || len(input.ModelName) > 512 || len(input.PricingEvidence) > 16384 || !validTokenBudgetBoundSource(input.BoundSource) || !validBudgetDigest(input.PayloadSHA256) ||
		input.InputTokens < 0 || input.InputTokens > int64(common.MaxQuota) || input.MaxOutputTokens <= 0 || input.MaxOutputTokens > int64(common.MaxQuota) {
		return ErrTokenBudgetInvalid
	}
	if input.BoundSource == TokenBudgetBoundOpenAIChat &&
		(input.ModelName != TokenBudgetOpenAIChatModel || input.InputTokens != TokenBudgetOpenAIChatContext || input.MaxOutputTokens > TokenBudgetOpenAIChatMaxOutput) {
		return ErrTokenBudgetInvalid
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		budget, err := lockTokenBudget(tx, input.TokenID, false)
		if err != nil {
			return err
		}
		if (!budget.Enabled && !budget.FeeEnabled) || budget.UserID != input.UserID {
			return ErrTokenBudgetConflict
		}
		if err := eligibleTokenBudgetSubject(tx, input.TokenID, input.UserID); err != nil {
			return err
		}
		var existing TokenBudgetReservation
		err = tx.First(&existing, "request_id = ?", id).Error
		if err == nil {
			return ErrTokenBudgetDuplicate
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if budget.PendingRequestID != "" || budget.Reserved != 0 {
			return ErrTokenBudgetPending
		}
		bound := input.InputTokens + input.MaxOutputTokens
		if input.BoundSource == TokenBudgetBoundOpenAIChat {
			// The documented context contains both input and generated tokens.
			bound = TokenBudgetOpenAIChatContext
		}
		if budget.Enabled && (budget.Used > budget.Limit || bound > budget.Limit-budget.Used) {
			return ErrTokenBudgetExceeded
		}
		feeReserved := "0"
		if budget.FeeEnabled != input.FeeEnabled {
			return ErrTokenBudgetConflict
		}
		if budget.FeeEnabled {
			if input.RequestServiceTier != "default" {
				return ErrFeeBudgetInvalid
			}
			if len(input.FeePriceEvidence) == 0 || len(input.FeePriceEvidence) > 16384 {
				return ErrFeeBudgetInvalid
			}
			if _, err := common.CanonicalJSONObjectDigest([]byte(input.FeePriceEvidence)); err != nil {
				return ErrFeeBudgetInvalid
			}
			if err := feeBudgetFits(budget.FeeLimitUSD, budget.FeeUsedUSD, input.FeeReservedUSD); err != nil {
				return err
			}
			feeReserved = input.FeeReservedUSD
		} else if (input.FeeReservedUSD != "" && input.FeeReservedUSD != "0") || input.FeePriceEvidence != "" {
			return ErrFeeBudgetInvalid
		}
		now, err := taskRecoveryDBTimestamp(tx)
		if err != nil {
			return err
		}
		// Do not accept caller-provided terminal fields, timestamps or state.
		row := TokenBudgetReservation{RequestServiceTier: input.RequestServiceTier, FeeEnabled: budget.FeeEnabled, FeeReservedUSD: feeReserved, FeePriceEvidence: input.FeePriceEvidence, RequestID: id, TokenID: input.TokenID, UserID: input.UserID,
			ChannelID: input.ChannelID, ModelName: input.ModelName, PayloadSHA256: input.PayloadSHA256, BoundSource: input.BoundSource,
			InputTokens: input.InputTokens, MaxOutputTokens: input.MaxOutputTokens, Reserved: bound, PricingEvidence: input.PricingEvidence,
			State: TokenBudgetPrepared, CreatedAt: now, UpdatedAt: now}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		previous := budget.Revision
		budget.PendingRequestID, budget.Reserved, budget.Revision, budget.FeeReservedUSD = id, bound, previous+1, feeReserved
		return saveTokenBudget(tx, budget, previous)
	})
}

// Mutations serialize through the budget before reading the request. A sent
// request can never be released through the pre-dispatch cancellation action.
type TokenBudgetMutation struct {
	FeeUSD    *string
	TokenID   int
	RequestID string
	Action    string
	Input     int64
	Output    int64
	ActorID   int
	Evidence  string
	Reason    string
}

func MutateTokenBudgetRequest(ctx context.Context, db *gorm.DB, input TokenBudgetMutation) (*TokenBudgetReservation, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	id, err := normalizeAccountRequestID(input.RequestID)
	if err != nil || id != input.RequestID || input.TokenID <= 0 {
		return nil, ErrTokenBudgetInvalid
	}
	switch input.Action {
	case "send", "hold":
		if input.ActorID != 0 || input.Evidence != "" {
			return nil, ErrTokenBudgetInvalid
		}
	case "cancel":
		if input.ActorID < 0 || (input.ActorID == 0 && input.Evidence != "") {
			return nil, ErrTokenBudgetInvalid
		}
	case "settle", "reconcile":
		if input.Input < 0 || input.Output < 0 || input.Input > MaxTokenBudget || input.Output > MaxTokenBudget-input.Input {
			return nil, ErrTokenBudgetInvalid
		}
	default:
		return nil, ErrTokenBudgetInvalid
	}
	if input.FeeUSD != nil {
		if input.Action != "settle" && input.Action != "reconcile" {
			return nil, ErrFeeBudgetInvalid
		}
		normalized, err := NormalizeFeeBudgetUSD(*input.FeeUSD)
		if err != nil {
			return nil, err
		}
		input.FeeUSD = &normalized
	}
	input.Evidence = strings.TrimSpace(input.Evidence)
	manual := input.Action == "reconcile" || (input.Action == "cancel" && input.ActorID > 0)
	if manual && (input.ActorID <= 0 || input.Evidence == "" || len(input.Evidence) > 2048) {
		return nil, ErrTokenBudgetInvalid
	}
	if input.Action == "settle" && (input.ActorID != 0 || input.Evidence != "") {
		return nil, ErrTokenBudgetInvalid
	}
	if len(input.Reason) > 64 {
		return nil, ErrTokenBudgetInvalid
	}
	var result *TokenBudgetReservation
	var committedError error
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if manual {
			if err := authorizeUsageReviewer(tx, input.ActorID); err != nil {
				return err
			}
		}
		budget, err := lockTokenBudget(tx, input.TokenID, true)
		if err != nil {
			return err
		}
		var row TokenBudgetReservation
		if err := lockForUpdate(tx).First(&row, "request_id = ?", id).Error; err != nil {
			return err
		}
		if row.RequestID != id || row.TokenID != input.TokenID || row.UserID != budget.UserID {
			return ErrTokenBudgetConflict
		}
		terminal := input.Action == "settle" || input.Action == "reconcile"
		if terminal && row.FeeEnabled != (input.FeeUSD != nil) {
			return ErrFeeBudgetInvalid
		}
		if row.State == TokenBudgetSettled {
			if (input.Action != "settle" && input.Action != "reconcile") || row.ActualInput == nil || row.ActualOutput == nil ||
				*row.ActualInput != input.Input || *row.ActualOutput != input.Output || row.ReviewedBy != input.ActorID || row.EvidenceReference != input.Evidence {
				return ErrTokenBudgetConflict
			}
			if row.FeeEnabled && (row.ActualFeeUSD == nil || *row.ActualFeeUSD != *input.FeeUSD) {
				return ErrTokenBudgetConflict
			}
			result = &row
			return nil
		}
		if row.State == TokenBudgetCancelled && input.Action == "cancel" {
			if row.ReviewedBy != input.ActorID || row.EvidenceReference != input.Evidence {
				return ErrTokenBudgetConflict
			}
			result = &row
			return nil
		}
		if budget.PendingRequestID != id || budget.Reserved != row.Reserved || budget.FeeEnabled != row.FeeEnabled || budget.FeeReservedUSD != row.FeeReservedUSD {
			return ErrTokenBudgetConflict
		}
		if row.State == TokenBudgetUnknown && input.Action == "hold" {
			result = &row
			return nil
		}
		previousState := row.State
		updates := map[string]any{}
		switch input.Action {
		case "send":
			if row.State != TokenBudgetPrepared {
				return ErrTokenBudgetDuplicate
			}
			if err := eligibleTokenBudgetSubject(tx, input.TokenID, row.UserID); err != nil {
				return err
			}
			row.State = TokenBudgetSent
		case "cancel":
			if row.State != TokenBudgetPrepared {
				return ErrTokenBudgetPending
			}
			row.State = TokenBudgetCancelled
			if manual {
				row.ReviewedBy, row.EvidenceReference = input.ActorID, input.Evidence
				row.EvidenceDigest, err = budgetDigest(struct {
					Request  string
					Actor    int
					Evidence string
				}{id, input.ActorID, input.Evidence})
				if err != nil {
					return err
				}
				updates["reviewed_by"], updates["evidence_reference"], updates["evidence_digest"] = row.ReviewedBy, row.EvidenceReference, row.EvidenceDigest
			}
		case "hold":
			if row.State != TokenBudgetSent && row.State != TokenBudgetUnknown {
				return ErrTokenBudgetConflict
			}
			row.State, row.Reason = TokenBudgetUnknown, input.Reason
		case "settle", "reconcile":
			if input.Action == "settle" && row.State != TokenBudgetSent {
				return ErrTokenBudgetPending
			}
			if input.Action == "reconcile" && row.State != TokenBudgetSent && row.State != TokenBudgetUnknown {
				return ErrTokenBudgetConflict
			}
			withinBound := input.Input == row.InputTokens && input.Output <= row.MaxOutputTokens
			if row.BoundSource == TokenBudgetBoundOpenAIChat {
				withinBound = row.ModelName == TokenBudgetOpenAIChatModel && row.InputTokens == TokenBudgetOpenAIChatContext &&
					row.Reserved == TokenBudgetOpenAIChatContext && row.MaxOutputTokens <= TokenBudgetOpenAIChatMaxOutput &&
					input.Input <= row.InputTokens && input.Output <= row.MaxOutputTokens && input.Input+input.Output <= row.Reserved
			} else if row.BoundSource != TokenBudgetBoundOpenAIResponses {
				return ErrTokenBudgetConflict
			}
			if input.Action == "settle" && !withinBound {
				row.State, row.Reason = TokenBudgetUnknown, "bound_mismatch"
				row.ObservedInput, row.ObservedOutput = &input.Input, &input.Output
				updates["observed_input"], updates["observed_output"] = input.Input, input.Output
				committedError = ErrTokenBudgetBound
				break
			}
			if row.FeeEnabled {
				actualFee, err := feeBudgetDecimal(*input.FeeUSD)
				if err != nil {
					return err
				}
				reservedFee, err := feeBudgetDecimal(row.FeeReservedUSD)
				if err != nil {
					return err
				}
				if input.Action == "settle" && actualFee.GreaterThan(reservedFee) {
					row.State, row.Reason = TokenBudgetUnknown, "fee_bound_mismatch"
					row.ObservedInput, row.ObservedOutput, row.ObservedFeeUSD = &input.Input, &input.Output, input.FeeUSD
					updates["observed_input"], updates["observed_output"], updates["observed_fee_usd"] = input.Input, input.Output, *input.FeeUSD
					committedError = ErrTokenBudgetBound
					break
				}
				budget.FeeUsedUSD, err = feeBudgetAdd(budget.FeeUsedUSD, *input.FeeUSD)
				if err != nil {
					return err
				}
				row.ActualFeeUSD = input.FeeUSD
				updates["actual_fee_usd"] = *input.FeeUSD
			}
			actual := input.Input + input.Output
			if budget.Used > MaxTokenBudget-actual {
				return ErrTokenBudgetInvalid
			}
			budget.Used += actual
			row.State, row.ActualInput, row.ActualOutput = TokenBudgetSettled, &input.Input, &input.Output
			row.ReviewedBy, row.EvidenceReference = input.ActorID, input.Evidence
			row.EvidenceDigest, err = budgetDigest(struct {
				Request       string
				Input, Output int64
				Actor         int
				Evidence      string
			}{id, input.Input, input.Output, input.ActorID, input.Evidence})
			if err != nil {
				return err
			}
			if row.FeeEnabled {
				row.EvidenceDigest, err = budgetDigest(struct {
					TokenDigest string
					FeeUSD      string
				}{row.EvidenceDigest, *input.FeeUSD})
				if err != nil {
					return err
				}
			}
			updates["actual_input"], updates["actual_output"] = input.Input, input.Output
			updates["reviewed_by"], updates["evidence_reference"], updates["evidence_digest"] = row.ReviewedBy, row.EvidenceReference, row.EvidenceDigest
		}
		if row.State == TokenBudgetSettled || row.State == TokenBudgetCancelled {
			budget.PendingRequestID, budget.Reserved, budget.FeeReservedUSD = "", 0, "0"
		}
		row.UpdatedAt, err = taskRecoveryDBTimestamp(tx)
		if err != nil {
			return err
		}
		updates["state"], updates["reason"], updates["updated_at"] = row.State, row.Reason, row.UpdatedAt
		updated := tx.Model(&TokenBudgetReservation{}).Where("request_id = ? AND state = ?", id, previousState).Updates(updates)
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return ErrTokenBudgetConflict
		}
		previous := budget.Revision
		budget.Revision++
		if err := saveTokenBudget(tx, budget, previous); err != nil {
			return err
		}
		result = &row
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, committedError
}
