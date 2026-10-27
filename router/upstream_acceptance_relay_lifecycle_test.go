package router

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
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
	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/ForceMind/MyAPI/setting/model_setting"
	"github.com/ForceMind/MyAPI/setting/ratio_setting"
	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Observe the real downstream write, after the upstream body has been consumed
// but before quota settlement and controller.Relay's deferred cleanup finish.
type acceptanceRelayLifecycleWriter struct {
	gin.ResponseWriter
	onWrite func()
	once    sync.Once
}

func (writer *acceptanceRelayLifecycleWriter) Write(data []byte) (int, error) {
	writer.once.Do(writer.onWrite)
	return writer.ResponseWriter.Write(data)
}

func (writer *acceptanceRelayLifecycleWriter) WriteString(data string) (int, error) {
	writer.once.Do(writer.onWrite)
	return writer.ResponseWriter.WriteString(data)
}

func TestUpstreamAcceptanceRelayRetainsOperationThroughControllerReturn(t *testing.T) {
	t.Setenv("MYAPI_UPSTREAM_ACCEPTANCE_ENABLED", "true")
	setupRelayRouterTestDB(t)
	require.NoError(t, i18n.Init())
	db := model.DB
	// The same fresh legacy-writer billing fixture used by actual relay failover
	// tests: settlement and source/account eligibility remain production paths.
	require.NoError(t, db.AutoMigrate(
		&model.UpstreamAcceptanceRun{}, &model.RelayAccountHold{}, &model.ChannelQuotaSnapshot{}, &model.Option{},
		&model.SubscriptionPlan{}, &model.UserSubscription{}, &model.SubscriptionPreConsumeRecord{},
		&model.UserQuotaMutationReceipt{}, &model.AccountQuotaMutationReceipt{}, &model.AccountQuotaReservationHead{},
		&model.AccountQuotaTerminalRecoveryObligation{}, &model.AccountQuotaRefundFact{},
		&model.AccountQuotaSettlementIntent{}, &model.AccountQuotaSettlementFact{}, &model.LegacyUsageReservation{},
		&model.SystemTask{}, &model.SystemTaskLock{}, &model.QuotaWriterEpoch{}, &model.QuotaProjectionObligation{},
		&model.QuotaBalanceBatchDrain{}, &model.QuotaBalanceBatchSubject{}, &model.QuotaWorkCursor{},
	))
	require.NoError(t, model.EnsureQuotaWriterEpochStateWithDB(db))
	oldMemory, oldBatch, oldLog, oldRetries := common.MemoryCacheEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.RetryTimes
	oldCooldown, oldTimeout, oldTLS := common.RelayFailureCooldownSeconds, common.RelayFailoverTimeoutSeconds, common.TLSInsecureSkipVerify
	oldStreaming := constant.StreamingTimeout
	common.MemoryCacheEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.RetryTimes = false, false, true, 0
	common.RelayFailureCooldownSeconds, common.RelayFailoverTimeoutSeconds, common.TLSInsecureSkipVerify = 0, 0, false
	constant.StreamingTimeout = 30
	oldRatios, oldGroups, oldPrices := ratio_setting.ModelRatio2JSONString(), ratio_setting.GroupRatio2JSONString(), ratio_setting.ModelPrice2JSONString()
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gpt-5-codex":1}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{}`))
	global := config.GlobalConfig.Get("global")
	oldGlobal := model_setting.GetGlobalSettings()
	oldPolicy, err := common.Marshal(oldGlobal.ChatCompletionsToResponsesPolicy)
	require.NoError(t, err)
	t.Cleanup(func() {
		common.MemoryCacheEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.RetryTimes = oldMemory, oldBatch, oldLog, oldRetries
		common.RelayFailureCooldownSeconds, common.RelayFailoverTimeoutSeconds, common.TLSInsecureSkipVerify = oldCooldown, oldTimeout, oldTLS
		constant.StreamingTimeout = oldStreaming
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldRatios))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldGroups))
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(oldPrices))
		require.NoError(t, config.UpdateConfigFromMap(global, map[string]string{
			"chat_completions_to_responses_policy": string(oldPolicy),
			"pass_through_request_enabled":         strconv.FormatBool(oldGlobal.PassThroughRequestEnabled),
		}))
	})

	user := model.User{Username: "acceptance-relay-owner", AffCode: "acceptance-relay-owner", Status: common.UserStatusEnabled, Group: "default", Quota: 1000000, AuthVersion: 1}
	require.NoError(t, db.Create(&user).Error)
	token := model.Token{UserId: user.Id, Key: "acceptancerelayfixture", Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 1000000}
	require.NoError(t, db.Create(&token).Error)
	originalTokenID, originalTokenKey := token.Id, token.Key

	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		assert.Equal(t, "/backend-api/codex/responses", r.URL.Path)
		assert.Equal(t, "Bearer synthetic-relay-access", r.Header.Get("Authorization"))
		assert.Equal(t, "synthetic-relay-account", r.Header.Get("ChatGPT-Account-ID"))
		var request struct {
			Model                string `json:"model"`
			Stream               bool   `json:"stream"`
			Store                bool   `json:"store"`
			PromptCacheKey       string `json:"prompt_cache_key"`
			PromptCacheRetention string `json:"prompt_cache_retention"`
		}
		if !assert.NoError(t, common.DecodeJson(r.Body, &request)) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		assert.Equal(t, "gpt-5-codex", request.Model)
		assert.True(t, request.Stream)
		assert.False(t, request.Store)
		assert.Equal(t, "synthetic-cache-session", request.PromptCacheKey)
		assert.Equal(t, "24h", request.PromptCacheRetention)
		payload := "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"acceptance-relay-response\",\"object\":\"response\",\"status\":\"completed\",\"model\":\"gpt-5-codex\",\"output\":[{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"synthetic reply\"}]}],\"usage\":{\"input_tokens\":10,\"output_tokens\":5,\"total_tokens\":15}}}\n\n"
		w.Header().Set("Content-Type", "text/event-stream")
		// An exact finite length makes EOF observable in the final upstream read,
		// independent of socket scheduling or a delayed server-side close.
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		_, _ = fmt.Fprint(w, payload)
	}))
	t.Cleanup(upstream.Close)
	t.Cleanup(service.SetHttpClientForTest(upstream.Client()))
	credential, err := common.Marshal(service.CodexOAuthKey{AccessToken: "synthetic-relay-access", RefreshToken: "synthetic-relay-refresh", AccountID: "synthetic-relay-account"})
	require.NoError(t, err)
	channel := model.Channel{Type: constant.ChannelTypeCodex, Name: "acceptance-relay", Key: string(credential), BaseURL: &upstream.URL, Status: common.ChannelStatusEnabled, Group: "default", Models: "gpt-5-codex"}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, channel.AddAbilities(nil))
	policy, err := common.Marshal(model_setting.ChatCompletionsToResponsesPolicy{Enabled: true, ChannelIDs: []int{channel.Id}, ModelPatterns: []string{"^gpt-5-codex$"}})
	require.NoError(t, err)
	require.NoError(t, config.UpdateConfigFromMap(global, map[string]string{
		"chat_completions_to_responses_policy": string(policy), "pass_through_request_enabled": "false",
	}))
	run, err := service.StartUpstreamAcceptanceRun(context.Background(), channel.Id, service.UpstreamAcceptanceLimits{DurationSeconds: 60, MaxTotal: 1, MaxGeneration: 1})
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = service.StopUpstreamAcceptanceRun(context.Background(), run.ID) })

	var atWrite, atStop, afterRelay *service.UpstreamAcceptanceStatus
	var observeErr, stopErr, afterRelayErr error
	wroteResponse := false
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set(common.RequestIdKey, "acceptance-real-relay-request")
		common.SetContextKey(c, constant.ContextKeyAuditLogged, true)
		c.Writer = &acceptanceRelayLifecycleWriter{ResponseWriter: c.Writer, onWrite: func() {
			wroteResponse = true
			atWrite, observeErr = service.GetUpstreamAcceptanceStatus(context.Background(), run.ID)
			if observeErr == nil && atWrite.HTTPInFlight == 0 && atWrite.Operations == 1 {
				atStop, stopErr = service.StopUpstreamAcceptanceRun(context.Background(), run.ID)
			}
		}}
		c.Next()
	})
	engine.POST("/v1/chat/completions", middleware.TokenAuth(), middleware.Distribute(), func(c *gin.Context) {
		controller.Relay(c, types.RelayFormatOpenAI)
		afterRelay, afterRelayErr = service.GetUpstreamAcceptanceStatus(context.Background(), run.ID)
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5-codex","messages":[{"role":"user","content":"synthetic hello"}],"max_tokens":10,"stream":false,"prompt_cache_key":"synthetic-cache-session","prompt_cache_retention":"24h"}`))
	req.Header.Set("Authorization", "Bearer sk-"+token.Key)
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, req)
	// Drain existing asynchronous relay bookkeeping before restoring its DB.
	require.Eventually(t, func() bool { return gopool.WorkerCount() == 0 }, 5*time.Second, time.Millisecond)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.True(t, wroteResponse)
	require.NoError(t, observeErr)
	require.NotNil(t, atWrite)
	assert.Zero(t, atWrite.HTTPInFlight)
	assert.Equal(t, 1, atWrite.Operations)
	require.NoError(t, stopErr)
	require.NotNil(t, atStop, "the operation must still be registered when downstream JSON is written")
	assert.Equal(t, model.UpstreamAcceptanceRunStopping, atStop.Run.Status)
	assert.Equal(t, 1, atStop.Operations)
	require.NoError(t, afterRelayErr)
	require.NotNil(t, afterRelay)
	assert.Equal(t, model.UpstreamAcceptanceRunStopped, afterRelay.Run.Status)
	assert.Zero(t, afterRelay.Operations)
	assert.Zero(t, afterRelay.HTTPInFlight)
	assert.False(t, afterRelay.DrainError)
	assert.Equal(t, int64(1), afterRelay.Run.Generation)
	assert.Equal(t, int64(1), afterRelay.Run.Total)
	assert.Equal(t, int32(1), upstreamCalls.Load())
	assert.Contains(t, response.Body.String(), "synthetic reply")
	assert.Contains(t, response.Header().Get("Content-Type"), "application/json")
	require.NoError(t, db.First(&token, originalTokenID).Error)
	assert.Equal(t, originalTokenID, token.Id)
	assert.Equal(t, originalTokenKey, token.Key)
	assert.Greater(t, token.UsedQuota, int64(0), "stopping after known usage must still settle the normal paid request")
	assert.Equal(t, int64(1000000)-token.UsedQuota, token.RemainQuota)
	var storedChannel model.Channel
	require.NoError(t, db.First(&storedChannel, channel.Id).Error)
	assert.Equal(t, channel.Key, storedChannel.Key)
}
