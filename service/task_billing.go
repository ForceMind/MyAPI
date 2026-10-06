package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"strings"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/logger"
	"github.com/ForceMind/MyAPI/model"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/setting/ratio_setting"
	"github.com/ForceMind/MyAPI/types"
	"github.com/gin-gonic/gin"
)

// LogTaskConsumption 记录任务消费日志和统计信息（仅记录，不涉及实际扣费）。
// 实际扣费已由 BillingSession（PreConsumeBilling + SettleBilling）完成。
func LogTaskConsumption(c *gin.Context, info *relaycommon.RelayInfo) {
	tokenName := c.GetString("token_name")
	logContent := fmt.Sprintf("操作 %s", info.Action)
	// 支持任务仅按次计费
	if common.StringsContains(constant.TaskPricePatches, info.OriginModelName) {
		logContent = fmt.Sprintf("%s，按次计费", logContent)
	} else {
		if otherRatios := info.PriceData.OtherRatios(); len(otherRatios) > 0 {
			var contents []string
			for key, ra := range otherRatios {
				if 1.0 != ra {
					contents = append(contents, fmt.Sprintf("%s: %.2f", key, ra))
				}
			}
			if len(contents) > 0 {
				logContent = fmt.Sprintf("%s, 计算参数：%s", logContent, strings.Join(contents, ", "))
			}
		}
	}
	other := make(map[string]interface{})
	other["is_task"] = true
	other["request_path"] = c.Request.URL.Path
	other["model_price"] = info.PriceData.ModelPrice
	if info.PriceData.ModelRatio > 0 {
		other["model_ratio"] = info.PriceData.ModelRatio
	}
	other["group_ratio"] = info.PriceData.GroupRatioInfo.GroupRatio
	if info.PriceData.GroupRatioInfo.HasSpecialRatio {
		other["user_group_ratio"] = info.PriceData.GroupRatioInfo.GroupSpecialRatio
	}
	if info.IsModelMapped {
		other["is_model_mapped"] = true
		other["upstream_model_name"] = info.UpstreamModelName
	}
	appendBillingInfo(info, other)
	attachQuotaSaturation(c, info, other)
	model.RecordConsumeLog(c, info.UserId, model.RecordConsumeLogParams{
		ChannelId: info.ChannelId,
		ModelName: info.OriginModelName,
		TokenName: tokenName,
		Quota:     info.PriceData.Quota,
		Content:   logContent,
		TokenId:   info.TokenId,
		Group:     info.UsingGroup,
		Other:     other,
	})
	model.UpdateUserUsedQuotaAndRequestCount(info.UserId, info.PriceData.Quota)
	model.UpdateChannelUsedQuota(info.ChannelId, info.PriceData.Quota)
}

// ---------------------------------------------------------------------------
// 异步任务计费辅助函数
// ---------------------------------------------------------------------------

// resolveTokenKey 通过 TokenId 运行时获取令牌 Key（用于 Redis 缓存操作）。
// 如果令牌已被删除或查询失败，返回空字符串。
func resolveTokenKey(ctx context.Context, tokenId int, taskID string) string {
	token, err := model.GetTokenById(tokenId)
	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("获取令牌 key 失败 (tokenId=%d, task=%s): %s", tokenId, taskID, err.Error()))
		return ""
	}
	return token.Key
}

// taskIsSubscription 判断任务是否通过订阅计费。
func taskIsSubscription(task *model.Task) bool {
	return task.PrivateData.BillingSource == BillingSourceSubscription && task.PrivateData.SubscriptionId > 0
}

// taskAdjustFunding 调整任务的资金来源（钱包或订阅），delta > 0 表示扣费，delta < 0 表示退还。
func taskAdjustFunding(task *model.Task, delta int) error {
	if taskIsSubscription(task) {
		return model.PostConsumeUserSubscriptionDelta(task.PrivateData.SubscriptionId, int64(delta))
	}
	if delta > 0 {
		return model.DecreaseUserQuota(task.UserId, delta, false)
	}
	return model.IncreaseUserQuota(task.UserId, -delta, false)
}

