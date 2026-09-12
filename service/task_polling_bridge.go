package service

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/logger"
	"github.com/ForceMind/MyAPI/model"
)

// DurableSettleTaskOnComplete bridges async task completion to the durable accounting
// settlement system (T3). If the task is associated with a durable TaskSubmissionOperation,
// it executes SettleTaskQuotaReservation atomically and transitions the operation to succeeded.
//
// Returns (handled=true, nil) if processed by durable accounting.
// Returns (handled=false, nil) if the task is not backed by a durable operation (caller should use legacy settlement).
type DurableTerminalEvidence struct {
	ID      string
	Hash    string
	Version int
}

func DurableSettleTaskOnComplete(ctx context.Context, task *model.Task, actualQuota int, reason string, clamps ...*common.QuotaClamp) (bool, error) {
	pollingContext, err := LoadTaskPollingContext(model.DB, task)
	if err != nil {
		return false, err
	}
	return durableTerminalWithoutEvidenceUsingContext(ctx, pollingContext, task, "succeeded", actualQuota, reason, clamps...)
}

func DurableReleaseTaskOnFailure(ctx context.Context, task *model.Task, reason string) (bool, error) {
	pollingContext, err := LoadTaskPollingContext(model.DB, task)
	if err != nil {
		return false, err
	}
	return durableTerminalWithoutEvidenceUsingContext(ctx, pollingContext, task, "failed", 0, reason)
}

func durableTerminalWithoutEvidenceUsingContext(ctx context.Context, pollingContext *TaskPollingContext, task *model.Task, outcome string, actualQuota int, reason string, clamps ...*common.QuotaClamp) (bool, error) {
	return durableTerminalManualReviewUsingContext(ctx, pollingContext, task, outcome, actualQuota, reason, DurableTerminalEvidence{}, clamps...)
}

func DurableTerminalManualReviewWithEvidence(ctx context.Context, task *model.Task, outcome string, actualQuota int, reason string, evidence DurableTerminalEvidence, clamps ...*common.QuotaClamp) (bool, error) {
	pollingContext, err := LoadTaskPollingContext(model.DB, task)
	if err != nil {
		return false, err
	}
	return durableTerminalManualReviewUsingContext(ctx, pollingContext, task, outcome, actualQuota, reason, evidence, clamps...)
}

func durableTerminalManualReviewUsingContext(ctx context.Context, pollingContext *TaskPollingContext, task *model.Task, outcome string, actualQuota int, reason string, evidence DurableTerminalEvidence, clamps ...*common.QuotaClamp) (bool, error) {
	if model.DB == nil || task == nil || task.TaskID == "" || pollingContext == nil || pollingContext.Operation == nil {
		return false, nil
	}
	op := pollingContext.Operation
	reasonCode := sanitizeReasonCode(reason, "terminal_evidence_missing")
	var clamp *common.QuotaClamp
	for _, candidate := range clamps {
		if candidate != nil {
			clamp = candidate
			break
		}
	}
	_, err := model.CreateOrLoadTaskTerminalObservation(model.DB, terminalObservationInput(op, task, outcome, actualQuota, reasonCode, evidence, true, clamp))
	if err != nil {
		return true, err
	}
	logger.LogWarn(ctx, "durable terminal result requires manual review")
	return true, model.ErrTaskTerminalObservationManualReview
}

func DurableSettleTaskOnCompleteWithEvidence(ctx context.Context, task *model.Task, actualQuota int, reason string, evidence DurableTerminalEvidence, clamps ...*common.QuotaClamp) (bool, error) {
	pollingContext, err := LoadTaskPollingContext(model.DB, task)
	if err != nil {
		return false, err
	}
	return durableSettleTaskOnCompleteUsingContext(ctx, pollingContext, task, actualQuota, reason, evidence, clamps...)
}

