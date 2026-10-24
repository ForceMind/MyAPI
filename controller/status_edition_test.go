package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetStatusExposesProductEditionCapabilities(t *testing.T) {
	paymentWebhookTestDB(t)

	capabilitiesFor := func(enabled bool) map[string]any {
		return map[string]any{
			"payments":                 enabled,
			"commercial_subscriptions": enabled,
			"redemption_codes":         enabled,
			"checkin":                  enabled,
			"affiliate":                enabled,
			"public_pricing":           enabled,
			"rankings":                 enabled,
			"perf_metrics":             enabled,
			"external_oauth":           enabled,
			"midjourney":               enabled,
			"model_deployments":        enabled,
			"site_content":             enabled,
			"user_registration":        enabled,
		}
	}

	tests := []struct {
		name         string
		edition      string
		wantEdition  string
		wantFeatures map[string]any
	}{
		{name: "unset falls back to full", edition: "", wantEdition: "full", wantFeatures: capabilitiesFor(true)},
		{name: "full", edition: "full", wantEdition: "full", wantFeatures: capabilitiesFor(true)},
		{name: "lite", edition: "lite", wantEdition: "lite", wantFeatures: capabilitiesFor(false)},
		{name: "legacy lan resolves to lite", edition: "lan", wantEdition: "lite", wantFeatures: capabilitiesFor(false)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("MYAPI_EDITION", tt.edition)
			response := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(response)
			context.Request = httptest.NewRequest(http.MethodGet, "/api/status", nil)

			GetStatus(context)

			var payload struct {
				Success bool           `json:"success"`
				Data    map[string]any `json:"data"`
			}
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &payload))
			require.True(t, payload.Success)
			assert.Equal(t, tt.wantEdition, payload.Data["product_edition"])
			assert.Equal(t, tt.wantFeatures, payload.Data["capabilities"])
			assert.Equal(t, "default", payload.Data["theme"])
		})
	}
}
