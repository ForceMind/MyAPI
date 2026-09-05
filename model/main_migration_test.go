package model

import (
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestEnsureSubscriptionPlanTableSQLiteAddsRequiredColumnsToExistingRows(t *testing.T) {
	previousDB := DB
	previousType := common.MainDatabaseType()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		DB = previousDB
		common.SetMainDatabaseType(previousType)
	})

	// An older installation can have rows while missing newly introduced
	// required columns. SQLite only permits adding NOT NULL columns when a
	// default is supplied.
	require.NoError(t, db.Exec("CREATE TABLE subscription_plans (id INTEGER PRIMARY KEY)").Error)
	require.NoError(t, db.Exec("INSERT INTO subscription_plans (id) VALUES (1)").Error)
	require.NoError(t, ensureSubscriptionPlanTableSQLite())

	var plan SubscriptionPlan
	require.NoError(t, db.First(&plan, 1).Error)
	require.Empty(t, plan.Title)
	require.Zero(t, plan.PriceAmount)
	require.NoError(t, ensureSubscriptionPlanTableSQLite())
	var count int64
	require.NoError(t, db.Model(&SubscriptionPlan{}).Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestMigrationTypeConversionsUseHandleDialect(t *testing.T) {
	previousDB := DB
	previousType := common.MainDatabaseType()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	// Deliberately disagree with the SQLite handle. The migration helpers must
	// never use this process-wide value to select dialect-specific SQL.
	common.SetMainDatabaseType(common.DatabaseTypeMySQL)
	t.Cleanup(func() {
		DB = previousDB
		common.SetMainDatabaseType(previousType)
	})

	require.NoError(t, db.Exec("CREATE TABLE subscription_plans (id INTEGER PRIMARY KEY, price_amount REAL)").Error)
	require.NoError(t, db.Exec("CREATE TABLE tokens (id INTEGER PRIMARY KEY, model_limits VARCHAR(1024))").Error)
	require.NoError(t, migrateSubscriptionPlanPriceAmount())
	require.NoError(t, migrateTokenModelLimitsToText())
}

func useB2MainMigrationSQLite(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	previousDB, previousLogDB := DB, LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	DB, LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	initCol()
	t.Cleanup(func() {
		DB, LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
		initCol()
		require.NoError(t, sqlDB.Close())
	})
	return db
}

func assertB2RecoveryMainSchema(t *testing.T, db *gorm.DB) {
	t.Helper()
	for _, model := range []interface{}{
		&TaskRecoveryIdentity{},
		&TaskSubmissionOperation{},
		&TaskSubmissionAttempt{},
		&TaskBillingEvent{},
		&TaskBillingLogOutbox{},
	} {
		assert.True(t, db.Migrator().HasTable(model))
	}
	for _, index := range []struct {
		model interface{}
		name  string
	}{
		{&TaskSubmissionOperation{}, "uidx_task_submission_public_id"},
		{&TaskSubmissionOperation{}, "uidx_task_submission_idempotency"},
		{&TaskSubmissionAttempt{}, "uidx_task_submission_attempt"},
		{&TaskBillingEvent{}, "uidx_task_billing_event_id"},
		{&TaskBillingEvent{}, "uidx_task_billing_event_key"},
		{&TaskBillingLogOutbox{}, "uidx_task_billing_outbox_event"},
	} {
		assert.True(t, db.Migrator().HasIndex(index.model, index.name), "missing index %s", index.name)
	}
}

func TestB2RecoveryMainMigrationEntriesPreserveLegacyLogsSQLite(t *testing.T) {
	for _, entry := range []struct {
		name string
		run  func() error
	}{
		{"normal", migrateDB},
		{"fast", migrateDBFast},
	} {
		t.Run(entry.name, func(t *testing.T) {
			db := useB2MainMigrationSQLite(t)
			legacy := createB2LegacyLogsFixture(t, db)
			require.NoError(t, entry.run())
			assertB2RecoveryMainSchema(t, db)
			assertB2LegacyLogsMigrated(t, db, legacy)
			require.NoError(t, entry.run())
			assertB2RecoveryMainSchema(t, db)
			assertB2LegacyLogsMigrated(t, db, legacy)
		})
	}
}

func TestB2RecoveryLogMigrationEntryPreservesLegacyLogsSQLite(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	previousLogDB, previousLogType := LOG_DB, common.LogDatabaseType()
	LOG_DB = db
	common.SetLogDatabaseType(common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		LOG_DB = previousLogDB
		common.SetLogDatabaseType(previousLogType)
		require.NoError(t, sqlDB.Close())
	})
	legacy := createB2LegacyLogsFixture(t, db)
	require.NoError(t, migrateLOGDB())
	assertB2LegacyLogsMigrated(t, db, legacy)
	require.NoError(t, migrateLOGDB())
	assertB2LegacyLogsMigrated(t, db, legacy)
}
