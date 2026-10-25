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

func TestLogResultRejectsUnknownValuesBeforeAnyDatabaseRead(t *testing.T) {
	for _, handler := range []struct {
		name string
		call gin.HandlerFunc
	}{{"admin-list", GetAllLogs}, {"self-list", GetUserLogs}, {"admin-stats", GetLogsStat}, {"self-stats", GetLogsSelfStat}} {
		for _, query := range []string{"result=pending", "result=FAILED", "result=error", "result=failed&result=failed"} {
			t.Run(handler.name+"/"+query, func(t *testing.T) {
				r := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(r)
				c.Request = httptest.NewRequest(http.MethodGet, "/api/log/?"+query, nil)
				c.Set("id", 1)
				c.Set("username", "owner")
				handler.call(c)
				require.Equal(t, http.StatusBadRequest, r.Code)
			})
		}
	}
}

func TestLogResultFailedHandlersKeepOwnerScope(t *testing.T) {
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
		{UserId: 1, Username: "alice", Type: model.LogTypeConsume, CreatedAt: 100, Quota: 15, RequestId: "owner-consume", Other: `{"usage_accuracy":"reported"}`},
		{UserId: 1, Username: "alice", Type: model.LogTypeError, CreatedAt: 101, RequestId: "owner-failed", Content: "status_code=502", Other: `{"status_code":502}`},
		{UserId: 1, Username: "alice", Type: model.LogTypeError, CreatedAt: 102, RequestId: "owner-review", Content: "usage_pending_review", Other: `{"settlement_status":"pending_review"}`},
		{UserId: 1, Username: "alice", Type: model.LogTypeError, CreatedAt: 103, RequestId: "owner-preflight", Content: "input limit", Other: `{"request_outcome":"rejected","preflight_rejected":true}`},
		{UserId: 2, Username: "bob", Type: model.LogTypeError, CreatedAt: 104, RequestId: "foreign-failed", Content: "status_code=500", Other: `{"status_code":500}`},
	} {
		require.NoError(t, model.CreateLog(db, &row))
	}

	for _, tt := range []struct {
		name    string
		path    string
		handler gin.HandlerFunc
		wantIDs []string
	}{
		{"self failed", "/api/log/self?result=failed", GetUserLogs, []string{"owner-failed"}},
		{"self failed cannot widen with username", "/api/log/self?result=failed&username=bob", GetUserLogs, []string{"owner-failed"}},
		{"self type 5 unchanged", "/api/log/self?type=5", GetUserLogs, []string{"owner-failed", "owner-review", "owner-preflight"}},
		{"self empty result unchanged", "/api/log/self?type=5&result=", GetUserLogs, []string{"owner-failed", "owner-review", "owner-preflight"}},
		{"admin failed", "/api/log/?result=failed", GetAllLogs, []string{"owner-failed", "foreign-failed"}},
		{"admin failed with type 5", "/api/log/?type=5&result=failed", GetAllLogs, []string{"owner-failed", "foreign-failed"}},
		{"admin type 5 unchanged", "/api/log/?type=5", GetAllLogs, []string{"owner-failed", "owner-review", "owner-preflight", "foreign-failed"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(r)
			c.Request = httptest.NewRequest(http.MethodGet, tt.path, nil)
			c.Set("id", 1)
			c.Set("username", "alice")
			tt.handler(c)
			require.Equal(t, http.StatusOK, r.Code)
			var response struct {
				Success bool `json:"success"`
				Data    struct {
					Total int `json:"total"`
					Items []struct {
						RequestID string `json:"request_id"`
					} `json:"items"`
				} `json:"data"`
			}
			require.NoError(t, common.Unmarshal(r.Body.Bytes(), &response))
			require.True(t, response.Success)
			ids := make([]string, 0, len(response.Data.Items))
			for _, item := range response.Data.Items {
				ids = append(ids, item.RequestID)
			}
			assert.ElementsMatch(t, tt.wantIDs, ids)
			assert.Equal(t, len(tt.wantIDs), response.Data.Total)
		})
	}

	// The stat endpoints keep quota, rpm and tpm on confirmed consumption; the
	// result filter narrows only the record statistics that follow the list.
	for _, tt := range []struct {
		name        string
		path        string
		handler     gin.HandlerFunc
		wantRecords int64
	}{
		{"self stat failed", "/api/log/self/stat?result=failed", GetLogsSelfStat, 1},
		{"self stat unchanged", "/api/log/self/stat?type=5", GetLogsSelfStat, 3},
		{"admin stat failed", "/api/log/stat?result=failed", GetLogsStat, 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(r)
			c.Request = httptest.NewRequest(http.MethodGet, tt.path, nil)
			c.Set("id", 1)
			c.Set("username", "alice")
			tt.handler(c)
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
			assert.Equal(t, tt.wantRecords, response.Data.Tokens.Records)
			assert.Equal(t, 15, response.Data.Quota)
		})
	}
}
