package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
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

// setupChannelRoutingTest points the model layer at an isolated in-memory
// database and enables the memory cache, because channel routing only reads
// group2model2channels when the memory cache is on.
func setupChannelRoutingTest(t *testing.T) {
	t.Helper()

	previousMemoryCache := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true
	t.Cleanup(func() { common.MemoryCacheEnabled = previousMemoryCache })

	// common.RedisEnabled defaults to true and is only lowered by the runtime
	// Redis init, which a test binary never performs; leaving it set would send
	// the audit write on this path into a nil client.
	previousRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = previousRedis })

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.Log{}, &model.User{}))

	previousDB, previousLogDB := model.DB, model.LOG_DB
	model.DB, model.LOG_DB = db, db
	t.Cleanup(func() { model.DB, model.LOG_DB = previousDB, previousLogDB })

	model.InitChannelCache()
}

func addChannelViaHandler(t *testing.T, body string) {
	t.Helper()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/channel/", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	AddChannel(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"success":true`)
}

func TestAddChannelMakesNewChannelImmediatelyRoutable(t *testing.T) {
	setupChannelRoutingTest(t)

	selected, err := model.GetRandomSatisfiedChannel("default", "gpt-4o-mini", 0, "")
	require.NoError(t, err)
	require.Nil(t, selected, "model must not be routable before its channel exists")

	addChannelViaHandler(t, `{"mode":"single","channel":{"name":"fresh","type":1,"key":"sk-fresh","group":"default","models":"gpt-4o-mini","status":1}}`)

	selected, err = model.GetRandomSatisfiedChannel("default", "gpt-4o-mini", 0, "")
	require.NoError(t, err)
	require.NotNil(t, selected, "a newly created channel must serve traffic without waiting for the periodic cache sync")
	assert.Equal(t, "fresh", selected.Name)
}

func TestAddChannelAppliesPriorityWithoutWaitingForCacheSync(t *testing.T) {
	setupChannelRoutingTest(t)

	addChannelViaHandler(t, `{"mode":"single","channel":{"name":"backup","type":1,"key":"sk-backup","group":"default","models":"gpt-4o-mini","status":1,"priority":1}}`)
	addChannelViaHandler(t, `{"mode":"single","channel":{"name":"preferred","type":1,"key":"sk-preferred","group":"default","models":"gpt-4o-mini","status":1,"priority":100}}`)

	for i := 0; i < 20; i++ {
		selected, err := model.GetRandomSatisfiedChannel("default", "gpt-4o-mini", 0, "")
		require.NoError(t, err)
		require.NotNil(t, selected)
		require.Equal(t, "preferred", selected.Name, "the highest-priority channel must win every selection at retry 0")
	}
}

func TestRepeatedStatusUpdateRepairsCommittedCachePending(t *testing.T) {
	setupChannelRoutingTest(t)
	addChannelViaHandler(t, `{"mode":"single","channel":{"name":"repairable","type":1,"key":"sk-repair","group":"default","models":"repair-model","status":1}}`)

	var channel model.Channel
	require.NoError(t, model.DB.Where("name = ?", "repairable").First(&channel).Error)
	refreshError := errors.New("forced channel cache publish failure")
	require.NoError(t, model.DB.Callback().Query().After("gorm:query").Register("test:fail_status_cache_publish", func(tx *gorm.DB) {
		if strings.Contains(tx.Statement.SQL.String(), "JOIN abilities") {
			tx.AddError(refreshError)
		}
	}))
	callbackRegistered := true
	t.Cleanup(func() {
		if callbackRegistered {
			require.NoError(t, model.DB.Callback().Query().Remove("test:fail_status_cache_publish"))
		}
	})

	invokeStatusUpdate := func() *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(channel.Id)}}
		ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/status", strings.NewReader(`{"status":2}`))
		ctx.Request.Header.Set("Content-Type", "application/json")
		UpdateChannelStatus(ctx)
		return recorder
	}

	first := invokeStatusUpdate()
	require.Equal(t, http.StatusOK, first.Code)
	var firstResponse struct {
		Success             bool   `json:"success"`
		Committed           bool   `json:"committed"`
		CachePending        bool   `json:"cache_pending"`
		Code                string `json:"code"`
		DataGeneration      uint64 `json:"data_generation"`
		PublishedGeneration uint64 `json:"published_generation"`
	}
	require.NoError(t, common.Unmarshal(first.Body.Bytes(), &firstResponse))
	assert.True(t, firstResponse.Success)
	assert.True(t, firstResponse.Committed)
	assert.True(t, firstResponse.CachePending)
	assert.Equal(t, channelCachePublishPendingCode, firstResponse.Code)
	assert.Greater(t, firstResponse.DataGeneration, firstResponse.PublishedGeneration)

	var stored model.Channel
	require.NoError(t, model.DB.First(&stored, channel.Id).Error)
	assert.Equal(t, common.ChannelStatusManuallyDisabled, stored.Status)
	selected, err := model.GetRandomSatisfiedChannel("default", "repair-model", 0, "")
	require.NoError(t, err)
	require.NotNil(t, selected, "the last-good snapshot remains active while publication is pending")

	require.NoError(t, model.DB.Callback().Query().Remove("test:fail_status_cache_publish"))
	callbackRegistered = false
	second := invokeStatusUpdate()
	require.Equal(t, http.StatusOK, second.Code)
	var secondResponse struct {
		Success             bool   `json:"success"`
		Committed           bool   `json:"committed"`
		CachePending        bool   `json:"cache_pending"`
		Changed             bool   `json:"data"`
		DataGeneration      uint64 `json:"data_generation"`
		PublishedGeneration uint64 `json:"published_generation"`
	}
	require.NoError(t, common.Unmarshal(second.Body.Bytes(), &secondResponse))
	assert.True(t, secondResponse.Success)
	assert.True(t, secondResponse.Committed)
	assert.False(t, secondResponse.CachePending)
	assert.False(t, secondResponse.Changed)
	assert.Equal(t, secondResponse.DataGeneration, secondResponse.PublishedGeneration)

	selected, err = model.GetRandomSatisfiedChannel("default", "repair-model", 0, "")
	require.NoError(t, err)
	assert.Nil(t, selected)
}

