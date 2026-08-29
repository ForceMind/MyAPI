package middleware

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestEditionGuardBlocksLANCommercialRoutes(t *testing.T) {
	t.Setenv("MYAPI_EDITION", EditionLAN)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(EditionGuard())
	router.GET("/api/subscription/plans", func(c *gin.Context) { c.Status(http.StatusOK) })

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/subscription/plans", nil))
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected LAN commercial route to be forbidden, got %d", recorder.Code)
	}
}

func TestEditionGuardAllowsLANRelayAndSetup(t *testing.T) {
	t.Setenv("MYAPI_EDITION", EditionLAN)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(EditionGuard())
	router.GET("/api/status", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	router.POST("/v1/chat/completions", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	for _, route := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/status"},
		{http.MethodPost, "/v1/chat/completions"},
	} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(route.method, route.path, nil))
		if recorder.Code != http.StatusNoContent {
			t.Fatalf("expected LAN route %s to be allowed, got %d", route.path, recorder.Code)
		}
	}
}

func TestEditionGuardIsDisabledForFullEdition(t *testing.T) {
	for _, edition := range []string{"", EditionFull, "unknown"} {
		t.Run(edition, func(t *testing.T) {
			if edition == "" {
				os.Unsetenv("MYAPI_EDITION")
			} else {
				t.Setenv("MYAPI_EDITION", edition)
			}
			gin.SetMode(gin.TestMode)
			router := gin.New()
			router.Use(EditionGuard())
			router.GET("/api/pricing", func(c *gin.Context) { c.Status(http.StatusNoContent) })
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/pricing", nil))
			if recorder.Code != http.StatusNoContent {
				t.Fatalf("expected full edition route to be allowed, got %d", recorder.Code)
			}
		})
	}
}
