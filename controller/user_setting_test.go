package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/relaykit/dto"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func performUpdateUserSettingRequest(t *testing.T, userID int, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/user/setting", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("id", userID)
	UpdateUserSetting(c)
	return recorder
}

func TestUpdateUserSettingPreservesPreferencesOutsideNotificationForm(t *testing.T) {
	db := setupManageUserTestDB(t)
	user := model.User{
		Username: "notification-setting-user",
		Password: "password",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		Group:    "default",
	}
	user.SetSetting(dto.UserSetting{
		NotifyType:                       dto.NotifyTypeWebhook,
		QuotaWarningThreshold:            25,
		WebhookUrl:                       "https://old.example.test/hook",
		WebhookSecret:                    "old-secret",
		UpstreamModelUpdateNotifyEnabled: true,
		SidebarModules:                   `{"dashboard":{"visible":false}}`,
		BillingPreference:                "subscription_first",
		Language:                         "ja",
	})
	require.NoError(t, db.Create(&user).Error)

	recorder := performUpdateUserSettingRequest(t, user.Id, `{
		"notify_type":"email",
		"quota_warning_threshold":50,
		"notification_email":"alerts@example.test",
		"accept_unset_model_ratio_model":true,
		"record_ip_log":true,
		"upstream_model_update_notify_enabled":false
	}`)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"success":true`)
	var updated model.User
	require.NoError(t, db.First(&updated, user.Id).Error)
	settings := updated.GetSetting()
	assert.Equal(t, dto.NotifyTypeEmail, settings.NotifyType)
	assert.Equal(t, float64(50), settings.QuotaWarningThreshold)
	assert.Equal(t, "alerts@example.test", settings.NotificationEmail)
	assert.True(t, settings.AcceptUnsetRatioModel)
	assert.True(t, settings.RecordIpLog)
	assert.True(t, settings.UpstreamModelUpdateNotifyEnabled)
	assert.Equal(t, "ja", settings.Language)
	assert.Equal(t, `{"dashboard":{"visible":false}}`, settings.SidebarModules)
	assert.Equal(t, "subscription_first", settings.BillingPreference)
}

func TestUpdateUserSettingClearsSecretForInactiveNotificationMethod(t *testing.T) {
	db := setupManageUserTestDB(t)
	user := model.User{
		Username: "notification-secret-user",
		Password: "password",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		Group:    "default",
	}
	user.SetSetting(dto.UserSetting{
		NotifyType:            dto.NotifyTypeWebhook,
		QuotaWarningThreshold: 25,
		WebhookUrl:            "https://old.example.test/hook",
		WebhookSecret:         "old-secret",
		GotifyToken:           "old-gotify-token",
	})
	require.NoError(t, db.Create(&user).Error)

	recorder := performUpdateUserSettingRequest(t, user.Id, `{
		"notify_type":"email",
		"quota_warning_threshold":50,
		"notification_email":"alerts@example.test",
		"accept_unset_model_ratio_model":false,
		"record_ip_log":false
	}`)

	assert.Equal(t, http.StatusOK, recorder.Code)
	var updated model.User
	require.NoError(t, db.First(&updated, user.Id).Error)
	settings := updated.GetSetting()
	assert.Empty(t, settings.WebhookUrl)
	assert.Empty(t, settings.WebhookSecret)
	assert.Empty(t, settings.GotifyToken)
}
