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
	EditionLAN  = "lan"
)

// CurrentEdition returns the normalized MYAPI_EDITION value. Unknown and
// empty values fail open to the full edition for backwards compatibility.
func CurrentEdition() string {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("MYAPI_EDITION")), EditionLAN) {
		return EditionLAN
	}
	return EditionFull
}

func IsLANEdition() bool {
	return CurrentEdition() == EditionLAN
}

// EditionGuard enforces LAN edition's reduced surface at the HTTP boundary.
// API relay routes, setup, authentication, keys and channel management remain
// available; billing, public registration and external OAuth are disabled.
func EditionGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !IsLANEdition() || !isLANRestrictedPath(c.Request.URL.Path) {
			c.Next()
			return
		}

		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"success": false,
			"code":    "MYAPI_LAN_ROUTE_DISABLED",
			"message": "This route is not available in MyAPI LAN edition",
		})
	}
}

func isLANRestrictedPath(path string) bool {
	path = strings.TrimSuffix(path, "/")
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
