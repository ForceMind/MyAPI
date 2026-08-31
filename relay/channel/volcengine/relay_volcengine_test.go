package volcengine

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	relayconstant "github.com/ForceMind/MyAPI/relay/constant"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestHandleTTSResponseRejectsMalformedJSONWithoutUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
	resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewBufferString("{"))}

	usage, apiErr := handleTTSResponse(c, resp, info, "mp3")

	require.Nil(t, usage)
	require.NotNil(t, apiErr)
	require.Equal(t, types.ErrorCodeBadResponseBody, apiErr.GetErrorCode())
}

func TestConvertAudioRequestRejectsMalformedMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ApiKey: "app|token"}, RelayMode: relayconstant.RelayModeAudioSpeech}
	request := dto.AudioRequest{Input: "hello", ResponseFormat: "mp3", Metadata: []byte("{")}

	_, err := (&Adaptor{}).ConvertAudioRequest(c, info, request)

	require.Error(t, err)
	require.Contains(t, err.Error(), "error unmarshalling metadata")
}
