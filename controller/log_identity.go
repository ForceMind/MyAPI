package controller

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/model"
	"github.com/gin-gonic/gin"
)

func logIdentityFilter(c *gin.Context, userID int) (model.LogIdentityFilter, bool) {
	filter := model.LogIdentityFilter{UserID: userID, RequestID: c.Query("request_id"), UpstreamRequestID: c.Query("upstream_request_id")}
	query, err := url.ParseQuery(c.Request.URL.RawQuery)
	if values, present := query["token_id"]; present && err == nil {
		if len(values) != 1 {
			err = strconv.ErrSyntax
		} else {
			var id int64
			id, err = strconv.ParseInt(values[0], 10, 32)
			if id <= 0 {
				err = strconv.ErrRange
			}
			filter.TokenID = int(id)
		}
	}
	// Every log list and stat handler passes through here, so an unknown
	// result value is rejected before it reaches the model layer.
	if values, present := query["result"]; present && err == nil {
		if len(values) != 1 || (values[0] != "" && values[0] != model.LogResultFailed) {
			err = strconv.ErrSyntax
		}
	}
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"success": false, "message": common.TranslateMessage(c, i18n.MsgInvalidParams)})
		return model.LogIdentityFilter{}, false
	}
	return filter, true
}
