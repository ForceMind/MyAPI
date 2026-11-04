package controller

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"

	"github.com/gin-gonic/gin"
)

func ListSystemInstances(c *gin.Context) {
	instances, err := model.ListSystemInstances()
	if err != nil {
		common.ApiError(c, err)
		return
	}

	now := common.GetTimestamp()
	responses := make([]model.SystemInstanceResponse, 0, len(instances))
	for _, instance := range instances {
		response := instance.ToResponse(now)
		if c.GetInt("management_key_id") > 0 {
			// Keep observed resource counters exact until the management output
			// whitelist runs; decoding to any here would first round large integers.
			var info json.RawMessage
			if common.UnmarshalJsonStr(instance.Info, &info) == nil {
				response.Info = info
			} else {
				response.Info = nil
			}
		}
		responses = append(responses, response)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    responses,
	})
}

func DeleteStaleSystemInstances(c *gin.Context) {
	deletedCount, err := model.DeleteStaleSystemInstances(common.GetTimestamp())
	if err != nil {
		common.ApiError(c, err)
		return
	}

	common.ApiSuccess(c, gin.H{
		"deleted_count": deletedCount,
	})
}

func DeleteStaleSystemInstance(c *gin.Context) {
	nodeName := c.Param("node_name")
	if strings.TrimSpace(nodeName) == "" {
		common.ApiErrorMsg(c, "node name is required")
		return
	}

	deleted, err := model.DeleteStaleSystemInstance(nodeName, common.GetTimestamp())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !deleted {
		common.ApiErrorMsg(c, "instance is not stale or no longer exists")
		return
	}

	common.ApiSuccess(c, gin.H{
		"deleted_count": 1,
	})
}
