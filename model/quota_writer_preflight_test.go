package model

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/glebarez/sqlite"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type quotaWriterFixtureRow struct {
	id            int64
	schemaVersion int64
	mode          string
	epoch         int64
	lockVersion   int64
}

func sqliteFixtureURI(path string) string {
	return (&url.URL{Scheme: "file", Path: path}).String()
}

func createQuotaWriterSQLiteFixture(t *testing.T, path string, rows ...quotaWriterFixtureRow) {
	t.Helper()
	db, err := sql.Open(sqlite.DriverName, sqliteFixtureURI(path))
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	require.NoError(t, execQuotaWriterFixtureStatements(db,
		`CREATE TABLE quota_writer_epochs (
id INTEGER PRIMARY KEY,
schema_version INTEGER NOT NULL,
mode TEXT NOT NULL,
epoch INTEGER NOT NULL,
lock_version INTEGER NOT NULL
)`,
	))
	for _, row := range rows {
		_, err := db.Exec(
			"INSERT INTO quota_writer_epochs (id, schema_version, mode, epoch, lock_version) VALUES (?, ?, ?, ?, ?)",
			row.id, row.schemaVersion, row.mode, row.epoch, row.lockVersion,
		)
		require.NoError(t, err)
	}
	require.NoError(t, db.Close())
}

func execQuotaWriterFixtureStatements(db *sql.DB, statements ...string) error {
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			return err
		}
	}
	return nil
}

func readFileForQuotaPreflightTest(t *testing.T, path string) []byte {
	t.Helper()
	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	return contents
}

func directoryNamesForQuotaPreflightTest(t *testing.T, directory string) []string {
	t.Helper()
	entries, err := os.ReadDir(directory)
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names
}

func copyFileForQuotaPreflightTest(t *testing.T, source, destination string) {
	t.Helper()
	contents := readFileForQuotaPreflightTest(t, source)
	require.NoError(t, os.WriteFile(destination, contents, 0o600))
}

func TestParseQuotaBatchUpdateEnabled(t *testing.T) {
	for _, test := range []struct {
		configured string
		enabled    bool
		valid      bool
	}{
		{configured: "", enabled: false, valid: true},
		{configured: "false", enabled: false, valid: true},
		{configured: "true", enabled: true, valid: true},
		{configured: "TRUE"},
		{configured: "False"},
		{configured: "1"},
		{configured: " true"},
	} {
		t.Run(fmt.Sprintf("%q", test.configured), func(t *testing.T) {
			enabled, err := ParseQuotaBatchUpdateEnabled(test.configured)
			if test.valid {
				require.NoError(t, err)
				assert.Equal(t, test.enabled, enabled)
				return
			}
			require.EqualError(t, err, quotaBatchConfigurationError)
			assert.False(t, enabled)
		})
	}
}

func TestValidateQuotaBatchWriterMode(t *testing.T) {
	for _, test := range []struct {
		name           string
		mode           QuotaWriterMode
		redisAvailable bool
		wantError      string
	}{
		{name: "legacy with Redis", mode: QuotaWriterModeLegacy, redisAvailable: true},
		{name: "bridge with Redis", mode: QuotaWriterModeBridge, redisAvailable: true},
		{name: "authoritative with Redis", mode: QuotaWriterModeAuthoritative, redisAvailable: true},
		{name: "authoritative without Redis", mode: QuotaWriterModeAuthoritative},
		{name: "legacy without Redis", mode: QuotaWriterModeLegacy, wantError: "requires Redis"},
		{name: "bridge without Redis", mode: QuotaWriterModeBridge, wantError: "requires Redis"},
		{name: "invalid with Redis", mode: "invalid", redisAvailable: true, wantError: "cannot verify quota writer mode"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateQuotaBatchWriterMode(test.mode, test.redisAvailable)
			if test.wantError == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, test.wantError)
		})
	}
}

