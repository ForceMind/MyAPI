package controller

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func quotaChangesErrorMessage(t *testing.T, query string) string {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("GET", "/api/channel/quota/changes?"+query, nil)
	GetChannelQuotaChanges(ctx)
	require.Equal(t, 200, recorder.Code)
	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.False(t, response.Success)
	return response.Message
}

func TestGetChannelQuotaChangesRejectsInvalidRange(t *testing.T) {
	require.Contains(t, quotaChangesErrorMessage(t, "range=year"), "invalid range")
}

func TestGetChannelQuotaChangesRejectsInvalidTimestamps(t *testing.T) {
	require.Contains(t, quotaChangesErrorMessage(t, "range=custom&start=not-a-time"), "invalid start timestamp")
	require.Contains(t, quotaChangesErrorMessage(t, "range=custom&end=not-a-time"), "invalid end timestamp")
}

func TestGetChannelQuotaChangesRejectsUnsafeTimeRange(t *testing.T) {
	require.Contains(t, quotaChangesErrorMessage(t, "range=custom&start=1&end=15552002"), "invalid quota changes time range")
	require.Contains(t, quotaChangesErrorMessage(t, "range=custom&start=10&end=9"), "invalid quota changes time range")
}

func TestGetChannelQuotaChangesRejectsInvalidLimitAndChannelIDs(t *testing.T) {
	require.Contains(t, quotaChangesErrorMessage(t, "limit=0"), "invalid quota changes limit")
	require.Contains(t, quotaChangesErrorMessage(t, "limit=not-a-number"), "invalid quota changes limit")
	message := quotaChangesErrorMessage(t, "channel_ids=1,nope")
	require.True(t, strings.Contains(message, "invalid channel_ids"))
}

func TestGetChannelQuotaChangesCatalogueReturnsOnlyBoundedSeriesMetadata(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelQuotaSnapshot{}))
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		sqlDB, sqlErr := db.DB()
		require.NoError(t, sqlErr)
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.Create(&model.Channel{Id: 981, Name: "catalogue channel", Key: "fixture"}).Error)
	for index, source := range []string{"provider_primary", "provider_secondary"} {
		require.NoError(t, db.Create(&model.ChannelQuotaSnapshot{
			ChannelId: 981, ObservedAt: int64(100 + index), Available: float64(90 - index),
			MetricType: "rate_limit", WindowType: "weekly", Source: source,
			PlanType: "pro", Unit: "percent", WindowSeconds: 604800, Status: "success",
		}).Error)
	}

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("GET", "/api/channel/quota/changes?catalogue=true&limit=1&range=invalid&rate_window=invalid", nil)
	GetChannelQuotaChanges(ctx)
	require.Equal(t, 200, recorder.Code)
	var response struct {
		Success bool           `json:"success"`
		Data    map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success, recorder.Body.String())
	require.Equal(t, true, response.Data["catalogue"])
	require.Equal(t, true, response.Data["source_complete"])
	require.Equal(t, false, response.Data["items_complete"])
	require.Equal(t, float64(1), response.Data["returned_items"])
	require.Equal(t, float64(20000), response.Data["scan_limit"])
	require.Equal(t, float64(2), response.Data["scanned_items"])
	require.Equal(t, true, response.Data["truncated"])
	require.Equal(t, "items_limit", response.Data["truncation_reason"])
	require.NotNil(t, response.Data["generated_at"])
	items, ok := response.Data["items"].([]any)
	require.True(t, ok)
	require.Len(t, items, 1)
	item, ok := items[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, map[string]any{
		"channel_id":     float64(981),
		"name":           "catalogue channel",
		"account_label":  "catalogue channel",
		"metric_type":    "rate_limit",
		"window_type":    "weekly",
		"source":         "provider_primary",
		"plan_type":      "pro",
		"unit":           "percent",
		"currency":       "",
		"window_seconds": float64(604800),
	}, item)
	require.NotContains(t, response.Data, "summary")
	require.NotContains(t, response.Data, "data_quality")
}

func TestGetChannelQuotaChangesRejectsInvalidCatalogueParameters(t *testing.T) {
	require.Contains(t, quotaChangesErrorMessage(t, "catalogue=maybe"), "invalid catalogue")
	require.Contains(t, quotaChangesErrorMessage(t, "catalogue=true&limit=2001"), "between 1 and 2000")
}

func TestGetChannelQuotaChangesRejectsAnalysisWindowsOutsideSelectedRange(t *testing.T) {
	require.Contains(t, quotaChangesErrorMessage(t, "range=1h&rate_window=6h"), "selected range")
	require.Contains(t, quotaChangesErrorMessage(t, "range=1h&rate_window=1h&ewma_half_life=6h"), "selected range")
}

func TestGetChannelQuotaChangesRejectsInvalidOverviewPointLimit(t *testing.T) {
	for _, value := range []string{"0", "121", "invalid"} {
		require.Contains(
			t,
			quotaChangesErrorMessage(t, "range=1h&overview_points="+value),
			"overview_points",
		)
	}
}

func TestQuotaChangeQueryHelpersNormalizeAliasesAndDeduplicateIDs(t *testing.T) {
	seconds, ok := quotaChangeRangeSeconds("1h")
	require.True(t, ok)
	require.EqualValues(t, 60*60, seconds)
	seconds, ok = quotaChangeRangeSeconds("6h")
	require.True(t, ok)
	require.EqualValues(t, 6*60*60, seconds)
	seconds, ok = quotaChangeRangeSeconds("1d")
	require.True(t, ok)
	require.EqualValues(t, 24*60*60, seconds)
	ids, err := parseQuotaChangeChannelIDs("7, 7, 9")
	require.NoError(t, err)
	require.Equal(t, []int{7, 9}, ids)
	_, err = parseQuotaChangeChannelIDs("0")
	require.Error(t, err)
}
