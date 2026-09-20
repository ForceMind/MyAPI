package model

import (
	"errors"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/setting/operation_setting"
	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const b2SubmissionFixtureDatabase = "myapi_b2_test"

// b2LegacyLog is the complete logs schema immediately before the B2
// billing_event_id addition. It deliberately uses the same table name so the
// fixture exercises GORM's real cross-database additive migration path.
type b2LegacyLog struct {
	Id                int    `json:"id" gorm:"primaryKey;autoIncrement;index:idx_created_at_id,priority:2;index:idx_user_id_id,priority:2"`
	UserId            int    `json:"user_id" gorm:"index;index:idx_user_id_id,priority:1"`
	CreatedAt         int64  `json:"created_at" gorm:"bigint;index:idx_created_at_id,priority:1;index:idx_created_at_type"`
	Type              int    `json:"type" gorm:"index:idx_created_at_type"`
	Content           string `json:"content"`
	Username          string `json:"username" gorm:"index;index:index_username_model_name,priority:2;default:''"`
	TokenName         string `json:"token_name" gorm:"index;default:''"`
	ModelName         string `json:"model_name" gorm:"index;index:index_username_model_name,priority:1;default:''"`
	Quota             int    `json:"quota" gorm:"default:0"`
	PromptTokens      int    `json:"prompt_tokens" gorm:"default:0"`
	CompletionTokens  int    `json:"completion_tokens" gorm:"default:0"`
	UseTime           int    `json:"use_time" gorm:"default:0"`
	IsStream          bool   `json:"is_stream"`
	ChannelId         int    `json:"channel" gorm:"index"`
	ChannelName       string `json:"channel_name" gorm:"->"`
	TokenId           int    `json:"token_id" gorm:"default:0;index"`
	Group             string `json:"group" gorm:"index"`
	Ip                string `json:"ip" gorm:"index;default:''"`
	RequestId         string `json:"request_id,omitempty" gorm:"type:varchar(64);index:idx_logs_request_id;default:''"`
	UpstreamRequestId string `json:"upstream_request_id,omitempty" gorm:"type:varchar(128);index:idx_logs_upstream_request_id;default:''"`
	Other             string `json:"other"`
}

func (b2LegacyLog) TableName() string {
	return "logs"
}

func b2SubmissionDatabaseDialector(engine, dsn string) (gorm.Dialector, error) {
	unsafeTarget := errors.New("B2 submission tests require a literal loopback address and database " + b2SubmissionFixtureDatabase)
	switch engine {
	case "mysql":
		config, err := mysql.ParseDSN(dsn)
		if err != nil {
			return nil, errors.New("invalid B2 MySQL fixture DSN")
		}
		host, _, err := net.SplitHostPort(config.Addr)
		if err != nil || config.Net != "tcp" || !net.ParseIP(host).IsLoopback() || config.DBName != b2SubmissionFixtureDatabase {
			return nil, unsafeTarget
		}
		return gormmysql.New(gormmysql.Config{DSN: config.FormatDSN()}), nil
	case "postgres":
		config, err := pgx.ParseConfig(dsn)
		if err != nil {
			return nil, errors.New("invalid B2 PostgreSQL fixture DSN")
		}
		if !net.ParseIP(config.Host).IsLoopback() || config.Database != b2SubmissionFixtureDatabase {
			return nil, unsafeTarget
		}
		for _, fallback := range config.Fallbacks {
			if !net.ParseIP(fallback.Host).IsLoopback() {
				return nil, unsafeTarget
			}
		}
		config.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
		return postgres.New(postgres.Config{Conn: stdlib.OpenDB(*config)}), nil
	default:
		return nil, errors.New("unsupported B2 database engine")
	}
}

func TestB2SubmissionDatabaseTargetSafety(t *testing.T) {
	for _, fixture := range []struct{ name, engine, dsn string }{
		{"mysql-remote", "mysql", "fixture:fixture@tcp(192.0.2.1:3306)/myapi_b2_test"},
		{"mysql-wrong-database", "mysql", "fixture:fixture@tcp(127.0.0.1:3306)/myapi_s2a_test"},
		{"mysql-socket", "mysql", "fixture:fixture@unix(/tmp/mysql.sock)/myapi_b2_test"},
		{"postgres-remote", "postgres", "postgres://fixture:fixture@192.0.2.1:5432/myapi_b2_test?sslmode=disable"},
		{"postgres-wrong-database", "postgres", "postgres://fixture:fixture@127.0.0.1:5432/myapi_s2a_test?sslmode=disable"},
		{"postgres-remote-fallback", "postgres", "host=127.0.0.1,192.0.2.1 port=5432 dbname=myapi_b2_test user=fixture password=fixture sslmode=disable"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			_, err := b2SubmissionDatabaseDialector(fixture.engine, fixture.dsn)
			require.Error(t, err)
		})
	}
}

