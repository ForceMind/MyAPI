package service

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/model"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	relayconstant "github.com/ForceMind/MyAPI/relay/constant"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/tiktoken-go/tokenizer/codec"
)

var requestInputEncoder = codec.NewO200kBase()

// Read the already-selected channel without initializing RelayInfo.ChannelMeta.
// That initialization marks the first channel as consumed for retry selection
// and must remain owned by the dispatch path, after the initial billing hold.
func requestInputChannel(c *gin.Context, info *relaycommon.RelayInfo) *relaycommon.ChannelMeta {
	if c != nil {
		if channelID, ok := common.GetContextKeyType[int](c, constant.ContextKeyChannelId); ok {
			settings, _ := common.GetContextKeyType[dto.ChannelSettings](c, constant.ContextKeyChannelSetting)
			return &relaycommon.ChannelMeta{
				ChannelId:      channelID,
				ChannelType:    common.GetContextKeyInt(c, constant.ContextKeyChannelType),
				ChannelSetting: settings,
			}
		}
	}
	return info.ChannelMeta
}

// Count text locally even when billing estimation is disabled. Never fetch
// media, contact a provider or change the estimate used for billing.
func PrepareRequestInputEstimate(c *gin.Context, info *relaycommon.RelayInfo) {
	if info == nil || info.Request == nil {
		return
	}
	switch info.RelayMode {
	case relayconstant.RelayModeChatCompletions, relayconstant.RelayModeCompletions, relayconstant.RelayModeResponses, relayconstant.RelayModeResponsesCompact:
	default:
		return
	}
	meta := info.Request.GetTokenCountMeta()
	if meta == nil {
		return
	}
	method := "local_text_estimate"
	text := meta.CombineText
	complete := len(meta.Files) == 0
	messages := meta.MessagesCount
	if request, ok := info.Request.(*dto.OpenAIResponsesRequest); ok {
		messages = len(request.ParseInput())
		var input any
		if common.Unmarshal(request.Input, &input) != nil || responseInputHasUncountedContent(input) || request.PreviousResponseID != "" {
			complete = false
		} else if _, array := input.([]any); array {
			// ParseInput flattens messages but omits function-call arguments and
			// results. Include the raw input structure so those cannot bypass a
			// configured estimate limit. JSON framing remains an approximation.
			withoutInput := *request
			withoutInput.Input = nil
			text = string(request.Input) + "\n" + withoutInput.GetTokenCountMeta().CombineText
		}
	}
	channel := requestInputChannel(c, info)
	if channel != nil && channel.ChannelSetting.SystemPrompt != "" {
		text += "\n" + channel.ChannelSetting.SystemPrompt
	}
	var count int64
	if (channel != nil && channel.ChannelType == constant.ChannelTypeCodex) || strings.HasPrefix(info.OriginModelName, "gpt-5") || strings.HasPrefix(info.OriginModelName, "gpt-6") {
		tokens, err := requestInputEncoder.Count(text)
		if err != nil {
			return
		}
		count, method = int64(tokens), "o200k_base_text_estimate"
	} else {
		count = int64(CountTextToken(text, info.OriginModelName))
	}
	count += int64(messages)*4 + int64(meta.ToolsCount)*8 + 3
	info.RequestInputEstimate = &relaycommon.RequestInputEstimate{InputTokens: count, Method: method, Complete: complete}
	if c != nil {
		c.Set("request_input_estimate", info.RequestInputEstimate)
	}
}

func responseInputHasUncountedContent(input any) bool {
	switch value := input.(type) {
	case string:
		return false
	case []any:
		for _, part := range value {
			if responseInputHasUncountedContent(part) {
				return true
			}
		}
		return false
	case map[string]any:
		if kind, ok := value["type"].(string); ok {
			switch kind {
			case "input_image", "input_file", "input_audio":
				return true
			}
		}
		for key, part := range value {
			switch key {
			case "image_url", "file_url", "file_id", "file_data", "audio", "encrypted_content":
				return true
			}
			switch part.(type) {
			case []any, map[string]any:
				if responseInputHasUncountedContent(part) {
					return true
				}
			}
		}
		return false
	default:
		return true
	}
}

func AppendRequestInputDiagnostics(c *gin.Context, info *relaycommon.RelayInfo, other map[string]interface{}) {
	if info == nil || other == nil {
		return
	}
	if info.RequestInputEstimate != nil {
		other["request_input_estimate"] = info.RequestInputEstimate
	}
	if info.UpstreamErrorCode != "" {
		other["upstream_error_code"] = info.UpstreamErrorCode
	}
	if c != nil && c.Writer != nil && c.Writer.Written() {
		other["http_status"] = c.Writer.Status()
	}
}

// Run before initial billing and before each retry's dispatch. An absent model
// entry preserves existing admission. Catalog metadata is not a trusted bound.
func EnforceRequestInputLimit(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	if info == nil {
		return nil
	}
	channel := requestInputChannel(c, info)
	if channel == nil {
		return nil
	}
	limit := channel.ChannelSetting.InputTokenLimits[info.OriginModelName]
	if info.RequestInputEstimate != nil {
		info.RequestInputEstimate.LimitTokens = limit
	}
	if limit <= 0 {
		return nil
	}
	code := types.ErrorCode("request_input_limit_exceeded")
	var message string
	estimate := info.RequestInputEstimate
	if estimate == nil || !estimate.Complete {
		code = types.ErrorCode("request_input_limit_unavailable")
		message = i18n.T(c, i18n.MsgRequestInputLimitUnavailable)
	} else if estimate.InputTokens > limit {
		message = i18n.T(c, i18n.MsgRequestInputLimitExceeded, map[string]any{"Estimated": estimate.InputTokens, "Limit": limit})
	} else {
		return nil
	}
	apiErr := types.NewErrorWithStatusCode(errors.New(message), code, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	if c != nil && constant.ErrorLogEnabled {
		other := map[string]interface{}{"request_outcome": "rejected", "usage_outcome": "not_dispatched", "error_code": code, "http_status": http.StatusBadRequest, "preflight_rejected": true}
		AppendRequestInputDiagnostics(nil, info, other)
		if path := common.DiagnosticRequestPath(c); path != "" {
			other["request_path"] = path
		}
		elapsed := 0
		if !info.StartTime.IsZero() {
			elapsed = int(time.Since(info.StartTime).Seconds())
		}
		model.RecordErrorLog(c, info.UserId, channel.ChannelId, info.OriginModelName, c.GetString("token_name"), message, info.TokenId, elapsed, info.IsStream, info.UsingGroup, other)
	}
	return apiErr
}
