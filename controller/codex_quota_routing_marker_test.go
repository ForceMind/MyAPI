package controller

import (
	"context"
	"errors"
	"fmt"
	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/ForceMind/MyAPI/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func TestCodexUsageLimit429MarksAccountWithoutPermanentlyDisablingChannel(t *testing.T) {
	setupChannelRoutingTest(t)
	t.Setenv(common.ChannelQuotaIdentityKeysEnv, "")
	require.NoError(t, model.DB.AutoMigrate(&model.ChannelQuotaSnapshot{}))
	oldErrorLog := constant.ErrorLogEnabled
	constant.ErrorLogEnabled = false
	t.Cleanup(func() { constant.ErrorLogEnabled = oldErrorLog })
	autoBan := 1
	channel := &model.Channel{
		Type: constant.ChannelTypeCodex, Name: "quota-429-codex", Status: common.ChannelStatusEnabled,
		Key: `{"access_token":"fixture-access","account_id":"fixture-account"}`, AutoBan: &autoBan,
	}
	require.NoError(t, model.DB.Create(channel).Error)
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	apiErr := types.NewOpenAIError(errors.New("The usage limit has been reached"), types.ErrorCodeBadResponseStatusCode, http.StatusTooManyRequests)
	channelErr := *types.NewChannelError(channel.Id, channel.Type, channel.Name, false, channel.Key, true)

	require.NoError(t, processChannelError(ctx, channelErr, apiErr))
	var stored model.Channel
	require.NoError(t, model.DB.Where("id = ?", channel.Id).First(&stored).Error)
	assert.Equal(t, common.ChannelStatusEnabled, stored.Status)
	var marker model.ChannelQuotaSnapshot
	require.NoError(t, model.DB.Where("source = ? AND channel_id = ?", model.CodexQuotaRouteLimitSource, channel.Id).Take(&marker).Error)
	assert.Equal(t, model.CodexQuotaRouteLimitCode, marker.ErrorCode)
	assert.Zero(t, marker.ResetAt, "without a provider reset time the account remains held until later confirmed capacity")
	excluded, eligible, err := service.CodexQuotaEligibleKeys(ctx.Request.Context(), channel)
	require.NoError(t, err)
	assert.False(t, eligible)
	assert.True(t, excluded[0])
}

func TestManualCodexUsageExhaustionIsVisibleToRouting(t *testing.T) {
	for _, versionedIdentity := range []bool{false, true} {
		name := "legacy_identity"
		if versionedIdentity {
			name = "versioned_identity"
		}
		t.Run(name, func(t *testing.T) {
			setupChannelRoutingTest(t)
			t.Setenv(common.ChannelQuotaIdentityKeysEnv, "")
			require.NoError(t, model.DB.AutoMigrate(&model.ChannelQuotaSnapshot{}))
			if versionedIdentity {
				setupChannelQuotaIdentityFixture(t, model.DB)
			}
			channel := &model.Channel{
				Type: constant.ChannelTypeCodex, Name: name, Status: common.ChannelStatusEnabled,
				Key: `{"access_token":"fixture-access","account_id":"fixture-account"}`,
			}
			require.NoError(t, model.DB.Create(channel).Error)
			gin.SetMode(gin.TestMode)
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodGet, "/api/channel/"+strconv.Itoa(channel.Id)+"/codex/usage", nil)
			ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(channel.Id)}}
			body := []byte(fmt.Sprintf(`{"rate_limit":{"primary_window":{"used_percent":100,"reset_at":%d,"limit_window_seconds":18000}}}`, time.Now().Add(time.Hour).Unix()))
			fetchCodexChannelWhamData(ctx, func(context.Context, *http.Client, string, string, string) (int, []byte, error) {
				return http.StatusOK, body, nil
			}, "fixture usage", "safe user message", true)
			require.Equal(t, http.StatusOK, recorder.Code)
			assert.Contains(t, recorder.Body.String(), `"success":true`)

			var snapshot model.ChannelQuotaSnapshot
			require.NoError(t, model.DB.Where("channel_id = ? AND source = ?", channel.Id, "codex_wham_usage_primary").Take(&snapshot).Error)
			assert.Zero(t, snapshot.Available)
			if versionedIdentity {
				assert.NotEmpty(t, snapshot.SubjectRef)
				assert.Equal(t, model.ChannelQuotaIdentityQualityProviderConfirmed, snapshot.IdentityQuality)
				assert.Empty(t, snapshot.AccountRef)
			} else {
				assert.Equal(t, model.ChannelQuotaAccountRef("codex", "fixture-account"), snapshot.AccountRef)
			}
			excluded, eligible, err := service.CodexQuotaEligibleKeys(ctx.Request.Context(), channel)
			require.NoError(t, err)
			assert.False(t, eligible)
			assert.True(t, excluded[0])
		})
	}
}

func TestManualCodexUsageDoesNotClaimSuccessWhenSnapshotWriteFails(t *testing.T) {
	setupChannelRoutingTest(t)
	t.Setenv(common.ChannelQuotaIdentityKeysEnv, "")
	require.NoError(t, model.DB.AutoMigrate(&model.ChannelQuotaSnapshot{}))
	channel := &model.Channel{
		Type: constant.ChannelTypeCodex, Name: "manual-persistence-failure", Status: common.ChannelStatusEnabled,
		Key: `{"access_token":"fixture-access","account_id":"fixture-account"}`,
	}
	require.NoError(t, model.DB.Create(channel).Error)
	require.NoError(t, model.DB.Callback().Create().Before("gorm:create").Register("reject_manual_usage_snapshot", func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Table == "channel_quota_snapshots" {
			tx.AddError(errors.New("fixture snapshot write unavailable"))
		}
	}))
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/channel/"+strconv.Itoa(channel.Id)+"/codex/usage", nil)
	ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(channel.Id)}}
	body := []byte(fmt.Sprintf(`{"rate_limit":{"primary_window":{"used_percent":100,"reset_at":%d,"limit_window_seconds":18000}}}`, time.Now().Add(time.Hour).Unix()))
	fetchCodexChannelWhamData(ctx, func(context.Context, *http.Client, string, string, string) (int, []byte, error) {
		return http.StatusOK, body, nil
	}, "fixture usage", "safe user message", true)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"success":false`)
	assert.Contains(t, recorder.Body.String(), "safe user message")
	var count int64
	require.NoError(t, model.DB.Model(&model.ChannelQuotaSnapshot{}).Count(&count).Error)
	assert.Zero(t, count)
}
