package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestSetApiRouterFundingPostsUseDatabaseGateBeforeHandlers(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Option{}))
	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldRedis, oldMemory := common.RedisEnabled, common.MemoryCacheEnabled
	oldFunding := operation_setting.GetUserFundingSetting()
	oldMain, oldLog := common.MainDatabaseType(), common.LogDatabaseType()
	model.DB, model.LOG_DB = db, db
	common.RedisEnabled, common.MemoryCacheEnabled = false, false
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.OptionMapRWMutex.Lock()
	oldOptionMap := common.OptionMap
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.RedisEnabled, common.MemoryCacheEnabled = oldRedis, oldMemory
		common.SetDatabaseTypes(oldMain, oldLog)
		require.NoError(t, operation_setting.PublishUserFundingSnapshot(oldFunding.Mode, oldFunding.Epoch))
		common.OptionMapRWMutex.Lock()
		common.OptionMap = oldOptionMap
		common.OptionMapRWMutex.Unlock()
		require.NoError(t, sqlDB.Close())
	})

	pat := "funding-route-pat"
	user := model.User{Username: "funding-route-user", AffCode: "funding-route", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, AccessToken: &pat}
	require.NoError(t, db.Create(&user).Error)
	var state model.UserFundingStateSnapshot
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		var initErr error
		state, initErr = model.InitializeUserFundingStateTx(tx, operation_setting.UserFundingModeDisabled)
		return initErr
	}))
	require.NoError(t, model.PublishUserFundingState(state))

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetApiRouter(engine)
	paths := []string{
		"/api/user/topup",
		"/api/user/pay",
		"/api/user/amount",
		"/api/user/stripe/pay",
		"/api/user/stripe/amount",
		"/api/user/creem/pay",
		"/api/user/waffo/amount",
		"/api/user/waffo/pay",
		"/api/user/waffo-pancake/amount",
		"/api/user/waffo-pancake/pay",
		"/api/user/aff_transfer",
		"/api/subscription/balance/pay",
		"/api/subscription/epay/pay",
		"/api/subscription/stripe/pay",
		"/api/subscription/creem/pay",
		"/api/subscription/waffo-pancake/pay",
	}
	for _, path := range paths {
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
		request.Header.Set("Authorization", "Bearer "+pat)
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		assert.Equal(t, http.StatusForbidden, response.Code, path)
		var payload map[string]any
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
		assert.Equal(t, "user_funding_unavailable", payload["code"], path)
	}
}
