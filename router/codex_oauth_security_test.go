package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/middleware"
	"github.com/ForceMind/MyAPI/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestCodexOAuthActualRouteSecurityChain(t *testing.T) {
	previousGinMode := gin.Mode()
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousRedis, previousMemoryCache := common.RedisEnabled, common.MemoryCacheEnabled
	previousSessionSecret := common.SessionSecret
	previousLimitEnabled, previousLimitCount, previousLimitDuration := common.CriticalRateLimitEnable, common.CriticalRateLimitNum, common.CriticalRateLimitDuration
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSession{}, &model.AuthFlow{}, &model.Channel{}, &model.Log{}))
	model.DB, model.LOG_DB = db, db
	common.RedisEnabled = false
	common.MemoryCacheEnabled = false
	common.SessionSecret = "codex-oauth-route-test-secret"
	common.CriticalRateLimitEnable, common.CriticalRateLimitNum, common.CriticalRateLimitDuration = true, 20, 60
	t.Setenv("MYAPI_EDITION", middleware.EditionFull)
	t.Cleanup(func() {
		gin.SetMode(previousGinMode)
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.RedisEnabled, common.MemoryCacheEnabled = previousRedis, previousMemoryCache
		common.SessionSecret = previousSessionSecret
		common.CriticalRateLimitEnable, common.CriticalRateLimitNum, common.CriticalRateLimitDuration = previousLimitEnabled, previousLimitCount, previousLimitDuration
		require.NoError(t, sqlDB.Close())
	})
	pat := "codex-oauth-route-pat"
	root := &model.User{
		Username: "oauth-route-root", Password: "unused", Role: common.RoleRootUser,
		Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1,
		AccessToken: &pat, AffCode: "oauth-route-root-aff",
	}
	admin := &model.User{
		Username: "oauth-route-admin", Password: "unused", Role: common.RoleAdminUser,
		Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1,
		AffCode: "oauth-route-admin-aff",
	}
	require.NoError(t, db.Create(root).Error)
	require.NoError(t, db.Create(admin).Error)
	require.NoError(t, db.Create(&model.Channel{
		Id: 1, Type: constant.ChannelTypeCodex, Name: "route-codex",
		Key: `{"access_token":"old","refresh_token":"refresh","account_id":"account"}`,
	}).Error)
	_, rootToken := createCodexLocalRouteSession(t, root)
	_, adminToken := createCodexLocalRouteSession(t, admin)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyAuditLogged, true)
		c.Next()
	})
	engine.Use(middleware.EditionGuard())
	registerChannelRoutes(engine.Group("/api"))
	request := func(path, token, origin, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "http://myapi.local"+path, strings.NewReader(body))
		req.RemoteAddr = "203.0.113.88:43123"
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, req)
		return response
	}

	for _, path := range []string{"/api/channel/codex/oauth/start", "/api/channel/1/codex/oauth/start"} {
		valid := request(path, rootToken, "http://myapi.local", "{}")
		assert.Equal(t, http.StatusOK, valid.Code, path)
		assert.Contains(t, valid.Body.String(), "authorize_url", path)
		assert.Contains(t, valid.Header().Get("Cache-Control"), "no-store", path)
		crossOrigin := request(path, rootToken, "https://attacker.example", "{}")
		assert.Equal(t, http.StatusForbidden, crossOrigin.Code, path)
		assert.Contains(t, crossOrigin.Body.String(), "AUTH_ORIGIN_FORBIDDEN", path)
		assert.Equal(t, http.StatusForbidden, request(path, adminToken, "http://myapi.local", "{}").Code, path)
		patOnly := request(path, pat, "http://myapi.local", "{}")
		assert.Equal(t, http.StatusUnauthorized, patOnly.Code, path)
		assert.Contains(t, patOnly.Body.String(), "dashboard session required", path)
		assert.Contains(t, patOnly.Header().Get("Cache-Control"), "no-store", path)
	}
	missingOrigin := request("/api/channel/codex/oauth/start", rootToken, "", "{}")
	assert.Equal(t, http.StatusForbidden, missingOrigin.Code)
	assert.Contains(t, missingOrigin.Body.String(), "AUTH_ORIGIN_FORBIDDEN")
	for _, testCase := range []struct {
		path, response string
		status         int
	}{
		{"/api/channel/codex/oauth/complete", "invalid Codex callback URL", http.StatusOK},
		{"/api/channel/1/codex/oauth/complete", "invalid Codex callback URL", http.StatusOK},
	} {
		body := "{}"
		if testCase.path == "/api/channel/codex/oauth/complete" {
			body = `{"create":{"mode":"single","channel":{"type":57,"key":"","models":"gpt-5","setting":"{}"}}}`
		}
		validSession := request(testCase.path, rootToken, "http://myapi.local", body)
		assert.Equal(t, testCase.status, validSession.Code, testCase.path)
		assert.Contains(t, validSession.Body.String(), testCase.response, testCase.path)
		assert.Contains(t, validSession.Header().Get("Cache-Control"), "no-store", testCase.path)
		crossOrigin := request(testCase.path, rootToken, "https://attacker.example", "{}")
		assert.Equal(t, http.StatusForbidden, crossOrigin.Code, testCase.path)
		assert.Contains(t, crossOrigin.Body.String(), "AUTH_ORIGIN_FORBIDDEN", testCase.path)
		assert.Equal(t, http.StatusForbidden, request(testCase.path, adminToken, "http://myapi.local", "{}").Code, testCase.path)
		patOnly := request(testCase.path, pat, "http://myapi.local", "{}")
		assert.Equal(t, http.StatusUnauthorized, patOnly.Code, testCase.path)
		assert.Contains(t, patOnly.Body.String(), "dashboard session required", testCase.path)
		assert.Contains(t, patOnly.Header().Get("Cache-Control"), "no-store", testCase.path)
	}
	var limited *httptest.ResponseRecorder
	for range common.CriticalRateLimitNum + 1 {
		req := httptest.NewRequest(http.MethodPost, "http://myapi.local/api/channel/codex/oauth/complete", strings.NewReader("{}"))
		req.RemoteAddr = "203.0.113.89:43123"
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+rootToken)
		req.Header.Set("Origin", "http://myapi.local")
		limited = httptest.NewRecorder()
		engine.ServeHTTP(limited, req)
	}
	require.NotNil(t, limited)
	assert.Equal(t, http.StatusTooManyRequests, limited.Code)
	assert.Contains(t, limited.Header().Get("Cache-Control"), "no-store")
}
