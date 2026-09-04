package model

import (
	"errors"
	"net"
	"os"
	"sync"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/setting"
	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/ForceMind/MyAPI/setting/operation_setting"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// These fixtures are for disposable CI services only. The test never drops
// tables; the CI service/container lifecycle owns database cleanup.
type s1LegacyIdentityRow struct {
	ID    int    `gorm:"column:id;primaryKey"`
	Group string `gorm:"column:group;type:varchar(64)"`
}

type s1UserIdentityRow struct {
	ID            int    `gorm:"column:id;primaryKey"`
	Group         string `gorm:"column:group;type:varchar(64)"`
	AccountTierID string `gorm:"column:account_tier_id;type:varchar(64);default:'standard';index"`
}

type s1TokenIdentityRow struct {
	ID              int    `gorm:"column:id;primaryKey"`
	Group           string `gorm:"column:group;type:varchar(64)"`
	AccessProfileID string `gorm:"column:access_profile_id;type:varchar(64);default:'standard';index"`
}

func s2PaymentComplianceOptionValues() map[string]string {
	return map[string]string{
		"payment_setting.compliance_confirmed":     "true",
		"payment_setting.compliance_terms_version": operation_setting.CurrentComplianceTermsVersion,
		"payment_setting.compliance_confirmed_at":  "1700000000",
		"payment_setting.compliance_confirmed_by":  "42",
		"payment_setting.compliance_confirmed_ip":  "192.0.2.17",
	}
}

func s2PaymentComplianceOldOptionValues() map[string]string {
	return map[string]string{
		"payment_setting.compliance_confirmed":     "false",
		"payment_setting.compliance_terms_version": "v0",
		"payment_setting.compliance_confirmed_at":  "1600000000",
		"payment_setting.compliance_confirmed_by":  "7",
		"payment_setting.compliance_confirmed_ip":  "198.51.100.7",
	}
}

// s2PaymentComplianceBulkRollbackContract is intentionally run against every
// supported database fixture. The failure is tied to the second update, rather
// than a particular option key, so the contract does not depend on map order.
func s2PaymentComplianceBulkRollbackContract(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.AutoMigrate(&Option{}))

	common.OptionMapRWMutex.Lock()
	previousOptions := common.OptionMap
	common.OptionMap = map[string]string{"fixture": "before"}
	oldOptions := s2PaymentComplianceOldOptionValues()
	for key, value := range oldOptions {
		common.OptionMap[key] = value
	}
	common.OptionMapRWMutex.Unlock()
	paymentSetting := operation_setting.GetPaymentSetting()
	previousPaymentSetting := *paymentSetting
	baselinePaymentSetting := operation_setting.PaymentSetting{
		ComplianceTermsVersion: oldOptions["payment_setting.compliance_terms_version"],
		ComplianceConfirmedAt:  1600000000,
		ComplianceConfirmedBy:  7,
		ComplianceConfirmedIP:  oldOptions["payment_setting.compliance_confirmed_ip"],
	}
	*paymentSetting = baselinePaymentSetting
	restoreGlobals := sync.OnceFunc(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptions
		common.OptionMapRWMutex.Unlock()
		*paymentSetting = previousPaymentSetting
	})
	defer restoreGlobals()
	t.Cleanup(restoreGlobals)
	for key, value := range oldOptions {
		require.NoError(t, db.Create(&Option{Key: key, Value: value}).Error)
	}

	injected := errors.New("injected payment compliance bulk persistence failure")
	updates := 0
	const callbackName = "test:s2-payment-compliance-bulk-failure"
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		updates++
		if updates == 2 {
			tx.AddError(injected)
		}
	}))
	removeCallback := sync.OnceFunc(func() { require.NoError(t, db.Callback().Update().Remove(callbackName)) })
	defer removeCallback()
	t.Cleanup(removeCallback)

	require.ErrorIs(t, UpdateOptionsBulk(s2PaymentComplianceOptionValues()), injected)
	assert.Equal(t, 2, updates)
	var options []Option
	require.NoError(t, db.Find(&options).Error)
	persisted := make(map[string]string, len(oldOptions))
	for _, option := range options {
		if _, ok := oldOptions[option.Key]; ok {
			persisted[option.Key] = option.Value
		}
	}
	assert.Equal(t, oldOptions, persisted)
	common.OptionMapRWMutex.RLock()
	expectedPublished := map[string]string{"fixture": "before"}
	for key, value := range oldOptions {
		expectedPublished[key] = value
	}
	assert.Equal(t, expectedPublished, common.OptionMap)
	common.OptionMapRWMutex.RUnlock()
	assert.Equal(t, baselinePaymentSetting, *paymentSetting)
}

