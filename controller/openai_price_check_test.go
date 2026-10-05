package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestOpenAIPriceCheckHTTPRejectsNonRootAndClientControls(t *testing.T) {
	require.NoError(t, i18n.Init())
	for _, role := range []int{0, common.RoleCommonUser, common.RoleAdminUser} {
		for _, handler := range []gin.HandlerFunc{GetOpenAIPriceCheckStatus, CreateOpenAIPriceCheck} {
			response := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(response)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/api/ratio_sync/openai/check", strings.NewReader(`{}`))
			ctx.Set("role", role)
			handler(ctx)
			assert.Equal(t, http.StatusForbidden, response.Code)
			assert.Equal(t, "no-store", response.Header().Get("Cache-Control"))
		}
	}
	for _, body := range []string{`{"source_url":"https://example.invalid"}`, `{"rates":{"input":0}}`, `{"enabled":true}`, `{} {}`, `null`, ``, strings.Repeat("x", 1025)} {
		response := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(response)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/api/ratio_sync/openai/check", strings.NewReader(body))
		ctx.Set("role", common.RoleRootUser)
		CreateOpenAIPriceCheck(ctx)
		assert.Equal(t, http.StatusBadRequest, response.Code)
	}
	response := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(response)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/ratio_sync/openai/check?source_url=ignored", strings.NewReader(`{}`))
	ctx.Set("role", common.RoleRootUser)
	CreateOpenAIPriceCheck(ctx)
	assert.Equal(t, http.StatusBadRequest, response.Code)
}

func TestOpenAIPriceCheckHTTPStatusIsReadOnlyAndManualCheckIsDeduplicated(t *testing.T) {
	require.NoError(t, i18n.Init())
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/price-check-http.db"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.OfficialPriceVersion{}, &model.SystemTask{}, &model.SystemTaskLock{}))
	oldDB := model.DB
	model.DB = db
	common.OptionMapRWMutex.Lock()
	oldOptions := common.OptionMap
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		model.DB = oldDB
		common.OptionMapRWMutex.Lock()
		common.OptionMap = oldOptions
		common.OptionMapRWMutex.Unlock()
		require.NoError(t, sqlDB.Close())
	})
	response := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(response)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/ratio_sync/openai/check", nil)
	ctx.Set("role", common.RoleRootUser)
	GetOpenAIPriceCheckStatus(ctx)
	require.Equal(t, http.StatusOK, response.Code)
	var status struct {
		Success bool
		Data    service.OpenAIPriceCheckStatus
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &status))
	assert.True(t, status.Success)
	assert.False(t, status.Data.Enabled)
	assert.True(t, status.Data.Stale)
	assert.EqualValues(t, 86400, status.Data.IntervalSeconds)
	assert.EqualValues(t, 259200, status.Data.StaleAfterSeconds)
	assert.Empty(t, status.Data.SourceSHA256)
	assert.NotNil(t, status.Data.Diff)
	for _, table := range []any{&model.SystemTask{}, &model.SystemTaskLock{}, &model.Option{}, &model.OfficialPriceVersion{}} {
		var count int64
		require.NoError(t, db.Model(table).Count(&count).Error)
		assert.Zero(t, count, "status reads must not create tasks, configuration or evidence")
	}
	var firstID string
	for _, expectedCreated := range []bool{true, false} {
		response = httptest.NewRecorder()
		ctx, _ = gin.CreateTestContext(response)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/api/ratio_sync/openai/check", strings.NewReader(`{}`))
		ctx.Set("role", common.RoleRootUser)
		CreateOpenAIPriceCheck(ctx)
		require.Equal(t, http.StatusOK, response.Code)
		var result struct {
			Success bool
			Data    struct {
				Task    model.SystemTaskResponse
				Created bool
			}
		}
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
		assert.True(t, result.Success)
		assert.Equal(t, expectedCreated, result.Data.Created)
		assert.Equal(t, model.SystemTaskStatusPending, result.Data.Task.Status)
		assert.Equal(t, model.SystemTaskTypeOpenAIPriceCheck, result.Data.Task.Type)
		if firstID == "" {
			firstID = result.Data.Task.TaskID
		} else {
			assert.Equal(t, firstID, result.Data.Task.TaskID)
		}
	}
	var count int64
	require.NoError(t, db.Model(&model.SystemTask{}).Count(&count).Error)
	assert.EqualValues(t, 1, count)
	require.NoError(t, db.Model(&model.OfficialPriceVersion{}).Count(&count).Error)
	assert.Zero(t, count, "enqueue must not fetch or publish synchronously")
}
