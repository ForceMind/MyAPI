package model

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/glebarez/sqlite"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

const (
	quotaAmountSQLProbeTimeout    = 10 * time.Second
	quotaAmountSQLiteProbeTimeout = 60 * time.Second
)

// ErrQuotaAmountStartupUnverifiable is returned before migration when the
// configured database cannot prove that persisted quota states are exact and
// wide enough. It intentionally carries no DSN, path, or stored amount.
var ErrQuotaAmountStartupUnverifiable = errors.New("cannot verify quota amount storage before migration")

type quotaAmountColumnSpec struct {
	table    string
	column   string
	minimum  int64
	maximum  int64
	nullable bool
}

type quotaAmountTableSpec struct {
	name     string
	required bool
	columns  []quotaAmountColumnSpec
}

var quotaAmountTableSpecs = []quotaAmountTableSpec{
	{
		name:     "users",
		required: true,
		columns: []quotaAmountColumnSpec{
			quotaAmountBalanceColumn("users", "quota", true),
			quotaAmountBalanceColumn("users", "used_quota", false),
		},
	},
	{
		name:     "tokens",
		required: true,
		columns: []quotaAmountColumnSpec{
			quotaAmountBalanceColumn("tokens", "remain_quota", true),
			quotaAmountBalanceColumn("tokens", "used_quota", false),
		},
	},
	{
		name: "channels",
		columns: []quotaAmountColumnSpec{
			quotaAmountBalanceColumn("channels", "used_quota", false),
		},
	},
	{
		name: "quota_data",
		columns: []quotaAmountColumnSpec{
			quotaAmountBalanceColumn("quota_data", "quota", false),
		},
	},
	{
		name: "user_quota_mutation_receipts",
		columns: []quotaAmountColumnSpec{
			quotaAmountTransitionColumn("user_quota_mutation_receipts", "delta"),
			quotaAmountBalanceColumn("user_quota_mutation_receipts", "quota_before", true),
			quotaAmountBalanceColumn("user_quota_mutation_receipts", "quota_after", true),
		},
	},
	{
		name: "subscription_plans",
		columns: []quotaAmountColumnSpec{
			quotaAmountBalanceColumn("subscription_plans", "total_amount", false),
		},
	},
	{
		name: "user_subscriptions",
		columns: []quotaAmountColumnSpec{
			quotaAmountBalanceColumn("user_subscriptions", "amount_total", false),
			quotaAmountBalanceColumn("user_subscriptions", "amount_used", false),
		},
	},
	{
		name: "subscription_pre_consume_records",
		columns: []quotaAmountColumnSpec{
			quotaAmountChargeColumn("subscription_pre_consume_records", "pre_consumed", false, false),
		},
	},
	{
		name: "account_quota_mutation_receipts",
		columns: []quotaAmountColumnSpec{
			quotaAmountChargeColumn("account_quota_mutation_receipts", "requested_quota", false, false),
			quotaAmountChargeColumn("account_quota_mutation_receipts", "applied_quota", false, false),
		},
	},
	{
		name: "account_quota_reservation_heads",
		columns: []quotaAmountColumnSpec{
			quotaAmountChargeColumn("account_quota_reservation_heads", "applied_quota", false, false),
		},
	},
	{
		name: "account_quota_terminal_recovery_obligations",
		columns: []quotaAmountColumnSpec{
			quotaAmountChargeColumn("account_quota_terminal_recovery_obligations", "actual_quota", false, false),
		},
	},
	{
		name: "account_quota_refund_facts",
		columns: []quotaAmountColumnSpec{
			quotaAmountChargeColumn("account_quota_refund_facts", "wallet_quota", false, false),
			quotaAmountChargeColumn("account_quota_refund_facts", "subscription_quota", false, false),
			quotaAmountChargeColumn("account_quota_refund_facts", "token_quota", false, false),
		},
	},
	{
		name: "account_quota_settlement_intents",
		columns: []quotaAmountColumnSpec{
			quotaAmountChargeColumn("account_quota_settlement_intents", "actual_quota", false, false),
			quotaAmountChargeColumn("account_quota_settlement_intents", "delta", true, false),
		},
	},
	{
		name: "account_quota_settlement_facts",
		columns: []quotaAmountColumnSpec{
			quotaAmountChargeColumn("account_quota_settlement_facts", "actual_quota", false, false),
			quotaAmountChargeColumn("account_quota_settlement_facts", "delta", true, false),
		},
	},
	{
		name: "quota_mutation_receipts",
		columns: []quotaAmountColumnSpec{
			quotaAmountChargeColumn("quota_mutation_receipts", "quota", false, false),
			quotaAmountChargeColumn("quota_mutation_receipts", "estimated_quota", false, false),
		},
	},
	{
		name: "task_submission_operations",
		columns: []quotaAmountColumnSpec{
			quotaAmountChargeColumn("task_submission_operations", "reserved_quota", false, false),
			quotaAmountChargeColumn("task_submission_operations", "estimated_quota", false, false),
		},
	},
	{
		name: "task_terminal_observations",
		columns: []quotaAmountColumnSpec{
			quotaAmountChargeColumn("task_terminal_observations", "actual_quota", false, false),
		},
	},
	{
		name: "task_billing_events",
		columns: []quotaAmountColumnSpec{
			quotaAmountChargeColumn("task_billing_events", "quota_delta", true, false),
		},
	},
	{
		name: "legacy_usage_reservations",
		columns: []quotaAmountColumnSpec{
			quotaAmountChargeColumn("legacy_usage_reservations", "reserved_quota", false, false),
			quotaAmountChargeColumn("legacy_usage_reservations", "token_reserved_quota", false, false),
			quotaAmountChargeColumn("legacy_usage_reservations", "actual_quota", false, true),
		},
	},
	{
		name: "usage_review_decisions",
		columns: []quotaAmountColumnSpec{
			quotaAmountChargeColumn("usage_review_decisions", "actual_quota", false, false),
		},
	},
	{
		name: "admin_quota_adjustments",
		columns: []quotaAmountColumnSpec{
			quotaAmountTransitionColumn("admin_quota_adjustments", "delta"),
			quotaAmountBalanceColumn("admin_quota_adjustments", "target_quota", false),
			quotaAmountBalanceColumn("admin_quota_adjustments", "applied_quota_before", true),
			quotaAmountBalanceColumn("admin_quota_adjustments", "applied_quota_after", true),
		},
	},
}