func TestResolveDatabaseTargetMatchesChooseDBInputs(t *testing.T) {
	previousSQLitePath := common.SQLitePath
	common.SQLitePath = filepath.Join(t.TempDir(), "configured.db")
	t.Cleanup(func() { common.SQLitePath = previousSQLitePath })

	t.Setenv("SQL_DSN", "")
	target, err := resolveDatabaseTarget("SQL_DSN", false)
	require.NoError(t, err)
	assert.Equal(t, databaseTarget{databaseType: common.DatabaseTypeSQLite, dsn: common.SQLitePath}, target)

	t.Setenv("SQL_DSN", "local")
	target, err = resolveDatabaseTarget("SQL_DSN", false)
	require.NoError(t, err)
	assert.Equal(t, databaseTarget{databaseType: common.DatabaseTypeSQLite, dsn: common.SQLitePath}, target)

	t.Setenv("SQL_DSN", "postgresql://user:secret@example.invalid/database")
	target, err = resolveDatabaseTarget("SQL_DSN", false)
	require.NoError(t, err)
	assert.Equal(t, common.DatabaseTypePostgreSQL, target.databaseType)
	assert.Equal(t, "postgresql://user:secret@example.invalid/database", target.dsn)

	t.Setenv("SQL_DSN", "user:secret@tcp(example.invalid:3306)/database?charset=utf8mb4")
	target, err = resolveDatabaseTarget("SQL_DSN", false)
	require.NoError(t, err)
	assert.Equal(t, common.DatabaseTypeMySQL, target.databaseType)
	assert.Equal(t, "user:secret@tcp(example.invalid:3306)/database?charset=utf8mb4&parseTime=true", target.dsn)
}

func TestQuotaBatchPreflightDisabledRejectsUnverifiableSQLite(t *testing.T) {
	previousSQLitePath := common.SQLitePath
	common.SQLitePath = string([]byte{'i', 'n', 'v', 'a', 'l', 'i', 'd', 0, 'd', 'b'})
	t.Cleanup(func() { common.SQLitePath = previousSQLitePath })
	t.Setenv("SQL_DSN", "")

	for _, configured := range []string{"", "false"} {
		enabled, err := ValidateQuotaBatchPreMigrationConfiguration(context.Background(), configured, false)
		require.ErrorIs(t, err, ErrQuotaWriterStartupUnverifiable)
		assert.False(t, enabled)
	}
}

func TestQuotaWriterPreflightDisabledPreservesPersistentIdentity(t *testing.T) {
	for _, configured := range []string{"", "false"} {
		for _, test := range []struct {
			name       string
			statements []string
			writer     *quotaWriterFixtureRow
			wantError  bool
		}{
			{name: "fresh database"},
			{name: "legacy balances without durable accounting", statements: []string{`CREATE TABLE users (quota INTEGER)`}},
			{name: "durable accounting without writer", statements: []string{`CREATE TABLE quota_balance_batch_drains (id INTEGER)`}, wantError: true},
			{name: "missing singleton", writer: &quotaWriterFixtureRow{}, wantError: true},
			{name: "unknown writer mode", writer: &quotaWriterFixtureRow{1, 1, "unknown", 1, 1}, wantError: true},
			{name: "valid legacy without Redis", writer: &quotaWriterFixtureRow{1, 1, string(QuotaWriterModeLegacy), 1, 1}},
			{name: "valid authoritative without Redis", writer: &quotaWriterFixtureRow{1, 1, string(QuotaWriterModeAuthoritative), 7, 2}},
		} {
			t.Run(fmt.Sprintf("%q/%s", configured, test.name), func(t *testing.T) {
				directory := t.TempDir()
				path := filepath.Join(directory, "writer.db")
				if test.writer != nil {
					if test.writer.id == 0 {
						createQuotaWriterSQLiteFixture(t, path)
					} else {
						createQuotaWriterSQLiteFixture(t, path, *test.writer)
					}
				} else if len(test.statements) > 0 {
					createQuotaAmountSQLiteFixture(t, path, test.statements...)
				}
				var before []byte
				if test.writer != nil || len(test.statements) > 0 {
					before = readFileForQuotaPreflightTest(t, path)
				}
				names := directoryNamesForQuotaPreflightTest(t, directory)
				previousPath := common.SQLitePath
				common.SQLitePath = path
				t.Cleanup(func() { common.SQLitePath = previousPath })
				t.Setenv("SQL_DSN", "local")

				enabled, err := ValidateQuotaBatchPreMigrationConfiguration(context.Background(), configured, false)
				if test.wantError {
					require.ErrorIs(t, err, ErrQuotaWriterStartupUnverifiable)
				} else {
					require.NoError(t, err)
				}
				assert.False(t, enabled)
				assert.Equal(t, names, directoryNamesForQuotaPreflightTest(t, directory))
				if before != nil {
					assert.Equal(t, before, readFileForQuotaPreflightTest(t, path))
				}
			})
		}
	}
}

