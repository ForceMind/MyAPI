package model

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
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
	quotaExecutionSQLProbeTimeout    = 10 * time.Second
	quotaExecutionSQLiteProbeTimeout = 60 * time.Second
)

var ErrQuotaExecutionStartupUnverifiable = errors.New("cannot verify account quota execution gate")

var quotaExecutionSettlementTables = []string{
	"account_quota_settlement_intents",
	"account_quota_settlement_facts",
}

// ValidateQuotaExecutionPreMigrationConfiguration verifies the execution gate
// before GORM opens or migrates the primary database. The returned error never
// contains a DSN, path, event identity, or stored accounting value.
func ValidateQuotaExecutionPreMigrationConfiguration(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	target, err := resolveDatabaseTarget("SQL_DSN", false)
	if err != nil {
		return ErrQuotaExecutionStartupUnverifiable
	}
	if err := inspectQuotaExecutionTarget(ctx, target); err != nil {
		return ErrQuotaExecutionStartupUnverifiable
	}
	return nil
}

func inspectQuotaExecutionTarget(ctx context.Context, target databaseTarget) error {
	switch target.databaseType {
	case common.DatabaseTypeSQLite:
		probeContext, cancel := context.WithTimeout(ctx, quotaExecutionSQLiteProbeTimeout)
		defer cancel()
		return inspectSQLiteQuotaExecution(probeContext, target.dsn)
	case common.DatabaseTypeMySQL:
		probeContext, cancel := context.WithTimeout(ctx, quotaExecutionSQLProbeTimeout)
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
		return inspectRelationalQuotaExecution(probeContext, db, common.DatabaseTypeMySQL)
	case common.DatabaseTypePostgreSQL:
		probeContext, cancel := context.WithTimeout(ctx, quotaExecutionSQLProbeTimeout)
		defer cancel()
		config, err := pgx.ParseConfig(target.dsn)
		if err != nil {
			return err
		}
		config.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
		db := stdlib.OpenDB(*config)
		defer db.Close()
		return inspectRelationalQuotaExecution(probeContext, db, common.DatabaseTypePostgreSQL)
	default:
		return fmt.Errorf("unsupported primary database type")
	}
}

