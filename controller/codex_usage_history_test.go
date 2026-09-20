package controller

import (
	"context"
	"encoding/json"
	"math"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestNormalizeCodexUsageSnapshots(t *testing.T) {
	body := []byte(`{"plan_type":"pro","rate_limit":{"primary_window":{"used_percent":12.5,"reset_at":1700000100,"limit_window_seconds":18000},"secondary_window":{"used_percent":44,"reset_at":1700400000,"limit_window_seconds":604800}}}`)
	snapshots := normalizeCodexUsageSnapshots(7, 1700000000, 200, body)
	require.Len(t, snapshots, 2)
	require.Equal(t, "codex_rate_limit", snapshots[0].MetricType)
	require.Equal(t, "five_hour", snapshots[0].WindowType)
	require.Equal(t, int64(18000), snapshots[0].WindowSeconds)
	require.Equal(t, "pro", snapshots[0].PlanType)
	require.InDelta(t, 87.5, snapshots[0].Available, 0.001)
	require.Equal(t, "weekly", snapshots[1].WindowType)
}

func TestNormalizeCodexUsageSnapshotsUsesExactProviderWindowDurations(t *testing.T) {
	tests := []struct {
		name     string
		seconds  int64
		expected string
	}{
		{name: "five hour", seconds: 18000, expected: "five_hour"},
		{name: "daily", seconds: 86400, expected: "daily"},
		{name: "weekly", seconds: 604800, expected: "weekly"},
		{name: "unknown short duration", seconds: 7200, expected: "custom"},
		{name: "unknown long duration", seconds: 1209600, expected: "custom"},
		{name: "missing duration", seconds: 0, expected: "unknown"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.expected, codexWindowType(test.seconds))
		})
	}

	snapshots := normalizeCodexUsageSnapshots(7, 10, 200, []byte(`{"plan_type":"free","rate_limit":{"primary_window":{"used_percent":20,"limit_window_seconds":604800}}}`))
	require.Len(t, snapshots, 1)
	require.Equal(t, "weekly", snapshots[0].WindowType)
	require.Equal(t, "free", snapshots[0].PlanType)
}

func TestNormalizeCodexUsageSnapshotsRejectsInvalidPercent(t *testing.T) {
	body := []byte(`{"rate_limit":{"primary_window":{"used_percent":101},"secondary_window":{"used_percent":-1}}}`)
	snapshots := normalizeCodexUsageSnapshots(7, 10, 200, body)
	require.Len(t, snapshots, 1)
	require.Equal(t, "unsupported", snapshots[0].Status)
	require.Equal(t, "invalid_payload", snapshots[0].ErrorCode)
	snapshots = normalizeCodexUsageSnapshots(7, 10, 200, []byte(`not-json`))
	require.Len(t, snapshots, 1)
	require.Equal(t, "unsupported", snapshots[0].Status)
}

func TestNormalizeCodexUsageSnapshotsRecordsUpstreamFailure(t *testing.T) {
	snapshots := normalizeCodexUsageSnapshots(7, 1700000000, 503, nil)
	require.Len(t, snapshots, 1)
	require.Equal(t, "error", snapshots[0].Status)
	require.Equal(t, "upstream_http", snapshots[0].ErrorCode)
	require.Equal(t, int64(1700000000), snapshots[0].ObservedAt)
}

func TestNormalizeCodexUsageSnapshotsRecordsTransportFailure(t *testing.T) {
	snapshots := normalizeCodexUsageSnapshots(7, 1700000000, 0, nil)
	require.Len(t, snapshots, 1)
	require.Equal(t, "upstream_transport", snapshots[0].ErrorCode)
}

