package coze

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func cozeTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	return c, recorder
}

func cozeTestInfo() *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		StartTime:   time.Unix(1700000000, 0),
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "test-model"},
	}
}

func TestCozeChatHandlerRejectsMalformedJSON(t *testing.T) {
	c, recorder := cozeTestContext()
	resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewBufferString("{"))}

	usage, apiErr := cozeChatHandler(c, cozeTestInfo(), resp)

	require.Nil(t, usage)
	require.NotNil(t, apiErr)
	require.Empty(t, recorder.Body.Bytes())
}

func TestCozeChatStreamHandlerIgnoresMalformedEventAndCompletes(t *testing.T) {
	c, recorder := cozeTestContext()
	info := cozeTestInfo()
	info.IsStream = true
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewBufferString("event: conversation.message.delta\ndata: {\n\n")),
	}

	usage, apiErr := cozeChatStreamHandler(c, info, resp)

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "data: [DONE]\n\n", recorder.Body.String())
}
