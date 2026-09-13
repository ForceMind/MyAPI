package model

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/clickhouse"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var commonGroupCol string
var commonKeyCol string
var commonTrueVal string
var commonFalseVal string

var logKeyCol string
var logGroupCol string

func initCol() {
	// init common column names
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		commonGroupCol = `"group"`
		commonKeyCol = `"key"`
		commonTrueVal = "true"
		commonFalseVal = "false"
	} else {
		commonGroupCol = "`group`"
		commonKeyCol = "`key`"
		commonTrueVal = "1"
		commonFalseVal = "0"
	}
	switch common.LogDatabaseType() {
	case common.DatabaseTypePostgreSQL:
		logGroupCol = `"group"`
		logKeyCol = `"key"`
	default:
		logGroupCol = "`group`"
		logKeyCol = "`key`"
	}
}

var DB *gorm.DB

var LOG_DB *gorm.DB

func createRootAccountIfNeed() error {
	var user User
	//if user.Status != common.UserStatusEnabled {
	if err := DB.First(&user).Error; err != nil {
		common.SysLog("no user exists, create a root user for you: username is root, password is 123456")
		hashedPassword, err := common.Password2Hash("123456")
		if err != nil {
			return err
		}
		rootUser := User{
			Username:    "root",
			Password:    hashedPassword,
			Role:        common.RoleRootUser,
			Status:      common.UserStatusEnabled,
			DisplayName: "Root User",
			AccessToken: nil,
			Quota:       100000000,
		}
		DB.Create(&rootUser)
	}
	return nil
}

func CheckSetup() {
	setup := GetSetup()
	if setup == nil {
		// No setup record exists, check if we have a root user
		if RootUserExists() {
			common.SysLog("system is not initialized, but root user exists")
			// Create setup record
			newSetup := Setup{
				Version:       common.Version,
				InitializedAt: time.Now().Unix(),
			}
			err := DB.Create(&newSetup).Error
			if err != nil {
				common.SysLog("failed to create setup record: " + err.Error())
			}
			constant.Setup = true
		} else {
			common.SysLog("system is not initialized and no root user exists")
			constant.Setup = false
		}
	} else {
		// Setup record exists, system is initialized
		common.SysLog("system is already initialized at: " + time.Unix(setup.InitializedAt, 0).String())
		constant.Setup = true
	}
}

func isClickHouseDSN(dsn string) bool {
	return strings.HasPrefix(dsn, "clickhouse://") ||
		strings.HasPrefix(dsn, "tcp://") ||
		strings.HasPrefix(dsn, "http://") ||
		strings.HasPrefix(dsn, "https://")
}

func normalizeClickHouseDSN(dsn string) string {
	parsed, err := url.Parse(dsn)
	if err != nil || parsed.Scheme != "https" {
		return dsn
	}
	query := parsed.Query()
	if _, ok := query["secure"]; !ok {
		query.Set("secure", "true")
		parsed.RawQuery = query.Encode()
	}
	return parsed.String()
}

func chooseDB(envName string, isLog bool) (*gorm.DB, common.DatabaseType, error) {
	dsn := os.Getenv(envName)
	if dsn != "" {
		if isClickHouseDSN(dsn) {
			if !isLog {
				return nil, "", fmt.Errorf("%s does not support ClickHouse; use SQLite, MySQL, or PostgreSQL for the primary database and LOG_SQL_DSN for ClickHouse logs", envName)
			}
			common.SysLog("using ClickHouse as log database")
			db, err := gorm.Open(clickhouse.Open(normalizeClickHouseDSN(dsn)), newGormConfig(false))
			return db, common.DatabaseTypeClickHouse, err
		}
		if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
			// Use PostgreSQL
			common.SysLog("using PostgreSQL as database")
			db, err := gorm.Open(postgres.New(postgres.Config{
				DSN:                  dsn,
				PreferSimpleProtocol: true, // disables implicit prepared statement usage
			}), newGormConfig(true))
			return db, common.DatabaseTypePostgreSQL, err
		}
		if strings.HasPrefix(dsn, "local") {
			common.SysLog("SQL_DSN not set, using SQLite as database")
			db, err := gorm.Open(sqlite.Open(common.SQLitePath), newGormConfig(true))
			return db, common.DatabaseTypeSQLite, err
		}
		// Use MySQL
		common.SysLog("using MySQL as database")
		// check parseTime
		if !strings.Contains(dsn, "parseTime") {
			if strings.Contains(dsn, "?") {
				dsn += "&parseTime=true"
			} else {
				dsn += "?parseTime=true"
			}
		}
		db, err := gorm.Open(mysql.Open(dsn), newGormConfig(true))
		return db, common.DatabaseTypeMySQL, err
	}
	// Use SQLite
	common.SysLog("SQL_DSN not set, using SQLite as database")
	db, err := gorm.Open(sqlite.Open(common.SQLitePath), newGormConfig(true))
	return db, common.DatabaseTypeSQLite, err
}

func InitDB() (err error) {
	// Do not turn an explicitly requested recovery deployment into a legacy
	// writer when its dedicated key is missing or malformed. InitEnv keeps the
	// effective gate closed, but startup must still reject this configuration.
	if common.TaskRecoveryDeploymentRequested() {
		if _, err := common.TaskRecoveryIdempotencyKeyVerifier(); err != nil {
			return fmt.Errorf("task recovery deployment configuration is invalid: %w", err)
		}
	}
	db, dbType, err := chooseDB("SQL_DSN", false)
	if err == nil {
		common.SetMainDatabaseType(dbType)
		if os.Getenv("LOG_SQL_DSN") == "" {
			common.SetLogDatabaseType(dbType)
		}
		initCol()
		if common.DebugEnabled {
			db = db.Debug()
		}
		DB = db
		if err := registerLogCreateGuard(DB); err != nil {
			return fmt.Errorf("register main database log create guard: %w", err)
		}
		if err := registerTaskRecoveryGormGuards(DB); err != nil {
			return fmt.Errorf("register task recovery GORM guards: %w", err)
		}
		// MySQL charset/collation startup check: ensure Chinese-capable charset
		if common.UsingMainDatabase(common.DatabaseTypeMySQL) {
			if err := checkMySQLChineseSupport(DB); err != nil {
				panic(err)
			}
		}
		sqlDB, err := DB.DB()
		if err != nil {
			return err
		}
		sqlDB.SetMaxIdleConns(common.GetEnvOrDefault("SQL_MAX_IDLE_CONNS", 100))
		sqlDB.SetMaxOpenConns(common.GetEnvOrDefault("SQL_MAX_OPEN_CONNS", 1000))
		sqlDB.SetConnMaxLifetime(time.Second * time.Duration(common.GetEnvOrDefault("SQL_MAX_LIFETIME", 60)))

		if !common.IsMasterNode {
			if os.Getenv("LOG_SQL_DSN") == "" {
				if err := ValidateLogProjectionSchemaWithDB(DB); err != nil {
					return err
				}
			}
			if common.IsTaskRecoveryIdentityRequired() {
				if err := EnsureTaskRecoveryIdentity(DB); err != nil {
					return fmt.Errorf("task recovery database identity verification failed: %w", err)
				}
			}
			return nil
		}
		if common.UsingMainDatabase(common.DatabaseTypeMySQL) {
			//_, _ = sqlDB.Exec("ALTER TABLE channels MODIFY model_mapping TEXT;") // TODO: delete this line when most users have upgraded
		}
		common.SysLog("database migration started")
		if err := migrateDB(); err != nil {
			return err
		}
		if common.IsTaskRecoveryIdentityRequired() {
			if err := EnsureTaskRecoveryIdentity(DB); err != nil {
				return fmt.Errorf("task recovery database identity verification failed: %w", err)
			}
		}
		return nil
	} else {
		common.FatalLog(err)
	}
	return err
}