func TestCodexUsageTransitionEndsMissingSeriesWithoutDeletingHistory(t *testing.T) {
	db := setupCodexUsageHistoryTestDB(t, 981)
	observedAt := time.Now().Unix() - 10
	dualWindow := []byte(`{
		"plan_type":"free",
		"rate_limit":{
			"primary_window":{"used_percent":25,"reset_at":1900000000,"limit_window_seconds":18000},
			"secondary_window":{"used_percent":60,"reset_at":1900600000,"limit_window_seconds":604800}
		}
	}`)
	singleWeekly := []byte(`{
		"plan_type":"plus",
		"rate_limit":{
			"primary_window":{"used_percent":30,"reset_at":1901200000,"limit_window_seconds":604800}
		}
	}`)
	require.NoError(t, recordCodexUsageSnapshotsAtWithSampleID(981, observedAt, "sample-dual", 200, dualWindow))
	require.NoError(t, recordCodexUsageSnapshotsAtWithSampleID(981, observedAt, "sample-single", 200, singleWeekly))
	// An exact replay uses the immutable sample id and cannot duplicate or change
	// the committed provider rows and absence markers.
	require.NoError(t, recordCodexUsageSnapshotsAtWithSampleID(981, observedAt, "sample-single", 200, singleWeekly))

	var snapshots []model.ChannelQuotaSnapshot
	require.NoError(t, db.Where("channel_id = ?", 981).Order("observed_at ASC, id ASC").Find(&snapshots).Error)
	require.Len(t, snapshots, 5)

	markers := make(map[string]model.ChannelQuotaSnapshot)
	successes := 0
	for _, snapshot := range snapshots {
		if snapshot.Status == "success" {
			successes++
			continue
		}
		require.Equal(t, "unsupported", snapshot.Status)
		require.Equal(t, "window_absent", snapshot.ErrorCode)
		require.Equal(t, "sample-single", snapshot.SampleID)
		require.Nil(t, snapshot.Used)
		require.Nil(t, snapshot.Total)
		markers[snapshot.Source] = snapshot
	}
	require.Equal(t, 3, successes)
	require.Len(t, markers, 2)
	require.Equal(t, "five_hour", markers["codex_wham_usage_primary"].WindowType)
	require.Equal(t, int64(18000), markers["codex_wham_usage_primary"].WindowSeconds)
	require.Equal(t, "free", markers["codex_wham_usage_primary"].PlanType)
	require.Equal(t, int64(1900000000), markers["codex_wham_usage_primary"].ResetAt)
	require.Equal(t, "weekly", markers["codex_wham_usage_secondary"].WindowType)
	require.Equal(t, int64(604800), markers["codex_wham_usage_secondary"].WindowSeconds)
	require.Equal(t, "free", markers["codex_wham_usage_secondary"].PlanType)

	data := getCodexUsageHistoryTestData(t, 981, observedAt-1, observedAt+1)
	points := data["points"].([]any)
	require.Len(t, points, 2)
	first := points[0].(map[string]any)
	require.Equal(t, "sample-dual", first["sample_id"])
	require.Equal(t, float64(25), first["primary_used_percent"])
	require.Equal(t, float64(60), first["secondary_used_percent"])
	current := data["current"].(map[string]any)
	require.Equal(t, "sample-single", current["sample_id"])
	require.Equal(t, "success", current["status"])
	require.Equal(t, "plus", current["plan_type"])
	require.Equal(t, float64(30), current["primary_used_percent"])
	require.Equal(t, "weekly", current["primary_window_type"])
	require.NotContains(t, current, "secondary_used_percent")
	require.Equal(t, "unavailable", current["secondary_status"])
	require.Equal(t, float64(observedAt), current["secondary_last_seen_at"])
	require.Equal(t, float64(observedAt), current["secondary_ended_at"])

	seriesData := getCodexQuotaSeriesHistoryTestData(t, 981, observedAt-1, observedAt+1)
	seriesCurrent := seriesData["current"].(map[string]any)
	require.Equal(t, "unsupported", seriesCurrent["status"])
	require.Equal(t, "window_absent", seriesCurrent["error_code"])
	require.NotContains(t, seriesCurrent, "available")
	seriesPoints := seriesData["points"].([]any)
	require.Len(t, seriesPoints, 2)
	require.Equal(t, "success", seriesPoints[0].(map[string]any)["status"])
	require.Equal(t, "unsupported", seriesPoints[1].(map[string]any)["status"])

	aggregateRows, err := model.ListChannelQuotaAggregateRows(
		context.Background(), observedAt-1, observedAt+1, []int{981}, "codex_rate_limit", "", "",
	)
	require.NoError(t, err)
	items, _ := buildQuotaChangeItems(aggregateRows)
	var endedFiveHour, currentWeekly *quotaChangeItem
	for index := range items {
		item := &items[index]
		if item.Source == "codex_wham_usage_primary" && item.PlanType == "free" && item.WindowType == "five_hour" {
			endedFiveHour = item
		}
		if item.Source == "codex_wham_usage_primary" && item.PlanType == "plus" && item.WindowType == "weekly" {
			currentWeekly = item
		}
	}
	require.NotNil(t, endedFiveHour)
	require.Equal(t, "unsupported", endedFiveHour.Status)
	require.Nil(t, endedFiveHour.CurrentAvailable)
	require.Equal(t, observedAt, endedFiveHour.ObservedAt)
	require.NotNil(t, currentWeekly)
	require.Equal(t, "success", currentWeekly.Status)
	require.NotNil(t, currentWeekly.CurrentAvailable)
	require.InDelta(t, 70, *currentWeekly.CurrentAvailable, 0.001)
}

