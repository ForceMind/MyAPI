package router

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"math/big"
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
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/ForceMind/MyAPI/service"
	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/ForceMind/MyAPI/setting/operation_setting"
	"github.com/ForceMind/MyAPI/setting/ratio_setting"
	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const chatFlowUsage = `{"prompt_tokens":100,"completion_tokens":10,"total_tokens":110,"prompt_tokens_details":{"cached_tokens":40,"cache_write_tokens":30},"completion_tokens_details":{"reasoning_tokens":4}}`
const chatFlowRequest = `{"model":"gpt-6.1-sol","messages":[{"role":"user","content":"hello"}],"max_completion_tokens":20,"n":1,"service_tier":"default"}`
const chatFlowPriceDocument = `Prices per 1M tokens.
### Standard pricing data
| Model | Short context input | Short context cached input | Short context cache writes | Short context output | Long context input | Long context cached input | Long context cache writes | Long context output |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gpt-6.1-sol | $2 | $0.1 | $2.5 | $10 | $4 | $0.2 | $5 | $15 |

Short context: ≤272K input tokens. Long context: >272K input tokens.
`

type chatBudgetFlowFixture struct {
	engine  *gin.Engine
	user    model.User
	root    model.User
	channel model.Channel
	serial  atomic.Int64
	hits    atomic.Int64
	serve   atomic.Pointer[http.HandlerFunc]
}

// This is a real TLS connection to an isolated local fixture. The outbound URL,
// SNI, certificate validation, and exact official-host guard remain unchanged.
func chatBudgetFlowTLSClient(t *testing.T, handler http.HandlerFunc) *http.Client {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	certificate := &x509.Certificate{SerialNumber: big.NewInt(1), DNSNames: []string{"api.openai.com"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IsCA: true, BasicConstraintsValid: true}
	der, err := x509.CreateCertificate(rand.Reader, certificate, certificate, &key.PublicKey, key)
	require.NoError(t, err)
	parsed, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	server := httptest.NewUnstartedServer(handler)
	server.TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}, MinVersion: tls.VersionTLS12}
	server.StartTLS()
	roots := x509.NewCertPool()
	roots.AddCert(parsed)
	transport := server.Client().Transport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.TLSClientConfig = &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if address != "api.openai.com:443" {
			return nil, fmt.Errorf("unexpected fixture destination: %s", address)
		}
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}
	t.Cleanup(func() { transport.CloseIdleConnections(); server.Close() })
	return &http.Client{Transport: transport, Timeout: 10 * time.Second}
}