func TestB2SubmissionConfiguredDatabases(t *testing.T) {
	if os.Getenv("MYAPI_B2_DATABASE_TESTS") != "1" {
		t.Skip("disposable B2 database tests require MYAPI_B2_DATABASE_TESTS=1")
	}
	for _, engine := range []struct{ name, env string }{
		{"mysql", "MYAPI_B2_MYSQL_DSN"},
		{"postgres", "MYAPI_B2_POSTGRES_DSN"},
	} {
		t.Run(engine.name, func(t *testing.T) {
			dsn := os.Getenv(engine.env)
			require.NotEmpty(t, dsn, "%s must be configured when MYAPI_B2_DATABASE_TESTS=1", engine.env)
			dialector, err := b2SubmissionDatabaseDialector(engine.name, dsn)
			require.NoError(t, err)
			db, err := gorm.Open(dialector, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			require.NoError(t, err)
			require.NoError(t, registerTaskRecoveryGormGuards(db))
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })

			tables, err := db.Migrator().GetTables()
			require.NoError(t, err)
			require.Empty(t, tables, "refusing a non-empty B2 fixture database; CI owns its lifecycle and no tables are dropped")
			if engine.name == "mysql" {
				// The literal target has passed both loopback and empty-schema
				// checks. utf8mb4 exercises the widest supported index encoding.
				require.NoError(t, db.Exec("ALTER DATABASE `myapi_b2_test` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci").Error)
			}
			previousDB, previousLogDB := DB, LOG_DB
			previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
			DB, LOG_DB = db, db
			if engine.name == "mysql" {
				common.SetDatabaseTypes(common.DatabaseTypeMySQL, common.DatabaseTypeMySQL)
			} else {
				common.SetDatabaseTypes(common.DatabaseTypePostgreSQL, common.DatabaseTypePostgreSQL)
			}
			initCol()
			t.Cleanup(func() {
				DB, LOG_DB = previousDB, previousLogDB
				common.SetDatabaseTypes(previousMainType, previousLogType)
				initCol()
			})
			legacy := createB2LegacyLogsFixture(t, db)
			createTaskQuotaLegacyFixture(t, db)
			require.ErrorIs(t, migrateDB(), ErrLogProjectionMaintenanceRequired)
			assertTaskQuotaLegacyFixtureMigrated(t, db)
			assertB2RecoveryMainSchema(t, db)
			assertLegacyLogsStartupRequiresMaintenance(t, db, legacy)
			require.ErrorIs(t, migrateDBFast(), ErrLogProjectionMaintenanceRequired)
			assertTaskQuotaLegacyFixtureMigrated(t, db)
			assertB2RecoveryMainSchema(t, db)
			assertLegacyLogsStartupRequiresMaintenance(t, db, legacy)
			require.ErrorIs(t, migrateLOGDB(), ErrLogProjectionMaintenanceRequired)
			assertLegacyLogsStartupRequiresMaintenance(t, db, legacy)
			// The configured database fixture then models the explicit operator
			// maintenance step before exercising online index creation and backfill.
			require.NoError(t, EnsureLogProjectionSchemaWithDB(db))
			assertB2LegacyLogsMigrated(t, db, legacy)
			var mysqlLockWait, mysqlInnoDBLockWait int64
			var postgresLockTimeout string
			if engine.name == "mysql" {
				require.NoError(t, db.Raw("SELECT @@SESSION.lock_wait_timeout").Scan(&mysqlLockWait).Error)
				require.NoError(t, db.Raw("SELECT @@SESSION.innodb_lock_wait_timeout").Scan(&mysqlInnoDBLockWait).Error)
			} else {
				require.NoError(t, db.Raw("SHOW lock_timeout").Scan(&postgresLockTimeout).Error)
			}
			runLogProjectionBackfillDatabaseContract(t, db)
			if engine.name == "mysql" {
				var restoredLockWait, restoredInnoDBLockWait int64
				require.NoError(t, db.Raw("SELECT @@SESSION.lock_wait_timeout").Scan(&restoredLockWait).Error)
				require.NoError(t, db.Raw("SELECT @@SESSION.innodb_lock_wait_timeout").Scan(&restoredInnoDBLockWait).Error)
				assert.Equal(t, mysqlLockWait, restoredLockWait)
				assert.Equal(t, mysqlInnoDBLockWait, restoredInnoDBLockWait)
			} else {
				var restoredLockTimeout string
				require.NoError(t, db.Raw("SHOW lock_timeout").Scan(&restoredLockTimeout).Error)
				assert.Equal(t, postgresLockTimeout, restoredLockTimeout)
			}
			runLogDedupDatabaseContract(t, db)
			sqlDB.SetMaxOpenConns(2)
			runB2TaskRecoveryIdentityConcurrentContract(t, db)
			sqlDB.SetMaxOpenConns(1)
			runB2SubmissionDatabaseContract(t, db)
			runB2TaskOperationQueryDatabaseContract(t, db)
			t.Run("quota-reservation", func(t *testing.T) { runTaskQuotaReservationContract(t, db) })
			t.Run("quota-commit-acknowledgement", func(t *testing.T) { runTaskQuotaCommitAcknowledgementContract(t, db) })
			t.Run("business-credit-receipts", func(t *testing.T) { runBusinessCreditConfiguredDatabaseContract(t, db) })
		})
	}
}

