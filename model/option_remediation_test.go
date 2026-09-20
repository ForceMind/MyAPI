package model

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestPlanOptionRemediationKeyWhitelistPredicates(t *testing.T) {
	row := func(key, value string) OptionDiagnosticRow {
		return OptionDiagnosticRow{Key: key, Value: sql.NullString{String: value, Valid: true}}
	}
	for _, testCase := range []struct {
		name           string
		row            OptionDiagnosticRow
		found          bool
		actionFilter   map[string]bool
		decision       string
		action         string
		reason         string
		risk           string
		expectNewValue bool
	}{
		{"trim repair", row("passkey.enabled", " true "), true, nil, OptionRemediationDecisionRepair, OptionRemediationActionTrimSpace, "trimmed_scalar_becomes_effective", OptionRemediationRiskMedium, true},
		{"trim clean", row("passkey.enabled", "true"), true, nil, OptionRemediationDecisionKeep, "", "value_clean", "", false},
		{"trim still invalid", row("passkey.enabled", " maybe"), true, nil, OptionRemediationDecisionReject, "", "trimmed_value_still_invalid", "", false},
		{"string whitespace preserved", row("passkey.rp_display_name", " name"), true, nil, OptionRemediationDecisionKeep, "", "string_whitespace_preserved", "", false},
		{"json bom repair", row("global.thinking_model_blacklist", "\xEF\xBB\xBF [\"a\"] "), true, nil, OptionRemediationDecisionRepair, OptionRemediationActionJSONBOMTrim, "bom_trimmed_json_becomes_effective", OptionRemediationRiskMedium, true},
		{"json already valid", row("global.thinking_model_blacklist", "[\"a\"]"), true, nil, OptionRemediationDecisionKeep, "", "value_already_valid", "", false},
		{"json not repairable", row("global.thinking_model_blacklist", "{bad"), true, nil, OptionRemediationDecisionReject, "", "value_not_determinably_repairable", "", false},
		{"empty rejected", row("passkey.enabled", ""), true, nil, OptionRemediationDecisionReject, "", "empty_or_blank_value_no_determinable_repair", "", false},
		{"blank rejected", row("passkey.enabled", " \t"), true, nil, OptionRemediationDecisionReject, "", "empty_or_blank_value_no_determinable_repair", "", false},
		{"raw null fallback", OptionDiagnosticRow{Key: "passkey.enabled"}, true, nil, OptionRemediationDecisionFallback, "", "null_value_indeterminate", "", false},
		{"too large fallback", OptionDiagnosticRow{Key: "passkey.enabled", Value: sql.NullString{String: "true", Valid: true}, TooLarge: true}, true, nil, OptionRemediationDecisionFallback, "", "value_too_large_indeterminate", "", false},
		{"invalid utf8 fallback", row("passkey.enabled", string([]byte{0xff})), true, nil, OptionRemediationDecisionFallback, "", "value_invalid_utf8_indeterminate", "", false},
		{"unknown prefix kept", row("unknown.prefix", "value"), true, nil, OptionRemediationDecisionKeep, "", "key_class_not_actionable", "", false},
		{"legacy flat kept", row("DisplayInCurrencyEnabled", " true"), true, nil, OptionRemediationDecisionKeep, "", "key_class_not_actionable", "", false},
		{"retired kept", row("theme.frontend", "classic"), true, nil, OptionRemediationDecisionKeep, "", "key_class_not_actionable", "", false},
		{"malformed key rejected", OptionDiagnosticRow{Key: "passkey.enabled", KeyTooLarge: true}, true, nil, OptionRemediationDecisionReject, "", "malformed_key_no_determinable_mapping", "", false},
		{"absent kept", OptionDiagnosticRow{}, false, nil, OptionRemediationDecisionKeep, "", "key_absent", "", false},
		{"action filtered", row("passkey.enabled", " true "), true, map[string]bool{OptionRemediationActionJSONBOMTrim: true}, OptionRemediationDecisionKeep, "", "action_filtered", "", false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			plan := planOptionRemediationKey(testCase.row.Key, testCase.row, testCase.found, testCase.actionFilter)
			assert.Equal(t, testCase.decision, plan.Decision)
			assert.Equal(t, testCase.action, plan.Action)
			assert.Equal(t, testCase.reason, plan.Reason)
			assert.Equal(t, testCase.risk, plan.Risk)
			if testCase.expectNewValue {
				assert.NotEmpty(t, plan.newValue)
				assert.Equal(t, optionRemediationHash(plan.newValue), plan.NewValueHash)
				assert.Equal(t, optionRemediationHash(testCase.row.Value.String), plan.OldValueHash)
			} else {
				assert.Empty(t, plan.newValue)
				assert.Empty(t, plan.NewValueHash)
			}
		})
	}
}

