package controller

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCodexAuthorizationInput(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantCode  string
		wantState string
	}{
		{
			name:      "callback URL",
			input:     "http://localhost:1455/auth/callback?code=code-1&state=state-1",
			wantCode:  "code-1",
			wantState: "state-1",
		},
		{
			name:      "query string",
			input:     "code=code-2&state=state-2",
			wantCode:  "code-2",
			wantState: "state-2",
		},
		{
			name:      "legacy compact value",
			input:     "code-3#state-3",
			wantCode:  "code-3",
			wantState: "state-3",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			code, state, err := parseCodexAuthorizationInput(test.input)
			require.NoError(t, err)
			assert.Equal(t, test.wantCode, code)
			assert.Equal(t, test.wantState, state)
		})
	}
}

func TestParseCodexAuthorizationInputRejectsEmpty(t *testing.T) {
	_, _, err := parseCodexAuthorizationInput("   ")
	require.Error(t, err)
}

func TestStartCodexOAuthBindsStateToDashboardSession(t *testing.T) {
	setupAuthFlowControllerTest(t)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/channel/codex/oauth/start", nil)
	c.Set("id", 42)
	c.Set("session_id", "session-42")
	c.Set("auth_version", int64(1))
	c.Set("session_version", int64(1))

	StartCodexOAuth(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			AuthorizeURL string `json:"authorize_url"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	parsed, err := url.Parse(response.Data.AuthorizeURL)
	require.NoError(t, err)
	state := parsed.Query().Get("state")
	require.NotEmpty(t, state)

	flow, err := model.GetAuthFlow(state, model.AuthFlowMatch{
		Purpose:   model.AuthFlowPurposeCodexOAuth,
		Provider:  "openai",
		UserId:    42,
		SessionId: "session-42",
	})
	require.NoError(t, err)
	var payload codexOAuthFlowPayload
	require.NoError(t, common.Unmarshal([]byte(flow.Payload), &payload))
	assert.NotEmpty(t, payload.Verifier)
	assert.Zero(t, payload.ChannelID)
}

func TestStartCodexOAuthRequiresDashboardSession(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/channel/codex/oauth/start", nil)

	StartCodexOAuth(c)

	assert.Equal(t, http.StatusUnauthorized, recorder.Code)
}

func TestCompleteCodexOAuthNewChannelSavesCredentialWithoutReturningIt(t *testing.T) {
	setupAuthFlowControllerTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.User{}, &model.Log{}))
	previousLogDB, previousMemoryCache, previousRedis := model.LOG_DB, common.MemoryCacheEnabled, common.RedisEnabled
	model.LOG_DB = model.DB
	common.MemoryCacheEnabled = false
	common.RedisEnabled = false
	t.Cleanup(func() {
		model.LOG_DB = previousLogDB
		common.MemoryCacheEnabled = previousMemoryCache
		common.RedisEnabled = previousRedis
	})
	claims, err := common.Marshal(map[string]any{
		"https://api.openai.com/auth": map[string]any{"chatgpt_account_id": "test-account"},
	})
	require.NoError(t, err)
	previousExchange := exchangeCodexOAuthCode
	exchangedProxy := ""
	exchangeCodexOAuthCode = func(_ context.Context, _, _, proxy string) (*service.CodexOAuthTokenResult, error) {
		exchangedProxy = proxy
		return &service.CodexOAuthTokenResult{
			AccessToken:  "fixture." + base64.RawURLEncoding.EncodeToString(claims) + ".signature",
			RefreshToken: "synthetic-refresh-token",
			ExpiresAt:    time.Now().Add(time.Hour),
		}, nil
	}
	t.Cleanup(func() { exchangeCodexOAuthCode = previousExchange })
	payload, err := common.Marshal(codexOAuthFlowPayload{Verifier: "synthetic-verifier"})
	require.NoError(t, err)
	state, _, err := model.CreateAuthFlow(model.AuthFlowCreate{
		Purpose: model.AuthFlowPurposeCodexOAuth, Provider: "openai", UserId: 42,
		SessionId: "session-42", Payload: string(payload), ExpiresAt: time.Now().Add(time.Minute),
	})
	require.NoError(t, err)
	body, err := common.Marshal(gin.H{
		"input": "http://localhost:1455/auth/callback?code=synthetic-code&state=" + state + "#_=_",
		"create": gin.H{"mode": "single", "channel": gin.H{
			"name": "Codex fixture", "type": 57, "key": "", "models": "gpt-5", "group": "default", "setting": `{"proxy":"http://127.0.0.1:8888"}`,
		}},
	})
	require.NoError(t, err)
	complete := func() *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/channel/codex/oauth/complete", strings.NewReader(string(body)))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("id", 42)
		c.Set("session_id", "session-42")
		c.Set("auth_version", int64(1))
		c.Set("session_version", int64(1))
		CompleteCodexOAuth(c)
		return recorder
	}

	first := complete()
	require.Equal(t, http.StatusOK, first.Code)
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			ChannelID int    `json:"channel_id"`
			Key       string `json:"key"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(first.Body.Bytes(), &response))
	require.True(t, response.Success, first.Body.String())
	assert.Positive(t, response.Data.ChannelID)
	assert.Equal(t, "http://127.0.0.1:8888", exchangedProxy)
	assert.Empty(t, response.Data.Key)
	assert.NotContains(t, first.Body.String(), "synthetic-refresh-token")
	assert.NotContains(t, first.Body.String(), "fixture.")
	var saved model.Channel
	require.NoError(t, model.DB.First(&saved, response.Data.ChannelID).Error)
	assert.Contains(t, saved.Key, "synthetic-refresh-token")
	second := complete()
	assert.Contains(t, second.Body.String(), "Codex login session expired")
	var count int64
	require.NoError(t, model.DB.Model(&model.Channel{}).Count(&count).Error)
	assert.EqualValues(t, 1, count)
}