func quotaAmountBalanceColumn(table, column string, signed bool) quotaAmountColumnSpec {
	minimum := int64(0)
	if signed {
		minimum = -common.MaxQuotaBalance
	}
	return quotaAmountColumnSpec{table: table, column: column, minimum: minimum, maximum: common.MaxQuotaBalance}
}

func quotaAmountTransitionColumn(table, column string) quotaAmountColumnSpec {
	return quotaAmountColumnSpec{
		table: table, column: column,
		minimum: -common.MaxQuotaBalance,
		maximum: common.MaxQuotaBalance,
	}
}

func quotaAmountChargeColumn(table, column string, signed, nullable bool) quotaAmountColumnSpec {
	minimum := int64(0)
	if signed {
		minimum = int64(common.MinQuota)
	}
	return quotaAmountColumnSpec{
		table: table, column: column, minimum: minimum, maximum: int64(common.MaxQuota), nullable: nullable,
	}
}

type quotaAmountPhysicalColumn struct {
	dataType   string
	columnType string
	hidden     bool
}

// ValidateQuotaAmountPreMigrationConfiguration inspects the configured
// primary database through a native read-only connection. It never creates a
// database, changes a column, or runs a migration.
func ValidateQuotaAmountPreMigrationConfiguration(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	target, err := resolveDatabaseTarget("SQL_DSN", false)
	if err != nil {
		return ErrQuotaAmountStartupUnverifiable
	}
	if err := inspectQuotaAmountTarget(ctx, target); err != nil {
		if errors.Is(err, ErrQuotaAmountStartupUnverifiable) {
			return err
		}
		return ErrQuotaAmountStartupUnverifiable
	}
	return nil
}

func inspectQuotaAmountTarget(ctx context.Context, target databaseTarget) error {
	switch target.databaseType {
	case common.DatabaseTypeSQLite:
		probeContext, cancel := context.WithTimeout(ctx, quotaAmountSQLiteProbeTimeout)
		defer cancel()
		return inspectSQLiteQuotaAmounts(probeContext, target.dsn)
	case common.DatabaseTypeMySQL:
		probeContext, cancel := context.WithTimeout(ctx, quotaAmountSQLProbeTimeout)
		defer cancel()
		config, err := mysqldriver.ParseDSN(target.dsn)
		if err != nil {
			return err
		}
		connector, err := mysqldriver.NewConnector(config)
		if err != nil {
			return err
		}
		db := sql.OpenDB(connector)
		defer db.Close()
		return inspectRelationalQuotaAmounts(probeContext, db, common.DatabaseTypeMySQL)
	case common.DatabaseTypePostgreSQL:
		probeContext, cancel := context.WithTimeout(ctx, quotaAmountSQLProbeTimeout)
		defer cancel()
		config, err := pgx.ParseConfig(target.dsn)
		if err != nil {
			return err
		}
		config.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
		db := stdlib.OpenDB(*config)
		defer db.Close()
		return inspectRelationalQuotaAmounts(probeContext, db, common.DatabaseTypePostgreSQL)
	default:
		return fmt.Errorf("unsupported primary database type")
	}
}

