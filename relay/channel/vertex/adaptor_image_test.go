package vertex

import (
	"net/http/httptest"
	"testing"

	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func vertexImageContext(t *testing.T) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	return c
}

func vertexImagenInfo() *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "imagen-3"}}
}

func TestConvertOpenAIRequestImageNBounds(t *testing.T) {
	valid := dto.MaxImageN
	zero := 0
	cases := []struct {
		name      string
		n         *int
		wantCount int
		wantError bool
	}{
		{name: "default", wantCount: 1},
		{name: "zero uses default", n: &zero, wantCount: 1},
		{name: "maximum", n: &valid, wantCount: dto.MaxImageN},
		{name: "negative rejected", n: func() *int { n := -1; return &n }(), wantError: true},
		{name: "over maximum rejected", n: func() *int { n := dto.MaxImageN + 1; return &n }(), wantError: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request := &dto.GeneralOpenAIRequest{Model: "imagen-3", Prompt: "a tree", N: tc.n}
			converted, err := (&Adaptor{RequestMode: RequestModeGemini}).ConvertOpenAIRequest(vertexImageContext(t), vertexImagenInfo(), request)
			if tc.wantError {
				require.EqualError(t, err, "n must be an integer between 1 and 128")
				return
			}
			require.NoError(t, err)
			imageRequest, ok := converted.(dto.GeminiImageRequest)
			require.True(t, ok)
			require.Equal(t, tc.wantCount, imageRequest.Parameters.SampleCount)
		})
	}
}

func TestConvertOpenAIRequestImageExtraNRejectsInvalidValues(t *testing.T) {
	for _, extraBody := range []string{
		`{"n":-1}`,
		`{"n":129}`,
		`{"n":1.5}`,
	} {
		t.Run(extraBody, func(t *testing.T) {
			request := &dto.GeneralOpenAIRequest{Model: "imagen-3", Prompt: "a tree", ExtraBody: []byte(extraBody)}
			_, err := (&Adaptor{RequestMode: RequestModeGemini}).ConvertOpenAIRequest(vertexImageContext(t), vertexImagenInfo(), request)
			require.EqualError(t, err, "n must be an integer between 1 and 128")
		})
	}
}
