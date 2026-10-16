package controller

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/middleware"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const tokenAccountMigrationProofScope = "token.account_migration"

func tokenMaintenanceError(c *gin.Context, err error) {
	status, code, message := http.StatusServiceUnavailable, "token_maintenance_unknown", i18n.MsgOperationFailed
	switch {
	case errors.Is(err, model.ErrTokenMaintenanceInvalid):
		status, code, message = http.StatusBadRequest, "token_maintenance_invalid", i18n.MsgInvalidParams
	case errors.Is(err, model.ErrTokenMaintenanceDenied):
		status, code, message = http.StatusForbidden, "token_maintenance_denied", i18n.MsgForbidden
	case errors.Is(err, model.ErrTokenMaintenancePreconditionUnknown):
		status, code = http.StatusPreconditionFailed, "token_maintenance_precondition_unknown"
	case errors.Is(err, gorm.ErrRecordNotFound):
		status, code, message = http.StatusNotFound, "token_maintenance_not_found", i18n.MsgNotFound
	case errors.Is(err, model.ErrTokenMaintenanceConflict):
		status, code = http.StatusConflict, "token_maintenance_conflict"
	case errors.Is(err, model.ErrTokenMaintenanceBlocked):
		status, code = http.StatusConflict, "token_maintenance_blocked"
	}
	c.JSON(status, gin.H{"success": false, "code": code, "message": common.TranslateMessage(c, message)})
}

func tokenMaintenanceID(c *gin.Context) (int, error) {
	c.Header("Cache-Control", "no-store")
	tokenID, err := strconv.Atoi(c.Param("id"))
	if err != nil || tokenID <= 0 || c.Request.URL.RawQuery != "" {
		return 0, model.ErrTokenMaintenanceInvalid
	}
	return tokenID, nil
}

// Both mutations use the same exact field contract. Check names before Go's
// case-insensitive struct decoder, and reject nulls and duplicate nested keys.
func decodeTokenAccountMigrationRequest(c *gin.Context, committing bool) (model.TokenAccountMigrationCommand, error) {
	var command model.TokenAccountMigrationCommand
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8192)
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil || common.ValidateUniqueJSONKeys(raw) != nil {
		return command, model.ErrTokenMaintenanceInvalid
	}
	var fields map[string]json.RawMessage
	if common.Unmarshal(raw, &fields) != nil {
		return command, model.ErrTokenMaintenanceInvalid
	}
	want := 2
	if committing {
		want = 5
	}
	if len(fields) != want {
		return command, model.ErrTokenMaintenanceInvalid
	}
	for key, value := range fields {
		allowed := key == "target_channel_id" || key == "target_shares"
		if committing && (key == "operation_id" || key == "preview_revision" || key == "confirmed") {
			allowed = true
		}
		if !allowed || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return command, model.ErrTokenMaintenanceInvalid
		}
	}
	var shares []map[string]json.RawMessage
	if common.Unmarshal(fields["target_shares"], &shares) != nil || len(shares) == 0 || len(shares) > 2 {
		return command, model.ErrTokenMaintenanceInvalid
	}
	for _, share := range shares {
		if len(share) != 3 {
			return command, model.ErrTokenMaintenanceInvalid
		}
		for key, value := range share {
			if (key != "source_pool_id" && key != "target_pool_id" && key != "share_bps") || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
				return command, model.ErrTokenMaintenanceInvalid
			}
		}
	}
	if common.DecodeJsonStrict(bytes.NewReader(raw), &command) != nil {
		return command, model.ErrTokenMaintenanceInvalid
	}
	return command, nil
}

func GetTokenMaintenance(c *gin.Context) {
	tokenID, err := tokenMaintenanceID(c)
	if err != nil {
		tokenMaintenanceError(c, err)
		return
	}
	view, err := service.GetTokenMaintenance(c.Request.Context(), c.GetInt("id"), tokenID)
	if err != nil {
		tokenMaintenanceError(c, err)
		return
	}
	common.ApiSuccess(c, view)
}

func PreviewTokenAccountMigration(c *gin.Context) {
	tokenID, err := tokenMaintenanceID(c)
	if err != nil {
		tokenMaintenanceError(c, err)
		return
	}
	command, err := decodeTokenAccountMigrationRequest(c, false)
	if err != nil {
		tokenMaintenanceError(c, err)
		return
	}
	view, err := service.PreviewTokenAccountMigration(c.Request.Context(), c.GetInt("id"), tokenID, command.TokenAccountMigrationPreviewInput)
	if err != nil {
		tokenMaintenanceError(c, err)
		return
	}
	common.ApiSuccess(c, view)
}

func CommitTokenAccountMigration(c *gin.Context) {
	tokenID, err := tokenMaintenanceID(c)
	if err != nil {
		tokenMaintenanceError(c, err)
		return
	}
	if !middleware.RequireSecurityProof(c, tokenAccountMigrationProofScope, []string{"2fa", "passkey"}) {
		return
	}
	command, err := decodeTokenAccountMigrationRequest(c, true)
	if err != nil {
		tokenMaintenanceError(c, err)
		return
	}
	view, replayed, err := service.CommitTokenAccountMigration(c.Request.Context(), c.GetInt("id"), tokenID, command)
	if err != nil {
		tokenMaintenanceError(c, err)
		return
	}
	common.ApiSuccess(c, struct {
		*model.TokenMaintenanceOperationView
		Replayed bool `json:"replayed"`
	}{view, replayed})
}

func GetTokenMaintenanceOperation(c *gin.Context) {
	tokenID, err := tokenMaintenanceID(c)
	operationID := c.Param("operation_id")
	_, hexErr := hex.DecodeString(operationID)
	if err != nil || len(operationID) != 64 || strings.ToLower(operationID) != operationID || hexErr != nil {
		tokenMaintenanceError(c, model.ErrTokenMaintenanceInvalid)
		return
	}
	view, err := service.GetTokenMaintenanceOperation(c.Request.Context(), c.GetInt("id"), tokenID, operationID)
	if err != nil {
		tokenMaintenanceError(c, err)
		return
	}
	common.ApiSuccess(c, struct {
		*model.TokenMaintenanceOperationView
		Replayed bool `json:"replayed"`
	}{view, false})
}
