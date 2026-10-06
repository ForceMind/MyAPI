package relay

import (
	"context"
	"errors"
	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/model"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/service"
	"github.com/ForceMind/MyAPI/setting"
	"github.com/ForceMind/MyAPI/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResolveOriginTaskDoesNotSelectExhaustedCodexOriginKey(t *testing.T) {
	require.NoError(t, i18n.Init())
	previousDB := model.DB
	previousMemoryCache := common.MemoryCacheEnabled
	previousMainDatabaseType, previousLogDatabaseType := common.MainDatabaseType(), common.LogDatabaseType()
	originalUsableGroups := setting.UserUsableGroups2JSONString()
	originalGroupRatios := ratio_setting.GroupRatio2JSONString()
	t.Setenv(common.ChannelQuotaIdentityKeysEnv, "")
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.Task{}, &model.Channel{}, &model.Ability{}, &model.ChannelQuotaSnapshot{}))
	model.DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	model.InitColumnNamesForTest()
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		model.DB = previousDB
		common.MemoryCacheEnabled = previousMemoryCache
		common.SetDatabaseTypes(previousMainDatabaseType, previousLogDatabaseType)
		model.InitColumnNamesForTest()
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUsableGroups))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalGroupRatios))
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString("{\"group-b\":\"Group B\"}"))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString("{\"group-b\":1}"))

	priority := int64(0)
	weight := uint(1)
	channel := model.Channel{
		Id: 72, Type: constant.ChannelTypeCodex, Name: "origin-codex",
		Key:    "{\"access_token\":\"fixture-access\",\"account_id\":\"fixture-account\"}",
		Status: common.ChannelStatusEnabled, Group: "group-b", Models: "gpt-5-codex",
		Priority: &priority, Weight: &weight,
	}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group: "group-b", Model: "gpt-5-codex", ChannelId: channel.Id,
		Enabled: true, Priority: &priority, Weight: weight,
	}).Error)
	require.NoError(t, db.Create(&model.Task{
		TaskID: "origin-codex-remix", UserId: 17, Group: "group-b", ChannelId: channel.Id,
		Properties: model.Properties{OriginModelName: "gpt-5-codex"},
	}).Error)
	require.NoError(t, service.RecordCodexUsageLimit(context.Background(), channel.Id, channel.Key))
	excluded, eligible, err := service.CodexQuotaEligibleKeys(context.Background(), &channel)
	require.NoError(t, err)
	assert.True(t, excluded[0])
	assert.False(t, eligible)

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/videos/origin-codex-remix/remix", nil)
	ctx.Params = gin.Params{{Key: "video_id", Value: "origin-codex-remix"}}
	info := &relaycommon.RelayInfo{
		UserId: 17, UserGroup: "group-b", TokenGroup: "group-b",
		ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 999}, TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	}

	taskErr := ResolveOriginTask(ctx, info)
	require.NotNil(t, taskErr)
	assert.Equal(t, http.StatusServiceUnavailable, taskErr.StatusCode)
	assert.Equal(t, "channel_no_available_key", taskErr.Code)
	assert.Empty(t, common.GetContextKeyString(ctx, constant.ContextKeyChannelKey))

	healthyKey := "{\"access_token\":\"fixture-access-b\",\"account_id\":\"fixture-account-b\"}"
	fallback := model.Channel{
		Id: 73, Type: constant.ChannelTypeCodex, Name: "origin-codex-fallback",
		Key:    channel.Key + "\n" + healthyKey,
		Status: common.ChannelStatusEnabled, Group: "group-b", Models: "gpt-5-codex",
		Priority: &priority, Weight: &weight,
		ChannelInfo: model.ChannelInfo{IsMultiKey: true, MultiKeySize: 2, MultiKeyMode: constant.MultiKeyModeRandom},
	}
	require.NoError(t, db.Create(&fallback).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group: "group-b", Model: "gpt-5-codex", ChannelId: fallback.Id,
		Enabled: true, Priority: &priority, Weight: weight,
	}).Error)
	require.NoError(t, db.Model(&model.Task{}).Where("task_id = ?", "origin-codex-remix").Update("channel_id", fallback.Id).Error)
	next, _ := gin.CreateTestContext(httptest.NewRecorder())
	next.Request = httptest.NewRequest(http.MethodPost, "/v1/videos/origin-codex-remix/remix", nil)
	next.Params = gin.Params{{Key: "video_id", Value: "origin-codex-remix"}}
	nextInfo := &relaycommon.RelayInfo{
		UserId: 17, UserGroup: "group-b", TokenGroup: "group-b",
		ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 999}, TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	}
	require.Nil(t, ResolveOriginTask(next, nextInfo))
	assert.Equal(t, healthyKey, common.GetContextKeyString(next, constant.ContextKeyChannelKey))

	const callbackName = "test:codex-origin-quota-read-outage"
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == "channel_quota_snapshots" {
			tx.AddError(errors.New("synthetic quota read outage"))
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Query().Remove(callbackName) })
	unavailable, _ := gin.CreateTestContext(httptest.NewRecorder())
	unavailable.Request = httptest.NewRequest(http.MethodPost, "/v1/videos/origin-codex-remix/remix", nil)
	unavailable.Params = gin.Params{{Key: "video_id", Value: "origin-codex-remix"}}
	unavailableInfo := &relaycommon.RelayInfo{
		UserId: 17, UserGroup: "group-b", TokenGroup: "group-b",
		ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 999}, TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	}
	taskErr = ResolveOriginTask(unavailable, unavailableInfo)
	require.NotNil(t, taskErr)
	assert.Equal(t, http.StatusServiceUnavailable, taskErr.StatusCode)
	assert.Equal(t, "channel_no_available_key", taskErr.Code)
	assert.NotContains(t, taskErr.Message, "synthetic quota read outage")
	assert.Empty(t, common.GetContextKeyString(unavailable, constant.ContextKeyChannelKey))
}
