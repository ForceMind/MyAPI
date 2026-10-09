package controller

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/middleware"
	"github.com/ForceMind/MyAPI/model"
	relaytypes "github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/ForceMind/MyAPI/service"
	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const terminalSettlementStream = `data: {"id":"stream-fixture","object":"chat.completion.chunk","model":"gpt-6.1-sol","choices":[{"index":0,"delta":{"role":"assistant","content":"Complete answer"},"finish_reason":null}]}

data: {"id":"stream-fixture","object":"chat.completion.chunk","model":"gpt-6.1-sol","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

data: {"id":"stream-fixture","object":"chat.completion.chunk","model":"gpt-6.1-sol","service_tier":"default","choices":[],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15,"prompt_tokens_details":{"cached_tokens":0,"cache_write_tokens":0}}}

data: [DONE]

`

// Cancellation after a successful DONE write models clients that close as soon as
// they receive the protocol terminator. A failed DONE write remains observable
// by the real strictChatWriteObserver in the OpenAI adapter.
type streamTerminalWriter struct {
	*httptest.ResponseRecorder
	cancel        context.CancelFunc
	cancelAtDone  bool
	failAtDone    bool
	attemptedDone bool
	seenDone      bool
}

func (w *streamTerminalWriter) Write(p []byte) (int, error) {
	if strings.Contains(w.Body.String()+string(p), "[DONE]") {
		w.attemptedDone = true
		if w.failAtDone {
			w.cancel()
			return 0, io.ErrClosedPipe
		}
	}
	n, err := w.ResponseRecorder.Write(p)
	if strings.Contains(w.Body.String(), "[DONE]\n\n") {
		w.seenDone = true
		if w.cancelAtDone {
			w.cancel()
		}
	}
	return n, err
}
func (w *streamTerminalWriter) WriteString(s string) (int, error) { return w.Write([]byte(s)) }
func (w *streamTerminalWriter) Flush()                            { w.ResponseRecorder.Flush() }

