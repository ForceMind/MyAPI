package controller

import (
	"net/http"
	"strconv"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"

	"github.com/gin-gonic/gin"
)

const maxQuotaDataBuckets = int64(1500)

type quotaDataRequest struct {
	StartTimestamp          int64
	RequestedStartTimestamp int64
	EndTimestamp            int64
	Granularity             model.QuotaDataGranularity
	TimezoneOffsetMinutes   int
}

func parseFlowQuotaTimeRange(c *gin.Context) (int64, int64, bool) {
	startTimestamp, err := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	if err != nil || startTimestamp <= 0 {
		common.ApiErrorMsg(c, "invalid start_timestamp")
		return 0, 0, false
	}
	endTimestamp, err := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	if err != nil || endTimestamp <= 0 {
		common.ApiErrorMsg(c, "invalid end_timestamp")
		return 0, 0, false
	}
	if endTimestamp < startTimestamp {
		common.ApiErrorMsg(c, "invalid time range")
		return 0, 0, false
	}
	return startTimestamp, endTimestamp, true
}

func parseQuotaDataRequest(c *gin.Context) (quotaDataRequest, bool) {
	startTimestamp, endTimestamp, ok := parseFlowQuotaTimeRange(c)
	if !ok {
		return quotaDataRequest{}, false
	}

	granularityValue := c.Query("granularity")
	if granularityValue == "" {
		granularityValue = c.Query("default_time")
	}
	if granularityValue == "" {
		granularityValue = common.DataExportDefaultTime
	}
	granularity, ok := model.ParseQuotaDataGranularity(granularityValue)
	if !ok {
		common.ApiErrorMsg(c, "invalid granularity")
		return quotaDataRequest{}, false
	}

	timezoneOffsetMinutes := 0
	if value := c.Query("timezone_offset"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < -840 || parsed > 840 {
			common.ApiErrorMsg(c, "invalid timezone_offset")
			return quotaDataRequest{}, false
		}
		timezoneOffsetMinutes = parsed
	}

	startBucket := model.QuotaDataBucketStart(
		startTimestamp,
		granularity,
		timezoneOffsetMinutes,
	)
	endBucket := model.QuotaDataBucketStart(
		endTimestamp,
		granularity,
		timezoneOffsetMinutes,
	)
	bucketCount := (endBucket-startBucket)/granularity.BucketSeconds() + 1
	if bucketCount > maxQuotaDataBuckets {
		common.ApiErrorMsg(c, "time range contains too many buckets")
		return quotaDataRequest{}, false
	}
	// Source rows are stored at minute boundaries. Only normalize to that source
	// precision: expanding to the selected hour/day/week bucket would include
	// usage that predates the requested range.
	sourceStartTimestamp := startTimestamp - startTimestamp%60
	return quotaDataRequest{
		StartTimestamp:          sourceStartTimestamp,
		RequestedStartTimestamp: startTimestamp,
		EndTimestamp:            endTimestamp,
		Granularity:             granularity,
		TimezoneOffsetMinutes:   timezoneOffsetMinutes,
	}, true
}

func GetAllQuotaDates(c *gin.Context) {
	request, ok := parseQuotaDataRequest(c)
	if !ok {
		return
	}
	username := c.Query("username")
	dates, err := model.GetAllQuotaDatesWithGranularity(
		request.StartTimestamp,
		request.EndTimestamp,
		username,
		request.Granularity,
		request.TimezoneOffsetMinutes,
	)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    dates,
	})
	return
}

func GetQuotaDatesByUser(c *gin.Context) {
	request, ok := parseQuotaDataRequest(c)
	if !ok {
		return
	}
	dates, err := model.GetQuotaDataGroupByUserWithGranularity(
		request.StartTimestamp,
		request.EndTimestamp,
		request.Granularity,
		request.TimezoneOffsetMinutes,
	)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    dates,
	})
}

func GetUserQuotaDates(c *gin.Context) {
	userId := c.GetInt("id")
	request, ok := parseQuotaDataRequest(c)
	if !ok {
		return
	}
	// 判断时间跨度是否超过 1 个月
	if request.EndTimestamp-request.RequestedStartTimestamp > 2592000 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "时间跨度不能超过 1 个月",
		})
		return
	}
	dates, err := model.GetQuotaDataByUserIdWithGranularity(
		userId,
		request.StartTimestamp,
		request.EndTimestamp,
		request.Granularity,
		request.TimezoneOffsetMinutes,
	)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    dates,
	})
	return
}

func GetAllFlowQuotaDates(c *gin.Context) {
	startTimestamp, endTimestamp, ok := parseFlowQuotaTimeRange(c)
	if !ok {
		return
	}
	username := c.Query("username")
	dates, err := model.GetFlowQuotaData(startTimestamp, endTimestamp, username, 0, c.GetInt("role"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    dates,
	})
	return
}

func GetUserFlowQuotaDates(c *gin.Context) {
	userId := c.GetInt("id")
	startTimestamp, endTimestamp, ok := parseFlowQuotaTimeRange(c)
	if !ok {
		return
	}
	if endTimestamp-startTimestamp > 2592000 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "时间跨度不能超过 1 个月",
		})
		return
	}
	dates, err := model.GetFlowQuotaData(startTimestamp, endTimestamp, "", userId, common.RoleCommonUser)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    dates,
	})
	return
}