func durableSettleTaskOnCompleteUsingContext(ctx context.Context, pollingContext *TaskPollingContext, task *model.Task, actualQuota int, reason string, evidence DurableTerminalEvidence, clamps ...*common.QuotaClamp) (bool, error) {
	if model.DB == nil || task == nil || task.TaskID == "" || pollingContext == nil || pollingContext.Operation == nil {
		return false, nil
	}
	if pollingContext.ValidationErr != nil {
		return durableTerminalManualReviewUsingContext(ctx, pollingContext, task, "succeeded", 0, "polling_context_invalid", evidence)
	}
	if !validDurableTerminalEvidence(pollingContext, evidence) {
		return durableTerminalManualReviewUsingContext(ctx, pollingContext, task, "succeeded", actualQuota, reason, evidence, clamps...)
	}
	manualReview := false
	var quotaClamp *common.QuotaClamp
	for _, clamp := range clamps {
		if clamp != nil {
			manualReview = true
			quotaClamp = clamp
			break
		}
	}
	reasonCode := sanitizeReasonCode(reason, "task_poll_settle")
	if strings.TrimSpace(reason) != "" && reasonCode == "task_poll_settle" && strings.ToLower(strings.TrimSpace(reason)) != reasonCode {
		logger.LogWarn(ctx, "unsafe provider terminal reason replaced with task_poll_settle")
	}
	observation, err := model.CreateOrLoadTaskTerminalObservation(model.DB, terminalObservationInput(pollingContext.Operation, task, "succeeded", actualQuota, reasonCode, evidence, manualReview, quotaClamp))
	if err != nil {
		return true, err
	}
	if manualReview {
		return true, model.ErrTaskTerminalObservationManualReview
	}
	_, err = model.ApplyTaskTerminalObservation(model.DB, observation.ID)
	if err != nil {
		return true, fmt.Errorf("apply terminal settlement observation: %w", err)
	}
	if reloadErr := model.DB.First(task, task.ID).Error; reloadErr != nil {
		return true, reloadErr
	}
	logger.LogInfo(ctx, fmt.Sprintf("task %s durable terminal settlement applied", task.TaskID))
	return true, nil
}

// DurableReleaseTaskOnFailure bridges async task failure to the durable accounting
// release system (T4). If the task is associated with a durable TaskSubmissionOperation,
// it executes ReleaseTaskQuotaReservation atomically and transitions the operation to failed.
func DurableReleaseTaskOnFailureWithEvidence(ctx context.Context, task *model.Task, reason string, evidence DurableTerminalEvidence) (bool, error) {
	pollingContext, err := LoadTaskPollingContext(model.DB, task)
	if err != nil {
		return false, err
	}
	return durableReleaseTaskOnFailureUsingContext(ctx, pollingContext, task, reason, evidence)
}

func durableReleaseTaskOnFailureUsingContext(ctx context.Context, pollingContext *TaskPollingContext, task *model.Task, reason string, evidence DurableTerminalEvidence) (bool, error) {
	if model.DB == nil || task == nil || task.TaskID == "" || pollingContext == nil || pollingContext.Operation == nil {
		return false, nil
	}
	if pollingContext.ValidationErr != nil {
		return durableTerminalManualReviewUsingContext(ctx, pollingContext, task, "failed", 0, "polling_context_invalid", evidence)
	}
	if !validDurableTerminalEvidence(pollingContext, evidence) {
		return durableTerminalManualReviewUsingContext(ctx, pollingContext, task, "failed", 0, reason, evidence)
	}
	reasonCode := sanitizeReasonCode(reason, "task_poll_release")
	if strings.TrimSpace(reason) != "" && reasonCode == "task_poll_release" && strings.ToLower(strings.TrimSpace(reason)) != reasonCode {
		logger.LogWarn(ctx, "unsafe provider terminal reason replaced with task_poll_release")
	}
	observation, err := model.CreateOrLoadTaskTerminalObservation(model.DB, terminalObservationInput(pollingContext.Operation, task, "failed", 0, reasonCode, evidence, false, nil))
	if err != nil {
		return true, err
	}
	_, err = model.ApplyTaskTerminalObservation(model.DB, observation.ID)
	if err != nil {
		return true, fmt.Errorf("apply terminal failure observation: %w", err)
	}
	if reloadErr := model.DB.First(task, task.ID).Error; reloadErr != nil {
		return true, reloadErr
	}
	logger.LogInfo(ctx, fmt.Sprintf("task %s durable terminal failure applied", task.TaskID))
	return true, nil
}

func terminalObservationInput(op *model.TaskSubmissionOperation, task *model.Task, outcome string, actualQuota int, reasonCode string, evidence DurableTerminalEvidence, manualReview bool, clamp *common.QuotaClamp) model.TaskTerminalObservationInput {
	resolutionSource := ""
	if !manualReview {
		resolutionSource = model.TaskSubmissionResolutionSourceProviderVerified
	}
	return model.TaskTerminalObservationInput{
		OperationID: op.ID, TaskID: task.ID, Outcome: outcome, ActualQuota: int64(actualQuota), ReasonCode: reasonCode,
		RequestID: op.RequestID, ManualReview: manualReview, ResolutionSource: resolutionSource,
		EvidenceID: strings.TrimSpace(evidence.ID), EvidenceHash: strings.ToLower(strings.TrimSpace(evidence.Hash)), EvidenceVersion: evidence.Version,
		ResultURL: sanitizeTerminalResultURL(task.PrivateData.ResultURL), TaskData: terminalTaskDataProjection(task.Data),
		TaskStartTime: task.StartTime, TaskFinishTime: task.FinishTime, OperationalResultURL: terminalOperationalResultURL(task.PrivateData.ResultURL),
		TaskUpstreamID: strings.TrimSpace(task.PrivateData.UpstreamTaskID), QuotaClamp: clamp,
	}
}

