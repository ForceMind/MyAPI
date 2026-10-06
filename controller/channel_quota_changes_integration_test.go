package controller

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestGetChannelQuotaChangesSeparatesAccountsFromPersistedRows(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelQuotaSnapshot{}))
	previousDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })

	channel := model.Channel{Name: "shared-channel", Type: 57}
	require.NoError(t, db.Create(&channel).Error)
	accountRefs := []string{
		model.ChannelQuotaAccountRef("codex", "account-a"),
		model.ChannelQuotaAccountRef("codex", "account-b"),
	}
	now := time.Now().Unix()
	for index, ref := range accountRefs {
		for step := 0; step < 2; step++ {
			require.NoError(t, db.Create(&model.ChannelQuotaSnapshot{
				ChannelId: channel.Id, AccountRef: ref,
				ObservedAt: now - 120 + int64(step*60),
				Available:  float64(90 - index*20 - step*10),
				Unit:       "percent", MetricType: "codex_rate_limit",
				WindowType: "weekly", Source: "codex_wham_usage_primary",
				Status: "success",
			}).Error)
		}
	}

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("GET", "/api/channel/quota/changes?range=1h&overview_points=48&limit=64", nil)
	GetChannelQuotaChanges(ctx)

	require.Equal(t, 200, recorder.Code)
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			Items []struct {
				SeriesID         string   `json:"series_id"`
				CurrentAvailable *float64 `json:"current_available"`
			} `json:"items"`
			TotalItems int `json:"total_items"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	require.Equal(t, 2, response.Data.TotalItems)
	require.Len(t, response.Data.Items, 2)
	series := make(map[string]float64)
	for _, item := range response.Data.Items {
		require.Len(t, item.SeriesID, 64)
		require.NotNil(t, item.CurrentAvailable)
		series[item.SeriesID] = *item.CurrentAvailable
	}
	require.Len(t, series, 2)
	available := make(map[float64]bool)
	for _, value := range series {
		available[value] = true
	}
	require.Equal(t, map[float64]bool{80: true, 60: true}, available)
	for _, ref := range accountRefs {
		require.NotContains(t, recorder.Body.String(), ref)
	}
}
