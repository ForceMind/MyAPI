package zhipu

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type zhipuCloseNotifyRecorder struct {
	*httptest.ResponseRecorder
	closeNotify chan bool
}

func (r *zhipuCloseNotifyRecorder) CloseNotify() <-chan bool {
	return r.closeNotify
}

func zhipuTestContext() (*gin.Context, *zhipuCloseNotifyRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := &zhipuCloseNotifyRecorder{
		ResponseRecorder: httptest.NewRecorder(),
		closeNotify:      make(chan bool),
	}
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	return c, recorder
}

func zhipuTestInfo() *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "test-model"}, IsStream: true}
}

func TestZhipuHandlerRejectsMalformedJSONWithoutUsage(t *testing.T) {
	c, recorder := zhipuTestContext()
	resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewBufferString("{"))}

	usage, apiErr := zhipuHandler(c, zhipuTestInfo(), resp)

	require.Nil(t, usage)
	require.NotNil(t, apiErr)
	require.Empty(t, recorder.Body.Bytes())
}

func TestZhipuStreamHandlerRejectsMalformedMetaWithoutUsage(t *testing.T) {
	c, recorder := zhipuTestContext()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewBufferString("meta: {not-json}\n")),
	}

	usage, apiErr := zhipuStreamHandler(c, zhipuTestInfo(), resp)

	require.Nil(t, usage)
	require.NotNil(t, apiErr)
	require.Empty(t, recorder.Body.Bytes())
}

func TestZhipuHandlerUsesCommonJSONWrapper(t *testing.T) {
	c, recorder := zhipuTestContext()
	response := ZhipuResponse{Success: true, Data: ZhipuResponseData{TaskId: "task-1", Choices: []ZhipuMessage{{Role: "assistant", Content: "ok"}}}}
	body, err := common.Marshal(response)
	require.NoError(t, err)

	usage, apiErr := zhipuHandler(c, zhipuTestInfo(), &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(body))})

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"content":"ok"`)
}