func TestCodexUsageSameSecondSuccessThenErrorUsesLatestBatchOnlyForCurrent(t *testing.T) {
	setupCodexUsageHistoryTestDB(t, 982)
	observedAt := time.Now().Unix() - 10
	require.NoError(t, recordCodexUsageSnapshotsAtWithSampleID(982, observedAt, "sample-success", 200, []byte(`{"plan_type":"free","rate_limit":{"primary_window":{"used_percent":20,"limit_window_seconds":604800}}}`)))
	require.NoError(t, recordCodexUsageSnapshotsAtWithSampleID(982, observedAt, "sample-error", 503, nil))

	data := getCodexUsageHistoryTestData(t, 982, observedAt-1, observedAt+1)
	points := data["points"].([]any)
	require.Len(t, points, 2)
	require.Equal(t, "sample-success", points[0].(map[string]any)["sample_id"])
	require.Equal(t, float64(20), points[0].(map[string]any)["primary_used_percent"])
	require.Equal(t, "sample-error", points[1].(map[string]any)["sample_id"])
	require.NotContains(t, points[1].(map[string]any), "primary_used_percent")
	current := data["current"].(map[string]any)
	require.Equal(t, "sample-error", current["sample_id"])
	require.Equal(t, "error", current["status"])
	require.Equal(t, "upstream_http", current["error_code"])
	require.NotContains(t, current, "primary_used_percent")
}

func TestCodexUsageHistoryLimitKeepsSampleBatchIntact(t *testing.T) {
	setupCodexUsageHistoryTestDB(t, 984)
	observedAt := time.Now().Unix() - 10
	body := []byte(`{"plan_type":"free","rate_limit":{"primary_window":{"used_percent":20,"limit_window_seconds":18000},"secondary_window":{"used_percent":40,"limit_window_seconds":604800}}}`)
	require.NoError(t, recordCodexUsageSnapshotsAtWithSampleID(984, observedAt, "sample-batch", 200, body))

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: "984"}}
	ctx.Request = httptest.NewRequest("GET", "/api/channel/984/codex/usage/history?start="+strconv.FormatInt(observedAt-1, 10)+"&end="+strconv.FormatInt(observedAt+1, 10)+"&limit=1", nil)
	GetCodexChannelUsageHistory(ctx)
	require.Equal(t, 200, recorder.Code, recorder.Body.String())
	var response struct {
		Success bool           `json:"success"`
		Data    map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success, recorder.Body.String())
	points := response.Data["points"].([]any)
	require.Len(t, points, 1)
	point := points[0].(map[string]any)
	require.Equal(t, "sample-batch", point["sample_id"])
	require.Equal(t, float64(20), point["primary_used_percent"])
	require.Equal(t, float64(40), point["secondary_used_percent"])
}

func TestCodexUsageSameSecondNumericSamplesRemainSeparate(t *testing.T) {
	setupCodexUsageHistoryTestDB(t, 983)
	observedAt := time.Now().Unix() - 10
	first := []byte(`{"plan_type":"team","rate_limit":{"primary_window":{"used_percent":10,"limit_window_seconds":18000}}}`)
	second := []byte(`{"plan_type":"team","rate_limit":{"primary_window":{"used_percent":40,"limit_window_seconds":18000}}}`)
	require.NoError(t, recordCodexUsageSnapshotsAtWithSampleID(983, observedAt, "sample-value-1", 200, first))
	require.NoError(t, recordCodexUsageSnapshotsAtWithSampleID(983, observedAt, "sample-value-2", 200, second))

	data := getCodexUsageHistoryTestData(t, 983, observedAt-1, observedAt+1)
	points := data["points"].([]any)
	require.Len(t, points, 2)
	require.Equal(t, "sample-value-1", points[0].(map[string]any)["sample_id"])
	require.Equal(t, float64(10), points[0].(map[string]any)["primary_used_percent"])
	require.Equal(t, "sample-value-2", points[1].(map[string]any)["sample_id"])
	require.Equal(t, float64(40), points[1].(map[string]any)["primary_used_percent"])
	current := data["current"].(map[string]any)
	require.Equal(t, "sample-value-2", current["sample_id"])
	require.Equal(t, float64(40), current["primary_used_percent"])
}