func TestPlanOptionRemediationGroupRatioPairsDecisions(t *testing.T) {
	pair := groupRatioOptionPairs[0]
	row := func(key, value string) OptionDiagnosticRow {
		return OptionDiagnosticRow{Key: key, Value: sql.NullString{String: value, Valid: true}}
	}
	build := func(canonical, alias *OptionDiagnosticRow, complete bool, filter map[string]bool) []OptionRemediationPlan {
		rows := map[string]OptionDiagnosticRow{}
		found := map[string]bool{}
		if canonical != nil {
			rows[canonical.Key] = *canonical
			found[canonical.Key] = true
		}
		if alias != nil {
			rows[alias.Key] = *alias
			found[alias.Key] = true
		}
		return planOptionRemediationGroupRatioPairs(rows, found, complete, filter)
	}
	plansFor := func(plans []OptionRemediationPlan) (OptionRemediationPlan, OptionRemediationPlan) {
		var canonicalPlan, aliasPlan OptionRemediationPlan
		for _, plan := range plans {
			if plan.Key == pair.canonical {
				canonicalPlan = plan
			}
			if plan.Key == pair.alias {
				aliasPlan = plan
			}
		}
		return canonicalPlan, aliasPlan
	}

	t.Run("alias conflicting", func(t *testing.T) {
		canonical, alias := row(pair.canonical, `{"default":1}`), row(pair.alias, `{"default":2}`)
		canonicalPlan, aliasPlan := plansFor(build(&canonical, &alias, true, nil))
		assert.Equal(t, OptionRemediationDecisionKeep, canonicalPlan.Decision)
		assert.Equal(t, OptionRemediationDecisionRepair, aliasPlan.Decision)
		assert.Equal(t, OptionRemediationActionGroupRatioSync, aliasPlan.Action)
		assert.Equal(t, OptionRemediationRiskLow, aliasPlan.Risk)
		assert.Equal(t, "alias_conflicts_with_authoritative_canonical", aliasPlan.Reason)
		assert.Equal(t, optionRemediationHash(`{"default":1}`), aliasPlan.NewValueHash)
		assert.Equal(t, `{"default":1}`, aliasPlan.newValue)
		assert.Equal(t, optionRemediationHash(`{"default":2}`), aliasPlan.OldValueHash)
	})
	t.Run("alias normalized matching", func(t *testing.T) {
		canonical, alias := row(pair.canonical, `{"default":1}`), row(pair.alias, ` { "default" : 1.0 } `)
		_, aliasPlan := plansFor(build(&canonical, &alias, true, nil))
		assert.Equal(t, OptionRemediationDecisionKeep, aliasPlan.Decision)
		assert.Equal(t, "pair_normalized_matching", aliasPlan.Reason)
	})
	t.Run("canonical invalid alias valid", func(t *testing.T) {
		canonical, alias := row(pair.canonical, `{`), row(pair.alias, `{"default":2}`)
		canonicalPlan, aliasPlan := plansFor(build(&canonical, &alias, true, nil))
		assert.Equal(t, OptionRemediationDecisionRepair, canonicalPlan.Decision)
		assert.Equal(t, OptionRemediationRiskMedium, canonicalPlan.Risk)
		assert.Equal(t, "canonical_promoted_from_effective_alias", canonicalPlan.Reason)
		assert.Equal(t, optionRemediationHash(`{"default":2}`), canonicalPlan.NewValueHash)
		assert.Equal(t, OptionRemediationDecisionKeep, aliasPlan.Decision)
	})
	t.Run("canonical absent alias valid", func(t *testing.T) {
		alias := row(pair.alias, `{"default":2}`)
		canonicalPlan, _ := plansFor(build(nil, &alias, true, nil))
		assert.Equal(t, OptionRemediationDecisionRepair, canonicalPlan.Decision)
		assert.Empty(t, canonicalPlan.OldValueHash)
		assert.False(t, canonicalPlan.oldValueSet)
	})
	t.Run("both invalid rejected", func(t *testing.T) {
		canonical, alias := row(pair.canonical, `{`), row(pair.alias, `{`)
		canonicalPlan, aliasPlan := plansFor(build(&canonical, &alias, true, nil))
		assert.Equal(t, OptionRemediationDecisionReject, canonicalPlan.Decision)
		assert.Equal(t, OptionRemediationDecisionReject, aliasPlan.Decision)
		assert.Equal(t, "no_determinable_valid_source", canonicalPlan.Reason)
	})
	t.Run("raw null fallback", func(t *testing.T) {
		canonical, alias := OptionDiagnosticRow{Key: pair.canonical}, row(pair.alias, `{"default":1}`)
		canonicalPlan, aliasPlan := plansFor(build(&canonical, &alias, true, nil))
		assert.Equal(t, OptionRemediationDecisionFallback, canonicalPlan.Decision)
		assert.Equal(t, OptionRemediationDecisionFallback, aliasPlan.Decision)
		assert.Equal(t, "pair_source_indeterminate", canonicalPlan.Reason)
	})
	t.Run("incomplete coverage fallback", func(t *testing.T) {
		canonical, alias := row(pair.canonical, `{"default":1}`), row(pair.alias, `{"default":2}`)
		canonicalPlan, aliasPlan := plansFor(build(&canonical, &alias, false, nil))
		assert.Equal(t, OptionRemediationDecisionFallback, canonicalPlan.Decision)
		assert.Equal(t, "coverage_incomplete", aliasPlan.Reason)
	})
	t.Run("action filtered", func(t *testing.T) {
		canonical, alias := row(pair.canonical, `{"default":1}`), row(pair.alias, `{"default":2}`)
		_, aliasPlan := plansFor(build(&canonical, &alias, true, map[string]bool{OptionRemediationActionTrimSpace: true}))
		assert.Equal(t, OptionRemediationDecisionKeep, aliasPlan.Decision)
		assert.Equal(t, "action_filtered", aliasPlan.Reason)
	})
}

