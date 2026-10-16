package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/middleware"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenMaintenanceActualRoutesEnforceSessionOriginAndProof(t *testing.T) {
	setupRelayRouterTestDB(t)
	db := model.DB
	require.NoError(t, db.AutoMigrate(&model.UserSession{}, &model.TokenAccountMigrationReceipt{}))
	previousMemory, previousSecret := common.MemoryCacheEnabled, common.SessionSecret
	previousCritical := common.CriticalRateLimitEnable
	common.MemoryCacheEnabled, common.CriticalRateLimitEnable = false, false
	common.SessionSecret = "maintenance-route-synthetic-secret"
	t.Cleanup(func() {
		common.MemoryCacheEnabled, common.SessionSecret = previousMemory, previousSecret
		common.CriticalRateLimitEnable = previousCritical
	})
	pat := "maintenance-route-synthetic-pat"
	root := model.User{Id: 1, Username: "maintenance-root", AffCode: "maintenance-root", Role: common.RoleRootUser, Status: common.UserStatusEnabled, AuthVersion: 1, AccessToken: &pat}
	admin := model.User{Id: 2, Username: "maintenance-admin", AffCode: "maintenance-admin", Role: common.RoleAdminUser, Status: common.UserStatusEnabled, AuthVersion: 1}
	require.NoError(t, db.Create(&root).Error)
	require.NoError(t, db.Create(&admin).Error)
	identity, rootToken := createCodexLocalRouteSession(t, &root)
	_, adminToken := createCodexLocalRouteSession(t, &admin)
	wrongProof, _, err := service.IssueSecurityProof(identity, "2fa", []string{"channel.key.read"})
	require.NoError(t, err)
	proof, _, err := service.IssueSecurityProof(identity, "2fa", []string{"token.account_migration"})
	require.NoError(t, err)
	engine := gin.New()
	// This fixture owns only route authorization; background audit workers
	// must not outlive its temporary global database (audit has separate tests).
	engine.Use(func(c *gin.Context) { common.SetContextKey(c, constant.ContextKeyAuditLogged, true); c.Next() })
	routes := engine.Group("/api/token", middleware.UserAuth())
	registerTokenMaintenanceRoutes(routes)
	request := func(method, path, accessToken, origin, securityProof string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "http://maintenance.local"+path, strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		if accessToken != "" {
			req.Header.Set("Authorization", "Bearer "+accessToken)
		}
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		if securityProof != "" {
			req.Header.Set("X-Security-Proof", securityProof)
		}
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, req)
		return recorder
	}
	assert.Equal(t, http.StatusUnauthorized, request(http.MethodGet, "/api/token/21/maintenance", "", "", "").Code)
	assert.Equal(t, http.StatusUnauthorized, request(http.MethodGet, "/api/token/21/maintenance", pat, "", "").Code)
	assert.Equal(t, http.StatusForbidden, request(http.MethodGet, "/api/token/21/maintenance", adminToken, "", "").Code)
	for _, path := range []string{"/api/token/21/account-migration/preview", "/api/token/21/account-migration"} {
		for _, origin := range []string{"", "https://attacker.example"} {
			response := request(http.MethodPost, path, rootToken, origin, proof)
			assert.Equal(t, http.StatusForbidden, response.Code)
			assert.Contains(t, response.Body.String(), "AUTH_ORIGIN_FORBIDDEN")
		}
		response := request(http.MethodPost, path, pat, "http://maintenance.local", proof)
		assert.Equal(t, http.StatusUnauthorized, response.Code)
	}
	for _, invalidProof := range []string{"", wrongProof} {
		response := request(http.MethodPost, "/api/token/21/account-migration", rootToken, "http://maintenance.local", invalidProof)
		assert.Equal(t, http.StatusForbidden, response.Code)
		assert.Contains(t, response.Body.String(), "SECURITY_PROOF_")
		assert.Equal(t, "no-store", response.Header().Get("Cache-Control"))
	}
	response := request(http.MethodPost, "/api/token/21/account-migration", rootToken, "http://maintenance.local", proof)
	assert.Equal(t, http.StatusBadRequest, response.Code, "a valid dedicated proof reaches strict input validation")
	for _, path := range []string{"/api/token/21/rotation", "/api/token/21/key/rotate", "/api/token/21/maintenance/rotation", "/api/token/21/credential-rotation"} {
		assert.Equal(t, http.StatusNotFound, request(http.MethodPost, path, rootToken, "http://maintenance.local", proof).Code)
	}
	full := gin.New()
	SetApiRouter(full)
	for _, route := range full.Routes() {
		assert.False(t, strings.HasPrefix(route.Path, "/api/token/") && strings.Contains(route.Path, "rotat"), "the actual application must expose no rotation submit route")
	}
}