func runBusinessCreditConfiguredDatabaseContract(t *testing.T, db *gorm.DB) {
	t.Helper()
	previousQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 100
	t.Cleanup(func() { common.QuotaPerUnit = previousQuotaPerUnit })
	configureFixedCheckinAward(t, 25)
	setQuotaWriterStateForTest(t, db, QuotaWriterModeAuthoritative, 901)
	var fundingState UserFundingStateSnapshot
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		var err error
		fundingState, err = InitializeUserFundingStateTx(tx, operation_setting.UserFundingModeEnabled)
		return err
	}))
	require.NoError(t, PublishUserFundingState(fundingState))

	users := []User{
		{Id: 910001, Username: "b2-credit-topup", AffCode: "b2-credit-topup-aff", Status: common.UserStatusEnabled},
		{Id: 910002, Username: "b2-credit-rollback", AffCode: "b2-credit-rollback-aff", Status: common.UserStatusEnabled},
		{Id: 910003, Username: "b2-credit-redeem", AffCode: "b2-credit-redeem-aff", Status: common.UserStatusEnabled},
		{Id: 910004, Username: "b2-credit-other", AffCode: "b2-credit-other-aff", Status: common.UserStatusEnabled},
		{Id: 910005, Username: "b2-credit-checkin", AffCode: "b2-credit-checkin-aff", Status: common.UserStatusEnabled},
	}
	for index := range users {
		require.NoError(t, db.Create(&users[index]).Error)
	}
	order := TopUp{UserId: users[0].Id, Amount: 2, Money: 2, TradeNo: "b2-credit-topup-order", PaymentMethod: "alipay", PaymentProvider: PaymentProviderEpay, Status: common.TopUpStatusPending}
	rollbackOrder := TopUp{UserId: users[1].Id, Amount: 2, Money: 2, TradeNo: "b2-credit-rollback-order", PaymentMethod: "alipay", PaymentProvider: PaymentProviderEpay, Status: common.TopUpStatusPending}
	require.NoError(t, db.Create(&order).Error)
	require.NoError(t, db.Create(&rollbackOrder).Error)
	_, err := RechargeEpayTrusted(order.TradeNo, "alipay", "127.0.0.1")
	require.NoError(t, err)
	alreadyDone, err := RechargeEpayTrusted(order.TradeNo, "alipay", "127.0.0.1")
	require.NoError(t, err)
	assert.True(t, alreadyDone)
	var topUpReceipt UserQuotaMutationReceipt
	eventKey, err := topUpBusinessEventKey(&order)
	require.NoError(t, err)
	require.NoError(t, db.Where("business_event_key = ?", eventKey).First(&topUpReceipt).Error)
	assert.EqualValues(t, 200, topUpReceipt.Delta)

	const callbackName = "test:configured_business_credit_receipt_failure"
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if receipt, ok := tx.Statement.Dest.(*UserQuotaMutationReceipt); ok && receipt.UserID == users[1].Id {
			tx.AddError(errors.New("configured receipt failure"))
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Create().Remove(callbackName) })
	_, err = RechargeEpayTrusted(rollbackOrder.TradeNo, "alipay", "127.0.0.1")
	require.Error(t, err)
	require.NoError(t, db.Callback().Create().Remove(callbackName))
	var storedRollback TopUp
	require.NoError(t, db.Where("id = ?", rollbackOrder.Id).First(&storedRollback).Error)
	assert.Equal(t, common.TopUpStatusPending, storedRollback.Status)
	var rollbackUser User
	require.NoError(t, db.First(&rollbackUser, users[1].Id).Error)
	assert.Zero(t, rollbackUser.Quota)

	redemption := Redemption{Name: "b2-credit-redemption", Key: "80000000000000000000000000000001", Status: common.RedemptionCodeStatusEnabled, Quota: 30}
	require.NoError(t, db.Create(&redemption).Error)
	_, err = Redeem(redemption.Key, users[2].Id)
	require.NoError(t, err)
	_, err = Redeem(redemption.Key, users[2].Id)
	require.NoError(t, err)
	_, err = Redeem(redemption.Key, users[3].Id)
	require.ErrorIs(t, err, ErrUserQuotaMutationConflict)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(4)
	firstCheckin, err := UserCheckin(users[4].Id)
	require.NoError(t, err)
	replayedCheckin, err := UserCheckin(users[4].Id)
	require.NoError(t, err)
	assert.Equal(t, firstCheckin.Id, replayedCheckin.Id)
	const checkinWorkers = 4
	var wait sync.WaitGroup
	errorsByWorker := make(chan error, checkinWorkers)
	wait.Add(checkinWorkers)
	for range checkinWorkers {
		go func() {
			defer wait.Done()
			_, checkinErr := UserCheckin(users[4].Id)
			errorsByWorker <- checkinErr
		}()
	}
	wait.Wait()
	close(errorsByWorker)
	successes := 0
	for checkinErr := range errorsByWorker {
		if checkinErr == nil {
			successes++
		}
	}
	assert.Equal(t, checkinWorkers, successes)
	var checkinCount, checkinReceiptCount int64
	require.NoError(t, db.Model(&Checkin{}).Where("user_id = ?", users[4].Id).Count(&checkinCount).Error)
	require.NoError(t, db.Model(&UserQuotaMutationReceipt{}).Where("business_event_key = ?", "checkin:910005:"+time.Now().Format("2006-01-02")).Count(&checkinReceiptCount).Error)
	assert.EqualValues(t, 1, checkinCount)
	assert.EqualValues(t, 1, checkinReceiptCount)
	var checkinUser User
	require.NoError(t, db.First(&checkinUser, users[4].Id).Error)
	assert.Equal(t, 25, checkinUser.Quota)
	sqlDB.SetMaxOpenConns(1)
}

