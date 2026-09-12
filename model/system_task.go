package model

import (
	"context"
	"errors"
	"strings"

	"github.com/ForceMind/MyAPI/common"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type SystemTaskStatus string

const (
	SystemTaskStatusPending   SystemTaskStatus = "pending"
	SystemTaskStatusRunning   SystemTaskStatus = "running"
	SystemTaskStatusSucceeded SystemTaskStatus = "succeeded"
	SystemTaskStatusFailed    SystemTaskStatus = "failed"

	SystemTaskTypeLogCleanup                  = "log_cleanup"
	SystemTaskTypeChannelTest                 = "channel_test"
	SystemTaskTypeModelUpdate                 = "model_update"
	SystemTaskTypeMidjourneyPoll              = "midjourney_poll"
	SystemTaskTypeAsyncTaskPoll               = "async_task_poll"
	SystemTaskTypeChannelQuotaSnapshotCleanup = "channel_quota_snapshot_cleanup"
	// SystemTaskTypeChannelQuotaSnapshotSync periodically samples normalized
	// upstream account balances. Sampling is enabled by default; an explicit
	// environment override takes precedence over the administrator setting.
	SystemTaskTypeChannelQuotaSnapshotSync = "channel_quota_snapshot_sync"
	SystemTaskTypeTaskRecovery             = "task_recovery"
	SystemTaskTypeTaskBillingOutbox        = "task_billing_outbox"
	SystemTaskTypeLogProjectionBackfill    = "log_projection_backfill"
)

var ErrSystemTaskLockLost = errors.New("system task lock lost")

type SystemTask struct {
	ID         int64            `json:"id" gorm:"primary_key"`
	TaskID     string           `json:"task_id" gorm:"type:varchar(64);uniqueIndex"`
	Type       string           `json:"type" gorm:"type:varchar(64);index"`
	Status     SystemTaskStatus `json:"status" gorm:"type:varchar(32);index"`
	ActiveKey  *string          `json:"active_key,omitempty" gorm:"type:varchar(64);uniqueIndex"`
	Payload    string           `json:"payload" gorm:"type:text"`
	State      string           `json:"state" gorm:"type:text"`
	Result     string           `json:"result" gorm:"type:text"`
	Error      string           `json:"error" gorm:"type:text"`
	LockedBy   string           `json:"locked_by" gorm:"type:varchar(128);index"`
	FenceToken int64            `json:"fence_token" gorm:"bigint;not null;default:0"`
	CreatedAt  int64            `json:"created_at" gorm:"bigint;index"`
	UpdatedAt  int64            `json:"updated_at" gorm:"bigint;index"`
}

type SystemTaskLock struct {
	Type        string `json:"type" gorm:"type:varchar(64);primaryKey"`
	TaskID      string `json:"task_id" gorm:"type:varchar(64);index"`
	LockedBy    string `json:"locked_by" gorm:"type:varchar(128);index"`
	LockedUntil int64  `json:"locked_until" gorm:"bigint;index"`
	FenceToken  int64  `json:"fence_token" gorm:"bigint;not null;default:0"`
	UpdatedAt   int64  `json:"updated_at" gorm:"bigint;index"`
}

type SystemTaskResponse struct {
	ID         int64            `json:"id"`
	TaskID     string           `json:"task_id"`
	Type       string           `json:"type"`
	Status     SystemTaskStatus `json:"status"`
	ActiveKey  *string          `json:"active_key,omitempty"`
	Payload    any              `json:"payload"`
	State      any              `json:"state"`
	Result     any              `json:"result"`
	Error      string           `json:"error"`
	LockedBy   string           `json:"locked_by"`
	FenceToken int64            `json:"fence_token"`
	CreatedAt  int64            `json:"created_at"`
	UpdatedAt  int64            `json:"updated_at"`
}

func (task *SystemTask) BeforeCreate(_ *gorm.DB) error {
	now := common.GetTimestamp()
	if task.CreatedAt == 0 {
		task.CreatedAt = now
	}
	if task.UpdatedAt == 0 {
		task.UpdatedAt = now
	}
	return nil
}

func (lock *SystemTaskLock) BeforeCreate(_ *gorm.DB) error {
	if lock.UpdatedAt == 0 {
		lock.UpdatedAt = common.GetTimestamp()
	}
	return nil
}

func GenerateSystemTaskID() (string, error) {
	key, err := common.GenerateRandomCharsKey(32)
	if err != nil {
		return "", err
	}
	return "systask_" + key, nil
}

func CreateSystemTask(taskType string, payload any, state any) (*SystemTask, error) {
	taskID, err := GenerateSystemTaskID()
	if err != nil {
		return nil, err
	}
	payloadText, err := marshalSystemTaskJSON(payload)
	if err != nil {
		return nil, err
	}
	stateText, err := marshalSystemTaskJSON(state)
	if err != nil {
		return nil, err
	}

	task := &SystemTask{
		TaskID:    taskID,
		Type:      taskType,
		Status:    SystemTaskStatusPending,
		ActiveKey: &taskType,
		Payload:   payloadText,
		State:     stateText,
	}

	if err := DB.Create(task).Error; err != nil {
		return nil, err
	}
	return task, nil
}

func GetSystemTaskByTaskID(taskID string) (*SystemTask, error) {
	var task SystemTask
	if err := DB.Where("task_id = ?", taskID).First(&task).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &task, nil
}

func GetActiveSystemTask(taskType string) (*SystemTask, error) {
	var task SystemTask
	err := DB.Where("type = ? AND status IN ?", taskType, activeSystemTaskStatuses()).
		Order("id desc").
		First(&task).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &task, nil
}

func FindPendingSystemTasks(taskType string, limit int) ([]*SystemTask, error) {
	var tasks []*SystemTask
	if limit <= 0 {
		limit = 1
	}
	err := DB.Where("type = ? AND status = ?", taskType, SystemTaskStatusPending).
		Order("id asc").
		Limit(limit).
		Find(&tasks).Error
	return tasks, err
}

func FindEarliestPendingSystemTasks(taskTypes []string) (map[string]*SystemTask, error) {
	tasksByType := map[string]*SystemTask{}
	if len(taskTypes) == 0 {
		return tasksByType, nil
	}

	subQuery := DB.Model(&SystemTask{}).
		Select("MIN(id)").
		Where("type IN ? AND status = ?", taskTypes, SystemTaskStatusPending).
		Group("type")
	var tasks []*SystemTask
	if err := DB.Where("id IN (?)", subQuery).Find(&tasks).Error; err != nil {
		return nil, err
	}
	for _, task := range tasks {
		tasksByType[task.Type] = task
	}
	return tasksByType, nil
}

func ListSystemTasks(limit int) ([]*SystemTask, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	var tasks []*SystemTask
	err := DB.Order("id desc").Limit(limit).Find(&tasks).Error
	return tasks, err
}

// GetLatestSystemTask returns the most recent task row of the given type
// (any status) so the scheduler can decide whether enough time has elapsed
// since the last run. Returns (nil, nil) when no row exists.
func GetLatestSystemTask(taskType string) (*SystemTask, error) {
	var task SystemTask
	err := DB.Where("type = ?", taskType).Order("id desc").First(&task).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &task, nil
}

func GetLatestSystemTasks(taskTypes []string) (map[string]*SystemTask, error) {
	tasksByType := map[string]*SystemTask{}
	if len(taskTypes) == 0 {
		return tasksByType, nil
	}

	subQuery := DB.Model(&SystemTask{}).
		Select("MAX(id)").
		Where("type IN ?", taskTypes).
		Group("type")
	var tasks []*SystemTask
	if err := DB.Where("id IN (?)", subQuery).Find(&tasks).Error; err != nil {
		return nil, err
	}
	for _, task := range tasks {
		tasksByType[task.Type] = task
	}
	return tasksByType, nil
}

func ClaimSystemTask(id int64, taskType string, runnerID string, lockUntil int64) (*SystemTask, bool, error) {
	now := common.GetTimestamp()
	var task SystemTask
	if err := DB.Where("id = ? AND type = ? AND status = ?", id, taskType, SystemTaskStatusPending).First(&task).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}

	acquired, expiredTaskID, fenceToken, err := acquireSystemTaskLock(taskType, task.TaskID, runnerID, now, lockUntil)
	if err != nil || !acquired {
		return nil, acquired, err
	}
	if expiredTaskID != "" && expiredTaskID != task.TaskID {
		if err := MarkSystemTaskLeaseExpired(expiredTaskID); err != nil {
			_ = ReleaseSystemTaskLock(task.TaskID, runnerID)
			return nil, false, err
		}
	}

	result := DB.Model(&SystemTask{}).
		Where("id = ? AND type = ? AND status = ?", id, taskType, SystemTaskStatusPending).
		Updates(map[string]any{
			"status":      SystemTaskStatusRunning,
			"locked_by":   runnerID,
			"fence_token": fenceToken,
			"updated_at":  now,
		})
	if result.Error != nil {
		_ = ReleaseSystemTaskLock(task.TaskID, runnerID)
		return nil, false, result.Error
	}
	if result.RowsAffected == 0 {
		_ = ReleaseSystemTaskLock(task.TaskID, runnerID)
		return nil, false, nil
	}

	if err := DB.Where("id = ?", id).First(&task).Error; err != nil {
		return nil, false, err
	}
	return &task, true, nil
}

