package controller

import (
	"net/http"
	"net/http/httptest"
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