func TestPlanOptionRemediationsDryRunIsZeroWrite(t *testing.T) {
	db := optionDiagnosticsSQLiteFixture(t)
	require.NoError(t, db.Exec("CREATE TABLE options (`key` text primary key, value text)").Error)
	require.NoError(t, db.AutoMigrate(&OptionRemediation{}, &OptionRemediationBackup{}))
	const canary = "c09-remediation-canary-secret"
	require.NoError(t, db.Create(&Option{Key: "passkey.enabled", Value: " true "}).Error)
	require.NoError(t, db.Create(&Option{Key: "unknown.prefix", Value: canary}).Error)
	require.NoError(t, db.Create(&Option{Key: groupRatioOptionKey, Value: `{"default":1}`}).Error)
	require.NoError(t, db.Create(&Option{Key: groupRatioOptionAlias, Value: `{"default":2}`}).Error)

	writes := 0
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("option-remediation-dry-run-create", func(*gorm.DB) { writes++ }))
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("option-remediation-dry-run-update", func(*gorm.DB) { writes++ }))
	require.NoError(t, db.Callback().Delete().Before("gorm:delete").Register("option-remediation-dry-run-delete", func(*gorm.DB) { writes++ }))
	optionMapBefore := snapshotOptionRemediationOptionMap()

	require.NoError(t, db.Exec("PRAGMA query_only = ON").Error)
	plans, coverage, err := PlanOptionRemediations(nil, true, nil)
	require.NoError(t, err)
	require.True(t, coverage.Complete)
	assert.Zero(t, writes)
	assert.Equal(t, optionMapBefore, snapshotOptionRemediationOptionMap())

	var remediationRows int64
	require.NoError(t, db.Model(&OptionRemediation{}).Count(&remediationRows).Error)
	assert.Zero(t, remediationRows)

	encoded, err := common.Marshal(plans)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), canary)
	assert.NotContains(t, string(encoded), " true ")

	repairs := 0
	for _, plan := range plans {
		if plan.Decision == OptionRemediationDecisionRepair {
			repairs++
		}
	}
	// The whitespace trim and the conflicting group ratio alias both plan a repair.
	assert.Equal(t, 2, repairs)

	// An explicit-key dry-run is zero-write as well, and a pair request
	// surfaces both pair entries while unrelated pairs stay out of scope.
	explicit, explicitCoverage, err := PlanOptionRemediations([]string{groupRatioOptionAlias}, false, nil)
	require.NoError(t, err)
	require.True(t, explicitCoverage.Complete)
	assert.Zero(t, writes)
	require.Len(t, explicit, 2)
	for _, plan := range explicit {
		assert.Contains(t, []string{groupRatioOptionKey, groupRatioOptionAlias}, plan.Key)
	}
}

