package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOptionRemediateDryRunRejectsInvalidInputBeforeReading(t *testing.T) {
	queryReads := optionDiagnosticsControllerFixture(t)
	for _, testCase := range []struct {
		name string
		body string
	}{
		{"malformed json", `{"keys": [`},
		{"trailing json", `{"include_all": true} {}`},
		{"unknown field", `{"include_all": true, "bogus": 1}`},
		{"keys and include all", `{"keys": ["passkey.enabled"], "include_all": true}`},
		{"empty request", `{}`},
		{"empty keys", `{"keys": []}`},
		{"too many keys", `{"keys": [` + strings.TrimSuffix(strings.Repeat(`"k",`, 65), ",") + `]}`},
		{"duplicate keys", `{"keys": ["passkey.enabled", "passkey.enabled"]}`},
		{"empty key", `{"keys": [""]}`},
		{"unknown action", `{"keys": ["passkey.enabled"], "actions": ["drop.table"]}`},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			context.Request = httptest.NewRequest(http.MethodPost, "/api/option/diagnostics/remediate/dry-run", strings.NewReader(testCase.body))
			OptionRemediateDryRun(context)

			var response struct {
				Success bool   `json:"success"`
				Message string `json:"message"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			assert.False(t, response.Success)
			assert.Equal(t, i18n.MsgInvalidParams, response.Message)
		})
	}
	assert.Zero(t, *queryReads, "invalid input must be rejected before any database read")
}

func TestOptionRemediateApplyRejectsInvalidShape(t *testing.T) {
	queryReads := optionDiagnosticsControllerFixture(t)
	validHash := model.OptionRemediationHashForPreview("x")
	for _, testCase := range []struct{ name, body string }{
		{"empty items", `{"items": []}`},
		{"unknown action", `{"items": [{"key": "passkey.enabled", "action": "drop.table", "expected_old_value_hash": "` + validHash + `", "expected_new_value_hash": "` + validHash + `"}]}`},
		{"malformed hash", `{"items": [{"key": "passkey.enabled", "action": "value.trim_space", "expected_old_value_hash": "md5:x", "expected_new_value_hash": "` + validHash + `"}]}`},
		{"non actionable key", `{"items": [{"key": "unknown.prefix", "action": "value.trim_space", "expected_old_value_hash": "` + validHash + `", "expected_new_value_hash": "` + validHash + `"}]}`},
		{"unknown field", `{"items": [], "bogus": 1}`},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			context.Request = httptest.NewRequest(http.MethodPost, "/api/option/diagnostics/remediate/apply", strings.NewReader(testCase.body))
			OptionRemediateApply(context)

			var response struct {
				Success bool   `json:"success"`
				Message string `json:"message"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			assert.False(t, response.Success)
			assert.Equal(t, i18n.MsgInvalidParams, response.Message)
		})
	}
	assert.Zero(t, *queryReads, "invalid apply shapes must be rejected before any database read")
}

