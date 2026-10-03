package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/ForceMind/MyAPI/common"
	hosttypes "github.com/ForceMind/MyAPI/types"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

func ValidateUsageReviewSchema(db *gorm.DB) error {
	if db == nil {
		return gorm.ErrInvalidDB
	}
	for _, entity := range []any{&LegacyUsageReservation{}, &UsageReviewDecision{}, &AccountQuotaTerminalRecoveryObligation{}} {
		if !db.Migrator().HasTable(entity) {
			return fmt.Errorf("usage reconciliation schema migration is required")
		}
	}
	if !db.Migrator().HasColumn(&AccountQuotaTerminalRecoveryObligation{}, "review_metadata") {
		return fmt.Errorf("usage reconciliation metadata migration is required")
	}
	return nil
}

// UsageReviewDecision is the immutable evidence reference for a manual
// resolution. It contains no prompts, credentials or provider response bodies.
type UsageReviewDecision struct {
	ActualFeeUSD       *string `json:"actual_fee_usd,omitempty" gorm:"type:varchar(128)"`
	ActualInputTokens  *int64  `json:"actual_input_tokens,omitempty" gorm:"type:bigint"`
	ActualOutputTokens *int64  `json:"actual_output_tokens,omitempty" gorm:"type:bigint"`
	ID                 int64   `json:"id" gorm:"primaryKey"`
	RequestID          string  `json:"request_id" gorm:"type:varchar(64);not null;uniqueIndex"`
	ActorID            int     `json:"actor_id" gorm:"not null"`
	UserID             int     `json:"user_id" gorm:"not null"`
	TokenID            int     `json:"token_id" gorm:"not null"`
	ChannelID          int     `json:"channel_id" gorm:"not null"`
	ModelName          string  `json:"model_name" gorm:"size:512"`
	ActualQuota        int64   `json:"actual_quota" gorm:"type:bigint;not null"`
	EvidenceReference  string  `json:"evidence_reference" gorm:"size:2048;not null"`
	EvidenceDigest     string  `json:"evidence_digest" gorm:"type:char(64);not null"`
	CreatedAt          int64   `json:"created_at" gorm:"type:bigint;not null"`
	StatisticsApplied  bool    `json:"statistics_applied" gorm:"not null"`
	LogProjected       bool    `json:"log_projected" gorm:"not null;index"`
}

func (UsageReviewDecision) TableName() string            { return "usage_review_decisions" }
func (*UsageReviewDecision) BeforeUpdate(*gorm.DB) error { return ErrAccountQuotaReceiptImmutable }
func (*UsageReviewDecision) BeforeDelete(*gorm.DB) error { return ErrAccountQuotaReceiptImmutable }

type UsageReviewDetail struct {
	TextDispatchPending    bool                    `json:"text_dispatch_pending"`
	CanRecoverTextDispatch bool                    `json:"can_recover_text_dispatch"`
	TokenBudget            *TokenBudgetReservation `json:"token_budget,omitempty"`
	RequestID              string                  `json:"request_id"`
	UserID                 int                     `json:"user_id"`
	TokenID                int                     `json:"token_id"`
	ChannelID              int                     `json:"channel_id"`
	ModelName              string                  `json:"model_name"`
	Writer                 string                  `json:"writer"`
	State                  string                  `json:"state"`
	ReservedQuota          int64                   `json:"reserved_quota"`
	ActualQuota            *int64                  `json:"actual_quota"`
	Reason                 string                  `json:"reason"`
	ReviewMetadata         string                  `json:"review_metadata"`
	ReserveReceiptID       int64                   `json:"reserve_receipt_id,omitempty"`
	Decision               *UsageReviewDecision    `json:"decision,omitempty"`
}

