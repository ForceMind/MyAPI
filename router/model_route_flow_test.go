package router

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/ForceMind/MyAPI/service"
	"github.com/ForceMind/MyAPI/setting/ratio_setting"
	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestModelRouteActualApplicationFlow(t *testing.T) {
	setupRelayRouterTestDB(t)
	require.NoError(t, i18n.Init())
	oldStreaming := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldStreaming })
	db := model.DB
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelModelDiscovery{}, &model.ChannelQuotaSnapshot{}, &model.Option{}, &model.SubscriptionPlan{}, &model.UserSubscription{}, &model.SubscriptionPreConsumeRecord{}, &model.UserQuotaMutationReceipt{}, &model.AccountQuotaMutationReceipt{}, &model.AccountQuotaReservationHead{}, &model.AccountQuotaTerminalRecoveryObligation{}, &model.AccountQuotaRefundFact{}, &model.AccountQuotaSettlementIntent{}, &model.AccountQuotaSettlementFact{}, &model.LegacyUsageReservation{}, &model.SystemTask{}, &model.SystemTaskLock{}, &model.QuotaWriterEpoch{}, &model.QuotaProjectionObligation{}, &model.QuotaBalanceBatchDrain{}, &model.QuotaBalanceBatchSubject{}, &model.QuotaWorkCursor{}))
	require.NoError(t, model.EnsureQuotaWriterEpochStateWithDB(db))
	oldMemory, oldBatch, oldLog := common.MemoryCacheEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled
	common.MemoryCacheEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled = false, false, true
	oldModelRatio, oldGroupRatio := ratio_setting.ModelRatio2JSONString(), ratio_setting.GroupRatio2JSONString()
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"public-model":1,"actual-a":1,"actual-b":1}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
	t.Cleanup(func() {
		common.MemoryCacheEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled = oldMemory, oldBatch, oldLog
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldModelRatio))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldGroupRatio))
	})
	var hits atomic.Int32
	upstreams := make([]*httptest.Server, 0, 2)
	for _, target := range []string{"actual-a", "actual-b"} {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/v1/models" {
				fmt.Fprintf(w, `{"data":[{"id":%q}]}`, target)
				return
			}
			var payload struct {
				Model string `json:"model"`
			}
			require.NoError(t, common.DecodeJson(r.Body, &payload))
			require.Equal(t, target, payload.Model)
			hits.Add(1)
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Path == "/v1/responses" {
				fmt.Fprintf(w, `{"id":"fixture-response","object":"response","status":"completed","model":%q,"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15}}`, target)
				return
			}

			fmt.Fprintf(w, `{"id":"fixture","object":"chat.completion","model":%q,"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`, target)
		}))
		upstreams = append(upstreams, upstream)
		defer upstream.Close()
	}
	t.Cleanup(service.SetHttpClientForTest(upstreams[0].Client()))
	user := model.User{Username: "route-owner", AffCode: "route-owner", Status: common.UserStatusEnabled, Group: "default", Quota: 1000000, AuthVersion: 1}
	require.NoError(t, db.Create(&user).Error)
	token := model.Token{UserId: user.Id, Key: "routekeyfixture", Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 1000000, ModelLimitsEnabled: true, ModelLimits: "public-model"}
	require.NoError(t, db.Create(&token).Error)
	rootPAT := "route-root-pat"
	root := model.User{Username: "route-root", AffCode: "route-root", Status: common.UserStatusEnabled, Role: common.RoleRootUser, AccessToken: &rootPAT}
	require.NoError(t, db.Create(&root).Error)
	channelIDs := []int{}
	for index, target := range []string{"actual-a", "actual-b"} {
		settings, err := common.Marshal(dto.ChannelOtherSettings{ModelRoutes: []dto.ModelRoute{{PublicModel: "public-model", UpstreamModel: target, Endpoint: "/v1/chat/completions", Match: "exact"}, {PublicModel: "public-model", UpstreamModel: target, Endpoint: "/v1/responses", Match: "exact"}}})
		require.NoError(t, err)
		priority := int64(10 - index)
		channel := model.Channel{Type: constant.ChannelTypeOpenAI, Name: "route-" + target, Key: "synthetic", BaseURL: &upstreams[index].URL, Status: common.ChannelStatusEnabled, Models: "public-model", Group: "default", Priority: &priority, OtherSettings: string(settings)}
		require.NoError(t, db.Create(&channel).Error)
		require.NoError(t, channel.AddAbilities(nil))
		channelIDs = append(channelIDs, channel.Id)
	}
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set(common.RequestIdKey, "route-request-"+strconv.Itoa(int(hits.Load())))
		common.SetContextKey(c, constant.ContextKeyAuditLogged, true)
		c.Next()
	})
	SetApiRouter(engine)
	SetRelayRouter(engine)
	request := func(method, path, auth, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Host = "myapi.local"
		req.Header.Set("Authorization", "Bearer "+auth)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://myapi.local")
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, req)
		// Relay records failed-request performance asynchronously. Complete those
		// workers before this fixture restores process-wide Redis/database state.
		require.Eventually(t, func() bool { return gopool.WorkerCount() == 0 }, 5*time.Second, time.Millisecond)
		return response
	}
	for _, id := range channelIDs {
		path := "/api/channel/model-discovery/" + strconv.Itoa(id)
		forbidden := request(http.MethodPost, path, "sk-"+token.Key, "")
		require.NotEqual(t, http.StatusOK, forbidden.Code)
		discovered := request(http.MethodPost, path, rootPAT, "")
		require.Equal(t, http.StatusOK, discovered.Code, discovered.Body.String())
		require.Contains(t, discovered.Body.String(), `"status":"success"`)
	}
	preview := request(http.MethodGet, "/api/channel/routing-preview?group=default&model=public-model&request_path=/v1/chat/completions", rootPAT, "")
	require.Equal(t, http.StatusOK, preview.Code, preview.Body.String())
	require.Contains(t, preview.Body.String(), `"upstream_model":"actual-a"`)
	require.Contains(t, preview.Body.String(), `"upstream_model":"actual-b"`)
	body := `{"model":"public-model","messages":[{"role":"user","content":"hi"}],"max_tokens":10}`
	denied := request(http.MethodPost, "/v1/chat/completions", "sk-"+token.Key, body)
	require.Equal(t, http.StatusForbidden, denied.Code, denied.Body.String())
	require.Zero(t, hits.Load())
	require.NoError(t, db.Model(&model.Token{}).Where("id = ?", token.Id).Update("model_limits", "public-model,actual-a,actual-b").Error)
	response := request(http.MethodPost, "/v1/chat/completions", "sk-"+token.Key, body)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), "ok")
	require.EqualValues(t, 1, hits.Load())
	var logs []model.Log
	require.NoError(t, db.Where("type = ?", model.LogTypeConsume).Find(&logs).Error)
	require.Len(t, logs, 1)
	require.Contains(t, logs[0].Other, `"model_route"`)
	require.Contains(t, logs[0].Other, `"upstream_model":"actual-a"`)
	require.Contains(t, logs[0].Other, `"reason":"explicit_exact"`)
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", channelIDs[0]).Update("status", common.ChannelStatusManuallyDisabled).Error)
	response = request(http.MethodPost, "/v1/chat/completions", "sk-"+token.Key, body)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.EqualValues(t, 2, hits.Load())
	logs = nil
	require.NoError(t, db.Where("type = ?", model.LogTypeConsume).Order("id").Find(&logs).Error)
	require.Len(t, logs, 2)
	require.Contains(t, logs[1].Other, `"upstream_model":"actual-b"`)
	responses := request(http.MethodPost, "/v1/responses", "sk-"+token.Key, `{"model":"public-model","input":"hi","max_output_tokens":10}`)
	require.Equal(t, http.StatusOK, responses.Code, responses.Body.String())
	require.EqualValues(t, 3, hits.Load())
	require.NoError(t, db.First(&token, token.Id).Error)
	require.Greater(t, token.UsedQuota, 0)
	require.Equal(t, 1000000-token.UsedQuota, token.RemainQuota)
	codex := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Model string `json:"model"`
		}
		require.NoError(t, common.DecodeJson(r.Body, &payload))
		require.Equal(t, "gpt-5-codex", payload.Model)
		require.Equal(t, "/backend-api/codex/responses", r.URL.Path)
		hits.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"codex-fixture\",\"object\":\"response\",\"status\":\"completed\",\"model\":\"gpt-5-codex\",\"output\":[],\"usage\":{\"input_tokens\":10,\"output_tokens\":5,\"total_tokens\":15}}}\n\ndata: [DONE]\n\n")
	}))
	defer codex.Close()
	codexSettings, err := common.Marshal(dto.ChannelOtherSettings{ModelRoutes: []dto.ModelRoute{{PublicModel: "public-model", UpstreamModel: "gpt-5-codex", Endpoint: "/v1/responses", Match: "exact"}}})
	require.NoError(t, err)
	codexPriority := int64(100)
	codexChannel := model.Channel{Type: constant.ChannelTypeCodex, Name: "codex-route", Key: `{"access_token":"synthetic","account_id":"fixture-account"}`, BaseURL: &codex.URL, Status: common.ChannelStatusEnabled, Group: "default", Models: "public-model", Priority: &codexPriority, OtherSettings: string(codexSettings)}
	require.NoError(t, db.Create(&codexChannel).Error)
	require.NoError(t, codexChannel.AddAbilities(nil))
	require.NoError(t, db.Model(&model.Token{}).Where("id = ?", token.Id).Update("model_limits", "public-model,actual-a,actual-b,gpt-5-codex").Error)
	response = request(http.MethodPost, "/v1/responses", "sk-"+token.Key, `{"model":"public-model","input":"hi","max_output_tokens":10,"stream":true}`)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.EqualValues(t, 4, hits.Load())
	require.NoError(t, db.First(&token, token.Id).Error)
	usedBefore := token.UsedQuota
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", channelIDs[1]).Update("param_override", `{"model":"hidden-target"}`).Error)
	blocked := request(http.MethodPost, "/v1/chat/completions", "sk-"+token.Key, body)
	require.Equal(t, http.StatusBadRequest, blocked.Code, blocked.Body.String())
	require.EqualValues(t, 4, hits.Load(), "post-override target changes never reach upstream")
	require.NoError(t, db.First(&token, token.Id).Error)
	require.Equal(t, usedBefore, token.UsedQuota, "rejection before dispatch must not charge")

}
