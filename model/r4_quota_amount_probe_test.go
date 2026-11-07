package model

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
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
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type r4QuotaAmountProbeTarget struct {
	engine string
	dsn    string
	path   string
}

// This opt-in evidence probe deliberately retains the legal-debit acceptance
// contract. A red debit assertion is evidence of R4, never a passing regression.
func TestR4QuotaAmountPreflightProbe(t *testing.T) {
	if os.Getenv("MYAPI_R4_DATABASE_TESTS") != "1" {
		t.Skip("R4 synthetic three-database evidence requires MYAPI_R4_DATABASE_TESTS=1")
	}
	require.Equal(t, "true", os.Getenv("GITHUB_ACTIONS"), "R4 database evidence runs only in disposable Actions services")
	require.NotEmpty(t, os.Getenv("MYAPI_R4_MYSQL_DSN"), "R4 requires its explicit MySQL DSN")
	require.NotEmpty(t, os.Getenv("MYAPI_R4_POSTGRES_DSN"), "R4 requires its explicit PostgreSQL DSN")
	for _, engine := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(engine, func(t *testing.T) {
			for index, name := range []string{"legal_token_debit", "legal_token_credit", "ordinary_reserve_settle", "invalid_negative_charge"} {
				t.Run(name, func(t *testing.T) {
					target := newR4QuotaAmountProbeTarget(t, engine, index)
					db := target.open(t)
					previousDB, previousPath := DB, common.SQLitePath
					previousType := common.MainDatabaseType()
					previousRedis, previousRDB, previousBatch := common.RedisEnabled, common.RDB, common.BatchUpdateEnabled
					previousBinding, previousCapability := userQuotaBusinessSchemaBinding.Load(), userQuotaBusinessSchemaCapability.Load()
					DB, common.SQLitePath = db, target.path
					common.SetMainDatabaseType(common.DatabaseType(engine))
					common.RedisEnabled, common.RDB, common.BatchUpdateEnabled = false, nil, false
					initCol()
					t.Cleanup(func() {
						DB, common.SQLitePath = previousDB, previousPath
						common.SetMainDatabaseType(previousType)
						common.RedisEnabled, common.RDB, common.BatchUpdateEnabled = previousRedis, previousRDB, previousBatch
						userQuotaBusinessSchemaBinding.Store(previousBinding)
						userQuotaBusinessSchemaCapability.Store(previousCapability)
						initCol()
					})
					if engine == "sqlite" {
						t.Setenv("SQL_DSN", "local")
					} else {
						t.Setenv("SQL_DSN", target.dsn)
					}

					// Match the ordinary account-quota fixture's real model schema.
					require.NoError(t, db.AutoMigrate(&User{}, &Token{}, &SubscriptionPlan{}, &UserSubscription{}, &UserQuotaMutationReceipt{}, &AccountQuotaMutationReceipt{}, &AccountQuotaReservationHead{}, &AccountQuotaTerminalRecoveryObligation{}, &LegacyUsageReservation{}, &AccountQuotaRefundFact{}, &AccountQuotaSettlementIntent{}, &AccountQuotaSettlementFact{}, &QuotaWorkCursor{}, &QuotaWriterEpoch{}, &QuotaProjectionObligation{}, &AccountQuotaExecutionGate{}, &AccountQuotaExecutionPermit{}, &SubscriptionSharePool{}, &SubscriptionShareAllocation{}))
					require.NoError(t, EnsureFreshAccountQuotaExecutionGate(context.Background(), db))
					require.NoError(t, EnsureQuotaWriterEpochStateWithDB(db))
					require.NoError(t, db.Model(&QuotaWriterEpoch{}).Where("id = ?", quotaWriterEpochSingletonID).Updates(map[string]interface{}{"mode": string(QuotaWriterModeAuthoritative), "epoch": 7, "lock_version": gorm.Expr("lock_version + ?", 1)}).Error)
					state, err := GetQuotaWriterEpochState(db)
					require.NoError(t, err)
					require.Equal(t, string(QuotaWriterModeAuthoritative), state.Mode)
					require.EqualValues(t, 7, state.Epoch)
					var version string
					versionQuery := "SELECT version()"
					if engine == "sqlite" {
						versionQuery = "SELECT sqlite_version()"
					}
					require.NoError(t, db.Raw(versionQuery).Scan(&version).Error)
					require.NotEmpty(t, version)
					if engine == "mysql" {
						require.True(t, strings.HasPrefix(version, "5.7."), "R4 requires real MySQL 5.7")
					}
					if engine == "postgres" {
						require.Contains(t, version, "PostgreSQL 9.6.", "R4 requires real PostgreSQL 9.6")
					}

					// Reuse the native cross-database fixture's bounded identifiers.
					fixture := createHistoricalAccountingFixture(t, db, "r4", 1000, 500)
					baselinePool, err := db.DB()
					require.NoError(t, err)
					require.NoError(t, baselinePool.Close())
					require.NoError(t, ValidateQuotaAmountPreMigrationConfiguration(context.Background()), "clean real-model fixture must pass startup amount validation before the mutation")
					db = target.open(t)
					DB = db
					const amount = 100
					var receipt AccountQuotaMutationReceipt
					wantUser, wantRemain, wantUsed, wantVersion := int64(1000), int64(400), int64(100), int64(1)
					wantUserVersion, wantReceipts := int64(0), int64(1)
					if name == "legal_token_debit" || name == "legal_token_credit" {
						mutation := QuotaMutationContext{Context: context.Background(), EventKey: "r4:" + name, ReasonCode: "r4_synthetic_probe"}
						if name == "legal_token_credit" {
							// Credit reverses previously consumed quota; used quota remains nonnegative.
							require.NoError(t, db.Model(&Token{}).Where("id = ?", fixture.Token.Id).Update("used_quota", amount).Error)
							require.NoError(t, IncreaseTokenQuotaWithContext(mutation, fixture.Token.Id, fixture.Token.Key, amount))
							wantRemain, wantUsed = 600, 0
						} else {
							require.NoError(t, DecreaseTokenQuotaWithContext(mutation, fixture.Token.Id, fixture.Token.Key, amount))
						}
						var first AccountQuotaMutationReceipt
						require.NoError(t, db.Where("event_key = ?", mutation.EventKey).First(&first).Error)
						require.Equal(t, "direct_token", first.Phase)
						wantDelta := int64(-amount)
						if name == "legal_token_credit" {
							wantDelta = amount
						}
						require.Equal(t, wantDelta, first.RequestedQuota)
						require.Equal(t, wantDelta, first.AppliedQuota)
						require.Equal(t, AccountQuotaDeltas{TokenRemainQuota: wantDelta, TokenUsedQuota: -wantDelta}, first.AppliedDeltas)
						require.Equal(t, first.AppliedDeltas, first.RequestedDeltas)
						require.EqualValues(t, 500, first.Before.Token.RemainQuota)
						require.Zero(t, first.Before.Token.QuotaVersion)
						if name == "legal_token_credit" {
							require.EqualValues(t, amount, first.Before.Token.UsedQuota)
						} else {
							require.Zero(t, first.Before.Token.UsedQuota)
						}
						require.Equal(t, wantRemain, first.After.Token.RemainQuota)
						require.Equal(t, wantUsed, first.After.Token.UsedQuota)
						require.EqualValues(t, 1, first.After.Token.QuotaVersion)
						require.Equal(t, first.Before.User, first.After.User)
						beforeReplay := r4QuotaAmountRows(t, db)
						if name == "legal_token_credit" {
							require.NoError(t, IncreaseTokenQuotaWithContext(mutation, fixture.Token.Id, fixture.Token.Key, amount))
						} else {
							require.NoError(t, DecreaseTokenQuotaWithContext(mutation, fixture.Token.Id, fixture.Token.Key, amount))
						}
						require.True(t, reflect.DeepEqual(beforeReplay, r4QuotaAmountRows(t, db)), "same event key must not mutate rows on replay")
						require.NoError(t, db.Where("event_key = ?", mutation.EventKey).First(&receipt).Error)
						require.Equal(t, first.ID, receipt.ID)
					} else {
						input := accountReserveInput(fixture, "r4-"+name, "wallet_only", amount)
						reserve, err := ReserveAccountQuota(context.Background(), db, input)
						require.NoError(t, err)
						require.NotNil(t, reserve)
						require.Equal(t, AccountQuotaPhaseReserve, reserve.Phase)
						require.EqualValues(t, amount, reserve.RequestedQuota)
						require.EqualValues(t, amount, reserve.AppliedQuota)
						receipt = *reserve
						wantUser, wantUserVersion = 900, 1
						if name == "ordinary_reserve_settle" {
							settle, err := SettleAccountQuota(context.Background(), db, AccountQuotaTerminalInput{RequestID: input.RequestID, ReserveReceiptID: reserve.ID, ActualQuota: 60})
							require.NoError(t, err)
							require.NotNil(t, settle)
							require.Equal(t, AccountQuotaPhaseSettle, settle.Phase)
							require.EqualValues(t, 60, settle.RequestedQuota)
							require.EqualValues(t, 60, settle.AppliedQuota)
							wantUser, wantRemain, wantUsed, wantVersion, wantUserVersion, wantReceipts = 940, 440, 60, 2, 2, 2
						} else {
							// Only this disposable fixture bypasses immutability to represent corruption.
							require.NoError(t, db.Exec("UPDATE account_quota_mutation_receipts SET requested_quota = ?, applied_quota = ? WHERE id = ?", -1, -1, reserve.ID).Error)
							require.NoError(t, db.First(&receipt, reserve.ID).Error)
							require.Equal(t, AccountQuotaPhaseReserve, receipt.Phase)
							require.EqualValues(t, -1, receipt.RequestedQuota)
							require.EqualValues(t, -1, receipt.AppliedQuota)
						}
					}
					user, token, _ := loadAccountBalances(t, db, fixture)
					require.Equal(t, wantUser, user.Quota)
					require.Zero(t, user.UsedQuota)
					require.Equal(t, wantUserVersion, user.QuotaVersion)
					require.Equal(t, wantRemain, token.RemainQuota)
					require.Equal(t, wantUsed, token.UsedQuota)
					require.Equal(t, wantVersion, token.QuotaVersion)
					var receiptCount int64
					require.NoError(t, db.Model(&AccountQuotaMutationReceipt{}).Count(&receiptCount).Error)
					require.Equal(t, wantReceipts, receiptCount)
					rowsBefore := r4QuotaAmountRows(t, db)
					pool, err := db.DB()
					require.NoError(t, err)
					require.NoError(t, pool.Close(), "close writer before the real startup preflight")
					var filesBefore map[string][]byte
					if engine == "sqlite" {
						filesBefore = r4QuotaAmountFiles(t, filepath.Dir(target.path))
					}
					preflightErr := ValidateQuotaAmountPreMigrationConfiguration(context.Background())
					if engine == "sqlite" {
						require.True(t, reflect.DeepEqual(filesBefore, r4QuotaAmountFiles(t, filepath.Dir(target.path))), "startup amount preflight must preserve SQLite files byte for byte")
					}
					reopened := target.open(t)
					require.True(t, reflect.DeepEqual(rowsBefore, r4QuotaAmountRows(t, reopened)), "startup amount preflight must preserve every fixture row")
					amountGuard := errors.Is(preflightErr, ErrQuotaAmountStartupUnverifiable) &&
						(strings.Contains(preflightErr.Error(), "account_quota_mutation_receipts.requested_quota") || strings.Contains(preflightErr.Error(), "account_quota_mutation_receipts.applied_quota")) &&
						strings.Contains(preflightErr.Error(), "stored value is null, non-integer, or outside the exact range")
					assertion := "preflight_accepts_legal_amount"
					if name == "invalid_negative_charge" {
						assertion = "preflight_rejects_invalid_negative_charge"
					}
					marker, err := common.Marshal(map[string]interface{}{
						"database": engine, "database_version": version, "case": name,
						"stage": "target_assertion", "assertion": assertion, "controls_passed": true,
						"preflight_rejected": preflightErr != nil, "startup_error_is_amount_guard": amountGuard,
						"read_only_preserved": true, "baseline_preflight_passed": true, "writer_mode": state.Mode, "writer_epoch": state.Epoch,
						"user_balance": user.Quota, "user_version": user.QuotaVersion,
						"token_balance": token.RemainQuota, "token_used": token.UsedQuota, "token_version": token.QuotaVersion,
						"receipt_phase": receipt.Phase, "receipt_requested": receipt.RequestedQuota, "receipt_applied": receipt.AppliedQuota, "receipt_count": receiptCount,
					})
					require.NoError(t, err)
					t.Logf("R4_CASE %s", marker)
					if name == "invalid_negative_charge" {
						require.ErrorIs(t, preflightErr, ErrQuotaAmountStartupUnverifiable)
						require.True(t, amountGuard, "invalid reserve charge must be rejected by the receipt amount guard")
					} else if name == "legal_token_debit" {
						require.NoError(t, preflightErr, "legal token debit must survive startup amount validation")
					} else {
						require.NoError(t, preflightErr, "legal positive amounts must survive startup amount validation")
					}
				})
			}
		})
	}
}