// taskAdjustTokenQuota 调整任务的令牌额度，delta > 0 表示扣费，delta < 0 表示退还。
// 需要通过 resolveTokenKey 运行时获取 key（不从 PrivateData 中读取）。
func taskAdjustTokenQuota(ctx context.Context, task *model.Task, delta int) {
	if task.PrivateData.TokenId <= 0 || delta == 0 {
		return
	}
	tokenKey := resolveTokenKey(ctx, task.PrivateData.TokenId, task.TaskID)
	if tokenKey == "" {
		return
	}
	var err error
	if delta > 0 {
		err = model.DecreaseTokenQuota(task.PrivateData.TokenId, tokenKey, delta)
	} else {
		err = model.IncreaseTokenQuota(task.PrivateData.TokenId, tokenKey, -delta)
	}
	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("调整令牌额度失败 (delta=%d, task=%s): %s", delta, task.TaskID, err.Error()))
	}
}

// ---------------------------------------------------------------------------
// legacy 持久事实幂等
// ---------------------------------------------------------------------------

// taskLegacyFactModeEnabled 报告当前是否应经 legacy-kind 持久事实完成 Task/MJ
// 账务写入。仅 legacy writer 模式启用：authoritative 模式由 receipt 内核或
// durable 任务管道负责，bridge 及其余状态回落到既有直写（在 model 层 legacy
// 守卫 fail-closed），行为与迁移前一致。
func taskLegacyFactModeEnabled() bool {
	mode, err := postConsumeQuotaWriterMode()
	return err == nil && mode == model.QuotaWriterModeLegacy
}

// legacySettlementFactOutcome 描述一次 legacy-kind 结算事实的解析结果。
type legacySettlementFactOutcome int

const (
	// legacySettlementFactFailed 事实未能应用到 applied：调用方保留持久标记以待重试。
	legacySettlementFactFailed legacySettlementFactOutcome = iota
	// legacySettlementFactApplied 本次调用完成了资金与令牌写入。
	legacySettlementFactApplied
	// legacySettlementFactReplayed 事实在此调用前已应用：仅重放可重放的后续步骤。
	legacySettlementFactReplayed
)

// applyLegacySettlementFact 确保并应用一条 legacy-kind 结算事实（钱包或订阅）。
// 事实身份在首次创建时确定；后续调用先按事件键加载既有事实，并只校验不随余额
// 变化的稳定字段（资金来源/金额/主体）。ApplyToken 由创建时的令牌 used_quota
// 预检决定，应用后余额已变化，重放无法重算同一输入，故不参与校验。稳定字段
// 冲突（如同键不同差额）fail-closed；已 applied 的事实判为重放，不重复写余额；
// 租约被并发调用方持有时按未应用处理，由持有人在后续重试中完成。workerID 标识
// 租约持有者，需有界且稳定。
func applyLegacySettlementFact(ctx context.Context, factInput model.AccountQuotaSettlementFactInput, workerID string) legacySettlementFactOutcome {
	fact, err := model.FindAccountQuotaSettlementFactByEventKey(model.DB, factInput.EventKey)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("持久结算事实查询失败 key=%s: %s", factInput.EventKey, err.Error()))
		return legacySettlementFactFailed
	}
	if fact == nil {
		fact, err = model.EnsureAccountQuotaSettlementFact(ctx, model.DB, factInput)
		if err != nil {
			logger.LogError(ctx, fmt.Sprintf("持久结算事实登记失败 key=%s: %s", factInput.EventKey, err.Error()))
			return legacySettlementFactFailed
		}
	} else if fact.Kind != factInput.Kind || fact.Delta != factInput.Delta || fact.UserID != factInput.UserID ||
		fact.SubscriptionID != factInput.SubscriptionID || fact.RequestID != factInput.RequestID {
		logger.LogError(ctx, fmt.Sprintf("持久结算事实身份冲突 key=%s", factInput.EventKey))
		return legacySettlementFactFailed
	}
	if fact.State == model.AccountQuotaSettlementApplied {
		return legacySettlementFactReplayed
	}
	stored, recoverErr := model.RecoverAccountQuotaSettlementFact(ctx, model.DB, fact, workerID)
	if stored != nil && stored.State == model.AccountQuotaSettlementApplied {
		return legacySettlementFactApplied
	}
	if recoverErr != nil {
		logger.LogError(ctx, fmt.Sprintf("持久结算事实应用失败 key=%s: %s", factInput.EventKey, recoverErr.Error()))
	} else {
		logger.LogWarn(ctx, fmt.Sprintf("持久结算事实等待并发应用 key=%s", factInput.EventKey))
	}
	return legacySettlementFactFailed
}

