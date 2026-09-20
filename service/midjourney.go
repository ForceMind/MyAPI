package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/dto"
	"github.com/ForceMind/MyAPI/logger"
	"github.com/ForceMind/MyAPI/model"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	relayconstant "github.com/ForceMind/MyAPI/relay/constant"
	"github.com/ForceMind/MyAPI/setting"

	"github.com/gin-gonic/gin"
)

func CovertMjpActionToModelName(mjAction string) string {
	modelName := "mj_" + strings.ToLower(mjAction)
	if mjAction == constant.MjActionSwapFace {
		modelName = "swap_face"
	}
	return modelName
}

// PrepareMidjourneyTaskBilling sets the durable refund marker before the task is inserted.
func PrepareMidjourneyTaskBilling(relayInfo *relaycommon.RelayInfo, task *model.Midjourney, quota int, shouldBill bool) (bool, error) {
	if task == nil {
		return false, errors.New("Midjourney task is nil")
	}
	task.Quota = 0
	task.TokenId = 0
	task.BillingChannelId = 0
	if !shouldBill {
		return false, nil
	}
	if relayInfo == nil {
		return false, errors.New("relay info is nil")
	}
	if quota < 0 {
		return false, errors.New("quota cannot be negative")
	}
	if relayInfo.BillingSource == BillingSourceSubscription {
		return false, errors.New("legacy Midjourney billing does not support subscriptions")
	}

	task.Quota = quota
	task.BillingChannelId = task.ChannelId
	if relayInfo.ChannelMeta != nil && relayInfo.ChannelId > 0 {
		task.BillingChannelId = relayInfo.ChannelId
	}
	return true, nil
}

// SettleMidjourneyTaskBilling charges a persisted legacy task and records the applied stages.
// 稳定业务键为 "mj-billing:{mjId}"（mjId 缺失时回退请求 ID）：legacy 模式保持
// 迁移前直写机制与余额结果；authoritative 模式经 receipt 内核按事件键幂等；
// bridge 模式 fail-closed。
func SettleMidjourneyTaskBilling(relayInfo *relaycommon.RelayInfo, task *model.Midjourney, prepared bool) (bool, error) {
	if !prepared {
		return false, nil
	}
	if relayInfo == nil {
		return false, errors.New("relay info is nil")
	}
	if task == nil || task.Id == 0 {
		return false, errors.New("Midjourney task must be persisted before billing")
	}

	billingInfo := *relayInfo
	if task.MjId != "" {
		billingInfo.RequestId = task.MjId
	}
	result, billingErr := postConsumeQuotaWithEvent(&billingInfo, task.Quota, 0, true, postConsumeQuotaEvent{
		Namespace:  "mj-billing",
		ReasonCode: "mj_task_billing",
	})
	if !result.FundingApplied {
		task.Quota = 0
		task.TokenId = 0
		task.BillingChannelId = 0
		if updateErr := task.UpdateBillingState(); updateErr != nil {
			return false, errors.Join(billingErr, fmt.Errorf("clear Midjourney billing state: %w", updateErr))
		}
		return false, billingErr
	}

	task.TokenId = 0
	if result.TokenApplied {
		task.TokenId = relayInfo.TokenId
	}
	if updateErr := task.UpdateBillingState(); updateErr != nil {
		return true, errors.Join(billingErr, fmt.Errorf("update Midjourney billing state: %w", updateErr))
	}
	return true, billingErr
}