func setupChatBudgetFlow(t *testing.T) *chatBudgetFlowFixture {
	t.Helper()
	setupRelayRouterTestDB(t)
	require.NoError(t, i18n.Init())
	db := model.DB
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelQuotaSnapshot{}, &model.Option{},
		&model.SubscriptionPlan{}, &model.UserSubscription{}, &model.SubscriptionPreConsumeRecord{},
		&model.UserQuotaMutationReceipt{}, &model.AccountQuotaMutationReceipt{}, &model.AccountQuotaReservationHead{},
		&model.AccountQuotaTerminalRecoveryObligation{}, &model.AccountQuotaRefundFact{}, &model.AccountQuotaSettlementIntent{},
		&model.AccountQuotaSettlementFact{}, &model.LegacyUsageReservation{}, &model.SystemTask{}, &model.SystemTaskLock{},
		&model.QuotaWriterEpoch{}, &model.QuotaProjectionObligation{}, &model.QuotaBalanceBatchDrain{}, &model.QuotaBalanceBatchSubject{}, &model.QuotaWorkCursor{},
		&model.TokenBudgetReservation{}, &model.TokenBudgetPolicyChange{}, &model.UsageReviewDecision{},
		&model.OfficialPriceVersion{}, &model.PricePublication{}, &model.RelayAccountHold{}))
	require.NoError(t, model.EnsureQuotaWriterEpochStateWithDB(db))
	require.True(t, model.RefreshAccountQuotaSettlementIntentSchemaCapability(db))
	oldMemory, oldBatch, oldLog := common.MemoryCacheEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled
	oldCritical, oldGlobal, oldRetries := common.CriticalRateLimitEnable, common.GlobalApiRateLimitEnable, common.RetryTimes
	oldStreaming, oldUnit := constant.StreamingTimeout, common.QuotaPerUnit
	oldRatios, oldPrices, oldGroups := ratio_setting.ModelRatio2JSONString(), ratio_setting.ModelPrice2JSONString(), ratio_setting.GroupRatio2JSONString()
	oldAudio, oldAudioCompletion := ratio_setting.AudioRatio2JSONString(), ratio_setting.AudioCompletionRatio2JSONString()
	savedConfig := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error { savedConfig[key] = value; return nil }))
	common.OptionMapRWMutex.Lock()
	oldOptions := common.OptionMap
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()
	common.MemoryCacheEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled = false, false, true
	common.CriticalRateLimitEnable, common.GlobalApiRateLimitEnable, common.RetryTimes = false, false, 0
	constant.StreamingTimeout, common.QuotaPerUnit = 30, 500000
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gpt-6.1-sol":1,"chat-alias":1,"incompatible-target":1}`))
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
	require.NoError(t, ratio_setting.UpdateAudioRatioByJSONString(`{"gpt-6.1-sol":2}`))
	require.NoError(t, ratio_setting.UpdateAudioCompletionRatioByJSONString(`{"gpt-6.1-sol":4}`))
	require.NoError(t, config.UpdateConfigFromMap(config.GlobalConfig.Get("global"), map[string]string{"chat_completions_to_responses_policy": `{"enabled":false}`}))
	t.Cleanup(func() {
		// Remove only this isolated database's pricing state, then republish an
		// empty provenance generation before restoring the pre-test settings.
		require.NoError(t, db.Where("1 = 1").Delete(&model.Option{}).Error)
		model.InitOptionMap()
		require.NoError(t, config.GlobalConfig.LoadFromDB(savedConfig))
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldRatios))
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(oldPrices))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldGroups))
		require.NoError(t, ratio_setting.UpdateAudioRatioByJSONString(oldAudio))
		require.NoError(t, ratio_setting.UpdateAudioCompletionRatioByJSONString(oldAudioCompletion))
		common.MemoryCacheEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled = oldMemory, oldBatch, oldLog
		common.CriticalRateLimitEnable, common.GlobalApiRateLimitEnable, common.RetryTimes = oldCritical, oldGlobal, oldRetries
		constant.StreamingTimeout, common.QuotaPerUnit = oldStreaming, oldUnit
		common.OptionMapRWMutex.Lock()
		common.OptionMap = oldOptions
		common.OptionMapRWMutex.Unlock()
	})
	var funding model.UserFundingStateSnapshot
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		var err error
		funding, err = model.InitializeUserFundingStateTx(tx, operation_setting.UserFundingModeDisabled)
		return err
	}))
	require.NoError(t, model.PublishUserFundingState(funding))
	fixture := &chatBudgetFlowFixture{}
	fixture.user = model.User{Username: "chat-budget-owner", AffCode: "chat-budget-owner", Status: common.UserStatusEnabled, Group: "default", Quota: 1000000, AuthVersion: 1}
	rootPAT := "chat-budget-root-pat"
	fixture.root = model.User{Username: "chat-budget-root", AffCode: "chat-budget-root", Status: common.UserStatusEnabled, Role: common.RoleRootUser, AccessToken: &rootPAT}
	require.NoError(t, db.Create(&fixture.user).Error)
	require.NoError(t, db.Create(&fixture.root).Error)
	baseURL := "https://api.openai.com"
	fixture.channel = model.Channel{Type: constant.ChannelTypeOpenAI, Name: "synthetic-chat-budget", Key: "synthetic-only", BaseURL: &baseURL,
		Status: common.ChannelStatusEnabled, Models: model.TokenBudgetOpenAIChatModel + ",chat-alias", Group: "default"}
	fixture.channel.SetOtherSettings(dto.ChannelOtherSettings{AllowServiceTier: true})
	require.NoError(t, db.Create(&fixture.channel).Error)
	require.NoError(t, fixture.channel.AddAbilities(nil))
	fixture.engine = gin.New()
	fixture.engine.Use(func(c *gin.Context) {
		c.Set(common.RequestIdKey, "chat-flow-"+strconv.FormatInt(fixture.serial.Add(1), 10))
		common.SetContextKey(c, constant.ContextKeyAuditLogged, true)
		c.Next()
	})
	SetApiRouter(fixture.engine)
	SetRelayRouter(fixture.engine)
	client := chatBudgetFlowTLSClient(t, func(w http.ResponseWriter, r *http.Request) {
		fixture.hits.Add(1)
		assert.Equal(t, "api.openai.com", r.Host)
		assert.Equal(t, "/v1/chat/completions", r.URL.Path, "Chat never calls the Responses token-count endpoint")
		assert.Equal(t, "Bearer synthetic-only", r.Header.Get("Authorization"))
		var payload map[string]any
		body, err := io.ReadAll(r.Body)
		if !assert.NoError(t, err) || !assert.NoError(t, common.Unmarshal(body, &payload)) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		assert.Equal(t, model.TokenBudgetOpenAIChatModel, payload["model"])
		assert.Equal(t, "default", payload["service_tier"], "the explicitly qualified tier must survive the channel adaptor")
		assert.EqualValues(t, 20, payload["max_completion_tokens"])
		assert.EqualValues(t, 1, payload["n"])
		(*fixture.serve.Load())(w, r)
	})
	t.Cleanup(service.SetHttpClientForTest(client))
	t.Cleanup(func() {
		// No persistent workers belong to this fixture. Drain every request before
		// restoring process-wide settings or closing its temporary database.
		require.Eventually(t, func() bool { return gopool.WorkerCount() == 0 }, 5*time.Second, time.Millisecond)
	})
	fixture.publish(t, chatFlowPriceDocument, strings.Repeat("a", 64))
	return fixture
}

