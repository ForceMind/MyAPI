package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupPromptLearningControllerTest(t *testing.T) {
	t.Helper()
	require.NoError(t, i18n.Init())
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	previousDB := model.DB
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.AutoMigrate(&model.PromptLearningPolicy{}, &model.PromptLearningSample{}, &model.PromptLearningRun{}, &model.PromptInstructionVersion{}, &model.PromptInstructionApplication{}))
}

func promptLearningSessionContext(method, path, body string) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	context.Request.Header.Set("Content-Type", "application/json")
	context.Set("id", 17)
	context.Set("session_id", "prompt-learning-session")
	context.Set("auth_version", int64(1))
	context.Set("session_version", int64(1))
	return context, recorder
}

func TestPromptLearningControllersManageOnlySelfScopeWithDashboardSession(t *testing.T) {
	setupPromptLearningControllerTest(t)
	gin.SetMode(gin.TestMode)

	context, recorder := promptLearningSessionContext(http.MethodGet, "/api/user/prompt-learning", "")
	GetPromptLearningPolicy(context)
	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"enabled":false`)
	assert.Contains(t, recorder.Body.String(), `"generation":0`)

	context, recorder = promptLearningSessionContext(http.MethodPut, "/api/user/prompt-learning", `{"enabled":true}`)
	UpdatePromptLearningPolicy(context)
	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"enabled":true`)
	assert.Contains(t, recorder.Body.String(), `"generation":1`)

	context, recorder = promptLearningSessionContext(http.MethodPost, "/api/user/prompt-learning/versions", `{"command_id":"manual-1","content":"# Instructions\n\nUse canary deployments."}`)
	CreatePromptLearningVersion(context)
	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"created":true`)
	assert.NotContains(t, recorder.Body.String(), "user:17")

	context, recorder = promptLearningSessionContext(http.MethodGet, "/api/user/prompt-learning/versions", "")
	ListPromptLearningVersions(context)
	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"total":1`)
	assert.Contains(t, recorder.Body.String(), "Use canary deployments.")

	policy, err := model.GetPromptLearningPolicy(context.Request.Context(), 17, promptLearningSelfScope(17))
	require.NoError(t, err)
	_, _, err = model.CreatePromptLearningRun(context.Request.Context(), model.PromptLearningRunInput{
		UserID: 17, ScopeRef: promptLearningSelfScope(17), CommandID: "run-1", PolicyGeneration: policy.Generation,
		SampleUpperID: 9, SampleCount: 2, ModelRef: "gpt-6", TemplateVersion: "prompt-learning-v1",
		InputTokenLimit: 8000, OutputTokenLimit: 2000, MaximumAttempts: 2,
	})
	require.NoError(t, err)
	context, recorder = promptLearningSessionContext(http.MethodGet, "/api/user/prompt-learning/runs", "")
	ListPromptLearningRuns(context)
	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"total":1`)
	assert.Contains(t, recorder.Body.String(), `"state":"pending"`)
	assert.NotContains(t, recorder.Body.String(), "user:17")
	assert.NotContains(t, recorder.Body.String(), "run-1")

	context, recorder = promptLearningSessionContext(http.MethodPost, "/api/user/prompt-learning/runs/1/cancel", "")
	context.Params = gin.Params{{Key: "id", Value: "1"}}
	CancelPromptLearningRun(context)
	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"cancelled":true`)
}

func TestPromptLearningControllersRejectNonSessionAuth(t *testing.T) {
	setupPromptLearningControllerTest(t)
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPut, "/api/user/prompt-learning", strings.NewReader(`{"enabled":true}`))
	context.Set("id", 17)

	UpdatePromptLearningPolicy(context)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"success":false`)
	assert.Contains(t, recorder.Body.String(), "Unauthorized")
}

func TestPromptLearningControllerRejectsMalformedOrConflictingManualVersion(t *testing.T) {
	setupPromptLearningControllerTest(t)
	gin.SetMode(gin.TestMode)
	context, recorder := promptLearningSessionContext(http.MethodPost, "/api/user/prompt-learning/versions", `{"command_id":"","content":""}`)
	CreatePromptLearningVersion(context)
	assert.Contains(t, recorder.Body.String(), `"success":false`)

	context, recorder = promptLearningSessionContext(http.MethodPost, "/api/user/prompt-learning/versions", `{"command_id":"manual-1","content":"One"}`)
	CreatePromptLearningVersion(context)
	require.Equal(t, http.StatusOK, recorder.Code)
	context, recorder = promptLearningSessionContext(http.MethodPost, "/api/user/prompt-learning/versions", `{"command_id":"manual-1","content":"Two"}`)
	CreatePromptLearningVersion(context)
	assert.Equal(t, http.StatusConflict, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"success":false`)
}
