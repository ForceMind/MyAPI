package model

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/glebarez/sqlite"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

const (
	quotaBatchConfigurationError  = "BATCH_UPDATE_ENABLED must be exactly true or false"
	quotaBatchRedisError          = "legacy or bridge quota batching requires Redis; restore the reviewed cache configuration before restarting; do not disable batching or clear accounting queues to bypass this check"
	quotaWriterSQLProbeTimeout    = 10 * time.Second
	quotaWriterSQLiteProbeTimeout = 60 * time.Second
)

// ErrQuotaWriterStartupUnverifiable is safe to return without exposing a DSN,
// database path, or driver error.
var ErrQuotaWriterStartupUnverifiable = errors.New("cannot verify quota writer mode for batch startup; review the database and cache configuration before restarting")

// These tables were introduced with durable accounting. An installation with
// any of them but no quota_writer_epochs table is only partially migrated, so
// treating it as a historical legacy database would make the startup decision
// ambiguous.
var quotaWriterDurableSchemaTables = []string{
	"user_quota_mutation_receipts", "account_quota_mutation_receipts", "account_quota_reservation_heads",
	"account_quota_terminal_recovery_obligations", "account_quota_refund_facts", "account_quota_settlement_intents",
	"account_quota_settlement_facts", "quota_projection_obligations", "quota_work_cursors", "quota_balance_batch_drains",
	"quota_balance_batch_subjects", "quota_mutation_receipts",
}

// ParseQuotaBatchUpdateEnabled interprets the effective startup setting. The
// exact spelling is intentional: accepting a typo here could silently change
// whether quota mutations are buffered.
func ParseQuotaBatchUpdateEnabled(configured string) (bool, error) {
	switch configured {
	case "", "false":
		return false, nil
	case "true":
		return true, nil
	default:
		return false, errors.New(quotaBatchConfigurationError)
	}
}

// ValidateQuotaBatchWriterMode applies the cache/writer compatibility rule to
// a mode that has already been read and validated.
func ValidateQuotaBatchWriterMode(mode QuotaWriterMode, redisAvailable bool) error {
	if !validQuotaWriterMode(mode) {
		return ErrQuotaWriterStartupUnverifiable
	}
	if redisAvailable || mode == QuotaWriterModeAuthoritative {
		return nil
	}
	return errors.New(quotaBatchRedisError)
}

