package model

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func editionConfiguredFixture(t *testing.T, engine string, index int) (*gorm.DB, string, common.DatabaseType) {
	t.Helper()
	name := fmt.Sprintf("myapi_edition_%s_%x_%d", engine, time.Now().UnixNano(), index)
	var admin *sql.DB
	var targetDSN, create, drop string
	dialect := common.DatabaseTypeMySQL
	if engine == "mysql" {
		cfg, err := mysqldriver.ParseDSN(os.Getenv("MYAPI_EDITION_MYSQL_DSN"))
		require.True(t, err == nil, "invalid edition MySQL fixture DSN")
		host, _, err := net.SplitHostPort(cfg.Addr)
		require.True(t, err == nil && host == "127.0.0.1" && cfg.Net == "tcp" && cfg.DBName == "myapi_edition_test", "edition fixture must own its loopback admin database")
		connector, err := mysqldriver.NewConnector(cfg)
		require.True(t, err == nil)
		admin = sql.OpenDB(connector)
		cfg.DBName = name
		targetDSN = cfg.FormatDSN()
		create = "CREATE DATABASE `" + name + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"
		drop = "DROP DATABASE `" + name + "`"
	} else {
		dialect = common.DatabaseTypePostgreSQL
		cfg, err := pgx.ParseConfig(os.Getenv("MYAPI_EDITION_POSTGRES_DSN"))
		require.True(t, err == nil, "invalid edition PostgreSQL fixture DSN")
		require.True(t, cfg.Host == "127.0.0.1" && cfg.Database == "myapi_edition_test", "edition fixture must own its loopback admin database")
		for _, fallback := range cfg.Fallbacks {
			require.Equal(t, "127.0.0.1", fallback.Host)
		}
		cfg.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
		admin = stdlib.OpenDB(*cfg)
		targetDSN = (&url.URL{Scheme: "postgres", User: url.UserPassword(cfg.User, cfg.Password), Host: net.JoinHostPort(cfg.Host, strconv.Itoa(int(cfg.Port))), Path: "/" + name, RawQuery: "sslmode=disable"}).String()
		create = `CREATE DATABASE "` + name + `"`
		drop = `DROP DATABASE "` + name + `"`
	}
	admin.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, admin.Close()) })
	createContext, cancelCreate := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelCreate()
	_, err := admin.ExecContext(createContext, create)
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanupContext, cancelCleanup := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancelCleanup()
		_, e := admin.ExecContext(cleanupContext, drop)
		require.NoError(t, e)
	})
	var driver gorm.Dialector = gormmysql.Open(targetDSN)
	if engine != "mysql" {
		driver = postgres.Open(targetDSN)
	}
	db, err := gorm.Open(driver, &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	require.NoError(t, err)
	native, err := db.DB()
	require.NoError(t, err)
	native.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = native.Close() })
	return db, targetDSN, dialect
}

