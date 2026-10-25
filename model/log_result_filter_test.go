package model

import (
	"os"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// logResultFilterContract seeds every row kind that shares LogTypeError and
// checks that result=failed keeps only request failures, while queries without
// a result keep their previous rows.
func logResultFilterContract(t *testing.T, db *gorm.DB) {
	t.Helper()
	preflight := common.MapToJsonStr(map[string]interface{}{
		"request_outcome": "rejected", "usage_outcome": "not_dispatched", "error_code": "request_input_limit_exceeded",
		"http_status": 400, "preflight_rejected": true,
	})
	require.Contains(t, preflight, `"preflight_rejected":true`)
	rows := []Log{
		{UserId: 1, Username: "alice", Type: LogTypeConsume, CreatedAt: 100, RequestId: "consume", Quota: 7, Other: `{"usage_accuracy":"reported"}`},
		{UserId: 1, Username: "alice", Type: LogTypeError, CreatedAt: 101, RequestId: "relay-error", Content: "status_code=502, bad gateway", Other: `{"error_code":"bad_response_status_code","status_code":502}`},
		{UserId: 1, Username: "alice", Type: LogTypeError, CreatedAt: 102, RequestId: "pending-review", Content: "usage_pending_review", Other: `{"usage_accuracy":"unknown","settlement_status":"pending_review","actual_quota":null}`},
		{UserId: 1, Username: "alice", Type: LogTypeError, CreatedAt: 103, RequestId: "settlement-pending", Content: "usage_settlement_pending", Other: `{"settlement_status":"pending_review","actual_quota":null}`},
		{UserId: 1, Username: "alice", Type: LogTypeError, CreatedAt: 104, RequestId: "preflight-rejected", Content: "estimated input exceeds the channel limit", Other: preflight},
		{UserId: 1, Username: "alice", Type: LogTypeError, CreatedAt: 105, RequestId: "historical-empty-other", Content: "upstream timeout"},
		{UserId: 1, Username: "alice", Type: LogTypeError, CreatedAt: 106, RequestId: "historical-null-columns", Content: "placeholder", Other: "placeholder"},
		{UserId: 1, Username: "alice", Type: LogTypeError, CreatedAt: 107, RequestId: "explicit-not-preflight", Content: "status_code=429", Other: `{"preflight_rejected":false,"status_code":429}`},
		{UserId: 1, Username: "alice", Type: LogTypeError, CreatedAt: 108, RequestId: "content-mentions-review", Content: "upstream said usage_pending_review", Other: `{"status_code":500}`},
		{UserId: 2, Username: "bob", Type: LogTypeError, CreatedAt: 109, RequestId: "foreign-relay-error", Content: "status_code=500", Other: `{"status_code":500}`},
		{UserId: 2, Username: "bob", Type: LogTypeError, CreatedAt: 110, RequestId: "foreign-pending-review", Content: "usage_pending_review", Other: `{"settlement_status":"pending_review"}`},
		{UserId: 1, Username: "alice", Type: LogTypeSystem, CreatedAt: 111, RequestId: "system", Content: "task request rejected before dispatch: quota saturation"},
	}
	for index := range rows {
		require.NoError(t, CreateLog(db, &rows[index]))
	}
	// Rows written before the content/other columns were always populated.
	require.NoError(t, db.Exec("UPDATE logs SET content = NULL, other = NULL WHERE request_id = ?", "historical-null-columns").Error)

	userFailed := []string{"relay-error", "historical-empty-other", "historical-null-columns", "explicit-not-preflight", "content-mentions-review"}
	allFailed := append(append([]string{}, userFailed...), "foreign-relay-error")
	allErrors := append(append([]string{}, allFailed...), "pending-review", "settlement-pending", "preflight-rejected", "foreign-pending-review")
	requestIDs := func(logs []*Log) []string {
		ids := make([]string, 0, len(logs))
		for _, log := range logs {
			ids = append(ids, log.RequestId)
		}
		return ids
	}

	for _, tt := range []struct {
		name    string
		query   func() ([]*Log, int64, error)
		wantIDs []string
	}{
		{name: "admin without result keeps every error row", query: func() ([]*Log, int64, error) {
			return GetAllLogs(LogTypeError, 0, 0, "", "", "", 0, 50, 0, "", "", "", "")
		}, wantIDs: allErrors},
		{name: "admin all types without result keeps every row", query: func() ([]*Log, int64, error) {
			return GetAllLogs(LogTypeUnknown, 0, 0, "", "", "", 0, 50, 0, "", "", "", "")
		}, wantIDs: append(append([]string{}, allErrors...), "consume", "system")},
		{name: "admin failed", query: func() ([]*Log, int64, error) {
			return GetAllLogs(LogTypeUnknown, 0, 0, "", "", "", 0, 50, 0, "", "", "", LogResultFailed)
		}, wantIDs: allFailed},
		{name: "admin failed with error type", query: func() ([]*Log, int64, error) {
			return GetAllLogs(LogTypeError, 0, 0, "", "", "", 0, 50, 0, "", "", "", LogResultFailed)
		}, wantIDs: allFailed},
		{name: "admin failed with consume type is empty", query: func() ([]*Log, int64, error) {
			return GetAllLogs(LogTypeConsume, 0, 0, "", "", "", 0, 50, 0, "", "", "", LogResultFailed)
		}, wantIDs: []string{}},
		{name: "admin failed narrowed by username", query: func() ([]*Log, int64, error) {
			return GetAllLogs(LogTypeUnknown, 0, 0, "", "bob", "", 0, 50, 0, "", "", "", LogResultFailed)
		}, wantIDs: []string{"foreign-relay-error"}},
		{name: "self without result keeps every own error row", query: func() ([]*Log, int64, error) {
			return GetUserLogs(1, LogTypeError, 0, 0, "", "", 0, 50, "", "", "", "")
		}, wantIDs: append(append([]string{}, userFailed...), "pending-review", "settlement-pending", "preflight-rejected")},
		{name: "self failed stays within the owner", query: func() ([]*Log, int64, error) {
			return GetUserLogs(1, LogTypeUnknown, 0, 0, "", "", 0, 50, "", "", "", LogResultFailed)
		}, wantIDs: userFailed},
		{name: "other owner failed", query: func() ([]*Log, int64, error) {
			return GetUserLogs(2, LogTypeUnknown, 0, 0, "", "", 0, 50, "", "", "", LogResultFailed)
		}, wantIDs: []string{"foreign-relay-error"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			logs, total, err := tt.query()
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.wantIDs, requestIDs(logs))
			assert.EqualValues(t, len(tt.wantIDs), total)
		})
	}

	for _, tt := range []struct {
		name        string
		filter      LogTokenStatisticsFilter
		wantRecords int64
	}{
		{name: "statistics without result", filter: LogTokenStatisticsFilter{UserID: 1}, wantRecords: int64(1 + len(userFailed) + 3)},
		{name: "statistics failed", filter: LogTokenStatisticsFilter{UserID: 1, Result: LogResultFailed}, wantRecords: int64(len(userFailed))},
		{name: "statistics failed for admin", filter: LogTokenStatisticsFilter{Result: LogResultFailed}, wantRecords: int64(len(allFailed))},
		{name: "statistics failed with consume type", filter: LogTokenStatisticsFilter{Type: LogTypeConsume, Result: LogResultFailed}, wantRecords: 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			stat, err := GetLogTokenStatistics(t.Context(), db, tt.filter)
			require.NoError(t, err)
			assert.Equal(t, tt.wantRecords, stat.Records)
		})
	}

	_, _, err := GetAllLogs(LogTypeUnknown, 0, 0, "", "", "", 0, 50, 0, "", "", "", "pending")
	assert.ErrorIs(t, err, gorm.ErrInvalidData)
	_, _, err = GetUserLogs(1, LogTypeUnknown, 0, 0, "", "", 0, 50, "", "", "", "pending")
	assert.ErrorIs(t, err, gorm.ErrInvalidData)
	_, err = GetLogTokenStatistics(t.Context(), db, LogTokenStatisticsFilter{Result: "pending"})
	assert.ErrorIs(t, err, gorm.ErrInvalidData)
}

