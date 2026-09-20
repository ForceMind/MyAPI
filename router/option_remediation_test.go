package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestOptionRemediateRoutesAreRootOnlyAndNoStore(t *testing.T) {
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
	common.SessionSecret = "option-remediate-route-test-secret"
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
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSession{}, &model.Option{}, &model.OptionRemediation{}, &model.OptionRemediationBackup{}))

	root := &model.User{
		Username: "option-remediate-root", Password: "unused", Role: common.RoleRootUser,
		Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, AffCode: "option-remediate-root-aff",
	}
	admin := &model.User{
		Username: "option-remediate-admin", Password: "unused", Role: common.RoleAdminUser,
		Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, AffCode: "option-remediate-admin-aff",
	}
	require.NoError(t, db.Create(root).Error)
	require.NoError(t, db.Create(admin).Error)
	_, rootToken := createCodexLocalRouteSession(t, root)
	_, adminToken := createCodexLocalRouteSession(t, admin)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetApiRouter(engine)
	request := func(method, path, body, token string) *httptest.ResponseRecorder {
		var reader *strings.Reader
		if body == "" {
			reader = strings.NewReader("")
		} else {
			reader = strings.NewReader(body)
		}
		req := httptest.NewRequest(method, "http://myapi.local"+path, reader)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, req)
		return response
	}

	paths := []struct{ method, path, body string }{
		{http.MethodPost, "/api/option/diagnostics/remediate/dry-run", `{"include_all": true}`},
		{http.MethodPost, "/api/option/diagnostics/remediate/apply", `{"items": []}`},
		{http.MethodGet, "/api/option/diagnostics/remediations", ""},
	}
	for _, endpoint := range paths {
		t.Run(endpoint.path, func(t *testing.T) {
			unauthorized := request(endpoint.method, endpoint.path, endpoint.body, "")
			assert.Equal(t, http.StatusUnauthorized, unauthorized.Code)
			assert.Contains(t, unauthorized.Header().Get("Cache-Control"), "no-store")
			forbidden := request(endpoint.method, endpoint.path, endpoint.body, adminToken)
			assert.Equal(t, http.StatusForbidden, forbidden.Code)
			assert.Contains(t, forbidden.Header().Get("Cache-Control"), "no-store")
		})
	}

	dryRun := request(http.MethodPost, "/api/option/diagnostics/remediate/dry-run", `{"include_all": true}`, rootToken)
	assert.Equal(t, http.StatusOK, dryRun.Code)
	assert.Contains(t, dryRun.Header().Get("Cache-Control"), "no-store")
	assert.Contains(t, dryRun.Body.String(), `"schema_version":"c09-option-remediation-v1"`)

	badApply := request(http.MethodPost, "/api/option/diagnostics/remediate/apply", `{"items": []}`, rootToken)
	assert.Equal(t, http.StatusOK, badApply.Code)
	assert.Contains(t, badApply.Body.String(), `"success":false`)

	history := request(http.MethodGet, "/api/option/diagnostics/remediations", "", rootToken)
	assert.Equal(t, http.StatusOK, history.Code)
	assert.Contains(t, history.Header().Get("Cache-Control"), "no-store")
	assert.Contains(t, history.Body.String(), `"success":true`)
}
