package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/model"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/ForceMind/MyAPI/service"
	"github.com/ForceMind/MyAPI/setting"
	"github.com/ForceMind/MyAPI/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func playgroundAuthFixture(t *testing.T) (*gorm.DB, *model.User, *model.Token, string) {
	t.Helper()
	setupDashboardAuthMiddlewareTest(t)
	require.NoError(t, i18n.Init())
	gin.SetMode(gin.TestMode)
	model.InitColumnNamesForTest()
	db := model.DB
	require.NoError(t, db.AutoMigrate(&model.Token{}, &model.TokenBudget{}, &model.Option{}, &model.QuotaWriterEpoch{}))
	require.NoError(t, model.EnsureQuotaWriterEpochStateWithDB(db))
	user := createMiddlewarePATUser(t, "playground-session-owner", "playground-dashboard-pat")
	require.NoError(t, db.Model(user).Update("quota", 1000).Error)
	user.Quota = 1000
	session := &model.UserSession{SID: "playground-live-session", UserID: user.Id, Version: 1, UserAuthVersion: 1,
		Status: model.UserSessionStatusActive, RefreshHash: "fixture-refresh", LoginMethod: "password",
		LastActiveAt: time.Now().Unix(), ExpiresAt: time.Now().Add(time.Hour).Unix()}
	require.NoError(t, model.CreateUserSession(session))
	access, _, err := service.IssueAccessToken(service.AuthIdentity{UserID: user.Id, SessionID: session.SID, UserAuthVersion: 1, SessionVersion: 1})
	require.NoError(t, err)
	token := &model.Token{UserId: user.Id, Key: "playgroundsecret", Name: "Explicitly selected", Status: common.TokenStatusEnabled,
		ExpiredTime: -1, RemainQuota: 500, Group: "default", AccessProfileID: "standard"}
	require.NoError(t, db.Create(token).Error)
	return db, user, token, access
}

func playgroundAuthRouter(t *testing.T, handler gin.HandlerFunc, additional ...gin.HandlerFunc) *gin.Engine {
	t.Helper()
	router := gin.New()
	require.NoError(t, router.SetTrustedProxies(nil))
	router.Use(BodyStorageCleanup())
	chain := []gin.HandlerFunc{UserAuth(), PlaygroundSessionAuth(), PlaygroundKeyAuth()}
	chain = append(chain, additional...)
	chain = append(chain, handler)
	router.POST("/pg/chat/completions", chain...)
	router.GET("/pg/models", chain...)
	return router
}