func InitLogDB() (err error) {
	if os.Getenv("LOG_SQL_DSN") == "" {
		LOG_DB = DB
		common.SetLogDatabaseType(common.MainDatabaseType())
		initCol()
		return registerLogCreateGuard(LOG_DB)
	}
	db, dbType, err := chooseDB("LOG_SQL_DSN", true)
	if err == nil {
		common.SetLogDatabaseType(dbType)
		initCol()
		if common.DebugEnabled {
			db = db.Debug()
		}
		LOG_DB = db
		if err := registerLogCreateGuard(LOG_DB); err != nil {
			return fmt.Errorf("register log database create guard: %w", err)
		}
		// If log DB is MySQL, also ensure Chinese-capable charset
		if common.UsingLogDatabase(common.DatabaseTypeMySQL) {
			if err := checkMySQLChineseSupport(LOG_DB); err != nil {
				panic(err)
			}
		}
		sqlDB, err := LOG_DB.DB()
		if err != nil {
			return err
		}
		sqlDB.SetMaxIdleConns(common.GetEnvOrDefault("SQL_MAX_IDLE_CONNS", 100))
		sqlDB.SetMaxOpenConns(common.GetEnvOrDefault("SQL_MAX_OPEN_CONNS", 1000))
		sqlDB.SetConnMaxLifetime(time.Second * time.Duration(common.GetEnvOrDefault("SQL_MAX_LIFETIME", 60)))

		if !common.IsMasterNode {
			return ValidateLogProjectionSchemaWithDB(LOG_DB)
		}
		common.SysLog("database migration started")
		err = migrateLOGDB()
		return err
	} else {
		common.FatalLog(err)
	}
	return err
}

func migrateDB() error {
	if err := prepareAccessProfileIdentifiers(); err != nil {
		return err
	}
	// Migrate price_amount column from float/double to decimal for existing tables
	if err := migrateSubscriptionPlanPriceAmount(); err != nil {
		return err
	}
	// Migrate model_limits column from varchar to text for existing tables
	if err := migrateTokenModelLimitsToText(); err != nil {
		return err
	}

	err := DB.AutoMigrate(
		&Channel{},
		&ChannelQuotaSnapshot{},
		&Token{},
		&User{},
		&UserSession{},
		&AuthFlow{},
		&ExternalIdentityClaim{},
		&PasskeyCredential{},
		&Option{},
		&Redemption{},
		&Ability{},
		&Midjourney{},
		&TopUp{},
		&QuotaData{},
		&Task{},
		&TaskRecoveryIdentity{},
		&TaskSubmissionOperation{},
		&TaskSubmissionAttempt{},
		&TaskTerminalObservation{},
		&TaskBillingEvent{},
		&TaskBillingLogOutbox{},
		&QuotaMutationReceipt{},
		&UserQuotaMutationReceipt{},
		&QuotaWriterEpoch{},
		&QuotaProjectionObligation{},
		&Model{},
		&Vendor{},
		&PrefillGroup{},
		&Setup{},
		&TwoFA{},
		&TwoFABackupCode{},
		&Checkin{},
		&SubscriptionOrder{},
		&UserSubscription{},
		&SubscriptionPreConsumeRecord{},
		&CustomOAuthProvider{},
		&UserOAuthBinding{},
		&PerfMetric{},
		&SystemInstance{},
		&SystemTask{},
		&SystemTaskLock{},
		&LogProjectionBackfillState{},
		&CasbinRule{},
		&AuthzRole{},
	)
	if err != nil {
		return err
	}
	if os.Getenv("LOG_SQL_DSN") == "" {
		if err := migrateRelationalLogDBStartup(DB); err != nil {
			return err
		}
	}
	if err := ensureUserNormalizedEmail(); err != nil {
		return err
	}
	if err := ensureChannelQuotaSnapshotDedupeIndex(); err != nil {
		return err
	}
	if err := MigrateAccessProfileIdentifiers(); err != nil {
		return err
	}
	if err := InitializeUserAuthVersions(); err != nil {
		return err
	}
	if err := InitializeExternalIdentityClaims(); err != nil {
		return err
	}
	if err := ensureQuotaMutationReceiptSchema(); err != nil {
		return err
	}
	if err := EnsureQuotaWriterEpochStateWithDB(DB); err != nil {
		return err
	}
	if err := InitializeQuotaProjectionObligationsWithDB(DB); err != nil {
		return err
	}
	if err := ensureTaskTerminalObservationSchemaWithDB(DB); err != nil {
		return err
	}
	if common.UsingMainDatabase(common.DatabaseTypeSQLite) {
		if err := ensureSubscriptionPlanTableSQLite(); err != nil {
			return err
		}
	} else {
		if err := DB.AutoMigrate(&SubscriptionPlan{}); err != nil {
			return err
		}
	}
	return nil
}

