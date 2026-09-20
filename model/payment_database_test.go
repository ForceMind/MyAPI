package model

import (
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/logger"
	"github.com/glebarez/sqlite"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func s2aPaymentDatabaseDialector(engine, dsn string) (gorm.Dialector, error) {
	unsafeTarget := errors.New("S2-A payment tests require a literal loopback address and database myapi_s2a_test")
	switch engine {
	case "mysql":
		cfg, err := mysqldriver.ParseDSN(dsn)
		if err != nil {
			return nil, errors.New("invalid S2-A MySQL fixture DSN")
		}
		host, _, err := net.SplitHostPort(cfg.Addr)
		if err != nil || cfg.Net != "tcp" || !net.ParseIP(host).IsLoopback() || cfg.DBName != "myapi_s2a_test" {
			return nil, unsafeTarget
		}
		return mysql.New(mysql.Config{DSN: cfg.FormatDSN()}), nil
	case "postgres":
		cfg, err := pgx.ParseConfig(dsn)
		if err != nil {
			return nil, errors.New("invalid S2-A PostgreSQL fixture DSN")
		}
		if !net.ParseIP(cfg.Host).IsLoopback() || cfg.Database != "myapi_s2a_test" {
			return nil, unsafeTarget
		}
		for _, fallback := range cfg.Fallbacks {
			if !net.ParseIP(fallback.Host).IsLoopback() {
				return nil, unsafeTarget
			}
		}
		cfg.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
		return postgres.New(postgres.Config{Conn: stdlib.OpenDB(*cfg)}), nil
	default:
		return nil, errors.New("unsupported S2-A database engine")
	}
}

func TestS2APaymentDatabaseTargetSafety(t *testing.T) {
	for _, fixture := range []struct{ name, engine, dsn string }{
		{"mysql-remote", "mysql", "fixture:fixture@tcp(192.0.2.1:3306)/myapi_s2a_test"},
		{"mysql-s1-database", "mysql", "fixture:fixture@tcp(127.0.0.1:3306)/myapi_s1_test"},
		{"mysql-socket", "mysql", "fixture:fixture@unix(/tmp/mysql.sock)/myapi_s2a_test"},
		{"postgres-remote", "postgres", "postgres://fixture:fixture@192.0.2.1:5432/myapi_s2a_test?sslmode=disable"},
		{"postgres-s1-database", "postgres", "postgres://fixture:fixture@127.0.0.1:5432/myapi_s1_test?sslmode=disable"},
		{"postgres-remote-fallback", "postgres", "host=127.0.0.1,192.0.2.1 port=5432 dbname=myapi_s2a_test user=fixture password=fixture sslmode=disable"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			_, err := s2aPaymentDatabaseDialector(fixture.engine, fixture.dsn)
			require.Error(t, err)
		})
	}
}

