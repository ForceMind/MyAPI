package controller

import (
	"context"
	"net/http"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/service"
	"github.com/gin-gonic/gin"
)

func GetOpenAIPriceCheckStatus(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if c.GetInt("role") != common.RoleRootUser {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": common.TranslateMessage(c, i18n.MsgAuthInsufficientPrivilege)})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	status, err := service.ReadOpenAIPriceCheckStatus(ctx, common.GetTimestamp())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "official_price_check_status_failed", "message": common.TranslateMessage(c, i18n.MsgOperationFailed)})
		return
	}
	common.ApiSuccess(c, status)
}

func CreateOpenAIPriceCheck(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if c.GetInt("role") != common.RoleRootUser {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": common.TranslateMessage(c, i18n.MsgAuthInsufficientPrivilege)})
		return
	}
	var request *struct{}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
	if len(c.Request.URL.Query()) != 0 || common.DecodeJsonStrict(c.Request.Body, &request) != nil || request == nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "code": "official_price_check_invalid_input", "message": common.TranslateMessage(c, i18n.MsgInvalidParams)})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	task, created, err := service.EnqueueSystemTaskContext(ctx, model.SystemTaskTypeOpenAIPriceCheck, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "official_price_check_enqueue_failed", "message": common.TranslateMessage(c, i18n.MsgOperationFailed)})
		return
	}
	common.ApiSuccess(c, gin.H{"task": task.ToResponse(), "created": created})
}
