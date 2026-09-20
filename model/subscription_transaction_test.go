package model

import (
	"context"
	"database/sql"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

var errSubscriptionGlobalPoolBorrow = errors.New("subscription transaction tried to borrow a second global connection")

// This is a real SQLite pool/transaction wrapper, not an engine simulation.
// A forbidden second borrow fails immediately instead of deadlocking the
// single-connection fixture. Queries issued on the actual sql.Tx still run.
type subscriptionSingleConnectionPool struct {
	gorm.ConnPool
	active atomic.Bool
}

func (pool *subscriptionSingleConnectionPool) BeginTx(ctx context.Context, opts *sql.TxOptions) (gorm.ConnPool, error) {
	if pool.active.Load() {
		return nil, errSubscriptionGlobalPoolBorrow
	}
	tx, err := pool.ConnPool.(gorm.TxBeginner).BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	pool.active.Store(true)
	return &subscriptionSingleConnectionTx{Tx: tx, pool: pool}, nil
}

func (pool *subscriptionSingleConnectionPool) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	if pool.active.Load() {
		return nil, errSubscriptionGlobalPoolBorrow
	}
	return pool.ConnPool.ExecContext(ctx, query, args...)
}

func (pool *subscriptionSingleConnectionPool) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	if pool.active.Load() {
		return nil, errSubscriptionGlobalPoolBorrow
	}
	return pool.ConnPool.QueryContext(ctx, query, args...)
}

func (pool *subscriptionSingleConnectionPool) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	if pool.active.Load() {
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		return pool.ConnPool.QueryRowContext(cancelled, query, args...)
	}
	return pool.ConnPool.QueryRowContext(ctx, query, args...)
}

type subscriptionSingleConnectionTx struct {
	*sql.Tx
	pool *subscriptionSingleConnectionPool
}

func (tx *subscriptionSingleConnectionTx) Commit() error {
	defer tx.pool.active.Store(false)
	return tx.Tx.Commit()
}

func (tx *subscriptionSingleConnectionTx) Rollback() error {
	defer tx.pool.active.Store(false)
	return tx.Tx.Rollback()
}

const subscriptionFixtureDBTime = int64(1700000000)

func subscriptionTransactionFixture(t *testing.T) (*gorm.DB, *SubscriptionPlan, *SubscriptionOrder) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	previousDB, previousLogDB := DB, LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	previousRedis := common.RedisEnabled
	DB, LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	initCol()
	t.Cleanup(func() {
		DB, LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
		common.RedisEnabled = previousRedis
		initCol()
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.AutoMigrate(&User{}, &SubscriptionPlan{}, &SubscriptionOrder{}, &UserSubscription{}, &SubscriptionPreConsumeRecord{}, &TopUp{}, &Log{}))
	// A fixed database clock makes a silent application-time fallback observable.
	require.NoError(t, db.Callback().Row().Before("gorm:row").Register("test:subscription-clock", func(tx *gorm.DB) {
		if tx.Statement.SQL.String() == "SELECT strftime('%s','now')" {
			tx.Statement.SQL.Reset()
			tx.Statement.SQL.WriteString("SELECT 1700000000")
		}
	}))
	user := User{Username: "subscription-transaction-user", Password: "fixture", AffCode: "subscription-tx", Group: "default"}
	require.NoError(t, db.Create(&user).Error)
	plan := &SubscriptionPlan{Title: "Transaction plan", PriceAmount: 9, Enabled: true, DurationUnit: SubscriptionDurationMonth, DurationValue: 1, TotalAmount: 1000, UpgradeGroup: "priority", MaxPurchasePerUser: 1}
	require.NoError(t, db.Create(plan).Error)
	InvalidateSubscriptionPlanCache(plan.Id)
	t.Cleanup(func() { InvalidateSubscriptionPlanCache(plan.Id) })
	order := &SubscriptionOrder{UserId: user.Id, PlanId: plan.Id, Money: 9, TradeNo: "subscription-transaction-order", PaymentMethod: PaymentMethodStripe, PaymentProvider: PaymentProviderStripe, Status: common.TopUpStatusPending}
	require.NoError(t, db.Create(order).Error)
	pool := &subscriptionSingleConnectionPool{ConnPool: db.ConnPool}
	db.Config.ConnPool, db.Statement.ConnPool = pool, pool
	return db, plan, order
}

