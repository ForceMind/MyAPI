package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManagementKeyLegacyDashboardAndRelayIngressNeverFallback(t *testing.T) {
	setupDashboardAuthMiddlewareTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.ManagementKey{}, &model.ManagementKeyAudit{}))
	reserved := model.ManagementKeyPrefix + strings.Repeat("a", 64)
	user := createMiddlewarePATUser(t, "management-ingress", reserved)
	require.NoError(t, model.DB.Model(user).Update("role", common.RoleRootUser).Error)
	key := &model.ManagementKey{OwnerID: user.Id, Name: "ingress", Prefix: reserved[:19], SecretHash: model.ManagementKeyDigest(reserved), ScopesJSON: `["system.read"]`, ChannelIDsJSON: `[]`, TokenIDsJSON: `[]`, CreatedAt: time.Now().Unix(), ExpiresAt: time.Now().Unix() + 3600}
	require.NoError(t, model.InsertManagementKey(key))
	called := false
	handler := func(c *gin.Context) { called = true; c.JSON(200, gin.H{"success": true, "data": gin.H{"id": user.Id}}) }
	router := gin.New()
	// Deliberately omit the global boundary: the legacy auth itself must reject.
	router.GET("/api/user/token", UserAuth(), handler)
	router.GET("/optional", TryUserAuth(), handler)
	router.GET("/relay", TokenOrUserAuth(), handler)
	router.GET("/v1/messages", TokenAuth(), handler)
	router.GET("/v1beta/models", TokenAuth(), handler)
	router.GET("/readonly", TokenAuthReadOnly(), handler)
	for _, test := range []struct{ path, header, value string }{
		{"/api/user/token", "Authorization", "Bearer " + reserved},
		{"/optional", "Authorization", "Bearer " + reserved},
		{"/relay", "Authorization", "Bearer " + reserved},
		{"/v1/messages", "Authorization", "Bearer " + reserved},
		{"/v1/messages", "x-api-key", reserved},
		{"/v1beta/models", "x-goog-api-key", reserved},
		{"/v1beta/models?key=" + reserved, "", ""},
		{"/v1/messages", "Sec-WebSocket-Protocol", "realtime, openai-insecure-api-key." + reserved},
		{"/readonly", "Authorization", "Bearer sk-" + reserved},
	} {
		called = false
		request := httptest.NewRequest(http.MethodGet, test.path, nil)
		if test.header != "" {
			request.Header.Set(test.header, test.value)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		assert.False(t, called, test.path)
		assert.GreaterOrEqual(t, response.Code, 400, test.path)
		assert.NotContains(t, response.Body.String(), reserved)
	}
}