// Every case owns a newly created database; only that database is ever dropped.
func newR4QuotaAmountProbeTarget(t *testing.T, engine string, index int) r4QuotaAmountProbeTarget {
	t.Helper()
	if engine == "sqlite" {
		return r4QuotaAmountProbeTarget{engine: engine, path: filepath.Join(t.TempDir(), "r4.db")}
	}
	name := fmt.Sprintf("myapi_r4_%s_%x_%d", engine, time.Now().UnixNano(), index)
	var admin *sql.DB
	var targetDSN, create, drop string
	switch engine {
	case "mysql":
		config, err := mysqldriver.ParseDSN(os.Getenv("MYAPI_R4_MYSQL_DSN"))
		require.True(t, err == nil, "invalid R4 MySQL DSN")
		host, _, err := net.SplitHostPort(config.Addr)
		require.True(t, err == nil && host == "127.0.0.1" && config.Net == "tcp" && config.DBName == "myapi_r4_probe", "R4 MySQL requires literal 127.0.0.1 and myapi_r4_probe")
		connector, err := mysqldriver.NewConnector(config)
		require.True(t, err == nil, "invalid R4 MySQL connector")
		admin = sql.OpenDB(connector)
		config.DBName, config.ParseTime = name, true
		targetDSN = config.FormatDSN()
		create, drop = "CREATE DATABASE `"+name+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci", "DROP DATABASE `"+name+"`"
	case "postgres":
		input := os.Getenv("MYAPI_R4_POSTGRES_DSN")
		parsed, err := url.Parse(input)
		require.True(t, err == nil && (parsed.Scheme == "postgres" || parsed.Scheme == "postgresql"), "R4 PostgreSQL requires an explicit PostgreSQL URL")
		config, err := pgx.ParseConfig(input)
		require.True(t, err == nil, "invalid R4 PostgreSQL DSN")
		require.True(t, config.Host == "127.0.0.1" && config.Database == "myapi_r4_probe", "R4 PostgreSQL requires literal 127.0.0.1 and myapi_r4_probe")
		for _, fallback := range config.Fallbacks {
			require.Equal(t, "127.0.0.1", fallback.Host, "R4 PostgreSQL fallback must remain local")
		}
		require.Empty(t, config.RuntimeParams["search_path"], "R4 owns a new database with the default schema")
		config.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
		admin = stdlib.OpenDB(*config)
		parsed.Path = "/" + name
		targetDSN = parsed.String()
		targetConfig, err := pgx.ParseConfig(targetDSN)
		require.True(t, err == nil && targetConfig.Database == name && targetConfig.Host == "127.0.0.1", "derived R4 PostgreSQL target must select the newly created database")
		create, drop = `CREATE DATABASE "`+name+`"`, `DROP DATABASE "`+name+`"`
	default:
		require.FailNow(t, "unsupported R4 database")
	}
	admin.SetMaxOpenConns(1)
	t.Cleanup(func() { assert.NoError(t, admin.Close()) })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err := admin.ExecContext(ctx, create)
	require.True(t, err == nil, "could not create a fresh disposable R4 database")
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		_, cleanupErr := admin.ExecContext(cleanupCtx, drop)
		assert.True(t, cleanupErr == nil, "could not drop the database created by this R4 case")
	})
	return r4QuotaAmountProbeTarget{engine: engine, dsn: targetDSN}
}