// runS2APaymentReplay holds the first operation after its real SELECT FOR
// UPDATE and starts the second transaction's query before releasing it. The
// channels control the interleaving; timeouts only diagnose unexpected hangs.
// SQLite runs sequential replays of the same business operations; it neither
// supports nor simulates the other engines' row-lock/concurrency contract.
func runS2APaymentReplay(t *testing.T, db *gorm.DB, table string, first, second func() error) (error, error) {
	t.Helper()
	if db.Dialector.Name() == "sqlite" {
		firstErr := first()
		return firstErr, second()
	}
	locked, contender, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	resume := sync.OnceFunc(func() { close(release) })
	var entered atomic.Int32
	var held atomic.Bool
	var workers sync.WaitGroup
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register("test:s2a-contender", func(tx *gorm.DB) {
		if tx.Statement.Table == table && entered.Add(1) == 2 {
			close(contender)
		}
	}))
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("test:s2a-held", func(tx *gorm.DB) {
		if tx.Statement.Table == table && tx.Error == nil && held.CompareAndSwap(false, true) {
			close(locked)
			<-release
		}
	}))
	cleanup := sync.OnceFunc(func() {
		resume()
		workers.Wait()
		require.NoError(t, db.Callback().Query().Remove("test:s2a-contender"))
		require.NoError(t, db.Callback().Query().Remove("test:s2a-held"))
	})
	t.Cleanup(cleanup)
	firstResult, secondResult := make(chan error, 1), make(chan error, 1)
	workers.Add(1)
	go func() { defer workers.Done(); firstResult <- first() }()
	select {
	case <-locked:
	case err := <-firstResult:
		t.Fatalf("first payment operation ended before its row-lock barrier: %v", err)
	case <-time.After(20 * time.Second):
		t.Fatal("first payment operation did not reach its row-lock barrier")
	}
	workers.Add(1)
	go func() { defer workers.Done(); secondResult <- second() }()
	select {
	case <-contender:
	case err := <-secondResult:
		t.Fatalf("second payment operation ended before contending: %v", err)
	case <-time.After(20 * time.Second):
		t.Fatal("second payment operation did not reach its query barrier")
	}
	resume()
	var firstErr, secondErr error
	select {
	case firstErr = <-firstResult:
	case <-time.After(20 * time.Second):
		t.Fatal("first payment operation did not complete")
	}
	select {
	case secondErr = <-secondResult:
	case <-time.After(20 * time.Second):
		t.Fatal("second payment operation did not complete")
	}
	cleanup()
	return firstErr, secondErr
}

func s2aPaymentLogs(t *testing.T, db *gorm.DB, userID int) []Log {
	t.Helper()
	var logs []Log
	require.NoError(t, db.Where("user_id = ? AND type = ?", userID, LogTypeTopup).Order("id").Find(&logs).Error)
	return logs
}

// A committed payment adds exactly its expected Chinese log entry; replays,
// failed transactions, refunds, and late failure callbacks add none. Retain
// the earlier rows too, so an overwritten/missing entry cannot hide in counts.
func assertS2APaymentLogDelta(t *testing.T, db *gorm.DB, userID int, before []Log, content ...string) {
	t.Helper()
	logs := s2aPaymentLogs(t, db, userID)
	require.Len(t, logs, len(before)+len(content))
	for i, previous := range before {
		assert.Equal(t, previous, logs[i])
	}
	for i, expected := range content {
		assert.Equal(t, expected, logs[len(before)+i].Content)
	}
}

