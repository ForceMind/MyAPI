package controller

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/logger"
	"github.com/ForceMind/MyAPI/model"
	"github.com/gin-gonic/gin"
)

func GetTokenBudget(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		usageReviewError(c, model.ErrTokenBudgetInvalid)
		return
	}
	view, err := model.ReadTokenBudget(c.Request.Context(), model.DB, c.GetInt("id"), id)
	if err != nil {
		usageReviewError(c, err)
		return
	}
	common.ApiSuccess(c, view)
}

func UpdateTokenBudget(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	tokenID, err := strconv.Atoi(c.Param("id"))
	var request struct {
		ID               string `json:"id"`
		ExpectedRevision int64  `json:"expected_revision"`
		Enabled          *bool  `json:"enabled"`
		Limit            *int64 `json:"limit"`
		Confirmed        bool   `json:"confirmed"`
		Fee              *struct {
			Enabled  *bool   `json:"enabled"`
			LimitUSD *string `json:"limit_usd"`
		} `json:"fee"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	if err != nil || tokenID <= 0 || c.Request.URL.RawQuery != "" || common.DecodeJsonStrict(c.Request.Body, &request) != nil || request.Enabled == nil || request.Limit == nil || !request.Confirmed {
		usageReviewError(c, model.ErrTokenBudgetInvalid)
		return
	}
	var fee *model.FeeBudgetPolicyInput
	if request.Fee != nil {
		if request.Fee.Enabled == nil || request.Fee.LimitUSD == nil {
			usageReviewError(c, model.ErrFeeBudgetInvalid)
			return
		}
		fee = &model.FeeBudgetPolicyInput{Enabled: *request.Fee.Enabled, LimitUSD: *request.Fee.LimitUSD}
	}
	_, err = model.ConfigureTokenBudget(c.Request.Context(), model.DB, c.GetInt("id"), model.TokenBudgetPolicyInput{ID: request.ID, TokenID: tokenID, ExpectedRevision: request.ExpectedRevision, Enabled: *request.Enabled, Limit: *request.Limit, Fee: fee})
	if err != nil {
		usageReviewError(c, err)
		return
	}
	GetTokenBudget(c)
}

func RecoverTokenBudget(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	tokenID, err := strconv.Atoi(c.Param("id"))
	var request struct {
		RequestID   string  `json:"request_id"`
		Action      string  `json:"action"`
		ActualQuota *int64  `json:"actual_quota"`
		FeeUSD      *string `json:"actual_fee_usd"`
		Input       *int64  `json:"actual_input_tokens"`
		Output      *int64  `json:"actual_output_tokens"`
		Evidence    string  `json:"evidence_reference"`
		Confirmed   bool    `json:"confirmed_reliable_evidence"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	if err != nil || tokenID <= 0 || c.Request.URL.RawQuery != "" || common.DecodeJsonStrict(c.Request.Body, &request) != nil || !request.Confirmed {
		usageReviewError(c, model.ErrTokenBudgetInvalid)
		return
	}
	if request.Action == "cancel_not_sent" {
		if request.Input != nil || request.Output != nil || request.ActualQuota != nil || request.FeeUSD != nil {
			usageReviewError(c, model.ErrTokenBudgetInvalid)
			return
		}
		row, err := model.CancelTokenBudgetBeforeSend(c.Request.Context(), model.DB, c.GetInt("id"), tokenID, request.RequestID, request.Evidence)
		if err != nil {
			if row != nil {
				c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "code": "token_budget_cancel_quota_pending", "data": gin.H{"reservation": row}})
				return
			}
			usageReviewError(c, err)
			return
		}
	} else if request.Action == "reconcile" {
		if request.Input == nil || request.Output == nil || request.ActualQuota == nil {
			usageReviewError(c, model.ErrTokenBudgetInvalid)
			return
		}
		request.Evidence = strings.TrimSpace(request.Evidence)
		if request.Evidence == "" || len(request.Evidence) > 2048 || *request.ActualQuota < 0 || *request.ActualQuota > int64(common.MaxQuota) || *request.Input < 0 || *request.Input > int64(common.MaxQuota) || *request.Output < 0 || *request.Output > int64(common.MaxQuota)-*request.Input {
			usageReviewError(c, model.ErrTokenBudgetInvalid)
			return
		}
		if request.FeeUSD != nil {
			normalized, err := model.NormalizeFeeBudgetUSD(*request.FeeUSD)
			if err != nil {
				usageReviewError(c, err)
				return
			}
			request.FeeUSD = &normalized
		}
		_, err := model.PrepareTokenBudgetUsageReview(c.Request.Context(), model.DB, c.GetInt("id"), tokenID, request.RequestID, request.FeeUSD)
		if err != nil {
			usageReviewError(c, err)
			return
		}
		view, err := model.ReconcileUsageReview(c.Request.Context(), model.DB, c.GetInt("id"), request.RequestID, *request.ActualQuota, request.Evidence, model.UsageReviewTokenCounts{Input: *request.Input, Output: *request.Output, FeeUSD: request.FeeUSD})
		if err != nil {
			usageReviewError(c, err)
			return
		}
		if view.Decision != nil {
			if err := model.ProjectUsageReviewDecision(c.Request.Context(), model.DB, model.LOG_DB, view.Decision.ID); err != nil {
				logger.LogWarn(c, "token budget review projection remains pending")
			}
		}
	} else {
		usageReviewError(c, model.ErrTokenBudgetInvalid)
		return
	}
	GetTokenBudget(c)
}
