package model

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/glebarez/sqlite"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	historicalAccountingNativeGate = "MYAPI_HISTORICAL_ACCOUNTING_DATABASE_TESTS"
	historicalAccountingPrefix     = "myapi_historical_accounting_test_"
)

type historicalAccountingHarness struct {
	databaseType common.DatabaseType
	open         func() (*gorm.DB, error)
	probe        func(*gorm.DB) error
	execution    func(*gorm.DB) error
}

func TestHistoricalAccountingDatabaseContractSQLite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "historical-accounting.db")
	harness := historicalAccountingHarness{
		databaseType: common.DatabaseTypeSQLite,
		open: func() (*gorm.DB, error) {
			return gorm.Open(sqlite.Open(path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"),
				&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		},
		probe: func(*gorm.DB) error {
			ctx, cancel := context.WithTimeout(context.Background(), quotaAmountSQLiteProbeTimeout)
			defer cancel()
			return inspectSQLiteQuotaAmounts(ctx, path)
		},
		execution: func(*gorm.DB) error {
			ctx, cancel := context.WithTimeout(context.Background(), quotaExecutionSQLiteProbeTimeout)
			defer cancel()
			return inspectSQLiteQuotaExecution(ctx, path)
		},
	}
	runHistoricalAccountingDatabaseContract(t, harness)
}

func TestHistoricalAccountingDatabaseContractNative(t *testing.T) {
	if os.Getenv(historicalAccountingNativeGate) != "1" {
		t.Skip("native historical-accounting tests require explicit isolated database configuration")
	}
	for _, test := range []struct {
		name string
		env  string
	}{
		{name: "mysql", env: "MYAPI_HISTORICAL_ACCOUNTING_MYSQL_DSN"},
		{name: "postgres", env: "MYAPI_HISTORICAL_ACCOUNTING_POSTGRES_DSN"},
	} {
		t.Run(test.name, func(t *testing.T) {
			dsn := strings.TrimSpace(os.Getenv(test.env))
			if dsn == "" {
				t.Skip("native engine DSN is not explicitly configured")
			}
			harness, namespace, err := historicalAccountingNativeHarness(test.name, dsn)
			require.NoError(t, err)

			admin, err := harness.open()
			require.NoError(t, err)
			adminSQL, err := admin.DB()
			require.NoError(t, err)
			adminSQL.SetMaxOpenConns(1)
			t.Cleanup(func() { _ = adminSQL.Close() })
			if test.name == "postgres" {
				var exists bool
				require.NoError(t, admin.Raw("SELECT EXISTS (SELECT 1 FROM pg_catalog.pg_namespace WHERE nspname = ?)", namespace).Scan(&exists).Error)
				require.False(t, exists, "refusing to reuse an existing synthetic schema")
				require.NoError(t, admin.Exec(`CREATE SCHEMA "`+namespace+`"`).Error)
				t.Cleanup(func() {
					assert.NoError(t, admin.Exec(`DROP SCHEMA IF EXISTS "`+namespace+`" CASCADE`).Error)
				})
			} else {
				tables, tableErr := admin.Migrator().GetTables()
				require.NoError(t, tableErr)
				require.Empty(t, tables, "refusing to use a non-empty synthetic database")
				t.Cleanup(func() { dropHistoricalAccountingContractTables(t, admin) })
			}
			runHistoricalAccountingDatabaseContract(t, harness)
		})
	}
}

func TestHistoricalAccountingNativeTargetSafety(t *testing.T) {
	unsafeTargets := []struct{ engine, dsn string }{
		{"mysql", "fixture:secret@tcp(192.0.2.1:33306)/myapi_historical_accounting_test_unit"},
		{"mysql", "fixture:secret@tcp(127.0.0.1:3306)/myapi_historical_accounting_test_unit"},
		{"mysql", "fixture:secret@tcp(127.0.0.1:33306)/production"},
		{"postgres", "postgres://fixture:secret@192.0.2.1:35432/myapi_historical_accounting_test_unit?search_path=myapi_historical_accounting_test_unit"},
		{"postgres", "postgres://fixture:secret@127.0.0.1:5432/myapi_historical_accounting_test_unit?search_path=myapi_historical_accounting_test_unit"},
		{"postgres", "postgres://fixture:secret@127.0.0.1:35432/production?search_path=myapi_historical_accounting_test_unit"},
		{"postgres", "postgres://fixture:secret@127.0.0.1:35432/myapi_historical_accounting_test_unit"},
	}
	for _, target := range unsafeTargets {
		_, _, err := historicalAccountingNativeHarness(target.engine, target.dsn)
		require.Error(t, err)
	}
	for _, target := range []struct{ engine, dsn string }{
		{"mysql", "fixture:secret@tcp(127.0.0.1:33306)/myapi_historical_accounting_test_unit?parseTime=true"},
		{"postgres", "postgres://fixture:secret@127.0.0.1:35432/myapi_historical_accounting_test_unit?sslmode=disable&search_path=myapi_historical_accounting_test_unit"},
	} {
		_, _, err := historicalAccountingNativeHarness(target.engine, target.dsn)
		require.NoError(t, err)
	}
}

func historicalAccountingNativeHarness(engine, dsn string) (historicalAccountingHarness, string, error) {
	unsafeTarget := errors.New("native historical-accounting tests require an isolated loopback test namespace on a non-default port")
	safePort := func(hostPort string, defaultPort int) bool {
		host, portText, err := net.SplitHostPort(hostPort)
		if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
			return false
		}
		port, err := strconv.Atoi(portText)
		return err == nil && port > 0 && port <= 65535 && port != defaultPort
	}
	switch engine {
	case "mysql":
		config, err := mysqldriver.ParseDSN(dsn)
		if err != nil || config.Net != "tcp" || !safePort(config.Addr, 3306) || !validHistoricalAccountingNamespace(config.DBName) {
			return historicalAccountingHarness{}, "", unsafeTarget
		}
		formatted := config.FormatDSN()
		harness := historicalAccountingHarness{databaseType: common.DatabaseTypeMySQL}
		harness.open = func() (*gorm.DB, error) {
			return gorm.Open(gormmysql.New(gormmysql.Config{DSN: formatted}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		}
		harness.probe = func(db *gorm.DB) error {
			sqlDB, err := db.DB()
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(context.Background(), quotaAmountSQLProbeTimeout)
			defer cancel()
			return inspectRelationalQuotaAmounts(ctx, sqlDB, common.DatabaseTypeMySQL)
		}
		harness.execution = func(db *gorm.DB) error {
			sqlDB, err := db.DB()
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(context.Background(), quotaExecutionSQLProbeTimeout)
			defer cancel()
			return inspectRelationalQuotaExecution(ctx, sqlDB, common.DatabaseTypeMySQL)
		}
		return harness, config.DBName, nil
	case "postgres":
		config, err := pgx.ParseConfig(dsn)
		if err != nil || !safePort(net.JoinHostPort(config.Host, strconv.Itoa(int(config.Port))), 5432) || !validHistoricalAccountingNamespace(config.Database) {
			return historicalAccountingHarness{}, "", unsafeTarget
		}
		namespace := config.RuntimeParams["search_path"]
		if !validHistoricalAccountingNamespace(namespace) {
			return historicalAccountingHarness{}, "", unsafeTarget
		}
		for _, fallback := range config.Fallbacks {
			if !safePort(net.JoinHostPort(fallback.Host, strconv.Itoa(int(fallback.Port))), 5432) {
				return historicalAccountingHarness{}, "", unsafeTarget
			}
		}
		config.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
		harness := historicalAccountingHarness{databaseType: common.DatabaseTypePostgreSQL}
		harness.open = func() (*gorm.DB, error) {
			configCopy := config.Copy()
			return gorm.Open(postgres.New(postgres.Config{Conn: stdlib.OpenDB(*configCopy)}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		}
		harness.probe = func(db *gorm.DB) error {
			sqlDB, err := db.DB()
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(context.Background(), quotaAmountSQLProbeTimeout)
			defer cancel()
			return inspectRelationalQuotaAmounts(ctx, sqlDB, common.DatabaseTypePostgreSQL)
		}
		harness.execution = func(db *gorm.DB) error {
			sqlDB, err := db.DB()
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(context.Background(), quotaExecutionSQLProbeTimeout)
			defer cancel()
			return inspectRelationalQuotaExecution(ctx, sqlDB, common.DatabaseTypePostgreSQL)
		}
		return harness, namespace, nil
	default:
		return historicalAccountingHarness{}, "", unsafeTarget
	}
}

func validHistoricalAccountingNamespace(name string) bool {
	if !strings.HasPrefix(name, historicalAccountingPrefix) || len(name) <= len(historicalAccountingPrefix) || len(name) > 63 {
		return false
	}
	for _, char := range name {
		if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '_' {
			return false
		}
	}
	return true
}

func runHistoricalAccountingDatabaseContract(t *testing.T, harness historicalAccountingHarness) {
	t.Helper()
	previousRedisEnabled, previousRDB, previousBatch := common.RedisEnabled, common.RDB, common.BatchUpdateEnabled
	common.RedisEnabled, common.RDB, common.BatchUpdateEnabled = false, nil, false
	previousTokenHook := accountQuotaSettlementBeforeTokenHook
	t.Cleanup(func() {
		common.RedisEnabled, common.RDB, common.BatchUpdateEnabled = previousRedisEnabled, previousRDB, previousBatch
		accountQuotaSettlementBeforeTokenHook = previousTokenHook
	})

	db, err := harness.open()
	require.NoError(t, err)
	var liveSQL *sql.DB
	liveSQL, err = db.DB()
	require.NoError(t, err)
	liveSQL.SetMaxOpenConns(4)
	t.Cleanup(func() {
		if liveSQL != nil {
			_ = liveSQL.Close()
		}
	})

	require.NoError(t, db.AutoMigrate(&User{}, &Token{}, &LegacyUsageReservation{}, &AccountQuotaSettlementIntent{},
		&AccountQuotaSettlementFact{}, &QuotaWorkCursor{}, &AccountQuotaExecutionGate{}, &AccountQuotaExecutionPermit{}))
	require.NoError(t, EnsureFreshAccountQuotaExecutionGate(context.Background(), db))
	rollbackFixture := createHistoricalAccountingFixture(t, db, "rollback", 1000, 500)
	rollbackFailure := errors.New("synthetic outer transaction rollback")
	for _, operation := range []struct {
		name string
		run  func(*gorm.DB, AccountQuotaSettlementFactInput) error
	}{
		{name: "intent", run: func(tx *gorm.DB, input AccountQuotaSettlementFactInput) error {
			_, err := EnsureAccountQuotaSettlementIntent(context.Background(), tx, input)
			return err
		}},
		{name: "fact", run: func(tx *gorm.DB, input AccountQuotaSettlementFactInput) error {
			_, err := EnsureAccountQuotaSettlementFact(context.Background(), tx, input)
			return err
		}},
	} {
		input := legacyWalletSettlementInput(rollbackFixture, "historical-native-rollback-"+operation.name, 5)
		err := db.Transaction(func(tx *gorm.DB) error {
			if err := operation.run(tx, input); err != nil {
				return err
			}
			return rollbackFailure
		})
		require.ErrorIs(t, err, rollbackFailure)
		for _, row := range []any{&AccountQuotaSettlementIntent{}, &AccountQuotaSettlementFact{}, &AccountQuotaExecutionPermit{}} {
			var count int64
			require.NoError(t, db.Model(row).Where("event_key = ?", input.EventKey).Count(&count).Error)
			assert.Zero(t, count)
		}
	}

	const wideQuota int64 = 1<<31 + 100
	wide := createHistoricalAccountingFixture(t, db, "wide", wideQuota, wideQuota)
	require.NoError(t, harness.probe(db))
	assertHistoricalAccountingBalances(t, db, wide, wideQuota, wideQuota, 0)

	beforeType := historicalAccountingColumnType(t, db, "users", "quota")
	const unsafeQuota int64 = 1<<53 + 1
	require.NoError(t, db.Exec("UPDATE users SET quota = ? WHERE id = ?", unsafeQuota, wide.User.Id).Error)
	var unsafeValueBefore User
	require.NoError(t, db.First(&unsafeValueBefore, wide.User.Id).Error)
	probeErr := harness.probe(db)
	require.ErrorIs(t, probeErr, ErrQuotaAmountStartupUnverifiable)
	assert.ErrorContains(t, probeErr, "users.quota")
	var unsafeValueAfter User
	require.NoError(t, db.First(&unsafeValueAfter, wide.User.Id).Error)
	assert.Equal(t, unsafeValueBefore, unsafeValueAfter)
	assert.Equal(t, unsafeQuota, unsafeValueAfter.Quota)
	assert.Equal(t, beforeType, historicalAccountingColumnType(t, db, "users", "quota"))
	require.NoError(t, db.Exec("UPDATE users SET quota = ? WHERE id = ?", wideQuota, wide.User.Id).Error)

	gateFixture := createHistoricalAccountingFixture(t, db, "gate", 1000, 500)
	frozenInput := legacyWalletSettlementInput(gateFixture, "historical-native-frozen", 10)
	frozen := insertUnpermittedSettlementFact(t, db, frozenInput)
	frozenBefore := *frozen
	_, err = RecoverAccountQuotaSettlementFact(context.Background(), db, frozen, "frozen-direct")
	require.ErrorIs(t, err, ErrAccountQuotaExecutionFrozen)
	var frozenAfter AccountQuotaSettlementFact
	require.NoError(t, db.First(&frozenAfter, frozen.ID).Error)
	assert.Equal(t, frozenBefore, frozenAfter)
	require.NoError(t, harness.execution(db))
	require.NoError(t, db.First(&frozenAfter, frozen.ID).Error)
	assert.Equal(t, frozenBefore, frozenAfter)

	permitted, err := EnsureAccountQuotaSettlementFact(context.Background(), db,
		legacyWalletSettlementInput(gateFixture, "historical-native-permitted", 25))
	require.NoError(t, err)
	processed, err := RunAccountQuotaSettlementFacts(context.Background(), db, "native-gate-scanner", 1)
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	assertHistoricalAccountingBalances(t, db, gateFixture, 975, 475, 25)
	require.NoError(t, db.First(&frozenAfter, frozen.ID).Error)
	assert.Equal(t, frozenBefore, frozenAfter)
	var permittedAfter AccountQuotaSettlementFact
	require.NoError(t, db.First(&permittedAfter, permitted.ID).Error)
	assert.Equal(t, AccountQuotaSettlementApplied, permittedAfter.State)

	historical := createHistoricalAccountingFixture(t, db, "permit", 1000, 500)
	historicalInput := legacyWalletSettlementInput(historical, "historical-native-reopen", 40)
	historicalFact := insertUnpermittedSettlementFact(t, db, historicalInput)
	wrong := historicalInput
	wrong.Delta++
	_, wrongFingerprint, err := normalizeAccountQuotaSettlementInput(wrong)
	require.NoError(t, err)
	permit, err := PermitHistoricalAccountQuotaSettlement(context.Background(), db, HistoricalAccountQuotaExecutionPermitInput{
		FactID: historicalFact.ID, Settlement: wrong, RequestFingerprint: wrongFingerprint,
		AllowFunding: true, AllowToken: true, ApprovalID: "synthetic-native-review", EvidenceDigest: strings.Repeat("a", 64),
	})
	require.Nil(t, permit)
	require.ErrorIs(t, err, ErrAccountQuotaTerminalRecoveryConflict)
	permit, err = PermitHistoricalAccountQuotaSettlement(context.Background(), db, HistoricalAccountQuotaExecutionPermitInput{
		FactID: historicalFact.ID, Settlement: historicalInput, RequestFingerprint: historicalFact.RequestFingerprint,
		AllowFunding: true, AllowToken: true, ApprovalID: "synthetic-native-review", EvidenceDigest: strings.Repeat("a", 64),
	})
	require.NoError(t, err)
	require.NotNil(t, permit)
	permitOriginal := *permit
	require.NoError(t, db.Model(&AccountQuotaExecutionPermit{}).Where("id = ?", permit.ID).
		Update("binding_digest", strings.Repeat("f", 64)).Error)
	var invalidPermitBefore, invalidPermitAfter AccountQuotaExecutionPermit
	var boundFactBefore, boundFactAfter AccountQuotaSettlementFact
	require.NoError(t, db.First(&invalidPermitBefore, permit.ID).Error)
	require.NoError(t, db.First(&boundFactBefore, historicalFact.ID).Error)
	require.Error(t, harness.execution(db))
	require.NoError(t, db.First(&invalidPermitAfter, permit.ID).Error)
	require.NoError(t, db.First(&boundFactAfter, historicalFact.ID).Error)
	assert.Equal(t, invalidPermitBefore, invalidPermitAfter)
	assert.Equal(t, boundFactBefore, boundFactAfter)
	require.NoError(t, db.Model(&AccountQuotaExecutionPermit{}).Where("id = ?", permit.ID).
		Update("binding_digest", permitOriginal.BindingDigest).Error)
	require.NoError(t, harness.execution(db))

	keyFailure := errors.New("synthetic key-leg failure")
	accountQuotaSettlementBeforeTokenHook = func(*AccountQuotaSettlementFact) error { return keyFailure }
	failed, err := RecoverAccountQuotaSettlementFact(context.Background(), db, historicalFact, "native-key-failure")
	require.ErrorIs(t, err, keyFailure)
	require.True(t, failed.FundingApplied)
	require.False(t, failed.TokenApplied)
	assertHistoricalAccountingBalances(t, db, historical, 960, 500, 0)
	accountQuotaSettlementBeforeTokenHook = nil

	require.NoError(t, liveSQL.Close())
	liveSQL = nil
	db, err = harness.open()
	require.NoError(t, err)
	liveSQL, err = db.DB()
	require.NoError(t, err)
	liveSQL.SetMaxOpenConns(4)
	recovered, err := RecoverAccountQuotaSettlementFact(context.Background(), db, failed, "native-reopened-recovery")
	require.NoError(t, err)
	assert.Equal(t, AccountQuotaSettlementApplied, recovered.State)
	assertHistoricalAccountingBalances(t, db, historical, 960, 460, 40)
	replayed, err := RecoverAccountQuotaSettlementFact(context.Background(), db, recovered, "native-reopened-replay")
	require.NoError(t, err)
	assert.Equal(t, recovered.ID, replayed.ID)
	assertHistoricalAccountingBalances(t, db, historical, 960, 460, 40)

	dropHistoricalAccountingContractTables(t, db)
	unsafeType := "REAL"
	if harness.databaseType == common.DatabaseTypeMySQL {
		unsafeType = "DOUBLE"
	} else if harness.databaseType == common.DatabaseTypePostgreSQL {
		unsafeType = "DOUBLE PRECISION"
	}
	require.NoError(t, db.Exec(fmt.Sprintf("CREATE TABLE users (id BIGINT PRIMARY KEY, quota %s NOT NULL, used_quota BIGINT NOT NULL)", unsafeType)).Error)
	require.NoError(t, db.Exec("CREATE TABLE tokens (id BIGINT PRIMARY KEY, remain_quota BIGINT NOT NULL, used_quota BIGINT NOT NULL)").Error)
	require.NoError(t, db.Exec("INSERT INTO users (id, quota, used_quota) VALUES (1, 1, 0)").Error)
	require.NoError(t, db.Exec("INSERT INTO tokens (id, remain_quota, used_quota) VALUES (1, 1, 0)").Error)
	physicalBefore := historicalAccountingColumnType(t, db, "users", "quota")
	var rowBefore struct {
		ID        int64
		Quota     float64
		UsedQuota int64
	}
	require.NoError(t, db.Table("users").First(&rowBefore).Error)
	probeErr = harness.probe(db)
	require.ErrorIs(t, probeErr, ErrQuotaAmountStartupUnverifiable)
	assert.ErrorContains(t, probeErr, "users.quota")
	var rowAfter struct {
		ID        int64
		Quota     float64
		UsedQuota int64
	}
	require.NoError(t, db.Table("users").First(&rowAfter).Error)
	assert.Equal(t, rowBefore, rowAfter)
	assert.Equal(t, physicalBefore, historicalAccountingColumnType(t, db, "users", "quota"))
}

func createHistoricalAccountingFixture(t *testing.T, db *gorm.DB, label string, userQuota, tokenQuota int64) accountQuotaFixture {
	t.Helper()
	name := "native-" + label
	user := User{Username: name, AffCode: name, Password: "fixture-password", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Quota: userQuota, AuthVersion: 1}
	require.NoError(t, db.Create(&user).Error)
	token := Token{UserId: user.Id, Key: name, Status: common.TokenStatusEnabled, RemainQuota: tokenQuota, ExpiredTime: -1}
	require.NoError(t, db.Create(&token).Error)
	return accountQuotaFixture{User: user, Token: token}
}

func assertHistoricalAccountingBalances(t *testing.T, db *gorm.DB, fixture accountQuotaFixture, userQuota, tokenRemaining, tokenUsed int64) {
	t.Helper()
	user, token, _ := loadAccountBalances(t, db, fixture)
	assert.Equal(t, userQuota, user.Quota)
	assert.Equal(t, tokenRemaining, token.RemainQuota)
	assert.Equal(t, tokenUsed, token.UsedQuota)
}

func historicalAccountingColumnType(t *testing.T, db *gorm.DB, table, column string) string {
	t.Helper()
	columns, err := db.Migrator().ColumnTypes(table)
	require.NoError(t, err)
	for _, candidate := range columns {
		if strings.EqualFold(candidate.Name(), column) {
			definition, _ := candidate.ColumnType()
			return strings.ToLower(candidate.DatabaseTypeName() + ":" + definition)
		}
	}
	require.FailNow(t, "expected fixture column is missing")
	return ""
}

func dropHistoricalAccountingContractTables(t *testing.T, db *gorm.DB) {
	t.Helper()
	for _, table := range []any{
		&AccountQuotaExecutionPermit{}, &AccountQuotaSettlementFact{}, &AccountQuotaSettlementIntent{},
		&QuotaWorkCursor{}, &LegacyUsageReservation{}, &AccountQuotaExecutionGate{}, &Token{}, &User{},
	} {
		require.NoError(t, db.Migrator().DropTable(table))
	}
}
