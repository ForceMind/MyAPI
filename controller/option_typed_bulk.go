package controller

import (
	"errors"
	"net/http"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/model"

	"github.com/gin-gonic/gin"
)

// TypedBulkOptionsUpdateRequest is the frozen typed bulk request contract
// (C09-N3a): a bounded item set with a closed value-type union, plus an
// optional expected-revision CAS guard.
type TypedBulkOptionsUpdateRequest struct {
	ExpectedRevision *int64                  `json:"expected_revision"`
	Items            []model.TypedBulkOption `json:"items"`
}

// UpdateOptionsTypedBulk handles PUT /api/option/typed-bulk.
//
// Status contract: 400 for malformed bodies and rejected items (unknown or
// duplicate keys, out-of-range bounds, invalid values), 409 for an
// expected-revision mismatch (zero writes, current revision echoed for
// reload), 500 for database failures and post-commit publication aborts.
// On success the new revision and the sorted applied key list are returned.
func UpdateOptionsTypedBulk(c *gin.Context) {
	var request TypedBulkOptionsUpdateRequest
	if err := common.DecodeJsonStrict(c.Request.Body, &request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": common.TranslateMessage(c, i18n.MsgOptionTypedBulkInvalidBody),
		})
		return
	}
	revision, applied, err := model.UpdateOptionsTypedBulk(request.Items, request.ExpectedRevision)
	if err != nil {
		var fieldErr *model.TypedBulkFieldError
		var conflictErr *model.TypedBulkRevisionConflictError
		switch {
		case errors.As(err, &conflictErr):
			c.JSON(http.StatusConflict, gin.H{
				"success": false,
				"message": common.TranslateMessage(c, i18n.MsgOptionTypedBulkRevisionConflict, map[string]any{
					"Expected": conflictErr.Expected,
					"Actual":   conflictErr.Actual,
				}),
				"data": gin.H{"revision": conflictErr.Actual},
			})
		case errors.As(err, &fieldErr):
			messageKey := i18n.MsgOptionTypedBulkInvalidValue
			switch fieldErr.Kind {
			case model.ErrTypedBulkUnknownKey:
				messageKey = i18n.MsgOptionTypedBulkUnknownKey
			case model.ErrTypedBulkDuplicateKey:
				messageKey = i18n.MsgOptionTypedBulkDuplicateKey
			case model.ErrTypedBulkItemCount:
				messageKey = i18n.MsgOptionTypedBulkItemCount
			}
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": common.TranslateMessage(c, messageKey, map[string]any{
					"Key":    fieldErr.Key,
					"Reason": fieldErr.Reason,
					"Max":    model.MaxTypedBulkItems,
				}),
			})
		case errors.Is(err, model.ErrTypedBulkPublishFailed):
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": common.TranslateMessage(c, i18n.MsgOptionTypedBulkPublishFailed, map[string]any{
					"Reason": err.Error(),
				}),
			})
		default:
			common.SysError("typed bulk update failed: " + err.Error())
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": common.TranslateMessage(c, i18n.MsgOptionTypedBulkInternal),
			})
		}
		return
	}
	// 出于安全考虑只记录被修改的配置项名称，不记录配置值（可能含密钥等敏感信息）。
	recordManageAudit(c, "option.typed_bulk_update", map[string]interface{}{"keys": applied})
	common.ApiSuccess(c, gin.H{"revision": revision, "applied": applied})
}

// GetOptionsTypedBulkRevision handles GET /api/option/typed-bulk/revision and
// exposes the current persistent typed bulk revision so admin forms can load
// it before issuing a CAS-guarded bulk save.
func GetOptionsTypedBulkRevision(c *gin.Context) {
	revision, err := model.CurrentTypedBulkRevision()
	if err != nil {
		common.SysError("typed bulk revision read failed: " + err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": common.TranslateMessage(c, i18n.MsgOptionTypedBulkInternal),
		})
		return
	}
	common.ApiSuccess(c, gin.H{"revision": revision})
}
