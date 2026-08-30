package claude

import (
	"net/http"
	"net/http/httptest"
	"testing"

	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestClaudeRequestURLNormalizesBaseURL(t *testing.T) {
	adaptor := &Adaptor{}
	url, err := adaptor.GetRequestURL(&relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: "https://api.anthropic.com///"},
	})
	require.NoError(t, err)
	require.Equal(t, "https://api.anthropic.com/v1/messages", url)

	_, err = adaptor.GetRequestURL(nil)
	require.Error(t, err)
}

func TestClaudeSetupRequestHeaderDefaultsJSONContentTypeAndVersion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	req := make(http.Header)
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{ApiKey: "configured-secret"},
	}

	require.NoError(t, (&Adaptor{}).SetupRequestHeader(c, &req, info))
	require.Equal(t, "application/json", req.Get("Content-Type"))
	require.Equal(t, "2023-06-01", req.Get("anthropic-version"))
	require.Equal(t, "configured-secret", req.Get("x-api-key"))
}

