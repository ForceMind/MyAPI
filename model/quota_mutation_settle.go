package model

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/ForceMind/MyAPI/common"
	"gorm.io/gorm"
)

var (
	ErrTaskQuotaAlreadySettled         = errors.New("task quota is already settled")
	ErrTaskQuotaAlreadyRefunded        = errors.New("task quota is already refunded")
	ErrTaskQuotaSettlementInvalidInput = errors.New("invalid task quota settlement input")
	ErrTaskQuotaReleaseInvalidInput    = errors.New("invalid task quota release input")
)

// TaskQuotaReleaseInput is the complete gate-off T4 refund release input.
type TaskQuotaReleaseInput struct {
	OperationID               int64
	UserID                    int
	TokenID                   int
	ChannelID                 int
	ExpectedOperationVersion  int64
	ReasonCode                string
	BillingContext            TaskBillingContext
	TargetOperationStatus     TaskSubmissionOperationStatus
	ResolutionSource          string
	RequireStatisticsEvidence bool
	EvidenceID                string
	EvidenceHash              string
	EvidenceVersion           int
	RequestID                 string
}

// TaskQuotaSettlementInput is the complete gate-off T3 terminal settlement input.
type TaskQuotaSettlementInput struct {
	OperationID               int64
	UserID                    int
	TokenID                   int
	ChannelID                 int
	ExpectedOperationVersion  int64
	ActualQuota               int64
	ReasonCode                string
	BillingContext            TaskBillingContext
	TargetOperationStatus     TaskSubmissionOperationStatus
	ResolutionSource          string
	RequireStatisticsEvidence bool
	EvidenceID                string
	EvidenceHash              string
	EvidenceVersion           int
	RequestID                 string
}

