package controller

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/service"
	"github.com/gin-gonic/gin"
)

func ListManagementKeys(c *gin.Context) {
	keys, err := model.ListManagementKeys(c.GetInt("id"))
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgDatabaseError)
		return
	}
	common.ApiSuccess(c, keys)
}

func CreateManagementKey(c *gin.Context) {
	var input service.CreateManagementKeyInput
	if c.ShouldBindJSON(&input) != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"success": false, "code": "MANAGEMENT_INVALID_INPUT", "message": common.TranslateMessage(c, i18n.MsgInvalidParams)})
		return
	}
	owner, err := model.GetUserById(c.GetInt("id"), false)
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgDatabaseError)
		return
	}
	metadata, secret, err := service.CreateManagementKey(owner.ToBaseUser(), input)
	if err != nil {
		if errors.Is(err, service.ErrManagementKeyInput) {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"success": false, "code": "MANAGEMENT_INVALID_INPUT", "message": common.TranslateMessage(c, i18n.MsgInvalidParams)})
			return
		}
		common.ApiErrorI18n(c, i18n.MsgDatabaseError)
		return
	}
	recordManageAudit(c, "management_key.created", map[string]interface{}{"key_id": metadata.ID, "owner_id": owner.Id})
	common.ApiSuccess(c, gin.H{"key": metadata, "secret": secret})
}

func RevokeManagementKey(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if err := model.RevokeManagementKey(c.GetInt("id"), id, time.Now().Unix()); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	recordManageAudit(c, "management_key.revoked", map[string]interface{}{"key_id": id, "owner_id": c.GetInt("id")})
	common.ApiSuccess(c, nil)
}