// RefundMidjourneyQuota reverses every accounting element recorded for a billed legacy task.
// 幂等护栏不再只依赖 task.Quota 内存值：legacy 模式经持久退款事实
// "mj-refund:{mjId}"，authoritative 模式经同键 receipt，重复 notify/并发 poll
// 下最多退一次；bridge 模式 fail-closed。UpdateWithStatus CAS 仍是并发入口守卫。
func RefundMidjourneyQuota(ctx context.Context, task *model.Midjourney, reason string) bool {
	quota := task.Quota
	if quota == 0 {
		return true
	}

	if task.MjId != "" {
		if mode, modeErr := postConsumeQuotaWriterMode(); modeErr == nil {
			switch mode {
			case model.QuotaWriterModeLegacy:
				return refundMidjourneyQuotaLegacyFact(ctx, task, quota, reason)
			case model.QuotaWriterModeAuthoritative:
				return refundMidjourneyQuotaAuthoritative(ctx, task, quota, reason)
			default:
				// bridge：fail-closed，保留 quota 标记等待受控切换。
				logger.LogError(ctx, fmt.Sprintf("Midjourney 退款在当前配额 writer 模式下不可用 task %s", task.MjId))
				return false
			}
		}
		// 模式解析失败回落迁移前直写；model 层守卫决定最终成败。
	}

	if err := model.IncreaseUserQuota(task.UserId, quota, false); err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("退还 Midjourney 用户额度失败 task %s: %s", task.MjId, err.Error()))
		return false
	}

	refundMidjourneyTokenQuota(ctx, task, quota)

	refundMidjourneyUsageAndLog(task, quota, reason)

	task.Quota = 0
	if err := task.UpdateBillingState(); err != nil {
		logger.LogError(ctx, fmt.Sprintf("Midjourney 退款成功但清除 quota 失败 task %s: %s", task.MjId, err.Error()))
	}
	return true
}

// refundMidjourneyTokenQuota 按迁移前直写机制退还 Midjourney 令牌额度
// （best-effort，失败仅告警）。legacy 直写允许 used_quota 转负。
func refundMidjourneyTokenQuota(ctx context.Context, task *model.Midjourney, quota int) {
	if task.TokenId <= 0 {
		return
	}
	tokenKey := resolveTokenKey(ctx, task.TokenId, task.MjId)
	if tokenKey == "" {
		return
	}
	if err := model.IncreaseTokenQuota(task.TokenId, tokenKey, quota); err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("退还 Midjourney 令牌额度失败 task %s: %s", task.MjId, err.Error()))
	}
}

// refundMidjourneyUsageAndLog 回减用户/渠道用量并记录退款日志。统计列保持现状
// 语义，只在退款事实首次应用时执行一次。
func refundMidjourneyUsageAndLog(task *model.Midjourney, quota int, reason string) {
	billingChannelId := task.GetBillingChannelId()
	model.UpdateUserUsedQuota(task.UserId, -quota)
	model.UpdateChannelUsedQuota(billingChannelId, -quota)
	model.RecordTaskBillingLog(model.RecordTaskBillingLogParams{
		UserId:    task.UserId,
		LogType:   model.LogTypeRefund,
		Content:   "",
		ChannelId: billingChannelId,
		ModelName: CovertMjpActionToModelName(task.Action),
		Quota:     quota,
		TokenId:   task.TokenId,
		Other: map[string]interface{}{
			"task_id": task.MjId,
			"reason":  reason,
		},
	})
}

// refundMidjourneyQuotaLegacyFact 以持久退款事实 "mj-refund:{mjId}" 执行 legacy
// 退款：事实先行保证最多退一次；清除 task.Quota 是可重放步骤，失败不开启重复
// 退款窗口。Midjourney 在 Prepare 阶段已拒绝订阅计费，因此恒为 legacy_wallet。
func refundMidjourneyQuotaLegacyFact(ctx context.Context, task *model.Midjourney, quota int, reason string) bool {
	factInput := model.AccountQuotaSettlementFactInput{
		EventKey:  taskFactEventKey("mj-refund:", task.MjId),
		RequestID: taskFactRequestID(task.MjId),
		Kind:      model.AccountQuotaSettlementKindLegacyWallet,
		UserID:    task.UserId,
		Delta:     -int64(quota),
	}
	if task.TokenId > 0 {
		// 事实内核拒绝负 used_quota，而 legacy 直写允许转负；仅在应用后
		// used 不为负时让事实覆盖令牌侧，否则由 legacy 直写回退保持迁移前结果。
		if token, err := model.GetTokenById(task.TokenId); err == nil && token.Key != "" &&
			int64(token.UsedQuota) >= int64(quota) {
			factInput.TokenID = task.TokenId
			factInput.ApplyToken = true
		}
	}
	switch applyLegacySettlementFact(ctx, factInput, "mj-polling:"+taskFactRequestID(task.MjId)) {
	case legacySettlementFactFailed:
		return false
	case legacySettlementFactApplied:
		if !factInput.ApplyToken {
			refundMidjourneyTokenQuota(ctx, task, quota)
		}
		refundMidjourneyUsageAndLog(task, quota, reason)
	}

	task.Quota = 0
	if err := task.UpdateBillingState(); err != nil {
		logger.LogError(ctx, fmt.Sprintf("Midjourney 退款成功但清除 quota 失败 task %s: %s", task.MjId, err.Error()))
	}
	return true
}

