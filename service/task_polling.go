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
	"github.com/ForceMind/MyAPI/relaykit/dto"

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

// sweepTimedOutTasks 在主轮询之前独立清理超时任务。
// 每次最多处理 100 条，剩余的下个周期继续处理。
// 使用 per-task CAS (UpdateWithStatus) 防止覆盖被正常轮询已推进的任务。
func sweepTimedOutTasks(ctx context.Context) {
	if constant.TaskTimeoutMinutes <= 0 {
		return
	}
	cutoff := time.Now().Unix() - int64(constant.TaskTimeoutMinutes)*60
	tasks := model.GetTimedOutUnfinishedTasks(cutoff, 100)
	if len(tasks) == 0 {
		return
	}

	reason := fmt.Sprintf("任务超时（%d分钟）", constant.TaskTimeoutMinutes)
	legacyReason := "任务超时（旧系统遗留任务，不进行退款，请联系管理员）"
	now := time.Now().Unix()
	timedOutCount := 0

	for _, task := range tasks {
		operation, operationErr := durableOperationForTask(task)
		if operationErr != nil {
			logger.LogError(ctx, fmt.Sprintf("resolve durable timeout task %s: %v", task.TaskID, operationErr))
			continue
		}
		if operation != nil {
			handled, terminalErr := DurableReleaseTaskOnFailure(ctx, task, "task_timeout_unverified")
			if handled {
				timedOutCount++
				if terminalErr != nil && !errors.Is(terminalErr, model.ErrTaskTerminalObservationManualReview) {
					logger.LogError(ctx, fmt.Sprintf("record timeout observation for task %s: %v", task.TaskID, terminalErr))
				}
				continue
			}
		}
		isLegacy := task.SubmitTime > 0 && task.SubmitTime < model.TaskRefundLegacyCutoff

		oldStatus := task.Status
		task.Status = model.TaskStatusFailure
		task.Progress = "100%"
		task.FinishTime = now
		if isLegacy {
			task.FailReason = legacyReason
			// 旧系统任务明确不退款，随终态 CAS 一并清掉 quota，
			// 避免留下可再次退款的计费状态。
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
			logger.LogInfo(ctx, fmt.Sprintf("sweepTimedOutTasks: task %s already transitioned, skip", task.TaskID))
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

// RunTaskPollingOnce performs one async-task (Suno/video) polling pass
// synchronously. It honors ctx cancellation (the system-task runner cancels it
// when the lease is lost) and, when report is non-nil, reports progress as
// (processedPlatforms, totalPlatforms). It returns immediately if the task
// adaptor factory has not been wired yet, to avoid a nil call during startup.
func RunTaskPollingOnce(ctx context.Context, report func(processed, total int)) TaskPollSummary {
	summary := TaskPollSummary{}
	if GetTaskAdaptorFunc == nil {
		return summary
	}
	if ctx == nil {
		ctx = context.Background()
	}

	common.SysLog("任务进度轮询开始")
	sweepTimedOutTasks(ctx)
	allTasks := model.GetAllUnFinishSyncTasks(constant.TaskQueryLimit)
	summary.UnfinishedTasks = len(allTasks)
	platformTask := make(map[constant.TaskPlatform][]*model.Task)
	for _, t := range allTasks {
		platformTask[t.Platform] = append(platformTask[t.Platform], t)
	}

	totalPlatforms := len(platformTask)
	processedPlatforms := 0
	for platform, tasks := range platformTask {
		if ctx.Err() != nil {
			break
		}
		if report != nil {
			report(processedPlatforms, totalPlatforms)
		}
		processedPlatforms++
		if len(tasks) == 0 {
			continue
		}
		summary.PlatformsScanned++
		taskChannelM := make(map[int][]string)
		taskM := make(map[string]*model.Task)
		nullTaskIds := make([]int64, 0)
		for _, task := range tasks {
			operation, operationErr := durableOperationForTask(task)
			if operationErr != nil {
				logger.LogError(ctx, fmt.Sprintf("resolve durable task %s: %v", task.TaskID, operationErr))
				continue
			}
			if operation != nil && task.PrivateData.UpstreamTaskID == "" {
				summary.NullTasksFailed++
				_, terminalErr := DurableReleaseTaskOnFailure(ctx, task, "missing_upstream_task_id")
				if terminalErr != nil && !errors.Is(terminalErr, model.ErrTaskTerminalObservationManualReview) {
					logger.LogError(ctx, fmt.Sprintf("record missing upstream id observation for task %s: %v", task.TaskID, terminalErr))
				}
				continue
			}
			upstreamID := task.GetUpstreamTaskID()
			if upstreamID == "" {
				// 统计失败的未完成任务
				nullTaskIds = append(nullTaskIds, task.ID)
				continue
			}
			taskM[upstreamID] = task
			taskChannelM[task.ChannelId] = append(taskChannelM[task.ChannelId], upstreamID)
		}
		if len(nullTaskIds) > 0 {
			summary.NullTasksFailed += len(nullTaskIds)
			err := model.TaskBulkUpdateByID(nullTaskIds, map[string]any{
				"status":   "FAILURE",
				"progress": "100%",
			})
			if err != nil {
				logger.LogError(ctx, fmt.Sprintf("Fix null task_id task error: %v", err))
			} else {
				logger.LogInfo(ctx, fmt.Sprintf("Fix null task_id task success: %v", nullTaskIds))
			}
		}
		if len(taskChannelM) == 0 {
			continue
		}

		DispatchPlatformUpdate(ctx, platform, taskChannelM, taskM)
	}
	if report != nil && ctx.Err() == nil {
		report(totalPlatforms, totalPlatforms)
	}
	common.SysLog("任务进度轮询完成")
	return summary
}

// DispatchPlatformUpdate 按平台分发轮询更新
func DispatchPlatformUpdate(ctx context.Context, platform constant.TaskPlatform, taskChannelM map[int][]string, taskM map[string]*model.Task) {
	if ctx == nil {
		ctx = context.Background()
	}
	switch platform {
	case constant.TaskPlatformMidjourney:
		// MJ 轮询由其自身处理，这里预留入口
	case constant.TaskPlatformSuno:
		_ = UpdateSunoTasks(ctx, taskChannelM, taskM)
	default:
		if err := UpdateVideoTasks(ctx, platform, taskChannelM, taskM); err != nil {
			common.SysLog(fmt.Sprintf("UpdateVideoTasks fail: %s", err))
		}
	}
}

// UpdateSunoTasks 按渠道更新所有 Suno 任务
func UpdateSunoTasks(ctx context.Context, taskChannelM map[int][]string, taskM map[string]*model.Task) error {
	for channelId, taskIds := range taskChannelM {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		err := updateSunoTasks(ctx, channelId, taskIds, taskM)
		if err != nil {
			logger.LogError(ctx, fmt.Sprintf("渠道 #%d 更新异步任务失败: %s", channelId, err.Error()))
		}
	}
	return nil
}

func updateSunoTasks(ctx context.Context, channelId int, taskIds []string, taskM map[string]*model.Task) error {
	logger.LogInfo(ctx, fmt.Sprintf("渠道 #%d 未完成的任务有: %d", channelId, len(taskIds)))
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if len(taskIds) == 0 {
		return nil
	}
	ch, err := model.CacheGetChannel(channelId)
	if err != nil {
		common.SysLog(fmt.Sprintf("CacheGetChannel: %v", err))
		failedIDs := recordChannelFailureForDurableTasks(ctx, taskIds, taskM, "suno_channel_unavailable")
		err = model.TaskBulkUpdateByID(failedIDs, map[string]any{
			"fail_reason": fmt.Sprintf("获取渠道信息失败，请联系管理员，渠道ID：%d", channelId),
			"status":      "FAILURE",
			"progress":    "100%",
		})
		if err != nil {
			common.SysLog(fmt.Sprintf("UpdateSunoTask error: %v", err))
		}
		return err
	}
	adaptor := GetTaskAdaptorFunc(constant.TaskPlatformSuno)
	if adaptor == nil {
		return errors.New("adaptor not found")
	}
	proxy := ch.GetSetting().Proxy
	resp, err := adaptor.FetchTask(*ch.BaseURL, ch.Key, map[string]any{
		"ids": taskIds,
	}, proxy)
	if err != nil {
		common.SysLog(fmt.Sprintf("Get Task Do req error: %v", err))
		return err
	}
	if resp.StatusCode != http.StatusOK {
		logger.LogError(ctx, fmt.Sprintf("Get Task status code: %d", resp.StatusCode))
		return fmt.Errorf("Get Task status code: %d", resp.StatusCode)
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		common.SysLog(fmt.Sprintf("Get Suno Task parse body error: %v", err))
		return err
	}
	var responseItems taskdto.TaskResponse[[]taskdto.SunoDataResponse]
	err = common.Unmarshal(responseBody, &responseItems)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("Get Suno Task parse body error2: %v, bytes=%d", err, len(responseBody)))
		return err
	}
	if !responseItems.IsSuccess() {
		common.SysLog(fmt.Sprintf("渠道 #%d 未完成的任务有: %d, 上游批量响应失败", channelId, len(taskIds)))
		return fmt.Errorf("Suno batch response rejected with code %s", responseItems.Code)
	}

	for _, responseItem := range responseItems.Data {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		task := taskM[responseItem.TaskID]
		if task == nil {
			logger.LogWarn(ctx, fmt.Sprintf("Suno task response ignored: unknown task_id=%s", responseItem.TaskID))
			continue
		}
		if !taskNeedsUpdate(task, responseItem) {
			continue
		}

		prevStatus := task.Status
		task.Status = lo.If(model.TaskStatus(responseItem.Status) != "", model.TaskStatus(responseItem.Status)).Else(task.Status)
		task.FailReason = lo.If(responseItem.FailReason != "", responseItem.FailReason).Else(task.FailReason)
		task.SubmitTime = lo.If(responseItem.SubmitTime != 0, responseItem.SubmitTime).Else(task.SubmitTime)
		task.StartTime = lo.If(responseItem.StartTime != 0, responseItem.StartTime).Else(task.StartTime)
		task.FinishTime = lo.If(responseItem.FinishTime != 0, responseItem.FinishTime).Else(task.FinishTime)
		isFailure := responseItem.FailReason != "" || task.Status == model.TaskStatusFailure
		if isFailure {
			logger.LogInfo(ctx, fmt.Sprintf("Suno task %s reached provider failure", task.TaskID))
			task.Status = model.TaskStatusFailure
			task.Progress = "100%"
		}
		if responseItem.Status == model.TaskStatusSuccess {
			task.Progress = "100%"
		}
		task.Data = redactVideoResponseBody(responseItem.Data)
		isTerminal := isFailure || responseItem.Status == model.TaskStatusSuccess
		if isTerminal {
			operation, operationErr := durableOperationForTask(task)
			if operationErr != nil {
				logger.LogError(ctx, fmt.Sprintf("resolve durable Suno task %s: %v", task.TaskID, operationErr))
				continue
			}
			if operation != nil {
				evidence := terminalEvidenceFromSunoItem(responseItem)
				if isFailure {
					_, terminalErr := DurableReleaseTaskOnFailureWithEvidence(ctx, task, task.FailReason, evidence)
					if terminalErr != nil && !errors.Is(terminalErr, model.ErrTaskTerminalObservationManualReview) {
						logger.LogError(ctx, fmt.Sprintf("apply Suno failure for task %s: %v", task.TaskID, terminalErr))
					}
				} else {
					taskInfo := &relaycommon.TaskInfo{TaskID: responseItem.TaskID, Status: responseItem.Status}
					if terminalErr := settleTaskBillingOnCompleteWithEvidence(ctx, adaptor, task, taskInfo, evidence); terminalErr != nil && !errors.Is(terminalErr, model.ErrTaskTerminalObservationManualReview) {
						logger.LogError(ctx, fmt.Sprintf("apply Suno success for task %s: %v", task.TaskID, terminalErr))
					}
				}
				continue
			}
		}

		// 持久化走 CAS，防止重叠轮询/sweep/多实例/持久化失败重试导致重复退款或覆盖终态。
		won, err := task.UpdateWithStatus(prevStatus)
		if err != nil {
			logger.LogError(ctx, fmt.Sprintf("UpdateSunoTask task %s error: %v", task.TaskID, err))
		} else if !won {
			logger.LogWarn(ctx, fmt.Sprintf("Task %s CAS lost or no-op update, skip billing", task.TaskID))
		} else if isFailure && prevStatus != model.TaskStatusFailure && task.Quota != 0 {
			RefundTaskQuota(ctx, task, task.FailReason)
		}
	}
	return nil
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

// UpdateVideoTasks 按渠道更新所有视频任务
func UpdateVideoTasks(ctx context.Context, platform constant.TaskPlatform, taskChannelM map[int][]string, taskM map[string]*model.Task) error {
	channelIDs := make([]int, 0, len(taskChannelM))
	for channelID := range taskChannelM {
		channelIDs = append(channelIDs, channelID)
	}
	sort.Ints(channelIDs)

	var wg sync.WaitGroup
	for _, channelId := range channelIDs {
		taskIds := taskChannelM[channelId]
		if len(taskIds) == 0 {
			continue
		}
		taskIds = append([]string(nil), taskIds...)

		wg.Add(1)
		gopool.Go(func() {
			defer wg.Done()
			if err := updateVideoTasks(ctx, platform, channelId, taskIds, taskM); err != nil {
				logger.LogError(ctx, fmt.Sprintf("Channel #%d failed to update video async tasks: %s", channelId, err.Error()))
			}
		})
	}
	wg.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return nil
}

func updateVideoTasks(ctx context.Context, platform constant.TaskPlatform, channelId int, taskIds []string, taskM map[string]*model.Task) error {
	logger.LogInfo(ctx, fmt.Sprintf("Channel #%d pending video tasks: %d", channelId, len(taskIds)))
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if len(taskIds) == 0 {
		return nil
	}
	cacheGetChannel, err := model.CacheGetChannel(channelId)
	if err != nil {
		failedIDs := recordChannelFailureForDurableTasks(ctx, taskIds, taskM, "video_channel_unavailable")
		errUpdate := model.TaskBulkUpdateByID(failedIDs, map[string]any{
			"fail_reason": fmt.Sprintf("Failed to get channel info, channel ID: %d", channelId),
			"status":      "FAILURE",
			"progress":    "100%",
		})
		if errUpdate != nil {
			common.SysLog(fmt.Sprintf("UpdateVideoTask error: %v", errUpdate))
		}
		return fmt.Errorf("CacheGetChannel failed: %w", err)
	}
	adaptor := GetTaskAdaptorFunc(platform)
	if adaptor == nil {
		return fmt.Errorf("video adaptor not found")
	}
	info := &relaycommon.RelayInfo{}
	info.ChannelMeta = &relaycommon.ChannelMeta{
		ChannelBaseUrl: cacheGetChannel.GetBaseURL(),
	}
	info.ApiKey = cacheGetChannel.Key
	adaptor.Init(info)
	disablePollingSleep := cacheGetChannel.GetOtherSettings().DisableTaskPollingSleep
	for i, taskId := range taskIds {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := updateVideoSingleTask(ctx, adaptor, cacheGetChannel, taskId, taskM); err != nil {
			logger.LogError(ctx, fmt.Sprintf("Failed to update video task %s: %s", taskId, err.Error()))
		}
		if disablePollingSleep || i == len(taskIds)-1 {
			continue
		}

		// sleep 1 second between tasks for this channel only.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(1 * time.Second):
		}
	}
	return nil
}

func updateVideoSingleTask(ctx context.Context, adaptor TaskPollingAdaptor, ch *model.Channel, taskId string, taskM map[string]*model.Task) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	baseURL := constant.ChannelBaseURLs[ch.Type]
	if ch.GetBaseURL() != "" {
		baseURL = ch.GetBaseURL()
	}
	proxy := ch.GetSetting().Proxy

	task := taskM[taskId]
	if task == nil {
		logger.LogError(ctx, fmt.Sprintf("Task %s not found in taskM", taskId))
		return fmt.Errorf("task %s not found", taskId)
	}
	key := ch.Key

	privateData := task.PrivateData
	if privateData.Key != "" {
		key = privateData.Key
	}
	resp, err := adaptor.FetchTask(baseURL, key, map[string]any{
		"task_id": task.GetUpstreamTaskID(),
		"action":  task.Action,
	}, proxy)
	if err != nil {
		return fmt.Errorf("fetchTask failed for task %s: %w", taskId, err)
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("readAll failed for task %s: %w", taskId, err)
	}

	logger.LogDebug(ctx, "updateVideoSingleTask response bytes: %d", len(responseBody))

	snap := task.Snapshot()

	taskResult := &relaycommon.TaskInfo{}
	// try parse as New API response format
	var responseItems taskdto.TaskResponse[model.Task]
	if err = common.Unmarshal(responseBody, &responseItems); err == nil && responseItems.IsSuccess() {
		logger.LogDebug(ctx, "updateVideoSingleTask parsed as compatible upstream response format: %+v", responseItems)
		t := responseItems.Data
		taskResult.TaskID = t.TaskID
		taskResult.Status = string(t.Status)
		taskResult.Url = t.GetResultURL()
		taskResult.Progress = t.Progress
		taskResult.Reason = t.FailReason
		task.Data = t.Data
	} else if taskResult, err = adaptor.ParseTaskResult(responseBody); err != nil {
		return fmt.Errorf("parseTaskResult failed for task %s: %w", taskId, err)
	}

	task.Data = redactVideoResponseBody(responseBody)

	logger.LogDebug(ctx, "updateVideoSingleTask taskResult: %+v", taskResult)

	now := time.Now().Unix()
	if taskResult.Status == "" {
		//taskResult = relaycommon.FailTaskInfo("upstream returned empty status")
		errorResult := &dto.GeneralErrorResponse{}
		if err = common.Unmarshal(responseBody, &errorResult); err == nil {
			openaiError := errorResult.TryToOpenAIError()
			if openaiError != nil {
				// 返回规范的 OpenAI 错误格式，提取错误信息，判断错误是否为任务失败
				if openaiError.Code == "429" {
					// 429 错误通常表示请求过多或速率限制，暂时不认为是任务失败，保持原状态等待下一轮轮询
					return nil
				}

				// 其他错误认为是任务失败，记录错误信息并更新任务状态
				taskResult = relaycommon.FailTaskInfo("upstream returned error")
			} else {
				// unknown error format, log original response
				logger.LogError(ctx, fmt.Sprintf("Task %s returned empty status with unrecognized error format", taskId))
				taskResult = relaycommon.FailTaskInfo("upstream returned unrecognized message")
			}
		}
	}
	terminalEvidence := terminalEvidenceFromTaskInfo(taskResult, task.GetUpstreamTaskID())

	shouldRefund := false
	shouldSettle := false
	quota := task.Quota

	task.Status = model.TaskStatus(taskResult.Status)
	switch taskResult.Status {
	case model.TaskStatusSubmitted:
		task.Progress = taskcommon.ProgressSubmitted
	case model.TaskStatusQueued:
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
		if strings.HasPrefix(taskResult.Url, "data:") {
			// data: URI (e.g. Vertex base64 encoded video) — keep in Data, not in ResultURL
			task.PrivateData.ResultURL = taskcommon.BuildProxyURL(task.TaskID)
		} else if taskResult.Url != "" {
			// Direct upstream URL (e.g. Kling, Ali, Doubao, etc.)
			task.PrivateData.ResultURL = taskResult.Url
		} else {
			// No URL from adaptor — construct proxy URL using public task ID
			task.PrivateData.ResultURL = taskcommon.BuildProxyURL(task.TaskID)
		}
		shouldSettle = true
	case model.TaskStatusFailure:
		logger.LogInfo(ctx, fmt.Sprintf("Task %s reached provider failure", taskId))
		task.Status = model.TaskStatusFailure
		task.Progress = taskcommon.ProgressComplete
		if task.FinishTime == 0 {
			task.FinishTime = now
		}
		task.FailReason = taskResult.Reason
		logger.LogInfo(ctx, fmt.Sprintf("Task %s failure reason normalized before persistence", task.TaskID))
		taskResult.Progress = taskcommon.ProgressComplete
		if quota != 0 {
			shouldRefund = true
		}
	default:
		return fmt.Errorf("unknown task status %s for task %s", taskResult.Status, task.TaskID)
	}
	if taskResult.Progress != "" {
		task.Progress = taskResult.Progress
	}

	isDone := task.Status == model.TaskStatusSuccess || task.Status == model.TaskStatusFailure
	if isDone && model.DB != nil && model.DB.Migrator().HasTable(&model.TaskSubmissionOperation{}) {
		operation, err := model.GetTaskSubmissionOperationByPublicID(model.DB, task.TaskID)
		if err != nil {
			return err
		}
		if operation != nil {
			if shouldSettle {
				if err := settleTaskBillingOnCompleteWithEvidence(ctx, adaptor, task, taskResult, terminalEvidence); err != nil && !errors.Is(err, model.ErrTaskTerminalObservationManualReview) {
					return err
				}
			}
			if task.Status == model.TaskStatusFailure {
				if _, err := DurableReleaseTaskOnFailureWithEvidence(ctx, task, task.FailReason, terminalEvidence); err != nil && !errors.Is(err, model.ErrTaskTerminalObservationManualReview) {
					return err
				}
			}
			return nil
		}
	}
	if isDone && snap.Status != task.Status {
		won, err := task.UpdateWithStatus(snap.Status)
		if err != nil {
			logger.LogError(ctx, fmt.Sprintf("UpdateWithStatus failed for task %s: %s", task.TaskID, err.Error()))
			shouldRefund = false
			shouldSettle = false
		} else if !won {
			logger.LogWarn(ctx, fmt.Sprintf("Task %s CAS lost or no-op update, skip billing", task.TaskID))
			shouldRefund = false
			shouldSettle = false
		}
	} else if !snap.Equal(task.Snapshot()) {
		if _, err := task.UpdateWithStatus(snap.Status); err != nil {
			logger.LogError(ctx, fmt.Sprintf("Failed to update task %s: %s", task.TaskID, err.Error()))
		}
	} else {
		// No changes, skip update
		logger.LogDebug(ctx, "No update needed for task %s", task.TaskID)
	}

	if shouldSettle {
		settleTaskBillingOnComplete(ctx, adaptor, task, taskResult)
	}
	if shouldRefund {
		RefundTaskQuota(ctx, task, task.FailReason)
	}

	return nil
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

func durableOperationForTask(task *model.Task) (*model.TaskSubmissionOperation, error) {
	if task == nil || task.TaskID == "" || model.DB == nil {
		return nil, nil
	}
	if !model.DB.Migrator().HasTable(&model.TaskSubmissionOperation{}) {
		return nil, nil
	}
	return model.GetTaskSubmissionOperationByPublicID(model.DB, task.TaskID)
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

func recordChannelFailureForDurableTasks(ctx context.Context, taskIDs []string, taskM map[string]*model.Task, reasonCode string) []int64 {
	legacyIDs := make([]int64, 0, len(taskIDs))
	for _, upstreamID := range taskIDs {
		task := taskM[upstreamID]
		if task == nil {
			continue
		}
		op, err := durableOperationForTask(task)
		if err != nil {
			logger.LogError(ctx, fmt.Sprintf("resolve durable channel-failure task %s: %v", task.TaskID, err))
			continue
		}
		if op == nil {
			legacyIDs = append(legacyIDs, task.ID)
			continue
		}
		_, terminalErr := DurableReleaseTaskOnFailure(ctx, task, reasonCode)
		if terminalErr != nil && !errors.Is(terminalErr, model.ErrTaskTerminalObservationManualReview) {
			logger.LogError(ctx, fmt.Sprintf("record channel-failure observation for task %s: %v", task.TaskID, terminalErr))
		}
	}
	return legacyIDs
}

func settleTaskBillingOnCompleteWithEvidence(ctx context.Context, adaptor TaskPollingAdaptor, task *model.Task, taskResult *relaycommon.TaskInfo, evidence DurableTerminalEvidence) error {
	op, err := durableOperationForTask(task)
	if err != nil {
		return err
	}
	if op == nil {
		settleTaskBillingOnComplete(ctx, adaptor, task, taskResult)
		return nil
	}
	if taskResult.QuotaClamp != nil {
		_, err := DurableTerminalManualReviewWithEvidence(ctx, task, "succeeded", 0, "quota_clamp_manual_review", evidence, taskResult.QuotaClamp)
		return err
	}
	if _, snapshotErr := validateTaskBillingSnapshot(task.PrivateData.BillingContext); snapshotErr != nil {
		_, err := DurableTerminalManualReviewWithEvidence(ctx, task, "succeeded", 0, "billing_snapshot_invalid", evidence)
		return err
	}
	if task.PrivateData.BillingContext.PerCallBilling {
		_, err := DurableSettleTaskOnCompleteWithEvidence(ctx, task, task.Quota, "per_call_billing", evidence)
		return err
	}
	if actual := adaptor.AdjustBillingOnComplete(task, taskResult); actual > 0 {
		_, err := DurableSettleTaskOnCompleteWithEvidence(ctx, task, actual, "adaptor_billing_adjustment", evidence)
		return err
	}
	if taskResult.TotalTokens > 0 {
		if actual, clamp, ok := computeTaskQuotaFromTokens(task, taskResult.TotalTokens); ok {
			_, err := DurableSettleTaskOnCompleteWithEvidence(ctx, task, actual, "token_billing_recalculation", evidence, clamp)
			return err
		}
	}
	_, err = DurableTerminalManualReviewWithEvidence(ctx, task, "succeeded", 0, "terminal_actual_evidence_missing", evidence)
	return err
}