func (target r4QuotaAmountProbeTarget) open(t *testing.T) *gorm.DB {
	t.Helper()
	var dialector gorm.Dialector
	switch target.engine {
	case "sqlite":
		dialector = sqlite.Open(target.path)
	case "mysql":
		dialector = gormmysql.New(gormmysql.Config{DSN: target.dsn})
	case "postgres":
		dialector = postgres.New(postgres.Config{DSN: target.dsn, PreferSimpleProtocol: true})
	}
	db, err := gorm.Open(dialector, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.True(t, err == nil, "could not open the isolated R4 database")
	pool, err := db.DB()
	require.NoError(t, err)
	pool.SetMaxOpenConns(1)
	t.Cleanup(func() { assert.NoError(t, pool.Close()) })
	return db
}

func r4QuotaAmountRows(t *testing.T, db *gorm.DB) map[string][]string {
	t.Helper()
	tables, err := db.Migrator().GetTables()
	require.NoError(t, err)
	snapshot := make(map[string][]string, len(tables))
	for _, table := range tables {
		var rows []map[string]interface{}
		require.NoError(t, db.Table(table).Find(&rows).Error)
		encoded := make([]string, 0, len(rows))
		for _, row := range rows {
			data, err := common.Marshal(row)
			require.NoError(t, err)
			encoded = append(encoded, string(data))
		}
		sort.Strings(encoded)
		snapshot[table] = encoded
	}
	return snapshot
}

func r4QuotaAmountFiles(t *testing.T, directory string) map[string][]byte {
	t.Helper()
	entries, err := os.ReadDir(directory)
	require.NoError(t, err)
	snapshot := make(map[string][]byte, len(entries))
	for _, entry := range entries {
		require.False(t, entry.IsDir(), "the SQLite fixture directory contains only database files")
		data, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		require.NoError(t, err)
		snapshot[entry.Name()] = data
	}
	require.NotEmpty(t, snapshot)
	return snapshot
}