func TestAddChannelReportsCommittedCachePendingWithoutRetrySignal(t *testing.T) {
	setupChannelRoutingTest(t)
	refreshError := errors.New("forced create cache publish failure")
	require.NoError(t, model.DB.Callback().Query().After("gorm:query").Register("test:fail_create_cache_publish", func(tx *gorm.DB) {
		if strings.Contains(tx.Statement.SQL.String(), "JOIN abilities") {
			tx.AddError(refreshError)
		}
	}))
	callbackRegistered := true
	t.Cleanup(func() {
		if callbackRegistered {
			require.NoError(t, model.DB.Callback().Query().Remove("test:fail_create_cache_publish"))
		}
		require.NoError(t, model.InitChannelCache())
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/", strings.NewReader(`{"mode":"single","channel":{"name":"committed-create","type":1,"key":"sk-create","group":"default","models":"create-model","status":1}}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	AddChannel(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success      bool   `json:"success"`
		Committed    bool   `json:"committed"`
		CachePending bool   `json:"cache_pending"`
		Code         string `json:"code"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.True(t, response.Success)
	assert.True(t, response.Committed)
	assert.True(t, response.CachePending)
	assert.Equal(t, channelCachePublishPendingCode, response.Code)

	var count int64
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("name = ?", "committed-create").Count(&count).Error)
	assert.Equal(t, int64(1), count)

	require.NoError(t, model.DB.Callback().Query().Remove("test:fail_create_cache_publish"))
	callbackRegistered = false
}

func TestTagModeIgnoresPriorityAndWeightSortRequests(t *testing.T) {
	setupChannelRoutingTest(t)
	tag := "aggregate"
	highPriority := int64(100)
	lowPriority := int64(1)
	lowWeight := uint(1)
	highWeight := uint(100)
	channels := []*model.Channel{
		{Name: "priority-first", Type: 1, Key: "a", Group: "default", Models: "m", Status: common.ChannelStatusEnabled, Tag: &tag, Priority: &highPriority, Weight: &lowWeight},
		{Name: "weight-first", Type: 1, Key: "b", Group: "default", Models: "m", Status: common.ChannelStatusEnabled, Tag: &tag, Priority: &lowPriority, Weight: &highWeight},
	}
	for _, channel := range channels {
		require.NoError(t, model.DB.Create(channel).Error)
	}

	for _, sortBy := range []string{"priority", "weight"} {
		t.Run(sortBy, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodGet, "/api/channel/?tag_mode=true&sort_by="+sortBy+"&sort_order=asc", nil)
			GetAllChannels(ctx)

			require.Equal(t, http.StatusOK, recorder.Code)
			var response struct {
				Success bool `json:"success"`
				Data    struct {
					Items []model.Channel `json:"items"`
				} `json:"data"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			require.True(t, response.Success, recorder.Body.String())
			require.Len(t, response.Data.Items, 2)
			assert.Equal(t, "priority-first", response.Data.Items[0].Name)
			assert.Equal(t, "weight-first", response.Data.Items[1].Name)
		})
	}
}

func TestAddChannelIdempotencyReplaysBatchIDsAfterResponseLoss(t *testing.T) {
	setupChannelRoutingTest(t)
	body := `{"mode":"batch","channel":{"name":"idempotent","type":1,"key":"sk-one\nsk-two","group":"default","models":"idempotent-model","status":1}}`
	invoke := func(body string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/", strings.NewReader(body))
		ctx.Request.Header.Set("Content-Type", "application/json")
		ctx.Request.Header.Set("Idempotency-Key", "create-response-loss-key")
		AddChannel(ctx)
		return recorder
	}

	first := invoke(body)
	require.Equal(t, http.StatusOK, first.Code)
	var firstResponse struct {
		Success bool `json:"success"`
		Data    struct {
			IDs      []int `json:"ids"`
			Replayed bool  `json:"replayed"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(first.Body.Bytes(), &firstResponse))
	require.True(t, firstResponse.Success, first.Body.String())
	assert.False(t, firstResponse.Data.Replayed)
	require.Len(t, firstResponse.Data.IDs, 2)

	second := invoke(body)
	require.Equal(t, http.StatusOK, second.Code)
	var secondResponse struct {
		Success bool `json:"success"`
		Data    struct {
			IDs      []int `json:"ids"`
			Replayed bool  `json:"replayed"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(second.Body.Bytes(), &secondResponse))
	assert.True(t, secondResponse.Success)
	assert.True(t, secondResponse.Data.Replayed)
	assert.Equal(t, firstResponse.Data.IDs, secondResponse.Data.IDs)

	var count int64
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("name = ?", "idempotent").Count(&count).Error)
	assert.Equal(t, int64(2), count)

	conflict := invoke(`{"mode":"single","channel":{"name":"different","type":1,"key":"sk-other","group":"default","models":"idempotent-model","status":1}}`)
	assert.Equal(t, http.StatusConflict, conflict.Code)
	assert.Contains(t, conflict.Body.String(), channelOperationConflictCode)
}