func TestApplyOptionRemediationsSuccessBackupAndIdempotency(t *testing.T) {
	db := optionRemediationApplyFixture(t)
	require.NoError(t, db.Create(&Option{Key: "passkey.enabled", Value: "  true  "}).Error)
	publishCalls := stubOptionRemediationPublish(t, nil)

	item := planOptionRemediationApplyItem(t, "passkey.enabled")
	results, err := ApplyOptionRemediations(7, []OptionRemediationApplyItem{item})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, OptionRemediationApplyApplied, results[0].Status)
	assert.NotZero(t, results[0].RemediationId)

	var stored Option
	require.NoError(t, db.Where("key = ?", "passkey.enabled").Take(&stored).Error)
	assert.Equal(t, "true", stored.Value)
	require.Len(t, *publishCalls, 1)
	assert.Equal(t, [2]string{"passkey.enabled", "true"}, (*publishCalls)[0])

	remediations, total, err := ListOptionRemediations(db, 0, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, remediations, 1)
	registry := remediations[0]
	assert.Equal(t, OptionRemediationStatusApplied, registry.Status)
	assert.Equal(t, 7, registry.OperatorUserId)
	assert.Equal(t, item.ExpectedOldValueHash, registry.OldValueHash)
	assert.Equal(t, item.ExpectedNewValueHash, registry.NewValueHash)
	assert.NotZero(t, registry.BackupId)
	assert.NotZero(t, registry.AppliedAt)

	var backup OptionRemediationBackup
	require.NoError(t, db.Where("id = ?", registry.BackupId).Take(&backup).Error)
	assert.Equal(t, "  true  ", backup.OldValue)
	assert.Equal(t, item.ExpectedOldValueHash, backup.OldValueHash)

	// The registry and backup never serialize the option value.
	encoded, err := common.Marshal(remediations)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "  true  ")
	encodedBackup, err := common.Marshal(backup)
	require.NoError(t, err)
	assert.NotContains(t, string(encodedBackup), "  true  ")

	// Repeating the same action instance writes nothing and reports the prior
	// application.
	second, err := ApplyOptionRemediations(7, []OptionRemediationApplyItem{item})
	require.NoError(t, err)
	require.Len(t, second, 1)
	assert.Equal(t, OptionRemediationApplyAlreadyApplied, second[0].Status)
	assert.Equal(t, registry.Id, second[0].RemediationId)
	_, totalAfter, err := ListOptionRemediations(db, 0, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(1), totalAfter)
	assert.Len(t, *publishCalls, 1)
}

func TestApplyOptionRemediationsHashConflictSkipsAndRegisters(t *testing.T) {
	db := optionRemediationApplyFixture(t)
	require.NoError(t, db.Create(&Option{Key: "passkey.enabled", Value: " true "}).Error)
	publishCalls := stubOptionRemediationPublish(t, nil)

	item := planOptionRemediationApplyItem(t, "passkey.enabled")
	// A concurrent writer moves the row before the apply lands. The new value
	// still plans a trim repair, so the stale claim fails on the row hash.
	require.NoError(t, db.Model(&Option{}).Where("key = ?", "passkey.enabled").Update("value", "  false  ").Error)

	results, err := ApplyOptionRemediations(7, []OptionRemediationApplyItem{item})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, OptionRemediationApplySkipped, results[0].Status)
	assert.Equal(t, "hash_conflict", results[0].Reason)
	assert.NotZero(t, results[0].RemediationId)

	var stored Option
	require.NoError(t, db.Where("key = ?", "passkey.enabled").Take(&stored).Error)
	assert.Equal(t, "  false  ", stored.Value)
	assert.Empty(t, *publishCalls)

	remediations, total, err := ListOptionRemediations(db, 0, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, remediations, 1)
	assert.Equal(t, OptionRemediationStatusSkipped, remediations[0].Status)
	assert.Equal(t, "hash_conflict", remediations[0].Reason)
	assert.Zero(t, remediations[0].BackupId)

	var backups int64
	require.NoError(t, db.Model(&OptionRemediationBackup{}).Count(&backups).Error)
	assert.Zero(t, backups)
}

