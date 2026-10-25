package middleware

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEditionGuardResolvesEditionValues(t *testing.T) {
	editions := []struct {
		value      string
		restricted bool
	}{
		{value: "full", restricted: false},
		{value: "lite", restricted: true},
		{value: "LITE", restricted: true},
		{value: " lan ", restricted: true},
		{value: "lan", restricted: true},
		{value: "", restricted: false},
		{value: "litee", restricted: false},
	}
	paths := []struct {
		method      string
		path        string
		blockedLite bool
	}{
		{method: http.MethodGet, path: "/api/pricing", blockedLite: true},
		{method: http.MethodGet, path: "/api/subscription", blockedLite: true},
		// Lite keeps the caller's own subscriptions readable because quota
		// reset and subscription billing keep running in every edition.
		{method: http.MethodGet, path: "/api/subscription/self", blockedLite: false},
		{method: http.MethodGet, path: "/api/subscription/self/", blockedLite: false},
		{method: http.MethodPost, path: "/api/subscription/self", blockedLite: true},
		{method: http.MethodPut, path: "/api/subscription/self", blockedLite: true},
		{method: http.MethodDelete, path: "/api/subscription/self", blockedLite: true},
		{method: http.MethodHead, path: "/api/subscription/self", blockedLite: true},
		{method: http.MethodGet, path: "/api/subscription/self/preference", blockedLite: true},
		{method: http.MethodPut, path: "/api/subscription/self/preference", blockedLite: true},
		{method: http.MethodGet, path: "/api/subscription/plans", blockedLite: true},
		{method: http.MethodPost, path: "/api/subscription/balance/pay", blockedLite: true},
		{method: http.MethodGet, path: "/api/subscription/admin/plans", blockedLite: true},
		{method: http.MethodGet, path: "/api/subscription/epay/notify", blockedLite: true},
		{method: http.MethodGet, path: "/api/subscription-share/x", blockedLite: false},
		{method: http.MethodGet, path: "/api/subscription-share/channels/1/pools", blockedLite: false},
		{method: http.MethodPost, path: "/api/subscription-share/pools/1/close", blockedLite: false},
		{method: http.MethodPut, path: "/api/token/1/subscription-share", blockedLite: false},
		{method: http.MethodGet, path: "/api/user/topup", blockedLite: true},
		{method: http.MethodGet, path: "/api/redemption", blockedLite: true},
		{method: http.MethodGet, path: "/api/redemption/", blockedLite: true},
		{method: http.MethodGet, path: "/api/redemption/search", blockedLite: true},
		{method: http.MethodDelete, path: "/api/redemption/1", blockedLite: true},
		{method: http.MethodGet, path: "/api/deployments", blockedLite: true},
		{method: http.MethodGet, path: "/api/deployments/settings", blockedLite: true},
		{method: http.MethodPost, path: "/api/deployments/1/extend", blockedLite: true},
		{method: http.MethodGet, path: "/api/mj/self", blockedLite: true},
		{method: http.MethodGet, path: "/api/mj/", blockedLite: true},
		{method: http.MethodGet, path: "/mj/image/1", blockedLite: true},
		{method: http.MethodPost, path: "/mj/submit/imagine", blockedLite: true},
		{method: http.MethodGet, path: "/mj/task/1/fetch", blockedLite: true},
		{method: http.MethodPost, path: "/fast/mj/submit/imagine", blockedLite: true},
		{method: http.MethodGet, path: "/relax/mj/task/1/fetch", blockedLite: true},
		{method: http.MethodGet, path: "/turbo/mj/image/1", blockedLite: true},
		{method: http.MethodGet, path: "/api/mjx", blockedLite: false},
		{method: http.MethodPost, path: "/mjx/submit/imagine", blockedLite: false},
		{method: http.MethodGet, path: "/api/redemptions", blockedLite: false},
		{method: http.MethodGet, path: "/api/deployments-history", blockedLite: false},
		{method: http.MethodGet, path: "/api/channel/1/codex/local-auth", blockedLite: true},
		{method: http.MethodGet, path: "/api/channel/", blockedLite: false},
		{method: http.MethodPut, path: "/api/channel/1", blockedLite: false},
		{method: http.MethodPost, path: "/v1/chat/completions", blockedLite: false},
		{method: http.MethodGet, path: "/v1/models", blockedLite: false},
		{method: http.MethodPost, path: "/v1/images/generations", blockedLite: false},
		{method: http.MethodPost, path: "/suno/submit/music", blockedLite: false},
	}

	gin.SetMode(gin.TestMode)
	for _, edition := range editions {
		for _, route := range paths {
			t.Run("edition="+edition.value+" "+route.method+" "+route.path, func(t *testing.T) {
				t.Setenv("MYAPI_EDITION", edition.value)
				router := gin.New()
				router.Use(EditionGuard())
				router.Handle(route.method, route.path, func(c *gin.Context) { c.Status(http.StatusNoContent) })

				recorder := httptest.NewRecorder()
				router.ServeHTTP(recorder, httptest.NewRequest(route.method, route.path, nil))

				if !(edition.restricted && route.blockedLite) {
					assert.Equal(t, http.StatusNoContent, recorder.Code)
					return
				}
				require.Equal(t, http.StatusForbidden, recorder.Code)
				var body map[string]any
				require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
				assert.Equal(t, map[string]any{
					"success": false,
					"code":    "MYAPI_LAN_ROUTE_DISABLED",
					"message": "This route is not available in MyAPI LAN edition",
				}, body)
				assert.Contains(t, recorder.Header().Get("Cache-Control"), "no-store")
			})
		}
	}
}

