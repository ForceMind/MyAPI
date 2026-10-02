package service

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/model"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
)

var (
	ErrTokenBudgetUnsupported      = errors.New("strict token budget requires supported official Responses text input and an explicit output limit")
	ErrTokenBudgetCountUnavailable = errors.New("reliable input token count is unavailable")
)

// CountTokenBudgetBound is called on the final outbound request, after model
// conversion and overrides. It never accepts the gateway's estimated count.
// No prompt or credential is included in the returned persistence record.
// The supported source contract is documented at:
// https://developers.openai.com/api/docs/guides/token-counting
func CountTokenBudgetBound(ctx context.Context, client *http.Client, req *http.Request, info *relaycommon.RelayInfo) (*model.TokenBudgetReservation, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if client == nil || client.Jar != nil || req == nil || req.URL == nil || req.GetBody == nil || info == nil || info.ChannelMeta == nil ||
		info.ChannelType != constant.ChannelTypeOpenAI || info.ChannelId <= 0 || info.TokenId <= 0 || info.UserId <= 0 ||
		common.TLSInsecureSkipVerify || !tokenBudgetVerifiedTransport(client.Transport) || req.Method != http.MethodPost || req.URL.Scheme != "https" || req.URL.Host != "api.openai.com" ||
		req.URL.EscapedPath() != "/v1/responses" || req.URL.RawQuery != "" || req.URL.User != nil ||
		(req.Host != "" && req.Host != "api.openai.com") || len(info.HeadersOverride) != 0 || len(info.ParamOverride) != 0 ||
		req.Header.Get("OpenAI-Beta") != "" || req.Header.Get("Content-Encoding") != "" || req.Header.Get("Cookie") != "" {
		return nil, ErrTokenBudgetUnsupported
	}
	mediaType, _, err := mime.ParseMediaType(req.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return nil, ErrTokenBudgetUnsupported
	}
	reader, err := req.GetBody()
	if err != nil {
		return nil, ErrTokenBudgetUnsupported
	}
	defer reader.Close()
	body, err := io.ReadAll(io.LimitReader(reader, (1<<20)+1))
	if err != nil {
		return nil, ErrTokenBudgetUnsupported
	}
	digest, err := common.CanonicalJSONObjectDigest(body)
	if err != nil {
		return nil, ErrTokenBudgetUnsupported
	}
	var fields map[string]json.RawMessage
	if common.Unmarshal(body, &fields) != nil {
		return nil, ErrTokenBudgetUnsupported
	}
	// Only a complete, stateless text request is eligible in this first strict
	// slice. Unsupported keys are rejected, never removed from the sent request.
	for key := range fields {
		switch key {
		case "model", "input", "instructions", "max_output_tokens", "stream", "store", "service_tier":
		default:
			return nil, ErrTokenBudgetUnsupported
		}
	}
	var request struct {
		Model        string `json:"model"`
		Input        any    `json:"input"`
		Instructions any    `json:"instructions"`
		MaxOutput    *int64 `json:"max_output_tokens"`
		Stream       *bool  `json:"stream"`
		Store        *bool  `json:"store"`
		ServiceTier  string `json:"service_tier"`
	}
	if common.Unmarshal(body, &request) != nil || request.Model == "" || len(request.Model) > 512 ||
		request.Model != info.UpstreamModelName || request.MaxOutput == nil || *request.MaxOutput <= 0 || *request.MaxOutput > relaycommon.MaxRequestTokens ||
		!tokenBudgetStatelessText(request.Input) || (request.ServiceTier != "" && request.ServiceTier != "default" && request.ServiceTier != "standard") {
		return nil, ErrTokenBudgetUnsupported
	}
	if request.Instructions != nil {
		if _, ok := request.Instructions.(string); !ok {
			return nil, ErrTokenBudgetUnsupported
		}
	}
	countPayload := map[string]json.RawMessage{"model": fields["model"], "input": fields["input"]}
	if instructions, exists := fields["instructions"]; exists {
		countPayload["instructions"] = instructions
	}
	countBody, err := common.Marshal(countPayload)
	if err != nil {
		return nil, ErrTokenBudgetUnsupported
	}
	countCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	countReq, err := http.NewRequestWithContext(countCtx, http.MethodPost, "https://api.openai.com/v1/responses/input_tokens", bytes.NewReader(countBody))
	if err != nil {
		return nil, ErrTokenBudgetCountUnavailable
	}
	// Use the same already-authorized upstream identity, not a second account
	// or a locally guessed tokenizer. Never forward credentials to a redirect.
	for _, header := range []string{"Authorization", "OpenAI-Organization", "OpenAI-Project"} {
		if value := req.Header.Get(header); value != "" {
			countReq.Header.Set(header, value)
		}
	}
	countReq.Header.Set("Content-Type", "application/json")
	countReq.Header.Set("Accept", "application/json")
	countClient := *client
	countClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := countClient.Do(countReq)
	if err != nil {
		return nil, ErrTokenBudgetCountUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, ErrTokenBudgetCountUnavailable
	}
	countBytes, err := io.ReadAll(io.LimitReader(response.Body, 16385))
	if err != nil || len(countBytes) > 16384 {
		return nil, ErrTokenBudgetCountUnavailable
	}
	if _, err := common.CanonicalJSONObjectDigest(countBytes); err != nil {
		return nil, ErrTokenBudgetCountUnavailable
	}
	var count struct {
		Object      string `json:"object"`
		InputTokens *int64 `json:"input_tokens"`
	}
	if common.Unmarshal(countBytes, &count) != nil || count.Object != "response.input_tokens" || count.InputTokens == nil || *count.InputTokens < 0 || *count.InputTokens > int64(common.MaxQuota) {
		return nil, ErrTokenBudgetCountUnavailable
	}
	// Freeze the exact validated bytes for the subsequent send. Removing GetBody
	// prevents transport-level POST replay after a possible dispatch.
	if req.Body != nil {
		_ = req.Body.Close()
	}
	req.Body, req.ContentLength, req.GetBody = io.NopCloser(bytes.NewReader(body)), int64(len(body)), nil
	return &model.TokenBudgetReservation{RequestServiceTier: request.ServiceTier, RequestID: info.RequestId, TokenID: info.TokenId, UserID: info.UserId,
		ChannelID: info.ChannelId, ModelName: request.Model, PayloadSHA256: hex.EncodeToString(digest[:]), BoundSource: model.TokenBudgetBoundOpenAIResponses,
		InputTokens: *count.InputTokens, MaxOutputTokens: *request.MaxOutput}, nil
}

