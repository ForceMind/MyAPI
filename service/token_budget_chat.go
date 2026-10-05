package service

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relaykit/dto"
)

// The exact model's documented context contains input plus all completion
// tokens (including reasoning). No locally estimated framing count is used.
// https://developers.openai.com/api/docs/models/gpt-6.1-sol
// https://developers.openai.com/api/docs/guides/conversation-state
func boundOpenAIChatText(req *http.Request, info *relaycommon.RelayInfo, body []byte, fields map[string]json.RawMessage, digest [32]byte) (*model.TokenBudgetReservation, error) {
	for key, value := range fields {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return nil, ErrTokenBudgetUnsupported
		}
		switch key {
		case "model", "messages", "max_completion_tokens", "n", "stream", "stream_options", "store", "service_tier":
		default:
			return nil, ErrTokenBudgetUnsupported
		}
	}
	var request struct {
		Model         string                       `json:"model"`
		Messages      []map[string]json.RawMessage `json:"messages"`
		MaxOutput     *int64                       `json:"max_completion_tokens"`
		N             *int                         `json:"n"`
		Stream        *bool                        `json:"stream"`
		StreamOptions map[string]json.RawMessage   `json:"stream_options"`
		Store         *bool                        `json:"store"`
		ServiceTier   string                       `json:"service_tier"`
	}
	if common.Unmarshal(body, &request) != nil || request.Model != model.TokenBudgetOpenAIChatModel || request.Model != info.UpstreamModelName ||
		request.MaxOutput == nil || *request.MaxOutput <= 0 || *request.MaxOutput > model.TokenBudgetOpenAIChatMaxOutput ||
		request.N != nil && *request.N != 1 || len(request.Messages) == 0 ||
		request.ServiceTier != "" && request.ServiceTier != "default" {
		return nil, ErrTokenBudgetUnsupported
	}
	for _, message := range request.Messages {
		if len(message) != 2 {
			return nil, ErrTokenBudgetUnsupported
		}
		var role, content string
		if common.Unmarshal(message["role"], &role) != nil || common.Unmarshal(message["content"], &content) != nil ||
			bytes.Equal(bytes.TrimSpace(message["content"]), []byte("null")) ||
			(role != "system" && role != "developer" && role != "user" && role != "assistant") {
			return nil, ErrTokenBudgetUnsupported
		}
	}
	stream := request.Stream != nil && *request.Stream
	if stream {
		var include bool
		if len(request.StreamOptions) != 1 || common.Unmarshal(request.StreamOptions["include_usage"], &include) != nil || !include {
			return nil, ErrTokenBudgetUnsupported
		}
	} else if len(request.StreamOptions) != 0 {
		return nil, ErrTokenBudgetUnsupported
	}
	if req.Body != nil {
		_ = req.Body.Close()
	}
	req.Body, req.ContentLength, req.GetBody = io.NopCloser(bytes.NewReader(body)), int64(len(body)), nil
	return &model.TokenBudgetReservation{RequestServiceTier: request.ServiceTier, RequestID: info.RequestId, TokenID: info.TokenId, UserID: info.UserId,
		ChannelID: info.ChannelId, ModelName: request.Model, PayloadSHA256: hex.EncodeToString(digest[:]), BoundSource: model.TokenBudgetBoundOpenAIChat,
		InputTokens: model.TokenBudgetOpenAIChatContext, MaxOutputTokens: *request.MaxOutput}, nil
}

// Require exact native bytes to agree with the compatibility DTO before any
// strict reservation is released. Fee-only category presence is checked later.
func qualifiedChatBudgetUsage(row *model.TokenBudgetReservation, usage *dto.Usage) (*dto.ChatTextEvidence, error) {
	if row == nil || usage == nil || usage.BillingUsage == nil {
		return nil, model.ErrTokenBudgetPending
	}
	billing := usage.BillingUsage
	evidence, raw := billing.ChatTextEvidence, billing.OpenAIUsage
	if billing.Source != dto.BillingUsageSourceOAIChat || billing.Semantic != dto.BillingUsageSemanticOpenAI || billing.Estimated || billing.Incomplete || evidence == nil || raw == nil ||
		evidence.Model != row.ModelName || evidence.Model != model.TokenBudgetOpenAIChatModel ||
		evidence.PromptTokens != raw.PromptTokens || evidence.CompletionTokens != raw.CompletionTokens || evidence.TotalTokens != raw.TotalTokens ||
		evidence.PromptTokens < 0 || evidence.CompletionTokens < 0 || int64(evidence.PromptTokens)+int64(evidence.CompletionTokens) != int64(evidence.TotalTokens) ||
		raw.InputTokens != 0 || raw.OutputTokens != 0 || raw.InputTokensDetails != nil || raw.OutputTokensDetails != nil || raw.PromptCacheHitTokens != 0 ||
		raw.ClaudeCacheCreation5mTokens != 0 || raw.ClaudeCacheCreation1hTokens != 0 {
		return nil, model.ErrTokenBudgetPending
	}
	if evidence.ServiceTier != "" && evidence.ServiceTier != "default" || row.RequestServiceTier == "default" && evidence.ServiceTier != "default" {
		return nil, model.ErrTokenBudgetPending
	}
	details := raw.PromptTokensDetails
	if details.CachedCreationTokens != 0 || details.TextTokens != 0 || details.ImageTokens != 0 || details.AudioTokens != 0 || details.CachedTokensDetails != nil ||
		raw.CompletionTokenDetails.TextTokens != 0 || raw.CompletionTokenDetails.ImageTokens != 0 || raw.CompletionTokenDetails.AudioTokens != 0 {
		return nil, model.ErrTokenBudgetPending
	}
	read, write, reasoning := 0, 0, 0
	if evidence.CacheRead != nil {
		read = *evidence.CacheRead
	}
	if evidence.CacheWrite != nil {
		write = *evidence.CacheWrite
	}
	if evidence.Reasoning != nil {
		reasoning = *evidence.Reasoning
	}
	if read < 0 || write < 0 || reasoning < 0 || int64(read)+int64(write) > int64(evidence.PromptTokens) || reasoning > evidence.CompletionTokens ||
		read != details.CachedTokens || write != details.CacheWriteTokens || reasoning != raw.CompletionTokenDetails.ReasoningTokens {
		return nil, model.ErrTokenBudgetPending
	}
	return evidence, nil
}
