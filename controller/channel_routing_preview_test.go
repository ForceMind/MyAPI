package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupChannelRoutingPreviewTest(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB := model.DB
	previousMemoryCache := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}))
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		common.MemoryCacheEnabled = previousMemoryCache
	})
	return db
}

func TestGetChannelRoutingPreviewReturnsRedactedPolicy(t *testing.T) {
	setupChannelRoutingPreviewTest(t)
	priorityHigh := int64(20)
	priorityLow := int64(10)
	zero := uint(0)
	positive := uint(30)
	baseURL := "https://secret.example"
	channels := []*model.Channel{
		{Id: 2, Type: constant.ChannelTypeOpenAI, Key: "secret-key-two", BaseURL: &baseURL, Status: common.ChannelStatusEnabled, Name: "zero", Group: "default", Models: "gpt-test", Priority: &priorityHigh, Weight: &zero},
		{Id: 1, Type: constant.ChannelTypeOpenAI, Key: "secret-key-one", BaseURL: &baseURL, Status: common.ChannelStatusEnabled, Name: "positive", Group: "default", Models: "gpt-test", Priority: &priorityHigh, Weight: &positive},
		{Id: 3, Type: constant.ChannelTypeOpenAI, Key: "secret-key-three", BaseURL: &baseURL, Status: common.ChannelStatusEnabled, Name: "fallback", Group: "default", Models: "gpt-test", Priority: &priorityLow, Weight: &zero},
	}
	for _, channel := range channels {
		require.NoError(t, model.DB.Create(channel).Error)
		require.NoError(t, channel.AddAbilities(nil))
	}

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/channel/routing-preview?group=default&model=gpt-test&request_path=/v1/chat/completions", nil)

	GetChannelRoutingPreview(context)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.NotContains(t, recorder.Body.String(), "secret-key")
	assert.NotContains(t, recorder.Body.String(), "secret.example")
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			Tiers    []channelRoutingPreviewTier `json:"tiers"`
			Affinity struct {
				Evaluated       bool   `json:"evaluated"`
				Precedence      string `json:"precedence"`
				ExplanationCode string `json:"explanation_code"`
			} `json:"affinity"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success, recorder.Body.String())
	require.Len(t, response.Data.Tiers, 2)
	assert.Equal(t, int64(20), response.Data.Tiers[0].Priority)
	require.Len(t, response.Data.Tiers[0].Channels, 2)
	assert.Equal(t, 1, response.Data.Tiers[0].Channels[0].ID)
	assert.Equal(t, uint(30), response.Data.Tiers[0].Channels[0].EffectiveWeight)
	assert.Equal(t, 1.0, response.Data.Tiers[0].Channels[0].ExpectedShare)
	assert.Equal(t, 2, response.Data.Tiers[0].Channels[1].ID)
	assert.Equal(t, uint(0), response.Data.Tiers[0].Channels[1].EffectiveWeight)
	assert.Equal(t, 0.0, response.Data.Tiers[0].Channels[1].ExpectedShare)
	assert.False(t, response.Data.Affinity.Evaluated)
	assert.Equal(t, "before_priority_weight", response.Data.Affinity.Precedence)
	assert.Equal(t, channelRoutingPreviewAffinityNotEvaluatedCode, response.Data.Affinity.ExplanationCode)
}

func TestGetChannelRoutingPreviewMalformedSettingsDoesNotWriteChannel(t *testing.T) {
	setupChannelRoutingPreviewTest(t)
	priority := int64(10)
	weight := uint(2)
	baseURL := "https://must-remain.example"
	channel := &model.Channel{
		Id: 51, Type: constant.ChannelTypeAdvancedCustom, Key: "must-remain-key",
		Status: common.ChannelStatusEnabled, Name: "malformed", BaseURL: &baseURL,
		Models: "advanced-model", Group: "default", Priority: &priority, Weight: &weight,
		OtherSettings: `{"advanced_custom":`,
	}
	require.NoError(t, model.DB.Create(channel).Error)
	require.NoError(t, model.DB.Create(&model.Ability{
		Group: "default", Model: "advanced-model", ChannelId: channel.Id,
		Enabled: true, Priority: &priority, Weight: weight,
	}).Error)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/channel/routing-preview?group=default&model=advanced-model&request_path=/v1/responses", nil)

	GetChannelRoutingPreview(context)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			Tiers []channelRoutingPreviewTier `json:"tiers"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success, recorder.Body.String())
	assert.Empty(t, response.Data.Tiers)

	persisted, err := model.GetChannelById(channel.Id, true)
	require.NoError(t, err)
	assert.Equal(t, channel.Key, persisted.Key)
	assert.Equal(t, channel.Models, persisted.Models)
	assert.Equal(t, channel.Group, persisted.Group)
	assert.Equal(t, channel.OtherSettings, persisted.OtherSettings)
	require.NotNil(t, persisted.BaseURL)
	assert.Equal(t, baseURL, *persisted.BaseURL)
}

func TestGetChannelRoutingPreviewRejectsAutoGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/channel/routing-preview?group=auto&model=gpt-test", nil)

	GetChannelRoutingPreview(context)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	var response struct {
		Success bool   `json:"success"`
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.False(t, response.Success)
	assert.Equal(t, channelRoutingPreviewAutoGroupUnsupportedCode, response.Code)
	assert.NotEmpty(t, response.Message)
}

