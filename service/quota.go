package service

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/logger"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/pkg/billingexpr"
	perfmetrics "github.com/ForceMind/MyAPI/pkg/perf_metrics"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/ForceMind/MyAPI/setting/ratio_setting"
	"github.com/ForceMind/MyAPI/types"

	"github.com/bytedance/gopkg/util/gopool"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

type TokenDetails struct {
	TextTokens  int
	AudioTokens int
}

type QuotaInfo struct {
	InputDetails  TokenDetails
	OutputDetails TokenDetails
	ModelName     string
	UsePrice      bool
	ModelPrice    float64
	ModelRatio    float64
	GroupRatio    float64
	priceData     types.PriceData // carries the validated request currency unit
}

func hasCustomModelRatio(modelName string, currentRatio float64) bool {
	defaultRatio, exists := ratio_setting.GetDefaultModelRatioMap()[modelName]
	if !exists {
		return true
	}
	return currentRatio != defaultRatio
}

func calculateAudioQuota(info QuotaInfo) (int, *common.QuotaClamp) {
	if info.UsePrice {
		modelPrice := decimal.NewFromFloat(info.ModelPrice)
		quotaPerUnit := decimal.NewFromFloat(requestQuotaUnit(info.priceData))
		groupRatio := decimal.NewFromFloat(info.GroupRatio)

		quota := modelPrice.Mul(quotaPerUnit).Mul(groupRatio)
		return common.QuotaFromDecimalChecked(quota)
	}

	completion, audio, audioCompletion := requestAudioRatios(info.priceData, info.ModelName)
	completionRatio := decimal.NewFromFloat(completion)
	audioRatio := decimal.NewFromFloat(audio)
	audioCompletionRatio := decimal.NewFromFloat(audioCompletion)

	groupRatio := decimal.NewFromFloat(info.GroupRatio)
	modelRatio := decimal.NewFromFloat(info.ModelRatio)
	ratio := groupRatio.Mul(modelRatio)

	inputTextTokens := decimal.NewFromInt(int64(info.InputDetails.TextTokens))
	outputTextTokens := decimal.NewFromInt(int64(info.OutputDetails.TextTokens))
	inputAudioTokens := decimal.NewFromInt(int64(info.InputDetails.AudioTokens))
	outputAudioTokens := decimal.NewFromInt(int64(info.OutputDetails.AudioTokens))

	quota := decimal.Zero
	quota = quota.Add(inputTextTokens)
	quota = quota.Add(outputTextTokens.Mul(completionRatio))
	quota = quota.Add(inputAudioTokens.Mul(audioRatio))
	quota = quota.Add(outputAudioTokens.Mul(audioRatio).Mul(audioCompletionRatio))

	quota = quota.Mul(ratio)

	// If ratio is not zero and quota is less than or equal to zero, set quota to 1
	if !ratio.IsZero() && quota.LessThanOrEqual(decimal.Zero) {
		quota = decimal.NewFromInt(1)
	}

	return common.QuotaFromDecimalChecked(quota)
}

