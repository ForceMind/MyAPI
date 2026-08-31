package model

import (
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestEnsureChannelQuotaSnapshotDedupeIndexSQLite(t *testing.T) {
	previousDB := DB
	previousMainDatabaseType := common.MainDatabaseType()
	t.Cleanup(func() {
		DB = previousDB
		common.SetMainDatabaseType(previousMainDatabaseType)
	})

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)

	// Simulate an existing installation whose table predates dedupe_key.
	require.NoError(t, DB.Exec(`CREATE TABLE channel_quota_snapshots (id INTEGER PRIMARY KEY, available REAL)`).Error)
	require.NoError(t, DB.AutoMigrate(&ChannelQuotaSnapshot{}))
	require.NoError(t, ensureChannelQuotaSnapshotDedupeIndex())
	require.True(t, DB.Migrator().HasIndex(&ChannelQuotaSnapshot{}, "idx_channel_quota_dedupe_key"))

	// Re-running the migration must be idempotent.
	require.NoError(t, ensureChannelQuotaSnapshotDedupeIndex())
}
