package service

import (
	"context"
	"fmt"
	"time"

	"github.com/ForceMind/MyAPI/logger"
	"github.com/ForceMind/MyAPI/model"
)

type taskPollingDisposition string

const (
	taskPollingDispositionRetryable taskPollingDisposition = "retryable"
	taskPollingDispositionManual    taskPollingDisposition = "manual"
)

func recordTaskPollingUncertainty(ctx context.Context, entry *taskPollingEntry, disposition taskPollingDisposition, reason string) error {
	if entry == nil || entry.Task == nil || entry.Context == nil || !entry.Context.Durable() {
		return nil
	}
	reasonCode := sanitizeReasonCode(reason, "polling_uncertain")
	var won bool
	var err error
	switch disposition {
	case taskPollingDispositionRetryable:
		won, err = model.MarkTaskPollingRetryable(model.DB, entry.Task, reasonCode, time.Now().Unix())
	case taskPollingDispositionManual:
		won, err = model.MarkTaskPollingManual(model.DB, entry.Task, reasonCode, time.Now().Unix())
	default:
		return fmt.Errorf("unsupported task polling disposition %q", disposition)
	}
	if err != nil {
		return err
	}
	if won {
		logger.LogWarn(ctx, fmt.Sprintf("task polling deferred: task=%s disposition=%s reason=%s", entry.Task.TaskID, disposition, reasonCode))
	}
	return nil
}

func clearTaskPollingUncertainty(entry *taskPollingEntry) error {
	if entry == nil || entry.Task == nil || entry.Context == nil || !entry.Context.Durable() {
		return nil
	}
	_, err := model.ClearTaskPollingDisposition(model.DB, entry.Task)
	return err
}

// DeferTaskPolling records realtime-query uncertainty through the same private
// disposition used by the background poller.
func DeferTaskPolling(ctx context.Context, task *model.Task, manual bool, reason string) error {
	pollingContext, err := LoadTaskPollingContext(model.DB, task)
	if err != nil {
		return err
	}
	disposition := taskPollingDispositionRetryable
	if manual {
		disposition = taskPollingDispositionManual
	}
	return recordTaskPollingUncertainty(ctx, &taskPollingEntry{Task: task, Context: pollingContext}, disposition, reason)
}
