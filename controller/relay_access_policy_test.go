package controller

import (
	"errors"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/model"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/ForceMind/MyAPI/service"
	"github.com/ForceMind/MyAPI/setting"
	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/ForceMind/MyAPI/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestGetChannelRejectsAutomaticRetryOutsideScopedAccessPolicy(t *testing.T) {
	require.NoError(t, i18n.Init())
	previousDB := model.DB
	previousMemoryCache := common.MemoryCacheEnabled
	previousRetryTimes := common.RetryTimes
	originalRegistry := setting.GetAccessProfileSetting()
	originalProfiles, err := common.Marshal(originalRegistry.Profiles)
	require.NoError(t, err)
	originalTiers, err := common.Marshal(originalRegistry.AccountTiers)
	require.NoError(t, err)
	originalMode := setting.GetAccessPolicyModeSetting()
	originalScope, err := common.Marshal(originalMode.EnforceGroups)
	require.NoError(t, err)
	originalUsableGroups := setting.UserUsableGroups2JSONString()
	originalGroupRatios := ratio_setting.GroupRatio2JSONString()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}))
	model.DB = db
	common.MemoryCacheEnabled = false
	common.RetryTimes = 0
	t.Cleanup(func() {
		model.DB = previousDB
		common.MemoryCacheEnabled = previousMemoryCache
		common.RetryTimes = previousRetryTimes
		require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
			"access_profile_setting.profiles":           string(originalProfiles),
			"access_profile_setting.account_tiers":      string(originalTiers),
			"access_policy_mode_setting.mode":           originalMode.Mode,
			"access_policy_mode_setting.enforce_groups": string(originalScope),
		}))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUsableGroups))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalGroupRatios))
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"access_profile_setting.profiles":           `{"standard":{"label":"Standard","route_groups":["group-a"]}}`,
		"access_profile_setting.account_tiers":      `{"standard":{"label":"Standard","route_groups":["group-a"]}}`,
		"access_policy_mode_setting.mode":           setting.AccessPolicyModeEnforce,
		"access_policy_mode_setting.enforce_groups": `["group-a","group-b"]`,
	}))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"group-a":"Group A","group-b":"Group B"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"group-a":1,"group-b":1}`))

	priority := int64(0)
	weight := uint(1)
	for index, group := range []string{"group-a", "group-b"} {
		channel := model.Channel{
			Id:       index + 1,
			Type:     constant.ChannelTypeOpenAI,
			Name:     fmt.Sprintf("policy-%s", group),
			Key:      fmt.Sprintf("key-%s", group),
			Status:   common.ChannelStatusEnabled,
			Group:    group,
			Models:   "policy-model",
			Priority: &priority,
			Weight:   &weight,
		}
		require.NoError(t, db.Create(&channel).Error)
		require.NoError(t, db.Create(&model.Ability{Group: group, Model: "policy-model", ChannelId: channel.Id, Enabled: true, Priority: &priority, Weight: weight}).Error)
	}

	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	common.SetContextKey(context, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(context, constant.ContextKeyTokenAutoGroups, []string{"group-a", "group-b"})
	common.SetContextKey(context, constant.ContextKeyTokenCrossGroupRetry, true)
	common.SetContextKey(context, constant.ContextKeyAccountTierID, "standard")
	common.SetContextKey(context, constant.ContextKeyAccessProfileID, "standard")
	common.SetContextKey(context, constant.ContextKeyTokenGroup, "auto")

	retry := 0
	retryParam := &service.RetryParam{
		Ctx: context, TokenGroup: "auto", ModelName: "policy-model", RequestPath: "/v1/chat/completions", Retry: &retry,
	}
	relayInfo := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}, TokenGroup: "auto", OriginModelName: "policy-model"}

	first, firstErr := getChannel(context, relayInfo, retryParam)
	require.Nil(t, firstErr)
	require.NotNil(t, first)
	assert.Equal(t, "group-a", common.GetContextKeyString(context, constant.ContextKeyAutoGroup))
	retryParam.IncreaseRetry()

	second, secondErr := getChannel(context, relayInfo, retryParam)
	require.Nil(t, second)
	require.NotNil(t, secondErr)
	assert.Equal(t, 403, secondErr.StatusCode)
	assert.Equal(t, types.ErrorCodeAccessDenied, secondErr.GetErrorCode())
	assert.True(t, types.IsSkipRetryError(secondErr))
	assert.Equal(t, first.Id, common.GetContextKeyInt(context, constant.ContextKeyChannelId), "the denied retry must not replace the first selected channel or contact its upstream")
	assert.Equal(t, "group-b", common.GetContextKeyString(context, constant.ContextKeyAutoGroup))
}

func TestTaskChannelSelectionErrorPreservesAccessPolicyForbidden(t *testing.T) {
	policyErr := types.NewError(
		errors.New("policy denied"),
		types.ErrorCodeAccessDenied,
		types.ErrOptionWithStatusCode(403),
		types.ErrOptionWithSkipRetry(),
	)

	taskErr := taskChannelSelectionError(policyErr)

	require.NotNil(t, taskErr)
	assert.Equal(t, 403, taskErr.StatusCode)
	assert.Equal(t, "access_policy_denied", taskErr.Code)
}
