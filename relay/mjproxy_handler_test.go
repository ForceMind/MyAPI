package relay

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/dto"
	"github.com/ForceMind/MyAPI/model"
	relayconstant "github.com/ForceMind/MyAPI/relay/constant"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestCoverMidjourneyTaskDtoPreservesPersistedJSONValues(t *testing.T) {
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	origin := &model.Midjourney{
		MjId:       "task",
		Buttons:    `[{"customId":0,"emoji":false,"label":"","type":0,"style":null}]`,
		VideoUrls:  `[{"url":""}]`,
		Properties: `{"finalPrompt":"","finalZhPrompt":""}`,
	}

	got := coverMidjourneyTaskDto(context, origin)
	buttons, ok := got.Buttons.([]dto.ActionButton)
	require.True(t, ok)
	require.Len(t, buttons, 1)
	assert.Equal(t, float64(0), buttons[0].CustomId)
	assert.Equal(t, false, buttons[0].Emoji)
	assert.Equal(t, "", buttons[0].Label)
	assert.Equal(t, float64(0), buttons[0].Type)
	assert.Nil(t, buttons[0].Style)
	require.Len(t, got.VideoUrls, 1)
	assert.Equal(t, "", got.VideoUrls[0].Url)
	require.NotNil(t, got.Properties)
	assert.Equal(t, "", got.Properties.FinalPrompt)
	assert.Equal(t, "", got.Properties.FinalZhPrompt)

	empty := coverMidjourneyTaskDto(context, &model.Midjourney{
		Buttons:    `[]`,
		VideoUrls:  `[]`,
		Properties: `null`,
	})
	emptyButtons, ok := empty.Buttons.([]dto.ActionButton)
	require.True(t, ok)
	assert.NotNil(t, emptyButtons)
	assert.Empty(t, emptyButtons)
	assert.NotNil(t, empty.VideoUrls)
	assert.Empty(t, empty.VideoUrls)
	require.NotNil(t, empty.Properties, "JSON null historically produces a non-nil zero-value Properties")
	assert.Equal(t, dto.Properties{}, *empty.Properties)
}

func TestCoverMidjourneyTaskDtoMalformedJSONIsIgnoredIndependently(t *testing.T) {
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	for _, testCase := range []struct {
		name       string
		buttons    string
		videoURLs  string
		properties string
	}{
		{name: "buttons", buttons: `{`, videoURLs: `[{"url":"v"}]`, properties: `{"finalPrompt":"p"}`},
		{name: "video urls", buttons: `[]`, videoURLs: `[`, properties: `{"finalPrompt":"p"}`},
		{name: "properties", buttons: `[]`, videoURLs: `[{"url":"v"}]`, properties: `{`},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			got := coverMidjourneyTaskDto(context, &model.Midjourney{
				Buttons: testCase.buttons, VideoUrls: testCase.videoURLs, Properties: testCase.properties,
			})
			switch testCase.name {
			case "buttons":
				assert.Nil(t, got.Buttons)
				require.Len(t, got.VideoUrls, 1)
				require.NotNil(t, got.Properties)
			case "video urls":
				require.NotNil(t, got.Buttons)
				assert.Nil(t, got.VideoUrls)
				require.NotNil(t, got.Properties)
			case "properties":
				require.NotNil(t, got.Buttons)
				require.Len(t, got.VideoUrls, 1)
				assert.Nil(t, got.Properties)
			}
		})
	}
}

func TestRelayMidjourneyNotifyPersistsEmptyVideoURLs(t *testing.T) {
	db := setupMidjourneyTestDB(t)
	require.NoError(t, db.Create(&model.Midjourney{UserId: 7, MjId: "notify-task", VideoUrls: `[{"url":"old"}]`}).Error)

	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodPost, "/mj/notify", strings.NewReader(`{"id":"notify-task","videoUrls":[]}`))
	context.Request.Header.Set("Content-Type", "application/json")
	t.Cleanup(func() { common.CleanupBodyStorage(context) })

	response := RelayMidjourneyNotify(context)
	require.Nil(t, response)
	var stored model.Midjourney
	require.NoError(t, db.Where("mj_id = ?", "notify-task").First(&stored).Error)
	assert.Equal(t, `[]`, stored.VideoUrls)
}

func TestRelayMidjourneyTaskResponseShapes(t *testing.T) {
	db := setupMidjourneyTestDB(t)
	task := model.Midjourney{
		UserId: 9, MjId: "shape-task", PromptEn: "translated", SubmitTime: 123,
		Buttons: `[]`, VideoUrls: `[]`, Properties: `{"finalPrompt":"","finalZhPrompt":""}`,
	}
	require.NoError(t, db.Create(&task).Error)

	t.Run("single task is an object with camelCase fields", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(recorder)
		context.Set("id", 9)
		context.AddParam("id", "shape-task")

		response := RelayMidjourneyTask(context, relayconstant.RelayModeMidjourneyTaskFetch)
		require.Nil(t, response)
		assert.Equal(t, "application/json", recorder.Header().Get("Content-Type"))
		var body map[string]any
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
		assert.Equal(t, "shape-task", body["id"])
		assert.Equal(t, "translated", body["promptEn"])
		assert.Equal(t, float64(123), body["submitTime"])
		assert.Equal(t, []any{}, body["videoUrls"])
		assert.Equal(t, []any{}, body["buttons"])
		assert.Contains(t, body, "properties")
		assert.NotContains(t, body, "prompt_en")
		assert.NotContains(t, body, "submit_time")
	})

	t.Run("condition task is an array", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(recorder)
		context.Set("id", 9)
		context.Request = httptest.NewRequest(http.MethodPost, "/mj/task", strings.NewReader(`{"ids":["shape-task"]}`))
		context.Request.Header.Set("Content-Type", "application/json")

		response := RelayMidjourneyTask(context, relayconstant.RelayModeMidjourneyTaskFetchByCondition)
		require.Nil(t, response)
		assert.Equal(t, "application/json", recorder.Header().Get("Content-Type"))
		var body []map[string]any
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
		require.Len(t, body, 1)
		assert.Equal(t, "shape-task", body[0]["id"])
	})

	t.Run("empty condition is an empty array not null", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(recorder)
		context.Set("id", 9)
		context.Request = httptest.NewRequest(http.MethodPost, "/mj/task", strings.NewReader(`{"ids":[]}`))
		context.Request.Header.Set("Content-Type", "application/json")

		response := RelayMidjourneyTask(context, relayconstant.RelayModeMidjourneyTaskFetchByCondition)
		require.Nil(t, response)
		assert.Equal(t, "application/json", recorder.Header().Get("Content-Type"))
		assert.Equal(t, `[]`, recorder.Body.String())
	})
}

func setupMidjourneyTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	originalDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.Midjourney{}))
	model.DB = db
	t.Cleanup(func() {
		model.DB = originalDB
		require.NoError(t, sqlDB.Close())
	})
	return db
}
