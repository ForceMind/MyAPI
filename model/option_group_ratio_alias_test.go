package model

import (
	"bytes"
	"errors"
	"sync"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func preserveGroupRatioOptionTestState(t *testing.T) {
	t.Helper()
	previousGroupRatio := ratio_setting.GroupRatio2JSONString()
	previousGroupGroupRatio := ratio_setting.GroupGroupRatio2JSONString()
	previousSpecial := ratio_setting.GroupSpecialUsableGroup2JSONString()
	common.OptionMapRWMutex.Lock()
	previousOptions := common.OptionMap
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(previousGroupRatio))
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(previousGroupGroupRatio))
		require.NoError(t, ratio_setting.UpdateGroupSpecialUsableGroupByJSONString(previousSpecial))
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptions
		common.OptionMapRWMutex.Unlock()
	})
}

func requireGroupRatioOptionPair(t *testing.T, pair groupRatioOptionPair, expected string) {
	t.Helper()
	normalized, err := normalizeGroupRatioOptionValue(pair, expected)
	require.NoError(t, err)
	common.OptionMapRWMutex.RLock()
	assert.Equal(t, normalized, common.OptionMap[pair.canonical])
	assert.Equal(t, normalized, common.OptionMap[pair.alias])
	common.OptionMapRWMutex.RUnlock()
	if pair.canonical == groupRatioOptionKey {
		assert.Equal(t, normalized, ratio_setting.GroupRatio2JSONString())
	} else {
		assert.Equal(t, normalized, ratio_setting.GroupGroupRatio2JSONString())
	}
}

func persistedGroupRatioOptions(t *testing.T, db *gorm.DB) map[string]string {
	t.Helper()
	var options []Option
	require.NoError(t, db.Find(&options).Error)
	result := make(map[string]string)
	for _, option := range options {
		if _, ok := groupRatioOptionPairForKey(option.Key); ok {
			result[option.Key] = option.Value
		}
	}
	return result
}

func captureGroupRatioSystemLog(t *testing.T, run func()) string {
	t.Helper()
	var log bytes.Buffer
	common.LogWriterMu.Lock()
	previousWriter := gin.DefaultWriter
	gin.DefaultWriter = &log
	common.LogWriterMu.Unlock()
	restore := sync.OnceFunc(func() {
		common.LogWriterMu.Lock()
		gin.DefaultWriter = previousWriter
		common.LogWriterMu.Unlock()
	})
	defer restore()
	t.Cleanup(restore)
	run()
	return log.String()
}

