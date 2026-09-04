package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/middleware"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/service"
	"github.com/gin-gonic/gin"
)

const codexLocalAuthCreateBodyLimit = 256 << 10

var inspectCodexLocalAuth = service.InspectCodexLocalAuth

func GetCodexLocalAuthStatus(c *gin.Context) {
	if !requireCodexLocalAuthDashboardSession(c) {
		return
	}
	inspection, _ := inspectCodexLocalAuth(common.IsRunningInContainer())
	c.JSON(http.StatusOK, gin.H{"success": true, "data": inspection})
}

func ImportCodexLocalAuth(c *gin.Context) {
	if !requireCodexLocalAuthDashboardSession(c) {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, codexLocalAuthCreateBodyLimit)
	var request AddChannelRequest
	if err := common.DecodeJsonStrict(c.Request.Body, &request); err != nil {
		common.ApiError(c, errors.New("invalid Codex local import request"))
		return
	}
	if request.Mode != "single" || request.BatchAddSetKeyPrefix2Name || request.Channel == nil {
		common.ApiError(c, errors.New("Codex local import requires one channel"))
		return
	}
	channel := request.Channel
	if channel.Id != 0 || channel.Type != constant.ChannelTypeCodex || channel.ChannelInfo.IsMultiKey || strings.TrimSpace(channel.Key) != "" {
		common.ApiError(c, errors.New("invalid Codex local import channel"))
		return
	}

	inspection, key := inspectCodexLocalAuth(common.IsRunningInContainer())
	if !writeCodexLocalAuthUnavailable(c, inspection, key) {
		return
	}
	encoded, err := common.Marshal(key)
	if err != nil {
		common.ApiError(c, errors.New("failed to prepare Codex credential"))
		return
	}
	channel.Key = string(encoded)
	channel.CreatedTime = common.GetTimestamp()
	if err := validateChannel(channel, true); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	if err := model.InsertChannelWithAbilities(c.Request.Context(), channel); err != nil {
		common.ApiError(c, err)
		return
	}
	// Creating a channel also creates new group/model routing abilities, which
	// require the normal full cache rebuild. Credential-only updates below use
	// the CAS helper's targeted cache update instead.
	model.InitChannelCache()
	recordManageAudit(c, "channel.codex_local_import.create", map[string]interface{}{
		"channel_id": channel.Id,
		"source":     "local_codex_auth",
	})
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    codexLocalAuthImportResult(channel.Id, "created", inspection),
	})
}

func ImportCodexLocalAuthForChannel(c *gin.Context) {
	if !requireCodexLocalAuthDashboardSession(c) {
		return
	}
	channelID, err := strconv.Atoi(c.Param("id"))
	if err != nil || channelID <= 0 {
		common.ApiError(c, errors.New("invalid channel id"))
		return
	}
	channel, err := model.GetChannelById(channelID, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if channel.Type != constant.ChannelTypeCodex || channel.ChannelInfo.IsMultiKey {
		common.ApiError(c, errors.New("local credential import requires a single-key Codex channel"))
		return
	}

	inspection, key := inspectCodexLocalAuth(common.IsRunningInContainer())
	if !writeCodexLocalAuthUnavailable(c, inspection, key) {
		return
	}
	encoded, err := common.Marshal(key)
	if err != nil {
		common.ApiError(c, errors.New("failed to prepare Codex credential"))
		return
	}
	newKey := string(encoded)
	updated, updateErr := model.UpdateChannelCredentialIfUnchanged(
		c.Request.Context(),
		channel.Id,
		constant.ChannelTypeCodex,
		channel.Key,
		newKey,
	)
	if updateErr != nil {
		common.ApiError(c, updateErr)
		return
	}
	if !updated {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"code":    "CODEX_LOCAL_AUTH_CONFLICT",
			"message": "The channel credential changed while the local credential was being imported. Try again.",
		})
		return
	}
	recordManageAudit(c, "channel.codex_local_import.update", map[string]interface{}{
		"channel_id": channel.Id,
		"source":     "local_codex_auth",
	})
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    codexLocalAuthImportResult(channel.Id, "updated", inspection),
	})
}

func requireCodexLocalAuthDashboardSession(c *gin.Context) bool {
	if middleware.IsLANEdition() {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"code":    "MYAPI_LAN_ROUTE_DISABLED",
			"message": "Codex local credential import is not available in MyAPI LAN edition",
		})
		return false
	}
	if _, ok := middleware.GetSessionAuthIdentity(c); !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"code":    "AUTH_SESSION_REQUIRED",
			"message": "A live dashboard session is required",
		})
		return false
	}
	return true
}

func writeCodexLocalAuthUnavailable(c *gin.Context, inspection service.CodexLocalAuthInspection, key *service.CodexOAuthKey) bool {
	if inspection.State == service.CodexLocalAuthReady && key != nil {
		return true
	}
	c.JSON(http.StatusConflict, gin.H{
		"success": false,
		"code":    "CODEX_LOCAL_AUTH_" + strings.ToUpper(string(inspection.State)),
		"message": "A usable local Codex credential is not available",
		"data":    inspection,
	})
	return false
}

func codexLocalAuthImportResult(channelID int, operation string, inspection service.CodexLocalAuthInspection) gin.H {
	return gin.H{
		"channel_id":   channelID,
		"operation":    operation,
		"account_hint": inspection.AccountHint,
		"email_hint":   inspection.EmailHint,
		"last_refresh": inspection.LastRefresh,
		"can_refresh":  inspection.CanRefresh,
	}
}
