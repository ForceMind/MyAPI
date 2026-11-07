package model

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/ForceMind/MyAPI/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func quotaAmountFixtureURI(path string) string {
	return (&url.URL{Scheme: "file", Path: path}).String()
}

func createQuotaAmountSQLiteFixture(t *testing.T, path string, statements ...string) {
	t.Helper()
	db, err := sql.Open(sqlite.DriverName, quotaAmountFixtureURI(path))
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	for _, statement := range statements {
		_, err := db.Exec(statement)
		require.NoError(t, err)
	}
	require.NoError(t, db.Close())
}

func validQuotaAmountSQLiteStatements() []string {
	return []string{
		`CREATE TABLE users (quota INTEGER, used_quota INTEGER)`,
		`CREATE TABLE tokens (remain_quota INTEGER, used_quota INTEGER)`,
		`CREATE TABLE token_quota_adjustments (remain_quota_before INTEGER, remain_quota_after INTEGER, used_quota_snapshot INTEGER)`,
		`CREATE TABLE channels (used_quota INTEGER)`,
		`CREATE TABLE quota_data (quota INTEGER)`,
		`CREATE TABLE user_quota_mutation_receipts (delta INTEGER, quota_before INTEGER, quota_after INTEGER)`,
		`CREATE TABLE subscription_plans (total_amount INTEGER)`,
		`CREATE TABLE user_subscriptions (amount_total INTEGER, amount_used INTEGER)`,
		`CREATE TABLE subscription_pre_consume_records (pre_consumed INTEGER)`,
		`CREATE TABLE account_quota_mutation_receipts (requested_quota INTEGER, applied_quota INTEGER)`,
		`CREATE TABLE account_quota_reservation_heads (applied_quota INTEGER)`,
		`CREATE TABLE account_quota_terminal_recovery_obligations (actual_quota INTEGER)`,
		`CREATE TABLE account_quota_refund_facts (wallet_quota INTEGER, subscription_quota INTEGER, token_quota INTEGER)`,
		`CREATE TABLE account_quota_settlement_intents (actual_quota INTEGER, delta INTEGER)`,
		`CREATE TABLE account_quota_settlement_facts (actual_quota INTEGER, delta INTEGER)`,
		`CREATE TABLE quota_mutation_receipts (quota INTEGER, estimated_quota INTEGER)`,
		`CREATE TABLE task_submission_operations (reserved_quota INTEGER, estimated_quota INTEGER)`,
		`CREATE TABLE task_terminal_observations (actual_quota INTEGER)`,
		`CREATE TABLE task_billing_events (quota_delta INTEGER)`,
		`CREATE TABLE legacy_usage_reservations (reserved_quota INTEGER, token_reserved_quota INTEGER, actual_quota INTEGER)`,
		`CREATE TABLE usage_review_decisions (actual_quota INTEGER)`,
		`CREATE TABLE admin_quota_adjustments (delta INTEGER, target_quota INTEGER, applied_quota_before INTEGER, applied_quota_after INTEGER)`,
		`INSERT INTO users (quota, used_quota) VALUES (-9007199254740991, 2147483648)`,
		`INSERT INTO tokens (remain_quota, used_quota) VALUES (9007199254740991, 2147483648)`,
		`INSERT INTO token_quota_adjustments (remain_quota_before, remain_quota_after, used_quota_snapshot) VALUES (-9007199254740991, 100, 9007199254740991)`,
		`INSERT INTO channels (used_quota) VALUES (9007199254740991)`,
		`INSERT INTO quota_data (quota) VALUES (9007199254740991)`,
		`INSERT INTO user_quota_mutation_receipts (delta, quota_before, quota_after) VALUES (9007199254740991, -9007199254740991, 0), (-9007199254740991, 9007199254740991, 0)`,
		`INSERT INTO subscription_plans (total_amount) VALUES (0), (9007199254740991)`,
		`INSERT INTO user_subscriptions (amount_total, amount_used) VALUES (9007199254740991, 2147483648)`,
		`INSERT INTO subscription_pre_consume_records (pre_consumed) VALUES (2147483647)`,
		`INSERT INTO account_quota_mutation_receipts (requested_quota, applied_quota) VALUES (2147483647, 2147483647)`,
		`INSERT INTO account_quota_reservation_heads (applied_quota) VALUES (2147483647)`,
		`INSERT INTO account_quota_terminal_recovery_obligations (actual_quota) VALUES (2147483647)`,
		`INSERT INTO account_quota_refund_facts (wallet_quota, subscription_quota, token_quota) VALUES (2147483647, 2147483647, 2147483647)`,
		`INSERT INTO account_quota_settlement_intents (actual_quota, delta) VALUES (2147483647, -2147483647)`,
		`INSERT INTO account_quota_settlement_facts (actual_quota, delta) VALUES (2147483647, 2147483647)`,
		`INSERT INTO quota_mutation_receipts (quota, estimated_quota) VALUES (2147483647, 2147483647)`,
		`INSERT INTO task_submission_operations (reserved_quota, estimated_quota) VALUES (2147483647, 2147483647)`,
		`INSERT INTO task_terminal_observations (actual_quota) VALUES (2147483647)`,
		`INSERT INTO task_billing_events (quota_delta) VALUES (-2147483648), (2147483647)`,
		`INSERT INTO legacy_usage_reservations (reserved_quota, token_reserved_quota, actual_quota) VALUES (2147483647, 2147483647, NULL), (2147483647, 2147483647, 2147483647)`,
		`INSERT INTO usage_review_decisions (actual_quota) VALUES (2147483647)`,
		`INSERT INTO admin_quota_adjustments (delta, target_quota, applied_quota_before, applied_quota_after) VALUES (-9007199254740991, 0, 9007199254740991, 0), (9007199254740991, 0, -9007199254740991, 0)`,
	}
}