func TestCompleteCodexOAuthInvalidCreatePreservesAuthorizationCode(t *testing.T) {
	setupAuthFlowControllerTest(t)
	previousExchange := exchangeCodexOAuthCode
	exchanges := 0
	exchangeCodexOAuthCode = func(context.Context, string, string, string) (*service.CodexOAuthTokenResult, error) {
		exchanges++
		return nil, nil
	}
	t.Cleanup(func() { exchangeCodexOAuthCode = previousExchange })
	payload, err := common.Marshal(codexOAuthFlowPayload{Verifier: "synthetic-verifier"})
	require.NoError(t, err)
	state, _, err := model.CreateAuthFlow(model.AuthFlowCreate{
		Purpose: model.AuthFlowPurposeCodexOAuth, Provider: "openai", UserId: 42,
		SessionId: "session-42", Payload: string(payload), ExpiresAt: time.Now().Add(time.Minute),
	})
	require.NoError(t, err)
	for _, tc := range []struct {
		name  string
		field string
		value any
	}{
		{name: "invalid proxy", field: "setting", value: `{"proxy":"not-a-proxy-url"}`},
		{name: "overlong model", field: "models", value: strings.Repeat("m", 256)},
		{name: "client credential", field: "key", value: "client-supplied-key"},
		{name: "existing channel", field: "id", value: 99},
		{name: "wrong provider", field: "type", value: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			channel := gin.H{"name": "Codex fixture", "type": 57, "key": "", "models": "gpt-5", "group": "default", "setting": `{}`}
			channel[tc.field] = tc.value
			body, err := common.Marshal(gin.H{"input": "code=fixture&state=" + state, "create": gin.H{"mode": "single", "channel": channel}})
			require.NoError(t, err)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/api/channel/codex/oauth/complete", strings.NewReader(string(body)))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Set("id", 42)
			c.Set("session_id", "session-42")
			c.Set("auth_version", int64(1))
			c.Set("session_version", int64(1))
			CompleteCodexOAuth(c)
			assert.Equal(t, http.StatusBadRequest, recorder.Code)
			assert.Zero(t, exchanges)
			_, err = model.GetAuthFlow(state, model.AuthFlowMatch{Purpose: model.AuthFlowPurposeCodexOAuth, Provider: "openai", UserId: 42, SessionId: "session-42"})
			require.NoError(t, err, "invalid fields must not consume the one-time flow")
		})
	}
}

