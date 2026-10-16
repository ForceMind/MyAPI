package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func tokenModelLimitsRequestForTest(actor int, method, pathID, query, body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("id", actor)
	c.Params = gin.Params{{Key: "id", Value: pathID}}
	c.Request = httptest.NewRequest(method, "/api/token/"+pathID+"/model-limits"+query, strings.NewReader(body))
	if method == http.MethodGet {
		GetTokenModelLimits(c)
	} else {
		UpdateTokenModelLimits(c)
	}
	return recorder
}

func TestTokenModelLimitsAPIReadWriteOwnershipAndPrivacy(t *testing.T) {
	db := subscriptionShareControllerFixture(t)
	oldRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = oldRedis })
	require.NoError(t, db.Model(&model.Token{}).Where("id = ?", 11).Updates(map[string]any{"name": "Keep me", "remain_quota": 1<<54 + 7, "used_quota": 1<<54 + 9, "quota_version": 5, "group": "preserved-group", "access_profile_id": "preserved-profile", "allow_ips": "127.0.0.1"}).Error)
	response := tokenModelLimitsRequestForTest(2, http.MethodGet, "11", "", "")
	require.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, "no-store", response.Header().Get("Cache-Control"))
	var envelope struct {
		Success bool                       `json:"success"`
		Data    model.TokenModelLimitsView `json:"data"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &envelope))
	require.True(t, envelope.Success)
	assert.Equal(t, 11, envelope.Data.TokenID)
	assert.Equal(t, "Keep me", envelope.Data.TokenName)
	assert.Len(t, envelope.Data.Revision, 64)
	assert.Contains(t, response.Body.String(), `"models":[]`)
	for _, private := range []string{"share-api-key", "remain_quota", "used_quota", "quota_version", "127.0.0.1", "preserved-profile", "user_id", `"key":`} {
		assert.NotContains(t, response.Body.String(), private)
	}
	var before model.Token
	require.NoError(t, db.First(&before, 11).Error)
	body := fmt.Sprintf(`{"expected_revision":%q,"model_limits_enabled":true,"models":["ModelA","Retired/Model"]}`, envelope.Data.Revision)
	response = tokenModelLimitsRequestForTest(2, http.MethodPut, "11", "", body)
	require.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, "no-store", response.Header().Get("Cache-Control"))
	assert.NotContains(t, response.Body.String(), "share-api-key")
	var after model.Token
	require.NoError(t, db.First(&after, 11).Error)
	assert.True(t, after.ModelLimitsEnabled)
	assert.Equal(t, "ModelA,Retired/Model", after.ModelLimits)
	after.ModelLimitsEnabled, after.ModelLimits = before.ModelLimitsEnabled, before.ModelLimits
	assert.Equal(t, before, after)
	legacyRecorder := httptest.NewRecorder()
	legacyContext, _ := gin.CreateTestContext(legacyRecorder)
	legacyContext.Set("id", 2)
	legacyContext.Request = httptest.NewRequest(http.MethodPut, "/api/token/", strings.NewReader(`{"id":11,"name":"Metadata-only rename","expired_time":-1,"unlimited_quota":false,"group":"preserved-group","access_profile_id":"preserved-profile","allow_ips":"127.0.0.1"}`))
	UpdateToken(legacyContext)
	require.Equal(t, http.StatusOK, legacyRecorder.Code)
	var legacyResult struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(legacyRecorder.Body.Bytes(), &legacyResult))
	require.True(t, legacyResult.Success)
	require.NoError(t, db.First(&after, 11).Error)
	assert.True(t, after.ModelLimitsEnabled, "a general metadata edit omitting model scope does not clear it")
	assert.Equal(t, "ModelA,Retired/Model", after.ModelLimits)
	assert.Equal(t, before.RemainQuota, after.RemainQuota)
	assert.Equal(t, before.UsedQuota, after.UsedQuota)
	assert.Equal(t, before.QuotaVersion, after.QuotaVersion)
	response = tokenModelLimitsRequestForTest(2, http.MethodPut, "11", "", body)
	assert.Equal(t, http.StatusConflict, response.Code, "an old revision never overwrites the current scope")
	assert.Contains(t, response.Body.String(), "token_model_limits_conflict")
	for _, actor := range []int{1, 3} {
		response = tokenModelLimitsRequestForTest(actor, http.MethodGet, "11", "", "")
		assert.Equal(t, http.StatusNotFound, response.Code)
		response = tokenModelLimitsRequestForTest(actor, http.MethodPut, "11", "", body)
		assert.Equal(t, http.StatusNotFound, response.Code)
	}
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", 2).Update("status", common.UserStatusDisabled).Error)
	response = tokenModelLimitsRequestForTest(2, http.MethodGet, "11", "", "")
	assert.Equal(t, http.StatusForbidden, response.Code)
}

func TestTokenModelLimitsAPIRejectsAmbiguousBodiesWithoutChangingState(t *testing.T) {
	db := subscriptionShareControllerFixture(t)
	oldRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = oldRedis })
	view, err := model.ReadTokenModelLimits(t.Context(), 2, 11)
	require.NoError(t, err)
	var before model.Token
	require.NoError(t, db.First(&before, 11).Error)
	for _, body := range []string{
		`{}`,
		fmt.Sprintf(`{"expected_revision":%q,"model_limits_enabled":null,"models":[]}`, view.Revision),
		fmt.Sprintf(`{"expected_revision":%q,"model_limits_enabled":false,"models":null}`, view.Revision),
		fmt.Sprintf(`{"expected_revision":%q,"model_limits_enabled":false,"models":[] ,"remain_quota":10}`, view.Revision),
		fmt.Sprintf(`{"expected_revision":%q,"model_limits_enabled":false,"models":[] ,"id":12}`, view.Revision),
		fmt.Sprintf(`{"expected_revision":%q,"model_limits_enabled":false,"models":[],"Models":["other"]}`, view.Revision),
		fmt.Sprintf(`{"expected_revision":%q,"model_limits_enabled":false,"models":[],"models":["other"]}`, view.Revision),
		fmt.Sprintf(`{"Expected_Revision":%q,"model_limits_enabled":false,"models":[]}`, view.Revision),
		fmt.Sprintf(`{"expected_revision":%q,"model_limits_enabled":true,"models":["a,b"]}`, view.Revision),
		fmt.Sprintf(`{"expected_revision":%q,"model_limits_enabled":true,"models":["a","a"]}`, view.Revision),
		fmt.Sprintf(`{"expected_revision":%q,"model_limits_enabled":true,"models":[""]}`, view.Revision),
		fmt.Sprintf(`{"expected_revision":%q,"model_limits_enabled":false,"models":[]} {}`, view.Revision),
	} {
		response := tokenModelLimitsRequestForTest(2, http.MethodPut, "11", "", body)
		assert.Equal(t, http.StatusBadRequest, response.Code, body)
		assert.Equal(t, "no-store", response.Header().Get("Cache-Control"))
		var after model.Token
		require.NoError(t, db.First(&after, 11).Error)
		assert.Equal(t, before, after)
	}
	for _, path := range []string{"0", "-1", "not-an-id"} {
		response := tokenModelLimitsRequestForTest(2, http.MethodGet, path, "", "")
		assert.Equal(t, http.StatusBadRequest, response.Code)
	}
	response := tokenModelLimitsRequestForTest(2, http.MethodGet, "11", "?group=other", "")
	assert.Equal(t, http.StatusBadRequest, response.Code)
}

func TestTokenModelLimitsConcurrentChangeSurvivesLegacyPartialUpdates(t *testing.T) {
	for _, test := range []struct {
		name        string
		body        string
		statusOnly  bool
		oldEnabled  bool
		wantEnabled bool
		wantModels  string
	}{
		{"status only", `{"id":11,"status":2}`, true, false, true, "ConcurrentModel"},
		{"enabled only", `{"id":11,"name":"edit","expired_time":-1,"model_limits_enabled":false}`, false, true, false, "ConcurrentModel"},
		{"list only", `{"id":11,"name":"edit","expired_time":-1,"model_limits":"ChosenModel"}`, false, false, true, "ChosenModel"},
	} {
		t.Run(test.name, func(t *testing.T) {
			db := subscriptionShareControllerFixture(t)
			oldRedis := common.RedisEnabled
			common.RedisEnabled = false
			t.Cleanup(func() { common.RedisEnabled = oldRedis })
			require.NoError(t, db.Model(&model.Token{}).Where("id = ?", 11).Updates(map[string]any{"name": "Old name", "model_limits_enabled": test.oldEnabled, "model_limits": "OldModel"}).Error)
			interleaved := false
			const callbackName = "test:model-scope-interleaving"
			require.NoError(t, db.Callback().Query().After("gorm:query").Register(callbackName, func(tx *gorm.DB) {
				if interleaved || tx.Statement.Schema == nil || tx.Statement.Schema.Name != "Token" {
					return
				}
				interleaved = true
				// Commit after the handler captured its old token, before its write.
				require.NoError(t, db.Model(&model.Token{}).Where("id = ?", 11).Updates(map[string]any{"name": "Concurrent name", "model_limits_enabled": true, "model_limits": "ConcurrentModel"}).Error)
			}))
			t.Cleanup(func() { require.NoError(t, db.Callback().Query().Remove(callbackName)) })
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Set("id", 2)
			path := "/api/token/"
			if test.statusOnly {
				path += "?status_only=true"
			}
			c.Request = httptest.NewRequest(http.MethodPut, path, strings.NewReader(test.body))
			UpdateToken(c)
			require.True(t, interleaved)
			require.Equal(t, http.StatusOK, recorder.Code)
			var result struct{ Success bool }
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &result))
			require.True(t, result.Success)
			var current model.Token
			require.NoError(t, db.First(&current, 11).Error)
			assert.Equal(t, test.wantEnabled, current.ModelLimitsEnabled)
			assert.Equal(t, test.wantModels, current.ModelLimits)
			if test.statusOnly {
				assert.Equal(t, common.TokenStatusDisabled, current.Status)
				assert.Equal(t, "Concurrent name", current.Name)
			}
		})
	}
}
