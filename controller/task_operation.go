package controller

import (
	"errors"
	"net/http"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/middleware"
	"github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/ForceMind/MyAPI/service"
	"github.com/gin-gonic/gin"
)

// GetTaskOperation returns one sanitized durable operation to its owner.
func GetTaskOperation(c *gin.Context) {
	var tokenID *int
	if scopedTokenID, tokenScoped := middleware.GetTaskOperationTokenID(c); tokenScoped {
		if scopedTokenID <= 0 {
			writeTaskOperationError(c, http.StatusUnauthorized, common.TranslateMessage(c, i18n.MsgTokenInvalid), types.ErrorCodeAccessDenied)
			return
		}
		tokenID = &scopedTokenID
	}
	operation, err := service.GetTaskOperation(c.Param("id"), c.GetInt("id"), tokenID)
	if err == nil {
		c.JSON(http.StatusOK, operation)
		return
	}
	if errors.Is(err, service.ErrTaskOperationNotFound) {
		writeTaskOperationError(c, http.StatusNotFound, common.TranslateMessage(c, i18n.MsgNotFound), types.ErrorCodeInvalidRequest)
		return
	}
	writeTaskOperationError(c, http.StatusInternalServerError, common.TranslateMessage(c, i18n.MsgDatabaseError), types.ErrorCodeQueryDataError)
}

func writeTaskOperationError(c *gin.Context, status int, message string, code types.ErrorCode) {
	c.JSON(status, gin.H{
		"error": gin.H{
			"message": message,
			"type":    "invalid_request_error",
			"code":    string(code),
		},
	})
}
