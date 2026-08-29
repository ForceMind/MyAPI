package model

import (
	"context"
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
