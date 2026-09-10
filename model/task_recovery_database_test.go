package model

import (
	"errors"
	"net"
	"os"
	"testing"

	"github.com/ForceMind/MyAPI/common"
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
			require.NoError(t, migrateDB())
			assertTaskQuotaLegacyFixtureMigrated(t, db)
			assertB2RecoveryMainSchema(t, db)
			assertB2LegacyLogsMigrated(t, db, legacy)
			require.NoError(t, migrateDBFast())
			assertTaskQuotaLegacyFixtureMigrated(t, db)
			assertB2RecoveryMainSchema(t, db)
			assertB2LegacyLogsMigrated(t, db, legacy)
			require.NoError(t, migrateLOGDB())
			assertB2LegacyLogsMigrated(t, db, legacy)
			sqlDB.SetMaxOpenConns(2)
			runB2TaskRecoveryIdentityConcurrentContract(t, db)
			sqlDB.SetMaxOpenConns(1)
			runB2SubmissionDatabaseContract(t, db)
			runB2TaskOperationQueryDatabaseContract(t, db)
			t.Run("quota-reservation", func(t *testing.T) { runTaskQuotaReservationContract(t, db) })
			t.Run("quota-commit-acknowledgement", func(t *testing.T) { runTaskQuotaCommitAcknowledgementContract(t, db) })
		})
	}
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
	require.True(t, db.Migrator().HasColumn(&Log{}, "BillingEventID"))
	for _, index := range []string{
		"idx_logs_request_id",
		"idx_logs_upstream_request_id",
		"idx_logs_billing_event_id",
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
}

// migrateB2LegacyLogsFixture exercises the production log migration against
// the oldest supported B2 fixture shape before any current-schema tables are
// created. The fixture database was asserted empty above and is never dropped.
func migrateB2LegacyLogsFixture(t *testing.T, db *gorm.DB) {
	t.Helper()
	legacy := createB2LegacyLogsFixture(t, db)
	require.NoError(t, db.AutoMigrate(&Log{}))
	assertB2LegacyLogsMigrated(t, db, legacy)

	// Startup migrations must remain safe to repeat after an upgrade.
	require.NoError(t, db.AutoMigrate(&Log{}))
	require.True(t, db.Migrator().HasTable(&Log{}))
	require.True(t, db.Migrator().HasIndex(&Log{}, "idx_logs_billing_event_id"))

	current := Log{Content: "current-b2-log"}
	require.NoError(t, db.Create(&current).Error)
	require.NotZero(t, current.Id)
	assert.NotEqual(t, legacy.Id, current.Id)
}
