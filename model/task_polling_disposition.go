package model

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

const (
	TaskPollingDispositionRetryable = "retryable"
	TaskPollingDispositionManual    = "manual"
)

const (
	taskPollingInitialBackoff = 15 * time.Second
	taskPollingMaxBackoff     = 15 * time.Minute
)

func unfinishedTaskBaseQuery(query *gorm.DB) *gorm.DB {
	return query.
		Where("progress != ?", "100%").
		Where("status NOT IN ?", []TaskStatus{TaskStatusFailure, TaskStatusSuccess})
}

func unfinishedTaskPollingQuery(query *gorm.DB, now int64) *gorm.DB {
	return unfinishedTaskBaseQuery(query).
		Where("(polling_disposition = ? OR polling_disposition IS NULL OR (polling_disposition = ? AND next_poll_at <= ?))", "", TaskPollingDispositionRetryable, now)
}

func TaskPollingBackoff(attempt int) time.Duration {
	if attempt <= 1 {
		return taskPollingInitialBackoff
	}
	backoff := taskPollingInitialBackoff
	for i := 1; i < attempt && backoff < taskPollingMaxBackoff; i++ {
		backoff *= 2
		if backoff >= taskPollingMaxBackoff {
			return taskPollingMaxBackoff
		}
	}
	return backoff
}

// MarkTaskPollingRetryable records local uncertainty without changing the task
// lifecycle status. The attempt-count predicate prevents concurrent pollers from
// extending the backoff twice from the same snapshot.
func MarkTaskPollingRetryable(db *gorm.DB, task *Task, reasonCode string, now int64) (bool, error) {
	if db == nil {
		return false, gorm.ErrInvalidDB
	}
	if task == nil || task.ID <= 0 || reasonCode == "" || now <= 0 {
		return false, errors.New("invalid task polling retryable disposition")
	}
	attempt := task.PollingAttemptCount + 1
	nextPollAt := now + int64(TaskPollingBackoff(attempt)/time.Second)
	result := db.Model(&Task{}).
		Where("id = ? AND polling_attempt_count = ?", task.ID, task.PollingAttemptCount).
		Where("status NOT IN ?", []TaskStatus{TaskStatusFailure, TaskStatusSuccess}).
		Updates(map[string]interface{}{
			"polling_disposition":   TaskPollingDispositionRetryable,
			"next_poll_at":          nextPollAt,
			"polling_reason_code":   reasonCode,
			"polling_attempt_count": attempt,
		})
	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected == 1 {
		task.PollingDisposition = TaskPollingDispositionRetryable
		task.NextPollAt = nextPollAt
		task.PollingReasonCode = reasonCode
		task.PollingAttemptCount = attempt
		return true, nil
	}
	return false, nil
}

func MarkTaskPollingManual(db *gorm.DB, task *Task, reasonCode string, now int64) (bool, error) {
	if db == nil {
		return false, gorm.ErrInvalidDB
	}
	if task == nil || task.ID <= 0 || reasonCode == "" || now <= 0 {
		return false, errors.New("invalid task polling manual disposition")
	}
	attempt := task.PollingAttemptCount + 1
	result := db.Model(&Task{}).
		Where("id = ? AND polling_attempt_count = ?", task.ID, task.PollingAttemptCount).
		Where("status NOT IN ?", []TaskStatus{TaskStatusFailure, TaskStatusSuccess}).
		Updates(map[string]interface{}{
			"polling_disposition":   TaskPollingDispositionManual,
			"next_poll_at":          int64(0),
			"polling_reason_code":   reasonCode,
			"polling_attempt_count": attempt,
		})
	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected == 1 {
		task.PollingDisposition = TaskPollingDispositionManual
		task.NextPollAt = 0
		task.PollingReasonCode = reasonCode
		task.PollingAttemptCount = attempt
		return true, nil
	}
	return false, nil
}

func ClearTaskPollingDisposition(db *gorm.DB, task *Task) (bool, error) {
	if db == nil {
		return false, gorm.ErrInvalidDB
	}
	if task == nil || task.ID <= 0 {
		return false, errors.New("invalid task polling disposition clear")
	}
	if task.PollingDisposition == "" && task.NextPollAt == 0 && task.PollingReasonCode == "" && task.PollingAttemptCount == 0 {
		return false, nil
	}
	result := db.Model(&Task{}).
		Where("id = ? AND polling_attempt_count = ?", task.ID, task.PollingAttemptCount).
		Where("status NOT IN ?", []TaskStatus{TaskStatusFailure, TaskStatusSuccess}).
		Updates(map[string]interface{}{
			"polling_disposition":   "",
			"next_poll_at":          int64(0),
			"polling_reason_code":   "",
			"polling_attempt_count": 0,
		})
	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected == 1 {
		task.PollingDisposition = ""
		task.NextPollAt = 0
		task.PollingReasonCode = ""
		task.PollingAttemptCount = 0
		return true, nil
	}
	return false, nil
}
