package model

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/ForceMind/MyAPI/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func createQuotaExecutionSQLiteFixture(t *testing.T, path string, statements ...string) {
	t.Helper()
	db, err := sql.Open(sqlite.DriverName, sqliteFixtureURI(path))
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	require.NoError(t, execQuotaWriterFixtureStatements(db, statements...))
	require.NoError(t, db.Close())
}

func TestSQLiteQuotaExecutionPreflightFreshDatabaseDoesNotCreateFiles(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "absent.db")

	require.NoError(t, inspectSQLiteQuotaExecution(context.Background(), path))
	_, err := os.Stat(path)
	require.ErrorIs(t, err, os.ErrNotExist)
	assert.Empty(t, directoryNamesForQuotaPreflightTest(t, directory))
}

func TestFreshQuotaExecutionSchemaCreatesAndValidatesEnforcingSentinel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fresh-migrated.db")
	db, err := gorm.Open(sqlite.Open(sqliteFixtureURI(path)), newGormConfig(true))
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&AccountQuotaSettlementIntent{},
		&AccountQuotaSettlementFact{},
		&AccountQuotaExecutionGate{},
		&AccountQuotaExecutionPermit{},
	))
	require.NoError(t, EnsureFreshAccountQuotaExecutionGate(context.Background(), db))
	require.NoError(t, ValidateAccountQuotaExecutionGate(context.Background(), db))
	var gate AccountQuotaExecutionGate
	require.NoError(t, db.First(&gate, AccountQuotaExecutionGateSingletonID).Error)
	assert.Equal(t, AccountQuotaExecutionGateSchemaVersion, gate.SchemaVersion)
	assert.Equal(t, AccountQuotaExecutionGateStateEnforcing, gate.State)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	require.NoError(t, inspectSQLiteQuotaExecution(context.Background(), path))
}

func TestSQLiteQuotaExecutionPreflightRejectsHistoricalRowsWithoutGateWithoutWriting(t *testing.T) {
	for _, test := range []struct {
		name  string
		table string
		kind  string
		state string
	}{
		{name: "unfinished legacy intent", table: "account_quota_settlement_intents", kind: "legacy_wallet", state: "pending"},
		{name: "terminal authoritative fact still requires staging", table: "account_quota_settlement_facts", kind: "authoritative", state: "applied"},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "historical.db")
			createQuotaExecutionSQLiteFixture(t, path,
				`CREATE TABLE `+test.table+` (kind TEXT NOT NULL, state TEXT NOT NULL)`,
				`INSERT INTO `+test.table+` (kind, state) VALUES ('`+test.kind+`', '`+test.state+`')`,
			)
			before := readFileForQuotaPreflightTest(t, path)

			err := inspectSQLiteQuotaExecution(context.Background(), path)
			require.ErrorContains(t, err, "without an execution gate")
			assert.Equal(t, before, readFileForQuotaPreflightTest(t, path))
			assert.Equal(t, []string{"historical.db"}, directoryNamesForQuotaPreflightTest(t, filepath.Dir(path)))
		})
	}
}

func TestSQLiteQuotaExecutionPreflightAllowsEmptyHistoricalTablesWithoutGate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty-history.db")
	createQuotaExecutionSQLiteFixture(t, path,
		`CREATE TABLE quota_writer_epochs (id INTEGER PRIMARY KEY, schema_version INTEGER NOT NULL, mode TEXT NOT NULL, epoch INTEGER NOT NULL, lock_version INTEGER NOT NULL)`,
		`INSERT INTO quota_writer_epochs (id, schema_version, mode, epoch, lock_version) VALUES (1, 1, 'authoritative', 9, 1)`,
		`CREATE TABLE account_quota_settlement_intents (kind TEXT NOT NULL, state TEXT NOT NULL)`,
		`CREATE TABLE account_quota_settlement_facts (kind TEXT NOT NULL, state TEXT NOT NULL)`,
	)
	before := readFileForQuotaPreflightTest(t, path)

	require.NoError(t, inspectSQLiteQuotaExecution(context.Background(), path))
	previousPath := common.SQLitePath
	common.SQLitePath = path
	t.Cleanup(func() { common.SQLitePath = previousPath })
	t.Setenv("SQL_DSN", "local")
	enabled, err := ValidateQuotaBatchPreMigrationConfiguration(context.Background(), "true", false)
	require.NoError(t, err)
	assert.True(t, enabled)
	require.NoError(t, ValidateQuotaExecutionPreMigrationConfiguration(context.Background()))
	assert.Equal(t, before, readFileForQuotaPreflightTest(t, path))
}

