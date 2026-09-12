package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"gorm.io/gorm"
)

const (
	DefaultTaskEngineWorkerID = "task_engine_runner"
)

// TaskEngineConfig defines the configuration parameters for TaskEngineRunner.
type TaskEngineConfig struct {
	WorkerID                  string
	StaleDispatchThreshold    time.Duration
	StaleReservationThreshold time.Duration
	BatchSize                 int
	LeaseDuration             time.Duration
	OutboxMaxAttempts         int
	LogDB                     *gorm.DB
}

// TaskEngineReport contains the execution metrics and errors from a runner pass.
type TaskEngineReport struct {
	StaleDispatchingRecovered int           `json:"stale_dispatching_recovered"`
	StaleUnfinishedRecovered  int           `json:"stale_unfinished_recovered"`
	ExpiredBillingReclaimed   int           `json:"expired_billing_reclaimed"`
	OutboxDelivered           int           `json:"outbox_delivered"`
	Duration                  time.Duration `json:"duration"`
	Errors                    []string      `json:"errors,omitempty"`
}

// HasErrors returns true if any errors occurred during the runner execution.
func (r TaskEngineReport) HasErrors() bool {
	return len(r.Errors) > 0
}

// ToJSON serializes the report to a JSON string using common.Marshal.
func (r TaskEngineReport) ToJSON() (string, error) {
	data, err := common.Marshal(r)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// TaskEngineRunner coordinates background task recovery scans and billing log outbox deliveries.
type TaskEngineRunner struct {
	cfg            TaskEngineConfig
	db             *gorm.DB
	recoveryWorker *TaskRecoveryWorker
	outboxService  *TaskBillingOutboxService
}

// NewTaskEngineRunner creates a new TaskEngineRunner instance with the provided configuration and database.
func NewTaskEngineRunner(cfg TaskEngineConfig, db *gorm.DB) *TaskEngineRunner {
	if db == nil {
		db = model.DB
	}

	workerID := strings.TrimSpace(cfg.WorkerID)
	if workerID == "" {
		workerID = DefaultTaskEngineWorkerID
	}
	cfg.WorkerID = workerID

	if cfg.StaleDispatchThreshold <= 0 {
		cfg.StaleDispatchThreshold = DefaultStaleDispatchThreshold
	}
	if cfg.StaleReservationThreshold <= 0 {
		cfg.StaleReservationThreshold = DefaultStaleReservationThreshold
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = DefaultRecoveryBatchSize
	}
	if cfg.LeaseDuration <= 0 {
		cfg.LeaseDuration = DefaultRecoveryLeaseDuration
	}
	if cfg.OutboxMaxAttempts <= 0 {
		cfg.OutboxMaxAttempts = DefaultOutboxMaxAttempts
	}

	recoveryWorker := &TaskRecoveryWorker{
		WorkerID:                  cfg.WorkerID,
		StaleDispatchThreshold:    cfg.StaleDispatchThreshold,
		StaleReservationThreshold: cfg.StaleReservationThreshold,
		BatchSize:                 cfg.BatchSize,
		LeaseDuration:             cfg.LeaseDuration,
	}

	outboxLeaseSec := int64(cfg.LeaseDuration.Seconds())
	if outboxLeaseSec <= 0 && cfg.LeaseDuration > 0 {
		outboxLeaseSec = 1
	}

	outboxService := &TaskBillingOutboxService{
		WorkerID:      cfg.WorkerID,
		LeaseDuration: outboxLeaseSec,
		BatchSize:     cfg.BatchSize,
		MaxAttempts:   cfg.OutboxMaxAttempts,
		LogDB:         cfg.LogDB,
	}

	return &TaskEngineRunner{
		cfg:            cfg,
		db:             db,
		recoveryWorker: recoveryWorker,
		outboxService:  outboxService,
	}
}

// Config returns a copy of the configuration used by the runner.
func (r *TaskEngineRunner) Config() TaskEngineConfig {
	return r.cfg
}

// DB returns the database handle used by the runner.
func (r *TaskEngineRunner) DB() *gorm.DB {
	return r.db
}

// RecoveryWorker returns the underlying TaskRecoveryWorker instance.
func (r *TaskEngineRunner) RecoveryWorker() *TaskRecoveryWorker {
	return r.recoveryWorker
}

// OutboxService returns the underlying TaskBillingOutboxService instance.
func (r *TaskEngineRunner) OutboxService() *TaskBillingOutboxService {
	return r.outboxService
}

// RunRecoveryPass executes a single recovery pass across stale dispatching operations,
// stale unfinished prepared or reserved operations, and expired billing event leases.
func (r *TaskEngineRunner) RunRecoveryPass(ctx context.Context) (dispatching int, unfinished int, expiredBilling int, errs []error) {
	if r == nil {
		return 0, 0, 0, []error{errors.New("task engine runner is nil")}
	}
	if err := ctx.Err(); err != nil {
		return 0, 0, 0, []error{err}
	}
	db := r.db
	if db == nil {
		db = model.DB
	}
	if db == nil {
		return 0, 0, 0, []error{gorm.ErrInvalidDB}
	}
	if r.recoveryWorker == nil {
		return 0, 0, 0, []error{errors.New("task recovery worker is not initialized")}
	}

	d, err := r.recoveryWorker.RecoverStaleDispatching(ctx, db)
	dispatching = d
	if err != nil {
		errs = append(errs, err)
	}
	if ctx.Err() != nil {
		return dispatching, unfinished, expiredBilling, errs
	}

	u, err := r.recoveryWorker.RecoverStaleUnfinished(ctx, db)
	unfinished = u
	if err != nil {
		errs = append(errs, err)
	}
	if ctx.Err() != nil {
		return dispatching, unfinished, expiredBilling, errs
	}

	b, err := r.recoveryWorker.RecoverExpiredBillingEvents(ctx, db)
	expiredBilling = b
	if err != nil {
		errs = append(errs, err)
	}
	terminal, err := r.recoveryWorker.RecoverTerminalObservations(ctx, db)
	_ = terminal
	if err != nil {
		errs = append(errs, err)
	}

	return dispatching, unfinished, expiredBilling, errs
}

// RunOutboxPass executes a single pass delivering claimable task billing log outbox entries.
func (r *TaskEngineRunner) RunOutboxPass(ctx context.Context) (delivered int, err error) {
	if r == nil {
		return 0, errors.New("task engine runner is nil")
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	db := r.db
	if db == nil {
		db = model.DB
	}
	if db == nil {
		return 0, gorm.ErrInvalidDB
	}
	if r.outboxService == nil {
		return 0, errors.New("task billing outbox service is not initialized")
	}

	return r.outboxService.ProcessClaimableBatch(ctx, db)
}

// RunOnce executes a single unified engine pass consisting of recovery and outbox delivery,
// recording metrics and errors in the returned report.
func (r *TaskEngineRunner) RunOnce(ctx context.Context) TaskEngineReport {
	start := time.Now()
	if r == nil {
		return TaskEngineReport{
			Duration: time.Since(start),
			Errors:   []string{"task engine runner is nil"},
		}
	}

	var report TaskEngineReport

	dispatching, unfinished, expiredBilling, recErrs := r.RunRecoveryPass(ctx)
	report.StaleDispatchingRecovered = dispatching
	report.StaleUnfinishedRecovered = unfinished
	report.ExpiredBillingReclaimed = expiredBilling
	for _, err := range recErrs {
		if err != nil {
			report.Errors = append(report.Errors, err.Error())
		}
	}

	if ctx.Err() == nil {
		delivered, outErr := r.RunOutboxPass(ctx)
		report.OutboxDelivered = delivered
		if outErr != nil {
			report.Errors = append(report.Errors, outErr.Error())
		}
	} else if len(report.Errors) == 0 {
		report.Errors = append(report.Errors, ctx.Err().Error())
	}

	report.Duration = time.Since(start)
	return report
}