func inspectSQLiteQuotaAmounts(ctx context.Context, dsn string) error {
	path, memory, err := sqliteDatabasePath(dsn)
	if err != nil {
		return err
	}
	if memory {
		return nil
	}
	state, err := inspectSQLiteStorage(path)
	if err != nil {
		return err
	}
	if state.fresh {
		return nil
	}

	immutable := false
	var cleanWALDigest [32]byte
	if state.cleanWAL {
		cleanWALDigest, err = fileDigest(ctx, state.path)
		if err != nil {
			return err
		}
		if err := verifyCleanWALSourceMetadata(state); err != nil {
			return err
		}
		immutable = true
	}

	db, err := sql.Open(sqlite.DriverName, sqliteReadOnlyURI(state.path, immutable))
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(1)
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
		return inspectSQLiteQuotaAmountTx(ctx, tx)
	}()
	if sidecarErr := verifySQLiteSidecars(state); sidecarErr != nil {
		return sidecarErr
	}
	if state.cleanWAL {
		if sourceErr := verifyCleanWALSourceMetadata(state); sourceErr != nil {
			return sourceErr
		}
		afterDigest, digestErr := fileDigest(ctx, state.path)
		if digestErr != nil {
			return digestErr
		}
		if afterDigest != cleanWALDigest {
			return fmt.Errorf("clean WAL SQLite database content changed during read-only inspection")
		}
		if sourceErr := verifyCleanWALSourceMetadata(state); sourceErr != nil {
			return sourceErr
		}
	}
	return probeErr
}

func inspectSQLiteQuotaAmountTx(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, "SELECT name, type FROM sqlite_master WHERE type IN ('table', 'view')")
	if err != nil {
		return err
	}
	tableKinds := make(map[string]string)
	for rows.Next() {
		var name, kind string
		if err := rows.Scan(&name, &kind); err != nil {
			rows.Close()
			return err
		}
		tableKinds[strings.ToLower(name)] = strings.ToLower(kind)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if quotaAmountSchemaIsFresh(tableKinds) {
		return nil
	}

	columns := make(map[string]map[string]quotaAmountPhysicalColumn)
	for _, table := range quotaAmountTableSpecs {
		kind, exists := tableKinds[table.name]
		if !exists {
			if table.required {
				return quotaAmountSchemaError(table.name, "required table is missing")
			}
			continue
		}
		if kind != "table" {
			return quotaAmountSchemaError(table.name, "relation is not a base table")
		}
		tableColumns, err := sqliteQuotaAmountColumns(ctx, tx, table.name)
		if err != nil {
			return err
		}
		columns[table.name] = tableColumns
	}
	if err := validateQuotaAmountPhysicalSchema(common.DatabaseTypeSQLite, tableKinds, columns); err != nil {
		return err
	}
	return inspectQuotaAmountValues(ctx, tx, common.DatabaseTypeSQLite, tableKinds)
}