func migrateDBFast() error {
	if err := prepareAccessProfileIdentifiers(); err != nil {
		return err
	}

	migrations := []struct {
		model interface{}
		name  string
	}{
		{&Channel{}, "Channel"},
		{&ChannelQuotaSnapshot{}, "ChannelQuotaSnapshot"},
		{&Token{}, "Token"},
		{&User{}, "User"},
		{&UserSession{}, "UserSession"},
		{&AuthFlow{}, "AuthFlow"},
		{&ExternalIdentityClaim{}, "ExternalIdentityClaim"},
		{&PasskeyCredential{}, "PasskeyCredential"},
		{&Option{}, "Option"},
		{&Redemption{}, "Redemption"},
		{&Ability{}, "Ability"},
		{&Midjourney{}, "Midjourney"},
		{&TopUp{}, "TopUp"},
		{&QuotaData{}, "QuotaData"},
		{&Task{}, "Task"},
		{&TaskRecoveryIdentity{}, "TaskRecoveryIdentity"},
		{&TaskSubmissionOperation{}, "TaskSubmissionOperation"},
		{&TaskSubmissionAttempt{}, "TaskSubmissionAttempt"},
		{&TaskTerminalObservation{}, "TaskTerminalObservation"},
		{&TaskBillingEvent{}, "TaskBillingEvent"},
		{&TaskBillingLogOutbox{}, "TaskBillingLogOutbox"},
		{&QuotaMutationReceipt{}, "QuotaMutationReceipt"},
		{&UserQuotaMutationReceipt{}, "UserQuotaMutationReceipt"},
		{&QuotaWriterEpoch{}, "QuotaWriterEpoch"},
		{&QuotaProjectionObligation{}, "QuotaProjectionObligation"},
		{&Model{}, "Model"},
		{&Vendor{}, "Vendor"},
		{&PrefillGroup{}, "PrefillGroup"},
		{&Setup{}, "Setup"},
		{&TwoFA{}, "TwoFA"},
		{&TwoFABackupCode{}, "TwoFABackupCode"},
		{&Checkin{}, "Checkin"},
		{&SubscriptionOrder{}, "SubscriptionOrder"},
		{&UserSubscription{}, "UserSubscription"},
		{&SubscriptionPreConsumeRecord{}, "SubscriptionPreConsumeRecord"},
		{&CustomOAuthProvider{}, "CustomOAuthProvider"},
		{&UserOAuthBinding{}, "UserOAuthBinding"},
		{&PerfMetric{}, "PerfMetric"},
		{&SystemInstance{}, "SystemInstance"},
		{&SystemTask{}, "SystemTask"},
		{&SystemTaskLock{}, "SystemTaskLock"},
		{&LogProjectionBackfillState{}, "LogProjectionBackfillState"},
		{&CasbinRule{}, "CasbinRule"},
		{&AuthzRole{}, "AuthzRole"},
	}
	// Schema migrations must run serially.  Concurrent AutoMigrate calls on a
	// shared connection are prone to SQLite schema locks and can race on DDL
	// and index creation on MySQL/PostgreSQL.
	for _, m := range migrations {
		if err := DB.AutoMigrate(m.model); err != nil {
			return fmt.Errorf("failed to migrate %s: %w", m.name, err)
		}
	}
	if os.Getenv("LOG_SQL_DSN") == "" {
		if err := migrateRelationalLogDBStartup(DB); err != nil {
			return err
		}
	}
	if err := ensureUserNormalizedEmail(); err != nil {
		return err
	}
	if err := ensureChannelQuotaSnapshotDedupeIndex(); err != nil {
		return err
	}
	if err := MigrateAccessProfileIdentifiers(); err != nil {
		return err
	}
	if err := InitializeUserAuthVersions(); err != nil {
		return err
	}
	if err := InitializeExternalIdentityClaims(); err != nil {
		return err
	}
	if err := ensureQuotaMutationReceiptSchema(); err != nil {
		return err
	}
	if err := EnsureQuotaWriterEpochStateWithDB(DB); err != nil {
		return err
	}
	if err := InitializeQuotaProjectionObligationsWithDB(DB); err != nil {
		return err
	}
	if err := ensureTaskTerminalObservationSchemaWithDB(DB); err != nil {
		return err
	}
	if common.UsingMainDatabase(common.DatabaseTypeSQLite) {
		if err := ensureSubscriptionPlanTableSQLite(); err != nil {
			return err
		}
	} else {
		if err := DB.AutoMigrate(&SubscriptionPlan{}); err != nil {
			return err
		}
	}
	common.SysLog("database migrated")
	return nil
}

// ensureUserNormalizedEmail adds and backfills the portable email key before
// creating its unique index. It deliberately fails when legacy rows collide;
// silently choosing a winner would change account ownership and make OAuth
// bindings ambiguous. NULL is used for empty emails because all supported
// databases permit multiple NULL values in a unique index.
func ensureUserNormalizedEmail() error {
	if DB == nil || DB.Dialector == nil || DB.Dialector.Name() == string(common.DatabaseTypeClickHouse) {
		return nil
	}
	if !DB.Migrator().HasTable(&User{}) {
		return nil
	}
	if !DB.Migrator().HasColumn(&User{}, "email_normalized") {
		if err := DB.Exec("ALTER TABLE users ADD COLUMN email_normalized varchar(50)").Error; err != nil {
			return fmt.Errorf("add users.email_normalized: %w", err)
		}
	}
	lengthFunction := "CHAR_LENGTH"
	if DB.Dialector.Name() == string(common.DatabaseTypeSQLite) {
		lengthFunction = "LENGTH"
	}
	var oversized string
	if err := DB.Raw("SELECT email FROM users WHERE email IS NOT NULL AND " + lengthFunction + "(LOWER(TRIM(email))) > 50 LIMIT 1").Scan(&oversized).Error; err != nil {
		return fmt.Errorf("check normalized email length: %w", err)
	}
	if oversized != "" {
		return fmt.Errorf("normalized email exceeds 50 characters: %q", oversized)
	}
	if err := DB.Exec("UPDATE users SET email_normalized = NULL WHERE email IS NULL OR TRIM(email) = ''").Error; err != nil {
		return fmt.Errorf("clear empty users.email_normalized: %w", err)
	}
	if err := DB.Exec("UPDATE users SET email_normalized = LOWER(TRIM(email)) WHERE email IS NOT NULL AND TRIM(email) <> ''").Error; err != nil {
		return fmt.Errorf("backfill users.email_normalized: %w", err)
	}
	var conflict string
	if err := DB.Raw("SELECT email_normalized FROM users WHERE email_normalized IS NOT NULL AND email_normalized <> '' GROUP BY email_normalized HAVING COUNT(*) > 1 LIMIT 1").Scan(&conflict).Error; err != nil {
		return fmt.Errorf("check normalized email conflicts: %w", err)
	}
	if conflict != "" {
		return fmt.Errorf("normalized email conflict for %q: %w", conflict, ErrEmailAlreadyTaken)
	}
	const index = "idx_users_email_normalized_unique"
	migrator := DB.Migrator()
	if migrator.HasIndex(&User{}, index) {
		return nil
	}
	statement := "CREATE UNIQUE INDEX " + index + " ON users (email_normalized)"
	switch DB.Dialector.Name() {
	case string(common.DatabaseTypePostgreSQL), string(common.DatabaseTypeSQLite):
		statement = "CREATE UNIQUE INDEX IF NOT EXISTS " + index + " ON users (email_normalized)"
	}
	if err := DB.Exec(statement).Error; err != nil {
		if migrator.HasIndex(&User{}, index) {
			return nil
		}
		return fmt.Errorf("create normalized email unique index: %w", err)
	}
	return nil
}

