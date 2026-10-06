package service

import (
	"errors"
	"net"
	"reflect"
	"slices"
	"strconv"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/model"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/service/accesspolicy"
	"github.com/ForceMind/MyAPI/setting"
	"github.com/ForceMind/MyAPI/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

var ErrRelayEligibilityChanged = errors.New("relay_eligibility_changed")

// ValidateRelayFailoverDispatch re-reads admission and the selected credential
// just before HTTP dispatch. It never rotates keys or recalculates a frozen
// price. Revocations/configuration changes stop this request; a new request
// may obtain a fresh admission. The DB check cannot promise atomicity with an
// external provider accepting the subsequently sent request.
func ValidateRelayFailoverDispatch(c *gin.Context, info *relaycommon.RelayInfo) error {
	if c == nil || c.Request == nil || RelayFailoverFromContext(c.Request.Context()) == nil {
		return nil
	}
	if err := c.Request.Context().Err(); err != nil {
		return err
	}
	if info == nil || info.ChannelMeta == nil {
		return ErrRelayEligibilityChanged
	}
	channel, err := model.GetChannelById(info.ChannelId, true)
	if err != nil {
		return err
	}
	selectedDigest := c.GetString("relay_channel_config_digest")
	if selectedDigest == "" || model.RelayChannelConfigDigest(channel) != selectedDigest {
		return ErrRelayEligibilityChanged
	}
	organization := ""
	if channel.OpenAIOrganization != nil {
		organization = *channel.OpenAIOrganization
	}
	if organization != info.Organization {
		return ErrRelayEligibilityChanged
	}
	originalOverrides, _ := c.Get("relay_channel_param_override")
	if channel.Status != common.ChannelStatusEnabled || channel.Type != info.ChannelType || channel.GetBaseURL() != info.ChannelBaseUrl || !reflect.DeepEqual(channel.GetSetting(), info.ChannelSetting) || !reflect.DeepEqual(channel.GetParamOverride(), originalOverrides) || !reflect.DeepEqual(channel.GetHeaderOverride(), info.HeadersOverride) {
		return ErrRelayEligibilityChanged
	}
	route, err := model.ResolveChannelModelRoute(channel, info.OriginModelName, c.Request.URL.Path)
	if err != nil {
		return err
	}
	raw, _ := c.Get("model_route")
	frozen, _ := raw.(map[string]any)
	if route.UpstreamModel != info.UpstreamModelName || route.ConfigDigest != frozen["config_digest"] {
		return ErrRelayEligibilityChanged
	}
	group := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	if selected := common.GetContextKeyString(c, constant.ContextKeyAutoGroup); selected != "" {
		group = selected
	}
	if !slices.Contains(channel.GetGroups(), group) || !slices.Contains(channel.GetModels(), info.OriginModelName) && !slices.Contains(channel.GetModels(), ratio_setting.FormatMatchingModelName(info.OriginModelName)) {
		return ErrRelayEligibilityChanged
	}
	excluded, eligible, err := CodexQuotaEligibleKeys(c.Request.Context(), channel)
	if err != nil {
		return err
	}
	if !eligible {
		return ErrRelayEligibilityChanged
	}
	excluded, eligible, _, err = RelayCooldownEligibleKeys(c.Request.Context(), channel, c.Request.URL.Path, excluded)
	if err != nil {
		return err
	}
	if !eligible {
		return ErrRelayEligibilityChanged
	}
	excluded, eligible = RelayFailoverEligibleKeys(c.Request.Context(), channel, excluded)
	if !eligible {
		return ErrRelayEligibilityChanged
	}
	keys := []string{channel.Key}
	if channel.ChannelInfo.IsMultiKey {
		keys = channel.GetKeys()
	}
	found := false
	for index, key := range keys {
		status, hasStatus := channel.ChannelInfo.MultiKeyStatusList[index]
		if key == info.ApiKey && !excluded[index] && (!channel.ChannelInfo.IsMultiKey || !hasStatus || status == common.ChannelStatusEnabled) {
			found = true
			break
		}
	}
	if !found {
		return ErrRelayEligibilityChanged
	}
	token, err := model.GetTokenById(info.TokenId)
	if err != nil {
		return err
	}
	if token.UserId != info.UserId || token.Key != info.TokenKey || token.Status != common.TokenStatusEnabled || token.ExpiredTime != -1 && token.ExpiredTime <= time.Now().Unix() || token.Group != common.GetContextKeyString(c, constant.ContextKeyTokenGroup) || token.AccessProfileID != common.GetContextKeyString(c, constant.ContextKeyAccessProfileID) {
		return ErrRelayEligibilityChanged
	}
	budget, err := model.LookupTokenBudget(c.Request.Context(), model.DB, info.TokenId)
	if err != nil {
		return err
	}
	_, thresholdActive := common.AccountQuotaThresholdFromContext(c.Request.Context())
	if budget != nil {
		if budget.UserID != info.UserId || (budget.Enabled || budget.FeeEnabled) != info.StrictTokenBudget || budget.AccountThresholdEnabled != thresholdActive {
			return ErrRelayEligibilityChanged
		}
	} else if info.StrictTokenBudget || thresholdActive {
		return ErrRelayEligibilityChanged
	}
	if token.ModelLimitsEnabled {
		allowed := token.GetModelLimitsMap()
		for _, name := range []string{info.OriginModelName, info.UpstreamModelName} {
			if !allowed[ratio_setting.FormatMatchingModelName(name)] {
				return ErrRelayEligibilityChanged
			}
		}
	}
	if limits := token.GetIpLimits(); len(limits) > 0 && !common.IsIpInCIDRList(net.ParseIP(c.ClientIP()), limits) {
		return ErrRelayEligibilityChanged
	}
	user, err := model.GetUserById(info.UserId, false)
	if err != nil {
		return err
	}
	if user.Status != common.UserStatusEnabled || user.Group != info.UserGroup || user.AccountTierID != common.GetContextKeyString(c, constant.ContextKeyAccountTierID) {
		return ErrRelayEligibilityChanged
	}
	if token.Group == "auto" {
		groups := GetUserAutoGroup(user.Group)
		if token.AutoGroups != "" {
			configured, err := token.GetAutoGroups()
			if err != nil {
				return err
			}
			groups = FilterUserTokenAutoGroups(user.Group, configured)
		}
		if !slices.Contains(groups, group) {
			return ErrRelayEligibilityChanged
		}
	} else if group != user.Group && !GroupInUserUsableGroups(user.Group, group) {
		return ErrRelayEligibilityChanged
	}
	mode := setting.GetAccessPolicyMode()
	if mode == setting.AccessPolicyModeEnforce && setting.AccessPolicyEnforcedForGroup(group) {
		for _, name := range []string{info.OriginModelName, info.UpstreamModelName} {
			decision, err := accesspolicy.EvaluateRequest(accesspolicy.PolicyModeEnforce, accesspolicy.SnapshotSource{AccountTierID: user.AccountTierID, AccessProfileID: token.AccessProfileID, LegacyTokenGroup: token.Group, UsingGroup: group, UsingModel: name, LegacyAllowed: true, GroupRatio: strconv.FormatFloat(ratio_setting.GetGroupRatio(group), 'f', -1, 64)}, 0)
			if err != nil || len(accesspolicy.BlockingFindings(decision)) > 0 {
				return ErrRelayEligibilityChanged
			}
		}
	}
	c.Set(relayDispatchChannelKey, channel)
	return nil
}
