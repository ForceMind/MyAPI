package controller

import (
	"encoding/json"
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
)

func setupTypedBulkControllerTest(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Option{}))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	previousDB := model.DB
	model.DB = db
	previousRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	common.OptionMapRWMutex.Lock()
	previousMap := common.OptionMap
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()
	registered := config.GlobalConfig.Get("general_setting")
	require.NotNil(t, registered)
	beforeGeneral, err := config.ConfigToMap(registered)
	require.NoError(t, err)
	t.Cleanup(func() {
		model.DB = previousDB
		common.RedisEnabled = previousRedisEnabled
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousMap
		common.OptionMapRWMutex.Unlock()
		require.NoError(t, config.UpdateConfigFromMap(registered, beforeGeneral))
		require.NoError(t, sqlDB.Close())
	})
	return db
}

func performTypedBulkRequest(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest(http.MethodPut, "/api/option/typed-bulk", strings.NewReader(body))
	UpdateOptionsTypedBulk(context)
	return response
}

type typedBulkResponse struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func decodeTypedBulkResponse(t *testing.T, response *httptest.ResponseRecorder) typedBulkResponse {
	t.Helper()
	var payload typedBulkResponse
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &payload))
	return payload
}

func TestUpdateOptionsTypedBulkHappyPath(t *testing.T) {
	db := setupTypedBulkControllerTest(t)

	response := performTypedBulkRequest(t, `{"expected_revision":0,"items":[
		{"key":"general_setting.ping_interval_enabled","type":"boolean","value":true},
		{"key":"general_setting.docs_link","type":"string","value":"https://docs.example.com"}
	]}`)
	require.Equal(t, http.StatusOK, response.Code)
	payload := decodeTypedBulkResponse(t, response)
	require.True(t, payload.Success, payload.Message)
	var data struct {
		Revision int64    `json:"revision"`
		Applied  []string `json:"applied"`
	}
	require.NoError(t, common.Unmarshal(payload.Data, &data))
	assert.Equal(t, int64(1), data.Revision)
	assert.Equal(t, []string{"general_setting.docs_link", "general_setting.ping_interval_enabled"}, data.Applied)

	var row model.Option
	require.NoError(t, db.Where("key = ?", "general_setting.docs_link").First(&row).Error)
	assert.Equal(t, "https://docs.example.com", row.Value)
	assert.Equal(t, "https://docs.example.com", operation_setting.GetGeneralSetting().DocsLink)
	assert.True(t, operation_setting.GetGeneralSetting().PingIntervalEnabled)
	common.OptionMapRWMutex.RLock()
	assert.Equal(t, "https://docs.example.com", common.OptionMap["general_setting.docs_link"])
	common.OptionMapRWMutex.RUnlock()
}

func TestUpdateOptionsTypedBulkRejectsMalformedBody(t *testing.T) {
	setupTypedBulkControllerTest(t)

	for _, testCase := range []struct {
		name string
		body string
	}{
		{name: "not json", body: `not-json`},
		{name: "unknown top level field", body: `{"items":[],"unexpected":true}`},
		{name: "trailing json value", body: `{} {}`},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			response := performTypedBulkRequest(t, testCase.body)
			assert.Equal(t, http.StatusBadRequest, response.Code)
			payload := decodeTypedBulkResponse(t, response)
			assert.False(t, payload.Success)
			assert.NotEmpty(t, payload.Message)
		})
	}
}

func TestUpdateOptionsTypedBulkRejectsUnknownDuplicateAndInvalid(t *testing.T) {
	db := setupTypedBulkControllerTest(t)
	require.NoError(t, i18n.Init())

	for _, testCase := range []struct {
		name        string
		body        string
		expectedKey string
	}{
		{name: "unknown key", body: `{"items":[{"key":"general_setting.no_such_field","type":"string","value":"x"}]}`, expectedKey: "option.typed_bulk_unknown_key"},
		{name: "excluded family", body: `{"items":[{"key":"payment_setting.amount_options","type":"string","value":"[]"}]}`, expectedKey: "option.typed_bulk_unknown_key"},
		{name: "duplicate key", body: `{"items":[{"key":"general_setting.docs_link","type":"string","value":"a"},{"key":"general_setting.docs_link","type":"string","value":"b"}]}`, expectedKey: "option.typed_bulk_duplicate_key"},
		{name: "invalid value", body: `{"items":[{"key":"general_setting.ping_interval_seconds","type":"number","value":"not-a-number"}]}`, expectedKey: "option.typed_bulk_invalid_value"},
		{name: "empty items", body: `{"items":[]}`, expectedKey: "option.typed_bulk_item_count"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			response := performTypedBulkRequest(t, testCase.body)
			assert.Equal(t, http.StatusBadRequest, response.Code)
			payload := decodeTypedBulkResponse(t, response)
			assert.False(t, payload.Success)
			// i18n is initialized: the message must be the resolved English
			// translation, not the raw message key.
			assert.NotContains(t, payload.Message, testCase.expectedKey)
			assert.NotEmpty(t, payload.Message)
		})
	}

	var persisted int64
	require.NoError(t, db.Model(&model.Option{}).Count(&persisted).Error)
	assert.Zero(t, persisted, "rejected requests must not write the DB")
	common.OptionMapRWMutex.RLock()
	assert.Empty(t, common.OptionMap, "rejected requests must not touch OptionMap")
	common.OptionMapRWMutex.RUnlock()
}