func GetUsageReview(ctx context.Context, db *gorm.DB, actorID int, requestID string) (*UsageReviewDetail, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var actor User
	if actorID <= 0 {
		return nil, ErrAccountQuotaMutationIneligible
	}
	if err := db.WithContext(ctx).First(&actor, actorID).Error; err != nil {
		return nil, err
	}
	if actor.Status != common.UserStatusEnabled {
		return nil, ErrAccountQuotaMutationIneligible
	}
	var head AccountQuotaReservationHead
	err := db.WithContext(ctx).Where("request_id = ?", requestID).First(&head).Error
	view := &UsageReviewDetail{RequestID: requestID}
	if err == nil {
		if head.RequestID != requestID {
			return nil, gorm.ErrRecordNotFound
		}
		if actor.Role != common.RoleRootUser && actor.Id != head.UserID {
			return nil, gorm.ErrRecordNotFound
		}
		var lifecycle AccountQuotaTerminalRecoveryObligation
		if err := db.WithContext(ctx).Where("request_id = ?", requestID).First(&lifecycle).Error; err != nil {
			return nil, err
		}
		view.UserID, view.TokenID, view.Writer, view.State = head.UserID, head.TokenID, "authoritative", lifecycle.State
		view.ReservedQuota, view.ReserveReceiptID, view.Reason, view.ReviewMetadata = head.AppliedQuota, head.CurrentReceiptID, lifecycle.LastError, lifecycle.ReviewMetadata
		var reserved AccountQuotaMutationReceipt
		if err := db.WithContext(ctx).First(&reserved, head.CurrentReceiptID).Error; err != nil {
			return nil, err
		}
		view.ChannelID = reserved.BillingContext.ChannelID
		view.ModelName = reserved.BillingContext.OriginModelName
		if head.TerminalReceiptID > 0 {
			var terminal AccountQuotaMutationReceipt
			if err := db.WithContext(ctx).First(&terminal, head.TerminalReceiptID).Error; err != nil {
				return nil, err
			}
			if terminal.Phase == AccountQuotaPhaseSettle {
				amount := terminal.RequestedQuota
				view.ActualQuota = &amount
			}
		}
	} else if errors.Is(err, gorm.ErrRecordNotFound) {
		row, err := FindLegacyUsageReservation(ctx, db, requestID)
		if err != nil {
			return nil, err
		}
		if actor.Role != common.RoleRootUser && actor.Id != row.UserID {
			return nil, gorm.ErrRecordNotFound
		}
		view.UserID, view.TokenID, view.Writer, view.State = row.UserID, row.TokenID, "legacy", row.State
		view.ReservedQuota, view.Reason, view.ReviewMetadata = row.ReservedQuota, row.Reason, row.ReviewMetadata
		view.ChannelID = row.ChannelID
		view.ModelName = row.ModelName
		if row.State == LegacyUsageSettled {
			view.ActualQuota = row.ActualQuota
		}
	} else {
		return nil, err
	}
	view.TextDispatchPending, _ = TextDispatchPending(view.ReviewMetadata)
	view.CanRecoverTextDispatch = view.TextDispatchPending && (view.State == LegacyUsagePrepared || view.State == AccountQuotaTerminalRecoveryOpen)
	var pricingFlags struct {
		StrictTokenBudget bool `json:"strict_token_budget"`
	}
	if common.UnmarshalJsonStr(view.ReviewMetadata, &pricingFlags) == nil && pricingFlags.StrictTokenBudget {
		var budget TokenBudgetReservation
		if err := db.WithContext(ctx).First(&budget, "request_id = ?", requestID).Error; err != nil {
			return nil, err
		}
		if budget.RequestID != requestID || budget.TokenID != view.TokenID || budget.UserID != view.UserID {
			return nil, ErrTokenBudgetConflict
		}
		if actor.Role != common.RoleRootUser {
			budget.EvidenceReference = ""
		}
		view.TokenBudget = &budget
	}
	if actor.Role == common.RoleRootUser {
		var decision UsageReviewDecision
		err := db.WithContext(ctx).Where("request_id = ?", requestID).First(&decision).Error
		if err == nil {
			view.Decision = &decision
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}
	return view, nil
}

type UsageReviewTokenCounts struct {
	FeeUSD *string `json:"fee_usd,omitempty"`
	Input  int64
	Output int64
}

func ReconcileUsageReview(ctx context.Context, db *gorm.DB, actorID int, requestID string, actual int64, evidence string, tokenCounts ...UsageReviewTokenCounts) (*UsageReviewDetail, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := authorizeUsageReviewer(db.WithContext(ctx), actorID); err != nil {
		return nil, err
	}
	evidence = strings.TrimSpace(evidence)
	if evidence == "" || len(evidence) > 2048 || validateAccountQuotaValue(actual) != nil {
		return nil, ErrAccountQuotaMutationInvalidInput
	}
	view, err := GetUsageReview(ctx, db, actorID, requestID)
	if err != nil {
		return nil, err
	}
	if err := validateTextDispatchRecoveryDecision(view.ReviewMetadata, actorID, actual, evidence); err != nil {
		return nil, err
	}
	if len(tokenCounts) > 1 || (view.TokenBudget != nil) != (len(tokenCounts) == 1) {
		return nil, ErrTokenBudgetInvalid
	}
	if len(tokenCounts) == 1 {
		counts := tokenCounts[0]
		if view.TokenBudget.FeeEnabled != (counts.FeeUSD != nil) {
			return nil, ErrFeeBudgetInvalid
		}
		if counts.FeeUSD != nil {
			normalized, err := NormalizeFeeBudgetUSD(*counts.FeeUSD)
			if err != nil {
				return nil, err
			}
			counts.FeeUSD = &normalized
			tokenCounts = []UsageReviewTokenCounts{counts}
			if view.TokenBudget.ActualFeeUSD != nil && *view.TokenBudget.ActualFeeUSD != normalized {
				return nil, ErrTokenBudgetConflict
			}
		}
		if counts.Input < 0 || counts.Output < 0 || counts.Input > int64(common.MaxQuota) || counts.Output > int64(common.MaxQuota)-counts.Input {
			return nil, ErrTokenBudgetInvalid
		}
		if view.TokenBudget.State == TokenBudgetPrepared || view.TokenBudget.State == TokenBudgetCancelled {
			return nil, ErrTokenBudgetPending
		}
		if view.TokenBudget.State == TokenBudgetSettled && (view.TokenBudget.ActualInput == nil || view.TokenBudget.ActualOutput == nil || *view.TokenBudget.ActualInput != counts.Input || *view.TokenBudget.ActualOutput != counts.Output) {
			return nil, ErrTokenBudgetConflict
		}
	}
	if err := validateReviewedQuotaObligations(view.ReviewMetadata, actual); err != nil {
		return nil, err
	}
	if view.State != AccountQuotaTerminalRecoveryUsageUnknown && view.State != LegacyUsageReviewPending && view.Decision == nil {
		return nil, ErrAccountQuotaUsageUnresolved
	}
	// Freeze automatic token settlement before committing a manual decision.
	// If a concurrent relay settled first, fail before changing quota and let
	// the caller reload the now-known actual counts. No User lock is held here.
	if view.TokenBudget != nil && view.TokenBudget.State == TokenBudgetSent {
		if _, err := MutateTokenBudgetRequest(ctx, db, TokenBudgetMutation{TokenID: view.TokenID, RequestID: requestID, Action: "hold", Reason: "manual_review"}); err != nil {
			return nil, err
		}
	}
	encoded, err := common.Marshal(struct {
		RequestID   string
		ActorID     int
		ActualQuota int64
		Evidence    string
	}{requestID, actorID, actual, evidence})
	if err != nil {
		return nil, err
	}
	if len(tokenCounts) == 1 {
		encoded, err = common.Marshal(struct {
			Review json.RawMessage
			Tokens UsageReviewTokenCounts
		}{encoded, tokenCounts[0]})
		if err != nil {
			return nil, err
		}
	}
	digest := sha256.Sum256(encoded)
	hash := hex.EncodeToString(digest[:])
	var decision UsageReviewDecision
	err = db.WithContext(ctx).Where("request_id = ?", requestID).First(&decision).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		now, timeErr := taskRecoveryDBTimestamp(db.WithContext(ctx))
		if timeErr != nil {
			return nil, timeErr
		}
		candidate := UsageReviewDecision{RequestID: requestID, ActorID: actorID, UserID: view.UserID, TokenID: view.TokenID, ChannelID: view.ChannelID, ModelName: view.ModelName, ActualQuota: actual, EvidenceReference: evidence, EvidenceDigest: hash, CreatedAt: now}
		if len(tokenCounts) == 1 {
			candidate.ActualInputTokens, candidate.ActualOutputTokens, candidate.ActualFeeUSD = &tokenCounts[0].Input, &tokenCounts[0].Output, tokenCounts[0].FeeUSD
		}
		createErr := db.WithContext(ctx).Create(&candidate).Error
		if createErr != nil {
			if readErr := db.WithContext(ctx).Where("request_id = ?", requestID).First(&decision).Error; readErr != nil {
				return nil, errors.Join(createErr, readErr)
			}
		} else {
			decision = candidate
		}
	} else if err != nil {
		return nil, err
	}
	if decision.ActorID != actorID || decision.ActualQuota != actual || decision.EvidenceDigest != hash || decision.EvidenceReference != evidence {
		return nil, ErrAccountQuotaMutationConflict
	}
	if len(tokenCounts) == 1 && (decision.ActualInputTokens == nil || decision.ActualOutputTokens == nil || *decision.ActualInputTokens != tokenCounts[0].Input || *decision.ActualOutputTokens != tokenCounts[0].Output) {
		return nil, ErrTokenBudgetConflict
	}
	if len(tokenCounts) == 1 && ((decision.ActualFeeUSD == nil) != (tokenCounts[0].FeeUSD == nil) || (decision.ActualFeeUSD != nil && *decision.ActualFeeUSD != *tokenCounts[0].FeeUSD)) {
		return nil, ErrTokenBudgetConflict
	}
	if view.Writer == "authoritative" {
		_, err = ResolveAccountQuotaUnknownUsage(ctx, db, actorID, AccountQuotaTerminalInput{RequestID: requestID, ReserveReceiptID: view.ReserveReceiptID, ActualQuota: actual}, hash)
	} else {
		_, err = ResolveLegacyUnknownUsage(ctx, db, actorID, requestID, actual, hash)
	}
	if err != nil {
		return nil, err
	}
	if err := finalizeUsageReviewTokenBudget(ctx, db, &decision); err != nil {
		return nil, err
	}
	return GetUsageReview(ctx, db, actorID, requestID)
}

