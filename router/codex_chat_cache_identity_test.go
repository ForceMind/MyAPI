package router

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func codexChatCacheIdentityRequest(t *testing.T, fixture *codexSessionHeadersFixture, body string, withSession bool) (string, http.Header) {
	t.Helper()
	response := fixture.request(t, "/v1/chat/completions", []byte(body), withSession)
	require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
	var payload map[string]json.RawMessage
	require.NoError(t, common.Unmarshal(<-fixture.observedBodies, &payload))
	key := ""
	if raw, exists := payload["prompt_cache_key"]; exists {
		require.NoError(t, common.Unmarshal(raw, &key))
	}
	headers := <-fixture.observed
	assert.Empty(t, headers.Get("Cookie"))
	assert.Empty(t, headers.Get("X-Api-Key"))
	assert.Empty(t, headers.Get("Proxy-Authorization"))
	assert.Empty(t, headers.Get("X-Unrelated-Private"))
	assert.Equal(t, "Bearer synthetic-header-access", headers.Get("Authorization"))
	assert.Zero(t, fixture.external.Load())
	assert.NotContains(t, key, "synthetic-header-access")
	assert.NotContains(t, headers.Get("Session_id"), "synthetic-header-access")
	return key, headers
}

func TestCodexChatDefaultCacheIdentitySyntheticBoundary(t *testing.T) {
	const first = `{"model":"gpt-5-codex","messages":[{"role":"system","content":"synthetic instructions"},{"role":"user","content":"synthetic first question"}],"stream":true}`
	const appendTurn = `{"model":"gpt-5-codex","messages":[{"role":"system","content":"synthetic instructions"},{"role":"user","content":"synthetic first question"},{"role":"assistant","content":"synthetic reply"},{"role":"user","content":"synthetic second question"}],"stream":true}`
	var baseKey, baseSession string
	for _, test := range []struct {
		name    string
		tokenID int
		account string
	}{
		{name: "first and appended", tokenID: 101, account: "synthetic-account-a"},
		{name: "different API key", tokenID: 102, account: "synthetic-account-a"},
		{name: "different upstream account", tokenID: 101, account: "synthetic-account-b"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newCodexSessionHeadersFixture(t, "gpt-5-codex", false, false, "", func(f *codexSessionHeadersFixture) {
				f.compareAPIKeyID, f.compareAccountID = test.tokenID, test.account
				f.observedBodies = make(chan []byte, 1)
				f.responseStatus, f.responseSSE = http.StatusBadRequest, `{"error":{"type":"invalid_request_error","message":"synthetic boundary stop"}}`
			})
			t.Setenv("MYAPI_CODEX_CHAT_DEFAULT_CACHE_IDENTITY_ENABLED", "true")
			key, headers := codexChatCacheIdentityRequest(t, fixture, first, false)
			require.NotEmpty(t, key)
			require.NotEmpty(t, headers.Get("Session_id"))
			appendedKey, appendedHeaders := codexChatCacheIdentityRequest(t, fixture, appendTurn, false)
			assert.Equal(t, key, appendedKey)
			assert.Equal(t, headers.Get("Session_id"), appendedHeaders.Get("Session_id"))
			assert.Empty(t, headers.Get("Thread_id"))
			assert.Empty(t, headers.Get("X-Codex-Turn-State"))
			assert.EqualValues(t, 2, fixture.calls.Load(), "each synthetic generation is sent exactly once")
			if test.name == "first and appended" {
				baseKey, baseSession = key, headers.Get("Session_id")
			} else {
				assert.NotEqual(t, baseKey, key)
				assert.NotEqual(t, baseSession, headers.Get("Session_id"))
			}
		})
	}
}

