package controller

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
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
