package service

import (
	"context"
	"fmt"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/logger"
	"github.com/ForceMind/MyAPI/model"
)

const quotaProjectionSystemTaskBatchSize = 50

type quotaProjectionRecoveryHandler struct{}

func (quotaProjectionRecoveryHandler) Type() string {
	return model.SystemTaskTypeQuotaProjectionRecovery
}

func (quotaProjectionRecoveryHandler) Enabled() bool {
	return common.TaskRecoveryObligationRecoveryEnabled
}

func (quotaProjectionRecoveryHandler) Interval() time.Duration {
	return 30 * time.Second
}

func (quotaProjectionRecoveryHandler) NewPayload() any {
	return struct {
		BatchSize int `json:"batch_size"`
	}{BatchSize: quotaProjectionSystemTaskBatchSize}
}

func (quotaProjectionRecoveryHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	payload := struct {
		BatchSize int `json:"batch_size"`
	}{}
	if err := task.DecodePayload(&payload); err != nil {
		finishQuotaProjectionSystemTask(task, runnerID, 0, err)
		return
	}
	if payload.BatchSize <= 0 || payload.BatchSize > 100 {
		payload.BatchSize = quotaProjectionSystemTaskBatchSize
	}
	worker := NewTaskRecoveryWorker(runnerID)
	worker.BatchSize = payload.BatchSize
	processed, err := worker.RecoverQuotaProjectionObligations(ctx, model.DB)
	finishQuotaProjectionSystemTask(task, runnerID, processed, err)
}

func finishQuotaProjectionSystemTask(task *model.SystemTask, runnerID string, processed int, runErr error) {
	status := model.SystemTaskStatusSucceeded
	errorMessage := ""
	if runErr != nil {
		status = model.SystemTaskStatusFailed
		errorMessage = runErr.Error()
	}
	result := struct {
		Processed int `json:"processed"`
	}{Processed: processed}
	if err := model.FinishSystemTask(task.TaskID, runnerID, status, result, errorMessage); err != nil {
		logger.LogWarn(context.Background(), fmt.Sprintf("system task %s quota projection finish failed: %v", task.TaskID, err))
	}
}

func init() {
	RegisterSystemTaskHandler(quotaProjectionRecoveryHandler{})
}
