package model

import (
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
