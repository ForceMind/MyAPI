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
	return durableTerminalWithoutEvidence(ctx, task, "succeeded", actualQuota, reason, clamps...)
}

func DurableReleaseTaskOnFailure(ctx context.Context, task *model.Task, reason string) (bool, error) {
	return durableTerminalWithoutEvidence(ctx, task, "failed", 0, reason)
}

func durableTerminalWithoutEvidence(ctx context.Context, task *model.Task, outcome string, actualQuota int, reason string, clamps ...*common.QuotaClamp) (bool, error) {
	return DurableTerminalManualReviewWithEvidence(ctx, task, outcome, actualQuota, reason, DurableTerminalEvidence{}, clamps...)
}

func DurableTerminalManualReviewWithEvidence(ctx context.Context, task *model.Task, outcome string, actualQuota int, reason string, evidence DurableTerminalEvidence, clamps ...*common.QuotaClamp) (bool, error) {
	if model.DB == nil || task == nil || task.TaskID == "" {
		return false, nil
	}
	op, err := model.GetTaskSubmissionOperationByPublicID(model.DB, task.TaskID)
	if err != nil {
		return false, err
	}
	if op == nil {
		return false, nil
	}
	reasonCode := sanitizeReasonCode(reason, "terminal_evidence_missing")
	var clamp *common.QuotaClamp
	for _, candidate := range clamps {
		if candidate != nil {
			clamp = candidate
			break
		}
	}
	_, err = model.CreateOrLoadTaskTerminalObservation(model.DB, model.TaskTerminalObservationInput{OperationID: op.ID, TaskID: task.ID, Outcome: outcome, ActualQuota: int64(actualQuota), ReasonCode: reasonCode, RequestID: op.RequestID, ResultURL: sanitizeTerminalResultURL(task.PrivateData.ResultURL), TaskData: terminalTaskDataProjection(task.Data), TaskStartTime: task.StartTime, TaskFinishTime: task.FinishTime, OperationalResultURL: terminalOperationalResultURL(task.PrivateData.ResultURL), TaskUpstreamID: strings.TrimSpace(task.PrivateData.UpstreamTaskID), ManualReview: true, QuotaClamp: clamp, EvidenceID: strings.TrimSpace(evidence.ID), EvidenceHash: strings.ToLower(strings.TrimSpace(evidence.Hash)), EvidenceVersion: evidence.Version})
	if err != nil {
		return true, err
	}
	logger.LogWarn(ctx, "durable terminal result lacks immutable evidence and requires manual review")
	return true, model.ErrTaskTerminalObservationManualReview
}

func DurableSettleTaskOnCompleteWithEvidence(ctx context.Context, task *model.Task, actualQuota int, reason string, evidence DurableTerminalEvidence, clamps ...*common.QuotaClamp) (bool, error) {
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
	if !validDurableTerminalEvidence(op, evidence) {
		return durableTerminalWithoutEvidence(ctx, task, "succeeded", actualQuota, reason, clamps...)
	}
	manualReview := false
	for _, clamp := range clamps {
		if clamp != nil {
			manualReview = true
			break
		}
	}
	reasonCode := sanitizeReasonCode(reason, "task_poll_settle")
	if strings.TrimSpace(reason) != "" && reasonCode == "task_poll_settle" && strings.ToLower(strings.TrimSpace(reason)) != reasonCode {
		logger.LogWarn(ctx, "unsafe provider terminal reason replaced with task_poll_settle")
	}
	observation, err := model.CreateOrLoadTaskTerminalObservation(model.DB, model.TaskTerminalObservationInput{
		OperationID: op.ID, TaskID: task.ID, Outcome: "succeeded", ActualQuota: int64(actualQuota),
		ReasonCode: reasonCode, ManualReview: manualReview,
		RequestID: op.RequestID, ResolutionSource: model.TaskSubmissionResolutionSourceProviderVerified, EvidenceID: evidence.ID, EvidenceHash: strings.ToLower(strings.TrimSpace(evidence.Hash)), EvidenceVersion: evidence.Version,
		ResultURL: sanitizeTerminalResultURL(task.PrivateData.ResultURL),
		TaskData:  terminalTaskDataProjection(task.Data), TaskStartTime: task.StartTime, TaskFinishTime: task.FinishTime,
		OperationalResultURL: terminalOperationalResultURL(task.PrivateData.ResultURL), TaskUpstreamID: strings.TrimSpace(task.PrivateData.UpstreamTaskID),
		QuotaClamp: func() *common.QuotaClamp {
			for _, clamp := range clamps {
				if clamp != nil {
					return clamp
				}
			}
			return nil
		}(),
	})
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
//
// Returns (handled=true, nil) if processed by durable accounting.
// Returns (handled=false, nil) if the task is not backed by a durable operation (caller should use legacy refund).
func DurableReleaseTaskOnFailureWithEvidence(ctx context.Context, task *model.Task, reason string, evidence DurableTerminalEvidence) (bool, error) {
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
	if !validDurableTerminalEvidence(op, evidence) {
		return durableTerminalWithoutEvidence(ctx, task, "failed", 0, reason)
	}
	reasonCode := sanitizeReasonCode(reason, "task_poll_release")
	if strings.TrimSpace(reason) != "" && reasonCode == "task_poll_release" && strings.ToLower(strings.TrimSpace(reason)) != reasonCode {
		logger.LogWarn(ctx, "unsafe provider terminal reason replaced with task_poll_release")
	}
	observation, err := model.CreateOrLoadTaskTerminalObservation(model.DB, model.TaskTerminalObservationInput{
		OperationID: op.ID, TaskID: task.ID, Outcome: "failed", ActualQuota: 0,
		ReasonCode: reasonCode,
		RequestID:  op.RequestID, ResolutionSource: model.TaskSubmissionResolutionSourceProviderVerified, EvidenceID: evidence.ID, EvidenceHash: strings.ToLower(strings.TrimSpace(evidence.Hash)), EvidenceVersion: evidence.Version,
		ResultURL: sanitizeTerminalResultURL(task.PrivateData.ResultURL),
		TaskData:  terminalTaskDataProjection(task.Data), TaskStartTime: task.StartTime, TaskFinishTime: task.FinishTime,
		OperationalResultURL: terminalOperationalResultURL(task.PrivateData.ResultURL), TaskUpstreamID: strings.TrimSpace(task.PrivateData.UpstreamTaskID),
	})
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

func validDurableTerminalEvidence(op *model.TaskSubmissionOperation, evidence DurableTerminalEvidence) bool {
	if op == nil || evidence.Version != 1 {
		return false
	}
	attempt, err := model.FindAttemptByOperationID(model.DB, op.ID)
	if err != nil || attempt == nil {
		return false
	}
	id := strings.TrimSpace(evidence.ID)
	return id != "" && len(id) <= 191 && (id == attempt.ProviderOperationID || id == attempt.UpstreamRequestID)
}

func computeTaskQuotaFromTokens(task *model.Task, totalTokens int) (int, *common.QuotaClamp, bool) {
	if totalTokens <= 0 {
		return 0, nil, false
	}
	rates, _, err := resolveTaskTokenBillingRates(task)
	if err != nil {
		return task.Quota, nil, true
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