func s1DatabaseDialector(engine, dsn string) (gorm.Dialector, error) {
	unsafeTarget := errors.New("S1 database tests require a literal loopback address and database myapi_s1_test")
	switch engine {
	case "mysql":
		cfg, err := mysqldriver.ParseDSN(dsn)
		if err != nil {
			return nil, errors.New("invalid S1 MySQL fixture DSN")
		}
		host, _, err := net.SplitHostPort(cfg.Addr)
		if err != nil || cfg.Net != "tcp" || !net.ParseIP(host).IsLoopback() || cfg.DBName != "myapi_s1_test" {
			return nil, unsafeTarget
		}
		return mysql.New(mysql.Config{DSN: cfg.FormatDSN()}), nil
	case "postgres":
		cfg, err := pgx.ParseConfig(dsn)
		if err != nil {
			return nil, errors.New("invalid S1 PostgreSQL fixture DSN")
		}
		if !net.ParseIP(cfg.Host).IsLoopback() || cfg.Database != "myapi_s1_test" {
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
		return nil, errors.New("unsupported S1 database engine")
	}
}

func TestS1AccessProfileDatabaseTargetSafety(t *testing.T) {
	for _, fixture := range []struct {
		name, engine, dsn string
	}{
		{"mysql-remote", "mysql", "fixture:fixture@tcp(192.0.2.1:3306)/myapi_s1_test"},
		{"mysql-other-database", "mysql", "fixture:fixture@tcp(127.0.0.1:3306)/application"},
		{"mysql-socket", "mysql", "fixture:fixture@unix(/tmp/mysql.sock)/myapi_s1_test"},
		{"postgres-remote", "postgres", "postgres://fixture:fixture@192.0.2.1:5432/myapi_s1_test?sslmode=disable"},
		{"postgres-other-database", "postgres", "postgres://fixture:fixture@127.0.0.1:5432/application?sslmode=disable"},
		{"postgres-remote-fallback", "postgres", "host=127.0.0.1,192.0.2.1 port=5432 dbname=myapi_s1_test user=fixture password=fixture sslmode=disable"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			_, err := s1DatabaseDialector(fixture.engine, fixture.dsn)
			require.Error(t, err)
		})
	}
}

func TestS2PaymentComplianceBulkRollbackSQLite(t *testing.T) {
	s2PaymentComplianceBulkRollbackContract(t, accessProfileTestDB(t))
}

