package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/ForceMind/MyAPI/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupChannelRoutingControllerTest(t *testing.T) *gorm.DB {
	t.Helper()

	gin.SetMode(gin.TestMode)
	require.NoError(t, i18n.Init())
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	previousMemoryCache := common.MemoryCacheEnabled
	previousRedis := common.RedisEnabled
	previousPolicy := operation_setting.GetChannelRoutingPolicy()
	previousPolicyValue, err := operation_setting.MarshalChannelRoutingPolicy(previousPolicy)
	require.NoError(t, err)
	common.OptionMapRWMutex.RLock()
	previousOptionMap := make(map[string]string, len(common.OptionMap))
	for key, value := range common.OptionMap {
		previousOptionMap[key] = value
	}
	common.OptionMapRWMutex.RUnlock()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.MemoryCacheEnabled = true
	common.RedisEnabled = false
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.Channel{}, &model.Ability{}, &model.ChannelQuotaSnapshot{}, &model.User{}, &model.Log{}))

	t.Cleanup(func() {
		routingConfig := config.GlobalConfig.Get("routing_policy_setting")
		require.NoError(t, config.UpdateConfigFromMap(routingConfig, map[string]string{"policy": previousPolicyValue}))
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptionMap
		common.OptionMapRWMutex.Unlock()
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
		common.MemoryCacheEnabled = previousMemoryCache
		common.RedisEnabled = previousRedis
		require.NoError(t, sqlDB.Close())
	})
	return db
}

func callChannelRoutingHandler(t *testing.T, handler gin.HandlerFunc, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	context.Request.Header.Set("Content-Type", "application/json")
	if marker := "/channels/"; strings.Contains(target, marker) {
		context.Params = gin.Params{{Key: "id", Value: strings.TrimPrefix(target[strings.LastIndex(target, marker):], marker)}}
	}
	handler(context)
	return recorder
}

func TestChannelRoutingPolicyRequiresCompleteValidPolicy(t *testing.T) {
	db := setupChannelRoutingControllerTest(t)
	before := operation_setting.GetChannelRoutingPolicy()
	valid := map[string]any{
		"enabled":               true,
		"sticky_enabled":        false,
		"session_ttl_seconds":   7200,
		"quota_max_age_seconds": 120,
	}

	for field := range valid {
		request := make(map[string]any, len(valid)-1)
		for key, value := range valid {
			if key != field {
				request[key] = value
			}
		}
		body, err := common.Marshal(request)
		require.NoError(t, err)
		response := callChannelRoutingHandler(t, UpdateChannelRoutingPolicy, "/api/channel/routing", string(body))
		assert.Equal(t, http.StatusBadRequest, response.Code, field)
		assert.Equal(t, before, operation_setting.GetChannelRoutingPolicy(), field)
	}

	for _, body := range []string{
		`{"enabled":true,"sticky_enabled":true,"session_ttl_seconds":3599,"quota_max_age_seconds":120}`,
		`{"enabled":true,"sticky_enabled":true,"session_ttl_seconds":7200,"quota_max_age_seconds":86401}`,
	} {
		response := callChannelRoutingHandler(t, UpdateChannelRoutingPolicy, "/api/channel/routing", body)
		assert.Equal(t, http.StatusBadRequest, response.Code)
		assert.Equal(t, before, operation_setting.GetChannelRoutingPolicy())
	}
	var persisted int64
	require.NoError(t, db.Model(&model.Option{}).Count(&persisted).Error)
	assert.Zero(t, persisted, "rejected policy requests must not be persisted")
}

func TestChannelRoutingPolicySavesAllFields(t *testing.T) {
	db := setupChannelRoutingControllerTest(t)
	response := callChannelRoutingHandler(t, UpdateChannelRoutingPolicy, "/api/channel/routing",
		`{"enabled":true,"sticky_enabled":false,"session_ttl_seconds":7200,"quota_max_age_seconds":120}`)
	require.Equal(t, http.StatusOK, response.Code)
	var payload struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &payload))
	require.Truef(t, payload.Success, "preview response: %s", response.Body.String())

	want := operation_setting.ChannelRoutingPolicy{Enabled: true, StickyEnabled: false, SessionTTLSeconds: 7200, QuotaMaxAgeSeconds: 120}
	assert.Equal(t, want, operation_setting.GetChannelRoutingPolicy())
	var option model.Option
	require.NoError(t, db.First(&option, "key = ?", operation_setting.ChannelRoutingPolicyOptionKey).Error)
	var persisted operation_setting.ChannelRoutingPolicy
	require.NoError(t, common.Unmarshal([]byte(option.Value), &persisted))
	assert.Equal(t, want, persisted)
}