// taskFactEventKey 将有界事实事件键映射到任务身份。TaskID/MjId 最长 191 字符，
// 可能超过事实键 128 字符上限；超限时以 sha256 摘要保持键的确定性与稳定性。
func taskFactEventKey(prefix, taskID string) string {
	if len(prefix)+len(taskID) <= 128 {
		return prefix + taskID
	}
	sum := sha256.Sum256([]byte(taskID))
	return prefix + "sha256:" + hex.EncodeToString(sum[:])
}

// taskFactRequestID 将任务身份压缩进事实 RequestID 的 64 字符上限。
func taskFactRequestID(taskID string) string {
	if len(taskID) <= 64 {
		return taskID
	}
	sum := sha256.Sum256([]byte(taskID))
	return hex.EncodeToString(sum[:])
}

// taskSettlementFactInput 构建 Task 结算/退款共用的 legacy-kind 事实输入。
// delta > 0 为补扣，delta < 0 为退还。令牌侧仅在令牌可解析且应用后 used_quota
// 不为负时随事实应用：事实内核拒绝负 used，而 legacy 直写允许（used_quota 可
// 转负），该边界由调用方在事实应用后走 taskAdjustTokenQuota 保持迁移前结果。
// 令牌不可解析时跳过令牌侧，与 taskAdjustTokenQuota 的语义一致。
func taskSettlementFactInput(ctx context.Context, task *model.Task, eventKey string, delta int) model.AccountQuotaSettlementFactInput {
	input := model.AccountQuotaSettlementFactInput{
		EventKey:  eventKey,
		RequestID: taskFactRequestID(task.TaskID),
		Kind:      model.AccountQuotaSettlementKindLegacyWallet,
		UserID:    task.UserId,
		Delta:     int64(delta),
	}
	if taskIsSubscription(task) {
		input.Kind = model.AccountQuotaSettlementKindLegacySubscription
		input.SubscriptionID = task.PrivateData.SubscriptionId
	}
	if task.PrivateData.TokenId > 0 {
		if token, err := model.GetTokenById(task.PrivateData.TokenId); err == nil && token.Key != "" &&
			int64(token.UsedQuota)+int64(delta) >= 0 {
			input.TokenID = task.PrivateData.TokenId
			input.ApplyToken = true
		}
	}
	return input
}

// taskBillingOther 从 task 的 BillingContext 构建日志 Other 字段。
func taskBillingOther(task *model.Task, resolved ...*model.TaskBillingContext) map[string]interface{} {
	other := make(map[string]interface{})
	bc := task.PrivateData.BillingContext
	if len(resolved) > 0 {
		bc = resolved[0]
	}
	if bc != nil {
		if priceData := taskBillingContextPriceData(bc); priceData != nil {
			for k, v := range priceData.OtherRatios() {
				other[k] = v
			}
		}
		other["model_price"] = bc.ModelPrice
		if bc.ModelRatio > 0 || len(resolved) > 0 {
			other["model_ratio"] = bc.ModelRatio
		}
		other["group_ratio"] = bc.GroupRatio
	}
	props := task.Properties
	if props.UpstreamModelName != "" && props.UpstreamModelName != props.OriginModelName {
		other["is_model_mapped"] = true
		other["upstream_model_name"] = props.UpstreamModelName
	}
	return other
}

