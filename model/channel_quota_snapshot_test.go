package model

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestChannelQuotaSnapshotsAreNormalizedAndBounded(t *testing.T) {
	require.NotNil(t, DB)
	require.NoError(t, DB.AutoMigrate(&ChannelQuotaSnapshot{}))
	require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&ChannelQuotaSnapshot{}).Error)
	t.Cleanup(func() {
		require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&ChannelQuotaSnapshot{}).Error)
	})

	now := time.Now().Unix()
	available := 12.5
	require.NoError(t, RecordChannelQuotaSnapshot(&ChannelQuotaSnapshot{
		ChannelId:  901,
		ObservedAt: now - 10,
		Available:  available,
		Used:       &available,
		Status:     "success",
	}))
	require.NoError(t, RecordChannelQuotaSnapshot(&ChannelQuotaSnapshot{
		ChannelId:  901,
		ObservedAt: now,
		Status:     "error",
		ErrorCode:  "query_failed",
	}))

	rows, err := ListChannelQuotaSnapshots(901, now-60, now+60, "", "", 1)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "usd", rows[0].Unit)
	require.Equal(t, "balance", rows[0].MetricType)
	require.Equal(t, "none", rows[0].WindowType)
}

func TestListChannelQuotaSnapshotsKeepsMostRecentPoints(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&ChannelQuotaSnapshot{}))
	require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&ChannelQuotaSnapshot{}).Error)
	now := time.Now().Unix()
	for index := int64(0); index < 3; index++ {
		require.NoError(t, RecordChannelQuotaSnapshot(&ChannelQuotaSnapshot{
			ChannelId:  902,
			ObservedAt: now + index,
			Available:  100 - float64(index),
			Status:     "success",
		}))
	}
	rows, err := ListChannelQuotaSnapshots(902, now, now+3, "", "", 2)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, now+1, rows[0].ObservedAt)
	require.Equal(t, now+2, rows[1].ObservedAt)
}

func TestRecordChannelQuotaSnapshotDeduplicatesSeriesTimeBucket(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&ChannelQuotaSnapshot{}))
	require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&ChannelQuotaSnapshot{}).Error)
	t.Cleanup(func() {
		require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&ChannelQuotaSnapshot{}).Error)
	})

	now := time.Now().Unix()
	first := &ChannelQuotaSnapshot{
		ChannelId:     904,
		ObservedAt:    now,
		Available:     80,
		MetricType:    "rate_limit",
		WindowType:    "five_hour",
		Source:        "codex",
		PlanType:      "team",
		Unit:          "percent",
		WindowSeconds: 18000,
	}
	second := *first
	second.Available = 70
	require.NoError(t, RecordChannelQuotaSnapshot(first))
	require.NoError(t, RecordChannelQuotaSnapshot(&second))
	require.NotZero(t, first.Id)
	require.Equal(t, first.Id, second.Id)
	require.NotNil(t, first.DedupeKey)
	require.NotEmpty(t, *first.DedupeKey)
	require.Equal(t, first.DedupeKey, second.DedupeKey)

	var rows []ChannelQuotaSnapshot
	require.NoError(t, DB.Where("channel_id = ?", 904).Find(&rows).Error)
	require.Len(t, rows, 1)
	require.Equal(t, 80.0, rows[0].Available)
}

func TestChannelQuotaSnapshotDedupeKeySeparatesProviderSeries(t *testing.T) {
	base := &ChannelQuotaSnapshot{
		ChannelId:     905,
		ObservedAt:    1234,
		MetricType:    "rate_limit",
		WindowType:    "five_hour",
		Source:        "codex",
		PlanType:      "team",
		Unit:          "percent",
		WindowSeconds: 18000,
	}
	teamKey := channelQuotaSnapshotDedupeKey(base)
	pro := *base
	pro.PlanType = "pro"
	proKey := channelQuotaSnapshotDedupeKey(&pro)
	require.Len(t, teamKey, 64)
	require.Len(t, proKey, 64)
	require.NotEqual(t, teamKey, proKey)
}

