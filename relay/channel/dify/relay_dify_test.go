package dify

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/ForceMind/MyAPI/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func difyTestContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Set(common.RequestIdKey, "dify-test")
	return c
}

func difyTestInfo() *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl: "http://127.0.0.1",
			ApiKey:         "test-key",
		},
		IsStream: true,
	}
}

func TestRequestOpenAI2DifyRemoteImageUsesDifyFileFields(t *testing.T) {
	c := difyTestContext()
	req := dto.GeneralOpenAIRequest{
		User: json.RawMessage(`"test-user"`),
		Messages: []dto.Message{{Role: "user", Content: []any{
			map[string]any{"type": dto.ContentTypeImageURL, "image_url": map[string]any{"url": "https://example.com/a.png"}},
		}}},
	}

	converted, err := requestOpenAI2Dify(c, difyTestInfo(), req)
	require.NoError(t, err)
	require.Len(t, converted.Files, 1)
	require.Equal(t, "image", converted.Files[0].Type)
	require.Equal(t, "remote_url", converted.Files[0].TransferMode)
	require.Equal(t, "https://example.com/a.png", converted.Files[0].URL)
	body, err := common.Marshal(converted)
	require.NoError(t, err)
	require.Contains(t, string(body), `"transfer_method":"remote_url"`)
	require.NotContains(t, string(body), `"transfer_mode"`)
}

func TestDifyStreamHandlerRejectsMalformedEventWithoutUsage(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	c := difyTestContext()
	info := difyTestInfo()
	resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewBufferString("data: {not-json}\n\n"))}

	usage, apiErr := difyStreamHandler(c, info, resp)
	require.Nil(t, usage)
	require.Error(t, apiErr)
}

func TestDifyStreamHandlerRejectsErrorEventWithoutUsage(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	c := difyTestContext()
	info := difyTestInfo()
	resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewBufferString(`data: {"event":"error"}

`))}

	usage, apiErr := difyStreamHandler(c, info, resp)
	require.Nil(t, usage)
	require.Error(t, apiErr)
}

func TestCompleteDifyUsageFillsIncompleteMessageEndFromActualOutput(t *testing.T) {
	c := difyTestContext()
	reported := &dto.Usage{PromptTokens: 7}
	responseText := "The answer and its reasoning"
	estimated := service.ResponseText2Usage(c, responseText, "", 7)

	usage := completeDifyUsage(reported, responseText, "", 7, c)
	require.Equal(t, 7, usage.PromptTokens)
	require.Equal(t, estimated.CompletionTokens, usage.CompletionTokens)
	require.Equal(t, usage.PromptTokens+usage.CompletionTokens, usage.TotalTokens)
}

func TestCompleteDifyUsagePreservesCompleteUpstreamUsage(t *testing.T) {
	c := difyTestContext()
	reported := &dto.Usage{PromptTokens: 4, CompletionTokens: 6, TotalTokens: 1}
	usage := completeDifyUsage(reported, "ignored", "", 99, c)
	require.Equal(t, 4, usage.PromptTokens)
	require.Equal(t, 6, usage.CompletionTokens)
	require.Equal(t, 10, usage.TotalTokens)
}

func TestDifyStreamUsageEstimatesDebugWorkflowText(t *testing.T) {
	oldDebug := constant.DifyDebug
	constant.DifyDebug = true
	t.Cleanup(func() { constant.DifyDebug = oldDebug })
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	c := difyTestContext()
	info := difyTestInfo()
	stream := "data: {\"event\":\"workflow_started\",\"data\":{\"workflow_id\":\"wf-1\"}}\n\n" +
		"data: {\"event\":\"message\",\"answer\":\"visible answer\"}\n\n" +
		"data: {\"event\":\"message_end\",\"metadata\":{\"usage\":{\"prompt_tokens\":7}}}\n\n"
	resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewBufferString(stream))}

	estimated := service.ResponseText2Usage(c, "Workflow: wf-1\nvisible answer", "", 7)
	usage, apiErr := difyStreamHandler(c, info, resp)
	require.Nil(t, apiErr)
	require.Equal(t, 7, usage.PromptTokens)
	require.Equal(t, estimated.CompletionTokens, usage.CompletionTokens)
	require.Equal(t, 7+estimated.CompletionTokens, usage.TotalTokens)
}

func TestUploadDifyFileRejectsNon2xx(t *testing.T) {
	service.InitHttpClient()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"id":"file-1"}`))
	}))
	defer server.Close()
	// GetHttpClient is initialized by application startup in production. Use the
	// standard client here so this unit test remains independent of startup.
	// The service client is intentionally not replaced globally.

	// This assertion exercises the response contract through the helper when the
	// application HTTP client is available; otherwise the request must fail.
	info := difyTestInfo()
	info.ChannelBaseUrl = server.URL
	media := dto.MediaContent{Type: dto.ContentTypeImageURL, ImageUrl: &dto.MessageImageUrl{Url: "data:image/png;base64,AA==", MimeType: "image/png"}}
	_, err := uploadDifyFile(difyTestContext(), info, "user", media)
	require.Error(t, err)
}

func TestUploadDifyFileRejectsEmptyID(t *testing.T) {
	service.InitHttpClient()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":""}`))
	}))
	defer server.Close()
	info := difyTestInfo()
	info.ChannelBaseUrl = server.URL
	media := dto.MediaContent{Type: dto.ContentTypeImageURL, ImageUrl: &dto.MessageImageUrl{Url: "data:image/png;base64,AA==", MimeType: "image/png"}}
	_, err := uploadDifyFile(difyTestContext(), info, "user", media)
	require.Error(t, err)
}