func taskBillingContextPriceData(bc *model.TaskBillingContext) *types.PriceData {
	if bc == nil || len(bc.OtherRatios) == 0 {
		return nil
	}
	priceData := &types.PriceData{}
	if !priceData.ReplaceOtherRatios(bc.OtherRatios) {
		return nil
	}
	return priceData
}

// taskModelName 从 BillingContext 或 Properties 中获取模型名称。
func taskModelName(task *model.Task) string {
	if bc := task.PrivateData.BillingContext; bc != nil && bc.OriginModelName != "" {
		return bc.OriginModelName
	}
	return task.Properties.OriginModelName
}

// RefundTaskQuota 统一的任务失败退款逻辑。
// 当异步任务失败时，退还资金与令牌额度，并回减用户和渠道用量。
// 返回资金来源是否已成功退还；失败时保留 quota，供显式重试或人工对账。
func RefundTaskQuota(ctx context.Context, task *model.Task, reason string) bool {
	quota := task.Quota
	if quota == 0 {
		return true
	}

	if handled, err := DurableReleaseTaskOnFailure(ctx, task, reason); handled {
		if err != nil {
			logger.LogError(ctx, fmt.Sprintf("durable release failed for task %s: %v", task.TaskID, err))
			return false
		}
		return true
	}

	if task.TaskID != "" && taskLegacyFactModeEnabled() {
		return refundTaskQuotaLegacyFact(ctx, task, quota, reason)
	}

	// 非 legacy 模式或无稳定任务身份：保持迁移前直写机制；非 legacy 模式下由
	// model 层 legacy 守卫 fail-closed，行为与迁移前一致。
	// 1. 退还资金来源（钱包或订阅）
	if err := taskAdjustFunding(task, -quota); err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("退还资金来源失败 task %s: %s", task.TaskID, err.Error()))
		return false
	}

	// 2. 退还令牌额度
	taskAdjustTokenQuota(ctx, task, -quota)

	// 3. 回减预扣时累计的用户和渠道用量，请求次数保持不变
	model.UpdateUserUsedQuota(task.UserId, -quota)
	model.UpdateChannelUsedQuota(task.ChannelId, -quota)

	// 4. 记录日志
	other := taskBillingOther(task)
	other["task_id"] = task.TaskID
	other["reason"] = reason
	model.RecordTaskBillingLog(model.RecordTaskBillingLogParams{
		UserId:    task.UserId,
		LogType:   model.LogTypeRefund,
		Content:   "",
		ChannelId: task.ChannelId,
		ModelName: taskModelName(task),
		Quota:     quota,
		TokenId:   task.PrivateData.TokenId,
		Group:     task.Group,
		Other:     other,
	})

	// 5. 资金退款完成后再清除持久化标记。
	// 回写失败必须显式告警，避免漏掉潜在的重复退款风险。
	task.Quota = 0
	if err := task.UpdateQuota(); err != nil {
		logger.LogError(ctx, fmt.Sprintf("退款成功但清除 task quota 失败 task %s: %s", task.TaskID, err.Error()))
	}
	return true
}

