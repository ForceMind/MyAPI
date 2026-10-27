package router

import (
	"context"
	"errors"
	"fmt"
	"net"
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
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/ForceMind/MyAPI/service"
	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/ForceMind/MyAPI/setting/model_setting"
	"github.com/ForceMind/MyAPI/setting/operation_setting"
	"github.com/ForceMind/MyAPI/setting/ratio_setting"
	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Only the external HTTP boundary is synthetic. Authentication, distribution,
// affinity templates, conversion and dispatch guards remain production paths.
// This is not evidence of provider cache hits or required upstream semantics.
func TestCodexSessionHeadersReachSyntheticUpstream(t *testing.T) {
	for _, test := range []struct {
		name         string
		model        string
		cacheKey     bool
		chat         bool
		channelPass  bool
		globalPass   bool
		headerConfig string
		wantSession  string
	}{
		{name: "matching affinity", model: "gpt-5-codex", cacheKey: true, wantSession: "client-session"},
		{name: "missing cache key", model: "gpt-5-codex", wantSession: "client-session"},
		{name: "model outside default affinity pattern", model: "codex-auto-review", cacheKey: true, wantSession: "client-session"},
		{name: "Chat converted after distribution", model: "gpt-5-codex", cacheKey: true, chat: true, wantSession: "client-session"},
		{name: "channel body passthrough", model: "gpt-5-codex", cacheKey: true, channelPass: true, wantSession: "client-session"},
		{name: "global body passthrough", model: "gpt-5-codex", cacheKey: true, globalPass: true, wantSession: "client-session"},
		{name: "explicit channel header wins", model: "gpt-5-codex", cacheKey: true, headerConfig: `{"Session_id":"operator-session"}`, wantSession: "operator-session"},
		{name: "explicit headers with body passthrough", model: "gpt-5-codex", cacheKey: true, channelPass: true, headerConfig: `{"Session_id":"operator-session","Thread_id":"{client_header:Thread_id}","X-Codex-Turn-State":"{client_header:X-Codex-Turn-State}"}`, wantSession: "operator-session"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newCodexSessionHeadersFixture(t, test.model, test.channelPass, test.globalPass, test.headerConfig)
			path := "/v1/responses"
			payload := map[string]any{
				"model": test.model, "input": "synthetic hello", "instructions": "", "store": false, "stream": true,
			}
			if test.chat {
				path = "/v1/chat/completions"
				payload = map[string]any{
					"model": test.model, "messages": []map[string]string{{"role": "user", "content": "synthetic hello"}}, "stream": true,
				}
			}
			if test.cacheKey {
				payload["prompt_cache_key"] = "synthetic-header-cache-key"
			}
			body, err := common.Marshal(payload)
			require.NoError(t, err)

			response := fixture.request(t, path, body, true)
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			require.EqualValues(t, 1, fixture.calls.Load(), "one request must cause exactly one generation")
			first := <-fixture.observed
			assert.Equal(t, "Bearer synthetic-header-access", first.Get("Authorization"))
			assert.Equal(t, "synthetic-header-account", first.Get("ChatGPT-Account-ID"))
			assert.Equal(t, test.wantSession, first.Get("Session_id"), "session identity must survive the selected relay path")
			assert.Equal(t, "client-thread", first.Get("Thread_id"), "thread identity must survive the selected relay path")
			assert.Equal(t, "client-turn-state", first.Get("X-Codex-Turn-State"), "turn state must survive the selected relay path")
			assert.Empty(t, first.Get("Cookie"), "client cookies must not reach the provider")
			assert.Empty(t, first.Get("X-Api-Key"), "unrelated credentials must not reach the provider")
			assert.Empty(t, first.Get("Proxy-Authorization"))
			assert.Empty(t, first.Get("X-Unrelated-Private"), "the template is not wildcard passthrough")
			assert.Equal(t, "synthetic-next-turn", response.Header().Get("X-Codex-Turn-State"))

			// The same cache key must not carry over the prior request's client
			// headers. A static operator override remains static.
			next := fixture.request(t, path, body, false)
			require.Equal(t, http.StatusOK, next.Code, next.Body.String())
			require.EqualValues(t, 2, fixture.calls.Load(), "no hidden retry or extra generation is permitted")
			second := <-fixture.observed
			wantNextSession := ""
			if test.headerConfig != "" {
				wantNextSession = "operator-session"
			}
			assert.Equal(t, wantNextSession, second.Get("Session_id"))
			assert.Empty(t, second.Get("Thread_id"))
			assert.Empty(t, second.Get("X-Codex-Turn-State"))
			assert.Zero(t, fixture.external.Load(), "all other network targets are denied")
		})
	}
}

