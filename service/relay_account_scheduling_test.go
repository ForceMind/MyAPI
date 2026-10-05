package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelayCooldownRetryAfterBounds(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		value string
		want  int
	}{{"", 20}, {"bad", 20}, {"-5", 20}, {"0", 0}, {"15", 15}, {"9999999999999999999999999", 300}, {now.Add(12 * time.Second).Format(http.TimeFormat), 12}, {now.Add(time.Hour).Format(http.TimeFormat), 300}, {now.Add(-time.Second).Format(http.TimeFormat), 20}} {
		t.Run(test.value, func(t *testing.T) {
			assert.Equal(t, test.want, relayCooldownSeconds(test.value, 20, now))
			assert.Zero(t, relayCooldownSeconds(test.value, 0, now))
		})
	}
}

func TestRelayCooldownPersistsFutureGateWithoutAuthorizingReplay(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	require.NoError(t, db.AutoMigrate(&model.RelayAccountHold{}))
	createChannelSelectAutoGroupsChannel(t, db, 6001, "default", "public")
	channel, err := model.GetChannelById(6001, true)
	require.NoError(t, err)
	old := common.RelayFailureCooldownSeconds
	common.RelayFailureCooldownSeconds = 20
	t.Cleanup(func() { common.RelayFailureCooldownSeconds = old })
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	c.Set("model_route", map[string]any{"upstream_model": "public"})
	state := BeginRelayFailover(c)
	state.DispatchPossible = true
	c.Set(relayDispatchChannelKey, channel)
	common.SetContextKey(c, constant.ContextKeyChannelKey, channel.Key)
	ObserveRelayTransientFailure(c, &http.Response{StatusCode: 503, Header: make(http.Header)}, nil)
	assert.True(t, state.DispatchPossible, "health observation cannot authorize replay or refund")
	excluded, eligible, until, err := RelayCooldownEligibleKeys(context.Background(), channel, "/v1/chat/completions", nil)
	require.NoError(t, err)
	assert.False(t, eligible)
	assert.True(t, excluded[0])
	assert.Positive(t, until)
	require.NoError(t, model.InitChannelCache())
	view, err := GetAvailableChannelRoutingPolicy(context.Background(), "default", "public", "/v1/chat/completions")
	require.NoError(t, err)
	require.Len(t, view.Policy.Rejected, 1)
	assert.Equal(t, "account_cooling_down", view.Policy.Rejected[0].RouteError)
	assert.Equal(t, until, view.Policy.Rejected[0].CooldownUntil)
	common.RelayFailureCooldownSeconds = 0
	_, eligible, _, err = RelayCooldownEligibleKeys(context.Background(), channel, "/v1/chat/completions", nil)
	require.NoError(t, err)
	assert.True(t, eligible)
}