// ValidateQuotaBatchPreMigrationConfiguration verifies persisted writer state
// before GORM opens the primary database. Disabling batching only skips the
// cache compatibility rule; it cannot make persistent writer identity optional.
func ValidateQuotaBatchPreMigrationConfiguration(ctx context.Context, configured string, redisAvailable bool) (bool, error) {
	enabled, err := ParseQuotaBatchUpdateEnabled(configured)
	if err != nil {
		return enabled, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	target, err := resolveDatabaseTarget("SQL_DSN", false)
	if err != nil {
		return false, ErrQuotaWriterStartupUnverifiable
	}
	mode, err := inspectQuotaWriterTarget(ctx, target)
	if err != nil {
		return false, ErrQuotaWriterStartupUnverifiable
	}
	if enabled {
		if err := ValidateQuotaBatchWriterMode(mode, redisAvailable); err != nil {
			return false, err
		}
	}
	return enabled, nil
}

func quotaWriterSchemaStatus(tables []string) (hasWriterTable bool, incompatibleTable string) {
	tableSet := make(map[string]struct{}, len(tables))
	for _, table := range tables {
		tableSet[strings.ToLower(table)] = struct{}{}
	}
	if _, ok := tableSet["quota_writer_epochs"]; ok {
		return true, ""
	}
	for _, table := range quotaWriterDurableSchemaTables {
		if _, ok := tableSet[table]; ok {
			return false, table
		}
	}
	return false, ""
}

func inspectQuotaWriterTarget(ctx context.Context, target databaseTarget) (QuotaWriterMode, error) {
	switch target.databaseType {
	case common.DatabaseTypeSQLite:
		probeContext, cancel := context.WithTimeout(ctx, quotaWriterSQLiteProbeTimeout)
		defer cancel()
		return inspectSQLiteQuotaWriter(probeContext, target.dsn)
	case common.DatabaseTypeMySQL:
		probeContext, cancel := context.WithTimeout(ctx, quotaWriterSQLProbeTimeout)
		defer cancel()
		config, err := mysqldriver.ParseDSN(target.dsn)
		if err != nil {
			return "", err
		}
		connector, err := mysqldriver.NewConnector(config)
		if err != nil {
			return "", err
		}
		db := sql.OpenDB(connector)
		defer db.Close()
		return inspectRelationalQuotaWriter(probeContext, db, common.DatabaseTypeMySQL)
	case common.DatabaseTypePostgreSQL:
		probeContext, cancel := context.WithTimeout(ctx, quotaWriterSQLProbeTimeout)
		defer cancel()
		config, err := pgx.ParseConfig(target.dsn)
		if err != nil {
			return "", err
		}
		config.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
		db := stdlib.OpenDB(*config)
		defer db.Close()
		return inspectRelationalQuotaWriter(probeContext, db, common.DatabaseTypePostgreSQL)
	default:
		return "", fmt.Errorf("unsupported primary database type")
	}
}

func inspectRelationalQuotaWriter(ctx context.Context, db *sql.DB, databaseType common.DatabaseType) (QuotaWriterMode, error) {
	db.SetMaxOpenConns(1)
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	var catalogQuery, writerQuery string
	var validTableKind func(string) bool
	switch databaseType {
	case common.DatabaseTypeMySQL:
		catalogQuery = "SELECT TABLE_NAME, TABLE_TYPE FROM information_schema.tables WHERE TABLE_SCHEMA = DATABASE()"
		writerQuery = "SELECT `id`, `schema_version`, `mode`, `epoch`, `lock_version` FROM `quota_writer_epochs` LIMIT 2"
		validTableKind = func(kind string) bool { return strings.EqualFold(kind, "BASE TABLE") }
	case common.DatabaseTypePostgreSQL:
		// pg_table_is_visible follows the complete effective search_path. This
		// matters when the first schema has no matching relation and an
		// unqualified GORM table resolves from a later schema.
		catalogQuery = `SELECT c.relname, c.relkind::text
FROM pg_catalog.pg_class AS c
JOIN pg_catalog.pg_namespace AS n ON n.oid = c.relnamespace
WHERE n.nspname = ANY (current_schemas(false))
  AND pg_catalog.pg_table_is_visible(c.oid)`
		writerQuery = `SELECT "id", "schema_version", "mode", "epoch", "lock_version" FROM "quota_writer_epochs" LIMIT 2`
		validTableKind = func(kind string) bool { return kind == "r" || kind == "p" }
	default:
		return "", fmt.Errorf("unsupported relational database type")
	}
	return inspectQuotaWriterTx(ctx, tx, catalogQuery, writerQuery, validTableKind)
}

type quotaWriterQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func inspectQuotaWriterTx(ctx context.Context, queryer quotaWriterQueryer, catalogQuery, writerQuery string, validTableKind func(string) bool) (QuotaWriterMode, error) {
	rows, err := queryer.QueryContext(ctx, catalogQuery)
	if err != nil {
		return "", err
	}
	var tables []string
	tableKinds := make(map[string]string)
	for rows.Next() {
		var table, kind string
		if err := rows.Scan(&table, &kind); err != nil {
			rows.Close()
			return "", err
		}
		tables = append(tables, table)
		tableKinds[strings.ToLower(table)] = kind
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return "", err
	}
	if err := rows.Close(); err != nil {
		return "", err
	}

	hasWriterTable, incompatibleTable := quotaWriterSchemaStatus(tables)
	if !hasWriterTable {
		if incompatibleTable != "" {
			if !validTableKind(tableKinds[incompatibleTable]) {
				return "", fmt.Errorf("durable accounting relation %s is not a base table", incompatibleTable)
			}
			return "", fmt.Errorf("durable table %s exists without quota_writer_epochs", incompatibleTable)
		}
		return QuotaWriterModeLegacy, nil
	}
	if !validTableKind(tableKinds["quota_writer_epochs"]) {
		return "", fmt.Errorf("quota_writer_epochs is not a base table")
	}

	rows, err = queryer.QueryContext(ctx, writerQuery)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var id, schemaVersion, epoch, lockVersion int64
	var mode string
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return "", err
		}
		return "", fmt.Errorf("quota_writer_epochs singleton is missing")
	}
	if err := rows.Scan(&id, &schemaVersion, &mode, &epoch, &lockVersion); err != nil {
		return "", err
	}
	if rows.Next() {
		return "", fmt.Errorf("quota_writer_epochs contains multiple rows")
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return validateQuotaWriterEpochFields(id, schemaVersion, mode, epoch, lockVersion)
}

