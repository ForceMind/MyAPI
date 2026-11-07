package service

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"github.com/glebarez/sqlite"
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

// The history regression shares only the disposable B2 database infrastructure.
// It creates its own channel and removes only that channel's fixture rows.
func quotaHistoryDatabaseDialector(engine, dsn string) (gorm.Dialector, error) {
	unsafeTarget := errors.New("history tests require 127.0.0.1 and database myapi_b2_test")
	switch engine {
	case "mysql":
		config, err := mysql.ParseDSN(dsn)
		if err != nil {
			return nil, errors.New("invalid history MySQL fixture DSN")
		}
		host, _, err := net.SplitHostPort(config.Addr)
		if err != nil || config.Net != "tcp" || host != "127.0.0.1" || config.DBName != "myapi_b2_test" {
			return nil, unsafeTarget
		}
		return gormmysql.New(gormmysql.Config{DSN: config.FormatDSN()}), nil
	case "postgres":
		config, err := pgx.ParseConfig(dsn)
		if err != nil {
			return nil, errors.New("invalid history PostgreSQL fixture DSN")
		}
		if config.Host != "127.0.0.1" || config.Database != "myapi_b2_test" {
			return nil, unsafeTarget
		}
		for _, fallback := range config.Fallbacks {
			if fallback.Host != "127.0.0.1" {
				return nil, unsafeTarget
			}
		}
		config.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
		return postgres.New(postgres.Config{Conn: stdlib.OpenDB(*config)}), nil
	default:
		return nil, errors.New("unsupported history database engine")
	}
}

func TestChannelQuotaHistoryDatabaseTargetSafety(t *testing.T) {
	for _, fixture := range []struct{ name, engine, dsn string }{
		{"mysql-remote", "mysql", "fixture:fixture@tcp(192.0.2.1:3306)/myapi_b2_test"},
		{"mysql-other-database", "mysql", "fixture:fixture@tcp(127.0.0.1:3306)/production"},
		{"mysql-socket", "mysql", "fixture:fixture@unix(/tmp/mysql.sock)/myapi_b2_test"},
		{"postgres-remote", "postgres", "postgres://fixture:fixture@192.0.2.1:5432/myapi_b2_test?sslmode=disable"},
		{"postgres-other-database", "postgres", "postgres://fixture:fixture@127.0.0.1:5432/production?sslmode=disable"},
		{"postgres-remote-fallback", "postgres", "host=127.0.0.1,192.0.2.1 port=5432 dbname=myapi_b2_test user=fixture password=fixture sslmode=disable"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			_, err := quotaHistoryDatabaseDialector(fixture.engine, fixture.dsn)
			require.Error(t, err)
		})
	}
}

