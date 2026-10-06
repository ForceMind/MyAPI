package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/controller"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/middleware"
	"github.com/ForceMind/MyAPI/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupQuotaWriterRouteTest(t *testing.T) (*gin.Engine, string, string) {
	t.Helper()
	previousDB := model.DB
	previousRedis, previousMemoryCache := common.RedisEnabled, common.MemoryCacheEnabled
	previousSessionSecret := common.SessionSecret
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(
		&model.User{}, &model.UserSession{}, &model.Token{}, &model.Option{},
		&model.QuotaWriterEpoch{}, &model.QuotaWriterModeTransition{},
		&model.QuotaBalanceBatchDrain{}, &model.QuotaBalanceBatchSubject{},
		&model.QuotaWorkCursor{}, &model.QuotaProjectionObligation{}))
	require.NoError(t, model.EnsureQuotaWriterEpochStateWithDB(db))
	model.DB = db
	common.RedisEnabled = false
	common.MemoryCacheEnabled = false
	common.SessionSecret = "quota-writer-route-test-secret"
	require.NoError(t, i18n.Init())
	t.Cleanup(func() {
		model.DB = previousDB
		common.RedisEnabled = previousRedis
		common.MemoryCacheEnabled = previousMemoryCache
		common.SessionSecret = previousSessionSecret
	})

	admin := &model.User{
		Username: "quota-writer-admin", Password: "unused", Role: common.RoleAdminUser,
		Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, AffCode: "quota-writer-admin-aff",
	}
	require.NoError(t, db.Create(admin).Error)
	commonUser := &model.User{
		Username: "quota-writer-user", Password: "unused", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, AffCode: "quota-writer-user-aff",
	}
	require.NoError(t, db.Create(commonUser).Error)
	_, adminToken := createCodexLocalRouteSession(t, admin)
	_, userToken := createCodexLocalRouteSession(t, commonUser)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	// Keep the route test deterministic: production admin auditing is
	// asynchronous and covered separately by middleware tests.
	engine.Use(func(c *gin.Context) {
		if c.Request.Method == http.MethodPost {
			common.SetContextKey(c, constant.ContextKeyAuditLogged, true)
		}
		c.Next()
	})
	quotaWriterRoute := engine.Group("/api/quota-writer")
	quotaWriterRoute.Use(middleware.AdminAuth())
	{
		quotaWriterRoute.GET("/status", controller.GetQuotaWriterStatus)
		quotaWriterRoute.GET("/plan", controller.GetQuotaWriterTransitionPlan)
		quotaWriterRoute.POST("/apply", controller.ApplyQuotaWriterModeTransition)
		quotaWriterRoute.GET("/transitions", controller.GetQuotaWriterTransitions)
		quotaWriterRoute.POST("/drain", controller.DriveQuotaWriterDrains)
	}
	return engine, adminToken, userToken
}

func TestQuotaWriterRoutesRequireAdminAndServeOperations(t *testing.T) {
	engine, adminToken, userToken := setupQuotaWriterRouteTest(t)

	request := func(method, path, token, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "http://myapi.local"+path, strings.NewReader(body))
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, req)
		return response
	}

	// Non-admin callers are rejected on every endpoint.
	for _, endpoint := range []struct{ method, path string }{
		{http.MethodGet, "/api/quota-writer/status"},
		{http.MethodGet, "/api/quota-writer/plan?target=bridge"},
		{http.MethodGet, "/api/quota-writer/transitions"},
		{http.MethodPost, "/api/quota-writer/apply"},
		{http.MethodPost, "/api/quota-writer/drain"},
	} {
		assert.Equal(t, http.StatusForbidden, request(endpoint.method, endpoint.path, userToken, "").Code, "%s %s must reject non-admin callers", endpoint.method, endpoint.path)
		assert.Equal(t, http.StatusUnauthorized, request(endpoint.method, endpoint.path, "", "").Code, "%s %s must reject anonymous callers", endpoint.method, endpoint.path)
	}

	// Status reports the persisted legacy singleton and the in-flight count.
	status := request(http.MethodGet, "/api/quota-writer/status", adminToken, "")
	require.Equal(t, http.StatusOK, status.Code)
	assert.Contains(t, status.Body.String(), `"mode":"legacy"`)
	assert.Contains(t, status.Body.String(), `"inflight_sessions"`)

	// The plan endpoint stays read-only.
	plan := request(http.MethodGet, "/api/quota-writer/plan?target=bridge", adminToken, "")
	require.Equal(t, http.StatusOK, plan.Code)
	assert.Contains(t, plan.Body.String(), `"target_mode":"bridge"`)
	invalidPlan := request(http.MethodGet, "/api/quota-writer/plan?target=bogus", adminToken, "")
	assert.Equal(t, http.StatusOK, invalidPlan.Code, "i18n errors use the standard 200 envelope")
	assert.Contains(t, invalidPlan.Body.String(), `"success":false`)

	// Apply without an acknowledgement fails closed with the missing checklist.
	blocked := request(http.MethodPost, "/api/quota-writer/apply", adminToken, `{"target_mode":"bridge","expected_epoch":1}`)
	require.Equal(t, http.StatusConflict, blocked.Code)
	assert.Contains(t, blocked.Body.String(), "cluster_drain_ack")

	// Apply with the ack note switches to bridge and persists evidence.
	applied := request(http.MethodPost, "/api/quota-writer/apply", adminToken, `{"target_mode":"bridge","expected_epoch":1,"ack_note":"route test drain"}`)
	require.Equal(t, http.StatusOK, applied.Code)
	assert.Contains(t, applied.Body.String(), `"status":"succeeded"`)
	assert.Contains(t, applied.Body.String(), `"to_mode":"bridge"`)

	// A repeated apply with the stale epoch conflicts.
	conflict := request(http.MethodPost, "/api/quota-writer/apply", adminToken, `{"target_mode":"bridge","expected_epoch":1,"ack_note":"route test drain"}`)
	assert.Equal(t, http.StatusConflict, conflict.Code)

	// History returns the persisted evidence rows, newest first.
	history := request(http.MethodGet, "/api/quota-writer/transitions", adminToken, "")
	require.Equal(t, http.StatusOK, history.Code)
	assert.Contains(t, history.Body.String(), `"total":2`)

	// The drain drive is reachable and bounded.
	drain := request(http.MethodPost, "/api/quota-writer/drain?budget=2", adminToken, "")
	require.Equal(t, http.StatusOK, drain.Code)
	assert.Contains(t, drain.Body.String(), `"rounds"`)
}
