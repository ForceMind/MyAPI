package model

import (
	"context"
	"database/sql/driver"
	"errors"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/ForceMind/MyAPI/common"
	"github.com/glebarez/sqlite"
	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type clickHouseSQLiteDialector struct {
	gorm.Dialector
}

func (clickHouseSQLiteDialector) Name() string { return string(common.DatabaseTypeClickHouse) }

func openClickHouseSQLiteFixture(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := "file:" + stringsForSQLiteName(t.Name()) + "?mode=memory&cache=shared"
	db, err := gorm.Open(clickHouseSQLiteDialector{Dialector: sqlite.Open(dsn)}, &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.Exec(`CREATE TABLE logs (
		id INTEGER,
		user_id INTEGER DEFAULT 0,
		created_at INTEGER DEFAULT 0,
		type INTEGER DEFAULT 0,
		content TEXT DEFAULT '',
		username TEXT DEFAULT '',
		token_name TEXT DEFAULT '',
		model_name TEXT DEFAULT '',
		quota INTEGER DEFAULT 0,
		prompt_tokens INTEGER DEFAULT 0,
		completion_tokens INTEGER DEFAULT 0,
		use_time INTEGER DEFAULT 0,
		is_stream INTEGER DEFAULT 0,
		channel_id INTEGER DEFAULT 0,
		token_id INTEGER DEFAULT 0,
		"group" TEXT DEFAULT '',
		ip TEXT DEFAULT '',
		request_id TEXT DEFAULT '',
		upstream_request_id TEXT DEFAULT '',
		billing_event_id TEXT DEFAULT '',
		billing_projection_digest TEXT DEFAULT '',
		log_row_key TEXT DEFAULT '',
		other TEXT DEFAULT ''
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE billing_log_projection_identities (
		billing_event_id TEXT,
		digest TEXT,
		canonical_version INTEGER,
		status TEXT DEFAULT 'canonical',
		reason TEXT DEFAULT '',
		updated_at INTEGER
	)`).Error)
	return db
}

func stringsForSQLiteName(value string) string {
	result := make([]rune, 0, len(value))
	for _, r := range value {
		if r == '/' || r == ' ' {
			result = append(result, '_')
		} else {
			result = append(result, r)
		}
	}
	return string(result)
}

func TestLogProjectionIndexSQLContracts(t *testing.T) {
	spec := logProjectionIndexSpecs[0]
	postgresSQL, ok := logProjectionIndexSQL(string(common.DatabaseTypePostgreSQL), spec)
	require.True(t, ok)
	assert.Equal(t, "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_logs_billing_canonical ON logs (billing_event_id, billing_projection_digest, id)", postgresSQL)

	mysqlSQL, ok := logProjectionIndexSQL(string(common.DatabaseTypeMySQL), spec)
	require.True(t, ok)
	assert.Equal(t, "ALTER TABLE logs ADD INDEX idx_logs_billing_canonical (billing_event_id, billing_projection_digest, id), ALGORITHM=INPLACE, LOCK=NONE", mysqlSQL)

	sqliteSQL, ok := logProjectionIndexSQL(string(common.DatabaseTypeSQLite), spec)
	require.True(t, ok)
	assert.Equal(t, "CREATE INDEX IF NOT EXISTS idx_logs_billing_canonical ON logs (billing_event_id, billing_projection_digest, id)", sqliteSQL)

	_, ok = logProjectionIndexSQL("unknown", spec)
	assert.False(t, ok)
}

func TestCreateNextLogProjectionIndexMySQLFailureRequiresManualReview(t *testing.T) {
	sqliteDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	connection, err := sqliteDB.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, connection.Close()) })
	mysqlDB, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      connection,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{DisableAutomaticPing: true})
	require.NoError(t, err)

	done, manualReview, err := CreateNextLogProjectionIndex(t.Context(), mysqlDB)
	require.Error(t, err)
	assert.False(t, done)
	assert.True(t, manualReview)
	assert.Contains(t, err.Error(), "idx_logs_billing_canonical")
}

func TestClickHouseBillingProjectionIdentitySameAndConflict(t *testing.T) {
	db := openClickHouseSQLiteFixture(t)
	digestA := "1" + strings.Repeat("a", 64)
	digestB := "1" + strings.Repeat("b", 64)

	require.NoError(t, EnsureClickHouseBillingProjectionIdentity(t.Context(), db, "event-same", digestA))
	require.NoError(t, EnsureClickHouseBillingProjectionIdentity(t.Context(), db, "event-same", digestA))
	var count int64
	require.NoError(t, db.Table(clickHouseIdentityTable).Where("billing_event_id = ?", "event-same").Count(&count).Error)
	assert.Equal(t, int64(1), count)
	require.ErrorIs(t, EnsureClickHouseBillingProjectionIdentity(t.Context(), db, "event-same", digestB), ErrBillingProjectionConflict)
}

