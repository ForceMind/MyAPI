package cloudflare

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func cloudflareTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	return c, recorder
}

func cloudflareTestInfo() *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		StartTime:   time.Unix(1700000000, 0),
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "test-model"},
	}
}

func TestCfHandlerRejectsMalformedJSONWithoutWritingResponse(t *testing.T) {
	c, recorder := cloudflareTestContext()
	resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewBufferString("{"))}

	err, usage := cfHandler(c, cloudflareTestInfo(), resp)

	require.NotNil(t, err)
	require.Nil(t, usage)
	require.Empty(t, recorder.Body.Bytes())
}

func TestCfHandlerMapsResponseAndUsage(t *testing.T) {
	c, recorder := cloudflareTestContext()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewBufferString(`{"choices":[{"message":{"content":"hello"}}]}`)),
	}

	err, usage := cfHandler(c, cloudflareTestInfo(), resp)

	require.Nil(t, err)
	require.NotNil(t, usage)
	require.Equal(t, http.StatusOK, recorder.Code)
	var response dto.TextResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, "test-model", response.Model)
	require.Equal(t, "hello", response.Choices[0].Message.StringContent())
	require.Equal(t, usage.PromptTokens, response.Usage.PromptTokens)
}

func TestCfSTTHandlerRejectsMalformedJSON(t *testing.T) {
	c, recorder := cloudflareTestContext()
	resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewBufferString("not-json"))}

	err, usage := cfSTTHandler(c, cloudflareTestInfo(), resp)

	require.NotNil(t, err)
	require.Nil(t, usage)
	require.Empty(t, recorder.Body.Bytes())
}

func TestCfStreamHandlerIgnoresMalformedEventAndCompletes(t *testing.T) {
	c, recorder := cloudflareTestContext()
	info := cloudflareTestInfo()
	info.IsStream = true
	resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewBufferString("data: {\n\ndata: [DONE]\n"))}

	err, usage := cfStreamHandler(c, info, resp)

	require.Nil(t, err)
	require.NotNil(t, usage)
	require.Equal(t, http.StatusOK, recorder.Code)
}