func TestSQLiteQuotaExecutionPreflightAllowsFrozenHistoricalFactsWithEnforcingGate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "frozen.db")
	db, err := gorm.Open(sqlite.Open(sqliteFixtureURI(path)), newGormConfig(true))
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&AccountQuotaSettlementIntent{},
		&AccountQuotaSettlementFact{},
		&AccountQuotaExecutionGate{},
		&AccountQuotaExecutionPermit{},
	))
	require.NoError(t, db.Create(&AccountQuotaExecutionGate{
		ID: AccountQuotaExecutionGateSingletonID, SchemaVersion: AccountQuotaExecutionGateSchemaVersion,
		State: AccountQuotaExecutionGateStateEnforcing, CreatedAt: 100, UpdatedAt: 100,
	}).Error)
	require.NoError(t, db.Session(&gorm.Session{SkipHooks: true}).Create(&AccountQuotaSettlementFact{
		EventKey: "synthetic-frozen-event", RequestID: "synthetic-frozen-request",
		Kind: AccountQuotaSettlementKindLegacyWallet, UserID: 1, TokenID: 1,
		Delta: -1, RequestFingerprint: strings.Repeat("a", 64), State: AccountQuotaSettlementPending,
		LockVersion: 1, CreatedAt: 100, UpdatedAt: 100,
	}).Error)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	before := readFileForQuotaPreflightTest(t, path)

	require.NoError(t, inspectSQLiteQuotaExecution(context.Background(), path))
	assert.Equal(t, before, readFileForQuotaPreflightTest(t, path))
}

func closeQuotaExecutionPreflightFixtureForTest(t *testing.T, db *gorm.DB) string {
	t.Helper()
	var sequence int
	var schema, path string
	require.NoError(t, db.Raw("PRAGMA database_list").Row().Scan(&sequence, &schema, &path))
	require.NotEmpty(t, path)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	return path
}

func TestSQLiteQuotaExecutionPreflightValidatesAppliedFactTerminalState(t *testing.T) {
	t.Run("coherent unpermitted legacy terminal is accepted without writing", func(t *testing.T) {
		db := openAccountQuotaTestDB(t)
		fixture := newAccountQuotaFixture(t, db, "execution-preflight-applied-coherent", 1000, 500, false, 0, 0, false)
		fact := insertUnpermittedSettlementFact(t, db, legacyWalletSettlementInput(fixture, "execution-preflight-applied-coherent", 10))
		markUnpermittedSettlementFactAppliedForTest(t, db, fact, true, true, 0)
		path := closeQuotaExecutionPreflightFixtureForTest(t, db)
		before := readFileForQuotaPreflightTest(t, path)

		require.NoError(t, inspectSQLiteQuotaExecution(context.Background(), path))
		assert.Equal(t, before, readFileForQuotaPreflightTest(t, path))
	})

	for _, test := range []struct {
		name           string
		fundingApplied bool
		tokenApplied   bool
	}{
		{name: "neither leg is complete"},
		{name: "only funding leg is complete", fundingApplied: true},
		{name: "only token leg is complete", tokenApplied: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			db := openAccountQuotaTestDB(t)
			fixture := newAccountQuotaFixture(t, db, "execution-preflight-applied-incoherent", 1000, 500, false, 0, 0, false)
			fact := insertUnpermittedSettlementFact(t, db, legacyWalletSettlementInput(fixture, "execution-preflight-applied-incoherent", 10))
			markUnpermittedSettlementFactAppliedForTest(t, db, fact, test.fundingApplied, test.tokenApplied, 0)
			path := closeQuotaExecutionPreflightFixtureForTest(t, db)
			before := readFileForQuotaPreflightTest(t, path)

			err := inspectSQLiteQuotaExecution(context.Background(), path)
			require.ErrorContains(t, err, "terminal state")
			assert.Equal(t, before, readFileForQuotaPreflightTest(t, path))
		})
	}
}