func TestCodexSessionHeadersHonorHeaderOperations(t *testing.T) {
	for _, test := range []struct {
		name         string
		cacheKey     bool
		headerConfig string
		operations   []any
		wantSession  string
		wantNext     string
		deleteAll    bool
	}{
		{name: "delete defaults without affinity", deleteAll: true},
		{name: "delete after matching affinity", cacheKey: true, deleteAll: true},
		{
			name: "keep origin retains client session", wantSession: "client-session", wantNext: "operator-session",
			operations: []any{map[string]any{"mode": "set_header", "path": "Session_id", "value": "operator-session", "keep_origin": true}},
		},
		{
			name: "keep origin retains explicit channel session", headerConfig: `{"Session_id":"configured-session"}`, wantSession: "configured-session", wantNext: "configured-session",
			operations: []any{map[string]any{"mode": "set_header", "path": "Session_id", "value": "operator-session", "keep_origin": true}},
		},
		{
			name: "explicit operation replaces client session", wantSession: "operator-session", wantNext: "operator-session",
			operations: []any{map[string]any{"mode": "set_header", "path": "Session_id", "value": "operator-session"}},
		},
		{
			name: "explicit mapping reads client session", cacheKey: true, wantSession: "mapped-session",
			operations: []any{map[string]any{"mode": "set_header", "path": "Session_id", "value": map[string]any{"client-session": "mapped-session"}}},
		},
		{
			name: "unrelated body operation retains defaults", wantSession: "client-session",
			operations: []any{map[string]any{"mode": "set", "path": "instructions", "value": "synthetic instruction"}},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newCodexSessionHeadersFixture(t, "gpt-5-codex", false, false, test.headerConfig)
			operations := test.operations
			if test.deleteAll {
				for _, header := range []string{"Session_id", "Thread_id", "X-Codex-Turn-State"} {
					operations = append(operations, map[string]any{"mode": "delete_header", "path": header})
				}
			}
			paramConfig, err := common.Marshal(map[string]any{"operations": operations})
			require.NoError(t, err)
			require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", fixture.channelID).Update("param_override", string(paramConfig)).Error)
			payload := map[string]any{"model": "gpt-5-codex", "input": "synthetic hello", "instructions": "", "stream": true}
			if test.cacheKey {
				payload["prompt_cache_key"] = "synthetic-header-cache-key"
			}
			body, err := common.Marshal(payload)
			require.NoError(t, err)
			response := fixture.request(t, "/v1/responses", body, true)
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			require.EqualValues(t, 1, fixture.calls.Load())
			first := <-fixture.observed
			assert.Equal(t, test.wantSession, first.Get("Session_id"))
			if test.deleteAll {
				assert.Empty(t, first.Values("Session_id"), "deleted headers must be absent, not reintroduced or sent empty")
				assert.Empty(t, first.Values("Thread_id"))
				assert.Empty(t, first.Values("X-Codex-Turn-State"))
			} else {
				assert.Equal(t, "client-thread", first.Get("Thread_id"))
				assert.Equal(t, "client-turn-state", first.Get("X-Codex-Turn-State"))
			}
			next := fixture.request(t, "/v1/responses", body, false)
			require.Equal(t, http.StatusOK, next.Code, next.Body.String())
			require.EqualValues(t, 2, fixture.calls.Load())
			second := <-fixture.observed
			assert.Equal(t, test.wantNext, second.Get("Session_id"))
			assert.Empty(t, second.Get("Thread_id"))
			assert.Empty(t, second.Get("X-Codex-Turn-State"))
			assert.Zero(t, fixture.external.Load())
		})
	}
}

func TestCodexSessionHeadersDoNotReplayAmbiguousGeneration(t *testing.T) {
	fixture := newCodexSessionHeadersFixture(t, "gpt-5-codex", false, false, "")
	fixture.disconnect.Store(true)
	response := fixture.request(t, "/v1/responses", []byte(`{"model":"gpt-5-codex","input":"synthetic hello","stream":true,"prompt_cache_key":"synthetic-header-cache-key"}`), true)
	assert.NotContains(t, response.Body.String(), "response.completed")
	assert.EqualValues(t, 1, fixture.calls.Load(), "an upstream disconnect after receiving a generation must not cause replay")
	assert.Zero(t, fixture.external.Load())
}

