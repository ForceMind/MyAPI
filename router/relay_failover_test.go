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

func TestRelayFailoverActualDispatch(t *testing.T) {
	for _, test := range []struct {
		name                                                                                                                   string
		changedSchedulingConfig, affinity, cooldown, secondRequest, deadline                                                   bool
		status                                                                                                                 int
		sameTarget, free, multi, revoke, partial, fixed, disconnect, organization, autoGroups, changedKey, changedOrganization bool
		retries, wantA, wantB, wantStatus                                                                                      int
	}{
		{name: "dispatch settings changed after selection stop dispatch", sameTarget: true, changedSchedulingConfig: true, retries: 2, wantStatus: 403},
		{name: "success account affinity follows identity and retires replacement", status: 429, sameTarget: true, multi: true, free: true, affinity: true, retries: 2, wantA: 2, wantB: 0, wantStatus: 200},
		{name: "cooldown preserves unknown then routes future request", status: 503, sameTarget: true, free: true, cooldown: true, secondRequest: true, retries: 2, wantA: 1, wantB: 0, wantStatus: 503},
		{name: "overall deadline preserves fixed price hold", sameTarget: true, fixed: true, deadline: true, retries: 2, wantA: 1, wantB: 0, wantStatus: 500},
		{name: "cooldown retains healthy sibling eligibility", status: 429, sameTarget: true, multi: true, cooldown: true, retries: 2, wantA: 2, wantB: 0, wantStatus: 200},
		{name: "explicit refusal selects healthy same model", status: 429, sameTarget: true, retries: 2, wantA: 1, wantB: 1, wantStatus: 200},
		{name: "healthy credential in same channel remains usable", status: 429, sameTarget: true, multi: true, retries: 2, wantA: 2, wantB: 0, wantStatus: 200},
		{name: "different real model never receives failover", status: 429, retries: 2, wantA: 1, wantB: 0, wantStatus: 500},
		{name: "attempt limit is total sends", status: 429, sameTarget: true, retries: 0, wantA: 1, wantB: 0, wantStatus: 429},
		{name: "auto group resets never replenish total attempts", status: 429, sameTarget: true, autoGroups: true, retries: 1, wantA: 1, wantB: 1, wantStatus: 429},
		{name: "credential replaced after selection stops dispatch", sameTarget: true, changedKey: true, retries: 2, wantStatus: 403},
		{name: "organization replaced after selection stops dispatch", sameTarget: true, changedOrganization: true, retries: 2, wantStatus: 403},
		{name: "fixed price definite refusal refunds", status: 429, sameTarget: true, fixed: true, retries: 0, wantA: 1, wantStatus: 429},
		{name: "fixed price refusal then success charges once", status: 429, sameTarget: true, fixed: true, retries: 2, wantA: 1, wantB: 1, wantStatus: 200},
		{name: "fixed price server failure holds reservation", status: 503, sameTarget: true, fixed: true, retries: 2, wantA: 1, wantStatus: 503},
		{name: "fixed price disconnect holds reservation", sameTarget: true, fixed: true, disconnect: true, retries: 2, wantA: 1, wantStatus: 500},
		{name: "organization from first channel never leaks to fallback", status: 429, sameTarget: true, organization: true, retries: 2, wantA: 1, wantB: 1, wantStatus: 200},
		{name: "ambiguous paid rejection holds one dispatch", status: 503, sameTarget: true, retries: 2, wantA: 1, wantB: 0, wantStatus: 503},
		{name: "ambiguous free rejection never replays", status: 503, sameTarget: true, free: true, retries: 2, wantA: 1, wantB: 0, wantStatus: 503},
		{name: "partial free stream never replays", sameTarget: true, free: true, partial: true, retries: 2, wantA: 1, wantB: 0, wantStatus: 200},
		{name: "revoked token stops fallback before send", status: 429, sameTarget: true, revoke: true, retries: 2, wantA: 1, wantB: 0, wantStatus: 403},
	} {
		t.Run(test.name, func(t *testing.T) {
			setupRelayRouterTestDB(t)
			oldCooldown, oldTimeout := common.RelayFailureCooldownSeconds, common.RelayFailoverTimeoutSeconds
			common.RelayFailureCooldownSeconds, common.RelayFailoverTimeoutSeconds = 0, 0
			if test.cooldown {
				common.RelayFailureCooldownSeconds = 60
			}
			if test.deadline {
				common.RelayFailoverTimeoutSeconds = 1
			}
			t.Cleanup(func() {
				common.RelayFailureCooldownSeconds, common.RelayFailoverTimeoutSeconds = oldCooldown, oldTimeout
			})
			require.NoError(t, model.DB.AutoMigrate(&model.RelayAccountHold{}))
			require.NoError(t, i18n.Init())
			oldStreaming := constant.StreamingTimeout
			constant.StreamingTimeout = 30
			t.Cleanup(func() { constant.StreamingTimeout = oldStreaming })
			db := model.DB
			require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelQuotaSnapshot{}, &model.Option{}, &model.SubscriptionPlan{}, &model.UserSubscription{}, &model.SubscriptionPreConsumeRecord{}, &model.UserQuotaMutationReceipt{}, &model.AccountQuotaMutationReceipt{}, &model.AccountQuotaReservationHead{}, &model.AccountQuotaTerminalRecoveryObligation{}, &model.AccountQuotaRefundFact{}, &model.AccountQuotaSettlementIntent{}, &model.AccountQuotaSettlementFact{}, &model.LegacyUsageReservation{}, &model.SystemTask{}, &model.SystemTaskLock{}, &model.QuotaWriterEpoch{}, &model.QuotaProjectionObligation{}, &model.QuotaBalanceBatchDrain{}, &model.QuotaBalanceBatchSubject{}, &model.QuotaWorkCursor{}))
			require.NoError(t, model.EnsureQuotaWriterEpochStateWithDB(db))
			oldMemory, oldBatch, oldLog, oldRetries := common.MemoryCacheEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.RetryTimes
			common.MemoryCacheEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.RetryTimes = false, false, true, test.retries
			oldRatios, oldGroups := ratio_setting.ModelRatio2JSONString(), ratio_setting.GroupRatio2JSONString()
			oldPrices := ratio_setting.ModelPrice2JSONString()
			require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{}`))
			if test.fixed {
				require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"public":0.01}`))
			}
			t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(oldPrices)) })
			originalFree := operation_setting.GetQuotaSetting().EnableFreeModelPreConsume
			quotaConfig := config.GlobalConfig.Get("quota_setting")
			require.NoError(t, config.UpdateConfigFromMap(quotaConfig, map[string]string{"enable_free_model_pre_consume": "false"}))
			t.Cleanup(func() {
				require.NoError(t, config.UpdateConfigFromMap(quotaConfig, map[string]string{"enable_free_model_pre_consume": strconv.FormatBool(originalFree)}))
			})
			price := 1
			if test.free {
				price = 0
			}
			require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(fmt.Sprintf(`{"public":%d,"actual":1,"different":1}`, price)))
			require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
			t.Cleanup(func() {
				common.MemoryCacheEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.RetryTimes = oldMemory, oldBatch, oldLog, oldRetries
				require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldRatios))
				require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldGroups))
			})
			if test.autoGroups {
				oldUsable, oldMax := setting.UserUsableGroups2JSONString(), setting.GetMaxTokenAutoGroups()
				require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default","vip":"VIP","third":"Third","auto":"Auto"}`))
				require.NoError(t, setting.UpdateMaxTokenAutoGroups("3"))
				require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1,"third":1}`))
				t.Cleanup(func() {
					require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(oldUsable))
					require.NoError(t, setting.UpdateMaxTokenAutoGroups(strconv.Itoa(oldMax)))
				})
			}
			user := model.User{Username: "failover-owner", AffCode: "failover-owner", Status: common.UserStatusEnabled, Group: "default", Quota: 1000000, AuthVersion: 1}
			require.NoError(t, db.Create(&user).Error)
			token := model.Token{UserId: user.Id, Key: "failoverkey", Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 1000000}
			if test.autoGroups {
				token.Group = "auto"
				token.AutoGroups = `["default","vip","third"]`
				token.CrossGroupRetry = true
			}
			require.NoError(t, db.Create(&token).Error)
			var calls [3]atomic.Int32
			channelCount := 2
			if test.autoGroups {
				channelCount = 3
			}
			for index := 0; index < channelCount; index++ {
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls[index].Add(1)
					if test.deadline && index == 0 {
						io.Copy(io.Discard, r.Body)
						<-r.Context().Done()
						return
					}
					if test.organization {
						expected := ""
						if index == 0 {
							expected = "org-fixture"
						}
						require.Equal(t, expected, r.Header.Get("OpenAI-Organization"))
					}
					if test.disconnect && index == 0 {
						conn, _, err := w.(http.Hijacker).Hijack()
						require.NoError(t, err)
						require.NoError(t, conn.Close())
						return
					}
					if index == 0 && test.partial {
						w.Header().Set("Content-Type", "text/event-stream")
						fmt.Fprint(w, "data: {\"id\":\"partial\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"partial\"}}]}\n\n")
						w.(http.Flusher).Flush()
						return
					}
					if (index == 0 || test.autoGroups) && !(test.multi && r.Header.Get("Authorization") == "Bearer healthy") {
						if test.revoke {
							require.NoError(t, db.Model(&model.Token{}).Where("id = ?", token.Id).Update("status", common.TokenStatusDisabled).Error)
						}
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(test.status)
						fmt.Fprint(w, `{"error":{"message":"synthetic refusal","type":"rate_limit_error","code":"rate_limit"}}`)
						return
					}
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprint(w, `{"id":"ok","object":"chat.completion","model":"actual","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`)
				}))
				t.Cleanup(upstream.Close)
				if index == 0 {
					t.Cleanup(service.SetHttpClientForTest(upstream.Client()))
				}
				priority := int64(10 - index)
				target := "actual"
				if index == 1 && !test.sameTarget {
					target = "different"
				}
				mapping := fmt.Sprintf(`{"public":%q}`, target)
				channel := model.Channel{Type: constant.ChannelTypeOpenAI, Name: "failover-" + strconv.Itoa(index), Key: "failed", BaseURL: &upstream.URL, Status: common.ChannelStatusEnabled, Models: "public", Group: "default", Priority: &priority, ModelMapping: &mapping}
				if test.autoGroups {
					channel.Group = []string{"default", "vip", "third"}[index]
				}
				if index == 0 && test.organization {
					organization := "org-fixture"
					channel.OpenAIOrganization = &organization
				}
				if index == 0 && test.multi {
					channel.Key = "failed\nhealthy"
					channel.ChannelInfo = model.ChannelInfo{IsMultiKey: true, MultiKeySize: 2, MultiKeyMode: constant.MultiKeyModePolling}
				}
				require.NoError(t, db.Create(&channel).Error)
				require.NoError(t, channel.AddAbilities(nil))
			}
			if test.affinity {
				original := operation_setting.GetChannelAffinitySetting()
				registry := config.GlobalConfig.Get("channel_affinity_setting")
				originalRules, err := common.Marshal(original.Rules)
				require.NoError(t, err)
				rules, err := common.Marshal([]operation_setting.ChannelAffinityRule{{Name: "actual-account", ModelRegex: []string{"^public$"}, PathRegex: []string{"^/v1/chat/completions$"}, KeySources: []operation_setting.ChannelAffinityKeySource{{Type: "request_header", Key: "X-Affinity-Fixture"}}, TTLSeconds: 60, IncludeRuleName: true, IncludeModelName: true}})
				require.NoError(t, err)
				require.NoError(t, config.UpdateConfigFromMap(registry, map[string]string{"enabled": "true", "rules": string(rules)}))
				service.RebuildChannelAffinityCache()
				t.Cleanup(func() {
					require.NoError(t, config.UpdateConfigFromMap(registry, map[string]string{"enabled": strconv.FormatBool(original.Enabled), "rules": string(originalRules)}))
					service.RebuildChannelAffinityCache()
				})
			}
			var affinityEligible bool
			engine := gin.New()
			engine.Use(func(c *gin.Context) {
				c.Set(common.RequestIdKey, "failover-request")
				common.SetContextKey(c, constant.ContextKeyAuditLogged, true)
				c.Next()
				affinityEligible = c.GetBool("relay_success")
			})
			engine.POST("/v1/chat/completions", middleware.TokenAuth(), middleware.Distribute(), func(c *gin.Context) {
				if test.changedSchedulingConfig {
					require.NoError(t, db.Model(&model.Channel{}).Where("name = ?", "failover-0").Update("settings", `{"disable_store":true}`).Error)
				}
				if test.changedKey {
					require.NoError(t, db.Model(&model.Channel{}).Where("name = ?", "failover-0").Update("key", "replacement").Error)
				}
				if test.changedOrganization {
					require.NoError(t, db.Model(&model.Channel{}).Where("name = ?", "failover-0").Updates(&model.Channel{OpenAIOrganization: common.GetPointer("changed-org")}).Error)
				}
				controller.Relay(c, types.RelayFormatOpenAI)
			})
			body := `{"model":"public","messages":[{"role":"user","content":"hi"}],"max_tokens":10}`
			if test.partial {
				body = `{"model":"public","messages":[{"role":"user","content":"hi"}],"max_tokens":10,"stream":true}`
			}
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
			req.Header.Set("Authorization", "Bearer sk-"+token.Key)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Affinity-Fixture", t.Name())
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, req)
			require.Eventually(t, func() bool { return gopool.WorkerCount() == 0 }, 5*time.Second, time.Millisecond)
			require.EqualValues(t, test.wantA, calls[0].Load(), response.Body.String())
			require.EqualValues(t, test.wantB, calls[1].Load(), response.Body.String())
			require.Zero(t, calls[2].Load(), "third group must not bypass total attempt bound")
			require.Equal(t, test.wantStatus, response.Code, response.Body.String())
			require.Equal(t, test.wantStatus == 200 && !test.partial, affinityEligible, "only confirmed successful completion may update affinity")
			require.NoError(t, db.First(&token, token.Id).Error)
			if test.fixed && (test.status == 503 || test.disconnect || test.deadline) {
				require.Equal(t, 995000, token.RemainQuota, "unknown dispatch retains fixed-price reservation")
				review, err := model.GetUsageReview(req.Context(), db, user.Id, "failover-request")
				require.NoError(t, err)
				require.Equal(t, "usage_unknown", review.State)
				require.Nil(t, review.ActualQuota)
			}
			if test.fixed && test.retries == 0 && test.status == 429 {
				require.Equal(t, 1000000, token.RemainQuota)
				require.Zero(t, token.UsedQuota)
			}
			if test.affinity {
				requestAgain := func() {
					next := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
					next.Header.Set("Authorization", "Bearer sk-"+token.Key)
					next.Header.Set("Content-Type", "application/json")
					next.Header.Set("X-Affinity-Fixture", t.Name())
					rec := httptest.NewRecorder()
					engine.ServeHTTP(rec, next)
					require.Eventually(t, func() bool { return gopool.WorkerCount() == 0 }, 5*time.Second, time.Millisecond)
					require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
				}
				requestAgain()
				require.EqualValues(t, 3, calls[0].Load(), "second success must keep healthy account despite polling cursor")
				require.Zero(t, calls[1].Load())
				require.NoError(t, db.Model(&model.Channel{}).Where("name = ?", "failover-0").Update("key", "healthy\nfailed").Error)
				requestAgain()
				require.EqualValues(t, 4, calls[0].Load(), "reordering must keep the same account")
				require.Zero(t, calls[1].Load())
				require.NoError(t, db.Model(&model.Channel{}).Where("name = ?", "failover-0").Update("key", "replacement\nfailed").Error)
				requestAgain()
				require.EqualValues(t, 1, calls[1].Load(), "replaced account must fall back through normal gates")
			}
			if test.secondRequest {
				next := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
				next.Header.Set("Authorization", "Bearer sk-"+token.Key)
				next.Header.Set("Content-Type", "application/json")
				nextResponse := httptest.NewRecorder()
				engine.ServeHTTP(nextResponse, next)
				require.Eventually(t, func() bool { return gopool.WorkerCount() == 0 }, 5*time.Second, time.Millisecond)
				require.Equal(t, 200, nextResponse.Code, nextResponse.Body.String())
				require.EqualValues(t, 1, calls[0].Load(), "cooling account cannot receive a future request")
				require.EqualValues(t, 1, calls[1].Load())
			}
			if (test.wantB == 1 || test.multi) && !test.autoGroups && !test.free {
				require.Greater(t, token.UsedQuota, 0)
				if test.fixed {
					require.Equal(t, 5000, token.UsedQuota)
				}
				require.Equal(t, 1000000-token.UsedQuota, token.RemainQuota)
				var logs []model.Log
				require.NoError(t, db.Where("type = ?", model.LogTypeConsume).Find(&logs).Error)
				require.Len(t, logs, 1)
				require.Contains(t, logs[0].Other, `"relay_attempts"`)
				require.Contains(t, logs[0].Other, "retryable_refusal")
				require.Contains(t, logs[0].Other, `"outcome":"completed"`)
			}
		})
	}
}
