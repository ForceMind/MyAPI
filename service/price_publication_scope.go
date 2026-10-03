package service

import (
	"fmt"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/gin-gonic/gin"
)

// This first publication contract is a Standard text reference tariff. It
// cannot silently price multimodal, tools, or a different service tier. The
// reference remains distinct from actual API fees, particularly on subscription
// channels. Later fee budgets must establish their own upstream applicability.
func ValidatePublishedPriceRequest(info *relaycommon.RelayInfo) error {
	if info == nil || info.TieredBillingSnapshot == nil || info.TieredBillingSnapshot.OfficialPricePublicationID == "" {
		return nil
	}
	switch info.Request.(type) {
	case *dto.GeneralOpenAIRequest, *dto.OpenAIResponsesRequest, *dto.OpenAIResponsesCompactionRequest:
	default:
		return fmt.Errorf("published price requires a supported text request")
	}
	if info.BillingRequestInput == nil || len(info.BillingRequestInput.Body) == 0 {
		return fmt.Errorf("published price requires frozen request evidence")
	}
	var body map[string]any
	if err := common.Unmarshal(info.BillingRequestInput.Body, &body); err != nil || body == nil {
		return fmt.Errorf("invalid published-price request evidence")
	}
	if tier, exists := body["service_tier"]; exists && tier != "default" && tier != "standard" {
		return fmt.Errorf("published price only covers Standard service tier")
	}
	for _, key := range []string{"tools", "functions", "web_search_options", "audio", "prediction", "previous_response_id", "conversation"} {
		if value, exists := body[key]; exists && value != nil {
			if list, ok := value.([]any); ok && len(list) == 0 {
				continue
			}
			if value == "" {
				continue
			}
			return fmt.Errorf("published text price does not cover %s", key)
		}
	}
	if value, exists := body["modalities"]; exists {
		list, ok := value.([]any)
		if !ok || len(list) != 1 || list[0] != "text" {
			return fmt.Errorf("published price requires text-only modalities")
		}
	}
	foundText := false
	for _, key := range []string{"messages", "input", "prompt"} {
		if value, exists := body[key]; exists && value != nil {
			if !publishedPriceTextContent(value) {
				return fmt.Errorf("published price cannot establish text-only input")
			}
			foundText = true
		}
	}
	if foundText {
		return nil
	}
	return fmt.Errorf("published price requires explicit text input")
}

func publishedPriceUsageReviewReason(ctx *gin.Context, info *relaycommon.RelayInfo, usage *dto.Usage) string {
	if reason := textUsageReviewReason(ctx, usage); reason != "" {
		return reason
	}
	if info == nil || info.TieredBillingSnapshot == nil || info.TieredBillingSnapshot.OfficialPricePublicationID == "" {
		return ""
	}
	effective := effectiveBillingUsage(usage)
	if effective == nil {
		return "missing"
	}
	if effective.PromptTokensDetails.ImageTokens != 0 || effective.PromptTokensDetails.AudioTokens != 0 || effective.CompletionTokenDetails.ImageTokens != 0 || effective.CompletionTokenDetails.AudioTokens != 0 || effective.ClaudeCacheCreation1hTokens != 0 {
		return "price_scope_unqualified"
	}
	return ""
}

func publishedPriceTextContent(value any) bool {
	switch content := value.(type) {
	case string:
		return true
	case []any:
		for _, part := range content {
			if !publishedPriceTextContent(part) {
				return false
			}
		}
		return len(content) > 0
	case map[string]any:
		switch content["type"] {
		case "text", "input_text", "output_text":
			_, ok := content["text"].(string)
			return ok
		case nil, "message":
			role, ok := content["role"].(string)
			if !ok || (role != "system" && role != "developer" && role != "user" && role != "assistant") {
				return false
			}
			return publishedPriceTextContent(content["content"])
		}
	}
	return false
}

// Runs for each selected channel, including retries, before sending upstream.
// Overrides can change the qualified request after quote construction, so this
// bounded contract rejects them rather than claiming unverified applicability.
func ValidatePublishedPriceSelectedChannel(ctx *gin.Context, info *relaycommon.RelayInfo) error {
	if info == nil || info.TieredBillingSnapshot == nil || info.TieredBillingSnapshot.OfficialPricePublicationID == "" {
		return nil
	}
	if ctx == nil {
		return fmt.Errorf("published price requires selected-channel evidence")
	}
	var mappings map[string]string
	if raw := common.GetContextKeyString(ctx, constant.ContextKeyChannelModelMapping); raw != "" {
		if err := common.UnmarshalJsonStr(raw, &mappings); err != nil {
			return fmt.Errorf("invalid channel model mapping")
		}
		if mapped := mappings[info.OriginModelName]; mapped != "" && mapped != info.OriginModelName {
			return fmt.Errorf("published price cannot qualify a different upstream model")
		}
	}
	if len(common.GetContextKeyStringMap(ctx, constant.ContextKeyChannelParamOverride)) > 0 || len(common.GetContextKeyStringMap(ctx, constant.ContextKeyChannelHeaderOverride)) > 0 {
		return fmt.Errorf("published text price cannot qualify channel overrides")
	}
	return ValidatePublishedPriceRequest(info)
}
