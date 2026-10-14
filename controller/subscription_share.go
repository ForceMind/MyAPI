package controller

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func subscriptionShareError(c *gin.Context, err error) {
	status, code, message := service.SubscriptionShareErrorDetails(err)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		status, code, message = http.StatusNotFound, "subscription_share_not_found", i18n.MsgOperationFailed
	}
	c.JSON(status, gin.H{"success": false, "code": code, "message": common.TranslateMessage(c, message)})
}

func GetSubscriptionShares(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	tokenID, err := strconv.Atoi(c.Param("id"))
	if err != nil || tokenID <= 0 || c.Request.URL.RawQuery != "" {
		subscriptionShareError(c, model.ErrSubscriptionShareInvalid)
		return
	}
	now, err := model.ReadDatabaseUnixTime(c.Request.Context(), model.DB)
	if err != nil {
		subscriptionShareError(c, err)
		return
	}
	view, err := model.ReadSubscriptionShares(c.Request.Context(), model.DB, c.GetInt("id"), tokenID, now)
	if err != nil {
		subscriptionShareError(c, err)
		return
	}
	common.ApiSuccess(c, view)
}

func UpdateSubscriptionShare(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	tokenID, err := strconv.Atoi(c.Param("id"))
	var request struct {
		ID                   string `json:"id"`
		PoolID               *int64 `json:"pool_id"`
		SnapshotID           *int   `json:"snapshot_id"`
		ExpectedRevision     *int64 `json:"expected_revision"`
		ShareBPS             *int   `json:"share_bps"`
		ConfirmedApproximate bool   `json:"confirmed_approximate"`
		ReplaceWindow        bool   `json:"replace_window"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	if err != nil || tokenID <= 0 || c.Request.URL.RawQuery != "" || common.DecodeJsonStrict(c.Request.Body, &request) != nil || request.PoolID == nil || request.SnapshotID == nil || request.ExpectedRevision == nil || request.ShareBPS == nil || !request.ConfirmedApproximate {
		subscriptionShareError(c, model.ErrSubscriptionShareInvalid)
		return
	}
	now, err := model.ReadDatabaseUnixTime(c.Request.Context(), model.DB)
	if err != nil {
		subscriptionShareError(c, err)
		return
	}
	input := model.SubscriptionShareConfigInput{ID: request.ID, PoolID: *request.PoolID, SnapshotID: *request.SnapshotID, ExpectedRevision: *request.ExpectedRevision, ShareBPS: *request.ShareBPS, ConfirmedApproximate: request.ConfirmedApproximate, ReplaceWindow: request.ReplaceWindow}
	if err := model.ConfigureSubscriptionShare(c.Request.Context(), model.DB, c.GetInt("id"), tokenID, input, now); err != nil {
		subscriptionShareError(c, err)
		return
	}
	GetSubscriptionShares(c)
}