func playgroundAuthRequest(router http.Handler, method, path, access, keyID, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+access)
	request.Header.Set("Content-Type", "application/json")
	if keyID != "" {
		request.Header.Set("X-MyAPI-Key-ID", keyID)
	}
	request.RemoteAddr = "192.0.2.5:4321"
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func TestPlaygroundSelectsOwnedKeyWithoutExposingCredentialOrChangingOrigin(t *testing.T) {
	db, user, token, access := playgroundAuthFixture(t)
	require.NoError(t, db.Create(&model.Token{UserId: user.Id, Key: "newerkey", Name: "Must not auto-select", Status: common.TokenStatusEnabled, UnlimitedQuota: true, ExpiredTime: -1}).Error)
	router := playgroundAuthRouter(t, func(c *gin.Context) {
		assert.Equal(t, user.Id, c.GetInt("id"))
		assert.Equal(t, token.Id, c.GetInt("token_id"))
		assert.Equal(t, token.Key, c.GetString("token_key"))
		assert.False(t, c.GetBool("token_unlimited_quota"))
		assert.Equal(t, 500, c.GetInt("token_quota"))
		assert.Empty(t, c.GetHeader("Authorization"), "browser session credentials are not relay credentials")
		assert.NotContains(t, c.Request.Header, "X-Api-Key")
		assert.Equal(t, "/pg/chat/completions", c.FullPath())
		assert.Equal(t, "/pg/chat/completions", c.Request.RequestURI)
		assert.Equal(t, "/pg/chat/completions", c.GetString("playground_original_path"))
		assert.Equal(t, "/v1/chat/completions", c.Request.URL.Path)
		request, _, err := getModelRequest(c)
		require.NoError(t, err)
		assert.Empty(t, request.Group, "legacy body.group cannot replace the selected key's routing group")
		assert.Equal(t, "default", common.GetContextKeyString(c, constant.ContextKeyTokenGroup))
		assert.Equal(t, "default", common.GetContextKeyString(c, constant.ContextKeyUsingGroup))
		info, err := relaycommon.GenRelayInfo(c, types.RelayFormatOpenAI, nil, nil)
		require.NoError(t, err)
		assert.False(t, info.IsPlayground, "real-key playground requests must use normal token accounting")
		assert.Equal(t, token.Id, info.TokenId)
		assert.Equal(t, token.Key, info.TokenKey)
		c.Request.URL.Path = "/pg/chat/completions"
		legacyAliasInfo, err := relaycommon.GenRelayInfo(c, types.RelayFormatOpenAI, nil, nil)
		require.NoError(t, err)
		assert.False(t, legacyAliasInfo.IsPlayground, "real key accounting cannot depend solely on alias normalization")
		c.Request.URL.Path = "/v1/chat/completions"
		c.Status(http.StatusNoContent)
	})
	request := httptest.NewRequest(http.MethodPost, "/pg/chat/completions", strings.NewReader(`{"model":"gpt-4o","group":"vip"}`))
	request.Header.Set("Authorization", "Bearer "+access)
	request.Header.Set("X-MyAPI-Key-ID", strconv.Itoa(token.Id))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	require.Equal(t, http.StatusNoContent, response.Code, response.Body.String())
	assert.Equal(t, "/pg/chat/completions", request.URL.Path, "outer audit middleware sees the original URL after relay completion")
	assert.Equal(t, "Bearer "+access, request.Header.Get("Authorization"), "outer browser session remains unchanged")
}

func TestPlaygroundRejectsMissingForeignAndUnavailableKeys(t *testing.T) {
	tests := []struct {
		name   string
		keyID  string
		mutate func(*testing.T, *gorm.DB, *model.User, *model.Token)
		want   int
	}{
		{name: "missing selection", keyID: "", want: http.StatusBadRequest},
		{name: "invalid selection", keyID: "sk-secret", want: http.StatusBadRequest},
		{name: "negative selection", keyID: "-1", want: http.StatusBadRequest},
		{name: "missing key", keyID: "99999", want: http.StatusUnauthorized},
		{name: "other user's key", mutate: func(t *testing.T, db *gorm.DB, user *model.User, key *model.Token) {
			require.NoError(t, db.Model(key).Update("user_id", user.Id+1).Error)
		}, want: http.StatusUnauthorized},
		{name: "deleted key", mutate: func(t *testing.T, db *gorm.DB, _ *model.User, key *model.Token) {
			require.NoError(t, db.Delete(key).Error)
		}, want: http.StatusUnauthorized},
		{name: "disabled key", mutate: func(t *testing.T, db *gorm.DB, _ *model.User, key *model.Token) {
			require.NoError(t, db.Model(key).Update("status", common.TokenStatusDisabled).Error)
		}, want: http.StatusUnauthorized},
		{name: "expired key", mutate: func(t *testing.T, db *gorm.DB, _ *model.User, key *model.Token) {
			require.NoError(t, db.Model(key).Update("expired_time", time.Now().Add(-time.Hour).Unix()).Error)
		}, want: http.StatusUnauthorized},
		{name: "exhausted key", mutate: func(t *testing.T, db *gorm.DB, _ *model.User, key *model.Token) {
			require.NoError(t, db.Model(key).Update("remain_quota", 0).Error)
		}, want: http.StatusUnauthorized},
		{name: "ip restricted", mutate: func(t *testing.T, db *gorm.DB, _ *model.User, key *model.Token) {
			require.NoError(t, db.Model(key).Update("allow_ips", "198.51.100.0/24").Error)
		}, want: http.StatusForbidden},
		{name: "group restricted", mutate: func(t *testing.T, db *gorm.DB, _ *model.User, key *model.Token) {
			require.NoError(t, db.Model(key).Update("group", "inaccessible-test-group").Error)
		}, want: http.StatusForbidden},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db, user, token, access := playgroundAuthFixture(t)
			keyID := test.keyID
			if test.mutate != nil {
				test.mutate(t, db, user, token)
				keyID = strconv.Itoa(token.Id)
			}
			router := playgroundAuthRouter(t, func(c *gin.Context) { t.Error("rejected request reached relay"); c.Status(http.StatusNoContent) })
			response := playgroundAuthRequest(router, http.MethodPost, "/pg/chat/completions", access, keyID, `{"model":"gpt-4o"}`)
			assert.Equal(t, test.want, response.Code, response.Body.String())
			assert.NotContains(t, response.Body.String(), token.Key)
		})
	}
}

