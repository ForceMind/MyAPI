package router

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/middleware"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/service"
	"github.com/ForceMind/MyAPI/service/authz"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type managementHTTPFixture struct {
	engine                     *gin.Engine
	root, admin                *model.User
	session, adminSession, pat string
}

func newManagementHTTPFixture(t *testing.T) *managementHTTPFixture {
	t.Helper()
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousRedis, previousMemory, previousMaster := common.RedisEnabled, common.MemoryCacheEnabled, common.IsMasterNode
	previousSecret, previousGlobal, previousCritical := common.SessionSecret, common.GlobalApiRateLimitEnable, common.CriticalRateLimitEnable
	previousType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	conn, err := db.DB()
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSession{}, &model.Channel{}, &model.Ability{}, &model.Token{}, &model.Log{}, &model.BillingLogProjectionIdentity{}, &model.SystemInstance{}, &model.ManagementKey{}, &model.ManagementKeyAudit{}, &model.CasbinRule{}, &model.AuthzRole{}, &model.ChannelQuotaAlertEvent{}))
	model.DB, model.LOG_DB = db, db
	common.RedisEnabled, common.MemoryCacheEnabled, common.IsMasterNode = false, false, true
	common.SessionSecret = "management-http-test-session-secret"
	common.GlobalApiRateLimitEnable, common.CriticalRateLimitEnable = false, false
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	common.SetLogDatabaseType(common.DatabaseTypeSQLite)
	require.NoError(t, authz.Init(db))
	t.Setenv("MYAPI_EDITION", middleware.EditionFull)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.RedisEnabled, common.MemoryCacheEnabled, common.IsMasterNode = previousRedis, previousMemory, previousMaster
		common.SessionSecret, common.GlobalApiRateLimitEnable, common.CriticalRateLimitEnable = previousSecret, previousGlobal, previousCritical
		common.SetMainDatabaseType(previousType)
		common.SetLogDatabaseType(previousLogType)
		require.NoError(t, conn.Close())
	})
	pat := "management-root-old-pat"
	root := &model.User{Username: "mgmt-root", Password: "unused", Role: common.RoleRootUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, AccessToken: &pat, AffCode: "mgmt-root-aff"}
	admin := &model.User{Username: "mgmt-admin", Password: "unused", Role: common.RoleAdminUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, AffCode: "mgmt-admin-aff"}
	require.NoError(t, db.Create(root).Error)
	require.NoError(t, db.Create(admin).Error)
	_, session := createCodexLocalRouteSession(t, root)
	_, adminSession := createCodexLocalRouteSession(t, admin)
	settings := `{"api_key":"nested-channel-secret"}`
	override := `{"Authorization":"Bearer override-secret"}`
	require.NoError(t, db.Create(&model.Channel{Id: 1, Name: "channel one", Key: "channel-secret-full", Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Setting: &settings, HeaderOverride: &override, Other: "other-secret", OtherInfo: "other-info-secret"}).Error)
	require.NoError(t, db.Create(&model.Channel{Id: 2, Name: "channel two", Key: "channel-two-secret", Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled}).Error)
	require.NoError(t, db.Create(&model.Token{Id: 1, UserId: root.Id, Name: "owned token", Key: "sk-very-secret-token-contents", Status: common.TokenStatusEnabled, RemainQuota: 100, ExpiredTime: -1}).Error)
	require.NoError(t, db.Create(&model.Token{Id: 2, UserId: root.Id, Name: "second owned token", Key: "sk-second-secret-token", Status: common.TokenStatusEnabled, RemainQuota: 100, ExpiredTime: -1}).Error)
	require.NoError(t, db.Create(&model.Token{Id: 3, UserId: admin.Id, Name: "other token", Key: "sk-admin-secret-token", Status: common.TokenStatusEnabled, RemainQuota: 100, ExpiredTime: -1}).Error)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	// Keep these HTTP tests deterministic; the dedicated durable audit assertions
	// below cover key mutations, while actual status controllers retain their audit.
	engine.Use(func(c *gin.Context) { common.SetContextKey(c, constant.ContextKeyAuditLogged, true); c.Next() })
	engine.Use(middleware.EditionGuard())
	SetApiRouter(engine)
	return &managementHTTPFixture{engine: engine, root: root, admin: admin, session: session, adminSession: adminSession, pat: pat}
}