func TestCodexSessionHeadersDoNotBypassStrictAcceptanceOverrideGate(t *testing.T) {
	fixture := newCodexSessionHeadersFixture(t, "gpt-5-codex", false, false, `{"Session_id":"operator-session"}`)
	t.Setenv("MYAPI_UPSTREAM_ACCEPTANCE_ENABLED", "true")
	run, err := service.StartUpstreamAcceptanceRun(context.Background(), fixture.channelID, service.UpstreamAcceptanceLimits{
		DurationSeconds: 60, MaxTotal: 1, MaxGeneration: 1,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, stopErr := service.StopUpstreamAcceptanceRun(context.Background(), run.ID)
		assert.NoError(t, stopErr)
	})
	response := fixture.request(t, "/v1/responses", []byte(`{"model":"gpt-5-codex","input":"synthetic hello","stream":true}`), true)
	assert.Contains(t, response.Body.String(), "upstream_acceptance_denied")
	assert.Zero(t, fixture.calls.Load(), "strict acceptance must refuse configured overrides before sending")
	assert.Zero(t, fixture.external.Load())
}

type codexSessionHeadersFixture struct {
	engine     *gin.Engine
	tokenKey   string
	channelID  int
	disconnect atomic.Bool
	calls      atomic.Int32
	external   atomic.Int32
	observed   chan http.Header
}

