package helper

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestResolveIncomingBillingExprRequestInput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("Content-Type", "application/json")

	body := []byte(`{"service_tier":"fast"}`)
	ctx.Request.Body = io.NopCloser(bytes.NewReader(body))
	ctx.Set(common.KeyRequestBody, body)

	info := &relaycommon.RelayInfo{
		RequestHeaders: map[string]string{"Content-Type": "application/json"},
	}

	input, err := ResolveIncomingBillingExprRequestInput(ctx, info)
	require.NoError(t, err)
	require.Equal(t, body, input.Body)
	require.Equal(t, "application/json", input.Headers["Content-Type"])
}

func TestBuildBillingExprRequestInputFromRequest(t *testing.T) {
	request := &dto.GeneralOpenAIRequest{
		Model:  "gemini-3.1-pro-preview",
		Stream: lo.ToPtr(true),
		Messages: []dto.Message{
			{
				Role:    "user",
				Content: "hi",
			},
		},
		MaxTokens: lo.ToPtr(uint(3000)),
	}

	input, err := BuildBillingExprRequestInputFromRequest(request, map[string]string{
		"Content-Type": "application/json",
		"X-Test":       "1",
	})
	require.NoError(t, err)
	require.Equal(t, "application/json", input.Headers["Content-Type"])
	require.Equal(t, "1", input.Headers["X-Test"])
	require.True(t, gjson.GetBytes(input.Body, "stream").Bool())
	require.Equal(t, "user", gjson.GetBytes(input.Body, "messages.0.role").String())
	require.Equal(t, float64(3000), gjson.GetBytes(input.Body, "max_tokens").Float())
}

func TestModelPriceHelperUsesOneBillingGeneration(t *testing.T) {
	registered := config.GlobalConfig.Get("billing_setting")
	require.NotNil(t, registered)
	baseline, err := config.ConfigToMap(registered)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, config.UpdateConfigFromMap(registered, baseline))
	})

	require.NoError(t, config.UpdateConfigFromMap(registered, map[string]string{
		"billing_mode": `{"snapshot-billing-model":"tiered_expr"}`,
		"billing_expr": `{"snapshot-billing-model":"tier(\"base\", p * 2 + c * 3)"}`,
	}))

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	price, err := ModelPriceHelper(ctx, &relaycommon.RelayInfo{
		OriginModelName: "snapshot-billing-model",
		UsingGroup:      "default",
	}, 100, &types.TokenCountMeta{MaxTokens: 20})

	require.NoError(t, err)
	assert.False(t, price.FreeModel)
	assert.Greater(t, price.QuotaToPreConsume, 0)
}
