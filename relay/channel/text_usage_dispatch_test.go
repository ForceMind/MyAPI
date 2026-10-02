package channel

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrdinaryTextTransportTracksUncertainSuccessAndDisablesReplay(t *testing.T) {
	service.InitHttpClient()
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		_, _ = io.WriteString(w, `not-json`)
	}))
	defer upstream.Close()
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions?trace=fixture", nil)
	request, err := http.NewRequest("POST", upstream.URL, bytes.NewReader([]byte(`{"model":"synthetic"}`)))
	require.NoError(t, err)
	require.NotNil(t, request.GetBody)
	// This transport-only fixture never calls a monetary method or a database.
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}, Billing: &service.BillingSession{}}
	response, err := doRequest(ctx, request, info)
	require.NoError(t, err)
	defer response.Body.Close()
	assert.EqualValues(t, 1, calls.Load())
	assert.Nil(t, request.GetBody)
	assert.True(t, service.TextUsageDispatchNeedsReview(info), "a 200 response is not proof of parseable final usage")
}

func TestOrdinaryTextTransportDoesNotReplayAnAmbiguousIdempotentPost(t *testing.T) {
	service.InitHttpClient()
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/warm" {
			_, _ = io.WriteString(w, "warm")
			return
		}
		calls.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		connection, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			_ = connection.Close()
		}
	}))
	defer upstream.Close()
	// A reused connection plus an idempotency header would otherwise allow the
	// transport to replay a POST whose body may already have been processed.
	response, err := service.GetHttpClient().Get(upstream.URL + "/warm")
	require.NoError(t, err)
	_, _ = io.Copy(io.Discard, response.Body)
	require.NoError(t, response.Body.Close())
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	request, err := http.NewRequest("POST", upstream.URL, bytes.NewReader([]byte(`{"model":"synthetic"}`)))
	require.NoError(t, err)
	request.Header.Set("Idempotency-Key", "synthetic-dispatch")
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}, Billing: &service.BillingSession{}}
	_, err = doRequest(ctx, request, info)
	require.Error(t, err)
	assert.EqualValues(t, 1, calls.Load())
	assert.Nil(t, request.GetBody)
	assert.True(t, service.TextUsageDispatchNeedsReview(info))
}
