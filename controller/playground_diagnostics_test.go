package controller

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/service"
	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/ForceMind/MyAPI/setting/model_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type playgroundDiagnosticBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *playgroundDiagnosticBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}
func (b *playgroundDiagnosticBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestPlaygroundAttachmentDiagnosticsEndToEnd(t *testing.T) {
	for _, mode := range []model.QuotaWriterMode{model.QuotaWriterModeLegacy, model.QuotaWriterModeAuthoritative} {
		for _, debug := range []bool{false, true} {
			for _, scenario := range []string{"invalid-json", "empty-error", "structured-error", "untrusted-code", "embedded-error", "stream-error", "partial-stream-error", "missing-type-error", "empty-type-error", "empty-object-error", "string-error", "array-error", "boolean-error", "duplicate-error-null", "case-alias-error-null", "extra-field-error", "success"} {
				t.Run(fmt.Sprintf("%s/debug=%v/%s", mode, debug, scenario), func(t *testing.T) {
					db, user, key, access := playgroundControllerFixture(t, mode)
					oldDebug, oldErrorLog := common.DebugEnabled, constant.ErrorLogEnabled
					common.DebugEnabled, constant.ErrorLogEnabled = debug, true
					common.RetryTimes = 2
					t.Cleanup(func() { common.DebugEnabled, constant.ErrorLogEnabled = oldDebug, oldErrorLog })
					var backend playgroundDiagnosticBuffer
					common.LogWriterMu.Lock()
					oldWriter, oldErrorWriter := gin.DefaultWriter, gin.DefaultErrorWriter
					gin.DefaultWriter, gin.DefaultErrorWriter = &backend, &backend
					common.LogWriterMu.Unlock()
					t.Cleanup(func() {
						common.LogWriterMu.Lock()
						gin.DefaultWriter, gin.DefaultErrorWriter = oldWriter, oldErrorWriter
						common.LogWriterMu.Unlock()
					})
					logDir := t.TempDir()
					t.Setenv("FULL_CONTENT_LOG_ENABLED", "true")
					t.Setenv("FULL_CONTENT_LOG_DIR", logDir)
					imageData := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("\x89PNG\r\n\x1a\nprivate-diagnostic-image"))
					fileData := "data:application/pdf;base64," + base64.StdEncoding.EncodeToString([]byte("%PDF-1.7\nprivate-diagnostic-file\n%%EOF"))
					requestBody, err := common.Marshal(map[string]any{"model": playgroundRelayModel, "messages": []any{map[string]any{"role": "user", "content": []any{
						map[string]any{"type": "text", "text": "Inspect these attachments"},
						map[string]any{"type": "image_url", "image_url": map[string]any{"url": imageData}},
						map[string]any{"type": "file", "file": map[string]any{"filename": "private.pdf", "file_data": fileData}},
					}}}, "max_completion_tokens": 20, "stream": strings.Contains(scenario, "stream")})
					require.NoError(t, err)
					errorType, errorCode := "server_error", "server_error"
					if scenario == "untrusted-code" {
						errorType, errorCode = "private-attachment-type", "private-attachment-code"
					}
					errorBody, err := common.Marshal(map[string]any{"error": map[string]any{"message": string(requestBody), "type": errorType, "code": errorCode, "param": fileData, "metadata": map[string]any{"echo": imageData}}})
					require.NoError(t, err)
					calls := 0
					t.Cleanup(service.SetHttpClientForTest(&http.Client{Transport: testRoundTripper(func(request *http.Request) (*http.Response, error) {
						calls++
						raw, err := io.ReadAll(request.Body)
						require.NoError(t, err)
						assert.Contains(t, string(raw), imageData)
						assert.Contains(t, string(raw), fileData)
						assert.Equal(t, "Bearer privateupstream", request.Header.Get("Authorization"))
						status, contentType, body := 400, "application/json", string(errorBody)
						switch scenario {
						case "invalid-json":
							status, body = 502, string(requestBody)+" private-attachment-invalid"
						case "empty-error":
							status, body = 502, `{"echo":"`+imageData+`"}`
						case "embedded-error":
							status = 200
						case "missing-type-error", "empty-type-error", "empty-object-error", "string-error", "array-error", "boolean-error", "duplicate-error-null", "case-alias-error-null", "extra-field-error":
							status = 200
							envelope := map[string]any{"error": map[string]any{"message": string(requestBody), "code": "server_error", "param": fileData, "metadata": map[string]any{"echo": imageData}}, "usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15}}
							switch scenario {
							case "empty-type-error":
								envelope["error"].(map[string]any)["type"] = ""
							case "empty-object-error":
								envelope["error"] = map[string]any{}
							case "string-error":
								envelope["error"] = string(requestBody)
							case "array-error":
								envelope["error"] = []any{string(requestBody)}
							case "boolean-error":
								envelope["error"] = false
							case "extra-field-error":
								envelope["message"] = map[string]any{}
							}
							encoded, err := common.Marshal(envelope)
							require.NoError(t, err)
							body = string(encoded)
							if scenario == "duplicate-error-null" {
								body = strings.TrimSuffix(body, "}") + `,"error":null}`
							}
							if scenario == "case-alias-error-null" {
								body = strings.TrimSuffix(body, "}") + `,"ERROR":null}`
							}
						case "stream-error", "partial-stream-error":
							status, contentType, body = 200, "text/event-stream", "data: "+string(errorBody)+"\n\ndata: [DONE]\n\n"
							if scenario == "partial-stream-error" {
								body = strings.Repeat("data: "+`{"id":"partial","object":"chat.completion.chunk","model":"gpt-6.1-sol","choices":[{"index":0,"delta":{"content":"partial answer"},"finish_reason":null}]}`+"\n\n", 2) + body
							}
						case "success":
							status, body = 200, `{"id":"success","object":"chat.completion","model":"gpt-6.1-sol","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`
						}
						return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {contentType}}, Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
					})}))
					channel := &model.Channel{Id: 41, Name: "Diagnostic fixture", Type: constant.ChannelTypeOpenAI, Key: "privateupstream", Status: common.ChannelStatusEnabled, Group: "default", Models: playgroundRelayModel, BaseURL: common.GetPointer("https://api.openai.com")}
					require.NoError(t, db.Create(channel).Error)
					require.NoError(t, db.Create(&model.Ability{Group: "default", Model: playgroundRelayModel, ChannelId: channel.Id, Enabled: true, Weight: 1}).Error)
					success := true
					router := playgroundControllerRouter(func(c *gin.Context) { c.Next(); success = c.GetBool("relay_success") })
					requestID := "pg-diagnostic-" + scenario
					response := playgroundControllerRequest(router, http.MethodPost, "/pg/chat/completions", access, key.Id, string(requestBody), requestID)
					assert.Equal(t, 1, calls, "failed dispatched requests must not replay")
					assert.Equal(t, scenario == "success", success)
					if scenario == "partial-stream-error" {
						assert.Equal(t, 200, response.Code)
						assert.Contains(t, response.Body.String(), "partial answer")
						assert.Contains(t, response.Body.String(), `data: {"error":`)
						assert.NotContains(t, response.Body.String(), "[DONE]")
					} else if scenario == "success" {
						assert.Equal(t, 200, response.Code)
					} else {
						assert.GreaterOrEqual(t, response.Code, 400, response.Body.String())
						assert.Contains(t, response.Header().Get("Content-Type"), "application/json")
					}
					if scenario != "success" {
						assert.Contains(t, response.Body.String(), requestID)
						assert.Contains(t, backend.String(), "status=")
					}
					var entries []model.Log
					require.NoError(t, db.Where("request_id = ?", requestID).Find(&entries).Error)
					require.NotEmpty(t, entries)
					stored, err := common.Marshal(entries)
					require.NoError(t, err)
					for _, entry := range entries {
						if scenario != "success" {
							assert.NotEqual(t, model.LogTypeConsume, entry.Type, "error envelope must not settle fabricated usage")
						}
						if entry.Type == model.LogTypeConsume || entry.Type == model.LogTypeError {
							assert.Contains(t, entry.Other, `"request_path":"/pg/chat/completions"`)
						}
					}
					allDiagnostics := backend.String() + string(stored) + response.Body.String()
					files, err := filepath.Glob(filepath.Join(logDir, "full-content-*.jsonl"))
					require.NoError(t, err)
					require.NotEmpty(t, files)
					for _, file := range files {
						data, err := os.ReadFile(file)
						require.NoError(t, err)
						allDiagnostics += string(data)
					}
					for _, forbidden := range []string{imageData, fileData, "private-attachment", access, key.Key} {
						assert.NotContains(t, allDiagnostics, forbidden)
					}
					if scenario != "success" && scenario != "structured-error" && scenario != "untrusted-code" {
						view, err := model.GetUsageReview(context.Background(), db, user.Id, requestID)
						require.NoError(t, err)
						assert.Equal(t, "usage_unknown", view.State, "accepted/uncertain upstream failure must retain its reservation")
						assert.Nil(t, view.ActualQuota)
						assert.Greater(t, view.ReservedQuota, int64(0))
						require.NoError(t, db.First(user, user.Id).Error)
						require.NoError(t, db.First(key, key.Id).Error)
						assert.EqualValues(t, 1000000-view.ReservedQuota, user.Quota)
						assert.EqualValues(t, 1000000-view.ReservedQuota, key.RemainQuota)
					}
				})
			}
		}
	}
}

