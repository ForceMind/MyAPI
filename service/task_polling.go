package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	taskdto "github.com/ForceMind/MyAPI/dto"
	"github.com/ForceMind/MyAPI/logger"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/relay/channel/task/taskcommon"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/samber/lo"
)

// TaskPollingAdaptor 定义轮询所需的最小适配器接口，避免 service -> relay 的循环依赖
type TaskPollingAdaptor interface {
	Init(info *relaycommon.RelayInfo)
	FetchTask(baseURL string, key string, body map[string]any, proxy string) (*http.Response, error)
	ParseTaskResult(body []byte) (*relaycommon.TaskInfo, error)
	// AdjustBillingOnComplete 在任务到达终态（成功/失败）时由轮询循环调用。
	// 返回正数触发差额结算（补扣/退还），返回 0 保持预扣费金额不变。
	AdjustBillingOnComplete(task *model.Task, taskResult *relaycommon.TaskInfo) int
}

// GetTaskAdaptorFunc 由 main 包注入，用于获取指定平台的任务适配器。
// 打破 service -> relay -> relay/channel -> service 的循环依赖。
var GetTaskAdaptorFunc func(platform constant.TaskPlatform) TaskPollingAdaptor

// sweepTimedOutTasks runs independently from the provider polling window. A
// durable timeout is local uncertainty, so it is moved to manual polling review
// without creating a speculative terminal failure observation.
func sweepTimedOutTasks(ctx context.Context) {
	capability, err := InspectTaskPollingSchema(model.DB)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("inspect timeout polling schema: %v", err))
		return
	}
	sweepTimedOutTasksWithCapability(ctx, capability)
}

func sweepTimedOutTasksWithCapability(ctx context.Context, capability TaskPollingSchemaCapability) {
	if constant.TaskTimeoutMinutes <= 0 {
		return
	}
	cutoff := time.Now().Unix() - int64(constant.TaskTimeoutMinutes)*60
	tasks := model.GetTimedOutUnfinishedTasks(cutoff, 100)
	if len(tasks) == 0 {
		return
	}
	contexts, err := loadTaskPollingContextsWithCapability(model.DB, tasks, capability)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("load timeout polling context: %v", err))
		return
	}

	reason := fmt.Sprintf("任务超时（%d分钟）", constant.TaskTimeoutMinutes)
	legacyReason := "任务超时（旧系统遗留任务，不进行退款，请联系管理员）"
	now := time.Now().Unix()
	timedOutCount := 0

	for _, task := range tasks {
		pollingContext := contexts.ForTask(task)
		if pollingContext.Durable() {
			entry := &taskPollingEntry{Task: task, Context: pollingContext}
			if err := recordTaskPollingUncertainty(ctx, entry, taskPollingDispositionManual, "task_timeout_unverified"); err != nil {
				logger.LogError(ctx, fmt.Sprintf("record durable timeout disposition for task %s: %v", task.TaskID, err))
			}
			continue
		}
		isLegacy := task.SubmitTime > 0 && task.SubmitTime < model.TaskRefundLegacyCutoff

		oldStatus := task.Status
		task.Status = model.TaskStatusFailure
		task.Progress = "100%"
		task.FinishTime = now
		if isLegacy {
			task.FailReason = legacyReason
			task.Quota = 0
		} else {
			task.FailReason = reason
		}

		won, err := task.UpdateWithStatus(oldStatus)
		if err != nil {
			logger.LogError(ctx, fmt.Sprintf("sweepTimedOutTasks CAS update error for task %s: %v", task.TaskID, err))
			continue
		}
		if !won {
			continue
		}
		timedOutCount++
		if !isLegacy && task.Quota != 0 {
			RefundTaskQuota(ctx, task, reason)
		}
	}

	if timedOutCount > 0 {
		logger.LogInfo(ctx, fmt.Sprintf("sweepTimedOutTasks: timed out %d tasks", timedOutCount))
	}
}

// TaskPollSummary is the result recorded on an async_task_poll system task row,
// summarizing one polling pass.
type TaskPollSummary struct {
	UnfinishedTasks  int `json:"unfinished_tasks"`
	PlatformsScanned int `json:"platforms_scanned"`
	NullTasksFailed  int `json:"null_tasks_failed"`
}

