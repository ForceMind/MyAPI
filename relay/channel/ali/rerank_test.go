package ali

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRerankHandlerRejectsMalformedResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("{not-json")),
	}

	err, usage := RerankHandler(c, resp, nil)

	require.Nil(t, usage)
	require.NotNil(t, err)
	require.Equal(t, types.ErrorCodeBadResponseBody, err.GetErrorCode())
}
