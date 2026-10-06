package model

import (
	"context"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestChannelQuotaAggregateProjectionKeepsAccountIdentityInternal(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}, &ChannelQuotaSnapshot{}))
	previousDB := DB
	DB = db
	t.Cleanup(func() { DB = previousDB })

	channel := Channel{Name: "shared-account-channel", Type: 57}
	require.NoError(t, db.Create(&channel).Error)
	refs := []string{
		ChannelQuotaAccountRef("codex", "account-a"),
		ChannelQuotaAccountRef("codex", "account-b"),
	}
	for index, ref := range refs {
		require.NoError(t, db.Create(&ChannelQuotaSnapshot{
			ChannelId: channel.Id, AccountRef: ref, ObservedAt: 100,
			Available: float64(80 - index*20), Unit: "percent",
			MetricType: "codex_rate_limit", WindowType: "weekly",
			Source: "codex_wham_usage_primary", Status: "success",
		}).Error)
	}

	rows, err := ListChannelQuotaAggregateRows(context.Background(), 100, 100, []int{channel.Id}, "codex_rate_limit", "weekly", "")
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.NotEqual(t, rows[0].AccountRef, rows[1].AccountRef)
	projection, err := common.Marshal(rows)
	require.NoError(t, err)
	for _, ref := range refs {
		require.False(t, strings.Contains(string(projection), ref), "internal account references must not serialize")
	}
}