func inspectSQLiteQuotaExecution(ctx context.Context, dsn string) error {
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
	var cleanWALDigest [sha256.Size]byte
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
		return inspectQuotaExecutionTx(ctx, tx, common.DatabaseTypeSQLite)
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

func inspectRelationalQuotaExecution(ctx context.Context, db *sql.DB, databaseType common.DatabaseType) error {
	db.SetMaxOpenConns(1)
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	return inspectQuotaExecutionTx(ctx, tx, databaseType)
}

type quotaExecutionQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func inspectQuotaExecutionTx(ctx context.Context, queryer quotaExecutionQueryer, databaseType common.DatabaseType) error {
	tables, err := quotaExecutionTables(ctx, queryer, databaseType)
	if err != nil {
		return err
	}
	for name, kind := range tables {
		if !quotaAmountBaseTable(databaseType, kind) {
			return fmt.Errorf("account quota execution relation %s is not a base table", name)
		}
	}

	_, hasGate := tables[AccountQuotaExecutionGateTableName]
	_, hasPermit := tables[AccountQuotaExecutionPermitTableName]
	if hasGate != hasPermit {
		return fmt.Errorf("account quota execution gate schema is partial")
	}
	if !hasGate {
		// Automatic initialization is reserved for databases with no settlement
		// history at all. Even terminal history requires an explicit staging
		// operation so startup cannot silently bless an old accounting lineage.
		for _, table := range quotaExecutionSettlementTables {
			if _, exists := tables[table]; !exists {
				continue
			}
			var marker int
			err := queryer.QueryRowContext(ctx, "SELECT 1 FROM "+quotaExecutionQuote(databaseType, table)+" LIMIT 1").Scan(&marker)
			if err == nil {
				return fmt.Errorf("historical account quota settlement exists without an execution gate")
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return err
			}
		}
		return nil
	}

	if err := inspectQuotaExecutionGate(ctx, queryer, databaseType); err != nil {
		return err
	}
	if err := inspectQuotaExecutionSettlementStates(ctx, queryer, databaseType, tables); err != nil {
		return err
	}
	return inspectQuotaExecutionPermits(ctx, queryer, databaseType)
}

func quotaExecutionTables(ctx context.Context, queryer quotaExecutionQueryer, databaseType common.DatabaseType) (map[string]string, error) {
	var query string
	switch databaseType {
	case common.DatabaseTypeSQLite:
		query = `SELECT name, type FROM sqlite_master WHERE type IN ('table', 'view') AND name IN (` + quotaExecutionTableLiterals() + `)`
	case common.DatabaseTypeMySQL:
		query = `SELECT TABLE_NAME, TABLE_TYPE FROM information_schema.tables WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME IN (` + quotaExecutionTableLiterals() + `)`
	case common.DatabaseTypePostgreSQL:
		query = `SELECT c.relname, c.relkind::text
FROM pg_catalog.pg_class AS c
JOIN pg_catalog.pg_namespace AS n ON n.oid = c.relnamespace
WHERE n.nspname = ANY (current_schemas(false))
  AND pg_catalog.pg_table_is_visible(c.oid)
  AND c.relname IN (` + quotaExecutionTableLiterals() + `)`
	default:
		return nil, fmt.Errorf("unsupported database type")
	}
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
	return tables, rows.Err()
}

func quotaExecutionTableLiterals() string {
	return "'" + strings.Join([]string{
		AccountQuotaExecutionGateTableName,
		AccountQuotaExecutionPermitTableName,
		quotaExecutionSettlementTables[0],
		quotaExecutionSettlementTables[1],
		(AccountQuotaMutationReceipt{}).TableName(),
	}, "', '") + "'"
}

func inspectQuotaExecutionGate(ctx context.Context, queryer quotaExecutionQueryer, databaseType common.DatabaseType) error {
	query := "SELECT " + strings.Join([]string{
		quotaExecutionQuote(databaseType, "id"),
		quotaExecutionQuote(databaseType, "schema_version"),
		quotaExecutionQuote(databaseType, "state"),
		quotaExecutionQuote(databaseType, "created_at"),
		quotaExecutionQuote(databaseType, "updated_at"),
	}, ", ") + " FROM " + quotaExecutionQuote(databaseType, AccountQuotaExecutionGateTableName) + " LIMIT 2"
	rows, err := queryer.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()
	var id int64
	var schemaVersion int
	var state string
	var createdAt, updatedAt int64
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return err
		}
		return fmt.Errorf("account quota execution gate singleton is missing")
	}
	if err := rows.Scan(&id, &schemaVersion, &state, &createdAt, &updatedAt); err != nil {
		return err
	}
	if rows.Next() {
		return fmt.Errorf("account quota execution gate contains multiple rows")
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if id != AccountQuotaExecutionGateSingletonID || schemaVersion != AccountQuotaExecutionGateSchemaVersion ||
		state != AccountQuotaExecutionGateStateEnforcing || createdAt <= 0 || updatedAt < createdAt {
		return fmt.Errorf("account quota execution gate sentinel does not match")
	}
	return nil
}

