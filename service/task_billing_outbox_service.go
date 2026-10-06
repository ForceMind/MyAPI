package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/logger"
	"github.com/ForceMind/MyAPI/model"
	"gorm.io/gorm"
)

const (
	DefaultOutboxWorkerID      = "task_billing_outbox_worker"
	DefaultOutboxLeaseDuration = 60
	DefaultOutboxBatchSize     = 50
	DefaultOutboxMaxAttempts   = 5

	ErrorCodeLogDeliveryFailed         = "log_delivery_failed"
	ErrorCodeMaxAttemptsExceeded       = "max_attempts_exceeded"
	ErrorCodeBillingProjectionConflict = "billing_projection_conflict"
)

// TaskBillingOutboxService delivers pending and retryable task billing outbox items
// to the logs table with at-least-once durability and idempotent deduplication.
type TaskBillingOutboxService struct {
	WorkerID      string
	LeaseDuration int64 // 默认 60s
	BatchSize     int   // 默认 50
	MaxAttempts   int   // 默认 5
	LogDB         *gorm.DB
	NowFunc       func() int64
}

// NewTaskBillingOutboxService creates a new TaskBillingOutboxService with default settings.
func NewTaskBillingOutboxService(workerID string) *TaskBillingOutboxService {
	return &TaskBillingOutboxService{
		WorkerID:      workerID,
		LeaseDuration: DefaultOutboxLeaseDuration,
		BatchSize:     DefaultOutboxBatchSize,
		MaxAttempts:   DefaultOutboxMaxAttempts,
	}
}

func (s *TaskBillingOutboxService) getLogDB(db *gorm.DB) *gorm.DB {
	if s.LogDB != nil {
		return s.LogDB
	}
	if model.LOG_DB != nil {
		return model.LOG_DB
	}
	return db
}

func (s *TaskBillingOutboxService) getNow(db *gorm.DB) int64 {
	if s.NowFunc != nil {
		return s.NowFunc()
	}
	now, err := getDBTimestamp(db)
	if err != nil || now <= 0 {
		return time.Now().Unix()
	}
	return now
}

func computeBackoffDelay(attempt int) int64 {
	if attempt <= 0 {
		return 30
	}
	if attempt >= 12 {
		return model.TaskRecoveryMaxRetryDelaySeconds
	}
	delay := int64(30 * (1 << attempt))
	if delay > model.TaskRecoveryMaxRetryDelaySeconds {
		return model.TaskRecoveryMaxRetryDelaySeconds
	}
	return delay
}

func getDBTimestamp(tx *gorm.DB) (int64, error) {
	if tx == nil {
		return time.Now().Unix(), nil
	}
	tx = tx.Session(&gorm.Session{NewDB: true})
	var query string
	switch tx.Dialector.Name() {
	case "postgres":
		query = "SELECT FLOOR(EXTRACT(EPOCH FROM clock_timestamp()))::bigint"
	case "mysql":
		query = "SELECT UNIX_TIMESTAMP()"
	case "sqlite":
		query = "SELECT strftime('%s','now')"
	default:
		return time.Now().Unix(), nil
	}
	var databaseNow int64
	if err := tx.Raw(query).Scan(&databaseNow).Error; err != nil || databaseNow <= 0 {
		return time.Now().Unix(), nil
	}
	return databaseNow, nil
}

