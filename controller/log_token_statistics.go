package controller

import (
	"context"
	"strconv"
	"time"

	"github.com/ForceMind/MyAPI/model"
	"github.com/gin-gonic/gin"
)

func logTokenStatistics(c *gin.Context, userID int, tokenID int) (model.LogTokenStatistics, error) {
	logType, _ := strconv.Atoi(c.Query("type"))
	start, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	end, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	channel, _ := strconv.Atoi(c.Query("channel"))
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	return model.GetLogTokenStatistics(ctx, model.LOG_DB, model.LogTokenStatisticsFilter{
		UserID: userID, Type: logType, Start: start, End: end, TokenID: tokenID,
		Model: c.Query("model_name"), Username: c.Query("username"), Token: c.Query("token_name"),
		Channel: channel, Group: c.Query("group"), RequestID: c.Query("request_id"), UpstreamRequestID: c.Query("upstream_request_id"),
	})
}
