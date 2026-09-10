package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/logger"
	"github.com/ForceMind/MyAPI/model"
	"gorm.io/gorm"
)

// DurableSettleTaskOnComplete bridges async task completion to the durable accounting
// settlement system (T3). If the task is associated with a durable TaskSubmissionOperation,
// it executes SettleTaskQuotaReservation atomically and transitions the operation to succeeded.
//
// Returns (handled=true, nil) if processed by durable accounting.
// Returns (handled=false, nil) if the task is not backed by a durable operation (caller should use legacy settlement).
func DurableSettleTaskOnComplete(ctx context.Context, task *model.Task, actualQuota int, reason string) (bool, error) {
	if model.DB == nil || task == nil || task.TaskID == "" {
		return false, nil
	}

	op, err := model.GetTaskSubmissionOperationByPublicID(model.DB, task.TaskID)
	if err != nil {
		return false, fmt.Errorf("query task submission operation failed: %w", err)
	}
	if op == nil {
		return false, nil
	}

	// Idempotency: if operation is already in terminal state
	if op.Status == model.TaskSubmissionOperationStatusSucceeded {
		logger.LogInfo(ctx, fmt.Sprintf("task %s operation %s already succeeded, skip settlement", task.TaskID, op.PublicID))
		return true, nil
	}
	if op.Status == model.TaskSubmissionOperationStatusFailed ||
		op.Status == model.TaskSubmissionOperationStatusRejected ||
		op.Status == model.TaskSubmissionOperationStatusCanceled {
		logger.LogWarn(ctx, fmt.Sprintf("task %s operation %s already terminated (%s), skip settlement", task.TaskID, op.PublicID, op.Status))
		return true, nil
	}

	if op.Status != model.TaskSubmissionOperationStatusAccepted &&
		op.Status != model.TaskSubmissionOperationStatusOutcomeUnknown {
		return true, fmt.Errorf("cannot settle task %s: operation status %s is not accepted or outcome_unknown", task.TaskID, op.Status)
	}

	channelID := task.ChannelId
	if channelID <= 0 {
		attempt, _ := model.FindAttemptByOperationID(model.DB, op.ID)
		if attempt != nil && attempt.ChannelID > 0 {
			channelID = attempt.ChannelID
		}
	}
	if channelID <= 0 {
		channelID = 1
	}

	tokenID := op.TokenID
	if tokenID <= 0 {
		tokenID = task.PrivateData.TokenId
	}

	bc := resolveDurableTaskBillingContext(task)
	reasonCode := sanitizeReasonCode(reason, "task_poll_settle")

	input := model.TaskQuotaSettlementInput{
		OperationID:              op.ID,
		UserID:                   op.UserID,
		TokenID:                  tokenID,
		ChannelID:                channelID,
		ExpectedOperationVersion: op.LockVersion,
		ActualQuota:              int64(actualQuota),
		ReasonCode:               reasonCode,
		BillingContext:           bc,
		TargetOperationStatus:    model.TaskSubmissionOperationStatusSucceeded,
	}

	receipt, err := model.SettleTaskQuotaReservation(model.DB, input)
	if err != nil {
		if errors.Is(err, model.ErrTaskQuotaAlreadySettled) {
			logger.LogInfo(ctx, fmt.Sprintf("task %s operation %s already settled", task.TaskID, op.PublicID))
			return true, nil
		}
		return true, fmt.Errorf("settle task quota reservation failed for %s: %w", task.TaskID, err)
	}

	quotaDelta := actualQuota - task.Quota
	task.Quota = actualQuota
	if err := task.UpdateQuota(); err != nil {
		logger.LogError(ctx, fmt.Sprintf("durable settlement updated quota but failed to persist task quota: %v", err))
	}

	model.UpdateUserUsedQuota(task.UserId, quotaDelta)
	model.UpdateChannelUsedQuota(task.ChannelId, quotaDelta)

	createOutboxForBillingEvent(model.DB, receipt.BillingEventID, task, reason)

	logger.LogInfo(ctx, fmt.Sprintf("task %s durable settlement complete: actual=%d, delta=%d, receipt=%s",
		task.TaskID, actualQuota, quotaDelta, receipt.MutationKey))
	return true, nil
}

