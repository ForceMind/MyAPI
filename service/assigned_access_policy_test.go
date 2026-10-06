package service

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/model"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/service/accesspolicy"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func assignedAccessFixture(t *testing.T) (*gorm.DB, *model.User, *model.Token, *model.Channel, *gin.Context, *relaycommon.RelayInfo, *http.Request) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	prior, redisEnabled := model.DB, common.RedisEnabled
	memoryEnabled, cooldown := common.MemoryCacheEnabled, common.RelayFailureCooldownSeconds
	common.MemoryCacheEnabled = false
	common.RelayFailureCooldownSeconds = 0
	model.DB = db
	common.RedisEnabled = false
	model.InitColumnNamesForTest()
	t.Cleanup(func() {
		model.DB = prior
		common.RedisEnabled = redisEnabled
		common.MemoryCacheEnabled = memoryEnabled
		common.RelayFailureCooldownSeconds = cooldown
		_ = sqlDB.Close()
	})
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Channel{}, &model.Ability{}, &model.AssignedAccessPolicy{}, &model.QuotaWriterEpoch{}))
	require.NoError(t, model.EnsureQuotaWriterEpochStateWithDB(db))
	user := &model.User{Username: "policy-owner", Group: "default", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, AccountTierID: "standard"}
	require.NoError(t, db.Create(user).Error)
	token := &model.Token{UserId: user.Id, Key: "synthetic-assigned-key", Name: "Policy key", Group: "default", AccessProfileID: "standard", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}
	require.NoError(t, db.Create(token).Error)
	mapping := `{"public-alias":"actual-target"}`
	channel := &model.Channel{Type: constant.ChannelTypeOpenAI, Name: "Assigned channel", Status: common.ChannelStatusEnabled, Models: "public-alias,other-public", Group: "default", ModelMapping: &mapping, Key: "synthetic-upstream-key"}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Create(&[]model.Ability{{Group: "default", Model: "public-alias", ChannelId: channel.Id, Enabled: true}, {Group: "default", Model: "other-public", ChannelId: channel.Id, Enabled: true}}).Error)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Set("id", user.Id)
	c.Set("token_id", token.Id)
	c.Set("token_key", token.Key)
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
	common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
	c.Set("model_route", map[string]any{"upstream_model": "actual-target"})
	info := &relaycommon.RelayInfo{UserId: user.Id, TokenId: token.Id, TokenKey: token.Key, OriginModelName: "public-alias", ChannelMeta: &relaycommon.ChannelMeta{ChannelId: channel.Id, ChannelType: channel.Type, ChannelBaseUrl: channel.GetBaseURL(), ApiKey: channel.Key, UpstreamModelName: "actual-target"}}
	req, err := http.NewRequest(http.MethodPost, "https://synthetic.invalid/v1/responses", bytes.NewBufferString(`{"model":"actual-target","input":"test"}`))
	require.NoError(t, err)
	return db, user, token, channel, c, info, req
}

func saveAssignedPolicy(t *testing.T, db *gorm.DB, subject string, id int, revision int64, policy accesspolicy.AssignedPolicy) model.AssignedAccessPolicy {
	t.Helper()
	normalized, err := accesspolicy.NormalizeAssignedPolicy(policy)
	require.NoError(t, err)
	raw, err := common.Marshal(normalized)
	require.NoError(t, err)
	row, err := model.ConfigureAssignedAccessPolicy(context.Background(), db, subject, id, revision, string(raw))
	require.NoError(t, err)
	return row
}

func TestAssignedAccessDispatchReloadsConcurrentRevocation(t *testing.T) {
	db, user, token, channel, c, info, req := assignedAccessFixture(t)
	saveAssignedPolicy(t, db, "user", user.Id, 0, accesspolicy.AssignedPolicy{Enabled: true, PublicModels: []string{"public-alias"}, UpstreamModels: []string{"actual-target"}, ChannelIDs: []int{channel.Id}})
	require.NoError(t, ValidateAssignedAccessSelection(c, channel, "public-alias"))
	require.NoError(t, ValidateAssignedAccessDispatch(c, req, info))
	// The request has selected and frozen a permitted route. A separate admin
	// transaction commits before the send checkpoint; no sleeps or timing races.
	revoked := make(chan error, 1)
	go func() {
		_, err := model.ConfigureAssignedAccessPolicy(context.Background(), db, "token", token.Id, 0, `{"enabled":false,"public_models":null,"upstream_models":null,"channel_ids":null}`)
		revoked <- err
	}()
	require.NoError(t, <-revoked)
	assert.ErrorIs(t, ValidateAssignedAccessDispatch(c, req, info), ErrAssignedAccessDenied)
}

