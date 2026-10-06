package router

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/controller"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/service/authz"
	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestChannelConnectionRoutesSeparateOneClickFromOriginGuardedDetailedTests(t *testing.T) {
	assertChannelRoutePermission(t, http.MethodGet, "/test/:id", authz.ChannelOperate, controller.TestChannel)
	assertChannelRoutePermission(t, http.MethodPost, "/test/:id", authz.ChannelOperate, controller.TestChannel)
	require.NoError(t, i18n.Init())

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Channel{}, &model.Log{}))
	require.NoError(t, model.EnsureLogProjectionSchemaWithDB(db))
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousRedis, previousMemory := common.RedisEnabled, common.MemoryCacheEnabled
	previousMode := gin.Mode()
	previousLimitEnabled, previousLimitCount, previousLimitDuration := common.CriticalRateLimitEnable, common.CriticalRateLimitNum, common.CriticalRateLimitDuration
	model.DB, model.LOG_DB = db, db
	common.RedisEnabled, common.MemoryCacheEnabled = false, false
	common.CriticalRateLimitEnable, common.CriticalRateLimitNum, common.CriticalRateLimitDuration = true, 3, 60
	gin.SetMode(gin.TestMode)
	auditStarted, releaseAudits := make(chan struct{}, 1), make(chan struct{})
	auditRequests := int64(0)
	require.NoError(t, db.Callback().Create().Before("gorm:begin_transaction").Register("fixture:block_admin_audits", func(tx *gorm.DB) {
		if tx.Statement.Table != "logs" {
			return
		}
		select {
		case auditStarted <- struct{}{}:
		default:
		}
		<-releaseAudits
	}))
	t.Cleanup(func() {
		defer func() {
			model.DB, model.LOG_DB = previousDB, previousLogDB
			common.RedisEnabled, common.MemoryCacheEnabled = previousRedis, previousMemory
			gin.SetMode(previousMode)
			common.CriticalRateLimitEnable, common.CriticalRateLimitNum, common.CriticalRateLimitDuration = previousLimitEnabled, previousLimitCount, previousLimitDuration
			require.NoError(t, sqlDB.Close())
		}()
		close(releaseAudits)
		// Admin POSTs enqueue audit writes. Finish those while this fixture still
		// owns the process-wide database, before restoring or closing it.
		require.Eventually(t, func() bool { return gopool.WorkerCount() == 0 }, 5*time.Second, time.Millisecond)
		var auditCount int64
		require.NoError(t, db.Model(&model.Log{}).Where("type = ?", model.LogTypeManage).Count(&auditCount).Error)
		require.Equal(t, auditRequests, auditCount, "queued audits must finish in their own fixture before teardown")
	})

	pat := "channel-test-route-pat"
	user := model.User{Username: "channel-test-root", AffCode: "channel-test-aff", Role: common.RoleRootUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, AccessToken: &pat}
	require.NoError(t, db.Create(&user).Error)
	channel := model.Channel{Type: constant.ChannelTypeCodex, Name: "codex", Key: "private-key", Models: "gpt-5-codex"}
	require.NoError(t, db.Create(&channel).Error)
	previousOptions := model.ChannelTestOptions{Model: "gpt-5-codex", EndpointType: "openai-response", Stream: true, ChannelType: channel.Type}
	require.NoError(t, model.SaveLastSuccessfulChannelTestOptions(channel.Id, previousOptions))

	engine := gin.New()
	registerChannelRoutes(engine.Group("/api"))
	missingRequest := httptest.NewRequest(http.MethodGet, "http://myapi.local/api/channel/test/999999", nil)
	missingRequest.Header.Set("Authorization", "Bearer "+pat)
	missingResponse := httptest.NewRecorder()
	engine.ServeHTTP(missingResponse, missingRequest)
	assert.Contains(t, missingResponse.Header().Get("Cache-Control"), "no-store")

	request := func(origin, body string) *httptest.ResponseRecorder {
		auditRequests++
		req := httptest.NewRequest(http.MethodPost, "http://myapi.local/api/channel/test/"+strconv.Itoa(channel.Id), strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+pat)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", origin)
		req.RemoteAddr = "203.0.113.91:43123"
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, req)
		return response
	}

	blocked := request("https://attacker.example", `{"model":"gpt-5-codex","stream":true}`)
	assert.Equal(t, http.StatusForbidden, blocked.Code)
	assert.Contains(t, blocked.Header().Get("Cache-Control"), "no-store")

	invalid := request("http://myapi.local", `{"model":"gpt-5-codex","stream":true,"unexpected":1}`)
	assert.Equal(t, http.StatusOK, invalid.Code)
	assert.Contains(t, invalid.Header().Get("Cache-Control"), "no-store")
	assert.Contains(t, invalid.Body.String(), `"success":false`)
	saved, err := model.GetLastSuccessfulChannelTestOptions(channel.Id)
	require.NoError(t, err)
	assert.Equal(t, &previousOptions, saved)
	var limited *httptest.ResponseRecorder
	for range common.CriticalRateLimitNum + 1 {
		limited = request("http://myapi.local", `{"unexpected":1}`)
	}
	require.NotNil(t, limited)
	assert.Equal(t, http.StatusTooManyRequests, limited.Code)
	assert.Contains(t, limited.Header().Get("Cache-Control"), "no-store")
	// Hold an actual audit until cleanup, so the test covers worker lifetime as
	// well as the endpoint's origin and rate-limit checks without timing sleeps.
	select {
	case <-auditStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("admin request did not enqueue an audit")
	}
}
