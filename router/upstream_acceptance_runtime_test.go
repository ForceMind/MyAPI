package router

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This ordinary test exercises real API middleware, signed login sessions and
// HTTP transport against disposable SQLite and a literal-loopback upstream.
// Reset-credit reads need no invented native-source or billing qualification.
func TestUpstreamAcceptanceRuntimeAPI(t *testing.T) {
	setupRelayRouterTestDB(t)
	require.NoError(t, i18n.Init())
	db := model.DB
	require.NoError(t, db.AutoMigrate(&model.UserSession{}, &model.UpstreamAcceptanceRun{}))
	previousMemory, previousSecret := common.MemoryCacheEnabled, common.SessionSecret
	previousCritical, previousGlobal := common.CriticalRateLimitEnable, common.GlobalApiRateLimitEnable
	previousTLS := common.TLSInsecureSkipVerify
	common.MemoryCacheEnabled, common.CriticalRateLimitEnable, common.GlobalApiRateLimitEnable = false, false, false
	common.TLSInsecureSkipVerify = false
	common.SessionSecret = "acceptance-runtime-synthetic-session-secret"
	t.Cleanup(func() {
		common.MemoryCacheEnabled, common.SessionSecret = previousMemory, previousSecret
		common.CriticalRateLimitEnable, common.GlobalApiRateLimitEnable = previousCritical, previousGlobal
		common.TLSInsecureSkipVerify = previousTLS
	})
	t.Setenv("MYAPI_EDITION", "full")
	t.Setenv("MYAPI_UPSTREAM_ACCEPTANCE_ENABLED", "false")

	pat := "acceptance-runtime-synthetic-root-pat"
	root := model.User{Id: 1, Username: "acceptance-runtime-root", AffCode: "acceptance-runtime-root", Role: common.RoleRootUser,
		Status: common.UserStatusEnabled, AuthVersion: 1, AccessToken: &pat, Group: "default", Quota: 1000, UsedQuota: 21}
	admin := model.User{Id: 2, Username: "acceptance-runtime-admin", AffCode: "acceptance-runtime-admin", Role: common.RoleAdminUser,
		Status: common.UserStatusEnabled, AuthVersion: 1, Group: "default"}
	require.NoError(t, db.Create(&root).Error)
	require.NoError(t, db.Create(&admin).Error)
	_, rootSession := createCodexLocalRouteSession(t, &root)
	_, adminSession := createCodexLocalRouteSession(t, &admin)
	key := model.Token{Id: 21, UserId: root.Id, Key: "acceptance-runtime-synthetic-api-key", Name: "synthetic-key",
		Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 1000, UsedQuota: 21, Group: "default"}
	require.NoError(t, db.Create(&key).Error)
	var beforeKey model.Token
	require.NoError(t, db.First(&beforeKey, key.Id).Error)

	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/backend-api/wham/rate-limit-reset-credits", r.URL.Path)
		assert.Equal(t, "Bearer acceptance-synthetic-access", r.Header.Get("Authorization"))
		assert.Equal(t, "acceptance-synthetic-account", r.Header.Get("ChatGPT-Account-ID"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"credits":0}`)
	}))
	t.Cleanup(upstream.Close)
	channel := model.Channel{Id: 7, Type: constant.ChannelTypeCodex, Name: "Synthetic acceptance account", Status: common.ChannelStatusEnabled,
		BaseURL: &upstream.URL, Key: `{"access_token":"acceptance-synthetic-access","refresh_token":"acceptance-synthetic-refresh","account_id":"acceptance-synthetic-account"}`,
		Models: "synthetic-text-model", Group: "default"}
	require.NoError(t, db.Create(&channel).Error)

	engine := gin.New()
	// Audit persistence is outside this test; authorization and Origin checks
	// below are the application's real route chain, not injected identities.
	engine.Use(func(c *gin.Context) { common.SetContextKey(c, constant.ContextKeyAuditLogged, true); c.Next() })
	SetApiRouter(engine)
	server := httptest.NewServer(engine)
	t.Cleanup(server.Close)
	client := &http.Client{Timeout: 5 * time.Second}
	const runPath = "/api/upstream-acceptance/runs"
	const creditsPath = "/api/channel/7/codex/usage/reset-credits"
	const createBody = `{"channel_id":7,"limits":{"duration_seconds":120,"max_total":3,"max_auth":0,"max_refresh":0,"max_sample":1,"max_models":0,"max_generation":1}}`

	acceptanceRuntimeRequest(t, client, server.URL, http.MethodPost, runPath, rootSession, server.URL, createBody, http.StatusForbidden)
	assert.Zero(t, upstreamCalls.Load(), "disabled control must not dispatch")
	ordinaryBody := acceptanceRuntimeRequest(t, client, server.URL, http.MethodGet, creditsPath, rootSession, "", "", http.StatusOK)
	var ordinary struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(ordinaryBody, &ordinary))
	require.True(t, ordinary.Success, "default-off mode preserves ordinary Codex requests")
	assert.Equal(t, int32(1), upstreamCalls.Load())
	t.Setenv("MYAPI_UPSTREAM_ACCEPTANCE_ENABLED", "true")
	for _, test := range []struct {
		name, token, origin string
		status              int
	}{
		{"anonymous", "", server.URL, http.StatusUnauthorized},
		{"admin session", adminSession, server.URL, http.StatusForbidden},
		{"root PAT", pat, server.URL, http.StatusUnauthorized},
		{"wrong origin", rootSession, "https://acceptance-attacker.invalid", http.StatusForbidden},
		{"missing origin", rootSession, "", http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			acceptanceRuntimeRequest(t, client, server.URL, http.MethodPost, runPath, test.token, test.origin, createBody, test.status)
		})
	}
	var runsBefore int64
	require.NoError(t, db.Model(&model.UpstreamAcceptanceRun{}).Count(&runsBefore).Error)
	assert.Zero(t, runsBefore)

	createdBody := acceptanceRuntimeRequest(t, client, server.URL, http.MethodPost, runPath, rootSession, server.URL, createBody, http.StatusOK)
	var created struct {
		Success bool                        `json:"success"`
		Data    model.UpstreamAcceptanceRun `json:"data"`
	}
	require.NoError(t, common.Unmarshal(createdBody, &created))
	require.True(t, created.Success)
	require.NotEmpty(t, created.Data.ID)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = service.StopUpstreamAcceptanceRun(ctx, created.Data.ID)
	})
	statusPath := runPath + "/" + created.Data.ID
	acceptanceRuntimeRequest(t, client, server.URL, http.MethodGet, statusPath, pat, "", "", http.StatusUnauthorized)
	acceptanceRuntimeRequest(t, client, server.URL, http.MethodPost, statusPath+"/stop", rootSession, "https://acceptance-attacker.invalid", "{}", http.StatusForbidden)
	acceptanceRuntimeRequest(t, client, server.URL, http.MethodPost, statusPath+"/stop", pat, server.URL, "{}", http.StatusUnauthorized)

	sampleBody := acceptanceRuntimeRequest(t, client, server.URL, http.MethodGet, creditsPath, rootSession, "", "", http.StatusOK)
	var sample struct {
		Success        bool `json:"success"`
		UpstreamStatus int  `json:"upstream_status"`
	}
	require.NoError(t, common.Unmarshal(sampleBody, &sample))
	require.True(t, sample.Success)
	assert.Equal(t, http.StatusOK, sample.UpstreamStatus)
	assert.Equal(t, int32(2), upstreamCalls.Load(), "one ordinary request plus one controlled request")
	// The second real API call reaches the same operation but cannot spend a
	// second sample. Rejection does not add another physical HTTP dispatch.
	deniedBody := acceptanceRuntimeRequest(t, client, server.URL, http.MethodGet, creditsPath, rootSession, "", "", http.StatusOK)
	var denied struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(deniedBody, &denied))
	assert.False(t, denied.Success)
	assert.Equal(t, int32(2), upstreamCalls.Load())

	statusBody := acceptanceRuntimeRequest(t, client, server.URL, http.MethodGet, statusPath, rootSession, "", "", http.StatusOK)
	var status struct {
		Success bool                             `json:"success"`
		Data    service.UpstreamAcceptanceStatus `json:"data"`
	}
	require.NoError(t, common.Unmarshal(statusBody, &status))
	require.True(t, status.Success)
	require.NotNil(t, status.Data.Run)
	assert.True(t, status.Data.OwnerCurrent)
	assert.Equal(t, int64(1), status.Data.Run.Total)
	assert.Equal(t, int64(1), status.Data.Run.Sample)
	assert.Zero(t, status.Data.Operations)
	assert.Zero(t, status.Data.HTTPInFlight)

	stoppedBody := acceptanceRuntimeRequest(t, client, server.URL, http.MethodPost, statusPath+"/stop", rootSession, server.URL, "{}", http.StatusOK)
	require.NoError(t, common.Unmarshal(stoppedBody, &status))
	require.True(t, status.Success)
	require.NotNil(t, status.Data.Run)
	assert.Equal(t, model.UpstreamAcceptanceRunStopped, status.Data.Run.Status)
	assert.False(t, status.Data.CanDispatch)
	assert.False(t, status.Data.DrainError)
	for _, path := range []string{creditsPath, "/api/channel/7/codex/usage"} {
		acceptanceRuntimeRequest(t, client, server.URL, http.MethodGet, path, rootSession, "", "", http.StatusForbidden)
	}
	assert.Equal(t, int32(2), upstreamCalls.Load(), "stop prevents all later sample dispatches")
	var persisted model.UpstreamAcceptanceRun
	require.NoError(t, db.First(&persisted, "id = ?", created.Data.ID).Error)
	assert.Equal(t, int64(1), persisted.Total)
	assert.Equal(t, int64(1), persisted.Sample)
	assert.Zero(t, persisted.Generation)
	assert.Zero(t, persisted.GenerationInFlight)
	assert.Equal(t, model.UpstreamAcceptanceRunStopped, persisted.Status)
	var afterKey model.Token
	require.NoError(t, db.First(&afterKey, key.Id).Error)
	assert.Equal(t, beforeKey, afterKey, "Key identity, credential and accounting values remain unchanged")
	var afterRoot model.User
	require.NoError(t, db.First(&afterRoot, root.Id).Error)
	assert.Equal(t, root.Quota, afterRoot.Quota)
	assert.Equal(t, root.UsedQuota, afterRoot.UsedQuota)
	var afterChannel model.Channel
	require.NoError(t, db.First(&afterChannel, channel.Id).Error)
	assert.Equal(t, channel.Key, afterChannel.Key)
	t.Log("real HTTP control/session/Origin checks, one ordinary and one controlled loopback sample, budget refusal and stopped-run rejection; Key and financial values preserved")
}

func acceptanceRuntimeRequest(t *testing.T, client *http.Client, origin, method, path, token, requestOrigin, body string, expectedStatus int) []byte {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, origin+path, strings.NewReader(body))
	require.NoError(t, err)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if requestOrigin != "" {
		req.Header.Set("Origin", requestOrigin)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	require.NoError(t, err)
	require.Equal(t, expectedStatus, resp.StatusCode, "%s %s: %s", method, path, data)
	if strings.HasPrefix(path, "/api/upstream-acceptance/") {
		assert.Contains(t, resp.Header.Get("Cache-Control"), "no-store")
	}
	return data
}
