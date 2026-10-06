package controller

import (
	"errors"
	"io"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/service"
	"github.com/ForceMind/MyAPI/service/accesspolicy"
	"github.com/ForceMind/MyAPI/setting"
	"github.com/ForceMind/MyAPI/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type assignedAccessPolicyRequest struct {
	ExpectedRevision *int64   `json:"expected_revision"`
	Enabled          *bool    `json:"enabled"`
	PublicModels     []string `json:"public_models"`
	UpstreamModels   []string `json:"upstream_models"`
	ChannelIDs       []int    `json:"channel_ids"`
}

type assignedAccessAdminView struct {
	service.AssignedAccessPolicyView
	OwnerUserID int `json:"owner_user_id"`
}

func decodeAssignedAccessPolicyRequest(c *gin.Context, value any) error {
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return model.ErrAssignedAccessPolicyInvalid
	}
	return service.DecodeAssignedAccessPolicyJSON(raw, value)
}

func assignedAccessPolicyError(c *gin.Context, err error) {
	status, code, message := http.StatusInternalServerError, "access_policy_unavailable", i18n.MsgDatabaseError
	switch {
	case errors.Is(err, model.ErrAssignedAccessPolicyInvalid):
		status, code, message = http.StatusBadRequest, "access_policy_invalid", i18n.MsgInvalidParams
	case errors.Is(err, model.ErrAssignedAccessPolicyConflict):
		status, code, message = http.StatusConflict, "access_policy_conflict", i18n.MsgInvalidParams
	case errors.Is(err, gorm.ErrRecordNotFound):
		status, code, message = http.StatusNotFound, "access_policy_not_found", i18n.MsgInvalidParams
	case errors.Is(err, service.ErrAssignedAccessDenied), errors.Is(err, model.ErrAssignedAccessPolicyForbidden):
		status, code, message = http.StatusForbidden, "access_policy_forbidden", i18n.MsgAuthInsufficientPrivilege
	}
	c.JSON(status, gin.H{"success": false, "code": code, "message": common.TranslateMessage(c, message)})
}

func assignedAccessTarget(c *gin.Context) (string, int, *model.User, *model.Token, error) {
	subject := c.Param("subject")
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 || subject != "user" && subject != "token" {
		return "", 0, nil, nil, model.ErrAssignedAccessPolicyInvalid
	}
	var token *model.Token
	userID := id
	if subject == "token" {
		token, err = model.GetTokenById(id)
		if err != nil {
			return "", 0, nil, nil, err
		}
		userID = token.UserId
	}
	user, err := model.GetUserById(userID, false)
	if err != nil {
		return "", 0, nil, nil, err
	}
	if c.GetInt("role") < common.RoleAdminUser || !canManageTargetRole(c.GetInt("role"), user.Role) {
		return "", 0, nil, nil, service.ErrAssignedAccessDenied
	}
	return subject, id, user, token, nil
}

func GetAssignedAccessPolicy(c *gin.Context) {
	subject, id, user, _, err := assignedAccessTarget(c)
	if err != nil {
		assignedAccessPolicyError(c, err)
		return
	}
	view, err := service.ReadAssignedAccessPolicy(c.Request.Context(), subject, id)
	if err != nil {
		assignedAccessPolicyError(c, err)
		return
	}
	common.ApiSuccess(c, assignedAccessAdminView{AssignedAccessPolicyView: view, OwnerUserID: user.Id})
}