func TestAssignedAccessDispatchRejectsWarmStaleTokenCache(t *testing.T) {
	db, _, token, _, c, info, req := assignedAccessFixture(t)
	saveAssignedPolicy(t, db, "token", token.Id, 0, accesspolicy.AssignedPolicy{Enabled: true})
	server := miniredis.RunT(t)
	oldRDB := common.RDB
	common.RDB = redis.NewClient(&redis.Options{Addr: server.Addr()})
	common.RedisEnabled = true
	t.Cleanup(func() { _ = common.RDB.Close(); common.RDB = oldRDB; common.RedisEnabled = false })
	_, err := model.GetTokenByKey(token.Key, false)
	require.NoError(t, err)
	require.NoError(t, db.Model(&model.Token{}).Where("id = ?", token.Id).Update("status", common.TokenStatusDisabled).Error)
	cached, err := model.GetTokenByKey(token.Key, false)
	require.NoError(t, err)
	require.Equal(t, common.TokenStatusEnabled, cached.Status, "fixture must really retain stale cache authorization")
	assert.ErrorIs(t, ValidateAssignedAccessDispatch(c, req, info), ErrAssignedAccessDenied)
}

func TestAssignedAccessIntersectionAliasAndUnsupportedProtocols(t *testing.T) {
	db, user, token, channel, c, info, req := assignedAccessFixture(t)
	saveAssignedPolicy(t, db, "user", user.Id, 0, accesspolicy.AssignedPolicy{Enabled: true, PublicModels: []string{"public-alias"}})
	saveAssignedPolicy(t, db, "token", token.Id, 0, accesspolicy.AssignedPolicy{Enabled: true, UpstreamModels: []string{}})
	assert.ErrorIs(t, ValidateAssignedAccessSelection(c, channel, "public-alias"), ErrAssignedAccessDenied)
	models, _, err := AssignedModelAvailability(context.Background(), user, token, []string{"default"}, []string{"public-alias", "other-public"}, nil)
	require.NoError(t, err)
	require.Len(t, models, 2)
	assert.False(t, models[0].Allowed)
	assert.Contains(t, models[0].Reasons, "upstream_model_denied")
	assert.Contains(t, models[1].Reasons, "public_model_denied")
	saveAssignedPolicy(t, db, "token", token.Id, 1, accesspolicy.AssignedPolicy{Enabled: true, UpstreamModels: []string{"actual-target"}, ChannelIDs: []int{channel.Id}})
	require.NoError(t, ValidateAssignedAccessDispatch(c, req, info))
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/embeddings", nil)
	assert.ErrorIs(t, ValidateAssignedAccessSelection(c, channel, "public-alias"), ErrAssignedAccessDenied)
	assert.ErrorIs(t, ValidateAssignedAccessDispatch(c, req, info), ErrAssignedAccessDenied)
}

func TestAssignedAccessFinalBodyAndChannelRemainBound(t *testing.T) {
	db, _, token, channel, c, info, req := assignedAccessFixture(t)
	saveAssignedPolicy(t, db, "token", token.Id, 0, accesspolicy.AssignedPolicy{Enabled: true, ChannelIDs: []int{channel.Id}})
	require.NoError(t, ValidateAssignedAccessDispatch(c, req, info))
	altered, err := http.NewRequest(http.MethodPost, "https://synthetic.invalid/v1/responses", bytes.NewBufferString(`{"model":"other-target"}`))
	require.NoError(t, err)
	assert.ErrorIs(t, ValidateAssignedAccessDispatch(c, altered, info), ErrAssignedAccessDenied)
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", channel.Id).Update("model_mapping", `{"public-alias":"changed-target"}`).Error)
	assert.ErrorIs(t, ValidateAssignedAccessDispatch(c, req, info), ErrAssignedAccessDenied)
}

func TestAssignedAccessRoutingFiltersBeforePrioritySelection(t *testing.T) {
	db, user, token, channel, c, _, _ := assignedAccessFixture(t)
	saveAssignedPolicy(t, db, "token", token.Id, 0, accesspolicy.AssignedPolicy{Enabled: true, ChannelIDs: []int{channel.Id}})
	ctx, err := assignedAccessRoutingContext(c, context.Background())
	require.NoError(t, err)
	allowed := model.ChannelRoutingCandidate{ChannelID: channel.Id, ChannelType: channel.Type, Priority: 1, ModelRoute: &model.ChannelModelRoute{RequestedModel: "public-alias", UpstreamModel: "actual-target"}}
	denied := allowed
	denied.ChannelID++
	denied.Priority = 100
	filtered := model.BuildChannelRoutingPolicy([]model.ChannelRoutingCandidate{assignedAccessRoutingCandidate(ctx, denied, "/v1/responses"), assignedAccessRoutingCandidate(ctx, allowed, "/v1/responses")})
	id, found, err := model.SelectChannelFromRoutingPolicy(filtered, 0)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, channel.Id, id)
	high := int64(100)
	forbidden := &model.Channel{Type: channel.Type, Name: "Forbidden high priority", Status: common.ChannelStatusEnabled, Models: "public-alias", Group: "default", Key: "synthetic-forbidden", Priority: &high}
	require.NoError(t, db.Create(forbidden).Error)
	require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "public-alias", ChannelId: forbidden.Id, Enabled: true, Priority: &high}).Error)
	selected, group, err := CacheGetRandomSatisfiedChannel(&RetryParam{Ctx: c, TokenGroup: "default", ModelName: "public-alias", RequestPath: "/v1/responses"})
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, "default", group)
	assert.Equal(t, channel.Id, selected.Id)
	models, _, err := AssignedModelAvailability(context.Background(), user, token, []string{"default"}, []string{"public-alias"}, nil)
	require.NoError(t, err)
	require.Len(t, models, 1)
	assert.True(t, models[0].Allowed)
}