func PreWssConsumeQuota(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, usage *dto.RealtimeUsage) error {
	if relayInfo.UsePrice {
		return nil
	}
	modelName := relayInfo.OriginModelName
	textInputTokens := usage.InputTokenDetails.TextTokens
	textOutTokens := usage.OutputTokenDetails.TextTokens
	audioInputTokens := usage.InputTokenDetails.AudioTokens
	audioOutTokens := usage.OutputTokenDetails.AudioTokens
	modelRatio := relayInfo.PriceData.ModelRatio
	actualGroupRatio := relayInfo.PriceData.GroupRatioInfo.GroupRatio
	if relayInfo.PriceData.QuotedQuotaUnit(0) == 0 {
		// Only legacy callers without a quote use live configuration. The
		// normal request already froze these values before opening its socket.
		modelRatio, _, _ = ratio_setting.GetModelRatio(modelName)
		actualGroupRatio = ratio_setting.GetGroupRatio(relayInfo.UsingGroup)
		autoGroup, exists := common.GetContextKey(ctx, constant.ContextKeyAutoGroup)
		if exists {
			actualGroupRatio = ratio_setting.GetGroupRatio(autoGroup.(string))
			logger.LogDebug(ctx, "final group ratio: %f", actualGroupRatio)
			relayInfo.UsingGroup = autoGroup.(string)
		}
		if userGroupRatio, ok := ratio_setting.GetGroupGroupRatio(relayInfo.UserGroup, relayInfo.UsingGroup); ok {
			actualGroupRatio = userGroupRatio
		}
	}

	quotaInfo := QuotaInfo{
		InputDetails: TokenDetails{
			TextTokens:  textInputTokens,
			AudioTokens: audioInputTokens,
		},
		OutputDetails: TokenDetails{
			TextTokens:  textOutTokens,
			AudioTokens: audioOutTokens,
		},
		ModelName:  modelName,
		UsePrice:   relayInfo.UsePrice,
		ModelRatio: modelRatio,
		GroupRatio: actualGroupRatio,
		priceData:  relayInfo.PriceData,
	}

	quota, clamp := calculateAudioQuota(quotaInfo)
	noteQuotaClamp(relayInfo, clamp)
	if clamp != nil {
		return clamp
	}

	neededQuota := quota
	target := 0
	if relayInfo.Billing != nil {
		var clamp *common.QuotaClamp
		target, clamp = common.QuotaFromDecimalChecked(decimal.NewFromInt(int64(relayInfo.RealtimeQuotedQuota)).
			Add(decimal.NewFromInt(int64(quota))))
		noteQuotaClamp(relayInfo, clamp)
		if clamp != nil {
			return clamp
		}
		if pricing := relayInfo.RealtimeTieredPricing; pricing != nil && !pricing.Incomplete {
			target = pricing.Quota
		}
		neededQuota = max(0, target-relayInfo.Billing.GetPreConsumedQuota())
	}

	if neededQuota > 0 {
		userQuota, err := model.GetUserQuota(relayInfo.UserId, false)
		if err != nil {
			return err
		}
		if userQuota < neededQuota {
			return fmt.Errorf("user quota is not enough, user quota: %s, need quota: %s", logger.FormatQuota(userQuota), logger.FormatQuota(neededQuota))
		}
		if !relayInfo.IsPlayground || relayInfo.Billing == nil {
			token, err := model.GetTokenByKey(strings.TrimPrefix(relayInfo.TokenKey, "sk-"), false)
			if err != nil {
				return err
			}
			if !token.UnlimitedQuota && token.RemainQuota < neededQuota {
				return fmt.Errorf("token quota is not enough, token remain quota: %s, need quota: %s", logger.FormatQuota(token.RemainQuota), logger.FormatQuota(neededQuota))
			}
		}
	}
	if relayInfo.Billing != nil {
		if err := relayInfo.Billing.Reserve(target); err != nil {
			return err
		}
		relayInfo.RealtimeQuotedQuota = target
		relayInfo.RealtimeConsumeSeq++
		return nil
	}

	// realtime 流式连接在一次 RequestId 内可能多次计费（每个 response.done
	// 一次），authoritative 幂等键需要连接内稳定的按次序号。
	relayInfo.RealtimeConsumeSeq++
	_, err := postConsumeQuotaWithEvent(relayInfo, quota, 0, false, postConsumeQuotaEvent{
		Namespace:  "realtime",
		Qualifier:  strconv.Itoa(relayInfo.RealtimeConsumeSeq),
		ReasonCode: "realtime_consume",
	})
	if err != nil {
		return err
	}
	logger.LogInfo(ctx, "realtime streaming consume quota success, quota: "+fmt.Sprintf("%d", quota))
	return nil
}

