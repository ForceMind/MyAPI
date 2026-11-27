package model

import (
	"database/sql"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestNormalizeSQLiteDriverDSN(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantExact   string
		wantBase    string
		wantPragmas []string
		forbidKey   string
	}{
		{
			name:        "translates default legacy busy_timeout",
			input:       common.DefaultSQLitePath,
			wantBase:    "my-api.db",
			wantPragmas: []string{"busy_timeout(30000)", "journal_mode(WAL)"},
			forbidKey:   "_busy_timeout",
		},
		{
			name:        "preserves custom legacy busy_timeout value",
			input:       "one-api.db?_busy_timeout=15000",
			wantBase:    "one-api.db",
			wantPragmas: []string{"busy_timeout(15000)", "journal_mode(WAL)"},
			forbidKey:   "_busy_timeout",
		},
		{
			name:        "adds default pragmas to bare file path",
			input:       "/data/my-api.db",
			wantBase:    "/data/my-api.db",
			wantPragmas: []string{"busy_timeout(30000)", "journal_mode(WAL)"},
		},
		{
			name:        "preserves explicit driver pragmas without duplicating",
			input:       "/data/my-api.db?_pragma=busy_timeout(5000)&_pragma=journal_mode(DELETE)",
			wantBase:    "/data/my-api.db",
			wantPragmas: []string{"busy_timeout(5000)", "journal_mode(DELETE)"},
		},
		{
			name:        "read-only URI skips WAL pragma",
			input:       "file:/data/my-api.db?mode=ro",
			wantBase:    "file:/data/my-api.db",
			wantPragmas: []string{"busy_timeout(30000)"},
		},
		{
			name:        "shared memory URI skips WAL pragma",
			input:       "file:memdb?mode=memory&cache=shared",
			wantBase:    "file:memdb",
			wantPragmas: []string{"busy_timeout(30000)"},
		},
		{
			name:      "bare in-memory DSN stays verbatim",
			input:     ":memory:",
			wantExact: ":memory:",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeSQLiteDriverDSN(tc.input)
			if tc.wantExact != "" {
				require.Equal(t, tc.wantExact, got)
				return
			}
			base, rawQuery, ok := strings.Cut(got, "?")
			require.True(t, ok, "expected query string in %q", got)
			assert.Equal(t, tc.wantBase, base)
			query, err := url.ParseQuery(rawQuery)
			require.NoError(t, err)
			if tc.forbidKey != "" {
				assert.False(t, query.Has(tc.forbidKey))
			}
			assert.ElementsMatch(t, tc.wantPragmas, query["_pragma"])
		})
	}
}

func TestChooseDBAppliesSQLiteBusyTimeoutWALAndPoolDefaults(t *testing.T) {
	previousPath := common.SQLitePath
	t.Cleanup(func() { common.SQLitePath = previousPath })

	dbFile := filepath.Join(t.TempDir(), "my-api.db")
	common.SQLitePath = dbFile + "?_busy_timeout=30000"
	t.Setenv("SQL_DSN", "")
	t.Setenv("SQL_MAX_OPEN_CONNS", "")
	t.Setenv("SQL_MAX_IDLE_CONNS", "")
	t.Setenv("SQL_MAX_LIFETIME", "")

	db, dbType, err := chooseDB("SQL_DSN", false)
	require.NoError(t, err)
	require.Equal(t, common.DatabaseTypeSQLite, dbType)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })

	configureDatabasePool(sqlDB, dbType)

	var busyTimeout int
	require.NoError(t, db.Raw("PRAGMA busy_timeout").Scan(&busyTimeout).Error)
	assert.Equal(t, 30000, busyTimeout)

	var journalMode string
	require.NoError(t, db.Raw("PRAGMA journal_mode").Scan(&journalMode).Error)
	assert.Equal(t, "wal", strings.ToLower(journalMode))

	assert.Equal(t, 1, sqlDB.Stats().MaxOpenConnections)

	t.Setenv("SQL_MAX_OPEN_CONNS", "4")
	configureDatabasePool(sqlDB, dbType)
	assert.Equal(t, 4, sqlDB.Stats().MaxOpenConnections)
}

func TestSQLiteNormalizedConnectionHandlesConcurrentWritesWithoutBusy(t *testing.T) {
	previousPath := common.SQLitePath
	t.Cleanup(func() { common.SQLitePath = previousPath })

	dbFile := filepath.Join(t.TempDir(), "concurrent.db")
	common.SQLitePath = dbFile + "?_busy_timeout=30000"
	t.Setenv("SQL_DSN", "")
	t.Setenv("SQL_MAX_OPEN_CONNS", "")
	t.Setenv("SQL_MAX_IDLE_CONNS", "")
	t.Setenv("SQL_MAX_LIFETIME", "")

	db, dbType, err := chooseDB("SQL_DSN", false)
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	configureDatabasePool(sqlDB, dbType)

	require.NoError(t, db.Exec("CREATE TABLE sqlite_lock_counter (id INTEGER PRIMARY KEY, count INTEGER NOT NULL)").Error)
	require.NoError(t, db.Exec("INSERT INTO sqlite_lock_counter (id, count) VALUES (1, 0)").Error)

	const workers = 8
	const iterations = 3
	errCh := make(chan error, workers*iterations)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				txErr := db.Transaction(func(tx *gorm.DB) error {
					var current int
					if err := tx.Raw("SELECT count FROM sqlite_lock_counter WHERE id = 1").Scan(&current).Error; err != nil {
						return err
					}
					return tx.Exec("UPDATE sqlite_lock_counter SET count = ? WHERE id = 1", current+1).Error
				})
				if txErr != nil {
					errCh <- txErr
				}
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for txErr := range errCh {
		require.NoError(t, txErr)
	}

	// Verify real PRAGMA busy_timeout waits cleanly for a brief external writer
	// instead of returning SQLITE_BUSY immediately.
	externalDB, err := sql.Open(sqlite.DriverName, dbFile)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, externalDB.Close()) })

	extTx, err := externalDB.Begin()
	require.NoError(t, err)
	_, err = extTx.Exec("UPDATE sqlite_lock_counter SET count = count + 1 WHERE id = 1")
	require.NoError(t, err)

	releaseDone := make(chan error, 1)
	go func() {
		time.Sleep(50 * time.Millisecond)
		releaseDone <- extTx.Commit()
	}()

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return tx.Exec("UPDATE sqlite_lock_counter SET count = count + 1 WHERE id = 1").Error
	}))
	require.NoError(t, <-releaseDone)

	var finalCount int
	require.NoError(t, db.Raw("SELECT count FROM sqlite_lock_counter WHERE id = 1").Scan(&finalCount).Error)
	assert.Equal(t, workers*iterations+2, finalCount)
}
