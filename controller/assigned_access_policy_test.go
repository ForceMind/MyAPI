package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/ForceMind/MyAPI/service"
	"github.com/ForceMind/MyAPI/setting"
	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func assignedPolicyControllerFixture(t *testing.T) (*gorm.DB, *model.User, *model.Token) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	conn, err := db.DB()
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)
	previous, previousLog, previousRedis := model.DB, model.LOG_DB, common.RedisEnabled
	model.DB = db
	model.LOG_DB = db
	common.RedisEnabled = false
	model.InitColumnNamesForTest()
	t.Cleanup(func() {
		model.DB = previous
		model.LOG_DB = previousLog
		common.RedisEnabled = previousRedis
		_ = conn.Close()
	})
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.AssignedAccessPolicy{}, &model.Channel{}, &model.Ability{}, &model.Log{}))
	user := &model.User{Username: "assignment-user", AffCode: "assignment-user", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AccountTierID: "standard"}
	require.NoError(t, db.Create(user).Error)
	require.NoError(t, db.Create(&model.User{Id: 99, Username: "assignment-root", AffCode: "assignment-root", Role: common.RoleRootUser, Status: common.UserStatusEnabled, Group: "default"}).Error)
	token := &model.Token{UserId: user.Id, Key: "synthetic-owner-key", Name: "Owned key", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true, Group: "default", AccessProfileID: "standard"}
	require.NoError(t, db.Create(token).Error)
	mapping := `{"alias":"private-upstream-target"}`
	require.NoError(t, db.Create(&model.Channel{Id: 81, Type: constant.ChannelTypeOpenAI, Name: "private-channel-name", Key: "synthetic-preview-key", Models: "alias", Group: "default", Status: common.ChannelStatusEnabled, ModelMapping: &mapping}).Error)
	require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "alias", ChannelId: 81, Enabled: true}).Error)
	return db, user, token
}

func assignedPolicyControllerContext(subject string, id, userID, role int, method, body string) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(method, "/api/access-policy/"+subject+"/"+strconv.Itoa(id), strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "subject", Value: subject}, {Key: "id", Value: strconv.Itoa(id)}}
	c.Set("id", userID)
	c.Set("role", role)
	return c, recorder
}

func TestAssignedPolicyAdminCASPreviewAndOwnerIsolation(t *testing.T) {
	db, user, token := assignedPolicyControllerFixture(t)
	c, recorder := assignedPolicyControllerContext("token", token.Id, user.Id, common.RoleCommonUser, http.MethodPut, `{"expected_revision":0,"enabled":true,"public_models":[]}`)
	UpdateAssignedAccessPolicy(c)
	assert.Equal(t, http.StatusForbidden, recorder.Code)
	c, recorder = assignedPolicyControllerContext("token", token.Id, 99, common.RoleRootUser, http.MethodPut, `{"expected_revision":0,"enabled":true,"public_models":["alias"],"upstream_models":[],"channel_ids":[81]}`)
	UpdateAssignedAccessPolicy(c)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.Contains(t, recorder.Body.String(), `"owner_user_id":`+strconv.Itoa(user.Id))
	c, recorder = assignedPolicyControllerContext("token", token.Id, 99, common.RoleRootUser, http.MethodPut, `{"expected_revision":0,"enabled":true}`)
	UpdateAssignedAccessPolicy(c)
	assert.Equal(t, http.StatusConflict, recorder.Code)
	c, recorder = assignedPolicyControllerContext("token", token.Id, 99, common.RoleRootUser, http.MethodPost, `{"enabled":true,"public_models":["alias"],"upstream_models":["private-upstream-target"]}`)
	PreviewAssignedAccessPolicy(c)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.Contains(t, recorder.Body.String(), `"allowed":true`)
	row, err := model.ReadAssignedAccessPolicy(context.Background(), db, "token", token.Id)
	require.NoError(t, err)
	assert.EqualValues(t, 1, row.Revision)
	assert.Contains(t, row.PolicyJSON, `"upstream_models":[]`)
	c, recorder = assignedPolicyControllerContext("token", token.Id, user.Id, common.RoleCommonUser, http.MethodGet, "")
	GetTokenAssignedAccess(c)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.Contains(t, recorder.Body.String(), "upstream_model_denied")
	assert.NotContains(t, recorder.Body.String(), "private-upstream-target")
	assert.NotContains(t, recorder.Body.String(), "private-channel-name")
	assert.NotContains(t, recorder.Body.String(), "channel_ids")
	c, recorder = assignedPolicyControllerContext("token", token.Id, user.Id+1, common.RoleCommonUser, http.MethodGet, "")
	GetTokenAssignedAccess(c)
	assert.Equal(t, http.StatusNotFound, recorder.Code)
}

