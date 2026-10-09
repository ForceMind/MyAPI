package relay

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlaygroundPDFUsesOnlyQualifiedActualAdapter(t *testing.T) {
	require.NoError(t, i18n.Init())
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/pg/chat/completions", nil)
	var request dto.GeneralOpenAIRequest
	require.NoError(t, common.Unmarshal([]byte(`{"messages":[{"role":"user","content":[{"type":"file","file":{"filename":"notes.pdf","file_data":"data:application/pdf;base64,JVBERi0xLjQK"}}]}]}`), &request))
	require.Nil(t, validatePlaygroundMediaChannel(c, &request, constant.ChannelTypeOpenAI))
	for _, channelType := range []int{constant.ChannelTypeAnthropic, constant.ChannelTypeGemini, constant.ChannelTypeCodex} {
		err := validatePlaygroundMediaChannel(c, &request, channelType)
		require.NotNil(t, err)
		assert.Equal(t, 400, err.StatusCode)
		assert.Equal(t, "playground_file_provider_unsupported", string(err.GetErrorCode()))
	}
	// A prior compatible selection is not a reusable permit after failover.
	require.NotNil(t, validatePlaygroundMediaChannel(c, &request, constant.ChannelTypeAnthropic))
	request.Messages[0].Content = "text only"
	require.Nil(t, validatePlaygroundMediaChannel(c, &request, constant.ChannelTypeAnthropic))
}