func TestApplyOptionRemediationsCreatesAbsentCanonicalWithBackup(t *testing.T) {
	db := optionRemediationApplyFixture(t)
	require.NoError(t, db.Create(&Option{Key: groupRatioOptionAlias, Value: `{"default":2}`}).Error)
	stubOptionRemediationPublish(t, nil)

	plans, _, err := PlanOptionRemediations([]string{groupRatioOptionAlias}, false, nil)
	require.NoError(t, err)
	var canonicalPlan OptionRemediationPlan
	for _, plan := range plans {
		if plan.Key == groupRatioOptionKey {
			canonicalPlan = plan
		}
	}
	require.Equal(t, OptionRemediationDecisionRepair, canonicalPlan.Decision)
	require.Empty(t, canonicalPlan.OldValueHash)

	item := OptionRemediationApplyItem{
		Key:                  canonicalPlan.Key,
		Action:               canonicalPlan.Action,
		ExpectedOldValueHash: canonicalPlan.OldValueHash,
		ExpectedNewValueHash: canonicalPlan.NewValueHash,
	}
	results, err := ApplyOptionRemediations(3, []OptionRemediationApplyItem{item})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, OptionRemediationApplyApplied, results[0].Status)

	var stored Option
	require.NoError(t, db.Where("key = ?", groupRatioOptionKey).Take(&stored).Error)
	assert.Equal(t, `{"default":2}`, stored.Value)

	remediations, _, err := ListOptionRemediations(db, 0, 10)
	require.NoError(t, err)
	require.Len(t, remediations, 1)
	var backup OptionRemediationBackup
	require.NoError(t, db.Where("id = ?", remediations[0].BackupId).Take(&backup).Error)
	assert.Empty(t, backup.OldValue)
	assert.Empty(t, backup.OldValueHash)
}

func TestApplyOptionRemediationsPublishFailureIsMarkedFailed(t *testing.T) {
	db := optionRemediationApplyFixture(t)
	require.NoError(t, db.Create(&Option{Key: "passkey.enabled", Value: " true "}).Error)
	stubOptionRemediationPublish(t, errors.New("injected publication failure"))

	item := planOptionRemediationApplyItem(t, "passkey.enabled")
	results, err := ApplyOptionRemediations(7, []OptionRemediationApplyItem{item})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, OptionRemediationApplyFailed, results[0].Status)
	assert.Equal(t, "publish_failed", results[0].Reason)

	// The committed repair stands (documented post-commit boundary); the
	// registry row records the publication failure and the backup survives.
	var stored Option
	require.NoError(t, db.Where("key = ?", "passkey.enabled").Take(&stored).Error)
	assert.Equal(t, "true", stored.Value)
	remediations, _, err := ListOptionRemediations(db, 0, 10)
	require.NoError(t, err)
	require.Len(t, remediations, 1)
	assert.Equal(t, OptionRemediationStatusFailed, remediations[0].Status)
	assert.Equal(t, "publish_failed", remediations[0].Reason)
	var backup OptionRemediationBackup
	require.NoError(t, db.Where("id = ?", remediations[0].BackupId).Take(&backup).Error)
	assert.Equal(t, " true ", backup.OldValue)
}

func TestApplyOptionRemediationsRealPublishGroupRatio(t *testing.T) {
	db := optionRemediationApplyFixture(t)
	require.NoError(t, db.Create(&Option{Key: groupRatioOptionKey, Value: `{"default":1}`}).Error)
	require.NoError(t, db.Create(&Option{Key: groupRatioOptionAlias, Value: `{"default":2}`}).Error)

	optionMapBefore := snapshotOptionRemediationOptionMap()
	previousGroupRatio := ratio_setting.GroupRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(previousGroupRatio))
		restoreOptionRemediationOptionMap(optionMapBefore)
	})
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = map[string]string{}
	}
	common.OptionMapRWMutex.Unlock()

	item := planOptionRemediationApplyItem(t, groupRatioOptionAlias)
	results, err := ApplyOptionRemediations(9, []OptionRemediationApplyItem{item})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, OptionRemediationApplyApplied, results[0].Status)

	var stored Option
	require.NoError(t, db.Where("key = ?", groupRatioOptionAlias).Take(&stored).Error)
	assert.Equal(t, `{"default":1}`, stored.Value)
	// The existing publication chain keeps the runtime generation and
	// OptionMap consistent with the committed row.
	assert.Equal(t, `{"default":1}`, ratio_setting.GroupRatio2JSONString())
	common.OptionMapRWMutex.RLock()
	assert.Equal(t, `{"default":1}`, common.OptionMap[groupRatioOptionAlias])
	assert.Equal(t, `{"default":1}`, common.OptionMap[groupRatioOptionKey])
	common.OptionMapRWMutex.RUnlock()
}

