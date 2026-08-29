package service

import (
	"context"
	"fmt"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/logger"
	"github.com/ForceMind/MyAPI/model"
)

const (
	channelQuotaSnapshotCleanupBatchSize = 500
	// Bound one scheduled pass so a large backlog cannot monopolize the DB.
	channelQuotaSnapshotCleanupMaxRowsPerRun = 5000
)

type channelQuotaSnapshotCleanupHandler struct{}

func (channelQuotaSnapshotCleanupHandler) Type() string {
	return model.SystemTaskTypeChannelQuotaSnapshotCleanup
}

func (channelQuotaSnapshotCleanupHandler) Enabled() bool {
	return common.ChannelQuotaSnapshotRetentionDays > 0
}

func (channelQuotaSnapshotCleanupHandler) Interval() time.Duration {
	return 24 * time.Hour
}

func (channelQuotaSnapshotCleanupHandler) NewPayload() any {
	return channelQuotaSnapshotCleanupPayload{
		RetentionDays: common.ChannelQuotaSnapshotRetentionDays,
		BatchSize:     channelQuotaSnapshotCleanupBatchSize,
	}
}

type channelQuotaSnapshotCleanupPayload struct {
	RetentionDays int `json:"retention_days"`
	BatchSize     int `json:"batch_size"`
}

type channelQuotaSnapshotCleanupResult struct {
	DeletedCount int64 `json:"deleted_count"`
}

func (channelQuotaSnapshotCleanupHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	payload := channelQuotaSnapshotCleanupPayload{}
	if err := task.DecodePayload(&payload); err != nil {
		finishChannelQuotaSnapshotCleanupTask(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}
	if payload.RetentionDays <= 0 {
		finishChannelQuotaSnapshotCleanupTask(task, runnerID, model.SystemTaskStatusSucceeded, channelQuotaSnapshotCleanupResult{}, nil)
		return
	}
	if payload.BatchSize <= 0 {
		payload.BatchSize = channelQuotaSnapshotCleanupBatchSize
	}
	cutoff := common.GetTimestamp() - int64(payload.RetentionDays)*24*60*60
	var deleted int64
	for {
		if err := ctx.Err(); err != nil {
			finishChannelQuotaSnapshotCleanupTask(task, runnerID, model.SystemTaskStatusFailed, channelQuotaSnapshotCleanupResult{DeletedCount: deleted}, err)
			return
		}
		n, err := model.DeleteOldChannelQuotaSnapshotBatch(ctx, cutoff, payload.BatchSize)
		if err != nil {
			finishChannelQuotaSnapshotCleanupTask(task, runnerID, model.SystemTaskStatusFailed, channelQuotaSnapshotCleanupResult{DeletedCount: deleted}, err)
			return
		}
		deleted += n
		if n == 0 || deleted >= channelQuotaSnapshotCleanupMaxRowsPerRun {
			break
		}
	}
	finishChannelQuotaSnapshotCleanupTask(task, runnerID, model.SystemTaskStatusSucceeded, channelQuotaSnapshotCleanupResult{DeletedCount: deleted}, nil)
}

func finishChannelQuotaSnapshotCleanupTask(task *model.SystemTask, runnerID string, status model.SystemTaskStatus, result any, runErr error) {
	errorMessage := ""
	if runErr != nil {
		errorMessage = runErr.Error()
	}
	if err := model.FinishSystemTask(task.TaskID, runnerID, status, result, errorMessage); err != nil {
		logger.LogWarn(context.Background(), fmt.Sprintf("system task %s cleanup finish failed: %v", task.TaskID, err))
	}
}

func init() {
	RegisterSystemTaskHandler(channelQuotaSnapshotCleanupHandler{})
}
