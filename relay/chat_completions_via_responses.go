package relay

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/relay/channel"
	openaichannel "github.com/ForceMind/MyAPI/relay/channel/openai"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	relayconstant "github.com/ForceMind/MyAPI/relay/constant"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/ForceMind/MyAPI/service"

	"github.com/gin-gonic/gin"
)

func applySystemPromptIfNeeded(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) {
	if info == nil || request == nil {
		return
	}
	if info.ChannelSetting.SystemPrompt == "" {
		return
	}

	systemRole := request.GetSystemRoleName()

	containSystemPrompt := false
	for _, message := range request.Messages {
		if message.Role == systemRole {
			containSystemPrompt = true
			break
		}
	}
	if !containSystemPrompt {
		systemMessage := dto.Message{
			Role:    systemRole,
			Content: info.ChannelSetting.SystemPrompt,
		}
		request.Messages = append([]dto.Message{systemMessage}, request.Messages...)
		return
	}

	if !info.ChannelSetting.SystemPromptOverride {
		return
	}

	common.SetContextKey(c, constant.ContextKeySystemPromptOverride, true)
	for i, message := range request.Messages {
		if message.Role != systemRole {
			continue
		}
		if message.IsStringContent() {
			request.Messages[i].SetStringContent(info.ChannelSetting.SystemPrompt + "\n" + message.StringContent())
			return
		}
		contents := message.ParseContent()
		contents = append([]dto.MediaContent{
			{
				Type: dto.ContentTypeText,
				Text: info.ChannelSetting.SystemPrompt,
			},
		}, contents...)
		request.Messages[i].Content = contents
		return
	}
}

func chatCompletionsViaResponses(c *gin.Context, info *relaycommon.RelayInfo, adaptor channel.Adaptor, request *dto.GeneralOpenAIRequest) (*dto.Usage, *types.NewAPIError) {
	chatJSON, err := common.Marshal(request)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}

	chatJSON, err = relaycommon.RemoveDisabledFields(chatJSON, info.ChannelOtherSettings, info.ChannelSetting.PassThroughBodyEnabled)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}

	if len(info.ParamOverride) > 0 {
		chatJSON, err = relaycommon.ApplyParamOverrideWithRelayInfo(chatJSON, info)
		if err != nil {
			return nil, newAPIErrorFromParamOverride(err)
		}
	}

	var overriddenChatReq dto.GeneralOpenAIRequest
	if err := common.Unmarshal(chatJSON, &overriddenChatReq); err != nil {
		return nil, types.NewError(err, types.ErrorCodeChannelParamOverrideInvalid, types.ErrOptionWithSkipRetry())
	}

	result, err := service.ConvertRequestVia(c, info, &overriddenChatReq, types.RelayFormatOpenAI, types.RelayFormatOpenAIResponses)
	if err != nil {
		return nil, types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	responsesReq, ok := result.Value.(*dto.OpenAIResponsesRequest)
	if !ok {
		return nil, types.NewError(fmt.Errorf("expected OpenAI responses request, got %T", result.Value), types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}
	// The Codex upstream only accepts streaming Responses requests. Keep
	// info.IsStream unchanged so non-streaming Chat Completions clients still
	// receive a buffered JSON response below.
	if info.ChannelType == constant.ChannelTypeCodex {
		responsesReq.Stream = common.GetPointer(true)
	}

	savedRelayMode := info.RelayMode
	savedRequestURLPath := info.RequestURLPath
	defer func() {
		info.RelayMode = savedRelayMode
		info.RequestURLPath = savedRequestURLPath
	}()

	info.RelayMode = relayconstant.RelayModeResponses
	info.RequestURLPath = "/v1/responses"

	convertedRequest, err := adaptor.ConvertOpenAIResponsesRequest(c, info, *responsesReq)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}
	relaycommon.AppendRequestConversionFromRequest(info, convertedRequest)

	jsonData, err := common.Marshal(convertedRequest)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}

	jsonData, err = relaycommon.RemoveDisabledFields(jsonData, info.ChannelOtherSettings, info.ChannelSetting.PassThroughBodyEnabled)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}

	body, closer, err := relaycommon.NewOutboundJSONBody(jsonData)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}
	defer closer.Close()
	jsonData = nil
	var requestBody io.Reader = body

	var httpResp *http.Response
	resp, err := adaptor.DoRequest(c, info, requestBody)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	}
	if resp == nil {
		return nil, types.NewOpenAIError(nil, types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}

	statusCodeMappingStr := c.GetString("status_code_mapping")

	httpResp = resp.(*http.Response)
	clientStream := info.IsStream
	upstreamStream := responsesUpstreamIsStream(httpResp)
	info.IsStream = clientStream || upstreamStream
	if httpResp.StatusCode != http.StatusOK {
		newApiErr := service.RelayErrorHandler(c.Request.Context(), httpResp, false)
		service.ResetStatusCode(newApiErr, statusCodeMappingStr)
		return nil, newApiErr
	}

	if upstreamStream && clientStream {
		usage, newApiErr := openaichannel.OaiResponsesToChatStreamHandler(c, info, httpResp)
		if newApiErr != nil {
			service.ResetStatusCode(newApiErr, statusCodeMappingStr)
			return nil, newApiErr
		}
		return usage, nil
	}
	if upstreamStream {
		info.IsStream = false
		usage, newApiErr := openaichannel.OaiResponsesToChatBufferedStreamHandler(c, info, httpResp)
		if newApiErr != nil {
			service.ResetStatusCode(newApiErr, statusCodeMappingStr)
			return nil, newApiErr
		}
		return usage, nil
	}

	usage, newApiErr := openaichannel.OaiResponsesToChatHandler(c, info, httpResp)
	if newApiErr != nil {
		service.ResetStatusCode(newApiErr, statusCodeMappingStr)
		return nil, newApiErr
	}
	return usage, nil
}

func isResponsesEventStreamContentType(contentType string) bool {
	return strings.Contains(strings.ToLower(contentType), "text/event-stream")
}

type bufferedReadCloser struct {
	io.Reader
	io.Closer
}

// responsesUpstreamIsStream handles Responses backends (notably Codex) that
// return SSE without a Content-Type header. Peeking never consumes bytes, and
// the original body closer remains attached to the replacement body.
func responsesUpstreamIsStream(resp *http.Response) bool {
	if resp == nil {
		return false
	}
	if isResponsesEventStreamContentType(resp.Header.Get("Content-Type")) {
		return true
	}
	if resp.Body == nil {
		return false
	}

	originalBody := resp.Body
	reader := bufio.NewReader(originalBody)
	resp.Body = &bufferedReadCloser{Reader: reader, Closer: originalBody}

	for size := 1; size <= 64; size++ {
		prefix, err := reader.Peek(size)
		trimmed := bytes.TrimPrefix(prefix, []byte{0xef, 0xbb, 0xbf})
		trimmed = bytes.TrimLeft(trimmed, " \t\r\n")

		if bytes.HasPrefix(trimmed, []byte("event:")) || bytes.HasPrefix(trimmed, []byte("data:")) {
			return true
		}
		if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[' || trimmed[0] == '"') {
			return false
		}
		if err != nil {
			return false
		}
	}

	return false
}