func TestSQLiteQuotaExecutionPreflightValidatesAppliedAuthoritativeTerminalReceipt(t *testing.T) {
	for _, test := range []struct {
		name              string
		terminalReceiptID func(*testing.T, *gorm.DB, *AccountQuotaMutationReceipt) int64
	}{
		{
			name: "terminal receipt is missing",
			terminalReceiptID: func(*testing.T, *gorm.DB, *AccountQuotaMutationReceipt) int64 {
				return 0
			},
		},
		{
			name: "receipt has reserve phase",
			terminalReceiptID: func(_ *testing.T, _ *gorm.DB, reserve *AccountQuotaMutationReceipt) int64 {
				return reserve.ID
			},
		},
		{
			name: "terminal receipt belongs to another subject",
			terminalReceiptID: func(t *testing.T, db *gorm.DB, _ *AccountQuotaMutationReceipt) int64 {
				other := newAccountQuotaFixture(t, db, "execution-preflight-applied-other-subject", 1000, 500, false, 0, 0, false)
				otherReserve, err := ReserveAccountQuota(context.Background(), db, accountReserveInput(other, "execution-preflight-applied-other-subject", "wallet_only", 100))
				require.NoError(t, err)
				terminal, err := SettleAccountQuota(context.Background(), db, AccountQuotaTerminalInput{
					RequestID: otherReserve.RequestID, ReserveReceiptID: otherReserve.ID, ActualQuota: 80,
				})
				require.NoError(t, err)
				return terminal.ID
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			db := openAccountQuotaTestDB(t)
			fixture := newAccountQuotaFixture(t, db, "execution-preflight-applied-authoritative", 1000, 500, false, 0, 0, false)
			reserve, err := ReserveAccountQuota(context.Background(), db, accountReserveInput(fixture, "execution-preflight-applied-authoritative", "wallet_only", 100))
			require.NoError(t, err)
			input := AccountQuotaSettlementFactInput{
				EventKey: "billing-settlement:execution-preflight-applied-authoritative:v2", RequestID: reserve.RequestID,
				Kind: AccountQuotaSettlementKindAuthoritative, UserID: reserve.UserID, TokenID: reserve.TokenID,
				SubscriptionID: reserve.SubscriptionID, ReserveReceiptID: reserve.ID, WriterEpoch: reserve.WriterEpoch, ActualQuota: 80,
			}
			fact := insertUnpermittedSettlementFact(t, db, input)
			markUnpermittedSettlementFactAppliedForTest(t, db, fact, true, true, test.terminalReceiptID(t, db, reserve))
			path := closeQuotaExecutionPreflightFixtureForTest(t, db)
			before := readFileForQuotaPreflightTest(t, path)

			err = inspectSQLiteQuotaExecution(context.Background(), path)
			require.ErrorContains(t, err, "terminal receipt")
			assert.Equal(t, before, readFileForQuotaPreflightTest(t, path))
		})
	}
}

func TestSQLiteQuotaExecutionPreflightAcceptsAppliedAuthoritativeMatchingTerminalReceipt(t *testing.T) {
	db := openAccountQuotaTestDB(t)
	fixture := newAccountQuotaFixture(t, db, "execution-preflight-applied-authoritative-valid", 1000, 500, false, 0, 0, false)
	reserve, err := ReserveAccountQuota(context.Background(), db, accountReserveInput(fixture, "execution-preflight-applied-authoritative-valid", "wallet_only", 100))
	require.NoError(t, err)
	terminal, err := SettleAccountQuota(context.Background(), db, AccountQuotaTerminalInput{
		RequestID: reserve.RequestID, ReserveReceiptID: reserve.ID, ActualQuota: 80,
	})
	require.NoError(t, err)
	input := AccountQuotaSettlementFactInput{
		EventKey: "billing-settlement:execution-preflight-applied-authoritative-valid:v2", RequestID: reserve.RequestID,
		Kind: AccountQuotaSettlementKindAuthoritative, UserID: reserve.UserID, TokenID: reserve.TokenID,
		SubscriptionID: reserve.SubscriptionID, ReserveReceiptID: reserve.ID, WriterEpoch: reserve.WriterEpoch, ActualQuota: 80,
	}
	fact := insertUnpermittedSettlementFact(t, db, input)
	markUnpermittedSettlementFactAppliedForTest(t, db, fact, true, true, terminal.ID)
	path := closeQuotaExecutionPreflightFixtureForTest(t, db)
	before := readFileForQuotaPreflightTest(t, path)

	require.NoError(t, inspectSQLiteQuotaExecution(context.Background(), path))
	assert.Equal(t, before, readFileForQuotaPreflightTest(t, path))
}

