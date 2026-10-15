package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSubscriptionSharePoolUsageAPIPrivacyAndNoStore(t *testing.T) {
	db := subscriptionShareControllerFixture(t)
	pool := model.SubscriptionSharePool{DescriptorKey: "usage-api-fixture", SubjectRef: "private-subject", ChannelID: 8, ChannelName: "usage account", WindowSource: "codex_wham_usage_primary", WindowType: "five_hour", WindowSeconds: 18000, PlanType: "pro", Enabled: true, Revision: 1, MaxAgeSeconds: 300}
	require.NoError(t, db.Create(&pool).Error)
	require.NoError(t, db.Create(&model.SubscriptionShareAllocation{PoolID: pool.ID, TokenID: 11, UserID: 2, ShareBPS: 2000}).Error)
	for _, test := range []struct {
		name  string
		actor int
		id    string
		query string
		want  int
	}{
		{"Root", 1, strconv.FormatInt(pool.ID, 10), "", http.StatusOK},
		{"owner denied", 2, strconv.FormatInt(pool.ID, 10), "", http.StatusForbidden},
		{"other denied", 3, strconv.FormatInt(pool.ID, 10), "", http.StatusForbidden},
		{"missing pool", 1, "99999", "", http.StatusNotFound},
		{"query rejected", 1, strconv.FormatInt(pool.ID, 10), "?token_id=11", http.StatusBadRequest},
		{"invalid id", 1, "0", "", http.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Set("id", test.actor)
			c.Params = gin.Params{{Key: "pool_id", Value: test.id}}
			c.Request = httptest.NewRequest(http.MethodGet, "/api/subscription-share/pools/"+test.id+"/usage"+test.query, nil)
			GetSubscriptionSharePoolUsage(c)
			assert.Equal(t, test.want, recorder.Code)
			assert.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
			assert.NotContains(t, recorder.Body.String(), "share-api-key")
			assert.NotContains(t, recorder.Body.String(), "private-subject")
			if test.want == http.StatusOK {
				assert.Contains(t, recorder.Body.String(), `"estimated_used_micro":null`)
				assert.Contains(t, recorder.Body.String(), `"initial_baseline_used_micro":null`)
			}
		})
	}
	var allocations int64
	require.NoError(t, db.Model(&model.SubscriptionShareAllocation{}).Count(&allocations).Error)
	assert.EqualValues(t, 1, allocations)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", 2).Update("role", common.RoleAdminUser).Error)
	_, err := model.ReadSubscriptionSharePoolUsage(t.Context(), db, 2, pool.ID, 100)
	require.ErrorIs(t, err, model.ErrSubscriptionShareDenied)
}
