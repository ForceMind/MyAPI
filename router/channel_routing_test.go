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
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/service/authz"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestChannelRoutingPermissionContract(t *testing.T) {
	assertChannelRoutePermission(t, http.MethodGet, "/routing", authz.ChannelRead, controller.GetChannelRouting)
	assertChannelRoutePermission(t, http.MethodPost, "/routing/preview", authz.ChannelRead, controller.PreviewChannelRouting)
	assertChannelRoutePermission(t, http.MethodPut, "/routing/channels/:id", authz.ChannelWrite, controller.UpdateChannelRoutingValues)
	assertChannelRoutePermission(t, http.MethodPut, "/routing", authz.ChannelWrite, controller.UpdateChannelRoutingPolicy)
}

func TestChannelRoutingActualAuthenticatedRoutes(t *testing.T) {
	require.NoError(t, i18n.Init())
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousRedis, previousMemory := common.RedisEnabled, common.MemoryCacheEnabled
	previousSecret := common.SessionSecret
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSession{}, &model.Channel{}, &model.ChannelQuotaSnapshot{}))
	model.DB, model.LOG_DB = db, db
	common.RedisEnabled, common.MemoryCacheEnabled = false, false
	common.SessionSecret = "routing-route-fixture"
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.RedisEnabled, common.MemoryCacheEnabled = previousRedis, previousMemory
		common.SessionSecret = previousSecret
		require.NoError(t, sqlDB.Close())
	})
	root := &model.User{Username: "routing-root", Password: "unused", Role: common.RoleRootUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, AffCode: "routing-root"}
	user := &model.User{Username: "routing-user", Password: "unused", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, AffCode: "routing-user"}
	require.NoError(t, db.Create(root).Error)
	require.NoError(t, db.Create(user).Error)
	_, rootToken := createCodexLocalRouteSession(t, root)
	_, userToken := createCodexLocalRouteSession(t, user)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(func(c *gin.Context) { common.SetContextKey(c, constant.ContextKeyAuditLogged, true); c.Next() })
	registerChannelRoutes(engine.Group("/api"))
	for _, route := range []struct {
		method, path string
		rootStatus   int
	}{
		{http.MethodGet, "/api/channel/routing", http.StatusOK},
		{http.MethodPost, "/api/channel/routing/preview", http.StatusBadRequest},
		{http.MethodPut, "/api/channel/routing", http.StatusBadRequest},
		{http.MethodPut, "/api/channel/routing/channels/1", http.StatusBadRequest},
	} {
		t.Run(route.method+route.path, func(t *testing.T) {
			for _, identity := range []struct {
				name, token string
				status      int
			}{
				{"anonymous", "", http.StatusUnauthorized},
				{"ordinary-user", userToken, http.StatusForbidden},
				{"root", rootToken, route.rootStatus},
			} {
				t.Run(identity.name, func(t *testing.T) {
					req := httptest.NewRequest(route.method, "http://myapi.local"+route.path, strings.NewReader("{}"))
					req.Header.Set("Content-Type", "application/json")
					if identity.token != "" {
						req.Header.Set("Authorization", "Bearer "+identity.token)
					}
					response := httptest.NewRecorder()
					engine.ServeHTTP(response, req)
					assert.Equal(t, identity.status, response.Code, response.Body.String())
				})
			}
		})
	}
}
