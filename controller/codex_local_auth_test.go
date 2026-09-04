package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/middleware"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestGetCodexLocalAuthStatusDoesNotExposeCredential(t *testing.T) {
	stubCodexLocalAuthInspection(t, service.CodexLocalAuthInspection{
		State:                 service.CodexLocalAuthReady,
		Platform:              "darwin",
		Environment:           service.CodexLocalEnvironmentNative,
		CodexInstalled:        true,
		AuthFileExists:        true,
		AuthReadable:          true,
		LoggedIn:              true,
		AutoImportAvailable:   true,
		ManualImportAvailable: true,
		AccountHint:           "acct…3456",
		CanRefresh:            true,
	}, &service.CodexOAuthKey{
		IDToken:      "identity-secret",
		AccessToken:  "access-secret",
		RefreshToken: "refresh-secret",
		AccountID:    "account-secret",
	})
	recorder, context := codexLocalAuthTestContext(t, http.MethodGet, "/api/channel/codex/local-auth/status", "")

	GetCodexLocalAuthStatus(context)

	require.Equal(t, http.StatusOK, recorder.Code)
	body := recorder.Body.String()
	assert.Contains(t, body, `"state":"ready"`)
	assert.Contains(t, body, `"account_hint":"acct…3456"`)
	assert.NotContains(t, body, "identity-secret")
	assert.NotContains(t, body, "access-secret")
	assert.NotContains(t, body, "refresh-secret")
	assert.NotContains(t, body, "account-secret")
}

func TestImportCodexLocalAuthCreatesChannelWithoutReturningCredential(t *testing.T) {
	setupCodexLocalAuthControllerDB(t)
	stubCodexLocalAuthInspection(t, readyCodexLocalAuthInspection(), &service.CodexOAuthKey{
		IDToken:      "identity-secret",
		AccessToken:  "access-secret",
		RefreshToken: "refresh-secret",
		AccountID:    "account-123456",
		Type:         "codex",
	})
	body := `{"mode":"single","channel":{"type":57,"name":"Local Codex","key":"","models":"gpt-5-codex","group":"default","status":1}}`
	recorder, context := codexLocalAuthTestContext(t, http.MethodPost, "/api/channel/codex/local-auth/import", body)

	ImportCodexLocalAuth(context)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.Contains(t, recorder.Body.String(), `"operation":"created"`)
	assert.NotContains(t, recorder.Body.String(), "identity-secret")
	assert.NotContains(t, recorder.Body.String(), "access-secret")
	assert.NotContains(t, recorder.Body.String(), "refresh-secret")
	var stored model.Channel
	require.NoError(t, model.DB.Where("name = ?", "Local Codex").First(&stored).Error)
	assert.Contains(t, stored.Key, "access-secret")
	var abilityCount int64
	require.NoError(t, model.DB.Model(&model.Ability{}).Where("channel_id = ?", stored.Id).Count(&abilityCount).Error)
	assert.Equal(t, int64(1), abilityCount)
}

func TestImportCodexLocalAuthForChannelUsesConditionalCredentialUpdate(t *testing.T) {
	setupCodexLocalAuthControllerDB(t)
	afterInspection := false
	queriesAfterInspection := 0
	original := inspectCodexLocalAuth
	inspectCodexLocalAuth = func(bool) (service.CodexLocalAuthInspection, *service.CodexOAuthKey) {
		afterInspection = true
		return readyCodexLocalAuthInspection(), &service.CodexOAuthKey{
			IDToken:      "new-identity-secret",
			AccessToken:  "new-access-secret",
			RefreshToken: "new-refresh-secret",
			AccountID:    "account-123456",
			Type:         "codex",
		}
	}
	t.Cleanup(func() { inspectCodexLocalAuth = original })
	require.NoError(t, model.DB.Callback().Query().Before("gorm:query").Register("test:detect_full_cache_reload", func(tx *gorm.DB) {
		if afterInspection && (tx.Statement.Table == "channels" || tx.Statement.Table == "abilities") {
			queriesAfterInspection++
		}
	}))
	channel := &model.Channel{Type: constant.ChannelTypeCodex, Name: "Existing Codex", Key: `{"access_token":"old"}`, Models: "gpt-5-codex", Group: "default", Status: 1}
	require.NoError(t, model.InsertChannelWithAbilities(t.Context(), channel))
	recorder, context := codexLocalAuthTestContext(t, http.MethodPost, "/api/channel/1/codex/local-auth/import", "")
	context.Params = gin.Params{{Key: "id", Value: strconv.Itoa(channel.Id)}}

	ImportCodexLocalAuthForChannel(context)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.Contains(t, recorder.Body.String(), `"operation":"updated"`)
	assert.NotContains(t, recorder.Body.String(), "new-identity-secret")
	assert.NotContains(t, recorder.Body.String(), "new-access-secret")
	assert.NotContains(t, recorder.Body.String(), "new-refresh-secret")
	assert.Zero(t, queriesAfterInspection, "credential-only updates must not trigger a full channel cache reload")
	afterInspection = false
	var stored model.Channel
	require.NoError(t, model.DB.First(&stored, channel.Id).Error)
	assert.Contains(t, stored.Key, "new-access-secret")
}

