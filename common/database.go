package common

import (
	"os"
	"strings"
)

type DatabaseType string

const (
	DatabaseTypeMySQL      DatabaseType = "mysql"
	DatabaseTypeSQLite     DatabaseType = "sqlite"
	DatabaseTypePostgreSQL DatabaseType = "postgres"
	DatabaseTypeClickHouse DatabaseType = "clickhouse"
)

var mainDatabaseType = DatabaseTypeSQLite
var logDatabaseType = DatabaseTypeSQLite

func MainDatabaseType() DatabaseType {
	return mainDatabaseType
}

func LogDatabaseType() DatabaseType {
	return logDatabaseType
}

func SetMainDatabaseType(databaseType DatabaseType) {
	mainDatabaseType = databaseType
}

func SetLogDatabaseType(databaseType DatabaseType) {
	logDatabaseType = databaseType
}

func SetDatabaseTypes(mainType DatabaseType, logType DatabaseType) {
	mainDatabaseType = mainType
	logDatabaseType = logType
}

func UsingMainDatabase(databaseType DatabaseType) bool {
	return mainDatabaseType == databaseType
}

func UsingLogDatabase(databaseType DatabaseType) bool {
	return logDatabaseType == databaseType
}

const (
	// DefaultSQLitePath is used for fresh MyAPI installations.
	DefaultSQLitePath = "my-api.db?_busy_timeout=30000"
	// LegacySQLitePath is retained as a read/write fallback for existing
	// installations that still have the historical database file.
	LegacySQLitePath = "one-api.db?_busy_timeout=30000"
)

// SQLitePath is the effective SQLite DSN.  InitEnv resolves it to the legacy
// path only when the canonical file does not exist and the legacy file does.
// An explicit SQLITE_PATH always wins, so operators can select either file.
var SQLitePath = DefaultSQLitePath

// ResolveSQLitePath selects the canonical SQLite file for new installations
// while preserving existing data without requiring a schema migration.
func ResolveSQLitePath(configured string) string {
	return resolveSQLitePath(configured, sqliteFileExists)
}

func resolveSQLitePath(configured string, fileExists func(string) bool) string {
	if configured = strings.TrimSpace(configured); configured != "" {
		return configured
	}
	canonicalFile := sqliteDSNFile(DefaultSQLitePath)
	legacyFile := sqliteDSNFile(LegacySQLitePath)
	if !fileExists(canonicalFile) && fileExists(legacyFile) {
		return LegacySQLitePath
	}
	return DefaultSQLitePath
}

func sqliteDSNFile(dsn string) string {
	if file, _, ok := strings.Cut(dsn, "?"); ok {
		return file
	}
	return dsn
}

func sqliteFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
