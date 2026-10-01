package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRecentLogOverviewHandlersKeepUserScopeAndRedactBodies(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.Log{}, &model.BillingLogProjectionIdentity{}))
	previousLogDB := model.LOG_DB
	model.LOG_DB = db
	t.Cleanup(func() {
		model.LOG_DB = previousLogDB
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.Create(&model.Log{UserId: 1, Username: "alice", Type: model.LogTypeConsume,
		CreatedAt: 100, ModelName: "gpt-a", Content: "private prompt", Other: "secret metadata",
	}).Error)
	require.NoError(t, db.Create(&model.Log{UserId: 2, Username: "bob", Type: model.LogTypeError,
		CreatedAt: 101, ModelName: "gpt-b", Content: "private upstream failure",
	}).Error)

	selfRecorder := httptest.NewRecorder()
	selfContext, _ := gin.CreateTestContext(selfRecorder)
	selfContext.Request = httptest.NewRequest(http.MethodGet, "/api/log/self/overview", nil)
	selfContext.Set("id", 1)
	GetUserRecentLogOverview(selfContext)
	require.Equal(t, http.StatusOK, selfRecorder.Code)
	var selfResponse struct {
		Success bool                    `json:"success"`
		Data    model.RecentLogOverview `json:"data"`
	}
	require.NoError(t, common.Unmarshal(selfRecorder.Body.Bytes(), &selfResponse))
	require.True(t, selfResponse.Success)
	require.Len(t, selfResponse.Data.Requests, 1)
	assert.Empty(t, selfResponse.Data.Errors)
	assert.NotContains(t, selfRecorder.Body.String(), "alice")
	assert.NotContains(t, selfRecorder.Body.String(), "bob")
	assert.NotContains(t, selfRecorder.Body.String(), "private")
	assert.NotContains(t, selfRecorder.Body.String(), "secret")

	adminRecorder := httptest.NewRecorder()
	adminContext, _ := gin.CreateTestContext(adminRecorder)
	adminContext.Request = httptest.NewRequest(http.MethodGet, "/api/log/overview", nil)
	GetAdminRecentLogOverview(adminContext)
	require.Equal(t, http.StatusOK, adminRecorder.Code)
	assert.Contains(t, adminRecorder.Body.String(), "bob")
	assert.NotContains(t, adminRecorder.Body.String(), "private")
}

func TestUserRecentLogOverviewRequiresAuthenticatedIdentity(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/log/self/overview", nil)

	GetUserRecentLogOverview(c)

	assert.Equal(t, http.StatusUnauthorized, recorder.Code)
}