func TestAssignedPolicyOwnerGenericKeyEditCannotEraseAssignment(t *testing.T) {
	db, user, token := assignedPolicyControllerFixture(t)
	_, err := model.ConfigureAssignedAccessPolicy(context.Background(), db, "token", token.Id, 0, `{"enabled":false,"public_models":null,"upstream_models":null,"channel_ids":null}`)
	require.NoError(t, err)
	c, recorder := assignedPolicyControllerContext("token", token.Id, user.Id, common.RoleCommonUser, http.MethodPut, `{"id":`+strconv.Itoa(token.Id)+`,"name":"Changed name","group":"default","access_profile_id":"standard","unlimited_quota":true,"expired_time":-1,"assigned":false,"public_models":null}`)
	UpdateToken(c)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	view, err := service.ReadAssignedAccessPolicy(context.Background(), "token", token.Id)
	require.NoError(t, err)
	assert.True(t, view.Assigned)
	assert.False(t, view.Enabled)
	assert.EqualValues(t, 1, view.Revision)
}

func TestAssignedPolicyAdminCannotManagePeerAndRemovalIsCAS(t *testing.T) {
	db, user, _ := assignedPolicyControllerFixture(t)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", user.Id).Update("role", common.RoleAdminUser).Error)
	c, recorder := assignedPolicyControllerContext("user", user.Id, 99, common.RoleAdminUser, http.MethodPut, `{"expected_revision":0,"enabled":false}`)
	UpdateAssignedAccessPolicy(c)
	assert.Equal(t, http.StatusForbidden, recorder.Code)
	c, recorder = assignedPolicyControllerContext("user", user.Id, 99, common.RoleRootUser, http.MethodPut, `{"expected_revision":0,"enabled":false}`)
	UpdateAssignedAccessPolicy(c)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	c, recorder = assignedPolicyControllerContext("user", user.Id, 99, common.RoleRootUser, http.MethodDelete, `{"expected_revision":1}`)
	DeleteAssignedAccessPolicy(c)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.Contains(t, recorder.Body.String(), `"assigned":false`)
	c, recorder = assignedPolicyControllerContext("user", user.Id, 99, common.RoleRootUser, http.MethodPut, `{"expected_revision":0,"enabled":true}`)
	UpdateAssignedAccessPolicy(c)
	assert.Equal(t, http.StatusConflict, recorder.Code)
}

func TestAssignedPolicyModelsHideDeniedAliasAndRejectRevokedToken(t *testing.T) {
	db, user, token := assignedPolicyControllerFixture(t)
	_, err := model.ConfigureAssignedAccessPolicy(context.Background(), db, "token", token.Id, 0, `{"enabled":true,"public_models":["alias"],"upstream_models":[],"channel_ids":null}`)
	require.NoError(t, err)
	c, _ := assignedPolicyControllerContext("token", token.Id, user.Id, common.RoleCommonUser, http.MethodGet, "")
	c.Set("token_id", token.Id)
	c.Set("token_key", token.Key)
	names, decisions, err := filterModelsByAssignedAccess(c, []string{"default"}, []string{"alias"})
	require.NoError(t, err)
	assert.Empty(t, names)
	require.Len(t, decisions, 1)
	assert.Contains(t, decisions[0].Reasons, "upstream_model_denied")
	require.NoError(t, db.Model(&model.Token{}).Where("id = ?", token.Id).Update("status", common.TokenStatusDisabled).Error)
	_, _, err = filterModelsByAssignedAccess(c, []string{"default"}, []string{"alias"})
	assert.ErrorIs(t, err, service.ErrAssignedAccessDenied)
}

func TestAssignedPolicyDiscoveryRechecksGroupAndAuthenticatedKey(t *testing.T) {
	db, user, token := assignedPolicyControllerFixture(t)
	require.NoError(t, db.Create(&model.Ability{Group: "vip", Model: "vip-only", ChannelId: 81, Enabled: true}).Error)
	c, _ := assignedPolicyControllerContext("token", token.Id, user.Id, common.RoleCommonUser, http.MethodGet, "")
	c.Set("token_id", token.Id)
	c.Set("token_key", token.Key)
	// Simulate cached auth from before a Key moved from vip to default. Current
	// DB group and current Key identity govern discovery even without assignment.
	names, _, err := filterModelsByAssignedAccess(c, []string{"vip"}, []string{"vip-only"})
	require.NoError(t, err)
	assert.Empty(t, names)
	require.NoError(t, db.Model(&model.Token{}).Where("id = ?", token.Id).Update("key", "replacement-key").Error)
	_, _, err = filterModelsByAssignedAccess(c, []string{"default"}, []string{"alias"})
	assert.ErrorIs(t, err, service.ErrAssignedAccessDenied)
}

