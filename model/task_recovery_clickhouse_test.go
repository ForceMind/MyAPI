package model

import (
	"errors"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"

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

	// New deployments receive the projection column from the canonical helper.
	require.NoError(t, migrateClickHouseLogDB())
	hasColumn, err := clickHouseLogColumnExists("billing_event_id")
	require.NoError(t, err)
	require.True(t, hasColumn)
	require.NoError(t, db.Exec("RENAME TABLE logs TO logs_b2_new_fixture").Error)

	// Build the old shape from the canonical create statement, then exercise the
	// production helper's additive upgrade and its repeatability on retained rows.
	legacyCreateSQL := strings.Replace(clickHouseLogCreateTableSQL(0), "\tbilling_event_id String DEFAULT '',\n", "", 1)
	require.NotEqual(t, clickHouseLogCreateTableSQL(0), legacyCreateSQL)
	require.NoError(t, db.Exec(legacyCreateSQL).Error)
	require.NoError(t, db.Exec("INSERT INTO logs (id, content) VALUES (?, ?)", 7, "legacy-b2-clickhouse-log").Error)

	require.NoError(t, migrateClickHouseLogDB())
	hasColumn, err = clickHouseLogColumnExists("billing_event_id")
	require.NoError(t, err)
	require.True(t, hasColumn)
	var retainedRows int64
	require.NoError(t, db.Table("logs").Where("id = ? AND content = ?", 7, "legacy-b2-clickhouse-log").Count(&retainedRows).Error)
	assert.Equal(t, int64(1), retainedRows)

	require.NoError(t, migrateClickHouseLogDB())
	hasColumn, err = clickHouseLogColumnExists("billing_event_id")
	require.NoError(t, err)
	assert.True(t, hasColumn)
}
