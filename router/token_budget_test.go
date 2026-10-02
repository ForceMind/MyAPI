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
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenBudgetActualRoutesAndUnlimitedKeyAdmission(t *testing.T) {
	setupRelayRouterTestDB(t)
	require.NoError(t, i18n.Init())
	db := model.DB
	require.NoError(t, db.AutoMigrate(&model.TokenBudgetReservation{}, &model.TokenBudgetPolicyChange{}))
	oldMemory, oldCritical, oldGlobal := common.MemoryCacheEnabled, common.CriticalRateLimitEnable, common.GlobalApiRateLimitEnable
	common.MemoryCacheEnabled, common.CriticalRateLimitEnable, common.GlobalApiRateLimitEnable = false, false, false
	t.Cleanup(func() {
		common.MemoryCacheEnabled, common.CriticalRateLimitEnable, common.GlobalApiRateLimitEnable = oldMemory, oldCritical, oldGlobal
	})
	var rootID, ownerID int
	for _, entry := range []struct {
		name string
		role int
	}{{"root", common.RoleRootUser}, {"owner", common.RoleCommonUser}, {"admin", common.RoleAdminUser}, {"outsider", common.RoleCommonUser}} {
		pat := "budget-pat-" + entry.name
		user := model.User{Username: pat, AffCode: pat, Role: entry.role, Status: common.UserStatusEnabled, Quota: 10000, AccessToken: &pat}
		require.NoError(t, db.Create(&user).Error)
		if entry.name == "root" {
			rootID = user.Id
		}
		if entry.name == "owner" {
			ownerID = user.Id
		}
	}
	key := model.Token{UserId: ownerID, Key: "budgetrelayfixture", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}
	require.NoError(t, db.Create(&key).Error)
	engine := gin.New()
	engine.Use(func(c *gin.Context) { common.SetContextKey(c, constant.ContextKeyAuditLogged, true); c.Next() })
	SetApiRouter(engine)
	var generations int
	for _, path := range []string{"/v1/responses", "/v1/chat/completions", "/mj/submit/imagine"} {
		engine.POST(path, middleware.TokenAuth(), func(c *gin.Context) {
			assert.True(t, common.GetContextKeyBool(c, constant.ContextKeyStrictTokenBudget))
			generations++
			c.Status(http.StatusNoContent)
		})
	}
	engine.GET("/v1/models", middleware.TokenAuth(), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	request := func(method, path, auth, origin, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "http://myapi.local"+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if auth != "" {
			req.Header.Set("Authorization", "Bearer "+auth)
		}
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, req)
		return response
	}
	path := fmt.Sprintf("/api/token/%d/budget", key.Id)
	assert.Equal(t, http.StatusUnauthorized, request(http.MethodGet, path, "", "", "").Code)
	assert.Equal(t, http.StatusNotFound, request(http.MethodGet, path, "budget-pat-outsider", "", "").Code)
	assert.Equal(t, http.StatusOK, request(http.MethodGet, path, "budget-pat-owner", "", "").Code)
	body := `{"id":"` + strings.Repeat("a", 64) + `","expected_revision":0,"enabled":true,"limit":100,"confirmed":true}`
	for _, actor := range []string{"owner", "admin"} {
		assert.Equal(t, http.StatusForbidden, request(http.MethodPut, path, "budget-pat-"+actor, "http://myapi.local", body).Code)
		assert.Equal(t, http.StatusForbidden, request(http.MethodPost, path+"/recover", "budget-pat-"+actor, "http://myapi.local", "{}").Code)
	}
	for _, origin := range []string{"", "http://evil.invalid"} {
		assert.Equal(t, http.StatusForbidden, request(http.MethodPut, path, "budget-pat-root", origin, body).Code)
		assert.Equal(t, http.StatusForbidden, request(http.MethodPost, path+"/recover", "budget-pat-root", origin, "{}").Code)
	}
	assert.Equal(t, http.StatusBadRequest, request(http.MethodPut, path, "budget-pat-root", "http://myapi.local", strings.Replace(body, `"confirmed":true`, `"confirmed":false`, 1)).Code)
	var audits int64
	require.NoError(t, db.Model(&model.TokenBudgetPolicyChange{}).Count(&audits).Error)
	assert.Zero(t, audits)
	for range 2 {
		response := request(http.MethodPut, path, "budget-pat-root", "http://myapi.local", body)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		assert.Contains(t, response.Header().Get("Cache-Control"), "no-store")
	}
	require.NoError(t, db.Model(&model.TokenBudgetPolicyChange{}).Count(&audits).Error)
	assert.EqualValues(t, 1, audits)
	assert.Equal(t, http.StatusNoContent, request(http.MethodPost, "/v1/responses", "sk-budgetrelayfixture", "", "{}").Code)
	for _, unsupported := range []string{"/v1/chat/completions", "/mj/submit/imagine"} {
		assert.Equal(t, http.StatusBadRequest, request(http.MethodPost, unsupported, "sk-budgetrelayfixture", "", "{}").Code)
	}
	require.NoError(t, model.ReserveTokenBudget(context.Background(), db, model.TokenBudgetReservation{RequestID: "route-budget-request", TokenID: key.Id, UserID: ownerID, ChannelID: 7, ModelName: "fixture", InputTokens: 10, MaxOutputTokens: 20, PayloadSHA256: strings.Repeat("b", 64), BoundSource: model.TokenBudgetBoundOpenAIResponses}))
	assert.Equal(t, http.StatusConflict, request(http.MethodPost, "/v1/responses", "sk-budgetrelayfixture", "", "{}").Code)
	assert.Equal(t, http.StatusNoContent, request(http.MethodGet, "/v1/models", "sk-budgetrelayfixture", "", "").Code)
	_, err := model.MutateTokenBudgetRequest(context.Background(), db, model.TokenBudgetMutation{TokenID: key.Id, RequestID: "route-budget-request", Action: "send"})
	require.NoError(t, err)
	_, err = model.MutateTokenBudgetRequest(context.Background(), db, model.TokenBudgetMutation{TokenID: key.Id, RequestID: "route-budget-request", Action: "settle", Input: 10, Output: 20})
	require.NoError(t, err)
	policy, err := model.LookupTokenBudget(context.Background(), db, key.Id)
	require.NoError(t, err)
	_, err = model.ConfigureTokenBudget(context.Background(), db, rootID, model.TokenBudgetPolicyInput{ID: strings.Repeat("c", 64), TokenID: key.Id, ExpectedRevision: policy.Revision, Enabled: true, Limit: 30})
	require.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, request(http.MethodPost, "/v1/responses", "sk-budgetrelayfixture", "", "{}").Code)
	assert.Equal(t, 1, generations, "unlimited legacy quota and alternate relay paths cannot bypass the new budget")
}
