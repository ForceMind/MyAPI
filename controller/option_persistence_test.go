package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestUpdateOptionReturnsFailureWhenPersistenceFails(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Option{}))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	previousDB := model.DB
	model.DB = db
	common.OptionMapRWMutex.Lock()
	previousMap := common.OptionMap
	common.OptionMap = map[string]string{"Notice": "Before"}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		model.DB = previousDB
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousMap
		common.OptionMapRWMutex.Unlock()
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:fail-option-create", func(tx *gorm.DB) {
		tx.AddError(errors.New("injected persistence failure"))
	}))
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest(http.MethodPut, "/api/option/", strings.NewReader(`{"key":"Notice","value":"After"}`))
	UpdateOption(context)
	var payload struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &payload))
	assert.False(t, payload.Success)
	assert.NotEmpty(t, payload.Message)
	common.OptionMapRWMutex.RLock()
	assert.Equal(t, "Before", common.OptionMap["Notice"])
	common.OptionMapRWMutex.RUnlock()
}

func TestUpdateOptionRatioPersistenceFailureDoesNotPrepublishRuntime(t *testing.T) {
	for _, testCase := range []struct {
		key       string
		get       func() string
		update    func(string) error
		errorText string
	}{
		{key: "ImageRatio", get: ratio_setting.ImageRatio2JSONString, update: ratio_setting.UpdateImageRatioByJSONString, errorText: "图片倍率设置失败"},
		{key: "AudioRatio", get: ratio_setting.AudioRatio2JSONString, update: ratio_setting.UpdateAudioRatioByJSONString, errorText: "音频倍率设置失败"},
		{key: "AudioCompletionRatio", get: ratio_setting.AudioCompletionRatio2JSONString, update: ratio_setting.UpdateAudioCompletionRatioByJSONString, errorText: "音频补全倍率设置失败"},
		{key: "CreateCacheRatio", get: ratio_setting.CreateCacheRatio2JSONString, update: ratio_setting.UpdateCreateCacheRatioByJSONString, errorText: "缓存创建倍率设置失败"},
	} {
		t.Run(testCase.key, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
			require.NoError(t, err)
			require.NoError(t, db.AutoMigrate(&model.Option{}))
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			previousDB := model.DB
			model.DB = db
			previousRatio := testCase.get()
			const baselineRatio = `{"baseline-model":2}`
			require.NoError(t, testCase.update(baselineRatio))
			common.OptionMapRWMutex.Lock()
			previousMap := common.OptionMap
			common.OptionMap = map[string]string{testCase.key: baselineRatio}
			common.OptionMapRWMutex.Unlock()
			t.Cleanup(func() {
				model.DB = previousDB
				require.NoError(t, testCase.update(previousRatio))
				common.OptionMapRWMutex.Lock()
				common.OptionMap = previousMap
				common.OptionMapRWMutex.Unlock()
				require.NoError(t, sqlDB.Close())
			})
			require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:fail-ratio-option-create", func(tx *gorm.DB) {
				tx.AddError(errors.New("injected ratio persistence failure"))
			}))

			response := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(response)
			context.Request = httptest.NewRequest(http.MethodPut, "/api/option/", strings.NewReader(`{"key":"`+testCase.key+`","value":"{\"next-model\":3}"}`))
			UpdateOption(context)

			var payload struct {
				Success bool   `json:"success"`
				Message string `json:"message"`
			}
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &payload))
			assert.False(t, payload.Success)
			assert.NotEmpty(t, payload.Message)
			assert.NotContains(t, payload.Message, testCase.errorText, "validation succeeded; failure must come from persistence")
			assert.JSONEq(t, baselineRatio, testCase.get())
			common.OptionMapRWMutex.RLock()
			assert.Equal(t, baselineRatio, common.OptionMap[testCase.key])
			common.OptionMapRWMutex.RUnlock()
			var persisted int64
			require.NoError(t, db.Model(&model.Option{}).Count(&persisted).Error)
			assert.Zero(t, persisted)
		})
	}
}
