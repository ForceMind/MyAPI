package gemini

import (
	"net/http/httptest"
	"testing"

	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestConvertImageRequestRejectsOversizedSampleCount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	tooMany := uint(dto.MaxImageN + 1)

	_, err := (&Adaptor{}).ConvertImageRequest(c, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "imagen-3"}}, dto.ImageRequest{N: &tooMany})

	require.EqualError(t, err, "n must be an integer between 1 and 128")
}

func TestConvertImageRequestDefaultsZeroSampleCount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	zero := uint(0)

	converted, err := (&Adaptor{}).ConvertImageRequest(c, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "imagen-3"}}, dto.ImageRequest{N: &zero})

	require.NoError(t, err)
	request, ok := converted.(dto.GeminiImageRequest)
	require.True(t, ok)
	require.Equal(t, 1, request.Parameters.SampleCount)
}