func TestQuotaBatchPreflightFreshSQLiteDoesNotCreateFiles(t *testing.T) {
	for _, dsn := range []string{":memory:", "file::memory:?cache=shared", "file:named?mode=memory&cache=shared"} {
		mode, err := inspectSQLiteQuotaWriter(context.Background(), dsn)
		require.NoError(t, err)
		assert.Equal(t, QuotaWriterModeLegacy, mode)
	}

	directory := t.TempDir()
	absentPath := filepath.Join(directory, "absent.db")
	mode, err := inspectSQLiteQuotaWriter(context.Background(), absentPath)
	require.NoError(t, err)
	assert.Equal(t, QuotaWriterModeLegacy, mode)
	_, err = os.Stat(absentPath)
	require.ErrorIs(t, err, os.ErrNotExist)
	assert.Empty(t, directoryNamesForQuotaPreflightTest(t, directory))

	emptyPath := filepath.Join(directory, "empty.db")
	require.NoError(t, os.WriteFile(emptyPath, nil, 0o600))
	before, err := os.Stat(emptyPath)
	require.NoError(t, err)
	mode, err = inspectSQLiteQuotaWriter(context.Background(), emptyPath)
	require.NoError(t, err)
	assert.Equal(t, QuotaWriterModeLegacy, mode)
	after, err := os.Stat(emptyPath)
	require.NoError(t, err)
	assert.Zero(t, after.Size())
	assert.Equal(t, before.ModTime(), after.ModTime())
	assert.Equal(t, []string{"empty.db"}, directoryNamesForQuotaPreflightTest(t, directory))

	previousSQLitePath := common.SQLitePath
	common.SQLitePath = absentPath
	t.Cleanup(func() { common.SQLitePath = previousSQLitePath })
	t.Setenv("SQL_DSN", "")
	enabled, err := ValidateQuotaBatchPreMigrationConfiguration(context.Background(), "true", true)
	require.NoError(t, err)
	assert.True(t, enabled)
	_, err = os.Stat(absentPath)
	require.ErrorIs(t, err, os.ErrNotExist)
	enabled, err = ValidateQuotaBatchPreMigrationConfiguration(context.Background(), "true", false)
	require.ErrorContains(t, err, "requires Redis")
	assert.False(t, enabled)
	enabled, err = ValidateQuotaBatchPreMigrationConfiguration(nil, "true", true)
	require.NoError(t, err)
	assert.True(t, enabled)
}

func TestInitDBRejectsIncompatibleBatchingBeforeOpeningOrMigratingSQLite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	createQuotaWriterSQLiteFixture(t, path, quotaWriterFixtureRow{1, 1, string(QuotaWriterModeLegacy), 1, 1})
	before := readFileForQuotaPreflightTest(t, path)
	previousDB, previousPath := DB, common.SQLitePath
	previousRedisClient, previousRedisEnabled := common.RDB, common.RedisEnabled
	common.SQLitePath = path
	common.RDB = nil
	common.RedisEnabled = false
	t.Cleanup(func() {
		DB, common.SQLitePath = previousDB, previousPath
		common.RDB, common.RedisEnabled = previousRedisClient, previousRedisEnabled
	})
	t.Setenv("SQL_DSN", "local")
	t.Setenv("BATCH_UPDATE_ENABLED", "true")

	err := InitDB()
	require.ErrorContains(t, err, "requires Redis")
	assert.Same(t, previousDB, DB)
	assert.Equal(t, before, readFileForQuotaPreflightTest(t, path))
	assert.Equal(t, []string{"legacy.db"}, directoryNamesForQuotaPreflightTest(t, filepath.Dir(path)))
}