func groupRatioAliasDatabaseContract(t *testing.T, db *gorm.DB) {
	t.Helper()
	preserveGroupRatioOptionTestState(t)

	require.NoError(t, UpdateOption(groupRatioOptionAlias, `{"vip":2,"default":1}`))
	expected := map[string]string{
		groupRatioOptionKey:   `{"default":1,"vip":2}`,
		groupRatioOptionAlias: `{"default":1,"vip":2}`,
	}
	assert.Equal(t, expected, persistedGroupRatioOptions(t, db))
	requireGroupRatioOptionPair(t, groupRatioOptionPairs[0], expected[groupRatioOptionKey])

	require.NoError(t, UpdateOptionsBulk(map[string]string{
		groupRatioOptionKey:   ` {"default":1.0,"vip":3} `,
		groupRatioOptionAlias: "{\n\"vip\":3.0,\"default\":1\n}",
	}))
	requireGroupRatioOptionPair(t, groupRatioOptionPairs[0], `{"default":1,"vip":3}`)
	beforeConflict := persistedGroupRatioOptions(t, db)
	require.ErrorContains(t, UpdateOptionsBulk(map[string]string{
		groupRatioOptionKey:   `{"default":1,"vip":4}`,
		groupRatioOptionAlias: `{"default":1,"vip":5}`,
	}), groupRatioAliasConflictLogText)
	assert.Equal(t, beforeConflict, persistedGroupRatioOptions(t, db))

	require.NoError(t, db.Save(&Option{Key: groupRatioOptionKey, Value: `{"default":1,"vip":4}`}).Error)
	require.NoError(t, db.Save(&Option{Key: groupRatioOptionAlias, Value: `{"default":1,"vip":9}`}).Error)
	conflictingRows := persistedGroupRatioOptions(t, db)
	log := captureGroupRatioSystemLog(t, loadOptionsFromDatabase)
	requireGroupRatioOptionPair(t, groupRatioOptionPairs[0], `{"default":1,"vip":4}`)
	assert.Equal(t, conflictingRows, persistedGroupRatioOptions(t, db), "conflicting historical rows must only be logged")
	assert.Contains(t, log, groupRatioAliasConflictLogText)

	require.NoError(t, UpdateOption(groupRatioOptionKey, `{"default":1,"vip":6}`))
	beforeFailure := persistedGroupRatioOptions(t, db)
	writeErr := errors.New("injected configured-database group ratio alias failure")
	updates := 0
	const callbackName = "test:configured-group-ratio-alias-failure"
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		updates++
		if updates == 2 {
			tx.AddError(writeErr)
		}
	}))
	removeCallback := sync.OnceFunc(func() { require.NoError(t, db.Callback().Update().Remove(callbackName)) })
	defer removeCallback()
	t.Cleanup(removeCallback)
	require.ErrorIs(t, UpdateOption(groupRatioOptionAlias, `{"default":1,"vip":7}`), writeErr)
	assert.Equal(t, 2, updates)
	assert.Equal(t, beforeFailure, persistedGroupRatioOptions(t, db))
	requireGroupRatioOptionPair(t, groupRatioOptionPairs[0], `{"default":1,"vip":6}`)

	t.Run("create-rollback", func(t *testing.T) {
		groupRatioAliasCreateFailureContract(t, db)
	})
}

func groupRatioAliasCreateFailureContract(t *testing.T, db *gorm.DB) {
	t.Helper()
	for _, testCase := range []struct {
		name             string
		seedCanonical    bool
		failOnCreateCall int
	}{
		{name: "mixed-existing-canonical-and-missing-alias", seedCanonical: true, failOnCreateCall: 1},
		{name: "both-missing-second-create", failOnCreateCall: 2},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			preserveGroupRatioOptionTestState(t)
			require.NoError(t, db.Delete(&Option{Key: groupRatioOptionKey}).Error)
			require.NoError(t, db.Delete(&Option{Key: groupRatioOptionAlias}).Error)
			const baseline = `{"default":1,"vip":2}`
			require.NoError(t, publishGroupRatioOptionPair(groupRatioOptionPairs[0], baseline))
			if testCase.seedCanonical {
				require.NoError(t, db.Create(&Option{Key: groupRatioOptionKey, Value: baseline}).Error)
			}
			before := persistedGroupRatioOptions(t, db)

			writeErr := errors.New("injected group ratio alias create failure")
			creates := 0
			const callbackName = "test:group-ratio-alias-create-failure"
			require.NoError(t, db.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
				creates++
				if creates == testCase.failOnCreateCall {
					tx.AddError(writeErr)
				}
			}))
			removeCallback := sync.OnceFunc(func() { require.NoError(t, db.Callback().Create().Remove(callbackName)) })
			defer removeCallback()
			t.Cleanup(removeCallback)

			require.ErrorIs(t, UpdateOption(groupRatioOptionAlias, `{"default":1,"vip":8}`), writeErr)
			assert.Equal(t, testCase.failOnCreateCall, creates)
			assert.Equal(t, before, persistedGroupRatioOptions(t, db))
			requireGroupRatioOptionPair(t, groupRatioOptionPairs[0], baseline)
		})
	}
}

