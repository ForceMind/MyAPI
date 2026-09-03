package middleware

import (
	"net/http"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	relayconstant "github.com/ForceMind/MyAPI/relay/constant"
	"github.com/gin-gonic/gin"
)

func JimengRequestConvert() func(c *gin.Context) {
	return func(c *gin.Context) {
		action := c.Query("Action")
		if action == "" {
			abortWithOpenAiMessage(c, http.StatusBadRequest, "Action query parameter is required")
			return
		}

		// Handle Jimeng official API request
		var originalReq map[string]interface{}
		if err := common.UnmarshalBodyReusable(c, &originalReq); err != nil {
			abortWithOpenAiMessage(c, http.StatusBadRequest, "Invalid request body")
			return
		}
		model, _ := originalReq["req_key"].(string)
		unifiedReq := buildTaskSubmitEnvelope(originalReq, model)
		metadata, _ := unifiedReq["metadata"].(map[string]interface{})
		delete(metadata, "frames")
		if frames, exists := originalReq["frames"]; exists {
			framesNumber, ok := frames.(float64)
			if !ok || (framesNumber != 121 && framesNumber != 241) {
				abortWithOpenAiMessage(c, http.StatusBadRequest, "Invalid request body")
				return
			}
			if framesNumber == 241 {
				unifiedReq["duration"] = 10
			} else {
				unifiedReq["duration"] = 5
			}
		}

		jsonData, err := common.Marshal(unifiedReq)
		if err != nil {
			abortWithOpenAiMessage(c, http.StatusInternalServerError, "Failed to marshal request body")
			return
		}

		// Update request body
		if err := common.ReplaceRequestBody(c, jsonData); err != nil {
			abortWithOpenAiMessage(c, http.StatusInternalServerError, "Failed to replace request body")
			return
		}

		if image, ok := originalReq["image"]; !ok || image == "" {
			c.Set("action", constant.TaskActionTextGenerate)
		}

		c.Request.URL.Path = "/v1/video/generations"

		if action == "CVSync2AsyncGetResult" {
			taskId, ok := originalReq["task_id"].(string)
			if !ok || taskId == "" {
				abortWithOpenAiMessage(c, http.StatusBadRequest, "task_id is required for CVSync2AsyncGetResult")
				return
			}
			c.Request.URL.Path = "/v1/video/generations/" + taskId
			c.Request.Method = http.MethodGet
			c.Set("task_id", taskId)
			c.Set("relay_mode", relayconstant.RelayModeVideoFetchByID)
		}
		c.Next()
	}
}
