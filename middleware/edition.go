package middleware

import (
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

// Distribution editions are intentionally small and independent from the
// frontend build flag.  The backend guard below is the security boundary for
// the LAN distribution; hiding a menu item in React is not sufficient.
const (
	EditionFull = "full"
	EditionLite = "lite"
	// EditionLAN is the legacy MYAPI_EDITION value for the Lite edition.
	EditionLAN = "lan"
)

// EditionProfile is the product edition advertised to clients.
type EditionProfile struct {
	ProductEdition string
	Capabilities   map[string]bool
}

// CurrentEdition returns the normalized product edition: EditionFull or
// EditionLite, with the legacy "lan" value resolving to EditionLite. Unknown
// and empty values fail open to the full edition for backwards compatibility.
func CurrentEdition() string {
	edition := strings.TrimSpace(os.Getenv("MYAPI_EDITION"))
	if strings.EqualFold(edition, EditionLite) || strings.EqualFold(edition, EditionLAN) {
		return EditionLite
	}
	return EditionFull
}

// IsLANEdition reports whether the restricted Lite edition is active, whether
// it was selected as "lite" or through the legacy "lan" value.
func IsLANEdition() bool {
	return CurrentEdition() == EditionLite
}

// CurrentEditionProfile returns the product edition together with the UI
// capabilities it offers. Capabilities describe UI entry points; the backend
// blocking scope is defined by isLANRestrictedPath.
func CurrentEditionProfile() EditionProfile {
	edition := CurrentEdition()
	full := edition == EditionFull
	return EditionProfile{
		ProductEdition: edition,
		Capabilities: map[string]bool{
			"payments":                 full,
			"commercial_subscriptions": full,
			"redemption_codes":         full,
			"checkin":                  full,
			"affiliate":                full,
			"public_pricing":           full,
			"rankings":                 full,
			"perf_metrics":             full,
			"external_oauth":           full,
			"midjourney":               full,
			"model_deployments":        full,
			"site_content":             full,
			"user_registration":        full,
		},
	}
}

// EditionGuard enforces the Lite edition's reduced surface at the HTTP boundary.
// API relay routes, setup, authentication, keys and channel management remain
// available; billing, public registration and external OAuth are disabled.
func EditionGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !IsLANEdition() || !isLANRestrictedPath(c.Request.URL.Path) {
			c.Next()
			return
		}

		c.Header("Cache-Control", "no-store, no-cache, must-revalidate, private, max-age=0")
		c.Header("Pragma", "no-cache")
		c.Header("Expires", "0")
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"success": false,
			"code":    "MYAPI_LAN_ROUTE_DISABLED",
			"message": "This route is not available in MyAPI LAN edition",
		})
	}
}

func isLANRestrictedPath(path string) bool {
	path = strings.TrimSuffix(path, "/")
	if strings.HasPrefix(path, "/api/channel/") && strings.Contains(path, "/codex/local-auth") {
		return true
	}
	// Admin OAuth binding routes include a user id segment
	// (`/api/user/:id/oauth/...`) and therefore cannot be represented by a
	// simple static prefix without disabling every user endpoint.
	if strings.HasPrefix(path, "/api/user/") {
		userPath := strings.TrimPrefix(path, "/api/user/")
		userSegments := strings.Split(userPath, "/")
		if len(userSegments) >= 2 && userSegments[1] == "oauth" {
			return true
		}
	}
	for _, prefix := range []string{
		"/api/pricing",
		"/api/perf-metrics",
		"/api/ratio_config",
		"/api/rankings",
		"/api/subscription",
		"/api/oauth",
		"/api/stripe",
		"/api/creem",
		"/api/waffo",
		"/api/epay",
		"/api/user/register",
		"/api/user/topup",
		"/api/user/pay",
		"/api/user/amount",
		"/api/user/stripe",
		"/api/user/creem",
		"/api/user/waffo",
		"/api/user/epay",
		"/api/user/aff",
		"/api/user/aff_transfer",
		"/api/user/checkin",
		"/api/user/oauth",
		"/api/option/payment_compliance",
		"/api/option/rest_model_ratio",
		"/api/option/waffo-pancake",
		"/api/custom-oauth-provider",
		"/dashboard/billing",
		"/v1/dashboard/billing",
	} {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}
