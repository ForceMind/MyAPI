package service

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/model"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelAccountAffinityBindsOnlySuccessAndRechecksIdentity(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	require.NoError(t, db.AutoMigrate(&model.RelayAccountHold{}))
	createChannelSelectAutoGroupsChannel(t, db, 6201, "default", "public")
	channel, err := model.GetChannelById(6201, true)
	require.NoError(t, err)
	channel.Key = "first\nsecond"
	channel.ChannelInfo = model.ChannelInfo{IsMultiKey: true, MultiKeySize: 2}
	require.NoError(t, db.Save(channel).Error)
	restoreChannelAffinityCacheForTest(t)
	RebuildChannelAffinityCache()
	instance := getChannelAffinityCacheInstance()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	c.Set(relayDispatchChannelKey, channel)
	common.SetContextKey(c, constant.ContextKeyChannelKey, "second")
	recordChannelAccountAffinity(c, channel.Id, "affinity-case", time.Minute, instance)
	_, found, err := instance.accounts.Get("affinity-case")
	require.NoError(t, err)
	assert.False(t, found)
	c.Set("relay_success", true)
	recordChannelAccountAffinity(c, channel.Id, "affinity-case", time.Minute, instance)
	binding, found, err := instance.accounts.Get("affinity-case")
	require.NoError(t, err)
	require.True(t, found)
	assert.NotContains(t, binding.Identity, "second")
	assert.True(t, eligibleChannelAccountAffinity(c, channel, "affinity-case", instance))
	require.NotNil(t, PreferredChannelAccountIndex(c, channel))
	assert.Equal(t, 1, *PreferredChannelAccountIndex(c, channel))
	// Same identity follows reordering, never the former index.
	channel.Key = "second\nfirst"
	channel.Keys = nil
	assert.True(t, eligibleChannelAccountAffinity(c, channel, "affinity-case", instance))
	assert.Equal(t, 0, *PreferredChannelAccountIndex(c, channel))
	channel.Key = "replacement\nfirst"
	channel.Keys = nil
	assert.False(t, eligibleChannelAccountAffinity(c, channel, "affinity-case", instance))
}

func TestChannelAccountAffinityCooldownAndLifecycle(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	require.NoError(t, db.AutoMigrate(&model.RelayAccountHold{}))
	createChannelSelectAutoGroupsChannel(t, db, 6202, "default", "public")
	channel, err := model.GetChannelById(6202, true)
	require.NoError(t, err)
	restoreChannelAffinityCacheForTest(t)
	RebuildChannelAffinityCache()
	instance := getChannelAffinityCacheInstance()
	binding := channelAccountAffinity{ChannelID: channel.Id, Identity: RelayAccountIdentity(channel, channel.Key)}
	require.NoError(t, instance.accounts.SetWithTTL("hold-case", binding, time.Minute))
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	assert.True(t, eligibleChannelAccountAffinity(c, channel, "hold-case", instance))
	old := common.RelayFailureCooldownSeconds
	common.RelayFailureCooldownSeconds = 20
	t.Cleanup(func() { common.RelayFailureCooldownSeconds = old })
	_, err = model.RecordRelayAccountHold(context.Background(), db, channel, channel.Key, binding.Identity, 20)
	require.NoError(t, err)
	assert.False(t, eligibleChannelAccountAffinity(c, channel, "hold-case", instance))
	RebuildChannelAffinityCache()
	_, found, err := getChannelAffinityCacheInstance().accounts.Get("hold-case")
	require.NoError(t, err)
	assert.False(t, found)
	require.NoError(t, getChannelAffinityCacheInstance().accounts.SetWithTTL("orphan", binding, time.Minute))
	ClearChannelAffinityCacheAll()
	_, found, err = getChannelAffinityCacheInstance().accounts.Get("orphan")
	require.NoError(t, err)
	assert.False(t, found)
}

func TestChannelAccountAffinityRedisCompanionTTLAndRebuild(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	oldRedis, oldEnabled := common.RDB, common.RedisEnabled
	common.RDB, common.RedisEnabled = client, true
	t.Cleanup(func() {
		common.RDB, common.RedisEnabled = oldRedis, oldEnabled
		RebuildChannelAffinityCache()
		require.NoError(t, client.Close())
	})
	RebuildChannelAffinityCache()
	instance := getChannelAffinityCacheInstance()
	binding := channelAccountAffinity{ChannelID: 7, Identity: model.ChannelQuotaAccountRef("fixture", "account")}
	require.NoError(t, instance.cache.SetWithTTL("ttl-case", 7, time.Minute))
	require.NoError(t, instance.accounts.SetWithTTL("ttl-case", binding, time.Minute))
	assert.Equal(t, time.Minute, server.TTL(channelAffinityCacheNamespace+":ttl-case"))
	assert.Equal(t, time.Minute, server.TTL("new-api:channel_affinity_accounts:v1:ttl-case"))
	RebuildChannelAffinityCache()
	current := getChannelAffinityCacheInstance()
	id, found, err := current.cache.Get("ttl-case")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, 7, id)
	actual, found, err := current.accounts.Get("ttl-case")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, binding, actual)
	server.FastForward(time.Minute)
	_, found, err = current.cache.Get("ttl-case")
	require.NoError(t, err)
	assert.False(t, found)
	_, found, err = current.accounts.Get("ttl-case")
	require.NoError(t, err)
	assert.False(t, found)
	require.NoError(t, current.cache.SetWithTTL("clear-case", 7, time.Minute))
	require.NoError(t, current.accounts.SetWithTTL("clear-case", binding, time.Minute))
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	setChannelAffinityContext(c, channelAffinityMeta{CacheKey: channelAffinityCacheNamespace + ":clear-case", TTLSeconds: 60})
	assert.True(t, ClearCurrentChannelAffinityCache(c))
	_, found, err = current.accounts.Get("clear-case")
	require.NoError(t, err)
	assert.False(t, found)
}