func TestSQLiteQuotaExecutionPreflightRejectsTamperedAppliedAuthoritativeTerminalReceipt(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*testing.T, *gorm.DB, int64)
	}{
		{
			name: "stored request fingerprint is changed",
			mutate: func(t *testing.T, db *gorm.DB, receiptID int64) {
				require.NoError(t, db.Exec(
					"UPDATE account_quota_mutation_receipts SET request_fingerprint = ? WHERE id = ?",
					strings.Repeat("0", 64), receiptID,
				).Error)
			},
		},
		{
			name: "fingerprint input is changed",
			mutate: func(t *testing.T, db *gorm.DB, receiptID int64) {
				require.NoError(t, db.Exec(
					"UPDATE account_quota_mutation_receipts SET audit_key = ? WHERE id = ?",
					"tampered-terminal-audit", receiptID,
				).Error)
			},
		},
		{
			name: "receipt version is unknown",
			mutate: func(t *testing.T, db *gorm.DB, receiptID int64) {
				require.NoError(t, db.Exec(
					"UPDATE account_quota_mutation_receipts SET receipt_version = ? WHERE id = ?",
					AccountQuotaMutationReceiptVersion+1, receiptID,
				).Error)
			},
		},
		{
			name: "fingerprint version is unknown but self consistent",
			mutate: func(t *testing.T, db *gorm.DB, receiptID int64) {
				var receipt AccountQuotaMutationReceipt
				require.NoError(t, db.First(&receipt, receiptID).Error)
				receipt.FingerprintVersion = AccountQuotaFingerprintVersion + 1
				fingerprint, err := RecomputeAccountQuotaReceiptFingerprint(&receipt)
				require.NoError(t, err)
				require.NoError(t, db.Exec(
					"UPDATE account_quota_mutation_receipts SET fingerprint_version = ?, request_fingerprint = ? WHERE id = ?",
					receipt.FingerprintVersion, fingerprint, receiptID,
				).Error)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			db := openAccountQuotaTestDB(t)
			fixture := newAccountQuotaFixture(t, db, "execution-preflight-applied-authoritative-tampered", 1000, 500, false, 0, 0, false)
			reserve, err := ReserveAccountQuota(context.Background(), db, accountReserveInput(fixture, "execution-preflight-applied-authoritative-tampered", "wallet_only", 100))
			require.NoError(t, err)
			terminal, err := SettleAccountQuota(context.Background(), db, AccountQuotaTerminalInput{
				RequestID: reserve.RequestID, ReserveReceiptID: reserve.ID, ActualQuota: 80,
			})
			require.NoError(t, err)
			input := AccountQuotaSettlementFactInput{
				EventKey: "billing-settlement:execution-preflight-applied-authoritative-tampered:v2", RequestID: reserve.RequestID,
				Kind: AccountQuotaSettlementKindAuthoritative, UserID: reserve.UserID, TokenID: reserve.TokenID,
				SubscriptionID: reserve.SubscriptionID, ReserveReceiptID: reserve.ID, WriterEpoch: reserve.WriterEpoch, ActualQuota: 80,
			}
			fact := insertUnpermittedSettlementFact(t, db, input)
			markUnpermittedSettlementFactAppliedForTest(t, db, fact, true, true, terminal.ID)
			test.mutate(t, db, terminal.ID)
			path := closeQuotaExecutionPreflightFixtureForTest(t, db)
			before := readFileForQuotaPreflightTest(t, path)

			err = inspectSQLiteQuotaExecution(context.Background(), path)
			require.ErrorContains(t, err, "terminal receipt")
			assert.Equal(t, before, readFileForQuotaPreflightTest(t, path))
		})
	}
}

func TestSQLiteQuotaExecutionPreflightAcceptsRestartedRuntimeIntentFactBinding(t *testing.T) {
	db := openAccountQuotaTestDB(t)
	fixture := newAccountQuotaFixture(t, db, "execution-preflight-runtime", 1000, 500, false, 0, 0, false)
	input := AccountQuotaSettlementFactInput{
		EventKey: "billing-settlement:execution-preflight-runtime:v1", RequestID: "execution-preflight-runtime",
		Kind: AccountQuotaSettlementKindLegacyWallet, UserID: fixture.User.Id, TokenID: fixture.Token.Id,
		Delta: 10, ApplyToken: true,
	}
	intent, err := EnsureAccountQuotaSettlementIntent(context.Background(), db, input)
	require.NoError(t, err)
	_, fact, err := RecoverAccountQuotaSettlementIntent(context.Background(), db, intent, "execution-preflight-runtime")
	require.NoError(t, err)
	require.NotNil(t, fact)

	var permit AccountQuotaExecutionPermit
	require.NoError(t, db.Where("intent_id = ?", intent.ID).First(&permit).Error)
	require.Equal(t, fact.ID, permit.FactID)
	require.Equal(t, permit.BindingDigest, permit.EvidenceDigest)
	var sequence int
	var schema, path string
	require.NoError(t, db.Raw("PRAGMA database_list").Row().Scan(&sequence, &schema, &path))
	require.NotEmpty(t, path)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	require.NoError(t, inspectSQLiteQuotaExecution(context.Background(), path))
}

