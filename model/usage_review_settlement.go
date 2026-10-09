package model

import (
	"context"

	"gorm.io/gorm"
)

// projectUsageReviewSettlement is a read-only view of the durable accounting
// owner. Journal state and dispatch metadata remain untouched; neither an
// intended amount nor a partially applied fact is a confirmed final charge.
func projectUsageReviewSettlement(ctx context.Context, db *gorm.DB, view *UsageReviewDetail, head *AccountQuotaReservationHead, legacy *LegacyUsageReservation) error {
	var intents []AccountQuotaSettlementIntent
	if err := db.WithContext(ctx).Where("request_id = ?", view.RequestID).Limit(2).Find(&intents).Error; err != nil {
		return ErrAccountQuotaSettlementFactUnknown
	}
	var facts []AccountQuotaSettlementFact
	if err := db.WithContext(ctx).Where("request_id = ?", view.RequestID).Limit(2).Find(&facts).Error; err != nil {
		return ErrAccountQuotaSettlementFactUnknown
	}
	if len(intents) == 0 && len(facts) == 0 {
		return nil
	}
	// Any existing record blocks takeover, including conflicting or unsupported
	// records. Their internal errors and accounting identity are never serialized.
	view.CanRecoverTextDispatch = false
	view.CanReconcileUsage = false
	view.SettlementStatus = "manual"
	view.RecoveryBlockReason = "automatic_settlement_manual"
	if len(intents) > 1 || len(facts) > 1 {
		return nil
	}
	var input AccountQuotaSettlementFactInput
	var intent *AccountQuotaSettlementIntent
	var fact *AccountQuotaSettlementFact
	if len(intents) == 1 {
		intent = &intents[0]
		input = settlementFactInputFromIntent(intent)
	}
	if len(facts) == 1 {
		fact = &facts[0]
		input = AccountQuotaSettlementFactInput{EventKey: fact.EventKey, RequestID: fact.RequestID, Kind: fact.Kind, UserID: fact.UserID, TokenID: fact.TokenID, SubscriptionID: fact.SubscriptionID, ReserveReceiptID: fact.ReserveReceiptID, WriterEpoch: fact.WriterEpoch, ActualQuota: fact.ActualQuota, Delta: fact.Delta, ApplyToken: fact.ApplyToken}
	}
	normalized, fingerprint, err := normalizeAccountQuotaSettlementInput(input)
	if err != nil || normalized != input || input.RequestID != view.RequestID || input.UserID != view.UserID || input.TokenID != view.TokenID {
		return nil
	}
	if intent != nil && validateAccountQuotaSettlementIntent(intent, input, fingerprint) != nil {
		return nil
	}
	if fact != nil && validateAccountQuotaSettlementIdentity(fact, input, fingerprint) != nil {
		return nil
	}
	if intent != nil && fact != nil && intent.FactID != 0 && intent.FactID != fact.ID {
		return nil
	}
	actual := input.ActualQuota
	if legacy != nil {
		kind := AccountQuotaSettlementKindLegacyWallet
		switch legacy.FundingSource {
		case "wallet":
		case "subscription":
			kind = AccountQuotaSettlementKindLegacySubscription
		case BillingSourceSelfUse:
			if legacy.UsagePolicyRevision <= 0 {
				return nil
			}
			kind = AccountQuotaSettlementKindLegacySelfUse
		default:
			return nil
		}
		if input.Kind != kind || input.SubscriptionID != legacy.SubscriptionID || validateAccountQuotaValue(legacy.ReservedQuota) != nil {
			return nil
		}
		// Both operands are bounded before addition; a negative delta may represent
		// a legitimate release, but a negative or oversized final charge may not.
		actual = legacy.ReservedQuota + input.Delta
		if validateAccountQuotaValue(actual) != nil {
			return nil
		}
	} else if input.Kind != AccountQuotaSettlementKindAuthoritative || input.ReserveReceiptID != head.CurrentReceiptID || input.WriterEpoch != head.WriterEpoch || input.SubscriptionID != head.SubscriptionID {
		return nil
	}
	if intent != nil && !usageReviewSettlementKnownState(intent.State) || fact != nil && !usageReviewSettlementKnownState(fact.State) {
		return nil
	}
	// A persisted authoritative terminal receipt already proves accounting even
	// if the worker lost the response before acknowledging its intent or fact.
	applied := legacy == nil && view.ActualQuota != nil
	if fact != nil && fact.State == AccountQuotaSettlementApplied {
		if !fact.FundingApplied || !fact.TokenApplied {
			return nil
		}
		if legacy == nil && (view.ActualQuota == nil || head.TerminalReceiptID == 0 || fact.TerminalReceiptID != head.TerminalReceiptID) {
			return nil
		}
		applied = true
	}
	if applied {
		if view.ActualQuota != nil && *view.ActualQuota != actual {
			return nil
		}
		if legacy != nil && legacy.State != LegacyUsagePrepared && legacy.State != LegacyUsageSettled && !(view.TokenBudget != nil && legacy.State == LegacyUsageUnknown) {
			return nil
		}
		if legacy != nil {
			// A reservation extension may have committed between the journal and
			// fact reads. Never add a final delta to an older reservation amount.
			var unchanged int64
			if err := db.WithContext(ctx).Model(&LegacyUsageReservation{}).Where("id = ? AND lock_version = ?", legacy.ID, legacy.LockVersion).Count(&unchanged).Error; err != nil || unchanged != 1 {
				return ErrAccountQuotaSettlementFactUnknown
			}
		}
		view.ActualQuota = &actual
		view.TextDispatchPending = false
		view.SettlementStatus = "applied"
		view.RecoveryBlockReason = "automatic_settlement_applied"
		if view.State != LegacyUsageSettled && view.State != AccountQuotaTerminalRecoveryApplied || intent != nil && intent.State != AccountQuotaSettlementApplied {
			view.SettlementStatus = "applied_journal_pending"
		}
		// Accounting completion does not itself complete an independent strict
		// token/fee review. Preserve its established audited reconciliation path.
		view.CanReconcileUsage = view.TokenBudget != nil && (view.State == AccountQuotaTerminalRecoveryUsageUnknown || view.State == LegacyUsageReviewPending)
		if view.CanReconcileUsage {
			view.RecoveryBlockReason = ""
		}
		return nil
	}
	if intent != nil && (intent.State == AccountQuotaSettlementManual || intent.State == AccountQuotaSettlementApplied) || fact != nil && fact.State == AccountQuotaSettlementManual {
		return nil
	}
	view.SettlementStatus = "pending"
	view.RecoveryBlockReason = "automatic_settlement_pending"
	return nil
}

func usageReviewSettlementKnownState(state string) bool {
	switch state {
	case AccountQuotaSettlementPending, AccountQuotaSettlementMaterialized, AccountQuotaSettlementClaimed, AccountQuotaSettlementRetryable, AccountQuotaSettlementApplied, AccountQuotaSettlementManual:
		return true
	default:
		return false
	}
}