func TestQuotaBatchPreflightAllowsAuthoritativeSQLiteWithoutRedis(t *testing.T) {
	path := filepath.Join(t.TempDir(), "authoritative.db")
	createQuotaWriterSQLiteFixture(t, path, quotaWriterFixtureRow{1, 1, string(QuotaWriterModeAuthoritative), 11, 2})
	before := readFileForQuotaPreflightTest(t, path)
	previousPath := common.SQLitePath
	common.SQLitePath = path
	t.Cleanup(func() { common.SQLitePath = previousPath })
	t.Setenv("SQL_DSN", "local")

	enabled, err := ValidateQuotaBatchPreMigrationConfiguration(context.Background(), "true", false)
	require.NoError(t, err)
	assert.True(t, enabled)
	assert.Equal(t, before, readFileForQuotaPreflightTest(t, path))
}

func TestSQLiteQuotaWriterPreflightReadsValidModesWithoutChangingDatabase(t *testing.T) {
	for _, expectedMode := range []QuotaWriterMode{QuotaWriterModeLegacy, QuotaWriterModeBridge, QuotaWriterModeAuthoritative} {
		t.Run(string(expectedMode), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "writer.db")
			createQuotaWriterSQLiteFixture(t, path, quotaWriterFixtureRow{1, 1, string(expectedMode), 7, 3})
			before := readFileForQuotaPreflightTest(t, path)
			mode, err := inspectSQLiteQuotaWriter(context.Background(), path)
			require.NoError(t, err)
			assert.Equal(t, expectedMode, mode)
			assert.Equal(t, before, readFileForQuotaPreflightTest(t, path))
			assert.Equal(t, []string{"writer.db"}, directoryNamesForQuotaPreflightTest(t, filepath.Dir(path)))
		})
	}
}

