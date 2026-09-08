package middleware

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
	"gorm.io/gorm/logger"
)

func TestTaskOperationAuthUsesAuthoritativeReadableTokenStatesAndIPLimits(t *testing.T) {
	previousDB, previousRedis, previousGinMode := model.DB, common.RedisEnabled, gin.Mode()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}))
	model.DB = db
	common.RedisEnabled = true
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() {
		model.DB = previousDB
		common.RedisEnabled = previousRedis
		gin.SetMode(previousGinMode)
		require.NoError(t, sqlDB.Close())
	})

	user := &model.User{Username: "task-auth-user", Password: "unused-password", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, AffCode: "task-auth-aff"}
	require.NoError(t, db.Create(user).Error)
	allowIPs := "198.51.100.0/24"
	tokens := []*model.Token{
		{UserId: user.Id, Key: "taskenabled", Status: common.TokenStatusEnabled},
		{UserId: user.Id, Key: "taskexpired", Status: common.TokenStatusExpired},
		{UserId: user.Id, Key: "taskexhausted", Status: common.TokenStatusExhausted},
		{UserId: user.Id, Key: "taskdisabled", Status: common.TokenStatusDisabled},
		{UserId: user.Id, Key: "taskunknownstatus", Status: 99},
		{UserId: user.Id, Key: "taskip", Status: common.TokenStatusEnabled, AllowIps: &allowIPs},
	}
	for _, token := range tokens {
		require.NoError(t, db.Create(token).Error)
	}

	router := gin.New()
	router.GET("/operation", TaskOperationAuth(), func(c *gin.Context) {
		tokenID, tokenScoped := GetTaskOperationTokenID(c)
		c.JSON(http.StatusOK, gin.H{"user_id": c.GetInt("id"), "token_id": tokenID, "token_scoped": tokenScoped})
	})
	request := func(key, remoteAddr string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/operation", nil)
		if key != "" {
			req.Header.Set("Authorization", "Bearer sk-"+key)
		}
		req.RemoteAddr = remoteAddr
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response
	}

	for _, token := range tokens[:3] {
		response := request(token.Key, "198.51.100.9:1234")
		assert.Equal(t, http.StatusOK, response.Code, token.Key)
		assert.Contains(t, response.Body.String(), `"token_scoped":true`)
		assert.Contains(t, response.Body.String(), `"token_id":`)
	}
	assert.Equal(t, http.StatusUnauthorized, request(tokens[3].Key, "198.51.100.9:1234").Code)
	assert.Equal(t, http.StatusUnauthorized, request(tokens[4].Key, "198.51.100.9:1234").Code)
	assert.Equal(t, http.StatusUnauthorized, request("unknownkey", "198.51.100.9:1234").Code)
	assert.Equal(t, http.StatusForbidden, request(tokens[5].Key, "203.0.113.9:1234").Code)
	assert.Equal(t, http.StatusOK, request(tokens[5].Key, "198.51.100.9:1234").Code)

	require.NoError(t, db.Model(user).Update("status", common.UserStatusDisabled).Error)
	assert.Equal(t, http.StatusForbidden, request(tokens[0].Key, "198.51.100.9:1234").Code)
	require.NoError(t, db.Model(user).Update("status", common.UserStatusEnabled).Error)

	require.NoError(t, db.Delete(tokens[0]).Error)
	assert.Equal(t, http.StatusUnauthorized, request(tokens[0].Key, "198.51.100.9:1234").Code)

	brokenUser := &model.User{Id: -10, Username: "task-auth-broken", Password: "unused-password", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, AffCode: "task-auth-broken-aff"}
	require.NoError(t, db.Create(brokenUser).Error)
	nonpositiveUserToken := &model.Token{UserId: brokenUser.Id, Key: "taskbrokenuser", Status: common.TokenStatusEnabled}
	require.NoError(t, db.Create(nonpositiveUserToken).Error)
	require.Positive(t, nonpositiveUserToken.Id)
	assert.Equal(t, http.StatusUnauthorized, request(nonpositiveUserToken.Key, "198.51.100.9:1234").Code)

	nonpositiveIDToken := &model.Token{Id: -20, UserId: user.Id, Key: "taskbrokentoken", Status: common.TokenStatusEnabled}
	require.NoError(t, db.Create(nonpositiveIDToken).Error)
	assert.Equal(t, http.StatusUnauthorized, request(nonpositiveIDToken.Key, "198.51.100.9:1234").Code)
}

func TestGetTaskOperationTokenIDPreservesInvalidTokenScope(t *testing.T) {
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Set(taskOperationTokenScopedContextKey, true)
	context.Set("token_id", 0)

	tokenID, tokenScoped := GetTaskOperationTokenID(context)
	assert.True(t, tokenScoped)
	assert.Zero(t, tokenID)
}
