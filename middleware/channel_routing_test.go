package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/service"
	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/ForceMind/MyAPI/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestDistributeRetriesSmartRoutingWhenSelectedChannelHasNoEnabledKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousDB := model.DB
	previousMemory := common.MemoryCacheEnabled
	previousRetries := common.RetryTimes
	previousPolicy := operation_setting.GetChannelRoutingPolicy()
	db, err := gorm.Open(sqlite.Open("file:middleware-channel-routing?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ChannelQuotaSnapshot{}))
	model.DB = db
	common.MemoryCacheEnabled = false
	common.RetryTimes = 1
	policy := operation_setting.ChannelRoutingPolicy{
		Enabled: true, StickyEnabled: false, SessionTTLSeconds: 3600, QuotaMaxAgeSeconds: 60,
	}
	encoded, err := operation_setting.MarshalChannelRoutingPolicy(policy)
	require.NoError(t, err)
	require.NoError(t, config.UpdateConfigFromMap(config.GlobalConfig.Get("routing_policy_setting"), map[string]string{"policy": encoded}))
	t.Cleanup(func() {
		model.DB = previousDB
		common.MemoryCacheEnabled = previousMemory
		common.RetryTimes = previousRetries
		previousEncoded, marshalErr := operation_setting.MarshalChannelRoutingPolicy(previousPolicy)
		require.NoError(t, marshalErr)
		require.NoError(t, config.UpdateConfigFromMap(config.GlobalConfig.Get("routing_policy_setting"), map[string]string{"policy": previousEncoded}))
		sqlDB, sqlErr := db.DB()
		require.NoError(t, sqlErr)
		require.NoError(t, sqlDB.Close())
	})

	badPriority, goodPriority := int64(10), int64(9)
	weight := uint(1)
	bad := model.Channel{
		Type: 1, Key: "disabled-key", Status: common.ChannelStatusEnabled, Name: "no-enabled-key",
		Models: "routing-model", Group: "default", Priority: &badPriority, Weight: &weight,
		ChannelInfo: model.ChannelInfo{
			IsMultiKey: true, MultiKeyStatusList: map[int]int{0: common.ChannelStatusManuallyDisabled},
		},
	}
	good := model.Channel{
		Type: 1, Key: "working-key", Status: common.ChannelStatusEnabled, Name: "working",
		Models: "routing-model", Group: "default", Priority: &goodPriority, Weight: &weight,
	}
	require.NoError(t, db.Create(&bad).Error)
	require.NoError(t, bad.AddAbilities(nil))
	require.NoError(t, db.Create(&good).Error)
	require.NoError(t, good.AddAbilities(nil))

	var selectedChannel int
	var selectedKey string
	var routingInfo map[string]interface{}
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
		common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
		common.SetContextKey(c, constant.ContextKeyUserId, 7)
		common.SetContextKey(c, constant.ContextKeyTokenId, 11)
		c.Next()
	}, Distribute())
	engine.POST("/v1/chat/completions", func(c *gin.Context) {
		selectedChannel = common.GetContextKeyInt(c, constant.ContextKeyChannelId)
		selectedKey = common.GetContextKeyString(c, constant.ContextKeyChannelKey)
		adminInfo := map[string]interface{}{}
		service.AppendChannelRoutingAdminInfo(c, adminInfo)
		routingInfo, _ = adminInfo["channel_routing"].(map[string]interface{})
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"routing-model"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)

	assert.Equal(t, http.StatusNoContent, response.Code)
	assert.Equal(t, good.Id, selectedChannel)
	assert.Equal(t, "working-key", selectedKey)
	require.NotNil(t, routingInfo)
	assert.Equal(t, "channel_no_available_key", routingInfo["switch_reason"])
}
