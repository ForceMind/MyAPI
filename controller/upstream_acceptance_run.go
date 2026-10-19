package controller

import (
	"bytes"
	"io"
	"net/http"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/middleware"
	"github.com/ForceMind/MyAPI/service"
	"github.com/gin-gonic/gin"
)

func writeUpstreamAcceptanceDenied(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{
		"success": false, "code": "upstream_acceptance_denied",
		"message": common.TranslateMessage(c, message),
	})
}

func upstreamAcceptanceControllerAuthorized(c *gin.Context) bool {
	identity, ok := middleware.GetSessionAuthIdentity(c)
	if !ok || c.GetBool("use_access_token") || identity.UserID != c.GetInt("id") {
		writeUpstreamAcceptanceDenied(c, http.StatusUnauthorized, i18n.MsgAuthNotLoggedIn)
		return false
	}
	if c.GetInt("role") < common.RoleRootUser || !service.UpstreamAcceptanceModeEnabled() {
		writeUpstreamAcceptanceDenied(c, http.StatusForbidden, i18n.MsgInvalidParams)
		return false
	}
	return true
}

func StartUpstreamAcceptanceRun(c *gin.Context) {
	if !upstreamAcceptanceControllerAuthorized(c) {
		return
	}
	var input struct {
		ChannelID int                              `json:"channel_id"`
		Limits    service.UpstreamAcceptanceLimits `json:"limits"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8192)
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil || common.ValidateUniqueJSONKeys(raw) != nil || common.DecodeJsonStrict(bytes.NewReader(raw), &input) != nil || input.ChannelID <= 0 {
		writeUpstreamAcceptanceDenied(c, http.StatusBadRequest, i18n.MsgInvalidParams)
		return
	}
	run, err := service.StartUpstreamAcceptanceRun(c.Request.Context(), input.ChannelID, input.Limits)
	if err != nil {
		writeUpstreamAcceptanceDenied(c, http.StatusConflict, i18n.MsgInvalidParams)
		return
	}
	common.ApiSuccess(c, run)
}

func GetUpstreamAcceptanceRunStatus(c *gin.Context) {
	if !upstreamAcceptanceControllerAuthorized(c) {
		return
	}
	status, err := service.GetUpstreamAcceptanceStatus(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeUpstreamAcceptanceDenied(c, http.StatusConflict, i18n.MsgInvalidParams)
		return
	}
	common.ApiSuccess(c, status)
}

func StopUpstreamAcceptanceRun(c *gin.Context) {
	if !upstreamAcceptanceControllerAuthorized(c) {
		return
	}
	status, err := service.StopUpstreamAcceptanceRun(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeUpstreamAcceptanceDenied(c, http.StatusConflict, i18n.MsgInvalidParams)
		return
	}
	common.ApiSuccess(c, status)
}
