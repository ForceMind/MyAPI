package controller

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	taskdto "github.com/ForceMind/MyAPI/dto"
	appI18n "github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/logger"
	"github.com/ForceMind/MyAPI/middleware"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/relay"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relay/helper"
	"github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/ForceMind/MyAPI/service"
	"github.com/gin-gonic/gin"
)

func relayTaskDurable(c *gin.Context, opKind string, originID string) {
	c.Header("Cache-Control", "no-store")
	ingressHeader := c.Request.Header.Clone()
	if _, err := service.ParseTaskSubmissionProtocol(c.Request.Header, c.Request.Method, opKind); err != nil {
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": err.Error(),
				"type":    "invalid_request_error",
				"code":    "invalid_idempotency_key",
			},
		})
		return
	}

	relayInfo, err := relaycommon.GenRelayInfo(c, types.RelayFormatTask, nil, nil)
	if err != nil {
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusInternalServerError, &taskdto.TaskError{
			Code:       "gen_relay_info_failed",
			Message:    err.Error(),
			StatusCode: http.StatusInternalServerError,
		})
		return
	}

	if opKind == model.TaskSubmissionOperationKindVideoRemix {
		if taskErr := relay.ResolveOriginTask(c, relayInfo); taskErr != nil {
			respondTaskError(c, taskErr)
			return
		}
	}

	bodyStorage, bodyErr := common.GetBodyStorage(c)
	if bodyErr != nil {
		if common.IsRequestBodyTooLargeError(bodyErr) || errors.Is(bodyErr, common.ErrRequestBodyTooLarge) {
			respondTaskError(c, service.TaskErrorWrapperLocal(bodyErr, "read_request_body_failed", http.StatusRequestEntityTooLarge))
		} else {
			respondTaskError(c, service.TaskErrorWrapperLocal(bodyErr, "read_request_body_failed", http.StatusBadRequest))
		}
		return
	}
	bodyBytes, err := bodyStorage.Bytes()
	if err != nil {
		respondTaskError(c, service.TaskErrorWrapperLocal(err, "read_request_body_failed", http.StatusBadRequest))
		return
	}

	retryParam := &service.RetryParam{
		Ctx:         c,
		TokenGroup:  relayInfo.TokenGroup,
		ModelName:   relayInfo.OriginModelName,
		RequestPath: c.Request.URL.Path,
		Retry:       common.GetPointer(0),
	}

	var channel *model.Channel
	if lockedCh, ok := relayInfo.LockedChannel.(*model.Channel); ok && lockedCh != nil {
		channel = lockedCh
	} else {
		var channelErr *types.NewAPIError
		channel, channelErr = getChannel(c, relayInfo, retryParam)
		if channelErr != nil {
			logger.LogError(c, channelErr.Error())
			respondTaskError(c, service.TaskErrorWrapperLocal(channelErr.Err, "get_channel_failed", http.StatusInternalServerError))
			return
		}
	}
	if channel != nil && channel.Key == "" && channel.Id > 0 {
		if fullCh, err := model.GetChannelById(channel.Id, true); err == nil && fullCh != nil {
			channel = fullCh
		}
	}
	addUsedChannel(c, channel.Id)
	if setupErr := middleware.SetupContextForSelectedChannel(c, channel, relayInfo.OriginModelName); setupErr != nil {
		respondTaskError(c, service.TaskErrorWrapperLocal(setupErr.Err, "setup_channel_failed", http.StatusInternalServerError))
		return
	}

	relayInfo.InitChannelMeta(c)
	platform := constant.TaskPlatform(c.GetString("platform"))
	if platform == "" {
		platform = relay.GetTaskPlatform(c)
	}
	adaptor := relay.GetTaskAdaptor(platform)
	if adaptor == nil {
		respondTaskError(c, service.TaskErrorWrapperLocal(fmt.Errorf("invalid api platform: %s", platform), "invalid_api_platform", http.StatusBadRequest))
		return
	}
	adaptor.Init(relayInfo)

	c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	if taskErr := adaptor.ValidateRequestAndSetAction(c, relayInfo); taskErr != nil {
		respondTaskError(c, taskErr)
		return
	}

	modelName := relayInfo.OriginModelName
	if modelName == "" {
		modelName = service.CoverTaskActionToModelName(platform, relayInfo.Action)
	}
	relayInfo.OriginModelName = modelName
	relayInfo.UpstreamModelName = modelName
	if err := helper.ModelMappedHelper(c, relayInfo, nil); err != nil {
		respondTaskError(c, service.TaskErrorWrapperLocal(err, "model_mapping_failed", http.StatusBadRequest))
		return
	}
	relayInfo.OriginModelName = modelName
	priceData, err := helper.ModelPriceHelperPerCall(c, relayInfo)
	if err != nil {
		respondTaskError(c, service.TaskErrorWrapper(err, "model_price_error", http.StatusBadRequest))
		return
	}
	relayInfo.PriceData = priceData
	if estimatedRatios := adaptor.EstimateBilling(c, relayInfo); len(estimatedRatios) > 0 {
		for k, v := range estimatedRatios {
			relayInfo.PriceData.AddOtherRatio(k, v)
		}
	}
	estimatedQuota := relayInfo.PriceData.Quota
	if !common.StringsContains(constant.TaskPricePatches, modelName) {
		quotaWithRatios := relayInfo.PriceData.ApplyOtherRatiosToFloat(float64(relayInfo.PriceData.Quota))
		quota, clamp := common.QuotaFromFloatChecked(quotaWithRatios)
		estimatedQuota = quota
		relayInfo.PriceData.Quota = quota
		if clamp != nil {
			relayInfo.QuotaClamp = clamp
		}
	}
	if rejectDurableTaskQuotaClamp(c, relayInfo) {
		return
	}

	provider := adaptor.GetChannelName()
	if provider == "" {
		provider = string(platform)
	}
	if provider == "" {
		provider = channel.Name
	}

	billingContext := model.NewTaskBillingContext(relayInfo)
	dispatcher := func(ctx context.Context, op *model.TaskSubmissionOperation, attempt *model.TaskSubmissionAttempt) (*service.TaskProviderDispatchResult, error) {
		relayInfo.BillingSource = op.BillingSource
		relayInfo.SubscriptionId = op.SubscriptionID
		c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		requestBody, err := adaptor.BuildRequestBody(c, relayInfo)
		if err != nil {
			return &service.TaskProviderDispatchResult{
				Status:       "rejected",
				ErrorCode:    "build_request_failed",
				ErrorMessage: err.Error(),
			}, nil
		}
		resp, err := adaptor.DoRequest(c, relayInfo, requestBody)
		if err != nil {
			return &service.TaskProviderDispatchResult{
				Status:       "unknown",
				ErrorCode:    "do_request_failed",
				ErrorMessage: err.Error(),
			}, nil
		}
		upstreamTaskID, taskData, taskErr := relay.ResolveLegacyTaskSubmitResponse(c, adaptor, resp, relayInfo)
		if taskErr != nil {
			if taskErr.StatusCode >= 400 && taskErr.StatusCode < 500 {
				return &service.TaskProviderDispatchResult{
					Status:       "rejected",
					ErrorCode:    taskErr.Code,
					ErrorMessage: taskErr.Message,
				}, nil
			}
			return &service.TaskProviderDispatchResult{
				Status:       "unknown",
				ErrorCode:    taskErr.Code,
				ErrorMessage: taskErr.Message,
			}, nil
		}

		task := model.InitTask(platform, relayInfo)
		task.TaskID = op.PublicID
		task.PrivateData.UpstreamTaskID = upstreamTaskID
		task.PrivateData.BillingPreference = op.BillingPreference
		task.PrivateData.BillingSource = op.BillingSource
		task.PrivateData.FreeModel = op.FreeModel
		task.PrivateData.SubscriptionId = op.SubscriptionID
		task.PrivateData.TokenId = relayInfo.TokenId
		task.PrivateData.NodeName = common.NodeName
		billingContextCopy := *billingContext
		task.PrivateData.BillingContext = &billingContextCopy
		task.Quota = service.TaskInitialQuota(op.BillingVersion, op.EstimatedQuota, op.ReservedQuota)
		task.Data = taskData
		task.Action = relayInfo.Action

		return &service.TaskProviderDispatchResult{
			Status:         "accepted",
			ProviderTaskID: upstreamTaskID,
			TaskCandidate:  task,
		}, nil
	}

	requestID := c.GetString(common.RequestIdKey)
	if requestID == "" {
		requestID = relayInfo.RequestId
	}
	ingressReq := service.TaskIngressRequest{
		UserID:             relayInfo.UserId,
		TokenID:            relayInfo.TokenId,
		ChannelID:          channel.Id,
		Provider:           provider,
		OperationKind:      opKind,
		OriginTaskPublicID: originID,
		HTTPMethod:         c.Request.Method,
		Header:             ingressHeader,
		ContentType:        c.Request.Header.Get("Content-Type"),
		Body:               bodyBytes,
		RequestID:          requestID,
		EstimatedQuota:     estimatedQuota,
		FreeModel:          relayInfo.PriceData.FreeModel,
		BillingContext:     *billingContext,
		InitialQuotaClamp:  relayInfo.QuotaClamp,
		Dispatcher:         dispatcher,
		DB:                 model.DB,
	}

	result, ingressErr := service.ExecuteTaskIngress(c.Request.Context(), ingressReq)
	if ingressErr != nil {
		c.Header("Cache-Control", "no-store")
		if errors.Is(ingressErr, model.ErrTaskQuotaReservationInsufficientQuota) {
			c.JSON(http.StatusForbidden, gin.H{
				"error": gin.H{
					"message": "用户额度不足",
					"type":    "insufficient_quota",
					"code":    "insufficient_quota",
				},
			})
			return
		}
		if errors.Is(ingressErr, service.ErrTaskSubmissionProtocolIdempotencyInvalid) ||
			errors.Is(ingressErr, service.ErrTaskSubmissionProtocolIdempotencyMissing) ||
			errors.Is(ingressErr, service.ErrTaskSubmissionProtocolIdempotencyMultiple) ||
			errors.Is(ingressErr, service.ErrTaskSubmissionProtocolIdempotencyAlias) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": gin.H{
					"message": ingressErr.Error(),
					"type":    "invalid_request_error",
					"code":    "invalid_idempotency_key",
				},
			})
			return
		}
		if errors.Is(ingressErr, service.ErrTaskSubmissionFingerprintContentType) ||
			errors.Is(ingressErr, service.ErrTaskSubmissionFingerprintBody) ||
			errors.Is(ingressErr, service.ErrTaskSubmissionFingerprintRoute) ||
			errors.Is(ingressErr, service.ErrTaskSubmissionFingerprintProtocol) ||
			errors.Is(ingressErr, service.ErrTaskSubmissionProtocolMethod) ||
			errors.Is(ingressErr, service.ErrTaskSubmissionProtocolOperationKind) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": gin.H{
					"message": ingressErr.Error(),
					"type":    "invalid_request_error",
					"code":    "invalid_request",
				},
			})
			return
		}
		respondTaskError(c, service.TaskErrorWrapper(ingressErr, "ingress_failed", http.StatusInternalServerError))
		return
	}

	if result.Conflict {
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusConflict, gin.H{
			"error": gin.H{
				"message": result.ErrorMsg,
				"type":    "idempotency_conflict",
				"code":    result.ErrorCode,
			},
		})
		return
	}

	if result.Response != nil {
		c.Header("Location", fmt.Sprintf("/v1/task-operations/%s", result.Response.ID))
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusAccepted, result.Response)
		return
	}

	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusAccepted, gin.H{"status": "accepted"})
}