func readQuotaAmountFixture(t *testing.T, path string) []byte {
	t.Helper()
	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	return contents
}

func quotaAmountFixtureDirectoryNames(t *testing.T, path string) []string {
	t.Helper()
	entries, err := os.ReadDir(path)
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names
}

func TestSQLiteQuotaAmountPreflightAcceptsExactWideIntegersWithoutWriting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "amounts.db")
	createQuotaAmountSQLiteFixture(t, path, validQuotaAmountSQLiteStatements()...)
	before := readQuotaAmountFixture(t, path)

	require.NoError(t, inspectSQLiteQuotaAmounts(context.Background(), path))
	require.NoError(t, inspectSQLiteQuotaAmounts(context.Background(), path))
	assert.Equal(t, before, readQuotaAmountFixture(t, path))
	assert.Equal(t, []string{"amounts.db"}, quotaAmountFixtureDirectoryNames(t, filepath.Dir(path)))
}

func TestSQLiteQuotaAmountPreflightAllowsMissingOptionalTables(t *testing.T) {
	path := filepath.Join(t.TempDir(), "core-only.db")
	createQuotaAmountSQLiteFixture(t, path,
		`CREATE TABLE users (quota INTEGER, used_quota INTEGER)`,
		`CREATE TABLE tokens (remain_quota INTEGER, used_quota INTEGER)`,
		`INSERT INTO users (quota, used_quota) VALUES (9007199254740991, 9007199254740991)`,
		`INSERT INTO tokens (remain_quota, used_quota) VALUES (-9007199254740991, 9007199254740991)`,
	)
	before := readQuotaAmountFixture(t, path)

	require.NoError(t, inspectSQLiteQuotaAmounts(context.Background(), path))
	assert.Equal(t, before, readQuotaAmountFixture(t, path))
}

