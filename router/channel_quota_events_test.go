package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/middleware"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/service/authz"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestChannelQuotaEventsActualRoleBoundary(t *testing.T) {
	previousDB, previousLogDB := model.DB, model.LOG_DB
	oldRedis, oldCache, oldSecret, oldMaster := common.RedisEnabled, common.MemoryCacheEnabled, common.SessionSecret, common.IsMasterNode
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/events.db"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSession{}, &model.Log{}, &model.CasbinRule{}, &model.AuthzRole{}, &model.ChannelQuotaAlertEvent{}))
	model.DB, model.LOG_DB = db, db
	common.RedisEnabled, common.MemoryCacheEnabled, common.IsMasterNode = false, false, true
	common.SessionSecret = "synthetic-quota-events"
	require.NoError(t, authz.Init(db))
	t.Setenv("MYAPI_EDITION", middleware.EditionFull)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.RedisEnabled, common.MemoryCacheEnabled, common.SessionSecret, common.IsMasterNode = oldRedis, oldCache, oldSecret, oldMaster
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
	})
	admin := &model.User{Username: "event-admin", Role: common.RoleAdminUser, Status: common.UserStatusEnabled, AuthVersion: 1, Group: "default", AffCode: "event-admin"}
	user := &model.User{Username: "event-user", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, AuthVersion: 1, Group: "default", AffCode: "event-user"}
	require.NoError(t, db.Create(admin).Error)
	require.NoError(t, db.Create(user).Error)
	_, adminToken := createCodexLocalRouteSession(t, admin)
	_, userToken := createCodexLocalRouteSession(t, user)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	registerChannelRoutes(engine.Group("/api"))
	request := func(path, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "http://myapi.local"+path, nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, req)
		return response
	}
	require.Equal(t, http.StatusUnauthorized, request("/api/channel/quota/events", "").Code)
	require.Equal(t, http.StatusForbidden, request("/api/channel/quota/events", userToken).Code)
	allowed := request("/api/channel/quota/events", adminToken)
	require.Equal(t, http.StatusOK, allowed.Code)
	require.Contains(t, allowed.Header().Get("Cache-Control"), "no-store")
	require.Contains(t, allowed.Body.String(), `"success":true`)
	require.Equal(t, http.StatusForbidden, request("/api/channel/quota/alerts/delivery", adminToken).Code, "in-app read permission must not grant external delivery administration")
	require.NoError(t, authz.SetUserPermissions(admin.Id, authz.PermissionsMap{"channel": map[string]bool{"read": false}}))
	require.Equal(t, http.StatusForbidden, request("/api/channel/quota/events", adminToken).Code)
}
