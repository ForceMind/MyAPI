package common

import (
	"context"

	"github.com/gin-gonic/gin"
)

type sensitiveRequestDiagnosticsKey struct{}

const sensitiveRequestDiagnosticsGinKey = "sensitive_request_diagnostics"

// MarkSensitiveRequestDiagnostics prevents request or response content from
// entering diagnostics. Keep the marker on both context forms: Gin does not
// forward Value calls to Request.Context unless ContextWithFallback is enabled.
func MarkSensitiveRequestDiagnostics(c *gin.Context) {
	c.Set(sensitiveRequestDiagnosticsGinKey, true)
	if c.Request != nil {
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), sensitiveRequestDiagnosticsKey{}, true))
	}
}

func SensitiveRequestDiagnostics(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	if c, ok := ctx.(*gin.Context); ok {
		if c == nil {
			return false
		}
		if c.GetBool(sensitiveRequestDiagnosticsGinKey) {
			return true
		}
		if c.Request != nil {
			ctx = c.Request.Context()
		}
	}
	marked, _ := ctx.Value(sensitiveRequestDiagnosticsKey{}).(bool)
	return marked
}

// DiagnosticRequestPath retains the browser route without changing the
// canonical relay path used by routing, policy, or accounting.
func DiagnosticRequestPath(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if SensitiveRequestDiagnostics(c) {
		if path := c.GetString("playground_original_path"); path != "" {
			return path
		}
	}
	if c.Request != nil && c.Request.URL != nil {
		return c.Request.URL.Path
	}
	return ""
}

// SafeDiagnosticIdentifier admits only short machine identifiers, never free
// text, URLs, JSON, or base64 data URLs. Unknown identifiers are omitted.
func SafeDiagnosticIdentifier(value string) string {
	if len(value) == 0 || len(value) > 80 {
		return "unknown_error"
	}
	for _, char := range value {
		if char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '_' || char == '.' || char == ':' || char == '-' {
			continue
		}
		return "unknown_error"
	}
	return value
}
