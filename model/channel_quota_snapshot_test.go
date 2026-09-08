package model

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestChannelQuotaSeriesCatalogueSQLite(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	runChannelQuotaSeriesCatalogueDatabaseContract(t, db)
}

func runChannelQuotaSeriesCatalogueDatabaseContract(t *testing.T, db *gorm.DB) {
	t.Helper()
	previousDB := DB
	DB = db
	t.Cleanup(func() { DB = previousDB })
	require.NoError(t, db.AutoMigrate(&Channel{}, &ChannelQuotaSnapshot{}))

	live := Channel{Name: "catalogue-live", Key: "fixture-live"}
	errorOnly := Channel{Name: "catalogue-error-only", Key: "fixture-error"}
	deleted := Channel{Name: "catalogue-deleted", Key: "fixture-deleted"}
	for _, channel := range []*Channel{&live, &errorOnly, &deleted} {
		require.NoError(t, db.Create(channel).Error)
	}
	snapshots := []ChannelQuotaSnapshot{
		{
			ChannelId: live.Id, ObservedAt: 100, Available: 90,
			MetricType: "rate_limit", WindowType: "five_hour", Source: "provider_primary",
			PlanType: "pro", Unit: "percent", WindowSeconds: 18000, ResetAt: 500, Status: "success",
		},
		{
			ChannelId: live.Id, ObservedAt: 200, Available: 80,
			MetricType: "rate_limit", WindowType: "five_hour", Source: "provider_primary",
			PlanType: "pro", Unit: "percent", WindowSeconds: 18000, ResetAt: 900, Status: "success",
		},
		{
			ChannelId: live.Id, ObservedAt: 300, Available: 70,
			MetricType: "rate_limit", WindowType: "weekly", Source: "provider_secondary",
			PlanType: "pro", Unit: "percent", Currency: "quota", WindowSeconds: 604800, Status: "success",
		},
		{
			ChannelId: errorOnly.Id, ObservedAt: 400,
			MetricType: "balance", WindowType: "none", Source: "provider_error",
			Unit: "usd", Status: "error", ErrorCode: "query_failed",
		},
		{
			ChannelId: deleted.Id, ObservedAt: 500, Available: 60,
			MetricType: "balance", WindowType: "none", Source: "provider_deleted",
			Unit: "usd", Status: "success",
		},
	}
	require.NoError(t, db.Create(&snapshots).Error)
	require.NoError(t, db.Model(&Channel{}).Where("id = ?", live.Id).Update("name", "catalogue-renamed").Error)
	require.NoError(t, db.Delete(&deleted).Error)
	var orphanCount int64
	require.NoError(t, db.Model(&ChannelQuotaSnapshot{}).Where("channel_id = ?", deleted.Id).Count(&orphanCount).Error)
	require.Equal(t, int64(1), orphanCount)

	result, err := ListChannelQuotaSeriesCatalogue(context.Background(), 10)
	require.NoError(t, err)
	require.Equal(t, 20000, result.ScanLimit)
	require.Equal(t, 5, result.ScannedItems)
	require.True(t, result.SourceComplete)
	require.True(t, result.ItemsComplete)
	require.Equal(t, []ChannelQuotaSeriesCatalogueRow{
		{
			ChannelID: live.Id, ChannelName: "catalogue-renamed", MetricType: "rate_limit",
			WindowType: "five_hour", Source: "provider_primary", PlanType: "pro",
			Unit: "percent", WindowSeconds: 18000,
		},
		{
			ChannelID: live.Id, ChannelName: "catalogue-renamed", MetricType: "rate_limit",
			WindowType: "weekly", Source: "provider_secondary", PlanType: "pro",
			Unit: "percent", Currency: "quota", WindowSeconds: 604800,
		},
	}, result.Rows)

	bounded, err := ListChannelQuotaSeriesCatalogue(context.Background(), 1)
	require.NoError(t, err)
	require.True(t, bounded.SourceComplete)
	require.False(t, bounded.ItemsComplete)
	require.Len(t, bounded.Rows, 1)

	sourceBounded, err := listChannelQuotaSeriesCatalogue(context.Background(), 10, 4)
	require.NoError(t, err)
	require.Equal(t, 4, sourceBounded.ScanLimit)
	require.Equal(t, 4, sourceBounded.ScannedItems)
	require.False(t, sourceBounded.SourceComplete)
	require.False(t, sourceBounded.ItemsComplete)
	require.Len(t, sourceBounded.Rows, 2)
}

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

func TestListChannelQuotaSnapshotsForHistoryKeepsFullRangeOrReportsIncomplete(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&ChannelQuotaSnapshot{}))
	require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&ChannelQuotaSnapshot{}).Error)
	t.Cleanup(func() {
		require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&ChannelQuotaSnapshot{}).Error)
	})

	now := time.Now().Unix()
	for offset, available := range []float64{90, 80, 70} {
		require.NoError(t, RecordChannelQuotaSnapshot(&ChannelQuotaSnapshot{
			ChannelId:     906,
			ObservedAt:    now + int64(offset),
			Available:     available,
			MetricType:    "rate_limit",
			WindowType:    "five_hour",
			Source:        "codex_wham_usage_primary",
			PlanType:      "pro",
			Unit:          "percent",
			WindowSeconds: 18000,
			Status:        "success",
		}))
	}
	filter := ChannelQuotaSnapshotQuery{
		MetricType: "rate_limit",
		Source:     "codex_wham_usage_primary",
		Statuses:   []string{"success"},
	}
	count, err := CountChannelQuotaSnapshotsWithQuery(context.Background(), 906, now-1, now+10, filter)
	require.NoError(t, err)
	require.Equal(t, int64(3), count)

	partial, err := ListChannelQuotaSnapshotsForHistory(context.Background(), 906, now-1, now+10, filter, 2)
	require.NoError(t, err)
	require.False(t, partial.Complete)
	require.Nil(t, partial.Snapshots)

	complete, err := ListChannelQuotaSnapshotsForHistory(context.Background(), 906, now-1, now+10, filter, 3)
	require.NoError(t, err)
	require.True(t, complete.Complete)
	require.Len(t, complete.Snapshots, 3)
	require.Equal(t, now, complete.Snapshots[0].ObservedAt)
	require.Equal(t, now+2, complete.Snapshots[2].ObservedAt)
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
