package controller

import (
	"net/http"
	"strconv"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"github.com/gin-gonic/gin"
)

func GetUserUsagePolicy(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		usageReviewError(c, model.ErrUserUsagePolicyInvalid)
		return
	}
	view, err := model.ReadUserUsagePolicy(c.Request.Context(), model.DB, c.GetInt("id"), id)
	if err != nil {
		usageReviewError(c, err)
		return
	}
	common.ApiSuccess(c, view)
}
func UpdateUserUsagePolicy(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, err := strconv.Atoi(c.Param("id"))
	var request struct {
		ID               string `json:"id"`
		ExpectedRevision *int64 `json:"expected_revision"`
		NoBalance        *bool  `json:"no_balance"`
		Confirmed        bool   `json:"confirmed"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
	if err != nil || id <= 0 || c.Request.URL.RawQuery != "" || common.DecodeJsonStrict(c.Request.Body, &request) != nil || request.ExpectedRevision == nil || request.NoBalance == nil || !request.Confirmed {
		usageReviewError(c, model.ErrUserUsagePolicyInvalid)
		return
	}
	view, err := model.ConfigureUserUsagePolicy(c.Request.Context(), model.DB, c.GetInt("id"), model.UserUsagePolicyInput{ID: request.ID, UserID: id, ExpectedRevision: *request.ExpectedRevision, NoBalance: *request.NoBalance})
	if err != nil {
		usageReviewError(c, err)
		return
	}
	common.ApiSuccess(c, view)
}

// Outbound-only view. The embedded User keeps private policy fields ignored
// during generic profile/user decoding, preventing mass-assignment writes.
type userUsagePolicyReadView struct {
	*model.User
	SelfUseNoBalance    bool  `json:"self_use_no_balance"`
	UsagePolicyRevision int64 `json:"usage_policy_revision"`
}

func userUsagePolicyRows(users []*model.User) []userUsagePolicyReadView {
	rows := make([]userUsagePolicyReadView, 0, len(users))
	for _, user := range users {
		rows = append(rows, userUsagePolicyReadView{User: user, SelfUseNoBalance: user.SelfUseNoBalance, UsagePolicyRevision: user.UsagePolicyRevision})
	}
	return rows
}
