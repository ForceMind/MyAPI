package router

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

func TestTaskOperationRouteIsOwnerScopedSanitizedAndNoStore(t *testing.T) {
	previousDB, previousRedis, previousSessionSecret := model.DB, common.RedisEnabled, common.SessionSecret
	previousGinMode := gin.Mode()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSession{}, &model.Token{}, &model.TaskSubmissionOperation{}))
	model.DB = db
	common.RedisEnabled = false
	common.SessionSecret = "task-operation-route-test-secret"
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() {
		model.DB = previousDB
		common.RedisEnabled = previousRedis
		common.SessionSecret = previousSessionSecret
		gin.SetMode(previousGinMode)
		require.NoError(t, sqlDB.Close())
	})

	owner := &model.User{Username: "operation-owner", Password: "unused-password", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, AuthVersion: 1, AffCode: "operation-owner-aff"}
	other := &model.User{Username: "operation-other", Password: "unused-password", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, AuthVersion: 1, AffCode: "operation-other-aff"}
	require.NoError(t, db.Create(owner).Error)
	require.NoError(t, db.Create(other).Error)
	ownerAllowIPs := "198.51.100.0/24"
	ownerToken := &model.Token{UserId: owner.Id, Key: "operationownerkey", Status: common.TokenStatusExpired, AllowIps: &ownerAllowIPs}
	sameUserOtherToken := &model.Token{UserId: owner.Id, Key: "operationwrongkey", Status: common.TokenStatusEnabled}
	otherToken := &model.Token{UserId: other.Id, Key: "operationotherkey", Status: common.TokenStatusEnabled}
	for _, token := range []*model.Token{ownerToken, sameUserOtherToken, otherToken} {
		require.NoError(t, db.Create(token).Error)
	}
	publicID := "task_eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	require.NoError(t, db.Exec(
		"INSERT INTO task_submission_operations (public_id, user_id, token_id, http_method, operation_kind, idempotency_key_hash, request_fingerprint, status, reason_code, resolution_source, lock_version, created_at, updated_at, dispatch_started_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		publicID, owner.Id, ownerToken.Id, "POST", model.TaskSubmissionOperationKindSunoMusic,
		"eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
		"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		model.TaskSubmissionOperationStatusDispatching, "private_reason", "private_source", 3, int64(300), int64(310), int64(305),
	).Error)
	invalidPublicID := "task_ffffffffffffffffffffffffffffffff"
	require.NoError(t, db.Exec(
		"INSERT INTO task_submission_operations (public_id, user_id, token_id, http_method, operation_kind, idempotency_key_hash, request_fingerprint, status, lock_version, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		invalidPublicID, owner.Id, ownerToken.Id, "POST", model.TaskSubmissionOperationKindVideoCreate,
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		model.TaskSubmissionOperationStatusDispatching, 1, int64(400), int64(410),
	).Error)
	_, ownerSessionToken := createCodexLocalRouteSession(t, owner)

	engine := gin.New()
	require.NoError(t, engine.SetTrustedProxies(nil))
	SetRelayRouter(engine)
	SetTaskOperationRouter(engine)
	SetVideoRouter(engine)
	request := func(credential, id, remoteAddr string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/v1/task-operations/"+id, nil)
		if credential != "" {
			req.Header.Set("Authorization", "Bearer "+credential)
		}
		req.RemoteAddr = remoteAddr
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, req)
		return response
	}

	unauthorized := request("", publicID, "198.51.100.30:4321")
	assert.Equal(t, http.StatusUnauthorized, unauthorized.Code)
	assert.Contains(t, unauthorized.Header().Get("Cache-Control"), "no-store")

	forbidden := request("sk-"+ownerToken.Key, publicID, "203.0.113.30:4321")
	assert.Equal(t, http.StatusForbidden, forbidden.Code)
	assert.Contains(t, forbidden.Header().Get("Cache-Control"), "no-store")

	wrongToken := request("sk-"+sameUserOtherToken.Key, publicID, "198.51.100.30:4321")
	assert.Equal(t, http.StatusNotFound, wrongToken.Code)
	assert.Contains(t, wrongToken.Header().Get("Cache-Control"), "no-store")

	wrongUser := request("sk-"+otherToken.Key, publicID, "198.51.100.30:4321")
	assert.Equal(t, http.StatusNotFound, wrongUser.Code)
	assert.Contains(t, wrongUser.Header().Get("Cache-Control"), "no-store")
	assert.Equal(t, wrongToken.Body.String(), wrongUser.Body.String())

	unavailable := request("sk-"+ownerToken.Key, invalidPublicID, "198.51.100.30:4321")
	assert.Equal(t, http.StatusInternalServerError, unavailable.Code)
	assert.Contains(t, unavailable.Header().Get("Cache-Control"), "no-store")

	apiResponse := request("sk-"+ownerToken.Key, publicID, "198.51.100.30:4321")
	assert.Equal(t, http.StatusOK, apiResponse.Code)
	assert.Contains(t, apiResponse.Header().Get("Cache-Control"), "no-store")
	var responseBody map[string]any
	require.NoError(t, common.Unmarshal(apiResponse.Body.Bytes(), &responseBody))
	assert.Equal(t, map[string]any{
		"id":                  publicID,
		"object":              "task_operation",
		"kind":                model.TaskSubmissionOperationKindSunoMusic,
		"status":              string(model.TaskSubmissionOperationStatusDispatching),
		"created_at":          float64(300),
		"updated_at":          float64(310),
		"dispatch_started_at": float64(305),
		"resolved_at":         nil,
	}, responseBody)
	for _, forbidden := range []string{"private_reason", "private_source", "idempotency", "fingerprint", "token_id", "user_id", "lock_version", "quota", "attempt"} {
		assert.NotContains(t, apiResponse.Body.String(), forbidden)
	}

	sessionResponse := request(ownerSessionToken, publicID, "198.51.100.30:4321")
	assert.Equal(t, http.StatusOK, sessionResponse.Code)
	assert.Contains(t, sessionResponse.Header().Get("Cache-Control"), "no-store")
}

func TestTaskOperationRouterRegistersOnlyReadRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetRelayRouter(engine)
	SetTaskOperationRouter(engine)
	SetVideoRouter(engine)
	routes := engine.Routes()
	matchingRoutes := 0
	for _, route := range routes {
		if route.Path != "/v1/task-operations/:id" {
			continue
		}
		matchingRoutes++
		assert.Equal(t, http.MethodGet, route.Method)
	}
	assert.Equal(t, 1, matchingRoutes)
}