func TestValidateOptionRemediationApplyItemsIsFailClosed(t *testing.T) {
	validHash := optionRemediationHash("x")
	validItem := OptionRemediationApplyItem{Key: "passkey.enabled", Action: OptionRemediationActionTrimSpace, ExpectedOldValueHash: validHash, ExpectedNewValueHash: validHash}
	for _, testCase := range []struct {
		name  string
		items []OptionRemediationApplyItem
	}{
		{"empty", nil},
		{"too many", make([]OptionRemediationApplyItem, optionRemediationMaxItems+1)},
		{"duplicate keys", []OptionRemediationApplyItem{validItem, validItem}},
		{"empty key", []OptionRemediationApplyItem{{Key: "", Action: OptionRemediationActionTrimSpace, ExpectedOldValueHash: validHash, ExpectedNewValueHash: validHash}}},
		{"unknown action", []OptionRemediationApplyItem{{Key: "passkey.enabled", Action: "drop.table", ExpectedOldValueHash: validHash, ExpectedNewValueHash: validHash}}},
		{"malformed old hash", []OptionRemediationApplyItem{{Key: "passkey.enabled", Action: OptionRemediationActionTrimSpace, ExpectedOldValueHash: "md5:abc", ExpectedNewValueHash: validHash}}},
		{"empty new hash", []OptionRemediationApplyItem{{Key: "passkey.enabled", Action: OptionRemediationActionTrimSpace, ExpectedOldValueHash: validHash, ExpectedNewValueHash: ""}}},
		{"non actionable key", []OptionRemediationApplyItem{{Key: "unknown.prefix", Action: OptionRemediationActionTrimSpace, ExpectedOldValueHash: validHash, ExpectedNewValueHash: validHash}}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			err := ValidateOptionRemediationApplyItems(testCase.items)
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrOptionRemediationInput)
		})
	}
	require.NoError(t, ValidateOptionRemediationApplyItems([]OptionRemediationApplyItem{validItem}))
}

func optionRemediationApplyFixture(t *testing.T) *gorm.DB {
	t.Helper()
	db := optionDiagnosticsSQLiteFixture(t)
	require.NoError(t, db.Exec("CREATE TABLE options (`key` text primary key, value text)").Error)
	require.NoError(t, db.AutoMigrate(&OptionRemediation{}, &OptionRemediationBackup{}))
	return db
}

// planOptionRemediationApplyItem derives the claimed action instance for one
// key exactly like an operator would carry it from a dry-run into an apply.
func planOptionRemediationApplyItem(t *testing.T, key string) OptionRemediationApplyItem {
	t.Helper()
	plans, _, err := PlanOptionRemediations([]string{key}, false, nil)
	require.NoError(t, err)
	for _, plan := range plans {
		if plan.Key == key && plan.Decision == OptionRemediationDecisionRepair {
			return OptionRemediationApplyItem{
				Key:                  plan.Key,
				Action:               plan.Action,
				ExpectedOldValueHash: plan.OldValueHash,
				ExpectedNewValueHash: plan.NewValueHash,
			}
		}
	}
	require.FailNow(t, "expected a repair plan for "+key)
	return OptionRemediationApplyItem{}
}

func stubOptionRemediationPublish(t *testing.T, inject error) *[][2]string {
	t.Helper()
	calls := &[][2]string{}
	previous := optionRemediationPublish
	optionRemediationPublish = func(key string, value string) error {
		if inject != nil {
			return inject
		}
		*calls = append(*calls, [2]string{key, value})
		return nil
	}
	t.Cleanup(func() { optionRemediationPublish = previous })
	return calls
}

func snapshotOptionRemediationOptionMap() map[string]string {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	if common.OptionMap == nil {
		return nil
	}
	snapshot := make(map[string]string, len(common.OptionMap))
	for key, value := range common.OptionMap {
		snapshot[key] = value
	}
	return snapshot
}

func restoreOptionRemediationOptionMap(snapshot map[string]string) {
	common.OptionMapRWMutex.Lock()
	defer common.OptionMapRWMutex.Unlock()
	common.OptionMap = snapshot
}