func migrateLOGDB() error {
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		return migrateClickHouseLogDB()
	}
	return migrateRelationalLogDBStartup(LOG_DB)
}

func relationalLogTableHasRows(db *gorm.DB) (bool, error) {
	var row int
	result := db.Table("logs").Select("1").Limit(1).Scan(&row)
	return row == 1, result.Error
}

func migrateRelationalLogDBStartup(db *gorm.DB) error {
	if db == nil || db.Dialector == nil {
		return gorm.ErrInvalidDB
	}
	if err := registerLogCreateGuard(db); err != nil {
		return err
	}
	if !db.Migrator().HasTable(&Log{}) {
		if err := db.AutoMigrate(&Log{}, &BillingLogProjectionIdentity{}); err != nil {
			return err
		}
		return EnsureLogProjectionSchemaWithDB(db)
	}
	if err := db.AutoMigrate(&BillingLogProjectionIdentity{}); err != nil {
		return fmt.Errorf("migrate billing log projection identity state: %w", err)
	}
	hasRows, err := relationalLogTableHasRows(db)
	if err != nil {
		return err
	}
	missing := make([]string, 0, 3)
	if !db.Migrator().HasColumn(&Log{}, "BillingEventID") {
		missing = append(missing, "billing_event_id")
	}
	if !db.Migrator().HasColumn(&Log{}, "BillingProjectionDigest") {
		missing = append(missing, "billing_projection_digest")
	}
	if !db.Migrator().HasColumn(&Log{}, "LogRowKey") {
		missing = append(missing, "log_row_key")
	}
	if len(missing) == 0 {
		return nil
	}
	if hasRows {
		reason := "existing non-empty logs table is missing required log projection schema: " + strings.Join(missing, ", ") + "; stop all application nodes, apply the documented additive column DDL, then restart the master node"
		if err := SetLogProjectionMaintenanceRequired(context.Background(), DB, db, reason); err != nil {
			return errors.Join(fmt.Errorf("%w: %s", ErrLogProjectionMaintenanceRequired, reason), err)
		}
		return fmt.Errorf("%w: %s", ErrLogProjectionMaintenanceRequired, reason)
	}
	return EnsureLogProjectionSchemaWithDB(db)
}

