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
	firstSample := *base
	firstSample.SampleID = "sample-a"
	secondSample := firstSample
	secondSample.SampleID = "sample-b"
	firstSampleKey := channelQuotaSnapshotDedupeKey(&firstSample)
	secondSampleKey := channelQuotaSnapshotDedupeKey(&secondSample)
	require.Len(t, teamKey, 64)
	require.Len(t, proKey, 64)
	require.NotEqual(t, teamKey, proKey)
	require.NotEqual(t, firstSampleKey, secondSampleKey)
}

func TestListChannelQuotaSnapshotsWithQueryKeepsSampleBatchIntact(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&ChannelQuotaSnapshot{}))
	require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&ChannelQuotaSnapshot{}).Error)
	t.Cleanup(func() {
		require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&ChannelQuotaSnapshot{}).Error)
	})

	now := time.Now().Unix()
	for _, snapshot := range []ChannelQuotaSnapshot{
		{ChannelId: 909, ObservedAt: now, SampleID: "sample-old", Available: 90, MetricType: "codex_rate_limit", WindowType: "five_hour", Source: "codex_wham_usage_primary", Unit: "percent", WindowSeconds: 18000, Status: "success"},
		{ChannelId: 909, ObservedAt: now, SampleID: "sample-old", Available: 80, MetricType: "codex_rate_limit", WindowType: "weekly", Source: "codex_wham_usage_secondary", Unit: "percent", WindowSeconds: 604800, Status: "success"},
		{ChannelId: 909, ObservedAt: now + 1, SampleID: "sample-new", Available: 70, MetricType: "codex_rate_limit", WindowType: "five_hour", Source: "codex_wham_usage_primary", Unit: "percent", WindowSeconds: 18000, Status: "success"},
		{ChannelId: 909, ObservedAt: now + 1, SampleID: "sample-new", Available: 60, MetricType: "codex_rate_limit", WindowType: "weekly", Source: "codex_wham_usage_secondary", Unit: "percent", WindowSeconds: 604800, Status: "success"},
	} {
		snapshot := snapshot
		require.NoError(t, RecordChannelQuotaSnapshot(&snapshot))
	}

	rows, err := ListChannelQuotaSnapshotsWithQuery(909, now-1, now+2, ChannelQuotaSnapshotQuery{
		MetricType: "codex_rate_limit",
	}, 1)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, "sample-new", rows[0].SampleID)
	require.Equal(t, "sample-new", rows[1].SampleID)
	require.Equal(t, now+1, rows[0].ObservedAt)
	require.Equal(t, now+1, rows[1].ObservedAt)
}

func TestListLatestChannelQuotaSnapshotBatchOrdersByObservedAtThenID(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&ChannelQuotaSnapshot{}))
	require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&ChannelQuotaSnapshot{}).Error)
	t.Cleanup(func() {
		require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&ChannelQuotaSnapshot{}).Error)
	})

	old := ChannelQuotaSnapshot{ChannelId: 910, ObservedAt: 200, SampleID: "sample-observed-new", Available: 80, MetricType: "codex_rate_limit", WindowType: "five_hour", Source: "codex_wham_usage_primary", Unit: "percent", Status: "success"}
	newerRowID := ChannelQuotaSnapshot{ChannelId: 910, ObservedAt: 100, SampleID: "sample-id-newer", Available: 70, MetricType: "codex_rate_limit", WindowType: "five_hour", Source: "codex_wham_usage_primary", Unit: "percent", Status: "success"}
	require.NoError(t, RecordChannelQuotaSnapshot(&old))
	require.NoError(t, RecordChannelQuotaSnapshot(&newerRowID))
	require.Greater(t, newerRowID.Id, old.Id)

	rows, err := ListLatestChannelQuotaSnapshotBatch(context.Background(), 910, 0, 300, "codex_rate_limit")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "sample-observed-new", rows[0].SampleID)
	require.Equal(t, int64(200), rows[0].ObservedAt)
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

