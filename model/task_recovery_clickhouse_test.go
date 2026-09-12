package model

import (
	"context"
	"errors"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/clickhouse"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	b2ClickHouseFixtureDatabase = "myapi_b2_clickhouse_test"
	b2ClickHouseFixtureUser     = "b2_fixture"
	b2ClickHouseFixturePassword = "b2-fixture-password"
	b2ClickHouseFixtureVersion  = "24.8"
)

func b2ClickHouseDatabaseDialector(dsn string) (gorm.Dialector, error) {
	unsafeTarget := errors.New("B2 ClickHouse tests require a literal loopback address and database " + b2ClickHouseFixtureDatabase)
	parsed, err := url.Parse(dsn)
	if err != nil {
		return nil, errors.New("invalid B2 ClickHouse fixture DSN")
	}
	host, port, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		return nil, unsafeTarget
	}
	portNumber, err := strconv.ParseUint(port, 10, 16)
	if err != nil || portNumber == 0 {
		return nil, unsafeTarget
	}
	if parsed.User == nil {
		return nil, unsafeTarget
	}
	password, hasPassword := parsed.User.Password()
	// clickhouse-go v2 accepts comma-separated hosts and query-controlled
	// connection/auth settings. This fixture deliberately accepts one native
	// endpoint with no query overrides.
	if parsed.Scheme != "clickhouse" || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" ||
		parsed.User.Username() != b2ClickHouseFixtureUser || !hasPassword || password != b2ClickHouseFixturePassword ||
		!net.ParseIP(host).IsLoopback() || parsed.Path != "/"+b2ClickHouseFixtureDatabase || parsed.RawPath != "" {
		return nil, unsafeTarget
	}
	return clickhouse.Open(normalizeClickHouseDSN(dsn)), nil
}

func TestB2ClickHouseDatabaseTargetSafety(t *testing.T) {
	for _, dsn := range []string{
		"clickhouse://b2_fixture:b2-fixture-password@192.0.2.1:9000/myapi_b2_clickhouse_test",
		"clickhouse://b2_fixture:b2-fixture-password@localhost:9000/myapi_b2_clickhouse_test",
		"clickhouse://b2_fixture:b2-fixture-password@127.0.0.1:9000/myapi_b2_test",
		"postgres://b2_fixture:b2-fixture-password@127.0.0.1:9000/myapi_b2_clickhouse_test",
		"clickhouse://b2_fixture:b2-fixture-password@127.0.0.1:9000,192.0.2.1:9000/myapi_b2_clickhouse_test",
		"clickhouse://b2_fixture:b2-fixture-password@127.0.0.1:9000/myapi_b2_clickhouse_test?alt_hosts=192.0.2.1:9000",
		"clickhouse://b2_fixture:b2-fixture-password@127.0.0.1:9000/myapi_b2_clickhouse_test?database=other",
		"clickhouse://b2_fixture:b2-fixture-password@127.0.0.1:9000/myapi_b2_clickhouse_test?protocol=http",
		"clickhouse://b2_fixture:b2-fixture-password@127.0.0.1:9000/myapi_b2_clickhouse_test?username=default&password=other",
		"clickhouse://b2_fixture:b2-fixture-password@127.0.0.1:9000/myapi_b2_clickhouse_test?http_proxy=http://192.0.2.1:8080",
		"clickhouse://default:@127.0.0.1:9000/myapi_b2_clickhouse_test",
	} {
		t.Run(dsn, func(t *testing.T) {
			_, err := b2ClickHouseDatabaseDialector(dsn)
			require.Error(t, err)
		})
	}
	_, err := b2ClickHouseDatabaseDialector("clickhouse://b2_fixture:b2-fixture-password@127.0.0.1:9000/myapi_b2_clickhouse_test")
	require.NoError(t, err)
}