func newCodexSessionHeadersFixture(t *testing.T, modelName string, channelPass, globalPass bool, headerConfig string) *codexSessionHeadersFixture {
	t.Helper()
	t.Setenv("MYAPI_UPSTREAM_ACCEPTANCE_ENABLED", "false")
	t.Setenv(common.ChannelQuotaIdentityKeysEnv, "")
	setupRelayRouterTestDB(t)
	require.NoError(t, i18n.Init())
	db := model.DB
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
	common.MemoryCacheEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.RetryTimes = false, false, false, 0
	common.RelayFailureCooldownSeconds, common.RelayFailoverTimeoutSeconds, common.TLSInsecureSkipVerify = 0, 0, false
	constant.StreamingTimeout = 30
	oldRatios, oldGroups, oldPrices := ratio_setting.ModelRatio2JSONString(), ratio_setting.GroupRatio2JSONString(), ratio_setting.ModelPrice2JSONString()
	ratio, err := common.Marshal(map[string]int{modelName: 1})
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(ratio)))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{}`))
	global := config.GlobalConfig.Get("global")
	oldGlobal := model_setting.GetGlobalSettings()
	oldPolicy, err := common.Marshal(oldGlobal.ChatCompletionsToResponsesPolicy)
	require.NoError(t, err)
	affinity := config.GlobalConfig.Get("channel_affinity_setting")
	oldAffinity := operation_setting.GetChannelAffinitySetting()
	oldRules, err := common.Marshal(oldAffinity.Rules)
	require.NoError(t, err)
	var codexRules []operation_setting.ChannelAffinityRule
	for _, rule := range oldAffinity.Rules {
		if rule.Name == "codex cli trace" {
			codexRules = append(codexRules, rule)
		}
	}
	require.Len(t, codexRules, 1, "use the existing Codex rule, not a replacement policy")
	rules, err := common.Marshal(codexRules)
	require.NoError(t, err)
	require.NoError(t, config.UpdateConfigFromMap(affinity, map[string]string{"enabled": "true", "rules": string(rules)}))
	service.RebuildChannelAffinityCache()
	t.Cleanup(func() {
		common.MemoryCacheEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.RetryTimes = oldMemory, oldBatch, oldLog, oldRetries
		common.RelayFailureCooldownSeconds, common.RelayFailoverTimeoutSeconds, common.TLSInsecureSkipVerify = oldCooldown, oldTimeout, oldTLS
		constant.StreamingTimeout = oldStreaming
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldRatios))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldGroups))
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(oldPrices))
		require.NoError(t, config.UpdateConfigFromMap(global, map[string]string{
			"chat_completions_to_responses_policy": string(oldPolicy), "pass_through_request_enabled": strconv.FormatBool(oldGlobal.PassThroughRequestEnabled),
		}))
		require.NoError(t, config.UpdateConfigFromMap(affinity, map[string]string{"enabled": strconv.FormatBool(oldAffinity.Enabled), "rules": string(oldRules)}))
		service.RebuildChannelAffinityCache()
	})

	fixture := &codexSessionHeadersFixture{observed: make(chan http.Header, 2)}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fixture.calls.Add(1)
		select {
		case fixture.observed <- r.Header.Clone():
		default:
			t.Error("unexpected extra upstream request")
		}
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/backend-api/codex/responses", r.URL.Path)
		var payload struct {
			Model string `json:"model"`
		}
		if !assert.NoError(t, common.DecodeJson(r.Body, &payload)) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		assert.Equal(t, modelName, payload.Model)
		if fixture.disconnect.Load() {
			connection, _, err := w.(http.Hijacker).Hijack()
			if assert.NoError(t, err) {
				_ = connection.Close()
			}
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("X-Codex-Turn-State", "synthetic-next-turn")
		fmt.Fprintf(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"synthetic-header-response\",\"object\":\"response\",\"status\":\"completed\",\"model\":%q,\"output\":[],\"usage\":{\"input_tokens\":10,\"output_tokens\":5,\"total_tokens\":15,\"input_tokens_details\":{\"cached_tokens\":0}}}}\n\ndata: [DONE]\n\n", modelName)
	}))
	t.Cleanup(upstream.Close)
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			if address != upstream.Listener.Addr().String() {
				fixture.external.Add(1)
				return nil, errors.New("synthetic header fixture denies non-loopback target")
			}
			return (&net.Dialer{}).DialContext(ctx, network, address)
		},
	}
	t.Cleanup(transport.CloseIdleConnections)
	t.Cleanup(service.SetHttpClientForTest(&http.Client{Transport: transport, Timeout: 10 * time.Second}))
	user := model.User{Username: "header-owner", AffCode: "header-owner", Status: common.UserStatusEnabled, Group: "default", Quota: 1000000, AuthVersion: 1}
	require.NoError(t, db.Create(&user).Error)
	token := model.Token{UserId: user.Id, Key: "syntheticheaderkey", Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 1000000}
	require.NoError(t, db.Create(&token).Error)
	fixture.tokenKey = token.Key
	settings, err := common.Marshal(dto.ChannelSettings{PassThroughBodyEnabled: channelPass})
	require.NoError(t, err)
	settingsString := string(settings)
	channel := model.Channel{
		Type: constant.ChannelTypeCodex, Name: "synthetic-header-channel", Key: `{"access_token":"synthetic-header-access","account_id":"synthetic-header-account"}`,
		BaseURL: &upstream.URL, Status: common.ChannelStatusEnabled, Group: "default", Models: modelName, Setting: &settingsString,
	}
	if headerConfig != "" {
		channel.HeaderOverride = &headerConfig
	}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, channel.AddAbilities(nil))
	fixture.channelID = channel.Id
	policy, err := common.Marshal(model_setting.ChatCompletionsToResponsesPolicy{Enabled: true, ChannelIDs: []int{channel.Id}, ModelPatterns: []string{"^gpt-5-codex$"}})
	require.NoError(t, err)
	require.NoError(t, config.UpdateConfigFromMap(global, map[string]string{
		"chat_completions_to_responses_policy": string(policy), "pass_through_request_enabled": strconv.FormatBool(globalPass),
	}))
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set(common.RequestIdKey, fmt.Sprintf("synthetic-header-request-%d", fixture.calls.Load()))
		common.SetContextKey(c, constant.ContextKeyAuditLogged, true)
		c.Next()
	})
	engine.POST("/v1/responses", middleware.TokenAuth(), middleware.Distribute(), func(c *gin.Context) { controller.Relay(c, types.RelayFormatOpenAIResponses) })
	engine.POST("/v1/chat/completions", middleware.TokenAuth(), middleware.Distribute(), func(c *gin.Context) { controller.Relay(c, types.RelayFormatOpenAI) })
	fixture.engine = engine
	return fixture
}

func (fixture *codexSessionHeadersFixture) request(t *testing.T, path string, body []byte, withSession bool) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(body)))
	request.Header.Set("Authorization", "Bearer sk-"+fixture.tokenKey)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Cookie", "synthetic-private-cookie=do-not-forward")
	request.Header.Set("X-Api-Key", "synthetic-private-api-key")
	request.Header.Set("Proxy-Authorization", "synthetic-private-proxy")
	request.Header.Set("X-Unrelated-Private", "synthetic-private-value")
	if withSession {
		request.Header.Set("Session_id", "client-session")
		request.Header.Set("Thread_id", "client-thread")
		request.Header.Set("X-Codex-Turn-State", "client-turn-state")
	}
	response := httptest.NewRecorder()
	fixture.engine.ServeHTTP(response, request)
	// Drain existing asynchronous bookkeeping before restoring fixture state.
	require.Eventually(t, func() bool { return gopool.WorkerCount() == 0 }, 5*time.Second, time.Millisecond)
	return response
}