// RunTaskPollingOnce performs one async-task (Suno/video) polling pass.
func RunTaskPollingOnce(ctx context.Context, report func(processed, total int)) TaskPollSummary {
	summary := TaskPollSummary{}
	if GetTaskAdaptorFunc == nil {
		return summary
	}
	if ctx == nil {
		ctx = context.Background()
	}

	common.SysLog("任务进度轮询开始")
	capability, err := InspectTaskPollingSchema(model.DB)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("inspect task polling schema: %v", err))
		return summary
	}
	sweepTimedOutTasksWithCapability(ctx, capability)
	allTasks, err := model.GetAllUnfinishedSyncTasksForPolling(constant.TaskQueryLimit, time.Now().Unix())
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("load unfinished polling tasks: %v", err))
		return summary
	}
	summary.UnfinishedTasks = len(allTasks)
	contexts, err := loadTaskPollingContextsWithCapability(model.DB, allTasks, capability)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("load task polling metadata: %v", err))
		return summary
	}

	eligible := make([]*model.Task, 0, len(allTasks))
	legacyNullIDs := make([]int64, 0)
	seenKeys := make(map[taskPollingKey]*model.Task)
	duplicateTasks := make(map[int64]*model.Task)
	for _, task := range allTasks {
		pollingContext := contexts.ForTask(task)
		entry := &taskPollingEntry{Task: task, Context: pollingContext}
		if pollingContext.ValidationErr != nil {
			if err := recordTaskPollingUncertainty(ctx, entry, taskPollingDispositionManual, "polling_context_invalid"); err != nil {
				logger.LogError(ctx, fmt.Sprintf("record invalid polling context for task %s: %v", task.TaskID, err))
			}
			continue
		}
		if pollingContext.Durable() && strings.TrimSpace(task.PrivateData.UpstreamTaskID) == "" {
			summary.NullTasksFailed++
			if err := recordTaskPollingUncertainty(ctx, entry, taskPollingDispositionManual, "missing_upstream_task_id"); err != nil {
				logger.LogError(ctx, fmt.Sprintf("record missing upstream id for task %s: %v", task.TaskID, err))
			}
			continue
		}
		upstreamID := strings.TrimSpace(task.GetUpstreamTaskID())
		if upstreamID == "" {
			summary.NullTasksFailed++
			legacyNullIDs = append(legacyNullIDs, task.ID)
			continue
		}
		key := taskPollingKey{ChannelID: task.ChannelId, UpstreamID: upstreamID}
		if previous := seenKeys[key]; previous != nil {
			duplicateTasks[previous.ID] = previous
			duplicateTasks[task.ID] = task
			continue
		}
		seenKeys[key] = task
		eligible = append(eligible, task)
	}
	if len(duplicateTasks) > 0 {
		for _, task := range duplicateTasks {
			entry := &taskPollingEntry{Task: task, Context: contexts.ForTask(task)}
			if err := recordTaskPollingUncertainty(ctx, entry, taskPollingDispositionManual, "duplicate_polling_key"); err != nil {
				logger.LogError(ctx, fmt.Sprintf("record duplicate polling key for task %s: %v", task.TaskID, err))
			}
		}
		filtered := eligible[:0]
		for _, task := range eligible {
			if duplicateTasks[task.ID] == nil {
				filtered = append(filtered, task)
			}
		}
		eligible = filtered
	}
	if len(legacyNullIDs) > 0 {
		if err := model.TaskBulkUpdateByID(legacyNullIDs, map[string]any{"status": model.TaskStatusFailure, "progress": "100%"}); err != nil {
			logger.LogError(ctx, fmt.Sprintf("fix legacy null upstream task IDs: %v", err))
		}
	}

	platformTasks := make(map[constant.TaskPlatform][]*model.Task)
	for _, task := range eligible {
		platformTasks[task.Platform] = append(platformTasks[task.Platform], task)
	}
	totalPlatforms := len(platformTasks)
	processedPlatforms := 0
	for platform, tasks := range platformTasks {
		if ctx.Err() != nil {
			break
		}
		if report != nil {
			report(processedPlatforms, totalPlatforms)
		}
		processedPlatforms++
		batch, duplicates := newTaskPollingBatch(tasks, contexts)
		if len(duplicates) != 0 {
			logger.LogError(ctx, "duplicate polling keys remained after pass validation")
			continue
		}
		summary.PlatformsScanned++
		dispatchPlatformUpdate(ctx, platform, batch)
	}
	if report != nil && ctx.Err() == nil {
		report(totalPlatforms, totalPlatforms)
	}
	common.SysLog("任务进度轮询完成")
	return summary
}

// DispatchPlatformUpdate is retained for direct callers. The production pass
// uses dispatchPlatformUpdate with a channel-scoped composite key batch.
func DispatchPlatformUpdate(ctx context.Context, platform constant.TaskPlatform, taskChannelM map[int][]string, taskM map[string]*model.Task) {
	tasks := make([]*model.Task, 0, len(taskM))
	for _, task := range taskM {
		tasks = append(tasks, task)
	}
	contexts, err := LoadTaskPollingContexts(model.DB, tasks)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("load direct polling metadata: %v", err))
		return
	}
	batch, err := pollingBatchFromLegacyMaps(taskChannelM, taskM, contexts)
	if err != nil {
		logger.LogError(ctx, err.Error())
		return
	}
	dispatchPlatformUpdate(ctx, platform, batch)
}

func dispatchPlatformUpdate(ctx context.Context, platform constant.TaskPlatform, batch *taskPollingBatch) {
	if ctx == nil {
		ctx = context.Background()
	}
	switch platform {
	case constant.TaskPlatformMidjourney:
	case constant.TaskPlatformSuno:
		_ = updateSunoPollingBatch(ctx, batch)
	default:
		if err := updateVideoPollingBatch(ctx, platform, batch); err != nil {
			common.SysLog(fmt.Sprintf("UpdateVideoTasks fail: %s", err))
		}
	}
}

func UpdateSunoTasks(ctx context.Context, taskChannelM map[int][]string, taskM map[string]*model.Task) error {
	tasks := make([]*model.Task, 0, len(taskM))
	for _, task := range taskM {
		tasks = append(tasks, task)
	}
	contexts, err := LoadTaskPollingContexts(model.DB, tasks)
	if err != nil {
		return err
	}
	batch, err := pollingBatchFromLegacyMaps(taskChannelM, taskM, contexts)
	if err != nil {
		return err
	}
	return updateSunoPollingBatch(ctx, batch)
}

func updateSunoPollingBatch(ctx context.Context, batch *taskPollingBatch) error {
	channelIDs := make([]int, 0, len(batch.channelTasks))
	for channelID := range batch.channelTasks {
		channelIDs = append(channelIDs, channelID)
	}
	sort.Ints(channelIDs)
	for _, channelID := range channelIDs {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := updateSunoChannelTasks(ctx, channelID, batch.channelTasks[channelID], batch); err != nil {
			logger.LogError(ctx, fmt.Sprintf("渠道 #%d 更新异步任务失败: %s", channelID, err.Error()))
		}
	}
	return nil
}

func updateSunoTasks(ctx context.Context, channelID int, taskIDs []string, taskM map[string]*model.Task) error {
	tasks := make([]*model.Task, 0, len(taskM))
	for _, task := range taskM {
		tasks = append(tasks, task)
	}
	contexts, err := LoadTaskPollingContexts(model.DB, tasks)
	if err != nil {
		return err
	}
	batch, err := pollingBatchFromLegacyMaps(map[int][]string{channelID: taskIDs}, taskM, contexts)
	if err != nil {
		return err
	}
	return updateSunoChannelTasks(ctx, channelID, batch.channelTasks[channelID], batch)
}