func UpdateAssignedAccessPolicy(c *gin.Context) {
	subject, id, user, _, err := assignedAccessTarget(c)
	if err != nil {
		assignedAccessPolicyError(c, err)
		return
	}
	var request assignedAccessPolicyRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 65535)
	if c.Request.URL.RawQuery != "" || decodeAssignedAccessPolicyRequest(c, &request) != nil || request.Enabled == nil || request.ExpectedRevision == nil || *request.ExpectedRevision < 0 {
		assignedAccessPolicyError(c, model.ErrAssignedAccessPolicyInvalid)
		return
	}
	policy, err := accesspolicy.NormalizeAssignedPolicy(accesspolicy.AssignedPolicy{Enabled: *request.Enabled, PublicModels: request.PublicModels, UpstreamModels: request.UpstreamModels, ChannelIDs: request.ChannelIDs})
	if err != nil {
		assignedAccessPolicyError(c, model.ErrAssignedAccessPolicyInvalid)
		return
	}
	data, err := common.Marshal(policy)
	if err != nil {
		assignedAccessPolicyError(c, err)
		return
	}
	policyJSON := string(data)
	row, err := model.MutateAssignedAccessPolicyAsAdmin(c.Request.Context(), model.DB, c.GetInt("id"), subject, id, *request.ExpectedRevision, &policyJSON)
	if err != nil {
		assignedAccessPolicyError(c, err)
		return
	}
	recordManageAuditFor(c, user.Id, "access_policy.update", map[string]interface{}{"subject": subject, "subject_id": id, "revision": row.Revision})
	common.ApiSuccess(c, assignedAccessAdminView{AssignedAccessPolicyView: service.AssignedAccessPolicyView{Subject: subject, SubjectID: id, Revision: row.Revision, Assigned: true, AssignedPolicy: policy}, OwnerUserID: user.Id})
}

