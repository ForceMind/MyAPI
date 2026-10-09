package openai

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	relayconstant "github.com/ForceMind/MyAPI/relay/constant"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSensitiveChatStreamErrorStopsWithoutEchoOrSuccess(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	content := "data: " + `{"id":"stream","object":"chat.completion.chunk","model":"gpt-6.1-sol","choices":[{"index":0,"delta":{"content":"partial answer"},"finish_reason":null}]}` + "\n\n"
	for _, prefix := range []string{"", content + content} {
		t.Run(strings.ReplaceAll(prefix, "\n", ""), func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			common.MarkSensitiveRequestDiagnostics(c)
			info := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatOpenAI, RelayMode: relayconstant.RelayModeChatCompletions,
				IsStream: true, StartTime: time.Now(), ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-6.1-sol"}}
			body := prefix + "data: " + `{"error":{"message":"private-attachment","type":"server_error","code":"server_error"}}` + "\n\ndata: [DONE]\n\n"
			usage, err := OaiStreamHandler(c, info, &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(body))})
			require.NotNil(t, err)
			assert.Nil(t, usage, "an error frame cannot produce successful billable usage")
			assert.Equal(t, 502, err.StatusCode)
			assert.Equal(t, 200, err.UpstreamStatusCode)
			assert.True(t, types.IsSkipRetryError(err), "an already dispatched stream must not be replayed")
			assert.True(t, info.StreamStatus.HasErrors())
			assert.False(t, info.StreamStatus.IsNormalEnd() && !info.StreamStatus.HasErrors(), "an eagerly read [DONE] cannot erase a handler error")
			assert.NotContains(t, info.StreamStatus.Summary(), "private-attachment")
			assert.NotContains(t, recorder.Body.String(), "private-attachment")
			assert.NotContains(t, recorder.Body.String(), "[DONE]")
			if prefix != "" {
				assert.Contains(t, recorder.Body.String(), "partial answer")
			}
		})
	}
}

func TestSensitiveOpenAIErrorEnvelopesFailClosed(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	for _, stream := range []bool{false, true} {
		for _, tc := range []struct{ name, fields string }{
			{"malformed-choices", `"choices":"private-attachment"`},
			{"missing-type", `"error":{"message":"private-attachment","code":"server_error","param":"private-param","metadata":{"echo":"private-media"}}`},
			{"empty-type", `"error":{"message":"private-attachment","type":""}`},
			{"empty-object", `"error":{}`},
			{"string", `"error":"private-attachment"`},
			{"array", `"error":["private-attachment"]`},
			{"boolean", `"error":false`},
			{"number", `"error":0`},
			{"extra-field-wrong-type", `"error":{"message":"private-attachment","type":"server_error"},"message":{}`},
			{"duplicate-error-null", `"error":{"message":"private-attachment","type":"server_error"},"error":null`},
			{"case-alias-error-null", `"error":{"message":"private-attachment","type":"server_error"},"ERROR":null`},
			{"malformed-error-code", `"error":{"message":"private-attachment","type":{},"code":["private-code"]}`},
		} {
			t.Run(fmt.Sprintf("stream=%v/%s", stream, tc.name), func(t *testing.T) {
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
				common.MarkSensitiveRequestDiagnostics(c)
				info := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatOpenAI, RelayMode: relayconstant.RelayModeChatCompletions, IsStream: stream,
					ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenAI, UpstreamModelName: "gpt-6.1-sol"}}
				body := `{` + tc.fields + `,"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`
				contentType := "application/json"
				if stream {
					body, contentType = "data: "+body+"\n\ndata: [DONE]\n\n", "text/event-stream"
				}
				response := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {contentType}}, Body: io.NopCloser(strings.NewReader(body))}
				var usage *dto.Usage
				var apiErr *types.NewAPIError
				if stream {
					usage, apiErr = OaiStreamHandler(c, info, response)
				} else {
					usage, apiErr = OpenaiHandler(c, info, response)
				}
				require.NotNil(t, apiErr)
				assert.Nil(t, usage, "upstream error or ambiguous envelope must never provide settled usage")
				assert.Equal(t, 502, apiErr.StatusCode)
				assert.Equal(t, 200, apiErr.UpstreamStatusCode)
				assert.True(t, types.IsSkipRetryError(apiErr))
				assert.NotContains(t, recorder.Body.String(), "private-")
				assert.NotContains(t, recorder.Body.String(), "[DONE]")
				if stream {
					assert.True(t, info.StreamStatus.HasErrors())
				}
			})
		}
	}
}

func TestSensitiveResponsesChatRejectsErrorEvents(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	for _, mode := range []string{"json", "buffered", "stream"} {
		for _, tc := range []struct{ name, body string }{
			{"malformed-delta", `{"type":"response.output_text.delta","delta":{"echo":"private-attachment"}}`},
			{"type-error", `{"type":"error","code":"server_error","message":"private-attachment","param":"private-media"}`},
			{"missing-type", `{"error":{"message":"private-attachment","code":"server_error","metadata":{"echo":"private-media"}},"usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15}}`},
			{"extra-field", `{"error":{"message":"private-attachment","type":"server_error"},"message":{}}`},
			{"duplicate-null", `{"error":{"message":"private-attachment","type":"server_error"},"error":null}`},
			{"case-alias-null", `{"error":{"message":"private-attachment","type":"server_error"},"ERROR":null}`},
		} {
			if mode == "json" && tc.name == "malformed-delta" {
				continue
			}
			t.Run(mode+"/"+tc.name, func(t *testing.T) {
				body := tc.body
				if mode != "json" {
					body = "data: " + body + "\n\ndata: [DONE]\n\n"
				}
				c, recorder, resp, info := newResponsesChatTestContext(t, body, mode == "stream")
				common.MarkSensitiveRequestDiagnostics(c)
				var usage *dto.Usage
				var apiErr *types.NewAPIError
				switch mode {
				case "json":
					usage, apiErr = OaiResponsesToChatHandler(c, info, resp)
				case "buffered":
					usage, apiErr = OaiResponsesToChatBufferedStreamHandler(c, info, resp)
				case "stream":
					usage, apiErr = OaiResponsesToChatStreamHandler(c, info, resp)
				}
				require.NotNil(t, apiErr)
				assert.Nil(t, usage)
				assert.Equal(t, 502, apiErr.StatusCode)
				assert.Equal(t, 200, apiErr.UpstreamStatusCode)
				assert.True(t, types.IsSkipRetryError(apiErr))
				assert.NotContains(t, recorder.Body.String(), "private-")
				assert.NotContains(t, recorder.Body.String(), "[DONE]")
				assert.NotContains(t, recorder.Body.String(), `"finish_reason":"stop"`)
				if mode == "stream" {
					assert.True(t, info.StreamStatus.HasErrors())
					assert.NotContains(t, info.StreamStatus.Summary(), "private-")
				}
			})
		}
	}
}
