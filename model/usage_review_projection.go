package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/ForceMind/MyAPI/common"
	"gorm.io/gorm"
)

// A committed terminal receipt is the authority. A saved manual decision alone
// never authorizes statistics or a consumption-log projection.
func verifyUsageReviewQuotaTerminal(tx *gorm.DB, decision *UsageReviewDecision) error {
	var head AccountQuotaReservationHead
	err := tx.Where("request_id = ?", decision.RequestID).First(&head).Error
	if err == nil {
		if head.UserID != decision.UserID || head.TokenID != decision.TokenID || head.TerminalReceiptID <= 0 {
			return ErrAccountQuotaUsageUnresolved
		}
		var receipt AccountQuotaMutationReceipt
		if err := tx.First(&receipt, head.TerminalReceiptID).Error; err != nil {
			return err
		}
		if receipt.Phase != AccountQuotaPhaseSettle || receipt.RequestedQuota != decision.ActualQuota || receipt.AuditKey != fmt.Sprintf("usage-review:%d:%s", decision.ActorID, decision.EvidenceDigest) {
			return ErrAccountQuotaMutationConflict
		}
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	var row LegacyUsageReservation
	if err := tx.Where("request_id = ?", decision.RequestID).First(&row).Error; err != nil {
		return err
	}
	if row.State != LegacyUsageSettled || row.ActualQuota == nil {
		return ErrAccountQuotaUsageUnresolved
	}
	if row.UserID != decision.UserID || row.TokenID != decision.TokenID || *row.ActualQuota != decision.ActualQuota || row.ReviewedBy != decision.ActorID || row.EvidenceDigest != decision.EvidenceDigest {
		return ErrAccountQuotaMutationConflict
	}
	return nil
}

func verifyUsageReviewTerminal(tx *gorm.DB, decision *UsageReviewDecision) error {
	if err := verifyUsageReviewQuotaTerminal(tx, decision); err != nil {
		return err
	}
	if decision.ActualInputTokens == nil && decision.ActualOutputTokens == nil {
		return nil
	}
	if decision.ActualInputTokens == nil || decision.ActualOutputTokens == nil || *decision.ActualInputTokens < 0 || *decision.ActualOutputTokens < 0 || *decision.ActualInputTokens > int64(common.MaxQuota) || *decision.ActualOutputTokens > int64(common.MaxQuota)-*decision.ActualInputTokens {
		return ErrTokenBudgetInvalid
	}
	var budget TokenBudgetReservation
	if err := tx.First(&budget, "request_id = ?", decision.RequestID).Error; err != nil {
		return err
	}
	if budget.RequestID != decision.RequestID || budget.UserID != decision.UserID || budget.TokenID != decision.TokenID {
		return ErrTokenBudgetConflict
	}
	if budget.State != TokenBudgetSettled {
		return ErrAccountQuotaUsageUnresolved
	}
	if budget.ActualInput == nil || budget.ActualOutput == nil || *budget.ActualInput != *decision.ActualInputTokens || *budget.ActualOutput != *decision.ActualOutputTokens {
		return ErrTokenBudgetConflict
	}
	if budget.ReviewedBy != 0 && (budget.ReviewedBy != decision.ActorID || budget.EvidenceReference != decision.EvidenceReference) {
		return ErrTokenBudgetConflict
	}
	return nil
}

// The saved immutable decision is the recovery intent for both units. Quota
// must finish first; token finalization is replayable and remains blocking on
// failure. Do this before projection locks to retain User -> Token lock order.
func finalizeUsageReviewTokenBudget(ctx context.Context, db *gorm.DB, decision *UsageReviewDecision) error {
	if decision.ActualInputTokens == nil && decision.ActualOutputTokens == nil {
		return nil
	}
	if decision.ActualInputTokens == nil || decision.ActualOutputTokens == nil {
		return ErrTokenBudgetInvalid
	}
	if err := verifyUsageReviewQuotaTerminal(db.WithContext(ctx), decision); err != nil {
		return err
	}
	if err := verifyUsageReviewTerminal(db.WithContext(ctx), decision); err == nil {
		return nil
	} else if !errors.Is(err, ErrAccountQuotaUsageUnresolved) {
		return err
	}
	_, err := MutateTokenBudgetRequest(ctx, db, TokenBudgetMutation{TokenID: decision.TokenID, RequestID: decision.RequestID, Action: "reconcile",
		Input: *decision.ActualInputTokens, Output: *decision.ActualOutputTokens, ActorID: decision.ActorID, Evidence: decision.EvidenceReference})
	return err
}

// ProjectUsageReviewDecision applies counters exactly once on the primary DB,
// then projects a stable event to the log DB. The existing log canonicalization
// deduplicates repeated deliveries after a lost cross-database acknowledgement.
func ProjectUsageReviewDecision(ctx context.Context, db, logDB *gorm.DB, decisionID int64) error {
	if db == nil || logDB == nil {
		return gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var decision UsageReviewDecision
	if err := db.WithContext(ctx).First(&decision, decisionID).Error; err != nil {
		return err
	}
	if err := finalizeUsageReviewTokenBudget(ctx, db, &decision); err != nil {
		return err
	}
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).First(&decision, decisionID).Error; err != nil {
			return err
		}
		if err := verifyUsageReviewTerminal(tx, &decision); err != nil {
			return err
		}
		if decision.StatisticsApplied {
			return nil
		}
		var user User
		if err := lockForUpdate(tx).First(&user, decision.UserID).Error; err != nil {
			return err
		}
		if user.UsedQuota < 0 || decision.ActualQuota > int64(common.MaxQuota-user.UsedQuota) || user.RequestCount >= common.MaxQuota {
			return ErrAccountQuotaMutationInvalidInput
		}
		if err := tx.Model(&User{}).Where("id = ?", user.Id).Updates(map[string]interface{}{"used_quota": int64(user.UsedQuota) + decision.ActualQuota, "request_count": user.RequestCount + 1}).Error; err != nil {
			return err
		}
		if decision.ChannelID > 0 {
			var channel Channel
			lookup := lockForUpdate(tx).First(&channel, decision.ChannelID).Error
			if lookup != nil && !errors.Is(lookup, gorm.ErrRecordNotFound) {
				return lookup
			}
			if lookup == nil {
				if channel.UsedQuota < 0 || decision.ActualQuota > int64(common.MaxQuota-channel.UsedQuota) {
					return ErrAccountQuotaMutationInvalidInput
				}
				if err := tx.Model(&Channel{}).Where("id = ?", channel.Id).Update("used_quota", int64(channel.UsedQuota)+decision.ActualQuota).Error; err != nil {
					return err
				}
			}
		}
		// Only delivery bookkeeping is mutable. Evidence fields stay immutable.
		result := tx.Table("usage_review_decisions").Where("id = ? AND statistics_applied = ?", decision.ID, false).Update("statistics_applied", true)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrAccountQuotaMutationCASLost
		}
		decision.StatisticsApplied = true
		return nil
	})
	if err != nil {
		return err
	}
	if decision.LogProjected {
		return nil
	}
	metadata, err := common.Marshal(map[string]interface{}{"settlement_status": "manually_reconciled", "usage_accuracy": "unknown", "actual_quota": decision.ActualQuota, "token_counts_confirmed": decision.ActualInputTokens != nil, "review_decision_id": decision.ID})
	if err != nil {
		return err
	}
	hash := sha256.Sum256([]byte("usage-review:" + decision.EvidenceDigest))
	record := &Log{UserId: decision.UserID, TokenId: decision.TokenID, ChannelId: decision.ChannelID, CreatedAt: decision.CreatedAt,
		ModelName: decision.ModelName,
		Type:      LogTypeConsume, Content: "usage_manually_reconciled", Quota: int(decision.ActualQuota), RequestId: decision.RequestID,
		BillingEventID: hex.EncodeToString(hash[:]), Other: string(metadata)}
	if decision.ActualInputTokens != nil && decision.ActualOutputTokens != nil {
		record.PromptTokens, record.CompletionTokens = int(*decision.ActualInputTokens), int(*decision.ActualOutputTokens)
	}
	if err := CreateLog(logDB.WithContext(ctx), record); err != nil {
		return err
	}
	return db.WithContext(ctx).Table("usage_review_decisions").Where("id = ? AND statistics_applied = ?", decision.ID, true).Update("log_projected", true).Error
}