func TestClickHouseBillingProjectionIdentityConcurrentDifferentDigestDetected(t *testing.T) {
	db := openClickHouseSQLiteFixture(t)
	digests := []string{"1" + strings.Repeat("c", 64), "1" + strings.Repeat("d", 64)}
	errs := make([]error, len(digests))
	var wg sync.WaitGroup
	for index, digest := range digests {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[index] = EnsureClickHouseBillingProjectionIdentity(context.Background(), db, "event-concurrent", digest)
		}()
	}
	wg.Wait()
	conflicts := 0
	for _, err := range errs {
		if errors.Is(err, ErrBillingProjectionConflict) {
			conflicts++
		}
	}
	assert.GreaterOrEqual(t, conflicts, 1)
}

func TestClickHouseProjectionBackfillCheckpointAndConflict(t *testing.T) {
	db := openClickHouseSQLiteFixture(t)
	insert := func(id int, eventID string, content string) {
		require.NoError(t, db.Exec(
			"INSERT INTO logs (id, billing_event_id, content, request_id) VALUES (?, ?, ?, ?)",
			id, eventID, content, "request-"+eventID,
		).Error)
	}
	insert(1, "event-a", "same")
	insert(2, "event-a", "same")
	insert(3, "event-b", "same")
	insert(4, "event-c", "first")
	insert(5, "event-c", "different")

	state, processed, err := BackfillClickHouseProjectionIdentityBatch(t.Context(), db, LogProjectionBackfillState{}, 2)
	require.NoError(t, err)
	assert.Equal(t, 2, processed)
	assert.Equal(t, "event-b", state.LastEventID)
	assert.False(t, state.Complete)

	next, processed, err := BackfillClickHouseProjectionIdentityBatch(t.Context(), db, state, 2)
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	assert.Equal(t, "event-c", next.LastEventID)
	assert.Equal(t, int64(1), next.QuarantinedEvents)
	var quarantine BillingLogProjectionIdentity
	require.NoError(t, db.Table(clickHouseIdentityTable).Where("billing_event_id = ? AND status = ?", "event-c", BillingLogProjectionIdentityStatusQuarantined).First(&quarantine).Error)
	assert.Contains(t, quarantine.Reason, "immutable projections")
}

func TestClickHouseIdentityAndBackfillSQLContracts(t *testing.T) {
	identityDDL := clickHouseBillingProjectionIdentityCreateTableSQL()
	assert.Contains(t, identityDDL, "CREATE TABLE IF NOT EXISTS billing_log_projection_identities")
	assert.Contains(t, identityDDL, "billing_event_id String")
	assert.Contains(t, identityDDL, "digest String")
	assert.Contains(t, identityDDL, "canonical_version UInt16")
	assert.Contains(t, identityDDL, "updated_at Int64")
	assert.Contains(t, identityDDL, "ORDER BY billing_event_id")

	backfillSQL := clickHouseBackfillEventIDsSQL()
	assert.Contains(t, backfillSQL, "billing_event_id > ?")
	assert.Contains(t, backfillSQL, "GROUP BY billing_event_id")
	assert.Contains(t, backfillSQL, "ORDER BY billing_event_id")
	assert.Contains(t, backfillSQL, "LIMIT ?")
}

func TestSQLiteLargeLogsRequireMaintenanceBeforeIndexDDL(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(2)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&Log{}))
	require.NoError(t, EnsureLogProjectionSchemaWithDB(db))
	require.NoError(t, db.Exec(`
WITH RECURSIVE seq(n) AS (
  SELECT 1
  UNION ALL
  SELECT n + 1 FROM seq WHERE n < ?
)
INSERT INTO logs (id, content, request_id, log_row_key) SELECT n, 'large', printf('request-%d', n), printf('row-%d', n) FROM seq`, LogProjectionSQLiteAutoIndexMaxRows+1).Error)

	done, manualReview, err := CreateNextLogProjectionIndex(t.Context(), db)
	require.ErrorIs(t, err, ErrLogProjectionMaintenanceRequired)
	assert.False(t, done)
	assert.True(t, manualReview)

	tx := db.Begin()
	require.NoError(t, tx.Error)
	require.NoError(t, tx.Exec("UPDATE logs SET content = content WHERE id = 1").Error)
	done, manualReview, err = CreateNextLogProjectionIndex(t.Context(), tx)
	require.ErrorIs(t, err, ErrLogProjectionMaintenanceRequired)
	assert.False(t, done)
	assert.True(t, manualReview)
	require.NoError(t, tx.Rollback().Error)
	assert.False(t, db.Migrator().HasIndex(&Log{}, logProjectionIndexSpecs[0].Name))
}

