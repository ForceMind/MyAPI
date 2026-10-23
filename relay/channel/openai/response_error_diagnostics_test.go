package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/constant"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponsesStreamPreservesSafeFailureCodeWithoutInventingUsage(t *testing.T) {
	old := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = old })
	for _, tc := range []struct{ name, body, code string }{
		{"explicit error", `{"type":"error","error":{"code":"context_length_exceeded","message":"private body must not enter metadata"}}`, "context_length_exceeded"},
		{"failed response", `{"type":"response.failed","response":{"status":"failed","error":{"code":"context_length_exceeded"},"usage":null}}`, "context_length_exceeded"},
		{"untrusted code", `{"type":"error","error":{"code":"https://private.invalid/secret"}}`, "unknown_error"},
		{"credential shaped code", `{"type":"error","error":{"code":"sk-private-upstream-token"}}`, "unknown_error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			info := &relaycommon.RelayInfo{DisablePing: true, ChannelMeta: &relaycommon.ChannelMeta{}}
			usage, err := OaiResponsesStreamHandler(c, info, &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("data: " + tc.body + "\n\ndata: [DONE]\n\n"))})
			require.Nil(t, err)
			require.NotNil(t, usage)
			assert.Equal(t, tc.code, info.UpstreamErrorCode)
			require.NotNil(t, usage.BillingUsage)
			assert.True(t, usage.BillingUsage.Estimated)
		})
	}
}