func updateSunoChannelTasks(ctx context.Context, channelID int, keys []taskPollingKey, batch *taskPollingBatch) error {
	logger.LogInfo(ctx, fmt.Sprintf("渠道 #%d 未完成的任务有: %d", channelID, len(keys)))
	if ctx.Err() != nil || len(keys) == 0 {
		return ctx.Err()
	}
	entries := make([]*taskPollingEntry, 0, len(keys))
	taskIDs := make([]string, 0, len(keys))
	for _, key := range keys {
		entry := batch.entries[key]
		if entry == nil || entry.Task == nil || key.ChannelID != channelID || entry.Task.ChannelId != channelID {
			return errors.New("Suno polling channel/task mismatch")
		}
		entries = append(entries, entry)
		taskIDs = append(taskIDs, key.UpstreamID)
	}
	ch, err := model.CacheGetChannel(channelID)
	if err != nil {
		legacyIDs := make([]int64, 0, len(entries))
		for _, entry := range entries {
			if entry.Context.Durable() {
				if dispositionErr := recordTaskPollingUncertainty(ctx, entry, taskPollingDispositionRetryable, "suno_channel_unavailable"); dispositionErr != nil {
					logger.LogError(ctx, dispositionErr.Error())
				}
			} else {
				legacyIDs = append(legacyIDs, entry.Task.ID)
			}
		}
		if len(legacyIDs) > 0 {
			_ = model.TaskBulkUpdateByID(legacyIDs, map[string]any{"fail_reason": fmt.Sprintf("获取渠道信息失败，请联系管理员，渠道ID：%d", channelID), "status": model.TaskStatusFailure, "progress": "100%"})
		}
		return err
	}
	adaptor := GetTaskAdaptorFunc(constant.TaskPlatformSuno)
	if adaptor == nil {
		return errors.New("adaptor not found")
	}
	resp, err := adaptor.FetchTask(ch.GetBaseURL(), ch.Key, map[string]any{"ids": taskIDs}, ch.GetSetting().Proxy)
	if err != nil {
		markPollingEntries(ctx, entries, taskPollingDispositionRetryable, "suno_fetch_failed")
		return err
	}
	if resp == nil {
		markPollingEntries(ctx, entries, taskPollingDispositionRetryable, "suno_empty_response")
		return errors.New("Suno polling returned nil response")
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		markPollingEntries(ctx, entries, pollingDispositionForHTTPStatus(resp.StatusCode), pollingReasonForHTTPStatus("suno", resp.StatusCode))
		return fmt.Errorf("Get Task status code: %d", resp.StatusCode)
	}
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		markPollingEntries(ctx, entries, taskPollingDispositionRetryable, "suno_read_failed")
		return err
	}
	var responseItems taskdto.TaskResponse[[]taskdto.SunoDataResponse]
	if err := common.Unmarshal(responseBody, &responseItems); err != nil {
		markPollingEntries(ctx, entries, taskPollingDispositionRetryable, "suno_json_invalid")
		return err
	}
	if !responseItems.IsSuccess() {
		markPollingEntries(ctx, entries, taskPollingDispositionRetryable, "suno_error_envelope")
		return fmt.Errorf("Suno batch response rejected with code %s", responseItems.Code)
	}

	responseCounts := make(map[string]int, len(responseItems.Data))
	for _, item := range responseItems.Data {
		responseCounts[strings.TrimSpace(item.TaskID)]++
	}
	seen := make(map[string]struct{}, len(responseItems.Data))
	for _, item := range responseItems.Data {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		upstreamID := strings.TrimSpace(item.TaskID)
		key := taskPollingKey{ChannelID: channelID, UpstreamID: upstreamID}
		entry := batch.entries[key]
		if entry == nil {
			logger.LogWarn(ctx, fmt.Sprintf("Suno task response ignored: unknown task_id=%s channel=%d", upstreamID, channelID))
			continue
		}
		seen[upstreamID] = struct{}{}
		if responseCounts[upstreamID] > 1 {
			_ = recordTaskPollingUncertainty(ctx, entry, taskPollingDispositionManual, "suno_duplicate_task_id")
			continue
		}
		if entry.Task.ChannelId != channelID || strings.TrimSpace(entry.Task.GetUpstreamTaskID()) != upstreamID {
			_ = recordTaskPollingUncertainty(ctx, entry, taskPollingDispositionManual, "suno_task_identity_mismatch")
			continue
		}
		if err := applySunoPollingItem(ctx, adaptor, entry, item); err != nil {
			logger.LogError(ctx, fmt.Sprintf("apply Suno polling item for task %s: %v", entry.Task.TaskID, err))
		}
	}
	for _, entry := range entries {
		if _, ok := seen[strings.TrimSpace(entry.Task.GetUpstreamTaskID())]; !ok {
			_ = recordTaskPollingUncertainty(ctx, entry, taskPollingDispositionRetryable, "suno_task_missing")
		}
	}
	return nil
}

func applySunoPollingItem(ctx context.Context, adaptor TaskPollingAdaptor, entry *taskPollingEntry, item taskdto.SunoDataResponse) error {
	task := entry.Task
	status := model.TaskStatus(strings.ToUpper(strings.TrimSpace(item.Status)))
	if entry.Context.Durable() && !knownTaskPollingStatus(status) {
		return recordTaskPollingUncertainty(ctx, entry, taskPollingDispositionRetryable, "suno_status_unknown")
	}
	if !taskNeedsUpdate(task, item) {
		return clearTaskPollingUncertainty(entry)
	}
	previousStatus := task.Status
	task.Status = lo.If(status != "", status).Else(task.Status)
	task.FailReason = lo.If(item.FailReason != "", item.FailReason).Else(task.FailReason)
	task.SubmitTime = lo.If(item.SubmitTime != 0, item.SubmitTime).Else(task.SubmitTime)
	task.StartTime = lo.If(item.StartTime != 0, item.StartTime).Else(task.StartTime)
	task.FinishTime = lo.If(item.FinishTime != 0, item.FinishTime).Else(task.FinishTime)
	task.Data = redactVideoResponseBody(item.Data)

	isFailure := status == model.TaskStatusFailure
	if !entry.Context.Durable() && item.FailReason != "" {
		isFailure = true
	}
	isSuccess := status == model.TaskStatusSuccess
	if isFailure || isSuccess {
		task.Progress = taskcommon.ProgressComplete
	}
	if entry.Context.Durable() && (isFailure || isSuccess) {
		evidence := terminalEvidenceFromSunoItem(item)
		if isFailure {
			_, err := durableReleaseTaskOnFailureUsingContext(ctx, entry.Context, task, task.FailReason, evidence)
			return normalizeManualReviewError(err)
		}
		taskInfo := &relaycommon.TaskInfo{TaskID: strings.TrimSpace(item.TaskID), Status: string(status)}
		return normalizeManualReviewError(settleTaskBillingOnCompleteWithEvidence(ctx, adaptor, entry.Context, task, taskInfo, evidence))
	}

	won, err := task.UpdateWithStatus(previousStatus)
	if err != nil {
		return err
	}
	if !won {
		return reloadTaskAfterPollingCAS(task)
	}
	if err := clearTaskPollingUncertainty(entry); err != nil {
		return err
	}
	if isFailure && previousStatus != model.TaskStatusFailure && task.Quota != 0 {
		RefundTaskQuota(ctx, task, task.FailReason)
	}
	return nil
}