func TestSQLiteQuotaExecutionPreflightRejectsInvalidSentinelAndDanglingPermit(t *testing.T) {
	for _, test := range []struct {
		name  string
		setup func(*gorm.DB)
	}{
		{
			name: "sentinel mismatch",
			setup: func(db *gorm.DB) {
				require.NoError(t, db.Create(&AccountQuotaExecutionGate{
					ID: AccountQuotaExecutionGateSingletonID, SchemaVersion: AccountQuotaExecutionGateSchemaVersion + 1,
					State: AccountQuotaExecutionGateStateEnforcing, CreatedAt: 100, UpdatedAt: 100,
				}).Error)
			},
		},
		{
			name: "dangling permit",
			setup: func(db *gorm.DB) {
				require.NoError(t, db.Create(&AccountQuotaExecutionGate{
					ID: AccountQuotaExecutionGateSingletonID, SchemaVersion: AccountQuotaExecutionGateSchemaVersion,
					State: AccountQuotaExecutionGateStateEnforcing, CreatedAt: 100, UpdatedAt: 100,
				}).Error)
				require.NoError(t, db.Create(&AccountQuotaExecutionPermit{
					EventKey: "synthetic-dangling-event", FactID: 991,
					Source:         AccountQuotaExecutionPermitSourceHistoricalReviewV1,
					BindingVersion: AccountQuotaExecutionPermitBindingVersion,
					BindingDigest:  strings.Repeat("b", 64), RequestID: "synthetic-dangling-request",
					RequestFingerprint: strings.Repeat("c", 64), Kind: AccountQuotaSettlementKindLegacyWallet,
					UserID: 1, TokenID: 1, Delta: -1, AllowFunding: true,
					State: AccountQuotaExecutionPermitStateActive, ApprovalID: "synthetic-approval",
					EvidenceDigest: strings.Repeat("d", 64), CreatedAt: 100,
				}).Error)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "invalid.db")
			db, err := gorm.Open(sqlite.Open(sqliteFixtureURI(path)), newGormConfig(true))
			require.NoError(t, err)
			require.NoError(t, db.AutoMigrate(
				&AccountQuotaSettlementIntent{},
				&AccountQuotaSettlementFact{},
				&AccountQuotaExecutionGate{},
				&AccountQuotaExecutionPermit{},
			))
			test.setup(db)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			require.NoError(t, sqlDB.Close())
			before := readFileForQuotaPreflightTest(t, path)

			require.Error(t, inspectSQLiteQuotaExecution(context.Background(), path))
			assert.Equal(t, before, readFileForQuotaPreflightTest(t, path))
		})
	}
}

func TestSQLiteQuotaExecutionPreflightRejectsNullBindingFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "null-binding.db")
	createQuotaExecutionSQLiteFixture(t, path,
		`CREATE TABLE account_quota_execution_gates (id INTEGER, schema_version INTEGER, state TEXT, created_at INTEGER, updated_at INTEGER)`,
		`INSERT INTO account_quota_execution_gates (id, schema_version, state, created_at, updated_at) VALUES (1, 1, 'enforcing', 100, 100)`,
		`CREATE TABLE account_quota_execution_permits (id INTEGER)`,
		`CREATE TABLE account_quota_settlement_intents (
id INTEGER, event_key TEXT, request_id TEXT, kind TEXT, user_id INTEGER, token_id INTEGER,
subscription_id INTEGER, reserve_receipt_id INTEGER, writer_epoch INTEGER, actual_quota INTEGER,
delta INTEGER, apply_token INTEGER, request_fingerprint TEXT, state TEXT, fact_id INTEGER, lock_version INTEGER
)`,
		`INSERT INTO account_quota_settlement_intents (id, kind, state) VALUES (1, 'legacy_wallet', 'pending')`,
	)
	before := readFileForQuotaPreflightTest(t, path)

	err := inspectSQLiteQuotaExecution(context.Background(), path)
	require.ErrorContains(t, err, "contains null fields")
	assert.Equal(t, before, readFileForQuotaPreflightTest(t, path))
}

