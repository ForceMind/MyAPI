package router

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/controller"
	"github.com/ForceMind/MyAPI/middleware"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/service"
	"github.com/ForceMind/MyAPI/service/authz"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestChannelStatusRoutesUseOperatePermission(t *testing.T) {
	assertChannelRoutePermission(t, http.MethodPost, "/:id/status", authz.ChannelOperate, controller.UpdateChannelStatus)
	assertChannelRoutePermission(t, http.MethodPost, "/status/batch", authz.ChannelOperate, controller.BatchUpdateChannelStatus)
	assertChannelRoutePermission(t, http.MethodPut, "/", authz.ChannelWrite, controller.UpdateChannel)
}

func TestChannelDeleteRoutesUseSensitiveWritePermission(t *testing.T) {
	assertChannelRoutePermission(t, http.MethodDelete, "/:id", authz.ChannelSensitiveWrite, controller.DeleteChannel)
	assertChannelRoutePermission(t, http.MethodPost, "/batch", authz.ChannelSensitiveWrite, controller.DeleteChannelBatch)
	assertChannelRoutePermission(t, http.MethodDelete, "/disabled", authz.ChannelSensitiveWrite, controller.DeleteDisabledChannel)
	assertChannelRoutePermission(t, http.MethodPut, "/", authz.ChannelWrite, controller.UpdateChannel)
	assertChannelRoutePermission(t, http.MethodPut, "/tag", authz.ChannelWrite, controller.EditTagChannels)
	assertChannelRoutePermission(t, http.MethodPost, "/batch/tag", authz.ChannelWrite, controller.BatchSetChannelTag)
}

func TestChannelStatusRoutesRegisterWithoutConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	api := engine.Group("/api")

	require.NotPanics(t, func() {
		registerChannelRoutes(api)
	})
}

func TestChannelQuotaChangesRouteUsesReadPermission(t *testing.T) {
	assertChannelRoutePermission(t, http.MethodGet, "/quota/changes", authz.ChannelRead, controller.GetChannelQuotaChanges)
	assertChannelRoutePermission(t, http.MethodGet, "/quota/status", authz.ChannelRead, controller.GetChannelQuotaSamplingStatus)
}

func TestChannelRoutingPreviewRouteUsesReadPermission(t *testing.T) {
	assertChannelRoutePermission(t, http.MethodGet, "/routing-preview", authz.ChannelRead, controller.GetChannelRoutingPreview)
}

func TestCodexLocalAuthRoutesUseSensitiveWritePermission(t *testing.T) {
	assertChannelRoutePermission(t, http.MethodGet, "/codex/local-auth/status", authz.ChannelSensitiveWrite, controller.GetCodexLocalAuthStatus)
	assertChannelRoutePermission(t, http.MethodPost, "/codex/local-auth/import", authz.ChannelSensitiveWrite, controller.ImportCodexLocalAuth)
	assertChannelRoutePermission(t, http.MethodPost, "/:id/codex/local-auth/import", authz.ChannelSensitiveWrite, controller.ImportCodexLocalAuthForChannel)
	assert.Len(t, findChannelPermissionRoute(t, "/codex/local-auth/status").middleware, 3, "root auth, rate limiting and no-store are required")
	assert.Len(t, findChannelPermissionRoute(t, "/codex/local-auth/import").middleware, 5, "root auth, origin, rate limiting, no-store and proof are required")
	assert.Len(t, findChannelPermissionRoute(t, "/:id/codex/local-auth/import").middleware, 5, "root auth, origin, rate limiting, no-store and proof are required")
}