func TestSQLiteQuotaWriterPreflightUsesSanitizedReadOnlyURI(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "writer space#percent%.db")
	createQuotaWriterSQLiteFixture(t, path, quotaWriterFixtureRow{1, 1, string(QuotaWriterModeAuthoritative), 9, 2})
	before := readFileForQuotaPreflightTest(t, path)

	workingDirectory, err := os.Getwd()
	require.NoError(t, err)
	relativePath, err := filepath.Rel(workingDirectory, path)
	require.NoError(t, err)
	fileURI := "file:" + (&url.URL{Path: relativePath}).EscapedPath() + "?_pragma=journal_mode(WAL)&immutable=1&mode=rwc"
	resolvedPath, memory, err := sqliteDatabasePath(fileURI)
	require.NoError(t, err)
	assert.False(t, memory)
	assert.Equal(t, path, resolvedPath)

	mode, err := inspectSQLiteQuotaWriter(context.Background(), fileURI)
	require.NoError(t, err)
	assert.Equal(t, QuotaWriterModeAuthoritative, mode)
	assert.Equal(t, before, readFileForQuotaPreflightTest(t, path))
	assert.Equal(t, []string{filepath.Base(path)}, directoryNamesForQuotaPreflightTest(t, directory))
	readOnlyURI, err := url.Parse(sqliteReadOnlyURI(path, false))
	require.NoError(t, err)
	assert.Equal(t, "file", readOnlyURI.Scheme)
	assert.Empty(t, readOnlyURI.Host)
	assert.Equal(t, path, readOnlyURI.Path)
	assert.Equal(t, "ro", readOnlyURI.Query().Get("mode"))
	assert.Empty(t, readOnlyURI.Query().Get("immutable"))

	mode, err = inspectSQLiteQuotaWriter(context.Background(), relativePath+"?_pragma=journal_mode(WAL)")
	require.NoError(t, err)
	assert.Equal(t, QuotaWriterModeAuthoritative, mode)
	assert.Equal(t, before, readFileForQuotaPreflightTest(t, path))

	questionPath := filepath.Join(directory, "writer?.db")
	createQuotaWriterSQLiteFixture(t, questionPath, quotaWriterFixtureRow{1, 1, string(QuotaWriterModeLegacy), 1, 1})
	absoluteFileURI := sqliteFixtureURI(questionPath) + "?_pragma=journal_mode(WAL)"
	mode, err = inspectSQLiteQuotaWriter(context.Background(), absoluteFileURI)
	require.NoError(t, err)
	assert.Equal(t, QuotaWriterModeLegacy, mode)
	_, err = os.Stat(questionPath + "-wal")
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestSQLiteQuotaWriterPreflightRejectsInvalidPersistedState(t *testing.T) {
	valid := quotaWriterFixtureRow{1, 1, string(QuotaWriterModeLegacy), 1, 1}
	for _, test := range []struct {
		name string
		rows []quotaWriterFixtureRow
	}{
		{name: "missing singleton"},
		{name: "multiple rows", rows: []quotaWriterFixtureRow{valid, {2, 1, string(QuotaWriterModeLegacy), 1, 1}}},
		{name: "wrong singleton id", rows: []quotaWriterFixtureRow{{2, 1, string(QuotaWriterModeLegacy), 1, 1}}},
		{name: "wrong schema", rows: []quotaWriterFixtureRow{{1, 2, string(QuotaWriterModeLegacy), 1, 1}}},
		{name: "invalid mode", rows: []quotaWriterFixtureRow{{1, 1, "invalid", 1, 1}}},
		{name: "zero epoch", rows: []quotaWriterFixtureRow{{1, 1, string(QuotaWriterModeLegacy), 0, 1}}},
		{name: "zero lock version", rows: []quotaWriterFixtureRow{{1, 1, string(QuotaWriterModeLegacy), 1, 0}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "invalid.db")
			createQuotaWriterSQLiteFixture(t, path, test.rows...)
			before := readFileForQuotaPreflightTest(t, path)
			_, err := inspectSQLiteQuotaWriter(context.Background(), path)
			require.Error(t, err)
			assert.Equal(t, before, readFileForQuotaPreflightTest(t, path))
		})
	}

	t.Run("incomplete writer table", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "incomplete.db")
		db, err := sql.Open(sqlite.DriverName, sqliteFixtureURI(path))
		require.NoError(t, err)
		require.NoError(t, execQuotaWriterFixtureStatements(db, "CREATE TABLE quota_writer_epochs (id INTEGER PRIMARY KEY)"))
		require.NoError(t, db.Close())
		_, err = inspectSQLiteQuotaWriter(context.Background(), path)
		require.Error(t, err)
	})

	t.Run("writer view", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "view.db")
		db, err := sql.Open(sqlite.DriverName, sqliteFixtureURI(path))
		require.NoError(t, err)
		require.NoError(t, execQuotaWriterFixtureStatements(db, `CREATE VIEW quota_writer_epochs AS
SELECT 1 AS id, 1 AS schema_version, 'authoritative' AS mode, 1 AS epoch, 1 AS lock_version`))
		require.NoError(t, db.Close())
		_, err = inspectSQLiteQuotaWriter(context.Background(), path)
		require.ErrorContains(t, err, "not a base table")
	})
}

func TestSQLiteQuotaWriterPreflightClassifiesHistoricalAndPartialSchemas(t *testing.T) {
	t.Run("historical schema without writer", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "historical.db")
		db, err := sql.Open(sqlite.DriverName, sqliteFixtureURI(path))
		require.NoError(t, err)
		require.NoError(t, execQuotaWriterFixtureStatements(db, "CREATE TABLE users (id INTEGER PRIMARY KEY)"))
		require.NoError(t, db.Close())
		mode, err := inspectSQLiteQuotaWriter(context.Background(), path)
		require.NoError(t, err)
		assert.Equal(t, QuotaWriterModeLegacy, mode)
	})

	for _, table := range quotaWriterDurableSchemaTables {
		t.Run(table, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "partial.db")
			db, err := sql.Open(sqlite.DriverName, sqliteFixtureURI(path))
			require.NoError(t, err)
			_, err = db.Exec("CREATE TABLE `" + table + "` (id INTEGER PRIMARY KEY)")
			require.NoError(t, err)
			require.NoError(t, db.Close())
			_, err = inspectSQLiteQuotaWriter(context.Background(), path)
			require.ErrorContains(t, err, "without quota_writer_epochs")
		})
	}
}

