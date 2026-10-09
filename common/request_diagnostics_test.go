package common

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestSensitiveRequestDiagnosticsPreservesContextAndOrigin(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	parent, cancel := context.WithCancel(context.WithValue(context.Background(), RequestIdKey, "original-request"))
	defer cancel()
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions?private=query", nil).WithContext(parent)
	c.Request.Header.Set("Authorization", "Bearer original-session")
	c.Set("playground_original_path", "/pg/chat/completions")
	assert.False(t, SensitiveRequestDiagnostics(c))
	MarkSensitiveRequestDiagnostics(c)
	for _, ctx := range []context.Context{c, c.Copy(), c.Request.Context(), context.WithValue(c.Request.Context(), "child", true)} {
		assert.True(t, SensitiveRequestDiagnostics(ctx))
	}
	assert.Equal(t, "original-request", c.Request.Context().Value(RequestIdKey))
	assert.Equal(t, "Bearer original-session", c.GetHeader("Authorization"))
	assert.Equal(t, "/v1/chat/completions", c.Request.URL.Path)
	assert.Equal(t, "/pg/chat/completions", DiagnosticRequestPath(c))
	cancel()
	assert.ErrorIs(t, c.Request.Context().Err(), context.Canceled)
	assert.False(t, SensitiveRequestDiagnostics(context.Background()))
	assert.False(t, SensitiveRequestDiagnostics(nil))
	assert.False(t, SensitiveRequestDiagnostics((*gin.Context)(nil)))
}

func TestDiagnosticRequestPathDoesNotTrustUnmarkedOrigin(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions?private=query", nil)
	c.Set("playground_original_path", "/pg/chat/completions")
	assert.Equal(t, "/v1/chat/completions", DiagnosticRequestPath(c))
	assert.Empty(t, DiagnosticRequestPath(nil))
}