func TestListChannelQuotaSnapshotsWithQuerySelectsOneProviderSeries(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&ChannelQuotaSnapshot{}))
	require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&ChannelQuotaSnapshot{}).Error)
	t.Cleanup(func() {
		require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&ChannelQuotaSnapshot{}).Error)
	})

	now := time.Now().Unix()
	windowSeconds := int64(18000)
	for index, available := range []float64{90, 80} {
		require.NoError(t, RecordChannelQuotaSnapshot(&ChannelQuotaSnapshot{
			ChannelId:     903,
			ObservedAt:    now + int64(index),
			Available:     available,
			MetricType:    "rate_limit",
			WindowType:    "five_hour",
			Source:        "codex",
			PlanType:      "team",
			Unit:          "percent",
			WindowSeconds: windowSeconds,
			Status:        "success",
		}))
	}
	require.NoError(t, RecordChannelQuotaSnapshot(&ChannelQuotaSnapshot{
		ChannelId:     903,
		ObservedAt:    now + 2,
		Available:     50,
		MetricType:    "rate_limit",
		WindowType:    "five_hour",
		Source:        "codex",
		PlanType:      "pro",
		Unit:          "percent",
		WindowSeconds: windowSeconds,
		Status:        "success",
	}))

	rows, err := ListChannelQuotaSnapshotsWithQuery(903, now-1, now+3, ChannelQuotaSnapshotQuery{
		MetricType:    "rate_limit",
		WindowType:    "five_hour",
		Source:        "codex",
		PlanType:      "team",
		Unit:          "percent",
		WindowSeconds: &windowSeconds,
	}, 10)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	for _, row := range rows {
		require.Equal(t, "team", row.PlanType)
	}
}

func TestDeleteOldChannelQuotaSnapshotBatchHonorsCutoffAndLimit(t *testing.T) {
	require.NotNil(t, DB)
	require.NoError(t, DB.AutoMigrate(&ChannelQuotaSnapshot{}))
	require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&ChannelQuotaSnapshot{}).Error)
	t.Cleanup(func() {
		require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&ChannelQuotaSnapshot{}).Error)
	})

	now := time.Now().Unix()
	for i := 0; i < 3; i++ {
		require.NoError(t, RecordChannelQuotaSnapshot(&ChannelQuotaSnapshot{
			ChannelId:  902,
			ObservedAt: now - int64(100+i),
			Available:  float64(i),
		}))
	}
	require.NoError(t, RecordChannelQuotaSnapshot(&ChannelQuotaSnapshot{
		ChannelId:  902,
		ObservedAt: now,
		Available:  99,
	}))

	deleted, err := DeleteOldChannelQuotaSnapshotBatch(context.Background(), now-50, 2)
	require.NoError(t, err)
	require.Equal(t, int64(2), deleted)
	deleted, err = DeleteOldChannelQuotaSnapshotBatch(context.Background(), now-50, 2)
	require.NoError(t, err)
	require.Equal(t, int64(1), deleted)
	deleted, err = DeleteOldChannelQuotaSnapshotBatch(context.Background(), now-50, 2)
	require.NoError(t, err)
	require.Zero(t, deleted)

	var remaining []ChannelQuotaSnapshot
	require.NoError(t, DB.Where("channel_id = ?", 902).Order("observed_at ASC").Find(&remaining).Error)
	require.Len(t, remaining, 1)
	require.Equal(t, float64(99), remaining[0].Available)
}

func TestRecordChannelQuotaSnapshotRejectsNonFiniteValues(t *testing.T) {
	require.NotNil(t, DB)
	for _, snapshot := range []*ChannelQuotaSnapshot{
		{Available: math.NaN()},
		{Available: math.Inf(1)},
		{Available: 1, Used: pointerToFloat64(math.Inf(-1))},
		{Available: 1, Total: pointerToFloat64(math.NaN())},
	} {
		require.Error(t, RecordChannelQuotaSnapshot(snapshot))
	}
}

func pointerToFloat64(value float64) *float64 {
	return &value
}
