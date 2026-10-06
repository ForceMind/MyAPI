package service

import (
	"strings"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/model"
	"github.com/gin-gonic/gin"
)

// Companion only: the existing integer cache remains readable by old callers.
// Account identity is private, never added to the affinity log metadata.
type channelAccountAffinity struct {
	ChannelID int    `json:"channel_id"`
	Identity  string `json:"account_identity"`
}

func (channelAccountAffinity) String() string     { return "channelAccountAffinity{Private:[REDACTED]}" }
func (v channelAccountAffinity) GoString() string { return v.String() }

const accountAffinityIndexKey = "relay_account_affinity_index"

func eligibleChannelAccountAffinity(c *gin.Context, channel *model.Channel, cacheKey string, instance *channelAffinityCacheInstance) bool {
	if c == nil || c.Request == nil || !RelayAccountSchedulingSupported(channel, c.Request.URL.Path) {
		return true
	}
	suffix := strings.TrimPrefix(cacheKey, channelAffinityCacheNamespace+":")
	binding, found, err := instance.accounts.Get(suffix)
	if err != nil || !found || binding.ChannelID != channel.Id || binding.Identity == "" {
		return false
	}
	excluded, usable, err := CodexQuotaEligibleKeys(c.Request.Context(), channel)
	if err != nil || !usable {
		return false
	}
	excluded, usable, _, err = RelayCooldownEligibleKeys(c.Request.Context(), channel, c.Request.URL.Path, excluded)
	if err != nil || !usable {
		return false
	}
	keys := []string{channel.Key}
	if channel.ChannelInfo.IsMultiKey {
		keys = channel.GetKeys()
	}
	for index, key := range keys {
		status, has := channel.ChannelInfo.MultiKeyStatusList[index]
		if !excluded[index] && (!channel.ChannelInfo.IsMultiKey || !has || status == common.ChannelStatusEnabled) && RelayAccountIdentity(channel, key) == binding.Identity {
			c.Set(accountAffinityIndexKey, binding)
			return true
		}
	}
	return false
}

// PreferredChannelAccountIndex selects the current key, never a cached raw key.
// Final dispatch still validates current permissions, configuration and quota.
func PreferredChannelAccountIndex(c *gin.Context, channel *model.Channel) *int {
	value, ok := c.Get(accountAffinityIndexKey)
	if !ok {
		return nil
	}
	binding, ok := value.(channelAccountAffinity)
	if !ok || binding.ChannelID != channel.Id {
		return nil
	}
	keys := []string{channel.Key}
	if channel.ChannelInfo.IsMultiKey {
		keys = channel.GetKeys()
	}
	for index, key := range keys {
		if RelayAccountIdentity(channel, key) == binding.Identity {
			return &index
		}
	}
	return nil
}

func recordChannelAccountAffinity(c *gin.Context, channelID int, cacheKey string, ttl time.Duration, instance *channelAffinityCacheInstance) {
	if c == nil || c.Request == nil || !c.GetBool("relay_success") {
		return
	}
	raw, ok := c.Get(relayDispatchChannelKey)
	channel, typed := raw.(*model.Channel)
	if !ok || !typed || channel.Id != channelID || !RelayAccountSchedulingSupported(channel, c.Request.URL.Path) {
		return
	}
	identity := RelayAccountIdentity(channel, common.GetContextKeyString(c, constant.ContextKeyChannelKey))
	if identity == "" {
		return
	}
	suffix := strings.TrimPrefix(cacheKey, channelAffinityCacheNamespace+":")
	if err := instance.accounts.SetWithTTL(suffix, channelAccountAffinity{ChannelID: channelID, Identity: identity}, ttl); err != nil {
		common.SysError("account affinity cache set failed")
	}
}