func (fixture *managementHTTPFixture) request(method, path, credential, body, origin, confirm string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, "http://myapi.local"+path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+credential)
	if origin != "" {
		request.Header.Set("Origin", origin)
	}
	if confirm != "" {
		request.Header.Set("X-MyAPI-Confirm", confirm)
	}
	response := httptest.NewRecorder()
	fixture.engine.ServeHTTP(response, request)
	return response
}

func (fixture *managementHTTPFixture) create(t *testing.T, session string, scopes []string, channels, tokens []int) (model.ManagementKeyMetadata, string) {
	t.Helper()
	body, err := common.Marshal(service.CreateManagementKeyInput{Name: "test key", Scopes: scopes, ChannelIDs: channels, TokenIDs: tokens})
	require.NoError(t, err)
	response := fixture.request("POST", "/api/management/keys", session, string(body), "http://myapi.local", "")
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var envelope struct {
		Success bool `json:"success"`
		Data    struct {
			Key    model.ManagementKeyMetadata `json:"key"`
			Secret string                      `json:"secret"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &envelope))
	require.True(t, envelope.Success, response.Body.String())
	require.True(t, strings.HasPrefix(envelope.Data.Secret, model.ManagementKeyPrefix))
	return envelope.Data.Key, envelope.Data.Secret
}

func TestManagementKeyHTTPAuthenticationAndLifecycle(t *testing.T) {
	fixture := newManagementHTTPFixture(t)
	metadata, secret := fixture.create(t, fixture.session, service.ManagementKeyScopes, nil, nil)
	assert.InDelta(t, time.Now().Add(7*24*time.Hour).Unix(), metadata.ExpiresAt, 5)
	listed := fixture.request("GET", "/api/management/keys", fixture.session, "", "", "")
	require.Equal(t, 200, listed.Code)
	assert.NotContains(t, listed.Body.String(), secret)
	assert.NotContains(t, listed.Body.String(), "secret_hash")
	var listEnvelope struct {
		Data []model.ManagementKeyMetadata `json:"data"`
	}
	require.NoError(t, common.Unmarshal(listed.Body.Bytes(), &listEnvelope))
	require.Len(t, listEnvelope.Data, 1)
	assert.Equal(t, 403, fixture.request("GET", "/api/management/keys", fixture.pat, "", "", "").Code)
	assert.Equal(t, 403, fixture.request("GET", "/api/management/keys", secret, "", "", "").Code)
	assert.Equal(t, 403, fixture.request("POST", "/api/management/keys", fixture.session, `{"name":"bad","scopes":["logs.read"]}`, "https://foreign.invalid", "").Code)
	for _, body := range []string{`{"name":"bad","scopes":["unknown"]}`, `{"name":"bad","scopes":["logs.read","logs.read"]}`, `{"name":"bad","scopes":["logs.read"],"expires_at":1}`, fmt.Sprintf(`{"name":"bad","scopes":["logs.read"],"expires_at":%d}`, time.Now().Add(91*24*time.Hour).Unix()), `{"name":"bad","scopes":["tokens.read"],"token_ids":[3]}`} {
		response := fixture.request("POST", "/api/management/keys", fixture.session, body, "http://myapi.local", "")
		assert.Equal(t, 400, response.Code, response.Body.String())
	}
	capabilities := fixture.request("GET", "/api/management/capabilities", secret, "", "", "")
	require.Equal(t, 200, capabilities.Code, capabilities.Body.String())
	assert.Contains(t, capabilities.Body.String(), "channels.set_status")
	assert.NotContains(t, capabilities.Body.String(), secret)
	// A management-prefixed legacy PAT must never gain the legacy PAT behavior.
	reserved := model.ManagementKeyPrefix + "synthetic-legacy-pat"
	require.NoError(t, model.DB.Model(fixture.root).Update("access_token", reserved).Error)
	assert.Equal(t, 401, fixture.request("GET", "/api/user/self", reserved, "", "", "").Code)
	for _, path := range []string{"/api/channel/test/1", "/api/channel/update_balance/1", "/api/channel/1/codex/usage", "/api/user/token", "/api/status"} {
		response := fixture.request("GET", path, secret, "", "", "")
		assert.Equal(t, 403, response.Code, path)
		assert.NotContains(t, response.Body.String(), "secret")
	}
	assert.Equal(t, 403, fixture.request("POST", "/api/channel/1/key", secret, "{}", "", "").Code)
	// The existing secret proof requirement is still active for a live session.
	protected := fixture.request("POST", "/api/channel/1/key", fixture.session, "{}", "", "")
	assert.Equal(t, 403, protected.Code, protected.Body.String())
	assert.NotContains(t, protected.Body.String(), "channel-secret-full")
	require.NoError(t, model.DB.Model(fixture.root).Update("status", common.UserStatusDisabled).Error)
	assert.Equal(t, 401, fixture.request("GET", "/api/user/self", secret, "", "", "").Code)
	require.NoError(t, model.DB.Model(fixture.root).Updates(map[string]any{"status": common.UserStatusEnabled, "role": common.RoleCommonUser}).Error)
	assert.Equal(t, 401, fixture.request("GET", "/api/user/self", secret, "", "", "").Code)
	require.NoError(t, model.DB.Model(fixture.root).Update("role", common.RoleRootUser).Error)
	assert.Equal(t, 200, fixture.request("GET", "/api/user/self", secret, "", "", "").Code)
	assert.Equal(t, 200, fixture.request("DELETE", fmt.Sprintf("/api/management/keys/%d", metadata.ID), fixture.session, "", "http://myapi.local", "").Code)
	assert.Equal(t, 401, fixture.request("GET", "/api/user/self", secret, "", "", "").Code)
	var audit []model.ManagementKeyAudit
	require.NoError(t, model.DB.Where("key_id = ?", metadata.ID).Order("id").Find(&audit).Error)
	require.Len(t, audit, 2)
	assert.Equal(t, "created", audit[0].Action)
	assert.Equal(t, "revoked", audit[1].Action)
	expiry, expirySecret := fixture.create(t, fixture.session, []string{"system.read"}, nil, nil)
	require.NoError(t, model.DB.Model(&model.ManagementKey{}).Where("id = ?", expiry.ID).Update("expires_at", time.Now().Unix()-1).Error)
	assert.Equal(t, 401, fixture.request("GET", "/api/user/self", expirySecret, "", "", "").Code)
}

func TestManagementKeyHTTPTargetsAndCredentialProjection(t *testing.T) {
	fixture := newManagementHTTPFixture(t)
	// These are the original resource-scoped dashboard permissions. Host
	// operations deliberately cannot be combined with channel/token ID scopes.
	_, secret := fixture.create(t, fixture.session, []string{"channels.read", "channels.operate", "tokens.read", "logs.read", "system.read"}, []int{1}, []int{1})
	for _, path := range []string{"/api/channel/", "/api/channel/ops", "/api/channel/quota/status", "/api/channel/quota/events", "/api/token/", "/api/log/", "/api/log/stat", "/api/log/overview", "/api/channel/2", "/api/token/2", "/api/channel/2/quota/history"} {
		assert.Equal(t, 403, fixture.request("GET", path, secret, "", "", "").Code, path)
	}
	for _, path := range []string{"/api/channel/1", "/api/token/1"} {
		response := fixture.request("GET", path, secret, "", "", "")
		require.Equal(t, 200, response.Code, response.Body.String())
		for _, sensitive := range []string{"channel-secret-full", "nested-channel-secret", "override-secret", "other-secret", "other-info-secret", "sk-very-secret-token-contents", `"key"`, `"setting"`, `"header_override"`, `"other"`, `"allow_ips"`} {
			assert.NotContains(t, response.Body.String(), sensitive)
		}
	}
	assert.Equal(t, 403, fixture.request("POST", "/api/channel/1/status", secret, `{"status":2}`, "", "").Code)
	assert.Equal(t, 403, fixture.request("POST", "/api/channel/1/status", secret, `{"status":2}`, "", "channel.status:2:2").Code)
	assert.Equal(t, 403, fixture.request("POST", "/api/channel/1/status", secret, `{"status":2}`, "", "channel.status:1:1").Code)
	assert.Equal(t, 403, fixture.request("POST", "/api/channel/2/status", secret, `{"status":2}`, "", "channel.status:2:2").Code)
	assert.Equal(t, 403, fixture.request("POST", "/api/channel/1/status", secret, `{"status":1,"status":2}`, "", "channel.status:1:2").Code)
	response := fixture.request("POST", "/api/channel/1/status", secret, `{"status":2}`, "", "channel.status:1:2")
	require.Equal(t, 200, response.Code, response.Body.String())
	var channel model.Channel
	require.NoError(t, model.DB.First(&channel, 1).Error)
	assert.Equal(t, common.ChannelStatusManuallyDisabled, channel.Status)
	var logs []model.Log
	require.NoError(t, model.DB.Where("type = ?", model.LogTypeManage).Find(&logs).Error)
	audited := false
	for _, entry := range logs {
		if strings.Contains(entry.Other, `"auth_method":"management_key"`) {
			audited = true
			assert.Contains(t, entry.Other, `"management_key_id"`)
		}
		assert.NotContains(t, entry.Other, secret)
	}
	assert.True(t, audited)
	_, unrestricted := fixture.create(t, fixture.session, service.ManagementKeyScopes, nil, nil)
	response = fixture.request("GET", "/api/token/3", unrestricted, "", "", "")
	assert.NotContains(t, response.Body.String(), "sk-admin-secret-token")
	assert.Contains(t, response.Body.String(), `"success":false`)
	// A token-only restriction cannot be mistaken for a channel restriction, or vice versa.
	_, channelsOnly := fixture.create(t, fixture.session, []string{"channels.read", "tokens.read"}, []int{1}, nil)
	assert.Equal(t, 200, fixture.request("GET", "/api/token/2", channelsOnly, "", "", "").Code)
	_, tokensOnly := fixture.create(t, fixture.session, []string{"channels.read", "tokens.read"}, nil, []int{1})
	assert.Equal(t, 200, fixture.request("GET", "/api/channel/2", tokensOnly, "", "", "").Code)
	// Permission revocation persisted by another instance is effective immediately,
	// even while this instance's enforcer still has the previous allow snapshot.

	_, adminOperate := fixture.create(t, fixture.adminSession, []string{"channels.operate"}, nil, nil)
	require.NoError(t, model.DB.Create(&model.CasbinRule{Ptype: "p", V0: authz.UserSubject(fixture.admin.Id), V1: "channel", V2: "operate", V3: authz.EffectDeny}).Error)
	assert.Equal(t, 403, fixture.request("POST", "/api/channel/1/status", adminOperate, `{"status":1}`, "", "channel.status:1:1").Code)
	require.NoError(t, model.DB.First(&channel, 1).Error)
	assert.Equal(t, common.ChannelStatusManuallyDisabled, channel.Status)
	_, adminSecret := fixture.create(t, fixture.adminSession, []string{"channels.read", "system.read"}, nil, nil)
	assert.Equal(t, 200, fixture.request("GET", "/api/channel/1", adminSecret, "", "", "").Code)
	require.NoError(t, model.DB.Create(&model.CasbinRule{Ptype: "p", V0: authz.UserSubject(fixture.admin.Id), V1: "channel", V2: "read", V3: authz.EffectDeny}).Error)
	assert.Equal(t, 403, fixture.request("GET", "/api/channel/1", adminSecret, "", "", "").Code)
	assert.Equal(t, 403, fixture.request("GET", "/api/system-info/instances", adminSecret, "", "", "").Code)
	capabilities := fixture.request("GET", "/api/management/capabilities", adminSecret, "", "", "")
	var parsed struct {
		Data struct {
			Operations []string `json:"operations"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(capabilities.Body.Bytes(), &parsed))
	assert.NotContains(t, parsed.Data.Operations, "channels.get")
	assert.NotContains(t, parsed.Data.Operations, "system.instances")
}

func TestManagementKeyHTTPFailureResponsesNeverForwardSecrets(t *testing.T) {
	fixture := newManagementHTTPFixture(t)
	_, secret := fixture.create(t, fixture.session, []string{"channels.read"}, nil, nil)
	// The real channel handler reports an unknown row through ApiError; projected
	// management failures use a translated generic error, never the raw DB error.
	missing := fixture.request("GET", "/api/channel/9999", secret, "", "", "")
	assert.Equal(t, 502, missing.Code)
	assert.NotContains(t, missing.Body.String(), "record not found")
	assert.Contains(t, missing.Body.String(), "MANAGEMENT_RESPONSE_REJECTED")
	// Keep the scalar/null/zero distinction in the actual response contract.
	response := fixture.request("GET", "/api/channel/1", secret, "", "", "")
	var envelope map[string]json.RawMessage
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &envelope))
	var data map[string]json.RawMessage
	require.NoError(t, common.Unmarshal(envelope["data"], &data))
	assert.Equal(t, "0", string(data["used_quota"]))
	assert.Equal(t, "null", string(data["tag"]))
	assert.NotContains(t, data, "setting")
}