func TestChatStreamTerminalCancellationSettlementBothWriters(t *testing.T) {
	for _, mode := range []model.QuotaWriterMode{model.QuotaWriterModeLegacy, model.QuotaWriterModeAuthoritative} {
		for _, policy := range []string{"ordinary", "tokens", "fee-only"} {
			strict, feeOnly := policy != "ordinary", policy == "fee-only"
			for _, endpoint := range []string{"playground", "public-api"} {
				for _, terminal := range []string{"connected", "cancel-after-done", "done-write-failed", "upstream-eof", "missing-usage"} {
					if !strict && terminal != "connected" && terminal != "cancel-after-done" {
						continue
					}
					cancelDone, failDone := terminal == "cancel-after-done", terminal == "done-write-failed"
					t.Run(fmt.Sprintf("%s/%s/%s/%s", mode, endpoint, policy, terminal), func(t *testing.T) {
						db, user, key, access := playgroundControllerFixture(t, mode)
						// The notification limiter owns a permanent worker. Start
						// it before counting, then drain relay metrics/notifications
						// before the fixture restores shared DB/Redis settings.
						_, err := service.CheckNotificationLimit(user.Id, "stream-terminal-fixture")
						require.NoError(t, err)
						backgroundWorkers := gopool.WorkerCount()
						t.Cleanup(func() {
							require.Eventually(t, func() bool { return gopool.WorkerCount() <= backgroundWorkers }, 5*time.Second, time.Millisecond)
						})
						if strict {
							require.NoError(t, db.Create(&model.TokenBudget{TokenID: key.Id, UserID: user.Id, Enabled: !feeOnly, Limit: model.TokenBudgetOpenAIChatContext, FeeEnabled: feeOnly, FeeLimitUSD: "10", Revision: 1}).Error)
						}
						expectedQuota := 15
						if feeOnly {
							playgroundStrictFeePrices(t, db)
							expectedQuota = 35
						}
						channel := &model.Channel{Id: 51, Name: "Synthetic strict", Type: constant.ChannelTypeOpenAI, Key: "synthetic-only", Status: common.ChannelStatusEnabled, Group: "default", Models: playgroundRelayModel, BaseURL: common.GetPointer("https://api.openai.com"), OtherSettings: `{"allow_service_tier":true}`}
						require.NoError(t, db.Create(channel).Error)
						require.NoError(t, db.Create(&model.Ability{Group: "default", Model: playgroundRelayModel, ChannelId: channel.Id, Enabled: true, Weight: 1}).Error)
						client := playgroundStrictTextClient(t, func(w http.ResponseWriter, r *http.Request) {
							w.Header().Set("Content-Type", "text/event-stream")
							body := terminalSettlementStream
							if terminal == "upstream-eof" {
								body = strings.TrimSuffix(body, "data: [DONE]\n\n")
							}
							if terminal == "missing-usage" {
								frames := strings.Split(body, "\n\n")
								body = frames[0] + "\n\n" + frames[1] + "\n\ndata: [DONE]\n\n"
							}
							fmt.Fprint(w, body)
						})
						t.Cleanup(service.SetHttpClientForTest(client))
						req := httptest.NewRequest(http.MethodPost, "/pg/chat/completions", strings.NewReader(`{"model":"gpt-6.1-sol","messages":[{"role":"user","content":"hello"}],"stream":true,"max_completion_tokens":4096,"service_tier":"default","stream_options":{"include_usage":true}}`))
						req.Header.Set("Authorization", "Bearer "+access)
						req.Header.Set("Content-Type", "application/json")
						req.Header.Set("X-MyAPI-Key-ID", strconv.Itoa(key.Id))
						req.Header.Set("X-Test-Request-ID", "stream-terminal-cancel")
						ctx, cancel := context.WithCancel(req.Context())
						defer cancel()
						req = req.WithContext(ctx)
						out := &streamTerminalWriter{ResponseRecorder: httptest.NewRecorder(), cancel: cancel, cancelAtDone: cancelDone, failAtDone: failDone}
						router := playgroundControllerRouter()
						if endpoint == "public-api" {
							req.URL.Path = "/v1/chat/completions"
							req.Header.Set("Authorization", "Bearer sk-"+key.Key)
							router.POST("/v1/chat/completions", middleware.TokenAuth(), middleware.ModelRequestRateLimit(), middleware.Distribute(), func(c *gin.Context) { Relay(c, relaytypes.RelayFormatOpenAI) })
						}
						router.ServeHTTP(out, req)
						require.Equal(t, http.StatusOK, out.Code, out.Body.String())
						require.True(t, out.attemptedDone)
						require.Equal(t, !failDone, out.seenDone)
						require.Equal(t, cancelDone || failDone, ctx.Err() != nil)
						require.Contains(t, out.Body.String(), "Complete answer")
						var entries []model.Log
						require.NoError(t, db.Where("request_id = ?", "stream-terminal-cancel").Find(&entries).Error)
						require.Len(t, entries, 1)
						pending := strict && (failDone || terminal == "upstream-eof" || terminal == "missing-usage")
						if pending {
							require.Equal(t, "usage_pending_review", entries[0].Content)
						} else {
							require.Equal(t, model.LogTypeConsume, entries[0].Type)
							require.Equal(t, expectedQuota, entries[0].Quota)
							require.NotContains(t, entries[0].Other, `"settlement_status":"pending_review"`)
							require.NoError(t, db.First(key, key.Id).Error)
							require.NoError(t, db.First(user, user.Id).Error)
							require.Equal(t, 1000000-expectedQuota, key.RemainQuota)
							require.Equal(t, expectedQuota, key.UsedQuota)
							require.Equal(t, 1000000-expectedQuota, user.Quota)
						}
						if strict {
							var row model.TokenBudgetReservation
							require.NoError(t, db.First(&row, "request_id = ?", "stream-terminal-cancel").Error)
							want := model.TokenBudgetSettled
							if pending {
								want = model.TokenBudgetUnknown
							}
							require.Equal(t, want, row.State)
							budget, err := model.LookupTokenBudget(context.Background(), db, key.Id)
							require.NoError(t, err)
							if pending {
								require.Zero(t, budget.Used)
								require.EqualValues(t, model.TokenBudgetOpenAIChatContext, budget.Reserved)
								require.Nil(t, row.ActualInput)
								if feeOnly {
									require.Equal(t, "0", budget.FeeUsedUSD)
									require.NotEqual(t, "0", budget.FeeReservedUSD)
								}
							} else {
								require.EqualValues(t, 15, budget.Used)
								require.Zero(t, budget.Reserved)
								require.Empty(t, budget.PendingRequestID)
								if feeOnly {
									require.Equal(t, "0.00007", budget.FeeUsedUSD)
									require.Equal(t, "0", budget.FeeReservedUSD)
								}
							}
						}
					})
				}
			}
		}
	}

}
