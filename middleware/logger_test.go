package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAccessLoggerPaymentWebhookPaths(t *testing.T) {
	for _, tc := range []struct {
		name, route, requestPath, loggedPath string
	}{
		{"stripe", "/api/stripe/webhook", "/api/stripe/webhook?secret=private-query-token", "/api/stripe/webhook"},
		{"creem", "/api/creem/webhook", "/api/creem/webhook?secret=private-query-token", "/api/creem/webhook"},
		{"waffo", "/api/waffo/webhook", "/api/waffo/webhook?secret=private-query-token", "/api/waffo/webhook"},
		{"pancake test", "/api/waffo-pancake/webhook/:env", "/api/waffo-pancake/webhook/test?secret=private-query-token", "/api/waffo-pancake/webhook/:env"},
		{"pancake prod", "/api/waffo-pancake/webhook/:env", "/api/waffo-pancake/webhook/prod?secret=private-query-token", "/api/waffo-pancake/webhook/:env"},
		{"pancake unrecognized env", "/api/waffo-pancake/webhook/:env", "/api/waffo-pancake/webhook/private-env-token?secret=private-query-token", "/api/waffo-pancake/webhook/:env"},
		{"ordinary query preserved", "/api/status", "/api/status?view=summary", "/api/status?view=summary"},
		{"ordinary parameter preserved", "/api/example/:id", "/api/example/42?view=summary", "/api/example/42?view=summary"},
		{"similar path preserved", "/api/waffo-pancake/webhook-history/:id", "/api/waffo-pancake/webhook-history/42?view=summary", "/api/waffo-pancake/webhook-history/42?view=summary"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var output bytes.Buffer
			common.LogWriterMu.Lock()
			writer, errorWriter := gin.DefaultWriter, gin.DefaultErrorWriter
			gin.DefaultWriter, gin.DefaultErrorWriter = &output, &output
			common.LogWriterMu.Unlock()
			t.Cleanup(func() {
				common.LogWriterMu.Lock()
				gin.DefaultWriter, gin.DefaultErrorWriter = writer, errorWriter
				common.LogWriterMu.Unlock()
			})
			router := gin.New()
			SetUpLogger(router)
			router.Use(RouteTag("api"))
			router.POST(tc.route, func(c *gin.Context) {
				c.Set(common.RequestIdKey, "request-fixture")
				assert.Equal(t, tc.requestPath, c.Request.RequestURI, "logging must not mutate the request")
				if strings.HasPrefix(tc.name, "pancake") {
					assert.NotEmpty(t, c.Param("env"))
				}
				c.Status(http.StatusAccepted)
			})
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, tc.requestPath, nil))
			require.Equal(t, http.StatusAccepted, response.Code)
			lines := strings.Split(strings.TrimSpace(output.String()), "\n")
			accessLog := lines[len(lines)-1]
			assert.True(t, strings.HasPrefix(accessLog, "[GIN] "), accessLog)
			assert.Contains(t, accessLog, " | api | request-fixture | 202 | ")
			assert.True(t, strings.HasSuffix(accessLog, "POST "+tc.loggedPath), accessLog)
			assert.NotContains(t, accessLog, "private-query-token")
			assert.NotContains(t, accessLog, "private-env-token")
		})
	}
}
