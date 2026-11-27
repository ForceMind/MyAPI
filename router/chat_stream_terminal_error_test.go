package router

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const responsesChatTerminalPrefix = "data: " + `{"type":"response.created","response":{"id":"synthetic-stream","model":"gpt-5-codex","created_at":1710000000}}` + "\n\n" +
	"data: " + `{"type":"response.output_text.delta","delta":"synthetic-stream-prefix"}` + "\n\n"

const responsesChatTerminalFailure = "data: " + `{"type":"response.failed","response":{"status":"failed","error":{"message":"private-upstream-message","type":"private-upstream-type","param":"private-upstream-param","code":"private-upstream-code","metadata":{"echo":"private-upstream-metadata"}}}}` + "\n\n"

const responsesChatTerminalSuccess = "data: " + `{"type":"response.completed","response":{"id":"synthetic-stream","status":"completed","usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15,"input_tokens_details":{"cached_tokens":0}}}}` + "\n\ndata: [DONE]\n\n"

type responsesChatTerminalRecorder struct {
	*httptest.ResponseRecorder
	ctx                context.Context
	cancelOnDelta      context.CancelFunc
	failTerminalWrite  bool
	writesAfterCancel  int
	flushesAfterCancel int
	terminalWrites     int
	flushesAfterError  int
}

func (w *responsesChatTerminalRecorder) Write(p []byte) (int, error) {
	if w.ctx.Err() != nil {
		w.writesAfterCancel++
	}
	if strings.HasPrefix(string(p), "data: {\"error\":") {
		w.terminalWrites++
		if w.failTerminalWrite {
			return 0, io.ErrClosedPipe
		}
	}
	n, err := w.ResponseRecorder.Write(p)
	if w.cancelOnDelta != nil && strings.HasSuffix(w.Body.String(), "\n\n") &&
		strings.Contains(w.Body.String(), "synthetic-stream-prefix") {
		w.cancelOnDelta()
	}
	return n, err
}

func (w *responsesChatTerminalRecorder) WriteString(s string) (int, error) {
	return w.Write([]byte(s))
}

func (w *responsesChatTerminalRecorder) Flush() {
	if w.ctx.Err() != nil {
		w.flushesAfterCancel++
	}
	if w.failTerminalWrite && w.terminalWrites > 0 {
		w.flushesAfterError++
	}
	w.ResponseRecorder.Flush()
}

