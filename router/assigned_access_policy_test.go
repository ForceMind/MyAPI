package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAssignedPolicyRelayRejectsEmptyCandidatesWithForbidden(t *testing.T) {
	setupRelayRouterTestDB(t)
	require.NoError(t, i18n.Init())
	memory, cooldown := common.MemoryCacheEnabled, common.RelayFailureCooldownSeconds
	common.MemoryCacheEnabled = false
	common.RelayFailureCooldownSeconds = 0
	t.Cleanup(func() { common.MemoryCacheEnabled = memory; common.RelayFailureCooldownSeconds = cooldown })
	user := model.User{Username: "assigned-router-user", Status: common.UserStatusEnabled, Group: "default", Quota: 1000}
	require.NoError(t, model.DB.Create(&user).Error)
	token := model.Token{UserId: user.Id, Key: "assignedrouterkey", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}
	require.NoError(t, model.DB.Create(&token).Error)
	channel := model.Channel{Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Models: "alias", Group: "default", Key: "synthetic-channel-key"}
	require.NoError(t, model.DB.Create(&channel).Error)
	require.NoError(t, model.DB.Create(&model.Ability{Group: "default", Model: "alias", ChannelId: channel.Id, Enabled: true}).Error)
	_, err := model.ConfigureAssignedAccessPolicy(context.Background(), model.DB, "token", token.Id, 0, `{"enabled":false,"public_models":null,"upstream_models":null,"channel_ids":null}`)
	require.NoError(t, err)
	engine := gin.New()
	SetRelayRouter(engine)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"alias","input":"test"}`))
	request.Header.Set("Authorization", "Bearer "+token.Key)
	request.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusForbidden, recorder.Code, recorder.Body.String())
	assert.NotContains(t, recorder.Body.String(), channel.Key)
}