func inspectQuotaExecutionSettlementStates(ctx context.Context, queryer quotaExecutionQueryer, databaseType common.DatabaseType, tables map[string]string) error {
	validKinds := "'authoritative', 'legacy_wallet', 'legacy_self_use', 'legacy_subscription'"
	for _, table := range quotaExecutionSettlementTables {
		if _, exists := tables[table]; !exists {
			continue
		}
		quotedTable := quotaExecutionQuote(databaseType, table)
		kind := quotaExecutionQuote(databaseType, "kind")
		state := quotaExecutionQuote(databaseType, "state")
		bindingColumns := []string{
			"id", "event_key", "request_id", "kind", "user_id", "token_id", "subscription_id",
			"reserve_receipt_id", "writer_epoch", "actual_quota", "delta", "apply_token",
			"request_fingerprint", "state", "fact_id", "lock_version",
		}
		if table == "account_quota_settlement_facts" {
			bindingColumns = []string{
				"id", "event_key", "request_id", "kind", "user_id", "token_id", "subscription_id",
				"reserve_receipt_id", "writer_epoch", "actual_quota", "delta", "apply_token",
				"request_fingerprint", "state", "funding_applied", "token_applied", "terminal_receipt_id",
			}
		}
		nullChecks := make([]string, len(bindingColumns))
		for i, column := range bindingColumns {
			nullChecks[i] = quotaExecutionQuote(databaseType, column) + " IS NULL"
		}
		query := "SELECT 1 FROM " + quotedTable + " WHERE " + strings.Join(nullChecks, " OR ") + " LIMIT 1"
		var marker int
		err := queryer.QueryRowContext(ctx, query).Scan(&marker)
		if err == nil {
			return fmt.Errorf("account quota settlement binding contains null fields")
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		validStates := "'pending', 'claimed', 'retryable', 'applied', 'manual'"
		if table == "account_quota_settlement_intents" {
			validStates = "'pending', 'materialized', 'applied', 'manual'"
		}
		query = "SELECT 1 FROM " + quotedTable + " WHERE " + kind + " NOT IN (" + validKinds + ") OR " + state + " NOT IN (" + validStates + ") LIMIT 1"
		err = queryer.QueryRowContext(ctx, query).Scan(&marker)
		if err == nil {
			return fmt.Errorf("account quota settlement state cannot be verified")
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	return inspectQuotaExecutionAppliedTerminals(ctx, queryer, databaseType, tables)
}

func inspectQuotaExecutionAppliedTerminals(ctx context.Context, queryer quotaExecutionQueryer, databaseType common.DatabaseType, tables map[string]string) error {
	factTable := (AccountQuotaSettlementFact{}).TableName()
	if _, exists := tables[factTable]; !exists {
		return nil
	}
	columns := []string{
		"id", "request_id", "kind", "user_id", "token_id", "subscription_id", "reserve_receipt_id",
		"writer_epoch", "actual_quota", "state", "funding_applied", "token_applied", "terminal_receipt_id",
	}
	quotedColumns := make([]string, len(columns))
	for index, column := range columns {
		quotedColumns[index] = quotaExecutionQuote(databaseType, column)
	}
	query := "SELECT " + strings.Join(quotedColumns, ", ") +
		" FROM " + quotaExecutionQuote(databaseType, factTable) +
		" WHERE " + quotaExecutionQuote(databaseType, "state") + " = 'applied'"
	rows, err := queryer.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	var facts []AccountQuotaSettlementFact
	for rows.Next() {
		var fact AccountQuotaSettlementFact
		if err := rows.Scan(
			&fact.ID, &fact.RequestID, &fact.Kind, &fact.UserID, &fact.TokenID, &fact.SubscriptionID,
			&fact.ReserveReceiptID, &fact.WriterEpoch, &fact.ActualQuota, &fact.State,
			&fact.FundingApplied, &fact.TokenApplied, &fact.TerminalReceiptID,
		); err != nil {
			rows.Close()
			return err
		}
		facts = append(facts, fact)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}

	receiptTable := (AccountQuotaMutationReceipt{}).TableName()
	for index := range facts {
		fact := &facts[index]
		if !fact.FundingApplied || !fact.TokenApplied {
			return fmt.Errorf("account quota settlement terminal state cannot be verified: %w", ErrAccountQuotaTerminalRecoveryConflict)
		}
		if fact.Kind != AccountQuotaSettlementKindAuthoritative {
			continue
		}
		if fact.TerminalReceiptID <= 0 {
			return fmt.Errorf("account quota settlement terminal receipt cannot be verified: %w", ErrAccountQuotaTerminalRecoveryConflict)
		}
		if _, exists := tables[receiptTable]; !exists {
			return fmt.Errorf("account quota settlement terminal receipt cannot be verified: receipt relation is missing")
		}
		receiptColumns := []string{
			"id", "receipt_version", "fingerprint_version", "writer_epoch", "request_id", "phase", "event_key",
			"parent_receipt_id", "audit_key", "reserve_fingerprint", "request_fingerprint", "billing_preference",
			"user_id", "token_id", "subscription_id", "requested_quota", "billing_context",
		}
		quotedReceiptColumns := make([]string, len(receiptColumns))
		for columnIndex, column := range receiptColumns {
			quotedReceiptColumns[columnIndex] = quotaExecutionQuote(databaseType, column)
		}
		placeholder := "?"
		if databaseType == common.DatabaseTypePostgreSQL {
			placeholder = "$1"
		}
		var terminal AccountQuotaMutationReceipt
		err := queryer.QueryRowContext(ctx,
			"SELECT "+strings.Join(quotedReceiptColumns, ", ")+" FROM "+quotaExecutionQuote(databaseType, receiptTable)+
				" WHERE "+quotaExecutionQuote(databaseType, "id")+" = "+placeholder+" LIMIT 1",
			fact.TerminalReceiptID,
		).Scan(
			&terminal.ID, &terminal.ReceiptVersion, &terminal.FingerprintVersion, &terminal.WriterEpoch,
			&terminal.RequestID, &terminal.Phase, &terminal.EventKey, &terminal.ParentReceiptID,
			&terminal.AuditKey, &terminal.ReserveFingerprint, &terminal.RequestFingerprint, &terminal.BillingPreference,
			&terminal.UserID, &terminal.TokenID, &terminal.SubscriptionID, &terminal.RequestedQuota, &terminal.BillingContext,
		)
		if err != nil {
			return fmt.Errorf("account quota settlement terminal receipt cannot be verified: %w", err)
		}
		if err := validateAccountQuotaSettlementAppliedTerminal(fact, &terminal); err != nil {
			return fmt.Errorf("account quota settlement terminal receipt cannot be verified: %w", err)
		}
	}
	return nil
}

func inspectQuotaExecutionPermits(ctx context.Context, queryer quotaExecutionQueryer, databaseType common.DatabaseType) error {
	permitColumns := []string{
		"id", "event_key", "intent_id", "fact_id", "source", "binding_version", "binding_digest",
		"request_id", "request_fingerprint", "kind", "user_id", "token_id", "subscription_id",
		"reserve_receipt_id", "writer_epoch", "actual_quota", "delta", "apply_token",
		"initial_funding_applied", "initial_token_applied", "allow_funding", "allow_token",
		"state", "approval_id", "evidence_digest", "created_at",
	}
	for _, column := range []string{"event_key", "intent_id", "fact_id"} {
		condition := ""
		if column != "event_key" {
			condition = " WHERE " + quotaExecutionQuote(databaseType, column) + " > 0"
		}
		query := "SELECT 1 FROM " + quotaExecutionQuote(databaseType, AccountQuotaExecutionPermitTableName) + condition +
			" GROUP BY " + quotaExecutionQuote(databaseType, column) + " HAVING COUNT(*) > 1 LIMIT 1"
		var marker int
		err := queryer.QueryRowContext(ctx, query).Scan(&marker)
		if err == nil {
			return fmt.Errorf("account quota execution permit binding is duplicated")
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}

	intentColumns := []string{
		"id", "event_key", "request_id", "kind", "user_id", "token_id", "subscription_id",
		"reserve_receipt_id", "writer_epoch", "actual_quota", "delta", "apply_token",
		"request_fingerprint", "state", "fact_id", "lock_version",
	}
	factColumns := []string{
		"id", "event_key", "request_id", "kind", "user_id", "token_id", "subscription_id",
		"reserve_receipt_id", "writer_epoch", "actual_quota", "delta", "apply_token",
		"request_fingerprint", "state", "funding_applied", "token_applied",
	}
	selects := make([]string, 0, len(permitColumns)+len(intentColumns)+len(factColumns))
	for _, column := range permitColumns {
		selects = append(selects, quotaExecutionQualified(databaseType, "p", column))
	}
	for _, column := range intentColumns {
		fallback := "0"
		if column == "event_key" || column == "request_id" || column == "kind" || column == "request_fingerprint" || column == "state" {
			fallback = "''"
		} else if column == "apply_token" {
			// FALSE is a native boolean in PostgreSQL and a supported zero-valued
			// boolean literal in SQLite and MySQL 5.7+.
			fallback = "false"
		}
		selects = append(selects, "COALESCE("+quotaExecutionQualified(databaseType, "i", column)+", "+fallback+")")
	}
	for _, column := range factColumns {
		fallback := "0"
		if column == "event_key" || column == "request_id" || column == "kind" || column == "request_fingerprint" || column == "state" {
			fallback = "''"
		} else if column == "apply_token" || column == "funding_applied" || column == "token_applied" {
			fallback = "false"
		}
		selects = append(selects, "COALESCE("+quotaExecutionQualified(databaseType, "f", column)+", "+fallback+")")
	}
	query := "SELECT " + strings.Join(selects, ", ") +
		" FROM " + quotaExecutionQuote(databaseType, AccountQuotaExecutionPermitTableName) + " AS p" +
		" LEFT JOIN " + quotaExecutionQuote(databaseType, quotaExecutionSettlementTables[0]) + " AS i ON " +
		quotaExecutionQualified(databaseType, "i", "id") + " = " + quotaExecutionQualified(databaseType, "p", "intent_id") +
		" LEFT JOIN " + quotaExecutionQuote(databaseType, quotaExecutionSettlementTables[1]) + " AS f ON " +
		quotaExecutionQualified(databaseType, "f", "id") + " = " + quotaExecutionQualified(databaseType, "p", "fact_id")
	rows, err := queryer.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var permit AccountQuotaExecutionPermit
		var intent AccountQuotaSettlementIntent
		var fact AccountQuotaSettlementFact
		if err := rows.Scan(
			&permit.ID, &permit.EventKey, &permit.IntentID, &permit.FactID, &permit.Source, &permit.BindingVersion, &permit.BindingDigest,
			&permit.RequestID, &permit.RequestFingerprint, &permit.Kind, &permit.UserID, &permit.TokenID, &permit.SubscriptionID,
			&permit.ReserveReceiptID, &permit.WriterEpoch, &permit.ActualQuota, &permit.Delta, &permit.ApplyToken,
			&permit.InitialFundingApplied, &permit.InitialTokenApplied, &permit.AllowFunding, &permit.AllowToken,
			&permit.State, &permit.ApprovalID, &permit.EvidenceDigest, &permit.CreatedAt,
			&intent.ID, &intent.EventKey, &intent.RequestID, &intent.Kind, &intent.UserID, &intent.TokenID, &intent.SubscriptionID,
			&intent.ReserveReceiptID, &intent.WriterEpoch, &intent.ActualQuota, &intent.Delta, &intent.ApplyToken,
			&intent.RequestFingerprint, &intent.State, &intent.FactID, &intent.LockVersion,
			&fact.ID, &fact.EventKey, &fact.RequestID, &fact.Kind, &fact.UserID, &fact.TokenID, &fact.SubscriptionID,
			&fact.ReserveReceiptID, &fact.WriterEpoch, &fact.ActualQuota, &fact.Delta, &fact.ApplyToken,
			&fact.RequestFingerprint, &fact.State, &fact.FundingApplied, &fact.TokenApplied,
		); err != nil {
			return err
		}
		var intentPtr *AccountQuotaSettlementIntent
		if intent.ID > 0 {
			intentPtr = &intent
		}
		var factPtr *AccountQuotaSettlementFact
		if fact.ID > 0 {
			factPtr = &fact
		}
		if err := inspectQuotaExecutionPermitBinding(&permit, intentPtr, factPtr); err != nil {
			return err
		}
	}
	return rows.Err()
}

func inspectQuotaExecutionPermitBinding(permit *AccountQuotaExecutionPermit, intent *AccountQuotaSettlementIntent, fact *AccountQuotaSettlementFact) error {
	if permit == nil {
		return fmt.Errorf("account quota execution permit is missing")
	}
	evidence, evidenceErr := hex.DecodeString(permit.EvidenceDigest)
	if permit.CreatedAt <= 0 || strings.TrimSpace(permit.ApprovalID) == "" || len(permit.ApprovalID) > 128 ||
		evidenceErr != nil || len(evidence) != sha256.Size || strings.ToLower(permit.EvidenceDigest) != permit.EvidenceDigest {
		return fmt.Errorf("account quota execution permit audit identity is invalid")
	}
	if permit.Source == AccountQuotaExecutionPermitSourceRuntimeV1 &&
		(permit.ApprovalID != AccountQuotaExecutionPermitSourceRuntimeV1 || permit.EvidenceDigest != permit.BindingDigest) {
		return fmt.Errorf("runtime account quota execution permit audit identity is invalid")
	}
	return validateAccountQuotaExecutionPermitBinding(permit, intent, fact)
}

func quotaExecutionQualified(databaseType common.DatabaseType, alias, column string) string {
	return alias + "." + quotaExecutionQuote(databaseType, column)
}

func quotaExecutionQuote(databaseType common.DatabaseType, identifier string) string {
	if databaseType == common.DatabaseTypeMySQL {
		return "`" + identifier + "`"
	}
	return `"` + identifier + `"`
}