func TestSQLiteQuotaAmountPreflightRejectsUnsafeSchemaAndValues(t *testing.T) {
	tests := []struct {
		name       string
		statements []string
		field      string
	}{
		{
			name: "key adjustment receipt above exact range",
			statements: []string{
				`CREATE TABLE users (quota INTEGER, used_quota INTEGER)`,
				`CREATE TABLE tokens (remain_quota INTEGER, used_quota INTEGER)`,
				`CREATE TABLE token_quota_adjustments (remain_quota_before INTEGER, remain_quota_after INTEGER, used_quota_snapshot INTEGER)`,
				`INSERT INTO token_quota_adjustments (remain_quota_before, remain_quota_after, used_quota_snapshot) VALUES (9007199254740992, 100, 0)`,
			},
			field: "token_quota_adjustments.remain_quota_before",
		},
		{
			name: "key adjustment receipt has negative used snapshot",
			statements: []string{
				`CREATE TABLE users (quota INTEGER, used_quota INTEGER)`,
				`CREATE TABLE tokens (remain_quota INTEGER, used_quota INTEGER)`,
				`CREATE TABLE token_quota_adjustments (remain_quota_before INTEGER, remain_quota_after INTEGER, used_quota_snapshot INTEGER)`,
				`INSERT INTO token_quota_adjustments (remain_quota_before, remain_quota_after, used_quota_snapshot) VALUES (100, 120, -1)`,
			},
			field: "token_quota_adjustments.used_quota_snapshot",
		},
		{
			name: "missing core table",
			statements: []string{
				`CREATE TABLE users (quota INTEGER, used_quota INTEGER)`,
			},
			field: "tokens",
		},
		{
			name: "missing core column",
			statements: []string{
				`CREATE TABLE users (quota INTEGER)`,
				`CREATE TABLE tokens (remain_quota INTEGER, used_quota INTEGER)`,
			},
			field: "users.used_quota",
		},
		{
			name: "non integer affinity",
			statements: []string{
				`CREATE TABLE users (quota REAL, used_quota INTEGER)`,
				`CREATE TABLE tokens (remain_quota INTEGER, used_quota INTEGER)`,
			},
			field: "users.quota",
		},
		{
			name: "real storage",
			statements: []string{
				`CREATE TABLE users (quota INTEGER, used_quota INTEGER)`,
				`CREATE TABLE tokens (remain_quota INTEGER, used_quota INTEGER)`,
				`INSERT INTO users (quota, used_quota) VALUES (1.5, 0)`,
			},
			field: "users.quota",
		},
		{
			name: "text storage",
			statements: []string{
				`CREATE TABLE users (quota INTEGER, used_quota INTEGER)`,
				`CREATE TABLE tokens (remain_quota INTEGER, used_quota INTEGER)`,
				`INSERT INTO users (quota, used_quota) VALUES ('not-an-integer', 0)`,
			},
			field: "users.quota",
		},
		{
			name: "null storage",
			statements: []string{
				`CREATE TABLE users (quota INTEGER, used_quota INTEGER)`,
				`CREATE TABLE tokens (remain_quota INTEGER, used_quota INTEGER)`,
				`INSERT INTO users (quota, used_quota) VALUES (NULL, 0)`,
			},
			field: "users.quota",
		},
		{
			name: "balance above exact range",
			statements: []string{
				`CREATE TABLE users (quota INTEGER, used_quota INTEGER)`,
				`CREATE TABLE tokens (remain_quota INTEGER, used_quota INTEGER)`,
				`INSERT INTO users (quota, used_quota) VALUES (9007199254740992, 0)`,
			},
			field: "users.quota",
		},
		{
			name: "negative used value",
			statements: []string{
				`CREATE TABLE users (quota INTEGER, used_quota INTEGER)`,
				`CREATE TABLE tokens (remain_quota INTEGER, used_quota INTEGER)`,
				`INSERT INTO tokens (remain_quota, used_quota) VALUES (0, -1)`,
			},
			field: "tokens.used_quota",
		},
		{
			name: "core view",
			statements: []string{
				`CREATE TABLE user_source (quota INTEGER, used_quota INTEGER)`,
				`CREATE VIEW users AS SELECT quota, used_quota FROM user_source`,
				`CREATE TABLE tokens (remain_quota INTEGER, used_quota INTEGER)`,
			},
			field: "users",
		},
		{
			name: "existing optional table missing amount column",
			statements: []string{
				`CREATE TABLE users (quota INTEGER, used_quota INTEGER)`,
				`CREATE TABLE tokens (remain_quota INTEGER, used_quota INTEGER)`,
				`CREATE TABLE user_subscriptions (amount_total INTEGER)`,
			},
			field: "user_subscriptions.amount_used",
		},
		{
			name: "nullable actual amount above charge range",
			statements: []string{
				`CREATE TABLE users (quota INTEGER, used_quota INTEGER)`,
				`CREATE TABLE tokens (remain_quota INTEGER, used_quota INTEGER)`,
				`CREATE TABLE legacy_usage_reservations (reserved_quota INTEGER, token_reserved_quota INTEGER, actual_quota INTEGER)`,
				`INSERT INTO legacy_usage_reservations (reserved_quota, token_reserved_quota, actual_quota) VALUES (1, 1, 2147483648)`,
			},
			field: "legacy_usage_reservations.actual_quota",
		},
		{
			name: "transition outside exact difference range",
			statements: []string{
				`CREATE TABLE users (quota INTEGER, used_quota INTEGER)`,
				`CREATE TABLE tokens (remain_quota INTEGER, used_quota INTEGER)`,
				`CREATE TABLE user_quota_mutation_receipts (delta INTEGER, quota_before INTEGER, quota_after INTEGER)`,
				`INSERT INTO user_quota_mutation_receipts (delta, quota_before, quota_after) VALUES (9007199254740992, 0, 0)`,
			},
			field: "user_quota_mutation_receipts.delta",
		},
		{
			name: "task delta below single fee range",
			statements: []string{
				`CREATE TABLE users (quota INTEGER, used_quota INTEGER)`,
				`CREATE TABLE tokens (remain_quota INTEGER, used_quota INTEGER)`,
				`CREATE TABLE task_billing_events (quota_delta INTEGER)`,
				`INSERT INTO task_billing_events (quota_delta) VALUES (-2147483649)`,
			},
			field: "task_billing_events.quota_delta",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "unsafe.db")
			createQuotaAmountSQLiteFixture(t, path, test.statements...)
			before := readQuotaAmountFixture(t, path)

			err := inspectSQLiteQuotaAmounts(context.Background(), path)
			require.ErrorIs(t, err, ErrQuotaAmountStartupUnverifiable)
			assert.ErrorContains(t, err, test.field)
			assert.Equal(t, before, readQuotaAmountFixture(t, path))
		})
	}
}