func acquireSystemTaskLock(taskType string, taskID string, lockedBy string, now int64, lockUntil int64) (bool, string, int64, error) {
	return acquireSystemTaskLockWithContext(context.Background(), taskType, taskID, lockedBy, now, lockUntil)
}

// TryAcquireNamedSystemTaskLockWithContext attempts to acquire a reusable
// lease identified by lockType. It does not create a SystemTask row, making it
// suitable for short cross-instance coordination that reuses the migrated
// system_task_locks table.
func TryAcquireNamedSystemTaskLockWithContext(ctx context.Context, lockType string, taskID string, lockedBy string, lockUntil int64) (bool, error) {
	if DB == nil {
		return false, gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	lockType = strings.TrimSpace(lockType)
	taskID = strings.TrimSpace(taskID)
	lockedBy = strings.TrimSpace(lockedBy)
	if lockType == "" || len(lockType) > 64 || taskID == "" || len(taskID) > 64 || lockedBy == "" || len(lockedBy) > 128 {
		return false, errors.New("invalid named system task lock identity")
	}
	now := common.GetTimestamp()
	if lockUntil <= now {
		return false, errors.New("named system task lock expiry must be in the future")
	}
	acquired, _, _, err := acquireSystemTaskLockWithContext(ctx, lockType, taskID, lockedBy, now, lockUntil)
	return acquired, err
}

func acquireSystemTaskLockWithContext(ctx context.Context, taskType string, taskID string, lockedBy string, now int64, lockUntil int64) (bool, string, int64, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	db := DB.WithContext(ctx).Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})
	lock := &SystemTaskLock{
		Type:        taskType,
		TaskID:      taskID,
		LockedBy:    lockedBy,
		LockedUntil: lockUntil,
		FenceToken:  1,
		UpdatedAt:   now,
	}
	if err := db.Create(lock).Error; err == nil {
		return true, "", lock.FenceToken, nil
	}
	if err := ctx.Err(); err != nil {
		return false, "", 0, err
	}

	var existing SystemTaskLock
	err := db.Where("type = ?", taskType).First(&existing).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, "", 0, nil
		}
		return false, "", 0, err
	}
	if existing.LockedUntil >= now {
		return false, "", 0, nil
	}

	nextFenceToken := existing.FenceToken + 1
	if nextFenceToken <= 0 {
		nextFenceToken = 1
	}
	result := db.Model(&SystemTaskLock{}).
		Where("type = ? AND task_id = ? AND locked_by = ? AND locked_until = ? AND fence_token = ? AND locked_until < ?", taskType, existing.TaskID, existing.LockedBy, existing.LockedUntil, existing.FenceToken, now).
		Updates(map[string]any{
			"task_id":      taskID,
			"locked_by":    lockedBy,
			"locked_until": lockUntil,
			"fence_token":  nextFenceToken,
			"updated_at":   now,
		})
	if result.Error != nil {
		return false, "", 0, result.Error
	}
	if result.RowsAffected == 0 {
		return false, "", 0, nil
	}
	return true, existing.TaskID, nextFenceToken, nil
}

