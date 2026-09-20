package controller

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/middleware"
	"github.com/ForceMind/MyAPI/model"
	"github.com/gin-gonic/gin"
)

type promptLearningPolicyRequest struct {
	Enabled *bool `json:"enabled"`
}

type promptLearningVersionRequest struct {
	CommandID string `json:"command_id"`
	ParentID  *int64 `json:"parent_id"`
	Content   string `json:"content"`
}

// promptLearningVersionView exposes a user's own instruction history without
// leaking server scope receipts, command idempotency keys or internal user IDs.
type promptLearningVersionView struct {
	ID          int64  `json:"id"`
	ParentID    *int64 `json:"parent_id,omitempty"`
	Content     string `json:"content"`
	ContentHash string `json:"content_hash"`
	Source      string `json:"source"`
	CreatedAt   int64  `json:"created_at"`
}

// promptLearningRunView intentionally omits scope, command, lease owner and
// raw model input. It exposes the lifecycle necessary for a user to know if a
// review is pending, cancelled, failed, or requires unknown-outcome handling.
type promptLearningRunView struct {
	ID              int64  `json:"id"`
	State           string `json:"state"`
	SampleCount     int    `json:"sample_count"`
	ModelRef        string `json:"model_ref"`
	TemplateVersion string `json:"template_version"`
	CreatedAt       int64  `json:"created_at"`
	UpdatedAt       int64  `json:"updated_at"`
}

func promptLearningSelfScope(userID int) string {
	return "user:" + strconv.Itoa(userID)
}

func promptLearningVersionViewFromModel(version model.PromptInstructionVersion) promptLearningVersionView {
	return promptLearningVersionView{
		ID: version.ID, ParentID: version.ParentID, Content: version.Content, ContentHash: version.ContentHash,
		Source: version.Source, CreatedAt: version.CreatedAt,
	}
}

func promptLearningRunViewFromModel(run model.PromptLearningRun) promptLearningRunView {
	return promptLearningRunView{
		ID: run.ID, State: run.State, SampleCount: run.SampleCount, ModelRef: run.ModelRef,
		TemplateVersion: run.TemplateVersion, CreatedAt: run.CreatedAt, UpdatedAt: run.UpdatedAt,
	}
}

// Prompt-learning authorization changes are deliberately dashboard-session
// only. API keys may relay on behalf of a user, but cannot silently enable
// collection or create instructions that later become eligible for a host
// application.
func requirePromptLearningSession(c *gin.Context) bool {
	if _, ok := middleware.GetSessionAuthIdentity(c); ok {
		return true
	}
	common.ApiErrorI18n(c, i18n.MsgUnauthorized)
	return false
}

func GetPromptLearningPolicy(c *gin.Context) {
	if !requirePromptLearningSession(c) {
		return
	}
	userID := c.GetInt("id")
	scopeRef := promptLearningSelfScope(userID)
	policy, err := model.GetPromptLearningPolicy(c.Request.Context(), userID, scopeRef)
	if errors.Is(err, model.ErrPromptLearningPolicyNotFound) {
		common.ApiSuccess(c, gin.H{"scope": "self", "enabled": false, "generation": 0})
		return
	}
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgDatabaseError)
		return
	}
	common.ApiSuccess(c, gin.H{"scope": "self", "enabled": policy.Enabled, "generation": policy.Generation})
}

func UpdatePromptLearningPolicy(c *gin.Context) {
	if !requirePromptLearningSession(c) {
		return
	}
	var request promptLearningPolicyRequest
	if err := common.DecodeJsonStrict(c.Request.Body, &request); err != nil || request.Enabled == nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	userID := c.GetInt("id")
	policy, err := model.UpsertPromptLearningPolicy(c.Request.Context(), model.PromptLearningPolicyInput{
		UserID: userID, ScopeRef: promptLearningSelfScope(userID), Enabled: *request.Enabled,
	})
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgDatabaseError)
		return
	}
	common.ApiSuccess(c, gin.H{"scope": "self", "enabled": policy.Enabled, "generation": policy.Generation})
}

func ListPromptLearningVersions(c *gin.Context) {
	if !requirePromptLearningSession(c) {
		return
	}
	userID := c.GetInt("id")
	pageInfo := common.GetPageQuery(c)
	items, total, err := model.ListPromptInstructionVersions(c.Request.Context(), userID, promptLearningSelfScope(userID), pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgDatabaseError)
		return
	}
	pageInfo.SetTotal(int(total))
	views := make([]promptLearningVersionView, 0, len(items))
	for _, item := range items {
		views = append(views, promptLearningVersionViewFromModel(item))
	}
	pageInfo.SetItems(views)
	common.ApiSuccess(c, pageInfo)
}

func ListPromptLearningRuns(c *gin.Context) {
	if !requirePromptLearningSession(c) {
		return
	}
	userID := c.GetInt("id")
	pageInfo := common.GetPageQuery(c)
	items, total, err := model.ListPromptLearningRuns(c.Request.Context(), userID, promptLearningSelfScope(userID), pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgDatabaseError)
		return
	}
	pageInfo.SetTotal(int(total))
	views := make([]promptLearningRunView, 0, len(items))
	for _, item := range items {
		views = append(views, promptLearningRunViewFromModel(item))
	}
	pageInfo.SetItems(views)
	common.ApiSuccess(c, pageInfo)
}

func CancelPromptLearningRun(c *gin.Context) {
	if !requirePromptLearningSession(c) {
		return
	}
	runID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || runID <= 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	cancelled, err := model.CancelPromptLearningRun(c.Request.Context(), c.GetInt("id"), promptLearningSelfScope(c.GetInt("id")), runID)
	if err != nil {
		if errors.Is(err, model.ErrPromptInstructionNotFound) {
			common.ApiErrorI18n(c, i18n.MsgInvalidParams)
			return
		}
		common.ApiErrorI18n(c, i18n.MsgDatabaseError)
		return
	}
	if !cancelled {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": i18n.T(c, i18n.MsgInvalidParams)})
		return
	}
	common.ApiSuccess(c, gin.H{"cancelled": true})
}

func CreatePromptLearningVersion(c *gin.Context) {
	if !requirePromptLearningSession(c) {
		return
	}
	var request promptLearningVersionRequest
	if err := common.DecodeJsonStrict(c.Request.Body, &request); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	userID := c.GetInt("id")
	version, created, err := model.CreatePromptInstructionVersion(c.Request.Context(), model.PromptInstructionVersionInput{
		UserID: userID, ScopeRef: promptLearningSelfScope(userID), CommandID: request.CommandID, ParentID: request.ParentID,
		Content: request.Content, Source: "manual", CreatedBy: userID,
	})
	if err != nil {
		switch {
		case errors.Is(err, model.ErrPromptInstructionConflict):
			c.JSON(http.StatusConflict, gin.H{"success": false, "message": i18n.T(c, i18n.MsgInvalidParams)})
		case errors.Is(err, model.ErrPromptInstructionNotFound), errors.Is(err, model.ErrPromptLearningInvalidInput):
			common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		default:
			common.ApiErrorI18n(c, i18n.MsgDatabaseError)
		}
		return
	}
	common.ApiSuccess(c, gin.H{"version": promptLearningVersionViewFromModel(*version), "created": created})
}
