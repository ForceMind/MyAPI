package controller

import (
	"errors"
	"net/http"
	"testing"

	"github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/stretchr/testify/assert"
)

func TestUsageDispatchGuardErrorCannotBeForgedByUpstream(t *testing.T) {
	local := types.NewErrorWithStatusCode(errors.New("synthetic"), types.ErrorCode("usage_dispatch_unresolved"), http.StatusServiceUnavailable, types.ErrOptionWithSkipRetry())
	assert.True(t, isTextUsageDispatchGuardError(local))
	for _, status := range []int{http.StatusTooManyRequests, http.StatusServiceUnavailable} {
		provider := types.WithOpenAIError(types.OpenAIError{Code: "usage_dispatch_unresolved", Type: "usage_limit_error", Message: "synthetic"}, status, types.ErrOptionWithSkipRetry())
		assert.False(t, isTextUsageDispatchGuardError(provider), "an upstream lookalike must still reach normal 429/error processing")
	}
	withoutLocalFlag := types.NewErrorWithStatusCode(errors.New("synthetic"), types.ErrorCode("usage_dispatch_unresolved"), http.StatusServiceUnavailable)
	assert.False(t, isTextUsageDispatchGuardError(withoutLocalFlag))
}
