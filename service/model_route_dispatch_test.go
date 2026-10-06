package service

import (
	"bytes"
	"context"
	"errors"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/iotest"
)

func TestModelRouteDispatchRejectsPostOverrideModelChange(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("model_route", map[string]any{"upstream_model": "actual-target"})
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "actual-target"}}
	req, err := http.NewRequest(http.MethodPost, "https://synthetic.invalid/v1/responses", bytes.NewBufferString(`{"model":"actual-target","input":"test"}`))
	require.NoError(t, err)
	require.NoError(t, ValidateModelRouteDispatch(c, req, info))
	req, err = http.NewRequest(http.MethodPost, "https://synthetic.invalid/v1/responses", bytes.NewBufferString(`{"model":"other-target"}`))
	require.NoError(t, err)
	require.EqualError(t, ValidateModelRouteDispatch(c, req, info), "model_route_target_changed")
	req.GetBody = nil
	require.EqualError(t, ValidateModelRouteDispatch(c, req, info), "model_route_evidence_unavailable")
}

func TestAssignedModelRouteRejectsAmbiguousModelProof(t *testing.T) {
	for _, body := range []string{
		`{"model":"forbidden-target","Model":"allowed-target"}`,
		`{"model":"allowed-target","model":"forbidden-target","Model":"allowed-target"}`,
		`{"model":"allowed-target","model":"allowed-target"}`,
		`{"model":"forbidden-target","\u006dodel":"allowed-target"}`,
		`{"MODEL":"allowed-target"}`,
	} {
		t.Run(body, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Set("model_route", map[string]any{"upstream_model": "allowed-target"})
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "allowed-target"}}
			req, err := http.NewRequest(http.MethodPost, "https://synthetic.invalid/v1/responses", bytes.NewBufferString(body))
			require.NoError(t, err)
			require.Error(t, ValidateAssignedModelRouteDispatch(c, req, info))
		})
	}
}

func TestAssignedModelRouteProofKeepsExistingOneMiBBound(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("model_route", map[string]any{"upstream_model": "allowed-target"})
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "allowed-target"}}
	prefix, suffix := `{"model":"allowed-target","input":"`, `"}`
	for _, size := range []int{1 << 20, (1 << 20) + 1} {
		body := prefix + strings.Repeat("a", size-len(prefix)-len(suffix)) + suffix
		req, err := http.NewRequest(http.MethodPost, "https://synthetic.invalid/v1/responses", bytes.NewBufferString(body))
		require.NoError(t, err)
		if size == 1<<20 {
			require.NoError(t, ValidateAssignedModelRouteDispatch(c, req, info))
		} else {
			require.Error(t, ValidateAssignedModelRouteDispatch(c, req, info))
			require.NoError(t, ValidateModelRouteDispatch(c, req, info), "unassigned legacy limit is unchanged")
		}
	}
}

type assignedBudgetProofBody struct {
	io.Reader
	closed, readBytes int
	closeErr          error
}

func (body *assignedBudgetProofBody) Read(buffer []byte) (int, error) {
	n, err := body.Reader.Read(buffer)
	body.readBytes += n
	return n, err
}
func (body *assignedBudgetProofBody) Close() error { body.closed++; return body.closeErr }

func TestAssignedModelRouteStrictBudgetBodyPreservesNoReplayAndEvidence(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("model_route", map[string]any{"upstream_model": "allowed-target"})
	info := &relaycommon.RelayInfo{StrictTokenBudget: true, TokenBudgetAudit: map[string]interface{}{"reserved": int64(120), "fee_reserved_usd": "0.00045"}, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "allowed-target"}}
	const payload = `{"model":"allowed-target","input":"unchanged bytes"}`
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://synthetic.invalid/v1/responses", strings.NewReader(payload))
	require.NoError(t, err)
	original := &assignedBudgetProofBody{Reader: strings.NewReader(payload)}
	req.Body, req.GetBody = original, nil
	req.Header.Set("Authorization", "Bearer synthetic-proof-key")
	headers, length := req.Header.Clone(), req.ContentLength
	require.NoError(t, ValidateAssignedModelRouteDispatch(c, req, info))
	assert.Equal(t, 1, original.closed)
	require.Nil(t, req.GetBody, "proof must never re-enable transport POST replay")
	require.NoError(t, ValidateAssignedModelRouteDispatch(c, req, info), "repeat proof still inspects the current unsent bytes")
	require.Nil(t, req.GetBody)
	assert.Equal(t, length, req.ContentLength)
	assert.Equal(t, headers, req.Header)
	assert.Equal(t, ctx, req.Context())
	assert.Equal(t, map[string]interface{}{"reserved": int64(120), "fee_reserved_usd": "0.00045"}, info.TokenBudgetAudit)
	actual, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	assert.Equal(t, payload, string(actual))
	require.NoError(t, req.Body.Close())
}

func TestAssignedModelRouteStrictBudgetBodyErrorsFailClosed(t *testing.T) {
	for _, test := range []struct {
		name, payload     string
		readErr, closeErr error
	}{
		{name: "read error", readErr: errors.New("synthetic proof read failed")},
		{name: "close error", payload: `{"model":"allowed-target"}`, closeErr: errors.New("synthetic proof close failed")},
		{name: "wrong target", payload: `{"model":"forbidden-target"}`},
		{name: "duplicate field", payload: `{"model":"allowed-target","model":"allowed-target"}`},
		{name: "case variant", payload: `{"model":"allowed-target","Model":"allowed-target"}`},
		{name: "oversized", payload: `{"model":"allowed-target","input":"` + strings.Repeat("a", 1<<20) + `"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Set("model_route", map[string]any{"upstream_model": "allowed-target"})
			info := &relaycommon.RelayInfo{StrictTokenBudget: true, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "allowed-target"}}
			req, err := http.NewRequest(http.MethodPost, "https://synthetic.invalid/v1/responses", strings.NewReader(test.payload))
			require.NoError(t, err)
			original := &assignedBudgetProofBody{Reader: strings.NewReader(test.payload), closeErr: test.closeErr}
			if test.readErr != nil {
				original.Reader = iotest.ErrReader(test.readErr)
			}
			req.Body, req.GetBody = original, nil
			length := req.ContentLength
			require.Error(t, ValidateAssignedModelRouteDispatch(c, req, info))
			assert.Equal(t, 1, original.closed)
			assert.LessOrEqual(t, original.readBytes, (1<<20)+1)
			assert.Nil(t, req.GetBody)
			assert.Equal(t, length, req.ContentLength)
		})
	}
}

func TestAssignedModelRouteBodyFallbackIsStrictBudgetOnly(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("model_route", map[string]any{"upstream_model": "allowed-target"})
	req, err := http.NewRequest(http.MethodPost, "https://synthetic.invalid/v1/responses", strings.NewReader(`{"model":"allowed-target"}`))
	require.NoError(t, err)
	original := &assignedBudgetProofBody{Reader: strings.NewReader(`{"model":"allowed-target"}`)}
	req.Body, req.GetBody = original, nil
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "allowed-target"}}
	require.Error(t, ValidateAssignedModelRouteDispatch(c, req, info))
	assert.Zero(t, original.readBytes)
	assert.Zero(t, original.closed)
	info.StrictTokenBudget = true
	require.Error(t, ValidateModelRouteDispatch(c, req, info), "unassigned legacy proof behavior is unchanged")
	assert.Zero(t, original.readBytes)
	assert.Zero(t, original.closed)
	require.NoError(t, original.Close())
}
