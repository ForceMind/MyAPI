package relay

import (
	"errors"
	"net/http"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/gin-gonic/gin"
)

// Validate the actual selected adapter on every attempt, including failover.
// Images are qualified for OpenAI-compatible and Codex conversion paths; PDF
// transport is qualified only for the OpenAI-compatible adapter.
// Other adapters must not silently discard the file content. The provider/model
// remains responsible for accepting the documented PDF modality.
func validatePlaygroundMediaChannel(c *gin.Context, request *dto.GeneralOpenAIRequest, channelType int) *types.NewAPIError {
	for _, message := range request.Messages {
		for _, part := range message.ParseContent() {
			if part.Type == dto.ContentTypeImageURL && channelType != constant.ChannelTypeOpenAI && channelType != constant.ChannelTypeCodex {
				return types.NewErrorWithStatusCode(errors.New(common.TranslateMessage(c, i18n.MsgPlaygroundImageProviderUnsupported)), types.ErrorCode("playground_image_provider_unsupported"), http.StatusBadRequest, types.ErrOptionWithSkipRetry())
			}
			if part.Type != dto.ContentTypeFile {
				continue
			}
			if channelType != constant.ChannelTypeOpenAI {
				return types.NewErrorWithStatusCode(errors.New(common.TranslateMessage(c, i18n.MsgPlaygroundFileProviderUnsupported)), types.ErrorCode("playground_file_provider_unsupported"), http.StatusBadRequest, types.ErrOptionWithSkipRetry())
			}
		}
	}
	return nil
}
