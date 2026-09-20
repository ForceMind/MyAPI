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
	// Deliberately disagree with the process-wide setting: the migration must
	// use the actual handle dialect when choosing the idempotent DDL form.
	common.SetMainDatabaseType(common.DatabaseTypeMySQL)

	// Simulate an existing installation whose table predates dedupe_key.
	require.NoError(t, DB.Exec(`CREATE TABLE channel_quota_snapshots (id INTEGER PRIMARY KEY, available REAL)`).Error)
	require.NoError(t, DB.AutoMigrate(&ChannelQuotaSnapshot{}))
	require.True(t, DB.Migrator().HasColumn(&ChannelQuotaSnapshot{}, "SampleID"))
	require.True(t, DB.Migrator().HasIndex(&ChannelQuotaSnapshot{}, "idx_channel_quota_sample"))
	require.NoError(t, ensureChannelQuotaSnapshotDedupeIndex())
	require.True(t, DB.Migrator().HasIndex(&ChannelQuotaSnapshot{}, "idx_channel_quota_dedupe_key"))

	// Re-running the migration must be idempotent.
	require.NoError(t, ensureChannelQuotaSnapshotDedupeIndex())
}

func TestEnsureChannelQuotaSnapshotDedupeIndexPreservesRealCreateErrors(t *testing.T) {
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

	// A duplicate value must make the UNIQUE index DDL fail. The concurrent
	// MySQL duplicate-index path may be ignored only when the index now exists;
	// unrelated DDL failures must still reach the caller.
	require.NoError(t, DB.Exec(`CREATE TABLE channel_quota_snapshots (id INTEGER PRIMARY KEY, dedupe_key TEXT)`).Error)
	require.NoError(t, DB.Exec(`INSERT INTO channel_quota_snapshots (dedupe_key) VALUES ('duplicate'), ('duplicate')`).Error)

	require.Error(t, ensureChannelQuotaSnapshotDedupeIndex())
	require.False(t, DB.Migrator().HasIndex(&ChannelQuotaSnapshot{}, "idx_channel_quota_dedupe_key"))
}