func (f *chatBudgetFlowFixture) setResponder(handler http.HandlerFunc) { f.serve.Store(&handler) }

func (f *chatBudgetFlowFixture) request(method, path, auth, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Host = "myapi.local"
	req.Header.Set("Authorization", "Bearer "+auth)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://myapi.local")
	response := httptest.NewRecorder()
	f.engine.ServeHTTP(response, req)
	return response
}

func (f *chatBudgetFlowFixture) publish(t *testing.T, document, id string) string {
	t.Helper()
	// The stored document is synthetic. Production source reparsing and the
	// Root-authenticated preview/publication endpoints perform qualification.
	source, err := model.StoreOfficialPriceVersion(context.Background(), model.DB, document, time.Now().Unix())
	require.NoError(t, err)
	response := f.request(http.MethodGet, "/api/ratio_sync/openai/versions/"+source.ContentSHA256+"/publication-preview", *f.root.AccessToken, "")
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var preview struct {
		Data service.PricePublicationPreview
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &preview))
	require.Len(t, preview.Data.Rows, 1)
	require.True(t, preview.Data.Rows[0].Eligible)
	body, err := common.Marshal(service.PricePublicationRequest{ID: id, ExpectedDigest: preview.Data.ExpectedDigest, Action: "publish", SourceSHA256: source.ContentSHA256,
		Confirmed: true, Models: []service.PricePublicationSelection{{Model: model.TokenBudgetOpenAIChatModel, Locked: common.GetPointer(false)}}})
	require.NoError(t, err)
	response = f.request(http.MethodPost, "/api/ratio_sync/openai/publications", *f.root.AccessToken, string(body))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), `"runtime_ready":true`)
	return source.ContentSHA256
}

func (f *chatBudgetFlowFixture) token(t *testing.T, limits string) model.Token {
	t.Helper()
	key := fmt.Sprintf("chatbudgetfixture%d", f.serial.Add(1))
	token := model.Token{UserId: f.user.Id, Name: key, Key: key, Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 1000000,
		ModelLimitsEnabled: true, ModelLimits: limits}
	require.NoError(t, model.DB.Create(&token).Error)
	body := fmt.Sprintf(`{"id":"%064x","expected_revision":0,"enabled":true,"limit":2000000,"confirmed":true,"fee":{"enabled":true,"limit_usd":"10"}}`, f.serial.Add(1))
	response := f.request(http.MethodPut, fmt.Sprintf("/api/token/%d/budget", token.Id), *f.root.AccessToken, body)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	return token
}