func TestLogResultFilterSQLite(t *testing.T) {
	logResultFilterContract(t, useLogDedupTestDB(t))
}

func TestLogResultFilterConfiguredDatabases(t *testing.T) {
	if os.Getenv("MYAPI_S1_DATABASE_TESTS") != "1" {
		t.Skip("requires explicitly enabled disposable S1 databases")
	}
	for _, engine := range []struct {
		name, env string
		dbType    common.DatabaseType
	}{
		{"mysql", "MYAPI_S1_MYSQL_DSN", common.DatabaseTypeMySQL},
		{"postgres", "MYAPI_S1_POSTGRES_DSN", common.DatabaseTypePostgreSQL},
	} {
		t.Run(engine.name, func(t *testing.T) {
			dsn := os.Getenv(engine.env)
			require.NotEmpty(t, dsn)
			dialector, err := s1DatabaseDialector(engine.name, dsn)
			require.NoError(t, err)
			db, err := gorm.Open(dialector, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			tables, err := db.Migrator().GetTables()
			require.NoError(t, err)
			for _, table := range []string{"logs", clickHouseIdentityTable} {
				require.NotContains(t, tables, table, "refusing an existing log schema")
			}
			require.NoError(t, db.AutoMigrate(&Log{}, &BillingLogProjectionIdentity{}))
			// The disposable database is shared with later log tests in the
			// same job, which also refuse an existing log schema.
			t.Cleanup(func() {
				require.NoError(t, db.Migrator().DropTable(&Log{}, &BillingLogProjectionIdentity{}))
				require.NoError(t, sqlDB.Close())
			})

			previousDB, previousLogDB := DB, LOG_DB
			previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
			previousMemoryCache := common.MemoryCacheEnabled
			DB, LOG_DB = db, db
			common.SetDatabaseTypes(engine.dbType, engine.dbType)
			common.MemoryCacheEnabled = false
			initCol()
			t.Cleanup(func() {
				DB, LOG_DB = previousDB, previousLogDB
				common.SetDatabaseTypes(previousMainType, previousLogType)
				common.MemoryCacheEnabled = previousMemoryCache
				initCol()
			})

			logResultFilterContract(t, db)
		})
	}
}
