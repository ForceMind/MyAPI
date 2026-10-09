package controller

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/middleware"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/service"
	"github.com/ForceMind/MyAPI/setting"
	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/ForceMind/MyAPI/setting/model_setting"
	"github.com/ForceMind/MyAPI/setting/operation_setting"
	"github.com/ForceMind/MyAPI/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const playgroundRelayModel = "gpt-6.1-sol"

func playgroundControllerFixture(t *testing.T, mode model.QuotaWriterMode) (*gorm.DB, *model.User, *model.Token, string) {
	t.Helper()
	require.NoError(t, i18n.Init())
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "playground.db")+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	conn, err := db.DB()
	require.NoError(t, err)
	conn.SetMaxOpenConns(4)
	oldDB, oldLog := model.DB, model.LOG_DB
	oldRedis, oldMemory, oldBatch, oldLogEnabled := common.RedisEnabled, common.MemoryCacheEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled
	oldSecret, oldRetries, oldCount := common.SessionSecret, common.RetryTimes, constant.CountToken
	oldStreamTimeout := constant.StreamingTimeout
	oldMain, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
	oldSelfUse := operation_setting.SelfUseModeEnabled
	oldModelRatio, oldCompletionRatio := ratio_setting.ModelRatio2JSONString(), ratio_setting.CompletionRatio2JSONString()
	oldRate := setting.GetModelRequestRateLimitConfig()
	model.DB, model.LOG_DB = db, db
	common.RedisEnabled, common.MemoryCacheEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled = false, false, false, true
	common.SessionSecret, common.RetryTimes, constant.CountToken = "playground-controller-fixture", 0, false
	constant.StreamingTimeout = 30
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	model.InitColumnNamesForTest()
	operation_setting.SelfUseModeEnabled = true
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gpt-6.1-sol":1}`))
	require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(`{"gpt-6.1-sol":1}`))
	rate := oldRate
	rate.Enabled = false
	require.NoError(t, setting.ApplyModelRequestRateLimitConfig(rate))
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLog
		common.RedisEnabled, common.MemoryCacheEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled = oldRedis, oldMemory, oldBatch, oldLogEnabled
		common.SessionSecret, common.RetryTimes, constant.CountToken = oldSecret, oldRetries, oldCount
		constant.StreamingTimeout = oldStreamTimeout
		common.SetDatabaseTypes(oldMain, oldLogType)
		operation_setting.SelfUseModeEnabled = oldSelfUse
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldModelRatio))
		require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(oldCompletionRatio))
		require.NoError(t, setting.ApplyModelRequestRateLimitConfig(oldRate))
		_ = conn.Close()
	})
	require.NoError(t, db.AutoMigrate(
		&model.User{}, &model.UserSession{}, &model.Token{}, &model.TokenBudget{}, &model.TokenBudgetReservation{}, &model.TokenBudgetPolicyChange{},
		&model.Option{}, &model.AssignedAccessPolicy{}, &model.Channel{}, &model.Ability{}, &model.Model{}, &model.Vendor{}, &model.RelayAccountHold{},
		&model.SubscriptionPlan{}, &model.UserSubscription{}, &model.UserQuotaMutationReceipt{}, &model.AccountQuotaMutationReceipt{}, &model.AccountQuotaReservationHead{},
		&model.AccountQuotaTerminalRecoveryObligation{}, &model.AccountQuotaRefundFact{}, &model.AccountQuotaSettlementIntent{}, &model.AccountQuotaSettlementFact{},
		&model.LegacyUsageReservation{}, &model.SystemTask{}, &model.SystemTaskLock{}, &model.UsageReviewDecision{}, &model.Log{}, &model.BillingLogProjectionIdentity{},
		&model.QuotaWriterEpoch{}, &model.QuotaProjectionObligation{}, &model.QuotaBalanceBatchDrain{}, &model.QuotaBalanceBatchSubject{}, &model.QuotaWorkCursor{},
	))
	require.True(t, model.RefreshAccountQuotaSettlementIntentSchemaCapability(db))
	require.NoError(t, model.EnsureQuotaWriterEpochStateWithDB(db))
	require.NoError(t, db.Model(&model.QuotaWriterEpoch{}).Where("id = ?", 1).Updates(map[string]any{"mode": string(mode), "epoch": int64(31), "lock_version": gorm.Expr("lock_version + ?", 1)}).Error)
	user := &model.User{Username: "playground-owner", AffCode: "playground-owner", Password: "fixture", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", Quota: 1000000, AuthVersion: 1, Setting: `{"billing_preference":"wallet_only"}`}
	require.NoError(t, db.Create(user).Error)
	session := &model.UserSession{SID: "playground-controller-session", UserID: user.Id, UserAuthVersion: 1, Version: 1, Status: model.UserSessionStatusActive, RefreshHash: "fixture-refresh", LoginMethod: "password", LastActiveAt: time.Now().Unix(), ExpiresAt: time.Now().Add(time.Hour).Unix()}
	require.NoError(t, model.CreateUserSession(session))
	access, _, err := service.IssueAccessToken(service.AuthIdentity{UserID: user.Id, SessionID: session.SID, UserAuthVersion: 1, SessionVersion: 1})
	require.NoError(t, err)
	key := &model.Token{UserId: user.Id, Key: "playgroundcontrollersecret", Name: "Selected real key", Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 1000000, Group: "default", AccessProfileID: "standard"}
	require.NoError(t, db.Create(key).Error)
	return db, user, key, access
}