func markPollingEntries(ctx context.Context, entries []*taskPollingEntry, disposition taskPollingDisposition, reason string) {
	for _, entry := range entries {
		if err := recordTaskPollingUncertainty(ctx, entry, disposition, reason); err != nil {
			logger.LogError(ctx, fmt.Sprintf("record polling uncertainty for task %s: %v", entry.Task.TaskID, err))
		}
	}
}

// taskNeedsUpdate 检查 Suno 任务是否需要更新
func taskNeedsUpdate(oldTask *model.Task, newTask taskdto.SunoDataResponse) bool {
	if oldTask.SubmitTime != newTask.SubmitTime {
		return true
	}
	if oldTask.StartTime != newTask.StartTime {
		return true
	}
	if oldTask.FinishTime != newTask.FinishTime {
		return true
	}
	if string(oldTask.Status) != newTask.Status {
		return true
	}
	if oldTask.FailReason != newTask.FailReason {
		return true
	}

	if (oldTask.Status == model.TaskStatusFailure || oldTask.Status == model.TaskStatusSuccess) && oldTask.Progress != "100%" {
		return true
	}

	oldData, _ := common.Marshal(oldTask.Data)
	newData, _ := common.Marshal(newTask.Data)

	sort.Slice(oldData, func(i, j int) bool {
		return oldData[i] < oldData[j]
	})
	sort.Slice(newData, func(i, j int) bool {
		return newData[i] < newData[j]
	})

	if string(oldData) != string(newData) {
		return true
	}
	return false
}

// UpdateVideoTasks is retained for direct callers. The production polling pass
// uses updateVideoPollingBatch so identical upstream IDs remain channel-scoped.
func UpdateVideoTasks(ctx context.Context, platform constant.TaskPlatform, taskChannelM map[int][]string, taskM map[string]*model.Task) error {
	tasks := make([]*model.Task, 0, len(taskM))
	for _, task := range taskM {
		tasks = append(tasks, task)
	}
	contexts, err := LoadTaskPollingContexts(model.DB, tasks)
	if err != nil {
		return err
	}
	batch, err := pollingBatchFromLegacyMaps(taskChannelM, taskM, contexts)
	if err != nil {
		return err
	}
	return updateVideoPollingBatch(ctx, platform, batch)
}

func updateVideoPollingBatch(ctx context.Context, platform constant.TaskPlatform, batch *taskPollingBatch) error {
	channelIDs := make([]int, 0, len(batch.channelTasks))
	for channelID := range batch.channelTasks {
		channelIDs = append(channelIDs, channelID)
	}
	sort.Ints(channelIDs)

	var wg sync.WaitGroup
	for _, channelID := range channelIDs {
		keys := append([]taskPollingKey(nil), batch.channelTasks[channelID]...)
		if len(keys) == 0 {
			continue
		}
		wg.Add(1)
		gopool.Go(func() {
			defer wg.Done()
			if err := updateVideoChannelTasks(ctx, platform, channelID, keys, batch); err != nil {
				logger.LogError(ctx, fmt.Sprintf("Channel #%d failed to update video async tasks: %s", channelID, err.Error()))
			}
		})
	}
	wg.Wait()
	return ctx.Err()
}

func updateVideoChannelTasks(ctx context.Context, platform constant.TaskPlatform, channelID int, keys []taskPollingKey, batch *taskPollingBatch) error {
	logger.LogInfo(ctx, fmt.Sprintf("Channel #%d pending video tasks: %d", channelID, len(keys)))
	if ctx.Err() != nil || len(keys) == 0 {
		return ctx.Err()
	}
	entries := make([]*taskPollingEntry, 0, len(keys))
	for _, key := range keys {
		entry := batch.entries[key]
		if entry == nil || entry.Task == nil || key.ChannelID != channelID || entry.Task.ChannelId != channelID || strings.TrimSpace(entry.Task.GetUpstreamTaskID()) != key.UpstreamID {
			return errors.New("video polling channel/task mismatch")
		}
		entries = append(entries, entry)
	}
	ch, err := model.CacheGetChannel(channelID)
	if err != nil {
		legacyIDs := make([]int64, 0, len(entries))
		for _, entry := range entries {
			if entry.Context.Durable() {
				_ = recordTaskPollingUncertainty(ctx, entry, taskPollingDispositionRetryable, "video_channel_unavailable")
			} else {
				legacyIDs = append(legacyIDs, entry.Task.ID)
			}
		}
		if len(legacyIDs) > 0 {
			_ = model.TaskBulkUpdateByID(legacyIDs, map[string]any{"fail_reason": fmt.Sprintf("Failed to get channel info, channel ID: %d", channelID), "status": model.TaskStatusFailure, "progress": "100%"})
		}
		return fmt.Errorf("CacheGetChannel failed: %w", err)
	}
	adaptor := GetTaskAdaptorFunc(platform)
	if adaptor == nil {
		markPollingEntries(ctx, entries, taskPollingDispositionManual, "video_adaptor_unavailable")
		return errors.New("video adaptor not found")
	}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: ch.GetBaseURL()}}
	info.ApiKey = ch.Key
	adaptor.Init(info)
	disablePollingSleep := ch.GetOtherSettings().DisableTaskPollingSleep
	for i, key := range keys {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := updateVideoPollingEntry(ctx, adaptor, ch, key, batch.entries[key]); err != nil {
			logger.LogError(ctx, fmt.Sprintf("Failed to update video task %s: %s", key.UpstreamID, err.Error()))
		}
		if disablePollingSleep || i == len(keys)-1 {
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return nil
}

func updateVideoTasks(ctx context.Context, platform constant.TaskPlatform, channelID int, taskIDs []string, taskM map[string]*model.Task) error {
	return UpdateVideoTasks(ctx, platform, map[int][]string{channelID: taskIDs}, taskM)
}

func updateVideoSingleTask(ctx context.Context, adaptor TaskPollingAdaptor, ch *model.Channel, taskID string, taskM map[string]*model.Task) error {
	if ch == nil {
		return errors.New("video polling channel is nil")
	}
	task := taskM[taskID]
	if task == nil {
		return fmt.Errorf("task %s not found", taskID)
	}
	if task.ChannelId != ch.Id || strings.TrimSpace(task.GetUpstreamTaskID()) != strings.TrimSpace(taskID) {
		return errors.New("video polling direct task identity mismatch")
	}
	pollingContext, err := LoadTaskPollingContext(model.DB, task)
	if err != nil {
		return err
	}
	key := taskPollingKey{ChannelID: ch.Id, UpstreamID: strings.TrimSpace(taskID)}
	return updateVideoPollingEntry(ctx, adaptor, ch, key, &taskPollingEntry{Task: task, Context: pollingContext})
}

func updateVideoPollingEntry(ctx context.Context, adaptor TaskPollingAdaptor, ch *model.Channel, key taskPollingKey, entry *taskPollingEntry) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if entry == nil || entry.Task == nil || entry.Task.ChannelId != key.ChannelID || ch == nil || ch.Id != key.ChannelID {
		return errors.New("video polling entry identity mismatch")
	}
	task := entry.Task
	if strings.TrimSpace(task.GetUpstreamTaskID()) != key.UpstreamID {
		return recordTaskPollingUncertainty(ctx, entry, taskPollingDispositionManual, "video_task_identity_mismatch")
	}
	if entry.Context.ValidationErr != nil {
		return recordTaskPollingUncertainty(ctx, entry, taskPollingDispositionManual, "polling_context_invalid")
	}
	baseURL := constant.ChannelBaseURLs[ch.Type]
	if ch.GetBaseURL() != "" {
		baseURL = ch.GetBaseURL()
	}
	apiKey := ch.Key
	if task.PrivateData.Key != "" {
		apiKey = task.PrivateData.Key
	}
	resp, err := adaptor.FetchTask(baseURL, apiKey, map[string]any{"task_id": key.UpstreamID, "action": task.Action}, ch.GetSetting().Proxy)
	if err != nil {
		_ = recordTaskPollingUncertainty(ctx, entry, taskPollingDispositionRetryable, "video_fetch_failed")
		return fmt.Errorf("fetchTask failed for task %s: %w", key.UpstreamID, err)
	}
	return applyVideoPollingHTTPResponse(ctx, adaptor, ch, key, entry, resp)
}

