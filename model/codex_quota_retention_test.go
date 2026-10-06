package model

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCodexQuotaRetentionKeepsLatestExhaustionEvidence(t *testing.T) {
	db := openQuotaIdentityDB(t)
	require.NoError(t, db.AutoMigrate(&ChannelQuotaSnapshot{}))
	previousDB := DB
	DB = db
	t.Cleanup(func() { DB = previousDB })
	account := ChannelQuotaAccountRef("codex", "retention-account")
	rows := []ChannelQuotaSnapshot{
		{ChannelId: 1, AccountRef: account, ObservedAt: 50, Available: 80, Source: codexQuotaWindowPrimarySource},
		{ChannelId: 1, AccountRef: account, ObservedAt: 60, Status: "error", Source: CodexQuotaRouteLimitSource, ErrorCode: "ordinary_rate_limit"},
		{ChannelId: 1, AccountRef: account, ObservedAt: 100, Status: "error", Source: CodexQuotaRouteLimitSource, ErrorCode: CodexQuotaRouteLimitCode},
	}
	for _, row := range rows {
		recordCodexRouteSnapshot(t, db, row)
	}
	deleted, err := DeleteOldChannelQuotaSnapshotBatch(context.Background(), 500, 20)
	require.NoError(t, err)
	require.EqualValues(t, 1, deleted)
	state, err := ReadCodexQuotaRouteState(context.Background(), db, "", account, 1000)
	require.NoError(t, err)
	require.True(t, state.Blocked, "cleanup must not release an account with an unknown reset")
}
