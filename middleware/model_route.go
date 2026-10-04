package middleware

import (
	"errors"
	"net/http"
	"slices"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/ForceMind/MyAPI/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

// prepareSelectedModelRoute freezes the exact selected channel mapping before
// credentials or quota are touched. The existing relay mapper consumes the
// resulting single mapping, including on diagnostic tests and retries.
func prepareSelectedModelRoute(c *gin.Context, channel *model.Channel, requested string, diagnostic bool) *types.NewAPIError {
	c.Set("model_route", nil)
	if !model.IsBasicModelRouteChannel(channel.Type) || c.Request == nil {
		return nil
	}
	endpoint := c.Request.URL.Path
	route, err := model.ResolveChannelModelRoute(channel, requested, endpoint)
	if err != nil {
		return types.NewErrorWithStatusCode(errors.New(common.TranslateMessage(c, i18n.MsgInvalidParams)), types.ErrorCode(err.Error()), http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	if endpoint != "/v1/chat/completions" && endpoint != "/v1/responses" {
		return nil
	}
	if !diagnostic {
		if !slices.Contains(channel.GetModels(), requested) && !slices.Contains(channel.GetModels(), ratio_setting.FormatMatchingModelName(requested)) {
			return types.NewErrorWithStatusCode(errors.New(i18n.T(c, i18n.MsgDistributorTokenModelForbidden, map[string]any{"Model": requested})), types.ErrorCodeAccessDenied, http.StatusForbidden, types.ErrOptionWithSkipRetry())
		}
		if common.GetContextKeyBool(c, constant.ContextKeyTokenModelLimitEnabled) {
			value, _ := common.GetContextKey(c, constant.ContextKeyTokenModelLimit)
			allowed, _ := value.(map[string]bool)
			for _, name := range []string{requested, route.UpstreamModel} {
				if !allowed[ratio_setting.FormatMatchingModelName(name)] {
					return types.NewErrorWithStatusCode(errors.New(i18n.T(c, i18n.MsgDistributorTokenModelForbidden, map[string]any{"Model": name})), types.ErrorCodeAccessDenied, http.StatusForbidden, types.ErrOptionWithSkipRetry())
				}
			}
		}
		group := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
		if resolved := common.GetContextKeyString(c, constant.ContextKeyAutoGroup); resolved != "" {
			group = resolved
		}
		if err := EnforceAccessPolicyForSelectedGroup(c, group, route.UpstreamModel); err != nil {
			return err
		}
	}
	mapping, err := common.Marshal(map[string]string{requested: route.UpstreamModel})
	if err != nil {
		return types.NewError(err, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	}
	common.SetContextKey(c, constant.ContextKeyChannelModelMapping, string(mapping))
	c.Set("model_route", map[string]any{"requested_model": route.RequestedModel, "upstream_model": route.UpstreamModel, "endpoint": route.Endpoint, "reason": route.Reason, "channel_id": route.ChannelID, "config_digest": route.ConfigDigest, "priority": channel.GetPriority(), "weight": channel.GetWeight(), "selection_policy": "existing_priority_weight_or_affinity"})
	return nil
}
