package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenMaintenanceDistinguishesRejectedPreconditionFromUnknownCommit(t *testing.T) {
	require.NoError(t, i18n.Init())
	for _, test := range []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"precondition rejected before migration", model.ErrTokenMaintenancePreconditionUnknown, http.StatusPreconditionFailed, "token_maintenance_precondition_unknown"},
		{"commit result remains unknown", model.ErrTokenMaintenanceUnknown, http.StatusServiceUnavailable, "token_maintenance_unknown"},
	} {
		t.Run(test.name, func(t *testing.T) {
			c, recorder := tokenMaintenanceContext(t, 1, "11", "")
			tokenMaintenanceError(c, test.err)
			assert.Equal(t, test.status, recorder.Code)
			var response struct {
				Success bool   `json:"success"`
				Code    string `json:"code"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			assert.False(t, response.Success)
			assert.Equal(t, test.code, response.Code)
		})
	}
}

func tokenMaintenanceContext(t *testing.T, actor int, pathID, body string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("id", actor)
	c.Params = gin.Params{{Key: "id", Value: pathID}}
	c.Request = httptest.NewRequest(http.MethodPost, "/api/token/"+pathID+"/account-migration/preview", strings.NewReader(body))
	return c, recorder
}

func TestTokenMaintenancePreviewRejectsAmbiguousBodiesWithoutChangingKey(t *testing.T) {
	db := subscriptionShareControllerFixture(t)
	var before model.Token
	require.NoError(t, db.First(&before, 11).Error)
	for _, test := range []struct{ name, body string }{
		{"missing fields", `{}`},
		{"case alias", `{"Target_Channel_ID":7,"target_shares":[{"source_pool_id":1,"target_pool_id":2,"share_bps":2000}]}`},
		{"duplicate target", `{"target_channel_id":7,"target_channel_id":8,"target_shares":[]}`},
		{"extra identity", `{"target_channel_id":7,"target_shares":[],"user_id":1}`},
		{"null target", `{"target_channel_id":null,"target_shares":[{"source_pool_id":1,"target_pool_id":2,"share_bps":2000}]}`},
		{"null windows", `{"target_channel_id":7,"target_shares":null}`},
		{"empty windows", `{"target_channel_id":7,"target_shares":[]}`},
		{"null window", `{"target_channel_id":7,"target_shares":[null]}`},
		{"nested alias", `{"target_channel_id":7,"target_shares":[{"Source_Pool_ID":1,"target_pool_id":2,"share_bps":2000}]}`},
		{"nested duplicate", `{"target_channel_id":7,"target_shares":[{"source_pool_id":1,"target_pool_id":2,"share_bps":2000,"share_bps":3000}]}`},
		{"null share", `{"target_channel_id":7,"target_shares":[{"source_pool_id":1,"target_pool_id":2,"share_bps":null}]}`},
		{"trailing object", `{"target_channel_id":7,"target_shares":[{"source_pool_id":1,"target_pool_id":2,"share_bps":2000}]} {}`},
		{"commit fields in preview", `{"target_channel_id":7,"target_shares":[{"source_pool_id":1,"target_pool_id":2,"share_bps":2000}],"confirmed":true}`},
		{"oversized", `{"target_channel_id":7,"target_shares":[],"padding":"` + strings.Repeat("x", 8192) + `"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			c, recorder := tokenMaintenanceContext(t, 1, "11", test.body)
			PreviewTokenAccountMigration(c)
			assert.Equal(t, http.StatusBadRequest, recorder.Code)
			assert.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
			assert.Contains(t, recorder.Body.String(), "token_maintenance_invalid")
			var after model.Token
			require.NoError(t, db.First(&after, 11).Error)
			assert.Equal(t, before, after)
		})
	}
}

func TestTokenMaintenanceCommitRequiresSessionAndDedicatedProof(t *testing.T) {
	subscriptionShareControllerFixture(t)
	previousSecret := common.SessionSecret
	common.SessionSecret = "maintenance-proof-synthetic-secret"
	t.Cleanup(func() { common.SessionSecret = previousSecret })
	identity := service.AuthIdentity{UserID: 1, SessionID: "maintenance-synthetic-sid", UserAuthVersion: 1, SessionVersion: 1}
	for _, test := range []struct {
		name, scope, sid string
		withSession      bool
	}{
		{"PAT has no browser session", "", "", false},
		{"missing proof", "", identity.SessionID, true},
		{"channel read proof is not migration permission", "channel.key.read", identity.SessionID, true},
		{"proof from previous session", tokenAccountMigrationProofScope, "other-synthetic-sid", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			c, recorder := tokenMaintenanceContext(t, 1, "11", `{}`)
			if test.withSession {
				c.Set("session_id", test.sid)
				c.Set("auth_version", int64(1))
				c.Set("session_version", int64(1))
			}
			if test.scope != "" {
				proof, _, err := service.IssueSecurityProof(identity, "2fa", []string{test.scope})
				require.NoError(t, err)
				c.Request.Header.Set("X-Security-Proof", proof)
			}
			CommitTokenAccountMigration(c)
			assert.Equal(t, http.StatusForbidden, recorder.Code)
			assert.Contains(t, recorder.Body.String(), "SECURITY_PROOF_")
			assert.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
		})
	}
	assert.True(t, isAllowedSecurityProofScope(tokenAccountMigrationProofScope))
	assert.False(t, isAllowedSecurityProofScope("token.key.rotate"), "unqualified rotation has no proof or submit capability")
}

func TestTokenMaintenanceRejectsMalformedPathAndOperationID(t *testing.T) {
	subscriptionShareControllerFixture(t)
	for _, pathID := range []string{"0", "-1", "invalid"} {
		c, recorder := tokenMaintenanceContext(t, 1, pathID, "")
		GetTokenMaintenance(c)
		assert.Equal(t, http.StatusBadRequest, recorder.Code)
	}
	for _, operationID := range []string{"", "bad", strings.Repeat("A", 64), strings.Repeat("z", 64)} {
		c, recorder := tokenMaintenanceContext(t, 1, "11", "")
		c.Params = append(c.Params, gin.Param{Key: "operation_id", Value: operationID})
		GetTokenMaintenanceOperation(c)
		assert.Equal(t, http.StatusBadRequest, recorder.Code)
	}
	c, recorder := tokenMaintenanceContext(t, 1, "11", "")
	c.Request = httptest.NewRequest(http.MethodGet, "/api/token/11/maintenance?owner=1", nil)
	GetTokenMaintenance(c)
	assert.Equal(t, http.StatusBadRequest, recorder.Code)
}
