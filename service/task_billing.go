package service

import (
	"context"
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

		// 调整资金来源
		if err := taskAdjustFunding(task, quotaDelta); err != nil {
			logger.LogError(ctx, fmt.Sprintf("差额结算资金调整失败 task %s: %s", task.TaskID, err.Error()))
			return
		}

		// 调整令牌额度
		taskAdjustTokenQuota(ctx, task, quotaDelta)

		task.Quota = actualQuota
		if err := task.UpdateQuota(); err != nil {
			logger.LogError(ctx, fmt.Sprintf("差额结算回写 quota 失败 task %s: %s", task.TaskID, err.Error()))
		}

		// 提交阶段已经累计过一次请求；结算阶段只调整最终用量。
		model.UpdateUserUsedQuota(task.UserId, quotaDelta)
		model.UpdateChannelUsedQuota(task.ChannelId, quotaDelta)
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
