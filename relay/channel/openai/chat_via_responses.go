package openai

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/logger"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relay/helper"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/ForceMind/MyAPI/relaykit/relayconvert"
	"github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/ForceMind/MyAPI/service"

	"github.com/gin-gonic/gin"
)

func OaiResponsesToChatHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}

	defer service.CloseResponseBodyGracefully(resp)

	var responsesResp dto.OpenAIResponsesResponse
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}

	if common.SensitiveRequestDiagnostics(c) {
		if responseError := sensitiveOpenAIResponseError(body, resp.StatusCode); responseError != nil {
			return nil, responseError
		}
	}

	normalizedBody, observedUsage, err := normalizeResponsesChatUsage(body, false)
	if err == nil {
		err = common.Unmarshal(normalizedBody, &responsesResp)
	}
	if err != nil {
		if common.SensitiveRequestDiagnostics(c) {
			return nil, sensitiveOpenAIProtocolError(err, resp.StatusCode)
		}
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	if oaiError := responsesResp.GetOpenAIError(); oaiError != nil && oaiError.Type != "" {
		return nil, types.WithOpenAIError(*oaiError, resp.StatusCode)
	}
	sanitizeResponsesChatAccounting(responsesResp.Usage)

	chatResult, err := relayconvert.ConvertResponse(c, info, types.RelayFormatOpenAI, &responsesResp)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	chatResp, ok := chatResult.Value.(*dto.OpenAITextResponse)
	if !ok {
		return nil, types.NewOpenAIError(fmt.Errorf("expected OpenAI chat response, got %T", chatResult.Value), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if chatID := helper.GetResponseID(c); chatID != "" {
		chatResp.Id = chatID
	}
	usage := chatResult.Usage

	upstreamUsage := usage != nil && usage.TotalTokens != 0
	if !upstreamUsage {
		text := service.ExtractOutputTextFromResponses(&responsesResp)
		usage = service.ResponseText2Usage(c, text, info.UpstreamModelName, info.GetEstimatePromptTokens())
		chatResp.Usage = *usage
	}

	responseValue := any(chatResp)
	if info.RelayFormat != types.RelayFormatOpenAI {
		targetResult, err := relayconvert.ConvertResponse(c, info, info.RelayFormat, chatResp)
		if err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
		responseValue = targetResult.Value
	}
	responseBody, err := observedUsage.clientBody(responseValue, info.RelayFormat == types.RelayFormatOpenAI)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
	}

	// Cache-field presence is recorded only after the client body is marshalled:
	// chatResp shares this usage envelope and internal evidence must not be sent.
	if upstreamUsage && usage.BillingUsage != nil {
		observedUsage.captureEvidence(usage)
	}

	// The body has been converted from a Responses payload into Chat
	// Completions JSON, so do not leak an absent or stale upstream content type.
	resp.Header.Set("Content-Type", "application/json; charset=utf-8")
	service.IOCopyBytesGracefully(c, resp, responseBody)
	return usage, nil
}

func OaiResponsesToChatBufferedStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	defer service.CloseResponseBodyGracefully(resp)

	accumulator := relayconvert.NewResponsesBufferedAccumulator()
	var finalResponse *dto.OpenAIResponsesResponse
	var observedUsage responsesChatUsage
	var streamErr *types.NewAPIError

	scanner := helper.NewStreamScanner(resp.Body)
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) < 6 || line[:5] != "data:" {
			continue
		}
		data := line[5:]
		data = strings.TrimSpace(data)
		if data == "" || data == "[DONE]" {
			if data == "[DONE]" {
				break
			}
			continue
		}

		if common.SensitiveRequestDiagnostics(c) {
			if streamErr = sensitiveOpenAIResponseError(common.StringToByteSlice(data), resp.StatusCode); streamErr != nil {
				break
			}
		}

		var streamResp dto.ResponsesStreamResponse
		normalized, observed, decodeErr := normalizeResponsesChatUsage(common.StringToByteSlice(data), true)
		if decodeErr == nil {
			decodeErr = common.Unmarshal(normalized, &streamResp)
		}
		if err := decodeErr; err != nil {
			logger.LogError(c, "failed to unmarshal buffered responses stream event: "+err.Error())
			if common.SensitiveRequestDiagnostics(c) {
				streamErr = sensitiveOpenAIProtocolError(err, resp.StatusCode)
			} else {
				streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
			}
			break
		}
		if streamResp.Response != nil {
			sanitizeResponsesChatAccounting(streamResp.Response.Usage)
			if streamResp.Type != "response.completed" && streamResp.Type != "response.done" && streamResp.Type != "response.incomplete" {
				streamResp.Response.Usage = nil
			}
		}
		accumulator.ProcessEvent(&streamResp)
		switch streamResp.Type {
		case "response.completed", "response.done", "response.incomplete":
			var previousUsage *dto.Usage
			previousResponse := finalResponse
			if finalResponse != nil {
				previousUsage = finalResponse.Usage
			}
			if streamResp.Response != nil {
				finalResponse = streamResp.Response
				if previousResponse != nil {
					if finalResponse.ID == "" {
						finalResponse.ID = previousResponse.ID
					}
					if finalResponse.Model == "" {
						finalResponse.Model = previousResponse.Model
					}
					if finalResponse.CreatedAt == 0 {
						finalResponse.CreatedAt = previousResponse.CreatedAt
					}
				}
			}
			if observed.present {
				observedUsage = observed
			}
			if finalResponse != nil && finalResponse.Usage == nil {
				finalResponse.Usage = previousUsage
			}
			if streamResp.Type == "response.incomplete" {
				if finalResponse == nil {
					finalResponse = &dto.OpenAIResponsesResponse{}
				}
				if len(finalResponse.Status) == 0 {
					finalResponse.Status = []byte(`"incomplete"`)
				}
			}
		case "response.failed", "response.error":
			if streamResp.Response != nil {
				if oaiErr := streamResp.Response.GetOpenAIError(); oaiErr != nil && oaiErr.Type != "" {
					streamErr = types.WithOpenAIError(*oaiErr, http.StatusInternalServerError)
					break
				}
			}
			streamErr = types.NewOpenAIError(fmt.Errorf("responses stream error: %s", streamResp.Type), types.ErrorCodeBadResponse, http.StatusInternalServerError)
		}
		if streamErr != nil {
			break
		}
	}
	if streamErr != nil {
		return nil, streamErr
	}
	if err := scanner.Err(); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	if finalResponse == nil {
		finalResponse = &dto.OpenAIResponsesResponse{
			ID:        helper.GetResponseID(c),
			CreatedAt: int(time.Now().Unix()),
			Model:     info.UpstreamModelName,
			Status:    []byte(`"completed"`),
		}
	}
	accumulator.SupplementResponseOutput(finalResponse)

	chatResult, err := relayconvert.ConvertResponse(c, info, types.RelayFormatOpenAI, finalResponse)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	chatResp, ok := chatResult.Value.(*dto.OpenAITextResponse)
	if !ok {
		return nil, types.NewOpenAIError(fmt.Errorf("expected OpenAI chat response, got %T", chatResult.Value), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if chatID := helper.GetResponseID(c); chatID != "" {
		chatResp.Id = chatID
	}
	usage := chatResult.Usage
	upstreamUsage := usage != nil && usage.TotalTokens != 0
	if !upstreamUsage {
		text := service.ExtractOutputTextFromResponses(finalResponse)
		usage = service.ResponseText2Usage(c, text, info.UpstreamModelName, info.GetEstimatePromptTokens())
		chatResp.Usage = *usage
	}

	responseValue := any(chatResp)
	if info.RelayFormat != types.RelayFormatOpenAI {
		targetResult, err := relayconvert.ConvertResponse(c, info, info.RelayFormat, chatResp)
		if err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
		responseValue = targetResult.Value
	}
	responseBody, err := observedUsage.clientBody(responseValue, info.RelayFormat == types.RelayFormatOpenAI)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
	}

	// Recorded only after the client body is marshalled: chatResp shares this
	// usage envelope and internal evidence must not be sent.
	if upstreamUsage && usage.BillingUsage != nil {
		observedUsage.captureEvidence(usage)
	}

	// Buffered SSE is emitted to the client as one Chat Completions JSON body.
	resp.Header.Set("Content-Type", "application/json; charset=utf-8")
	service.IOCopyBytesGracefully(c, resp, responseBody)
	return usage, nil
}

func OaiResponsesToChatStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}

	defer service.CloseResponseBodyGracefully(resp)

	responseId := helper.GetResponseID(c)
	createAt := time.Now().Unix()
	state, err := relayconvert.NewResponseStreamState(types.RelayFormatOpenAIResponses, info.RelayFormat, relayconvert.ResponseStreamOptions{
		ID:      responseId,
		Model:   info.UpstreamModelName,
		Created: createAt,
	})
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	streamErr := (*types.NewAPIError)(nil)
	observedUsage := responsesChatUsage{}

	if info.RelayFormat == types.RelayFormatClaude && info.ClaudeConvertInfo == nil {
		info.ClaudeConvertInfo = &relaycommon.ClaudeConvertInfo{LastMessagesType: relaycommon.LastMessageTypeNone}
	}

	sendGeminiResponse := func(geminiResponse *dto.GeminiChatResponse) bool {
		if geminiResponse == nil {
			return true
		}
		geminiResponseStr, err := common.Marshal(geminiResponse)
		if err != nil {
			streamErr = types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
			return false
		}
		c.Render(-1, common.CustomEvent{Data: "data: " + string(geminiResponseStr)})
		_ = helper.FlushWriter(c)
		return true
	}

	sendStreamResult := func(result relayconvert.ResponseResult) bool {
		switch value := result.Value.(type) {
		case dto.ChatCompletionsStreamResponse:
			if len(value.Choices) == 0 && value.Usage == nil {
				return true
			}
			body, err := observedUsage.clientBody(&value, true)
			if err == nil {
				err = helper.StringData(c, string(body))
			}
			if err != nil {
				streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
				return false
			}
			return true
		case *dto.ChatCompletionsStreamResponse:
			if value == nil || (len(value.Choices) == 0 && value.Usage == nil) {
				return true
			}
			body, err := observedUsage.clientBody(value, true)
			if err == nil {
				err = helper.StringData(c, string(body))
			}
			if err != nil {
				streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
				return false
			}
			return true
		case dto.ClaudeResponse:
			if err := helper.ClaudeData(c, value); err != nil {
				streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
				return false
			}
			return true
		case *dto.ClaudeResponse:
			if value == nil {
				return true
			}
			if err := helper.ClaudeData(c, *value); err != nil {
				streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
				return false
			}
			return true
		case dto.GeminiChatResponse:
			return sendGeminiResponse(&value)
		case *dto.GeminiChatResponse:
			return sendGeminiResponse(value)
		default:
			streamErr = types.NewOpenAIError(fmt.Errorf("unsupported converted stream response type %T", result.Value), types.ErrorCodeBadResponse, http.StatusInternalServerError)
			return false
		}
	}

	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
		if streamErr != nil {
			sr.Stop(service.SafeRelayError(c, streamErr))
			return
		}

		if common.SensitiveRequestDiagnostics(c) {
			if streamErr = sensitiveOpenAIResponseError(common.StringToByteSlice(data), resp.StatusCode); streamErr != nil {
				sr.Stop(service.SafeRelayError(c, streamErr))
				return
			}
		}

		var streamResp dto.ResponsesStreamResponse
		normalized, observed, decodeErr := normalizeResponsesChatUsage(common.StringToByteSlice(data), true)
		if decodeErr == nil {
			decodeErr = common.Unmarshal(normalized, &streamResp)
		}
		if err := decodeErr; err != nil {
			logger.LogError(c, "failed to unmarshal responses stream event: "+err.Error())
			if common.SensitiveRequestDiagnostics(c) {
				streamErr = sensitiveOpenAIProtocolError(err, resp.StatusCode)
				sr.Stop(service.SafeRelayError(c, streamErr))
				return
			}
			sr.Error(err)
			return
		}

		if streamResp.Type == "response.error" || streamResp.Type == "response.failed" {
			if streamResp.Response != nil {
				if oaiErr := streamResp.Response.GetOpenAIError(); oaiErr != nil && oaiErr.Type != "" {
					streamErr = types.WithOpenAIError(*oaiErr, http.StatusInternalServerError)
					sr.Stop(service.SafeRelayError(c, streamErr))
					return
				}
			}
			streamErr = types.NewOpenAIError(fmt.Errorf("responses stream error: %s", streamResp.Type), types.ErrorCodeBadResponse, http.StatusInternalServerError)
			sr.Stop(service.SafeRelayError(c, streamErr))
			return
		}
		if streamResp.Response != nil {
			sanitizeResponsesChatAccounting(streamResp.Response.Usage)
			if streamResp.Type != "response.completed" && streamResp.Type != "response.done" && streamResp.Type != "response.incomplete" {
				streamResp.Response.Usage = nil
			}
		}

		// Mirrors the converter, which keeps the usage of the latest terminal
		// event that carries one.
		switch streamResp.Type {
		case "response.completed", "response.done", "response.incomplete":
			if streamResp.Response != nil && streamResp.Response.Usage != nil {
				observedUsage = observed
			}
		}

		results, err := relayconvert.ConvertStreamResponseChunk(c, info, state, &streamResp)
		if err != nil {
			streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
			sr.Stop(service.SafeRelayError(c, streamErr))
			return
		}
		for _, result := range results {
			if !sendStreamResult(result) {
				sr.Stop(service.SafeRelayError(c, streamErr))
				return
			}
		}
	})

	if streamErr != nil {
		return nil, streamErr
	}

	usage := state.Usage()
	upstreamUsage := usage != nil && usage.TotalTokens != 0
	if !upstreamUsage {
		usage = service.ResponseText2Usage(c, state.UsageText(), info.UpstreamModelName, info.GetEstimatePromptTokens())
		state.SetUsage(usage)
	}

	if info.RelayFormat == types.RelayFormatClaude && info.ClaudeConvertInfo != nil {
		info.ClaudeConvertInfo.Usage = usage
	}
	finalResults, err := relayconvert.FinalizeStreamResponse(c, info, state)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	for _, result := range finalResults {
		if !sendStreamResult(result) {
			return nil, streamErr
		}
	}
	if info.RelayFormat == types.RelayFormatOpenAI && info.ShouldIncludeUsage && usage != nil {
		body, err := observedUsage.clientBody(helper.GenerateFinalUsageResponse(responseId, createAt, info.UpstreamModelName, *usage), true)
		if err == nil {
			err = helper.StringData(c, string(body))
		}
		if err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
		}
	}

	if info.RelayFormat == types.RelayFormatOpenAI {
		helper.Done(c)
	}
	// Recorded only after every client frame is emitted: the final usage frame
	// shares this usage envelope and internal evidence must not be sent.
	if upstreamUsage && usage.BillingUsage != nil {
		observedUsage.captureEvidence(usage)
	}
	return usage, nil
}