func ValidateLogProjectionSchemaWithDB(db *gorm.DB) error {
	if db == nil || db.Dialector == nil {
		return gorm.ErrInvalidDB
	}
	missing := make([]string, 0, 5)
	if db.Dialector.Name() == string(common.DatabaseTypeClickHouse) {
		logsExists := false
		for _, table := range []string{"logs", clickHouseIdentityTable} {
			var count int64
			if err := db.Raw("SELECT count() FROM system.tables WHERE database = currentDatabase() AND name = ?", table).Scan(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				missing = append(missing, "table "+table)
			} else if table == "logs" {
				logsExists = true
			}
		}
		for _, column := range []string{"billing_event_id", "billing_projection_digest", "log_row_key"} {
			var count int64
			if err := db.Raw("SELECT count() FROM system.columns WHERE database = currentDatabase() AND table = 'logs' AND name = ?", column).Scan(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				missing = append(missing, "column "+column)
			}
		}
		if logsExists {
			var createSQL string
			if err := db.Raw("SHOW CREATE TABLE logs").Scan(&createSQL).Error; err != nil {
				return err
			}
			if !strings.Contains(strings.ToLower(createSQL), "projection "+strings.ToLower(clickHouseCanonicalProjection)) {
				missing = append(missing, "projection "+clickHouseCanonicalProjection)
			}
		}
	} else {
		if !db.Migrator().HasTable(&Log{}) {
			missing = append(missing, "table logs")
		} else {
			for _, column := range []struct{ field, name string }{
				{"BillingEventID", "billing_event_id"}, {"BillingProjectionDigest", "billing_projection_digest"}, {"LogRowKey", "log_row_key"},
			} {
				if !db.Migrator().HasColumn(&Log{}, column.field) {
					missing = append(missing, "column "+column.name)
				}
			}
		}
		if !db.Migrator().HasTable(&BillingLogProjectionIdentity{}) {
			missing = append(missing, "table "+clickHouseIdentityTable)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("%w: missing %s; run the master-node additive log projection migration before starting non-master nodes", ErrLogProjectionMaintenanceRequired, strings.Join(missing, ", "))
	}
	return nil
}

func EnsureLogProjectionSchemaWithDB(db *gorm.DB) error {
	if db == nil || db.Dialector == nil || db.Dialector.Name() == string(common.DatabaseTypeClickHouse) {
		return nil
	}
	if !db.Migrator().HasTable(&Log{}) {
		if err := db.AutoMigrate(&Log{}); err != nil {
			return err
		}
	}
	if err := db.AutoMigrate(&BillingLogProjectionIdentity{}); err != nil {
		return fmt.Errorf("migrate billing log projection identity state: %w", err)
	}
	for _, column := range []struct {
		field      string
		name       string
		definition string
	}{
		{"BillingEventID", "billing_event_id", "varchar(64) NOT NULL DEFAULT ''"},
		{"BillingProjectionDigest", "billing_projection_digest", "varchar(65) NOT NULL DEFAULT ''"},
		{"LogRowKey", "log_row_key", "varchar(64) NOT NULL DEFAULT ''"},
	} {
		if db.Migrator().HasColumn(&Log{}, column.field) {
			continue
		}
		if err := db.Exec("ALTER TABLE logs ADD COLUMN " + column.name + " " + column.definition).Error; err != nil {
			return fmt.Errorf("add log projection column %s: %w", column.name, err)
		}
	}
	return nil
}

// ensureChannelQuotaSnapshotDedupeIndex adds the database-level arbiter used
// by concurrent quota samplers. The column itself is deliberately nullable
// and migrated without a UNIQUE clause because SQLite rejects
// `ALTER TABLE ... ADD COLUMN ... UNIQUE` for existing databases.
func ensureChannelQuotaSnapshotDedupeIndex() error {
	if DB == nil || common.UsingMainDatabase(common.DatabaseTypeClickHouse) {
		return nil
	}
	migrator := DB.Migrator()
	const index = "idx_channel_quota_dedupe_key"
	if migrator.HasIndex(&ChannelQuotaSnapshot{}, index) {
		return nil
	}
	table := "channel_quota_snapshots"
	column := "dedupe_key"
	var statement string
	// Use the handle's dialect instead of the process-wide setting. This keeps
	// test/database handles and multi-database startup paths from selecting an
	// incompatible CREATE INDEX form.
	switch DB.Dialector.Name() {
	case string(common.DatabaseTypePostgreSQL), string(common.DatabaseTypeSQLite):
		statement = fmt.Sprintf("CREATE UNIQUE INDEX IF NOT EXISTS %s ON %s (%s)", index, table, column)
	default:
		// MySQL has no portable IF NOT EXISTS form for CREATE INDEX. Concurrent
		// migrators may race after HasIndex; re-checking after an error makes the
		// duplicate-index race idempotent while preserving real DDL errors.
		statement = fmt.Sprintf("CREATE UNIQUE INDEX %s ON %s (%s)", index, table, column)
	}
	if err := DB.Exec(statement).Error; err != nil {
		if migrator.HasIndex(&ChannelQuotaSnapshot{}, index) {
			return nil
		}
		return fmt.Errorf("create channel quota snapshot dedupe index: %w", err)
	}
	return nil
}

func ensureQuotaMutationReceiptSchema() error {
	return ensureQuotaMutationReceiptSchemaWithDB(DB)
}

func ensureQuotaMutationReceiptSchemaWithDB(db *gorm.DB) error {
	if db == nil || !db.Migrator().HasTable(&QuotaMutationReceipt{}) {
		return nil
	}
	migrator := db.Migrator()
	const oldIndex = "uidx_quota_mutation_receipt_operation"
	if migrator.HasIndex(&QuotaMutationReceipt{}, oldIndex) {
		if err := migrator.DropIndex(&QuotaMutationReceipt{}, oldIndex); err != nil {
			common.SysError(fmt.Sprintf("drop legacy quota mutation receipt operation index: %v", err))
		}
	}
	return nil
}

func migrateClickHouseLogDB() error {
	if err := registerLogCreateGuard(LOG_DB); err != nil {
		return err
	}
	ttlDays := clickHouseLogTTLDays()
	var tableCount int64
	if err := LOG_DB.Raw("SELECT count() FROM system.tables WHERE database = currentDatabase() AND name = 'logs'").Scan(&tableCount).Error; err != nil {
		return err
	}
	if tableCount == 0 {
		if err := LOG_DB.Exec(clickHouseLogCreateTableSQL(ttlDays)).Error; err != nil {
			return err
		}
		if err := ensureClickHouseProjectionIdentityTable(); err != nil {
			return err
		}
		return registerLogCreateGuard(LOG_DB)
	}
	if err := ensureClickHouseProjectionIdentityTable(); err != nil {
		return err
	}
	var hasRow int
	if err := LOG_DB.Raw("SELECT 1 FROM logs LIMIT 1").Scan(&hasRow).Error; err != nil {
		return err
	}
	missing := make([]string, 0, 4)
	for _, column := range []string{"billing_event_id", "billing_projection_digest", "log_row_key"} {
		exists, err := clickHouseLogColumnExists(column)
		if err != nil {
			return err
		}
		if !exists {
			missing = append(missing, column)
		}
	}
	projectionExists, err := clickHouseCanonicalProjectionExists()
	if err != nil {
		return err
	}
	if !projectionExists {
		missing = append(missing, "projection "+clickHouseCanonicalProjection)
	}
	if hasRow == 1 && len(missing) > 0 {
		reason := "existing non-empty ClickHouse logs table is missing required log projection schema: " + strings.Join(missing, ", ") + "; stop all application nodes, apply additive columns/projection DDL, then restart the master node"
		if err := SetLogProjectionMaintenanceRequired(context.Background(), DB, LOG_DB, reason); err != nil {
			return errors.Join(fmt.Errorf("%w: %s", ErrLogProjectionMaintenanceRequired, reason), err)
		}
		return fmt.Errorf("%w: %s", ErrLogProjectionMaintenanceRequired, reason)
	}
	for _, column := range []struct {
		name       string
		definition string
	}{
		{"billing_event_id", "billing_event_id String DEFAULT ''"},
		{"billing_projection_digest", "billing_projection_digest String DEFAULT ''"},
		{"log_row_key", "log_row_key String DEFAULT ''"},
	} {
		if err := ensureClickHouseLogColumn(column.name, column.definition); err != nil {
			return err
		}
	}
	if err := ensureClickHouseCanonicalProjection(); err != nil {
		return err
	}
	return syncClickHouseLogTTL(ttlDays)
}

func ensureClickHouseProjectionIdentityTable() error {
	if err := LOG_DB.Exec(clickHouseBillingProjectionIdentityCreateTableSQL()).Error; err != nil {
		return fmt.Errorf("create ClickHouse billing projection identity table: %w", err)
	}
	for _, column := range []struct {
		name       string
		definition string
	}{
		{"status", "status String DEFAULT 'canonical'"},
		{"reason", "reason String DEFAULT ''"},
	} {
		var count int64
		if err := LOG_DB.Raw("SELECT count() FROM system.columns WHERE database = currentDatabase() AND table = ? AND name = ?", clickHouseIdentityTable, column.name).Scan(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			if err := LOG_DB.Exec("ALTER TABLE " + clickHouseIdentityTable + " ADD COLUMN IF NOT EXISTS " + column.definition).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func ensureClickHouseLogColumn(name string, definition string) error {
	hasColumn, err := clickHouseLogColumnExists(name)
	if err != nil || hasColumn {
		return err
	}
	if err := LOG_DB.Exec("ALTER TABLE logs ADD COLUMN IF NOT EXISTS " + definition).Error; err != nil {
		hasColumn, checkErr := clickHouseLogColumnExists(name)
		if checkErr == nil && hasColumn {
			return nil
		}
		return err
	}
	return nil
}

func clickHouseCanonicalProjectionExists() (bool, error) {
	var createTableSQL string
	if err := LOG_DB.Raw("SHOW CREATE TABLE logs").Scan(&createTableSQL).Error; err != nil {
		return false, err
	}
	return strings.Contains(strings.ToLower(createTableSQL), "projection "+strings.ToLower(clickHouseCanonicalProjection)), nil
}

func ensureClickHouseCanonicalProjection() error {
	definition := strings.TrimPrefix(clickHouseCanonicalProjectionDefinition(), "PROJECTION ")
	if err := LOG_DB.Exec("ALTER TABLE logs ADD PROJECTION IF NOT EXISTS " + definition).Error; err != nil {
		return err
	}
	exists, err := clickHouseCanonicalProjectionExists()
	if err != nil {
		return err
	}
	if !exists {
		return errors.New("ClickHouse canonical projection was not observable in SHOW CREATE TABLE")
	}
	return nil
}

func clickHouseLogColumnExists(column string) (bool, error) {
	var count int64
	err := LOG_DB.Raw(
		"SELECT count() FROM system.columns WHERE database = currentDatabase() AND table = ? AND name = ?",
		"logs",
		column,
	).Scan(&count).Error
	return count > 0, err
}

func clickHouseLogTTLDays() int {
	ttlDays := common.GetEnvOrDefault("LOG_SQL_CLICKHOUSE_TTL_DAYS", 0)
	if ttlDays < 0 {
		return 0
	}
	return ttlDays
}

func clickHouseLogTTLExpression(ttlDays int) string {
	if ttlDays <= 0 {
		return ""
	}
	return fmt.Sprintf("toDateTime(created_at) + INTERVAL %d DAY DELETE WHERE billing_event_id = ''", ttlDays)
}

func clickHouseLogTTLClause(ttlDays int) string {
	expression := clickHouseLogTTLExpression(ttlDays)
	if expression == "" {
		return ""
	}
	return "\nTTL " + expression
}

func clickHouseLogCreateTableSQL(ttlDays int) string {
	return fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS logs (
	id Int64 DEFAULT 0,
	user_id Int32 DEFAULT 0,
	created_at Int64 DEFAULT 0,
	type Int32 DEFAULT 0,
	content String DEFAULT '',
	username String DEFAULT '',
	token_name String DEFAULT '',
	model_name String DEFAULT '',
	quota Int32 DEFAULT 0,
	prompt_tokens Int32 DEFAULT 0,
	completion_tokens Int32 DEFAULT 0,
	use_time Int32 DEFAULT 0,
	is_stream UInt8 DEFAULT 0,
	channel_id Int32 DEFAULT 0,
	token_id Int32 DEFAULT 0,
	`+"`group`"+` String DEFAULT '',
	ip String DEFAULT '',
	request_id String DEFAULT '',
	upstream_request_id String DEFAULT '',
	billing_event_id String DEFAULT '',
	billing_projection_digest String DEFAULT '',
	log_row_key String DEFAULT '',
	other String DEFAULT '',
	%s
)
ENGINE = MergeTree()
PARTITION BY toYYYYMM(toDateTime(created_at))
ORDER BY (created_at, request_id, log_row_key)%s`, clickHouseCanonicalProjectionDefinition(), clickHouseLogTTLClause(ttlDays))
}

func clickHouseBillingProjectionIdentityCreateTableSQL() string {
	return `
CREATE TABLE IF NOT EXISTS billing_log_projection_identities (
	billing_event_id String,
	digest String,
	canonical_version UInt16,
	status String DEFAULT 'canonical',
	reason String DEFAULT '',
	updated_at Int64
)
ENGINE = MergeTree()
ORDER BY billing_event_id`
}

func syncClickHouseLogTTL(ttlDays int) error {
	expression := clickHouseLogTTLExpression(ttlDays)
	if expression != "" {
		return LOG_DB.Exec("ALTER TABLE logs MODIFY TTL " + expression).Error
	}

	hasTTL, err := clickHouseLogTableHasTTL()
	if err != nil {
		return err
	}
	if !hasTTL {
		return nil
	}
	return LOG_DB.Exec("ALTER TABLE logs REMOVE TTL").Error
}

func clickHouseLogTableHasTTL() (bool, error) {
	var createTableSQL string
	if err := LOG_DB.Raw("SHOW CREATE TABLE logs").Scan(&createTableSQL).Error; err != nil {
		return false, err
	}
	return clickHouseCreateTableHasTTL(createTableSQL), nil
}

func clickHouseCreateTableHasTTL(createTableSQL string) bool {
	upperSQL := strings.ToUpper(createTableSQL)
	return strings.Contains(upperSQL, "\nTTL ") || strings.Contains(upperSQL, " TTL ")
}

type sqliteColumnDef struct {
	Name string
	DDL  string
}

func ensureSubscriptionPlanTableSQLite() error {
	if !common.UsingMainDatabase(common.DatabaseTypeSQLite) {
		return nil
	}
	tableName := "subscription_plans"
	if !DB.Migrator().HasTable(tableName) {
		createSQL := `CREATE TABLE ` + "`" + tableName + "`" + ` (
` + "`id`" + ` integer,
` + "`title`" + ` varchar(128) NOT NULL,
` + "`subtitle`" + ` varchar(255) DEFAULT '',
` + "`price_amount`" + ` decimal(10,6) NOT NULL,
` + "`currency`" + ` varchar(8) NOT NULL DEFAULT 'USD',
` + "`duration_unit`" + ` varchar(16) NOT NULL DEFAULT 'month',
` + "`duration_value`" + ` integer NOT NULL DEFAULT 1,
` + "`custom_seconds`" + ` bigint NOT NULL DEFAULT 0,
` + "`enabled`" + ` numeric DEFAULT 1,
` + "`sort_order`" + ` integer DEFAULT 0,
` + "`allow_balance_pay`" + ` numeric DEFAULT 1,
` + "`allow_wallet_overflow`" + ` numeric DEFAULT 1,
` + "`stripe_price_id`" + ` varchar(128) DEFAULT '',
` + "`creem_product_id`" + ` varchar(128) DEFAULT '',
` + "`waffo_pancake_product_id`" + ` varchar(128) DEFAULT '',
` + "`max_purchase_per_user`" + ` integer DEFAULT 0,
` + "`upgrade_group`" + ` varchar(64) DEFAULT '',
` + "`downgrade_group`" + ` varchar(64) DEFAULT '',
` + "`total_amount`" + ` bigint NOT NULL DEFAULT 0,
` + "`quota_reset_period`" + ` varchar(16) DEFAULT 'never',
` + "`quota_reset_custom_seconds`" + ` bigint DEFAULT 0,
` + "`created_at`" + ` bigint,
` + "`updated_at`" + ` bigint,
PRIMARY KEY (` + "`id`" + `)
)`
		return DB.Exec(createSQL).Error
	}
	var cols []struct {
		Name string `gorm:"column:name"`
	}
	if err := DB.Raw("PRAGMA table_info(`" + tableName + "`)").Scan(&cols).Error; err != nil {
		return err
	}
	existing := make(map[string]struct{}, len(cols))
	for _, c := range cols {
		existing[c.Name] = struct{}{}
	}
	required := []sqliteColumnDef{
		// SQLite rejects adding a NOT NULL column without a default when rows
		// already exist. Defaults preserve legacy rows and are also valid for
		// empty tables; normal writes still receive the model's required fields.
		{Name: "title", DDL: "`title` varchar(128) NOT NULL DEFAULT ''"},
		{Name: "subtitle", DDL: "`subtitle` varchar(255) DEFAULT ''"},
		{Name: "price_amount", DDL: "`price_amount` decimal(10,6) NOT NULL DEFAULT 0"},
		{Name: "currency", DDL: "`currency` varchar(8) NOT NULL DEFAULT 'USD'"},
		{Name: "duration_unit", DDL: "`duration_unit` varchar(16) NOT NULL DEFAULT 'month'"},
		{Name: "duration_value", DDL: "`duration_value` integer NOT NULL DEFAULT 1"},
		{Name: "custom_seconds", DDL: "`custom_seconds` bigint NOT NULL DEFAULT 0"},
		{Name: "enabled", DDL: "`enabled` numeric DEFAULT 1"},
		{Name: "sort_order", DDL: "`sort_order` integer DEFAULT 0"},
		{Name: "allow_balance_pay", DDL: "`allow_balance_pay` numeric DEFAULT 1"},
		{Name: "allow_wallet_overflow", DDL: "`allow_wallet_overflow` numeric DEFAULT 1"},
		{Name: "stripe_price_id", DDL: "`stripe_price_id` varchar(128) DEFAULT ''"},
		{Name: "creem_product_id", DDL: "`creem_product_id` varchar(128) DEFAULT ''"},
		{Name: "waffo_pancake_product_id", DDL: "`waffo_pancake_product_id` varchar(128) DEFAULT ''"},
		{Name: "max_purchase_per_user", DDL: "`max_purchase_per_user` integer DEFAULT 0"},
		{Name: "upgrade_group", DDL: "`upgrade_group` varchar(64) DEFAULT ''"},
		{Name: "downgrade_group", DDL: "`downgrade_group` varchar(64) DEFAULT ''"},
		{Name: "total_amount", DDL: "`total_amount` bigint NOT NULL DEFAULT 0"},
		{Name: "quota_reset_period", DDL: "`quota_reset_period` varchar(16) DEFAULT 'never'"},
		{Name: "quota_reset_custom_seconds", DDL: "`quota_reset_custom_seconds` bigint DEFAULT 0"},
		{Name: "created_at", DDL: "`created_at` bigint"},
		{Name: "updated_at", DDL: "`updated_at` bigint"},
	}
	for _, col := range required {
		if _, ok := existing[col.Name]; ok {
			continue
		}
		if err := DB.Exec("ALTER TABLE `" + tableName + "` ADD COLUMN " + col.DDL).Error; err != nil {
			return err
		}
	}
	return nil
}

// migrateTokenModelLimitsToText migrates model_limits column from varchar(1024) to text
// This is safe to run multiple times - it checks the column type first
func migrateTokenModelLimitsToText() error {
	dialect := ""
	if DB != nil && DB.Dialector != nil {
		dialect = DB.Dialector.Name()
	}
	// SQLite uses type affinity, so TEXT and VARCHAR are effectively the same — no migration needed
	if dialect == string(common.DatabaseTypeSQLite) {
		return nil
	}

	tableName := "tokens"
	columnName := "model_limits"

	if !DB.Migrator().HasTable(tableName) {
		return nil
	}

	if !DB.Migrator().HasColumn(&Token{}, columnName) {
		return nil
	}

	var alterSQL string
	if dialect == string(common.DatabaseTypePostgreSQL) {
		var dataType string
		if err := DB.Raw(`SELECT data_type FROM information_schema.columns
			WHERE table_schema = current_schema() AND table_name = ? AND column_name = ?`,
			tableName, columnName).Scan(&dataType).Error; err != nil {
			common.SysLog(fmt.Sprintf("Warning: failed to query metadata for %s.%s: %v", tableName, columnName, err))
		} else if dataType == "text" {
			return nil
		}
		alterSQL = fmt.Sprintf(`ALTER TABLE %s ALTER COLUMN %s TYPE text`, tableName, columnName)
	} else if dialect == string(common.DatabaseTypeMySQL) {
		var columnType string
		if err := DB.Raw(`SELECT COLUMN_TYPE FROM information_schema.columns
				WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?`,
			tableName, columnName).Scan(&columnType).Error; err != nil {
			common.SysLog(fmt.Sprintf("Warning: failed to query metadata for %s.%s: %v", tableName, columnName, err))
		} else if strings.ToLower(columnType) == "text" {
			return nil
		}
		alterSQL = fmt.Sprintf("ALTER TABLE %s MODIFY COLUMN %s text", tableName, columnName)
	} else {
		return nil
	}

	if alterSQL != "" {
		if err := DB.Exec(alterSQL).Error; err != nil {
			return fmt.Errorf("failed to migrate %s.%s to text: %w", tableName, columnName, err)
		}
		common.SysLog(fmt.Sprintf("Successfully migrated %s.%s to text", tableName, columnName))
	}
	return nil
}

// migrateSubscriptionPlanPriceAmount migrates price_amount column from float/double to decimal(10,6)
// This is safe to run multiple times - it checks the column type first
func migrateSubscriptionPlanPriceAmount() error {
	dialect := ""
	if DB != nil && DB.Dialector != nil {
		dialect = DB.Dialector.Name()
	}
	// SQLite doesn't support ALTER COLUMN, and its type affinity handles this automatically
	// Skip early to avoid GORM parsing the existing table DDL which may cause issues
	if dialect == string(common.DatabaseTypeSQLite) {
		return nil
	}

	tableName := "subscription_plans"
	columnName := "price_amount"

	// Check if table exists first
	if !DB.Migrator().HasTable(tableName) {
		return nil
	}

	// Check if column exists
	if !DB.Migrator().HasColumn(&SubscriptionPlan{}, columnName) {
		return nil
	}

	var alterSQL string
	if dialect == string(common.DatabaseTypePostgreSQL) {
		// PostgreSQL: Check if already decimal/numeric
		var dataType string
		if err := DB.Raw(`SELECT data_type FROM information_schema.columns
			WHERE table_schema = current_schema() AND table_name = ? AND column_name = ?`,
			tableName, columnName).Scan(&dataType).Error; err != nil {
			common.SysLog(fmt.Sprintf("Warning: failed to query metadata for %s.%s: %v", tableName, columnName, err))
		} else if dataType == "numeric" {
			return nil // Already decimal/numeric
		}
		alterSQL = fmt.Sprintf(`ALTER TABLE %s ALTER COLUMN %s TYPE decimal(10,6) USING %s::decimal(10,6)`,
			tableName, columnName, columnName)
	} else if dialect == string(common.DatabaseTypeMySQL) {
		// MySQL: Check if already decimal
		var columnType string
		if err := DB.Raw(`SELECT COLUMN_TYPE FROM information_schema.columns
				WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?`,
			tableName, columnName).Scan(&columnType).Error; err != nil {
			common.SysLog(fmt.Sprintf("Warning: failed to query metadata for %s.%s: %v", tableName, columnName, err))
		} else if strings.HasPrefix(strings.ToLower(columnType), "decimal") {
			return nil // Already decimal
		}
		alterSQL = fmt.Sprintf("ALTER TABLE %s MODIFY COLUMN %s decimal(10,6) NOT NULL DEFAULT 0",
			tableName, columnName)
	} else {
		return nil
	}

	if alterSQL != "" {
		if err := DB.Exec(alterSQL).Error; err != nil {
			return fmt.Errorf("failed to migrate %s.%s to decimal: %w", tableName, columnName, err)
		} else {
			common.SysLog(fmt.Sprintf("Successfully migrated %s.%s to decimal(10,6)", tableName, columnName))
		}
	}
	return nil
}

func closeDB(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	err = sqlDB.Close()
	return err
}

func CloseDB() error {
	if LOG_DB != DB {
		err := closeDB(LOG_DB)
		if err != nil {
			return err
		}
	}
	return closeDB(DB)
}

// checkMySQLChineseSupport ensures the MySQL connection and current schema
// default charset/collation can store Chinese characters. It allows common
// Chinese-capable charsets (utf8mb4, utf8, gbk, big5, gb18030) and panics otherwise.
func checkMySQLChineseSupport(db *gorm.DB) error {
	// 仅检测：当前库默认字符集/排序规则 + 各表的排序规则（隐含字符集）

	// Read current schema defaults
	var schemaCharset, schemaCollation string
	err := db.Raw("SELECT DEFAULT_CHARACTER_SET_NAME, DEFAULT_COLLATION_NAME FROM information_schema.SCHEMATA WHERE SCHEMA_NAME = DATABASE()").Row().Scan(&schemaCharset, &schemaCollation)
	if err != nil {
		return fmt.Errorf("读取当前库默认字符集/排序规则失败 / Failed to read schema default charset/collation: %v", err)
	}

	toLower := func(s string) string { return strings.ToLower(s) }
	// Allowed charsets that can store Chinese text
	allowedCharsets := map[string]string{
		"utf8mb4": "utf8mb4_",
		"utf8":    "utf8_",
		"gbk":     "gbk_",
		"big5":    "big5_",
		"gb18030": "gb18030_",
	}
	isChineseCapable := func(cs, cl string) bool {
		csLower := toLower(cs)
		clLower := toLower(cl)
		if prefix, ok := allowedCharsets[csLower]; ok {
			if clLower == "" {
				return true
			}
			return strings.HasPrefix(clLower, prefix)
		}
		// 如果仅提供了排序规则，尝试按排序规则前缀判断
		for _, prefix := range allowedCharsets {
			if strings.HasPrefix(clLower, prefix) {
				return true
			}
		}
		return false
	}

	// 1) 当前库默认值必须支持中文
	if !isChineseCapable(schemaCharset, schemaCollation) {
		return fmt.Errorf("当前库默认字符集/排序规则不支持中文：schema(%s/%s)。请将库设置为 utf8mb4/utf8/gbk/big5/gb18030 / Schema default charset/collation is not Chinese-capable: schema(%s/%s). Please set to utf8mb4/utf8/gbk/big5/gb18030",
			schemaCharset, schemaCollation, schemaCharset, schemaCollation)
	}

	// 2) 所有物理表的排序规则（隐含字符集）必须支持中文
	type tableInfo struct {
		Name      string
		Collation *string
	}
	var tables []tableInfo
	if err := db.Raw("SELECT TABLE_NAME, TABLE_COLLATION FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_TYPE = 'BASE TABLE'").Scan(&tables).Error; err != nil {
		return fmt.Errorf("读取表排序规则失败 / Failed to read table collations: %v", err)
	}

	var badTables []string
	for _, t := range tables {
		// NULL 或空表示继承库默认设置，已在上面校验库默认，视为通过
		if t.Collation == nil || *t.Collation == "" {
			continue
		}
		cl := *t.Collation
		// 仅凭排序规则判断是否中文可用
		ok := false
		lower := strings.ToLower(cl)
		for _, prefix := range allowedCharsets {
			if strings.HasPrefix(lower, prefix) {
				ok = true
				break
			}
		}
		if !ok {
			badTables = append(badTables, fmt.Sprintf("%s(%s)", t.Name, cl))
		}
	}

	if len(badTables) > 0 {
		// 限制输出数量以避免日志过长
		maxShow := 20
		shown := badTables
		if len(shown) > maxShow {
			shown = shown[:maxShow]
		}
		return fmt.Errorf(
			"存在不支持中文的表，请修复其排序规则/字符集。示例（最多展示 %d 项）：%v / Found tables not Chinese-capable. Please fix their collation/charset. Examples (showing up to %d): %v",
			maxShow, shown, maxShow, shown,
		)
	}
	return nil
}

var (
	lastPingTime time.Time
	pingMutex    sync.Mutex
)

func PingDB() error {
	pingMutex.Lock()
	defer pingMutex.Unlock()

	if time.Since(lastPingTime) < time.Second*10 {
		return nil
	}

	sqlDB, err := DB.DB()
	if err != nil {
		log.Printf("Error getting sql.DB from GORM: %v", err)
		return err
	}

	err = sqlDB.Ping()
	if err != nil {
		log.Printf("Error pinging DB: %v", err)
		return err
	}

	lastPingTime = time.Now()
	common.SysLog("Database pinged successfully")
	return nil
}