func TestCodexLocalAuthActualRouteSecurityChain(t *testing.T) {
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousRedis, previousMemoryCache := common.RedisEnabled, common.MemoryCacheEnabled
	previousSessionSecret := common.SessionSecret
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSession{}, &model.Log{}, &model.Channel{}))
	model.DB, model.LOG_DB = db, db
	common.RedisEnabled = false
	common.MemoryCacheEnabled = false
	common.SessionSecret = "codex-local-route-test-secret"
	t.Setenv("MYAPI_EDITION", middleware.EditionFull)
	t.Setenv("MYAPI_RUNTIME_ENV", "container")
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.RedisEnabled = previousRedis
		common.MemoryCacheEnabled = previousMemoryCache
		common.SessionSecret = previousSessionSecret
	})

	pat := "root-route-pat"
	root := &model.User{
		Username: "route-root", Password: "unused", Role: common.RoleRootUser,
		Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1,
		AccessToken: &pat, AffCode: "route-root-aff",
	}
	require.NoError(t, db.Create(root).Error)
	admin := &model.User{
		Username: "route-admin", Password: "unused", Role: common.RoleAdminUser,
		Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1,
		AffCode: "route-admin-aff",
	}
	require.NoError(t, db.Create(admin).Error)
	require.NoError(t, db.Create(&model.Channel{
		Id: 1, Type: constant.ChannelTypeCodex, Name: "route-codex",
		Key: `{"access_token":"old","refresh_token":"refresh","account_id":"account"}`,
	}).Error)

	rootIdentity, rootToken := createCodexLocalRouteSession(t, root)
	_, adminToken := createCodexLocalRouteSession(t, admin)
	proof, _, err := service.IssueSecurityProof(rootIdentity, "passkey", []string{middleware.SecurityProofScopeCodexLocalImport})
	require.NoError(t, err)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	// Keep the route-security test deterministic: production admin auditing is
	// asynchronous and is covered separately by middleware tests.
	engine.Use(func(c *gin.Context) {
		if c.Request.Method == http.MethodPost {
			common.SetContextKey(c, constant.ContextKeyAuditLogged, true)
		}
		c.Next()
	})
	engine.Use(middleware.EditionGuard())
	registerChannelRoutes(engine.Group("/api"))

	request := func(method, path, token, origin, securityProof string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "http://myapi.local"+path, strings.NewReader(""))
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		if securityProof != "" {
			req.Header.Set("X-Security-Proof", securityProof)
		}
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, req)
		return response
	}

	status := request(http.MethodGet, "/api/channel/codex/local-auth/status", rootToken, "", "")
	assert.Equal(t, http.StatusOK, status.Code)
	assert.Contains(t, status.Header().Get("Cache-Control"), "no-store")
	assert.Contains(t, status.Body.String(), "container_host_unavailable")

	patStatus := request(http.MethodGet, "/api/channel/codex/local-auth/status", pat, "", "")
	assert.Equal(t, http.StatusUnauthorized, patStatus.Code)
	assert.Contains(t, patStatus.Body.String(), "AUTH_SESSION_REQUIRED")
	assert.Contains(t, patStatus.Header().Get("Cache-Control"), "no-store")

	adminStatus := request(http.MethodGet, "/api/channel/codex/local-auth/status", adminToken, "", "")
	assert.Equal(t, http.StatusForbidden, adminStatus.Code)

	crossOrigin := request(http.MethodPost, "/api/channel/1/codex/local-auth/import", rootToken, "https://attacker.example", proof)
	assert.Equal(t, http.StatusForbidden, crossOrigin.Code)
	assert.Contains(t, crossOrigin.Body.String(), "AUTH_ORIGIN_FORBIDDEN")
	assert.Contains(t, crossOrigin.Header().Get("Cache-Control"), "no-store")

	missingProof := request(http.MethodPost, "/api/channel/1/codex/local-auth/import", rootToken, "http://myapi.local", "")
	assert.Equal(t, http.StatusForbidden, missingProof.Code)
	assert.Contains(t, missingProof.Body.String(), "SECURITY_PROOF_REQUIRED")
	assert.Contains(t, missingProof.Header().Get("Cache-Control"), "no-store")

	validImport := request(http.MethodPost, "/api/channel/1/codex/local-auth/import", rootToken, "http://myapi.local", proof)
	assert.Equal(t, http.StatusConflict, validImport.Code)
	assert.Contains(t, validImport.Body.String(), "CODEX_LOCAL_AUTH_CONTAINER_HOST_UNAVAILABLE")
	assert.Contains(t, validImport.Header().Get("Cache-Control"), "no-store")

	if common.CriticalRateLimitEnable {
		var rateLimited *httptest.ResponseRecorder
		for range common.CriticalRateLimitNum + 1 {
			req := httptest.NewRequest(http.MethodGet, "http://myapi.local/api/channel/codex/local-auth/status", nil)
			req.RemoteAddr = "203.0.113.77:43123"
			req.Header.Set("Authorization", "Bearer "+rootToken)
			rateLimited = httptest.NewRecorder()
			engine.ServeHTTP(rateLimited, req)
		}
		require.NotNil(t, rateLimited)
		assert.Equal(t, http.StatusTooManyRequests, rateLimited.Code)
		assert.Contains(t, rateLimited.Header().Get("Cache-Control"), "no-store")
	}

	t.Setenv("MYAPI_EDITION", middleware.EditionLAN)
	lanStatus := request(http.MethodGet, "/api/channel/codex/local-auth/status", rootToken, "", "")
	assert.Equal(t, http.StatusForbidden, lanStatus.Code)
	assert.Contains(t, lanStatus.Body.String(), "MYAPI_LAN_ROUTE_DISABLED")
	assert.Contains(t, lanStatus.Header().Get("Cache-Control"), "no-store")
}