// refundMidjourneyQuotaAuthoritative 经 receipt 内核退还 authoritative 模式的
// Midjourney 计费。退款与结算共用同一命名空间的不同事件键：结算收据
// "mj-billing:{mjId}"，退款收据 "mj-refund:{mjId}"（user/token 各自收据表可
// 复用同键）。同键重放幂等；重放仅补齐 task.Quota 清零，不重复回减统计列。
func refundMidjourneyQuotaAuthoritative(ctx context.Context, task *model.Midjourney, quota int, reason string) bool {
	eventKey := taskFactEventKey("mj-refund:", task.MjId)
	existing, err := model.FindUserQuotaMutationReceiptByEventKey(model.DB, eventKey)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("查询 Midjourney 退款收据失败 task %s: %s", task.MjId, err.Error()))
		return false
	}
	if existing == nil {
		mutation := model.QuotaMutationContext{Context: ctx, EventKey: eventKey, ReasonCode: "mj_task_refund"}
		if err := model.IncreaseUserQuotaWithContext(mutation, task.UserId, quota); err != nil {
			logger.LogWarn(ctx, fmt.Sprintf("退还 Midjourney 用户额度失败 task %s: %s", task.MjId, err.Error()))
			return false
		}
		if task.TokenId > 0 {
			if tokenKey := resolveTokenKey(ctx, task.TokenId, task.MjId); tokenKey != "" {
				if err := model.IncreaseTokenQuotaWithContext(mutation, task.TokenId, tokenKey, quota); err != nil {
					logger.LogWarn(ctx, fmt.Sprintf("退还 Midjourney 令牌额度失败 task %s: %s", task.MjId, err.Error()))
				}
			}
		}
		refundMidjourneyUsageAndLog(task, quota, reason)
	}

	task.Quota = 0
	if err := task.UpdateBillingState(); err != nil {
		logger.LogError(ctx, fmt.Sprintf("Midjourney 退款成功但清除 quota 失败 task %s: %s", task.MjId, err.Error()))
	}
	return true
}

func GetMjRequestModel(relayMode int, midjRequest *dto.MidjourneyRequest) (string, *dto.MidjourneyResponse, bool) {
	action := ""
	if relayMode == relayconstant.RelayModeMidjourneyAction {
		// plus request
		err := CoverPlusActionToNormalAction(midjRequest)
		if err != nil {
			return "", err, false
		}
		action = midjRequest.Action
	} else {
		switch relayMode {
		case relayconstant.RelayModeMidjourneyImagine:
			action = constant.MjActionImagine
		case relayconstant.RelayModeMidjourneyVideo:
			action = constant.MjActionVideo
		case relayconstant.RelayModeMidjourneyEdits:
			action = constant.MjActionEdits
		case relayconstant.RelayModeMidjourneyDescribe:
			action = constant.MjActionDescribe
		case relayconstant.RelayModeMidjourneyBlend:
			action = constant.MjActionBlend
		case relayconstant.RelayModeMidjourneyShorten:
			action = constant.MjActionShorten
		case relayconstant.RelayModeMidjourneyChange:
			action = midjRequest.Action
		case relayconstant.RelayModeMidjourneyModal:
			action = constant.MjActionModal
		case relayconstant.RelayModeSwapFace:
			action = constant.MjActionSwapFace
		case relayconstant.RelayModeMidjourneyUpload:
			action = constant.MjActionUpload
		case relayconstant.RelayModeMidjourneySimpleChange:
			params := ConvertSimpleChangeParams(midjRequest.Content)
			if params == nil {
				return "", MidjourneyErrorWrapper(constant.MjRequestError, "invalid_request"), false
			}
			action = params.Action
		case relayconstant.RelayModeMidjourneyTaskFetch, relayconstant.RelayModeMidjourneyTaskFetchByCondition, relayconstant.RelayModeMidjourneyNotify:
			return "", nil, true
		default:
			return "", MidjourneyErrorWrapper(constant.MjRequestError, "unknown_relay_action"), false
		}
	}
	modelName := CovertMjpActionToModelName(action)
	return modelName, nil, true
}

