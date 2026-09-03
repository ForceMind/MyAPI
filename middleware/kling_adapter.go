package middleware

import (
	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"

	"github.com/gin-gonic/gin"
)

func KlingRequestConvert() func(c *gin.Context) {
	return func(c *gin.Context) {
		var originalReq map[string]interface{}
		if err := common.UnmarshalBodyReusable(c, &originalReq); err != nil {
			c.Next()
			return
		}

		// Support both model_name and model fields
		model, _ := originalReq["model_name"].(string)
		if model == "" {
			model, _ = originalReq["model"].(string)
		}
		unifiedReq := buildTaskSubmitEnvelope(originalReq, model)

		jsonData, err := common.Marshal(unifiedReq)
		if err != nil {
			c.Next()
			return
		}

		// Rewrite request body and path
		if err := common.ReplaceRequestBody(c, jsonData); err != nil {
			c.Next()
			return
		}
		c.Request.URL.Path = "/v1/video/generations"
		if image, ok := originalReq["image"]; !ok || image == "" {
			c.Set("action", constant.TaskActionTextGenerate)
		}

		c.Next()
	}
}

var taskSubmitEnvelopeFields = []string{
	"model",
	"prompt",
	"mode",
	"image",
	"images",
	"size",
	"duration",
	"seconds",
	"input_reference",
}

// buildTaskSubmitEnvelope preserves the standard task fields at the top level
// and confines provider-only options to metadata. Explicit zero values, false,
// empty arrays, and nested objects are copied without normalization.
func buildTaskSubmitEnvelope(original map[string]interface{}, model string) map[string]interface{} {
	envelope := make(map[string]interface{}, len(taskSubmitEnvelopeFields)+1)
	for _, field := range taskSubmitEnvelopeFields {
		if value, exists := original[field]; exists {
			envelope[field] = value
		}
	}
	// The provider-specific alias selected by the compatibility adapter is the
	// authoritative routing and billing model, even when a generic model was
	// also present in the client request.
	envelope["model"] = model

	metadata := make(map[string]interface{})
	if nested, ok := original["metadata"].(map[string]interface{}); ok {
		for key, value := range nested {
			metadata[key] = value
		}
	}
	for key, value := range original {
		if key == "metadata" || isTaskSubmitEnvelopeField(key) {
			continue
		}
		metadata[key] = value
	}
	for _, field := range taskSubmitEnvelopeFields {
		delete(metadata, field)
	}
	delete(metadata, "model_name")
	delete(metadata, "req_key")
	envelope["metadata"] = metadata
	return envelope
}

func isTaskSubmitEnvelopeField(field string) bool {
	for _, known := range taskSubmitEnvelopeFields {
		if field == known {
			return true
		}
	}
	return false
}