func TestEditionCompatibilityConfiguredDatabases(t *testing.T) {
	if os.Getenv("MYAPI_EDITION_DATABASE_TESTS") != "1" {
		t.Skip("edition database evidence requires its dedicated opt-in")
	}
	require.Equal(t, "true", os.Getenv("GITHUB_ACTIONS"), "only disposable Actions services may run this fixture")
	require.NotEmpty(t, os.Getenv("MYAPI_EDITION_MYSQL_DSN"))
	require.NotEmpty(t, os.Getenv("MYAPI_EDITION_POSTGRES_DSN"))
	t.Run("sqlite", TestEditionCompatibilitySQLite)
	for _, engine := range []string{"mysql", "postgres"} {
		t.Run(engine, func(t *testing.T) {
			if engine == "mysql" {
				t.Run("nontransactional-engine", func(t *testing.T) {
					db, dsn, dialect := editionConfiguredFixture(t, engine, 200)
					editionCreateSchema(t, db, dialect)
					editionSeedCase(t, db, dialect, "terminal-and-local-candidates")
					t.Setenv("SQL_DSN", dsn)
					control, err := InspectEditionCompatibilityPreMigration(context.Background())
					require.NoError(t, err)
					require.True(t, control.Complete)
					require.NoError(t, db.Exec("ALTER TABLE `top_ups` ENGINE=MyISAM").Error)
					var beforeEngine string
					require.NoError(t, db.Raw("SELECT ENGINE FROM information_schema.tables WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='top_ups'").Scan(&beforeEngine).Error)
					require.Equal(t, "MyISAM", beforeEngine)
					before := editionFixtureRows(t, db, dialect)
					native, err := db.DB()
					require.NoError(t, err)
					require.NoError(t, native.Close())
					facts, err := InspectEditionCompatibilityPreMigration(context.Background())
					require.ErrorIs(t, err, ErrEditionCompatibilityUnverifiable)
					assert.False(t, facts.Complete)
					assert.Nil(t, facts.TopUps)
					assert.Nil(t, facts.SubscriptionOrders)
					assert.Nil(t, facts.Authentication)
					reader, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
					require.NoError(t, err)
					assert.Equal(t, before, editionFixtureRows(t, reader, dialect))
					var afterEngine string
					require.NoError(t, reader.Raw("SELECT ENGINE FROM information_schema.tables WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='top_ups'").Scan(&afterEngine).Error)
					assert.Equal(t, "MyISAM", afterEngine)
					pool, err := reader.DB()
					require.NoError(t, err)
					require.NoError(t, pool.Close())
				})
			}
			for index, name := range []string{"terminal-and-local-candidates", "pending-and-unknown", "invalid-flags", "malformed-local-records", "known-no-local-path"} {
				t.Run(name, func(t *testing.T) {
					db, dsn, dialect := editionConfiguredFixture(t, engine, index)
					editionCreateSchema(t, db, dialect)
					editionSeedCase(t, db, dialect, name)
					before := editionFixtureRows(t, db, dialect)
					native, err := db.DB()
					require.NoError(t, err)
					require.NoError(t, native.Close())
					baseline := common.PasswordLoginEnabled
					common.PasswordLoginEnabled = true
					t.Cleanup(func() { common.PasswordLoginEnabled = baseline })
					t.Setenv("SQL_DSN", dsn)
					facts, err := InspectEditionCompatibilityPreMigration(context.Background())
					require.NoError(t, err)
					editionAssertCase(t, facts, name)
					var driver gorm.Dialector = gormmysql.Open(dsn)
					if engine == "postgres" {
						driver = postgres.Open(dsn)
					}
					reader, err := gorm.Open(driver, &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
					require.NoError(t, err)
					assert.Equal(t, before, editionFixtureRows(t, reader, dialect))
					pool, err := reader.DB()
					require.NoError(t, err)
					require.NoError(t, pool.Close())
					assert.NotContains(t, fmt.Sprint(facts), dsn)
				})
			}
			for index, partial := range []bool{false, true} {
				t.Run(fmt.Sprintf("partial-schema-%t", partial), func(t *testing.T) {
					db, dsn, _ := editionConfiguredFixture(t, engine, 100+index)
					if partial {
						require.NoError(t, db.Exec("CREATE TABLE users (id BIGINT PRIMARY KEY)").Error)
					}
					before, err := db.Migrator().GetTables()
					require.NoError(t, err)
					native, err := db.DB()
					require.NoError(t, err)
					require.NoError(t, native.Close())
					t.Setenv("SQL_DSN", dsn)
					facts, err := InspectEditionCompatibilityPreMigration(context.Background())
					if partial {
						require.ErrorIs(t, err, ErrEditionCompatibilityUnverifiable)
						assert.Equal(t, "partial", facts.Schema)
						assert.False(t, facts.Complete)
						assert.Nil(t, facts.TopUps)
						assert.Nil(t, facts.Authentication)
					} else {
						require.NoError(t, err)
						assert.Equal(t, "fresh", facts.Schema)
						assert.True(t, facts.Complete)
					}
					var driver gorm.Dialector = gormmysql.Open(dsn)
					if engine == "postgres" {
						driver = postgres.Open(dsn)
					}
					reader, err := gorm.Open(driver, &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
					require.NoError(t, err)
					after, err := reader.Migrator().GetTables()
					require.NoError(t, err)
					assert.Equal(t, before, after)
					pool, err := reader.DB()
					require.NoError(t, err)
					require.NoError(t, pool.Close())
				})
			}
		})
	}
}