func TestValidateEditionRejectsUnknownValues(t *testing.T) {
	tests := []struct {
		value   string
		wantErr bool
	}{
		{value: "", wantErr: false},
		{value: "   ", wantErr: false},
		{value: "full", wantErr: false},
		{value: "FULL", wantErr: false},
		{value: "lite", wantErr: false},
		{value: " Lite ", wantErr: false},
		{value: "lan", wantErr: false},
		{value: "LAN", wantErr: false},
		{value: "litee", wantErr: true},
		{value: "lan-lite", wantErr: true},
		{value: "desktop", wantErr: true},
		{value: "full,lite", wantErr: true},
	}
	for _, tt := range tests {
		t.Run("edition="+tt.value, func(t *testing.T) {
			t.Setenv("MYAPI_EDITION", tt.value)
			err := ValidateEdition()
			if !tt.wantErr {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Equal(t, `unsupported MYAPI_EDITION "`+strings.TrimSpace(tt.value)+`": use full, lite, or the legacy alias lan`, err.Error())
		})
	}
}

func TestCurrentEditionProfile(t *testing.T) {
	capabilitiesFor := func(enabled bool) map[string]bool {
		return map[string]bool{
			"payments":                 enabled,
			"commercial_subscriptions": enabled,
			"redemption_codes":         enabled,
			"checkin":                  enabled,
			"affiliate":                enabled,
			"public_pricing":           enabled,
			"rankings":                 enabled,
			"perf_metrics":             enabled,
			"external_oauth":           enabled,
			"midjourney":               enabled,
			"model_deployments":        enabled,
			"site_content":             enabled,
			"user_registration":        enabled,
		}
	}
	tests := []struct {
		value string
		want  EditionProfile
	}{
		{value: "full", want: EditionProfile{ProductEdition: EditionFull, Capabilities: capabilitiesFor(true)}},
		{value: "", want: EditionProfile{ProductEdition: EditionFull, Capabilities: capabilitiesFor(true)}},
		{value: "litee", want: EditionProfile{ProductEdition: EditionFull, Capabilities: capabilitiesFor(true)}},
		{value: "lite", want: EditionProfile{ProductEdition: EditionLite, Capabilities: capabilitiesFor(false)}},
		{value: "LITE", want: EditionProfile{ProductEdition: EditionLite, Capabilities: capabilitiesFor(false)}},
		{value: " lan ", want: EditionProfile{ProductEdition: EditionLite, Capabilities: capabilitiesFor(false)}},
		{value: "lan", want: EditionProfile{ProductEdition: EditionLite, Capabilities: capabilitiesFor(false)}},
	}
	for _, tt := range tests {
		t.Run("edition="+tt.value, func(t *testing.T) {
			t.Setenv("MYAPI_EDITION", tt.value)
			assert.Equal(t, tt.want, CurrentEditionProfile())
			assert.Equal(t, tt.want.ProductEdition == EditionLite, IsLANEdition())
		})
	}
}

func TestEditionGuardBlocksLANCommercialRoutes(t *testing.T) {
	t.Setenv("MYAPI_EDITION", EditionLAN)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(EditionGuard())
	for _, route := range []string{
		"/api/subscription/plans",
		"/api/perf-metrics/summary",
		"/api/ratio_config",
		"/api/option/payment_compliance",
		"/api/custom-oauth-provider",
		"/v1/dashboard/billing/subscription",
		"/api/user/aff_transfer",
		"/api/user/oauth/bindings",
		"/api/user/42/oauth/bindings",
		"/api/channel/codex/local-auth/status",
		"/api/channel/codex/local-auth/import",
		"/api/channel/42/codex/local-auth/import",
	} {
		router.Any(route, func(c *gin.Context) { c.Status(http.StatusOK) })
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, route, nil))
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("expected LAN commercial route %s to be forbidden, got %d", route, recorder.Code)
		}
		if cacheControl := recorder.Header().Get("Cache-Control"); !strings.Contains(cacheControl, "no-store") {
			t.Fatalf("expected LAN commercial route %s to disable caching, got %q", route, cacheControl)
		}
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