func RunUsageReviewProjections(ctx context.Context, db, logDB *gorm.DB, limit int) (int, error) {
	if db == nil || logDB == nil {
		return 0, gorm.ErrInvalidDB
	}
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	if ctx == nil {
		ctx = context.Background()
	}
	cursor, err := loadQuotaWorkCursor(ctx, db, "usage_review_projection_v1")
	if err != nil {
		return 0, err
	}
	if err := beginQuotaWorkCycle(ctx, db, cursor, &UsageReviewDecision{}); err != nil {
		return 0, err
	}
	var rows []UsageReviewDecision
	if err := db.WithContext(ctx).Where("id > ? AND id <= ? AND log_projected = ?", cursor.LastID, cursor.HighWatermark, false).Order("id ASC").Limit(limit).Find(&rows).Error; err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, finishQuotaWorkCycle(ctx, db, cursor)
	}
	processed := 0
	var failures []error
	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return processed, errors.Join(append(failures, err)...)
		}
		cursor.LastID = row.ID
		if err := ProjectUsageReviewDecision(ctx, db, logDB, row.ID); err != nil {
			if !errors.Is(err, ErrAccountQuotaUsageUnresolved) {
				failures = append(failures, err)
			}
		} else {
			processed++
		}
	}
	if err := saveQuotaWorkCursor(ctx, db, cursor); err != nil {
		failures = append(failures, err)
	}
	return processed, errors.Join(failures...)
}