func PostWssConsumeQuota(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, modelName string,
	usage *dto.RealtimeUsage, extraContent string) {

	var tieredResult *billingexpr.TieredResult
	var tieredUsedVars map[string]bool
	if snapshot := relayInfo.TieredBillingSnapshot; snapshot != nil {
		tieredUsedVars = billingexpr.UsedVars(snapshot.ExprString)
	}
	projected := &dto.Usage{
		PromptTokens: usage.InputTokens, CompletionTokens: usage.OutputTokens,
		PromptTokensDetails: usage.InputTokenDetails, CompletionTokenDetails: usage.OutputTokenDetails,
	}
	var tieredOk bool
	var tieredQuota int
	var tieredRes *billingexpr.TieredResult
	perResponsePricing := false
	snapshot := relayInfo.TieredBillingSnapshot
	if pricing := relayInfo.RealtimeTieredPricing; pricing != nil && !pricing.Incomplete && pricing.Responses > 0 &&
		snapshot != nil && snapshot.BillingMode == "tiered_expr" && pricing.ExprHash == snapshot.ExprHash && pricing.ExprHash == billingexpr.ExprHashString(snapshot.ExprString) &&
		pricing.QuotaPerUnit == snapshot.QuotaPerUnit && pricing.GroupRatio == snapshot.GroupRatio &&
		pricing.InputTokens == usage.InputTokens && pricing.OutputTokens == usage.OutputTokens && pricing.TotalTokens == usage.TotalTokens {
		tieredOk, tieredQuota, tieredRes = true, pricing.Quota, nil
		perResponsePricing = true
	} else {
		tieredOk, tieredQuota, tieredRes = TryTieredSettle(relayInfo, BuildTieredTokenParams(projected, false, tieredUsedVars))
	}
	if tieredOk {
		tieredResult = tieredRes
	}

	useTimeSeconds := time.Now().Unix() - relayInfo.StartTime.Unix()
	textInputTokens := usage.InputTokenDetails.TextTokens
	textOutTokens := usage.OutputTokenDetails.TextTokens

	audioInputTokens := usage.InputTokenDetails.AudioTokens
	audioOutTokens := usage.OutputTokenDetails.AudioTokens

	tokenName := ctx.GetString("token_name")
	completion, audio, audioCompletion := requestAudioRatios(relayInfo.PriceData, modelName)
	if relayInfo.PriceData.QuotedQuotaUnit(0) == 0 {
		// Preserve the legacy log's origin-model audio label; captured quotes
		// use the same request ratios for both settlement and logging.
		audio = ratio_setting.GetAudioRatio(relayInfo.OriginModelName)
	}
	completionRatio := decimal.NewFromFloat(completion)
	audioRatio := decimal.NewFromFloat(audio)
	audioCompletionRatio := decimal.NewFromFloat(audioCompletion)

	modelRatio := relayInfo.PriceData.ModelRatio
	groupRatio := relayInfo.PriceData.GroupRatioInfo.GroupRatio
	modelPrice := relayInfo.PriceData.ModelPrice
	usePrice := relayInfo.PriceData.UsePrice

	quotaInfo := QuotaInfo{
		InputDetails: TokenDetails{
			TextTokens:  textInputTokens,
			AudioTokens: audioInputTokens,
		},
		OutputDetails: TokenDetails{
			TextTokens:  textOutTokens,
			AudioTokens: audioOutTokens,
		},
		ModelName:  modelName,
		UsePrice:   usePrice,
		ModelPrice: modelPrice,
		ModelRatio: modelRatio,
		GroupRatio: groupRatio,
		priceData:  relayInfo.PriceData,
	}

	quota, clamp := calculateAudioQuota(quotaInfo)
	noteQuotaClamp(relayInfo, clamp)
	if tieredOk {
		quota = tieredQuota
	}

	totalTokens := usage.TotalTokens
	var logContent string
	if !usePrice {
		logContent = fmt.Sprintf("模型倍率 %.2f，补全倍率 %.2f，音频倍率 %.2f，音频补全倍率 %.2f，分组倍率 %.2f",
			modelRatio, completionRatio.InexactFloat64(), audioRatio.InexactFloat64(), audioCompletionRatio.InexactFloat64(), groupRatio)
	} else {
		logContent = fmt.Sprintf("模型价格 %.2f，分组倍率 %.2f", modelPrice, groupRatio)
	}

	// record all the consume log even if quota is 0
	if totalTokens == 0 {
		// in this case, must be some error happened
		// we cannot just return, because we may have to return the pre-consumed quota
		quota = 0
		logContent += "（可能是上游超时）"
		logger.LogError(ctx, fmt.Sprintf("total tokens is 0, cannot consume quota, userId %d, channelId %d, "+
			"tokenId %d, model %s， pre-consumed quota %d", relayInfo.UserId, relayInfo.ChannelId, relayInfo.TokenId, modelName, relayInfo.FinalPreConsumedQuota))
	} else {
		model.UpdateUserUsedQuotaAndRequestCount(relayInfo.UserId, quota)
		model.UpdateChannelUsedQuota(relayInfo.ChannelId, quota)
	}

	if err := SettleBilling(ctx, relayInfo, quota); err != nil {
		logger.LogError(ctx, "error settling billing: "+err.Error())
	}

	logModel := modelName
	if extraContent != "" {
		logContent += ", " + extraContent
	}
	other := GenerateWssOtherInfo(ctx, relayInfo, usage, modelRatio, groupRatio,
		completionRatio.InexactFloat64(), audioRatio.InexactFloat64(), audioCompletionRatio.InexactFloat64(), modelPrice, relayInfo.PriceData.GroupRatioInfo.GroupSpecialRatio)
	if perResponsePricing {
		other["billing_mode"] = "tiered_expr"
		other["realtime_pricing_scope"] = "per_response"
		other["realtime_priced_responses"] = relayInfo.RealtimeTieredPricing.Responses
		other["realtime_tiered_quota"] = relayInfo.RealtimeTieredPricing.Quota
	}
	if tieredResult != nil {
		InjectTieredBillingInfo(other, relayInfo, tieredResult)
	}
	attachQuotaSaturation(ctx, relayInfo, other)
	model.RecordConsumeLog(ctx, relayInfo.UserId, model.RecordConsumeLogParams{
		ChannelId:        relayInfo.ChannelId,
		PromptTokens:     usage.InputTokens,
		CompletionTokens: usage.OutputTokens,
		ModelName:        logModel,
		TokenName:        tokenName,
		Quota:            quota,
		Content:          logContent,
		TokenId:          relayInfo.TokenId,
		UseTimeSeconds:   int(useTimeSeconds),
		IsStream:         relayInfo.IsStream,
		Group:            relayInfo.UsingGroup,
		Other:            other,
	})
}

