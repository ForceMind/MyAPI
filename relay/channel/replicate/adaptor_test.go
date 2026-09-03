package replicate

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestConvertImageRequestOutputFormat(t *testing.T) {
	tests := []struct{ name, raw, want string }{
		{"string", `" png "`, " png "}, {"empty", `""`, ""}, {"null", `null`, ""},
		{"number", `1`, ""}, {"boolean", `false`, ""}, {"array", `[]`, ""},
		{"object", `{}`, ""}, {"malformed", `{`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			result, err := (&Adaptor{}).ConvertImageRequest(c, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}, dto.ImageRequest{Prompt: "p", OutputFormat: []byte(tt.raw)})
			require.NoError(t, err)
			payload, ok := result.(map[string]any)
			require.True(t, ok)
			input, ok := payload["input"].(map[string]any)
			require.True(t, ok)
			if tt.want == "" {
				_, ok := input["output_format"]
				require.False(t, ok)
			} else {
				require.Equal(t, tt.want, input["output_format"])
			}
		})
	}
}

func TestConvertImageRequestExtraPrecedence(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	result, err := (&Adaptor{}).ConvertImageRequest(c, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}, dto.ImageRequest{Prompt: "p", OutputFormat: []byte(`"png"`), ExtraFields: []byte(`{"output_format":"jpeg"}`), Extra: map[string]json.RawMessage{"input": []byte(`{"output_format":"webp"}`)}})
	require.NoError(t, err)
	payload, ok := result.(map[string]any)
	require.True(t, ok)
	input, ok := payload["input"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "webp", input["output_format"])
}