// ProcessClaimableBatch queries claimable task billing outbox records, claims processing leases via CAS,
// delivers the records idempotently to the logs sink, and advances their outbox state to delivered or retryable.
func (s *TaskBillingOutboxService) ProcessClaimableBatch(ctx context.Context, db *gorm.DB) (int, error) {
	if db == nil {
		db = model.DB
	}
	if db == nil {
		return 0, gorm.ErrInvalidDB
	}

	workerID := strings.TrimSpace(s.WorkerID)
	if workerID == "" {
		workerID = DefaultOutboxWorkerID
	}

	leaseDuration := s.LeaseDuration
	if leaseDuration <= 0 {
		leaseDuration = DefaultOutboxLeaseDuration
	}
	if leaseDuration > model.TaskRecoveryMaxProcessingLeaseSeconds {
		leaseDuration = model.TaskRecoveryMaxProcessingLeaseSeconds
	}

	batchSize := s.BatchSize
	if batchSize <= 0 {
		batchSize = DefaultOutboxBatchSize
	}

	maxAttempts := s.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = DefaultOutboxMaxAttempts
	}

	now := s.getNow(db)
	outboxes, err := model.ListClaimableTaskBillingLogOutboxes(db, now, batchSize)
	if err != nil {
		return 0, fmt.Errorf("list claimable task billing log outboxes: %w", err)
	}
	if len(outboxes) == 0 {
		return 0, nil
	}

	logDB := s.getLogDB(db)
	deliveredCount := 0

	for _, outbox := range outboxes {
		if err := ctx.Err(); err != nil {
			return deliveredCount, err
		}
		if outbox == nil || outbox.ID <= 0 {
			continue
		}

		// 1. Claim outbox lease via CAS
		var won bool
		var claimErr error
		switch outbox.State {
		case model.TaskBillingLogOutboxStatePending, model.TaskBillingLogOutboxStateRetryable:
			won, claimErr = model.TransitionTaskBillingLogOutbox(db, outbox.ID, model.TaskBillingLogOutboxTransition{
				From:            outbox.State,
				To:              model.TaskBillingLogOutboxStateClaimed,
				ExpectedVersion: outbox.LockVersion,
				Lease: model.TaskRecoveryProcessingLease{
					WorkerID:     workerID,
					LeaseSeconds: leaseDuration,
				},
			})
		case model.TaskBillingLogOutboxStateClaimed:
			// Claimed lease expired; reclaim with current worker
			won, claimErr = model.ReclaimExpiredTaskBillingLogOutbox(db, outbox.ID, model.TaskRecoveryProcessingLease{
				WorkerID:     workerID,
				LeaseSeconds: leaseDuration,
			}, outbox.LockVersion, 0)
		default:
			continue
		}

		if claimErr != nil || !won {
			// CAS contention or database error; another worker claimed it or state advanced
			continue
		}

		// Track optimistic lock version and attempt count reflecting the claim CAS update
		outbox.LockVersion++
		outbox.AttemptCount++

		// 2. Deliver the immutable log projection idempotently. Existing rows
		// are validated against the authoritative outbox payload before a replay
		// is accepted as already delivered.
		logRecord := model.Log{
			UserId:            outbox.Payload.UserID,
			CreatedAt:         outbox.Payload.CreatedAt,
			Type:              outbox.Payload.Type,
			Content:           outbox.Payload.Content,
			Username:          outbox.Payload.Username,
			TokenName:         outbox.Payload.TokenName,
			ModelName:         outbox.Payload.ModelName,
			Quota:             outbox.Payload.Quota,
			PromptTokens:      outbox.Payload.PromptTokens,
			CompletionTokens:  outbox.Payload.CompletionTokens,
			UseTime:           outbox.Payload.UseTime,
			IsStream:          outbox.Payload.IsStream,
			ChannelId:         outbox.Payload.ChannelID,
			TokenId:           outbox.Payload.TokenID,
			Group:             outbox.Payload.Group,
			Ip:                outbox.Payload.IP,
			RequestId:         outbox.Payload.RequestID,
			UpstreamRequestId: outbox.Payload.UpstreamRequestID,
			BillingEventID:    outbox.BillingEventID,
			Other:             outbox.Payload.Other,
		}
		if logRecord.RequestId == "" {
			if outbox.BillingEventID != "" {
				digest := sha256.Sum256([]byte(outbox.BillingEventID))
				logRecord.RequestId = "billing_" + hex.EncodeToString(digest[:])[:48]
			} else {
				logRecord.RequestId = common.NewRequestId()
			}
		}
		if logRecord.CreatedAt == 0 {
			logRecord.CreatedAt = now
		}

		var deliverErr error
		if err := model.PrepareLogProjectionIdentity(&logRecord); err != nil {
			deliverErr = err
		} else if logDB.Dialector.Name() == string(common.DatabaseTypeClickHouse) {
			if err := model.EnsureClickHouseBillingProjectionIdentity(
				ctx,
				logDB,
				logRecord.BillingEventID,
				logRecord.BillingProjectionDigest,
			); err != nil {
				deliverErr = err
			} else {
				deliverErr = model.CreateLog(logDB.WithContext(ctx), &logRecord)
			}
		} else {
			var existingLogs []*model.Log
			if err := logDB.WithContext(ctx).
				Where("billing_event_id = ?", outbox.BillingEventID).
				Find(&existingLogs).Error; err != nil {
				deliverErr = err
			} else if err := model.ValidateBillingProjectionCompatibility(&logRecord, existingLogs); err != nil {
				deliverErr = err
			} else if len(existingLogs) == 0 {
				deliverErr = model.CreateLog(logDB.WithContext(ctx), &logRecord)
			}
		}

		// 3. Complete transition based on delivery result
		if deliverErr == nil {
			deliveredWon, transitionErr := model.TransitionTaskBillingLogOutbox(db, outbox.ID, model.TaskBillingLogOutboxTransition{
				From:            model.TaskBillingLogOutboxStateClaimed,
				To:              model.TaskBillingLogOutboxStateDelivered,
				WorkerID:        workerID,
				ExpectedVersion: outbox.LockVersion,
			})
			if transitionErr != nil || !deliveredWon {
				logger.LogError(ctx, fmt.Sprintf("failed to transition task billing outbox %d to delivered: won=%v, err=%v", outbox.ID, deliveredWon, transitionErr))
			} else {
				deliveredCount++
			}
		} else {
			logger.LogError(ctx, fmt.Sprintf("task billing outbox delivery failed for %s (id=%d, attempt=%d): %v", outbox.BillingEventID, outbox.ID, outbox.AttemptCount, deliverErr))
			if errors.Is(deliverErr, model.ErrBillingProjectionConflict) || errors.Is(deliverErr, model.ErrBillingProjectionDigestMismatch) {
				quarantineErr := model.QuarantineBillingProjectionEvent(ctx, logDB, outbox.BillingEventID, deliverErr.Error())
				if quarantineErr != nil {
					logger.LogError(ctx, fmt.Sprintf("failed to persist billing projection quarantine for outbox %d: %v", outbox.ID, quarantineErr))
					deliverErr = errors.Join(deliverErr, quarantineErr)
				} else {
					_, quarantineErr = model.TransitionTaskBillingLogOutbox(db, outbox.ID, model.TaskBillingLogOutboxTransition{
						From:            model.TaskBillingLogOutboxStateClaimed,
						To:              model.TaskBillingLogOutboxStateQuarantined,
						WorkerID:        workerID,
						ExpectedVersion: outbox.LockVersion,
						LastErrorCode:   ErrorCodeBillingProjectionConflict,
					})
					if quarantineErr != nil {
						logger.LogError(ctx, fmt.Sprintf("failed to quarantine task billing outbox %d: %v", outbox.ID, quarantineErr))
					}
					continue
				}
			}
			errorCode := ErrorCodeLogDeliveryFailed
			var retryDelay int64

			if outbox.AttemptCount >= maxAttempts {
				logger.LogError(ctx, fmt.Sprintf("task billing outbox %s (id=%d) exceeded max attempts (%d), marked for long backoff", outbox.BillingEventID, outbox.ID, maxAttempts))
				errorCode = ErrorCodeMaxAttemptsExceeded
				retryDelay = model.TaskRecoveryMaxRetryDelaySeconds
			} else {
				retryDelay = computeBackoffDelay(outbox.AttemptCount)
			}

			_, retryTransitionErr := model.TransitionTaskBillingLogOutbox(db, outbox.ID, model.TaskBillingLogOutboxTransition{
				From:              model.TaskBillingLogOutboxStateClaimed,
				To:                model.TaskBillingLogOutboxStateRetryable,
				WorkerID:          workerID,
				ExpectedVersion:   outbox.LockVersion,
				LastErrorCode:     errorCode,
				RetryDelaySeconds: retryDelay,
			})
			if retryTransitionErr != nil {
				logger.LogError(ctx, fmt.Sprintf("failed to transition task billing outbox %d to retryable: %v", outbox.ID, retryTransitionErr))
			}
		}
	}

	return deliveredCount, nil
}