func TestMySQLLogProjectionDDLErrorClassification(t *testing.T) {
	assert.False(t, classifyMySQLLogProjectionDDLError(driver.ErrBadConn))
	assert.False(t, classifyMySQLLogProjectionDDLError(context.DeadlineExceeded))
	for _, number := range []uint16{1040, 1053, 1158, 1159, 1160, 1161, 1203, 1205, 1213, 2006, 2013, 9999} {
		assert.False(t, classifyMySQLLogProjectionDDLError(&mysqlDriver.MySQLError{Number: number, Message: "retryable"}))
	}
	for _, number := range []uint16{1061, 1064, 1071, 1072, 1142, 1143, 1227, 1235, 1709, 1846, 1847} {
		assert.True(t, classifyMySQLLogProjectionDDLError(&mysqlDriver.MySQLError{Number: number, Message: "manual"}))
	}
}

func TestPostgreSQLLogProjectionIndexInspectionContract(t *testing.T) {
	assert.Contains(t, postgresLogProjectionIndexInspectionSQL, "pg_class")
	assert.Contains(t, postgresLogProjectionIndexInspectionSQL, "pg_index")
	assert.Contains(t, postgresLogProjectionIndexInspectionSQL, "indisvalid")
	assert.Contains(t, postgresLogProjectionIndexInspectionSQL, "indisready")
	assert.Contains(t, postgresLogProjectionIndexInspectionSQL, "WITH ORDINALITY")
	assert.Contains(t, postgresLogProjectionIndexInspectionSQL, "ORDER BY ord.ordinality")
	assert.Contains(t, postgresLogProjectionIndexInspectionSQL, "current_schema()")
}

func TestPostgreSQLInvalidLogProjectionIndexDropsRecreatesAndRevalidates(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB, PreferSimpleProtocol: true}), &gorm.Config{DisableAutomaticPing: true})
	require.NoError(t, err)
	spec := logProjectionIndexSpecs[0]
	inspection := regexp.QuoteMeta(strings.Replace(postgresLogProjectionIndexInspectionSQL, "?", "$1", 1))

	mock.ExpectQuery(inspection).WithArgs(spec.Name).
		WillReturnRows(sqlmock.NewRows([]string{"indisvalid", "indisready", "columns"}).AddRow(false, false, spec.Columns))
	mock.ExpectQuery(regexp.QuoteMeta("SHOW lock_timeout")).
		WillReturnRows(sqlmock.NewRows([]string{"lock_timeout"}).AddRow("0"))
	mock.ExpectExec(regexp.QuoteMeta("SET lock_timeout = '5s'")).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta("DROP INDEX CONCURRENTLY IF EXISTS " + spec.Name)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT set_config('lock_timeout', $1, false)")).WithArgs("0").
		WillReturnRows(sqlmock.NewRows([]string{"set_config"}).AddRow("0"))
	mock.ExpectQuery(regexp.QuoteMeta("SHOW lock_timeout")).
		WillReturnRows(sqlmock.NewRows([]string{"lock_timeout"}).AddRow("0"))
	mock.ExpectExec(regexp.QuoteMeta("SET lock_timeout = '5s'")).WillReturnResult(sqlmock.NewResult(0, 0))
	createSQL, ok := logProjectionIndexSQL(string(common.DatabaseTypePostgreSQL), spec)
	require.True(t, ok)
	mock.ExpectExec(regexp.QuoteMeta(createSQL)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT set_config('lock_timeout', $1, false)")).WithArgs("0").
		WillReturnRows(sqlmock.NewRows([]string{"set_config"}).AddRow("0"))
	mock.ExpectQuery(inspection).WithArgs(spec.Name).
		WillReturnRows(sqlmock.NewRows([]string{"indisvalid", "indisready", "columns"}).AddRow(true, true, spec.Columns))
	mock.ExpectClose()

	done, manualReview, err := CreateNextLogProjectionIndex(t.Context(), db)
	require.NoError(t, err)
	assert.False(t, done)
	assert.False(t, manualReview)
	require.NoError(t, sqlDB.Close())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSQLiteIndexDDLDoesNotUseSessionTimeoutSettings(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.Exec("CREATE TABLE logs (id INTEGER PRIMARY KEY, billing_event_id TEXT, billing_projection_digest TEXT, log_row_key TEXT)").Error)
	require.NoError(t, executeLogProjectionIndexDDL(t.Context(), db, "CREATE INDEX idx_sqlite_session_test ON logs (log_row_key)"))
	assert.True(t, db.Migrator().HasIndex("logs", "idx_sqlite_session_test"))
}
