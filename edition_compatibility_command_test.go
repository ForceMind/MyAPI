package main

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/service"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func editionCommandDatabase(t *testing.T, status string) string {
	t.Helper()
	file := filepath.Join(t.TempDir(), "primary.db")
	db, err := sql.Open(sqlite.DriverName, file)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	for _, statement := range []string{
		`CREATE TABLE top_ups (id INTEGER PRIMARY KEY, status TEXT)`,
		`CREATE TABLE subscription_orders (id INTEGER PRIMARY KEY, status TEXT)`,
		`CREATE TABLE users (id INTEGER PRIMARY KEY, role INTEGER, status INTEGER, password TEXT, deleted_at DATETIME)`,
		`CREATE TABLE options (key TEXT PRIMARY KEY, value TEXT)`,
		`CREATE TABLE passkey_credentials (user_id INTEGER, credential_id TEXT, public_key TEXT, deleted_at DATETIME)`,
	} {
		_, err := db.Exec(statement)
		require.NoError(t, err)
	}
	password, err := common.Password2Hash("synthetic-local-password")
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO users (id,role,status,password) VALUES (1,100,1,?)`, password)
	require.NoError(t, err)
	if status != "" {
		_, err = db.Exec(`INSERT INTO top_ups (id,status) VALUES (1,?)`, status)
		require.NoError(t, err)
	}
	require.NoError(t, db.Close())
	return file
}

func TestEditionCompatibilityCommandUsesResolvedPrimaryWithoutInitialization(t *testing.T) {
	for _, tc := range []struct {
		status, state string
		code          int
	}{{"", "compatible", 0}, {"pending", "blocked", 1}, {"UNRECOGNIZED", "unverifiable", 2}} {
		t.Run(tc.state, func(t *testing.T) {
			file := editionCommandDatabase(t, tc.status)
			directory := t.TempDir()
			t.Chdir(directory)
			t.Setenv("SQL_DSN", "local")
			t.Setenv("SQLITE_PATH", file)
			t.Setenv("MYAPI_EDITION", "full")
			t.Setenv("SESSION_SECRET", "random_string") // InitEnv would terminate; a read-only check must not invoke it.
			previousPath, previousDB := common.SQLitePath, model.DB
			before, err := os.ReadFile(file)
			require.NoError(t, err)
			var output bytes.Buffer
			handled, code := handleEditionCompatibilityCommand([]string{"--edition-check=lite", "--json"}, &output)
			require.True(t, handled)
			assert.Equal(t, tc.code, code)
			var report service.EditionCompatibilityReport
			require.NoError(t, common.Unmarshal(output.Bytes(), &report))
			assert.Equal(t, tc.state, report.State)
			require.NotNil(t, report.SourceEdition)
			assert.Equal(t, "full", *report.SourceEdition)
			assert.False(t, report.RuntimeLoginVerified)
			assert.Equal(t, "configuration", report.Scope)
			after, err := os.ReadFile(file)
			require.NoError(t, err)
			assert.Equal(t, before, after)
			assert.Equal(t, previousPath, common.SQLitePath)
			assert.Same(t, previousDB, model.DB)
			entries, err := os.ReadDir(directory)
			require.NoError(t, err)
			assert.Empty(t, entries, "the check must not create logs, configuration or data in its working directory")
			assert.NotContains(t, output.String(), "synthetic-local-password")
			assert.NotContains(t, output.String(), "$2")
			assert.NotContains(t, output.String(), "random_string")
		})
	}
}

func TestEditionCompatibilityCommandKeepsNormalArgumentsAndRejectsMixedStartup(t *testing.T) {
	var output bytes.Buffer
	handled, _ := handleEditionCompatibilityCommand([]string{"--port=3000"}, &output)
	assert.False(t, handled)
	assert.Empty(t, output.String())
	for _, args := range [][]string{{"--json", "--edition-check=lite", "positional"}, {"--json", "--edition-check=lite", "--port=3000"}} {
		output.Reset()
		handled, code := handleEditionCompatibilityCommand(args, &output)
		require.True(t, handled)
		assert.Equal(t, 2, code)
		var report service.EditionCompatibilityReport
		require.NoError(t, common.Unmarshal(output.Bytes(), &report))
		assert.Equal(t, []string{"invalid-request"}, report.Unknowns)
		assert.Nil(t, report.Facts)
		assert.Nil(t, report.TargetEdition)
	}
}

func TestEditionCompatibilityHumanReportKeepsUnknownCounts(t *testing.T) {
	var output bytes.Buffer
	err := writeEditionCompatibilitySummary(&output, service.EditionCompatibilityReport{State: "unverifiable", Facts: &model.EditionCompatibilityFacts{Schema: "unavailable", Source: "configured-primary"}, Unknowns: []string{"database-unavailable"}})
	require.NoError(t, err)
	assert.Contains(t, output.String(), "Top-up orders: unknown")
	assert.Contains(t, output.String(), "Subscription purchase orders: unknown")
	assert.Contains(t, output.String(), "Local login configuration: unknown")
	assert.NotContains(t, output.String(), "0 pending")
}

func TestEditionCompatibilityMainProcess(t *testing.T) {
	mode := os.Getenv("MYAPI_EDITION_COMMAND_CHILD")
	if mode == "1" || mode == "blocked-startup" {
		if mode == "1" {
			os.Args = []string{os.Args[0], "--edition-check=lite", "--json"}
		} else {
			os.Args = []string{os.Args[0], "--log-dir", filepath.Join(os.Getenv("MYAPI_EDITION_COMMAND_DIRECTORY"), "logs")}
		}
		main()
		return
	}
	directory := t.TempDir()
	file := filepath.Join(directory, "new-primary.db")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestEditionCompatibilityMainProcess$")
	command.Dir = directory
	command.Env = append(os.Environ(), "MYAPI_EDITION_COMMAND_CHILD=1", "MYAPI_EDITION=full", "SQL_DSN=local", "SQLITE_PATH="+file, "SESSION_SECRET=random_string")
	output, err := command.CombinedOutput()
	require.NoError(t, err, "standalone checker must exit without ordinary startup: %s", output)
	var report service.EditionCompatibilityReport
	require.NoError(t, common.Unmarshal(output, &report), fmt.Sprintf("checker output: %s", output))
	assert.Equal(t, "compatible", report.State)
	require.NotNil(t, report.Facts)
	assert.Equal(t, "fresh", report.Facts.Schema)
	assert.False(t, report.RuntimeLoginVerified)
	entries, err := os.ReadDir(directory)
	require.NoError(t, err)
	assert.Empty(t, entries, "actual main flag must not create the database, logs or a service")
}

func TestEditionCompatibilityLiteStartupStopsBeforeMigration(t *testing.T) {
	file := editionCommandDatabase(t, "pending")
	before, err := os.ReadFile(file)
	require.NoError(t, err)
	directory := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestEditionCompatibilityMainProcess$")
	command.Dir = directory
	command.Env = append(os.Environ(), "MYAPI_EDITION_COMMAND_CHILD=blocked-startup", "MYAPI_EDITION_COMMAND_DIRECTORY="+directory,
		"MYAPI_EDITION=lite", "SQL_DSN=local", "SQLITE_PATH="+file, "SESSION_SECRET=synthetic-startup-secret-for-edition-check",
		"SESSION_COOKIE_TRUSTED_URL=https://edition.example.test", "SESSION_COOKIE_SECURE=true", "FRONTEND_BASE_URL=https://edition.example.test")
	output, err := command.CombinedOutput()
	var exit *exec.ExitError
	require.ErrorAs(t, err, &exit)
	assert.Equal(t, 1, exit.ExitCode())
	assert.Contains(t, string(output), "Lite configuration check blocked (pending-top-ups)")
	assert.NotContains(t, string(output), "started")
	assert.NotContains(t, string(output), "synthetic-startup-secret")
	after, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Equal(t, before, after, "the real startup branch must stop before schema or accounting initialization")
	entries, err := os.ReadDir(directory)
	require.NoError(t, err)
	for _, entry := range entries {
		assert.Equal(t, "logs", entry.Name())
		assert.True(t, entry.IsDir())
		logs, err := os.ReadDir(filepath.Join(directory, entry.Name()))
		require.NoError(t, err)
		assert.Empty(t, logs, "the compatibility gate runs before the logger and other initializers")
	}
}