func TestQuotaAmountPreflightAllowsFreshSQLiteWithoutCreatingIt(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "absent.db")
	previousPath := common.SQLitePath
	common.SQLitePath = path
	t.Cleanup(func() { common.SQLitePath = previousPath })
	t.Setenv("SQL_DSN", "local")

	require.NoError(t, ValidateQuotaAmountPreMigrationConfiguration(nil))
	_, err := os.Stat(path)
	require.ErrorIs(t, err, os.ErrNotExist)
	assert.Empty(t, quotaAmountFixtureDirectoryNames(t, directory))
}

func TestQuotaAmountPreflightUsesConfiguredSQLiteAndSanitizesErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "unsafe.db")
	createQuotaAmountSQLiteFixture(t, path,
		`CREATE TABLE users (quota INTEGER, used_quota INTEGER)`,
		`CREATE TABLE tokens (remain_quota INTEGER, used_quota INTEGER)`,
		`INSERT INTO users (quota, used_quota) VALUES (9007199254740992, 0)`,
	)
	previousPath := common.SQLitePath
	common.SQLitePath = path
	t.Cleanup(func() { common.SQLitePath = previousPath })
	t.Setenv("SQL_DSN", "local")

	err := ValidateQuotaAmountPreMigrationConfiguration(context.Background())
	require.ErrorIs(t, err, ErrQuotaAmountStartupUnverifiable)
	assert.NotContains(t, err.Error(), "9007199254740992")
	assert.NotContains(t, err.Error(), path)

	secret := "quota-amount-secret"
	t.Setenv("SQL_DSN", "postgresql://user:"+secret+"@private.invalid/%zz")
	err = ValidateQuotaAmountPreMigrationConfiguration(context.Background())
	require.ErrorIs(t, err, ErrQuotaAmountStartupUnverifiable)
	assert.NotContains(t, err.Error(), secret)
	assert.NotContains(t, err.Error(), "private.invalid")
}

