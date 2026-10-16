package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateChannelRequiresSecureFlowForCodexCredentialChanges(t *testing.T) {
	setupChannelRoutingTest(t)
	channel := &model.Channel{
		Type: constant.ChannelTypeCodex, Name: "Codex guarded", Key: `{"access_token":"original","account_id":"account-a"}`,
		Models: "gpt-5-codex", Group: "default", Status: common.ChannelStatusEnabled,
	}
	require.NoError(t, model.InsertChannelWithAbilities(t.Context(), channel))

	body := `{"id":` + strconv.Itoa(channel.Id) + `,"name":"Codex changed","type":` + strconv.Itoa(constant.ChannelTypeCodex) + `,"key":"{\"access_token\":\"replacement\",\"account_id\":\"account-b\"}","models":"gpt-5-codex","group":"default"}`
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/channel/", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("id", 1)
	ctx.Set("role", common.RoleRootUser)

	UpdateChannel(ctx)

	assert.Equal(t, http.StatusConflict, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"code":"CODEX_CREDENTIAL_UPDATE_REQUIRES_SECURE_FLOW"`)
	var stored model.Channel
	require.NoError(t, model.DB.First(&stored, channel.Id).Error)
	assert.Equal(t, channel.Key, stored.Key)
	assert.Equal(t, channel.Name, stored.Name)
}

func TestUpdateChannelAllowsCodexMetadataWithoutCredentialChange(t *testing.T) {
	setupChannelRoutingTest(t)
	channel := &model.Channel{
		Type: constant.ChannelTypeCodex, Name: "Codex guarded", Key: `{"access_token":"original","account_id":"account-a"}`,
		Models: "gpt-5-codex", Group: "default", Status: common.ChannelStatusEnabled,
	}
	require.NoError(t, model.InsertChannelWithAbilities(t.Context(), channel))

	body := `{"id":` + strconv.Itoa(channel.Id) + `,"name":"Codex renamed","type":` + strconv.Itoa(constant.ChannelTypeCodex) + `,"models":"gpt-5-codex","group":"default"}`
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/channel/", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("id", 1)
	ctx.Set("role", common.RoleRootUser)

	UpdateChannel(ctx)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"success":true`)
	var stored model.Channel
	require.NoError(t, model.DB.First(&stored, channel.Id).Error)
	assert.Equal(t, channel.Key, stored.Key)
	assert.Equal(t, "Codex renamed", stored.Name)
}