func playgroundControllerRouter(beforeRelay ...gin.HandlerFunc) *gin.Engine {
	router := gin.New()
	router.Use(middleware.BodyStorageCleanup(), func(c *gin.Context) { c.Set(common.RequestIdKey, c.GetHeader("X-Test-Request-ID")); c.Next() })
	group := router.Group("/pg", middleware.UserAuth(), middleware.PlaygroundSessionAuth())
	group.GET("/keys", PlaygroundKeys)
	group.GET("/models", middleware.PlaygroundKeyAuth(), PlaygroundListModels)
	relayChain := []gin.HandlerFunc{middleware.PlaygroundKeyAuth(), middleware.PlaygroundMediaGuard(), middleware.FullContentLogger(), middleware.ModelRequestRateLimit(), middleware.Distribute()}
	relayChain = append(relayChain, beforeRelay...)
	relayChain = append(relayChain, Playground)
	group.POST("/chat/completions", relayChain...)
	router.GET("/v1/models", middleware.TokenAuth(), func(c *gin.Context) { ListModels(c, constant.ChannelTypeOpenAI) })
	return router
}

func playgroundControllerRequest(router http.Handler, method, path, access string, keyID int, body, requestID string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+access)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Test-Request-ID", requestID)
	if keyID > 0 {
		request.Header.Set("X-MyAPI-Key-ID", strconv.Itoa(keyID))
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func TestPlaygroundKeysSafeOwnedPaginatedMetadata(t *testing.T) {
	db, user, key, access := playgroundControllerFixture(t, model.QuotaWriterModeLegacy)
	other := &model.Token{UserId: user.Id, Key: "otherownedsecret", Name: "Newest key", Status: common.TokenStatusDisabled, ExpiredTime: -1, UnlimitedQuota: true}
	require.NoError(t, db.Create(other).Error)
	require.NoError(t, db.Create(&model.Token{UserId: user.Id + 1, Key: "foreignsecret", Name: "Foreign key"}).Error)
	deleted := &model.Token{UserId: user.Id, Key: "deletedsecret", Name: "Deleted key"}
	require.NoError(t, db.Create(deleted).Error)
	require.NoError(t, db.Delete(deleted).Error)
	router := playgroundControllerRouter()
	first := playgroundControllerRequest(router, http.MethodGet, "/pg/keys?p=1&page_size=1", access, 0, "", "")
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	var payload struct {
		Success bool `json:"success"`
		Data    struct {
			Page     int              `json:"page"`
			PageSize int              `json:"page_size"`
			Total    int              `json:"total"`
			Items    []map[string]any `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(first.Body.Bytes(), &payload))
	assert.True(t, payload.Success)
	assert.Equal(t, 2, payload.Data.Total)
	require.Len(t, payload.Data.Items, 1)
	assert.EqualValues(t, other.Id, payload.Data.Items[0]["id"])
	assert.ElementsMatch(t, []string{"id", "name", "status", "group", "access_profile_id", "remain_quota", "used_quota", "unlimited_quota", "expired_time", "model_limits_enabled", "strict_token_budget"}, func() []string {
		keys := []string{}
		for k := range payload.Data.Items[0] {
			keys = append(keys, k)
		}
		return keys
	}())
	for _, secret := range []string{key.Key, other.Key, "foreignsecret", "deletedsecret", "allow_ips", "user_id", "model_limits\""} {
		assert.NotContains(t, first.Body.String(), secret)
	}
	second := playgroundControllerRequest(router, http.MethodGet, "/pg/keys?p=2&page_size=1", access, 0, "", "")
	require.NoError(t, common.Unmarshal(second.Body.Bytes(), &payload))
	require.Len(t, payload.Data.Items, 1)
	assert.EqualValues(t, key.Id, payload.Data.Items[0]["id"])
	bounded := playgroundControllerRequest(router, http.MethodGet, "/pg/keys?p=-1&page_size=-1", access, 0, "", "")
	require.NoError(t, common.Unmarshal(bounded.Body.Bytes(), &payload))
	assert.Equal(t, 1, payload.Data.Page)
	assert.Equal(t, 10, payload.Data.PageSize)
	bounded = playgroundControllerRequest(router, http.MethodGet, "/pg/keys?page_size=100000", access, 0, "", "")
	require.NoError(t, common.Unmarshal(bounded.Body.Bytes(), &payload))
	assert.Equal(t, 100, payload.Data.PageSize)
}

func TestPlaygroundKeysReportsCurrentOwnedStrictPolicy(t *testing.T) {
	for _, tc := range []struct {
		name   string
		budget *model.TokenBudget
		strict bool
	}{
		{name: "no budget"},
		{name: "disabled budget", budget: &model.TokenBudget{Revision: 1}},
		{name: "token budget", budget: &model.TokenBudget{Enabled: true, Limit: model.TokenBudgetOpenAIChatContext, Revision: 1}, strict: true},
		{name: "fee-only budget", budget: &model.TokenBudget{FeeEnabled: true, FeeLimitUSD: "10", Revision: 1}, strict: true},
		{name: "account threshold only", budget: &model.TokenBudget{AccountThresholdEnabled: true, AccountMinRemainingBPS: 2000, AccountMaxAgeSeconds: 300, Revision: 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, user, key, access := playgroundControllerFixture(t, model.QuotaWriterModeLegacy)
			if tc.budget != nil {
				tc.budget.TokenID, tc.budget.UserID = key.Id, user.Id
				require.NoError(t, db.Create(tc.budget).Error)
			}
			response := playgroundControllerRequest(playgroundControllerRouter(), http.MethodGet, "/pg/keys", access, 0, "", "")
			require.Equal(t, http.StatusOK, response.Code)
			var payload struct {
				Success bool `json:"success"`
				Data    struct {
					Items []map[string]any `json:"items"`
				} `json:"data"`
			}
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &payload))
			require.True(t, payload.Success, response.Body.String())
			require.Len(t, payload.Data.Items, 1)
			assert.Equal(t, tc.strict, payload.Data.Items[0]["strict_token_budget"])
			assert.NotContains(t, response.Body.String(), key.Key)
		})
	}
}

func TestPlaygroundKeysDoesNotGuessPolicyOnReadFailureOrOwnershipConflict(t *testing.T) {
	for _, broken := range []string{"unavailable", "identity conflict"} {
		t.Run(broken, func(t *testing.T) {
			db, user, key, access := playgroundControllerFixture(t, model.QuotaWriterModeLegacy)
			if broken == "unavailable" {
				require.NoError(t, db.Migrator().DropTable(&model.TokenBudget{}))
			} else {
				require.NoError(t, db.Create(&model.TokenBudget{TokenID: key.Id, UserID: user.Id + 1, Enabled: true, Limit: 100, Revision: 1}).Error)
			}
			response := playgroundControllerRequest(playgroundControllerRouter(), http.MethodGet, "/pg/keys", access, 0, "", "")
			var payload map[string]any
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &payload))
			assert.Equal(t, false, payload["success"])
			assert.NotContains(t, payload, "data")
			assert.NotContains(t, response.Body.String(), key.Key)
		})
	}
}

func TestPlaygroundModelsMatchSelectedKeyAPIContractAndCapabilities(t *testing.T) {
	db, _, key, access := playgroundControllerFixture(t, model.QuotaWriterModeLegacy)
	require.NoError(t, db.Model(key).Updates(map[string]any{"model_limits_enabled": true, "model_limits": playgroundRelayModel}).Error)
	require.NoError(t, db.Create(&model.Channel{Id: 11, Name: "Codex model listing", Type: constant.ChannelTypeCodex, Key: "privateupstream", Status: common.ChannelStatusEnabled, Group: "default", Models: playgroundRelayModel + ",hidden-model"}).Error)
	require.NoError(t, db.Create(&[]model.Ability{{Group: "default", Model: playgroundRelayModel, ChannelId: 11, Enabled: true}, {Group: "default", Model: "hidden-model", ChannelId: 11, Enabled: true}}).Error)
	router := playgroundControllerRouter()
	pg := playgroundControllerRequest(router, http.MethodGet, "/pg/models?group=other", access, key.Id, "", "")
	api := playgroundControllerRequest(router, http.MethodGet, "/v1/models", key.Key, 0, "", "")
	require.Equal(t, http.StatusOK, pg.Code, pg.Body.String())
	require.Equal(t, http.StatusOK, api.Code, api.Body.String())
	var pgPayload, apiPayload map[string]any
	require.NoError(t, common.Unmarshal(pg.Body.Bytes(), &pgPayload))
	require.NoError(t, common.Unmarshal(api.Body.Bytes(), &apiPayload))
	assert.Equal(t, apiPayload["data"], pgPayload["data"])
	assert.Equal(t, "list", pgPayload["object"])
	models := pgPayload["data"].([]any)
	require.Len(t, models, 1)
	assert.Equal(t, playgroundRelayModel, models[0].(map[string]any)["id"])
	caps := pgPayload["capabilities"].(map[string]any)
	require.Contains(t, caps, playgroundRelayModel)
	assert.Equal(t, "Codex", caps[playgroundRelayModel].(map[string]any)["provider"])
	assert.NotContains(t, pg.Body.String(), key.Key)
	assert.NotContains(t, pg.Body.String(), "privateupstream")
	assert.NotContains(t, apiPayload, "capabilities")
}

func TestPlaygroundRelayMediaPreservesPayloadAndRealKeyAccounting(t *testing.T) {
	for _, mode := range []model.QuotaWriterMode{model.QuotaWriterModeLegacy, model.QuotaWriterModeAuthoritative} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%v", mode, stream), func(t *testing.T) {
				db, user, key, access := playgroundControllerFixture(t, mode)
				logDir := t.TempDir()
				t.Setenv("FULL_CONTENT_LOG_ENABLED", "true")
				t.Setenv("FULL_CONTENT_LOG_DIR", logDir)
				imageData := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("\x89PNG\r\n\x1a\nplayground-private-image"))
				fileData := "data:application/pdf;base64," + base64.StdEncoding.EncodeToString([]byte("%PDF-1.7\nplayground-private-file\n%%EOF"))
				parts := []any{map[string]any{"type": "text", "text": "Describe the attachments"}, map[string]any{"type": "image_url", "image_url": map[string]any{"url": imageData}}, map[string]any{"type": "file", "file": map[string]any{"filename": "private.pdf", "file_data": fileData}}}
				body, err := common.Marshal(map[string]any{"model": playgroundRelayModel, "messages": []any{map[string]any{"role": "user", "content": parts}}, "max_completion_tokens": 20, "stream": stream})
				require.NoError(t, err)
				var upstreamCalls atomic.Int32
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					upstreamCalls.Add(1)
					assert.Equal(t, "/v1/chat/completions", r.URL.Path)
					assert.Equal(t, "Bearer privateupstream", r.Header.Get("Authorization"))
					raw, err := io.ReadAll(r.Body)
					assert.NoError(t, err)
					var received map[string]any
					assert.NoError(t, common.Unmarshal(raw, &received))
					messages, ok := received["messages"].([]any)
					if assert.True(t, ok) && assert.Len(t, messages, 1) {
						assert.Equal(t, parts, messages[0].(map[string]any)["content"])
					}
					if stream {
						w.Header().Set("Content-Type", "text/event-stream")
						fmt.Fprint(w, "data: {\"id\":\"fixture\",\"object\":\"chat.completion.chunk\",\"model\":\"gpt-6.1-sol\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"},\"finish_reason\":null}]}\n\ndata: {\"id\":\"fixture\",\"object\":\"chat.completion.chunk\",\"model\":\"gpt-6.1-sol\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":5,\"total_tokens\":15}}\n\ndata: [DONE]\n\n")
					} else {
						w.Header().Set("Content-Type", "application/json")
						fmt.Fprint(w, `{"id":"fixture","object":"chat.completion","model":"gpt-6.1-sol","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`)
					}
				}))
				defer upstream.Close()
				target, err := url.Parse(upstream.URL)
				require.NoError(t, err)
				transport := http.DefaultTransport.(*http.Transport).Clone()
				defer transport.CloseIdleConnections()
				t.Cleanup(service.SetHttpClientForTest(&http.Client{Transport: testRoundTripper(func(request *http.Request) (*http.Response, error) {
					clone := request.Clone(request.Context())
					urlCopy := *request.URL
					urlCopy.Scheme = target.Scheme
					urlCopy.Host = target.Host
					clone.URL = &urlCopy
					return transport.RoundTrip(clone)
				}), Timeout: 5 * time.Second}))
				channel := &model.Channel{Id: 21, Name: "Playground upstream", Type: constant.ChannelTypeOpenAI, Key: "privateupstream", Status: common.ChannelStatusEnabled, Group: "default", Models: playgroundRelayModel, BaseURL: common.GetPointer("https://api.openai.com")}
				require.NoError(t, db.Create(channel).Error)
				require.NoError(t, db.Create(&model.Ability{Group: "default", Model: playgroundRelayModel, ChannelId: channel.Id, Enabled: true, Weight: 1}).Error)
				other := &model.Token{UserId: user.Id, Key: "unselectedsecret", Name: "Unselected", Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 1000}
				require.NoError(t, db.Create(other).Error)
				router := playgroundControllerRouter()
				response := playgroundControllerRequest(router, http.MethodPost, "/pg/chat/completions", access, key.Id, string(body), "pg-media-success")
				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
				assert.Contains(t, response.Body.String(), "ok")
				assert.EqualValues(t, 1, upstreamCalls.Load())
				require.NoError(t, db.First(key, key.Id).Error)
				require.NoError(t, db.First(user, user.Id).Error)
				require.NoError(t, db.First(other, other.Id).Error)
				assert.Equal(t, 15, key.UsedQuota)
				assert.Equal(t, 1000000-15, key.RemainQuota)
				assert.Equal(t, 1000000-15, user.Quota)
				assert.Equal(t, 1000, other.RemainQuota)
				assert.Zero(t, other.UsedQuota)
				var usage model.Log
				require.NoError(t, db.Where("request_id = ? AND type = ?", "pg-media-success", model.LogTypeConsume).First(&usage).Error)
				assert.Equal(t, key.Id, usage.TokenId)
				assert.Equal(t, user.Id, usage.UserId)
				assert.Equal(t, 10, usage.PromptTokens)
				assert.Equal(t, 5, usage.CompletionTokens)
				assert.Equal(t, 15, usage.Quota)
				assert.Equal(t, stream, usage.IsStream)
				files, err := filepath.Glob(filepath.Join(logDir, "full-content-*.jsonl"))
				require.NoError(t, err)
				require.NotEmpty(t, files)
				for _, file := range files {
					raw, err := os.ReadFile(file)
					require.NoError(t, err)
					assert.NotContains(t, string(raw), imageData)
					assert.NotContains(t, string(raw), fileData)
					assert.NotContains(t, string(raw), key.Key)
					assert.Contains(t, string(raw), `"path":"/pg/chat/completions"`)
					assert.Contains(t, string(raw), `"token_id":`+strconv.Itoa(key.Id))
				}
				// The same media payload must never lose its file when routing changes
				// to a converter that has not been qualified for inline PDF transport.
				require.NoError(t, db.Model(channel).Update("type", constant.ChannelTypeAnthropic).Error)
				unsupported := playgroundControllerRequest(router, http.MethodPost, "/pg/chat/completions", access, key.Id, string(body), "pg-media-unsupported")
				assert.Equal(t, http.StatusBadRequest, unsupported.Code, unsupported.Body.String())
				assert.Contains(t, unsupported.Body.String(), "playground_image_provider_unsupported")
				pdfBody, err := common.Marshal(map[string]any{"model": playgroundRelayModel, "messages": []any{map[string]any{"role": "user", "content": []any{parts[2]}}}, "max_completion_tokens": 20, "stream": stream})
				require.NoError(t, err)
				unsupportedPDF := playgroundControllerRequest(router, http.MethodPost, "/pg/chat/completions", access, key.Id, string(pdfBody), "pg-pdf-unsupported")
				assert.Equal(t, http.StatusBadRequest, unsupportedPDF.Code, unsupportedPDF.Body.String())
				assert.Contains(t, unsupportedPDF.Body.String(), "playground_file_provider_unsupported")
				assert.EqualValues(t, 1, upstreamCalls.Load())
				require.NoError(t, db.First(key, key.Id).Error)
				assert.Equal(t, 15, key.UsedQuota)
				assert.Equal(t, 1000000-15, key.RemainQuota)
				require.NoError(t, db.Model(channel).Update("type", constant.ChannelTypeOpenAI).Error)
				// The same selected real key now opts into strict text-only budgets.
				// Both media types must fail before a second upstream dispatch.
				require.NoError(t, db.Create(&model.TokenBudget{TokenID: key.Id, UserID: user.Id, Enabled: true, Limit: model.TokenBudgetOpenAIChatContext, Revision: 1}).Error)
				for i, part := range parts[1:] {
					strictBody, err := common.Marshal(map[string]any{"model": playgroundRelayModel, "messages": []any{map[string]any{"role": "user", "content": []any{part}}}, "max_completion_tokens": 20, "service_tier": "default"})
					require.NoError(t, err)
					denied := playgroundControllerRequest(router, http.MethodPost, "/pg/chat/completions", access, key.Id, string(strictBody), fmt.Sprintf("pg-strict-media-%d", i))
					assert.Equal(t, http.StatusBadRequest, denied.Code, denied.Body.String())
					assert.Contains(t, denied.Body.String(), "token_budget_unsupported_request")
				}
				assert.EqualValues(t, 1, upstreamCalls.Load())
				require.NoError(t, db.First(key, key.Id).Error)
				assert.Equal(t, 15, key.UsedQuota)
				assert.Equal(t, 1000000-15, key.RemainQuota)
			})
		}
	}
}

