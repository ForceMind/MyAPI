package model

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	sqlitedriver "github.com/glebarez/go-sqlite"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type sqliteReadOnlyURIContractEntry struct {
	Mode     os.FileMode
	Contents []byte
}

func sqliteReadOnlyURIContractFiles(t *testing.T, root string) map[string]sqliteReadOnlyURIContractEntry {
	t.Helper()
	entries := make(map[string]sqliteReadOnlyURIContractEntry)
	err := filepath.WalkDir(root, func(filename string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return fmt.Errorf("unexpected entry type in the owned SQLite fixture")
		}
		relative, err := filepath.Rel(root, filename)
		if err != nil {
			return err
		}
		var contents []byte
		if info.Mode().IsRegular() {
			contents, err = os.ReadFile(filename)
			if err != nil {
				return err
			}
		}
		entries[relative] = sqliteReadOnlyURIContractEntry{Mode: info.Mode(), Contents: contents}
		return nil
	})
	require.NoError(t, err, "observe all entries in the owned SQLite fixture")
	return entries
}

func TestSQLiteReadOnlyURIExistingFileContract(t *testing.T) {
	for _, test := range []struct {
		name      string
		directory string
		filename  string
	}{
		{name: "plain", filename: "marker.db"},
		{name: "unicode-space-hash-percent", directory: "库 中文 space#%25", filename: "marker 中文 #%.db"},
	} {
		for _, immutable := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/immutable_%t", test.name, immutable), func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				t.Cleanup(cancel)
				root := t.TempDir()
				directory := filepath.Join(root, test.directory)
				require.NoError(t, os.MkdirAll(directory, 0o700), "create the independent fixture directory")
				filename := filepath.Join(directory, test.filename)
				require.True(t, filepath.IsAbs(filename))

				// Setup uses the native filename, not the URI function under test
				// or a second URL builder that could share its platform behavior.
				writer, err := sql.Open(sqlite.DriverName, filename)
				require.NoError(t, err, "open the independent native-filename fixture")
				t.Cleanup(func() { assert.NoError(t, writer.Close()) })
				writer.SetMaxOpenConns(1)
				var journalMode string
				require.NoError(t, writer.QueryRowContext(ctx, "PRAGMA journal_mode=DELETE").Scan(&journalMode), "set the fixture journal mode")
				require.Equal(t, "delete", journalMode)
				_, err = writer.ExecContext(ctx, "CREATE TABLE uri_marker (id INTEGER PRIMARY KEY, marker TEXT NOT NULL)")
				require.NoError(t, err, "create the independent marker table")
				_, err = writer.ExecContext(ctx, "INSERT INTO uri_marker (id, marker) VALUES (?, ?)", 1, "initial")
				require.NoError(t, err, "seed the independent marker")
				marker := fmt.Sprintf("%s/%t/已存在 #%%", test.name, immutable)
				update := "UPDATE uri_marker SET marker = ? WHERE id = ?"
				_, err = writer.ExecContext(ctx, update, marker, 1)
				require.NoError(t, err, "the UPDATE is valid on the writable setup connection")
				var observed string
				require.NoError(t, writer.QueryRowContext(ctx, "SELECT marker FROM uri_marker WHERE id = ?", 1).Scan(&observed))
				require.Equal(t, marker, observed)
				require.NoError(t, writer.Close(), "close every fixture writer before either immutable mode")

				before := sqliteReadOnlyURIContractFiles(t, root)
				expectedNames := []string{".", filepath.Join(test.directory, test.filename)}
				if test.directory != "" {
					expectedNames = append(expectedNames, test.directory)
				}
				var observedNames []string
				for name := range before {
					observedNames = append(observedNames, name)
				}
				require.ElementsMatch(t, expectedNames, observedNames, "the closed DELETE-mode fixture has no sidecars or additional database")
				// Later connection cleanups run first, including if a URI connection
				// fails. Preserve the complete original tree on both red and green.
				t.Cleanup(func() {
					assert.Equal(t, before, sqliteReadOnlyURIContractFiles(t, root), "read-only access preserves every file byte, directory entry, type and mode")
				})

				reader, err := sql.Open(sqlite.DriverName, sqliteReadOnlyURI(filename, immutable))
				require.NoError(t, err, "construct the actual read-only URI connection")
				t.Cleanup(func() { assert.NoError(t, reader.Close()) })
				reader.SetMaxOpenConns(1)
				conn, err := reader.Conn(ctx)
				require.NoError(t, err, "connect through the URI after the independent fixture was closed")
				t.Cleanup(func() { assert.NoError(t, conn.Close()) })
				require.NoError(t, conn.PingContext(ctx), "the actual URI connection is usable")
				require.NoError(t, conn.QueryRowContext(ctx, "SELECT marker FROM uri_marker WHERE id = ?", 1).Scan(&observed), "read the pre-existing marker through the URI")
				require.Equal(t, marker, observed, "the URI opened the exact independently seeded database")

				// Do not enable query_only or start a read-only transaction: URI
				// behavior itself must reject these otherwise valid statements.
				_, err = conn.ExecContext(ctx, update, "forbidden mutation", 1)
				var updateError *sqlitedriver.Error
				require.ErrorAs(t, err, &updateError, "UPDATE must fail with a typed SQLite error")
				assert.Equal(t, 8, updateError.Code()&0xff, "UPDATE must return SQLITE_READONLY, not a lock or syntax error")
				_, err = conn.ExecContext(ctx, "CREATE TABLE uri_forbidden (id INTEGER PRIMARY KEY)")
				var createError *sqlitedriver.Error
				require.ErrorAs(t, err, &createError, "CREATE TABLE must fail with a typed SQLite error")
				assert.Equal(t, 8, createError.Code()&0xff, "CREATE TABLE must return SQLITE_READONLY")
				require.NoError(t, conn.QueryRowContext(ctx, "SELECT marker FROM uri_marker WHERE id = ?", 1).Scan(&observed))
				assert.Equal(t, marker, observed, "the rejected UPDATE did not change the original marker")
				var forbiddenTables int
				require.NoError(t, conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type = ? AND name = ?", "table", "uri_forbidden").Scan(&forbiddenTables))
				assert.Zero(t, forbiddenTables, "the rejected CREATE TABLE did not change the schema")
			})
		}
	}
}