func TestRelayCooldownNeverRecordsClientCancellationOrLocalErrors(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	require.NoError(t, db.AutoMigrate(&model.RelayAccountHold{}))
	createChannelSelectAutoGroupsChannel(t, db, 6002, "default", "public")
	channel, err := model.GetChannelById(6002, true)
	require.NoError(t, err)
	old := common.RelayFailureCooldownSeconds
	common.RelayFailureCooldownSeconds = 20
	t.Cleanup(func() { common.RelayFailureCooldownSeconds = old })
	for _, status := range []int{200, 400, 401, 403, 404, 408, 409, 422} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
		c.Set("model_route", map[string]any{"upstream_model": "public"})
		BeginRelayFailover(c)
		c.Set(relayDispatchChannelKey, channel)
		common.SetContextKey(c, constant.ContextKeyChannelKey, channel.Key)
		ObserveRelayTransientFailure(c, &http.Response{StatusCode: status, Header: make(http.Header)}, nil)
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	ctx, cancel := context.WithCancel(c.Request.Context())
	c.Request = c.Request.WithContext(ctx)
	c.Set("model_route", map[string]any{"upstream_model": "public"})
	BeginRelayFailover(c)
	c.Set(relayDispatchChannelKey, channel)
	common.SetContextKey(c, constant.ContextKeyChannelKey, channel.Key)
	cancel()
	ObserveRelayTransientFailure(c, nil, errors.New("transport failed after cancel"))
	var count int64
	require.NoError(t, db.Model(&model.RelayAccountHold{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestRelayFailoverOptionalOverallDeadlineAndLegacyDisabled(t *testing.T) {
	old := common.RelayFailoverTimeoutSeconds
	t.Cleanup(func() { common.RelayFailoverTimeoutSeconds = old })
	for _, seconds := range []int{0, 5} {
		common.RelayFailoverTimeoutSeconds = seconds
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
		c.Set("model_route", map[string]any{"upstream_model": "actual"})
		state := BeginRelayFailover(c)
		_, bounded := c.Request.Context().Deadline()
		assert.Equal(t, seconds > 0, bounded)
		state.Close()
		if seconds > 0 {
			assert.ErrorIs(t, c.Request.Context().Err(), context.Canceled)
		}
	}
}

func TestRelayCooldownDisabledDoesNotRequireHealthDatabase(t *testing.T) {
	oldDB, old := model.DB, common.RelayFailureCooldownSeconds
	model.DB = nil
	common.RelayFailureCooldownSeconds = 0
	t.Cleanup(func() { model.DB = oldDB; common.RelayFailureCooldownSeconds = old })
	channel := &model.Channel{Id: 1, Type: constant.ChannelTypeOpenAI, Key: "fixture"}
	excluded, eligible, until, err := RelayCooldownEligibleKeys(context.Background(), channel, "/v1/responses", map[int]bool{1: true})
	require.NoError(t, err)
	assert.True(t, eligible)
	assert.Zero(t, until)
	assert.True(t, excluded[1])
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	ObserveRelayTransientFailure(c, &http.Response{StatusCode: 429}, nil)
}

func TestRelayCooldownRecoveryNeverOverridesCodexExhaustion(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	t.Setenv(common.ChannelQuotaIdentityKeysEnv, "")
	require.NoError(t, db.AutoMigrate(&model.ChannelQuotaSnapshot{}, &model.RelayAccountHold{}))
	createCodexQuotaRoutingChannel(t, db, 6301, 100, `{"access_token":"synthetic","account_id":"same-account"}`)
	channel, err := model.GetChannelById(6301, true)
	require.NoError(t, err)
	old := common.RelayFailureCooldownSeconds
	common.RelayFailureCooldownSeconds = 20
	t.Cleanup(func() { common.RelayFailureCooldownSeconds = old })
	_, err = model.RecordRelayAccountHold(context.Background(), db, channel, channel.Key, RelayAccountIdentity(channel, channel.Key), 20)
	require.NoError(t, err)
	recordCodexRoutingUsage(t, channel.Id, "same-account", 0, time.Now().Add(time.Hour).Unix())
	require.NoError(t, db.Model(&model.RelayAccountHold{}).Where("channel_id = ?", channel.Id).Update("hold_until", 1).Error)
	require.NoError(t, model.InitChannelCache())
	view, err := GetAvailableChannelRoutingPolicy(context.Background(), "default", "gpt-5-codex", "/v1/responses")
	require.NoError(t, err)
	require.Len(t, view.Policy.Rejected, 1)
	assert.Equal(t, "account_not_eligible", view.Policy.Rejected[0].RouteError)
	instance := getChannelAffinityCacheInstance()
	require.NoError(t, instance.accounts.SetWithTTL("exhausted-case", channelAccountAffinity{ChannelID: channel.Id, Identity: RelayAccountIdentity(channel, channel.Key)}, time.Minute))
	t.Cleanup(func() { _, _ = instance.accounts.DeleteMany([]string{"exhausted-case"}) })
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	assert.False(t, eligibleChannelAccountAffinity(c, channel, "exhausted-case", instance))
}

func TestRelayCooldownRejectsLocalHTTPClientValidationFailures(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	require.NoError(t, db.AutoMigrate(&model.RelayAccountHold{}))
	createChannelSelectAutoGroupsChannel(t, db, 6302, "default", "public")
	channel, err := model.GetChannelById(6302, true)
	require.NoError(t, err)
	old := common.RelayFailureCooldownSeconds
	common.RelayFailureCooldownSeconds = 20
	t.Cleanup(func() { common.RelayFailureCooldownSeconds = old })
	for _, test := range []struct {
		name, url     string
		invalidHeader bool
	}{{"invalid header", "http://127.0.0.1/", true}, {"unsupported scheme", "unsupported://fixture/", false}} {
		t.Run(test.name, func(t *testing.T) {
			req, err := http.NewRequest("POST", test.url, nil)
			require.NoError(t, err)
			if test.invalidHeader {
				req.Header.Set("X-Fixture", "invalid\nvalue")
			}
			response, dispatchErr := http.DefaultClient.Do(req)
			require.Error(t, dispatchErr)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
			c.Set("model_route", map[string]any{"upstream_model": "public"})
			BeginRelayFailover(c)
			c.Set(relayDispatchChannelKey, channel)
			common.SetContextKey(c, constant.ContextKeyChannelKey, channel.Key)
			ObserveRelayTransientFailure(c, response, dispatchErr)
			var count int64
			require.NoError(t, db.Model(&model.RelayAccountHold{}).Count(&count).Error)
			assert.Zero(t, count)
		})
	}
}

func TestRelayCooldownDoesNotLabelMissingCredentialAsCooling(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	require.NoError(t, db.AutoMigrate(&model.RelayAccountHold{}))
	createChannelSelectAutoGroupsChannel(t, db, 6303, "default", "public")
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", 6303).Update("key", "").Error)
	require.NoError(t, model.InitChannelCache())
	old := common.RelayFailureCooldownSeconds
	common.RelayFailureCooldownSeconds = 20
	t.Cleanup(func() { common.RelayFailureCooldownSeconds = old })
	view, err := GetAvailableChannelRoutingPolicy(context.Background(), "default", "public", "/v1/chat/completions")
	require.NoError(t, err)
	require.Len(t, view.Policy.Rejected, 1)
	assert.Equal(t, "account_not_eligible", view.Policy.Rejected[0].RouteError)
	assert.Zero(t, view.Policy.Rejected[0].CooldownUntil)
}