func CalcOpenRouterCacheCreateTokens(usage dto.Usage, priceData types.PriceData) (int, error) {
	if priceData.CacheCreationRatio == 1 {
		return 0, nil
	}
	quotaPrice := priceData.ModelRatio / requestQuotaUnit(priceData)
	promptCacheCreatePrice := quotaPrice * priceData.CacheCreationRatio
	promptCacheReadPrice := quotaPrice * priceData.CacheRatio
	completionPrice := quotaPrice * priceData.CompletionRatio
	denominator := promptCacheCreatePrice - quotaPrice
	if denominator == 0 || math.IsNaN(denominator) || math.IsInf(denominator, 0) {
		return 0, fmt.Errorf("invalid OpenRouter cache creation price denominator")
	}

	cost, _ := usage.Cost.(float64)
	totalPromptTokens := float64(usage.PromptTokens)
	completionTokens := float64(usage.CompletionTokens)
	promptCacheReadTokens := float64(usage.PromptTokensDetails.CachedTokens)

	raw := (cost -
		totalPromptTokens*quotaPrice +
		promptCacheReadTokens*(quotaPrice-promptCacheReadPrice) -
		completionTokens*completionPrice) / denominator
	if math.IsNaN(raw) || math.IsInf(raw, 0) || raw < 0 {
		return 0, fmt.Errorf("invalid OpenRouter cache creation token estimate")
	}
	quota, clamp := common.QuotaFromFloatChecked(raw)
	if clamp != nil {
		return 0, clamp
	}
	return quota, nil
}