func TestPlaygroundResponsesErrorsRetainUnknownUsageAcrossWriters(t *testing.T) {
	for _, writer := range []model.QuotaWriterMode{model.QuotaWriterModeLegacy, model.QuotaWriterModeAuthoritative} {
		for _, channelType := range []int{constant.ChannelTypeOpenAI, constant.ChannelTypeCodex} {
			for _, stream := range []bool{false, true} {
				for _, envelope := range []string{"flat-error", "wrapped-error"} {
					t.Run(fmt.Sprintf("%s/channel=%d/stream=%v/%s", writer, channelType, stream, envelope), func(t *testing.T) {
						db, user, key, access := playgroundControllerFixture(t, writer)
						common.RetryTimes = 2
						t.Setenv(common.ChannelQuotaIdentityKeysEnv, "")
						require.NoError(t, db.AutoMigrate(&model.ChannelQuotaSnapshot{}))
						prior := model_setting.GetGlobalSettings().ChatCompletionsToResponsesPolicy
						priorPolicy, err := common.Marshal(map[string]any{"enabled": prior.Enabled, "all_channels": prior.AllChannels, "channel_ids": prior.ChannelIDs, "channel_types": prior.ChannelTypes, "model_patterns": prior.ModelPatterns})
						require.NoError(t, err)
						require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"global.chat_completions_to_responses_policy": `{"enabled":true,"all_channels":false,"channel_types":[1,57],"model_patterns":["^gpt-6\\.1-sol$"]}`}))
						t.Cleanup(func() {
							require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"global.chat_completions_to_responses_policy": string(priorPolicy)}))
						})
						imageData := "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jU1sAAAAASUVORK5CYII="
						errorObject := map[string]any{"type": "error", "code": "server_error", "message": "private-attachment " + imageData}
						var responseObject any = errorObject
						if envelope == "wrapped-error" {
							responseObject = map[string]any{"error": errorObject, "usage": map[string]any{"input_tokens": 10, "output_tokens": 5, "total_tokens": 15}}
						}
						responseBody, err := common.Marshal(responseObject)
						require.NoError(t, err)
						calls := 0
						t.Cleanup(service.SetHttpClientForTest(&http.Client{Transport: testRoundTripper(func(request *http.Request) (*http.Response, error) {
							calls++
							assert.True(t, strings.HasSuffix(request.URL.Path, "/responses"), request.URL.Path)
							raw, err := io.ReadAll(request.Body)
							require.NoError(t, err)
							assert.Contains(t, string(raw), imageData)
							body := "data: " + string(responseBody) + "\n\ndata: [DONE]\n\n"
							return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
						})}))
						channelKey := "synthetic-openai"
						if channelType == constant.ChannelTypeCodex {
							channelKey = `{"access_token":"synthetic-codex-access","account_id":"synthetic-codex-account"}`
						}
						channel := &model.Channel{Id: 51, Name: "Responses error fixture", Type: channelType, Key: channelKey, Status: common.ChannelStatusEnabled, Group: "default", Models: playgroundRelayModel, BaseURL: common.GetPointer("https://api.openai.com")}
						require.NoError(t, db.Create(channel).Error)
						require.NoError(t, db.Create(&model.Ability{Group: "default", Model: playgroundRelayModel, ChannelId: channel.Id, Enabled: true, Weight: 1}).Error)
						body, err := common.Marshal(map[string]any{"model": playgroundRelayModel, "messages": []any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "image_url", "image_url": map[string]any{"url": imageData}}}}}, "max_completion_tokens": 20, "stream": stream})
						require.NoError(t, err)
						success := true
						router := playgroundControllerRouter(func(c *gin.Context) { c.Next(); success = c.GetBool("relay_success") })
						requestID := "pg-converted-error"
						response := playgroundControllerRequest(router, http.MethodPost, "/pg/chat/completions", access, key.Id, string(body), requestID)
						assert.Equal(t, 502, response.Code, response.Body.String())
						assert.False(t, success)
						assert.Equal(t, 1, calls)
						assert.NotContains(t, response.Body.String(), "private-attachment")
						assert.NotContains(t, response.Body.String(), imageData)
						assert.NotContains(t, response.Body.String(), "[DONE]")
						var settled int64
						require.NoError(t, db.Model(&model.Log{}).Where("request_id = ? AND type = ?", requestID, model.LogTypeConsume).Count(&settled).Error)
						assert.Zero(t, settled)
						view, err := model.GetUsageReview(context.Background(), db, user.Id, requestID)
						require.NoError(t, err)
						assert.Equal(t, "usage_unknown", view.State)
						assert.Nil(t, view.ActualQuota)
						require.NoError(t, db.First(user, user.Id).Error)
						require.NoError(t, db.First(key, key.Id).Error)
						assert.EqualValues(t, 1000000-view.ReservedQuota, user.Quota)
						assert.EqualValues(t, 1000000-view.ReservedQuota, key.RemainQuota)
					})
				}
			}
		}
	}
}