func TestLoadGroupRatioAliasesUsesCanonicalPrecedenceIndependentOfRowOrder(t *testing.T) {
	testCases := []struct {
		name        string
		rows        []Option
		expected    string
		wantWarning bool
	}{
		{
			name:     "canonical-only",
			rows:     []Option{{Key: groupRatioOptionKey, Value: ` { "vip": 2, "default": 1 } `}},
			expected: `{"default":1,"vip":2}`,
		},
		{
			name:     "alias-only",
			rows:     []Option{{Key: groupRatioOptionAlias, Value: ` { "vip": 3, "default": 1 } `}},
			expected: `{"default":1,"vip":3}`,
		},
		{
			name: "conflict-canonical-inserted-first",
			rows: []Option{
				{Key: groupRatioOptionKey, Value: `{"default":1,"vip":4}`},
				{Key: groupRatioOptionAlias, Value: `{"default":1,"vip":9}`},
			},
			expected:    `{"default":1,"vip":4}`,
			wantWarning: true,
		},
		{
			name: "conflict-alias-inserted-first",
			rows: []Option{
				{Key: groupRatioOptionAlias, Value: `{"default":1,"vip":9}`},
				{Key: groupRatioOptionKey, Value: `{"default":1,"vip":4}`},
			},
			expected:    `{"default":1,"vip":4}`,
			wantWarning: true,
		},
		{
			name: "semantic-equivalence",
			rows: []Option{
				{Key: groupRatioOptionAlias, Value: "{\n  \"vip\": 2.0, \"default\": 1\n}"},
				{Key: groupRatioOptionKey, Value: `{"default":1.0,"vip":2}`},
			},
			expected: `{"default":1,"vip":2}`,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			db := accessProfileTestDB(t)
			require.NoError(t, db.AutoMigrate(&Option{}))
			preserveGroupRatioOptionTestState(t)
			require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"baseline":1}`))
			require.NoError(t, db.Create(&testCase.rows).Error)

			log := captureGroupRatioSystemLog(t, loadOptionsFromDatabase)

			requireGroupRatioOptionPair(t, groupRatioOptionPairs[0], testCase.expected)
			assert.Equal(t, testCase.wantWarning, bytes.Contains([]byte(log), []byte(groupRatioAliasConflictLogText)))
			assert.Equal(t, len(testCase.rows), len(persistedGroupRatioOptions(t, db)), "loading must not migrate, delete, or synthesize database rows")
			for _, row := range testCase.rows {
				assert.Equal(t, row.Value, persistedGroupRatioOptions(t, db)[row.Key])
			}
		})
	}
}

func TestLoadInvalidGroupRatioAliasesPublishesOnlyValidDeterministicValues(t *testing.T) {
	for _, pair := range groupRatioOptionPairs {
		pair := pair
		validCanonical, validAlias, invalidCanonical, invalidAlias, baseline :=
			`{"default":1,"vip":4}`, `{"vip":7,"default":1}`, `{"vip":-1}`, `{"vip":-2}`, `{"baseline":1}`
		if pair.canonical == groupGroupRatioOptionKey {
			validCanonical = `{"vip":{"default":0.4}}`
			validAlias = `{"vip":{"default":0.7}}`
			invalidCanonical = `{"vip":{"default":-1}}`
			invalidAlias = `{"vip":{"default":-2}}`
			baseline = `{"baseline":{"default":1}}`
		}
		for _, testCase := range []struct {
			name           string
			rows           []Option
			expected       string
			logContains    string
			logNotContains string
		}{
			{
				name: "invalid-canonical-valid-alias-canonical-first",
				rows: []Option{
					{Key: pair.canonical, Value: invalidCanonical},
					{Key: pair.alias, Value: validAlias},
				},
				expected:       validAlias,
				logContains:    "falling back to valid alias",
				logNotContains: "takes precedence",
			},
			{
				name: "invalid-canonical-valid-alias-alias-first",
				rows: []Option{
					{Key: pair.alias, Value: validAlias},
					{Key: pair.canonical, Value: invalidCanonical},
				},
				expected:       validAlias,
				logContains:    "falling back to valid alias",
				logNotContains: "takes precedence",
			},
			{
				name: "valid-canonical-invalid-alias-canonical-first",
				rows: []Option{
					{Key: pair.canonical, Value: validCanonical},
					{Key: pair.alias, Value: invalidAlias},
				},
				expected:    validCanonical,
				logContains: "valid canonical " + pair.canonical + " remains authoritative",
			},
			{
				name: "valid-canonical-invalid-alias-alias-first",
				rows: []Option{
					{Key: pair.alias, Value: invalidAlias},
					{Key: pair.canonical, Value: validCanonical},
				},
				expected:    validCanonical,
				logContains: "valid canonical " + pair.canonical + " remains authoritative",
			},
			{
				name: "both-invalid-canonical-first",
				rows: []Option{
					{Key: pair.canonical, Value: invalidCanonical},
					{Key: pair.alias, Value: invalidAlias},
				},
				expected:    baseline,
				logContains: "both group ratio option aliases for " + pair.canonical + " are invalid",
			},
			{
				name: "both-invalid-alias-first",
				rows: []Option{
					{Key: pair.alias, Value: invalidAlias},
					{Key: pair.canonical, Value: invalidCanonical},
				},
				expected:    baseline,
				logContains: "both group ratio option aliases for " + pair.canonical + " are invalid",
			},
			{
				name:        "canonical-only-invalid",
				rows:        []Option{{Key: pair.canonical, Value: invalidCanonical}},
				expected:    baseline,
				logContains: "invalid canonical group ratio option " + pair.canonical,
			},
		} {
			t.Run(pair.canonical+"/"+testCase.name, func(t *testing.T) {
				db := accessProfileTestDB(t)
				require.NoError(t, db.AutoMigrate(&Option{}))
				preserveGroupRatioOptionTestState(t)
				require.NoError(t, publishGroupRatioOptionPair(pair, baseline))
				require.NoError(t, db.Create(&testCase.rows).Error)
				before := persistedGroupRatioOptions(t, db)

				log := captureGroupRatioSystemLog(t, loadOptionsFromDatabase)

				requireGroupRatioOptionPair(t, pair, testCase.expected)
				assert.Equal(t, before, persistedGroupRatioOptions(t, db), "loading invalid historical rows must not modify the database")
				assert.Contains(t, log, testCase.logContains)
				if testCase.logNotContains != "" {
					assert.NotContains(t, log, testCase.logNotContains)
				}
			})
		}
	}
}

func TestInitOptionMapPublishesAliasFallbackUnderBothKeys(t *testing.T) {
	db := accessProfileTestDB(t)
	require.NoError(t, db.AutoMigrate(&Option{}))
	preserveGroupRatioOptionTestState(t)
	require.NoError(t, db.Create(&Option{Key: groupGroupRatioOptionAlias, Value: `{"vip":{"default":0.75}}`}).Error)

	InitOptionMap()

	requireGroupRatioOptionPair(t, groupRatioOptionPairs[1], `{"vip":{"default":0.75}}`)
	assert.Equal(t, map[string]string{groupGroupRatioOptionAlias: `{"vip":{"default":0.75}}`}, persistedGroupRatioOptions(t, db))
}

func TestUpdateGroupRatioAliasesPersistAndPublishOneCanonicalValue(t *testing.T) {
	db := accessProfileTestDB(t)
	require.NoError(t, db.AutoMigrate(&Option{}))
	preserveGroupRatioOptionTestState(t)

	require.NoError(t, UpdateOption(groupRatioOptionAlias, ` { "vip": 2.0, "default": 1 } `))
	requireGroupRatioOptionPair(t, groupRatioOptionPairs[0], `{"default":1,"vip":2}`)
	assert.Equal(t, map[string]string{
		groupRatioOptionKey:   `{"default":1,"vip":2}`,
		groupRatioOptionAlias: `{"default":1,"vip":2}`,
	}, persistedGroupRatioOptions(t, db))

	require.NoError(t, UpdateOptionsBulk(map[string]string{
		groupGroupRatioOptionKey:   `{"vip":{"default":0.5,"fast":0}}`,
		groupGroupRatioOptionAlias: "{ \"vip\": { \"fast\": -0.0, \"default\": 0.50 } }",
	}))
	requireGroupRatioOptionPair(t, groupRatioOptionPairs[1], `{"vip":{"default":0.5,"fast":0}}`)
	stored := persistedGroupRatioOptions(t, db)
	assert.Equal(t, stored[groupGroupRatioOptionKey], stored[groupGroupRatioOptionAlias])

	before := persistedGroupRatioOptions(t, db)
	err := UpdateOptionsBulk(map[string]string{
		"Notice":                   "must-not-persist",
		groupGroupRatioOptionKey:   `{"vip":{"default":0.6}}`,
		groupGroupRatioOptionAlias: `{"vip":{"default":0.7}}`,
		groupRatioOptionKey:        `{"default":1}`,
		groupRatioOptionAlias:      `{"default":1.0}`,
	})
	require.ErrorContains(t, err, groupRatioAliasConflictLogText)
	assert.Equal(t, before, persistedGroupRatioOptions(t, db))
	var notice Option
	assert.ErrorIs(t, db.First(&notice, Option{Key: "Notice"}).Error, gorm.ErrRecordNotFound)
}

func TestUpdateGroupRatioAliasDatabaseFailureRollsBackPairAndRuntime(t *testing.T) {
	db := accessProfileTestDB(t)
	require.NoError(t, db.AutoMigrate(&Option{}))
	preserveGroupRatioOptionTestState(t)
	const baseline = `{"default":1,"vip":2}`
	require.NoError(t, UpdateOption(groupRatioOptionKey, baseline))
	before := persistedGroupRatioOptions(t, db)

	writeErr := errors.New("injected group ratio alias persistence failure")
	updates := 0
	const callbackName = "test:group-ratio-alias-update-failure"
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		updates++
		if updates == 2 {
			tx.AddError(writeErr)
		}
	}))
	removeCallback := sync.OnceFunc(func() { require.NoError(t, db.Callback().Update().Remove(callbackName)) })
	defer removeCallback()
	t.Cleanup(removeCallback)

	assert.ErrorIs(t, UpdateOption(groupRatioOptionAlias, `{"default":1,"vip":8}`), writeErr)
	assert.Equal(t, 2, updates)
	assert.Equal(t, before, persistedGroupRatioOptions(t, db))
	requireGroupRatioOptionPair(t, groupRatioOptionPairs[0], baseline)
}

func TestUpdateGroupRatioAliasCreateFailuresRollBackEveryWrite(t *testing.T) {
	db := accessProfileTestDB(t)
	require.NoError(t, db.AutoMigrate(&Option{}))
	groupRatioAliasCreateFailureContract(t, db)
}

func TestGroupSpecialUsableGroupRemainsAnIndependentOption(t *testing.T) {
	db := accessProfileTestDB(t)
	require.NoError(t, db.AutoMigrate(&Option{}))
	preserveGroupRatioOptionTestState(t)
	const key = "group_ratio_setting.group_special_usable_group"
	const value = `{"vip":{"fast":"enabled"}}`

	require.NoError(t, UpdateOption(key, value))

	var options []Option
	require.NoError(t, db.Find(&options).Error)
	require.Len(t, options, 1)
	assert.Equal(t, key, options[0].Key)
	assert.Equal(t, value, options[0].Value)
	groups, ok := ratio_setting.GetGroupSpecialUsableGroup("vip")
	require.True(t, ok)
	assert.Equal(t, map[string]string{"fast": "enabled"}, groups)
}