func TestManagementKeyHTTPLogUsageAndSystemProjection(t *testing.T) {
	fixture := newManagementHTTPFixture(t)
	_, secret := fixture.create(t, fixture.session, []string{"logs.read", "system.read"}, nil, nil)
	logs := []model.Log{
		{Id: 101, UserId: fixture.root.Id, CreatedAt: 100, Type: model.LogTypeConsume, RequestId: "request-known", Content: "raw-prompt-secret", Other: `{"cache_tokens":0,"model_ratio":1.5,"token_usage_evidence":{"schema":1,"input_tokens":9007199254740993,"output_tokens":0,"cache_read_tokens":null},"admin_info":{"access_token":"raw-admin-secret"},"prompt":"raw-prompt-secret","stream_status":{"end_error":"raw-upstream-secret"}}`},
		{Id: 102, UserId: fixture.root.Id, CreatedAt: 101, Type: model.LogTypeConsume, RequestId: "request-unknown", Other: `{"model_ratio":2}`},
	}
	require.NoError(t, model.DB.Create(&logs).Error)
	response := fixture.request("GET", "/api/log/?type=2", secret, "", "", "")
	require.Equal(t, 200, response.Code, response.Body.String())
	var envelope struct {
		Success bool `json:"success"`
		Data    struct {
			Items []map[string]json.RawMessage `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &envelope))
	require.True(t, envelope.Success)
	require.Len(t, envelope.Data.Items, 2)
	for _, sensitive := range []string{"raw-prompt-secret", "raw-admin-secret", "raw-upstream-secret", `"other"`, `"admin_info"`, `"content"`} {
		assert.NotContains(t, response.Body.String(), sensitive)
	}
	byRequest := map[string]map[string]json.RawMessage{}
	for _, item := range envelope.Data.Items {
		var requestID string
		require.NoError(t, common.Unmarshal(item["request_id"], &requestID))
		byRequest[requestID] = item
	}
	var usage map[string]json.RawMessage
	require.NoError(t, common.Unmarshal(byRequest["request-known"]["usage"], &usage))
	assert.Equal(t, "0", string(usage["cache_tokens"]))
	var evidence map[string]json.RawMessage
	require.NoError(t, common.Unmarshal(usage["token_usage_evidence"], &evidence))
	assert.Equal(t, "9007199254740993", string(evidence["input_tokens"]))
	assert.Equal(t, "null", string(evidence["cache_read_tokens"]))
	require.NoError(t, common.Unmarshal(byRequest["request-unknown"]["usage"], &usage))
	assert.NotContains(t, string(byRequest["request-unknown"]["usage"]), "cache_tokens")
	instanceInfo := `{"schema_version":1,"runtime":{"version":"v-test","goos":"linux","goarch":"amd64","started_at":0,"key":"runtime-secret"},"resources":{"cpu":{"usage_percent":0},"memory":{"usage_percent":null},"storage":{"total_bytes":9007199254740993}},"extra":{"password":"extra-secret"},"host":{"hostname":"internal-secret"}}`
	require.NoError(t, model.DB.Create(&model.SystemInstance{NodeName: "test-node", Info: instanceInfo, StartedAt: 1, LastSeenAt: time.Now().Unix()}).Error)
	response = fixture.request("GET", "/api/system-info/instances", secret, "", "", "")
	require.Equal(t, 200, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), `"version":"v-test"`)
	assert.Contains(t, response.Body.String(), `"total_bytes":9007199254740993`)
	assert.Contains(t, response.Body.String(), `"usage_percent":null`)
	for _, value := range []string{"runtime-secret", "extra-secret", "internal-secret"} {
		assert.NotContains(t, response.Body.String(), value)
	}
}

func TestManagementKeyHTTPUnicodeNameBoundaries(t *testing.T) {
	fixture := newManagementHTTPFixture(t)
	for _, test := range []struct {
		name           string
		length, status int
	}{
		{"43 Chinese characters", 43, http.StatusOK},
		{"128 Chinese characters", 128, http.StatusOK},
		{"129 Chinese characters", 129, http.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			name := strings.Repeat("管", test.length)
			body, err := common.Marshal(service.CreateManagementKeyInput{Name: "  " + name + "  ", Scopes: []string{"system.read"}})
			require.NoError(t, err)
			response := fixture.request("POST", "/api/management/keys", fixture.session, string(body), "http://myapi.local", "")
			require.Equal(t, test.status, response.Code, response.Body.String())
			if test.status == http.StatusBadRequest {
				assert.Contains(t, response.Body.String(), "MANAGEMENT_INVALID_INPUT")
				return
			}
			var envelope struct {
				Success bool `json:"success"`
				Data    struct {
					Key model.ManagementKeyMetadata `json:"key"`
				} `json:"data"`
			}
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &envelope))
			require.True(t, envelope.Success, response.Body.String())
			assert.Equal(t, name, envelope.Data.Key.Name)
			var saved model.ManagementKey
			require.NoError(t, model.DB.First(&saved, envelope.Data.Key.ID).Error)
			assert.Equal(t, name, saved.Name)
		})
	}
}

func TestManagementKeyHTTPReadOnlyDiagnosticsProjection(t *testing.T) {
	fixture := newManagementHTTPFixture(t)
	_, secret := fixture.create(t, fixture.session, []string{"logs.read", "channels.read"}, nil, nil)

	capabilities := fixture.request("GET", "/api/management/capabilities", secret, "", "", "")
	require.Equal(t, http.StatusOK, capabilities.Code, capabilities.Body.String())
	assert.Contains(t, capabilities.Body.String(), "channels.quota_events")

	// 1. Log diagnostics projection by request_id, sanitizing error identifiers
	// and dropping raw admin_info secrets, content, and non-whitelisted fields.
	diagOther := `{"request_path":"/v1/chat/completions","status_code":503,"upstream_status_code":503,"error_type":"upstream_error","error_code":"raw provider secret!","channel_id":2,"channel_type":1,"admin_info":{"access_token":"raw-admin-secret","use_channel":["1","2","bad"],"is_multi_key":true,"multi_key_index":1,"relay_attempts":[{"channel_id":1,"key_index":0,"upstream_model":"gpt-5-codex","outcome":"retryable_refusal","status":429,"cooldown_seconds":60},{"channel_id":2,"key_index":1,"upstream_model":"gpt-5-codex","outcome":"failed","status":503}],"model_route":{"requested_model":"gpt-5-codex","upstream_model":"gpt-5-codex","endpoint":"/v1/chat/completions","reason":"configured_model","channel_id":1,"priority":10,"weight":1,"selection_policy":"existing_priority_weight_or_affinity","config_digest":"digest-ignored"},"billing_settlement":{"state":"applied","actual_quota":50,"event_key":"event-ignored"}}}`
	require.NoError(t, model.DB.Create(&model.Log{
		Id:        201,
		UserId:    fixture.root.Id,
		CreatedAt: 1795700100,
		Type:      model.LogTypeError,
		ChannelId: 2,
		ModelName: "gpt-5-codex",
		RequestId: "diag-req-503",
		Content:   "raw-error-body-secret",
		Other:     diagOther,
	}).Error)

	logResp := fixture.request("GET", "/api/log/?request_id=diag-req-503&type=5", secret, "", "", "")
	require.Equal(t, http.StatusOK, logResp.Code, logResp.Body.String())
	for _, forbidden := range []string{"raw-error-body-secret", "raw-admin-secret", "raw provider secret!", "digest-ignored", "event-ignored", `"other"`, `"admin_info"`, `"content"`} {
		assert.NotContains(t, logResp.Body.String(), forbidden)
	}
	var logEnvelope struct {
		Success bool `json:"success"`
		Data    struct {
			Items []struct {
				RequestID   string `json:"request_id"`
				Diagnostics struct {
					RequestPath        string   `json:"request_path"`
					StatusCode         int      `json:"status_code"`
					UpstreamStatusCode int      `json:"upstream_status_code"`
					ErrorType          string   `json:"error_type"`
					ErrorCode          string   `json:"error_code"`
					ChannelID          int      `json:"channel_id"`
					ChannelType        int      `json:"channel_type"`
					UseChannel         []string `json:"use_channel"`
					IsMultiKey         bool     `json:"is_multi_key"`
					MultiKeyIndex      int      `json:"multi_key_index"`
					RelayAttempts      []struct {
						ChannelID       int    `json:"channel_id"`
						KeyIndex        int    `json:"key_index"`
						UpstreamModel   string `json:"upstream_model"`
						Outcome         string `json:"outcome"`
						Status          int    `json:"status"`
						CooldownSeconds int    `json:"cooldown_seconds"`
					} `json:"relay_attempts"`
					ModelRoute struct {
						RequestedModel string `json:"requested_model"`
						UpstreamModel  string `json:"upstream_model"`
						ChannelID      int    `json:"channel_id"`
					} `json:"model_route"`
					BillingSettlement struct {
						State       string `json:"state"`
						ActualQuota int64  `json:"actual_quota"`
					} `json:"billing_settlement"`
				} `json:"diagnostics"`
			} `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(logResp.Body.Bytes(), &logEnvelope))
	require.True(t, logEnvelope.Success)
	require.Len(t, logEnvelope.Data.Items, 1)
	item := logEnvelope.Data.Items[0]
	assert.Equal(t, "diag-req-503", item.RequestID)
	assert.Equal(t, "/v1/chat/completions", item.Diagnostics.RequestPath)
	assert.Equal(t, 503, item.Diagnostics.StatusCode)
	assert.Equal(t, 503, item.Diagnostics.UpstreamStatusCode)
	assert.Equal(t, "upstream_error", item.Diagnostics.ErrorType)
	assert.Equal(t, "unknown_error", item.Diagnostics.ErrorCode)
	assert.Equal(t, []string{"1", "2"}, item.Diagnostics.UseChannel)
	assert.True(t, item.Diagnostics.IsMultiKey)
	assert.Equal(t, 1, item.Diagnostics.MultiKeyIndex)
	require.Len(t, item.Diagnostics.RelayAttempts, 2)
	assert.Equal(t, 1, item.Diagnostics.RelayAttempts[0].ChannelID)
	assert.Equal(t, "retryable_refusal", item.Diagnostics.RelayAttempts[0].Outcome)
	assert.Equal(t, 60, item.Diagnostics.RelayAttempts[0].CooldownSeconds)
	assert.Equal(t, 2, item.Diagnostics.RelayAttempts[1].ChannelID)
	assert.Equal(t, 1, item.Diagnostics.RelayAttempts[1].KeyIndex)
	assert.Equal(t, "gpt-5-codex", item.Diagnostics.ModelRoute.UpstreamModel)
	assert.Equal(t, "applied", item.Diagnostics.BillingSettlement.State)
	assert.Equal(t, int64(50), item.Diagnostics.BillingSettlement.ActualQuota)

	// 2. Channel status_time and multi-key status projection without exposing
	// status_reason, multi_key_disabled_reason, or raw credentials.
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", 2).Updates(map[string]any{
		"status":     common.ChannelStatusAutoDisabled,
		"other_info": `{"status_time":1795700105,"status_reason":"raw-channel-disable-reason-secret"}`,
		"channel_info": model.ChannelInfo{
			IsMultiKey:             true,
			MultiKeySize:           2,
			MultiKeyStatusList:     map[int]int{0: common.ChannelStatusAutoDisabled, 1: common.ChannelStatusEnabled},
			MultiKeyDisabledReason: map[int]string{0: "raw-multikey-reason-secret"},
			MultiKeyDisabledTime:   map[int]int64{0: 1795700104},
			MultiKeyPollingIndex:   1,
			MultiKeyMode:           constant.MultiKeyModePolling,
		},
	}).Error)

	chResp := fixture.request("GET", "/api/channel/2", secret, "", "", "")
	require.Equal(t, http.StatusOK, chResp.Code, chResp.Body.String())
	for _, forbidden := range []string{"channel-two-secret", "raw-channel-disable-reason-secret", "raw-multikey-reason-secret", `"other_info"`, `"multi_key_disabled_reason"`, `"key"`} {
		assert.NotContains(t, chResp.Body.String(), forbidden)
	}
	var chEnvelope struct {
		Success bool `json:"success"`
		Data    struct {
			Status      int   `json:"status"`
			StatusTime  int64 `json:"status_time"`
			ChannelInfo struct {
				IsMultiKey           bool          `json:"is_multi_key"`
				MultiKeySize         int           `json:"multi_key_size"`
				MultiKeyStatusList   map[int]int   `json:"multi_key_status_list"`
				MultiKeyDisabledTime map[int]int64 `json:"multi_key_disabled_time"`
				MultiKeyPollingIndex int           `json:"multi_key_polling_index"`
				MultiKeyMode         string        `json:"multi_key_mode"`
			} `json:"channel_info"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(chResp.Body.Bytes(), &chEnvelope))
	require.True(t, chEnvelope.Success)
	assert.Equal(t, common.ChannelStatusAutoDisabled, chEnvelope.Data.Status)
	assert.Equal(t, int64(1795700105), chEnvelope.Data.StatusTime)
	assert.True(t, chEnvelope.Data.ChannelInfo.IsMultiKey)
	assert.Equal(t, 2, chEnvelope.Data.ChannelInfo.MultiKeySize)
	assert.Equal(t, common.ChannelStatusAutoDisabled, chEnvelope.Data.ChannelInfo.MultiKeyStatusList[0])
	assert.Equal(t, common.ChannelStatusEnabled, chEnvelope.Data.ChannelInfo.MultiKeyStatusList[1])
	assert.Equal(t, 1, chEnvelope.Data.ChannelInfo.MultiKeyPollingIndex)
	assert.Equal(t, string(constant.MultiKeyModePolling), chEnvelope.Data.ChannelInfo.MultiKeyMode)
	assert.Empty(t, chEnvelope.Data.ChannelInfo.MultiKeyDisabledTime)

	// 3. Quota alert events read-only projection.
	require.NoError(t, model.DB.Create(&model.ChannelQuotaAlertEvent{
		EventKey:      "evt-diag-1",
		SeriesKey:     strings.Repeat("a", 64),
		SnapshotID:    7,
		ChannelID:     2,
		Status:        "critical",
		Kind:          "threshold",
		State:         "delivered",
		AttemptCount:  1,
		LastErrorCode: "",
		ObservedAt:    1795700102,
		CreatedAt:     1795700103,
		UpdatedAt:     1795700104,
		EvidenceJSON:  `{"secret_webhook":"must-not-leak"}`,
	}).Error)

	eventsResp := fixture.request("GET", "/api/channel/quota/events?channel_id=2", secret, "", "", "")
	require.Equal(t, http.StatusOK, eventsResp.Code, eventsResp.Body.String())
	assert.NotContains(t, eventsResp.Body.String(), "must-not-leak")
	assert.NotContains(t, eventsResp.Body.String(), `"series_ref"`)
	var eventsEnvelope struct {
		Success bool `json:"success"`
		Data    struct {
			Total int `json:"total"`
			Items []struct {
				EventKey   string `json:"event_key"`
				ChannelID  int    `json:"channel_id"`
				Status     string `json:"status"`
				State      string `json:"state"`
				ObservedAt int64  `json:"observed_at"`
			} `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(eventsResp.Body.Bytes(), &eventsEnvelope))
	require.True(t, eventsEnvelope.Success)
	require.Equal(t, 1, eventsEnvelope.Data.Total)
	require.Len(t, eventsEnvelope.Data.Items, 1)
	assert.Equal(t, "evt-diag-1", eventsEnvelope.Data.Items[0].EventKey)
	assert.Equal(t, 2, eventsEnvelope.Data.Items[0].ChannelID)
	assert.Equal(t, "critical", eventsEnvelope.Data.Items[0].Status)
	assert.Equal(t, "delivered", eventsEnvelope.Data.Items[0].State)
	assert.Equal(t, int64(1795700102), eventsEnvelope.Data.Items[0].ObservedAt)
}
