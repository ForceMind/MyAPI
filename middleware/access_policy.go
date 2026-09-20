package middleware

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/logger"
	"github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/ForceMind/MyAPI/service/accesspolicy"
	"github.com/ForceMind/MyAPI/setting"
	"github.com/ForceMind/MyAPI/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

// accessPolicyRejectsSelectedGroup runs the D04 evaluation after legacy route
// resolution. It has no response side effect, so every selected-channel path
// (initial selection, retry, task submission and a locked channel replay) can
// use the same enforcement decision before it contacts an upstream.
func accessPolicyRejectsSelectedGroup(c *gin.Context, usingGroup, usingModel string) bool {
	mode := setting.GetAccessPolicyMode()
	if mode == setting.AccessPolicyModeOff {
		return false
	}

	source := accesspolicy.SnapshotSource{
		AccountTierID:    common.GetContextKeyString(c, constant.ContextKeyAccountTierID),
		AccessProfileID:  common.GetContextKeyString(c, constant.ContextKeyAccessProfileID),
		LegacyTokenGroup: common.GetContextKeyString(c, constant.ContextKeyTokenGroup),
		UsingGroup:       usingGroup,
		UsingModel:       usingModel,
		LegacyAllowed:    true,
		GroupRatio:       legacyGroupRatioString(usingGroup),
	}

	policyMode := accesspolicy.PolicyModeAudit
	if mode == setting.AccessPolicyModeEnforce {
		policyMode = accesspolicy.PolicyModeEnforce
	}
	enforceScope := mode == setting.AccessPolicyModeEnforce && setting.AccessPolicyEnforcedForGroup(usingGroup)
	decision, err := accesspolicy.EvaluateRequest(policyMode, source, 0)
	if err != nil {
		logger.LogWarn(c.Request.Context(), "access policy evaluation failed: "+err.Error())
		return enforceScope
	}

	if len(decision.Findings) > 0 {
		logger.LogWarn(c.Request.Context(), formatAccessPolicyAudit(c, usingGroup, decision))
	}

	if !enforceScope {
		return false
	}
	return len(accesspolicy.BlockingFindings(decision)) > 0
}

// EnforceAccessPolicyForSelectedGroup returns a non-retryable 403 error when
// the final selected group and model are outside scoped D04 entitlements. It
// is used by controller retry paths, which must return their normal protocol
// error instead of having middleware write a second response.
func EnforceAccessPolicyForSelectedGroup(c *gin.Context, usingGroup, usingModel string) *types.NewAPIError {
	if !accessPolicyRejectsSelectedGroup(c, usingGroup, usingModel) {
		return nil
	}
	return types.NewError(
		errors.New(i18n.T(c, i18n.MsgDistributorPolicyDenied, map[string]any{"Group": usingGroup})),
		types.ErrorCodeAccessDenied,
		types.ErrOptionWithStatusCode(http.StatusForbidden),
		types.ErrOptionWithSkipRetry(),
	)
}

// applyAccessPolicyDecision adapts the shared selected-group enforcement to
// the initial middleware path, which owns the HTTP response at that point.
func applyAccessPolicyDecision(c *gin.Context, usingGroup, usingModel string) bool {
	if policyErr := EnforceAccessPolicyForSelectedGroup(c, usingGroup, usingModel); policyErr != nil {
		abortWithOpenAiMessage(c, policyErr.StatusCode, policyErr.Error(), policyErr.GetErrorCode())
		return true
	}
	return false
}

func formatAccessPolicyAudit(c *gin.Context, usingGroup string, decision accesspolicy.PolicyDecision) string {
	requestID, _ := c.Get(common.RequestIdKey)
	tokenID := ""
	if id, ok := common.GetContextKey(c, constant.ContextKeyTokenId); ok {
		if typed, ok := id.(int); ok {
			tokenID = strconv.Itoa(typed)
		}
	}
	message := "access policy audit: digest=" + decision.Snapshot.ContentDigest +
		" group=" + usingGroup + " findings="
	for index, finding := range decision.Findings {
		if index > 0 {
			message += ","
		}
		message += finding.Code
		if finding.Subject != "" {
			message += "(" + finding.Subject + ")"
		}
	}
	if id, ok := requestID.(string); ok && id != "" {
		message += " request_id=" + id
	}
	if tokenID != "" {
		message += " token_id=" + tokenID
	}
	return message
}

// legacyGroupRatioString renders the effective group ratio as the canonical
// decimal string the policy envelope accepts. Ratio lookups that miss keep
// the field empty rather than inventing a value.
func legacyGroupRatioString(group string) string {
	if !ratio_setting.ContainsGroupRatio(group) {
		return ""
	}
	ratio := ratio_setting.GetGroupRatio(group)
	return strconv.FormatFloat(ratio, 'f', -1, 64)
}
