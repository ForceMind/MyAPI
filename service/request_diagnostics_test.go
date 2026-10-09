package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSensitiveRelayErrorHandlerDoesNotExposeUpstreamContent(t *testing.T) {
	for _, debug := range []bool{false, true} {
		for _, showBody := range []bool{false, true} {
			for _, body := range []string{
				"private-attachment-invalid-json",
				`{"echo":"private-attachment-empty-message"}`,
				`{"message":"private-attachment-message"}`,
				`{"error":{"message":"private-attachment-message","type":"server_error","code":"server_error","param":"private-attachment-param","metadata":{"raw":"private-attachment-metadata"}}}`,
			} {
				t.Run(fmt.Sprintf("debug=%v/show=%v/%s", debug, showBody, body), func(t *testing.T) {
					withDebugEnabled(t, debug)
					var logs bytes.Buffer
					common.LogWriterMu.Lock()
					old := gin.DefaultErrorWriter
					gin.DefaultErrorWriter = &logs
					common.LogWriterMu.Unlock()
					t.Cleanup(func() { common.LogWriterMu.Lock(); gin.DefaultErrorWriter = old; common.LogWriterMu.Unlock() })
					c, _ := gin.CreateTestContext(httptest.NewRecorder())
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
					common.MarkSensitiveRequestDiagnostics(c)
					for _, ctx := range []context.Context{c, c.Request.Context()} {
						resp := &http.Response{StatusCode: 502, Body: io.NopCloser(strings.NewReader(body))}
						original := RelayErrorHandler(ctx, resp, showBody)
						err := SafeRelayError(ctx, original)
						require.NotNil(t, err)
						assert.Equal(t, 502, err.StatusCode)
						assert.Equal(t, 502, err.UpstreamStatusCode)
						encoded, marshalErr := common.Marshal(err.ToOpenAIError())
						require.NoError(t, marshalErr)
						assert.NotContains(t, string(encoded), "private-attachment")
						assert.NotContains(t, err.Error(), "private-attachment")
					}
					assert.NotContains(t, logs.String(), "private-attachment")
				})
			}
		}
	}
}

func TestSafeRelayErrorPreservesGuardsAndDoesNotMutateBusinessError(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Set(common.RequestIdKey, "guard-request-id")
	common.MarkSensitiveRequestDiagnostics(c)
	for _, code := range []string{"token_budget_exceeded", "token_budget_pending", "account_threshold_unavailable", "usage_dispatch_unresolved", "relay_eligibility_changed"} {
		t.Run(code, func(t *testing.T) {
			original := types.NewErrorWithStatusCode(errors.New("private-attachment"), types.ErrorCode(code), 503, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
			original.UpstreamStatusCode = 429
			safe := SafeRelayError(c, original)
			assert.NotSame(t, original, safe)
			assert.Equal(t, "private-attachment", original.Error())
			assert.Equal(t, types.ErrorCode(code), safe.GetErrorCode())
			assert.Equal(t, types.ErrorTypeNewAPIError, safe.GetErrorType())
			assert.Equal(t, 503, safe.StatusCode)
			assert.Equal(t, 429, safe.UpstreamStatusCode)
			assert.True(t, types.IsSkipRetryError(safe))
			assert.False(t, types.IsRecordErrorLog(safe))
			assert.Contains(t, safe.ToOpenAIError().Message, "guard-request-id")
			assert.NotContains(t, safe.ToOpenAIError().Message, "private-attachment")
			assert.Same(t, original, SafeRelayError(context.Background(), original))
		})
	}
	original := types.WithOpenAIError(types.OpenAIError{Message: "private-attachment", Code: "private-attachment", Type: "private-attachment", Param: "private-attachment", Metadata: []byte(`{"echo":"private-attachment"}`)}, 200)
	safe := SafeRelayError(c, original)
	assert.Equal(t, 502, safe.StatusCode, "an error embedded in HTTP 200 must remain an error")
	assert.Equal(t, types.ErrorCode("upstream_error"), safe.GetErrorCode())
	assert.Equal(t, "upstream_error", safe.ToOpenAIError().Type)
	assert.Empty(t, safe.ToOpenAIError().Param)
	assert.Empty(t, safe.Metadata)
	assert.Empty(t, safe.ToOpenAIError().Metadata)
	assert.Contains(t, original.Error(), "private-attachment")
}

func TestSensitiveFileLoadingKeepsAttachmentOutOfDebugLog(t *testing.T) {
	withDebugEnabled(t, true)
	var logs bytes.Buffer
	common.LogWriterMu.Lock()
	old := gin.DefaultErrorWriter
	gin.DefaultErrorWriter = &logs
	common.LogWriterMu.Unlock()
	t.Cleanup(func() { common.LogWriterMu.Lock(); gin.DefaultErrorWriter = old; common.LogWriterMu.Unlock() })
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	common.MarkSensitiveRequestDiagnostics(c)
	content := base64.StdEncoding.EncodeToString([]byte("%PDF-1.7\nprivate-attachment-for-diagnostics\n%%EOF"))
	source := types.NewBase64FileSource(content, "application/pdf")
	file, err := LoadFileSource(c, source)
	require.NoError(t, err)
	require.NotNil(t, file)
	assert.NotContains(t, logs.String(), content[:30])
	assert.NotContains(t, logs.String(), "base64:")
}

func TestSensitiveConsumeLogKeepsPlaygroundOrigin(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions?private=query", nil)
	c.Set("playground_original_path", "/pg/chat/completions")
	common.MarkSensitiveRequestDiagnostics(c)
	other := make(map[string]interface{})
	appendRequestPath(c, nil, other)
	assert.Equal(t, "/pg/chat/completions", other["request_path"])
	assert.Equal(t, "/v1/chat/completions", c.Request.URL.Path)
}