// ApplyRealtimeTaskPollingResponse applies one already-fetched provider response
// through the same durable/legacy validation and accounting boundary as the
// background poller.
func ApplyRealtimeTaskPollingResponse(ctx context.Context, adaptor TaskPollingAdaptor, ch *model.Channel, task *model.Task, resp *http.Response) error {
	if ch == nil || task == nil || task.ChannelId != ch.Id {
		return errors.New("realtime task polling identity mismatch")
	}
	pollingContext, err := LoadTaskPollingContext(model.DB, task)
	if err != nil {
		return err
	}
	key := taskPollingKey{ChannelID: ch.Id, UpstreamID: strings.TrimSpace(task.GetUpstreamTaskID())}
	return applyVideoPollingHTTPResponse(ctx, adaptor, ch, key, &taskPollingEntry{Task: task, Context: pollingContext}, resp)
}

func applyVideoPollingHTTPResponse(ctx context.Context, adaptor TaskPollingAdaptor, ch *model.Channel, key taskPollingKey, entry *taskPollingEntry, resp *http.Response) error {
	if entry == nil || entry.Task == nil || entry.Task.ChannelId != key.ChannelID || ch == nil || ch.Id != key.ChannelID {
		return errors.New("video polling response identity mismatch")
	}
	task := entry.Task
	if resp == nil {
		deferVideoPollingResponse(ctx, entry, taskPollingDispositionRetryable, "video_empty_response")
		return errors.New("video polling returned nil response")
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		deferVideoPollingResponse(ctx, entry, pollingDispositionForHTTPStatus(resp.StatusCode), pollingReasonForHTTPStatus("video", resp.StatusCode))
		return fmt.Errorf("video polling unexpected HTTP status %d", resp.StatusCode)
	}
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		deferVideoPollingResponse(ctx, entry, taskPollingDispositionRetryable, "video_read_failed")
		return fmt.Errorf("readAll failed for task %s: %w", key.UpstreamID, err)
	}
	if err := validateTaskLevelResponseEnvelope(responseBody); err != nil {
		deferVideoPollingResponse(ctx, entry, taskPollingDispositionRetryable, "video_error_envelope")
		return err
	}

	parsed, err := parseVideoTaskResult(adaptor, responseBody)
	if err != nil || parsed == nil || parsed.Info == nil {
		deferVideoPollingResponse(ctx, entry, taskPollingDispositionRetryable, "video_task_schema_invalid")
		if err == nil {
			err = errors.New("video task parser returned nil result")
		}
		return err
	}
	taskResult := parsed.Info
	status := model.TaskStatus(strings.ToUpper(strings.TrimSpace(taskResult.Status)))
	if !knownTaskPollingStatus(status) {
		deferVideoPollingResponse(ctx, entry, taskPollingDispositionRetryable, "video_status_unknown")
		return fmt.Errorf("unknown task status %q", taskResult.Status)
	}
	responseTaskID := strings.TrimSpace(taskResult.TaskID)
	if responseTaskID == "" {
		responseTaskID = taskLevelResponseID(responseBody)
	}
	if responseTaskID != "" && responseTaskID != key.UpstreamID {
		deferVideoPollingResponse(ctx, entry, taskPollingDispositionManual, "video_response_id_mismatch")
		return errors.New("video polling response task id mismatch")
	}
	isTerminal := status == model.TaskStatusSuccess || status == model.TaskStatusFailure
	if isTerminal && responseTaskID == "" && !taskPollingAllowsRequestScopedEvidence(ch.Type) {
		deferVideoPollingResponse(ctx, entry, taskPollingDispositionManual, "video_response_id_missing")
		return errors.New("video polling terminal response omitted task id")
	}

	previous := task.Snapshot()
	applyVideoTaskProjection(task, parsed, responseBody, status)
	if entry.Context == nil || !entry.Context.Durable() {
		return applyLegacyVideoPollingResult(ctx, adaptor, task, taskResult, previous.Status, !previous.Equal(task.Snapshot()), isTerminal)
	}
	if isTerminal {
		evidence := terminalEvidenceFromTaskInfo(taskResult, key.UpstreamID)
		if status == model.TaskStatusFailure {
			_, err := durableReleaseTaskOnFailureUsingContext(ctx, entry.Context, task, task.FailReason, evidence)
			return normalizeManualReviewError(err)
		}
		return normalizeManualReviewError(settleTaskBillingOnCompleteWithEvidence(ctx, adaptor, entry.Context, task, taskResult, evidence))
	}
	if previous.Equal(task.Snapshot()) {
		return clearTaskPollingUncertainty(entry)
	}
	won, err := task.UpdateWithStatus(previous.Status)
	if err != nil {
		return err
	}
	if !won {
		return reloadTaskAfterPollingCAS(task)
	}
	return clearTaskPollingUncertainty(entry)
}

