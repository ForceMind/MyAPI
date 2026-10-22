package controller

import (
	"context"
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

func TestLogTokenStatisticsHandlersPreserveScopeFiltersAndUnknown(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.Log{}, &model.BillingLogProjectionIdentity{}))
	previous := model.LOG_DB
	model.LOG_DB = db
	t.Cleanup(func() { model.LOG_DB = previous; require.NoError(t, sqlDB.Close()) })
	for _, row := range []model.Log{
		{UserId: 1, Username: "alice", Type: model.LogTypeConsume, CreatedAt: 100, Quota: 15, PromptTokens: 10, CompletionTokens: 5, RequestId: "owner-request", UpstreamRequestId: "up-owner", Other: `{"usage_accuracy":"reported","cache_tokens":2}`, Content: "private prompt"},
		{UserId: 1, Username: "alice", Type: model.LogTypeError, CreatedAt: 101, Other: `{"request_outcome":"completed","usage_outcome":"missing"}`},
		{UserId: 2, Username: "bob", Type: model.LogTypeConsume, CreatedAt: 100, PromptTokens: 999, Other: `{"usage_accuracy":"reported"}`},
	} {
		require.NoError(t, model.CreateLog(db, &row))
	}
	for _, item := range []struct {
		name, path string
		handler    gin.HandlerFunc
		actorName  string
		records    int64
	}{
		{"self", "/api/log/self/stat?username=bob&channel=999", GetLogsSelfStat, "alice", 2},
		{"self-request", "/api/log/self/stat?request_id=owner-request&upstream_request_id=up-owner", GetLogsSelfStat, "alice", 1},
		{"admin", "/api/log/stat?request_id=owner-request", GetLogsStat, "alice", 1},
		{"renamed-self", "/api/log/self/stat", GetLogsSelfStat, "renamed-alice", 2},
	} {
		t.Run(item.name, func(t *testing.T) {
			r := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(r)
			c.Request = httptest.NewRequest(http.MethodGet, item.path, nil)
			c.Set("id", 1)
			c.Set("username", item.actorName)
			item.handler(c)
			require.Equal(t, http.StatusOK, r.Code)
			var response struct {
				Success bool `json:"success"`
				Data    struct {
					Tokens model.LogTokenStatistics `json:"token_usage"`
					Quota  int                      `json:"quota"`
				} `json:"data"`
			}
			require.NoError(t, common.Unmarshal(r.Body.Bytes(), &response))
			require.True(t, response.Success)
			assert.Equal(t, item.records, response.Data.Tokens.Records)
			assert.Equal(t, 15, response.Data.Quota)
			assert.Equal(t, "15", response.Data.Tokens.KnownTotalTokens)
			if item.records == 2 {
				assert.Nil(t, response.Data.Tokens.TotalTokens)
				assert.Contains(t, r.Body.String(), `"total_tokens":null`)
			} else {
				require.NotNil(t, response.Data.Tokens.TotalTokens)
				assert.Equal(t, "15", *response.Data.Tokens.TotalTokens)
			}
			assert.NotContains(t, r.Body.String(), "private prompt")
			assert.NotContains(t, r.Body.String(), "bob")
		})
	}
}

func TestLogTokenStatisticsSelfRequiresAuthenticatedIdentity(t *testing.T) {
	for _, id := range []int{0, -1} {
		r := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(r)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/log/self/stat?username=alice", nil)
		c.Set("id", id)
		GetLogsSelfStat(c)
		assert.Equal(t, http.StatusUnauthorized, r.Code)
		assert.Empty(t, r.Body.String())
	}
}

func TestLogTokenStatisticsCancelledReadDoesNotReturnZeroTotals(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Log{}, &model.BillingLogProjectionIdentity{}))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	previous := model.LOG_DB
	model.LOG_DB = db
	t.Cleanup(func() { model.LOG_DB = previous })
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	r := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(r)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/log/stat", nil).WithContext(ctx)
	GetLogsStat(c)
	var response struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(r.Body.Bytes(), &response))
	assert.False(t, response.Success)
	assert.NotContains(t, r.Body.String(), `"token_usage"`)
}