func validDurableTerminalEvidence(pollingContext *TaskPollingContext, evidence DurableTerminalEvidence) bool {
	if pollingContext == nil || pollingContext.Operation == nil || pollingContext.Attempt == nil || evidence.Version != 1 {
		return false
	}
	id := strings.TrimSpace(evidence.ID)
	return id != "" && len(id) <= 191 && (id == pollingContext.Attempt.ProviderOperationID || id == pollingContext.Attempt.UpstreamRequestID)
}

func immutableTaskInitialQuota(pollingContext *TaskPollingContext) (int, error) {
	if pollingContext == nil || pollingContext.Operation == nil || pollingContext.ReserveReceipt == nil || pollingContext.ValidationErr != nil {
		return 0, fmt.Errorf("durable polling billing context is unavailable")
	}
	receipt := pollingContext.ReserveReceipt
	if receipt.FreeModel {
		if receipt.EstimatedQuota != 0 {
			return 0, fmt.Errorf("free model has non-zero estimated quota")
		}
		return 0, nil
	}
	return TaskInitialQuota(receipt.RequestFingerprintVersion, receipt.EstimatedQuota, receipt.Quota), nil
}

func immutableTaskBillingContext(pollingContext *TaskPollingContext) (*model.TaskBillingContext, error) {
	if pollingContext == nil || pollingContext.ReserveReceipt == nil || pollingContext.ValidationErr != nil {
		return nil, fmt.Errorf("durable polling billing context is unavailable")
	}
	billingContext := model.TaskBillingContext(pollingContext.ReserveReceipt.BillingContext)
	if _, err := validateTaskBillingSnapshot(&billingContext); err != nil {
		return nil, err
	}
	return &billingContext, nil
}

func computeTaskQuotaFromTokensWithContext(task *model.Task, pollingContext *TaskPollingContext, totalTokens int) (int, *common.QuotaClamp, bool) {
	if totalTokens <= 0 {
		return 0, nil, false
	}
	var rates *model.TaskBillingContext
	if pollingContext != nil && pollingContext.Durable() {
		var err error
		rates, err = immutableTaskBillingContext(pollingContext)
		if err != nil {
			return 0, nil, false
		}
	} else {
		var err error
		rates, _, err = resolveTaskTokenBillingRates(task)
		if err != nil {
			return 0, nil, false
		}
	}
	otherMultiplier := 1.0
	if priceData := taskBillingContextPriceData(rates); priceData != nil {
		otherMultiplier = priceData.OtherRatioMultiplier()
	}
	if rates.ModelRatio == 0 || rates.GroupRatio == 0 {
		return 0, nil, true
	}
	actualQuota, clamp := common.QuotaFromFloatChecked(float64(totalTokens) * rates.ModelRatio * rates.GroupRatio * otherMultiplier)
	return actualQuota, clamp, true
}

func computeTaskQuotaFromTokens(task *model.Task, totalTokens int) (int, *common.QuotaClamp, bool) {
	return computeTaskQuotaFromTokensWithContext(task, nil, totalTokens)
}

func sanitizeReasonCode(reason string, defaultCode string) string {
	reason = strings.ToLower(strings.TrimSpace(reason))
	if reason == "" || len(reason) > 64 {
		return defaultCode
	}
	for _, c := range reason {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '.' && c != '_' && c != '-' {
			return defaultCode
		}
	}
	return reason
}

func terminalTaskDataProjection(raw []byte) string {
	if len(raw) == 0 || len(raw) > 60*1024 {
		return ""
	}
	var decoded interface{}
	if err := common.Unmarshal(raw, &decoded); err != nil {
		return ""
	}
	canonical, err := common.Marshal(decoded)
	if err != nil || len(canonical) > 60*1024 {
		return ""
	}
	return string(canonical)
}

func terminalOperationalResultURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 8*1024 {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.User != nil {
		return ""
	}
	if parsed.IsAbs() {
		if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return ""
		}
		return parsed.String()
	}
	if parsed.Host != "" || !strings.HasPrefix(parsed.Path, "/") {
		return ""
	}
	return parsed.String()
}

func sanitizeTerminalResultURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return ""
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.RawFragment = ""
	parsed.ForceQuery = false
	value := parsed.String()
	if len(value) > 4096 {
		return ""
	}
	return value
}