// ReleaseTaskQuotaReservation applies gate-off T4 atomically. It fully releases
// previously reserved task quota back to the user, token, and subscription.
func ReleaseTaskQuotaReservation(tx *gorm.DB, input TaskQuotaReleaseInput) (*QuotaMutationReceipt, error) {
	if tx == nil {
		return nil, gorm.ErrInvalidDB
	}
	normalized, fingerprint, err := normalizeTaskQuotaReleaseInput(input)
	if err != nil {
		return nil, err
	}

	// Must have a valid T1 reservation receipt
	reserveReceipt, err := findTaskQuotaReservation(tx, normalized.OperationID, normalized.UserID, normalized.TokenID)
	if err != nil {
		return nil, err
	}
	if reserveReceipt == nil {
		return nil, ErrTaskQuotaReservationNotFound
	}
	if (input.RequireStatisticsEvidence || reserveReceipt.RequestFingerprintVersion >= 2) && (!reserveReceipt.StatisticsApplied || reserveReceipt.StatisticsVersion != 1) {
		return nil, ErrTaskTerminalObservationManualReview
	}

	var result *QuotaMutationReceipt
	err = taskRecoveryAtomicTransaction(tx, "task_quota_release", func(writeDB *gorm.DB) error {
		databaseNow, err := taskRecoveryDBTimestamp(writeDB)
		if err != nil {
			return err
		}

		// 1. Lock User
		var user User
		if err := lockForUpdate(writeDB).Where("id = ?", normalized.UserID).First(&user).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("%w: user does not exist", ErrTaskQuotaReservationIneligible)
			}
			return err
		}
		if !quotaMutationVersionValid(user.QuotaVersion) {
			return fmt.Errorf("%w: user has an invalid quota version", ErrTaskQuotaReservationIneligible)
		}

		// 2. Lock Token
		var token Token
		if err := lockForUpdate(writeDB).Where("id = ?", normalized.TokenID).First(&token).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("%w: token does not exist", ErrTaskQuotaReservationIneligible)
			}
			return err
		}
		if token.UserId != normalized.UserID || !quotaMutationVersionValid(token.QuotaVersion) {
			return fmt.Errorf("%w: token owner or quota version is invalid", ErrTaskQuotaReservationIneligible)
		}

		// 3. Lock Subscription if applicable
		var subscription *UserSubscription
		if reserveReceipt.BillingSource == "subscription" {
			var locked UserSubscription
			if err := lockForUpdate(writeDB).Where("id = ?", reserveReceipt.SubscriptionID).First(&locked).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return fmt.Errorf("%w: subscription does not exist", ErrTaskQuotaReservationIneligible)
				}
				return err
			}
			if locked.UserId != normalized.UserID || !quotaMutationVersionValid(locked.QuotaVersion) {
				return fmt.Errorf("%w: subscription owner or quota version is invalid", ErrTaskQuotaReservationIneligible)
			}
			subscription = &locked
		}

		// 4. Lock Operation
		var operation TaskSubmissionOperation
		if err := lockForUpdate(writeDB).Where("id = ?", normalized.OperationID).First(&operation).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("%w: operation does not exist", ErrTaskQuotaReservationIneligible)
			}
			return err
		}
		if err := validateStoredTaskSubmissionOperation(writeDB, &operation); err != nil {
			return err
		}

		// 5. Lock Attempt
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
		if !taskMutationEvidenceMatchesAttempt(normalized.ResolutionSource, normalized.EvidenceID, normalized.EvidenceHash, normalized.EvidenceVersion, &attempt) {
			return ErrTaskQuotaReleaseInvalidInput
		}
		if existing, err := findTaskQuotaReceiptByType(writeDB, normalized.OperationID, string(TaskBillingEventTypeRefund), normalized.UserID, normalized.TokenID); err != nil {
			return err
		} else if existing != nil {
			if existing.RequestFingerprint != fingerprint {
				return ErrTaskQuotaReservationConflict
			}
			result = existing
			return nil
		}
		if settled, err := findTaskQuotaReceiptByType(writeDB, normalized.OperationID, string(TaskBillingEventTypeTerminalSettlement), normalized.UserID, normalized.TokenID); err != nil {
			return err
		} else if settled != nil {
			return ErrTaskQuotaAlreadySettled
		}
		if operation.UserID != normalized.UserID || operation.TokenID != normalized.TokenID || operation.LockVersion != normalized.ExpectedOperationVersion || operation.LockVersion == quotaMutationMaxInt64 {
			return ErrTaskQuotaReservationCASLost
		}

		refundQuota := reserveReceipt.Quota
		before := quotaMutationSnapshot(&user, &token, subscription)
		after, err := quotaMutationRefundAfter(before, reserveReceipt.BillingSource, refundQuota, databaseNow)
		if err != nil {
			return err
		}

		if err := applyTaskQuotaRefundBalances(writeDB, before, after, reserveReceipt.BillingSource, refundQuota); err != nil {
			return err
		}
		if reserveReceipt.StatisticsApplied {
			if err := applyTaskStatistics(writeDB, normalized.UserID, normalized.ChannelID, -refundQuota, 0); err != nil {
				return err
			}
		}

		operationID := operation.ID
		eventKey := "task:" + operation.PublicID + ":" + string(TaskBillingEventTypeRefund) + ":v1"
		event, err := CreateOrLoadTaskBillingEvent(writeDB, &TaskBillingEvent{
			OperationID:    &operationID,
			EventType:      TaskBillingEventTypeRefund,
			UserID:         normalized.UserID,
			TokenID:        normalized.TokenID,
			ChannelID:      normalized.ChannelID,
			BillingSource:  reserveReceipt.BillingSource,
			SubscriptionID: reserveReceipt.SubscriptionID,
			QuotaDelta:     refundQuota,
			ReasonCode:     normalized.ReasonCode,
			RequestID:      stableTaskBillingRequestID(taskMutationRequestID(normalized.RequestID, reserveReceipt.RequestID), eventKey),
			ResolutionSource: func() string {
				if normalized.ResolutionSource == TaskSubmissionResolutionSourceProviderVerified {
					return normalized.ResolutionSource
				}
				return ""
			}(),
			EvidenceID:        normalized.EvidenceID,
			EvidenceHash:      normalized.EvidenceHash,
			EvidenceVersion:   normalized.EvidenceVersion,
			StatisticsVersion: reserveReceipt.StatisticsVersion,
			StatisticsApplied: reserveReceipt.StatisticsApplied,
		})
		if err != nil {
			return err
		}
		if event.EventKey != eventKey {
			return fmt.Errorf("%w: refund event key mismatch", ErrTaskQuotaReservationConflict)
		}

		applied, err := applySynchronousTaskQuotaAppliedEvent(writeDB, event)
		if err != nil {
			return err
		}
		if !applied {
			return ErrTaskQuotaReservationCASLost
		}
		if err := writeDB.Where("id = ?", event.ID).First(event).Error; err != nil {
			return err
		}

		targetVersionAfter := operation.LockVersion
		if normalized.TargetOperationStatus != "" {
			targetVersionAfter = operation.LockVersion + 1
		}

		receipt := &QuotaMutationReceipt{
			ReceiptVersion:              quotaMutationReceiptVersion,
			MutationType:                string(TaskBillingEventTypeRefund),
			MutationKey:                 event.EventKey,
			RequestFingerprint:          fingerprint,
			OperationRequestFingerprint: operation.RequestFingerprint,
			OperationID:                 operation.ID,
			OperationPublicID:           operation.PublicID,
			ExpectedOperationVersion:    normalized.ExpectedOperationVersion,
			OperationVersionBefore:      operation.LockVersion,
			OperationVersionAfter:       targetVersionAfter,
			BillingEventID:              event.EventID,
			BillingEventKey:             event.EventKey,
			BillingEventVersion:         event.LockVersion,
			UserID:                      normalized.UserID,
			TokenID:                     normalized.TokenID,
			ChannelID:                   normalized.ChannelID,
			BillingSource:               reserveReceipt.BillingSource,
			SubscriptionID:              reserveReceipt.SubscriptionID,
			Quota:                       refundQuota,
			RequestID:                   event.RequestID,
			StatisticsVersion:           reserveReceipt.StatisticsVersion,
			StatisticsApplied:           reserveReceipt.StatisticsApplied,
			EvidenceID:                  normalized.EvidenceID,
			EvidenceHash:                normalized.EvidenceHash,
			EvidenceVersion:             normalized.EvidenceVersion,
			BillingContext:              TaskQuotaBillingContext(normalized.BillingContext),
			Before:                      before,
			After:                       after,
		}
		if err := quotaMutationReceiptCreateDB(writeDB).Create(receipt).Error; err != nil {
			return err
		}
		if _, err := createTaskMutationOutbox(writeDB, event, receipt, "task quota released"); err != nil {
			return err
		}

		if normalized.TargetOperationStatus != "" {
			won, err := TransitionTaskSubmissionOperation(writeDB, operation.ID, TaskSubmissionOperationTransition{
				From:             operation.Status,
				To:               normalized.TargetOperationStatus,
				ExpectedVersion:  operation.LockVersion,
				ReasonCode:       normalized.ReasonCode,
				ResolutionSource: normalized.ResolutionSource,
			})
			if err != nil {
				return err
			}
			if !won {
				return ErrTaskQuotaReservationCASLost
			}
		}

		result = receipt
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// SettleTaskQuotaReservation applies gate-off T3 atomically. It calculates the delta
// between reserved quota and actual quota, adjusting balances (surplus refund, deficit charge, or exact match).
func SettleTaskQuotaReservation(tx *gorm.DB, input TaskQuotaSettlementInput) (*QuotaMutationReceipt, error) {
	if tx == nil {
		return nil, gorm.ErrInvalidDB
	}
	normalized, fingerprint, err := normalizeTaskQuotaSettlementInput(input)
	if err != nil {
		return nil, err
	}

	// Must have a valid T1 reservation receipt
	reserveReceipt, err := findTaskQuotaReservation(tx, normalized.OperationID, normalized.UserID, normalized.TokenID)
	if err != nil {
		return nil, err
	}
	if reserveReceipt == nil {
		return nil, ErrTaskQuotaReservationNotFound
	}
	if (input.RequireStatisticsEvidence || reserveReceipt.RequestFingerprintVersion >= 2) && (!reserveReceipt.StatisticsApplied || reserveReceipt.StatisticsVersion != 1) {
		return nil, ErrTaskTerminalObservationManualReview
	}

	var result *QuotaMutationReceipt
	err = taskRecoveryAtomicTransaction(tx, "task_quota_settle", func(writeDB *gorm.DB) error {
		databaseNow, err := taskRecoveryDBTimestamp(writeDB)
		if err != nil {
			return err
		}

		// 1. Lock User
		var user User
		if err := lockForUpdate(writeDB).Where("id = ?", normalized.UserID).First(&user).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("%w: user does not exist", ErrTaskQuotaReservationIneligible)
			}
			return err
		}
		if !quotaMutationVersionValid(user.QuotaVersion) {
			return fmt.Errorf("%w: user has an invalid quota version", ErrTaskQuotaReservationIneligible)
		}

		// 2. Lock Token
		var token Token
		if err := lockForUpdate(writeDB).Where("id = ?", normalized.TokenID).First(&token).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("%w: token does not exist", ErrTaskQuotaReservationIneligible)
			}
			return err
		}
		if token.UserId != normalized.UserID || !quotaMutationVersionValid(token.QuotaVersion) {
			return fmt.Errorf("%w: token owner or quota version is invalid", ErrTaskQuotaReservationIneligible)
		}

		// 3. Lock Subscription if applicable
		var subscription *UserSubscription
		if reserveReceipt.BillingSource == "subscription" {
			var locked UserSubscription
			if err := lockForUpdate(writeDB).Where("id = ?", reserveReceipt.SubscriptionID).First(&locked).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return fmt.Errorf("%w: subscription does not exist", ErrTaskQuotaReservationIneligible)
				}
				return err
			}
			if locked.UserId != normalized.UserID || !quotaMutationVersionValid(locked.QuotaVersion) {
				return fmt.Errorf("%w: subscription owner or quota version is invalid", ErrTaskQuotaReservationIneligible)
			}
			subscription = &locked
		}

		// 4. Lock Operation
		var operation TaskSubmissionOperation
		if err := lockForUpdate(writeDB).Where("id = ?", normalized.OperationID).First(&operation).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("%w: operation does not exist", ErrTaskQuotaReservationIneligible)
			}
			return err
		}
		if err := validateStoredTaskSubmissionOperation(writeDB, &operation); err != nil {
			return err
		}

		// 5. Lock Attempt
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
		if !taskMutationEvidenceMatchesAttempt(normalized.ResolutionSource, normalized.EvidenceID, normalized.EvidenceHash, normalized.EvidenceVersion, &attempt) {
			return ErrTaskQuotaSettlementInvalidInput
		}
		if existing, err := findTaskQuotaReceiptByType(writeDB, normalized.OperationID, string(TaskBillingEventTypeTerminalSettlement), normalized.UserID, normalized.TokenID); err != nil {
			return err
		} else if existing != nil {
			if existing.RequestFingerprint != fingerprint {
				return ErrTaskQuotaReservationConflict
			}
			result = existing
			return nil
		}
		if refunded, err := findTaskQuotaReceiptByType(writeDB, normalized.OperationID, string(TaskBillingEventTypeRefund), normalized.UserID, normalized.TokenID); err != nil {
			return err
		} else if refunded != nil {
			return ErrTaskQuotaAlreadyRefunded
		}
		if operation.UserID != normalized.UserID || operation.TokenID != normalized.TokenID || operation.LockVersion != normalized.ExpectedOperationVersion || operation.LockVersion == quotaMutationMaxInt64 {
			return ErrTaskQuotaReservationCASLost
		}

		// Calculate settlement delta: expectedDelta = reservedQuota - actualQuota
		reservedQuota := reserveReceipt.Quota
		actualQuota := normalized.ActualQuota
		settlementDelta := reservedQuota - actualQuota

		before := quotaMutationSnapshot(&user, &token, subscription)
		after, err := quotaMutationSettlementAfter(before, reserveReceipt.BillingSource, settlementDelta, actualQuota, databaseNow)
		if err != nil {
			return err
		}

		if err := applyTaskQuotaSettlementBalances(writeDB, before, after, reserveReceipt.BillingSource, settlementDelta); err != nil {
			return err
		}
		if reserveReceipt.StatisticsApplied {
			if err := applyTaskStatistics(writeDB, normalized.UserID, normalized.ChannelID, actualQuota-reservedQuota, 0); err != nil {
				return err
			}
		}

		operationID := operation.ID
		eventKey := "task:" + operation.PublicID + ":" + string(TaskBillingEventTypeTerminalSettlement) + ":v1"
		event, err := CreateOrLoadTaskBillingEvent(writeDB, &TaskBillingEvent{
			OperationID:    &operationID,
			EventType:      TaskBillingEventTypeTerminalSettlement,
			UserID:         normalized.UserID,
			TokenID:        normalized.TokenID,
			ChannelID:      normalized.ChannelID,
			BillingSource:  reserveReceipt.BillingSource,
			SubscriptionID: reserveReceipt.SubscriptionID,
			QuotaDelta:     settlementDelta,
			ReasonCode:     normalized.ReasonCode,
			RequestID:      stableTaskBillingRequestID(taskMutationRequestID(normalized.RequestID, reserveReceipt.RequestID), eventKey),
			ResolutionSource: func() string {
				if normalized.ResolutionSource == TaskSubmissionResolutionSourceProviderVerified {
					return normalized.ResolutionSource
				}
				return ""
			}(),
			EvidenceID:        normalized.EvidenceID,
			EvidenceHash:      normalized.EvidenceHash,
			EvidenceVersion:   normalized.EvidenceVersion,
			StatisticsVersion: reserveReceipt.StatisticsVersion,
			StatisticsApplied: reserveReceipt.StatisticsApplied,
		})
		if err != nil {
			return err
		}
		if event.EventKey != eventKey {
			return fmt.Errorf("%w: settlement event key mismatch", ErrTaskQuotaReservationConflict)
		}

		applied, err := applySynchronousTaskQuotaAppliedEvent(writeDB, event)
		if err != nil {
			return err
		}
		if !applied {
			return ErrTaskQuotaReservationCASLost
		}
		if err := writeDB.Where("id = ?", event.ID).First(event).Error; err != nil {
			return err
		}

		targetVersionAfter := operation.LockVersion
		if normalized.TargetOperationStatus != "" {
			targetVersionAfter = operation.LockVersion + 1
		}

		receipt := &QuotaMutationReceipt{
			ReceiptVersion:              quotaMutationReceiptVersion,
			MutationType:                string(TaskBillingEventTypeTerminalSettlement),
			MutationKey:                 event.EventKey,
			RequestFingerprint:          fingerprint,
			OperationRequestFingerprint: operation.RequestFingerprint,
			OperationID:                 operation.ID,
			OperationPublicID:           operation.PublicID,
			ExpectedOperationVersion:    normalized.ExpectedOperationVersion,
			OperationVersionBefore:      operation.LockVersion,
			OperationVersionAfter:       targetVersionAfter,
			BillingEventID:              event.EventID,
			BillingEventKey:             event.EventKey,
			BillingEventVersion:         event.LockVersion,
			UserID:                      normalized.UserID,
			TokenID:                     normalized.TokenID,
			ChannelID:                   normalized.ChannelID,
			BillingSource:               reserveReceipt.BillingSource,
			SubscriptionID:              reserveReceipt.SubscriptionID,
			Quota:                       actualQuota,
			RequestID:                   event.RequestID,
			StatisticsVersion:           reserveReceipt.StatisticsVersion,
			StatisticsApplied:           reserveReceipt.StatisticsApplied,
			EvidenceID:                  normalized.EvidenceID,
			EvidenceHash:                normalized.EvidenceHash,
			EvidenceVersion:             normalized.EvidenceVersion,
			BillingContext:              TaskQuotaBillingContext(normalized.BillingContext),
			Before:                      before,
			After:                       after,
		}
		if err := quotaMutationReceiptCreateDB(writeDB).Create(receipt).Error; err != nil {
			return err
		}
		if _, err := createTaskMutationOutbox(writeDB, event, receipt, "task quota settled"); err != nil {
			return err
		}

		if normalized.TargetOperationStatus != "" {
			won, err := TransitionTaskSubmissionOperation(writeDB, operation.ID, TaskSubmissionOperationTransition{
				From:             operation.Status,
				To:               normalized.TargetOperationStatus,
				ExpectedVersion:  operation.LockVersion,
				ReasonCode:       normalized.ReasonCode,
				ResolutionSource: normalized.ResolutionSource,
			})
			if err != nil {
				return err
			}
			if !won {
				return ErrTaskQuotaReservationCASLost
			}
		}

		result = receipt
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func applySynchronousTaskQuotaAppliedEvent(tx *gorm.DB, event *TaskBillingEvent) (bool, error) {
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

func quotaMutationRefundAfter(before QuotaMutationAccountSnapshot, billingSource string, refundQuota int64, databaseNow int64) (QuotaMutationAccountSnapshot, error) {
	after := before
	if before.Subscription != nil {
		copySubscription := *before.Subscription
		after.Subscription = &copySubscription
	}
	if refundQuota == 0 {
		return after, nil
	}

	if billingSource == "wallet" {
		newUserQuota := int64(before.User.Quota) + refundQuota
		if newUserQuota > int64(common.MaxQuota) {
			return QuotaMutationAccountSnapshot{}, ErrTaskTerminalObservationManualReview
		}
		after.User.Quota = int(newUserQuota)
		after.User.QuotaVersion++
	} else {
		if after.Subscription == nil {
			return QuotaMutationAccountSnapshot{}, ErrTaskQuotaReservationInvalidInput
		}
		newUsed := after.Subscription.AmountUsed - refundQuota
		if newUsed < 0 {
			return QuotaMutationAccountSnapshot{}, ErrTaskTerminalObservationManualReview
		}
		after.Subscription.AmountUsed = newUsed
		after.Subscription.QuotaVersion++
		after.Subscription.UpdatedAt = databaseNow
	}

	newRemain := int64(before.Token.RemainQuota) + refundQuota
	if newRemain > int64(common.MaxQuota) {
		return QuotaMutationAccountSnapshot{}, ErrTaskTerminalObservationManualReview
	}
	newUsed := int64(before.Token.UsedQuota) - refundQuota
	if newUsed < 0 {
		return QuotaMutationAccountSnapshot{}, ErrTaskTerminalObservationManualReview
	}
	after.Token.RemainQuota = int(newRemain)
	after.Token.UsedQuota = int(newUsed)
	after.Token.AccessedTime = databaseNow
	after.Token.QuotaVersion++
	return after, nil
}

func applyTaskQuotaRefundBalances(tx *gorm.DB, before, after QuotaMutationAccountSnapshot, billingSource string, refundQuota int64) error {
	if refundQuota == 0 {
		return nil
	}
	if billingSource == "wallet" {
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
	} else {
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
	return nil
}

func quotaMutationSettlementAfter(before QuotaMutationAccountSnapshot, billingSource string, delta int64, actualQuota int64, databaseNow int64) (QuotaMutationAccountSnapshot, error) {
	after := before
	if before.Subscription != nil {
		copySubscription := *before.Subscription
		after.Subscription = &copySubscription
	}
	if delta == 0 {
		return after, nil
	}

	if billingSource == "wallet" {
		newUserQuota := int64(before.User.Quota) + delta
		if newUserQuota > int64(common.MaxQuota) {
			return QuotaMutationAccountSnapshot{}, ErrTaskTerminalObservationManualReview
		} else if newUserQuota < int64(common.MinQuota) {
			return QuotaMutationAccountSnapshot{}, ErrTaskTerminalObservationManualReview
		}
		after.User.Quota = int(newUserQuota)
		after.User.QuotaVersion++
	} else {
		if after.Subscription == nil {
			return QuotaMutationAccountSnapshot{}, ErrTaskQuotaReservationInvalidInput
		}
		newUsed := after.Subscription.AmountUsed - delta
		if newUsed < 0 {
			return QuotaMutationAccountSnapshot{}, ErrTaskTerminalObservationManualReview
		}
		after.Subscription.AmountUsed = newUsed
		after.Subscription.QuotaVersion++
		after.Subscription.UpdatedAt = databaseNow
	}

	newRemain := int64(before.Token.RemainQuota) + delta
	if newRemain > int64(common.MaxQuota) {
		return QuotaMutationAccountSnapshot{}, ErrTaskTerminalObservationManualReview
	} else if newRemain < int64(common.MinQuota) {
		return QuotaMutationAccountSnapshot{}, ErrTaskTerminalObservationManualReview
	}
	newUsed := int64(before.Token.UsedQuota) - delta
	if newUsed < 0 {
		return QuotaMutationAccountSnapshot{}, ErrTaskTerminalObservationManualReview
	} else if newUsed > int64(common.MaxQuota) {
		return QuotaMutationAccountSnapshot{}, ErrTaskTerminalObservationManualReview
	}
	after.Token.RemainQuota = int(newRemain)
	after.Token.UsedQuota = int(newUsed)
	after.Token.AccessedTime = databaseNow
	after.Token.QuotaVersion++
	return after, nil
}

func applyTaskQuotaSettlementBalances(tx *gorm.DB, before, after QuotaMutationAccountSnapshot, billingSource string, delta int64) error {
	if delta == 0 {
		return nil
	}
	if billingSource == "wallet" {
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
	} else {
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
	return nil
}

func normalizeTaskQuotaReleaseInput(input TaskQuotaReleaseInput) (TaskQuotaReleaseInput, string, error) {
	input.ReasonCode = strings.ToLower(strings.TrimSpace(input.ReasonCode))
	input.ResolutionSource = strings.TrimSpace(input.ResolutionSource)
	input.EvidenceID = strings.TrimSpace(input.EvidenceID)
	input.EvidenceHash = strings.ToLower(strings.TrimSpace(input.EvidenceHash))
	input.RequestID = strings.TrimSpace(input.RequestID)
	if input.ReasonCode == "" {
		input.ReasonCode = "task_t4_refund"
	}
	if !validTaskTerminalReasonCode(input.ReasonCode) {
		return input, "", ErrTaskQuotaReleaseInvalidInput
	}
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
		int64(input.UserID) > int64(common.MaxQuota) || int64(input.TokenID) > int64(common.MaxQuota) ||
		int64(input.ChannelID) > int64(common.MaxQuota) || len(input.ReasonCode) > 64 || len(input.ResolutionSource) > 32 || len(input.RequestID) > 64 {
		return input, "", ErrTaskQuotaReleaseInvalidInput
	}
	if input.TargetOperationStatus != "" && !input.TargetOperationStatus.Valid() {
		return input, "", ErrTaskQuotaReleaseInvalidInput
	}
	if !validTaskMutationEvidence(input.EvidenceID, input.EvidenceHash, input.EvidenceVersion) {
		return input, "", ErrTaskQuotaReleaseInvalidInput
	}
	if err := validateTaskQuotaBillingContext(input.BillingContext); err != nil {
		return input, "", err
	}
	payload := struct {
		Version                  int                           `json:"version"`
		OperationID              int64                         `json:"operation_id"`
		UserID                   int                           `json:"user_id"`
		TokenID                  int                           `json:"token_id"`
		ChannelID                int                           `json:"channel_id"`
		ExpectedOperationVersion int64                         `json:"expected_operation_version"`
		ReasonCode               string                        `json:"reason_code"`
		TargetOperationStatus    TaskSubmissionOperationStatus `json:"target_operation_status,omitempty"`
		ResolutionSource         string                        `json:"resolution_source,omitempty"`
		BillingContext           TaskBillingContext            `json:"billing_context"`
		EvidenceID               string                        `json:"evidence_id,omitempty"`
		EvidenceHash             string                        `json:"evidence_hash,omitempty"`
		EvidenceVersion          int                           `json:"evidence_version,omitempty"`
		RequestID                string                        `json:"request_id,omitempty"`
	}{
		Version: quotaMutationFingerprintVersion, OperationID: input.OperationID,
		UserID: input.UserID, TokenID: input.TokenID, ChannelID: input.ChannelID,
		ExpectedOperationVersion: input.ExpectedOperationVersion, ReasonCode: input.ReasonCode,
		TargetOperationStatus: input.TargetOperationStatus, BillingContext: input.BillingContext,
		ResolutionSource: input.ResolutionSource,
		EvidenceID:       input.EvidenceID, EvidenceHash: input.EvidenceHash, EvidenceVersion: input.EvidenceVersion,
		RequestID: input.RequestID,
	}
	data, err := common.Marshal(payload)
	if err != nil {
		return input, "", fmt.Errorf("%w: encode release fingerprint: %v", ErrTaskQuotaReleaseInvalidInput, err)
	}
	digest := sha256.Sum256(data)
	return input, hex.EncodeToString(digest[:]), nil
}

func normalizeTaskQuotaSettlementInput(input TaskQuotaSettlementInput) (TaskQuotaSettlementInput, string, error) {
	input.ReasonCode = strings.ToLower(strings.TrimSpace(input.ReasonCode))
	input.ResolutionSource = strings.TrimSpace(input.ResolutionSource)
	input.EvidenceID = strings.TrimSpace(input.EvidenceID)
	input.EvidenceHash = strings.ToLower(strings.TrimSpace(input.EvidenceHash))
	input.RequestID = strings.TrimSpace(input.RequestID)
	if input.ReasonCode == "" {
		input.ReasonCode = "task_t3_settle"
	}
	if !validTaskTerminalReasonCode(input.ReasonCode) {
		return input, "", ErrTaskQuotaSettlementInvalidInput
	}
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
		input.ActualQuota < 0 || input.ActualQuota > int64(common.MaxQuota) ||
		int64(input.UserID) > int64(common.MaxQuota) || int64(input.TokenID) > int64(common.MaxQuota) ||
		int64(input.ChannelID) > int64(common.MaxQuota) || len(input.ReasonCode) > 64 || len(input.ResolutionSource) > 32 || len(input.RequestID) > 64 {
		return input, "", ErrTaskQuotaSettlementInvalidInput
	}
	if input.TargetOperationStatus != "" && !input.TargetOperationStatus.Valid() {
		return input, "", ErrTaskQuotaSettlementInvalidInput
	}
	if !validTaskMutationEvidence(input.EvidenceID, input.EvidenceHash, input.EvidenceVersion) {
		return input, "", ErrTaskQuotaSettlementInvalidInput
	}
	if err := validateTaskQuotaBillingContext(input.BillingContext); err != nil {
		return input, "", err
	}
	payload := struct {
		Version                  int                           `json:"version"`
		OperationID              int64                         `json:"operation_id"`
		UserID                   int                           `json:"user_id"`
		TokenID                  int                           `json:"token_id"`
		ChannelID                int                           `json:"channel_id"`
		ExpectedOperationVersion int64                         `json:"expected_operation_version"`
		ActualQuota              int64                         `json:"actual_quota"`
		ReasonCode               string                        `json:"reason_code"`
		TargetOperationStatus    TaskSubmissionOperationStatus `json:"target_operation_status,omitempty"`
		ResolutionSource         string                        `json:"resolution_source,omitempty"`
		BillingContext           TaskBillingContext            `json:"billing_context"`
		EvidenceID               string                        `json:"evidence_id,omitempty"`
		EvidenceHash             string                        `json:"evidence_hash,omitempty"`
		EvidenceVersion          int                           `json:"evidence_version,omitempty"`
		RequestID                string                        `json:"request_id,omitempty"`
	}{
		Version: quotaMutationFingerprintVersion, OperationID: input.OperationID,
		UserID: input.UserID, TokenID: input.TokenID, ChannelID: input.ChannelID,
		ExpectedOperationVersion: input.ExpectedOperationVersion, ActualQuota: input.ActualQuota,
		ReasonCode: input.ReasonCode, TargetOperationStatus: input.TargetOperationStatus, ResolutionSource: input.ResolutionSource,
		BillingContext: input.BillingContext,
		EvidenceID:     input.EvidenceID, EvidenceHash: input.EvidenceHash, EvidenceVersion: input.EvidenceVersion,
		RequestID: input.RequestID,
	}
	data, err := common.Marshal(payload)
	if err != nil {
		return input, "", fmt.Errorf("%w: encode settlement fingerprint: %v", ErrTaskQuotaSettlementInvalidInput, err)
	}
	digest := sha256.Sum256(data)
	return input, hex.EncodeToString(digest[:]), nil
}

func validTaskMutationEvidence(id, hash string, version int) bool {
	if len(id) > 191 || version < 0 || (hash != "" && !validTaskRecoveryDigest(hash)) {
		return false
	}
	if version == 0 {
		return id == "" && hash == ""
	}
	return id != "" || hash != ""
}

func taskMutationRequestID(primary, fallback string) string {
	if strings.TrimSpace(primary) != "" {
		return strings.TrimSpace(primary)
	}
	return strings.TrimSpace(fallback)
}

func taskMutationEvidenceMatchesAttempt(resolutionSource, id, hash string, version int, attempt *TaskSubmissionAttempt) bool {
	if resolutionSource != TaskSubmissionResolutionSourceProviderVerified {
		return true
	}
	if attempt == nil || version != 1 || id == "" {
		return false
	}
	return id == attempt.ProviderOperationID || id == attempt.UpstreamRequestID
}