func deferVideoPollingResponse(ctx context.Context, entry *taskPollingEntry, disposition taskPollingDisposition, reason string) {
	if entry != nil && entry.Context != nil && entry.Context.Durable() {
		_ = recordTaskPollingUncertainty(ctx, entry, disposition, reason)
	}
}

type parsedVideoTaskResult struct {
	Info       *relaycommon.TaskInfo
	Compatible bool
	TaskData   []byte
	StartTime  int64
	FinishTime int64
}

func parseVideoTaskResult(adaptor TaskPollingAdaptor, responseBody []byte) (*parsedVideoTaskResult, error) {
	var responseItems taskdto.TaskResponse[model.Task]
	if err := common.Unmarshal(responseBody, &responseItems); err == nil && responseItems.IsSuccess() {
		task := responseItems.Data
		if task.StartTime < 0 || task.FinishTime < 0 || (task.StartTime > 0 && task.FinishTime > 0 && task.FinishTime < task.StartTime) {
			return nil, errors.New("compatible video task returned invalid timestamps")
		}
		return &parsedVideoTaskResult{
			Info:       &relaycommon.TaskInfo{TaskID: task.TaskID, Status: string(task.Status), Url: task.GetResultURL(), Progress: task.Progress, Reason: task.FailReason},
			Compatible: true,
			TaskData:   redactVideoResponseBody(task.Data),
			StartTime:  task.StartTime,
			FinishTime: task.FinishTime,
		}, nil
	}
	taskResult, err := adaptor.ParseTaskResult(responseBody)
	if err != nil {
		return nil, err
	}
	return &parsedVideoTaskResult{Info: taskResult}, nil
}

func applyVideoTaskProjection(task *model.Task, parsed *parsedVideoTaskResult, responseBody []byte, status model.TaskStatus) {
	if parsed.Compatible {
		if parsed.StartTime > 0 {
			task.StartTime = parsed.StartTime
		}
		if parsed.FinishTime > 0 {
			task.FinishTime = parsed.FinishTime
		}
		task.Data = parsed.TaskData
	} else {
		task.Data = redactVideoResponseBody(responseBody)
	}
	now := time.Now().Unix()
	task.Status = status
	switch status {
	case model.TaskStatusSubmitted:
		task.Progress = taskcommon.ProgressSubmitted
	case model.TaskStatusQueued, model.TaskStatusNotStart:
		task.Progress = taskcommon.ProgressQueued
	case model.TaskStatusInProgress:
		task.Progress = taskcommon.ProgressInProgress
		if task.StartTime == 0 {
			task.StartTime = now
		}
	case model.TaskStatusSuccess:
		task.Progress = taskcommon.ProgressComplete
		if task.FinishTime == 0 {
			task.FinishTime = now
		}
		if strings.HasPrefix(parsed.Info.Url, "data:") {
			task.PrivateData.ResultURL = taskcommon.BuildProxyURL(task.TaskID)
		} else if parsed.Info.Url != "" {
			task.PrivateData.ResultURL = parsed.Info.Url
		} else {
			task.PrivateData.ResultURL = taskcommon.BuildProxyURL(task.TaskID)
		}
	case model.TaskStatusFailure:
		task.Progress = taskcommon.ProgressComplete
		if task.FinishTime == 0 {
			task.FinishTime = now
		}
		task.FailReason = parsed.Info.Reason
	}
	if parsed.Info.Progress != "" {
		task.Progress = parsed.Info.Progress
	}
}

func applyLegacyVideoPollingResult(ctx context.Context, adaptor TaskPollingAdaptor, task *model.Task, taskResult *relaycommon.TaskInfo, previousStatus model.TaskStatus, changed bool, isTerminal bool) error {
	shouldSettle := task.Status == model.TaskStatusSuccess
	shouldRefund := task.Status == model.TaskStatusFailure && task.Quota != 0
	if isTerminal && previousStatus != task.Status {
		won, err := task.UpdateWithStatus(previousStatus)
		if err != nil {
			return err
		}
		if !won {
			return reloadTaskAfterPollingCAS(task)
		}
	} else if changed {
		won, err := task.UpdateWithStatus(previousStatus)
		if err != nil {
			return err
		}
		if !won {
			return reloadTaskAfterPollingCAS(task)
		}
	}
	if shouldSettle {
		settleTaskBillingOnComplete(ctx, adaptor, task, taskResult)
	}
	if shouldRefund {
		RefundTaskQuota(ctx, task, task.FailReason)
	}
	return nil
}

func reloadTaskAfterPollingCAS(task *model.Task) error {
	if model.DB == nil || task == nil || task.ID <= 0 {
		return errors.New("cannot reload task after polling CAS loss")
	}
	var current model.Task
	if err := model.DB.First(&current, task.ID).Error; err != nil {
		return err
	}
	*task = current
	return nil
}

func validateTaskLevelResponseEnvelope(responseBody []byte) error {
	var envelope map[string]interface{}
	if err := common.Unmarshal(responseBody, &envelope); err != nil {
		return fmt.Errorf("video polling response is not valid JSON: %w", err)
	}
	if errorValue, ok := envelope["error"]; ok && nonEmptyErrorEnvelopeValue(errorValue) && !isTaskLevelResponseEnvelope(envelope) {
		return errors.New("video polling returned an error envelope")
	}
	if success, ok := envelope["success"].(bool); ok && !success {
		return errors.New("video polling returned success=false")
	}
	if code, ok := envelope["code"].(float64); ok && code >= 400 {
		return errors.New("video polling returned an error code")
	}
	if code, ok := envelope["code"].(string); ok {
		normalized := strings.ToLower(strings.TrimSpace(code))
		if normalized == "error" || normalized == "failed" || normalized == "failure" {
			return errors.New("video polling returned an error code")
		}
	}
	return nil
}