// refundTaskQuotaLegacyFact 以持久事实 "task-refund:{taskID}" 执行 legacy 全额退款。
// 退款事实先行：同一 taskID 的重复轮询/重复退款经事实指纹幂等，不多退不多扣；
// 统计列与退款日志只在事实首次应用时执行；清除 task.Quota 是可重放的后续步骤，
// 回写失败不会开启重复退款窗口（重试按重放处理，仅补齐清零）。
// legacy 流程中一次任务只发生一次终态退款（状态 CAS 守卫），因此退款键无需
// 区分阶段；同键不同指纹（金额/资金来源变化）在 Ensure 处冲突并 fail-closed。
func refundTaskQuotaLegacyFact(ctx context.Context, task *model.Task, quota int, reason string) bool {
	factInput := taskSettlementFactInput(ctx, task, taskFactEventKey("task-refund:", task.TaskID), -quota)
	switch applyLegacySettlementFact(ctx, factInput, "task-polling:"+taskFactRequestID(task.TaskID)) {
	case legacySettlementFactFailed:
		return false
	case legacySettlementFactApplied:
		// 事实未覆盖令牌侧（used_quota 会转负的 legacy 宽容边界）时，按迁移前
		// 直写机制退还令牌额度；重放不会进入此分支，不会重复退还。
		if !factInput.ApplyToken {
			taskAdjustTokenQuota(ctx, task, -quota)
		}
		// 回减预扣时累计的用户和渠道用量，请求次数保持不变
		model.UpdateUserUsedQuota(task.UserId, -quota)
		model.UpdateChannelUsedQuota(task.ChannelId, -quota)

		// 记录日志
		other := taskBillingOther(task)
		other["task_id"] = task.TaskID
		other["reason"] = reason
		model.RecordTaskBillingLog(model.RecordTaskBillingLogParams{
			UserId:    task.UserId,
			LogType:   model.LogTypeRefund,
			Content:   "",
			ChannelId: task.ChannelId,
			ModelName: taskModelName(task),
			Quota:     quota,
			TokenId:   task.PrivateData.TokenId,
			Group:     task.Group,
			Other:     other,
		})
	}

	// 清除持久化标记是可重放步骤：失败仅告警，下次重试按重放补齐。
	task.Quota = 0
	if err := task.UpdateQuota(); err != nil {
		logger.LogError(ctx, fmt.Sprintf("退款成功但清除 task quota 失败 task %s: %s", task.TaskID, err.Error()))
	}
	return true
}

// RecalculateTaskQuota 通用的异步差额结算。
// actualQuota 是任务完成后的实际应扣额度，与预扣额度 (task.Quota) 做差额结算。
// reason 用于日志记录（例如 "token重算" 或 "adaptor调整"）。
// clamps 按发生顺序传入，首个非空事件记入 admin_info（仅管理员可见）。
// 存在饱和事件但差额为零时，仅记录 quota=0 的审计日志，不调整账务或请求次数。
func RecalculateTaskQuota(ctx context.Context, task *model.Task, actualQuota int, reason string, clamps ...*common.QuotaClamp) {
	recalculateTaskQuota(ctx, task, actualQuota, reason, taskQuotaSettlementOptions{}, clamps...)
}

// Only a validated explicit free rate may settle a real usage to zero. The
// public API retains its historical zero-means-unavailable behavior.
type taskQuotaSettlementOptions struct {
	allowZero bool
	audit     bool
	other     map[string]interface{}
}

