package service

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestOptionDiagnosticsDoesNotPublishOrExposeValues(t *testing.T) {
	previousDB, previousType := model.DB, common.MainDatabaseType()
	previousGinMode := gin.Mode()
	common.OptionMapRWMutex.RLock()
	var previousOptionMap map[string]string
	if common.OptionMap != nil {
		previousOptionMap = make(map[string]string, len(common.OptionMap))
		for key, value := range common.OptionMap {
			previousOptionMap[key] = value
		}
	}
	common.OptionMapRWMutex.RUnlock()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	model.DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() {
		model.DB = previousDB
		common.SetMainDatabaseType(previousType)
		gin.SetMode(previousGinMode)
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptionMap
		common.OptionMapRWMutex.Unlock()
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.Exec("CREATE TABLE options (`key` text primary key, value text)").Error)
	const canary = "c09-canary-secret-must-not-leak"
	for _, option := range []model.Option{
		{Key: "passkey.enabled", Value: "invalid"},
		{Key: "global.pass_through_request_enabled", Value: "invalid"},
		{Key: "unknown.prefix", Value: canary},
		{Key: "unknown." + canary, Value: "value"},
		{Key: strings.Repeat("k", model.OptionDiagnosticKeyParseCapChars) + canary, Value: "value"},
		{Key: "GroupRatio", Value: `{"default":1}`},
		{Key: "group_ratio_setting.group_ratio", Value: `{"default":1}`},
		{Key: "theme.frontend", Value: "classic"},
		{Key: "global.thinking_model_blacklist", Value: "{"},
		{Key: "blank", Value: " \t"},
		{Key: "large", Value: strings.Repeat("x", model.OptionDiagnosticValueParseCapChars+1)},
	} {
		require.NoError(t, db.Create(&option).Error)
	}
	require.NoError(t, db.Exec("INSERT INTO options (`key`, value) VALUES (?, NULL)", "raw-null").Error)

	common.OptionMapRWMutex.RLock()
	var optionMapBefore map[string]string
	if common.OptionMap != nil {
		optionMapBefore = make(map[string]string, len(common.OptionMap))
		for key, value := range common.OptionMap {
			optionMapBefore[key] = value
		}
	}
	common.OptionMapRWMutex.RUnlock()
	passkeyBefore := system_setting.GetPasskeySettings()

	diagnostics, err := GetOptionDiagnostics(true)
	require.NoError(t, err)
	encoded, err := common.Marshal(diagnostics)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), canary)
	assert.NotContains(t, string(encoded), `{"default":1}`)
	assert.Equal(t, "c09-option-diagnostic-v1", diagnostics.SchemaVersion)
	assert.Equal(t, "primary_database.options", diagnostics.Source)
	assert.Equal(t, "read_only", diagnostics.Mode)
	assert.Equal(t, model.OptionDiagnosticKeyParseCapChars, diagnostics.Coverage.KeyParseCapChars)
	assert.Equal(t, model.OptionDiagnosticValueParseCapChars, diagnostics.Coverage.ValueParseCapChars)
	assert.Contains(t, diagnostics.Summary.ByValidation, "invalid_boolean")
	assert.Contains(t, diagnostics.Summary.ByValidation, "invalid_json")
	assert.Equal(t, 1, diagnostics.Summary.ByKeyClass["malformed_key"])

	common.OptionMapRWMutex.RLock()
	assert.Equal(t, optionMapBefore, common.OptionMap)
	common.OptionMapRWMutex.RUnlock()
	assert.Equal(t, passkeyBefore, system_setting.GetPasskeySettings())
	require.Len(t, diagnostics.AliasGroups, 2)
	assert.Equal(t, "canonical_valid_alias_matching", diagnostics.AliasGroups[0].Status)
	oversizedKeyFinding := false
	for _, item := range diagnostics.Items {
		if item.KeyClass == "unknown_prefix" {
			assert.Empty(t, item.Key)
			assert.True(t, item.KeyRedacted)
		}
		if item.KeyClass == "malformed_key" {
			oversizedKeyFinding = true
			assert.Empty(t, item.Key)
			assert.True(t, item.KeyRedacted)
		}
	}
	assert.True(t, oversizedKeyFinding)
}

func TestOptionDiagnosticValueStates(t *testing.T) {
	for _, testCase := range []struct {
		name, expected string
		row            model.OptionDiagnosticRow
	}{
		{"raw null", "raw_null", model.OptionDiagnosticRow{}},
		{"empty", "empty", model.OptionDiagnosticRow{Value: sqlString("")}},
		{"blank", "blank", model.OptionDiagnosticRow{Value: sqlString(" \t")}},
		{"invalid utf8", "invalid_utf8", model.OptionDiagnosticRow{Value: sqlString(string([]byte{0xff}))}},
		{"too large", "too_large", model.OptionDiagnosticRow{Value: sqlString("x"), TooLarge: true}},
		{"present", "present", model.OptionDiagnosticRow{Value: sqlString("x")}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			assert.Equal(t, testCase.expected, optionDiagnosticValueState(testCase.row))
		})
	}
}

func TestOptionDiagnosticRedactsUntrustedKeys(t *testing.T) {
	const canary = "c09-key-canary-secret"
	for _, testCase := range []struct {
		name, key, keyClass string
		redacted            bool
	}{
		{"unknown prefix", "unknown." + canary, "unknown_prefix", true},
		{"unknown field", "global." + canary, "unknown_field", true},
		{"malformed", string([]byte{0xff}), "malformed_key", true},
		{"untrusted flat", "flat-" + canary, "legacy_flat", true},
		{"known external", "model_deployment.ionet." + canary, "known_external", true},
		{"oversized", strings.Repeat("a", model.OptionDiagnosticKeyParseCapChars), "malformed_key", true},
		{"retired", "ApiInfo", "retired_legacy", false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			row := model.OptionDiagnosticRow{Key: testCase.key, Value: sqlString("value")}
			if testCase.name == "oversized" {
				row.KeyTooLarge = true
			}
			item := assessOptionDiagnosticRow(row)
			assert.Equal(t, testCase.keyClass, item.KeyClass)
			assert.Equal(t, testCase.redacted, item.KeyRedacted)
			if testCase.redacted {
				assert.Empty(t, item.Key)
				encoded, err := common.Marshal(item)
				require.NoError(t, err)
				assert.NotContains(t, string(encoded), canary)
			}
		})
	}
}

func sqlString(value string) sql.NullString { return sql.NullString{String: value, Valid: true} }