func TestQuotaAmountPreflightDirectTokenDeltasPreserveChargeGuards(t *testing.T) {
	const identityColumns = "phase, billing_source, billing_preference, requested_quota, applied_quota"
	const identitySchema = `(phase TEXT COLLATE NOCASE, billing_source TEXT COLLATE NOCASE, billing_preference TEXT COLLATE NOCASE, requested_quota INTEGER, applied_quota INTEGER)`
	tests := []struct {
		name      string
		schema    string
		columns   string
		values    []any
		wantError bool
	}{
		{name: "legal token debit", values: []any{"direct_token", "token", "direct", -100, -100}},
		{name: "legal token credit", values: []any{"direct_token", "token", "direct", 100, 100}},
		{name: "signed minimum", values: []any{"direct_token", "token", "direct", common.MinQuota, common.MinQuota}},
		{name: "signed maximum", values: []any{"direct_token", "token", "direct", common.MaxQuota, common.MaxQuota}},
		{name: "ordinary reserve positive charge", values: []any{"reserve", "wallet", "wallet_only", 100, 100}},
		{name: "ordinary settle positive charge", values: []any{"settle", "wallet", "wallet_only", 60, 60}},
		{name: "ordinary reserve negative charge", values: []any{"reserve", "wallet", "wallet_only", -100, -100}, wantError: true},
		{name: "ordinary settle negative charge", values: []any{"settle", "wallet", "wallet_only", -60, -60}, wantError: true},
		{name: "unknown phase", values: []any{"unknown", "token", "direct", -100, -100}, wantError: true},
		{name: "wrong billing source", values: []any{"direct_token", "wallet", "direct", -100, -100}, wantError: true},
		{name: "wrong billing preference", values: []any{"direct_token", "token", "wallet_only", -100, -100}, wantError: true},
		{name: "null phase", values: []any{nil, "token", "direct", -100, -100}, wantError: true},
		{name: "null billing source", values: []any{"direct_token", nil, "direct", -100, -100}, wantError: true},
		{name: "null billing preference", values: []any{"direct_token", "token", nil, -100, -100}, wantError: true},
		{name: "NOCASE phase lookalike", values: []any{"DIRECT_TOKEN", "token", "direct", -100, -100}, wantError: true},
		{name: "NOCASE source lookalike", values: []any{"direct_token", "TOKEN", "direct", -100, -100}, wantError: true},
		{name: "NOCASE preference lookalike", values: []any{"direct_token", "token", "DIRECT", -100, -100}, wantError: true},
		{name: "phase trailing space", values: []any{"direct_token ", "token", "direct", -100, -100}, wantError: true},
		{name: "source trailing space", values: []any{"direct_token", "token ", "direct", -100, -100}, wantError: true},
		{name: "preference trailing space", values: []any{"direct_token", "token", "direct ", -100, -100}, wantError: true},
		{name: "different negative amounts", values: []any{"direct_token", "token", "direct", -100, -99}, wantError: true},
		{name: "negative request with zero applied", values: []any{"direct_token", "token", "direct", -100, 0}, wantError: true},
		{name: "zero request with negative applied", values: []any{"direct_token", "token", "direct", 0, -100}, wantError: true},
		{name: "null requested amount", values: []any{"direct_token", "token", "direct", nil, 100}, wantError: true},
		{name: "null applied amount", values: []any{"direct_token", "token", "direct", 100, nil}, wantError: true},
		{name: "negative request with null applied", values: []any{"direct_token", "token", "direct", -100, nil}, wantError: true},
		{name: "text amount", values: []any{"direct_token", "token", "direct", "not-an-amount", -100}, wantError: true},
		{name: "fractional amount", values: []any{"direct_token", "token", "direct", -100.5, -100.5}, wantError: true},
		{name: "below signed minimum", values: []any{"direct_token", "token", "direct", int64(common.MinQuota) - 1, int64(common.MinQuota) - 1}, wantError: true},
		{name: "above signed maximum", values: []any{"direct_token", "token", "direct", int64(common.MaxQuota) + 1, int64(common.MaxQuota) + 1}, wantError: true},
		{
			name: "historical amount-only positive receipt", schema: `(requested_quota INTEGER, applied_quota INTEGER)`,
			columns: "requested_quota, applied_quota", values: []any{100, 100},
		},
		{
			name: "historical amount-only negative receipt", schema: `(requested_quota INTEGER, applied_quota INTEGER)`,
			columns: "requested_quota, applied_quota", values: []any{-100, -100}, wantError: true,
		},
		{
			name: "missing phase", schema: `(billing_source TEXT, billing_preference TEXT, requested_quota INTEGER, applied_quota INTEGER)`,
			columns: "billing_source, billing_preference, requested_quota, applied_quota", values: []any{"token", "direct", -100, -100}, wantError: true,
		},
		{
			name: "missing billing source", schema: `(phase TEXT, billing_preference TEXT, requested_quota INTEGER, applied_quota INTEGER)`,
			columns: "phase, billing_preference, requested_quota, applied_quota", values: []any{"direct_token", "direct", -100, -100}, wantError: true,
		},
		{
			name: "missing billing preference", schema: `(phase TEXT, billing_source TEXT, requested_quota INTEGER, applied_quota INTEGER)`,
			columns: "phase, billing_source, requested_quota, applied_quota", values: []any{"direct_token", "token", -100, -100}, wantError: true,
		},
		{
			name: "hidden generated phase", schema: `(phase TEXT GENERATED ALWAYS AS ('direct_token') VIRTUAL, billing_source TEXT, billing_preference TEXT, requested_quota INTEGER, applied_quota INTEGER)`,
			columns: "billing_source, billing_preference, requested_quota, applied_quota", values: []any{"token", "direct", -100, -100}, wantError: true,
		},
		{
			name: "fixed CHAR phase", schema: `(phase CHAR(16), billing_source TEXT, billing_preference TEXT, requested_quota INTEGER, applied_quota INTEGER)`,
			values: []any{"direct_token", "token", "direct", -100, -100}, wantError: true,
		},
		{
			name: "unknown identity storage", schema: `(phase BLOB, billing_source TEXT, billing_preference TEXT, requested_quota INTEGER, applied_quota INTEGER)`,
			values: []any{"direct_token", "token", "direct", -100, -100}, wantError: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			path := filepath.Join(directory, "receipt.db")
			schema, columns := test.schema, test.columns
			if schema == "" {
				schema = identitySchema
			}
			if columns == "" {
				columns = identityColumns
			}
			createQuotaAmountSQLiteFixture(t, path,
				`CREATE TABLE users (quota INTEGER, used_quota INTEGER)`,
				`CREATE TABLE tokens (remain_quota INTEGER, used_quota INTEGER)`,
				"CREATE TABLE account_quota_mutation_receipts "+schema,
			)
			fixture, err := sql.Open(sqlite.DriverName, quotaAmountFixtureURI(path))
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, fixture.Close()) })
			placeholders := make([]string, len(test.values))
			for index := range placeholders {
				placeholders[index] = "?"
			}
			_, err = fixture.Exec("INSERT INTO account_quota_mutation_receipts ("+columns+") VALUES ("+strings.Join(placeholders, ", ")+")", test.values...)
			require.NoError(t, err)
			require.NoError(t, fixture.Close())
			before := readQuotaAmountFixture(t, path)
			beforeNames := quotaAmountFixtureDirectoryNames(t, directory)
			previousPath := common.SQLitePath
			common.SQLitePath = path
			t.Cleanup(func() { common.SQLitePath = previousPath })
			t.Setenv("SQL_DSN", "local")

			err = ValidateQuotaAmountPreMigrationConfiguration(context.Background())
			if test.wantError {
				require.ErrorIs(t, err, ErrQuotaAmountStartupUnverifiable)
				assert.ErrorContains(t, err, "account_quota_mutation_receipts.")
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, before, readQuotaAmountFixture(t, path), "startup inspection must not rewrite receipt storage")
			assert.Equal(t, beforeNames, quotaAmountFixtureDirectoryNames(t, directory), "startup inspection must not create or remove SQLite sidecars")
		})
	}
}