func recalculateTaskQuota(ctx context.Context, task *model.Task, actualQuota int, reason string, options taskQuotaSettlementOptions, clamps ...*common.QuotaClamp) {
	var clamp *common.QuotaClamp
	for _, candidate := range clamps {
		if candidate != nil {
			clamp = candidate
			break
		}
	}
	if clamp != nil {
		logger.LogWarn(ctx, fmt.Sprintf("quota saturation on task log: op=%s kind=%s original=%g clamped=%d task=%s user=%d model=%s",
			clamp.Op, clamp.Kind, clamp.Original, clamp.Clamped, task.TaskID, task.UserId, taskModelName(task)))
	}
	if actualQuota < 0 || (actualQuota == 0 && !options.allowZero) {
		if clamp == nil && !options.audit {
			return
		}
		// Unusable usage still keeps the original reservation, but must not
		// suppress the provider/conversion audit event.
		actualQuota = task.Quota
	}
	preConsumedQuota := task.Quota
	quotaDelta := actualQuota - preConsumedQuota
	settleReplayed := false

	if quotaDelta == 0 {
		logger.LogInfo(ctx, fmt.Sprintf("任务 %s 预扣费准确（%s，%s）",
			task.TaskID, logger.LogQuota(actualQuota), reason))
		if clamp == nil && !options.audit {
			return
		}
	} else {
		logger.LogInfo(ctx, fmt.Sprintf("任务 %s 差额结算：delta=%s（实际：%s，预扣：%s，%s）",
			task.TaskID,
			logger.LogQuota(quotaDelta),
			logger.LogQuota(actualQuota),
			logger.LogQuota(preConsumedQuota),
			reason,
		))

		if task.TaskID != "" && taskLegacyFactModeEnabled() {
			// legacy 持久事实 "task-settle:{taskID}"：重复轮询/重试按同一事实
			// 幂等收敛。legacy 流程中一次任务只在终态结算一次（状态 CAS 守卫），
			// 键无需区分阶段；同键不同指纹在 Ensure 处冲突并 fail-closed。
			factInput := taskSettlementFactInput(ctx, task, taskFactEventKey("task-settle:", task.TaskID), quotaDelta)
			switch applyLegacySettlementFact(ctx, factInput, "task-polling:"+taskFactRequestID(task.TaskID)) {
			case legacySettlementFactFailed:
				return
			case legacySettlementFactApplied:
				// 事实未覆盖令牌侧（used_quota 会转负的 legacy 宽容边界）时，
				// 按迁移前直写机制调整令牌额度；重放不会进入此分支。
				if !factInput.ApplyToken {
					taskAdjustTokenQuota(ctx, task, quotaDelta)
				}
			case legacySettlementFactReplayed:
				settleReplayed = true
			}
		} else {
			// 调整资金来源
			if err := taskAdjustFunding(task, quotaDelta); err != nil {
				logger.LogError(ctx, fmt.Sprintf("差额结算资金调整失败 task %s: %s", task.TaskID, err.Error()))
				return
			}

			// 调整令牌额度
			taskAdjustTokenQuota(ctx, task, quotaDelta)
		}

		task.Quota = actualQuota
		if err := task.UpdateQuota(); err != nil {
			logger.LogError(ctx, fmt.Sprintf("差额结算回写 quota 失败 task %s: %s", task.TaskID, err.Error()))
		}

		if !settleReplayed {
			// 提交阶段已经累计过一次请求；结算阶段只调整最终用量。
			model.UpdateUserUsedQuota(task.UserId, quotaDelta)
			model.UpdateChannelUsedQuota(task.ChannelId, quotaDelta)
		}
	}

	if settleReplayed {
		// 重放仅补齐 task.Quota 回写；结算日志已在事实首次应用时记录。
		return
	}

	var logType int
	var logQuota int
	if quotaDelta == 0 {
		// Audit-only events must not count as consumed requests or enter
		// quota exports, and remain visible when consume logging is disabled.
		logType = model.LogTypeSystem
	} else if quotaDelta > 0 {
		logType = model.LogTypeConsume
		logQuota = quotaDelta
	} else {
		logType = model.LogTypeRefund
		logQuota = -quotaDelta
	}
	other := options.other
	if other == nil {
		other = taskBillingOther(task)
	}
	other["task_id"] = task.TaskID
	other["pre_consumed_quota"] = preConsumedQuota
	other["actual_quota"] = actualQuota
	attachQuotaSaturationToOther(other, clamp)
	model.RecordTaskBillingLog(model.RecordTaskBillingLogParams{
		UserId:    task.UserId,
		LogType:   logType,
		Content:   reason,
		ChannelId: task.ChannelId,
		ModelName: taskModelName(task),
		Quota:     logQuota,
		TokenId:   task.PrivateData.TokenId,
		Group:     task.Group,
		Other:     other,
		NodeName:  task.PrivateData.NodeName,
	})
}

