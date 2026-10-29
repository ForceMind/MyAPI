package common

import "net/http"

// UpstreamDiagnosticContextKey carries a request-scoped observer installed by
// the opt-in content logger. Relay code must not install a replay hook or read
// a body a second time for diagnostics.
const UpstreamDiagnosticContextKey = "myapi.upstream_diagnostic"

type UpstreamDiagnosticObserver interface {
	ObserveRequest(req *http.Request, channelID int) func(*http.Response)
}