func TestSubscriptionTransactionCompletionUsesOneConnection(t *testing.T) {
	for _, warm := range []bool{false, true} {
		name := "cold-cache"
		if warm {
			name = "warm-cache"
		}
		t.Run(name, func(t *testing.T) {
			db, plan, order := subscriptionTransactionFixture(t)
			if warm {
				_, err := GetSubscriptionPlanById(plan.Id)
				require.NoError(t, err)
			}
			require.NoError(t, CompleteSubscriptionOrderTrusted(order.TradeNo, "fixture-payload", PaymentProviderStripe, ""))
			require.NoError(t, CompleteSubscriptionOrderTrusted(order.TradeNo, "fixture-payload", PaymentProviderStripe, ""))
			var subscriptions []UserSubscription
			require.NoError(t, db.Where("user_id = ?", order.UserId).Find(&subscriptions).Error)
			require.Len(t, subscriptions, 1)
			assert.Equal(t, subscriptionFixtureDBTime, subscriptions[0].StartTime)
			assert.Equal(t, plan.TotalAmount, subscriptions[0].AmountTotal)
			var storedOrder SubscriptionOrder
			require.NoError(t, db.First(&storedOrder, order.Id).Error)
			assert.Equal(t, common.TopUpStatusSuccess, storedOrder.Status)
			var user User
			require.NoError(t, db.First(&user, order.UserId).Error)
			assert.Equal(t, "priority", user.Group)
			var topups int64
			require.NoError(t, db.Model(&TopUp{}).Where("trade_no = ?", order.TradeNo).Count(&topups).Error)
			assert.EqualValues(t, 1, topups)
		})
	}
}

func TestSubscriptionTransactionRefundRollsBackWithIdempotencyRecord(t *testing.T) {
	db, plan, order := subscriptionTransactionFixture(t)
	sub := UserSubscription{UserId: order.UserId, PlanId: plan.Id, AmountTotal: 1000, AmountUsed: 900, Status: "active"}
	require.NoError(t, db.Create(&sub).Error)
	record := SubscriptionPreConsumeRecord{RequestId: "subscription-refund-fixture", UserId: order.UserId, UserSubscriptionId: sub.Id, PreConsumed: 100, Status: "consumed"}
	require.NoError(t, db.Create(&record).Error)
	writeErr := errors.New("injected refund record save failure")
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:refund-record-failure", func(tx *gorm.DB) {
		if tx.Statement.Table == "subscription_pre_consume_records" {
			tx.AddError(writeErr)
		}
	}))
	assert.ErrorIs(t, RefundSubscriptionPreConsume(record.RequestId), writeErr)
	require.NoError(t, db.First(&sub, sub.Id).Error)
	require.NoError(t, db.First(&record, record.Id).Error)
	assert.EqualValues(t, 900, sub.AmountUsed)
	assert.Equal(t, "consumed", record.Status)
	require.NoError(t, db.Callback().Update().Remove("test:refund-record-failure"))
	require.NoError(t, RefundSubscriptionPreConsume(record.RequestId))
	require.NoError(t, RefundSubscriptionPreConsume(record.RequestId))
	require.NoError(t, db.First(&sub, sub.Id).Error)
	require.NoError(t, db.First(&record, record.Id).Error)
	assert.EqualValues(t, 800, sub.AmountUsed)
	assert.Equal(t, "refunded", record.Status)
}

func TestSubscriptionTransactionCompletionRollsBackEntitlements(t *testing.T) {
	db, plan, order := subscriptionTransactionFixture(t)
	_, err := GetSubscriptionPlanById(plan.Id)
	require.NoError(t, err)
	writeErr := errors.New("injected subscription order save failure")
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:subscription-order-save", func(tx *gorm.DB) {
		if tx.Statement.Table == "subscription_orders" {
			tx.AddError(writeErr)
		}
	}))
	assert.ErrorIs(t, CompleteSubscriptionOrderTrusted(order.TradeNo, "fixture", PaymentProviderStripe, ""), writeErr)
	require.NoError(t, db.First(order, order.Id).Error)
	assert.Equal(t, common.TopUpStatusPending, order.Status)
	var user User
	require.NoError(t, db.First(&user, order.UserId).Error)
	assert.Equal(t, "default", user.Group)
	for _, entity := range []interface{}{&UserSubscription{}, &TopUp{}} {
		var count int64
		require.NoError(t, db.Model(entity).Count(&count).Error)
		assert.Zero(t, count)
	}
	require.NoError(t, db.Callback().Update().Remove("test:subscription-order-save"))
	require.NoError(t, CompleteSubscriptionOrderTrusted(order.TradeNo, "fixture", PaymentProviderStripe, ""))
}

