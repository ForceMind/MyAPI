package model

import (
	"context"
	"errors"

	"github.com/ForceMind/MyAPI/common"
	"gorm.io/gorm"
)

// A Root-confirmed review can recover a lost post-dispatch hold using the
// pricing evidence saved before dispatch. It never releases the token reserve
// or manufactures a terminal amount; the ordinary audited review follows.
func PrepareTokenBudgetUsageReview(ctx context.Context, db *gorm.DB, actorID, tokenID int, requestID string, feeUSD ...*string) (*UsageReviewDetail, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := authorizeUsageReviewer(db.WithContext(ctx), actorID); err != nil {
		return nil, err
	}
	var budget TokenBudgetReservation
	if err := db.WithContext(ctx).First(&budget, "request_id = ?", requestID).Error; err != nil {
		return nil, err
	}
	if budget.RequestID != requestID || budget.TokenID != tokenID || budget.BoundSource != TokenBudgetBoundOpenAIResponses {
		return nil, ErrTokenBudgetConflict
	}
	if len(feeUSD) > 1 || budget.FeeEnabled != (len(feeUSD) == 1 && feeUSD[0] != nil) {
		return nil, ErrFeeBudgetInvalid
	}
	if budget.FeeEnabled {
		normalized, err := NormalizeFeeBudgetUSD(*feeUSD[0])
		if err != nil {
			return nil, err
		}
		if budget.ActualFeeUSD != nil && *budget.ActualFeeUSD != normalized {
			return nil, ErrTokenBudgetConflict
		}
	}
	if budget.State != TokenBudgetSent && budget.State != TokenBudgetUnknown && budget.State != TokenBudgetSettled {
		return nil, ErrTokenBudgetPending
	}
	var evidence struct {
		StrictTokenBudget bool `json:"strict_token_budget"`
	}
	if common.UnmarshalJsonStr(budget.PricingEvidence, &evidence) != nil || !evidence.StrictTokenBudget {
		return nil, ErrAccountQuotaUsageUnresolved
	}
	view, err := GetUsageReview(ctx, db, actorID, requestID)
	if err != nil {
		return nil, err
	}
	if view.UserID != budget.UserID || view.TokenID != budget.TokenID {
		return nil, ErrTokenBudgetConflict
	}
	if view.TokenBudget != nil {
		return view, nil
	}
	if view.Writer == "authoritative" && view.State == AccountQuotaTerminalRecoveryOpen {
		_, err = HoldAccountQuotaUnknownUsage(ctx, db, requestID, view.ReserveReceiptID, "missing", budget.PricingEvidence)
	} else if view.Writer == "legacy" && view.State == LegacyUsagePrepared {
		row, readErr := FindLegacyUsageReservation(ctx, db, requestID)
		if readErr != nil {
			return nil, readErr
		}
		err = UpdateLegacyUsageReservation(ctx, db, requestID, row.ReservedQuota, row.TokenReservedQuota, LegacyUsageUnknown, "missing", nil, budget.PricingEvidence)
	} else {
		return nil, ErrAccountQuotaUsageUnresolved // Do not replace prior evidence.
	}
	if err != nil {
		return nil, err
	}
	return GetUsageReview(ctx, db, actorID, requestID)
}

// CancelTokenBudgetBeforeSend records the audited no-send proof and the old
// quota refund intent together. The old recovery engine applies that intent;
// neither a sent request nor unknown usage can take this path.
func CancelTokenBudgetBeforeSend(ctx context.Context, db *gorm.DB, actorID, tokenID int, requestID, evidence string) (*TokenBudgetReservation, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if actorID <= 0 {
		return nil, ErrTokenBudgetInvalid
	}
	if err := authorizeUsageReviewer(db.WithContext(ctx), actorID); err != nil {
		return nil, err
	}
	var result *TokenBudgetReservation
	var fact *AccountQuotaRefundFact
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		result, err = MutateTokenBudgetRequest(ctx, tx, TokenBudgetMutation{TokenID: tokenID, RequestID: requestID, Action: "cancel", ActorID: actorID, Evidence: evidence})
		if err != nil {
			return err
		}
		var head AccountQuotaReservationHead
		err = tx.First(&head, "request_id = ?", requestID).Error
		input := AccountQuotaRefundFactInput{RequestID: requestID, UserID: result.UserID, TokenID: result.TokenID}
		if err == nil {
			if head.RequestID != requestID || head.UserID != result.UserID || head.TokenID != result.TokenID {
				return ErrTokenBudgetConflict
			}
			var receipt AccountQuotaMutationReceipt
			if err := tx.First(&receipt, head.CurrentReceiptID).Error; err != nil {
				return err
			}
			input.Kind, input.EventKey, input.AuditKey = AccountQuotaRefundFactKindAuthoritative, "billing-refund:"+requestID+":v1", "upstream-failure:"+requestID
			input.ReserveReceiptID, input.WriterEpoch = receipt.ID, receipt.WriterEpoch
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			legacy, err := FindLegacyUsageReservation(ctx, tx, requestID)
			if err != nil {
				return err
			}
			if legacy.UserID != result.UserID || legacy.TokenID != result.TokenID || (legacy.State != LegacyUsagePrepared && legacy.State != LegacyUsageRefunded) {
				return ErrTokenBudgetConflict
			}
			input.EventKey, input.TokenQuota = "billing-refund:"+requestID+":v2", legacy.TokenReservedQuota
			switch legacy.FundingSource {
			case BillingSourceSelfUse:
				input.Kind = AccountQuotaRefundFactKindLegacySelfUse
			case "wallet":
				input.Kind, input.WalletQuota = AccountQuotaRefundFactKindLegacyWallet, legacy.ReservedQuota
			case "subscription":
				input.Kind, input.SubscriptionID, input.SubscriptionQuota = AccountQuotaRefundFactKindLegacySubscription, legacy.SubscriptionID, legacy.ReservedQuota
			default:
				return ErrTokenBudgetUnsupportedFunding
			}
		} else {
			return err
		}
		fact, err = EnsureAccountQuotaRefundFact(ctx, tx, input)
		return err
	})
	if err != nil {
		return nil, err
	}
	stored, _, err := RecoverAccountQuotaRefundFact(ctx, db, fact, "token-budget-cancel:"+requestID)
	if err != nil {
		return result, err
	}
	if stored == nil || stored.State != AccountQuotaRefundFactApplied {
		return result, ErrAccountQuotaRefundPending
	}
	if fact.Kind != AccountQuotaRefundFactKindAuthoritative {
		legacy, readErr := FindLegacyUsageReservation(ctx, db, requestID)
		if readErr != nil {
			return result, readErr
		}
		if err := UpdateLegacyUsageReservation(ctx, db, requestID, legacy.ReservedQuota, legacy.TokenReservedQuota, LegacyUsageRefunded, "", nil); err != nil {
			return result, err
		}
	}
	return result, nil
}

var ErrTokenBudgetUnsupportedFunding = errors.New("strict token budget has unsupported quota funding")