func TestAccessProfileConfiguredDatabases(t *testing.T) {
	if os.Getenv("MYAPI_S1_DATABASE_TESTS") != "1" {
		t.Skip("disposable database tests are disabled; set MYAPI_S1_DATABASE_TESTS=1 explicitly")
	}
	for _, engine := range []struct {
		name, env string
		typeID    common.DatabaseType
	}{
		{"mysql", "MYAPI_S1_MYSQL_DSN", common.DatabaseTypeMySQL},
		{"postgres", "MYAPI_S1_POSTGRES_DSN", common.DatabaseTypePostgreSQL},
	} {
		t.Run(engine.name, func(t *testing.T) {
			dsn := os.Getenv(engine.env)
			if dsn == "" {
				t.Skip(engine.env + " is not configured")
			}
			dialector, err := s1DatabaseDialector(engine.name, dsn)
			require.NoError(t, err)
			db, err := gorm.Open(dialector, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
			// Check the complete table inventory before any writes. Query errors
			// must fail closed, rather than treating HasTable=false as proof.
			tables, err := db.Migrator().GetTables()
			require.NoError(t, err)
			for _, table := range []string{"users", "tokens", "options"} {
				require.NotContains(t, tables, table, "refusing a non-empty S1 fixture schema")
			}
			previousDB := DB
			previousType := common.MainDatabaseType()
			DB = db
			common.SetMainDatabaseType(engine.typeID)
			initCol()
			common.OptionMapRWMutex.Lock()
			previousOptions := common.OptionMap
			common.OptionMap = make(map[string]string)
			common.OptionMapRWMutex.Unlock()
			profiles, err := common.Marshal(setting.GetAccessProfileSetting().Profiles)
			require.NoError(t, err)
			t.Cleanup(func() {
				DB = previousDB
				common.SetMainDatabaseType(previousType)
				initCol()
				common.OptionMapRWMutex.Lock()
				common.OptionMap = previousOptions
				common.OptionMapRWMutex.Unlock()
				require.NoError(t, setting.UpdateAccessProfileDefinitionsByJSONString(string(profiles)))
			})

			require.NoError(t, db.Table("users").AutoMigrate(&s1LegacyIdentityRow{}))
			require.NoError(t, db.Table("tokens").AutoMigrate(&s1LegacyIdentityRow{}))
			require.NoError(t, db.Table("users").Create(&[]s1LegacyIdentityRow{{ID: 1, Group: "vip"}, {ID: 2, Group: "default"}}).Error)
			require.NoError(t, db.Table("tokens").Create(&[]s1LegacyIdentityRow{{ID: 1, Group: "auto"}, {ID: 2, Group: "default"}}).Error)
			backfillErr := errors.New("injected S1 token backfill failure")
			require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:s1-backfill-failure", func(tx *gorm.DB) {
				if tx.Statement.Table == "tokens" {
					tx.AddError(backfillErr)
				}
			}))
			require.ErrorIs(t, prepareAccessProfileIdentifiers(), backfillErr)
			for _, identity := range []struct{ table, column string }{{"users", "account_tier_id"}, {"tokens", "access_profile_id"}} {
				var missing int64
				require.NoError(t, db.Table(identity.table).Where(identity.column+" IS NULL").Count(&missing).Error)
				assert.Equal(t, int64(2), missing, "backfill writes must roll back while added nullable columns remain")
			}
			require.NoError(t, db.Callback().Update().Remove("test:s1-backfill-failure"))
			require.NoError(t, prepareAccessProfileIdentifiers())
			var users []s1UserIdentityRow
			var tokens []s1TokenIdentityRow
			require.NoError(t, db.Table("users").Order("id").Find(&users).Error)
			require.NoError(t, db.Table("tokens").Order("id").Find(&tokens).Error)
			require.Len(t, users, 2)
			require.Len(t, tokens, 2)
			assert.Equal(t, "priority", users[0].AccountTierID)
			assert.Equal(t, "automatic", tokens[0].AccessProfileID)
			assert.Equal(t, "standard", users[1].AccountTierID)
			assert.Equal(t, "standard", tokens[1].AccessProfileID)

			// Apply the existing model defaults only after legacy values have
			// been derived; no unrelated application tables are migrated.
			require.NoError(t, db.Table("users").AutoMigrate(&s1UserIdentityRow{}))
			require.NoError(t, db.Table("tokens").AutoMigrate(&s1TokenIdentityRow{}))
			for _, identity := range []struct{ table, column string }{{"users", "account_tier_id"}, {"tokens", "access_profile_id"}} {
				require.NoError(t, db.Table(identity.table).Where("id = ?", 1).Update(identity.column, "standard").Error)
				require.NoError(t, db.Table(identity.table).Where("id = ?", 2).Update(identity.column, "custom-team").Error)
				// Map insert deliberately omits identity so the database, not
				// GORM's struct default handling, supplies the standard value.
				require.NoError(t, db.Table(identity.table).Create(map[string]interface{}{"id": 3, "group": "default"}).Error)
			}
			require.NoError(t, prepareAccessProfileIdentifiers())
			require.NoError(t, MigrateAccessProfileIdentifiers())
			users, tokens = nil, nil
			require.NoError(t, db.Table("users").Order("id").Find(&users).Error)
			require.NoError(t, db.Table("tokens").Order("id").Find(&tokens).Error)
			require.Len(t, users, 3)
			require.Len(t, tokens, 3)
			assert.Equal(t, []string{"standard", "custom-team", "standard"}, []string{users[0].AccountTierID, users[1].AccountTierID, users[2].AccountTierID})
			assert.Equal(t, []string{"standard", "custom-team", "standard"}, []string{tokens[0].AccessProfileID, tokens[1].AccessProfileID, tokens[2].AccessProfileID})
			assert.Equal(t, "vip", users[0].Group)
			assert.Equal(t, "auto", tokens[0].Group)

			require.NoError(t, db.AutoMigrate(&Option{}))
			const key = "access_profile_setting.profiles"
			const before = `{"standard":{"label":"Before"}}`
			const after = `{"standard":{"label":"After"}}`
			require.NoError(t, setting.UpdateAccessProfileDefinitionsByJSONString(before))
			common.OptionMapRWMutex.Lock()
			common.OptionMap[key] = before
			common.OptionMapRWMutex.Unlock()
			writeErr := errors.New("injected S1 option persistence failure")
			for _, failure := range []string{"create-new", "save-new", "save-existing"} {
				t.Run(failure, func(t *testing.T) {
					if failure == "save-existing" {
						require.NoError(t, UpdateOption(key, before))
					}
					if failure == "create-new" {
						require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:s1-option-create", func(tx *gorm.DB) { tx.AddError(writeErr) }))
					} else {
						require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:s1-option-save", func(tx *gorm.DB) { tx.AddError(writeErr) }))
					}
					assert.ErrorIs(t, UpdateOption(key, after), writeErr)
					if failure == "create-new" {
						require.NoError(t, db.Callback().Create().Remove("test:s1-option-create"))
					} else {
						require.NoError(t, db.Callback().Update().Remove("test:s1-option-save"))
					}
					profile, ok := setting.GetAccessProfileDefinition("standard")
					require.True(t, ok)
					assert.Equal(t, "Before", profile.Label)
					var stored Option
					err := db.First(&stored, Option{Key: key}).Error
					common.OptionMapRWMutex.RLock()
					published := common.OptionMap[key]
					common.OptionMapRWMutex.RUnlock()
					if failure == "save-existing" {
						require.NoError(t, err)
						assert.Equal(t, stored.Value, published)
						assert.Equal(t, config.GlobalConfig.ExportAllConfigs()[key], stored.Value)
					} else {
						assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
						assert.Equal(t, before, published)
					}
				})
			}
			s2PaymentComplianceBulkRollbackContract(t, db)
			require.NoError(t, UpdateOption(key, after))
			var stored Option
			require.NoError(t, db.First(&stored, Option{Key: key}).Error)
			profile, ok := setting.GetAccessProfileDefinition("standard")
			require.True(t, ok)
			assert.Equal(t, "After", profile.Label)
			assert.Equal(t, config.GlobalConfig.ExportAllConfigs()[key], stored.Value)
		})
	}
}
