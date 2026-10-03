package router

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/middleware"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSelfUsePolicyActualRoutesProtectAuthorityAndUnsupportedPaths(t *testing.T) {
	setupRelayRouterTestDB(t)
	require.NoError(t, i18n.Init())
	db := model.DB
	require.NoError(t, db.AutoMigrate(&model.UserUsagePolicyChange{}, &model.Option{}))
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		_, err := model.InitializeUserFundingStateTx(tx, operation_setting.UserFundingModeDisabled)
		return err
	}))
	oldMemory, oldCritical, oldGlobal := common.MemoryCacheEnabled, common.CriticalRateLimitEnable, common.GlobalApiRateLimitEnable
	common.MemoryCacheEnabled, common.CriticalRateLimitEnable, common.GlobalApiRateLimitEnable = false, false, false
	t.Cleanup(func() {
		common.MemoryCacheEnabled, common.CriticalRateLimitEnable, common.GlobalApiRateLimitEnable = oldMemory, oldCritical, oldGlobal
	})
	var ownerID int
	for _, entry := range []struct {
		name string
		role int
	}{{"root", common.RoleRootUser}, {"owner", common.RoleCommonUser}, {"admin", common.RoleAdminUser}, {"outsider", common.RoleCommonUser}} {
		pat := "usage-policy-" + entry.name
		user := model.User{Username: pat, AffCode: pat, Role: entry.role, Status: common.UserStatusEnabled, Quota: 0, AccessToken: &pat}
		require.NoError(t, db.Create(&user).Error)
		if entry.name == "owner" {
			ownerID = user.Id
		}
	}
	key := model.Token{UserId: ownerID, Key: "selfuserelayfixture", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}
	require.NoError(t, db.Create(&key).Error)
	engine := gin.New()
	engine.Use(func(c *gin.Context) { common.SetContextKey(c, constant.ContextKeyAuditLogged, true); c.Next() })
	SetApiRouter(engine)
	calls := 0
	for _, path := range []string{"/v1/chat/completions", "/v1/responses", "/v1/responses/compact", "/mj/submit/imagine", "/v1/audio/speech"} {
		engine.POST(path, middleware.TokenAuth(), func(c *gin.Context) { calls++; c.Status(http.StatusNoContent) })
	}
	request := func(method, path, auth, origin, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "http://myapi.local"+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if auth != "" {
			req.Header.Set("Authorization", "Bearer "+auth)
		}
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		result := httptest.NewRecorder()
		engine.ServeHTTP(result, req)
		return result
	}
	path := fmt.Sprintf("/api/user/%d/usage-policy", ownerID)
	assert.Equal(t, http.StatusUnauthorized, request(http.MethodGet, path, "", "", "").Code)
	assert.Equal(t, http.StatusNotFound, request(http.MethodGet, path, "usage-policy-outsider", "", "").Code)
	initial := request(http.MethodGet, path, "usage-policy-owner", "", "")
	require.Equal(t, http.StatusOK, initial.Code)
	assert.Contains(t, initial.Body.String(), `"no_balance":false`)
	body := `{"id":"` + strings.Repeat("c", 64) + `","expected_revision":0,"no_balance":true,"confirmed":true}`
	for _, actor := range []string{"owner", "admin"} {
		assert.Equal(t, http.StatusForbidden, request(http.MethodPut, path, "usage-policy-"+actor, "http://myapi.local", body).Code)
	}
	for _, origin := range []string{"", "http://evil.invalid"} {
		assert.Equal(t, http.StatusForbidden, request(http.MethodPut, path, "usage-policy-root", origin, body).Code)
	}
	for _, invalid := range []string{strings.Replace(body, `"confirmed":true`, `"confirmed":false`, 1), strings.Replace(body, `"no_balance":true,`, "", 1), strings.Replace(body, `"expected_revision":0`, `"expected_revision":-1`, 1)} {
		assert.Equal(t, http.StatusBadRequest, request(http.MethodPut, path, "usage-policy-root", "http://myapi.local", invalid).Code)
	}
	for range 2 {
		result := request(http.MethodPut, path, "usage-policy-root", "http://myapi.local", body)
		require.Equal(t, http.StatusOK, result.Code, result.Body.String())
		assert.Contains(t, result.Header().Get("Cache-Control"), "no-store")
	}
	var audits int64
	require.NoError(t, db.Model(&model.UserUsagePolicyChange{}).Count(&audits).Error)
	assert.EqualValues(t, 1, audits)
	for _, allowed := range []string{"/v1/chat/completions", "/v1/responses", "/v1/responses/compact"} {
		assert.Equal(t, http.StatusNoContent, request(http.MethodPost, allowed, "sk-selfuserelayfixture", "", "{}").Code)
	}
	for _, blocked := range []string{"/mj/submit/imagine", "/v1/audio/speech", "/v1/responses?unknown=1"} {
		result := request(http.MethodPost, blocked, "sk-selfuserelayfixture", "", "{}")
		assert.Equal(t, http.StatusBadRequest, result.Code)
		assert.Contains(t, result.Body.String(), "self_use_unsupported_request")
	}
	assert.Equal(t, 3, calls, "unsupported paths stop before handler or upstream work")
	owner, err := model.GetUserById(ownerID, false)
	require.NoError(t, err)
	assert.Zero(t, owner.Quota)
	active, err := model.ResolveSelfUseNoBalanceAdmission(context.Background(), db, ownerID)
	require.NoError(t, err)
	assert.True(t, active)
}