// RecalculateTaskQuotaByTokens 根据实际 token 消耗重新计费（异步差额结算）。
// 当任务成功且返回了 totalTokens 时，根据模型倍率和分组倍率重新计算实际扣费额度，
// 与预扣费的差额进行补扣或退还。支持钱包和订阅计费来源。
func RecalculateTaskQuotaByTokens(ctx context.Context, task *model.Task, totalTokens int, clamps ...*common.QuotaClamp) {
	if auditInvalidTaskBillingSnapshot(ctx, task, clamps...) {
		return
	}
	var usageClamp *common.QuotaClamp
	for _, clamp := range clamps {
		if clamp != nil {
			usageClamp = clamp
			break
		}
	}
	if totalTokens <= 0 {
		if usageClamp != nil {
			RecalculateTaskQuota(ctx, task, task.Quota, "上游计费用量不可用，保持预扣额度", usageClamp)
		}
		return
	}
	if bc := task.PrivateData.BillingContext; bc != nil && bc.PerCallBilling {
		if usageClamp != nil {
			RecalculateTaskQuota(ctx, task, task.Quota, "按次计费，保持预扣额度", usageClamp)
		}
		return
	}

	rates, source, err := resolveTaskTokenBillingRates(task)
	other := taskBillingOther(task, rates)
	other["billing_rate_source"] = source
	if bc := task.PrivateData.BillingContext; bc != nil {
		other["billing_rate_version"] = bc.Version
	}
	if err != nil {
		other["billing_rate_error"] = err.Error()
		logger.LogWarn(ctx, fmt.Sprintf("task billing rates unavailable: task=%s source=%s error=%s", task.TaskID, source, err))
		recalculateTaskQuota(ctx, task, task.Quota, "计费费率不可用，保持预扣额度", taskQuotaSettlementOptions{audit: true, other: other}, usageClamp)
		return
	}

	// 计算 OtherRatios 乘积（视频折扣、时长等）
	otherMultiplier := 1.0
	if priceData := taskBillingContextPriceData(rates); priceData != nil {
		otherMultiplier = priceData.OtherRatioMultiplier()
	}
	freeRate := rates.ModelRatio == 0 || rates.GroupRatio == 0
	actualQuota := 0
	var clamp *common.QuotaClamp
	if !freeRate {
		actualQuota, clamp = common.QuotaFromFloatChecked(float64(totalTokens) * rates.ModelRatio * rates.GroupRatio * otherMultiplier)
	}
	reason := fmt.Sprintf("token重算：tokens=%d, modelRatio=%.2f, groupRatio=%.2f, otherMultiplier=%.4f", totalTokens, rates.ModelRatio, rates.GroupRatio, otherMultiplier)
	// The earliest event is authoritative, including a provider clamp that
	// becomes in-range after applying small model/group/other ratios.
	recalculateTaskQuota(ctx, task, actualQuota, reason, taskQuotaSettlementOptions{
		allowZero: freeRate, audit: source == "legacy_current", other: other,
	}, usageClamp, clamp)
}

// validateTaskBillingSnapshot validates only persisted metadata. Incomplete
// legacy contexts (including zero model rates on old per-call tasks) remain
// valid here; deciding whether token repricing needs live rates is separate.
func validateTaskBillingSnapshot(bc *model.TaskBillingContext) (string, error) {
	if bc != nil {
		if bc.Version != 0 && bc.Version != model.TaskBillingContextVersion {
			return "unsupported_snapshot", fmt.Errorf("unsupported billing context version %d", bc.Version)
		}
		if (bc.Version == model.TaskBillingContextVersion && !bc.Complete) || (bc.Version == 0 && bc.Complete) {
			return "invalid_snapshot", fmt.Errorf("incomplete billing context marker")
		}
		for name, rate := range map[string]float64{"model_ratio": bc.ModelRatio, "group_ratio": bc.GroupRatio} {
			if rate < 0 || math.IsNaN(rate) || math.IsInf(rate, 0) {
				return "invalid_snapshot", fmt.Errorf("invalid %s", name)
			}
		}
		// -1 is the existing token-priced task sentinel, not a negative rate.
		if math.IsNaN(bc.ModelPrice) || math.IsInf(bc.ModelPrice, 0) || (bc.ModelPrice < 0 && bc.ModelPrice != -1) {
			return "invalid_snapshot", fmt.Errorf("invalid model_price")
		}
		for _, rate := range bc.OtherRatios {
			if rate <= 0 || math.IsNaN(rate) || math.IsInf(rate, 0) {
				return "invalid_snapshot", fmt.Errorf("invalid other_ratio")
			}
		}
		if bc.Version == model.TaskBillingContextVersion {
			if bc.OriginModelName == "" {
				return "invalid_snapshot", fmt.Errorf("missing origin model name")
			}
			return "snapshot_v1", nil
		}
		if bc.ModelRatio > 0 && bc.GroupRatio > 0 && bc.OriginModelName != "" {
			return "legacy_snapshot", nil
		}
	}
	return "legacy_current", nil
}

