package service

import "net/http"

// Shared by early middleware and the frozen-source check before dispatch.
func SupportsSelfUseMeteredRequest(request *http.Request) bool {
	if request == nil || request.URL == nil || request.Method != http.MethodPost || request.URL.RawQuery != "" {
		return false
	}
	switch request.URL.Path {
	case "/v1/chat/completions", "/v1/responses", "/v1/responses/compact":
		return true
	default:
		return false
	}
}