func TestSQLiteQuotaExecutionPreflightReadsLiveAndCleanWALWithoutWriting(t *testing.T) {
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
			`CREATE TABLE account_quota_settlement_facts (kind TEXT NOT NULL, state TEXT NOT NULL)`,
		))
		var busy, logFrames, checkpointed int
		require.NoError(t, writer.QueryRow("PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &logFrames, &checkpointed))
		require.Zero(t, busy)
		_, err = writer.Exec(`INSERT INTO account_quota_settlement_facts (kind, state) VALUES ('legacy_wallet', 'pending')`)
		require.NoError(t, err)
		beforeNames := directoryNamesForQuotaPreflightTest(t, directory)
		mainBefore := readFileForQuotaPreflightTest(t, path)
		walBefore := readFileForQuotaPreflightTest(t, path+"-wal")

		require.Error(t, inspectSQLiteQuotaExecution(context.Background(), path))
		assert.Equal(t, mainBefore, readFileForQuotaPreflightTest(t, path))
		assert.Equal(t, walBefore, readFileForQuotaPreflightTest(t, path+"-wal"))
		assert.Equal(t, beforeNames, directoryNamesForQuotaPreflightTest(t, directory))
	})

	t.Run("clean WAL", func(t *testing.T) {
		directory := t.TempDir()
		path := filepath.Join(directory, "clean.db")
		writer, err := sql.Open(sqlite.DriverName, sqliteFixtureURI(path))
		require.NoError(t, err)
		var journalMode string
		require.NoError(t, writer.QueryRow("PRAGMA journal_mode=WAL").Scan(&journalMode))
		require.Equal(t, "wal", strings.ToLower(journalMode))
		require.NoError(t, execQuotaWriterFixtureStatements(writer,
			`CREATE TABLE account_quota_settlement_intents (kind TEXT NOT NULL, state TEXT NOT NULL)`,
			`INSERT INTO account_quota_settlement_intents (kind, state) VALUES ('legacy_subscription', 'materialized')`,
		))
		require.NoError(t, writer.Close())
		beforeNames := directoryNamesForQuotaPreflightTest(t, directory)
		before := readFileForQuotaPreflightTest(t, path)

		require.Error(t, inspectSQLiteQuotaExecution(context.Background(), path))
		assert.Equal(t, before, readFileForQuotaPreflightTest(t, path))
		assert.Equal(t, beforeNames, directoryNamesForQuotaPreflightTest(t, directory))
	})
}