func TestListLatestSuccessfulChannelQuotaSnapshotBatchUsesImmutableSampleID(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&ChannelQuotaSnapshot{}))
	require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&ChannelQuotaSnapshot{}).Error)
	t.Cleanup(func() {
		require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&ChannelQuotaSnapshot{}).Error)
	})

	for _, snapshot := range []ChannelQuotaSnapshot{
		{ChannelId: 907, ObservedAt: 100, SampleID: "sample-old", MetricType: "codex_rate_limit", WindowType: "five_hour", Source: "codex_wham_usage_primary", PlanType: "pro", Unit: "percent", WindowSeconds: 18000, Status: "success"},
		{ChannelId: 907, ObservedAt: 100, SampleID: "sample-old", MetricType: "codex_rate_limit", WindowType: "weekly", Source: "codex_wham_usage_secondary", PlanType: "pro", Unit: "percent", WindowSeconds: 604800, Status: "success"},
		{ChannelId: 907, ObservedAt: 100, SampleID: "sample-current", MetricType: "codex_rate_limit", WindowType: "weekly", Source: "codex_wham_usage_primary", PlanType: "team", Unit: "percent", WindowSeconds: 604800, Status: "success"},
		{ChannelId: 907, ObservedAt: 100, SampleID: "sample-error", MetricType: "codex_rate_limit", WindowType: "none", Source: "codex_wham_usage", Unit: "percent", Status: "error"},
	} {
		snapshot := snapshot
		require.NoError(t, RecordChannelQuotaSnapshot(&snapshot))
	}

	rows, err := ListLatestSuccessfulChannelQuotaSnapshotBatch(
		context.Background(),
		907,
		"codex_rate_limit",
		[]string{"codex_wham_usage_primary", "codex_wham_usage_secondary"},
	)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "sample-current", rows[0].SampleID)
	require.Equal(t, "codex_wham_usage_primary", rows[0].Source)
	require.Equal(t, "team", rows[0].PlanType)

	latest, err := ListLatestChannelQuotaSnapshotBatch(context.Background(), 907, 99, 101, "codex_rate_limit")
	require.NoError(t, err)
	require.Len(t, latest, 1)
	require.Equal(t, "sample-error", latest[0].SampleID)
	require.Equal(t, "error", latest[0].Status)
}

func TestLatestChannelQuotaSnapshotBatchReadsLegacyNullSampleID(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&ChannelQuotaSnapshot{}))
	require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&ChannelQuotaSnapshot{}).Error)
	t.Cleanup(func() {
		require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&ChannelQuotaSnapshot{}).Error)
	})
	require.NoError(t, DB.Exec(`
		INSERT INTO channel_quota_snapshots
		(channel_id, observed_at, available, unit, currency, metric_type, window_type, plan_type, window_seconds, reset_at, source, status, sample_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, ?)`,
		908, 100, 80, "percent", "", "codex_rate_limit", "five_hour", "", 0, 0, "codex_wham_usage_primary", "success", time.Now(),
	).Error)

	latest, err := ListLatestChannelQuotaSnapshotBatch(context.Background(), 908, 99, 101, "codex_rate_limit")
	require.NoError(t, err)
	require.Len(t, latest, 1)
	require.Empty(t, latest[0].SampleID)

	duplicate := &ChannelQuotaSnapshot{
		ChannelId: 908, ObservedAt: 100, Available: 80, Unit: "percent", MetricType: "codex_rate_limit",
		WindowType: "five_hour", Source: "codex_wham_usage_primary", Status: "success",
	}
	require.NoError(t, RecordChannelQuotaSnapshot(duplicate))
	var count int64
	require.NoError(t, DB.Model(&ChannelQuotaSnapshot{}).Where("channel_id = ?", 908).Count(&count).Error)
	require.Equal(t, int64(1), count)
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

func TestChannelQuotaAccountRefIsStableAndNonReversible(t *testing.T) {
	first := ChannelQuotaAccountRef("codex", "account-123")
	require.Len(t, first, 64)
	require.Equal(t, first, ChannelQuotaAccountRef(" codex ", " account-123 "))
	require.NotEqual(t, first, ChannelQuotaAccountRef("codex", "account-456"))
	require.Empty(t, ChannelQuotaAccountRef("codex", ""))
	require.Empty(t, ChannelQuotaAccountRef("", "account-123"))
}

func TestRecordChannelQuotaSnapshotSeparatesAccountReferences(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&ChannelQuotaSnapshot{}))
	require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&ChannelQuotaSnapshot{}).Error)
	t.Cleanup(func() {
		require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&ChannelQuotaSnapshot{}).Error)
	})

	now := time.Now().Unix()
	first := &ChannelQuotaSnapshot{
		ChannelId: 905, ObservedAt: now, Available: 80, MetricType: "rate_limit",
		WindowType: "five_hour", Source: "codex", Unit: "percent", WindowSeconds: 18000,
		AccountRef: ChannelQuotaAccountRef("codex", "account-a"),
	}
	second := *first
	second.AccountRef = ChannelQuotaAccountRef("codex", "account-b")
	require.NoError(t, RecordChannelQuotaSnapshot(first))
	require.NoError(t, RecordChannelQuotaSnapshot(&second))
	require.NotEqual(t, first.Id, second.Id)
	require.NotEqual(t, first.DedupeKey, second.DedupeKey)

	var count int64
	require.NoError(t, DB.Model(&ChannelQuotaSnapshot{}).Where("channel_id = ?", 905).Count(&count).Error)
	require.Equal(t, int64(2), count)
}

func pointerToFloat64(value float64) *float64 {
	return &value
}
