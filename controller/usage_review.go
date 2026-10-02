package controller

import (
	"errors"
	"net/http"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/logger"
	"github.com/ForceMind/MyAPI/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func GetUsageReview(c *gin.Context) {
	result, err := model.GetUsageReview(c.Request.Context(), model.DB, c.GetInt("id"), c.Param("request_id"))
	if err != nil {
		usageReviewError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

func ReconcileUsageReview(c *gin.Context) {
	var request struct {
		ActualInputTokens         *int64 `json:"actual_input_tokens"`
		ActualOutputTokens        *int64 `json:"actual_output_tokens"`
		ActualQuota               *int64 `json:"actual_quota"`
		EvidenceReference         string `json:"evidence_reference"`
		ConfirmedReliableEvidence bool   `json:"confirmed_reliable_evidence"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	if len(c.Request.URL.Query()) != 0 || common.DecodeJsonStrict(c.Request.Body, &request) != nil || request.ActualQuota == nil || !request.ConfirmedReliableEvidence {
		usageReviewError(c, model.ErrAccountQuotaMutationInvalidInput)
		return
	}
	if (request.ActualInputTokens == nil) != (request.ActualOutputTokens == nil) {
		usageReviewError(c, model.ErrTokenBudgetInvalid)
		return
	}
	var counts []model.UsageReviewTokenCounts
	if request.ActualInputTokens != nil {
		counts = append(counts, model.UsageReviewTokenCounts{Input: *request.ActualInputTokens, Output: *request.ActualOutputTokens})
	}
	result, err := model.ReconcileUsageReview(c.Request.Context(), model.DB, c.GetInt("id"), c.Param("request_id"), *request.ActualQuota, request.EvidenceReference, counts...)
	if err != nil {
		usageReviewError(c, err)
		return
	}
	if result.Decision != nil {
		if err := model.ProjectUsageReviewDecision(c.Request.Context(), model.DB, model.LOG_DB, result.Decision.ID); err != nil {
			logger.LogWarn(c, "usage review projection remains pending")
		} else if refreshed, err := model.GetUsageReview(c.Request.Context(), model.DB, c.GetInt("id"), c.Param("request_id")); err == nil {
			result = refreshed
		}
	}
	common.ApiSuccess(c, result)
}

func usageReviewError(c *gin.Context, err error) {
	status, code := http.StatusInternalServerError, "usage_review_unavailable"
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		status, code = http.StatusNotFound, "usage_review_not_found"
	case errors.Is(err, model.ErrAccountQuotaMutationIneligible):
		status, code = http.StatusForbidden, "usage_review_forbidden"
	case errors.Is(err, model.ErrAccountQuotaMutationInvalidInput), errors.Is(err, model.ErrTokenBudgetInvalid), errors.Is(err, model.ErrTokenBudgetExceeded):
		status, code = http.StatusBadRequest, "usage_review_invalid_input"
	case errors.Is(err, model.ErrAccountQuotaMutationConflict), errors.Is(err, model.ErrAccountQuotaUsageUnresolved), errors.Is(err, model.ErrAccountQuotaSettlementPending), errors.Is(err, model.ErrTokenBudgetConflict), errors.Is(err, model.ErrTokenBudgetPending):
		status, code = http.StatusConflict, "usage_review_conflict"
	}
	c.JSON(status, gin.H{"success": false, "code": code, "message": common.TranslateMessage(c, i18n.MsgOperationFailed)})
}