type sqliteStorageState struct {
	path     string
	info     os.FileInfo
	fresh    bool
	cleanWAL bool
	sidecars map[string]bool
}

func inspectSQLiteQuotaWriter(ctx context.Context, dsn string) (QuotaWriterMode, error) {
	path, memory, err := sqliteDatabasePath(dsn)
	if err != nil {
		return "", err
	}
	if memory {
		return QuotaWriterModeLegacy, nil
	}
	state, err := inspectSQLiteStorage(path)
	if err != nil {
		return "", err
	}
	if state.fresh {
		return QuotaWriterModeLegacy, nil
	}

	immutable := false
	var cleanWALDigest [sha256.Size]byte
	if state.cleanWAL {
		// A cleanly closed WAL database has no sidecars. Opening it in ordinary
		// read-only mode may create a new shm file, so this narrowly verified
		// state uses immutable=1 on the source. This only checks the short
		// startup window; deployments must still stop every writer before an
		// accounting migration or recovery decision.
		cleanWALDigest, err = fileDigest(ctx, state.path)
		if err != nil {
			return "", err
		}
		if err := verifyCleanWALSourceMetadata(state); err != nil {
			return "", err
		}
		immutable = true
	}

	readOnlyURI := sqliteReadOnlyURI(state.path, immutable)
	db, err := sql.Open(sqlite.DriverName, readOnlyURI)
	if err != nil {
		return "", err
	}
	db.SetMaxOpenConns(1)
	var mode QuotaWriterMode
	probeErr := func() error {
		defer db.Close()
		conn, err := db.Conn(ctx)
		if err != nil {
			return err
		}
		defer conn.Close()
		if _, err := conn.ExecContext(ctx, "PRAGMA query_only=ON"); err != nil {
			return err
		}
		var queryOnly int
		if err := conn.QueryRowContext(ctx, "PRAGMA query_only").Scan(&queryOnly); err != nil {
			return err
		}
		if queryOnly != 1 {
			return fmt.Errorf("SQLite query_only could not be enabled")
		}
		tx, err := conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
		if err != nil {
			return err
		}
		defer tx.Rollback()
		mode, err = inspectQuotaWriterTx(
			ctx,
			tx,
			"SELECT name, type FROM sqlite_master WHERE type IN ('table', 'view')",
			"SELECT `id`, `schema_version`, `mode`, `epoch`, `lock_version` FROM `quota_writer_epochs` LIMIT 2",
			func(kind string) bool { return kind == "table" },
		)
		return err
	}()
	if sidecarErr := verifySQLiteSidecars(state); sidecarErr != nil {
		return "", sidecarErr
	}
	if state.cleanWAL {
		if sourceErr := verifyCleanWALSourceMetadata(state); sourceErr != nil {
			return "", sourceErr
		}
		afterDigest, digestErr := fileDigest(ctx, state.path)
		if digestErr != nil {
			return "", digestErr
		}
		if afterDigest != cleanWALDigest {
			return "", fmt.Errorf("clean WAL SQLite database content changed during read-only inspection")
		}
		if sourceErr := verifyCleanWALSourceMetadata(state); sourceErr != nil {
			return "", sourceErr
		}
	}
	if probeErr != nil {
		return "", probeErr
	}
	return mode, nil
}

func sqliteReadOnlyURI(absolutePath string, immutable bool) string {
	query := url.Values{"mode": []string{"ro"}}
	if immutable {
		query.Set("immutable", "1")
	}
	return (&url.URL{Scheme: "file", Path: absolutePath, RawQuery: query.Encode()}).String()
}

func sqliteDatabasePath(dsn string) (path string, memory bool, err error) {
	if strings.HasPrefix(dsn, "file:") {
		parsed, parseErr := url.Parse(dsn)
		if parseErr != nil {
			return "", false, parseErr
		}
		if parsed.Host != "" || parsed.User != nil || parsed.Fragment != "" {
			return "", false, fmt.Errorf("unsupported SQLite file URI")
		}
		query, queryErr := url.ParseQuery(parsed.RawQuery)
		if queryErr != nil {
			return "", false, queryErr
		}
		if strings.EqualFold(query.Get("mode"), "memory") {
			return "", true, nil
		}
		if parsed.Opaque != "" {
			path, err = url.PathUnescape(parsed.Opaque)
			if err != nil {
				return "", false, err
			}
		} else {
			path = parsed.Path
		}
	} else {
		path, _, _ = strings.Cut(dsn, "?")
	}
	if path == "" || path == ":memory:" || strings.ContainsRune(path, '\x00') {
		if strings.ContainsRune(path, '\x00') {
			return "", false, fmt.Errorf("invalid SQLite database path")
		}
		return "", true, nil
	}
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return "", false, err
	}
	return filepath.Clean(absolutePath), false, nil
}

