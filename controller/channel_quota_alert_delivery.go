package controller

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/service"
	"github.com/gin-gonic/gin"
)

type channelQuotaAlertDeliverySettingsRequest struct {
	WebhookURL    string `json:"webhook_url"`
	WebhookSecret string `json:"webhook_secret"`
}

func GetChannelQuotaAlertDeliveryStatus(c *gin.Context) {
	status, err := service.GetChannelQuotaAlertDeliveryStatus()
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgDatabaseError)
		return
	}
	common.ApiSuccess(c, status)
}

func UpdateChannelQuotaAlertDeliverySettings(c *gin.Context) {
	var request channelQuotaAlertDeliverySettingsRequest
	if err := common.DecodeJsonStrict(c.Request.Body, &request); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	status, err := service.UpdateChannelQuotaAlertDeliverySettings(request.WebhookURL, request.WebhookSecret)
	if err != nil {
		if errors.Is(err, service.ErrChannelQuotaAlertDeliveryInvalidInput) {
			common.ApiErrorI18n(c, i18n.MsgInvalidParams)
			return
		}
		common.ApiErrorI18n(c, i18n.MsgDatabaseError)
		return
	}
	common.ApiSuccess(c, status)
}

func GetChannelQuotaAlertDeliveryEvents(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	filter := model.ChannelQuotaAlertEventFilter{
		State:  strings.TrimSpace(c.Query("state")),
		Status: strings.TrimSpace(c.Query("status")),
		Kind:   strings.TrimSpace(c.Query("kind")),
	}
	if rawChannelID := strings.TrimSpace(c.Query("channel_id")); rawChannelID != "" {
		channelID, err := strconv.Atoi(rawChannelID)
		if err != nil || channelID <= 0 {
			common.ApiErrorI18n(c, i18n.MsgInvalidParams)
			return
		}
		filter.ChannelID = channelID
	}
	if !model.ValidChannelQuotaAlertEventFilter(filter) {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	items, total, err := service.ListChannelQuotaAlertDeliveryEvents(
		c.Request.Context(), filter, pageInfo.GetStartIdx(), pageInfo.GetPageSize(),
	)
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgDatabaseError)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)
	common.ApiSuccess(c, pageInfo)
}

func RunChannelQuotaAlertDelivery(c *gin.Context) {
	workerID := fmt.Sprintf("quota-alert-manual-%d-%s", c.GetInt("id"), common.GetRandomString(8))
	summary, err := service.RunChannelQuotaAlertDeliveryPass(c.Request.Context(), workerID)
	if err != nil {
		if errors.Is(err, service.ErrChannelQuotaAlertDeliveryDisabled) || errors.Is(err, service.ErrChannelQuotaAlertDeliveryInvalidInput) {
			common.ApiErrorI18n(c, i18n.MsgFeatureDisabled)
			return
		}
		common.ApiErrorI18n(c, i18n.MsgDatabaseError)
		return
	}
	common.ApiSuccess(c, summary)
}
