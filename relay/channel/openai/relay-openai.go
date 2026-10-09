package openai

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/logger"
	"github.com/ForceMind/MyAPI/relay/channel/openrouter"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	relayconstant "github.com/ForceMind/MyAPI/relay/constant"
	"github.com/ForceMind/MyAPI/relay/helper"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/ForceMind/MyAPI/relaykit/relayconvert"
	"github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/ForceMind/MyAPI/service"

	"github.com/gin-gonic/gin"
)

func sendStreamData(c *gin.Context, info *relaycommon.RelayInfo, data string, forceFormat bool, thinkToContent bool) error {
	if data == "" {
		return nil
	}

	if !forceFormat && !thinkToContent {
		return helper.StringData(c, data)
	}

	var lastStreamResponse dto.ChatCompletionsStreamResponse
	if err := common.UnmarshalJsonStr(data, &lastStreamResponse); err != nil {
		return err
	}

	if !thinkToContent {
		return helper.ObjectData(c, lastStreamResponse)
	}

	hasThinkingContent := false
	hasContent := false
	var thinkingContent strings.Builder
	for _, choice := range lastStreamResponse.Choices {
		if len(choice.Delta.GetReasoningContent()) > 0 {
			hasThinkingContent = true
			thinkingContent.WriteString(choice.Delta.GetReasoningContent())
		}
		if len(choice.Delta.GetContentString()) > 0 {
			hasContent = true
		}
	}

	// Handle think to content conversion
	if info.ThinkingContentInfo.IsFirstThinkingContent {
		if hasThinkingContent {
			response := lastStreamResponse.Copy()
			for i := range response.Choices {
				// send `think` tag with thinking content
				response.Choices[i].Delta.SetContentString("<think>\n" + thinkingContent.String())
				response.Choices[i].Delta.ReasoningContent = nil
				response.Choices[i].Delta.Reasoning = nil
			}
			info.ThinkingContentInfo.IsFirstThinkingContent = false
			info.ThinkingContentInfo.HasSentThinkingContent = true
			return helper.ObjectData(c, response)
		}
	}

	if lastStreamResponse.Choices == nil || len(lastStreamResponse.Choices) == 0 {
		return helper.ObjectData(c, lastStreamResponse)
	}

	// Process each choice
	for i, choice := range lastStreamResponse.Choices {
		// Handle transition from thinking to content
		// only send `</think>` tag when previous thinking content has been sent
		if hasContent && !info.ThinkingContentInfo.SendLastThinkingContent && info.ThinkingContentInfo.HasSentThinkingContent {
			response := lastStreamResponse.Copy()
			for j := range response.Choices {
				response.Choices[j].Delta.SetContentString("\n</think>\n")
				response.Choices[j].Delta.ReasoningContent = nil
				response.Choices[j].Delta.Reasoning = nil
			}
			info.ThinkingContentInfo.SendLastThinkingContent = true
			helper.ObjectData(c, response)
		}

		// Convert reasoning content to regular content if any
		if len(choice.Delta.GetReasoningContent()) > 0 {
			lastStreamResponse.Choices[i].Delta.SetContentString(choice.Delta.GetReasoningContent())
			lastStreamResponse.Choices[i].Delta.ReasoningContent = nil
			lastStreamResponse.Choices[i].Delta.Reasoning = nil
		} else if !hasThinkingContent && !hasContent {
			// flush thinking content
			lastStreamResponse.Choices[i].Delta.ReasoningContent = nil
			lastStreamResponse.Choices[i].Delta.Reasoning = nil
		}
	}

	return helper.ObjectData(c, lastStreamResponse)
}

func OaiStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		logger.LogError(c, "invalid response or response body")
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}

	defer service.CloseResponseBodyGracefully(resp)
	if info.StrictTokenBudget && info.RelayMode == relayconstant.RelayModeChatCompletions {
		original := c.Writer
		observed := &strictChatWriteObserver{ResponseWriter: original, requestContext: c.Request.Context()}
		c.Writer = observed
		defer func() {
			c.Writer = original
			// StringData can skip writes entirely after cancellation. Require a
			// complete terminal write before ignoring that late cancellation;
			// upstream completion and native usage are validated separately.
			interrupted := c.Request.Context().Err() != nil && !observed.doneWritten.Load()
			if (observed.failed.Load() || interrupted) && info.StreamStatus != nil {
				info.StreamStatus.RecordError("strict Chat downstream write failed")
			}
		}()
	}

	model := info.UpstreamModelName
	var responseId string
	var createAt int64 = 0
	var systemFingerprint string
	var containStreamUsage bool
	var responseTextBuilder strings.Builder
	var toolCount int
	var usage = &dto.Usage{}
	var lastStreamData string
	var usageStreamData string
	var secondLastStreamData string
	seenStreamToolCalls := make(map[string]struct{})
	var streamFunctionCallNames []string
	var strictEvidence strictChatStreamEvidence
	var sensitiveStreamError *types.NewAPIError

	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
		if lastStreamData != "" {
			if err := HandleStreamFormat(c, info, lastStreamData, info.ChannelSetting.ForceFormat, info.ChannelSetting.ThinkingToContent); err != nil {
				if common.SensitiveRequestDiagnostics(c) {
					logger.LogError(c, "error handling stream format: "+err.Error())
				} else {
					common.SysLog("error handling stream format: " + err.Error())
				}
				sr.Error(err)
			}
		}
		if len(data) > 0 {
			if common.SensitiveRequestDiagnostics(c) {
				if sensitiveStreamError = sensitiveOpenAIResponseError(common.StringToByteSlice(data), resp.StatusCode); sensitiveStreamError != nil {
					// Keep provider details only for in-memory classification;
					// neither forward them nor persist them in StreamStatus.
					lastStreamData = ""
					sr.Stop(service.SafeRelayError(c, sensitiveStreamError))
					return
				}
			}
			if info.StrictTokenBudget {
				strictEvidence.observe(common.StringToByteSlice(data), info.UpstreamModelName)
			}
			secondLastStreamData = lastStreamData
			lastStreamData = data
			collectStreamFunctionCallNames(data, seenStreamToolCalls, &streamFunctionCallNames)
			if err := processTokenData(info.RelayMode, data, &responseTextBuilder, &toolCount); err != nil {
				logger.LogError(c, "error processing stream token data: "+err.Error())
				if common.SensitiveRequestDiagnostics(c) {
					sensitiveStreamError = sensitiveOpenAIProtocolError(err, resp.StatusCode)
					lastStreamData = ""
					sr.Stop(service.SafeRelayError(c, sensitiveStreamError))
					return
				}
				sr.Error(err)
			}
		}
	})

	if sensitiveStreamError != nil {
		// The outer dispatch finalizer retains unknown usage. Do not synthesize
		// usage, emit [DONE], settle, refund, or retry an interrupted stream.
		return nil, sensitiveStreamError
	}

	// Keep the existing terminal boundary: Chat's last chunk, or the audio
	// protocol's penultimate usage. Arbitrary intermediate usage is not final.
	usageStreamData = lastStreamData
	if strings.Contains(strings.ToLower(model), "audio") && secondLastStreamData != "" {
		var chunk struct {
			Usage *dto.Usage `json:"usage"`
		}
		if common.UnmarshalJsonStr(secondLastStreamData, &chunk) == nil && chunk.Usage != nil {
			usage, usageStreamData, containStreamUsage = chunk.Usage, secondLastStreamData, true
		}
	}
	var finalChunk struct {
		Usage *dto.Usage `json:"usage"`
	}
	if common.UnmarshalJsonStr(lastStreamData, &finalChunk) == nil && finalChunk.Usage != nil {
		usageStreamData = lastStreamData
	}

	// 处理最后的响应
	shouldSendLastResp := true
	if err := handleLastResponse(lastStreamData, &responseId, &createAt, &systemFingerprint, &model, &usage,
		&containStreamUsage, info, &shouldSendLastResp); err != nil {
		logger.LogError(c, fmt.Sprintf("error handling last response: %s, lastStreamData: [%s]", err.Error(), lastStreamData))
	}

	if info.RelayFormat == types.RelayFormatOpenAI {
		if shouldSendLastResp {
			_ = sendStreamData(c, info, lastStreamData, info.ChannelSetting.ForceFormat, info.ChannelSetting.ThinkingToContent)
		}
	}

	if !containStreamUsage {
		usage = service.ResponseText2Usage(c, responseTextBuilder.String(), info.UpstreamModelName, info.GetEstimatePromptTokens())
		usage.CompletionTokens += toolCount * 7
	}

	applyUsagePostProcessing(info, usage, common.StringToByteSlice(lastStreamData))
	if containStreamUsage {
		captureChatUsageEvidence(usage, common.StringToByteSlice(usageStreamData))
	} else {
		usage.BillingUsage = dto.CloneBillingUsage(&dto.BillingUsage{Source: dto.BillingUsageSourceOAIChat, Semantic: dto.BillingUsageSemanticOpenAI, Estimated: true, OpenAIUsage: usage})
	}

	if info.StrictTokenBudget && usage.BillingUsage != nil && (strictEvidence.invalid || !strictEvidence.seenUsage) {
		usage.BillingUsage.ChatTextEvidence = nil
	}
	for _, name := range streamFunctionCallNames {
		info.CountBillableToolCall(dto.BuildInCallFunctionCall, name)
	}

	clientUsage := *usage
	clientUsage.BillingUsage = nil
	HandleFinalResponse(c, info, lastStreamData, responseId, createAt, model, systemFingerprint, &clientUsage, containStreamUsage)

	return usage, nil
}