func TestChannelQuotaHistoryConfiguredDatabases(t *testing.T) {
	for _, engine := range []struct{ name, env string }{
		{"sqlite", ""},
		{"mysql", "MYAPI_B2_MYSQL_DSN"},
		{"postgres", "MYAPI_B2_POSTGRES_DSN"},
	} {
		t.Run(engine.name, func(t *testing.T) {
			var dialector gorm.Dialector
			if engine.name == "sqlite" {
				dialector = sqlite.Open(filepath.Join(t.TempDir(), "history.db"))
			} else {
				if os.Getenv("MYAPI_B2_DATABASE_TESTS") != "1" {
					t.Skip("relational history fixtures require MYAPI_B2_DATABASE_TESTS=1")
				}
				dsn := os.Getenv(engine.env)
				require.NotEmpty(t, dsn, "%s is required when MYAPI_B2_DATABASE_TESTS=1", engine.env)
				var err error
				dialector, err = quotaHistoryDatabaseDialector(engine.name, dsn)
				require.NoError(t, err)
			}
			db, err := gorm.Open(dialector, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			t.Cleanup(func() { assert.NoError(t, sqlDB.Close()) })
			require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelQuotaSnapshot{}))
			previousDB, previousType := model.DB, common.MainDatabaseType()
			model.DB = db
			common.SetMainDatabaseType(common.DatabaseType(engine.name))
			t.Cleanup(func() {
				model.DB = previousDB
				common.SetMainDatabaseType(previousType)
			})

			t.Run("full-range-before-display-limit", func(t *testing.T) {
				channelID := createQuotaHistoryFixtureChannel(t, db)
				base := int64(1_700_000_000)
				for index, available := range []float64{100, 80, 70} {
					used, total := 100-available, float64(100)
					require.NoError(t, db.Create(&model.ChannelQuotaSnapshot{
						ChannelId: channelID, ObservedAt: base + int64(index*3600+17),
						Available: available, Used: &used, Total: &total,
						MetricType: "codex_rate_limit", WindowType: "five_hour", WindowSeconds: 18000,
						Source: "codex_wham_usage_primary", PlanType: "pro", Unit: "percent", Status: "success",
					}).Error)
				}
				request := ChannelQuotaHistoryRequest{
					ChannelID: channelID, Start: base, End: base + 8000, Limit: 2,
					Granularity: QuotaHistoryHour, RateWindowSeconds: 8000, EWMAHalfLifeSeconds: 4000,
				}
				before := readQuotaHistoryFixtureRows(t, db, channelID)
				data := readQuotaHistoryFixtureResponse(t, request)
				assert.Equal(t, float64(3), data["raw_observations"])
				assert.Equal(t, float64(3), data["available_points"])
				assert.Equal(t, float64(2), data["returned_points"])
				assert.Equal(t, true, data["source_complete"])
				assert.Equal(t, false, data["points_complete"])
				assert.Equal(t, false, data["complete"])
				assert.Equal(t, "point_limit", data["truncation_reason"])
				summary := data["summary"].(map[string]any)
				assert.Equal(t, float64(100), summary["start_available"])
				assert.Equal(t, float64(70), summary["end_available"])
				assert.Equal(t, float64(-30), summary["change"])
				current := data["current"].(map[string]any)
				assert.Equal(t, float64(base+7217), current["observed_at"])
				assert.Equal(t, float64(70), current["available"])
				request.Granularity, request.Limit = QuotaHistoryRaw, 1
				raw := readQuotaHistoryFixtureResponse(t, request)
				assert.Equal(t, data["analysis"], raw["analysis"])
				assert.Equal(t, data["summary"], raw["summary"])
				assert.Equal(t, before, readQuotaHistoryFixtureRows(t, db, channelID), "history reads must not alter stored observations")
			})

			t.Run("latest-failure-does-not-become-zero", func(t *testing.T) {
				channelID := createQuotaHistoryFixtureChannel(t, db)
				for _, row := range []model.ChannelQuotaSnapshot{
					{ChannelId: channelID, ObservedAt: 100, Available: 10, MetricType: "balance", WindowType: "none", Source: "provider", Unit: "usd", Status: "success"},
					{ChannelId: channelID, ObservedAt: 160, Available: 8, MetricType: "balance", WindowType: "none", Source: "provider", Unit: "usd", Status: "success"},
					{ChannelId: channelID, ObservedAt: 220, MetricType: "balance", WindowType: "none", Source: "provider", Unit: "usd", Status: "error", ErrorCode: "query_failed"},
				} {
					require.NoError(t, db.Create(&row).Error)
				}
				data := readQuotaHistoryFixtureResponse(t, ChannelQuotaHistoryRequest{
					ChannelID: channelID, Start: 1, End: 300, Limit: 500, Granularity: QuotaHistoryRaw,
					RateWindowSeconds: 299, EWMAHalfLifeSeconds: 149,
				})
				assert.Equal(t, float64(3), data["raw_observations"])
				current := data["current"].(map[string]any)
				assert.Equal(t, "error", current["status"])
				assert.Equal(t, float64(220), current["observed_at"])
				assert.Equal(t, "query_failed", current["error_code"])
				assert.NotContains(t, current, "available")
				assert.NotContains(t, current, "used")
				assert.NotContains(t, current, "total")
				points := data["points"].([]any)
				require.Len(t, points, 3)
				assert.NotContains(t, points[2].(map[string]any), "available")
				assert.Equal(t, float64(8), data["summary"].(map[string]any)["end_available"])
			})

			t.Run("resolved-empty-identity-is-exact", func(t *testing.T) {
				channelID := createQuotaHistoryFixtureChannel(t, db)
				for _, row := range []model.ChannelQuotaSnapshot{
					{ChannelId: channelID, ObservedAt: 100, Available: 100, MetricType: "balance", WindowType: "none", Source: "provider", Unit: "usd", Currency: "USD", Status: "success"},
					{ChannelId: channelID, ObservedAt: 160, Available: 90, MetricType: "balance", WindowType: "none", Source: "provider", Unit: "usd", Status: "success"},
					{ChannelId: channelID, ObservedAt: 220, MetricType: "balance", WindowType: "none", Source: "provider", Unit: "usd", PlanType: "other-plan", Status: "error"},
				} {
					require.NoError(t, db.Create(&row).Error)
				}
				data := readQuotaHistoryFixtureResponse(t, ChannelQuotaHistoryRequest{
					ChannelID: channelID, Start: 1, End: 300, Limit: 500, Granularity: QuotaHistoryRaw,
					RateWindowSeconds: 299, EWMAHalfLifeSeconds: 149,
				})
				assert.Equal(t, float64(1), data["raw_observations"])
				assert.Equal(t, "", data["currency"])
				assert.Equal(t, "", data["plan_type"])
				current := data["current"].(map[string]any)
				assert.Equal(t, "success", current["status"])
				assert.Equal(t, float64(160), current["observed_at"])
			})

			t.Run("empty-range-and-database-error", func(t *testing.T) {
				channelID := createQuotaHistoryFixtureChannel(t, db)
				request := ChannelQuotaHistoryRequest{ChannelID: channelID, Start: 1, End: 300, Limit: 500, Granularity: QuotaHistoryRaw, RateWindowSeconds: 299, EWMAHalfLifeSeconds: 149}
				data := readQuotaHistoryFixtureResponse(t, request)
				assert.Empty(t, data["points"])
				assert.NotContains(t, data, "summary")
				assert.NotContains(t, data, "current")
				assert.Equal(t, true, data["complete"])
				// The service must propagate a failed read rather than fabricate
				// the successful empty response above.
				model.DB = nil
				t.Cleanup(func() { model.DB = db })
				response, err := GetChannelQuotaHistory(context.Background(), request)
				require.ErrorIs(t, err, gorm.ErrInvalidDB)
				assert.Nil(t, response)
			})
		})
	}
}

