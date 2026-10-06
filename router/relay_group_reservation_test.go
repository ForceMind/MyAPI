package router

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/controller"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/middleware"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/ForceMind/MyAPI/service"
	"github.com/ForceMind/MyAPI/setting"
	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/ForceMind/MyAPI/setting/operation_setting"
	"github.com/ForceMind/MyAPI/setting/ratio_setting"
	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRelayFailoverGroupReservation(t *testing.T) {
	for _, mode := range []model.QuotaWriterMode{model.QuotaWriterModeLegacy, model.QuotaWriterModeAuthoritative} {
		for _, fixed := range []bool{false, true} {
			for _, test := range []struct {
				name                                   string
				status                                 int
				disconnect, lowToken, lowWallet, topUp bool
			}{
				{name: "server failure holds paid reservation", status: 503},
				{name: "transport loss holds paid reservation", disconnect: true},
				{name: "success settles paid usage once", status: 200},
				{name: "definite refusal refunds paid reservation", status: 429},
				{name: "token budget denies paid dispatch", status: 200, lowToken: true},
				{name: "wallet budget denies paid dispatch", status: 200, lowWallet: true},
				{name: "higher paid group reserves before send", status: 503, topUp: true},
			} {
				t.Run(fmt.Sprintf("%s/fixed=%t/%s", mode, fixed, test.name), func(t *testing.T) {
					setupRelayRouterTestDB(t)
					db := model.DB
					require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelQuotaSnapshot{}, &model.RelayAccountHold{}, &model.Option{}, &model.SubscriptionPlan{}, &model.UserSubscription{}, &model.SubscriptionPreConsumeRecord{}, &model.UserQuotaMutationReceipt{}, &model.AccountQuotaMutationReceipt{}, &model.AccountQuotaReservationHead{}, &model.AccountQuotaTerminalRecoveryObligation{}, &model.AccountQuotaRefundFact{}, &model.AccountQuotaSettlementIntent{}, &model.AccountQuotaSettlementFact{}, &model.LegacyUsageReservation{}, &model.SystemTask{}, &model.SystemTaskLock{}, &model.QuotaWriterEpoch{}, &model.QuotaProjectionObligation{}, &model.QuotaBalanceBatchDrain{}, &model.QuotaBalanceBatchSubject{}, &model.QuotaWorkCursor{}))
					require.NoError(t, model.EnsureQuotaWriterEpochStateWithDB(db))
					require.NoError(t, db.Model(&model.QuotaWriterEpoch{}).Where("id = ?", 1).Updates(map[string]any{"mode": string(mode), "epoch": int64(41)}).Error)
					require.True(t, model.RefreshAccountQuotaSettlementIntentSchemaCapability(db))
					require.NoError(t, i18n.Init())
					oldMemory, oldBatch, oldLog, oldRetries := common.MemoryCacheEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.RetryTimes
					oldPreConsume, oldUnit, oldCount := common.PreConsumedQuota, common.QuotaPerUnit, constant.CountToken
					oldCooldown, oldTimeout := common.RelayFailureCooldownSeconds, common.RelayFailoverTimeoutSeconds
					common.MemoryCacheEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.RetryTimes = false, false, true, 1
					common.PreConsumedQuota, common.QuotaPerUnit, constant.CountToken = 500, 500000, false
					common.RelayFailureCooldownSeconds, common.RelayFailoverTimeoutSeconds = 0, 0
					t.Cleanup(func() {
						common.MemoryCacheEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.RetryTimes = oldMemory, oldBatch, oldLog, oldRetries
						common.PreConsumedQuota, common.QuotaPerUnit, constant.CountToken = oldPreConsume, oldUnit, oldCount
						common.RelayFailureCooldownSeconds, common.RelayFailoverTimeoutSeconds = oldCooldown, oldTimeout
					})
					oldRatios, oldPrices, oldGroups := ratio_setting.ModelRatio2JSONString(), ratio_setting.ModelPrice2JSONString(), ratio_setting.GroupRatio2JSONString()
					oldCompletion := ratio_setting.CompletionRatio2JSONString()
					oldUsable, oldMax := setting.UserUsableGroups2JSONString(), setting.GetMaxTokenAutoGroups()
					originalFree := operation_setting.GetQuotaSetting().EnableFreeModelPreConsume
					quotaConfig := config.GlobalConfig.Get("quota_setting")
					require.NoError(t, config.UpdateConfigFromMap(quotaConfig, map[string]string{"enable_free_model_pre_consume": "false"}))
					require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"public":1,"actual":1}`))
					require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(`{"public":1}`))
					require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{}`))
					if fixed {
						require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"public":0.01}`))
					}
					initialRatio := 0.0
					if test.topUp {
						initialRatio = 0.5
					}
					require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(fmt.Sprintf(`{"default":%g,"vip":1}`, initialRatio)))
					require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default","vip":"VIP","auto":"Auto"}`))
					require.NoError(t, setting.UpdateMaxTokenAutoGroups("2"))
					t.Cleanup(func() {
						require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldRatios))
						require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(oldCompletion))
						require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(oldPrices))
						require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldGroups))
						require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(oldUsable))
						require.NoError(t, setting.UpdateMaxTokenAutoGroups(strconv.Itoa(oldMax)))
						require.NoError(t, config.UpdateConfigFromMap(quotaConfig, map[string]string{"enable_free_model_pre_consume": strconv.FormatBool(originalFree)}))
					})
					user := model.User{Username: "group-owner", AffCode: "group-owner", Status: common.UserStatusEnabled, Group: "default", Quota: 1000000, AuthVersion: 1}
					if test.lowWallet {
						user.Quota = 100
					}
					require.NoError(t, db.Create(&user).Error)
					token := model.Token{UserId: user.Id, Key: "groupreservationkey", Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 1000000, Group: "auto", AutoGroups: `["default","vip"]`, CrossGroupRetry: true}
					if test.lowToken {
						token.RemainQuota = 100
					}
					require.NoError(t, db.Create(&token).Error)
					initialUser, initialToken := user.Quota, token.RemainQuota
					reserved, actual := 510, 15
					if fixed {
						reserved, actual = 5000, 5000
					}
					var calls [2]atomic.Int32
					for index := range 2 {
						upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							calls[index].Add(1)
							_, _ = io.Copy(io.Discard, r.Body)
							if index == 0 {
								w.WriteHeader(http.StatusTooManyRequests)
								fmt.Fprint(w, `{"error":{"message":"synthetic refusal","type":"rate_limit_error"}}`)
								return
							}
							// The paid target must already have durable accounting evidence.
							var currentToken model.Token
							require.NoError(t, db.First(&currentToken, token.Id).Error)
							require.Equal(t, initialToken-reserved, currentToken.RemainQuota)
							view, err := model.GetUsageReview(r.Context(), db, user.Id, "group-reservation-request")
							require.NoError(t, err)
							require.EqualValues(t, reserved, view.ReservedQuota)
							require.True(t, view.TextDispatchPending)
							if test.disconnect {
								conn, _, err := w.(http.Hijacker).Hijack()
								require.NoError(t, err)
								require.NoError(t, conn.Close())
								return
							}
							w.Header().Set("Content-Type", "application/json")
							w.WriteHeader(test.status)
							if test.status != 200 {
								fmt.Fprint(w, `{"error":{"message":"synthetic failure","type":"upstream_error"}}`)
								return
							}
							fmt.Fprint(w, `{"id":"ok","object":"chat.completion","model":"actual","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`)
						}))
						t.Cleanup(upstream.Close)
						if index == 0 {
							t.Cleanup(service.SetHttpClientForTest(upstream.Client()))
						}
						mapping := `{"public":"actual"}`
						channel := model.Channel{Type: constant.ChannelTypeOpenAI, Name: fmt.Sprintf("group-%d", index), Key: "synthetic", BaseURL: &upstream.URL, Status: common.ChannelStatusEnabled, Models: "public", Group: []string{"default", "vip"}[index], ModelMapping: &mapping}
						require.NoError(t, db.Create(&channel).Error)
						require.NoError(t, channel.AddAbilities(nil))
					}
					engine := gin.New()
					engine.Use(func(c *gin.Context) {
						c.Set(common.RequestIdKey, "group-reservation-request")
						common.SetContextKey(c, constant.ContextKeyAuditLogged, true)
						c.Next()
					})
					engine.POST("/v1/chat/completions", middleware.TokenAuth(), middleware.Distribute(), func(c *gin.Context) { controller.Relay(c, types.RelayFormatOpenAI) })
					req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"public","messages":[{"role":"user","content":"hi"}],"max_tokens":10}`))
					req.Header.Set("Authorization", "Bearer sk-"+token.Key)
					req.Header.Set("Content-Type", "application/json")
					response := httptest.NewRecorder()
					engine.ServeHTTP(response, req)
					require.Eventually(t, func() bool { return gopool.WorkerCount() == 0 }, 5*time.Second, time.Millisecond)
					require.EqualValues(t, 1, calls[0].Load(), response.Body.String())
					wantPaid, wantStatus, charged := 1, test.status, actual
					unknown := test.disconnect || test.status == 503
					if unknown {
						charged = reserved
					}
					if test.disconnect {
						wantStatus = 500
					}
					if test.status == 429 {
						charged = 0
					}
					if test.lowToken || test.lowWallet {
						wantPaid, wantStatus, charged = 0, 403, 0
					}
					require.EqualValues(t, wantPaid, calls[1].Load(), response.Body.String())
					require.Equal(t, wantStatus, response.Code, response.Body.String())
					require.NoError(t, db.First(&token, token.Id).Error)
					require.NoError(t, db.First(&user, user.Id).Error)
					require.Equal(t, initialToken-charged, token.RemainQuota)
					require.Equal(t, initialUser-charged, user.Quota)
					if unknown {
						view, err := model.GetUsageReview(req.Context(), db, user.Id, "group-reservation-request")
						require.NoError(t, err)
						require.Equal(t, "usage_unknown", view.State)
						require.EqualValues(t, reserved, view.ReservedQuota)
						require.Nil(t, view.ActualQuota)
					} else if wantStatus == 200 {
						require.Equal(t, actual, token.UsedQuota)
						var logs []model.Log
						require.NoError(t, db.Where("type = ?", model.LogTypeConsume).Find(&logs).Error)
						require.Len(t, logs, 1)
						require.Equal(t, actual, logs[0].Quota)
					}
				})
			}
		}
	}
}