func TestQuotaAmountPhysicalTypesByDialect(t *testing.T) {
	tests := []struct {
		name         string
		databaseType common.DatabaseType
		column       quotaAmountPhysicalColumn
		valid        bool
	}{
		{name: "SQLite INTEGER", databaseType: common.DatabaseTypeSQLite, column: quotaAmountPhysicalColumn{dataType: "integer"}, valid: true},
		{name: "SQLite BIGINT affinity", databaseType: common.DatabaseTypeSQLite, column: quotaAmountPhysicalColumn{dataType: "bigint"}, valid: true},
		{name: "SQLite REAL", databaseType: common.DatabaseTypeSQLite, column: quotaAmountPhysicalColumn{dataType: "real"}},
		{name: "SQLite hidden generated column", databaseType: common.DatabaseTypeSQLite, column: quotaAmountPhysicalColumn{dataType: "integer", hidden: true}},
		{name: "MySQL signed BIGINT", databaseType: common.DatabaseTypeMySQL, column: quotaAmountPhysicalColumn{dataType: "bigint", columnType: "bigint(20)"}, valid: true},
		{name: "MySQL BIGINT unsigned", databaseType: common.DatabaseTypeMySQL, column: quotaAmountPhysicalColumn{dataType: "bigint", columnType: "bigint unsigned"}},
		{name: "MySQL INT", databaseType: common.DatabaseTypeMySQL, column: quotaAmountPhysicalColumn{dataType: "int", columnType: "int(11)"}},
		{name: "PostgreSQL int8", databaseType: common.DatabaseTypePostgreSQL, column: quotaAmountPhysicalColumn{dataType: "int8"}, valid: true},
		{name: "PostgreSQL int4", databaseType: common.DatabaseTypePostgreSQL, column: quotaAmountPhysicalColumn{dataType: "int4"}},
		{name: "PostgreSQL numeric", databaseType: common.DatabaseTypePostgreSQL, column: quotaAmountPhysicalColumn{dataType: "numeric"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.valid, quotaAmountColumnIsExact(test.databaseType, test.column))
		})
	}
}