func createCodexLocalRouteSession(t *testing.T, user *model.User) (service.AuthIdentity, string) {
	t.Helper()
	now := time.Now().Unix()
	session := &model.UserSession{
		SID: "codex-local-route-session-" + user.Username, UserID: user.Id,
		Version: 1, UserAuthVersion: user.AuthVersion, Status: model.UserSessionStatusActive,
		RefreshHash: "unused-refresh-hash-" + user.Username, LoginMethod: "password",
		CreatedAt: now, LastActiveAt: now, ExpiresAt: now + 3600,
	}
	require.NoError(t, model.CreateUserSession(session))
	identity := service.AuthIdentity{
		UserID: user.Id, SessionID: session.SID,
		UserAuthVersion: session.UserAuthVersion, SessionVersion: session.Version,
	}
	token, _, err := service.IssueAccessToken(identity)
	require.NoError(t, err)
	return identity, token
}

func assertChannelRoutePermission(t *testing.T, method string, path string, permission authz.Permission, handler any) {
	t.Helper()
	for _, route := range channelPermissionRoutes {
		if route.method == method && route.path == path {
			assert.Equal(t, permission, route.permission)
			assert.Equal(t, reflect.ValueOf(handler).Pointer(), reflect.ValueOf(route.handler).Pointer())
			return
		}
	}
	t.Fatalf("route %s %s not found", method, path)
}

func findChannelPermissionRoute(t *testing.T, path string) permissionRoute {
	t.Helper()
	for _, route := range channelPermissionRoutes {
		if route.path == path {
			return route
		}
	}
	t.Fatalf("route %s not found", path)
	return permissionRoute{}
}

func TestChannelRoutingPreviewActualAuthChain(t *testing.T) {
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousRedis, previousMemoryCache := common.RedisEnabled, common.MemoryCacheEnabled
	previousSessionSecret := common.SessionSecret
	db, err := gorm.Open(sqlite.Open("file:channel-routing-auth?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.User{}, &model.UserSession{}, &model.Log{}, &model.Channel{}, &model.Ability{},
		&model.CasbinRule{}, &model.AuthzRole{},
	))
	model.DB, model.LOG_DB = db, db
	common.RedisEnabled = false
	common.MemoryCacheEnabled = false
	common.SessionSecret = "channel-routing-auth-secret"
	previousMaster := common.IsMasterNode
	common.IsMasterNode = true
	require.NoError(t, authz.Init(db))
	t.Setenv("MYAPI_EDITION", middleware.EditionFull)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.RedisEnabled = previousRedis
		common.MemoryCacheEnabled = previousMemoryCache
		common.SessionSecret = previousSessionSecret
		common.IsMasterNode = previousMaster
	})

	admin := &model.User{
		Username: "routing-admin", Password: "unused", Role: common.RoleAdminUser,
		Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1,
		AffCode: "routing-admin-aff",
	}
	require.NoError(t, db.Create(admin).Error)
	user := &model.User{
		Username: "routing-user", Password: "unused", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1,
		AffCode: "routing-user-aff",
	}
	require.NoError(t, db.Create(user).Error)
	_, adminToken := createCodexLocalRouteSession(t, admin)
	_, userToken := createCodexLocalRouteSession(t, user)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	registerChannelRoutes(engine.Group("/api"))
	request := func(token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "http://myapi.local/api/channel/routing-preview?group=default&model=gpt-test", nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, req)
		return response
	}

	unauthenticated := request("")
	assert.Equal(t, http.StatusUnauthorized, unauthenticated.Code)
	forbidden := request(userToken)
	assert.Equal(t, http.StatusForbidden, forbidden.Code)
	authorized := request(adminToken)
	assert.Equal(t, http.StatusOK, authorized.Code)
	assert.Contains(t, authorized.Body.String(), `"source":"database"`)
	assert.Contains(t, authorized.Body.String(), `"cluster_committed_epoch":`)
}