func chatBudgetFlowResponse(usage string, stream, done bool) string {
	if !stream {
		return `{"id":"chat-fixture","object":"chat.completion","model":"gpt-6.1-sol","service_tier":"default","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":` + usage + `}`
	}
	body := "data: " + `{"id":"chat-fixture","object":"chat.completion.chunk","model":"gpt-6.1-sol","choices":[{"index":0,"delta":{"content":"ok"},"finish_reason":null}],"usage":null}` + "\n\n"
	body += "data: " + `{"id":"chat-fixture","object":"chat.completion.chunk","model":"gpt-6.1-sol","service_tier":"default","choices":[],"usage":` + usage + "}\n\n"
	if done {
		body += "data: [DONE]\n\n"
	}
	return body
}

func TestChatBudgetActualApplicationFlow(t *testing.T) {
	f := setupChatBudgetFlow(t)
	for _, test := range []struct {
		name, usage, cost     string
		stream, done, pending bool
		input, output         int64
	}{
		{name: "reported cache and reasoning", usage: chatFlowUsage, cost: "0.000239", input: 100, output: 10},
		{name: "long input uses frozen long rates", usage: strings.NewReplacer(`"prompt_tokens":100`, `"prompt_tokens":272001`, `"total_tokens":110`, `"total_tokens":272011`).Replace(chatFlowUsage), cost: "1.088032", input: 272001, output: 10},
		{name: "unexpected audio with configured audio pricing", usage: strings.Replace(chatFlowUsage, `"cached_tokens":40`, `"cached_tokens":40,"audio_tokens":1`, 1), pending: true},
		{name: "explicit zero", usage: `{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0,"prompt_tokens_details":{"cached_tokens":0,"cache_write_tokens":0}}`, cost: "0"},
		{name: "missing cache counter retains both holds", usage: strings.Replace(chatFlowUsage, `,"cache_write_tokens":30`, "", 1), pending: true},
		{name: "cache overlap retains both holds", usage: strings.Replace(chatFlowUsage, `"cache_write_tokens":30`, `"cache_write_tokens":61`, 1), pending: true},
		{name: "terminal stream", usage: chatFlowUsage, stream: true, done: true, cost: "0.000239", input: 100, output: 10},
		{name: "EOF with complete usage retains both holds", usage: chatFlowUsage, stream: true, pending: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			token := f.token(t, model.TokenBudgetOpenAIChatModel)
			f.setResponder(func(w http.ResponseWriter, r *http.Request) {
				contentType := "application/json"
				if test.stream {
					contentType = "text/event-stream"
				}
				w.Header().Set("Content-Type", contentType)
				_, _ = io.WriteString(w, chatBudgetFlowResponse(test.usage, test.stream, test.done))
			})
			body := chatFlowRequest
			if test.stream {
				body = strings.TrimSuffix(body, "}") + `,"stream":true,"stream_options":{"include_usage":true}}`
			}
			before := f.hits.Load()
			response := f.request(http.MethodPost, "/v1/chat/completions", "sk-"+token.Key, body)
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			require.Equal(t, before+1, f.hits.Load())
			var row model.TokenBudgetReservation
			require.NoError(t, model.DB.Where("token_id = ?", token.Id).First(&row).Error)
			require.Equal(t, model.TokenBudgetBoundOpenAIChat, row.BoundSource)
			require.EqualValues(t, 1050000, row.InputTokens)
			require.EqualValues(t, 1050000, row.Reserved, "input and completion share one context ceiling")
			require.EqualValues(t, 20, row.MaxOutputTokens)
			require.Equal(t, "5.2502", row.FeeReservedUSD)
			require.Contains(t, row.FeePriceEvidence, `"version":2`)
			require.Contains(t, row.FeePriceEvidence, `"long_rates"`)
			policy, err := model.LookupTokenBudget(context.Background(), model.DB, token.Id)
			require.NoError(t, err)
			var logs []model.Log
			require.NoError(t, model.DB.Where("request_id = ? AND type = ?", row.RequestID, model.LogTypeConsume).Find(&logs).Error)
			if test.pending {
				require.Equal(t, model.TokenBudgetUnknown, row.State)
				require.Nil(t, row.ActualInput)
				require.Nil(t, row.ActualFeeUSD)
				require.Zero(t, policy.Used)
				require.Equal(t, "0", policy.FeeUsedUSD)
				require.EqualValues(t, 1050000, policy.Reserved)
				require.Equal(t, "5.2502", policy.FeeReservedUSD)
				require.Equal(t, row.RequestID, policy.PendingRequestID)
				require.Empty(t, logs, "unknown usage cannot appear in confirmed consumption")
				legacy, err := model.FindLegacyUsageReservation(context.Background(), model.DB, row.RequestID)
				require.NoError(t, err)
				require.Equal(t, model.LegacyUsageUnknown, legacy.State)
				require.Greater(t, legacy.ReservedQuota, int64(0))
				require.Nil(t, legacy.ActualQuota)
				blocked := f.request(http.MethodPost, "/v1/chat/completions", "sk-"+token.Key, body)
				require.Equal(t, http.StatusConflict, blocked.Code, blocked.Body.String())
				require.Equal(t, before+1, f.hits.Load())
				var pending model.Log
				require.NoError(t, model.DB.Where("request_id = ? AND type = ?", row.RequestID, model.LogTypeError).First(&pending).Error)
				require.Contains(t, pending.Other, `"settlement_status":"pending_review"`)
				if test.name == "missing cache counter retains both holds" {
					f.recover(t, token, row)
				}
				return
			}
			require.Equal(t, model.TokenBudgetSettled, row.State)
			require.NotNil(t, row.ActualInput)
			require.NotNil(t, row.ActualOutput)
			require.NotNil(t, row.ActualFeeUSD)
			require.Equal(t, test.input, *row.ActualInput)
			require.Equal(t, test.output, *row.ActualOutput)
			require.Equal(t, test.cost, *row.ActualFeeUSD)
			require.Equal(t, test.input+test.output, policy.Used, "cache and reasoning are included subsets")
			require.Equal(t, test.cost, policy.FeeUsedUSD)
			require.Zero(t, policy.Reserved)
			require.Equal(t, "0", policy.FeeReservedUSD)
			require.Empty(t, policy.PendingRequestID)
			require.Len(t, logs, 1)
			require.EqualValues(t, test.input, logs[0].PromptTokens)
			require.EqualValues(t, test.output, logs[0].CompletionTokens)
			require.Contains(t, logs[0].Other, `"amount_usd":"`+test.cost+`"`)
			require.Contains(t, logs[0].Other, `"bound_source":"openai_chat_context_window"`)
			require.NoError(t, model.DB.First(&token, token.Id).Error)
			require.Equal(t, logs[0].Quota, token.UsedQuota)
			require.Equal(t, 1000000-token.UsedQuota, token.RemainQuota)
		})
	}
}

