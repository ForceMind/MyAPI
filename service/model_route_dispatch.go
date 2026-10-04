package service

import (
	"errors"
	"io"
	"net/http"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/gin-gonic/gin"
)

// ValidateModelRouteDispatch checks the final post-override body, not just the
// caller DTO. It never sends a request, counts tokens or consumes quota.
func ValidateModelRouteDispatch(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	value, exists := c.Get("model_route")
	if !exists || value == nil {
		return nil
	}
	route, ok := value.(map[string]any)
	if !ok || info == nil || req == nil || req.GetBody == nil {
		return errors.New("model_route_evidence_unavailable")
	}
	expected, ok := route["upstream_model"].(string)
	if !ok || expected == "" || info.UpstreamModelName != expected {
		return errors.New("model_route_target_changed")
	}
	reader, err := req.GetBody()
	if err != nil {
		return errors.New("model_route_evidence_unavailable")
	}
	defer reader.Close()
	maxMB := constant.MaxRequestBodyMB
	if maxMB <= 0 {
		maxMB = 128
	}
	body, err := io.ReadAll(io.LimitReader(reader, int64(maxMB)*1024*1024+1))
	if err != nil || int64(len(body)) > int64(maxMB)*1024*1024 {
		return errors.New("model_route_evidence_unavailable")
	}
	var payload struct {
		Model string `json:"model"`
	}
	if err := common.Unmarshal(body, &payload); err != nil || payload.Model != expected {
		return errors.New("model_route_target_changed")
	}
	return nil
}
