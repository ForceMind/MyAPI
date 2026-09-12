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
		&LogProjectionBackfillState{},
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

func assertLegacyLogsStartupRequiresMaintenance(t *testing.T, db *gorm.DB, legacy b2LegacyLog) {
	t.Helper()
	for _, column := range []string{"BillingEventID", "BillingProjectionDigest", "LogRowKey"} {
		assert.False(t, db.Migrator().HasColumn(&Log{}, column), "startup must not add %s to a non-empty logs table", column)
	}
	var historical b2LegacyLog
	require.NoError(t, db.First(&historical, legacy.Id).Error)
	assert.Equal(t, legacy, historical)
	state, err := GetLogProjectionBackfillState(t.Context(), db)
	require.NoError(t, err)
	require.NotNil(t, state)
	assert.Equal(t, LogProjectionBackfillStatusMaintenanceRequired, state.Status)
	assert.Contains(t, state.LastError, "existing non-empty logs table")
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
			require.ErrorIs(t, entry.run(), ErrLogProjectionMaintenanceRequired)
			assertB2RecoveryMainSchema(t, db)
			assertLegacyLogsStartupRequiresMaintenance(t, db, legacy)
			require.ErrorIs(t, entry.run(), ErrLogProjectionMaintenanceRequired)
			assertB2RecoveryMainSchema(t, db)
			assertLegacyLogsStartupRequiresMaintenance(t, db, legacy)
		})
	}
}

func TestB2RecoveryLogMigrationEntryPreservesLegacyLogsSQLite(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	previousDB, previousLogDB, previousLogType := DB, LOG_DB, common.LogDatabaseType()
	DB, LOG_DB = db, db
	common.SetLogDatabaseType(common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		DB, LOG_DB = previousDB, previousLogDB
		common.SetLogDatabaseType(previousLogType)
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.AutoMigrate(&LogProjectionBackfillState{}))
	legacy := createB2LegacyLogsFixture(t, db)
	require.ErrorIs(t, migrateLOGDB(), ErrLogProjectionMaintenanceRequired)
	assertLegacyLogsStartupRequiresMaintenance(t, db, legacy)
	require.ErrorIs(t, migrateLOGDB(), ErrLogProjectionMaintenanceRequired)
	assertLegacyLogsStartupRequiresMaintenance(t, db, legacy)
}

func TestSeparateLogDatabaseLeavesMainLogsSchemaUntouched(t *testing.T) {
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
			t.Setenv("LOG_SQL_DSN", "configured-separate-log-database")
			require.NoError(t, entry.run())
			for _, column := range []string{"BillingEventID", "BillingProjectionDigest", "LogRowKey"} {
				assert.False(t, db.Migrator().HasColumn(&Log{}, column), "main database logs must remain untouched when LOG_SQL_DSN is independent")
			}
			var historical b2LegacyLog
			require.NoError(t, db.First(&historical, legacy.Id).Error)
			assert.Equal(t, legacy, historical)
		})
	}
}

func TestNonMasterLogGuardRejectsDirectCreateAndAllowsCreateLog(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&Log{}))
	require.NoError(t, EnsureLogProjectionSchemaWithDB(db))

	previousMaster := common.IsMasterNode
	common.IsMasterNode = false
	t.Cleanup(func() { common.IsMasterNode = previousMaster })
	require.NoError(t, registerLogCreateGuard(db))

	direct := &Log{Content: "direct"}
	require.ErrorContains(t, db.Create(direct).Error, "CreateLog or CreateLogs")
	controlled := &Log{Content: "controlled"}
	require.NoError(t, CreateLog(db, controlled))
	assert.NotEmpty(t, controlled.LogRowKey)
}

func TestNonMasterIndependentLogDatabaseGuardIsRegistered(t *testing.T) {
	mainDB, err := gorm.Open(sqlite.Open("file:"+t.Name()+"_main?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	logDB, err := gorm.Open(sqlite.Open("file:"+t.Name()+"_log?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	mainSQL, err := mainDB.DB()
	require.NoError(t, err)
	logSQL, err := logDB.DB()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, mainSQL.Close())
		require.NoError(t, logSQL.Close())
	})
	require.NoError(t, logDB.AutoMigrate(&Log{}))
	require.NoError(t, EnsureLogProjectionSchemaWithDB(logDB))

	previousMaster := common.IsMasterNode
	common.IsMasterNode = false
	t.Cleanup(func() { common.IsMasterNode = previousMaster })
	require.NoError(t, registerLogCreateGuard(mainDB))
	require.NoError(t, registerLogCreateGuard(logDB))

	require.ErrorContains(t, logDB.Create(&Log{Content: "direct-independent"}).Error, "CreateLog or CreateLogs")
	require.NoError(t, CreateLog(logDB, &Log{Content: "controlled-independent"}))
	assert.False(t, mainDB.Migrator().HasTable(&Log{}), "independent LOG_DB initialization must not create main-database logs")
}

func TestEnsureLogProjectionSchemaAddsAllColumnsToEmptyLegacySQLiteTable(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.Exec("CREATE TABLE logs (id INTEGER PRIMARY KEY, content TEXT)").Error)
	require.NoError(t, EnsureLogProjectionSchemaWithDB(db))
	for _, column := range []string{"BillingEventID", "BillingProjectionDigest", "LogRowKey"} {
		assert.True(t, db.Migrator().HasColumn(&Log{}, column), "missing column %s", column)
	}
}

func TestNonMasterStartupValidatesSharedAndIndependentLegacyLogSchemaReadOnly(t *testing.T) {
	for _, mode := range []string{"shared", "independent"} {
		t.Run(mode, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
			require.NoError(t, db.Exec("CREATE TABLE logs (id INTEGER PRIMARY KEY, content TEXT)").Error)

			err = ValidateLogProjectionSchemaWithDB(db)
			require.ErrorIs(t, err, ErrLogProjectionMaintenanceRequired)
			assert.Contains(t, err.Error(), "billing_event_id")
			assert.Contains(t, err.Error(), clickHouseIdentityTable)
			for _, column := range []string{"BillingEventID", "BillingProjectionDigest", "LogRowKey"} {
				assert.False(t, db.Migrator().HasColumn(&Log{}, column), "non-master validation must not add %s", column)
			}
			assert.False(t, db.Migrator().HasTable(&BillingLogProjectionIdentity{}), "non-master validation must not create identity state")
		})
	}
}