func CoverPlusActionToNormalAction(midjRequest *dto.MidjourneyRequest) *dto.MidjourneyResponse {
	// "customId": "MJ::JOB::upsample::2::3dbbd469-36af-4a0f-8f02-df6c579e7011"
	customId := midjRequest.CustomId
	if customId == "" {
		return MidjourneyErrorWrapper(constant.MjRequestError, "custom_id_is_required")
	}
	splits := strings.Split(customId, "::")
	var action string
	if splits[1] == "JOB" {
		action = splits[2]
	} else {
		action = splits[1]
	}

	if action == "" {
		return MidjourneyErrorWrapper(constant.MjRequestError, "unknown_action")
	}
	if strings.Contains(action, "upsample") {
		index, err := strconv.Atoi(splits[3])
		if err != nil {
			return MidjourneyErrorWrapper(constant.MjRequestError, "index_parse_failed")
		}
		midjRequest.Index = index
		midjRequest.Action = constant.MjActionUpscale
	} else if strings.Contains(action, "variation") {
		midjRequest.Index = 1
		if action == "variation" {
			index, err := strconv.Atoi(splits[3])
			if err != nil {
				return MidjourneyErrorWrapper(constant.MjRequestError, "index_parse_failed")
			}
			midjRequest.Index = index
			midjRequest.Action = constant.MjActionVariation
		} else if action == "low_variation" {
			midjRequest.Action = constant.MjActionLowVariation
		} else if action == "high_variation" {
			midjRequest.Action = constant.MjActionHighVariation
		}
	} else if strings.Contains(action, "pan") {
		midjRequest.Action = constant.MjActionPan
		midjRequest.Index = 1
	} else if strings.Contains(action, "reroll") {
		midjRequest.Action = constant.MjActionReRoll
		midjRequest.Index = 1
	} else if action == "Outpaint" {
		midjRequest.Action = constant.MjActionZoom
		midjRequest.Index = 1
	} else if action == "CustomZoom" {
		midjRequest.Action = constant.MjActionCustomZoom
		midjRequest.Index = 1
	} else if action == "Inpaint" {
		midjRequest.Action = constant.MjActionInPaint
		midjRequest.Index = 1
	} else {
		return MidjourneyErrorWrapper(constant.MjRequestError, "unknown_action:"+customId)
	}
	return nil
}

func ConvertSimpleChangeParams(content string) *dto.MidjourneyRequest {
	split := strings.Split(content, " ")
	if len(split) != 2 {
		return nil
	}

	action := strings.ToLower(split[1])
	changeParams := &dto.MidjourneyRequest{}
	changeParams.TaskId = split[0]

	if action[0] == 'u' {
		changeParams.Action = "UPSCALE"
	} else if action[0] == 'v' {
		changeParams.Action = "VARIATION"
	} else if action == "r" {
		changeParams.Action = "REROLL"
		return changeParams
	} else {
		return nil
	}

	index, err := strconv.Atoi(action[1:2])
	if err != nil || index < 1 || index > 4 {
		return nil
	}
	changeParams.Index = index
	return changeParams
}

// decodeMidjourneyResponse normalizes the scalar and upload response shapes.
// The original response body remains available to callers for exact
// pass-through; Result is projected to the first uploaded URL for the legacy
// scalar response type.
func decodeMidjourneyResponse(responseBody []byte) (dto.MidjourneyResponse, error) {
	var response dto.MidjourneyResponse
	if err := common.Unmarshal(responseBody, &response); err == nil {
		return response, nil
	}

	var uploadResponse dto.MidjourneyUploadResponse
	if err := common.Unmarshal(responseBody, &uploadResponse); err != nil {
		return dto.MidjourneyResponse{}, err
	}
	response.Code = uploadResponse.Code
	response.Description = uploadResponse.Description
	if len(uploadResponse.Result) > 0 {
		response.Result = uploadResponse.Result[0]
	}
	return response, nil
}