func (f *chatBudgetFlowFixture) recover(t *testing.T, token model.Token, row model.TokenBudgetReservation) {
	t.Helper()
	path := fmt.Sprintf("/api/token/%d/budget/recover", token.Id)
	body := fmt.Sprintf(`{"request_id":%q,"action":"reconcile","actual_quota":120,"actual_input_tokens":100,"actual_output_tokens":10,"actual_fee_usd":"0.000239","evidence_reference":"synthetic verified terminal fixture","confirmed_reliable_evidence":true}`, row.RequestID)
	denied := f.request(http.MethodPost, path, "sk-"+token.Key, body)
	require.NotEqual(t, http.StatusOK, denied.Code)
	for range 2 {
		response := f.request(http.MethodPost, path, *f.root.AccessToken, body)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	}
	policy, err := model.LookupTokenBudget(context.Background(), model.DB, token.Id)
	require.NoError(t, err)
	require.EqualValues(t, 110, policy.Used)
	require.Equal(t, "0.000239", policy.FeeUsedUSD)
	require.Zero(t, policy.Reserved)
	require.Equal(t, "0", policy.FeeReservedUSD)
	require.Empty(t, policy.PendingRequestID)
	var decisions int64
	require.NoError(t, model.DB.Model(&model.UsageReviewDecision{}).Where("request_id = ?", row.RequestID).Count(&decisions).Error)
	require.EqualValues(t, 1, decisions)
	var logs []model.Log
	require.NoError(t, model.DB.Where("request_id = ? AND type = ?", row.RequestID, model.LogTypeConsume).Find(&logs).Error)
	require.Len(t, logs, 1)
	require.Equal(t, 120, logs[0].Quota)
	require.Contains(t, logs[0].Other, `"token_counts_confirmed":true`)
	require.Contains(t, logs[0].Other, `"settlement_status":"manually_reconciled"`)
	require.NoError(t, model.DB.First(&token, token.Id).Error)
	require.Equal(t, 120, token.UsedQuota, "replayed recovery cannot charge again")
	require.Equal(t, 1000000-120, token.RemainQuota)
}

