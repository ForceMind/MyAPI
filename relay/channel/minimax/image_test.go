package minimax

import (
	"net/http/httptest"
	"testing"

	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relay/constant"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOAIImage2MiniMaxImageRequestDefaultsZeroN(t *testing.T) {
	zero := uint(0)
	request, err := oaiImage2MiniMaxImageRequest(dto.ImageRequest{N: &zero})

	require.NoError(t, err)
	require.Equal(t, 1, request.N)
}

func TestOAIImage2MiniMaxImageRequestRejectsOversizedN(t *testing.T) {
	tooMany := uint(dto.MaxImageN + 1)

	_, err := oaiImage2MiniMaxImageRequest(dto.ImageRequest{N: &tooMany})

	require.EqualError(t, err, "n must be an integer between 1 and 128")
}

func TestConvertImageRequestRejectsHugeUnsignedNBeforeIntConversion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	tooMany := ^uint(0)

	_, err := (&Adaptor{}).ConvertImageRequest(c, &relaycommon.RelayInfo{
		RelayMode: constant.RelayModeImagesGenerations,
	}, dto.ImageRequest{N: &tooMany})

	require.EqualError(t, err, "n must be an integer between 1 and 128")
}
