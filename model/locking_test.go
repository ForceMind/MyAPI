package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/utils/tests"
)

type namedDummyDialector struct {
	tests.DummyDialector
	name string
}

func (d namedDummyDialector) Name() string { return d.name }

// lockForUpdate must emit FOR UPDATE on databases that support it and skip
// it on SQLite, where the syntax does not exist.
//
// The dummy dialector is used because SQLite drivers strip locking clauses
// from the generated SQL, which would mask what the helper itself does.
func TestLockForUpdateEmitsRowLock(t *testing.T) {
	buildSQL := func(name string) string {
		dummyDB, err := gorm.Open(namedDummyDialector{name: name}, &gorm.Config{DryRun: true})
		require.NoError(t, err)
		var rows []Redemption
		return lockForUpdate(dummyDB).Where("id = ?", 1).Find(&rows).Statement.SQL.String()
	}

	assert.Contains(t, buildSQL("mysql"), "FOR UPDATE")
	assert.Contains(t, buildSQL("postgres"), "FOR UPDATE")
	assert.NotContains(t, buildSQL("sqlite"), "FOR UPDATE")
	assert.NotContains(t, buildSQL("unknown"), "FOR UPDATE")
}

func TestLockForUpdateUsesHandleDialectWhenGlobalTypeDiffers(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{DryRun: true})
	require.NoError(t, err)
	var rows []Redemption
	// A SQLite handle must never inherit FOR UPDATE from a stale global
	// configuration (the helper no longer consults that global value).
	assert.NotContains(t, lockForUpdate(db).Where("id = ?", 1).Find(&rows).Statement.SQL.String(), "FOR UPDATE")
}

func TestNormalizedEmailLockUsesCaseInsensitiveAvailabilityPredicate(t *testing.T) {
	db, err := gorm.Open(namedDummyDialector{name: "mysql"}, &gorm.Config{DryRun: true})
	require.NoError(t, err)

	var ids []int
	query := normalizedEmailLockQuery(db, "  Mixed@Example.COM ")
	statement := query.Find(&ids).Statement

	assert.Contains(t, statement.SQL.String(), "email_normalized = ?")
	assert.Contains(t, statement.SQL.String(), "FOR UPDATE")
	assert.Len(t, statement.Vars, 1)
	assert.Equal(t, "mixed@example.com", statement.Vars[0])
}