func TestChatBudgetActualApplicationRejectsUnqualifiedTargets(t *testing.T) {
	f := setupChatBudgetFlow(t)
	f.setResponder(func(w http.ResponseWriter, r *http.Request) { t.Error("an unqualified request reached the provider") })
	for _, test := range []struct {
		name, public, limits, mapping, base, override string
		status                                        int
	}{
		{name: "compatible host", public: model.TokenBudgetOpenAIChatModel, limits: model.TokenBudgetOpenAIChatModel, base: "https://compatible.invalid", status: http.StatusBadRequest},
		{name: "incompatible final target", public: model.TokenBudgetOpenAIChatModel, limits: "gpt-6.1-sol,incompatible-target", mapping: `{"gpt-6.1-sol":"incompatible-target"}`, status: http.StatusBadRequest},
		{name: "alias lacks actual permission", public: "chat-alias", limits: "chat-alias", mapping: `{"chat-alias":"gpt-6.1-sol"}`, status: http.StatusForbidden},
		{name: "alias lacks exact frozen price", public: "chat-alias", limits: "chat-alias,gpt-6.1-sol", mapping: `{"chat-alias":"gpt-6.1-sol"}`, status: http.StatusServiceUnavailable},
		{name: "configured Chat to Responses conversion", public: model.TokenBudgetOpenAIChatModel, limits: model.TokenBudgetOpenAIChatModel, status: http.StatusBadRequest},
		{name: "post override changes target", public: model.TokenBudgetOpenAIChatModel, limits: "gpt-6.1-sol,incompatible-target", override: `{"model":"incompatible-target"}`, status: http.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.name == "configured Chat to Responses conversion" {
				registry := config.GlobalConfig.Get("global")
				require.NoError(t, config.UpdateConfigFromMap(registry, map[string]string{"chat_completions_to_responses_policy": `{"enabled":true,"all_channels":true,"model_patterns":["gpt-6.1-sol"]}`}))
				t.Cleanup(func() {
					require.NoError(t, config.UpdateConfigFromMap(registry, map[string]string{"chat_completions_to_responses_policy": `{"enabled":false}`}))
				})
			}
			base := test.base
			if base == "" {
				base = "https://api.openai.com"
			}
			require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", f.channel.Id).Updates(map[string]any{"base_url": base, "model_mapping": test.mapping, "param_override": test.override}).Error)
			token := f.token(t, test.limits)
			response := f.request(http.MethodPost, "/v1/chat/completions", "sk-"+token.Key, strings.Replace(chatFlowRequest, model.TokenBudgetOpenAIChatModel, test.public, 1))
			require.Equal(t, test.status, response.Code, response.Body.String())
			require.Zero(t, f.hits.Load())
			var count int64
			require.NoError(t, model.DB.Model(&model.TokenBudgetReservation{}).Where("token_id = ?", token.Id).Count(&count).Error)
			require.Zero(t, count, "unqualified requests cannot create a sent reservation")
			require.NoError(t, model.DB.First(&token, token.Id).Error)
			require.Zero(t, token.UsedQuota)
			require.Equal(t, 1000000, token.RemainQuota)
		})
	}
}