func isTaskLevelResponseEnvelope(envelope map[string]interface{}) bool {
	_, hasStatus := envelope["status"]
	_, hasDone := envelope["done"].(bool)
	if name, ok := envelope["name"].(string); ok && strings.TrimSpace(name) != "" && (hasDone || hasStatus) {
		return true
	}
	if !hasStatus {
		return false
	}
	for _, key := range []string{"task_id", "id"} {
		if value, ok := envelope[key].(string); ok && strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

func taskLevelResponseID(responseBody []byte) string {
	var envelope map[string]interface{}
	if err := common.Unmarshal(responseBody, &envelope); err != nil {
		return ""
	}
	return taskLevelResponseIDFromEnvelope(envelope)
}

func taskLevelResponseIDFromEnvelope(envelope map[string]interface{}) string {
	for _, key := range []string{"task_id", "id"} {
		if value, ok := envelope[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	name, _ := envelope["name"].(string)
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	_, hasDone := envelope["done"].(bool)
	_, hasStatus := envelope["status"]
	if !hasDone && !hasStatus {
		return ""
	}
	return taskcommon.EncodeLocalTaskID(name)
}

func nonEmptyErrorEnvelopeValue(value interface{}) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(typed) != ""
	case map[string]interface{}:
		return len(typed) != 0
	case []interface{}:
		return len(typed) != 0
	default:
		return true
	}
}

func knownTaskPollingStatus(status model.TaskStatus) bool {
	switch status {
	case model.TaskStatusNotStart, model.TaskStatusSubmitted, model.TaskStatusQueued, model.TaskStatusInProgress, model.TaskStatusSuccess, model.TaskStatusFailure:
		return true
	default:
		return false
	}
}

func taskPollingAllowsRequestScopedEvidence(channelType int) bool {
	switch channelType {
	case constant.ChannelTypeAli, constant.ChannelTypeGemini, constant.ChannelTypeVertexAi, constant.ChannelTypeMiniMax,
		constant.ChannelTypeJimeng, constant.ChannelTypeVidu, constant.ChannelTypeDoubaoVideo, constant.ChannelTypeSora:
		return true
	default:
		return false
	}
}

func pollingDispositionForHTTPStatus(status int) taskPollingDisposition {
	if status == http.StatusUnauthorized || status == http.StatusForbidden || status == http.StatusNotFound {
		return taskPollingDispositionManual
	}
	return taskPollingDispositionRetryable
}

func pollingReasonForHTTPStatus(prefix string, status int) string {
	switch status {
	case http.StatusUnauthorized:
		return prefix + "_http_401"
	case http.StatusForbidden:
		return prefix + "_http_403"
	case http.StatusNotFound:
		return prefix + "_http_404"
	default:
		if status >= 500 {
			return prefix + "_http_5xx"
		}
		return prefix + "_http_non_2xx"
	}
}

func normalizeManualReviewError(err error) error {
	if errors.Is(err, model.ErrTaskTerminalObservationManualReview) {
		return nil
	}
	return err
}

func redactVideoResponseBody(body []byte) []byte {
	var value interface{}
	if err := common.Unmarshal(body, &value); err != nil {
		return nil
	}
	redactVideoResponseValue(value)
	b, err := common.Marshal(value)
	if err != nil || len(b) > 60*1024 {
		return nil
	}
	return b
}

func redactVideoResponseValue(value interface{}) {
	switch current := value.(type) {
	case map[string]interface{}:
		for key, child := range current {
			normalizedKey := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(key), "-", "_"))
			switch normalizedKey {
			case "prompt", "input", "request", "request_body", "body", "key", "api_key", "authorization", "token", "access_token", "secret", "credentials", "headers", "reason", "fail_reason", "error", "message", "bytesbase64encoded", "bytes_base64_encoded":
				delete(current, key)
				continue
			case "video":
				if encoded, ok := child.(string); ok && len(encoded) > 256 {
					current[key] = truncateBase64(encoded)
					continue
				}
			}
			redactVideoResponseValue(child)
		}
	case []interface{}:
		for _, child := range current {
			redactVideoResponseValue(child)
		}
	}
}

func truncateBase64(s string) string {
	const maxKeep = 256
	if len(s) <= maxKeep {
		return s
	}
	return s[:maxKeep] + "..."
}