func TestSQLiteQuotaWriterPreflightReadsLiveWALWithoutImmutable(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "live.db")
	writer, err := sql.Open(sqlite.DriverName, sqliteFixtureURI(path))
	require.NoError(t, err)
	writer.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, writer.Close()) })
	var journalMode string
	require.NoError(t, writer.QueryRow("PRAGMA journal_mode=WAL").Scan(&journalMode))
	require.Equal(t, "wal", strings.ToLower(journalMode))
	require.NoError(t, execQuotaWriterFixtureStatements(writer,
		"PRAGMA wal_autocheckpoint=0",
		`CREATE TABLE quota_writer_epochs (
id INTEGER PRIMARY KEY,
schema_version INTEGER NOT NULL,
mode TEXT NOT NULL,
epoch INTEGER NOT NULL,
lock_version INTEGER NOT NULL
)`,
	))
	var busy, logFrames, checkpointed int
	require.NoError(t, writer.QueryRow("PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &logFrames, &checkpointed))
	require.Zero(t, busy)
	_, err = writer.Exec("INSERT INTO quota_writer_epochs (id, schema_version, mode, epoch, lock_version) VALUES (1, 1, 'authoritative', 41, 4)")
	require.NoError(t, err)

	beforeNames := directoryNamesForQuotaPreflightTest(t, directory)
	require.Contains(t, beforeNames, "live.db-wal")
	require.Contains(t, beforeNames, "live.db-shm")
	mainBefore := readFileForQuotaPreflightTest(t, path)
	walBefore := readFileForQuotaPreflightTest(t, path+"-wal")
	mode, err := inspectSQLiteQuotaWriter(context.Background(), path+"?_pragma=journal_mode(DELETE)&immutable=1")
	require.NoError(t, err)
	assert.Equal(t, QuotaWriterModeAuthoritative, mode)
	assert.Equal(t, mainBefore, readFileForQuotaPreflightTest(t, path))
	assert.Equal(t, walBefore, readFileForQuotaPreflightTest(t, path+"-wal"))
	assert.Equal(t, beforeNames, directoryNamesForQuotaPreflightTest(t, directory))

	for _, missingSuffix := range []string{"-wal", "-shm"} {
		t.Run("missing "+missingSuffix, func(t *testing.T) {
			copyPath := filepath.Join(directory, "missing"+strings.TrimPrefix(missingSuffix, "-")+".db")
			copyFileForQuotaPreflightTest(t, path, copyPath)
			for _, suffix := range []string{"-wal", "-shm"} {
				if suffix != missingSuffix {
					copyFileForQuotaPreflightTest(t, path+suffix, copyPath+suffix)
				}
			}
			_, err := inspectSQLiteQuotaWriter(context.Background(), copyPath)
			require.ErrorContains(t, err, "requires both wal and shm")
			_, statErr := os.Stat(copyPath + missingSuffix)
			require.ErrorIs(t, statErr, os.ErrNotExist)
		})
	}
}

func TestSQLiteQuotaWriterPreflightReadsCleanWALWithConditionalImmutable(t *testing.T) {
	sourceDirectory := t.TempDir()
	path := filepath.Join(sourceDirectory, "clean.db")
	writer, err := sql.Open(sqlite.DriverName, sqliteFixtureURI(path))
	require.NoError(t, err)
	writer.SetMaxOpenConns(1)
	var journalMode string
	require.NoError(t, writer.QueryRow("PRAGMA journal_mode=WAL").Scan(&journalMode))
	require.Equal(t, "wal", strings.ToLower(journalMode))
	require.NoError(t, execQuotaWriterFixtureStatements(writer,
		`CREATE TABLE quota_writer_epochs (
id INTEGER PRIMARY KEY,
schema_version INTEGER NOT NULL,
mode TEXT NOT NULL,
epoch INTEGER NOT NULL,
lock_version INTEGER NOT NULL
)`,
		"INSERT INTO quota_writer_epochs (id, schema_version, mode, epoch, lock_version) VALUES (1, 1, 'authoritative', 52, 6)",
	))
	require.NoError(t, writer.Close())

	beforeNames := directoryNamesForQuotaPreflightTest(t, sourceDirectory)
	require.Equal(t, []string{"clean.db"}, beforeNames)
	before := readFileForQuotaPreflightTest(t, path)
	require.Equal(t, byte(2), before[18])
	require.Equal(t, byte(2), before[19])
	mode, err := inspectSQLiteQuotaWriter(context.Background(), path)
	require.NoError(t, err)
	assert.Equal(t, QuotaWriterModeAuthoritative, mode)
	assert.Equal(t, before, readFileForQuotaPreflightTest(t, path))
	assert.Equal(t, beforeNames, directoryNamesForQuotaPreflightTest(t, sourceDirectory))
	immutableURI, err := url.Parse(sqliteReadOnlyURI(path, true))
	require.NoError(t, err)
	assert.Equal(t, "ro", immutableURI.Query().Get("mode"))
	assert.Equal(t, "1", immutableURI.Query().Get("immutable"))
}

func TestSQLiteQuotaWriterReadOnlyContractRejectsWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "readonly.db")
	createQuotaWriterSQLiteFixture(t, path, quotaWriterFixtureRow{1, 1, string(QuotaWriterModeLegacy), 1, 1})
	before := readFileForQuotaPreflightTest(t, path)
	db, err := sql.Open(sqlite.DriverName, sqliteReadOnlyURI(path, false))
	require.NoError(t, err)
	defer db.Close()
	_, err = db.Exec("CREATE TABLE forbidden_write (id INTEGER)")
	require.Error(t, err)
	assert.Equal(t, before, readFileForQuotaPreflightTest(t, path))
	assert.Equal(t, []string{"readonly.db"}, directoryNamesForQuotaPreflightTest(t, filepath.Dir(path)))
}