// DurableReleaseTaskOnFailure bridges async task failure to the durable accounting
// release system (T4). If the task is associated with a durable TaskSubmissionOperation,
// it executes ReleaseTaskQuotaReservation atomically and transitions the operation to failed.
//
// Returns (handled=true, nil) if processed by durable accounting.
// Returns (handled=false, nil) if the task is not backed by a durable operation (caller should use legacy refund).
func DurableReleaseTaskOnFailure(ctx context.Context, task *model.Task, reason string) (bool, error) {
	if model.DB == nil || task == nil || task.TaskID == "" {
		return false, nil
	}

	op, err := model.GetTaskSubmissionOperationByPublicID(model.DB, task.TaskID)
	if err != nil {
		return false, fmt.Errorf("query task submission operation failed: %w", err)
	}
	if op == nil {
		return false, nil
	}

	// Idempotency: if operation is already in terminal state
	if op.Status == model.TaskSubmissionOperationStatusFailed ||
		op.Status == model.TaskSubmissionOperationStatusRejected ||
		op.Status == model.TaskSubmissionOperationStatusCanceled {
		logger.LogInfo(ctx, fmt.Sprintf("task %s operation %s already terminated (%s), skip release", task.TaskID, op.PublicID, op.Status))
		return true, nil
	}
	if op.Status == model.TaskSubmissionOperationStatusSucceeded {
		logger.LogWarn(ctx, fmt.Sprintf("task %s operation %s already succeeded, cannot release", task.TaskID, op.PublicID))
		return true, model.ErrTaskQuotaAlreadySettled
	}

	if op.Status != model.TaskSubmissionOperationStatusAccepted &&
		op.Status != model.TaskSubmissionOperationStatusOutcomeUnknown {
		return true, fmt.Errorf("cannot release task %s: operation status %s is not accepted or outcome_unknown", task.TaskID, op.Status)
	}

	channelID := task.ChannelId
	if channelID <= 0 {
		attempt, _ := model.FindAttemptByOperationID(model.DB, op.ID)
		if attempt != nil && attempt.ChannelID > 0 {
			channelID = attempt.ChannelID
		}
	}
	if channelID <= 0 {
		channelID = 1
	}

	tokenID := op.TokenID
	if tokenID <= 0 {
		tokenID = task.PrivateData.TokenId
	}

	bc := resolveDurableTaskBillingContext(task)
	reasonCode := sanitizeReasonCode(reason, "task_poll_failed")

	input := model.TaskQuotaReleaseInput{
		OperationID:              op.ID,
		UserID:                   op.UserID,
		TokenID:                  tokenID,
		ChannelID:                channelID,
		ExpectedOperationVersion: op.LockVersion,
		ReasonCode:               reasonCode,
		BillingContext:           bc,
		TargetOperationStatus:    model.TaskSubmissionOperationStatusFailed,
	}

	receipt, err := model.ReleaseTaskQuotaReservation(model.DB, input)
	if err != nil {
		if errors.Is(err, model.ErrTaskQuotaAlreadyRefunded) {
			logger.LogInfo(ctx, fmt.Sprintf("task %s operation %s already refunded", task.TaskID, op.PublicID))
			return true, nil
		}
		return true, fmt.Errorf("release task quota reservation failed for %s: %w", task.TaskID, err)
	}

	model.UpdateUserUsedQuota(task.UserId, -task.Quota)
	model.UpdateChannelUsedQuota(task.ChannelId, -task.Quota)

	task.Quota = 0
	if err := task.UpdateQuota(); err != nil {
		logger.LogError(ctx, fmt.Sprintf("durable release refunded quota but failed to persist task quota: %v", err))
	}

	createOutboxForBillingEvent(model.DB, receipt.BillingEventID, task, reason)

	logger.LogInfo(ctx, fmt.Sprintf("task %s durable release complete: receipt=%s", task.TaskID, receipt.MutationKey))
	return true, nil
}

func createOutboxForBillingEvent(db *gorm.DB, eventID string, task *model.Task, reason string) {
	if db == nil || eventID == "" {
		return
	}
	var event model.TaskBillingEvent
	if err := db.Where("event_id = ?", eventID).First(&event).Error; err != nil {
		return
	}
	modelName := taskModelName(task)
	if modelName == "" {
		modelName = "default"
	}
	group := task.Group
	if group == "" {
		group = "default"
	}
	content := reason
	if content == "" {
		content = fmt.Sprintf("task billing event %s", event.EventType)
	}

	outboxCandidate, err := model.NewTaskBillingLogOutbox(&event, model.TaskBillingLogPayload{
		Content:   content,
		ModelName: modelName,
		Group:     group,
	})
	if err != nil {
		return
	}
	_, _ = model.CreateOrLoadTaskBillingLogOutbox(db, outboxCandidate)
}

func computeTaskQuotaFromTokens(task *model.Task, totalTokens int) (int, bool) {
	if totalTokens <= 0 {
		return 0, false
	}
	rates, _, err := resolveTaskTokenBillingRates(task)
	if err != nil {
		return task.Quota, true
	}
	otherMultiplier := 1.0
	if priceData := taskBillingContextPriceData(rates); priceData != nil {
		otherMultiplier = priceData.OtherRatioMultiplier()
	}
	if rates.ModelRatio == 0 || rates.GroupRatio == 0 {
		return 0, true
	}
	actualQuota, _ := common.QuotaFromFloatChecked(float64(totalTokens) * rates.ModelRatio * rates.GroupRatio * otherMultiplier)
	return actualQuota, true
}

func resolveDurableTaskBillingContext(task *model.Task) model.TaskBillingContext {
	if task.PrivateData.BillingContext != nil {
		bc := *task.PrivateData.BillingContext
		if bc.Version == 0 {
			bc.Version = model.TaskBillingContextVersion
		}
		bc.Complete = true
		if bc.OriginModelName == "" {
			bc.OriginModelName = taskModelName(task)
		}
		if bc.OriginModelName == "" {
			bc.OriginModelName = "default"
		}
		return bc
	}

	modelName := taskModelName(task)
	if modelName == "" {
		modelName = "default"
	}
	return model.TaskBillingContext{
		Version:         model.TaskBillingContextVersion,
		Complete:        true,
		OriginModelName: modelName,
		ModelPrice:      0,
		ModelRatio:      1.0,
		GroupRatio:      1.0,
	}
}

func sanitizeReasonCode(reason string, defaultCode string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return defaultCode
	}
	// Limit to 64 bytes safely
	if len(reason) > 64 {
		runes := []rune(reason)
		for len(string(runes)) > 64 && len(runes) > 0 {
			runes = runes[:len(runes)-1]
		}
		reason = string(runes)
	}
	return reason
}