func DeleteAssignedAccessPolicy(c *gin.Context) {
	subject, id, user, _, err := assignedAccessTarget(c)
	if err != nil {
		assignedAccessPolicyError(c, err)
		return
	}
	var request struct {
		ExpectedRevision *int64 `json:"expected_revision"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 256)
	if c.Request.URL.RawQuery != "" || decodeAssignedAccessPolicyRequest(c, &request) != nil || request.ExpectedRevision == nil || *request.ExpectedRevision < 0 {
		assignedAccessPolicyError(c, model.ErrAssignedAccessPolicyInvalid)
		return
	}
	row, err := model.MutateAssignedAccessPolicyAsAdmin(c.Request.Context(), model.DB, c.GetInt("id"), subject, id, *request.ExpectedRevision, nil)
	if err != nil {
		assignedAccessPolicyError(c, err)
		return
	}
	recordManageAuditFor(c, user.Id, "access_policy.remove", map[string]interface{}{"subject": subject, "subject_id": id, "revision": row.Revision})
	common.ApiSuccess(c, assignedAccessAdminView{AssignedAccessPolicyView: service.AssignedAccessPolicyView{Subject: subject, SubjectID: id, Revision: row.Revision, Assigned: false, AssignedPolicy: accesspolicy.AssignedPolicy{Enabled: true}}, OwnerUserID: user.Id})
}

func PreviewAssignedAccessPolicy(c *gin.Context) {
	subject, id, user, token, err := assignedAccessTarget(c)
	if err != nil {
		assignedAccessPolicyError(c, err)
		return
	}
	var request assignedAccessPolicyRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 65535)
	if c.Request.URL.RawQuery != "" || decodeAssignedAccessPolicyRequest(c, &request) != nil || request.Enabled == nil {
		assignedAccessPolicyError(c, model.ErrAssignedAccessPolicyInvalid)
		return
	}
	policy, err := accesspolicy.NormalizeAssignedPolicy(accesspolicy.AssignedPolicy{Enabled: *request.Enabled, PublicModels: request.PublicModels, UpstreamModels: request.UpstreamModels, ChannelIDs: request.ChannelIDs})
	if err != nil {
		assignedAccessPolicyError(c, model.ErrAssignedAccessPolicyInvalid)
		return
	}
	current, err := service.ReadAssignedAccessPolicy(c.Request.Context(), subject, id)
	if err != nil {
		assignedAccessPolicyError(c, err)
		return
	}
	current.Assigned = true
	current.AssignedPolicy = policy
	groups, err := service.AssignedAccessGroups(user, token)
	if err != nil {
		assignedAccessPolicyError(c, err)
		return
	}
	models, _, err := service.AssignedModelAvailability(c.Request.Context(), user, token, groups, service.GetGroupsEnabledModels(groups), &current)
	if err != nil {
		assignedAccessPolicyError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"policy": assignedAccessAdminView{AssignedAccessPolicyView: current, OwnerUserID: user.Id}, "models": models})
}

func GetTokenAssignedAccess(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		assignedAccessPolicyError(c, model.ErrAssignedAccessPolicyInvalid)
		return
	}
	token, err := model.GetTokenByIds(id, c.GetInt("id"))
	if err != nil {
		assignedAccessPolicyError(c, err)
		return
	}
	user, err := model.GetUserById(token.UserId, false)
	if err != nil {
		assignedAccessPolicyError(c, err)
		return
	}
	groups, err := service.AssignedAccessGroups(user, token)
	if err != nil {
		assignedAccessPolicyError(c, err)
		return
	}
	models, state, err := service.AssignedModelAvailability(c.Request.Context(), user, token, groups, service.GetGroupsEnabledModels(groups), nil)
	if err != nil {
		assignedAccessPolicyError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"token_id": token.Id, "assigned": state.Assigned(), "user_revision": state.User.Revision, "token_revision": state.Token.Revision, "models": models})
}

// filterModelsByAssignedAccess leaves existing discovery unchanged while no
// assignment or enforced legacy policy applies. Once constrained, discovery
// evaluates real targets and never advertises a policy-denied alias.
func filterModelsByAssignedAccess(c *gin.Context, groups, names []string) ([]string, []service.AssignedModelAccess, error) {
	return projectModelsByAssignedAccess(c, groups, names, false)
}

// Metadata lookup retains the static catalog contract for unassigned,
// non-enforced requests while still checking current credentials.
func projectModelsByAssignedAccess(c *gin.Context, groups, names []string, preserveUnassignedMetadata bool) ([]string, []service.AssignedModelAccess, error) {
	if c.GetInt("id") <= 0 {
		return names, nil, nil
	}
	user, err := model.GetUserById(c.GetInt("id"), false)
	if err != nil {
		return nil, nil, err
	}
	if user.Status != common.UserStatusEnabled {
		return nil, nil, service.ErrAssignedAccessDenied
	}
	var token *model.Token
	if id := c.GetInt("token_id"); id > 0 {
		token, err = model.GetTokenByIds(id, user.Id)
		if err != nil {
			return nil, nil, err
		}
		if c.GetString("token_key") != token.Key {
			return nil, nil, service.ErrAssignedAccessDenied
		}
		if token.Status != common.TokenStatusEnabled || token.ExpiredTime != -1 && token.ExpiredTime <= time.Now().Unix() {
			return nil, nil, service.ErrAssignedAccessDenied
		}
	}
	currentGroups, err := service.AssignedAccessGroups(user, token)
	if err != nil {
		return nil, nil, err
	}
	authorizedGroups := make([]string, 0, len(groups))
	for _, group := range groups {
		if slices.Contains(currentGroups, group) {
			authorizedGroups = append(authorizedGroups, group)
		}
	}
	groups = authorizedGroups
	tokenID := 0
	if token != nil {
		tokenID = token.Id
	}
	state, err := service.LoadAssignedAccessState(c.Request.Context(), user.Id, tokenID)
	if err != nil {
		return nil, nil, err
	}
	enforce := false
	// Current authorization determines whether legacy enforcement applies.
	// A revoked cached group must not make an empty intersection look exempt.
	for _, group := range currentGroups {
		if setting.AccessPolicyEnforcedForGroup(group) {
			enforce = true
			break
		}
	}
	if !state.Assigned() && !enforce && preserveUnassignedMetadata {
		return names, nil, nil
	}
	currentlyEnabled := service.GetGroupsEnabledModels(groups)
	currentNames := make([]string, 0, len(names))
	for _, name := range names {
		if !slices.Contains(currentlyEnabled, name) {
			continue
		}
		if token != nil && token.ModelLimitsEnabled && !token.GetModelLimitsMap()[ratio_setting.FormatMatchingModelName(name)] {
			continue
		}
		currentNames = append(currentNames, name)
	}
	names = currentNames
	if !state.Assigned() && !enforce {
		return names, nil, nil
	}
	decisions, _, err := service.AssignedModelAvailability(c.Request.Context(), user, token, groups, names, nil)
	if err != nil {
		return nil, nil, err
	}
	allowed := make([]string, 0, len(decisions))
	for _, decision := range decisions {
		if decision.Allowed {
			allowed = append(allowed, decision.Model)
		}
	}
	return allowed, decisions, nil
}