func DoMidjourneyHttpRequest(c *gin.Context, timeout time.Duration, fullRequestURL string) (*dto.MidjourneyResponseWithStatusCode, []byte, error) {
	var nullBytes []byte
	//var requestBody io.Reader
	//requestBody = c.Request.Body
	// read request body to json, delete accountFilter and notifyHook
	var mapResult map[string]interface{}
	// if get request, no need to read request body
	if c.Request.Method != "GET" {
		err := common.DecodeJson(c.Request.Body, &mapResult)
		if err != nil {
			return MidjourneyErrorWithStatusCodeWrapper(constant.MjErrorUnknown, "read_request_body_failed", http.StatusInternalServerError), nullBytes, err
		}
		if !setting.MjAccountFilterEnabled {
			delete(mapResult, "accountFilter")
		}
		if !setting.MjNotifyEnabled {
			delete(mapResult, "notifyHook")
		}
		//req, err := http.NewRequest(c.Request.Method, fullRequestURL, requestBody)
		// make new request with mapResult
	}
	if setting.MjModeClearEnabled {
		if prompt, ok := mapResult["prompt"].(string); ok {
			prompt = strings.Replace(prompt, "--fast", "", -1)
			prompt = strings.Replace(prompt, "--relax", "", -1)
			prompt = strings.Replace(prompt, "--turbo", "", -1)

			mapResult["prompt"] = prompt
		}
	}
	reqBody, err := common.Marshal(mapResult)
	if err != nil {
		return MidjourneyErrorWithStatusCodeWrapper(constant.MjErrorUnknown, "marshal_request_body_failed", http.StatusInternalServerError), nullBytes, err
	}
	req, err := http.NewRequest(c.Request.Method, fullRequestURL, strings.NewReader(string(reqBody)))
	if err != nil {
		return MidjourneyErrorWithStatusCodeWrapper(constant.MjErrorUnknown, "create_request_failed", http.StatusInternalServerError), nullBytes, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	// 使用带有超时的 context 创建新的请求
	req = req.WithContext(ctx)
	req.Header.Set("Content-Type", c.Request.Header.Get("Content-Type"))
	req.Header.Set("Accept", c.Request.Header.Get("Accept"))
	auth := common.GetContextKeyString(c, constant.ContextKeyChannelKey)
	if auth != "" {
		auth = strings.TrimPrefix(auth, "Bearer ")
		req.Header.Set("mj-api-secret", auth)
	}
	defer cancel()
	resp, err := GetHttpClient().Do(req)
	if err != nil {
		common.SysLog("do request failed: " + err.Error())
		return MidjourneyErrorWithStatusCodeWrapper(constant.MjErrorUnknown, "do_request_failed", http.StatusInternalServerError), nullBytes, err
	}
	statusCode := resp.StatusCode
	//if statusCode != 200  {
	//	return MidjourneyErrorWithStatusCodeWrapper(constant.MjErrorUnknown, "bad_response_status_code", statusCode), nullBytes, nil
	//}
	err = req.Body.Close()
	if err != nil {
		return MidjourneyErrorWithStatusCodeWrapper(constant.MjErrorUnknown, "close_request_body_failed", statusCode), nullBytes, err
	}
	err = c.Request.Body.Close()
	if err != nil {
		return MidjourneyErrorWithStatusCodeWrapper(constant.MjErrorUnknown, "close_request_body_failed", statusCode), nullBytes, err
	}
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return MidjourneyErrorWithStatusCodeWrapper(constant.MjErrorUnknown, "read_response_body_failed", statusCode), nullBytes, err
	}
	CloseResponseBodyGracefully(resp)
	logger.LogDebug(c, "midjourney response body: %s", responseBody)
	if len(responseBody) == 0 {
		return MidjourneyErrorWithStatusCodeWrapper(constant.MjErrorUnknown, "empty_response_body", statusCode), responseBody, nil
	}
	midjResponse, err := decodeMidjourneyResponse(responseBody)
	if err != nil {
		return MidjourneyErrorWithStatusCodeWrapper(constant.MjErrorUnknown, "unmarshal_response_body_failed", statusCode), responseBody, err
	}
	//for k, v := range resp.Header {
	//	c.Writer.Header().Set(k, v[0])
	//}
	return &dto.MidjourneyResponseWithStatusCode{
		StatusCode: statusCode,
		Response:   midjResponse,
	}, responseBody, nil
}
