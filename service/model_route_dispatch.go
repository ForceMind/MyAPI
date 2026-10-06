package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/gin-gonic/gin"
)

// ValidateModelRouteDispatch checks the final post-override body, not just the
// caller DTO. It never sends a request, counts tokens or consumes quota.
func ValidateModelRouteDispatch(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	return validateModelRouteDispatch(c, req, info, false)
}

// ValidateAssignedModelRouteDispatch requires an unambiguous, exact target in
// the bytes being sent. Struct decoding alone accepts case-insensitive keys
// and last-wins duplicates which can disagree with a provider's JSON parser.
// The existing canonical validator bounds assigned target proof to 1 MiB;
// larger evidence is rejected, never truncated or silently treated as inherit.
func ValidateAssignedModelRouteDispatch(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	return validateModelRouteDispatch(c, req, info, true)
}

func validateModelRouteDispatch(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo, strict bool) error {
	value, exists := c.Get("model_route")
	if !exists || value == nil {
		return nil
	}
	route, ok := value.(map[string]any)
	if !ok || info == nil || req == nil {
		return errors.New("model_route_evidence_unavailable")
	}
	expected, ok := route["upstream_model"].(string)
	if !ok || expected == "" || info.UpstreamModelName != expected {
		return errors.New("model_route_target_changed")
	}
	// Strict budget preparation freezes the exact unsent bytes and deliberately
	// removes GetBody. Inspect those bytes without restoring transport replay.
	budgetFrozenBody := strict && info.StrictTokenBudget && req.GetBody == nil
	var reader io.ReadCloser
	var err error
	if budgetFrozenBody {
		reader = req.Body
	} else if req.GetBody != nil {
		reader, err = req.GetBody()
	}
	if err != nil || reader == nil {
		return errors.New("model_route_evidence_unavailable")
	}
	maxMB := constant.MaxRequestBodyMB
	if maxMB <= 0 {
		maxMB = 128
	}
	limit := int64(maxMB) * 1024 * 1024
	if strict && limit > 1<<20 {
		limit = 1 << 20
	}
	body, err := io.ReadAll(io.LimitReader(reader, limit+1))
	closeErr := reader.Close()
	if err != nil || strict && closeErr != nil || int64(len(body)) > limit {
		return errors.New("model_route_evidence_unavailable")
	}
	if strict {
		if _, err := common.CanonicalJSONObjectDigest(body); err != nil {
			return errors.New("model_route_evidence_unavailable")
		}
		var fields map[string]json.RawMessage
		if err := common.Unmarshal(body, &fields); err != nil {
			return errors.New("model_route_evidence_unavailable")
		}
		for key := range fields {
			if key != "model" && strings.EqualFold(key, "model") {
				return errors.New("model_route_target_changed")
			}
		}
		var actual string
		if err := common.Unmarshal(fields["model"], &actual); err != nil || actual != expected {
			return errors.New("model_route_target_changed")
		}
		if budgetFrozenBody {
			req.Body = io.NopCloser(bytes.NewReader(body))
			// Keep ContentLength, context, headers and frozen budget evidence intact.
			// GetBody stays nil, including after a successful repeated proof check.
		}
		return nil
	}
	var payload struct {
		Model string `json:"model"`
	}
	if err := common.Unmarshal(body, &payload); err != nil || payload.Model != expected {
		return errors.New("model_route_target_changed")
	}
	return nil
}
