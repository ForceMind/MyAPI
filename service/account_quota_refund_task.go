package service

import (
	"context"
	"errors"
	"time"

	"github.com/ForceMind/MyAPI/model"
)

type accountQuotaRefundRecoveryHandler struct{}

func (accountQuotaRefundRecoveryHandler) Type() string {
	return model.SystemTaskTypeAccountQuotaRefundRecovery
}

func (accountQuotaRefundRecoveryHandler) Enabled() bool { return true }

func (accountQuotaRefundRecoveryHandler) Interval() time.Duration { return 30 * time.Second }

type accountQuotaRecoveryPayload struct {
	BatchSize int `json:"batch_size"`
}

func (accountQuotaRefundRecoveryHandler) NewPayload() any {
	return accountQuotaRecoveryPayload{BatchSize: quotaProjectionSystemTaskBatchSize}
}

func (accountQuotaRefundRecoveryHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	payload := accountQuotaRecoveryPayload{}
	if err := task.DecodePayload(&payload); err != nil {
		finishQuotaProjectionSystemTask(task, runnerID, 0, err)
		return
	}
	worker := NewTaskRecoveryWorker(runnerID)
	if payload.BatchSize > 0 && payload.BatchSize <= 100 {
		worker.BatchSize = payload.BatchSize
	}
	processed, recoveryErr := worker.RecoverAccountQuotaRefundFacts(ctx, model.DB)
	finishQuotaProjectionSystemTask(task, runnerID, processed, recoveryErr)
}

func init() {
	RegisterSystemTaskHandler(accountQuotaRefundRecoveryHandler{})
	RegisterSystemTaskHandler(quotaMaintenanceBackfillHandler{})
}

type quotaMaintenanceBackfillHandler struct{}

func (quotaMaintenanceBackfillHandler) Type() string {
	return model.SystemTaskTypeQuotaMaintenanceBackfill
}

func (quotaMaintenanceBackfillHandler) Enabled() bool {
	if model.DB == nil {
		return false
	}
	complete, err := model.QuotaMaintenanceBackfillsComplete(context.Background(), model.DB)
	return err != nil || !complete
}

func (quotaMaintenanceBackfillHandler) Interval() time.Duration { return 5 * time.Second }

func (quotaMaintenanceBackfillHandler) NewPayload() any {
	return struct{}{}
}

func (quotaMaintenanceBackfillHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	if err := ctx.Err(); err != nil {
		finishQuotaProjectionSystemTask(task, runnerID, 0, err)
		return
	}
	accountErr := model.InitializeAccountQuotaReservationHeadsWithDB(model.DB.WithContext(ctx))
	if errors.Is(accountErr, model.ErrQuotaWorkIncomplete) {
		accountErr = nil
	}
	balanceErr := model.InitializeQuotaBalanceBatchDrainsWithDB(model.DB.WithContext(ctx))
	if errors.Is(balanceErr, model.ErrQuotaWorkIncomplete) {
		balanceErr = nil
	}
	complete, statusErr := model.QuotaMaintenanceBackfillsComplete(ctx, model.DB)
	processed := 0
	if complete {
		processed = 1
	}
	finishQuotaProjectionSystemTask(task, runnerID, processed, errors.Join(accountErr, balanceErr, statusErr))
}
