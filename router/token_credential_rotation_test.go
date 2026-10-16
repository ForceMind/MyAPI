package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenCredentialRotationIsolatedRoutesRejectBeforeApply(t *testing.T) {
	setupRelayRouterTestDB(t)
	db := model.DB
	require.NoError(t, db.AutoMigrate(&model.UserSession{}, &model.TokenCredentialRotationReceipt{}))
	previousMemory, previousSecret, previousCritical := common.MemoryCacheEnabled, common.SessionSecret, common.CriticalRateLimitEnable
	common.MemoryCacheEnabled, common.CriticalRateLimitEnable = false, false
	common.SessionSecret = "rotation-route-synthetic-secret"
	t.Cleanup(func() {
		common.MemoryCacheEnabled, common.SessionSecret, common.CriticalRateLimitEnable = previousMemory, previousSecret, previousCritical
	})
	pat := "rotation-route-synthetic-pat"
	root := model.User{Id: 1, Username: "rotation-root", AffCode: "rotation-root", Role: common.RoleRootUser, Status: common.UserStatusEnabled, AuthVersion: 1, AccessToken: &pat}
	admin := model.User{Id: 2, Username: "rotation-admin", AffCode: "rotation-admin", Role: common.RoleAdminUser, Status: common.UserStatusEnabled, AuthVersion: 1}
	require.NoError(t, db.Create(&root).Error)
	require.NoError(t, db.Create(&admin).Error)
	identity, rootAccess := createCodexLocalRouteSession(t, &root)
	_, adminAccess := createCodexLocalRouteSession(t, &admin)
	proof, _, err := service.IssueSecurityProof(identity, "2fa", []string{"token.credential_rotation"})
	require.NoError(t, err)
	wrongProof, _, err := service.IssueSecurityProof(identity, "2fa", []string{"token.account_migration"})
	require.NoError(t, err)
	engine := gin.New()
	engine.Use(func(c *gin.Context) { common.SetContextKey(c, constant.ContextKeyAuditLogged, true); c.Next() })
	registerTokenCredentialRotationRoutes(engine.Group("/api"))
	for _, test := range []struct {
		name, access, origin, proof string
		status                      int
	}{
		{"no session", "", "http://rotation.local", proof, http.StatusUnauthorized},
		{"PAT is not browser session", pat, "http://rotation.local", proof, http.StatusUnauthorized},
		{"admin is not Root", adminAccess, "http://rotation.local", proof, http.StatusForbidden},
		{"missing Origin", rootAccess, "", proof, http.StatusForbidden},
		{"cross Origin", rootAccess, "https://attacker.example", proof, http.StatusForbidden},
		{"missing proof", rootAccess, "http://rotation.local", "", http.StatusForbidden},
		{"migration scope is not rotation", rootAccess, "http://rotation.local", wrongProof, http.StatusForbidden},
		{"dedicated proof reaches exact body", rootAccess, "http://rotation.local", proof, http.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "http://rotation.local/api/token/21/credential-rotation", strings.NewReader(`{}`))
			req.Header.Set("Content-Type", "application/json")
			if test.access != "" {
				req.Header.Set("Authorization", "Bearer "+test.access)
			}
			if test.origin != "" {
				req.Header.Set("Origin", test.origin)
			}
			if test.proof != "" {
				req.Header.Set("X-Security-Proof", test.proof)
			}
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, req)
			assert.Equal(t, test.status, recorder.Code)
			assert.Equal(t, "not_accepted", recorder.Header().Get("X-MyAPI-Credential-Rotation-Disposition"))
		})
	}
	var count int64
	require.NoError(t, db.Model(&model.TokenCredentialRotationReceipt{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestTokenCredentialRotationGlobalLimitRejectsBeforeApply(t *testing.T) {
	setupRelayRouterTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.TokenCredentialRotationReceipt{}))
	previousEnabled, previousNum, previousDuration := common.GlobalApiRateLimitEnable, common.GlobalApiRateLimitNum, common.GlobalApiRateLimitDuration
	common.GlobalApiRateLimitEnable, common.GlobalApiRateLimitNum, common.GlobalApiRateLimitDuration = true, 1, 60
	t.Cleanup(func() {
		common.GlobalApiRateLimitEnable, common.GlobalApiRateLimitNum, common.GlobalApiRateLimitDuration = previousEnabled, previousNum, previousDuration
	})
	engine := gin.New()
	engine.Use(func(c *gin.Context) { common.SetContextKey(c, constant.ContextKeyAuditLogged, true); c.Next() })
	SetApiRouter(engine)
	for _, status := range []int{http.StatusUnauthorized, http.StatusTooManyRequests} {
		request := httptest.NewRequest(http.MethodPost, "http://rotation.local/api/token/21/credential-rotation", strings.NewReader(`{}`))
		request.RemoteAddr = "203.0.113.251:12345"
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		assert.Equal(t, status, response.Code)
		assert.Equal(t, "not_accepted", response.Header().Get("X-MyAPI-Credential-Rotation-Disposition"))
	}
	var count int64
	require.NoError(t, model.DB.Model(&model.TokenCredentialRotationReceipt{}).Count(&count).Error)
	assert.Zero(t, count)
}
