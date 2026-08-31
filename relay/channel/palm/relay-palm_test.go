package palm

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func palmHTTPResponse(body string, status int) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

func TestPalmHandlerRejectsMalformedResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	info := &common.RelayInfo{}

	usage, apiErr := palmHandler(c, info, palmHTTPResponse("{not-json", http.StatusOK))

	require.Nil(t, usage)
	require.NotNil(t, apiErr)
	require.Equal(t, types.ErrorCodeBadResponseBody, apiErr.GetErrorCode())
	require.Equal(t, http.StatusInternalServerError, apiErr.StatusCode)
}

func TestPalmHandlerPropagatesUpstreamError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	info := &common.RelayInfo{}

	usage, apiErr := palmHandler(c, info, palmHTTPResponse(`{"error":{"code":400,"message":"bad prompt","status":"INVALID_ARGUMENT"}}`, http.StatusBadRequest))

	require.Nil(t, usage)
	require.NotNil(t, apiErr)
	require.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
	require.Equal(t, types.ErrorTypeOpenAIError, apiErr.GetErrorType())
}
