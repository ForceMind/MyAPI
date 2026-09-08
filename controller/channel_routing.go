package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/service"
	"github.com/ForceMind/MyAPI/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type channelRoutingPreviewRequest struct {
	Model string `json:"model"`
	Group string `json:"group"`
	Path  string `json:"path"`
}

type channelRoutingChannelUpdate struct {
	Priority               *int64  `json:"priority"`
	Weight                 *int64  `json:"weight"`
	ExpectedPriority       *int64  `json:"expected_priority"`
	ExpectedWeight         *int64  `json:"expected_weight"`
	ExpectedLegacyPriority *string `json:"expected_legacy_priority"`
	ExpectedLegacyWeight   *string `json:"expected_legacy_weight"`
}

type channelRoutingPolicyUpdate struct {
	Enabled            *bool `json:"enabled"`
	StickyEnabled      *bool `json:"sticky_enabled"`
	SessionTTLSeconds  *int  `json:"session_ttl_seconds"`
	QuotaMaxAgeSeconds *int  `json:"quota_max_age_seconds"`
}

func GetChannelRouting(c *gin.Context) {
	data, err := service.GetChannelRoutingManagementData(c.Request.Context())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

func UpdateChannelRoutingPolicy(c *gin.Context) {
	previous := operation_setting.GetChannelRoutingPolicy()
	var request channelRoutingPolicyUpdate
	if err := common.UnmarshalBodyReusable(c, &request); err != nil || request.Enabled == nil || request.StickyEnabled == nil || request.SessionTTLSeconds == nil || request.QuotaMaxAgeSeconds == nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": i18n.T(c, i18n.MsgInvalidParams)})
		return
	}
	policy := operation_setting.ChannelRoutingPolicy{
		Enabled: *request.Enabled, StickyEnabled: *request.StickyEnabled,
		SessionTTLSeconds: *request.SessionTTLSeconds, QuotaMaxAgeSeconds: *request.QuotaMaxAgeSeconds,
	}
	encoded, err := operation_setting.MarshalChannelRoutingPolicy(policy)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": i18n.T(c, i18n.MsgInvalidParams)})
		return
	}
	if err := model.UpdateOption(operation_setting.ChannelRoutingPolicyOptionKey, encoded); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "channel.routing_policy_update", map[string]interface{}{"before": previous, "after": policy})
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func PreviewChannelRouting(c *gin.Context) {
	var request channelRoutingPreviewRequest
	if err := common.UnmarshalBodyReusable(c, &request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": i18n.T(c, i18n.MsgInvalidParams)})
		return
	}
	request.Model = strings.TrimSpace(request.Model)
	request.Group = strings.TrimSpace(request.Group)
	request.Path = strings.TrimSpace(request.Path)
	if request.Model == "" || request.Group == "" || request.Path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": i18n.T(c, i18n.MsgInvalidParams)})
		return
	}
	if request.Group == "auto" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": i18n.T(c, i18n.MsgInvalidParams)})
		return
	}
	data, err := service.PreviewChannelRouting(c.Request.Context(), request.Model, request.Group, request.Path)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

func UpdateChannelRoutingValues(c *gin.Context) {
	channelID, err := strconv.Atoi(c.Param("id"))
	if err != nil || channelID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": i18n.T(c, i18n.MsgInvalidId)})
		return
	}
	var request channelRoutingChannelUpdate
	if err := common.UnmarshalBodyReusable(c, &request); err != nil || request.Priority == nil || request.Weight == nil || request.ExpectedPriority == nil || request.ExpectedWeight == nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": i18n.T(c, i18n.MsgInvalidParams)})
		return
	}
	if *request.Priority < model.MinChannelRoutingPriority || *request.Priority > model.MaxChannelRoutingPriority ||
		*request.ExpectedPriority < model.MinChannelRoutingPriority || *request.ExpectedPriority > model.MaxChannelRoutingPriority ||
		*request.Weight < 0 || *request.Weight > int64(model.MaxChannelRoutingWeight) ||
		*request.ExpectedWeight < 0 || *request.ExpectedWeight > int64(model.MaxChannelRoutingWeight) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": i18n.T(c, i18n.MsgInvalidParams)})
		return
	}
	expectedPriority := *request.ExpectedPriority
	beforePriority := interface{}(expectedPriority)
	if request.ExpectedLegacyPriority != nil {
		raw := strings.TrimSpace(*request.ExpectedLegacyPriority)
		parsed, parseErr := strconv.ParseInt(raw, 10, 64)
		if parseErr != nil || raw != strconv.FormatInt(parsed, 10) ||
			(parsed >= model.MinChannelRoutingPriority && parsed <= model.MaxChannelRoutingPriority) ||
			((parsed < model.MinChannelRoutingPriority && expectedPriority != model.MinChannelRoutingPriority) ||
				(parsed > model.MaxChannelRoutingPriority && expectedPriority != model.MaxChannelRoutingPriority)) {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": i18n.T(c, i18n.MsgInvalidParams)})
			return
		}
		expectedPriority = parsed
		beforePriority = raw
	}
	expectedWeight := uint(*request.ExpectedWeight)
	beforeWeight := interface{}(*request.ExpectedWeight)
	if request.ExpectedLegacyWeight != nil {
		raw := strings.TrimSpace(*request.ExpectedLegacyWeight)
		parsed, parseErr := strconv.ParseUint(raw, 10, strconv.IntSize)
		if parseErr != nil || raw != strconv.FormatUint(parsed, 10) ||
			parsed <= uint64(model.MaxChannelRoutingWeight) || *request.ExpectedWeight != int64(model.MaxChannelRoutingWeight) {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": i18n.T(c, i18n.MsgInvalidParams)})
			return
		}
		expectedWeight = uint(parsed)
		beforeWeight = raw
	}
	updated, err := model.UpdateChannelRoutingValuesCAS(channelID, *request.Priority, uint(*request.Weight), expectedPriority, expectedWeight)
	if errors.Is(err, model.ErrChannelRoutingConflict) {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": i18n.T(c, i18n.MsgUpdateFailed)})
		return
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": i18n.T(c, i18n.MsgNotFound)})
		return
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "channel.routing_values_update", map[string]interface{}{
		"id":     channelID,
		"before": map[string]interface{}{"priority": beforePriority, "weight": beforeWeight},
		"after":  map[string]interface{}{"priority": updated.GetPriority(), "weight": updated.GetWeight()},
	})
	c.JSON(http.StatusOK, gin.H{"success": true})
}
