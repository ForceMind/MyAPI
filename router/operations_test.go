package router

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func operationsPlanFixture() map[string]interface{} {
	return map[string]interface{}{
		"id": strings.Repeat("a", 64), "digest": strings.Repeat("b", 64), "instance_id": "primary", "owner_id": 1, "key_id": 1,
		"state": "prepared", "created_at": time.Now().Unix(), "expires_at": time.Now().Add(30 * time.Minute).Unix(),
		"source_image_id": "sha256:" + strings.Repeat("1", 64), "target_image_id": "sha256:" + strings.Repeat("2", 64),
		"source_container_id": strings.Repeat("3", 64), "source_config_digest": strings.Repeat("4", 64),
		"source_sha": strings.Repeat("5", 40), "tree_sha": strings.Repeat("6", 40), "run_id": "41871314022", "producer_attempt": "1", "artifact_id": "13247808840",
		"manifest_sha256": strings.Repeat("7", 64), "source_version": "0.2.0-beta.32", "version": "0.2.0-beta.33", "platform": "linux/amd64",
		"backup_created": false, "target_start_attempted": false,
	}
}

func TestOperationsPanelRequiresBoundProofAndNeverForwardsCredentials(t *testing.T) {
	fixture := newManagementHTTPFixture(t)
	identity, internal, err := service.ParseDashboardAccessToken(fixture.session)
	require.NoError(t, err)
	require.True(t, internal)
	directory := t.TempDir()
	socket := filepath.Join(directory, "control.sock")
	bridgeToken := strings.Repeat("B", 48)
	tokenFile := filepath.Join(directory, "bridge.token")
	require.NoError(t, os.WriteFile(tokenFile, []byte(bridgeToken), 0600))
	listener, err := net.Listen("unix", socket)
	require.NoError(t, err)
	var mu sync.Mutex
	var received []string
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		received = append(received, r.Method+" "+r.URL.Path)
		assert.Equal(t, "Bearer "+bridgeToken, r.Header.Get("Authorization"))
		assert.Equal(t, "1", r.Header.Get("X-MyAPI-Actor-ID"))
		assert.Equal(t, "primary", r.Header.Get("X-MyAPI-Instance-ID"))
		assert.Empty(t, r.Header.Get("X-Security-Proof"))
		plan := operationsPlanFixture()
		plan["secret"] = "daemon-secret-must-not-leave-bridge"
		if r.Method == http.MethodPost {
			body, readErr := io.ReadAll(r.Body)
			assert.NoError(t, readErr)
			assert.JSONEq(t, `{"digest":"`+strings.Repeat("b", 64)+`"}`, string(body))
			plan["state"] = "approved"
		}
		var data interface{} = plan
		if r.URL.Path == "/v1/plans" {
			data = []interface{}{plan}
		}
		encoded, marshalErr := common.Marshal(map[string]interface{}{"success": true, "data": data})
		assert.NoError(t, marshalErr)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(encoded)
	})}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { require.NoError(t, server.Close()) })
	t.Setenv("MYAPI_OPERATIONS_SOCKET", socket)
	t.Setenv("MYAPI_OPERATIONS_BRIDGE_TOKEN_FILE", tokenFile)
	t.Setenv("MYAPI_OPERATIONS_INSTANCE_ID", "primary")

	path := "/api/operations/plans/" + strings.Repeat("a", 64) + "/approve"
	body := `{"digest":"` + strings.Repeat("b", 64) + `"}`
	for _, credential := range []string{fixture.pat, fixture.adminSession} {
		result := fixture.request("POST", path, credential, body, "http://myapi.local", "")
		assert.GreaterOrEqual(t, result.Code, 400)
	}
	_, key := fixture.create(t, fixture.session, []string{"operations.read", "operations.execute"}, nil, nil)
	assert.Equal(t, 403, fixture.request("POST", path, key, body, "http://myapi.local", "").Code)
	assert.Equal(t, 403, fixture.request("POST", path, fixture.session, body, "http://myapi.local", "").Code)
	proof, _, err := service.IssueSecurityProof(identity, "2fa", []string{service.OperationsApprovalScopeFor(strings.Repeat("a", 64), strings.Repeat("c", 64))})
	require.NoError(t, err)
	post := func(proofToken, origin string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "http://myapi.local"+path, strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", "Bearer "+fixture.session)
		request.Header.Set("Origin", origin)
		request.Header.Set("X-Security-Proof", proofToken)
		response := httptest.NewRecorder()
		fixture.engine.ServeHTTP(response, request)
		return response
	}
	assert.Equal(t, 403, post(proof, "http://myapi.local").Code)
	proof, _, err = service.IssueSecurityProof(identity, "passkey", []string{service.OperationsApprovalScopeFor(strings.Repeat("a", 64), strings.Repeat("b", 64))})
	require.NoError(t, err)
	assert.Equal(t, 403, post(proof, "http://other.local").Code)
	response := post(proof, "http://myapi.local")
	require.Equal(t, 200, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), `"state":"approved"`)
	for _, secret := range []string{bridgeToken, fixture.session, proof, "daemon-secret-must-not-leave-bridge"} {
		assert.NotContains(t, response.Body.String(), secret)
	}
	mu.Lock()
	assert.Equal(t, []string{"POST /v1/plans/" + strings.Repeat("a", 64) + "/approve"}, received)
	mu.Unlock()
	response = fixture.request("GET", "/api/operations/plans", fixture.session, "", "", "")
	require.Equal(t, 200, response.Code, response.Body.String())
	assert.NotContains(t, response.Body.String(), "daemon-secret-must-not-leave-bridge")
	require.NoError(t, model.DB.Model(fixture.root).Update("role", common.RoleAdminUser).Error)
	assert.Equal(t, 403, post(proof, "http://myapi.local").Code)
}

func TestOperationsDisabledAndPrivilegesDoNotUpgradeExistingKeys(t *testing.T) {
	fixture := newManagementHTTPFixture(t)
	t.Setenv("MYAPI_OPERATIONS_SOCKET", "")
	t.Setenv("MYAPI_OPERATIONS_BRIDGE_TOKEN_FILE", "")
	t.Setenv("MYAPI_OPERATIONS_INSTANCE_ID", "")
	response := fixture.request("GET", "/api/operations/plans", fixture.session, "", "", "")
	assert.Equal(t, 503, response.Code)
	assert.Contains(t, response.Body.String(), "OPERATIONS_UNAVAILABLE")
	for _, input := range []string{
		`{"name":"denied","scopes":["operations.execute"]}`,
		`{"name":"denied","scopes":["operations.read"],"channel_ids":[1]}`,
	} {
		credential := fixture.adminSession
		if strings.Contains(input, "channel_ids") {
			credential = fixture.session
		}
		response := fixture.request("POST", "/api/management/keys", credential, input, "http://myapi.local", "")
		assert.Equal(t, 400, response.Code, response.Body.String())
	}
	_, secret := fixture.create(t, fixture.session, []string{"system.read"}, nil, nil)
	response = fixture.request("GET", "/api/management/capabilities", secret, "", "", "")
	assert.Equal(t, 200, response.Code)
	assert.NotContains(t, response.Body.String(), "operations.execute")
	assert.NotContains(t, response.Body.String(), "operations.prepare")
}
