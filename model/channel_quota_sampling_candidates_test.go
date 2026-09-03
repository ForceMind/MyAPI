package model

import (
	"context"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestQuotaSamplingCandidatesRotateByLastAttemptRegardlessOfPriority(t *testing.T) {
	previousDB := DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}, &ChannelQuotaSnapshot{}))
	DB = db
	t.Cleanup(func() { DB = previousDB })

	highPriority := int64(100)
	channels := []Channel{
		{Id: 1, Name: "never sampled", Key: "fixture", Status: common.ChannelStatusEnabled},
		{Id: 2, Name: "older failure", Key: "fixture", Status: common.ChannelStatusEnabled},
		{Id: 3, Name: "recent success", Key: "fixture", Status: common.ChannelStatusEnabled, Priority: &highPriority},
		{Id: 4, Name: "disabled", Key: "fixture", Status: common.ChannelStatusManuallyDisabled},
		{Id: 5, Name: "multi key", Key: "fixture", Status: common.ChannelStatusEnabled, ChannelInfo: ChannelInfo{IsMultiKey: true}},
	}
	require.NoError(t, db.Create(&channels).Error)
	require.NoError(t, RecordChannelQuotaSnapshot(&ChannelQuotaSnapshot{ChannelId: 2, ObservedAt: 100, Status: "error"}))
	require.NoError(t, RecordChannelQuotaSnapshot(&ChannelQuotaSnapshot{ChannelId: 3, ObservedAt: 200, Status: "success"}))

	for index, expectedID := range []int{1, 2, 3} {
		candidates, queryErr := GetChannelsForQuotaSnapshotSync(1)
		require.NoError(t, queryErr)
		require.Len(t, candidates, 1)
		require.Equal(t, expectedID, candidates[0].Id)
		// Failure/unsupported observations also advance the turn. Otherwise an
		// unavailable provider could consume every later batch's first slot.
		require.NoError(t, RecordChannelQuotaSnapshot(&ChannelQuotaSnapshot{
			ChannelId: expectedID, ObservedAt: int64(300 + index), Status: "unsupported",
		}))
	}
}

func TestQuotaSamplingCandidatesSkipMultiKeyWithoutExhaustingEligibleLimit(t *testing.T) {
	previousDB := DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}, &ChannelQuotaSnapshot{}))
	DB = db
	t.Cleanup(func() { DB = previousDB })

	channels := []Channel{
		{Id: 1, Key: "fixture", Status: common.ChannelStatusEnabled, ChannelInfo: ChannelInfo{IsMultiKey: true}},
		{Id: 2, Key: "fixture", Status: common.ChannelStatusEnabled, ChannelInfo: ChannelInfo{IsMultiKey: true}},
		{Id: 3, Key: "fixture", Status: common.ChannelStatusEnabled},
		{Id: 4, Key: "fixture", Status: common.ChannelStatusEnabled},
	}
	require.NoError(t, db.Create(&channels).Error)
	candidates, err := GetChannelsForQuotaSnapshotSync(2)
	require.NoError(t, err)
	require.Len(t, candidates, 2)
	require.Equal(t, 3, candidates[0].Id)
	require.Equal(t, 4, candidates[1].Id)
}

func TestQuotaSamplingCandidatesReachAccountsAfterLargeMultiKeyPrefixInOneQuery(t *testing.T) {
	previousDB, previousType := DB, common.MainDatabaseType()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}, &ChannelQuotaSnapshot{}))
	DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	t.Cleanup(func() { DB = previousDB; common.SetMainDatabaseType(previousType) })
	channels := make([]Channel, 2051)
	for index := range channels {
		channels[index] = Channel{Id: index + 1, Key: "fixture", Status: common.ChannelStatusEnabled, ChannelInfo: ChannelInfo{IsMultiKey: index < 2048}}
	}
	require.NoError(t, db.CreateInBatches(&channels, 100).Error)
	queries := 0
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("quota_candidates_count", func(tx *gorm.DB) {
		if tx.Statement.Table == "channels" {
			queries++
		}
	}))
	for index, expectedID := range []int{2049, 2050, 2051} {
		queries = 0
		candidates, err := GetChannelsForQuotaSnapshotSyncContext(context.Background(), 1)
		require.NoError(t, err)
		require.Len(t, candidates, 1)
		require.Equal(t, expectedID, candidates[0].Id)
		require.False(t, candidates[0].ChannelInfo.IsMultiKey)
		require.Equal(t, 1, queries, "excluded prefixes must not cause application-side pagination")
		require.NoError(t, RecordChannelQuotaSnapshot(&ChannelQuotaSnapshot{ChannelId: expectedID, ObservedAt: int64(100 + index), Status: "error"}))
	}
}