func TestImportCodexLocalAuthForChannelDoesNotOverwriteConcurrentCredential(t *testing.T) {
	setupCodexLocalAuthControllerDB(t)
	channel := &model.Channel{Type: constant.ChannelTypeCodex, Name: "Concurrent Codex", Key: `{"access_token":"old"}`, Models: "gpt-5-codex", Group: "default", Status: 1}
	require.NoError(t, model.InsertChannelWithAbilities(t.Context(), channel))
	concurrentKey := `{"access_token":"concurrent","refresh_token":"concurrent-refresh","account_id":"account-123456"}`
	original := inspectCodexLocalAuth
	inspectCodexLocalAuth = func(bool) (service.CodexLocalAuthInspection, *service.CodexOAuthKey) {
		require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", channel.Id).Update("key", concurrentKey).Error)
		return readyCodexLocalAuthInspection(), &service.CodexOAuthKey{
			AccessToken:  "local-access-secret",
			RefreshToken: "local-refresh-secret",
			AccountID:    "account-123456",
			Type:         "codex",
		}
	}
	t.Cleanup(func() { inspectCodexLocalAuth = original })
	recorder, context := codexLocalAuthTestContext(t, http.MethodPost, "/api/channel/1/codex/local-auth/import", "")
	context.Params = gin.Params{{Key: "id", Value: strconv.Itoa(channel.Id)}}

	ImportCodexLocalAuthForChannel(context)

	assert.Equal(t, http.StatusConflict, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "CODEX_LOCAL_AUTH_CONFLICT")
	assert.NotContains(t, recorder.Body.String(), "local-access-secret")
	var stored model.Channel
	require.NoError(t, model.DB.First(&stored, channel.Id).Error)
	assert.Equal(t, concurrentKey, stored.Key)
}

func TestImportCodexLocalAuthSameTargetStillDetectsConcurrentK0ToK1(t *testing.T) {
	setupCodexLocalAuthControllerDB(t)
	localCredential := &service.CodexOAuthKey{
		AccessToken:  "local-access",
		RefreshToken: "local-refresh",
		AccountID:    "account-123456",
		Type:         "codex",
	}
	encodedLocal, err := common.Marshal(localCredential)
	require.NoError(t, err)
	channel := &model.Channel{Type: constant.ChannelTypeCodex, Name: "Same Target Codex", Key: string(encodedLocal), Models: "gpt-5-codex", Group: "default", Status: 1}
	require.NoError(t, model.InsertChannelWithAbilities(t.Context(), channel))
	concurrentKey := `{"access_token":"concurrent","refresh_token":"concurrent-refresh","account_id":"account-123456"}`
	original := inspectCodexLocalAuth
	inspectCodexLocalAuth = func(bool) (service.CodexLocalAuthInspection, *service.CodexOAuthKey) {
		require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", channel.Id).Update("key", concurrentKey).Error)
		return readyCodexLocalAuthInspection(), localCredential
	}
	t.Cleanup(func() { inspectCodexLocalAuth = original })
	recorder, context := codexLocalAuthTestContext(t, http.MethodPost, "/api/channel/1/codex/local-auth/import", "")
	context.Params = gin.Params{{Key: "id", Value: strconv.Itoa(channel.Id)}}

	ImportCodexLocalAuthForChannel(context)

	assert.Equal(t, http.StatusConflict, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "CODEX_LOCAL_AUTH_CONFLICT")
	var stored model.Channel
	require.NoError(t, model.DB.First(&stored, channel.Id).Error)
	assert.Equal(t, concurrentKey, stored.Key)
}