// A review cannot silently erase independently known tool/per-response fees.
// This is a lower bound, not a reconstructed bill for the unknown token portion.
func validateReviewedQuotaObligations(metadata string, actual int64) error {
	if metadata == "" {
		return nil
	}
	var evidence struct {
		IncompletePricingEvidence bool                               `json:"incomplete_pricing_evidence"`
		QuotaUnit                 float64                            `json:"quota_unit"`
		UnitCaptured              bool                               `json:"quota_unit_captured"`
		GroupRatio                float64                            `json:"group_ratio"`
		KnownRealtimeQuota        *int64                             `json:"known_realtime_quota"`
		RealtimeCheckpoint        *hosttypes.RealtimeUsageCheckpoint `json:"realtime_usage_checkpoint"`
		Tools                     []struct {
			Count int64   `json:"count"`
			Price float64 `json:"price"`
		} `json:"known_tool_obligations"`
	}
	if len(metadata) > 16384 || common.UnmarshalJsonStr(metadata, &evidence) != nil {
		return ErrAccountQuotaMutationInvalidInput
	}
	if evidence.IncompletePricingEvidence {
		return ErrAccountQuotaUsageUnresolved
	}
	if evidence.RealtimeCheckpoint != nil && (!validRealtimeCheckpoint(evidence.RealtimeCheckpoint) || evidence.KnownRealtimeQuota == nil || int64(evidence.RealtimeCheckpoint.Quota) != *evidence.KnownRealtimeQuota) {
		return ErrAccountQuotaUsageUnresolved
	}
	if evidence.KnownRealtimeQuota != nil && (*evidence.KnownRealtimeQuota < 0 || actual < *evidence.KnownRealtimeQuota) {
		return ErrAccountQuotaMutationConflict
	}
	if len(evidence.Tools) == 0 {
		return nil
	}
	if !evidence.UnitCaptured || evidence.QuotaUnit <= 0 || evidence.GroupRatio < 0 || math.IsNaN(evidence.QuotaUnit) || math.IsInf(evidence.QuotaUnit, 0) || math.IsNaN(evidence.GroupRatio) || math.IsInf(evidence.GroupRatio, 0) {
		return ErrAccountQuotaUsageUnresolved
	}
	minimum := decimal.Zero
	for _, tool := range evidence.Tools {
		if tool.Count < 0 || tool.Count > int64(common.MaxQuota) || tool.Price < 0 || math.IsNaN(tool.Price) || math.IsInf(tool.Price, 0) {
			return ErrAccountQuotaMutationInvalidInput
		}
		minimum = minimum.Add(decimal.NewFromInt(tool.Count).Mul(decimal.NewFromFloat(tool.Price)).Shift(-3).Mul(decimal.NewFromFloat(evidence.QuotaUnit)).Mul(decimal.NewFromFloat(evidence.GroupRatio)))
	}
	// Preserve the old integral quota convention without inventing fractional
	// actual tokens or rounding the unknown portion into a confirmed charge.
	floor, clamp := common.QuotaFromDecimalChecked(minimum)
	if clamp != nil {
		return clamp
	}
	if actual < int64(floor) {
		return ErrAccountQuotaMutationConflict
	}
	return nil
}
