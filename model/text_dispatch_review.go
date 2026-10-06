package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/ForceMind/MyAPI/common"
	"gorm.io/gorm"
)

// The create transaction shares the lifecycle lock with manual takeover. A
// check before Create alone cannot prevent a late automatic intent racing a hold.
func (intent *AccountQuotaSettlementIntent) BeforeCreate(tx *gorm.DB) error {
	return guardUsageSettlementCreation(tx, settlementFactInputFromIntent(intent))
}
func (fact *AccountQuotaSettlementFact) BeforeCreate(tx *gorm.DB) error {
	return guardUsageSettlementCreation(tx, AccountQuotaSettlementFactInput{EventKey: fact.EventKey, RequestID: fact.RequestID, Kind: fact.Kind, UserID: fact.UserID, TokenID: fact.TokenID, SubscriptionID: fact.SubscriptionID, ReserveReceiptID: fact.ReserveReceiptID, WriterEpoch: fact.WriterEpoch, ActualQuota: fact.ActualQuota, Delta: fact.Delta, ApplyToken: fact.ApplyToken})
}
func guardUsageSettlementCreation(tx *gorm.DB, input AccountQuotaSettlementFactInput) error {
	if input.Kind != AccountQuotaSettlementKindAuthoritative {
		if err := validateLegacyUsageSettlement(tx, input); err != nil {
			return err
		}
		var row LegacyUsageReservation
		err := lockForUpdate(tx).Where("request_id = ?", input.RequestID).First(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if row.ReviewedBy > 0 && (row.State == LegacyUsageSettled || input.EventKey != "billing-settlement:"+input.RequestID+":v1") {
			return ErrAccountQuotaUsageUnresolved
		}
		return nil
	}
	var head AccountQuotaReservationHead
	if err := lockForUpdate(tx).Where("request_id = ?", input.RequestID).First(&head).Error; err != nil {
		return err
	}
	if head.RequestID != input.RequestID || head.UserID != input.UserID || head.TokenID != input.TokenID {
		return ErrAccountQuotaMutationConflict
	}
	var lifecycle AccountQuotaTerminalRecoveryObligation
	if err := lockForUpdate(tx).Where("request_id = ?", input.RequestID).First(&lifecycle).Error; err != nil {
		return err
	}
	if lifecycle.State == AccountQuotaTerminalRecoveryApplied {
		var terminal AccountQuotaMutationReceipt
		if err := tx.First(&terminal, head.TerminalReceiptID).Error; err != nil {
			return err
		}
		if strings.HasPrefix(terminal.AuditKey, "usage-review:") {
			return ErrAccountQuotaUsageUnresolved
		}
		if terminal.Phase != AccountQuotaPhaseSettle || terminal.RequestedQuota != input.ActualQuota {
			return ErrAccountQuotaMutationConflict
		}
		return nil
	}
	if lifecycle.State != AccountQuotaTerminalRecoveryOpen {
		return ErrAccountQuotaUsageUnresolved
	}
	return nil
}

type textDispatchRecoveryEvidence struct {
	Actor    int    `json:"actor_id"`
	Evidence string `json:"evidence_sha256"`
	Actual   int64  `json:"actual_quota"`
	Finished bool   `json:"confirmed_finished"`
}

func validateTextDispatchRecoveryDecision(metadata string, actor int, actual int64, evidence string) error {
	if metadata == "" {
		return nil
	}
	var saved struct {
		Recovery *textDispatchRecoveryEvidence `json:"text_dispatch_recovery"`
	}
	if common.UnmarshalJsonStr(metadata, &saved) != nil {
		return ErrAccountQuotaUsageUnresolved
	}
	if saved.Recovery == nil {
		return nil
	}
	digest := sha256.Sum256([]byte(evidence))
	if !saved.Recovery.Finished || saved.Recovery.Actor != actor || saved.Recovery.Actual != actual || saved.Recovery.Evidence != hex.EncodeToString(digest[:]) {
		return ErrAccountQuotaMutationConflict
	}
	return nil
}

// RecoverTextDispatchUsage requires an explicit finished-request confirmation
// at the API. It cannot infer no-send from age or refund an unknown request.
func RecoverTextDispatchUsage(ctx context.Context, db *gorm.DB, actorID int, requestID string, actual int64, evidence string) (*UsageReviewDetail, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	evidence = strings.TrimSpace(evidence)
	if validateAccountQuotaValue(actual) != nil || evidence == "" || len(evidence) > 2048 {
		return nil, ErrAccountQuotaMutationInvalidInput
	}
	if err := authorizeUsageReviewer(db.WithContext(ctx), actorID); err != nil {
		return nil, err
	}
	view, err := GetUsageReview(ctx, db, actorID, requestID)
	if err != nil {
		return nil, err
	}
	if view.TokenBudget != nil {
		return nil, ErrTokenBudgetInvalid
	}
	if view.Decision != nil || view.State == LegacyUsageReviewPending || view.State == AccountQuotaTerminalRecoveryUsageUnknown {
		return ReconcileUsageReview(ctx, db, actorID, requestID, actual, evidence)
	}
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := authorizeUsageReviewer(tx, actorID); err != nil {
			return err
		}
		var metadata string
		var reserved, tokenReserved, reserveID int64
		if view.Writer == "authoritative" {
			var head AccountQuotaReservationHead
			if err := lockForUpdate(tx).Where("request_id = ?", requestID).First(&head).Error; err != nil {
				return err
			}
			if head.RequestID != requestID || head.TerminalReceiptID != 0 {
				return ErrAccountQuotaMutationConflict
			}
			var lifecycle AccountQuotaTerminalRecoveryObligation
			if err := lockForUpdate(tx).Where("request_id = ?", requestID).First(&lifecycle).Error; err != nil {
				return err
			}
			if lifecycle.State != AccountQuotaTerminalRecoveryOpen {
				return ErrAccountQuotaUsageUnresolved
			}
			metadata, reserveID = lifecycle.ReviewMetadata, head.CurrentReceiptID
		} else {
			var row LegacyUsageReservation
			if err := lockForUpdate(tx).Where("request_id = ?", requestID).First(&row).Error; err != nil {
				return err
			}
			if row.RequestID != requestID || row.State != LegacyUsagePrepared {
				return ErrAccountQuotaUsageUnresolved
			}
			metadata, reserved, tokenReserved = row.ReviewMetadata, row.ReservedQuota, row.TokenReservedQuota
		}
		pending, err := TextDispatchPending(metadata)
		if err != nil || !pending {
			return ErrAccountQuotaUsageUnresolved
		}
		if err := validateReviewedQuotaObligations(metadata, actual); err != nil {
			return err
		}
		// A known settlement already owns recovery. Never replace it with a manual
		// amount. Locking reads see committed contenders before the lifecycle fence.
		var intent AccountQuotaSettlementIntent
		err = lockForUpdate(tx).Where("request_id = ?", requestID).First(&intent).Error
		if err == nil {
			return ErrAccountQuotaSettlementPending
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var fact AccountQuotaSettlementFact
		err = lockForUpdate(tx).Where("request_id = ?", requestID).First(&fact).Error
		if err == nil {
			return ErrAccountQuotaSettlementPending
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var fields map[string]json.RawMessage
		if common.UnmarshalJsonStr(metadata, &fields) != nil || fields == nil {
			return ErrAccountQuotaUsageUnresolved
		}
		digest := sha256.Sum256([]byte(evidence))
		audit, err := common.Marshal(textDispatchRecoveryEvidence{actorID, hex.EncodeToString(digest[:]), actual, true})
		if err != nil {
			return err
		}
		fields["text_dispatch_recovery"] = audit
		encoded, err := common.Marshal(fields)
		if err != nil || len(encoded) > 16384 {
			return ErrAccountQuotaMutationInvalidInput
		}
		if view.Writer == "authoritative" {
			_, err = HoldAccountQuotaUnknownUsage(ctx, tx, requestID, reserveID, "missing", string(encoded))
		} else {
			err = UpdateLegacyUsageReservation(ctx, tx, requestID, reserved, tokenReserved, LegacyUsageUnknown, "missing", nil, string(encoded))
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return ReconcileUsageReview(ctx, db, actorID, requestID, actual, evidence)
}

type UsageReviewPage struct {
	Items     []*UsageReviewDetail `json:"items"`
	NextAfter string               `json:"next_after"`
}

// A bounded keyset scan, not a COUNT or an unbounded scan of all historical
// requests. Empty pages may carry a cursor when non-text open rows were skipped.
func ListPendingUsageReviews(ctx context.Context, db *gorm.DB, actorID int, writer string, after int64) (*UsageReviewPage, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if after < 0 || (writer != "legacy" && writer != "authoritative") {
		return nil, ErrAccountQuotaMutationInvalidInput
	}
	if err := authorizeUsageReviewer(db.WithContext(ctx), actorID); err != nil {
		return nil, err
	}
	type candidate struct {
		ID                               int64
		RequestID, State, ReviewMetadata string
	}
	var rows []candidate
	var entity any = &AccountQuotaTerminalRecoveryObligation{}
	states := []string{AccountQuotaTerminalRecoveryOpen, AccountQuotaTerminalRecoveryUsageUnknown}
	if writer == "legacy" {
		entity = &LegacyUsageReservation{}
		states = []string{LegacyUsagePrepared, LegacyUsageUnknown, LegacyUsageReviewPending}
	}
	if err := db.WithContext(ctx).Model(entity).Select("id", "request_id", "state", "review_metadata").Where("id > ? AND state IN ?", after, states).Order("id ASC").Limit(100).Find(&rows).Error; err != nil {
		return nil, err
	}
	page := &UsageReviewPage{Items: []*UsageReviewDetail{}}
	for i, row := range rows {
		pending, err := TextDispatchPending(row.ReviewMetadata)
		if err != nil || pending || row.State == LegacyUsageUnknown || row.State == LegacyUsageReviewPending {
			detail, err := GetUsageReview(ctx, db, actorID, row.RequestID)
			if err != nil {
				return nil, err
			}
			if detail.State == LegacyUsagePrepared || detail.State == AccountQuotaTerminalRecoveryOpen || detail.State == LegacyUsageUnknown || detail.State == LegacyUsageReviewPending {
				page.Items = append(page.Items, detail)
			}
		}
		if len(page.Items) == 20 || i == 99 {
			page.NextAfter = strconv.FormatInt(row.ID, 10)
			break
		}
	}
	return page, nil
}