func TestImportCodexLocalAuthRejectsUnknownPathFieldBeforeInspection(t *testing.T) {
	setupCodexLocalAuthControllerDB(t)
	called := false
	original := inspectCodexLocalAuth
	inspectCodexLocalAuth = func(bool) (service.CodexLocalAuthInspection, *service.CodexOAuthKey) {
		called = true
		return readyCodexLocalAuthInspection(), nil
	}
	t.Cleanup(func() { inspectCodexLocalAuth = original })
	body := `{"mode":"single","path":"/tmp/secret","channel":{"type":57,"name":"Local Codex","key":"","models":"gpt-5-codex","group":"default","status":1}}`
	recorder, context := codexLocalAuthTestContext(t, http.MethodPost, "/api/channel/codex/local-auth/import", body)

	ImportCodexLocalAuth(context)

	assert.False(t, called)
	var response struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.False(t, response.Success)
}

func TestCodexLocalAuthRequiresLiveDashboardSession(t *testing.T) {
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/channel/codex/local-auth/status", nil)

	GetCodexLocalAuthStatus(context)

	assert.Equal(t, http.StatusUnauthorized, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "AUTH_SESSION_REQUIRED")
}

func TestCodexLocalAuthDoesNotInspectHostCredentialInLANEdition(t *testing.T) {
	t.Setenv("MYAPI_EDITION", middleware.EditionLAN)
	called := false
	original := inspectCodexLocalAuth
	inspectCodexLocalAuth = func(bool) (service.CodexLocalAuthInspection, *service.CodexOAuthKey) {
		called = true
		return readyCodexLocalAuthInspection(), nil
	}
	t.Cleanup(func() { inspectCodexLocalAuth = original })
	recorder, context := codexLocalAuthTestContext(t, http.MethodGet, "/api/channel/codex/local-auth/status", "")

	GetCodexLocalAuthStatus(context)

	assert.Equal(t, http.StatusForbidden, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "MYAPI_LAN_ROUTE_DISABLED")
	assert.False(t, called)
}

func setupCodexLocalAuthControllerDB(t *testing.T) {
	t.Helper()
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMemoryCache, previousRedisEnabled := common.MemoryCacheEnabled, common.RedisEnabled
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.Log{}, &model.User{}))
	model.DB, model.LOG_DB = db, db
	common.MemoryCacheEnabled = false
	common.RedisEnabled = false
	require.NoError(t, db.Create(&model.User{Id: 1, Username: "root", Role: common.RoleRootUser, Status: common.UserStatusEnabled}).Error)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.MemoryCacheEnabled = previousMemoryCache
		common.RedisEnabled = previousRedisEnabled
	})
}

func stubCodexLocalAuthInspection(t *testing.T, inspection service.CodexLocalAuthInspection, key *service.CodexOAuthKey) {
	t.Helper()
	original := inspectCodexLocalAuth
	inspectCodexLocalAuth = func(bool) (service.CodexLocalAuthInspection, *service.CodexOAuthKey) {
		return inspection, key
	}
	t.Cleanup(func() { inspectCodexLocalAuth = original })
}

func readyCodexLocalAuthInspection() service.CodexLocalAuthInspection {
	return service.CodexLocalAuthInspection{
		State:                 service.CodexLocalAuthReady,
		Platform:              "darwin",
		Environment:           service.CodexLocalEnvironmentNative,
		AuthFileExists:        true,
		AuthReadable:          true,
		LoggedIn:              true,
		AutoImportAvailable:   true,
		ManualImportAvailable: true,
		AccountHint:           "acco…3456",
		CanRefresh:            true,
	}
}

func codexLocalAuthTestContext(t *testing.T, method string, target string, body string) (*httptest.ResponseRecorder, *gin.Context) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(method, target, strings.NewReader(body))
	context.Request.Header.Set("Content-Type", "application/json")
	context.Set("id", 1)
	context.Set("session_id", "session-1")
	context.Set("auth_version", int64(1))
	context.Set("session_version", int64(1))
	return recorder, context
}
