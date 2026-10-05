package service

import (
	"net/http/httptest"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelayFailoverCredentialExclusionSurvivesReordering(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	c.Set("model_route", map[string]any{"upstream_model": "actual"})
	state := BeginRelayFailover(c)
	require.NotNil(t, state)
	c.Set("channel_id", 12)
	common.SetContextKey(c, constant.ContextKeyChannelKey, "key-a")
	state.StartAttempt(c)
	state.FinishAttempt(c, "retryable_refusal", 429, true)
	channel := &model.Channel{Id: 12, Key: "key-b\nkey-a\nkey-c", ChannelInfo: model.ChannelInfo{IsMultiKey: true}}
	excluded, usable := RelayFailoverEligibleKeys(c.Request.Context(), channel, map[int]bool{2: true})
	require.True(t, usable)
	assert.False(t, excluded[0])
	assert.True(t, excluded[1])
	assert.True(t, excluded[2])
	channel.Key = "replacement"
	channel.ChannelInfo.IsMultiKey = false
	excluded, usable = RelayFailoverEligibleKeys(c.Request.Context(), channel, nil)
	assert.True(t, usable)
	assert.False(t, excluded[0])
	admin := make(map[string]interface{})
	AppendRelayFailoverAdminInfo(c, admin)
	encoded, err := common.Marshal(admin)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "key-a")
	assert.Contains(t, string(encoded), "retryable_refusal")
}

func TestRelayFailoverOnlyDefiniteRawRefusalAllowsReplay(t *testing.T) {
	for _, status := range []int{0, 200, 302, 400, 401, 403, 404, 408, 409, 429, 500, 503, 504} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
		c.Set("model_route", map[string]any{"upstream_model": "actual"})
		state := BeginRelayFailover(c)
		state.DispatchPossible = true
		ObserveRelayFailoverResponse(c, status)
		refused := status == 400 || status == 401 || status == 403 || status == 404 || status == 429
		assert.Equal(t, !refused, state.DispatchPossible, "raw status %d", status)
	}
}

func TestRelayFailoverSelectionKeepsHealthyPeerAndSameTarget(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	for _, id := range []int{5001, 5002, 5003} {
		createChannelSelectAutoGroupsChannel(t, db, id, "default", "public")
	}
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", 5001).Updates(map[string]any{"key": "failed\nhealthy", "channel_info": model.ChannelInfo{IsMultiKey: true}}).Error)
	require.NoError(t, db.Model(&model.Channel{}).Where("id IN ?", []int{5001, 5002}).Update("model_mapping", `{"public":"actual"}`).Error)
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", 5003).Update("model_mapping", `{"public":"different"}`).Error)
	require.NoError(t, model.InitChannelCache())
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	c.Set("model_route", map[string]any{"upstream_model": "actual"})
	state := BeginRelayFailover(c)
	c.Set("channel_id", 5001)
	common.SetContextKey(c, constant.ContextKeyChannelKey, "failed")
	state.StartAttempt(c)
	state.FinishAttempt(c, "retryable_refusal", 429, true)
	view, err := GetAvailableChannelRoutingPolicy(c.Request.Context(), "default", "public", "/v1/chat/completions")
	require.NoError(t, err)
	require.Len(t, view.Policy.Tiers, 1)
	require.Len(t, view.Policy.Tiers[0].Candidates, 2)
	require.Len(t, view.Policy.Rejected, 1)
	assert.Equal(t, "retry_target_mismatch", view.Policy.Rejected[0].RouteError)
	c.Set("channel_id", 5002)
	common.SetContextKey(c, constant.ContextKeyChannelKey, "key-5002")
	state.StartAttempt(c)
	state.FinishAttempt(c, "retryable_refusal", 429, true)
	c.Set("channel_id", 5001)
	common.SetContextKey(c, constant.ContextKeyChannelKey, "healthy")
	state.StartAttempt(c)
	state.FinishAttempt(c, "retryable_refusal", 429, true)
	selected, err := getRandomQuotaSatisfiedChannel(c.Request.Context(), "default", "public", 99, "/v1/chat/completions")
	require.NoError(t, err)
	assert.Nil(t, selected, "exhausted candidates must not loop back to the last tier")
}
