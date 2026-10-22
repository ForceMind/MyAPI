package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestLogIdentityRejectsInvalidIDsBeforeAnyDatabaseRead(t *testing.T) {
	for _, handler := range []struct {
		name string
		call gin.HandlerFunc
	}{{"admin-list", GetAllLogs}, {"self-list", GetUserLogs}, {"admin-stats", GetLogsStat}, {"self-stats", GetLogsSelfStat}} {
		for _, query := range []string{"token_id=0", "token_id=-1", "token_id=", "token_id=1.5", "token_id=2147483648", "token_id=7&token_id=8", "token_id=%zz"} {
			t.Run(handler.name+"/"+query, func(t *testing.T) {
				r := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(r)
				c.Request = httptest.NewRequest(http.MethodGet, "/api/log/?"+query, nil)
				c.Set("id", 1)
				c.Set("username", "owner")
				handler.call(c)
				require.Equal(t, http.StatusBadRequest, r.Code)
			})
		}
	}
}