func TestCompleteCodexOAuthForChannelRejectsDifferentAccount(t *testing.T) {
	setupAuthFlowControllerTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}))
	previousMemoryCache := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	t.Cleanup(func() { common.MemoryCacheEnabled = previousMemoryCache })
	channel := &model.Channel{Type: constant.ChannelTypeCodex, Name: "Existing Codex", Key: `{"access_token":"old","refresh_token":"old-refresh","account_id":"account-original"}`}
	require.NoError(t, model.DB.Create(channel).Error)
	establishCodexControllerCredentialEvidence(t, channel.Id, channel.Key, "account-original")
	establishCodexControllerCredentialEvidence(t, channel.Id+1000, "known-different-account-credential", "account-different")
	claims, err := common.Marshal(map[string]any{
		"https://api.openai.com/auth": map[string]any{"chatgpt_account_id": "account-different"},
	})
	require.NoError(t, err)
	previousExchange := exchangeCodexOAuthCode
	exchangeCodexOAuthCode = func(context.Context, string, string, string) (*service.CodexOAuthTokenResult, error) {
		return &service.CodexOAuthTokenResult{
			AccessToken:  "fixture." + base64.RawURLEncoding.EncodeToString(claims) + ".signature",
			RefreshToken: "different-refresh",
			ExpiresAt:    time.Now().Add(time.Hour),
		}, nil
	}
	t.Cleanup(func() { exchangeCodexOAuthCode = previousExchange })
	payload, err := common.Marshal(codexOAuthFlowPayload{Verifier: "synthetic-verifier", ChannelID: channel.Id, TargetCredentialDigest: codexCredentialDigest(channel.Key)})
	require.NoError(t, err)
	state, _, err := model.CreateAuthFlow(model.AuthFlowCreate{
		Purpose: model.AuthFlowPurposeCodexOAuth, Provider: "openai", UserId: 42,
		SessionId: "session-42", Payload: string(payload), ExpiresAt: time.Now().Add(time.Minute),
	})
	require.NoError(t, err)
	body, err := common.Marshal(gin.H{"input": "code=fixture&state=" + state})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/channel/codex/oauth/complete", strings.NewReader(string(body)))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: strconv.Itoa(channel.Id)}}
	c.Set("id", 42)
	c.Set("session_id", "session-42")
	c.Set("auth_version", int64(1))
	c.Set("session_version", int64(1))

	CompleteCodexOAuthForChannel(c)

	assert.Equal(t, http.StatusConflict, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"code":"CODEX_CREDENTIAL_SUBJECT_CONFLICT"`)
	assert.NotContains(t, recorder.Body.String(), "different-refresh")
	var stored model.Channel
	require.NoError(t, model.DB.First(&stored, channel.Id).Error)
	assert.Equal(t, channel.Key, stored.Key)
}

func establishCodexControllerCredentialEvidence(t *testing.T, channelID int, credential, accountID string) {
	t.Helper()
	setupChannelQuotaIdentityFixture(t, model.DB)
	require.NoError(t, model.DB.AutoMigrate(&model.ChannelQuotaSnapshot{}))
	keyring, err := common.LoadChannelQuotaIdentityKeyring()
	require.NoError(t, err)
	provider := "channel_type_" + strconv.Itoa(constant.ChannelTypeCodex)
	credentialIdentity, err := model.ResolveChannelQuotaIdentity(t.Context(), model.DB, keyring, provider, common.ChannelQuotaIdentityKindCredential, []byte(credential))
	require.NoError(t, err)
	accountIdentity, err := model.ResolveChannelQuotaIdentity(t.Context(), model.DB, keyring, provider, common.ChannelQuotaIdentityKindProviderAccount, []byte(accountID))
	require.NoError(t, err)
	now := time.Now().Unix()
	require.NoError(t, model.DB.Create(&model.ChannelQuotaSamplingTarget{
		ChannelID: channelID, SubjectRef: credentialIdentity.SubjectRef, IdentityQuality: credentialIdentity.Quality,
		ConfirmedSubjectRef: accountIdentity.SubjectRef, Active: true, LastResult: "sampled", CreatedAt: now, UpdatedAt: now,
	}).Error)
	require.NoError(t, model.DB.Create(&model.ChannelQuotaSnapshot{
		ChannelId: channelID, SubjectRef: accountIdentity.SubjectRef, IdentityQuality: accountIdentity.Quality,
		CodexObservationQualified: true, ObservedAt: now, MetricType: "codex_rate_limit", WindowType: "five_hour",
		WindowSeconds: 18000, ResetAt: now + 18000, Source: "codex_wham_usage_primary", Status: "success", Unit: "percent",
	}).Error)
}

func codexControllerAccountJWT(t *testing.T, accountID string) string {
	t.Helper()
	claims, err := common.Marshal(map[string]any{
		"https://api.openai.com/auth": map[string]any{"chatgpt_account_id": accountID},
	})
	require.NoError(t, err)
	return "fixture." + base64.RawURLEncoding.EncodeToString(claims) + ".signature"
}