func rejectDurableTaskQuotaClamp(c *gin.Context, relayInfo *relaycommon.RelayInfo) bool {
	if relayInfo == nil || relayInfo.QuotaClamp == nil {
		return false
	}
	clamp := relayInfo.QuotaClamp
	logger.LogWarn(c, fmt.Sprintf("durable task quota rejected before dispatch: op=%s kind=%s clamped=%d user=%d model=%s",
		clamp.Op, clamp.Kind, clamp.Clamped, relayInfo.UserId, relayInfo.OriginModelName))
	adminInfo := map[string]interface{}{
		"quota_saturation": clamp.AuditMap(),
		"request_id":       c.GetString(common.RequestIdKey),
		"model":            relayInfo.OriginModelName,
	}
	auditDB := model.LOG_DB
	if auditDB != nil {
		channelID := 0
		if relayInfo.ChannelMeta != nil {
			channelID = relayInfo.ChannelMeta.ChannelId
		}
		if err := model.CreateLog(auditDB, &model.Log{
			UserId: relayInfo.UserId, Username: c.GetString("username"), CreatedAt: common.GetTimestamp(),
			Type: model.LogTypeSystem, Content: "task request rejected before dispatch: quota saturation",
			ModelName: relayInfo.OriginModelName, TokenId: relayInfo.TokenId, ChannelId: channelID,
			RequestId: c.GetString(common.RequestIdKey), Other: common.MapToJsonStr(map[string]interface{}{"admin_info": adminInfo}),
		}); err != nil {
			logger.LogError(c, "failed to persist durable task quota clamp audit: "+err.Error())
		}
	}
	respondTaskError(c, service.TaskErrorWrapperLocal(errors.New(appI18n.T(c, appI18n.MsgTaskQuotaOutOfRange)), "quota_out_of_range", http.StatusBadRequest))
	return true
}