func TestAssignedPolicyUnassignedMetadataKeepsStaticCatalog(t *testing.T) {
	_, user, token := assignedPolicyControllerFixture(t)
	previous := setting.GetAccessPolicyModeSetting()
	groupsJSON, err := common.Marshal(previous.EnforceGroups)
	require.NoError(t, err)
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"access_policy_mode_setting.mode": "off", "access_policy_mode_setting.enforce_groups": ""}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"access_policy_mode_setting.mode": previous.Mode, "access_policy_mode_setting.enforce_groups": string(groupsJSON)}))
	})
	const name = "synthetic-static-catalog-only"
	original, existed := openAIModelsMap[name]
	openAIModelsMap[name] = dto.OpenAIModels{Id: name, Object: "model", OwnedBy: "synthetic-provider"}
	t.Cleanup(func() {
		if existed {
			openAIModelsMap[name] = original
		} else {
			delete(openAIModelsMap, name)
		}
	})
	c, recorder := assignedPolicyControllerContext("token", token.Id, user.Id, common.RoleCommonUser, http.MethodGet, "")
	c.Set("token_id", token.Id)
	c.Set("token_key", token.Key)
	c.Params = gin.Params{{Key: "model", Value: name}}
	RetrieveModel(c, constant.ChannelTypeOpenAI)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var response dto.OpenAIModels
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, name, response.Id)
}

func TestAssignedPolicyMetadataUsesCurrentEnforcementAfterGroupRevocation(t *testing.T) {
	_, user, token := assignedPolicyControllerFixture(t)
	previousMode := setting.GetAccessPolicyModeSetting()
	previousGroups, err := common.Marshal(previousMode.EnforceGroups)
	require.NoError(t, err)
	previousTiers, err := common.Marshal(setting.GetAccessProfileSetting().AccountTiers)
	require.NoError(t, err)
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"access_policy_mode_setting.mode": "enforce", "access_policy_mode_setting.enforce_groups": "default"}))
	require.NoError(t, setting.UpdateAccountTierDefinitionsByJSONString(`{"standard":{"label":"Standard account","enabled":true,"model_allowlist":[]}}`))
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateAccountTierDefinitionsByJSONString(string(previousTiers)))
		require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"access_policy_mode_setting.mode": previousMode.Mode, "access_policy_mode_setting.enforce_groups": string(previousGroups)}))
	})
	c, _ := assignedPolicyControllerContext("token", token.Id, user.Id, common.RoleCommonUser, http.MethodGet, "")
	c.Set("token_id", token.Id)
	c.Set("token_key", token.Key)
	// No assigned envelope exists. Current default-group legacy enforcement must
	// still govern metadata after a stale cached vip group has been revoked.
	names, _, err := projectModelsByAssignedAccess(c, []string{"vip"}, []string{"alias"}, true)
	require.NoError(t, err)
	assert.Empty(t, names)
	names, _, err = projectModelsByAssignedAccess(c, []string{"default"}, []string{"alias"}, true)
	require.NoError(t, err)
	assert.Empty(t, names)
}

func TestAssignedPolicyMutationRejectsDuplicateAndCaseVariantFields(t *testing.T) {
	for _, test := range []struct {
		name, method, body string
		handler            func(*gin.Context)
	}{
		{"put_duplicate_enabled", http.MethodPut, `{"expected_revision":0,"enabled":false,"enabled":true}`, UpdateAssignedAccessPolicy},
		{"put_duplicate_revision", http.MethodPut, `{"expected_revision":1,"expected_revision":0,"enabled":true}`, UpdateAssignedAccessPolicy},
		{"put_enabled_alias", http.MethodPut, `{"expected_revision":0,"enabled":false,"Enabled":true}`, UpdateAssignedAccessPolicy},
		{"preview_duplicate_enabled", http.MethodPost, `{"enabled":false,"enabled":true}`, PreviewAssignedAccessPolicy},
		{"preview_escaped_duplicate", http.MethodPost, `{"enabled":false,"\u0065nabled":true}`, PreviewAssignedAccessPolicy},
		{"preview_revision_alias", http.MethodPost, `{"enabled":true,"Expected_Revision":0}`, PreviewAssignedAccessPolicy},
		{"delete_duplicate_revision", http.MethodDelete, `{"expected_revision":1,"expected_revision":0}`, DeleteAssignedAccessPolicy},
		{"delete_revision_alias", http.MethodDelete, `{"Expected_Revision":0}`, DeleteAssignedAccessPolicy},
	} {
		t.Run(test.name, func(t *testing.T) {
			db, _, token := assignedPolicyControllerFixture(t)
			c, recorder := assignedPolicyControllerContext("token", token.Id, 99, common.RoleRootUser, test.method, test.body)
			test.handler(c)
			require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
			row, err := model.ReadAssignedAccessPolicy(context.Background(), db, "token", token.Id)
			require.NoError(t, err)
			assert.False(t, row.Assigned)
			assert.Zero(t, row.Revision)
		})
	}
}
