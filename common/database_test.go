package common

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveSQLitePathPrefersExplicitConfiguration(t *testing.T) {
	configured := "file:/var/lib/myapi/custom.db?cache=shared"
	got := resolveSQLitePath(configured, func(string) bool { return true })
	require.Equal(t, configured, got)
}

func TestResolveSQLitePathUsesCanonicalForFreshInstall(t *testing.T) {
	got := resolveSQLitePath("", func(string) bool { return false })
	require.Equal(t, DefaultSQLitePath, got)
}

func TestResolveSQLitePathFallsBackToLegacyFile(t *testing.T) {
	got := resolveSQLitePath("", func(path string) bool {
		return path == sqliteDSNFile(LegacySQLitePath)
	})
	require.Equal(t, LegacySQLitePath, got)
}

func TestResolveSQLitePathPrefersCanonicalWhenBothFilesExist(t *testing.T) {
	got := resolveSQLitePath("", func(string) bool { return true })
	require.Equal(t, DefaultSQLitePath, got)
}
