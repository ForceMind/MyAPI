package controller

import (
	"errors"
	"io"
	"net/http"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/middleware"
	"github.com/ForceMind/MyAPI/service"
	"github.com/gin-gonic/gin"
)

func operationsResult(c *gin.Context, action, digest string) {
	data, err := service.OperationsBridge(c.Request.Context(), c.GetInt("id"), action, c.Param("id"), digest)
	if err != nil {
		status, code := http.StatusServiceUnavailable, "OPERATIONS_UNAVAILABLE"
		var failure *service.OperationsError
		if errors.As(err, &failure) {
			status, code = failure.Status, failure.Code
		}
		c.JSON(status, gin.H{"success": false, "code": code, "message": common.TranslateMessage(c, i18n.MsgOperationFailed)})
		return
	}
	if action == "approve" {
		recordManageAudit(c, "operations.upgrade_approved", map[string]interface{}{"plan_id": c.Param("id"), "digest": digest})
	}
	common.ApiSuccess(c, data)
}

func ListOperationsPlans(c *gin.Context)     { operationsResult(c, "plans", "") }
func ListOperationsInstances(c *gin.Context) { operationsResult(c, "instances", "") }
func GetOperationsPlan(c *gin.Context)       { operationsResult(c, "plan", "") }

func ApproveOperationsPlan(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
	body, err := io.ReadAll(c.Request.Body)
	var input map[string]string
	if err != nil || common.ValidateUniqueJSONKeys(body) != nil || common.Unmarshal(body, &input) != nil || len(input) != 1 || !service.ValidOperationsPlan(c.Param("id"), input["digest"]) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "code": "OPERATIONS_INVALID_PLAN", "message": common.TranslateMessage(c, i18n.MsgInvalidParams)})
		return
	}
	// The existing 2FA/Passkey proof is bound to this exact immutable plan.
	// It cannot authorize another plan during the proof's validity window.
	if !middleware.RequireSecurityProof(c, service.OperationsApprovalScopeFor(c.Param("id"), input["digest"]), []string{"2fa", "passkey"}) {
		return
	}
	operationsResult(c, "approve", input["digest"])
}
