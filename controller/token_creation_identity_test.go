package controller

import (
	"net/http"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func tokenCreationIdentityFixture(t *testing.T) *gorm.DB {
	t.Helper()
	db := subscriptionShareControllerFixture(t)
	previousRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = previousRedis })
	settings, ok := config.GlobalConfig.Get("token_setting").(config.MapConfig)
	require.True(t, ok)
	previous, err := settings.ExportConfigMap()
	require.NoError(t, err)
	require.NoError(t, settings.UpdateConfigMap(map[string]string{"max_user_tokens": "10"}))
	t.Cleanup(func() { require.NoError(t, settings.UpdateConfigMap(previous)) })
	return db
}

func TestAddTokenReturnsOnlyTheExactCreatedIdentity(t *testing.T) {
	db := tokenCreationIdentityFixture(t)
	var returnedIDs []int
	// Identical names cannot be used to infer which object was just created.
	for range 2 {
		ctx, recorder := newAuthenticatedContext(t, http.MethodPost, "/api/token/", map[string]any{
			"id": 9999, "user_id": 3, "name": "same-name", "key": "synthetic-client-supplied-secret",
			"status": common.TokenStatusDisabled, "created_time": 123, "used_quota": 900,
			"remain_quota": 100, "unlimited_quota": false, "expired_time": -1, "group": "default",
		}, 2)
		AddToken(ctx)
		require.Equal(t, http.StatusOK, recorder.Code)
		var envelope struct {
			Success bool           `json:"success"`
			Message string         `json:"message"`
			Data    map[string]int `json:"data"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &envelope))
		require.True(t, envelope.Success, envelope.Message)
		require.Empty(t, envelope.Message)
		require.Len(t, envelope.Data, 1)
		id := envelope.Data["id"]
		require.Positive(t, id)
		require.NotEqual(t, 9999, id)
		var stored model.Token
		require.NoError(t, db.First(&stored, id).Error)
		assert.Equal(t, 2, stored.UserId)
		assert.Equal(t, "same-name", stored.Name)
		assert.Equal(t, int64(100), stored.RemainQuota)
		assert.Zero(t, stored.UsedQuota)
		assert.Equal(t, common.TokenStatusEnabled, stored.Status)
		assert.Greater(t, stored.CreatedTime, int64(123))
		assert.NotEmpty(t, stored.Key)
		assert.NotEqual(t, "synthetic-client-supplied-secret", stored.Key)
		var projected map[string]any
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &projected))
		assert.Len(t, projected, 3)
		for _, private := range []string{stored.Key, "synthetic-client-supplied-secret", `"user_id"`, `"name"`, `"key"`, `"used_quota"`} {
			assert.NotContains(t, recorder.Body.String(), private)
		}
		returnedIDs = append(returnedIDs, id)
	}
	assert.NotEqual(t, returnedIDs[0], returnedIDs[1])
	var ownerCount, otherCount int64
	require.NoError(t, db.Model(&model.Token{}).Where("user_id = ?", 2).Count(&ownerCount).Error)
	require.NoError(t, db.Model(&model.Token{}).Where("user_id = ?", 3).Count(&otherCount).Error)
	assert.Equal(t, int64(3), ownerCount) // fixture token plus two explicit creates
	assert.Zero(t, otherCount)
}

func TestAddTokenRejectedCreationDoesNotReturnAnIdentity(t *testing.T) {
	db := tokenCreationIdentityFixture(t)
	var before int64
	require.NoError(t, db.Model(&model.Token{}).Count(&before).Error)
	ctx, recorder := newAuthenticatedContext(t, http.MethodPost, "/api/token/", map[string]any{
		"name": strings.Repeat("x", 51), "remain_quota": 0,
	}, 2)
	AddToken(ctx)
	var response map[string]any
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, false, response["success"])
	assert.NotContains(t, response, "data")
	var after int64
	require.NoError(t, db.Model(&model.Token{}).Count(&after).Error)
	assert.Equal(t, before, after)
}
