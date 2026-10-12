package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/constant"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	relayconstant "github.com/ForceMind/MyAPI/relay/constant"
	"github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const completionContentChunk = `{"id":"chatcmpl-completion","object":"chat.completion.chunk","model":"gpt-test","choices":[{"index":0,"delta":{"content":"answer"},"finish_reason":null}]}`
const completionFinishChunk = `{"id":"chatcmpl-completion","object":"chat.completion.chunk","model":"gpt-test","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`

func completionTestRelayInfo() *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenAI, UpstreamModelName: "gpt-test"},
		IsStream:    true,
		DisablePing: true,
		RelayMode:   relayconstant.RelayModeChatCompletions,
		RelayFormat: types.RelayFormatOpenAI,
	}
}

func TestChatResponseCompletionMarkerRequiresFinishedDeliveredStream(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	for _, tc := range []struct {
		name      string
		body      string
		completed bool
	}{
		{name: "finished stream", body: "data: " + completionContentChunk + "\n\ndata: " + completionFinishChunk + "\n\ndata: [DONE]\n\n", completed: true},
		{name: "malformed terminator", body: "data: " + completionContentChunk + "\n\ndata: " + completionFinishChunk + "\n\ndata: [DONE]garbage\n\n"},
		{name: "missing finish reason", body: "data: " + completionContentChunk + "\n\ndata: [DONE]\n\n"},
		{name: "eof without done", body: "data: " + completionContentChunk + "\n\ndata: " + completionFinishChunk + "\n\n"},
		{name: "error envelope", body: "data: " + completionContentChunk + "\n\ndata: {\"error\":{\"type\":\"server_error\",\"message\":\"upstream failed\"}}\n\ndata: [DONE]\n\n"},
		{name: "parse failure", body: "data: {bad}\n\ndata: [DONE]\n\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			info := completionTestRelayInfo()
			usage, apiErr := OaiStreamHandler(ctx, info, &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": {"text/event-stream"}},
				Body:       io.NopCloser(strings.NewReader(tc.body)),
			})
			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			require.NotNil(t, usage.BillingUsage)
			assert.True(t, usage.BillingUsage.Estimated, "missing upstream usage stays explicitly estimated")
			assert.Equal(t, tc.completed, ctx.GetBool("relay_response_completed"))
			if tc.completed {
				assert.Contains(t, recorder.Body.String(), "answer")
				assert.Contains(t, recorder.Body.String(), "data: [DONE]\n\n")
			}
		})
	}
}

func TestOpenaiHandlerErrorEnvelopeWithoutDiagnosticsIsNotCompletionEvidence(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatOpenAI,
		RelayMode:   relayconstant.RelayModeChatCompletions,
		ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenAI, UpstreamModelName: "gpt-test"},
	}
	body := `{"error":{"message":"upstream failed"}}`
	usage, apiErr := OpenaiHandler(ctx, info, &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	})

	require.Nil(t, apiErr, "metadata evidence must not change the existing diagnostics-off response contract")
	require.NotNil(t, usage)
	require.NotNil(t, usage.BillingUsage)
	assert.True(t, usage.BillingUsage.Estimated)
	assert.Contains(t, recorder.Body.String(), "upstream failed")
	assert.False(t, ctx.GetBool("relay_response_completed"))
}

func TestOpenaiHandlerEmptySuccessShapeIsNotCompletionEvidence(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatOpenAI,
		RelayMode:   relayconstant.RelayModeChatCompletions,
		ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenAI, UpstreamModelName: "gpt-test"},
	}
	usage, apiErr := OpenaiHandler(ctx, info, &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{}`)),
	})

	require.Nil(t, apiErr, "metadata evidence must not change the existing diagnostics-off response contract")
	require.NotNil(t, usage)
	assert.Contains(t, recorder.Body.String(), `"usage"`)
	assert.False(t, ctx.GetBool("relay_response_completed"))
}

func TestOpenaiHandlerValidResponseIsCompletionEvidence(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatOpenAI,
		RelayMode:   relayconstant.RelayModeChatCompletions,
		ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenAI, UpstreamModelName: "gpt-test"},
	}
	body := `{"id":"chatcmpl-completion","choices":[{"index":0,"message":{"role":"assistant","content":"answer"},"finish_reason":"stop"}]}`
	usage, apiErr := OpenaiHandler(ctx, info, &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	})

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	require.NotNil(t, usage.BillingUsage)
	assert.True(t, usage.BillingUsage.Estimated)
	assert.Contains(t, recorder.Body.String(), "answer")
	assert.True(t, ctx.GetBool("relay_response_completed"))
}

type completionWriteFailure struct {
	*httptest.ResponseRecorder
}

func (w *completionWriteFailure) Write(data []byte) (int, error) {
	if strings.Contains(string(data), "[DONE]") {
		return 0, io.ErrClosedPipe
	}
	return w.ResponseRecorder.Write(data)
}

func (w *completionWriteFailure) WriteString(data string) (int, error) {
	return w.Write([]byte(data))
}

func TestChatResponseCompletionMarkerRejectsDownstreamWriteFailure(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	writer := &completionWriteFailure{ResponseRecorder: httptest.NewRecorder()}
	ctx, _ := gin.CreateTestContext(writer)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := completionTestRelayInfo()
	body := "data: " + completionContentChunk + "\n\ndata: " + completionFinishChunk + "\n\ndata: [DONE]\n\n"
	usage, apiErr := OaiStreamHandler(ctx, info, &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	})
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.False(t, ctx.GetBool("relay_response_completed"))
	assert.False(t, info.StreamStatus.HasErrors(), "metadata observation must not change ordinary retry or affinity semantics")
}
