package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestResetPasswordRejectsMalformedJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/user/reset", strings.NewReader(`{"email":`))

	ResetPassword(context)

	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Body.String(), `"success":false`)
}
