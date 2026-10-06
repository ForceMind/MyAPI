package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestRecentLogOverviewRoutesRequireAuthenticationAndDisableCache(t *testing.T) {
	previousMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(previousMode) })
	engine := gin.New()
	SetApiRouter(engine)

	for _, path := range []string{"/api/log/overview", "/api/log/self/overview"} {
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		assert.Equal(t, http.StatusUnauthorized, response.Code, path)
		assert.Contains(t, response.Header().Get("Cache-Control"), "no-store", path)
	}
}