func TestChannelRoutingPreviewIsReadOnlyAndReturnsCandidateContract(t *testing.T) {
	db := setupChannelRoutingControllerTest(t)
	priority, weight := int64(10), uint(3)
	channel := model.Channel{
		Id:       8101,
		Type:     1,
		Key:      "routing-preview-secret",
		Status:   common.ChannelStatusEnabled,
		Name:     "routing-preview",
		Models:   "routing-preview-model",
		Group:    "routing-preview-group",
		Priority: &priority,
		Weight:   &weight,
	}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, channel.UpdateAbilities(nil))
	model.InitChannelCache()
	var beforeChannels, beforeAbilities, beforeSnapshots int64
	require.NoError(t, db.Model(&model.Channel{}).Count(&beforeChannels).Error)
	require.NoError(t, db.Model(&model.Ability{}).Count(&beforeAbilities).Error)
	require.NoError(t, db.Model(&model.ChannelQuotaSnapshot{}).Count(&beforeSnapshots).Error)

	response := callChannelRoutingHandler(t, PreviewChannelRouting, "/api/channel/routing/preview",
		`{"model":" routing-preview-model ","group":" routing-preview-group ","path":" /v1/chat/completions "}`)
	require.Equal(t, http.StatusOK, response.Code)
	var payload struct {
		Success bool `json:"success"`
		Data    struct {
			Workload   string `json:"workload"`
			Reason     string `json:"reason"`
			Candidates []struct {
				ID     int     `json:"id"`
				Name   string  `json:"name"`
				Share  float64 `json:"share"`
				Reason string  `json:"reason"`
			} `json:"candidates"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &payload))
	require.Truef(t, payload.Success, "preview response: %s", response.Body.String())
	assert.Equal(t, "chat", payload.Data.Workload)
	require.Len(t, payload.Data.Candidates, 1)
	assert.Equal(t, channel.Id, payload.Data.Candidates[0].ID)
	assert.Equal(t, channel.Name, payload.Data.Candidates[0].Name)
	assert.InDelta(t, 1.0, payload.Data.Candidates[0].Share, 0.000001)
	assert.NotEmpty(t, payload.Data.Reason)
	assert.NotEmpty(t, payload.Data.Candidates[0].Reason)
	assert.NotContains(t, response.Body.String(), channel.Key)

	var afterChannels, afterAbilities, afterSnapshots int64
	require.NoError(t, db.Model(&model.Channel{}).Count(&afterChannels).Error)
	require.NoError(t, db.Model(&model.Ability{}).Count(&afterAbilities).Error)
	require.NoError(t, db.Model(&model.ChannelQuotaSnapshot{}).Count(&afterSnapshots).Error)
	assert.Equal(t, beforeChannels, afterChannels)
	assert.Equal(t, beforeAbilities, afterAbilities)
	assert.Equal(t, beforeSnapshots, afterSnapshots)
}

func TestChannelRoutingValuesRequireCASAndSynchronizeAbilities(t *testing.T) {
	db := setupChannelRoutingControllerTest(t)
	priority, weight := int64(10), uint(2)
	channel := model.Channel{
		Id:       8102,
		Type:     1,
		Key:      "routing-update-secret",
		Status:   common.ChannelStatusEnabled,
		Name:     "routing-update",
		Models:   "routing-update-model",
		Group:    "routing-update-group",
		Priority: &priority,
		Weight:   &weight,
	}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, channel.UpdateAbilities(nil))

	valid := map[string]any{"priority": int64(12), "weight": int64(7), "expected_priority": int64(10), "expected_weight": int64(2)}
	for field := range valid {
		request := make(map[string]any, len(valid)-1)
		for key, value := range valid {
			if key != field {
				request[key] = value
			}
		}
		body, err := common.Marshal(request)
		require.NoError(t, err)
		response := callChannelRoutingHandler(t, UpdateChannelRoutingValues, fmt.Sprintf("/api/channel/routing/channels/%d", channel.Id), string(body))
		assert.Equal(t, http.StatusBadRequest, response.Code, field)
	}
	for _, body := range []string{
		`{"priority":1000001,"weight":7,"expected_priority":10,"expected_weight":2}`,
		`{"priority":12,"weight":-1,"expected_priority":10,"expected_weight":2}`,
		`{"priority":12,"weight":1000001,"expected_priority":10,"expected_weight":2}`,
	} {
		response := callChannelRoutingHandler(t, UpdateChannelRoutingValues, fmt.Sprintf("/api/channel/routing/channels/%d", channel.Id), body)
		assert.Equal(t, http.StatusBadRequest, response.Code)
	}

	conflict := callChannelRoutingHandler(t, UpdateChannelRoutingValues, fmt.Sprintf("/api/channel/routing/channels/%d", channel.Id),
		`{"priority":12,"weight":7,"expected_priority":9,"expected_weight":2}`)
	assert.Equal(t, http.StatusConflict, conflict.Code)

	success := callChannelRoutingHandler(t, UpdateChannelRoutingValues, fmt.Sprintf("/api/channel/routing/channels/%d", channel.Id),
		`{"priority":12,"weight":7,"expected_priority":10,"expected_weight":2}`)
	require.Equal(t, http.StatusOK, success.Code)
	var payload struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(success.Body.Bytes(), &payload))
	assert.True(t, payload.Success)

	var updated model.Channel
	require.NoError(t, db.First(&updated, channel.Id).Error)
	assert.Equal(t, int64(12), updated.GetPriority())
	assert.Equal(t, 7, updated.GetWeight())
	assert.Equal(t, channel.Name, updated.Name)
	assert.Equal(t, channel.Key, updated.Key)
	var ability model.Ability
	require.NoError(t, db.Where("channel_id = ?", channel.Id).First(&ability).Error)
	require.NotNil(t, ability.Priority)
	assert.Equal(t, int64(12), *ability.Priority)
	assert.Equal(t, uint(7), ability.Weight)
}

func TestChannelRoutingValuesLegacyTokensAreExactCAS(t *testing.T) {
	db := setupChannelRoutingControllerTest(t)
	legacyPriority := model.MaxChannelRoutingPriority + 42
	legacyWeight := model.MaxChannelRoutingWeight + 42
	channel := model.Channel{
		Type: 1, Key: "legacy-routing-secret", Status: common.ChannelStatusEnabled, Name: "legacy-routing",
		Models: "legacy-routing-model", Group: "legacy-routing-group",
		Priority: &legacyPriority, Weight: &legacyWeight,
	}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, channel.UpdateAbilities(nil))
	target := fmt.Sprintf("/api/channel/routing/channels/%d", channel.Id)

	for _, body := range []string{
		`{"priority":12,"weight":7,"expected_priority":1000000,"expected_weight":1000000,"expected_legacy_priority":"not-an-integer","expected_legacy_weight":"1000042"}`,
		`{"priority":12,"weight":7,"expected_priority":1000000,"expected_weight":1000000,"expected_legacy_priority":"1000042","expected_legacy_weight":"01000042"}`,
	} {
		response := callChannelRoutingHandler(t, UpdateChannelRoutingValues, target, body)
		assert.Equal(t, http.StatusBadRequest, response.Code)
	}

	missingBoth := callChannelRoutingHandler(t, UpdateChannelRoutingValues, target,
		`{"priority":12,"weight":7,"expected_priority":1000000,"expected_weight":1000000}`)
	assert.Equal(t, http.StatusConflict, missingBoth.Code)
	missingWeight := callChannelRoutingHandler(t, UpdateChannelRoutingValues, target,
		`{"priority":12,"weight":7,"expected_priority":1000000,"expected_weight":1000000,"expected_legacy_priority":"1000042"}`)
	assert.Equal(t, http.StatusConflict, missingWeight.Code)

	concurrentPriority := legacyPriority + 1
	concurrentWeight := legacyWeight + 1
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", channel.Id).Updates(map[string]interface{}{
		"priority": concurrentPriority, "weight": concurrentWeight,
	}).Error)
	stalePriority := callChannelRoutingHandler(t, UpdateChannelRoutingValues, target,
		`{"priority":12,"weight":7,"expected_priority":1000000,"expected_weight":1000000,"expected_legacy_priority":"1000042","expected_legacy_weight":"1000043"}`)
	assert.Equal(t, http.StatusConflict, stalePriority.Code)
	staleWeight := callChannelRoutingHandler(t, UpdateChannelRoutingValues, target,
		`{"priority":12,"weight":7,"expected_priority":1000000,"expected_weight":1000000,"expected_legacy_priority":"1000043","expected_legacy_weight":"1000042"}`)
	assert.Equal(t, http.StatusConflict, staleWeight.Code)
	missingPriority := callChannelRoutingHandler(t, UpdateChannelRoutingValues, target,
		`{"priority":12,"weight":7,"expected_priority":1000000,"expected_weight":1000000,"expected_legacy_weight":"1000043"}`)
	assert.Equal(t, http.StatusConflict, missingPriority.Code)

	current := callChannelRoutingHandler(t, UpdateChannelRoutingValues, target,
		`{"priority":12,"weight":7,"expected_priority":1000000,"expected_weight":1000000,"expected_legacy_priority":"1000043","expected_legacy_weight":"1000043"}`)
	require.Equal(t, http.StatusOK, current.Code)
	var stored model.Channel
	require.NoError(t, db.First(&stored, channel.Id).Error)
	assert.Equal(t, int64(12), stored.GetPriority())
	assert.Equal(t, 7, stored.GetWeight())
}