func TestSubscriptionTransactionOrderQueryFailureIsNotMissing(t *testing.T) {
	db, _, order := subscriptionTransactionFixture(t)
	readErr := errors.New("injected subscription order query failure")
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register("test:subscription-order-query", func(tx *gorm.DB) {
		if tx.Statement.Table == "subscription_orders" {
			tx.AddError(readErr)
		}
	}))
	err := CompleteSubscriptionOrderTrusted(order.TradeNo, "fixture", PaymentProviderStripe, "")
	assert.ErrorIs(t, err, readErr)
	assert.NotErrorIs(t, err, ErrSubscriptionOrderNotFound)
	err = ExpireSubscriptionOrderTrusted(order.TradeNo, PaymentProviderStripe)
	assert.ErrorIs(t, err, readErr)
	assert.NotErrorIs(t, err, ErrSubscriptionOrderNotFound)
	found, err := GetSubscriptionOrderByTradeNoWithError(order.TradeNo)
	assert.Nil(t, found)
	assert.ErrorIs(t, err, readErr)
	assert.NotErrorIs(t, err, ErrSubscriptionOrderNotFound)
	require.NoError(t, db.Callback().Query().Remove("test:subscription-order-query"))
	assert.ErrorIs(t, CompleteSubscriptionOrderTrusted("missing-order", "fixture", PaymentProviderStripe, ""), ErrSubscriptionOrderNotFound)
	found, err = GetSubscriptionOrderByTradeNoWithError("missing-order")
	assert.Nil(t, found)
	assert.ErrorIs(t, err, ErrSubscriptionOrderNotFound)
	found, err = GetSubscriptionOrderByTradeNoWithError(order.TradeNo)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, order.Id, found.Id)
}

func TestSubscriptionTransactionPlanQueryFailureDoesNotUseWarmCache(t *testing.T) {
	db, plan, order := subscriptionTransactionFixture(t)
	_, err := GetSubscriptionPlanById(plan.Id)
	require.NoError(t, err)
	readErr := errors.New("injected subscription plan query failure")
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register("test:subscription-plan-query", func(tx *gorm.DB) {
		if tx.Statement.Table == "subscription_plans" {
			tx.AddError(readErr)
		}
	}))
	assert.ErrorIs(t, CompleteSubscriptionOrderTrusted(order.TradeNo, "fixture", PaymentProviderStripe, ""), readErr)
	require.NoError(t, db.First(order, order.Id).Error)
	assert.Equal(t, common.TopUpStatusPending, order.Status)
	var subscriptions int64
	require.NoError(t, db.Model(&UserSubscription{}).Count(&subscriptions).Error)
	assert.Zero(t, subscriptions)
}

func TestSubscriptionTransactionPlanSnapshotDoesNotLeakRollback(t *testing.T) {
	db, plan, _ := subscriptionTransactionFixture(t)
	_, err := GetSubscriptionPlanById(plan.Id)
	require.NoError(t, err)
	abort := errors.New("rollback plan fixture")
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(plan).Update("total_amount", 2000).Error; err != nil {
			return err
		}
		current, err := getSubscriptionPlanByIdTx(tx, plan.Id)
		if err != nil {
			return err
		}
		assert.EqualValues(t, 2000, current.TotalAmount)
		return abort
	})
	require.ErrorIs(t, err, abort)
	current, err := GetSubscriptionPlanById(plan.Id)
	require.NoError(t, err)
	assert.EqualValues(t, 1000, current.TotalAmount)
	require.NoError(t, db.First(plan, plan.Id).Error)
	assert.EqualValues(t, 1000, plan.TotalAmount)
}

func TestSubscriptionTransactionPreConsumeQueryFailureIsNotInsufficientQuota(t *testing.T) {
	db, _, order := subscriptionTransactionFixture(t)
	readErr := errors.New("injected active subscription query failure")
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register("test:subscription-preconsume-query", func(tx *gorm.DB) {
		if tx.Statement.Table == "user_subscriptions" {
			tx.AddError(readErr)
		}
	}))
	result, err := PreConsumeUserSubscription("preconsume-query-failure", order.UserId, "fixture-model", 0, 100)
	assert.Nil(t, result)
	require.Error(t, err)
	assert.ErrorIs(t, err, readErr)
	assert.NotContains(t, err.Error(), "no active subscription")
	var records int64
	require.NoError(t, db.Model(&SubscriptionPreConsumeRecord{}).Count(&records).Error)
	assert.Zero(t, records)
}

func TestSubscriptionTransactionDatabaseTimeUsesHandleAndPreservesFallback(t *testing.T) {
	db, _, _ := subscriptionTransactionFixture(t)
	// Deliberately disagree with the live handle: SQL dialect follows the
	// transaction's SQLite connection, not unrelated process-global metadata.
	common.SetMainDatabaseType(common.DatabaseTypePostgreSQL)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		assert.Equal(t, subscriptionFixtureDBTime, getDBTimestampTx(tx))
		return nil
	}))
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	require.NoError(t, db.Callback().Row().After("test:subscription-clock").Before("gorm:row").Register("test:subscription-clock-error", func(tx *gorm.DB) {
		if tx.Statement.SQL.String() == "SELECT 1700000000" {
			tx.AddError(errors.New("injected database clock failure"))
		}
	}))
	before := common.GetTimestamp()
	fallback := GetDBTimestamp()
	after := common.GetTimestamp()
	assert.GreaterOrEqual(t, fallback, before)
	assert.LessOrEqual(t, fallback, after)
}