func TestAssignedAccessMalformedStoredPolicyFailsClosed(t *testing.T) {
	db, _, token, channel, c, _, _ := assignedAccessFixture(t)
	_, err := model.ConfigureAssignedAccessPolicy(context.Background(), db, "token", token.Id, 0, `{"enabled":true,"public_models":null,"unexpected":"grant"}`)
	require.NoError(t, err)
	assert.Error(t, ValidateAssignedAccessSelection(c, channel, "public-alias"))
}

func TestAssignedAccessDiagnosticsCannotBeRequestedByClientFields(t *testing.T) {
	db, user, _, _, c, info, req := assignedAccessFixture(t)
	saveAssignedPolicy(t, db, "user", user.Id, 0, accesspolicy.AssignedPolicy{Enabled: false})
	c.Set("is_channel_test", true)
	c.Request.Header.Set("X-Is-Channel-Test", "true")
	assert.ErrorIs(t, ValidateAssignedAccessDispatch(c, req, info), ErrAssignedAccessDenied)
	// RelayInfo.IsChannelTest is only set by authenticated/internal channel-test
	// construction; neither request body, headers nor Gin client fields set it.
	info.IsChannelTest = true
	assert.ErrorIs(t, ValidateAssignedAccessDispatch(c, req, info), ErrAssignedAccessDenied)
	info.TokenId = 0
	require.NoError(t, ValidateAssignedAccessDispatch(c, req, info))
}

func TestAssignedAccessDiscoveryRejectsDisabledAbilityAndMissingKey(t *testing.T) {
	db, user, token, channel, c, info, req := assignedAccessFixture(t)
	saveAssignedPolicy(t, db, "token", token.Id, 0, accesspolicy.AssignedPolicy{Enabled: true, ChannelIDs: []int{channel.Id}})
	require.NoError(t, db.Model(&model.Ability{}).Where("channel_id = ?", channel.Id).Update("enabled", false).Error)
	other := &model.Channel{Type: constant.ChannelTypeOpenAI, Name: "Outside scope", Status: common.ChannelStatusEnabled, Models: "public-alias", Group: "default", Key: "synthetic-other-key"}
	require.NoError(t, db.Create(other).Error)
	require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "public-alias", ChannelId: other.Id, Enabled: true}).Error)
	decisions, _, err := AssignedModelAvailability(context.Background(), user, token, []string{"default"}, []string{"public-alias"}, nil)
	require.NoError(t, err)
	require.Len(t, decisions, 1)
	assert.False(t, decisions[0].Allowed)
	assert.ErrorIs(t, ValidateAssignedAccessDispatch(c, req, info), ErrAssignedAccessDenied)
	require.NoError(t, db.Model(&model.Ability{}).Where("channel_id = ?", channel.Id).Update("enabled", true).Error)
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", channel.Id).Update("key", "").Error)
	decisions, _, err = AssignedModelAvailability(context.Background(), user, token, []string{"default"}, []string{"public-alias"}, nil)
	require.NoError(t, err)
	require.Len(t, decisions, 1)
	assert.False(t, decisions[0].Allowed)
}

func TestAssignedAccessStoredAmbiguousFieldsFailClosed(t *testing.T) {
	for _, raw := range []string{
		`{"enabled":false,"enabled":true}`,
		`{"enabled":false,"\u0065nabled":true}`,
		`{"enabled":false,"Enabled":true}`,
		`{"Enabled":true}`,
	} {
		t.Run(raw, func(t *testing.T) {
			db, _, token, channel, c, _, _ := assignedAccessFixture(t)
			require.NoError(t, db.Create(&model.AssignedAccessPolicy{SubjectType: "token", SubjectID: token.Id, Revision: 1, Assigned: true, PolicyJSON: raw}).Error)
			_, err := ReadAssignedAccessPolicy(context.Background(), "token", token.Id)
			require.Error(t, err)
			assert.Error(t, ValidateAssignedAccessSelection(c, channel, "public-alias"))
		})
	}
}