func TestOptionRemediateDryRunAndApplyRoundTrip(t *testing.T) {
	optionDiagnosticsControllerFixture(t)
	previousRedisEnabled, previousMemoryCache := common.RedisEnabled, common.MemoryCacheEnabled
	common.RedisEnabled = false
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		common.RedisEnabled = previousRedisEnabled
		common.MemoryCacheEnabled = previousMemoryCache
	})
	require.NoError(t, model.DB.Exec("CREATE TABLE options (`key` text primary key, value text)").Error)
	require.NoError(t, model.DB.AutoMigrate(&model.OptionRemediation{}, &model.OptionRemediationBackup{}))
	const canary = "c09-remediation-controller-canary"
	require.NoError(t, model.DB.Create(&model.Option{Key: "GroupRatio", Value: `{"default":1}`}).Error)
	require.NoError(t, model.DB.Create(&model.Option{Key: "group_ratio_setting.group_ratio", Value: `{"default":2}`}).Error)
	require.NoError(t, model.DB.Create(&model.Option{Key: "unknown.prefix", Value: canary}).Error)

	previousGroupRatio := ratio_setting.GroupRatio2JSONString()
	optionMapBefore := snapshotOptionRemediationControllerOptionMap()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(previousGroupRatio))
		restoreOptionRemediationControllerOptionMap(optionMapBefore)
	})
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = map[string]string{}
	}
	common.OptionMapRWMutex.Unlock()

	dryRunRecorder := httptest.NewRecorder()
	dryRunContext, _ := gin.CreateTestContext(dryRunRecorder)
	dryRunContext.Request = httptest.NewRequest(http.MethodPost, "/api/option/diagnostics/remediate/dry-run", strings.NewReader(`{"keys": ["GroupRatio"]}`))
	OptionRemediateDryRun(dryRunContext)

	var dryRunResponse struct {
		Success bool `json:"success"`
		Data    struct {
			Items []struct {
				Key                  string `json:"key"`
				Action               string `json:"action"`
				Decision             string `json:"decision"`
				OldValueHash         string `json:"old_value_hash"`
				NewValueHash         string `json:"new_value_hash"`
			} `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(dryRunRecorder.Body.Bytes(), &dryRunResponse))
	require.True(t, dryRunResponse.Success)
	assert.NotContains(t, dryRunRecorder.Body.String(), canary)
	assert.NotContains(t, dryRunRecorder.Body.String(), `{"default":1}`)

	var applyItemBody string
	for _, item := range dryRunResponse.Data.Items {
		if item.Decision != model.OptionRemediationDecisionRepair {
			continue
		}
		applyItemBody = `{"items": [{"key": "` + item.Key + `", "action": "` + item.Action + `", "expected_old_value_hash": "` + item.OldValueHash + `", "expected_new_value_hash": "` + item.NewValueHash + `"}]}`
	}
	require.NotEmpty(t, applyItemBody)

	applyRecorder := httptest.NewRecorder()
	applyContext, _ := gin.CreateTestContext(applyRecorder)
	applyContext.Request = httptest.NewRequest(http.MethodPost, "/api/option/diagnostics/remediate/apply", strings.NewReader(applyItemBody))
	OptionRemediateApply(applyContext)

	var applyResponse struct {
		Success bool `json:"success"`
		Data    struct {
			Summary struct {
				Applied int `json:"applied"`
			} `json:"summary"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(applyRecorder.Body.Bytes(), &applyResponse))
	require.True(t, applyResponse.Success)
	assert.Equal(t, 1, applyResponse.Data.Summary.Applied)
	assert.NotContains(t, applyRecorder.Body.String(), `{"default":1}`)

	// The registry is served through the paginated history endpoint without
	// ever carrying an option value.
	historyRecorder := httptest.NewRecorder()
	historyContext, _ := gin.CreateTestContext(historyRecorder)
	historyContext.Request = httptest.NewRequest(http.MethodGet, "/api/option/diagnostics/remediations?page=1&page_size=10", nil)
	GetOptionRemediations(historyContext)
	assert.Contains(t, historyRecorder.Body.String(), `"status":"applied"`)
	assert.NotContains(t, historyRecorder.Body.String(), `{"default":1}`)

	assert.Equal(t, `{"default":1}`, ratio_setting.GroupRatio2JSONString())
}

func snapshotOptionRemediationControllerOptionMap() map[string]string {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	if common.OptionMap == nil {
		return nil
	}
	snapshot := make(map[string]string, len(common.OptionMap))
	for key, value := range common.OptionMap {
		snapshot[key] = value
	}
	return snapshot
}

func restoreOptionRemediationControllerOptionMap(snapshot map[string]string) {
	common.OptionMapRWMutex.Lock()
	defer common.OptionMapRWMutex.Unlock()
	common.OptionMap = snapshot
}
