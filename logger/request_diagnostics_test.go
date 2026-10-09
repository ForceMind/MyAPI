package logger

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

type diagnosticPayload struct{ marshaled *bool }

func (p diagnosticPayload) MarshalJSON() ([]byte, error) {
	*p.marshaled = true
	return []byte(`{"content":"private-attachment"}`), nil
}

func TestSensitiveRequestDiagnosticsSuppressPayloadAcrossContextForms(t *testing.T) {
	output := isolateLogger(t)
	oldDebug := common.DebugEnabled
	common.DebugEnabled = true
	t.Cleanup(func() { common.DebugEnabled = oldDebug })
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	common.MarkSensitiveRequestDiagnostics(c)
	for _, ctx := range []context.Context{c, c.Request.Context(), c.Copy()} {
		marshaled := false
		LogDebug(ctx, "private-attachment %s", "private-attachment")
		LogJson(ctx, "private-attachment", diagnosticPayload{&marshaled})
		LogInfo(ctx, "private-attachment")
		LogWarn(ctx, "private-attachment")
		LogError(ctx, "private-attachment")
		assert.False(t, marshaled, "sensitive payload must not even be marshaled for diagnostics")
	}
	assert.NotContains(t, output.String(), "private-attachment")
	LogDebug(context.Background(), "ordinary-api-debug")
	LogError(context.Background(), "ordinary-api-error")
	assert.Contains(t, output.String(), "ordinary-api-debug")
	assert.Contains(t, output.String(), "ordinary-api-error")
}

func TestSensitiveErrorMetadataPreservesSafeCorrelation(t *testing.T) {
	output := isolateLogger(t)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	c.Set(common.RequestIdKey, "request-123")
	common.MarkSensitiveRequestDiagnostics(c)
	LogErrorMetadata(c, "new_api_error", "usage_dispatch_unresolved", 503)
	assert.Contains(t, output.String(), "request-123 | relay error: type=new_api_error code=usage_dispatch_unresolved status=503")
	LogErrorMetadata(c, "type\nprivate-attachment", "data:image/png;base64,private-attachment", 502)
	assert.NotContains(t, output.String(), "private-attachment")
	assert.Contains(t, output.String(), "type=unknown_error code=unknown_error status=502")
}