// createB2LegacyLogsFixture builds the oldest supported B2 log shape before
// any current-schema tables are created. The caller chooses the production
// migration entry point, which keeps normal, fast, and LOG_DB paths testable.
func createB2LegacyLogsFixture(t *testing.T, db *gorm.DB) b2LegacyLog {
	t.Helper()
	require.NoError(t, db.AutoMigrate(&b2LegacyLog{}))
	legacy := b2LegacyLog{
		UserId:            17,
		CreatedAt:         1_700_000_000,
		Type:              LogTypeConsume,
		Content:           "legacy-b2-log",
		Username:          "legacy-user",
		TokenName:         "legacy-token",
		ModelName:         "legacy-model",
		Quota:             -123,
		PromptTokens:      45,
		CompletionTokens:  67,
		UseTime:           89,
		IsStream:          true,
		ChannelId:         23,
		TokenId:           29,
		Group:             "legacy-group",
		Ip:                "127.0.0.1",
		RequestId:         "legacy-request",
		UpstreamRequestId: "legacy-upstream-request",
		Other:             `{"legacy":true}`,
	}
	require.NoError(t, db.Create(&legacy).Error)
	require.NotZero(t, legacy.Id)
	return legacy
}

func assertB2LegacyLogsMigrated(t *testing.T, db *gorm.DB, legacy b2LegacyLog) {
	t.Helper()
	for _, column := range []string{"BillingEventID", "BillingProjectionDigest", "LogRowKey"} {
		require.True(t, db.Migrator().HasColumn(&Log{}, column), "missing column %s", column)
	}
	for _, index := range []string{
		"idx_logs_request_id",
		"idx_logs_upstream_request_id",
	} {
		require.True(t, db.Migrator().HasIndex(&Log{}, index), "missing index %s", index)
	}

	var historical Log
	require.NoError(t, db.First(&historical, legacy.Id).Error)
	assert.Equal(t, legacy.Id, historical.Id)
	assert.Equal(t, legacy.UserId, historical.UserId)
	assert.Equal(t, legacy.CreatedAt, historical.CreatedAt)
	assert.Equal(t, legacy.Type, historical.Type)
	assert.Equal(t, legacy.Content, historical.Content)
	assert.Equal(t, legacy.Username, historical.Username)
	assert.Equal(t, legacy.TokenName, historical.TokenName)
	assert.Equal(t, legacy.ModelName, historical.ModelName)
	assert.Equal(t, legacy.Quota, historical.Quota)
	assert.Equal(t, legacy.PromptTokens, historical.PromptTokens)
	assert.Equal(t, legacy.CompletionTokens, historical.CompletionTokens)
	assert.Equal(t, legacy.UseTime, historical.UseTime)
	assert.Equal(t, legacy.IsStream, historical.IsStream)
	assert.Equal(t, legacy.ChannelId, historical.ChannelId)
	assert.Equal(t, legacy.TokenId, historical.TokenId)
	assert.Equal(t, legacy.Group, historical.Group)
	assert.Equal(t, legacy.Ip, historical.Ip)
	assert.Equal(t, legacy.RequestId, historical.RequestId)
	assert.Equal(t, legacy.UpstreamRequestId, historical.UpstreamRequestId)
	assert.Equal(t, legacy.Other, historical.Other)
	assert.Empty(t, historical.BillingEventID)
	assert.Empty(t, historical.BillingProjectionDigest)
	assert.Empty(t, historical.LogRowKey)
}

// migrateB2LegacyLogsFixture exercises the production log migration against
// the oldest supported B2 fixture shape before any current-schema tables are
// created. The fixture database was asserted empty above and is never dropped.
func migrateB2LegacyLogsFixture(t *testing.T, db *gorm.DB) {
	t.Helper()
	legacy := createB2LegacyLogsFixture(t, db)
	require.NoError(t, db.AutoMigrate(&Log{}))
	require.NoError(t, EnsureLogProjectionSchemaWithDB(db))
	assertB2LegacyLogsMigrated(t, db, legacy)

	// Startup migrations must remain safe to repeat after an upgrade.
	require.NoError(t, db.AutoMigrate(&Log{}))
	require.NoError(t, EnsureLogProjectionSchemaWithDB(db))
	require.True(t, db.Migrator().HasTable(&Log{}))
	require.False(t, db.Migrator().HasIndex(&Log{}, "idx_logs_billing_canonical"))

	current := Log{Content: "current-b2-log"}
	require.NoError(t, CreateLog(db, &current))
	require.NotZero(t, current.Id)
	assert.NotEqual(t, legacy.Id, current.Id)
}
