package model

import (
	"database/sql"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// TestPostgresRepeatedMigrationPreservesDefaults uses the real model tags and
// server metadata: a driver that repeatedly rewrites an unchanged default can
// otherwise pass row-value assertions while locking production tables at startup.
func TestPostgresRepeatedMigrationPreservesDefaults(t *testing.T) {
	if os.Getenv("MYAPI_B2_DATABASE_TESTS") != "1" {
		t.Skip("requires explicitly configured disposable B2 database")
	}
	dsn := strings.TrimSpace(os.Getenv("MYAPI_B2_POSTGRES_DSN"))
	require.NotEmpty(t, dsn)
	dialector, err := b2SubmissionDatabaseDialector("postgres", dsn)
	require.NoError(t, err)
	recorder := &migrationSQLRecorder{}
	db, err := gorm.Open(dialector, &gorm.Config{Logger: recorder})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })

	// The owned schema isolates explicit production table and index names from
	// other B2 contracts. Every table access is qualified; no search_path change
	// or cleanup of existing fixture tables is needed.
	schemaName := fmt.Sprintf("migration_defaults_%d", time.Now().UnixNano())
	require.NoError(t, db.Exec("CREATE SCHEMA ?", clause.Table{Name: schemaName}).Error)
	t.Cleanup(func() {
		require.NoError(t, db.Exec("DROP SCHEMA ? CASCADE", clause.Table{Name: schemaName}).Error)
	})

	for _, fixture := range []struct {
		name        string
		primaryKey  string
		existingID  any
		defaultID   any
		existingRow any
		defaultRow  any
		defaults    map[string]string
	}{
		{
			name: "channel_quota_snapshots", primaryKey: "id", existingID: 101, defaultID: 102,
			existingRow: &ChannelQuotaSnapshot{Id: 101, ChannelId: 7, Available: 37.5,
				Unit: "percent", MetricType: "codex_rate_limit", WindowType: "five_hour", Status: "error",
				CodexObservationQualified: true, CodexThresholdQualified: true},
			defaultRow: &ChannelQuotaSnapshot{Id: 102, ChannelId: 8},
			defaults:   map[string]string{"unit": "usd", "metric_type": "balance", "window_type": "none", "status": "success"},
		},
		{
			name: "token_budgets", primaryKey: "token_id", existingID: 101, defaultID: 102,
			existingRow: &TokenBudget{TokenID: 101, UserID: 7, FeeEnabled: true,
				FeeLimitUSD: "123.45", FeeUsedUSD: "12.5", FeeReservedUSD: "2.25", Revision: 9},
			defaultRow: &TokenBudget{TokenID: 102, UserID: 8},
			defaults:   map[string]string{"fee_limit_usd": "0", "fee_used_usd": "0", "fee_reserved_usd": "0"},
		},
		{
			name: "token_budget_reservations", primaryKey: "request_id", existingID: "existing", defaultID: "default",
			existingRow: &TokenBudgetReservation{RequestID: "existing", TokenID: 101, UserID: 7,
				FeeEnabled: true, FeeReservedUSD: "2.25", RequestServiceTier: "priority", BoundSource: "fixture"},
			defaultRow: &TokenBudgetReservation{RequestID: "default", TokenID: 102, UserID: 8},
			defaults:   map[string]string{"fee_reserved_usd": "0", "request_service_tier": "", "bound_source": ""},
		},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			table := schemaName + "." + fixture.name
			require.NoError(t, db.Table(table).AutoMigrate(fixture.existingRow))
			require.NoError(t, db.Table(table).Create(fixture.existingRow).Error)
			beforeRow := reflect.New(reflect.TypeOf(fixture.existingRow).Elem()).Interface()
			require.NoError(t, db.Table(table).Where(fixture.primaryKey+" = ?", fixture.existingID).First(beforeRow).Error)
			beforeColumns := postgresMigrationDefaultMetadata(t, db, schemaName, fixture.name)
			columns := make([]string, 0, len(fixture.defaults))
			for column, value := range fixture.defaults {
				actual, exists := beforeColumns[column]
				require.True(t, exists, "missing default metadata for %s", column)
				require.True(t, actual.Valid, "missing SQL default for %s", column)
				assert.Equal(t, "'"+value+"'::character varying", actual.String, column)
				columns = append(columns, column)
			}
			sort.Strings(columns)

			for repeat := 1; repeat <= 2; repeat++ {
				recorder.reset()
				require.NoError(t, db.Table(table).AutoMigrate(fixture.existingRow))
				var defaultChanges []string
				for _, statement := range recorder.schemaMutations() {
					if strings.Contains(strings.ToUpper(statement), " SET DEFAULT ") {
						defaultChanges = append(defaultChanges, statement)
					}
				}
				assert.Empty(t, defaultChanges, "unchanged migration %d must not rewrite existing defaults", repeat)
				assert.Equal(t, beforeColumns, postgresMigrationDefaultMetadata(t, db, schemaName, fixture.name))
				afterRow := reflect.New(reflect.TypeOf(fixture.existingRow).Elem()).Interface()
				require.NoError(t, db.Table(table).Where(fixture.primaryKey+" = ?", fixture.existingID).First(afterRow).Error)
				assert.Equal(t, beforeRow, afterRow, "unchanged migration must preserve stored values")
			}

			// Omit the fields from INSERT so PostgreSQL, rather than GORM's
			// model-default population, supplies their values after both repeats.
			require.NoError(t, db.Table(table).Omit(columns...).Create(fixture.defaultRow).Error)
			var defaultValues map[string]any
			require.NoError(t, db.Table(table).Select(columns).Where(fixture.primaryKey+" = ?", fixture.defaultID).Take(&defaultValues).Error)
			for column, expected := range fixture.defaults {
				assert.Equal(t, expected, defaultValues[column], "database default for %s", column)
			}
		})
	}
}

func postgresMigrationDefaultMetadata(t *testing.T, db *gorm.DB, schemaName, tableName string) map[string]sql.NullString {
	t.Helper()
	var columns []struct {
		ColumnName    string
		ColumnDefault sql.NullString
	}
	// Inspect PostgreSQL's actual expressions independently of the driver's
	// ColumnTypes default normalization, which is the regression under test.
	require.NoError(t, db.Table("information_schema.columns").Select("column_name", "column_default").
		Where("table_schema = ? AND table_name = ?", schemaName, tableName).Find(&columns).Error)
	require.NotEmpty(t, columns)
	defaults := make(map[string]sql.NullString, len(columns))
	for _, column := range columns {
		defaults[column.ColumnName] = column.ColumnDefault
	}
	return defaults
}