func TestChatBudgetActualApplicationFreezesInflightPublicationAndRollback(t *testing.T) {
	f := setupChatBudgetFlow(t)
	for index, rollback := range []bool{false, true} {
		name := "new publication"
		if rollback {
			name = "rollback"
		}
		t.Run(name, func(t *testing.T) {
			token := f.token(t, model.TokenBudgetOpenAIChatModel)
			entered, release := make(chan struct{}), make(chan struct{})
			f.setResponder(func(w http.ResponseWriter, r *http.Request) {
				close(entered)
				select {
				case <-release:
				case <-r.Context().Done():
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, chatBudgetFlowResponse(chatFlowUsage, false, false))
			})
			finished := make(chan *httptest.ResponseRecorder, 1)
			completed := false
			go func() {
				finished <- f.request(http.MethodPost, "/v1/chat/completions", "sk-"+token.Key, chatFlowRequest)
			}()
			t.Cleanup(func() {
				select {
				case <-release:
				default:
					close(release)
				}
				if !completed {
					select {
					case <-finished:
					case <-time.After(10 * time.Second):
						t.Error("relay worker did not stop")
					}
				}
			})
			select {
			case <-entered:
			case response := <-finished:
				completed = true
				t.Fatalf("relay ended before dispatch: status=%d body=%s", response.Code, response.Body.String())
			case <-time.After(5 * time.Second):
				t.Fatal("relay did not reach the TLS fixture")
			}
			var before model.TokenBudgetReservation
			require.NoError(t, model.DB.Where("token_id = ?", token.Id).First(&before).Error)
			require.Equal(t, model.TokenBudgetSent, before.State)
			if !rollback {
				f.publish(t, strings.Replace(chatFlowPriceDocument, "| $2 |", "| $3 |", 1), strings.Repeat("b", 64))
			} else {
				state, err := model.ReadPricePublicationSnapshot(context.Background())
				require.NoError(t, err)
				digest, err := state.Digest()
				require.NoError(t, err)
				body, err := common.Marshal(service.PricePublicationRequest{ID: strings.Repeat("c", 64), ExpectedDigest: digest, Action: "rollback", RollbackOf: strings.Repeat("b", 64), Confirmed: true})
				require.NoError(t, err)
				response := f.request(http.MethodPost, "/api/ratio_sync/openai/publications", *f.root.AccessToken, string(body))
				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			}
			// A second generation on the same key must remain blocked while the
			// first is in flight, even if the price generation has just changed.
			blocked := f.request(http.MethodPost, "/v1/chat/completions", "sk-"+token.Key, chatFlowRequest)
			require.Equal(t, http.StatusConflict, blocked.Code, blocked.Body.String())
			require.EqualValues(t, index+1, f.hits.Load())
			close(release)
			select {
			case response := <-finished:
				completed = true
				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			case <-time.After(5 * time.Second):
				t.Fatal("in-flight relay did not finish")
			}
			var after model.TokenBudgetReservation
			require.NoError(t, model.DB.Where("token_id = ?", token.Id).First(&after).Error)
			require.Equal(t, model.TokenBudgetSettled, after.State)
			require.Equal(t, before.FeePriceEvidence, after.FeePriceEvidence)
			require.Equal(t, before.PricingEvidence, after.PricingEvidence)
			require.NotNil(t, after.ActualFeeUSD)
			want := "0.000239"
			if rollback {
				want = "0.000269"
			}
			require.Equal(t, want, *after.ActualFeeUSD, "settlement uses the request's frozen publication")
		})
	}
}