// settleTaskBillingOnComplete 任务完成时的统一计费调整。
// 优先级：1. adaptor.AdjustBillingOnComplete 返回正数 → 使用 adaptor 计算的额度
//
//  2. taskResult.TotalTokens > 0 → 按 token 重算
//  3. 都不满足 → 保持预扣额度不变
func settleTaskBillingOnComplete(ctx context.Context, adaptor TaskPollingAdaptor, task *model.Task, taskResult *relaycommon.TaskInfo) {
	if auditInvalidTaskBillingSnapshot(ctx, task, taskResult.QuotaClamp) {
		return
	}
	// 0. 按次计费的任务不做差额结算
	if bc := task.PrivateData.BillingContext; bc != nil && bc.PerCallBilling {
		logger.LogInfo(ctx, fmt.Sprintf("任务 %s 按次计费，跳过差额结算", task.TaskID))
		if handled, err := DurableSettleTaskOnComplete(ctx, task, task.Quota, "per_call_billing", taskResult.QuotaClamp); handled {
			if err != nil {
				logger.LogError(ctx, fmt.Sprintf("durable settlement failed for task %s: %v", task.TaskID, err))
			}
			return
		}
		if taskResult.QuotaClamp != nil {
			RecalculateTaskQuota(ctx, task, task.Quota, "按次计费，保持预扣额度", taskResult.QuotaClamp)
		}
		return
	}
	// 1. 优先让 adaptor 决定最终额度
	if actualQuota := adaptor.AdjustBillingOnComplete(task, taskResult); actualQuota > 0 {
		if handled, err := DurableSettleTaskOnComplete(ctx, task, actualQuota, "adaptor计费调整", taskResult.QuotaClamp); handled {
			if err != nil {
				logger.LogError(ctx, fmt.Sprintf("durable settlement failed for task %s: %v", task.TaskID, err))
			}
			return
		}
		RecalculateTaskQuota(ctx, task, actualQuota, "adaptor计费调整", taskResult.QuotaClamp)
		return
	}
	// 2. 回退到 token 重算
	if taskResult.TotalTokens > 0 {
		if computedQuota, tokenClamp, ok := computeTaskQuotaFromTokens(task, taskResult.TotalTokens); ok {
			if handled, err := DurableSettleTaskOnComplete(ctx, task, computedQuota, fmt.Sprintf("token重算:tokens=%d", taskResult.TotalTokens), taskResult.QuotaClamp, tokenClamp); handled {
				if err != nil {
					logger.LogError(ctx, fmt.Sprintf("durable settlement failed for task %s: %v", task.TaskID, err))
				}
				return
			}
		}
		RecalculateTaskQuotaByTokens(ctx, task, taskResult.TotalTokens, taskResult.QuotaClamp)
		return
	}
	// 3. 无调整，保持预扣额度
	if handled, err := DurableSettleTaskOnComplete(ctx, task, task.Quota, "保持预扣额度", taskResult.QuotaClamp); handled {
		if err != nil {
			logger.LogError(ctx, fmt.Sprintf("durable settlement failed for task %s: %v", task.TaskID, err))
		}
		return
	}
	if taskResult.QuotaClamp != nil {
		RecalculateTaskQuota(ctx, task, task.Quota, "上游计费用量不可用，保持预扣额度", taskResult.QuotaClamp)
	}
}

func terminalEvidenceFromTaskInfo(info *relaycommon.TaskInfo, upstreamTaskID string) DurableTerminalEvidence {
	if info == nil {
		return DurableTerminalEvidence{}
	}
	payload := struct {
		Version          int    `json:"version"`
		UpstreamTaskID   string `json:"upstream_task_id"`
		Status           string `json:"status"`
		CompletionTokens int    `json:"completion_tokens"`
		TotalTokens      int    `json:"total_tokens"`
		ResultURL        string `json:"result_url"`
		ReasonCode       string `json:"reason_code"`
	}{1, strings.TrimSpace(upstreamTaskID), strings.ToLower(strings.TrimSpace(info.Status)), info.CompletionTokens, info.TotalTokens, sanitizeTerminalResultURL(info.Url), sanitizeReasonCode(info.Reason, "provider_terminal")}
	data, err := common.Marshal(payload)
	if err != nil {
		return DurableTerminalEvidence{}
	}
	digest := sha256.Sum256(data)
	return DurableTerminalEvidence{ID: strings.TrimSpace(upstreamTaskID), Hash: hex.EncodeToString(digest[:]), Version: 1}
}

func terminalEvidenceFromSunoItem(item taskdto.SunoDataResponse) DurableTerminalEvidence {
	payload := struct {
		Version    int    `json:"version"`
		TaskID     string `json:"task_id"`
		Status     string `json:"status"`
		FinishTime int64  `json:"finish_time"`
		ReasonCode string `json:"reason_code"`
	}{1, strings.TrimSpace(item.TaskID), strings.ToLower(strings.TrimSpace(item.Status)), item.FinishTime, sanitizeReasonCode(item.FailReason, "provider_terminal")}
	data, err := common.Marshal(payload)
	if err != nil {
		return DurableTerminalEvidence{}
	}
	digest := sha256.Sum256(data)
	return DurableTerminalEvidence{ID: strings.TrimSpace(item.TaskID), Hash: hex.EncodeToString(digest[:]), Version: 1}
}

func settleTaskBillingOnCompleteWithEvidence(ctx context.Context, adaptor TaskPollingAdaptor, pollingContext *TaskPollingContext, task *model.Task, taskResult *relaycommon.TaskInfo, evidence DurableTerminalEvidence) error {
	if pollingContext == nil || !pollingContext.Durable() {
		settleTaskBillingOnComplete(ctx, adaptor, task, taskResult)
		return nil
	}
	if pollingContext.ValidationErr != nil {
		_, err := durableTerminalManualReviewUsingContext(ctx, pollingContext, task, "succeeded", 0, "polling_context_invalid", evidence)
		return err
	}
	if taskResult.QuotaClamp != nil {
		_, err := durableTerminalManualReviewUsingContext(ctx, pollingContext, task, "succeeded", 0, "quota_clamp_manual_review", evidence, taskResult.QuotaClamp)
		return err
	}
	billingContext, err := immutableTaskBillingContext(pollingContext)
	if err != nil {
		_, manualErr := durableTerminalManualReviewUsingContext(ctx, pollingContext, task, "succeeded", 0, "billing_snapshot_invalid", evidence)
		return manualErr
	}
	if billingContext.PerCallBilling {
		actualQuota, quotaErr := immutableTaskInitialQuota(pollingContext)
		if quotaErr != nil {
			_, manualErr := durableTerminalManualReviewUsingContext(ctx, pollingContext, task, "succeeded", 0, "per_call_quota_invalid", evidence)
			return manualErr
		}
		_, err := durableSettleTaskOnCompleteUsingContext(ctx, pollingContext, task, actualQuota, "per_call_billing", evidence)
		return err
	}
	if actual := adaptor.AdjustBillingOnComplete(task, taskResult); actual > 0 {
		_, err := durableSettleTaskOnCompleteUsingContext(ctx, pollingContext, task, actual, "adaptor_billing_adjustment", evidence)
		return err
	}
	if taskResult.TotalTokens > 0 {
		if actual, clamp, ok := computeTaskQuotaFromTokensWithContext(task, pollingContext, taskResult.TotalTokens); ok {
			_, err := durableSettleTaskOnCompleteUsingContext(ctx, pollingContext, task, actual, "token_billing_recalculation", evidence, clamp)
			return err
		}
		_, err := durableTerminalManualReviewUsingContext(ctx, pollingContext, task, "succeeded", 0, "token_billing_rates_unavailable", evidence)
		return err
	}
	_, err = durableTerminalManualReviewUsingContext(ctx, pollingContext, task, "succeeded", 0, "terminal_actual_evidence_missing", evidence)
	return err
}