func TestFiniteCodexPercent(t *testing.T) {
	for _, value := range []float64{0, 50, 100} {
		require.True(t, finiteCodexPercent(value))
	}
	for _, value := range []float64{-1, 101, math.NaN(), math.Inf(1)} {
		require.False(t, finiteCodexPercent(value))
	}
}

func TestCodexSnapshotAccountReferencesDoNotCrossMarkAbsentWindows(t *testing.T) {
	db := setupCodexUsageHistoryTestDB(t, 986)
	observedAt := int64(1700000000)
	dualWindow := []byte(`{"rate_limit":{"primary_window":{"used_percent":25,"limit_window_seconds":18000},"secondary_window":{"used_percent":60,"limit_window_seconds":604800}}}`)
	singleWindow := []byte(`{"rate_limit":{"primary_window":{"used_percent":30,"limit_window_seconds":18000}}}`)

	require.NoError(t, recordCodexUsageSnapshotsAtWithSampleIDForAccount(986, "account-a", observedAt, "account-a-dual", 200, dualWindow))
	require.NoError(t, recordCodexUsageSnapshotsAtWithSampleIDForAccount(986, "account-b", observedAt+1, "account-b-single", 200, singleWindow))

	var snapshots []model.ChannelQuotaSnapshot
	require.NoError(t, db.Where("channel_id = ?", 986).Order("id ASC").Find(&snapshots).Error)
	require.Len(t, snapshots, 3)
	for _, snapshot := range snapshots {
		if snapshot.SampleID == "account-a-dual" {
			require.Equal(t, model.ChannelQuotaAccountRef("codex", "account-a"), snapshot.AccountRef)
		}
		if snapshot.SampleID == "account-b-single" {
			require.Equal(t, model.ChannelQuotaAccountRef("codex", "account-b"), snapshot.AccountRef)
			require.Equal(t, "success", snapshot.Status)
		}
	}
}

func setupCodexUsageHistoryTestDB(t *testing.T, channelID int) *gorm.DB {
	t.Helper()
	previousDB := model.DB
	previousMemoryCache := common.MemoryCacheEnabled
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelQuotaSnapshot{}))
	model.DB = db
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		model.DB = previousDB
		common.MemoryCacheEnabled = previousMemoryCache
	})
	require.NoError(t, db.Create(&model.Channel{Id: channelID, Type: constant.ChannelTypeCodex, Name: "codex-history-test", Key: "test"}).Error)
	return db
}

func getCodexUsageHistoryTestData(t *testing.T, channelID int, start, end int64) map[string]any {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(channelID)}}
	ctx.Request = httptest.NewRequest("GET", "/api/channel/"+strconv.Itoa(channelID)+"/codex/usage/history?start="+strconv.FormatInt(start, 10)+"&end="+strconv.FormatInt(end, 10), nil)
	GetCodexChannelUsageHistory(ctx)
	require.Equal(t, 200, recorder.Code, recorder.Body.String())
	var response struct {
		Success bool           `json:"success"`
		Data    map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success, recorder.Body.String())
	return response.Data
}

func getCodexQuotaSeriesHistoryTestData(t *testing.T, channelID int, start, end int64) map[string]any {
	t.Helper()
	query := "range=custom&start=" + strconv.FormatInt(start, 10) +
		"&end=" + strconv.FormatInt(end, 10) +
		"&metric_type=codex_rate_limit&window_type=five_hour" +
		"&source=codex_wham_usage_primary&plan_type=free&unit=percent" +
		"&window_seconds=18000&granularity=raw"
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(channelID)}}
	ctx.Request = httptest.NewRequest("GET", "/api/channel/"+strconv.Itoa(channelID)+"/quota/history?"+query, nil)
	GetChannelQuotaHistory(ctx)
	require.Equal(t, 200, recorder.Code, recorder.Body.String())
	var response struct {
		Success bool           `json:"success"`
		Data    map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success, recorder.Body.String())
	return response.Data
}