func sqliteQuotaAmountColumns(ctx context.Context, tx *sql.Tx, table string) (map[string]quotaAmountPhysicalColumn, error) {
	rows, err := tx.QueryContext(ctx, "PRAGMA table_xinfo("+quoteSQLiteQuotaAmountIdentifier(table)+")")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	columns := make(map[string]quotaAmountPhysicalColumn)
	for rows.Next() {
		var cid, notNull, primaryKey, hidden int
		var name, declaredType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &declaredType, &notNull, &defaultValue, &primaryKey, &hidden); err != nil {
			return nil, err
		}
		columns[strings.ToLower(name)] = quotaAmountPhysicalColumn{
			dataType: strings.ToLower(strings.TrimSpace(declaredType)),
			hidden:   hidden != 0,
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return columns, nil
}

func inspectRelationalQuotaAmounts(ctx context.Context, db *sql.DB, databaseType common.DatabaseType) error {
	db.SetMaxOpenConns(1)
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	return inspectRelationalQuotaAmountTx(ctx, tx, databaseType)
}

func inspectRelationalQuotaAmountTx(ctx context.Context, tx *sql.Tx, databaseType common.DatabaseType) error {
	var catalogQuery, columnsQuery string
	tableNames := quotaAmountCatalogTableNames()
	switch databaseType {
	case common.DatabaseTypeMySQL:
		catalogQuery = fmt.Sprintf(`SELECT TABLE_NAME, TABLE_TYPE
FROM information_schema.tables
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME IN (%s)`, tableNames)
		columnsQuery = fmt.Sprintf(`SELECT TABLE_NAME, COLUMN_NAME, DATA_TYPE, COLUMN_TYPE
FROM information_schema.columns
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME IN (%s)`, tableNames)
	case common.DatabaseTypePostgreSQL:
		catalogQuery = fmt.Sprintf(`SELECT c.relname, c.relkind::text
FROM pg_catalog.pg_class AS c
JOIN pg_catalog.pg_namespace AS n ON n.oid = c.relnamespace
WHERE n.nspname = ANY (current_schemas(false))
  AND pg_catalog.pg_table_is_visible(c.oid)
  AND c.relname IN (%s)`, tableNames)
		columnsQuery = fmt.Sprintf(`SELECT c.relname, a.attname, t.typname, t.typname
FROM pg_catalog.pg_class AS c
JOIN pg_catalog.pg_namespace AS n ON n.oid = c.relnamespace
JOIN pg_catalog.pg_attribute AS a ON a.attrelid = c.oid
JOIN pg_catalog.pg_type AS t ON t.oid = a.atttypid
WHERE n.nspname = ANY (current_schemas(false))
  AND pg_catalog.pg_table_is_visible(c.oid)
  AND c.relname IN (%s)
  AND a.attnum > 0
  AND NOT a.attisdropped`, tableNames)
	default:
		return fmt.Errorf("unsupported relational database type")
	}

	tableKinds, err := relationalQuotaAmountTables(ctx, tx, catalogQuery)
	if err != nil {
		return err
	}
	if quotaAmountSchemaIsFresh(tableKinds) {
		return nil
	}
	columns, err := relationalQuotaAmountColumns(ctx, tx, columnsQuery)
	if err != nil {
		return err
	}
	if err := validateQuotaAmountPhysicalSchema(databaseType, tableKinds, columns); err != nil {
		return err
	}
	return inspectQuotaAmountValues(ctx, tx, databaseType, tableKinds)
}

func quotaAmountCatalogTableNames() string {
	names := make([]string, 0, len(quotaAmountTableSpecs))
	for _, table := range quotaAmountTableSpecs {
		names = append(names, "'"+table.name+"'")
	}
	return strings.Join(names, ", ")
}

type quotaAmountQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func relationalQuotaAmountTables(ctx context.Context, queryer quotaAmountQueryer, query string) (map[string]string, error) {
	rows, err := queryer.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tables := make(map[string]string)
	for rows.Next() {
		var name, kind string
		if err := rows.Scan(&name, &kind); err != nil {
			return nil, err
		}
		tables[strings.ToLower(name)] = strings.ToLower(kind)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return tables, nil
}

func relationalQuotaAmountColumns(ctx context.Context, queryer quotaAmountQueryer, query string) (map[string]map[string]quotaAmountPhysicalColumn, error) {
	rows, err := queryer.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	columns := make(map[string]map[string]quotaAmountPhysicalColumn)
	for rows.Next() {
		var table, column, dataType, columnType string
		if err := rows.Scan(&table, &column, &dataType, &columnType); err != nil {
			return nil, err
		}
		table = strings.ToLower(table)
		if columns[table] == nil {
			columns[table] = make(map[string]quotaAmountPhysicalColumn)
		}
		columns[table][strings.ToLower(column)] = quotaAmountPhysicalColumn{
			dataType:   strings.ToLower(strings.TrimSpace(dataType)),
			columnType: strings.ToLower(strings.TrimSpace(columnType)),
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return columns, nil
}

func quotaAmountSchemaIsFresh(tableKinds map[string]string) bool {
	for _, table := range quotaAmountTableSpecs {
		if _, exists := tableKinds[table.name]; exists {
			return false
		}
	}
	return true
}

func validateQuotaAmountPhysicalSchema(databaseType common.DatabaseType, tableKinds map[string]string, columns map[string]map[string]quotaAmountPhysicalColumn) error {
	for _, table := range quotaAmountTableSpecs {
		kind, exists := tableKinds[table.name]
		if !exists {
			if table.required {
				return quotaAmountSchemaError(table.name, "required table is missing")
			}
			continue
		}
		if !quotaAmountBaseTable(databaseType, kind) {
			return quotaAmountSchemaError(table.name, "relation is not a base table")
		}
		for _, spec := range table.columns {
			column, exists := columns[table.name][spec.column]
			if !exists {
				return quotaAmountSchemaError(spec.table+"."+spec.column, "required column is missing")
			}
			if !quotaAmountColumnIsExact(databaseType, column) {
				return quotaAmountSchemaError(spec.table+"."+spec.column, "column is not an exact signed integer storage type")
			}
		}
	}
	return nil
}

func quotaAmountBaseTable(databaseType common.DatabaseType, kind string) bool {
	switch databaseType {
	case common.DatabaseTypeSQLite:
		return strings.EqualFold(kind, "table")
	case common.DatabaseTypeMySQL:
		return strings.EqualFold(kind, "base table")
	case common.DatabaseTypePostgreSQL:
		return kind == "r" || kind == "p"
	default:
		return false
	}
}

func quotaAmountColumnIsExact(databaseType common.DatabaseType, column quotaAmountPhysicalColumn) bool {
	switch databaseType {
	case common.DatabaseTypeSQLite:
		return !column.hidden && strings.Contains(strings.ToUpper(column.dataType), "INT")
	case common.DatabaseTypeMySQL:
		return strings.EqualFold(column.dataType, "bigint") && !strings.Contains(strings.ToLower(column.columnType), "unsigned")
	case common.DatabaseTypePostgreSQL:
		return strings.EqualFold(column.dataType, "int8")
	default:
		return false
	}
}

func inspectQuotaAmountValues(ctx context.Context, queryer quotaAmountQueryer, databaseType common.DatabaseType, tableKinds map[string]string) error {
	for _, table := range quotaAmountTableSpecs {
		if _, exists := tableKinds[table.name]; !exists {
			continue
		}
		for _, spec := range table.columns {
			query := quotaAmountInvalidValueQuery(databaseType, spec)
			var marker int
			err := queryer.QueryRowContext(ctx, query, spec.minimum, spec.maximum).Scan(&marker)
			switch {
			case err == nil:
				return quotaAmountSchemaError(spec.table+"."+spec.column, "stored value is null, non-integer, or outside the exact range")
			case errors.Is(err, sql.ErrNoRows):
				continue
			default:
				return err
			}
		}
	}
	return nil
}

func quotaAmountInvalidValueQuery(databaseType common.DatabaseType, spec quotaAmountColumnSpec) string {
	var table, column, minimumPlaceholder, maximumPlaceholder string
	conditions := make([]string, 0, 3)
	switch databaseType {
	case common.DatabaseTypeSQLite:
		table = quoteSQLiteQuotaAmountIdentifier(spec.table)
		column = quoteSQLiteQuotaAmountIdentifier(spec.column)
		conditions = append(conditions, "typeof("+column+") <> 'integer'")
		minimumPlaceholder, maximumPlaceholder = "?", "?"
	case common.DatabaseTypeMySQL:
		table = "`" + spec.table + "`"
		column = "`" + spec.column + "`"
		minimumPlaceholder, maximumPlaceholder = "?", "?"
	case common.DatabaseTypePostgreSQL:
		table = `"` + spec.table + `"`
		column = `"` + spec.column + `"`
		minimumPlaceholder, maximumPlaceholder = "$1", "$2"
	}
	conditions = append(conditions,
		column+" < "+minimumPlaceholder,
		column+" > "+maximumPlaceholder,
	)
	invalidValue := strings.Join(conditions, " OR ")
	if spec.nullable {
		return "SELECT 1 FROM " + table + " WHERE " + column + " IS NOT NULL AND (" + invalidValue + ") LIMIT 1"
	}
	return "SELECT 1 FROM " + table + " WHERE " + column + " IS NULL OR " + invalidValue + " LIMIT 1"
}

func quoteSQLiteQuotaAmountIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

func quotaAmountSchemaError(object, reason string) error {
	return fmt.Errorf("%w: %s: %s", ErrQuotaAmountStartupUnverifiable, object, reason)
}