func TestB2ClickHouseConfiguredDatabase(t *testing.T) {
	if os.Getenv("MYAPI_B2_CLICKHOUSE_TESTS") != "1" {
		t.Skip("disposable B2 ClickHouse test requires MYAPI_B2_CLICKHOUSE_TESTS=1")
	}
	dsn := os.Getenv("MYAPI_B2_CLICKHOUSE_DSN")
	require.NotEmpty(t, dsn, "MYAPI_B2_CLICKHOUSE_DSN must be configured when MYAPI_B2_CLICKHOUSE_TESTS=1")
	dialector, err := b2ClickHouseDatabaseDialector(dsn)
	require.NoError(t, err)
	db, err := gorm.Open(dialector, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })

	var version string
	require.NoError(t, db.Raw("SELECT version()").Scan(&version).Error)
	require.True(t, strings.HasPrefix(version, b2ClickHouseFixtureVersion+"."), "B2 fixture must run ClickHouse %s.x, got %s", b2ClickHouseFixtureVersion, version)

	var tableCount int64
	require.NoError(t, db.Raw("SELECT count() FROM system.tables WHERE database = currentDatabase()").Scan(&tableCount).Error)
	require.Zero(t, tableCount, "refusing a non-empty B2 ClickHouse fixture database; CI owns its lifecycle and no tables are dropped")

	originalLogDB := LOG_DB
	originalLogDatabaseType := common.LogDatabaseType()
	LOG_DB = db
	common.SetLogDatabaseType(common.DatabaseTypeClickHouse)
	t.Cleanup(func() {
		LOG_DB = originalLogDB
		common.SetLogDatabaseType(originalLogDatabaseType)
	})
	t.Setenv("LOG_SQL_CLICKHOUSE_TTL_DAYS", "0")

	// New deployments receive all projection identity columns and the aggregate
	// canonical projection through the production LOG_DB migration entry.
	require.NoError(t, migrateLOGDB())
	var identityTableCount int64
	require.NoError(t, db.Raw(
		"SELECT count() FROM system.tables WHERE database = currentDatabase() AND name = ?",
		clickHouseIdentityTable,
	).Scan(&identityTableCount).Error)
	require.Equal(t, int64(1), identityTableCount)
	for _, column := range []string{"billing_event_id", "billing_projection_digest", "log_row_key"} {
		hasColumn, err := clickHouseLogColumnExists(column)
		require.NoError(t, err)
		require.True(t, hasColumn, "missing ClickHouse column %s", column)
	}
	projectionExists, err := clickHouseCanonicalProjectionExists()
	require.NoError(t, err)
	require.True(t, projectionExists)
	runLogDedupDatabaseContract(t, db)
	require.NoError(t, db.Exec("RENAME TABLE logs TO logs_b2_new_fixture").Error)

	// Build the old shape from the canonical create statement, then exercise the
	// production helper's additive column/projection definition upgrade and repeatability on retained rows.
	legacyCreateSQL := clickHouseLogCreateTableSQL(0)
	legacyCreateSQL = strings.Replace(legacyCreateSQL, "\tbilling_event_id String DEFAULT '',\n", "", 1)
	legacyCreateSQL = strings.Replace(legacyCreateSQL, "\tbilling_projection_digest String DEFAULT '',\n", "", 1)
	legacyCreateSQL = strings.Replace(legacyCreateSQL, "\tlog_row_key String DEFAULT '',\n", "", 1)
	legacyCreateSQL = strings.Replace(
		legacyCreateSQL,
		"\tother String DEFAULT '',\n\t"+clickHouseCanonicalProjectionDefinition()+"\n",
		"\tother String DEFAULT ''\n",
		1,
	)
	legacyCreateSQL = strings.Replace(legacyCreateSQL, "ORDER BY (created_at, request_id, log_row_key)", "ORDER BY (created_at, request_id)", 1)
	require.NotEqual(t, clickHouseLogCreateTableSQL(0), legacyCreateSQL)
	require.NoError(t, db.Exec(legacyCreateSQL).Error)
	require.NoError(t, db.Exec("INSERT INTO logs (id, content) VALUES (?, ?)", 7, "legacy-b2-clickhouse-log").Error)

	require.ErrorIs(t, migrateLOGDB(), ErrLogProjectionMaintenanceRequired)
	for _, column := range []struct{ name, definition string }{
		{"billing_event_id", "billing_event_id String DEFAULT ''"},
		{"billing_projection_digest", "billing_projection_digest String DEFAULT ''"},
		{"log_row_key", "log_row_key String DEFAULT ''"},
	} {
		hasColumn, err := clickHouseLogColumnExists(column.name)
		require.NoError(t, err)
		assert.False(t, hasColumn)
		require.NoError(t, ensureClickHouseLogColumn(column.name, column.definition))
	}
	require.NoError(t, ensureClickHouseCanonicalProjection())
	for _, column := range []string{"billing_event_id", "billing_projection_digest", "log_row_key"} {
		hasColumn, err := clickHouseLogColumnExists(column)
		require.NoError(t, err)
		require.True(t, hasColumn, "missing upgraded ClickHouse column %s", column)
	}
	var retainedRows int64
	require.NoError(t, db.Table("logs").Where("id = ? AND content = ?", 7, "legacy-b2-clickhouse-log").Count(&retainedRows).Error)
	assert.Equal(t, int64(1), retainedRows)
	var legacyRowKey string
	require.NoError(t, db.Table("logs").Select("log_row_key").Where("id = ?", 7).Scan(&legacyRowKey).Error)
	assert.Empty(t, legacyRowKey, "startup migration must not rewrite historical rows")
	projectionExists, err = clickHouseCanonicalProjectionExists()
	require.NoError(t, err)
	require.True(t, projectionExists)

	require.NoError(t, migrateLOGDB())
	projectionExists, err = clickHouseCanonicalProjectionExists()
	require.NoError(t, err)
	assert.True(t, projectionExists)
	var repeatedLegacyRowKey string
	require.NoError(t, db.Table("logs").Select("log_row_key").Where("id = ?", 7).Scan(&repeatedLegacyRowKey).Error)
	assert.Equal(t, legacyRowKey, repeatedLegacyRowKey, "startup migration must leave historical row keys unchanged")
	require.NoError(t, db.Exec(
		"INSERT INTO logs (id, created_at, billing_event_id, content, request_id, log_row_key) VALUES (?, ?, ?, ?, ?, ?)",
		8, 0, "configured-cleanup-event", "cleanup", "configured-cleanup-request", "configured-cleanup-row",
	).Error)
	unsafeRows, err := CountUnsafeOldLogs(t.Context(), 1)
	require.NoError(t, err)
	assert.Equal(t, int64(1), unsafeRows, "the legacy empty-event row must be reported as unsafe instead of blocking cleanup")
	cleanupResult, err := DeleteOldLogBatchDetailed(t.Context(), 1, 1)
	require.NoError(t, err)
	assert.Equal(t, int64(1), cleanupResult.Deleted)
	remainingDeletable, err := CountOldLog(t.Context(), 1)
	require.NoError(t, err)
	assert.Zero(t, remainingDeletable, "unsafe legacy rows must not starve later safe cleanup candidates")
	var retainedUnsafe int64
	require.NoError(t, db.Table("logs").Where("id = ?", 7).Count(&retainedUnsafe).Error)
	assert.Equal(t, int64(1), retainedUnsafe)

	digestA := "1" + strings.Repeat("a", 64)
	digestB := "1" + strings.Repeat("b", 64)
	require.NoError(t, EnsureClickHouseBillingProjectionIdentity(t.Context(), db, "configured-identity", digestA))
	require.NoError(t, EnsureClickHouseBillingProjectionIdentity(t.Context(), db, "configured-identity", digestA))
	require.ErrorIs(t, EnsureClickHouseBillingProjectionIdentity(t.Context(), db, "configured-identity", digestB), ErrBillingProjectionConflict)

	sqlDB.SetMaxOpenConns(2)
	concurrentDigests := []string{"1" + strings.Repeat("c", 64), "1" + strings.Repeat("d", 64)}
	concurrentErrors := make([]error, len(concurrentDigests))
	var waitGroup sync.WaitGroup
	for index, digest := range concurrentDigests {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			concurrentErrors[index] = EnsureClickHouseBillingProjectionIdentity(context.Background(), db, "configured-concurrent-identity", digest)
		}()
	}
	waitGroup.Wait()
	identities, err := loadClickHouseBillingProjectionIdentities(t.Context(), db, "configured-concurrent-identity")
	require.NoError(t, err)
	require.Len(t, identities, 2)
	require.ErrorIs(t, validateBillingLogProjectionIdentities("configured-concurrent-identity", concurrentDigests[0], identities), ErrBillingProjectionConflict)
	sqlDB.SetMaxOpenConns(1)

	for _, fixture := range []struct {
		id      int
		eventID string
	}{
		{11, "configured-backfill-a"},
		{12, "configured-backfill-b"},
	} {
		require.NoError(t, db.Exec(
			"INSERT INTO logs (id, billing_event_id, content, request_id) VALUES (?, ?, ?, ?)",
			fixture.id, fixture.eventID, "configured backfill", "request-"+fixture.eventID,
		).Error)
	}
	backfillState := LogProjectionBackfillState{}
	backfillState, processed, err := BackfillClickHouseProjectionIdentityBatch(t.Context(), db, backfillState, 1)
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	assert.Equal(t, "configured-backfill-a", backfillState.LastEventID)
	backfillState, processed, err = BackfillClickHouseProjectionIdentityBatch(t.Context(), db, backfillState, 1)
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	assert.Equal(t, "configured-backfill-b", backfillState.LastEventID)
	backfillState, processed, err = BackfillClickHouseProjectionIdentityBatch(t.Context(), db, backfillState, 1)
	require.NoError(t, err)
	assert.Zero(t, processed)
	assert.True(t, backfillState.Complete)

	requestedAt := time.Now().UnixMilli()
	require.NoError(t, StartClickHouseCanonicalProjectionMaterialize(t.Context(), db))
	mutations, err := FindClickHouseCanonicalProjectionMutations(t.Context(), db, requestedAt)
	require.NoError(t, err)
	require.Len(t, mutations, 1)
	mutationID := mutations[0].MutationID
	mutation, err := GetClickHouseCanonicalProjectionMutation(t.Context(), db, mutationID)
	require.NoError(t, err)
	require.NotNil(t, mutation, "the asynchronous materialization mutation must remain queryable by its persisted id")
	assert.Equal(t, mutationID, mutation.MutationID)
}