func PostAudioConsumeQuota(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, usage *dto.Usage, extraContent string) {

	var tieredUsedVars map[string]bool
	if snap := relayInfo.TieredBillingSnapshot; snap != nil {
		tieredUsedVars = billingexpr.UsedVars(snap.ExprString)
	}
	var tieredResult *billingexpr.TieredResult
	tieredOk, tieredQuota, tieredRes := TryTieredSettle(relayInfo, BuildTieredTokenParams(usage, false, tieredUsedVars))
	if tieredOk {
		tieredResult = tieredRes
	}

	useTimeSeconds := time.Now().Unix() - relayInfo.StartTime.Unix()
	textInputTokens := usage.PromptTokensDetails.TextTokens
	textOutTokens := usage.CompletionTokenDetails.TextTokens

	audioInputTokens := usage.PromptTokensDetails.AudioTokens
	audioOutTokens := usage.CompletionTokenDetails.AudioTokens

	tokenName := ctx.GetString("token_name")
	completion, audio, audioCompletion := requestAudioRatios(relayInfo.PriceData, relayInfo.OriginModelName)
	completionRatio := decimal.NewFromFloat(completion)
	audioRatio := decimal.NewFromFloat(audio)
	audioCompletionRatio := decimal.NewFromFloat(audioCompletion)

	modelRatio := relayInfo.PriceData.ModelRatio
	groupRatio := relayInfo.PriceData.GroupRatioInfo.GroupRatio
	modelPrice := relayInfo.PriceData.ModelPrice
	usePrice := relayInfo.PriceData.UsePrice

	quotaInfo := QuotaInfo{
		InputDetails: TokenDetails{
			TextTokens:  textInputTokens,
			AudioTokens: audioInputTokens,
		},
		OutputDetails: TokenDetails{
			TextTokens:  textOutTokens,
			AudioTokens: audioOutTokens,
		},
		ModelName:  relayInfo.OriginModelName,
		UsePrice:   usePrice,
		ModelPrice: modelPrice,
		ModelRatio: modelRatio,
		GroupRatio: groupRatio,
		priceData:  relayInfo.PriceData,
	}

	quota, clamp := calculateAudioQuota(quotaInfo)
	noteQuotaClamp(relayInfo, clamp)
	if tieredOk {
		quota = tieredQuota
	}

	totalTokens := usage.TotalTokens
	var logContent string
	if !usePrice {
		logContent = fmt.Sprintf("模型倍率 %.2f，补全倍率 %.2f，音频倍率 %.2f，音频补全倍率 %.2f，分组倍率 %.2f",
			modelRatio, completionRatio.InexactFloat64(), audioRatio.InexactFloat64(), audioCompletionRatio.InexactFloat64(), groupRatio)
	} else {
		logContent = fmt.Sprintf("模型价格 %.2f，分组倍率 %.2f", modelPrice, groupRatio)
	}

	// record all the consume log even if quota is 0
	if totalTokens == 0 {
		// in this case, must be some error happened
		// we cannot just return, because we may have to return the pre-consumed quota
		quota = 0
		logContent += "（可能是上游超时）"
		logger.LogError(ctx, fmt.Sprintf("total tokens is 0, cannot consume quota, userId %d, channelId %d, "+
			"tokenId %d, model %s， pre-consumed quota %d", relayInfo.UserId, relayInfo.ChannelId, relayInfo.TokenId, relayInfo.OriginModelName, relayInfo.FinalPreConsumedQuota))
	} else {
		model.UpdateUserUsedQuotaAndRequestCount(relayInfo.UserId, quota)
		model.UpdateChannelUsedQuota(relayInfo.ChannelId, quota)
	}

	if err := SettleBilling(ctx, relayInfo, quota); err != nil {
		logger.LogError(ctx, "error settling billing: "+err.Error())
	}

	logModel := relayInfo.OriginModelName
	if extraContent != "" {
		logContent += ", " + extraContent
	}
	other := GenerateAudioOtherInfo(ctx, relayInfo, usage, modelRatio, groupRatio,
		completionRatio.InexactFloat64(), audioRatio.InexactFloat64(), audioCompletionRatio.InexactFloat64(), modelPrice, relayInfo.PriceData.GroupRatioInfo.GroupSpecialRatio)
	if tieredResult != nil {
		InjectTieredBillingInfo(other, relayInfo, tieredResult)
	}
	attachQuotaSaturation(ctx, relayInfo, other)
	model.RecordConsumeLog(ctx, relayInfo.UserId, model.RecordConsumeLogParams{
		ChannelId:        relayInfo.ChannelId,
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		ModelName:        logModel,
		TokenName:        tokenName,
		Quota:            quota,
		Content:          logContent,
		TokenId:          relayInfo.TokenId,
		UseTimeSeconds:   int(useTimeSeconds),
		IsStream:         relayInfo.IsStream,
		Group:            relayInfo.UsingGroup,
		Other:            other,
	})
	gopool.Go(func() {
		perfmetrics.RecordRelaySample(relayInfo, true, int64(usage.CompletionTokens))
	})
}

