package controller

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/middleware"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/service"
	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVideoProxyPrivateResponses(t *testing.T) {
	oldDB, oldLogDB := model.DB, model.LOG_DB
	initModelListColumnNames(t)
	model.DB, model.LOG_DB = oldDB, oldLogDB
	db := paymentWebhookTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Task{}, &model.Channel{}, &model.Token{}))
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", 1).Updates(map[string]any{
		"status": common.UserStatusEnabled, "role": common.RoleCommonUser, "group": "default", "auth_version": 1,
	}).Error)
	require.NoError(t, db.Create(&model.User{Id: 2, Username: "video-other", AffCode: "video-other-fixture", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Group: "default", AuthVersion: 1}).Error)
	for userID, key := range map[int]string{1: "videoownerfixture", 2: "videootherfixture"} {
		require.NoError(t, db.Create(&model.Token{UserId: userID, Key: key, Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}).Error)
	}
	oldMemoryCache := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	t.Cleanup(func() { common.MemoryCacheEnabled = oldMemoryCache })

	var requests atomic.Int32
	const content = "synthetic-private-video-content"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path == "/fail" {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Add("Cache-Control", "public, max-age=86400")
		w.Header().Add("Cache-Control", "s-maxage=86400")
		w.Header().Set("CDN-Cache-Control", "public, max-age=86400")
		w.Header().Set("Cloudflare-CDN-Cache-Control", "public, max-age=86400")
		w.Header().Set("Surrogate-Control", "max-age=86400")
		w.Header().Set("X-Accel-Expires", "86400")
		w.Header().Set("Expires", "Wed, 01 Jan 2042 00:00:00 GMT")
		_, _ = w.Write([]byte(content))
	}))
	t.Cleanup(upstream.Close)
	upstreamURL, err := url.Parse(upstream.URL)
	require.NoError(t, err)
	fetchSettings := config.GlobalConfig.Get("fetch_setting")
	require.NotNil(t, fetchSettings)
	oldFetch, err := config.ConfigToMap(fetchSettings)
	require.NoError(t, err)
	require.NoError(t, config.UpdateConfigFromMap(fetchSettings, map[string]string{
		"enable_ssrf_protection": "true",
		"allow_private_ip":       "true",
		"allowed_ports":          `["` + upstreamURL.Port() + `"]`,
	}))
	t.Cleanup(func() {
		require.NoError(t, config.UpdateConfigFromMap(fetchSettings, oldFetch))
	})
	if service.GetSSRFProtectedHTTPClient() == nil {
		service.InitHttpClient()
	}
	channel := model.Channel{Type: constant.ChannelTypeKling, Name: "video-fixture"}
	require.NoError(t, db.Create(&channel).Error)
	for _, task := range []model.Task{
		{TaskID: "http-video", UserId: 1, ChannelId: channel.Id, Status: model.TaskStatusSuccess, PrivateData: model.TaskPrivateData{ResultURL: upstream.URL + "/content"}},
		{TaskID: "data-video", UserId: 1, ChannelId: channel.Id, Status: model.TaskStatusSuccess, PrivateData: model.TaskPrivateData{ResultURL: "data:video/mp4;base64," + base64.StdEncoding.EncodeToString([]byte(content))}},
		{TaskID: "invalid-data", UserId: 1, ChannelId: channel.Id, Status: model.TaskStatusSuccess, PrivateData: model.TaskPrivateData{ResultURL: "data:video/mp4;base64,%%%"}},
		{TaskID: "pending-video", UserId: 1, ChannelId: channel.Id, Status: model.TaskStatusInProgress},
		{TaskID: "upstream-failure", UserId: 1, ChannelId: channel.Id, Status: model.TaskStatusSuccess, PrivateData: model.TaskPrivateData{ResultURL: upstream.URL + "/fail"}},
	} {
		require.NoError(t, db.Create(&task).Error)
	}
	router := gin.New()
	router.GET("/v1/videos/:task_id/content", middleware.TokenOrUserAuth(), VideoProxy)
	for _, tc := range []struct {
		name, task, key string
		status          int
		upstreamCalls   int32
	}{
		{"owner http", "http-video", "videoownerfixture", http.StatusOK, 1},
		{"owner data url", "data-video", "videoownerfixture", http.StatusOK, 0},
		{"non owner", "http-video", "videootherfixture", http.StatusNotFound, 0},
		{"anonymous", "http-video", "", http.StatusUnauthorized, 0},
		{"not found", "unknown", "videoownerfixture", http.StatusNotFound, 0},
		{"not complete", "pending-video", "videoownerfixture", http.StatusBadRequest, 0},
		{"invalid data url", "invalid-data", "videoownerfixture", http.StatusBadGateway, 0},
		{"upstream error", "upstream-failure", "videoownerfixture", http.StatusBadGateway, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := requests.Load()
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/v1/videos/"+tc.task+"/content", nil)
			if tc.key != "" {
				request.Header.Set("Authorization", "Bearer sk-"+tc.key)
			}
			router.ServeHTTP(response, request)
			require.Equal(t, tc.status, response.Code, response.Body.String())
			assert.Equal(t, tc.upstreamCalls, requests.Load()-before)
			if tc.key != "" {
				// Authentication rejections occur before this controller; they must
				// still never expose content or mark the response public.
				assert.Equal(t, "private, no-store", response.Header().Get("Cache-Control"))
			}
			for _, header := range []string{"Cache-Control", "CDN-Cache-Control", "Cloudflare-CDN-Cache-Control", "Surrogate-Control"} {
				assert.NotContains(t, strings.Join(response.Header().Values(header), ","), "public")
				assert.NotContains(t, strings.Join(response.Header().Values(header), ","), "max-age")
			}
			assert.Empty(t, response.Header().Get("Expires"))
			assert.Empty(t, response.Header().Get("X-Accel-Expires"))
			if tc.status == http.StatusOK {
				assert.Equal(t, content, response.Body.String())
				assert.Equal(t, "video/mp4", response.Header().Get("Content-Type"))
			} else {
				assert.NotContains(t, response.Body.String(), content)
			}
		})
	}

	t.Run("ssrf protection remains active", func(t *testing.T) {
		require.NoError(t, config.UpdateConfigFromMap(fetchSettings, map[string]string{
			"allow_private_ip": "false",
		}))
		before := requests.Load()
		request := httptest.NewRequest(http.MethodGet, "/v1/videos/http-video/content", nil)
		request.Header.Set("Authorization", "Bearer sk-videoownerfixture")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		assert.Equal(t, http.StatusForbidden, response.Code)
		assert.Equal(t, before, requests.Load())
		assert.Equal(t, "private, no-store", response.Header().Get("Cache-Control"))
	})
}
