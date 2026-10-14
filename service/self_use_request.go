package service

import (
	"net/http"

	"github.com/ForceMind/MyAPI/pkg/personalusage"
)

// Shared by early middleware and the frozen-source check before dispatch.
func SupportsSelfUseMeteredRequest(request *http.Request) bool {
	if request == nil || request.URL == nil {
		return false
	}
	return personalusage.SupportsMeteredRequest(request.Method, request.URL.Path, request.URL.RawQuery)
}