func inspectSQLiteStorage(path string) (sqliteStorageState, error) {
	state := sqliteStorageState{path: path, sidecars: make(map[string]bool, 3)}
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		info, err := os.Lstat(path + suffix)
		switch {
		case err == nil:
			if !info.Mode().IsRegular() {
				return state, fmt.Errorf("SQLite sidecar is not a regular file")
			}
			state.sidecars[suffix] = true
		case os.IsNotExist(err):
			state.sidecars[suffix] = false
		default:
			return state, err
		}
	}

	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		if sqliteHasSidecar(state.sidecars) {
			return state, fmt.Errorf("SQLite database is missing while sidecars exist")
		}
		state.fresh = true
		return state, nil
	}
	if err != nil {
		return state, err
	}
	if !info.Mode().IsRegular() {
		return state, fmt.Errorf("SQLite database is not a regular file")
	}
	state.info = info
	if info.Size() == 0 {
		if sqliteHasSidecar(state.sidecars) {
			return state, fmt.Errorf("empty SQLite database has sidecars")
		}
		state.fresh = true
		return state, nil
	}

	file, err := os.Open(path)
	if err != nil {
		return state, err
	}
	header := make([]byte, 100)
	_, readErr := io.ReadFull(file, header)
	closeErr := file.Close()
	if readErr != nil {
		return state, readErr
	}
	if closeErr != nil {
		return state, closeErr
	}
	if string(header[:16]) != "SQLite format 3\x00" {
		return state, fmt.Errorf("invalid SQLite header")
	}
	switch {
	case header[18] == 1 && header[19] == 1:
		if sqliteHasSidecar(state.sidecars) {
			return state, fmt.Errorf("rollback-journal SQLite database has sidecars")
		}
	case header[18] == 2 && header[19] == 2:
		if state.sidecars["-journal"] || state.sidecars["-wal"] != state.sidecars["-shm"] {
			return state, fmt.Errorf("WAL SQLite database requires both wal and shm sidecars or neither")
		}
		state.cleanWAL = !state.sidecars["-wal"]
	default:
		return state, fmt.Errorf("unsupported SQLite journal header")
	}
	return state, nil
}

func sqliteHasSidecar(sidecars map[string]bool) bool {
	return sidecars["-wal"] || sidecars["-shm"] || sidecars["-journal"]
}

func verifySQLiteSidecars(before sqliteStorageState) error {
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		_, err := os.Lstat(before.path + suffix)
		exists := err == nil
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if exists != before.sidecars[suffix] {
			return fmt.Errorf("SQLite sidecar set changed during read-only inspection")
		}
	}
	return nil
}

func fileDigest(ctx context.Context, path string) (digest [sha256.Size]byte, err error) {
	file, err := os.Open(path)
	if err != nil {
		return digest, err
	}
	defer file.Close()
	hash := sha256.New()
	buffer := make([]byte, 128*1024)
	for {
		if err := ctx.Err(); err != nil {
			return digest, err
		}
		read, readErr := file.Read(buffer)
		if read > 0 {
			if _, err := hash.Write(buffer[:read]); err != nil {
				return digest, err
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return digest, readErr
		}
	}
	copy(digest[:], hash.Sum(nil))
	return digest, nil
}

func verifyCleanWALSourceMetadata(before sqliteStorageState) error {
	current, err := os.Lstat(before.path)
	if err != nil {
		return err
	}
	if before.info == nil || !os.SameFile(before.info, current) || current.Size() != before.info.Size() || current.Mode() != before.info.Mode() || !current.ModTime().Equal(before.info.ModTime()) {
		return fmt.Errorf("clean WAL SQLite database changed during read-only inspection")
	}
	if err := verifySQLiteSidecars(before); err != nil {
		return err
	}
	return nil
}
