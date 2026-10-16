package controller

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type tokenModelLimitsRequest struct {
	ExpectedRevision   string    `json:"expected_revision"`
	ModelLimitsEnabled *bool     `json:"model_limits_enabled"`
	Models             *[]string `json:"models"`
}

func (request *tokenModelLimitsRequest) UnmarshalJSON(data []byte) error {
	if err := common.ValidateUniqueJSONKeys(data); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := common.Unmarshal(data, &fields); err != nil {
		return err
	}
	for name := range fields {
		switch name {
		case "expected_revision", "model_limits_enabled", "models":
		default:
			return model.ErrTokenModelLimitsInvalid
		}
	}
	// encoding/json otherwise converts null string elements into empty IDs,
	// which could accidentally match an empty segment in a legacy allowlist.
	if raw, exists := fields["models"]; exists {
		var names []*string
		if err := common.Unmarshal(raw, &names); err != nil {
			return err
		}
		for _, name := range names {
			if name == nil {
				return model.ErrTokenModelLimitsInvalid
			}
		}
	}

	type requestAlias tokenModelLimitsRequest
	var decoded requestAlias
	if err := common.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*request = tokenModelLimitsRequest(decoded)
	return nil
}

func tokenModelLimitsError(c *gin.Context, err error) {
	status, code, message := http.StatusServiceUnavailable, "token_model_limits_unavailable", i18n.MsgOperationFailed
	switch {
	case errors.Is(err, model.ErrTokenModelLimitsInvalid):
		status, code, message = http.StatusBadRequest, "token_model_limits_invalid", i18n.MsgInvalidParams
	case errors.Is(err, model.ErrTokenModelLimitsDenied):
		status, code, message = http.StatusForbidden, "token_model_limits_denied", i18n.MsgForbidden
	case errors.Is(err, gorm.ErrRecordNotFound):
		status, code, message = http.StatusNotFound, "token_model_limits_not_found", i18n.MsgNotFound
	case errors.Is(err, model.ErrTokenModelLimitsConflict):
		status, code, message = http.StatusConflict, "token_model_limits_conflict", i18n.MsgTokenModelLimitsConflict
	}
	c.JSON(status, gin.H{"success": false, "code": code, "message": common.TranslateMessage(c, message)})
}

func GetTokenModelLimits(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	tokenID, err := strconv.Atoi(c.Param("id"))
	if err != nil || tokenID <= 0 || c.Request.URL.RawQuery != "" {
		tokenModelLimitsError(c, model.ErrTokenModelLimitsInvalid)
		return
	}
	view, err := service.GetTokenModelLimits(c.Request.Context(), c.GetInt("id"), tokenID)
	if err != nil {
		tokenModelLimitsError(c, err)
		return
	}
	common.ApiSuccess(c, view)
}

func UpdateTokenModelLimits(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	tokenID, err := strconv.Atoi(c.Param("id"))
	var request tokenModelLimitsRequest
	// Allow escaped JSON for the portable TEXT-column limit while bounding input.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 6*model.MaxTokenModelLimitsBytes+4096)
	if err != nil || tokenID <= 0 || c.Request.URL.RawQuery != "" || common.DecodeJsonStrict(c.Request.Body, &request) != nil || request.ModelLimitsEnabled == nil || request.Models == nil {
		tokenModelLimitsError(c, model.ErrTokenModelLimitsInvalid)
		return
	}
	command := model.TokenModelLimitsCommand{ExpectedRevision: request.ExpectedRevision, ModelLimitsEnabled: *request.ModelLimitsEnabled, Models: *request.Models}
	view, err := service.SetTokenModelLimits(c.Request.Context(), c.GetInt("id"), tokenID, command)
	if err != nil {
		tokenModelLimitsError(c, err)
		return
	}
	common.ApiSuccess(c, view)
}