// ReleaseNamedSystemTaskLockWithContext releases only the exact named lease
// held by taskID and lockedBy. A lost/expired lease is an idempotent no-op.
func ReleaseNamedSystemTaskLockWithContext(ctx context.Context, lockType string, taskID string, lockedBy string) error {
	if DB == nil {
		return gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result := DB.WithContext(ctx).
		Where("type = ? AND task_id = ? AND locked_by = ?", lockType, taskID, lockedBy).
		Delete(&SystemTaskLock{})
	return result.Error
}

func UpdateSystemTaskState(taskID string, lockedBy string, state any) error {
	return UpdateSystemTaskStateWithContext(context.Background(), taskID, lockedBy, state)
}

func UpdateSystemTaskStateWithContext(ctx context.Context, taskID string, lockedBy string, state any) error {
	stateText, err := marshalSystemTaskJSON(state)
	if err != nil {
		return err
	}
	now := common.GetTimestamp()
	result := DB.WithContext(ctx).Model(&SystemTask{}).
		Where("task_id = ? AND status = ? AND locked_by = ?", taskID, SystemTaskStatusRunning, lockedBy).
		Where("EXISTS (SELECT 1 FROM system_task_locks WHERE system_task_locks.task_id = system_tasks.task_id AND system_task_locks.locked_by = ? AND system_task_locks.locked_until >= ?)", lockedBy, now).
		Updates(map[string]any{
			"state":      stateText,
			"updated_at": now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrSystemTaskLockLost
	}
	return nil
}

func RenewSystemTaskLock(taskID string, lockedBy string, lockUntil int64) error {
	return RenewSystemTaskLockWithContext(context.Background(), taskID, lockedBy, lockUntil)
}

func RenewSystemTaskLockWithContext(ctx context.Context, taskID string, lockedBy string, lockUntil int64) error {
	return renewSystemTaskLockWithFence(ctx, taskID, lockedBy, 0, lockUntil)
}

func RenewSystemTaskLockWithFence(ctx context.Context, taskID string, lockedBy string, fenceToken int64, lockUntil int64) error {
	return renewSystemTaskLockWithFence(ctx, taskID, lockedBy, fenceToken, lockUntil)
}

func renewSystemTaskLockWithFence(ctx context.Context, taskID string, lockedBy string, fenceToken int64, lockUntil int64) error {
	now := common.GetTimestamp()
	query := DB.WithContext(ctx).Model(&SystemTaskLock{}).
		Where("task_id = ? AND locked_by = ? AND locked_until >= ?", taskID, lockedBy, now)
	if fenceToken > 0 {
		query = query.Where("fence_token = ?", fenceToken)
	}
	result := query.
		Updates(map[string]any{
			"locked_until": lockUntil,
			"updated_at":   now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrSystemTaskLockLost
	}
	return nil
}

func MarkSystemTaskLeaseExpired(taskID string) error {
	result := DB.Model(&SystemTask{}).
		Where("task_id = ? AND status = ?", taskID, SystemTaskStatusRunning).
		Updates(map[string]any{
			"status":     SystemTaskStatusFailed,
			"active_key": nil,
			"error":      "task lease expired",
			"updated_at": common.GetTimestamp(),
		})
	return result.Error
}

func ExpireStaleSystemTaskLocks(now int64) error {
	var locks []*SystemTaskLock
	if err := DB.Where("locked_until < ?", now).Find(&locks).Error; err != nil {
		return err
	}
	for _, lock := range locks {
		if err := MarkSystemTaskLeaseExpired(lock.TaskID); err != nil {
			return err
		}
		result := DB.Where("type = ? AND task_id = ? AND locked_by = ? AND locked_until < ?", lock.Type, lock.TaskID, lock.LockedBy, now).
			Delete(&SystemTaskLock{})
		if result.Error != nil {
			return result.Error
		}
	}
	return nil
}

func ReleaseSystemTaskLock(taskID string, lockedBy string) error {
	return ReleaseSystemTaskLockWithContext(context.Background(), taskID, lockedBy)
}

func ReleaseSystemTaskLockWithContext(ctx context.Context, taskID string, lockedBy string) error {
	result := DB.WithContext(ctx).Where("task_id = ? AND locked_by = ?", taskID, lockedBy).Delete(&SystemTaskLock{})
	return result.Error
}

func FinishSystemTask(taskID string, lockedBy string, status SystemTaskStatus, resultPayload any, errorMessage string) error {
	return FinishSystemTaskWithContext(context.Background(), taskID, lockedBy, status, resultPayload, errorMessage)
}

func FinishSystemTaskWithContext(ctx context.Context, taskID string, lockedBy string, status SystemTaskStatus, resultPayload any, errorMessage string) error {
	resultText, err := marshalSystemTaskJSON(resultPayload)
	if err != nil {
		return err
	}
	now := common.GetTimestamp()
	result := DB.WithContext(ctx).Model(&SystemTask{}).
		Where("task_id = ? AND status = ? AND locked_by = ?", taskID, SystemTaskStatusRunning, lockedBy).
		Where("EXISTS (SELECT 1 FROM system_task_locks WHERE system_task_locks.task_id = system_tasks.task_id AND system_task_locks.locked_by = ? AND system_task_locks.locked_until >= ?)", lockedBy, now).
		Updates(map[string]any{
			"status":     status,
			"active_key": nil,
			"result":     resultText,
			"error":      errorMessage,
			"updated_at": now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrSystemTaskLockLost
	}
	return ReleaseSystemTaskLockWithContext(ctx, taskID, lockedBy)
}

func (task *SystemTask) DecodePayload(v any) error {
	return decodeSystemTaskJSONString(task.Payload, v)
}

func (task *SystemTask) DecodeState(v any) error {
	return decodeSystemTaskJSONString(task.State, v)
}

func (task *SystemTask) ToResponse() SystemTaskResponse {
	return SystemTaskResponse{
		ID:         task.ID,
		TaskID:     task.TaskID,
		Type:       task.Type,
		Status:     task.Status,
		ActiveKey:  task.ActiveKey,
		Payload:    decodeSystemTaskJSONValue(task.Payload),
		State:      decodeSystemTaskJSONValue(task.State),
		Result:     decodeSystemTaskJSONValue(task.Result),
		Error:      task.Error,
		LockedBy:   task.LockedBy,
		FenceToken: task.FenceToken,
		CreatedAt:  task.CreatedAt,
		UpdatedAt:  task.UpdatedAt,
	}
}

func activeSystemTaskStatuses() []string {
	return []string{string(SystemTaskStatusPending), string(SystemTaskStatusRunning)}
}

func marshalSystemTaskJSON(v any) (string, error) {
	if v == nil {
		return "", nil
	}
	data, err := common.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func decodeSystemTaskJSONString(data string, v any) error {
	if data == "" {
		return nil
	}
	return common.UnmarshalJsonStr(data, v)
}

func decodeSystemTaskJSONValue(data string) any {
	if data == "" {
		return nil
	}
	var value any
	if err := common.UnmarshalJsonStr(data, &value); err != nil {
		return data
	}
	return value
}
