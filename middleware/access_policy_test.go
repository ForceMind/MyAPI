package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/ForceMind/MyAPI/setting"
	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setAccessPolicyModeForMiddlewareTest(t *testing.T, mode string, groups []string) {
	t.Helper()
	require.NoError(t, i18n.Init())
	previous := setting.GetAccessPolicyModeSetting()
	previousGroups, err := common.Marshal(previous.EnforceGroups)
	require.NoError(t, err)

	update := func(nextMode string, nextGroups string) {
		require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
			"access_policy_mode_setting.mode":           nextMode,
			"access_policy_mode_setting.enforce_groups": nextGroups,
		}))
	}
	nextGroups, err := common.Marshal(groups)
	require.NoError(t, err)
	update(mode, string(nextGroups))
	t.Cleanup(func() {
		update(previous.Mode, string(previousGroups))
	})
}

func TestAccessPolicyEvaluationErrorRejectsWithinEnforceScope(t *testing.T) {
	setAccessPolicyModeForMiddlewareTest(t, setting.AccessPolicyModeEnforce, []string{"default"})

	previousGinMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(previousGinMode) })
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	common.SetContextKey(context, constant.ContextKeyAccountTierID, "standard")
	common.SetContextKey(context, constant.ContextKeyAccessProfileID, "standard")
	common.SetContextKey(context, constant.ContextKeyTokenGroup, "default")

	rejected := applyAccessPolicyDecision(context, "default", "invalid\nmodel")

	assert.True(t, rejected)
	assert.True(t, context.IsAborted())
	assert.Equal(t, http.StatusForbidden, recorder.Code)
}

func TestAccessPolicyEvaluationErrorRemainsAuditOnlyOutsideEnforceScope(t *testing.T) {
	setAccessPolicyModeForMiddlewareTest(t, setting.AccessPolicyModeEnforce, []string{"priority"})

	previousGinMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(previousGinMode) })
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	common.SetContextKey(context, constant.ContextKeyAccountTierID, "standard")
	common.SetContextKey(context, constant.ContextKeyAccessProfileID, "standard")
	common.SetContextKey(context, constant.ContextKeyTokenGroup, "default")

	rejected := applyAccessPolicyDecision(context, "default", "invalid\nmodel")

	assert.False(t, rejected)
	assert.False(t, context.IsAborted())
	assert.Equal(t, http.StatusOK, recorder.Code)
}

func TestEnforceAccessPolicyForSelectedGroupReturnsNonRetryableForbidden(t *testing.T) {
	setAccessPolicyModeForMiddlewareTest(t, setting.AccessPolicyModeEnforce, []string{"default"})

	previousGinMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(previousGinMode) })
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	common.SetContextKey(context, constant.ContextKeyAccountTierID, "standard")
	common.SetContextKey(context, constant.ContextKeyAccessProfileID, "standard")
	common.SetContextKey(context, constant.ContextKeyTokenGroup, "default")

	policyErr := EnforceAccessPolicyForSelectedGroup(context, "default", "invalid\nmodel")

	require.NotNil(t, policyErr)
	assert.Equal(t, http.StatusForbidden, policyErr.StatusCode)
	assert.Equal(t, types.ErrorCodeAccessDenied, policyErr.GetErrorCode())
	assert.True(t, types.IsSkipRetryError(policyErr))
	assert.False(t, context.IsAborted(), "controller retry paths own their protocol response")
}