func TestChatBudgetActualApplicationReservationExhaustion(t *testing.T) {
	f := setupChatBudgetFlow(t)
	f.setResponder(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, chatBudgetFlowResponse(chatFlowUsage, false, false))
	})
	for _, test := range []struct {
		name, fee, code string
		limit           int64
	}{
		{name: "token ceiling", limit: 1050000, fee: "10", code: "token_budget_exceeded"},
		{name: "USD ceiling", limit: 2000000, fee: "5.2502", code: "token_budget_fee_exceeded"},
	} {
		t.Run(test.name, func(t *testing.T) {
			token := f.token(t, model.TokenBudgetOpenAIChatModel)
			body := fmt.Sprintf(`{"id":"%064x","expected_revision":1,"enabled":true,"limit":%d,"confirmed":true,"fee":{"enabled":true,"limit_usd":%q}}`, f.serial.Add(1), test.limit, test.fee)
			configured := f.request(http.MethodPut, fmt.Sprintf("/api/token/%d/budget", token.Id), *f.root.AccessToken, body)
			require.Equal(t, http.StatusOK, configured.Code, configured.Body.String())
			before := f.hits.Load()
			response := f.request(http.MethodPost, "/v1/chat/completions", "sk-"+token.Key, chatFlowRequest)
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			require.Equal(t, before+1, f.hits.Load())
			// Actual usage fits, but another conservative reservation no longer
			// fits. Refusal must happen before the second upstream dispatch.
			response = f.request(http.MethodPost, "/v1/chat/completions", "sk-"+token.Key, chatFlowRequest)
			require.Equal(t, http.StatusForbidden, response.Code, response.Body.String())
			require.Contains(t, response.Body.String(), test.code)
			require.Equal(t, before+1, f.hits.Load())
			policy, err := model.LookupTokenBudget(context.Background(), model.DB, token.Id)
			require.NoError(t, err)
			require.EqualValues(t, 110, policy.Used)
			require.Equal(t, "0.000239", policy.FeeUsedUSD)
			require.Zero(t, policy.Reserved)
			require.Equal(t, "0", policy.FeeReservedUSD)
			require.Empty(t, policy.PendingRequestID)
			require.NoError(t, model.DB.First(&token, token.Id).Error)
			require.Equal(t, 120, token.UsedQuota)
			require.Equal(t, 1000000-120, token.RemainQuota, "failed reservation refunds the original pre-consume")
		})
	}
}

func TestChatBudgetActualApplicationPreservesResponsesV1(t *testing.T) {
	f := setupChatBudgetFlow(t)
	var counts, generations atomic.Int32
	client := chatBudgetFlowTLSClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "api.openai.com", r.Host)
		assert.Equal(t, "Bearer synthetic-only", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/responses/input_tokens":
			counts.Add(1)
			_, _ = io.WriteString(w, `{"object":"response.input_tokens","input_tokens":100}`)
		case "/v1/responses":
			generations.Add(1)
			_, _ = io.WriteString(w, `{"id":"responses-fixture","object":"response","status":"completed","model":"gpt-6.1-sol","service_tier":"default","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":100,"output_tokens":10,"total_tokens":110,"input_tokens_details":{"cached_tokens":40,"cache_write_tokens":30},"output_tokens_details":{"reasoning_tokens":4}}}`)
		default:
			t.Errorf("unexpected Responses fixture path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})
	t.Cleanup(service.SetHttpClientForTest(client))
	token := f.token(t, model.TokenBudgetOpenAIChatModel)
	response := f.request(http.MethodPost, "/v1/responses", "sk-"+token.Key, `{"model":"gpt-6.1-sol","input":"hello","max_output_tokens":20,"service_tier":"default"}`)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.EqualValues(t, 1, counts.Load())
	require.EqualValues(t, 1, generations.Load())
	var row model.TokenBudgetReservation
	require.NoError(t, model.DB.Where("token_id = ?", token.Id).First(&row).Error)
	require.Equal(t, model.TokenBudgetBoundOpenAIResponses, row.BoundSource)
	require.Equal(t, model.TokenBudgetSettled, row.State)
	require.EqualValues(t, 100, row.InputTokens)
	require.EqualValues(t, 120, row.Reserved)
	require.Equal(t, "0.00045", row.FeeReservedUSD)
	require.Contains(t, row.FeePriceEvidence, `"version":1`)
	require.NotNil(t, row.ActualFeeUSD)
	require.Equal(t, "0.000239", *row.ActualFeeUSD)
	policy, err := model.LookupTokenBudget(context.Background(), model.DB, token.Id)
	require.NoError(t, err)
	require.EqualValues(t, 110, policy.Used)
	require.Zero(t, policy.Reserved)
	require.Equal(t, "0.000239", policy.FeeUsedUSD)
	require.Equal(t, "0", policy.FeeReservedUSD)
}