func TestPlaygroundRejectsPATAndRevokedDashboardSession(t *testing.T) {
	db, _, token, access := playgroundAuthFixture(t)
	router := playgroundAuthRouter(t, func(c *gin.Context) { t.Error("unauthorized dashboard identity reached relay") })
	response := playgroundAuthRequest(router, http.MethodGet, "/pg/models", "playground-dashboard-pat", strconv.Itoa(token.Id), "")
	assert.Equal(t, http.StatusForbidden, response.Code, response.Body.String())
	require.NoError(t, db.Model(&model.UserSession{}).Where("sid = ?", "playground-live-session").Update("status", "revoked").Error)
	response = playgroundAuthRequest(router, http.MethodGet, "/pg/models", access, strconv.Itoa(token.Id), "")
	assert.Equal(t, http.StatusUnauthorized, response.Code, response.Body.String())
}

func TestPlaygroundDatabaseOwnershipAndRevocationOverrideStaleCredentialCache(t *testing.T) {
	for _, mutation := range []string{"status", "user_id", "deleted_at"} {
		t.Run(mutation, func(t *testing.T) {
			db, user, token, access := playgroundAuthFixture(t)
			useRateLimitMiniRedis(t)
			_, err := model.GetTokenByKey(token.Key, false)
			require.NoError(t, err)
			switch mutation {
			case "status":
				require.NoError(t, db.Model(token).Update("status", common.TokenStatusDisabled).Error)
			case "user_id":
				require.NoError(t, db.Model(token).Update("user_id", user.Id+1).Error)
			case "deleted_at":
				require.NoError(t, db.Delete(token).Error)
			}
			cached, err := model.GetTokenByKey(token.Key, false)
			require.NoError(t, err)
			require.Equal(t, common.TokenStatusEnabled, cached.Status, "fixture must retain the stale credential cache")
			require.Equal(t, user.Id, cached.UserId)
			router := playgroundAuthRouter(t, func(c *gin.Context) { t.Error("stale credential cache authorized playground") })
			response := playgroundAuthRequest(router, http.MethodGet, "/pg/models", access, strconv.Itoa(token.Id), "")
			assert.Equal(t, http.StatusUnauthorized, response.Code, response.Body.String())
		})
	}
}

