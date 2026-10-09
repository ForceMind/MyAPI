package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/relay/channel"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlaygroundAuthMarksBothContextsWithoutChangingSession(t *testing.T) {
	_, _, token, access := playgroundAuthFixture(t)
	router := playgroundAuthRouter(t, func(c *gin.Context) {
		assert.True(t, common.SensitiveRequestDiagnostics(c))
		assert.True(t, common.SensitiveRequestDiagnostics(c.Request.Context()))
		assert.Empty(t, c.GetHeader("Authorization"))
		assert.Equal(t, token.Key, c.GetString("token_key"))
		assert.Equal(t, "/pg/chat/completions", common.DiagnosticRequestPath(c))
		c.Status(http.StatusNoContent)
	})
	response := playgroundAuthRequest(router, http.MethodPost, "/pg/chat/completions", access, strconv.Itoa(token.Id), `{"model":"gpt-4o"}`)
	assert.Equal(t, http.StatusNoContent, response.Code, response.Body.String())
}

func TestPlaygroundCredentialsCannotReachHeaderOverridesOrParamSnapshots(t *testing.T) {
	_, _, token, access := playgroundAuthFixture(t)
	credentials := http.Header{
		"Authorization": {"Bearer " + access}, "Cookie": {"session=browser-cookie"},
		"Proxy-Authorization": {"private-proxy"}, "X-Api-Key": {"private-api-key"},
		"X-Goog-Api-Key": {"private-google-key"}, "Mj-Api-Secret": {"private-mj-secret"},
		"Sec-Websocket-Protocol": {"openai-insecure-api-key.private-ws-key"}, "New-Api-User": {"9999"},
		"X-Myapi-Key-Id": {strconv.Itoa(token.Id)}, "X-Auth-Token": {"private-auth-token"}, "X-Access-Token": {"private-access-token"},
	}
	var downstreamRequest *http.Request
	router := playgroundAuthRouter(t, func(c *gin.Context) {
		info, err := relaycommon.GenRelayInfo(c, types.RelayFormatOpenAI, nil, nil)
		require.NoError(t, err)
		assert.Equal(t, token.Id, info.TokenId)
		assert.Equal(t, token.Key, info.TokenKey)
		info.ChannelMeta = &relaycommon.ChannelMeta{HeadersOverride: map[string]interface{}{"*": ""}}
		for name := range credentials {
			assert.Empty(t, c.GetHeader(name), name)
			info.HeadersOverride["copied-"+name] = "{client_header:" + name + "}"
		}
		headers, err := channel.ResolveHeaderOverride(info, c)
		require.NoError(t, err)
		assert.Equal(t, "safe-value", headers["x-safe-header"])
		encoded, err := common.Marshal(map[string]any{"headers": headers, "params": relaycommon.BuildParamOverrideContext(info), "snapshot": info.RequestHeaders})
		require.NoError(t, err)
		for name, values := range credentials {
			assert.NotContains(t, headers, "copied-"+strings.ToLower(name))
			if name != "X-Myapi-Key-Id" {
				assert.NotContains(t, string(encoded), values[0])
			}
		}
		assert.NotContains(t, info.RequestHeaders, "X-Myapi-Key-Id")
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), "downstream-dispatch", "preserved"))
		downstreamRequest = c.Request
		c.Status(http.StatusNoContent)
	})
	request := httptest.NewRequest(http.MethodPost, "/pg/chat/completions", strings.NewReader(`{"model":"gpt-4o"}`))
	request.Header = credentials.Clone()
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Safe-Header", "safe-value")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	require.Equal(t, http.StatusNoContent, response.Code, response.Body.String())
	assert.Equal(t, "/pg/chat/completions", request.URL.Path)
	assert.Equal(t, "Bearer "+access, request.Header.Get("Authorization"))
	assert.Equal(t, "session=browser-cookie", request.Header.Get("Cookie"))
	assert.Equal(t, "/pg/chat/completions", downstreamRequest.URL.Path)
	assert.Equal(t, "preserved", downstreamRequest.Context().Value("downstream-dispatch"))
	assert.True(t, common.SensitiveRequestDiagnostics(downstreamRequest.Context()))
}
