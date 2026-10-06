package model

import (
	"context"
	"errors"

	"github.com/ForceMind/MyAPI/common"
	"gorm.io/gorm"
)

const OpenAIOfficialPriceCheckEnabledOptionKey = "OpenAIOfficialPriceCheckEnabled"

// CheckedAt records a successful observation, independently of the immutable
// source version's first fetch time and of any administrator publication.
type OpenAIPriceCheckResult struct {
	CheckedAt       int64  `json:"checked_at"`
	SourceSHA256    string `json:"source_sha256"`
	SourceFetchedAt int64  `json:"source_fetched_at"`
}

// CompleteOpenAIPriceCheck commits frozen evidence and the successful task
// receipt together. The source callback must only save immutable evidence; it
// must never publish prices. An expired/canceled/fenced worker cannot save a
// new source or replace the last successful receipt. A nil callback finishes a
// failed attempt without touching source evidence.
func CompleteOpenAIPriceCheck(ctx context.Context, task *SystemTask, runnerID string, saveSource func(*gorm.DB) (*OpenAIPriceCheckResult, error), errorCode string) error {
	if DB == nil {
		return gorm.ErrInvalidDB
	}
	if task == nil || task.Type != SystemTaskTypeOpenAIPriceCheck || task.FenceToken <= 0 || runnerID == "" || (saveSource == nil && errorCode == "") {
		return gorm.ErrInvalidData
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var lease SystemTaskLock
		err := lockForUpdate(tx).Where("type = ? AND task_id = ? AND locked_by = ? AND fence_token = ? AND locked_until >= ?", task.Type, task.TaskID, runnerID, task.FenceToken, common.GetTimestamp()).Take(&lease).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrSystemTaskLockLost
		}
		if err != nil {
			return err
		}
		var running SystemTask
		err = lockForUpdate(tx).Where("task_id = ? AND type = ? AND status = ? AND locked_by = ? AND fence_token = ?", task.TaskID, task.Type, SystemTaskStatusRunning, runnerID, task.FenceToken).Take(&running).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrSystemTaskLockLost
		}
		if err != nil {
			return err
		}
		status, resultText := SystemTaskStatusFailed, ""
		if saveSource != nil {
			result, err := saveSource(tx)
			if err != nil {
				return err
			}
			if result == nil || result.CheckedAt <= 0 || result.CheckedAt > common.GetTimestamp() || result.SourceFetchedAt <= 0 {
				return gorm.ErrInvalidData
			}
			// Validate the reference inside the same transaction, including its
			// content hash; a successful receipt cannot point at missing data.
			version, err := GetOfficialPriceVersion(ctx, tx, result.SourceSHA256)
			if err != nil {
				return err
			}
			if result.SourceFetchedAt != version.FetchedAt {
				return gorm.ErrInvalidData
			}
			resultText, err = marshalSystemTaskJSON(result)
			if err != nil {
				return err
			}
			status, errorCode = SystemTaskStatusSucceeded, ""
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		now := common.GetTimestamp()
		if lease.LockedUntil < now {
			return ErrSystemTaskLockLost
		}
		liveLease := tx.Model(&SystemTaskLock{}).Select("task_id").Where("type = ? AND task_id = ? AND locked_by = ? AND fence_token = ? AND locked_until >= ?", task.Type, task.TaskID, runnerID, task.FenceToken, now)
		updated := tx.Model(&SystemTask{}).Where("task_id = ? AND type = ? AND status = ? AND locked_by = ? AND fence_token = ?", task.TaskID, task.Type, SystemTaskStatusRunning, runnerID, task.FenceToken).
			Where("task_id IN (?)", liveLease).
			Updates(map[string]any{"status": status, "active_key": nil, "result": resultText, "error": errorCode, "updated_at": now})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return ErrSystemTaskLockLost
		}
		return tx.Where("type = ? AND task_id = ? AND locked_by = ? AND fence_token = ?", task.Type, task.TaskID, runnerID, task.FenceToken).Delete(&SystemTaskLock{}).Error
	})
}

// ReadOpenAIPriceCheckTasks reads two bounded task records. Failed attempts do
// not overwrite the last good observation, even when the source hash repeats.
func ReadOpenAIPriceCheckTasks(ctx context.Context) (latest, successful *SystemTask, err error) {
	if DB == nil {
		return nil, nil, gorm.ErrInvalidDB
	}
	latest = &SystemTask{}
	err = DB.WithContext(ctx).Where("type = ?", SystemTaskTypeOpenAIPriceCheck).Order("id DESC").Take(latest).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	successful = &SystemTask{}
	err = DB.WithContext(ctx).Where("type = ? AND status = ? AND id <= ?", SystemTaskTypeOpenAIPriceCheck, SystemTaskStatusSucceeded, latest.ID).Order("id DESC").Take(successful).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return latest, nil, nil
	}
	return latest, successful, err
}