// PreConsumeTokenQuota 仅服务 legacy 计费路径（BillingSession.preConsume /
// reserveToken）。authoritative 模式下 BillingSession 的 token 预扣经
// ReserveAccountQuota 内核完成（见 newAuthoritativeBillingSession）；
// model.TryReserveTokenQuota 在非 legacy 模式下经 requireLegacyQuotaWriterCall
// fail-closed，二者共同保证本函数不会在 authoritative/bridge 模式下生效。
func PreConsumeTokenQuota(relayInfo *relaycommon.RelayInfo, quota int) error {
	if quota < 0 {
		return errors.New("quota 不能为负数！")
	}
	if relayInfo.IsPlayground {
		return nil
	}
	// 原子预扣：检查与扣减在同一操作中完成，并发请求不可能同时通过检查后超扣。
	reserved, err := model.TryReserveTokenQuota(relayInfo.TokenId, relayInfo.TokenKey, quota, relayInfo.TokenUnlimited)
	if err != nil {
		return err
	}
	if !reserved {
		remainQuota := 0
		if token, tokenErr := model.GetTokenByKey(relayInfo.TokenKey, false); tokenErr == nil && token != nil {
			remainQuota = token.RemainQuota
		}
		return fmt.Errorf("token quota is not enough, token remain quota: %s, need quota: %s", logger.FormatQuota(remainQuota), logger.FormatQuota(quota))
	}
	return nil
}

type postConsumeQuotaResult struct {
	FundingApplied bool
	TokenApplied   bool
}

// postConsumeQuotaEvent 携带 authoritative 配额写入所需的持久业务身份。
// Namespace 按 caller 区分（如 "billing-settlement" / "realtime" /
// "violation-fee"），与请求 ID 组合成稳定幂等键；Qualifier 用于同一请求内
// 存在多次独立计费的场景（realtime 流式连接的按次序号）。
// ReasonCode 为稳定的英文 snake_case 审计码。
type postConsumeQuotaEvent struct {
	Namespace  string
	Qualifier  string
	ReasonCode string
}

// postConsumeQuotaWriterMode 解析当前配额 writer 模式。尚未迁移 epoch 表的
// 数据库（pre-WP3 或隔离测试夹具）按 legacy 处理，与 NewBillingSession 一致。
func postConsumeQuotaWriterMode() (model.QuotaWriterMode, error) {
	if model.DB == nil || !model.DB.Migrator().HasTable(&model.QuotaWriterEpoch{}) {
		return model.QuotaWriterModeLegacy, nil
	}
	state, err := model.GetQuotaWriterEpochState(model.DB)
	if err != nil {
		return "", err
	}
	return model.QuotaWriterMode(state.Mode), nil
}

func PostConsumeQuota(relayInfo *relaycommon.RelayInfo, quota int, preConsumedQuota int, sendEmail bool) error {
	_, err := postConsumeQuotaWithResult(relayInfo, quota, preConsumedQuota, sendEmail)
	return err
}

func postConsumeQuotaWithResult(relayInfo *relaycommon.RelayInfo, quota int, preConsumedQuota int, sendEmail bool) (result postConsumeQuotaResult, err error) {
	// 未迁移的 caller（如 midjourney 任务计费）不提供稳定业务键：保持 legacy
	// 直写机制；非 legacy 模式下由 model 层 legacy 守卫 fail-closed，
	// 行为与迁移前完全一致。
	return postConsumeQuotaWithEvent(relayInfo, quota, preConsumedQuota, sendEmail, postConsumeQuotaEvent{})
}

// postConsumeQuotaWithEvent 按配额 writer 模式分发后结算直写：
//   - legacy：与迁移前相同的写入机制与结果（含缓存投影）；
//   - authoritative：经 QuotaMutationContext WithContext 壳进入 receipt 内核，
//     以 Namespace + 请求 ID（+ Qualifier）构成的稳定业务键幂等；
//   - bridge：fail-closed，受控切换由后续批次处理。
func postConsumeQuotaWithEvent(relayInfo *relaycommon.RelayInfo, quota int, preConsumedQuota int, sendEmail bool, event postConsumeQuotaEvent) (result postConsumeQuotaResult, err error) {
	if event.Namespace != "" {
		var mode model.QuotaWriterMode
		mode, err = postConsumeQuotaWriterMode()
		if err != nil {
			return result, err
		}
		switch mode {
		case model.QuotaWriterModeLegacy:
			// fall through to the legacy writer below
		case model.QuotaWriterModeAuthoritative:
			result, err = postConsumeQuotaAuthoritative(relayInfo, quota, event)
			if err == nil && sendEmail && (quota+preConsumedQuota) != 0 {
				checkAndSendQuotaNotify(relayInfo, quota, preConsumedQuota)
			}
			return result, err
		default:
			return result, model.ErrDurableQuotaWriterModeDisabled
		}
	}
	result, err = postConsumeQuotaLegacy(relayInfo, quota)
	if err == nil && sendEmail && (quota+preConsumedQuota) != 0 {
		checkAndSendQuotaNotify(relayInfo, quota, preConsumedQuota)
	}
	return result, err
}

