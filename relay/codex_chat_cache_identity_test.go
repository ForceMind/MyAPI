package relay

import (
	"encoding/json"
	"testing"

	"github.com/ForceMind/MyAPI/constant"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCodexChatCacheIdentityStablePrefixAndNamespaces(t *testing.T) {
	request := &dto.GeneralOpenAIRequest{
		Model: "gpt-5-codex",
		Messages: []dto.Message{
			{Role: "system", Content: "synthetic system"},
			{Role: "developer", Content: "synthetic developer"},
			{Role: "user", Content: "synthetic first question"},
		},
	}
	key, session := deriveCodexChatCacheIdentity(request, 11, 21, "synthetic-account-a", "")
	require.NotEmpty(t, key)
	require.NotEmpty(t, session)
	appended := *request
	appended.Messages = append(append([]dto.Message{}, request.Messages...), dto.Message{Role: "assistant", Content: "synthetic reply"}, dto.Message{Role: "user", Content: "synthetic next question"})
	appendKey, appendSession := deriveCodexChatCacheIdentity(&appended, 11, 21, "synthetic-account-a", "")
	assert.Equal(t, key, appendKey)
	assert.Equal(t, session, appendSession)
	for _, test := range []struct {
		name    string
		userID  int
		tokenID int
		account string
	}{
		{name: "different user", userID: 12, tokenID: 21, account: "synthetic-account-a"},
		{name: "different API key", userID: 11, tokenID: 22, account: "synthetic-account-a"},
		{name: "different upstream account", userID: 11, tokenID: 21, account: "synthetic-account-b"},
	} {
		t.Run(test.name, func(t *testing.T) {
			otherKey, otherSession := deriveCodexChatCacheIdentity(request, test.userID, test.tokenID, test.account, "")
			require.NotEmpty(t, otherKey)
			assert.NotEqual(t, key, otherKey)
			assert.NotEqual(t, session, otherSession)
		})
	}
	for _, test := range []struct {
		name   string
		change func(*dto.GeneralOpenAIRequest)
	}{
		{name: "model", change: func(r *dto.GeneralOpenAIRequest) { r.Model = "gpt-6.1-sol" }},
		{name: "system", change: func(r *dto.GeneralOpenAIRequest) { r.Messages[0].Content = "changed system" }},
		{name: "developer", change: func(r *dto.GeneralOpenAIRequest) { r.Messages[1].Content = "changed developer" }},
		{name: "first user", change: func(r *dto.GeneralOpenAIRequest) { r.Messages[2].Content = "changed question" }},
		{name: "legacy functions", change: func(r *dto.GeneralOpenAIRequest) { r.Functions = json.RawMessage(`[{"name":"synthetic_function"}]`) }},
		{name: "tools", change: func(r *dto.GeneralOpenAIRequest) {
			r.Tools = []dto.ToolCallRequest{{Type: "function", Function: dto.FunctionRequest{Name: "synthetic_function"}}}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			changed := *request
			changed.Messages = append([]dto.Message{}, request.Messages...)
			test.change(&changed)
			otherKey, otherSession := deriveCodexChatCacheIdentity(&changed, 11, 21, "synthetic-account-a", "")
			assert.NotEqual(t, key, otherKey)
			assert.NotEqual(t, session, otherSession)
		})
	}
}

func TestCodexChatCacheIdentityExplicitValuesAndCanonicalContent(t *testing.T) {
	request := &dto.GeneralOpenAIRequest{PromptCacheKey: "synthetic-explicit-key"}
	key, session := deriveCodexChatCacheIdentity(request, 11, 21, "synthetic-account", "")
	assert.Equal(t, request.PromptCacheKey, key)
	require.NotEmpty(t, session)
	request.Messages = []dto.Message{{Role: "user", Content: "a changed history"}}
	nextKey, nextSession := deriveCodexChatCacheIdentity(request, 11, 21, "synthetic-account", "")
	assert.Equal(t, key, nextKey)
	assert.Equal(t, session, nextSession)
	_, otherSession := deriveCodexChatCacheIdentity(request, 11, 22, "synthetic-account", "")
	assert.NotEqual(t, session, otherSession, "generated session remains isolated even with the same explicit cache key")
	request.PromptCacheKey = ""
	key, session = deriveCodexChatCacheIdentity(request, 11, 21, "synthetic-account", "synthetic-explicit-session")
	require.NotEmpty(t, key)
	require.NotEmpty(t, session)
	request.Messages = nil
	nextKey, _ = deriveCodexChatCacheIdentity(request, 11, 21, "synthetic-account", "synthetic-explicit-session")
	assert.Equal(t, key, nextKey)

	request.Model = "gpt-5-codex"
	request.Messages = []dto.Message{{Role: "user", Content: "synthetic question"}}
	request.Functions = json.RawMessage(`[{"name":"synthetic","parameters":{"type":"object","description":"test"}}]`)
	key, session = deriveCodexChatCacheIdentity(request, 11, 21, "synthetic-account", "")
	request.Functions = json.RawMessage(`[{"parameters":{"description":"test","type":"object"},"name":"synthetic"}]`)
	nextKey, nextSession = deriveCodexChatCacheIdentity(request, 11, 21, "synthetic-account", "")
	assert.Equal(t, key, nextKey, "JSON object key order does not create a different conversation")
	assert.Equal(t, session, nextSession)
}

func TestCodexChatCacheIdentityOversizedSeedSafelySkips(t *testing.T) {
	request := &dto.GeneralOpenAIRequest{Model: "gpt-5-codex", Messages: []dto.Message{{Role: "user", Content: make([]byte, 1024*1024)}}}
	key, session := deriveCodexChatCacheIdentity(request, 11, 21, "synthetic-account", "")
	assert.Empty(t, key)
	assert.Empty(t, session)
	assert.Empty(t, request.PromptCacheKey)
}

func TestCodexChatCacheIdentitySkipsMissingStableNamespace(t *testing.T) {
	request := &dto.GeneralOpenAIRequest{Model: "gpt-5-codex", Messages: []dto.Message{{Role: "user", Content: "synthetic question"}}}
	for _, test := range []struct {
		name    string
		request *dto.GeneralOpenAIRequest
		userID  int
		tokenID int
		account string
	}{
		{name: "no request", userID: 11, tokenID: 21, account: "synthetic-account"},
		{name: "no authenticated user", request: request, tokenID: 21, account: "synthetic-account"},
		{name: "no API key ID", request: request, userID: 11, account: "synthetic-account"},
		{name: "no stable account", request: request, userID: 11, tokenID: 21, account: " "},
		{name: "no first user", request: &dto.GeneralOpenAIRequest{Model: "gpt-5-codex"}, userID: 11, tokenID: 21, account: "synthetic-account"},
	} {
		t.Run(test.name, func(t *testing.T) {
			key, session := deriveCodexChatCacheIdentity(test.request, test.userID, test.tokenID, test.account, "")
			assert.Empty(t, key)
			assert.Empty(t, session)
		})
	}
}

func TestCodexChatCacheIdentitySameAccountAcrossChannelsAndCredentialRefresh(t *testing.T) {
	t.Setenv("MYAPI_CODEX_CHAT_DEFAULT_CACHE_IDENTITY_ENABLED", "true")
	t.Setenv("MYAPI_UPSTREAM_ACCEPTANCE_ENABLED", "false")
	var baselineKey, baselineSession string
	for _, test := range []struct {
		name       string
		channelID  int
		credential string
	}{
		{name: "initial channel", channelID: 31, credential: `{"account_id":"synthetic-account","access_token":"synthetic-old-access","refresh_token":"synthetic-old-refresh"}`},
		{name: "same account another channel", channelID: 32, credential: `{"account_id":"synthetic-account","access_token":"synthetic-old-access","refresh_token":"synthetic-old-refresh"}`},
		{name: "same account refreshed credentials", channelID: 31, credential: `{"account_id":"synthetic-account","access_token":"synthetic-new-access","refresh_token":"synthetic-new-refresh","last_refresh":"2026-10-29"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			info := &relaycommon.RelayInfo{UserId: 11, TokenId: 21, RelayFormat: types.RelayFormatOpenAI, ChannelMeta: &relaycommon.ChannelMeta{
				ChannelType: constant.ChannelTypeCodex, ChannelId: test.channelID, ApiKey: test.credential,
			}}
			request := &dto.GeneralOpenAIRequest{Model: "gpt-5-codex", Messages: []dto.Message{{Role: "user", Content: "synthetic first question"}}}
			seedCodexChatCacheIdentity(nil, info, request)
			require.NotEmpty(t, request.PromptCacheKey)
			session, ok := relaycommon.GetEffectiveHeaderOverride(info)["session_id"].(string)
			require.True(t, ok)
			require.NotEmpty(t, session)
			if test.name == "initial channel" {
				baselineKey, baselineSession = request.PromptCacheKey, session
			} else {
				assert.Equal(t, baselineKey, request.PromptCacheKey)
				assert.Equal(t, baselineSession, session)
			}
			assert.NotContains(t, request.PromptCacheKey, "synthetic-account")
			assert.NotContains(t, session, "synthetic-account")
		})
	}
}

func TestCodexChatCacheIdentityEligibilityAndStrictMode(t *testing.T) {
	for _, test := range []struct {
		name       string
		flag       string
		strict     string
		changeInfo func(*relaycommon.RelayInfo)
	}{
		{name: "off by default"},
		{name: "strict mode disables all default generation", flag: "true", strict: "true"},
		{name: "channel test", flag: "true", changeInfo: func(info *relaycommon.RelayInfo) { info.IsChannelTest = true }},
		{name: "other channel", flag: "true", changeInfo: func(info *relaycommon.RelayInfo) { info.ChannelType = constant.ChannelTypeOpenAI }},
		{name: "native Responses", flag: "true", changeInfo: func(info *relaycommon.RelayInfo) { info.RelayFormat = types.RelayFormatOpenAIResponses }},
		{name: "Claude conversion", flag: "true", changeInfo: func(info *relaycommon.RelayInfo) { info.RelayFormat = types.RelayFormatClaude }},
		{name: "missing stable account", flag: "true", changeInfo: func(info *relaycommon.RelayInfo) { info.ApiKey = `{"access_token":"synthetic-access-only"}` }},
		{name: "invalid credential JSON", flag: "true", changeInfo: func(info *relaycommon.RelayInfo) { info.ApiKey = "synthetic-invalid-key" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("MYAPI_CODEX_CHAT_DEFAULT_CACHE_IDENTITY_ENABLED", test.flag)
			t.Setenv("MYAPI_UPSTREAM_ACCEPTANCE_ENABLED", test.strict)
			info := &relaycommon.RelayInfo{UserId: 11, TokenId: 21, RelayFormat: types.RelayFormatOpenAI, ChannelMeta: &relaycommon.ChannelMeta{
				ChannelType: constant.ChannelTypeCodex, ApiKey: `{"account_id":"synthetic-account","access_token":"synthetic-access"}`,
			}}
			if test.changeInfo != nil {
				test.changeInfo(info)
			}
			request := &dto.GeneralOpenAIRequest{Model: "gpt-5-codex", Messages: []dto.Message{{Role: "user", Content: "synthetic question"}}}
			seedCodexChatCacheIdentity(nil, info, request)
			assert.Empty(t, request.PromptCacheKey)
			assert.False(t, info.UseRuntimeHeadersOverride)
			assert.Empty(t, info.RuntimeHeadersOverride)
		})
	}
}
