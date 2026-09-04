package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ForceMind/MyAPI/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCodexLocalImportVerificationRequired(t *testing.T) {
	identity := service.AuthIdentity{
		UserID:          7,
		SessionID:       "session-7",
		UserAuthVersion: 2,
		SessionVersion:  3,
	}
	validProof, _, err := service.IssueSecurityProof(identity, "passkey", []string{SecurityProofScopeCodexLocalImport})
	require.NoError(t, err)
	wrongProof, _, err := service.IssueSecurityProof(identity, "passkey", []string{"channel.key.read"})
	require.NoError(t, err)

	for _, test := range []struct {
		name     string
		proof    string
		wantCode int
		wantBody string
	}{
		{name: "valid", proof: validProof, wantCode: http.StatusNoContent},
		{name: "missing", wantCode: http.StatusForbidden, wantBody: "SECURITY_PROOF_REQUIRED"},
		{name: "wrong scope", proof: wrongProof, wantCode: http.StatusForbidden, wantBody: "SECURITY_PROOF_SCOPE_MISMATCH"},
	} {
		t.Run(test.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			engine := gin.New()
			engine.POST("/import", func(c *gin.Context) {
				c.Set("id", identity.UserID)
				c.Set("session_id", identity.SessionID)
				c.Set("auth_version", identity.UserAuthVersion)
				c.Set("session_version", identity.SessionVersion)
				c.Next()
			}, CodexLocalImportVerificationRequired(), func(c *gin.Context) {
				c.Status(http.StatusNoContent)
			})
			request := httptest.NewRequest(http.MethodPost, "/import", nil)
			if test.proof != "" {
				request.Header.Set("X-Security-Proof", test.proof)
			}
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, request)
			assert.Equal(t, test.wantCode, recorder.Code)
			assert.Contains(t, recorder.Body.String(), test.wantBody)
		})
	}
}