// postConsumeQuotaLegacy 保持迁移前的 legacy 写入机制与结果，不做任何模式分发。
func postConsumeQuotaLegacy(relayInfo *relaycommon.RelayInfo, quota int) (result postConsumeQuotaResult, err error) {

	// 1) Consume from wallet quota OR subscription item
	if relayInfo != nil && relayInfo.BillingSource == BillingSourceSubscription {
		if relayInfo.SubscriptionId == 0 {
			return result, errors.New("subscription id is missing")
		}
		delta := int64(quota)
		if delta != 0 {
			if err := model.PostConsumeUserSubscriptionDelta(relayInfo.SubscriptionId, delta); err != nil {
				return result, err
			}
			relayInfo.SubscriptionPostDelta += delta
		}
	} else {
		// Wallet
		if quota > 0 {
			err = model.DecreaseUserQuota(relayInfo.UserId, quota, false)
		} else {
			err = model.IncreaseUserQuota(relayInfo.UserId, -quota, false)
		}
		if err != nil {
			return result, err
		}
	}
	result.FundingApplied = true

	if !relayInfo.IsPlayground {
		if quota > 0 {
			err = model.DecreaseTokenQuota(relayInfo.TokenId, relayInfo.TokenKey, quota)
		} else {
			err = model.IncreaseTokenQuota(relayInfo.TokenId, relayInfo.TokenKey, -quota)
		}
		if err != nil {
			return result, err
		}
		result.TokenApplied = true
	}

	return result, nil
}

// postConsumeQuotaAuthoritative 经 receipt 内核完成 user/token 直写，余额结果
// 与 legacy 路径一致；同一事件键重放幂等，键冲突（不同金额）显式报错。
func postConsumeQuotaAuthoritative(relayInfo *relaycommon.RelayInfo, quota int, event postConsumeQuotaEvent) (result postConsumeQuotaResult, err error) {
	requestID := ""
	if relayInfo != nil {
		requestID = strings.TrimSpace(relayInfo.RequestId)
	}
	if requestID == "" {
		// authoritative 写入必须以稳定请求身份幂等；缺失时 fail-closed。
		return result, errors.Join(model.ErrAccountQuotaMutationInvalidInput,
			errors.New("authoritative post-consume requires a stable request id"))
	}
	eventKey := event.Namespace + ":" + requestID
	if event.Qualifier != "" {
		eventKey += ":" + event.Qualifier
	}
	if relayInfo.BillingSource == BillingSourceSubscription {
		// 订阅资金源的直写没有 receipt 内核入口（订阅经 Reserve/Settle 链计费），
		// 订阅场景的 authoritative 后结算由后续批次处理，本路径 fail-closed。
		return result, model.ErrDurableQuotaWriterModeDisabled
	}
	mutation := model.QuotaMutationContext{EventKey: eventKey, ReasonCode: event.ReasonCode}
	// 零差额在 legacy 路径下是余额无操作（quota ± 0）；内核拒绝零增量收据，
	// 因此跳过内核调用但保持 Applied 标记与 legacy 一致。
	if quota != 0 {
		if quota > 0 {
			err = model.DecreaseUserQuotaWithContext(mutation, relayInfo.UserId, quota)
		} else {
			err = model.IncreaseUserQuotaWithContext(mutation, relayInfo.UserId, -quota)
		}
		if err != nil {
			return result, err
		}
	}
	result.FundingApplied = true

	if !relayInfo.IsPlayground {
		if quota != 0 {
			if quota > 0 {
				err = model.DecreaseTokenQuotaWithContext(mutation, relayInfo.TokenId, relayInfo.TokenKey, quota)
			} else {
				err = model.IncreaseTokenQuotaWithContext(mutation, relayInfo.TokenId, relayInfo.TokenKey, -quota)
			}
			if err != nil {
				return result, err
			}
		}
		result.TokenApplied = true
	}

	return result, nil
}

