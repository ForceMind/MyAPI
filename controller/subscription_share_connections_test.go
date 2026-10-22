package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ForceMind/MyAPI/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSubscriptionShareConnectionPoolsAPI(t *testing.T) {
	db := subscriptionShareControllerFixture(t)
	require.NoError(t, db.Create(&model.Channel{Id: 7, Name: "saved-connection", Key: "not-for-response"}).Error)
	for _, tc := range []struct {
		name    string
		actor   int
		channel string
		query   string
		status  int
	}{
		{"root empty", 1, "7", "", http.StatusOK},
		{"owner denied", 2, "7", "", http.StatusForbidden},
		{"missing", 1, "8", "", http.StatusNotFound},
		{"invalid channel", 1, "0", "", http.StatusBadRequest},
		{"invalid page", 1, "7", "?page=0", http.StatusBadRequest},
		{"bounded page", 1, "7", "?page=1000001", http.StatusBadRequest},
		{"duplicate page", 1, "7", "?page=1&page=2", http.StatusBadRequest},
		{"unknown query", 1, "7", "?token_id=11", http.StatusBadRequest},
		{"malformed query", 1, "7", "?%zz", http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodGet, "/api/subscription-share/channels/"+tc.channel+"/pools"+tc.query, nil)
			c.Params = gin.Params{{Key: "channel_id", Value: tc.channel}}
			c.Set("id", tc.actor)
			GetSubscriptionShareConnectionPools(c)
			require.Equal(t, tc.status, recorder.Code)
			require.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
			require.NotContains(t, recorder.Body.String(), "not-for-response")
			if tc.status == http.StatusOK {
				require.Contains(t, recorder.Body.String(), `"items":[]`)
				require.Contains(t, recorder.Body.String(), `"channel_id":7`)
			}
		})
	}
}
