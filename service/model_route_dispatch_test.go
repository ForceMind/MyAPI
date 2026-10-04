package service

import (
	"bytes"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
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