func checkAndSendQuotaNotify(relayInfo *relaycommon.RelayInfo, quota int, preConsumedQuota int) {
	gopool.Go(func() {
		userSetting := relayInfo.UserSetting
		threshold := common.QuotaRemindThreshold
		if userSetting.QuotaWarningThreshold != 0 {
			threshold = int(userSetting.QuotaWarningThreshold)
		}

		//noMoreQuota := userCache.Quota-(quota+preConsumedQuota) <= 0
		quotaTooLow := false
		consumeQuota := quota + preConsumedQuota
		if relayInfo.UserQuota-consumeQuota < threshold {
			quotaTooLow = true
		}
		if quotaTooLow {
			prompt := "您的额度即将用尽"
			topUpLink := PaymentReturnURL("/wallet")

			// 根据通知方式生成不同的内容格式
			var content string
			var values []interface{}

			notifyType := userSetting.NotifyType
			if notifyType == "" {
				notifyType = dto.NotifyTypeEmail
			}

			if notifyType == dto.NotifyTypeBark {
				// Bark推送使用简短文本，不支持HTML
				content = "{{value}}，剩余额度：{{value}}，请及时充值"
				values = []interface{}{prompt, logger.FormatQuota(relayInfo.UserQuota)}
			} else if notifyType == dto.NotifyTypeGotify {
				content = "{{value}}，当前剩余额度为 {{value}}，请及时充值。"
				values = []interface{}{prompt, logger.FormatQuota(relayInfo.UserQuota)}
			} else {
				// 默认内容格式，适用于Email和Webhook（支持HTML）
				content = "{{value}}，当前剩余额度为 {{value}}，为了不影响您的使用，请及时充值。<br/>充值链接：<a href='{{value}}'>{{value}}</a>"
				values = []interface{}{prompt, logger.FormatQuota(relayInfo.UserQuota), topUpLink, topUpLink}
			}

			err := NotifyUser(relayInfo.UserId, relayInfo.UserEmail, relayInfo.UserSetting, dto.NewNotify(dto.NotifyTypeQuotaExceed, prompt, content, values))
			if err != nil {
				common.SysError(fmt.Sprintf("failed to send quota notify to user %d: %s", relayInfo.UserId, err.Error()))
			}
		}
	})
}

func checkAndSendSubscriptionQuotaNotify(relayInfo *relaycommon.RelayInfo) {
	gopool.Go(func() {
		if relayInfo == nil {
			return
		}
		if relayInfo.SubscriptionId == 0 || relayInfo.SubscriptionAmountTotal <= 0 {
			return
		}

		userSetting := relayInfo.UserSetting
		threshold := common.QuotaRemindThreshold
		if userSetting.QuotaWarningThreshold != 0 {
			threshold = int(userSetting.QuotaWarningThreshold)
		}

		usedAfter := relayInfo.SubscriptionAmountUsedAfterPreConsume + relayInfo.SubscriptionPostDelta
		remaining := relayInfo.SubscriptionAmountTotal - usedAfter
		if remaining >= int64(threshold) {
			return
		}

		prompt := "您的订阅额度即将用尽"
		topUpLink := PaymentReturnURL("/wallet")

		var content string
		var values []interface{}
		notifyType := userSetting.NotifyType
		if notifyType == "" {
			notifyType = dto.NotifyTypeEmail
		}

		if notifyType == dto.NotifyTypeBark {
			content = "{{value}}，剩余额度：{{value}}，请及时充值"
			values = []interface{}{prompt, logger.FormatQuota(int(remaining))}
		} else if notifyType == dto.NotifyTypeGotify {
			content = "{{value}}，当前剩余额度为 {{value}}，请及时充值。"
			values = []interface{}{prompt, logger.FormatQuota(int(remaining))}
		} else {
			content = "{{value}}，当前剩余额度为 {{value}}，为了不影响您的使用，请及时充值。<br/>充值链接：<a href='{{value}}'>{{value}}</a>"
			values = []interface{}{prompt, logger.FormatQuota(int(remaining)), topUpLink, topUpLink}
		}

		if err := NotifyUser(relayInfo.UserId, relayInfo.UserEmail, relayInfo.UserSetting, dto.NewNotify(dto.NotifyTypeQuotaExceed, prompt, content, values)); err != nil {
			common.SysError(fmt.Sprintf("failed to send subscription quota notify to user %d: %s", relayInfo.UserId, err.Error()))
		}
	})
}