func createQuotaHistoryFixtureChannel(t *testing.T, db *gorm.DB) int {
	t.Helper()
	channel := model.Channel{Name: "a4-history-fixture", Key: "history-fixture-secret"}
	require.NoError(t, db.Create(&channel).Error)
	t.Cleanup(func() {
		assert.NoError(t, db.Where("channel_id = ?", channel.Id).Delete(&model.ChannelQuotaSnapshot{}).Error)
		assert.NoError(t, db.Where("id = ?", channel.Id).Delete(&model.Channel{}).Error)
	})
	return channel.Id
}

func readQuotaHistoryFixtureRows(t *testing.T, db *gorm.DB, channelID int) []model.ChannelQuotaSnapshot {
	t.Helper()
	var rows []model.ChannelQuotaSnapshot
	require.NoError(t, db.Where("channel_id = ?", channelID).Order("id ASC").Find(&rows).Error)
	return rows
}

func readQuotaHistoryFixtureResponse(t *testing.T, request ChannelQuotaHistoryRequest) map[string]any {
	t.Helper()
	response, err := GetChannelQuotaHistory(context.Background(), request)
	require.NoError(t, err)
	payload, err := common.Marshal(response)
	require.NoError(t, err)
	require.NotContains(t, string(payload), "history-fixture-secret")
	var data map[string]any
	require.NoError(t, common.Unmarshal(payload, &data))
	return data
}
