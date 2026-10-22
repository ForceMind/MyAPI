package controller

import (
	"errors"
	"net/http"
	"net/url"
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

func GetSubscriptionSharePoolUsage(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	poolID, err := strconv.ParseInt(c.Param("pool_id"), 10, 64)
	if err != nil || poolID <= 0 || c.Request.URL.RawQuery != "" {
		subscriptionShareError(c, model.ErrSubscriptionShareInvalid)
		return
	}
	now, err := model.ReadDatabaseUnixTime(c.Request.Context(), model.DB)
	if err != nil {
		subscriptionShareError(c, err)
		return
	}
	view, err := model.ReadSubscriptionSharePoolUsage(c.Request.Context(), model.DB, c.GetInt("id"), poolID, now)
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

func CloseSubscriptionSharePool(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	poolID, err := strconv.ParseInt(c.Param("pool_id"), 10, 64)
	var request struct {
		ID               string `json:"id"`
		ExpectedRevision *int64 `json:"expected_revision"`
		ConfirmedClose   bool   `json:"confirmed_close"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	if err != nil || poolID <= 0 || c.Request.URL.RawQuery != "" || common.DecodeJsonStrict(c.Request.Body, &request) != nil || request.ExpectedRevision == nil || !request.ConfirmedClose {
		subscriptionShareError(c, model.ErrSubscriptionShareInvalid)
		return
	}
	now, err := model.ReadDatabaseUnixTime(c.Request.Context(), model.DB)
	if err != nil {
		subscriptionShareError(c, err)
		return
	}
	input := model.SubscriptionShareCloseInput{ID: request.ID, PoolID: poolID, ExpectedRevision: *request.ExpectedRevision, ConfirmedClose: request.ConfirmedClose}
	if err := model.CloseSubscriptionSharePool(c.Request.Context(), model.DB, c.GetInt("id"), input, now); err != nil {
		subscriptionShareError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func GetSubscriptionShareConnectionPools(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	channelID, err := strconv.Atoi(c.Param("channel_id"))
	page, pageErr := strconv.Atoi(c.DefaultQuery("page", "1"))
	query, queryErr := url.ParseQuery(c.Request.URL.RawQuery)
	if queryErr != nil || err != nil || pageErr != nil || channelID <= 0 || page < 1 || page > 1000000 || len(query) > 1 || len(query["page"]) > 1 || len(query) == 1 && query.Get("page") == "" {
		subscriptionShareError(c, model.ErrSubscriptionShareInvalid)
		return
	}
	view, err := model.ReadSubscriptionShareConnectionPools(c.Request.Context(), model.DB, c.GetInt("id"), channelID, page)
	if err != nil {
		subscriptionShareError(c, err)
		return
	}
	common.ApiSuccess(c, view)
}
