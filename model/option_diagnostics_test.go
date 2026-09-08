package model

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestReadOptionDiagnosticRowsIsBoundedAndReadOnly(t *testing.T) {
	db := optionDiagnosticsSQLiteFixture(t)
	require.NoError(t, db.Exec("CREATE TABLE options (`key` text primary key, value text)").Error)
	require.NoError(t, db.Create(&Option{Key: "a", Value: "one"}).Error)
	require.NoError(t, db.Exec("INSERT INTO options (`key`, value) VALUES (?, NULL)", "b").Error)
	require.NoError(t, db.Create(&Option{Key: "large", Value: strings.Repeat("x", OptionDiagnosticValueParseCapChars+1)}).Error)

	writes := 0
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("option-diagnostics-create", func(*gorm.DB) { writes++ }))
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("option-diagnostics-update", func(*gorm.DB) { writes++ }))
	require.NoError(t, db.Callback().Delete().Before("gorm:delete").Register("option-diagnostics-delete", func(*gorm.DB) { writes++ }))

	// SQLite query_only rejects INSERT/UPDATE/DELETE issued through either GORM
	// callbacks or a raw Exec path, while permitting the diagnostics SELECT.
	require.NoError(t, db.Exec("PRAGMA query_only = ON").Error)
	rows, complete, err := ReadOptionDiagnosticRows()
	require.NoError(t, err)
	require.True(t, complete)
	require.Len(t, rows, 3)
	assert.Equal(t, []string{"a", "b", "large"}, []string{rows[0].Key, rows[1].Key, rows[2].Key})
	assert.True(t, rows[1].Value.Valid == false)
	assert.True(t, rows[2].TooLarge)
	assert.Equal(t, OptionDiagnosticValueParseCapChars+1, len([]rune(rows[2].Value.String)))
	assert.Zero(t, writes)
}

func TestReadOptionDiagnosticRowsBoundsOversizedKeysInSQLite(t *testing.T) {
	db := optionDiagnosticsSQLiteFixture(t)
	require.NoError(t, db.Exec("CREATE TABLE options (`key` text primary key, value text)").Error)

	const tailCanary = "c09-oversized-key-tail-canary"
	exactKey := strings.Repeat("a", OptionDiagnosticKeyParseCapChars)
	oversizedKey := strings.Repeat("b", OptionDiagnosticKeyParseCapChars) + tailCanary
	require.NoError(t, db.Create(&Option{Key: exactKey, Value: "exact"}).Error)
	require.NoError(t, db.Create(&Option{Key: oversizedKey, Value: "oversized"}).Error)

	rows, complete, err := ReadOptionDiagnosticRows()
	require.NoError(t, err)
	require.True(t, complete)
	require.Len(t, rows, 2)
	assert.Equal(t, exactKey, rows[0].Key)
	assert.False(t, rows[0].KeyTooLarge)
	assert.Equal(t, strings.Repeat("b", OptionDiagnosticKeyParseCapChars), rows[1].Key)
	assert.True(t, rows[1].KeyTooLarge)
	assert.Len(t, []rune(rows[1].Key), OptionDiagnosticKeyParseCapChars)
	assert.NotContains(t, rows[1].Key, tailCanary)
}

func TestReadOptionDiagnosticRowsMarksAnIncompleteSnapshotAtTheRowLimit(t *testing.T) {
	db := optionDiagnosticsSQLiteFixture(t)
	require.NoError(t, db.Exec("CREATE TABLE options (`key` text primary key, value text)").Error)
	options := make([]Option, 0, OptionDiagnosticRowLimit+1)
	for index := 0; index <= OptionDiagnosticRowLimit; index++ {
		options = append(options, Option{Key: fmt.Sprintf("key-%04d", index), Value: "value"})
	}
	require.NoError(t, db.CreateInBatches(&options, 128).Error)

	rows, complete, err := ReadOptionDiagnosticRows()
	require.NoError(t, err)
	assert.False(t, complete)
	assert.Len(t, rows, OptionDiagnosticRowLimit)
}

