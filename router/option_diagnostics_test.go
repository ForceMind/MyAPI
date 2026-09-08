package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestOptionDiagnosticsRouteIsRootOnlyAndNoStore(t *testing.T) {
	previousDB, previousRedis, previousMemoryCache := model.DB, common.RedisEnabled, common.MemoryCacheEnabled
	previousSessionSecret, previousType, previousGinMode := common.SessionSecret, common.MainDatabaseType(), gin.Mode()
	previousGlobalRateLimitEnable := common.GlobalApiRateLimitEnable
	previousGlobalRateLimitNum := common.GlobalApiRateLimitNum
	previousGlobalRateLimitDuration := common.GlobalApiRateLimitDuration
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	model.DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	common.MemoryCacheEnabled = false
	common.SessionSecret = "option-diagnostics-route-test-secret"
	common.GlobalApiRateLimitEnable = false
	common.GlobalApiRateLimitNum = 0
	common.GlobalApiRateLimitDuration = 0
	t.Cleanup(func() {
		model.DB = previousDB
		common.RedisEnabled = previousRedis
		common.MemoryCacheEnabled = previousMemoryCache
		common.SessionSecret = previousSessionSecret
		common.GlobalApiRateLimitEnable = previousGlobalRateLimitEnable
		common.GlobalApiRateLimitNum = previousGlobalRateLimitNum
		common.GlobalApiRateLimitDuration = previousGlobalRateLimitDuration
		common.SetMainDatabaseType(previousType)
		gin.SetMode(previousGinMode)
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSession{}, &model.Option{}))

	root := &model.User{
		Username: "option-route-root", Password: "unused", Role: common.RoleRootUser,
		Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, AffCode: "option-route-root-aff",
	}
	admin := &model.User{
		Username: "option-route-admin", Password: "unused", Role: common.RoleAdminUser,
		Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, AffCode: "option-route-admin-aff",
	}
	require.NoError(t, db.Create(root).Error)
	require.NoError(t, db.Create(admin).Error)
	_, rootToken := createCodexLocalRouteSession(t, root)
	_, adminToken := createCodexLocalRouteSession(t, admin)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetApiRouter(engine)
	request := func(token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "http://myapi.local/api/option/diagnostics?include_valid=false", nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, req)
		return response
	}

	unauthorized := request("")
	assert.Equal(t, http.StatusUnauthorized, unauthorized.Code)
	assert.Contains(t, unauthorized.Header().Get("Cache-Control"), "no-store")
	forbidden := request(adminToken)
	assert.Equal(t, http.StatusForbidden, forbidden.Code)
	assert.Contains(t, forbidden.Header().Get("Cache-Control"), "no-store")
	rootResponse := request(rootToken)
	assert.Equal(t, http.StatusOK, rootResponse.Code)
	assert.Contains(t, rootResponse.Header().Get("Cache-Control"), "no-store")
	assert.Contains(t, rootResponse.Body.String(), `"schema_version":"c09-option-diagnostic-v1"`)
}

func TestOptionDiagnosticsRouteDisablesCacheBeforeGlobalRateLimit(t *testing.T) {
	previousRedisEnabled, previousRedisClient := common.RedisEnabled, common.RDB
	previousGlobalRateLimitEnable := common.GlobalApiRateLimitEnable
	previousGlobalRateLimitNum := common.GlobalApiRateLimitNum
	previousGlobalRateLimitDuration := common.GlobalApiRateLimitDuration
	previousGinMode := gin.Mode()
	rateLimitRedis, err := miniredis.Run()
	require.NoError(t, err)
	rateLimitClient := redis.NewClient(&redis.Options{Addr: rateLimitRedis.Addr()})
	t.Cleanup(func() {
		require.NoError(t, rateLimitClient.Close())
		rateLimitRedis.Close()
		common.RedisEnabled = previousRedisEnabled
		common.RDB = previousRedisClient
		common.GlobalApiRateLimitEnable = previousGlobalRateLimitEnable
		common.GlobalApiRateLimitNum = previousGlobalRateLimitNum
		common.GlobalApiRateLimitDuration = previousGlobalRateLimitDuration
		gin.SetMode(previousGinMode)
	})

	gin.SetMode(gin.TestMode)

	t.Run("rejection", func(t *testing.T) {
		common.RedisEnabled = true
		common.RDB = rateLimitClient
		common.GlobalApiRateLimitEnable = true
		common.GlobalApiRateLimitNum = 1
		common.GlobalApiRateLimitDuration = 60

		engine := gin.New()
		require.NoError(t, engine.SetTrustedProxies(nil))
		SetApiRouter(engine)
		request := func() *httptest.ResponseRecorder {
			req := httptest.NewRequest(http.MethodGet, "http://myapi.local/api/option/diagnostics", nil)
			req.RemoteAddr = "198.51.100.91:43123"
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, req)
			return response
		}

		assert.Equal(t, http.StatusUnauthorized, request().Code)
		rejected := request()
		assert.Equal(t, http.StatusTooManyRequests, rejected.Code)
		assert.Contains(t, rejected.Header().Get("Cache-Control"), "no-store")
	})

	t.Run("backend error", func(t *testing.T) {
		common.RedisEnabled = true
		common.RDB = nil
		common.GlobalApiRateLimitEnable = true
		common.GlobalApiRateLimitNum = 1
		common.GlobalApiRateLimitDuration = 60

		engine := gin.New()
		require.NoError(t, engine.SetTrustedProxies(nil))
		SetApiRouter(engine)
		req := httptest.NewRequest(http.MethodGet, "http://myapi.local/api/option/diagnostics", nil)
		req.RemoteAddr = "198.51.100.92:43123"
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, req)

		assert.Equal(t, http.StatusInternalServerError, response.Code)
		assert.Contains(t, response.Header().Get("Cache-Control"), "no-store")
	})
}