func TestGetChannelRoutingPreviewRejectsMissingModelWithStableCode(t *testing.T) {
	previousTranslateMessage := common.TranslateMessage
	common.TranslateMessage = func(_ *gin.Context, key string, _ ...map[string]any) string {
		return "translated:" + key
	}
	t.Cleanup(func() { common.TranslateMessage = previousTranslateMessage })

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/channel/routing-preview?group=default", nil)

	GetChannelRoutingPreview(context)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	var response struct {
		Success bool   `json:"success"`
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.False(t, response.Success)
	assert.Equal(t, channelRoutingPreviewInvalidParamsCode, response.Code)
	assert.Equal(t, "translated:"+i18n.MsgInvalidParams, response.Message)
}

func TestGetChannelRoutingPreviewDatabaseFailureReturnsStableCode(t *testing.T) {
	db := setupChannelRoutingPreviewTest(t)
	forcedError := errors.New("forced routing preview query failure")
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("test:fail_routing_preview_query", func(tx *gorm.DB) {
		if tx.Statement.Table == "abilities" {
			tx.AddError(forcedError)
		}
	}))
	t.Cleanup(func() {
		require.NoError(t, db.Callback().Query().Remove("test:fail_routing_preview_query"))
	})

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/channel/routing-preview?group=default&model=gpt-test", nil)

	GetChannelRoutingPreview(context)

	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
	var response struct {
		Success bool   `json:"success"`
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.False(t, response.Success)
	assert.Equal(t, channelRoutingPreviewDatabaseErrorCode, response.Code)
	assert.NotEmpty(t, response.Message)
}

func TestGetChannelRoutingPreviewUsesLastGoodCacheGeneration(t *testing.T) {
	setupChannelRoutingPreviewTest(t)
	common.MemoryCacheEnabled = true
	priority := int64(10)
	initialWeight := uint(2)
	channel := &model.Channel{
		Id: 61, Type: constant.ChannelTypeOpenAI, Key: "secret", Status: common.ChannelStatusEnabled,
		Name: "last-good", Group: "default", Models: "cache-model", Priority: &priority, Weight: &initialWeight,
	}
	require.NoError(t, model.DB.Create(channel).Error)
	require.NoError(t, channel.AddAbilities(nil))
	require.NoError(t, model.InitChannelCache())
	before := model.GetChannelCachePublicationState()

	updatedWeight := uint(9)
	channel.Weight = &updatedWeight
	require.NoError(t, channel.Update())
	dirty := model.GetChannelCachePublicationState()
	require.True(t, dirty.CachePending)
	require.Greater(t, dirty.DataGeneration, dirty.PublishedGeneration)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/channel/routing-preview?group=default&model=cache-model", nil)
	GetChannelRoutingPreview(context)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			Source              string                      `json:"source"`
			Generation          uint64                      `json:"generation"`
			DataGeneration      uint64                      `json:"data_generation"`
			PublishedGeneration uint64                      `json:"published_generation"`
			CachePending        bool                        `json:"cache_pending"`
			Tiers               []channelRoutingPreviewTier `json:"tiers"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success, recorder.Body.String())
	assert.Equal(t, model.ChannelRoutingSourceCache, response.Data.Source)
	assert.Equal(t, before.PublishedGeneration, response.Data.Generation)
	assert.Equal(t, dirty.DataGeneration, response.Data.DataGeneration)
	assert.Equal(t, dirty.PublishedGeneration, response.Data.PublishedGeneration)
	assert.True(t, response.Data.CachePending)
	require.Len(t, response.Data.Tiers, 1)
	require.Len(t, response.Data.Tiers[0].Channels, 1)
	assert.Equal(t, initialWeight, response.Data.Tiers[0].Channels[0].Weight)
}

func TestGetChannelRoutingPreviewDatabaseModeUsesSingleJoin(t *testing.T) {
	db := setupChannelRoutingPreviewTest(t)
	priority := int64(10)
	weight := uint(3)
	channel := &model.Channel{
		Id: 62, Type: constant.ChannelTypeOpenAI, Key: "secret", Status: common.ChannelStatusEnabled,
		Name: "normalized", Group: "default", Models: "gpt-4o-gizmo-*", Priority: &priority, Weight: &weight,
	}
	require.NoError(t, model.DB.Create(channel).Error)
	require.NoError(t, model.DB.Create(&model.Ability{
		Group: "default", Model: "gpt-4o-gizmo-*", ChannelId: channel.Id,
		Enabled: true, Priority: &priority, Weight: weight,
	}).Error)

	queryCount := 0
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("test:count_routing_preview_join", func(tx *gorm.DB) {
		if strings.Contains(tx.Statement.SQL.String(), "JOIN channels") {
			queryCount++
		}
	}))
	t.Cleanup(func() {
		require.NoError(t, db.Callback().Query().Remove("test:count_routing_preview_join"))
	})

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/channel/routing-preview?group=default&model=gpt-4o-gizmo-tenant", nil)
	GetChannelRoutingPreview(context)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			Source     string                      `json:"source"`
			Generation uint64                      `json:"generation"`
			Tiers      []channelRoutingPreviewTier `json:"tiers"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success, recorder.Body.String())
	assert.Equal(t, model.ChannelRoutingSourceDatabase, response.Data.Source)
	assert.Equal(t, 1, queryCount)
	require.Len(t, response.Data.Tiers, 1)
	assert.Equal(t, channel.Id, response.Data.Tiers[0].Channels[0].ID)
}