func TestPlaygroundRelayRechecksUserAndKeyAssignmentsAfterSelection(t *testing.T) {
	for _, subject := range []string{"user", "token"} {
		t.Run(subject, func(t *testing.T) {
			db, user, key, access := playgroundControllerFixture(t, model.QuotaWriterModeLegacy)
			var upstreamCalls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				upstreamCalls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"id":"assigned","object":"chat.completion","model":"gpt-6.1-sol","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`)
			}))
			defer upstream.Close()
			t.Cleanup(service.SetHttpClientForTest(upstream.Client()))
			channel := &model.Channel{Id: 31, Name: "Assigned playground channel", Type: constant.ChannelTypeOpenAI, Key: "assignedupstream", Status: common.ChannelStatusEnabled, Group: "default", Models: playgroundRelayModel, BaseURL: &upstream.URL}
			require.NoError(t, db.Create(channel).Error)
			require.NoError(t, db.Create(&model.Ability{Group: "default", Model: playgroundRelayModel, ChannelId: channel.Id, Enabled: true, Weight: 1}).Error)
			subjectID := user.Id
			if subject == "token" {
				subjectID = key.Id
			}
			_, err := model.ConfigureAssignedAccessPolicy(context.Background(), db, subject, subjectID, 0, `{"enabled":true,"public_models":[],"upstream_models":null,"channel_ids":null}`)
			require.NoError(t, err)
			revokeAfterSelection := false
			router := playgroundControllerRouter(func(c *gin.Context) {
				if revokeAfterSelection {
					assert.Equal(t, key.Id, c.GetInt("token_id"))
					assert.Equal(t, channel.Id, c.GetInt("channel_id"))
					_, err := model.ConfigureAssignedAccessPolicy(context.Background(), db, subject, subjectID, 2, `{"enabled":false,"public_models":null,"upstream_models":null,"channel_ids":null}`)
					require.NoError(t, err)
					revokeAfterSelection = false
				}
				c.Next()
			})
			body := `{"model":"gpt-6.1-sol","messages":[{"role":"user","content":"hello"}],"max_completion_tokens":20}`
			denied := playgroundControllerRequest(router, http.MethodPost, "/pg/chat/completions", access, key.Id, body, "pg-assignment-denied")
			assert.Equal(t, http.StatusForbidden, denied.Code, denied.Body.String())
			assert.Zero(t, upstreamCalls.Load())
			allowedPolicy := `{"enabled":true,"public_models":["gpt-6.1-sol"],"upstream_models":["gpt-6.1-sol"],"channel_ids":[31]}`
			_, err = model.ConfigureAssignedAccessPolicy(context.Background(), db, subject, subjectID, 1, allowedPolicy)
			require.NoError(t, err)
			allowed := playgroundControllerRequest(router, http.MethodPost, "/pg/chat/completions", access, key.Id, body, "pg-assignment-allowed")
			require.Equal(t, http.StatusOK, allowed.Code, allowed.Body.String())
			assert.EqualValues(t, 1, upstreamCalls.Load())
			revokeAfterSelection = true
			revoked := playgroundControllerRequest(router, http.MethodPost, "/pg/chat/completions", access, key.Id, body, "pg-assignment-revoked")
			assert.Equal(t, http.StatusForbidden, revoked.Code, revoked.Body.String())
			assert.EqualValues(t, 1, upstreamCalls.Load(), "previously selected channel cannot retain revoked access")
			require.NoError(t, db.First(key, key.Id).Error)
			assert.Equal(t, 15, key.UsedQuota)
			assert.Equal(t, 1000000-15, key.RemainQuota)
			_, err = model.ConfigureAssignedAccessPolicy(context.Background(), db, subject, subjectID, 3, allowedPolicy)
			require.NoError(t, err)
			require.NoError(t, db.Model(key).Update("status", common.TokenStatusDisabled).Error)
			disabled := playgroundControllerRequest(router, http.MethodPost, "/pg/chat/completions", access, key.Id, body, "pg-key-disabled")
			assert.Equal(t, http.StatusUnauthorized, disabled.Code, disabled.Body.String())
			assert.Contains(t, disabled.Body.String(), "playground_key_invalid")
			assert.EqualValues(t, 1, upstreamCalls.Load())
		})
	}
}