func TestQuotaBatchPreflightSanitizesDatabaseErrors(t *testing.T) {
	secret := "preflight-secret-value"
	t.Setenv("SQL_DSN", "postgresql://user:"+secret+"@private.invalid/%zz")
	enabled, err := ValidateQuotaBatchPreMigrationConfiguration(context.Background(), "true", true)
	require.ErrorContains(t, err, "cannot verify quota writer mode")
	assert.False(t, enabled)
	assert.NotContains(t, err.Error(), secret)
	assert.NotContains(t, err.Error(), "private.invalid")
}

func TestQuotaWriterPreflightConfiguredDatabases(t *testing.T) {
	if os.Getenv("MYAPI_B2_DATABASE_TESTS") != "1" {
		t.Skip("disposable B2 database tests require MYAPI_B2_DATABASE_TESTS=1")
	}
	t.Run("mysql", testQuotaWriterPreflightMySQL)
	t.Run("postgres search path", testQuotaWriterPreflightPostgreSQL)
}

func quotaWriterPreflightFixtureSuffix(t *testing.T) string {
	t.Helper()
	random := make([]byte, 8)
	_, err := rand.Read(random)
	require.NoError(t, err)
	return hex.EncodeToString(random)
}

func testQuotaWriterPreflightMySQL(t *testing.T) {
	t.Helper()
	baseDSN := strings.TrimSpace(os.Getenv("MYAPI_B2_MYSQL_DSN"))
	require.NotEmpty(t, baseDSN)
	baseConfig, err := mysqldriver.ParseDSN(baseDSN)
	require.NoError(t, err)
	adminConnector, err := mysqldriver.NewConnector(baseConfig)
	require.NoError(t, err)
	admin := sql.OpenDB(adminConnector)
	t.Cleanup(func() { assert.NoError(t, admin.Close()) })
	databaseName := "myapi_quota_preflight_" + quotaWriterPreflightFixtureSuffix(t)
	_, err = admin.Exec("CREATE DATABASE `" + databaseName + "`")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, dropErr := admin.Exec("DROP DATABASE `" + databaseName + "`")
		assert.NoError(t, dropErr)
	})
	fixtureConfig := baseConfig.Clone()
	fixtureConfig.DBName = databaseName
	fixtureConnector, err := mysqldriver.NewConnector(fixtureConfig)
	require.NoError(t, err)
	fixture := sql.OpenDB(fixtureConnector)
	t.Cleanup(func() { assert.NoError(t, fixture.Close()) })

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mode, err := inspectRelationalQuotaWriter(ctx, fixture, common.DatabaseTypeMySQL)
	require.NoError(t, err)
	assert.Equal(t, QuotaWriterModeLegacy, mode)
	require.NoError(t, execQuotaWriterFixtureStatements(fixture,
		`CREATE TABLE quota_writer_epochs (
id BIGINT PRIMARY KEY,
schema_version BIGINT NOT NULL,
mode VARCHAR(24) NOT NULL,
epoch BIGINT NOT NULL,
lock_version BIGINT NOT NULL
)`,
		"INSERT INTO quota_writer_epochs (id, schema_version, mode, epoch, lock_version) VALUES (1, 1, 'authoritative', 71, 8)",
	))
	mode, err = inspectRelationalQuotaWriter(ctx, fixture, common.DatabaseTypeMySQL)
	require.NoError(t, err)
	assert.Equal(t, QuotaWriterModeAuthoritative, mode)
	assertQuotaWriterReadOnlyTransactionRejectsUpdate(t, ctx, fixture)
}