// auditInvalidTaskBillingSnapshot must run before any settlement early return
// or adaptor override. It never loads current pricing and never changes funds.
func auditInvalidTaskBillingSnapshot(ctx context.Context, task *model.Task, clamps ...*common.QuotaClamp) bool {
	bc := task.PrivateData.BillingContext
	source, err := validateTaskBillingSnapshot(bc)
	if err == nil {
		return false
	}
	// Do not copy invalid prices into Other: NaN/Inf would discard the entire
	// JSON payload. Error text is fixed metadata validation text, not upstream
	// response content or a dump of the persisted snapshot.
	other := taskBillingOther(task, nil)
	other["billing_rate_source"] = source
	other["billing_rate_version"] = bc.Version
	other["billing_rate_error"] = err.Error()
	logger.LogWarn(ctx, fmt.Sprintf("task billing snapshot unavailable: task=%s source=%s error=%s", task.TaskID, source, err))
	recalculateTaskQuota(ctx, task, task.Quota, "计费快照不可用，保持预扣额度", taskQuotaSettlementOptions{audit: true, other: other}, clamps...)
	return true
}

// resolveTaskTokenBillingRates consumes the existing persisted context. Only
// genuinely incomplete legacy contexts consult live configuration; the
// temporary context returned in that case is used for calculation/logging,
// never written back over submission history.
func resolveTaskTokenBillingRates(task *model.Task) (*model.TaskBillingContext, string, error) {
	bc := task.PrivateData.BillingContext
	source, err := validateTaskBillingSnapshot(bc)
	if err != nil {
		return nil, source, err
	}
	if source != "legacy_current" {
		return bc, source, nil
	}

	modelName := taskModelName(task)
	modelRatio, configured, _ := ratio_setting.GetModelRatio(modelName)
	if !configured || modelRatio <= 0 || math.IsNaN(modelRatio) || math.IsInf(modelRatio, 0) {
		return nil, "legacy_current", fmt.Errorf("legacy model ratio unavailable")
	}
	user, err := model.GetUserById(task.UserId, false)
	if err != nil || user.Group == "" {
		return nil, "legacy_current", fmt.Errorf("legacy user group unavailable")
	}
	group := task.Group
	if group == "" {
		group = user.Group
	}
	if group == "" {
		return nil, "legacy_current", fmt.Errorf("legacy routing group unavailable")
	}
	var groupRatio float64
	if specialRatio, ok := ratio_setting.GetGroupGroupRatio(user.Group, group); ok {
		groupRatio = specialRatio
	} else {
		if !ratio_setting.ContainsGroupRatio(group) {
			return nil, "legacy_current", fmt.Errorf("legacy group ratio unavailable")
		}
		groupRatio = ratio_setting.GetGroupRatio(group)
	}
	if groupRatio <= 0 || math.IsNaN(groupRatio) || math.IsInf(groupRatio, 0) {
		return nil, "legacy_current", fmt.Errorf("legacy group ratio unavailable")
	}
	rates := &model.TaskBillingContext{ModelPrice: -1, ModelRatio: modelRatio, GroupRatio: groupRatio, OriginModelName: modelName}
	if bc != nil {
		if priceData := taskBillingContextPriceData(bc); priceData != nil {
			rates.OtherRatios = priceData.OtherRatios()
		}
	}
	return rates, "legacy_current", nil
}
