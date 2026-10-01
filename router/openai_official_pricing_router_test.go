package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestOfficialPricingSnapshotRouteRequiresRoot(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/pricing-router.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Log{}))
	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldRedis, oldMemory := common.RedisEnabled, common.MemoryCacheEnabled
	oldMain, oldLog := common.MainDatabaseType(), common.LogDatabaseType()
	model.DB = db
	model.LOG_DB = db
	common.RedisEnabled, common.MemoryCacheEnabled = false, false
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.RedisEnabled, common.MemoryCacheEnabled = oldRedis, oldMemory
		common.SetDatabaseTypes(oldMain, oldLog)
		require.NoError(t, sqlDB.Close())
	})
	for _, tc := range []struct {
		name string
		role int
	}{
		{"common", common.RoleCommonUser}, {"admin", common.RoleAdminUser},
		{"root", common.RoleRootUser},
	} {
		pat := "pricing-source-" + tc.name
		user := model.User{Username: "pricing-source-" + tc.name, AffCode: "pricing-source-" + tc.name, Role: tc.role, Status: common.UserStatusEnabled, AccessToken: &pat}
		require.NoError(t, db.Create(&user).Error)
	}
	previousMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(previousMode) })
	engine := gin.New()
	SetApiRouter(engine)
	auditWritten := make(chan struct{}, 2)
	require.NoError(t, db.Callback().Create().After("gorm:create").Register("pricing-router-audit-complete", func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Name == "Log" && tx.Error == nil {
			auditWritten <- struct{}{}
		}
	}))
	for _, tc := range []struct {
		pat  string
		want int
	}{
		{"", http.StatusUnauthorized},
		{"pricing-source-common", http.StatusForbidden},
		{"pricing-source-admin", http.StatusForbidden},
	} {
		for _, endpoint := range []struct{ method, path string }{
			{http.MethodGet, "/api/ratio_sync/openai"},
			{http.MethodPost, "/api/ratio_sync/openai/versions"},
			{http.MethodGet, "/api/ratio_sync/openai/versions/" + strings.Repeat("a", 64)},
		} {
			response := httptest.NewRecorder()
			request := httptest.NewRequest(endpoint.method, endpoint.path, nil)
			if tc.pat != "" {
				request.Header.Set("Authorization", "Bearer "+tc.pat)
			}
			engine.ServeHTTP(response, request)
			assert.Equal(t, tc.want, response.Code)
		}
	}
	for _, origin := range []string{"", "http://cross-site.invalid"} {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "http://myapi.local/api/ratio_sync/openai/versions", nil)
		request.Header.Set("Authorization", "Bearer pricing-source-root")
		if origin != "" {
			request.Header.Set("Origin", origin)
		}
		engine.ServeHTTP(response, request)
		assert.Equal(t, http.StatusForbidden, response.Code, "Root authentication must not bypass origin guard")
		assert.Contains(t, strings.Split(response.Header().Get("Cache-Control"), ", "), "no-store")
	}
	// Authentication records rejected Root writes asynchronously. Wait for
	// their real DB writes before restoring global DB/cache fixture state.
	auditContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for range 2 {
		select {
		case <-auditWritten:
		case <-auditContext.Done():
			t.Fatal("Root audit did not finish before fixture cleanup")
		}
	}
}
