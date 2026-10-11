package model

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitDBQuotaAmountPreflightRejectsBeforeOpeningOrMigratingSQLite(t *testing.T) {
	tests := []struct {
		name       string
		statements []string
		field      string
	}{
		{
			name: "unsafe physical schema",
			statements: []string{
				`CREATE TABLE users (quota REAL, used_quota INTEGER)`,
				`CREATE TABLE tokens (remain_quota INTEGER, used_quota INTEGER)`,
				`INSERT INTO users (quota, used_quota) VALUES (1, 0)`,
			},
			field: "users.quota",
		},
		{
			name: "amount outside exact range",
			statements: []string{
				`CREATE TABLE users (quota INTEGER, used_quota INTEGER)`,
				`CREATE TABLE tokens (remain_quota INTEGER, used_quota INTEGER)`,
				`INSERT INTO users (quota, used_quota) VALUES (9007199254740992, 0)`,
			},
			field: "users.quota",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			path := filepath.Join(directory, "unsafe.db")
			createQuotaAmountSQLiteFixture(t, path, test.statements...)
			before := readFileForQuotaPreflightTest(t, path)
			beforeNames := directoryNamesForQuotaPreflightTest(t, directory)

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
			t.Setenv("BATCH_UPDATE_ENABLED", "false")

			err := InitDB()
			require.ErrorIs(t, err, ErrQuotaAmountStartupUnverifiable)
			assert.ErrorContains(t, err, test.field)
			assert.Same(t, previousDB, DB)
			assert.Equal(t, before, readFileForQuotaPreflightTest(t, path))
			assert.Equal(t, beforeNames, directoryNamesForQuotaPreflightTest(t, directory))
		})
	}
}

func TestSQLiteQuotaAmountPreflightReadsLiveAndCleanWALWithoutWriting(t *testing.T) {
	t.Run("live WAL", func(t *testing.T) {
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
			`CREATE TABLE users (quota INTEGER, used_quota INTEGER)`,
			`CREATE TABLE tokens (remain_quota INTEGER, used_quota INTEGER)`,
			`INSERT INTO users (quota, used_quota) VALUES (9007199254740992, 0)`,
			`INSERT INTO tokens (remain_quota, used_quota) VALUES (0, 0)`,
		))
		var busy, logFrames, checkpointed int
		require.NoError(t, writer.QueryRow("PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &logFrames, &checkpointed))
		require.Zero(t, busy)
		_, err = writer.Exec("UPDATE users SET quota = ?", common.MaxQuotaBalance)
		require.NoError(t, err)

		beforeNames := directoryNamesForQuotaPreflightTest(t, directory)
		require.Contains(t, beforeNames, "live.db-wal")
		require.Contains(t, beforeNames, "live.db-shm")
		mainBefore := readFileForQuotaPreflightTest(t, path)
		walBefore := readFileForQuotaPreflightTest(t, path+"-wal")

		require.NoError(t, inspectSQLiteQuotaAmounts(context.Background(), path+"?_pragma=journal_mode(DELETE)&immutable=1"))
		assert.Equal(t, mainBefore, readFileForQuotaPreflightTest(t, path))
		assert.Equal(t, walBefore, readFileForQuotaPreflightTest(t, path+"-wal"))
		assert.Equal(t, beforeNames, directoryNamesForQuotaPreflightTest(t, directory))
	})

	t.Run("clean WAL", func(t *testing.T) {
		directory := t.TempDir()
		path := filepath.Join(directory, "clean.db")
		writer, err := sql.Open(sqlite.DriverName, sqliteFixtureURI(path))
		require.NoError(t, err)
		writer.SetMaxOpenConns(1)
		var journalMode string
		require.NoError(t, writer.QueryRow("PRAGMA journal_mode=WAL").Scan(&journalMode))
		require.Equal(t, "wal", strings.ToLower(journalMode))
		require.NoError(t, execQuotaWriterFixtureStatements(writer,
			`CREATE TABLE users (quota INTEGER, used_quota INTEGER)`,
			`CREATE TABLE tokens (remain_quota INTEGER, used_quota INTEGER)`,
			`INSERT INTO users (quota, used_quota) VALUES (-9007199254740991, 9007199254740991)`,
			`INSERT INTO tokens (remain_quota, used_quota) VALUES (9007199254740991, 9007199254740991)`,
		))
		require.NoError(t, writer.Close())

		beforeNames := directoryNamesForQuotaPreflightTest(t, directory)
		require.Equal(t, []string{"clean.db"}, beforeNames)
		before := readFileForQuotaPreflightTest(t, path)
		require.Equal(t, byte(2), before[18])
		require.Equal(t, byte(2), before[19])

		require.NoError(t, inspectSQLiteQuotaAmounts(context.Background(), path))
		assert.Equal(t, before, readFileForQuotaPreflightTest(t, path))
		assert.Equal(t, beforeNames, directoryNamesForQuotaPreflightTest(t, directory))
	})
}

func TestQuotaStartupPreflightsAllowAuthoritativeBatchWithoutRedis(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "authoritative.db")
	createQuotaWriterSQLiteFixture(t, path, quotaWriterFixtureRow{1, 1, string(QuotaWriterModeAuthoritative), 11, 2})
	createQuotaAmountSQLiteFixture(t, path,
		`CREATE TABLE users (quota INTEGER, used_quota INTEGER)`,
		`CREATE TABLE tokens (remain_quota INTEGER, used_quota INTEGER)`,
		`INSERT INTO users (quota, used_quota) VALUES (-9007199254740991, 9007199254740991)`,
		`INSERT INTO tokens (remain_quota, used_quota) VALUES (9007199254740991, 9007199254740991)`,
	)
	before := readFileForQuotaPreflightTest(t, path)
	beforeNames := directoryNamesForQuotaPreflightTest(t, directory)

	previousPath := common.SQLitePath
	previousRedisClient, previousRedisEnabled := common.RDB, common.RedisEnabled
	common.SQLitePath = path
	common.RDB = nil
	common.RedisEnabled = false
	t.Cleanup(func() {
		common.SQLitePath = previousPath
		common.RDB, common.RedisEnabled = previousRedisClient, previousRedisEnabled
	})
	t.Setenv("SQL_DSN", "local")

	enabled, err := ValidateQuotaBatchPreMigrationConfiguration(context.Background(), "true", false)
	require.NoError(t, err)
	assert.True(t, enabled)
	require.NoError(t, ValidateQuotaAmountPreMigrationConfiguration(context.Background()))
	assert.Equal(t, before, readFileForQuotaPreflightTest(t, path))
	assert.Equal(t, beforeNames, directoryNamesForQuotaPreflightTest(t, directory))
}