func tokenBudgetStatelessText(value any) bool {
	if !publishedPriceTextContent(value) {
		return false
	}
	switch content := value.(type) {
	case []any:
		for _, part := range content {
			if !tokenBudgetStatelessText(part) {
				return false
			}
		}
	case map[string]any:
		for key := range content {
			switch key {
			case "role", "type", "content", "text":
			default:
				return false
			}
		}
		if nested, ok := content["content"]; ok {
			return tokenBudgetStatelessText(nested)
		}
	}
	return true
}

// Inspect cached transports rather than trusting only the current global TLS
// switch: a client created before a settings change can retain its old policy.
func tokenBudgetVerifiedTransport(rt http.RoundTripper) bool {
	if rt == nil {
		rt = http.DefaultTransport
	}
	switch transport := rt.(type) {
	case *http.Transport:
		if transport == nil || transport.DialTLS != nil || transport.DialTLSContext != nil {
			return false
		}
		config := transport.TLSClientConfig
		return config == nil || (!config.InsecureSkipVerify && (config.ServerName == "" || config.ServerName == "api.openai.com"))
	case *shardedRoundTripper:
		if transport == nil || len(transport.shards) == 0 {
			return false
		}
		for _, shard := range transport.shards {
			if !tokenBudgetVerifiedTransport(shard) {
				return false
			}
		}
		return true
	default:
		return false
	}
}