func collectStreamFunctionCallNames(data string, seen map[string]struct{}, names *[]string) {
	var streamResponse dto.ChatCompletionsStreamResponse
	if err := common.UnmarshalJsonStr(data, &streamResponse); err != nil {
		return
	}
	for _, choice := range streamResponse.Choices {
		for i, tc := range choice.Delta.ToolCalls {
			name := tc.Function.Name
			if name == "" {
				continue
			}
			toolIdx := i
			if tc.Index != nil {
				toolIdx = *tc.Index
			}
			key := fmt.Sprintf("%d-%d", choice.Index, toolIdx)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			*names = append(*names, name)
		}
	}
}

// sensitiveOpenAIProtocolError marks an accepted but unreadable response as
// terminal without treating missing usage as a refund or retry opportunity.
func sensitiveOpenAIProtocolError(err error, upstreamStatus int) *types.NewAPIError {
	result := types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusBadGateway, types.ErrOptionWithSkipRetry())
	result.UpstreamStatusCode = upstreamStatus
	return result
}

// sensitiveOpenAIResponseError inspects only the error envelope, independent of
// unrelated fields and before any raw response is forwarded. Ambiguous keys,
// invalid JSON and non-object envelopes fail closed on sensitive requests.
func sensitiveOpenAIResponseError(body []byte, upstreamStatus int) *types.NewAPIError {
	validationErr := common.ValidateUniqueJSONKeys(body)
	var fields map[string]json.RawMessage
	if validationErr == nil {
		validationErr = common.Unmarshal(body, &fields)
	}
	protocolFields := make(map[string]json.RawMessage)
	if validationErr == nil {
		for key, value := range fields {
			canonical := strings.ToLower(key)
			switch canonical {
			case "error", "type", "response":
				if _, exists := protocolFields[canonical]; exists {
					validationErr = fmt.Errorf("ambiguous upstream error envelope")
				}
				protocolFields[canonical] = value
			}
		}
	}
	errorField := protocolFields["error"]
	var eventType string
	if typeField, exists := protocolFields["type"]; validationErr == nil && exists {
		if common.GetJsonType(typeField) != "string" || common.Unmarshal(typeField, &eventType) != nil {
			validationErr = fmt.Errorf("invalid upstream event type")
		} else if eventType == "error" {
			// Responses can use a flat error event without an error wrapper.
			errorField = body
		}
	}
	if validationErr == nil && strings.HasPrefix(eventType, "response.") && eventType != "response.failed" && eventType != "response.error" {
		if responseField, exists := protocolFields["response"]; exists {
			if nestedError := sensitiveOpenAIResponseError(responseField, upstreamStatus); nestedError != nil {
				return nestedError
			}
		}
	}
	var result *types.NewAPIError
	if validationErr != nil || fields == nil {
		result = types.NewOpenAIError(fmt.Errorf("invalid upstream response envelope"), types.ErrorCodeBadResponseBody, http.StatusBadGateway)
	} else if len(errorField) == 0 || common.GetJsonType(errorField) == "null" {
		return nil
	} else {
		var providerError types.OpenAIError
		if common.GetJsonType(errorField) == "object" && common.Unmarshal(errorField, &providerError) == nil {
			result = types.WithOpenAIError(providerError, http.StatusBadGateway)
		} else {
			// String errors may carry provider markers needed by billing policy;
			// retain them only on the in-memory error, never in diagnostics.
			message := "invalid upstream error envelope"
			if common.GetJsonType(errorField) == "string" {
				_ = common.Unmarshal(errorField, &message)
			}
			result = types.NewOpenAIError(fmt.Errorf("%s", message), types.ErrorCodeBadResponse, http.StatusBadGateway)
		}
	}
	result.UpstreamStatusCode = upstreamStatus
	types.ErrOptionWithSkipRetry()(result)
	return result
}

func OpenaiHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	var simpleResponse dto.OpenAITextResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	logger.LogDebug(c, "upstream response body: %s", responseBody)
	// Unmarshal to simpleResponse
	if info.ChannelType == constant.ChannelTypeOpenRouter && info.ChannelOtherSettings.IsOpenRouterEnterprise() {
		// 尝试解析为 openrouter enterprise
		var enterpriseResponse openrouter.OpenRouterEnterpriseResponse
		err = common.Unmarshal(responseBody, &enterpriseResponse)
		if err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
		if enterpriseResponse.Success {
			responseBody = enterpriseResponse.Data
		} else {
			logger.LogError(c, fmt.Sprintf("openrouter enterprise response success=false, data: %s", enterpriseResponse.Data))
			return nil, types.NewOpenAIError(fmt.Errorf("openrouter response success=false"), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
	}

	if common.SensitiveRequestDiagnostics(c) {
		if responseError := sensitiveOpenAIResponseError(responseBody, resp.StatusCode); responseError != nil {
			return nil, responseError
		}
	}

	err = common.Unmarshal(responseBody, &simpleResponse)
	if err != nil {
		if common.SensitiveRequestDiagnostics(c) {
			return nil, sensitiveOpenAIProtocolError(err, resp.StatusCode)
		}
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	if oaiError := simpleResponse.GetOpenAIError(); oaiError != nil && oaiError.Type != "" {
		return nil, types.WithOpenAIError(*oaiError, resp.StatusCode)
	}

	for _, choice := range simpleResponse.Choices {
		if choice.FinishReason == constant.FinishReasonContentFilter {
			common.SetContextKey(c, constant.ContextKeyAdminRejectReason, "openai_finish_reason=content_filter")
			break
		}
	}

	for _, choice := range simpleResponse.Choices {
		for _, tc := range choice.Message.ParseToolCalls() {
			info.CountBillableToolCall(dto.BuildInCallFunctionCall, tc.Function.Name)
		}
	}

	forceFormat := false
	if info.ChannelSetting.ForceFormat {
		forceFormat = true
	}

	usageModified := false
	if !captureChatUsageEvidence(&simpleResponse.Usage, responseBody) {
		completionTokens := simpleResponse.Usage.CompletionTokens
		if completionTokens == 0 {
			for _, choice := range simpleResponse.Choices {
				ctkm := service.CountTextToken(choice.Message.StringContent()+choice.Message.GetReasoningContent(), info.UpstreamModelName)
				completionTokens += ctkm
			}
		}
		simpleResponse.Usage = dto.Usage{
			PromptTokens:     info.GetEstimatePromptTokens(),
			CompletionTokens: completionTokens,
			TotalTokens:      info.GetEstimatePromptTokens() + completionTokens,
		}
		usageModified = true
	}

	applyUsagePostProcessing(info, &simpleResponse.Usage, responseBody)
	if usageModified {
		simpleResponse.Usage.BillingUsage = dto.CloneBillingUsage(&dto.BillingUsage{Source: dto.BillingUsageSourceOAIChat, Semantic: dto.BillingUsageSemanticOpenAI, Estimated: true, OpenAIUsage: &simpleResponse.Usage})
	} else {
		captureChatUsageEvidence(&simpleResponse.Usage, responseBody)
	}
	billingUsage := simpleResponse.Usage.BillingUsage
	simpleResponse.Usage.BillingUsage = nil

	switch info.RelayFormat {
	case types.RelayFormatOpenAI:
		if usageModified {
			var bodyMap map[string]interface{}
			err = common.Unmarshal(responseBody, &bodyMap)
			if err != nil {
				return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
			}
			bodyMap["usage"] = simpleResponse.Usage
			responseBody, _ = common.Marshal(bodyMap)
		}
		if forceFormat {
			responseBody, err = common.Marshal(simpleResponse)
			if err != nil {
				return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
			}
		} else {
			break
		}
	case types.RelayFormatClaude:
		convertResult, err := relayconvert.ConvertResponse(c, info, types.RelayFormatClaude, &simpleResponse)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
		}
		claudeRespStr, err := common.Marshal(convertResult.Value)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
		}
		responseBody = claudeRespStr
	case types.RelayFormatGemini:
		convertResult, err := relayconvert.ConvertResponse(c, info, types.RelayFormatGemini, &simpleResponse)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
		}
		geminiRespStr, err := common.Marshal(convertResult.Value)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
		}
		responseBody = geminiRespStr
	}

	service.IOCopyBytesGracefully(c, resp, responseBody)

	simpleResponse.Usage.BillingUsage = billingUsage
	return &simpleResponse.Usage, nil
}