func TestPlaygroundCodexImageChatConversionReachesResponsesUpstream(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%v", stream), func(t *testing.T) {
			db, user, key, access := playgroundControllerFixture(t, model.QuotaWriterModeLegacy)
			t.Setenv(common.ChannelQuotaIdentityKeysEnv, "")
			require.NoError(t, db.AutoMigrate(&model.ChannelQuotaSnapshot{}))
			prior := model_setting.GetGlobalSettings().ChatCompletionsToResponsesPolicy
			priorPolicy, err := common.Marshal(map[string]any{"enabled": prior.Enabled, "all_channels": prior.AllChannels, "channel_ids": prior.ChannelIDs, "channel_types": prior.ChannelTypes, "model_patterns": prior.ModelPatterns})
			require.NoError(t, err)
			require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"global.chat_completions_to_responses_policy": `{"enabled":true,"all_channels":false,"channel_types":[57],"model_patterns":["^gpt-6\\.1-sol$"]}`}))
			t.Cleanup(func() {
				require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"global.chat_completions_to_responses_policy": string(priorPolicy)}))
			})
			imageData := "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jU1sAAAAASUVORK5CYII="
			var upstreamCalls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				upstreamCalls.Add(1)
				assert.Equal(t, "/backend-api/codex/responses", r.URL.Path)
				assert.Equal(t, "Bearer synthetic-codex-access", r.Header.Get("Authorization"))
				assert.Equal(t, "synthetic-codex-account", r.Header.Get("Chatgpt-Account-Id"))
				raw, readErr := io.ReadAll(r.Body)
				assert.NoError(t, readErr)
				var request struct {
					Stream bool `json:"stream"`
					Store  bool `json:"store"`
					Input  []struct {
						Role    string `json:"role"`
						Content []struct {
							Type     string `json:"type"`
							Text     string `json:"text"`
							ImageURL string `json:"image_url"`
						} `json:"content"`
					} `json:"input"`
				}
				if assert.NoError(t, common.Unmarshal(raw, &request)) {
					assert.True(t, request.Stream, "Codex upstream is streaming even for buffered Chat clients")
					assert.False(t, request.Store)
					if assert.Len(t, request.Input, 1) && assert.Len(t, request.Input[0].Content, 2) {
						assert.Equal(t, "user", request.Input[0].Role)
						assert.Equal(t, "input_text", request.Input[0].Content[0].Type)
						assert.Equal(t, "Describe the image", request.Input[0].Content[0].Text)
						assert.Equal(t, "input_image", request.Input[0].Content[1].Type)
						assert.Equal(t, imageData, request.Input[0].Content[1].ImageURL)
					}
				}
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, "data: {\"type\":\"response.created\",\"response\":{\"id\":\"codex-image\",\"model\":\"gpt-6.1-sol\",\"created_at\":1710000000}}\n\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"Image received\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"codex-image\",\"model\":\"gpt-6.1-sol\",\"status\":\"completed\",\"usage\":{\"input_tokens\":10,\"output_tokens\":5,\"total_tokens\":15}}}\n\ndata: [DONE]\n\n")
			}))
			defer upstream.Close()
			t.Cleanup(service.SetHttpClientForTest(upstream.Client()))
			channel := &model.Channel{Id: 41, Name: "Synthetic Codex", Type: constant.ChannelTypeCodex, Key: `{"access_token":"synthetic-codex-access","account_id":"synthetic-codex-account"}`, Status: common.ChannelStatusEnabled, Group: "default", Models: playgroundRelayModel, BaseURL: &upstream.URL}
			require.NoError(t, db.Create(channel).Error)
			require.NoError(t, db.Create(&model.Ability{Group: "default", Model: playgroundRelayModel, ChannelId: channel.Id, Enabled: true, Weight: 1}).Error)
			body, err := common.Marshal(map[string]any{"model": playgroundRelayModel, "messages": []any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "text", "text": "Describe the image"}, map[string]any{"type": "image_url", "image_url": map[string]any{"url": imageData}}}}}, "max_completion_tokens": 20, "stream": stream})
			require.NoError(t, err)
			response := playgroundControllerRequest(playgroundControllerRouter(), http.MethodPost, "/pg/chat/completions", access, key.Id, string(body), "pg-codex-image")
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			assert.Contains(t, response.Body.String(), "Image received")
			assert.EqualValues(t, 1, upstreamCalls.Load())
			if stream {
				assert.Contains(t, response.Body.String(), "data: [DONE]")
			} else {
				assert.Contains(t, response.Body.String(), `"object":"chat.completion"`)
				assert.NotContains(t, response.Body.String(), "data:")
			}
			require.NoError(t, db.First(key, key.Id).Error)
			require.NoError(t, db.First(user, user.Id).Error)
			assert.Equal(t, 15, key.UsedQuota)
			assert.Equal(t, 1000000-15, key.RemainQuota)
			assert.Equal(t, 1000000-15, user.Quota)
			var usage model.Log
			require.NoError(t, db.Where("request_id = ? AND type = ?", "pg-codex-image", model.LogTypeConsume).First(&usage).Error)
			assert.Equal(t, key.Id, usage.TokenId)
			assert.Equal(t, 10, usage.PromptTokens)
			assert.Equal(t, 5, usage.CompletionTokens)
		})
	}
}

