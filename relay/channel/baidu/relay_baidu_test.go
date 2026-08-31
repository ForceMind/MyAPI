package baidu

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestBaiduHandlerRejectsMalformedJSONResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(common.RequestIdKey, "baidu-test")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewBufferString("{not-json}")),
	}

	apiErr, usage := baiduHandler(c, &relaycommon.RelayInfo{}, resp)
	require.Error(t, apiErr)
	require.Nil(t, usage)
}
