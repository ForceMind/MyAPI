package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/model"

	"github.com/gin-gonic/gin"
)

// GetQuotaWriterStatus serves the admin quota-writer overview: persisted
// epoch/mode, the fail-closed durable-write audit and this process's
// in-flight billing session count.
func GetQuotaWriterStatus(c *gin.Context) {
	ctx := c.Request.Context()
	state, err := model.GetQuotaWriterEpochState(model.DB)
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgDatabaseError)
		return
	}
	audit, err := model.CanEnableDurableQuotaWrites(ctx, model.DB)
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgDatabaseError)
		return
	}
	common.ApiSuccess(c, gin.H{
		"state":             state,
		"audit":             audit,
		"inflight_sessions": model.QuotaWriterInflightSessions(),
	})
}

// GetQuotaWriterTransitionPlan serves the read-only transition plan for the
// requested target mode.
func GetQuotaWriterTransitionPlan(c *gin.Context) {
	target := model.QuotaWriterMode(strings.TrimSpace(c.Query("target")))
	if target != model.QuotaWriterModeLegacy && target != model.QuotaWriterModeBridge && target != model.QuotaWriterModeAuthoritative {
		common.ApiErrorI18n(c, i18n.MsgQuotaWriterInvalidTargetMode)
		return
	}
	plan, err := model.PlanQuotaWriterModeTransition(c.Request.Context(), model.DB, target)
	if plan == nil {
		common.ApiErrorI18n(c, i18n.MsgDatabaseError)
		return
	}
	// A failed audit still yields a reviewable plan; Ready stays false.
	_ = err
	common.ApiSuccess(c, plan)
}

type quotaWriterApplyRequest struct {
	TargetMode    string `json:"target_mode"`
	ExpectedEpoch int64  `json:"expected_epoch"`
	AckNote       string `json:"ack_note"`
}

// ApplyQuotaWriterModeTransition applies the gated mode switch. The response
// always carries the persisted evidence row when one was written; condition
// failures additionally list the missing audit checks.
func ApplyQuotaWriterModeTransition(c *gin.Context) {
	var req quotaWriterApplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	target := model.QuotaWriterMode(strings.TrimSpace(req.TargetMode))
	if target != model.QuotaWriterModeLegacy && target != model.QuotaWriterModeBridge && target != model.QuotaWriterModeAuthoritative {
		common.ApiErrorI18n(c, i18n.MsgQuotaWriterInvalidTargetMode)
		return
	}
	transition, err := model.ApplyQuotaWriterModeTransition(c.Request.Context(), model.DB, model.QuotaWriterModeTransitionInput{
		TargetMode:          target,
		ExpectedEpoch:       req.ExpectedEpoch,
		OperatorUserId:      c.GetInt("id"),
		ClusterDrainAckNote: req.AckNote,
	})
	if err != nil {
		var conditionsErr *model.QuotaWriterTransitionConditionsError
		switch {
		case errors.As(err, &conditionsErr):
			c.JSON(http.StatusConflict, gin.H{
				"success": false,
				"message": i18n.T(c, i18n.MsgQuotaWriterPreconditionsFailed),
				"data":    gin.H{"transition": transition, "missing": conditionsErr.Missing},
			})
		case errors.Is(err, model.ErrQuotaWriterTransitionConflict):
			c.JSON(http.StatusConflict, gin.H{"success": false, "message": i18n.T(c, i18n.MsgQuotaWriterTransitionConflict)})
		case errors.Is(err, model.ErrQuotaWriterTransitionInvalid):
			common.ApiErrorI18n(c, i18n.MsgQuotaWriterTransitionInvalid)
		default:
			if transition != nil && transition.Status == model.QuotaWriterTransitionStatusFailed {
				c.JSON(http.StatusInternalServerError, gin.H{
					"success": false,
					"message": i18n.T(c, i18n.MsgQuotaWriterPostAuditFailed),
					"data":    gin.H{"transition": transition},
				})
				return
			}
			common.ApiErrorI18n(c, i18n.MsgDatabaseError)
		}
		return
	}
	common.ApiSuccess(c, transition)
}

// GetQuotaWriterTransitions serves the paginated transition evidence history.
func GetQuotaWriterTransitions(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	items, total, err := model.ListQuotaWriterModeTransitions(model.DB, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgDatabaseError)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)
	common.ApiSuccess(c, pageInfo)
}

// DriveQuotaWriterDrains is the admin recovery surface that synchronously
// drives the batch update queue, balance drain generations and projection
// obligations towards zero, bounded by the requested budget.
func DriveQuotaWriterDrains(c *gin.Context) {
	budget := 10
	if raw := strings.TrimSpace(c.DefaultQuery("budget", "")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			common.ApiErrorI18n(c, i18n.MsgInvalidParams)
			return
		}
		budget = parsed
	}
	report, err := model.DriveQuotaWriterDrains(c.Request.Context(), model.DB, budget)
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgDatabaseError)
		return
	}
	common.ApiSuccess(c, report)
}