func TestPlaygroundSharesAPIModelLimitsAndBudgetAdmission(t *testing.T) {
	for _, test := range []struct {
		name       string
		modelLimit bool
		budget     *model.TokenBudget
		path       string
		status     int
		code       string
	}{
		{name: "model rejected", modelLimit: true, path: "/chat/completions", status: http.StatusForbidden},
		{name: "strict budget admitted", budget: &model.TokenBudget{Enabled: true, Limit: 100, Revision: 1}, path: "/chat/completions", status: http.StatusNoContent},
		{name: "strict budget exhausted", budget: &model.TokenBudget{Enabled: true, Limit: 100, Used: 100, Revision: 1}, path: "/chat/completions", status: http.StatusForbidden, code: "token_budget_exceeded"},
		{name: "strict budget pending", budget: &model.TokenBudget{Enabled: true, Limit: 100, PendingRequestID: "pending", Revision: 1}, path: "/chat/completions", status: http.StatusConflict, code: "token_budget_pending"},
		{name: "query remains unsupported", budget: &model.TokenBudget{Enabled: true, Limit: 100, Revision: 1}, path: "/chat/completions?extra=1", status: http.StatusBadRequest, code: "token_budget_unsupported_request"},
	} {
		t.Run(test.name, func(t *testing.T) {
			db, user, token, access := playgroundAuthFixture(t)
			if test.modelLimit {
				require.NoError(t, db.Model(token).Updates(map[string]any{"model_limits_enabled": true, "model_limits": "allowed-model"}).Error)
			}
			if test.budget != nil {
				test.budget.TokenID = token.Id
				test.budget.UserID = user.Id
				require.NoError(t, db.Create(test.budget).Error)
			}
			terminal := func(c *gin.Context) {
				if test.budget != nil {
					assert.True(t, c.GetBool(string(constant.ContextKeyStrictTokenBudget)))
				}
				c.Status(http.StatusNoContent)
			}
			var additional []gin.HandlerFunc
			if test.modelLimit {
				additional = append(additional, Distribute())
			}
			pg := playgroundAuthRouter(t, terminal, additional...)
			api := gin.New()
			api.Use(BodyStorageCleanup())
			chain := append([]gin.HandlerFunc{TokenAuth()}, additional...)
			chain = append(chain, terminal)
			api.POST("/v1/chat/completions", chain...)
			pgResponse := playgroundAuthRequest(pg, http.MethodPost, "/pg"+test.path, access, strconv.Itoa(token.Id), `{"model":"forbidden-model"}`)
			apiResponse := playgroundAuthRequest(api, http.MethodPost, "/v1"+test.path, token.Key, "", `{"model":"forbidden-model"}`)
			assert.Equal(t, test.status, pgResponse.Code, pgResponse.Body.String())
			assert.Equal(t, apiResponse.Code, pgResponse.Code)
			if test.code != "" {
				assert.Contains(t, pgResponse.Body.String(), test.code)
				assert.Contains(t, apiResponse.Body.String(), test.code)
			}
		})
	}
}

func TestPlaygroundSharesSelfUseAdmissionAndRateLimit(t *testing.T) {
	db, user, token, access := playgroundAuthFixture(t)
	require.NoError(t, db.Model(user).Updates(map[string]any{"self_use_no_balance": true, "usage_policy_revision": 1}).Error)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		_, err := model.InitializeUserFundingStateTx(tx, operation_setting.UserFundingModeDisabled)
		return err
	}))
	prior := setting.GetModelRequestRateLimitConfig()
	require.NoError(t, setting.ApplyModelRequestRateLimitConfig(setting.ModelRequestRateLimitConfig{Enabled: true, DurationMinutes: 1, Success: 1, Group: map[string][2]int{}}))
	t.Cleanup(func() { require.NoError(t, setting.ApplyModelRequestRateLimitConfig(prior)) })
	router := playgroundAuthRouter(t, func(c *gin.Context) {
		assert.True(t, service.SupportsSelfUseMeteredRequest(c.Request))
		c.Status(http.StatusNoContent)
	}, ModelRequestRateLimit())
	first := playgroundAuthRequest(router, http.MethodPost, "/pg/chat/completions", access, strconv.Itoa(token.Id), `{"model":"gpt-4o"}`)
	require.Equal(t, http.StatusNoContent, first.Code, first.Body.String())
	api := gin.New()
	api.POST("/v1/chat/completions", TokenAuth(), ModelRequestRateLimit(), func(c *gin.Context) { t.Error("API and playground must share the same per-user rate limit") })
	second := playgroundAuthRequest(api, http.MethodPost, "/v1/chat/completions", token.Key, "", `{"model":"gpt-4o"}`)
	assert.Equal(t, http.StatusTooManyRequests, second.Code, second.Body.String())
	active, err := model.ResolveSelfUseNoBalanceAdmission(context.Background(), db, user.Id)
	require.NoError(t, err)
	assert.True(t, active)
}