func TestOptionDiagnosticRowsNeverMarshalDatabaseFields(t *testing.T) {
	const canary = "c09-row-canary-secret"
	encoded, err := common.Marshal(OptionDiagnosticRow{Key: "c09-key-canary", KeyTooLarge: true, Value: sql.NullString{String: canary, Valid: true}, TooLarge: true})
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), canary)
	assert.NotContains(t, string(encoded), "c09-key-canary")
}

func TestAssessOptionDiagnosticAliasGroupsMatchesLoadPrecedence(t *testing.T) {
	for _, pair := range groupRatioOptionPairs {
		validCanonical, validAlias, differentAlias := optionDiagnosticAliasValues(pair)
		for _, testCase := range []struct {
			name, status, source string
			canonical, alias     *OptionDiagnosticRow
		}{
			{"absent", "absent", "unresolved", nil, nil},
			{"canonical only", "canonical_valid_alias_absent", "canonical", optionDiagnosticAliasRow(pair.canonical, validCanonical), nil},
			{"canonical alias invalid", "canonical_valid_alias_invalid", "canonical", optionDiagnosticAliasRow(pair.canonical, validCanonical), optionDiagnosticAliasRow(pair.alias, "{")},
			{"canonical alias normalized matching", "canonical_valid_alias_matching", "canonical", optionDiagnosticAliasRow(pair.canonical, validCanonical), optionDiagnosticAliasRow(pair.alias, validAlias)},
			{"canonical alias conflicting", "canonical_valid_alias_conflicting", "canonical", optionDiagnosticAliasRow(pair.canonical, validCanonical), optionDiagnosticAliasRow(pair.alias, differentAlias)},
			{"canonical invalid alias valid", "canonical_invalid_alias_valid", "alias", optionDiagnosticAliasRow(pair.canonical, "{"), optionDiagnosticAliasRow(pair.alias, validAlias)},
			{"canonical absent alias valid", "canonical_absent_alias_valid", "alias", nil, optionDiagnosticAliasRow(pair.alias, validAlias)},
			{"canonical raw null alias valid", "source_indeterminate", "unresolved", &OptionDiagnosticRow{Key: pair.canonical}, optionDiagnosticAliasRow(pair.alias, validAlias)},
			{"canonical invalid utf8 alias valid", "source_indeterminate", "unresolved", optionDiagnosticAliasRow(pair.canonical, string([]byte{0xff})), optionDiagnosticAliasRow(pair.alias, validAlias)},
			{"both invalid", "canonical_invalid_alias_invalid", "runtime_boot_fallback", optionDiagnosticAliasRow(pair.canonical, "{"), optionDiagnosticAliasRow(pair.alias, "{")},
			{"canonical invalid alias absent", "canonical_invalid_alias_absent", "runtime_boot_fallback", optionDiagnosticAliasRow(pair.canonical, "{"), nil},
			{"canonical absent alias raw null", "source_indeterminate", "unresolved", nil, &OptionDiagnosticRow{Key: pair.alias}},
			{"canonical valid alias blank", "canonical_valid_alias_invalid", "canonical", optionDiagnosticAliasRow(pair.canonical, validCanonical), optionDiagnosticAliasRow(pair.alias, " \t")},
			{"canonical valid alias invalid utf8", "source_indeterminate", "unresolved", optionDiagnosticAliasRow(pair.canonical, validCanonical), optionDiagnosticAliasRow(pair.alias, string([]byte{0xff}))},
			{"canonical valid alias too large", "source_indeterminate", "unresolved", optionDiagnosticAliasRow(pair.canonical, validCanonical), &OptionDiagnosticRow{Key: pair.alias, Value: sql.NullString{String: validAlias, Valid: true}, TooLarge: true}},
		} {
			t.Run(pair.canonical+"/"+testCase.name, func(t *testing.T) {
				rows := make([]OptionDiagnosticRow, 0, 2)
				if testCase.canonical != nil {
					rows = append(rows, *testCase.canonical)
				}
				if testCase.alias != nil {
					rows = append(rows, *testCase.alias)
				}
				assessment := AssessOptionDiagnosticAliasGroups(rows, true)
				require.Len(t, assessment, 2)
				var actual OptionDiagnosticAliasAssessment
				for _, candidate := range assessment {
					if candidate.Keys[0] == pair.canonical {
						actual = candidate
						break
					}
				}
				assert.Equal(t, testCase.status, actual.Status)
				assert.Equal(t, testCase.source, actual.EffectiveSource)
			})
		}
	}
}