func testQuotaWriterPreflightPostgreSQL(t *testing.T) {
	t.Helper()
	baseDSN := strings.TrimSpace(os.Getenv("MYAPI_B2_POSTGRES_DSN"))
	require.NotEmpty(t, baseDSN)
	baseConfig, err := pgx.ParseConfig(baseDSN)
	require.NoError(t, err)
	admin := stdlib.OpenDB(*baseConfig)
	t.Cleanup(func() { assert.NoError(t, admin.Close()) })
	suffix := quotaWriterPreflightFixtureSuffix(t)
	firstSchema := "quota_preflight_first_" + suffix
	secondSchema := "quota_preflight_second_" + suffix
	for _, schema := range []string{firstSchema, secondSchema} {
		_, err = admin.Exec(`CREATE SCHEMA "` + schema + `"`)
		require.NoError(t, err)
		schemaToDrop := schema
		t.Cleanup(func() {
			_, dropErr := admin.Exec(`DROP SCHEMA "` + schemaToDrop + `" CASCADE`)
			assert.NoError(t, dropErr)
		})
	}
	fixtureConfig := baseConfig.Copy()
	fixtureConfig.RuntimeParams["search_path"] = firstSchema + "," + secondSchema
	fixtureConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	fixture := stdlib.OpenDB(*fixtureConfig)
	t.Cleanup(func() { assert.NoError(t, fixture.Close()) })

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mode, err := inspectRelationalQuotaWriter(ctx, fixture, common.DatabaseTypePostgreSQL)
	require.NoError(t, err)
	assert.Equal(t, QuotaWriterModeLegacy, mode)
	_, err = admin.Exec(`CREATE TABLE "` + secondSchema + `".quota_writer_epochs (
id BIGINT PRIMARY KEY,
schema_version BIGINT NOT NULL,
mode VARCHAR(24) NOT NULL,
epoch BIGINT NOT NULL,
lock_version BIGINT NOT NULL
)`)
	require.NoError(t, err)
	_, err = admin.Exec(`INSERT INTO "` + secondSchema + `".quota_writer_epochs
(id, schema_version, mode, epoch, lock_version) VALUES (1, 1, 'authoritative', 81, 9)`)
	require.NoError(t, err)
	mode, err = inspectRelationalQuotaWriter(ctx, fixture, common.DatabaseTypePostgreSQL)
	require.NoError(t, err)
	assert.Equal(t, QuotaWriterModeAuthoritative, mode)
	assertQuotaWriterReadOnlyTransactionRejectsUpdate(t, ctx, fixture)
}

func assertQuotaWriterReadOnlyTransactionRejectsUpdate(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	var beforeMode string
	var beforeEpoch, beforeLockVersion int64
	require.NoError(t, db.QueryRowContext(ctx, "SELECT mode, epoch, lock_version FROM quota_writer_epochs WHERE id = 1").Scan(&beforeMode, &beforeEpoch, &beforeLockVersion))
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, "UPDATE quota_writer_epochs SET epoch = epoch + 1 WHERE id = 1")
	require.Error(t, err)
	require.NoError(t, tx.Rollback())
	var afterMode string
	var afterEpoch, afterLockVersion int64
	require.NoError(t, db.QueryRowContext(ctx, "SELECT mode, epoch, lock_version FROM quota_writer_epochs WHERE id = 1").Scan(&afterMode, &afterEpoch, &afterLockVersion))
	assert.Equal(t, beforeMode, afterMode)
	assert.Equal(t, beforeEpoch, afterEpoch)
	assert.Equal(t, beforeLockVersion, afterLockVersion)
}