func TestResponsesChatTerminalErrorsAreFramedWithoutReplay(t *testing.T) {
	for _, test := range []struct {
		name      string
		status    int
		sensitive bool
		cancel    bool
		deadline  bool
		failWrite bool
		success   bool
	}{
		{name: "before first event 401", status: http.StatusUnauthorized},
		{name: "before first event 429", status: http.StatusTooManyRequests},
		{name: "before first event 503", status: http.StatusServiceUnavailable},
		{name: "internal deadline before first event", status: http.StatusInternalServerError, deadline: true},
		{name: "partial stream error"},
		{name: "internal deadline during partial stream", deadline: true},
		{name: "sensitive partial stream error", sensitive: true},
		{name: "canceled during partial stream", cancel: true},
		{name: "terminal writer failure", failWrite: true},
		{name: "successful stream", success: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newCodexSessionHeadersFixture(t, "gpt-5-codex", false, false, "", func(f *codexSessionHeadersFixture) {
				switch {
				case test.deadline && test.status != 0:
					f.holdUntilDone = true
				case test.deadline:
					f.responseSSE = responsesChatTerminalPrefix
					f.holdUntilDone = true
				case test.status != 0:
					f.responseStatus = test.status
					f.responseSSE = `{"error":{"message":"synthetic refusal","type":"api_error","code":"api_error"}}`
				case test.success:
					f.responseSSE = responsesChatTerminalPrefix + responsesChatTerminalSuccess
				default:
					f.responseSSE = responsesChatTerminalPrefix + responsesChatTerminalFailure
				}
			})
			common.LogConsumeEnabled = true
			if test.deadline {
				common.RelayFailoverTimeoutSeconds = 1
			}
			if test.status == 0 || test.deadline {
				common.RetryTimes = 2
			}
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(
				`{"model":"gpt-5-codex","messages":[{"role":"user","content":"synthetic request"}],"stream":true,"stream_options":{"include_usage":true}}`,
			))
			req.Header.Set("Authorization", "Bearer sk-"+fixture.tokenKey)
			req.Header.Set("Content-Type", "application/json")
			ctx, cancel := context.WithCancel(req.Context())
			defer cancel()
			req = req.WithContext(ctx)
			if test.sensitive {
				marker, _ := gin.CreateTestContext(httptest.NewRecorder())
				marker.Request = req
				common.MarkSensitiveRequestDiagnostics(marker)
				req = marker.Request
			}

			out := &responsesChatTerminalRecorder{
				ResponseRecorder: httptest.NewRecorder(),
				ctx:               req.Context(),
				failTerminalWrite: test.failWrite,
			}
			if test.cancel {
				out.cancelOnDelta = cancel
			}
			fixture.engine.ServeHTTP(out, req)
			require.Eventually(t, func() bool { return gopool.WorkerCount() == 0 }, 5*time.Second, time.Millisecond)
			require.EqualValues(t, 1, fixture.calls.Load())
			require.Zero(t, fixture.external.Load())
			require.Zero(t, out.writesAfterCancel)
			require.Zero(t, out.flushesAfterCancel)
			require.Zero(t, out.flushesAfterError)
			if test.cancel {
				require.ErrorIs(t, ctx.Err(), context.Canceled)
			} else {
				require.NoError(t, ctx.Err())
			}

			body := out.Body.String()
			require.NotContains(t, body, "private-upstream-")
			if test.status != 0 {
				require.Equal(t, test.status, out.Code)
				require.Equal(t, "application/json", out.Header().Get("Content-Type"))
				var envelope struct {
					Error types.OpenAIError `json:"error"`
				}
				require.NoError(t, common.Unmarshal(out.Body.Bytes(), &envelope))
				if test.deadline {
					require.Contains(t, envelope.Error.Message, "synthetic-header-request-0")
					require.NotEmpty(t, envelope.Error.Code)
				} else {
					require.Contains(t, envelope.Error.Message, "synthetic refusal")
				}
				require.Zero(t, out.terminalWrites)
			} else {
				require.Equal(t, http.StatusOK, out.Code)
				require.Equal(t, "text/event-stream", out.Header().Get("Content-Type"))
				require.Contains(t, body, "synthetic-stream-prefix")
				require.True(t, strings.HasSuffix(body, "\n\n"), "the final SSE frame must be terminated")

				frames := strings.Split(strings.TrimSuffix(body, "\n\n"), "\n\n")
				var errorFrames, doneFrames int
				for index, frame := range frames {
					require.True(t, strings.HasPrefix(frame, "data: "), "unexpected SSE frame: %q", frame)
					data := strings.TrimPrefix(frame, "data: ")
					if data == "[DONE]" {
						doneFrames++
						require.Equal(t, len(frames)-1, index)
						continue
					}
					var event struct {
						Error *types.OpenAIError `json:"error"`
					}
					require.NoError(t, common.Unmarshal([]byte(data), &event))
					if event.Error != nil {
						errorFrames++
						require.Equal(t, len(frames)-1, index)
						require.Contains(t, event.Error.Message, "synthetic-header-request-0")
						require.NotEmpty(t, event.Error.Code)
						require.Empty(t, event.Error.Param)
					}
				}
				switch {
				case test.success:
					require.Equal(t, 1, doneFrames)
					require.Zero(t, errorFrames)
					require.Zero(t, out.terminalWrites)
				case test.cancel:
					require.Zero(t, doneFrames)
					require.Zero(t, errorFrames)
					require.Zero(t, out.terminalWrites)
				case test.failWrite:
					require.Zero(t, doneFrames)
					require.Zero(t, errorFrames)
					require.Equal(t, 1, out.terminalWrites)
				default:
					require.Zero(t, doneFrames)
					require.Equal(t, 1, errorFrames)
					require.Equal(t, 1, out.terminalWrites)
				}
			}

			var consumption []model.Log
			require.NoError(t, model.LOG_DB.Where("request_id = ? AND type = ?", "synthetic-header-request-0", model.LogTypeConsume).Find(&consumption).Error)
			if !test.success {
				require.Empty(t, consumption)
				return
			}
			require.Len(t, consumption, 1)
			entry := consumption[0]
			require.Equal(t, 10, entry.PromptTokens)
			require.Equal(t, 5, entry.CompletionTokens)
			require.Positive(t, entry.Quota)
			var token model.Token
			var user model.User
			var channel model.Channel
			require.NoError(t, model.DB.Where(&model.Token{Key: fixture.tokenKey}).First(&token).Error)
			require.NoError(t, model.DB.First(&user, token.UserId).Error)
			require.NoError(t, model.DB.First(&channel, fixture.channelID).Error)
			require.EqualValues(t, entry.Quota, token.UsedQuota)
			require.EqualValues(t, entry.Quota, user.UsedQuota)
			require.EqualValues(t, entry.Quota, channel.UsedQuota)
			require.EqualValues(t, 1000000-entry.Quota, token.RemainQuota)
			require.EqualValues(t, 1000000-entry.Quota, user.Quota)
		})
	}
}
