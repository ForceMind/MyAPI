package model

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSubscriptionShareRetentionKeepsEvidenceAfterActiveCursor(t *testing.T) {
	db := openQuotaIdentityDB(t)
	models := []any{&ChannelQuotaSnapshot{}}
	models = append(models, SubscriptionShareModels()...)
	require.NoError(t, db.AutoMigrate(models...))
	previousDB := DB
	DB = db
	t.Cleanup(func() { DB = previousDB })

	subject, err := newChannelQuotaIdentitySubjectRef()
	require.NoError(t, err)
	total := float64(100)
	used := float64(10)
	rows := []ChannelQuotaSnapshot{
		{ChannelId: 1, ObservedAt: 90, Available: 1, MetricType: "unrelated", Source: "old_unrelated", Status: "success"},
		{ChannelId: 1, SubjectRef: subject, IdentityQuality: ChannelQuotaIdentityQualityProviderConfirmed, ObservedAt: 100, Available: 90, Used: &used, Total: &total, MetricType: "codex_rate_limit", WindowType: "five_hour", PlanType: "plus", WindowSeconds: 18000, ResetAt: 1000, Source: codexQuotaWindowPrimarySource, Status: "success", Unit: "percent"},
		{ChannelId: 1, SubjectRef: subject, IdentityQuality: ChannelQuotaIdentityQualityProviderConfirmed, ObservedAt: 110, Available: 90, Used: &used, Total: &total, MetricType: "codex_rate_limit", WindowType: "five_hour", PlanType: "plus", WindowSeconds: 18000, ResetAt: 1000, Source: codexQuotaWindowPrimarySource, Status: "success", Unit: "percent"},
		{ChannelId: 1, SubjectRef: subject, IdentityQuality: ChannelQuotaIdentityQualityProviderConfirmed, ObservedAt: 120, MetricType: "codex_rate_limit", WindowType: "five_hour", PlanType: "plus", WindowSeconds: 18000, ResetAt: 1000, Source: codexQuotaWindowPrimarySource, Status: "error", ErrorCode: "upstream_http", Unit: "percent"},
		{ChannelId: 1, SubjectRef: subject, IdentityQuality: ChannelQuotaIdentityQualityProviderConfirmed, ObservedAt: 130, Available: 70, MetricType: "descriptor_probe", WindowType: "weekly", PlanType: "plus", WindowSeconds: 604800, ResetAt: 2000, Source: codexQuotaWindowSecondarySource, Status: "success", Unit: "percent"},
	}
	for index := range rows {
		require.NoError(t, db.Create(&rows[index]).Error)
	}

	pool := SubscriptionSharePool{
		DescriptorKey:      "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		SubjectRef:         subject,
		SnapshotID:         rows[1].Id,
		ChannelID:          1,
		ChannelName:        "retention fixture",
		WindowSource:       codexQuotaWindowPrimarySource,
		WindowType:         "five_hour",
		WindowSeconds:      18000,
		PlanType:           "plus",
		Revision:           1,
		CurrentGeneration:  1,
		SelectedObservedAt: 100,
		ConfiguredAt:       100,
		MaxAgeSeconds:      300,
		Enabled:            true,
		Approximate:        true,
	}
	require.NoError(t, db.Create(&pool).Error)
	require.NoError(t, db.Create(&SubscriptionShareGeneration{
		PoolID:         pool.ID,
		Generation:     1,
		BaselineTime:   100,
		LastUsedMicro:  10_000_000,
		LastSnapshotID: rows[1].Id,
		ObservedAt:     100,
		ResetAt:        1000,
		State:          SubscriptionShareStateReady,
		Reason:         "eligible",
	}).Error)
	receipt := SubscriptionShareSampleApplication{
		PoolID:          pool.ID,
		Generation:      1,
		SnapshotID:      rows[1].Id,
		FromSnapshotID:  rows[1].Id,
		BeforeUsedMicro: 10_000_000,
		AfterUsedMicro:  10_000_000,
		AllocationJSON:  "[]",
		Reason:          "baseline",
		CreatedAt:       100,
	}
	require.NoError(t, db.Create(&receipt).Error)

	deleted, err := DeleteOldChannelQuotaSnapshotBatch(context.Background(), 1000, 100)
	require.NoError(t, err)
	assert.EqualValues(t, 1, deleted)

	var remaining []ChannelQuotaSnapshot
	require.NoError(t, db.Order("observed_at ASC, id ASC").Find(&remaining).Error)
	require.Len(t, remaining, 4)
	assert.Equal(t, []int64{100, 110, 120, 130}, []int64{remaining[0].ObservedAt, remaining[1].ObservedAt, remaining[2].ObservedAt, remaining[3].ObservedAt})

	var saved SubscriptionShareSampleApplication
	require.NoError(t, db.Where("pool_id = ? AND generation = ? AND snapshot_id = ?", pool.ID, 1, rows[1].Id).Take(&saved).Error)
	assert.Equal(t, "baseline", saved.Reason)
	assert.Equal(t, int64(10_000_000), saved.AfterUsedMicro)
}
