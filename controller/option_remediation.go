package controller

import (
	"errors"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/service"
	"github.com/gin-gonic/gin"
)

// optionRemediateDryRunRequest is the fail-closed dry-run shape: either an
// explicit bounded key set or one bounded full snapshot, never both.
type optionRemediateDryRunRequest struct {
	Keys       []string `json:"keys"`
	IncludeAll bool     `json:"include_all"`
	Actions    []string `json:"actions"`
}

type optionRemediateApplyRequest struct {
	Items []model.OptionRemediationApplyItem `json:"items"`
}

// OptionRemediateDryRun previews whitelisted option normalizations. The
// preview is strictly zero-write: no option row, OptionMap entry, runtime
// generation, or remediation registry row is created; the only side effect is
// the management audit entry (keys and hashes, never values).
func OptionRemediateDryRun(c *gin.Context) {
	var request optionRemediateDryRunRequest
	if err := common.DecodeJsonStrict(c.Request.Body, &request); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if request.IncludeAll == (len(request.Keys) > 0) {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	preview, err := service.DryRunOptionRemediations(request.Keys, request.IncludeAll, request.Actions)
	if err != nil {
		optionRemediateError(c, err)
		return
	}
	keys := make([]string, 0, len(preview.Items))
	repairs := 0
	for _, item := range preview.Items {
		if item.Key != "" {
			keys = append(keys, item.Key)
		}
		if item.Decision == model.OptionRemediationDecisionRepair {
			repairs++
		}
	}
	recordManageAudit(c, "option.remediate_dry_run", map[string]interface{}{
		"items":   preview.Summary.Returned,
		"planned": preview.Summary.Planned,
		"repairs": repairs,
		"keys":    auditBoundedStrings(keys, auditOptionRemediateMaxKeys),
		"actions": auditBoundedStrings(request.Actions, auditOptionRemediateMaxKeys),
	})
	common.ApiSuccess(c, preview)
}

// OptionRemediateApply applies action instances previously produced by the
// dry-run. The service/model chain rebuilds every judgment, enforces the
// per-row value-hash CAS, writes the backup and registry rows in the same
// transaction as each repair, and publishes through the existing runtime
// chain. Rows are independent: skipped and failed rows are registered, never
// rolled into a fake batch atomicity.
func OptionRemediateApply(c *gin.Context) {
	var request optionRemediateApplyRequest
	if err := common.DecodeJsonStrict(c.Request.Body, &request); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	response, err := service.ApplyOptionRemediations(c.GetInt("id"), request.Items)
	if err != nil {
		optionRemediateError(c, err)
		return
	}
	items := make([]map[string]interface{}, 0, len(response.Items))
	for _, item := range response.Items {
		items = append(items, map[string]interface{}{
			"key":            item.Key,
			"action":         item.Action,
			"status":         item.Status,
			"reason":         item.Reason,
			"old_value_hash": item.OldValueHash,
			"new_value_hash": item.NewValueHash,
			"remediation_id": item.RemediationId,
		})
	}
	recordManageAudit(c, "option.remediate_apply", map[string]interface{}{
		"items":           response.Summary.Items,
		"applied":         response.Summary.Applied,
		"skipped":         response.Summary.Skipped,
		"failed":          response.Summary.Failed,
		"already_applied": response.Summary.AlreadyApplied,
		"item_results":    items,
	})
	common.ApiSuccess(c, response)
}

// GetOptionRemediations serves the paginated remediation registry. Registry
// rows contain keys, hashes, statuses, and backup references only; backup
// values never leave the database through this or any other API.
func GetOptionRemediations(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	items, total, err := service.ListOptionRemediations(pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgDatabaseError)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)
	common.ApiSuccess(c, pageInfo)
}

// optionRemediateError maps service failures onto the fail-closed error
// contract: request-shape problems are invalid params, everything backend is
// an opaque database error without parser or storage details.
func optionRemediateError(c *gin.Context, err error) {
	if errors.Is(err, model.ErrOptionRemediationInput) {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	common.ApiErrorI18n(c, i18n.MsgDatabaseError)
}

const auditOptionRemediateMaxKeys = 64

// auditBoundedStrings caps an audit string list so a bounded request cannot
// produce an unbounded audit payload.
func auditBoundedStrings(values []string, limit int) []string {
	if len(values) <= limit {
		return values
	}
	return values[:limit]
}