func TestCodexChatDefaultCacheIdentityPrecedenceAndOptIn(t *testing.T) {
	for _, test := range []struct {
		name         string
		flag         string
		withSession  bool
		cacheKey     string
		headerConfig string
		operations   []any
		wantKey      string
		wantSession  string
		generatedKey bool
		generatedSID bool
	}{
		{name: "default off"},
		{name: "invalid flag stays off", flag: "invalid"},
		{name: "explicit key only", flag: "true", cacheKey: "synthetic-explicit-key", wantKey: "synthetic-explicit-key", generatedSID: true},
		{name: "explicit session only", flag: "true", withSession: true, wantSession: "client-session", generatedKey: true},
		{name: "both explicit", flag: "true", withSession: true, cacheKey: "synthetic-explicit-key", wantKey: "synthetic-explicit-key", wantSession: "client-session"},
		{name: "operator header", flag: "true", headerConfig: `{"Session_id":"synthetic-operator"}`, wantSession: "synthetic-operator", generatedKey: true},
		{name: "delete generated identities", flag: "true", operations: []any{
			map[string]any{"mode": "delete_header", "path": "Session_id"},
			map[string]any{"mode": "delete", "path": "prompt_cache_key"},
		}},
		{name: "replace generated identities", flag: "true", wantKey: "synthetic-operator-key", wantSession: "synthetic-operator-session", operations: []any{
			map[string]any{"mode": "set_header", "path": "Session_id", "value": "synthetic-operator-session"},
			map[string]any{"mode": "set", "path": "prompt_cache_key", "value": "synthetic-operator-key"},
		}},
		{name: "unrelated operations preserve generated identities", flag: "true", generatedKey: true, generatedSID: true, operations: []any{
			map[string]any{"mode": "set", "path": "temperature", "value": 0},
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newCodexSessionHeadersFixture(t, "gpt-5-codex", false, false, test.headerConfig, func(f *codexSessionHeadersFixture) {
				f.observedBodies = make(chan []byte, 1)
				f.responseStatus, f.responseSSE = http.StatusBadRequest, `{"error":{"type":"invalid_request_error","message":"synthetic boundary stop"}}`
			})
			t.Setenv("MYAPI_CODEX_CHAT_DEFAULT_CACHE_IDENTITY_ENABLED", test.flag)
			if len(test.operations) > 0 {
				operations, err := common.Marshal(map[string]any{"operations": test.operations})
				require.NoError(t, err)
				require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", fixture.channelID).Update("param_override", string(operations)).Error)
			}
			body, err := common.Marshal(map[string]any{"model": "gpt-5-codex", "messages": []map[string]string{{"role": "user", "content": "synthetic question"}}, "stream": true, "prompt_cache_key": test.cacheKey})
			require.NoError(t, err)
			key, headers := codexChatCacheIdentityRequest(t, fixture, string(body), test.withSession)
			if test.generatedKey {
				assert.NotEmpty(t, key)
			} else {
				assert.Equal(t, test.wantKey, key)
			}
			if test.generatedSID {
				assert.NotEmpty(t, headers.Get("Session_id"))
			} else {
				assert.Equal(t, test.wantSession, headers.Get("Session_id"))
			}
			assert.EqualValues(t, 1, fixture.calls.Load())
		})
	}
}

func TestCodexChatDefaultCacheIdentityStrictGateAndNoReplay(t *testing.T) {
	t.Run("strict acceptance still rejects configured overrides", func(t *testing.T) {
		fixture := newCodexSessionHeadersFixture(t, "gpt-5-codex", false, false, `{"Session_id":"synthetic-operator"}`)
		t.Setenv("MYAPI_CODEX_CHAT_DEFAULT_CACHE_IDENTITY_ENABLED", "true")
		t.Setenv("MYAPI_UPSTREAM_ACCEPTANCE_ENABLED", "true")
		run, err := service.StartUpstreamAcceptanceRun(context.Background(), fixture.channelID, service.UpstreamAcceptanceLimits{DurationSeconds: 60, MaxTotal: 1, MaxGeneration: 1})
		require.NoError(t, err)
		t.Cleanup(func() {
			_, stopErr := service.StopUpstreamAcceptanceRun(context.Background(), run.ID)
			assert.NoError(t, stopErr)
		})
		response := fixture.request(t, "/v1/chat/completions", []byte(`{"model":"gpt-5-codex","messages":[{"role":"user","content":"synthetic question"}],"stream":true}`), false)
		assert.Contains(t, response.Body.String(), "upstream_acceptance_denied")
		assert.Zero(t, fixture.calls.Load())
		assert.Zero(t, fixture.external.Load())
	})
	t.Run("ambiguous generation never replays", func(t *testing.T) {
		fixture := newCodexSessionHeadersFixture(t, "gpt-5-codex", false, false, "")
		t.Setenv("MYAPI_CODEX_CHAT_DEFAULT_CACHE_IDENTITY_ENABLED", "true")
		fixture.disconnect.Store(true)
		response := fixture.request(t, "/v1/chat/completions", []byte(`{"model":"gpt-5-codex","messages":[{"role":"user","content":"synthetic question"}],"stream":true}`), false)
		assert.NotContains(t, response.Body.String(), "response.completed")
		assert.EqualValues(t, 1, fixture.calls.Load())
		assert.Zero(t, fixture.external.Load())
		headers := <-fixture.observed
		assert.NotEmpty(t, headers.Get("Session_id"))
	})
}