func TestQuotaSamplingCandidatesHandleLegacyAndMalformedSQLiteJSON(t *testing.T) {
	previousDB, previousType := DB, common.MainDatabaseType()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}, &ChannelQuotaSnapshot{}))
	DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	t.Cleanup(func() { DB = previousDB; common.SetMainDatabaseType(previousType) })
	metadata := []any{nil, []byte(`{}`), []byte(`{"is_multi_key":false}`), []byte(`{"is_multi_key":true}`), []byte(`not-json`), []byte(`[]`), []byte(`{"is_multi_key":"false"}`)}
	for index, value := range metadata {
		require.NoError(t, db.Create(&Channel{Id: index + 1, Key: "fixture", Status: common.ChannelStatusEnabled}).Error)
		require.NoError(t, db.Model(&Channel{}).Where("id = ?", index+1).Update("channel_info", value).Error)
	}
	candidates, err := GetChannelsForQuotaSnapshotSync(10)
	require.NoError(t, err)
	require.Len(t, candidates, 3)
	require.Equal(t, []int{1, 2, 3}, []int{candidates[0].Id, candidates[1].Id, candidates[2].Id})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = GetChannelsForQuotaSnapshotSyncContext(ctx, 1)
	require.ErrorIs(t, err, context.Canceled)
}

func TestQuotaSamplingCandidatesGenerateDialectSpecificSingleQuery(t *testing.T) {
	previousDB, previousType := DB, common.MainDatabaseType()
	t.Cleanup(func() { DB = previousDB; common.SetMainDatabaseType(previousType) })
	sqliteDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := sqliteDB.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	// Reuse an inert connection only for SQL generation; these two cases do
	// not claim runtime verification against MySQL or PostgreSQL servers.
	for _, test := range []struct {
		name      string
		kind      common.DatabaseType
		dialector gorm.Dialector
		operator  string
	}{
		{"sqlite", common.DatabaseTypeSQLite, sqlite.Open(":memory:"), "json_valid(CAST(channel_info AS TEXT))"},
		{"mysql", common.DatabaseTypeMySQL, mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), "JSON_UNQUOTE(JSON_EXTRACT"},
		{"postgres", common.DatabaseTypePostgreSQL, postgres.New(postgres.Config{Conn: sqlDB}), "channel_info->>'is_multi_key'"},
	} {
		t.Run(test.name, func(t *testing.T) {
			dryDB, err := gorm.Open(test.dialector, &gorm.Config{DryRun: true, DisableAutomaticPing: true})
			require.NoError(t, err)
			DB = dryDB
			common.SetMainDatabaseType(test.kind)
			var generatedSQL string
			calls := 0
			require.NoError(t, dryDB.Callback().Query().After("gorm:query").Register("quota_candidates_sql", func(tx *gorm.DB) { generatedSQL = tx.Statement.SQL.String(); calls++ }))
			_, err = GetChannelsForQuotaSnapshotSync(2)
			require.NoError(t, err)
			require.Equal(t, 1, calls)
			require.Contains(t, generatedSQL, test.operator)
			require.Contains(t, generatedSQL, "MAX(observed_at)")
			require.Contains(t, generatedSQL, "LIMIT")
			require.NotContains(t, generatedSQL, "OFFSET")
		})
	}
}