func TestPlaygroundImageUnsupportedAdapterNeverDispatches(t *testing.T) {
	db, user, key, access := playgroundControllerFixture(t, model.QuotaWriterModeLegacy)
	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer upstream.Close()
	t.Cleanup(service.SetHttpClientForTest(upstream.Client()))
	channel := &model.Channel{Id: 51, Name: "Unqualified image adapter", Type: constant.ChannelTypeAnthropic, Key: "synthetic-anthropic", Status: common.ChannelStatusEnabled, Group: "default", Models: playgroundRelayModel, BaseURL: &upstream.URL}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Create(&model.Ability{Group: "default", Model: playgroundRelayModel, ChannelId: channel.Id, Enabled: true, Weight: 1}).Error)
	body := `{"model":"gpt-6.1-sol","messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jU1sAAAAASUVORK5CYII="}}]}],"max_completion_tokens":20}`
	response := playgroundControllerRequest(playgroundControllerRouter(), http.MethodPost, "/pg/chat/completions", access, key.Id, body, "pg-image-unsupported")
	assert.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), "playground_image_provider_unsupported")
	assert.Zero(t, upstreamCalls.Load())
	require.NoError(t, db.First(key, key.Id).Error)
	require.NoError(t, db.First(user, user.Id).Error)
	assert.Equal(t, 1000000, key.RemainQuota)
	assert.Zero(t, key.UsedQuota)
	assert.Equal(t, 1000000, user.Quota)
}