func TestAssessOptionDiagnosticAliasGroupsKeepsTruncatedSourcesIndeterminate(t *testing.T) {
	for _, pair := range groupRatioOptionPairs {
		validCanonical, validAlias, _ := optionDiagnosticAliasValues(pair)
		for _, testCase := range []struct {
			name string
			rows []OptionDiagnosticRow
		}{
			{"canonical too large alias valid", []OptionDiagnosticRow{{Key: pair.canonical, Value: sql.NullString{String: validCanonical, Valid: true}, TooLarge: true}, *optionDiagnosticAliasRow(pair.alias, validAlias)}},
			{"canonical too large alone", []OptionDiagnosticRow{{Key: pair.canonical, Value: sql.NullString{String: validCanonical, Valid: true}, TooLarge: true}}},
			{"canonical absent alias too large", []OptionDiagnosticRow{{Key: pair.alias, Value: sql.NullString{String: validAlias, Valid: true}, TooLarge: true}}},
		} {
			t.Run(pair.canonical+"/"+testCase.name, func(t *testing.T) {
				assessment := AssessOptionDiagnosticAliasGroups(testCase.rows, true)
				for _, candidate := range assessment {
					if candidate.Keys[0] == pair.canonical {
						assert.Equal(t, "source_indeterminate", candidate.Status)
						assert.Equal(t, "unresolved", candidate.EffectiveSource)
					}
				}
			})
		}
	}
}

func TestAssessOptionDiagnosticAliasGroupsMarksTruncatedCoverage(t *testing.T) {
	rows := make([]OptionDiagnosticRow, 0, OptionDiagnosticRowLimit)
	for index := 0; index < OptionDiagnosticRowLimit; index++ {
		rows = append(rows, OptionDiagnosticRow{Key: fmt.Sprintf("key-%04d", index), Value: sql.NullString{String: "value", Valid: true}})
	}
	rows[0] = *optionDiagnosticAliasRow(groupRatioOptionKey, `{"default":1}`)
	assessment := AssessOptionDiagnosticAliasGroups(rows, false)
	require.Len(t, assessment, 2)
	for _, group := range assessment {
		assert.Equal(t, "coverage_incomplete", group.Status)
		assert.Equal(t, "unresolved", group.EffectiveSource)
	}
}

func TestAssessOptionDiagnosticAliasGroupsIgnoresOversizedKeys(t *testing.T) {
	rows := []OptionDiagnosticRow{
		*optionDiagnosticAliasRow(groupRatioOptionKey, `{"default":1}`),
		{Key: groupRatioOptionKey, KeyTooLarge: true, Value: sql.NullString{String: `{"default":2}`, Valid: true}},
	}
	assessment := AssessOptionDiagnosticAliasGroups(rows, true)
	require.Len(t, assessment, 2)
	assert.Equal(t, "canonical_valid_alias_absent", assessment[0].Status)
	assert.Equal(t, "canonical", assessment[0].EffectiveSource)
}

func optionDiagnosticAliasRow(key, value string) *OptionDiagnosticRow {
	return &OptionDiagnosticRow{Key: key, Value: sql.NullString{String: value, Valid: true}}
}

func optionDiagnosticAliasValues(pair groupRatioOptionPair) (string, string, string) {
	if pair.canonical == groupRatioOptionKey {
		return `{"default":1}`, ` { "default" : 1.0 } `, `{"default":2}`
	}
	return `{"vip":{"default":1}}`, ` { "vip" : { "default" : 1.0 } } `, `{"vip":{"default":2}}`
}

func optionDiagnosticsSQLiteFixture(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB, previousType := DB, common.MainDatabaseType()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		DB = previousDB
		common.SetMainDatabaseType(previousType)
		require.NoError(t, sqlDB.Close())
	})
	return db
}
