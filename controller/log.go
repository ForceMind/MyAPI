package controller

import (
	"net/http"
	"strconv"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/model"

	"github.com/gin-gonic/gin"
)

const maxRecordedRequestSummaryRange = int64(31 * 24 * time.Hour / time.Second)
const maxRecordedRequestSummaryFutureSkew = int64(5 * time.Minute / time.Second)

type recordedRequestSummaryCoverage struct {
	Complete            bool   `json:"complete"`
	ConsumeLogsEnabled  bool   `json:"consume_logs_enabled"`
	ErrorLogsEnabled    bool   `json:"error_logs_enabled"`
	Reason              string `json:"reason"`
	IdentifiedRequests  int64  `json:"identified_requests"`
	UnidentifiedLogRows int64  `json:"unidentified_log_rows"`
	WindowSemantics     string `json:"window_semantics"`
	Deduplication       string `json:"deduplication"`
}

type recordedRequestSummaryResponse struct {
	StartTimestamp int64 `json:"start_timestamp"`
	EndTimestamp   int64 `json:"end_timestamp"`
	model.RecordedRequestSummary
	Coverage recordedRequestSummaryCoverage `json:"coverage"`
}

func parseRecordedRequestSummaryRange(c *gin.Context) (int64, int64, bool) {
	startTimestamp, err := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	if err != nil || startTimestamp <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": i18n.T(c, i18n.MsgInvalidParams)})
		return 0, 0, false
	}
	endTimestamp, err := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	if err != nil || endTimestamp <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": i18n.T(c, i18n.MsgInvalidParams)})
		return 0, 0, false
	}
	if endTimestamp <= startTimestamp || endTimestamp-startTimestamp > maxRecordedRequestSummaryRange ||
		endTimestamp > time.Now().Unix()+maxRecordedRequestSummaryFutureSkew {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": i18n.T(c, i18n.MsgInvalidParams)})
		return 0, 0, false
	}
	return startTimestamp, endTimestamp, true
}

func recordedRequestCoverage(summary model.RecordedRequestSummary) recordedRequestSummaryCoverage {
	consumeEnabled := common.LogConsumeEnabled
	errorEnabled := constant.ErrorLogEnabled
	return recordedRequestSummaryCoverage{
		Complete:            false,
		ConsumeLogsEnabled:  consumeEnabled,
		ErrorLogsEnabled:    errorEnabled,
		Reason:              "recorded_logs_only",
		IdentifiedRequests:  summary.IdentifiedRequests,
		UnidentifiedLogRows: summary.UnidentifiedLogRows,
		WindowSemantics:     "log_events_within_range",
		Deduplication:       "request_id_success_precedence",
	}
}

func getRecordedRequestSummary(c *gin.Context, userId *int) {
	startTimestamp, endTimestamp, ok := parseRecordedRequestSummaryRange(c)
	if !ok {
		return
	}
	summary, err := model.GetRecordedRequestSummary(startTimestamp, endTimestamp, userId)
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgDatabaseError)
		return
	}
	common.ApiSuccess(c, recordedRequestSummaryResponse{
		StartTimestamp:         startTimestamp,
		EndTimestamp:           endTimestamp,
		RecordedRequestSummary: summary,
		Coverage:               recordedRequestCoverage(summary),
	})
}

func GetRecordedRequestSummary(c *gin.Context) {
	getRecordedRequestSummary(c, nil)
}

func GetRecordedRequestSelfSummary(c *gin.Context) {
	userId := c.GetInt("id")
	getRecordedRequestSummary(c, &userId)
}

func GetAllLogs(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	username := c.Query("username")
	tokenName := c.Query("token_name")
	modelName := c.Query("model_name")
	channel, _ := strconv.Atoi(c.Query("channel"))
	group := c.Query("group")
	requestId := c.Query("request_id")
	upstreamRequestId := c.Query("upstream_request_id")
	logs, total, err := model.GetAllLogs(logType, startTimestamp, endTimestamp, modelName, username, tokenName, pageInfo.GetStartIdx(), pageInfo.GetPageSize(), channel, group, requestId, upstreamRequestId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(logs)
	common.ApiSuccess(c, pageInfo)
	return
}

func GetUserLogs(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	userId := c.GetInt("id")
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	tokenName := c.Query("token_name")
	modelName := c.Query("model_name")
	group := c.Query("group")
	requestId := c.Query("request_id")
	upstreamRequestId := c.Query("upstream_request_id")
	logs, total, err := model.GetUserLogs(userId, logType, startTimestamp, endTimestamp, modelName, tokenName, pageInfo.GetStartIdx(), pageInfo.GetPageSize(), group, requestId, upstreamRequestId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(logs)
	common.ApiSuccess(c, pageInfo)
	return
}

// Deprecated: SearchAllLogs 已废弃，前端未使用该接口。
func SearchAllLogs(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": false,
		"message": "该接口已废弃",
	})
}

// Deprecated: SearchUserLogs 已废弃，前端未使用该接口。
func SearchUserLogs(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": false,
		"message": "该接口已废弃",
	})
}

func GetLogByKey(c *gin.Context) {
	tokenId := c.GetInt("token_id")
	if tokenId == 0 {
		c.JSON(200, gin.H{
			"success": false,
			"message": "无效的令牌",
		})
		return
	}
	logs, err := model.GetLogByTokenId(tokenId)
	if err != nil {
		c.JSON(200, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(200, gin.H{
		"success": true,
		"message": "",
		"data":    logs,
	})
}

func GetLogsStat(c *gin.Context) {
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	tokenName := c.Query("token_name")
	username := c.Query("username")
	modelName := c.Query("model_name")
	channel, _ := strconv.Atoi(c.Query("channel"))
	group := c.Query("group")
	stat, err := model.SumUsedQuota(logType, startTimestamp, endTimestamp, modelName, username, tokenName, channel, group)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	//tokenNum := model.SumUsedToken(logType, startTimestamp, endTimestamp, modelName, username, "")
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"quota": stat.Quota,
			"rpm":   stat.Rpm,
			"tpm":   stat.Tpm,
		},
	})
	return
}

func GetLogsSelfStat(c *gin.Context) {
	username := c.GetString("username")
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	tokenName := c.Query("token_name")
	modelName := c.Query("model_name")
	channel, _ := strconv.Atoi(c.Query("channel"))
	group := c.Query("group")
	quotaNum, err := model.SumUsedQuota(logType, startTimestamp, endTimestamp, modelName, username, tokenName, channel, group)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	//tokenNum := model.SumUsedToken(logType, startTimestamp, endTimestamp, modelName, username, tokenName)
	c.JSON(200, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"quota": quotaNum.Quota,
			"rpm":   quotaNum.Rpm,
			"tpm":   quotaNum.Tpm,
			//"token": tokenNum,
		},
	})
	return
}