func TestUpdateOptionsTypedBulkRevisionConflictReturns409(t *testing.T) {
	db := setupTypedBulkControllerTest(t)
	require.NoError(t, i18n.Init())

	response := performTypedBulkRequest(t, `{"expected_revision":0,"items":[
		{"key":"general_setting.docs_link","type":"string","value":"https://first"}
	]}`)
	require.Equal(t, http.StatusOK, response.Code)
	require.True(t, decodeTypedBulkResponse(t, response).Success)

	// Replaying the same expected revision must conflict with zero writes.
	response = performTypedBulkRequest(t, `{"expected_revision":0,"items":[
		{"key":"general_setting.docs_link","type":"string","value":"https://second"}
	]}`)
	require.Equal(t, http.StatusConflict, response.Code)
	payload := decodeTypedBulkResponse(t, response)
	assert.False(t, payload.Success)
	var data struct {
		Revision int64 `json:"revision"`
	}
	require.NoError(t, common.Unmarshal(payload.Data, &data))
	assert.Equal(t, int64(1), data.Revision)

	var row model.Option
	require.NoError(t, db.Where("key = ?", "general_setting.docs_link").First(&row).Error)
	assert.Equal(t, "https://first", row.Value)
	assert.Equal(t, "https://first", operation_setting.GetGeneralSetting().DocsLink)
	common.OptionMapRWMutex.RLock()
	assert.Equal(t, "https://first", common.OptionMap["general_setting.docs_link"])
	common.OptionMapRWMutex.RUnlock()
}

func TestGetOptionsTypedBulkRevision(t *testing.T) {
	setupTypedBulkControllerTest(t)

	get := func() typedBulkResponse {
		response := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(response)
		context.Request = httptest.NewRequest(http.MethodGet, "/api/option/typed-bulk/revision", nil)
		GetOptionsTypedBulkRevision(context)
		require.Equal(t, http.StatusOK, response.Code)
		return decodeTypedBulkResponse(t, response)
	}
	readRevision := func(t *testing.T, payload typedBulkResponse) int64 {
		t.Helper()
		require.True(t, payload.Success)
		var data struct {
			Revision int64 `json:"revision"`
		}
		require.NoError(t, common.Unmarshal(payload.Data, &data))
		return data.Revision
	}

	assert.Zero(t, readRevision(t, get()))

	response := performTypedBulkRequest(t, `{"items":[
		{"key":"general_setting.docs_link","type":"string","value":"https://docs.example.com"}
	]}`)
	require.Equal(t, http.StatusOK, response.Code)
	require.True(t, decodeTypedBulkResponse(t, response).Success)

	assert.Equal(t, int64(1), readRevision(t, get()))
}

// TestUpdateOptionLegacyPutStillWorks pins the legacy single-key PUT behavior
// alongside the new typed bulk endpoint (C09-N3a compatibility requirement).
func TestUpdateOptionLegacyPutStillWorks(t *testing.T) {
	db := setupTypedBulkControllerTest(t)

	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest(http.MethodPut, "/api/option/", strings.NewReader(`{"key":"Notice","value":"hello"}`))
	UpdateOption(context)
	require.Equal(t, http.StatusOK, response.Code)
	var payload struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &payload))
	require.True(t, payload.Success, payload.Message)

	var row model.Option
	require.NoError(t, db.Where("key = ?", "Notice").First(&row).Error)
	assert.Equal(t, "hello", row.Value)
	common.OptionMapRWMutex.RLock()
	assert.Equal(t, "hello", common.OptionMap["Notice"])
	common.OptionMapRWMutex.RUnlock()

	// The legacy writer does not bump the typed bulk revision stream.
	revision, err := model.CurrentTypedBulkRevision()
	require.NoError(t, err)
	assert.Zero(t, revision)
}