func TestQuotaAmountInvalidValueQueriesUseDialectPlaceholders(t *testing.T) {
	spec := quotaAmountColumnSpec{
		table: "users", column: "quota",
		minimum: -common.MaxQuotaBalance, maximum: common.MaxQuotaBalance,
	}
	sqliteQuery := quotaAmountInvalidValueQuery(common.DatabaseTypeSQLite, spec)
	assert.Contains(t, sqliteQuery, `typeof("quota") <> 'integer'`)
	assert.Contains(t, sqliteQuery, `"quota" < ?`)

	mysqlQuery := quotaAmountInvalidValueQuery(common.DatabaseTypeMySQL, spec)
	assert.NotContains(t, mysqlQuery, "typeof")
	assert.Contains(t, mysqlQuery, "`quota` < ?")

	postgresQuery := quotaAmountInvalidValueQuery(common.DatabaseTypePostgreSQL, spec)
	assert.NotContains(t, postgresQuery, "typeof")
	assert.Contains(t, postgresQuery, `"quota" < $1`)
	assert.Contains(t, postgresQuery, `"quota" > $2`)
}

func TestRelationalQuotaAmountPreflightAcceptsSignedBigintSchema(t *testing.T) {
	tests := []struct {
		name         string
		databaseType common.DatabaseType
		tableQuery   string
		columnQuery  string
		tableKind    string
		dataType     string
		columnType   string
	}{
		{
			name: "MySQL", databaseType: common.DatabaseTypeMySQL,
			tableQuery: "SELECT TABLE_NAME, TABLE_TYPE", columnQuery: "SELECT TABLE_NAME, COLUMN_NAME, DATA_TYPE, COLUMN_TYPE",
			tableKind: "BASE TABLE", dataType: "bigint", columnType: "bigint(20)",
		},
		{
			name: "PostgreSQL", databaseType: common.DatabaseTypePostgreSQL,
			tableQuery: "SELECT c.relname, c.relkind::text", columnQuery: "SELECT c.relname, a.attname, t.typname, t.typname",
			tableKind: "r", dataType: "int8", columnType: "int8",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			t.Cleanup(func() { _ = db.Close() })

			mock.ExpectBegin()
			tableRows := sqlmock.NewRows([]string{"table_name", "table_type"})
			columnRows := sqlmock.NewRows([]string{"table_name", "column_name", "data_type", "column_type"})
			for _, table := range quotaAmountTableSpecs {
				tableRows.AddRow(table.name, test.tableKind)
				for _, column := range table.columns {
					columnRows.AddRow(table.name, column.column, test.dataType, test.columnType)
				}
			}
			mock.ExpectQuery(test.tableQuery).WillReturnRows(tableRows)
			mock.ExpectQuery(test.columnQuery).WillReturnRows(columnRows)
			for _, table := range quotaAmountTableSpecs {
				for _, column := range table.columns {
					mock.ExpectQuery(regexp.QuoteMeta(quotaAmountInvalidValueQuery(test.databaseType, column))).
						WithArgs(column.minimum, column.maximum).
						WillReturnRows(sqlmock.NewRows([]string{"1"}))
				}
			}
			mock.ExpectRollback()
			mock.ExpectClose()

			require.NoError(t, inspectRelationalQuotaAmounts(context.Background(), db, test.databaseType))
			require.NoError(t, db.Close())
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestRelationalQuotaAmountPreflightRejectsNarrowColumnBeforeReadingValues(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT TABLE_NAME, TABLE_TYPE").WillReturnRows(
		sqlmock.NewRows([]string{"TABLE_NAME", "TABLE_TYPE"}).
			AddRow("users", "BASE TABLE").
			AddRow("tokens", "BASE TABLE"),
	)
	mock.ExpectQuery("SELECT TABLE_NAME, COLUMN_NAME, DATA_TYPE, COLUMN_TYPE").WillReturnRows(
		sqlmock.NewRows([]string{"TABLE_NAME", "COLUMN_NAME", "DATA_TYPE", "COLUMN_TYPE"}).
			AddRow("users", "quota", "int", "int(11)").
			AddRow("users", "used_quota", "bigint", "bigint(20)").
			AddRow("tokens", "remain_quota", "bigint", "bigint(20)").
			AddRow("tokens", "used_quota", "bigint", "bigint(20)"),
	)
	mock.ExpectRollback()
	mock.ExpectClose()

	err = inspectRelationalQuotaAmounts(context.Background(), db, common.DatabaseTypeMySQL)
	require.ErrorIs(t, err, ErrQuotaAmountStartupUnverifiable)
	assert.ErrorContains(t, err, "users.quota")
	assert.NotContains(t, strings.ToLower(err.Error()), "int(11)")
	require.NoError(t, db.Close())
	require.NoError(t, mock.ExpectationsWereMet())
}
