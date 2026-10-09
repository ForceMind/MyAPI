package aws

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatRequestRespectsSensitiveDiagnosticsContext(t *testing.T) {
	oldDebug := common.DebugEnabled
	common.DebugEnabled = true
	t.Cleanup(func() { common.DebugEnabled = oldDebug })
	var logs bytes.Buffer
	common.LogWriterMu.Lock()
	old := gin.DefaultErrorWriter
	gin.DefaultErrorWriter = &logs
	common.LogWriterMu.Unlock()
	t.Cleanup(func() { common.LogWriterMu.Lock(); gin.DefaultErrorWriter = old; common.LogWriterMu.Unlock() })
	body := `{"messages":[{"role":"user","content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"private-attachment"}}]}],"max_tokens":20}`
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	common.MarkSensitiveRequestDiagnostics(c)
	for _, ctx := range []context.Context{c, c.Request.Context()} {
		request, err := formatRequest(ctx, strings.NewReader(body), http.Header{})
		require.NoError(t, err)
		assert.Equal(t, "bedrock-2023-05-31", request.AnthropicVersion)
		encoded, err := common.Marshal(request)
		require.NoError(t, err)
		assert.Contains(t, string(encoded), "private-attachment", "privacy logging must not alter provider payload")
	}
	assert.NotContains(t, logs.String(), "private-attachment")
	_, err := formatRequest(context.Background(), strings.NewReader(body), http.Header{})
	require.NoError(t, err)
	assert.Contains(t, logs.String(), "private-attachment", "ordinary API debug behavior is unchanged")
}