func TestRelationalQuotaExecutionPreflightUsesReadOnlySnapshotSQLMockOnly(t *testing.T) {
	for _, databaseType := range []common.DatabaseType{common.DatabaseTypeMySQL, common.DatabaseTypePostgreSQL} {
		t.Run(string(databaseType), func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			t.Cleanup(func() { _ = db.Close() })
			mock.ExpectBegin()
			catalogPrefix := "SELECT TABLE_NAME, TABLE_TYPE"
			if databaseType == common.DatabaseTypePostgreSQL {
				catalogPrefix = "SELECT c.relname, c.relkind::text"
			}
			kind := "BASE TABLE"
			if databaseType == common.DatabaseTypePostgreSQL {
				kind = "r"
			}
			catalogRows := sqlmock.NewRows([]string{"name", "kind"})
			for _, table := range []string{
				AccountQuotaExecutionGateTableName,
				AccountQuotaExecutionPermitTableName,
				quotaExecutionSettlementTables[0],
				quotaExecutionSettlementTables[1],
			} {
				catalogRows.AddRow(table, kind)
			}
			mock.ExpectQuery(regexp.QuoteMeta(catalogPrefix)).WillReturnRows(catalogRows)
			mock.ExpectQuery("SELECT .*schema_version.*state.*FROM.*account_quota_execution_gates").WillReturnRows(
				sqlmock.NewRows([]string{"id", "schema_version", "state", "created_at", "updated_at"}).AddRow(
					AccountQuotaExecutionGateSingletonID,
					AccountQuotaExecutionGateSchemaVersion,
					AccountQuotaExecutionGateStateEnforcing,
					100,
					100,
				),
			)
			for range len(quotaExecutionSettlementTables) * 2 {
				mock.ExpectQuery("SELECT 1 FROM .*account_quota_settlement_.*WHERE").WillReturnRows(sqlmock.NewRows([]string{"marker"}))
			}
			mock.ExpectQuery("SELECT .* FROM .*account_quota_settlement_facts.*WHERE.*state.*=.*'applied'").
				WillReturnRows(sqlmock.NewRows([]string{"empty"}))
			for range 3 {
				mock.ExpectQuery("SELECT 1 FROM .*account_quota_execution_permits.*GROUP BY").WillReturnRows(sqlmock.NewRows([]string{"marker"}))
			}
			mock.ExpectQuery("SELECT .* FROM .*account_quota_execution_permits.*LEFT JOIN").WillReturnRows(sqlmock.NewRows([]string{"empty"}))
			mock.ExpectRollback()
			mock.ExpectClose()

			require.NoError(t, inspectRelationalQuotaExecution(context.Background(), db, databaseType))
			require.NoError(t, db.Close())
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestRelationalQuotaExecutionPreflightReadsCompleteTerminalReceiptSQLMockOnly(t *testing.T) {
	for _, databaseType := range []common.DatabaseType{common.DatabaseTypeMySQL, common.DatabaseTypePostgreSQL} {
		t.Run(string(databaseType), func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			t.Cleanup(func() { _ = db.Close() })

			receipt := AccountQuotaMutationReceipt{
				ID: 31, ReceiptVersion: AccountQuotaMutationReceiptVersion, FingerprintVersion: AccountQuotaFingerprintVersion,
				WriterEpoch: 7, RequestID: "execution-preflight-terminal-scan", Phase: AccountQuotaPhaseSettle,
				EventKey: accountQuotaEventKey("execution-preflight-terminal-scan", AccountQuotaPhaseSettle), ParentReceiptID: 11,
				ReserveFingerprint: strings.Repeat("a", 64), BillingPreference: "wallet_only",
				UserID: 1, TokenID: 2, RequestedQuota: 80,
				BillingContext: AccountBillingContext{
					Version: 1, OriginModelName: "fixture-model", BillingPreference: "wallet_only",
				},
			}
			receipt.RequestFingerprint, err = RecomputeAccountQuotaReceiptFingerprint(&receipt)
			require.NoError(t, err)
			billingContext, err := receipt.BillingContext.Value()
			require.NoError(t, err)

			mock.ExpectBegin()
			catalogPrefix := "SELECT TABLE_NAME, TABLE_TYPE"
			kind := "BASE TABLE"
			if databaseType == common.DatabaseTypePostgreSQL {
				catalogPrefix = "SELECT c.relname, c.relkind::text"
				kind = "r"
			}
			catalogRows := sqlmock.NewRows([]string{"name", "kind"})
			for _, table := range []string{
				AccountQuotaExecutionGateTableName,
				AccountQuotaExecutionPermitTableName,
				quotaExecutionSettlementTables[0],
				quotaExecutionSettlementTables[1],
				(AccountQuotaMutationReceipt{}).TableName(),
			} {
				catalogRows.AddRow(table, kind)
			}
			mock.ExpectQuery(regexp.QuoteMeta(catalogPrefix)).WillReturnRows(catalogRows)
			mock.ExpectQuery("SELECT .*schema_version.*state.*FROM.*account_quota_execution_gates").WillReturnRows(
				sqlmock.NewRows([]string{"id", "schema_version", "state", "created_at", "updated_at"}).AddRow(
					AccountQuotaExecutionGateSingletonID,
					AccountQuotaExecutionGateSchemaVersion,
					AccountQuotaExecutionGateStateEnforcing,
					100,
					100,
				),
			)
			for range len(quotaExecutionSettlementTables) * 2 {
				mock.ExpectQuery("SELECT 1 FROM .*account_quota_settlement_.*WHERE").WillReturnRows(sqlmock.NewRows([]string{"marker"}))
			}
			mock.ExpectQuery("SELECT .* FROM .*account_quota_settlement_facts.*WHERE.*state.*=.*'applied'").WillReturnRows(
				sqlmock.NewRows([]string{
					"id", "request_id", "kind", "user_id", "token_id", "subscription_id", "reserve_receipt_id",
					"writer_epoch", "actual_quota", "state", "funding_applied", "token_applied", "terminal_receipt_id",
				}).AddRow(
					21, receipt.RequestID, AccountQuotaSettlementKindAuthoritative, receipt.UserID, receipt.TokenID, receipt.SubscriptionID, 11,
					receipt.WriterEpoch, receipt.RequestedQuota, AccountQuotaSettlementApplied, true, true, receipt.ID,
				),
			)
			mock.ExpectQuery("SELECT .*receipt_version.*fingerprint_version.*request_fingerprint.*billing_context.*FROM.*account_quota_mutation_receipts").
				WithArgs(receipt.ID).
				WillReturnRows(sqlmock.NewRows([]string{
					"id", "receipt_version", "fingerprint_version", "writer_epoch", "request_id", "phase", "event_key",
					"parent_receipt_id", "audit_key", "reserve_fingerprint", "request_fingerprint", "billing_preference",
					"user_id", "token_id", "subscription_id", "requested_quota", "billing_context",
				}).AddRow(
					receipt.ID, receipt.ReceiptVersion, receipt.FingerprintVersion, receipt.WriterEpoch, receipt.RequestID, receipt.Phase, receipt.EventKey,
					receipt.ParentReceiptID, receipt.AuditKey, receipt.ReserveFingerprint, receipt.RequestFingerprint, receipt.BillingPreference,
					receipt.UserID, receipt.TokenID, receipt.SubscriptionID, receipt.RequestedQuota, billingContext,
				))
			for range 3 {
				mock.ExpectQuery("SELECT 1 FROM .*account_quota_execution_permits.*GROUP BY").WillReturnRows(sqlmock.NewRows([]string{"marker"}))
			}
			mock.ExpectQuery("SELECT .* FROM .*account_quota_execution_permits.*LEFT JOIN").WillReturnRows(sqlmock.NewRows([]string{"empty"}))
			mock.ExpectRollback()
			mock.ExpectClose()

			require.NoError(t, inspectRelationalQuotaExecution(context.Background(), db, databaseType))
			require.NoError(t, db.Close())
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestInitDBQuotaExecutionPreflightRejectsBeforeOpeningOrMigratingSQLite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "startup.db")
	createQuotaExecutionSQLiteFixture(t, path,
		`CREATE TABLE users (quota INTEGER, used_quota INTEGER)`,
		`CREATE TABLE tokens (remain_quota INTEGER, used_quota INTEGER)`,
		`CREATE TABLE quota_writer_epochs (id INTEGER PRIMARY KEY, schema_version INTEGER NOT NULL, mode TEXT NOT NULL, epoch INTEGER NOT NULL, lock_version INTEGER NOT NULL)`,
		`INSERT INTO quota_writer_epochs (id, schema_version, mode, epoch, lock_version) VALUES (1, 1, 'authoritative', 9, 1)`,
		`CREATE TABLE account_quota_settlement_intents (kind TEXT NOT NULL, state TEXT NOT NULL, actual_quota INTEGER NOT NULL, delta INTEGER NOT NULL)`,
		`INSERT INTO account_quota_settlement_intents (kind, state, actual_quota, delta) VALUES ('legacy_wallet', 'pending', 0, -1)`,
	)
	before := readFileForQuotaPreflightTest(t, path)
	previousDB, previousPath := DB, common.SQLitePath
	sentinelDB := &gorm.DB{}
	DB, common.SQLitePath = sentinelDB, path
	t.Cleanup(func() { DB, common.SQLitePath = previousDB, previousPath })
	t.Setenv("SQL_DSN", "")
	t.Setenv("LOG_SQL_DSN", "")
	t.Setenv("BATCH_UPDATE_ENABLED", "false")

	err := InitDB()
	require.ErrorIs(t, err, ErrQuotaExecutionStartupUnverifiable)
	assert.Same(t, sentinelDB, DB)
	assert.Equal(t, before, readFileForQuotaPreflightTest(t, path))
	assert.Equal(t, []string{"startup.db"}, directoryNamesForQuotaPreflightTest(t, filepath.Dir(path)))
}

func TestQuotaExecutionPreflightSanitizesDatabaseErrors(t *testing.T) {
	secret := "execution-preflight-secret"
	t.Setenv("SQL_DSN", "postgresql://user:"+secret+"@private.invalid/%zz")
	err := ValidateQuotaExecutionPreMigrationConfiguration(context.Background())
	require.ErrorIs(t, err, ErrQuotaExecutionStartupUnverifiable)
	assert.NotContains(t, err.Error(), secret)
	assert.NotContains(t, err.Error(), "private.invalid")
}