func TestPlaygroundPartialImageStreamHoldsUnknownUsageWithoutRetry(t *testing.T) {
	db, user, key, access := playgroundControllerFixture(t, model.QuotaWriterModeLegacy)
	common.RetryTimes = 2
	var firstCalls, backupCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer synthetic-backup" {
			backupCalls.Add(1)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		firstCalls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"id\":\"partial-image\",\"object\":\"chat.completion.chunk\",\"model\":\"gpt-6.1-sol\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"partial result\"},\"finish_reason\":null}]}\n\n")
		w.(http.Flusher).Flush()
		// End the upstream transport without terminal usage or [DONE].
	}))
	defer upstream.Close()
	t.Cleanup(service.SetHttpClientForTest(upstream.Client()))
	for _, item := range []struct {
		id       int
		priority int64
		key      string
	}{{61, 100, "synthetic-partial"}, {62, 50, "synthetic-backup"}} {
		priority := item.priority
		channel := &model.Channel{Id: item.id, Name: "Stream interruption fixture", Type: constant.ChannelTypeOpenAI, Key: item.key, Status: common.ChannelStatusEnabled, Group: "default", Models: playgroundRelayModel, BaseURL: &upstream.URL, Priority: &priority}
		require.NoError(t, db.Create(channel).Error)
		require.NoError(t, db.Create(&model.Ability{Group: "default", Model: playgroundRelayModel, ChannelId: channel.Id, Enabled: true, Weight: 1, Priority: &priority}).Error)
	}
	body := `{"model":"gpt-6.1-sol","messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jU1sAAAAASUVORK5CYII="}}]}],"max_completion_tokens":20,"stream":true}`
	response := playgroundControllerRequest(playgroundControllerRouter(), http.MethodPost, "/pg/chat/completions", access, key.Id, body, "pg-image-partial")
	assert.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), "partial result")
	assert.EqualValues(t, 1, firstCalls.Load())
	assert.Zero(t, backupCalls.Load(), "an accepted partial stream must not be replayed on a healthy backup")
	view, err := model.GetUsageReview(context.Background(), db, user.Id, "pg-image-partial")
	require.NoError(t, err)
	assert.Equal(t, model.LegacyUsageUnknown, view.State)
	assert.Equal(t, user.Id, view.UserID)
	assert.Equal(t, key.Id, view.TokenID)
	assert.Nil(t, view.ActualQuota)
	assert.Positive(t, view.ReservedQuota)
	require.NoError(t, db.First(key, key.Id).Error)
	require.NoError(t, db.First(user, user.Id).Error)
	assert.EqualValues(t, 1000000-view.ReservedQuota, key.RemainQuota)
	assert.EqualValues(t, 1000000-view.ReservedQuota, user.Quota)
	var consumeCount int64
	require.NoError(t, db.Model(&model.Log{}).Where("request_id = ? AND type = ?", "pg-image-partial", model.LogTypeConsume).Count(&consumeCount).Error)
	assert.Zero(t, consumeCount, "unknown usage must not become a fabricated completed consume record")
}
