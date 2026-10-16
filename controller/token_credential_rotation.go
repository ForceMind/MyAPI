package controller

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/middleware"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	tokenCredentialRotationProofScope        = "token.credential_rotation"
	tokenCredentialRotationDispositionHeader = "X-MyAPI-Credential-Rotation-Disposition"
)

type tokenCredentialRotationReveal struct {
	Secret string `json:"secret"`
}

func tokenCredentialRotationError(c *gin.Context, err error) {
	status, code, message := http.StatusServiceUnavailable, "credential_rotation_unknown", i18n.MsgOperationFailed
	disposition := "unknown"
	switch {
	case errors.Is(err, model.ErrTokenCredentialRotationOutcomeUnknown):
		// The transaction may already have committed. Do not let an underlying
		// cache/read error turn this into a pre-execution rejection.
	case errors.Is(err, model.ErrTokenCredentialRotationOperationConflict):
		status, code = http.StatusConflict, "credential_rotation_operation_conflict"
	case errors.Is(err, model.ErrTokenMaintenanceInvalid):
		status, code, message, disposition = http.StatusBadRequest, "credential_rotation_invalid", i18n.MsgInvalidParams, "not_accepted"
	case errors.Is(err, model.ErrTokenMaintenanceDenied):
		status, code, message, disposition = http.StatusForbidden, "credential_rotation_denied", i18n.MsgForbidden, "not_accepted"
	case errors.Is(err, model.ErrTokenMaintenancePreconditionUnknown):
		status, code, disposition = http.StatusPreconditionFailed, "credential_rotation_precondition_unknown", "not_accepted"
	case errors.Is(err, model.ErrTokenMaintenanceConflict):
		status, code, disposition = http.StatusConflict, "credential_rotation_conflict", "not_accepted"
	case errors.Is(err, model.ErrTokenMaintenanceBlocked):
		status, code, disposition = http.StatusConflict, "credential_rotation_blocked", "not_accepted"
	case errors.Is(err, gorm.ErrRecordNotFound):
		status, code, message, disposition = http.StatusNotFound, "credential_rotation_not_found", i18n.MsgNotFound, "not_accepted"
	}
	c.Header(tokenCredentialRotationDispositionHeader, disposition)
	c.JSON(status, gin.H{
		"success": false, "code": code, "message": common.TranslateMessage(c, message),
		"data": gin.H{"commit_disposition": disposition},
	})
}

func decodeTokenCredentialRotationRequest(c *gin.Context, committing bool) (model.TokenCredentialRotationCommand, error) {
	var command model.TokenCredentialRotationCommand
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8192)
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil || common.ValidateUniqueJSONKeys(raw) != nil {
		return command, model.ErrTokenMaintenanceInvalid
	}
	var fields map[string]json.RawMessage
	if common.Unmarshal(raw, &fields) != nil || fields == nil {
		return command, model.ErrTokenMaintenanceInvalid
	}
	want := 0
	if committing {
		want = 3
	}
	if len(fields) != want {
		return command, model.ErrTokenMaintenanceInvalid
	}
	for key, value := range fields {
		if !committing || (key != "operation_id" && key != "preview_revision" && key != "confirmed") ||
			bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return command, model.ErrTokenMaintenanceInvalid
		}
	}
	if common.DecodeJsonStrict(bytes.NewReader(raw), &command) != nil {
		return command, model.ErrTokenMaintenanceInvalid
	}
	return command, nil
}

func PreviewTokenCredentialRotation(c *gin.Context) {
	tokenID, err := tokenMaintenanceID(c)
	if err != nil {
		tokenCredentialRotationError(c, err)
		return
	}
	if _, err = decodeTokenCredentialRotationRequest(c, false); err != nil {
		tokenCredentialRotationError(c, err)
		return
	}
	view, err := service.PreviewTokenCredentialRotation(c.Request.Context(), c.GetInt("id"), tokenID)
	if err != nil {
		tokenCredentialRotationError(c, err)
		return
	}
	common.ApiSuccess(c, view)
}

func CommitTokenCredentialRotation(c *gin.Context) {
	tokenID, err := tokenMaintenanceID(c)
	if err != nil {
		tokenCredentialRotationError(c, err)
		return
	}
	identity, ok := middleware.GetSessionAuthIdentity(c)
	if !ok {
		tokenCredentialRotationError(c, model.ErrTokenMaintenanceDenied)
		return
	}
	if _, err = service.VerifySecurityProof(strings.TrimSpace(c.GetHeader("X-Security-Proof")), identity,
		tokenCredentialRotationProofScope, []string{"2fa", "passkey"}); err != nil {
		tokenCredentialRotationError(c, model.ErrTokenMaintenanceDenied)
		return
	}
	command, err := decodeTokenCredentialRotationRequest(c, true)
	if err != nil {
		tokenCredentialRotationError(c, err)
		return
	}
	// From this point even a database error is conservative: only the model's
	// explicit pre-apply error classes can prove that no rotation was accepted.
	c.Header(tokenCredentialRotationDispositionHeader, "unknown")
	receipt, newKey, replayed, err := service.CommitTokenCredentialRotation(c.Request.Context(), c.GetInt("id"), tokenID, command)
	if err != nil {
		tokenCredentialRotationError(c, err)
		return
	}
	if receipt == nil || (!replayed && newKey == "") || (replayed && newKey != "") {
		tokenCredentialRotationError(c, model.ErrTokenCredentialRotationOutcomeUnknown)
		return
	}
	var reveal *tokenCredentialRotationReveal
	if !replayed && newKey != "" {
		reveal = &tokenCredentialRotationReveal{Secret: "sk-" + newKey}
	}
	common.ApiSuccess(c, struct {
		Receipt  *model.TokenCredentialRotationOperationView `json:"receipt"`
		Replayed bool                                        `json:"replayed"`
		Reveal   *tokenCredentialRotationReveal              `json:"reveal"`
	}{Receipt: receipt, Replayed: replayed, Reveal: reveal})
}

func GetTokenCredentialRotationOperation(c *gin.Context) {
	tokenID, err := tokenMaintenanceID(c)
	operationID := c.Param("operation_id")
	_, hexErr := hex.DecodeString(operationID)
	if err != nil || len(operationID) != 64 || strings.ToLower(operationID) != operationID || hexErr != nil {
		tokenCredentialRotationError(c, model.ErrTokenMaintenanceInvalid)
		return
	}
	view, err := service.GetTokenCredentialRotationOperation(c.Request.Context(), c.GetInt("id"), tokenID, operationID)
	if err != nil {
		tokenCredentialRotationError(c, err)
		return
	}
	common.ApiSuccess(c, view)
}