func TestS2APaymentConfiguredDatabases(t *testing.T) {
	if os.Getenv("MYAPI_S2A_DATABASE_TESTS") != "1" {
		t.Skip("disposable S2-A database tests require MYAPI_S2A_DATABASE_TESTS=1")
	}
	for _, engine := range []struct {
		name, env string
		typeID    common.DatabaseType
	}{
		{"mysql", "MYAPI_S2A_MYSQL_DSN", common.DatabaseTypeMySQL},
		{"postgres", "MYAPI_S2A_POSTGRES_DSN", common.DatabaseTypePostgreSQL},
	} {
		t.Run(engine.name, func(t *testing.T) {
			dsn := os.Getenv(engine.env)
			if dsn == "" {
				t.Skip(engine.env + " is not configured")
			}
			dialector, err := s2aPaymentDatabaseDialector(engine.name, dsn)
			require.NoError(t, err)
			db, err := gorm.Open(dialector, &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
			tables, err := db.Migrator().GetTables()
			require.NoError(t, err)
			require.Empty(t, tables, "refusing a non-empty S2-A fixture database; CI owns its lifecycle, no tables are dropped")
			if engine.typeID == common.DatabaseTypeMySQL {
				// All target/empty-schema guards have passed. This fixed disposable
				// database is the only allowed ALTER target; never interpolate a DSN.
				require.NoError(t, db.Exec("ALTER DATABASE `myapi_s2a_test` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci").Error)
			}
			runS2APaymentDatabaseMatrix(t, db, engine.typeID)
		})
	}
}

func TestS2APaymentSQLite(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	runS2APaymentDatabaseMatrix(t, db, common.DatabaseTypeSQLite)
}

func runS2APaymentDatabaseMatrix(t *testing.T, db *gorm.DB, databaseType common.DatabaseType) {
	t.Helper()
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	previousDB, previousLogDB := DB, LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	previousRedis, previousBatch, previousQuotaUnit := common.RedisEnabled, common.BatchUpdateEnabled, common.QuotaPerUnit
	DB, LOG_DB = db, db
	common.SetDatabaseTypes(databaseType, databaseType)
	common.RedisEnabled, common.BatchUpdateEnabled, common.QuotaPerUnit = false, false, 100
	initCol()
	t.Cleanup(func() {
		DB, LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
		common.RedisEnabled, common.BatchUpdateEnabled, common.QuotaPerUnit = previousRedis, previousBatch, previousQuotaUnit
		initCol()
		if previousDB != nil {
			_ = RefreshUserQuotaBusinessSchemaCapability(previousDB)
		}
	})
	if databaseType == common.DatabaseTypeMySQL {
		require.NoError(t, checkMySQLChineseSupport(db), "fixture database must support Chinese before migration")
	}
	require.NoError(t, db.AutoMigrate(&User{}, &SubscriptionPlan{}, &SubscriptionOrder{}, &UserSubscription{}, &SubscriptionPreConsumeRecord{}, &TopUp{}, &Log{}))
	require.NoError(t, RefreshUserQuotaBusinessSchemaCapability(db))
	if databaseType == common.DatabaseTypeMySQL {
		require.NoError(t, checkMySQLChineseSupport(db), "fixture tables must support Chinese after migration")
	}
	user := User{Username: "s2a-fixture-user", Password: "fixture-not-a-login", AffCode: "s2a-fixture", Quota: 1000, Group: "default"}
	require.NoError(t, db.Create(&user).Error)
	plan := SubscriptionPlan{Title: "S2-A中文订阅套餐", PriceAmount: 2, Enabled: true, DurationUnit: SubscriptionDurationDay, DurationValue: 7, TotalAmount: 1000, UpgradeGroup: "priority"}
	require.NoError(t, db.Create(&plan).Error)
	InvalidateSubscriptionPlanCache(plan.Id)
	t.Cleanup(func() { InvalidateSubscriptionPlanCache(plan.Id) })

	for _, temperature := range []string{"cold", "warm"} {
		t.Run("single-connection-complete-"+temperature, func(t *testing.T) {
			InvalidateSubscriptionPlanCache(plan.Id)
			if temperature == "warm" {
				_, err := GetSubscriptionPlanById(plan.Id)
				require.NoError(t, err)
			}
			order := SubscriptionOrder{UserId: user.Id, PlanId: plan.Id, Money: 2, TradeNo: "s2a-complete-" + temperature, PaymentMethod: PaymentMethodStripe, PaymentProvider: PaymentProviderStripe, Status: common.TopUpStatusPending}
			require.NoError(t, db.Create(&order).Error)
			logsBefore := s2aPaymentLogs(t, db, user.Id)
			expectedLog := fmt.Sprintf("订阅购买成功，套餐: %s，支付金额: 2.00，支付方式: %s", plan.Title, PaymentMethodStripe)
			var before int64
			require.NoError(t, db.Model(&UserSubscription{}).Count(&before).Error)
			beforeTime := GetDBTimestamp()
			require.NoError(t, CompleteSubscriptionOrderTrusted(order.TradeNo, "fixture", PaymentProviderStripe, ""))
			assertS2APaymentLogDelta(t, db, user.Id, logsBefore, expectedLog)
			require.NoError(t, CompleteSubscriptionOrderTrusted(order.TradeNo, "fixture", PaymentProviderStripe, ""))
			assertS2APaymentLogDelta(t, db, user.Id, logsBefore, expectedLog)
			var after int64
			require.NoError(t, db.Model(&UserSubscription{}).Count(&after).Error)
			assert.Equal(t, before+1, after)
			var sub UserSubscription
			require.NoError(t, db.Order("id desc").First(&sub).Error)
			assert.GreaterOrEqual(t, sub.StartTime, beforeTime)
			assert.LessOrEqual(t, sub.StartTime, GetDBTimestamp())
			require.NoError(t, db.First(&order, order.Id).Error)
			assert.Equal(t, common.TopUpStatusSuccess, order.Status)
		})
	}

	t.Run("completion-rollback", func(t *testing.T) {
		order := SubscriptionOrder{UserId: user.Id, PlanId: plan.Id, Money: 2, TradeNo: "s2a-completion-rollback", PaymentMethod: PaymentMethodStripe, PaymentProvider: PaymentProviderStripe, Status: common.TopUpStatusPending}
		require.NoError(t, db.Create(&order).Error)
		logsBefore := s2aPaymentLogs(t, db, user.Id)
		var before int64
		require.NoError(t, db.Model(&UserSubscription{}).Count(&before).Error)
		writeErr := errors.New("injected S2-A order save failure")
		require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:s2a-order-save", func(tx *gorm.DB) {
			if tx.Statement.Table == "subscription_orders" {
				tx.AddError(writeErr)
			}
		}))
		assert.ErrorIs(t, CompleteSubscriptionOrderTrusted(order.TradeNo, "fixture", PaymentProviderStripe, ""), writeErr)
		assertS2APaymentLogDelta(t, db, user.Id, logsBefore)
		require.NoError(t, db.Callback().Update().Remove("test:s2a-order-save"))
		require.NoError(t, db.First(&order, order.Id).Error)
		assert.Equal(t, common.TopUpStatusPending, order.Status)
		var after, topups int64
		require.NoError(t, db.Model(&UserSubscription{}).Count(&after).Error)
		require.NoError(t, db.Model(&TopUp{}).Where("trade_no = ?", order.TradeNo).Count(&topups).Error)
		assert.Equal(t, before, after)
		assert.Zero(t, topups)
		require.NoError(t, CompleteSubscriptionOrderTrusted(order.TradeNo, "fixture", PaymentProviderStripe, ""))
		assertS2APaymentLogDelta(t, db, user.Id, logsBefore, fmt.Sprintf("订阅购买成功，套餐: %s，支付金额: 2.00，支付方式: %s", plan.Title, PaymentMethodStripe))
	})

	t.Run("refund-rollback-and-replay", func(t *testing.T) {
		sub := UserSubscription{UserId: user.Id, PlanId: plan.Id, AmountTotal: 1000, AmountUsed: 900, Status: "active"}
		require.NoError(t, db.Create(&sub).Error)
		record := SubscriptionPreConsumeRecord{RequestId: "s2a-refund-rollback", UserId: user.Id, UserSubscriptionId: sub.Id, PreConsumed: 100, Status: "consumed"}
		require.NoError(t, db.Create(&record).Error)
		logsBefore := s2aPaymentLogs(t, db, user.Id)
		writeErr := errors.New("injected S2-A refund marker save failure")
		require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:s2a-refund-marker", func(tx *gorm.DB) {
			if tx.Statement.Table == "subscription_pre_consume_records" {
				tx.AddError(writeErr)
			}
		}))
		assert.ErrorIs(t, RefundSubscriptionPreConsume(record.RequestId), writeErr)
		assertS2APaymentLogDelta(t, db, user.Id, logsBefore)
		require.NoError(t, db.Callback().Update().Remove("test:s2a-refund-marker"))
		require.NoError(t, db.First(&sub, sub.Id).Error)
		require.NoError(t, db.First(&record, record.Id).Error)
		assert.EqualValues(t, 900, sub.AmountUsed)
		assert.Equal(t, "consumed", record.Status)
		require.NoError(t, RefundSubscriptionPreConsume(record.RequestId))
		require.NoError(t, RefundSubscriptionPreConsume(record.RequestId))
		require.NoError(t, db.First(&sub, sub.Id).Error)
		require.NoError(t, db.First(&record, record.Id).Error)
		assert.EqualValues(t, 800, sub.AmountUsed)
		assert.Equal(t, "refunded", record.Status)
		assertS2APaymentLogDelta(t, db, user.Id, logsBefore)
	})

	replayMode := "sequential"
	if databaseType != common.DatabaseTypeSQLite {
		sqlDB.SetMaxOpenConns(2)
		replayMode = "concurrent"
	}
	t.Run(replayMode+"-refund", func(t *testing.T) {
		sub := UserSubscription{UserId: user.Id, PlanId: plan.Id, AmountTotal: 1000, AmountUsed: 900, Status: "active"}
		require.NoError(t, db.Create(&sub).Error)
		record := SubscriptionPreConsumeRecord{RequestId: "s2a-concurrent-refund", UserId: user.Id, UserSubscriptionId: sub.Id, PreConsumed: 100, Status: "consumed"}
		require.NoError(t, db.Create(&record).Error)
		logsBefore := s2aPaymentLogs(t, db, user.Id)
		firstErr, secondErr := runS2APaymentReplay(t, db, "subscription_pre_consume_records",
			func() error { return RefundSubscriptionPreConsume(record.RequestId) },
			func() error { return RefundSubscriptionPreConsume(record.RequestId) })
		require.NoError(t, firstErr)
		require.NoError(t, secondErr)
		require.NoError(t, db.First(&sub, sub.Id).Error)
		require.NoError(t, db.First(&record, record.Id).Error)
		assert.EqualValues(t, 800, sub.AmountUsed)
		assert.Equal(t, "refunded", record.Status)
		assertS2APaymentLogDelta(t, db, user.Id, logsBefore)
	})

	for _, outcome := range []string{"duplicate-success", "late-failure"} {
		t.Run(replayMode+"-stripe-"+outcome, func(t *testing.T) {
			require.NoError(t, db.First(&user, user.Id).Error)
			beforeQuota := user.Quota
			topup := TopUp{UserId: user.Id, Amount: 2, Money: 2, TradeNo: "s2a-stripe-" + outcome, PaymentMethod: PaymentMethodStripe, PaymentProvider: PaymentProviderStripe, Status: common.TopUpStatusPending}
			require.NoError(t, db.Create(&topup).Error)
			logsBefore := s2aPaymentLogs(t, db, user.Id)
			second := func() error { return RechargeTrusted(topup.TradeNo, "fixture-customer", "127.0.0.1") }
			if outcome == "late-failure" {
				second = func() error {
					return UpdatePendingTopUpStatusTrusted(topup.TradeNo, PaymentProviderStripe, common.TopUpStatusFailed)
				}
			}
			firstErr, secondErr := runS2APaymentReplay(t, db, "top_ups",
				func() error { return RechargeTrusted(topup.TradeNo, "fixture-customer", "127.0.0.1") }, second)
			require.NoError(t, firstErr)
			if outcome == "late-failure" {
				assert.ErrorIs(t, secondErr, ErrTopUpStatusInvalid)
			} else {
				require.NoError(t, secondErr)
			}
			require.NoError(t, db.First(&topup, topup.Id).Error)
			require.NoError(t, db.First(&user, user.Id).Error)
			assert.Equal(t, common.TopUpStatusSuccess, topup.Status)
			assert.Equal(t, beforeQuota+200, user.Quota)
			assertS2APaymentLogDelta(t, db, user.Id, logsBefore, fmt.Sprintf("使用在线充值成功，充值金额: %v，支付金额：2", logger.FormatQuota(200)))
		})
	}
	t.Run("task-billing-snapshot-roundtrip", func(t *testing.T) {
		runTaskBillingSnapshotRoundTrip(t, db)
	})
}
