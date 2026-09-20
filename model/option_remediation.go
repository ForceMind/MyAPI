package model

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/setting/config"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ---------------------------------------------------------------------------
// C09-N5b controlled remediation for historical option rows.
//
// The remediation planner is the single source of truth for the dry-run and
// the apply paths: apply re-derives every judgment server-side with the same
// code and compares the resulting (action, old hash, new hash) triple against
// the caller's claim before any write. Values never leave this layer except
// into the options row, the same-transaction backup row, and the existing
// runtime publication chain.
// ---------------------------------------------------------------------------

const (
	OptionRemediationActionGroupRatioSync = "group_ratio.canonical_sync"
	OptionRemediationActionTrimSpace      = "value.trim_space"
	OptionRemediationActionJSONBOMTrim    = "json.bom_trim_repair"

	OptionRemediationDecisionKeep     = "keep"
	OptionRemediationDecisionRepair   = "repair"
	OptionRemediationDecisionReject   = "reject"
	OptionRemediationDecisionFallback = "fallback"

	OptionRemediationRiskLow    = "low"
	OptionRemediationRiskMedium = "medium"

	// OptionRemediationStatusPreviewed is reserved for future persisted
	// previews: the N5b dry-run is strictly zero-write and never stores rows.
	OptionRemediationStatusPreviewed = "previewed"
	OptionRemediationStatusApplied   = "applied"
	OptionRemediationStatusSkipped   = "skipped"
	OptionRemediationStatusFailed    = "failed"

	OptionRemediationApplyApplied        = "applied"
	OptionRemediationApplySkipped        = "skipped"
	OptionRemediationApplyFailed         = "failed"
	OptionRemediationApplyAlreadyApplied = "already_applied"

	optionRemediationMaxKeys  = 64
	optionRemediationMaxItems = 64
	optionRemediationHashPref = "sha256:"
)

var (
	ErrOptionRemediationUnavailable = errors.New("option remediation unavailable")
	ErrOptionRemediationInput       = errors.New("option remediation input invalid")

	optionRemediationHashPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

// OptionRemediation is the durable registry of one remediation judgment
// outcome. It stores hashes and a backup reference only; option values never
// appear here. Status "previewed" is reserved for future persisted previews:
// the N5b dry-run writes nothing, so only applied/skipped/failed are stored.
type OptionRemediation struct {
	Id             int64  `json:"id" gorm:"primaryKey"`
	OperatorUserId int    `json:"operator_user_id" gorm:"not null;index"`
	Action         string `json:"action" gorm:"type:varchar(64);not null"`
	TargetKey      string `json:"target_key" gorm:"type:varchar(256);not null"`
	OldValueHash   string `json:"old_value_hash" gorm:"type:varchar(80);not null;default:''"`
	NewValueHash   string `json:"new_value_hash" gorm:"type:varchar(80);not null;default:''"`
	BackupId       int64  `json:"backup_id" gorm:"not null;default:0"`
	Status         string `json:"status" gorm:"type:varchar(16);not null;index"`
	Reason         string `json:"reason" gorm:"type:varchar(512);not null;default:''"`
	CreatedAt      int64  `json:"created_at" gorm:"type:bigint;not null"`
	AppliedAt      int64  `json:"applied_at" gorm:"type:bigint;not null;default:0"`
}

func (OptionRemediation) TableName() string { return "option_remediations" }

// OptionRemediationBackup is the same-transaction pre-write value snapshot.
// OldValue is database-only evidence: it is never serialized and never leaves
// the model layer through any API or log channel.
type OptionRemediationBackup struct {
	Id           int64  `json:"id" gorm:"primaryKey"`
	TargetKey    string `json:"target_key" gorm:"type:varchar(256);not null"`
	OldValue     string `json:"-" gorm:"type:text"`
	OldValueHash string `json:"old_value_hash" gorm:"type:varchar(80);not null;default:''"`
	CreatedAt    int64  `json:"created_at" gorm:"type:bigint;not null"`
}

func (OptionRemediationBackup) TableName() string { return "option_remediation_backups" }

func ListOptionRemediations(db *gorm.DB, startIdx int, pageSize int) ([]OptionRemediation, int64, error) {
	if db == nil {
		return nil, 0, gorm.ErrInvalidDB
	}
	var total int64
	if err := db.Model(&OptionRemediation{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []OptionRemediation
	if err := db.Order("id DESC").Offset(startIdx).Limit(pageSize).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// OptionRemediationPlan is one per-row judgment. oldValue/newValue hold the
// exact bytes involved in the write; they are unexported so the service and
// controller layers can only ever observe their hashes.
type OptionRemediationPlan struct {
	Key          string
	KeyClass     string
	Action       string
	Decision     string
	Reason       string
	Risk         string
	ValueState   string
	Validation   string
	OldValueHash string
	NewValueHash string
	oldValue     string
	oldValueSet  bool
	newValue     string
}

// optionRemediationActions is the closed action allow-list. Every action
// declares its key family, predicate, transform, and risk at its call site
// below; this list exists for request validation only.
func optionRemediationActions() []string {
	return []string{
		OptionRemediationActionGroupRatioSync,
		OptionRemediationActionTrimSpace,
		OptionRemediationActionJSONBOMTrim,
	}
}

func optionRemediationActionKnown(action string) bool {
	for _, known := range optionRemediationActions() {
		if action == known {
			return true
		}
	}
	return false
}

func optionRemediationHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return optionRemediationHashPref + hex.EncodeToString(sum[:])
}

// OptionRemediationKnownActions exposes the closed action allow-list for
// request validation in the service layer.
func OptionRemediationKnownActions() []string {
	return optionRemediationActions()
}

// OptionRemediationHashForPreview hashes a runtime value for the dry-run
// conflict preview. The hash is metadata; the value never leaves the caller.
func OptionRemediationHashForPreview(value string) string {
	return optionRemediationHash(value)
}

func optionRemediationHashValid(hash string) bool {
	return hash == "" || optionRemediationHashPattern.MatchString(hash)
}

// optionRemediationActionable reports whether a key belongs to a whitelisted
// family: the two group ratio option pairs or a registered configuration
// field. Unknown, external, retired, and legacy flat keys are deliberately
// out of scope; they are preserved, never cleaned up.
func optionRemediationActionable(key string) bool {
	if _, ok := groupRatioOptionPairForKey(key); ok {
		return true
	}
	return config.GlobalConfig.AssessOptionDiagnostic(key, "").KeyClass == "registered_field"
}

// readOptionRemediationRow reads one option row through the same bounded
// projection as the read-only diagnostics: key capped at its safe maximum
// with a truncation probe, value capped at the parse cap plus one character.
// found is false when the row does not exist.
func readOptionRemediationRow(db *gorm.DB, key string) (row OptionDiagnosticRow, found bool, err error) {
	if db == nil {
		return row, false, ErrOptionRemediationUnavailable
	}
	keyColumn := commonKeyCol
	if keyColumn == "" {
		if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
			keyColumn = `"key"`
		} else {
			keyColumn = "`key`"
		}
	}
	rows := make([]OptionDiagnosticRow, 0, 1)
	selectColumns := fmt.Sprintf(
		"SUBSTR(%s, 1, ?) AS %s, CASE WHEN SUBSTR(%s, ?, 1) = '' THEN 0 ELSE 1 END AS key_too_large, "+
			"SUBSTR(value, 1, ?) AS value, CASE WHEN value IS NULL OR SUBSTR(value, ?, 1) = '' THEN 0 ELSE 1 END AS too_large",
		keyColumn, keyColumn, keyColumn,
	)
	err = db.Model(&Option{}).
		Select(
			selectColumns,
			OptionDiagnosticKeyParseCapChars,
			OptionDiagnosticKeyParseCapChars+1,
			OptionDiagnosticValueParseCapChars+1,
			OptionDiagnosticValueParseCapChars+1,
		).
		Where(keyColumn+" = ?", key).
		Limit(1).
		Scan(&rows).Error
	if err != nil {
		return row, false, err
	}
	if len(rows) == 0 {
		return row, false, nil
	}
	return rows[0], true, nil
}

func optionRemediationValueState(row OptionDiagnosticRow, found bool) string {
	if !found {
		return "absent"
	}
	if !row.Value.Valid {
		return "raw_null"
	}
	if row.TooLarge {
		return "too_large"
	}
	if !utf8.ValidString(row.Value.String) {
		return "invalid_utf8"
	}
	if row.Value.String == "" {
		return "empty"
	}
	if strings.TrimSpace(row.Value.String) == "" {
		return "blank"
	}
	return "present"
}

// planOptionRemediationKey derives the judgment for one non-pair key. The
// pair keys of the group ratio options are planned by
// planOptionRemediationGroupRatioPairs instead.
func planOptionRemediationKey(key string, row OptionDiagnosticRow, found bool, actionFilter map[string]bool) OptionRemediationPlan {
	plan := OptionRemediationPlan{Key: key, Decision: OptionRemediationDecisionKeep}
	if !found {
		plan.ValueState = "absent"
		plan.Reason = "key_absent"
		return plan
	}
	plan.Key = row.Key
	plan.ValueState = optionRemediationValueState(row, true)
	if row.KeyTooLarge {
		plan.KeyClass = "malformed_key"
		plan.Decision = OptionRemediationDecisionReject
		plan.Reason = "malformed_key_no_determinable_mapping"
		return plan
	}
	assessment := config.GlobalConfig.AssessOptionDiagnostic(row.Key, "")
	plan.KeyClass = assessment.KeyClass
	if plan.KeyClass != "registered_field" {
		// Unknown, external, retired, schema-indeterminate, and legacy flat
		// keys are preserved exactly as stored; remediation never cleans up
		// production history.
		if plan.KeyClass == "malformed_key" {
			plan.Decision = OptionRemediationDecisionReject
			plan.Reason = "malformed_key_no_determinable_mapping"
			return plan
		}
		plan.Reason = "key_class_not_actionable"
		return plan
	}
	switch plan.ValueState {
	case "raw_null":
		plan.Decision = OptionRemediationDecisionFallback
		plan.Reason = "null_value_indeterminate"
		return plan
	case "too_large":
		plan.Decision = OptionRemediationDecisionFallback
		plan.Reason = "value_too_large_indeterminate"
		return plan
	case "invalid_utf8":
		plan.Decision = OptionRemediationDecisionFallback
		plan.Reason = "value_invalid_utf8_indeterminate"
		return plan
	case "empty", "blank":
		plan.Decision = OptionRemediationDecisionReject
		plan.Reason = "empty_or_blank_value_no_determinable_repair"
		return plan
	}
	plan.oldValue = row.Value.String
	plan.oldValueSet = true
	plan.OldValueHash = optionRemediationHash(plan.oldValue)
	kind, ok := config.GlobalConfig.OptionDiagnosticFieldKind(row.Key)
	if !ok {
		plan.KeyClass = "schema_indeterminate"
		plan.Decision = OptionRemediationDecisionFallback
		plan.Reason = "schema_indeterminate"
		return plan
	}
	if kind == "json" {
		return planOptionRemediationJSONRepair(plan, actionFilter)
	}
	return planOptionRemediationTrimSpace(plan, kind, actionFilter)
}

// planOptionRemediationTrimSpace evaluates value.trim_space: trimming
// surrounding whitespace of a scalar numeric/boolean field. String fields are
// never trimmed because their whitespace can be meaningful.
func planOptionRemediationTrimSpace(plan OptionRemediationPlan, kind string, actionFilter map[string]bool) OptionRemediationPlan {
	switch kind {
	case "boolean", "integer", "unsigned_integer", "finite_number":
	default:
		if strings.TrimSpace(plan.oldValue) != plan.oldValue {
			plan.Reason = "string_whitespace_preserved"
		} else {
			plan.Reason = "value_clean"
		}
		return plan
	}
	trimmed := strings.TrimSpace(plan.oldValue)
	if trimmed == plan.oldValue {
		plan.Reason = "value_clean"
		return plan
	}
	if len(actionFilter) > 0 && !actionFilter[OptionRemediationActionTrimSpace] {
		plan.Reason = "action_filtered"
		return plan
	}
	if config.GlobalConfig.AssessOptionDiagnostic(plan.Key, trimmed).Validation != "" {
		plan.Decision = OptionRemediationDecisionReject
		plan.Reason = "trimmed_value_still_invalid"
		return plan
	}
	plan.Action = OptionRemediationActionTrimSpace
	plan.Decision = OptionRemediationDecisionRepair
	plan.Risk = OptionRemediationRiskMedium
	plan.Reason = "trimmed_scalar_becomes_effective"
	plan.newValue = trimmed
	plan.NewValueHash = optionRemediationHash(trimmed)
	return plan
}

// planOptionRemediationJSONRepair evaluates json.bom_trim_repair: a JSON
// field whose stored value fails to parse but becomes valid after stripping a
// UTF-8 byte-order mark and surrounding whitespace. Anything else is not
// determinably repairable and is rejected for manual handling.
func planOptionRemediationJSONRepair(plan OptionRemediationPlan, actionFilter map[string]bool) OptionRemediationPlan {
	if config.GlobalConfig.AssessOptionDiagnostic(plan.Key, plan.oldValue).Validation == "" {
		plan.Reason = "value_already_valid"
		return plan
	}
	stripped := strings.TrimSpace(strings.TrimPrefix(plan.oldValue, "\xEF\xBB\xBF"))
	if stripped == plan.oldValue {
		plan.Decision = OptionRemediationDecisionReject
		plan.Reason = "value_not_determinably_repairable"
		return plan
	}
	if len(actionFilter) > 0 && !actionFilter[OptionRemediationActionJSONBOMTrim] {
		plan.Reason = "action_filtered"
		return plan
	}
	if config.GlobalConfig.AssessOptionDiagnostic(plan.Key, stripped).Validation != "" {
		plan.Decision = OptionRemediationDecisionReject
		plan.Reason = "value_not_determinably_repairable"
		return plan
	}
	plan.Action = OptionRemediationActionJSONBOMTrim
	plan.Decision = OptionRemediationDecisionRepair
	plan.Risk = OptionRemediationRiskMedium
	plan.Reason = "bom_trimmed_json_becomes_effective"
	plan.newValue = stripped
	plan.NewValueHash = optionRemediationHash(stripped)
	return plan
}

// planOptionRemediationGroupRatioPairs derives the alias normalization
// judgments for the two group ratio option pairs. found must reflect row
// existence accurately for both pair keys; when the snapshot is incomplete
// every pair key falls back instead of guessing.
func planOptionRemediationGroupRatioPairs(rows map[string]OptionDiagnosticRow, found map[string]bool, complete bool, actionFilter map[string]bool) []OptionRemediationPlan {
	plans := make([]OptionRemediationPlan, 0, len(groupRatioOptionPairs)*2)
	for _, pair := range groupRatioOptionPairs {
		canonicalRow, canonicalFound := rows[pair.canonical], found[pair.canonical]
		aliasRow, aliasFound := rows[pair.alias], found[pair.alias]
		canonicalPlan := OptionRemediationPlan{Key: pair.canonical, KeyClass: "legacy_flat", Decision: OptionRemediationDecisionKeep, ValueState: optionRemediationValueState(canonicalRow, canonicalFound)}
		aliasPlan := OptionRemediationPlan{Key: pair.alias, KeyClass: "registered_field", Decision: OptionRemediationDecisionKeep, ValueState: optionRemediationValueState(aliasRow, aliasFound)}
		// Hashes and old values exist only when the stored value is fully
		// known: present, within the parse cap, and valid UTF-8. Raw NULL and
		// truncated rows keep an empty hash and stay indeterminate.
		if canonicalFound && canonicalRow.Value.Valid && !canonicalRow.TooLarge && utf8.ValidString(canonicalRow.Value.String) {
			canonicalPlan.OldValueHash = optionRemediationHash(canonicalRow.Value.String)
			canonicalPlan.oldValue, canonicalPlan.oldValueSet = canonicalRow.Value.String, true
		}
		if aliasFound && aliasRow.Value.Valid && !aliasRow.TooLarge && utf8.ValidString(aliasRow.Value.String) {
			aliasPlan.OldValueHash = optionRemediationHash(aliasRow.Value.String)
			aliasPlan.oldValue, aliasPlan.oldValueSet = aliasRow.Value.String, true
		}
		plans = append(plans, canonicalPlan, aliasPlan)
		current := &plans[len(plans)-2]
		other := &plans[len(plans)-1]
		if !complete {
			markOptionRemediationPairFallback(current, other, "coverage_incomplete")
			continue
		}
		canonicalState := optionRemediationPairValueState(pair, canonicalRow, canonicalFound)
		aliasState := optionRemediationPairValueState(pair, aliasRow, aliasFound)
		if canonicalState == optionDiagnosticAliasIndeterminate || aliasState == optionDiagnosticAliasIndeterminate {
			markOptionRemediationPairFallback(current, other, "pair_source_indeterminate")
			continue
		}
		filtered := len(actionFilter) > 0 && !actionFilter[OptionRemediationActionGroupRatioSync]
		switch {
		case canonicalState == optionDiagnosticAliasValid:
			canonicalNormalized, err := normalizeGroupRatioOptionValue(pair, canonicalRow.Value.String)
			if err != nil {
				markOptionRemediationPairFallback(current, other, "pair_source_indeterminate")
				continue
			}
			current.Reason = "pair_canonical_authoritative"
			switch aliasState {
			case optionDiagnosticAliasValid:
				aliasNormalized, err := normalizeGroupRatioOptionValue(pair, aliasRow.Value.String)
				if err != nil {
					markOptionRemediationPairFallback(current, other, "pair_source_indeterminate")
					continue
				}
				if aliasNormalized == canonicalNormalized {
					other.Reason = "pair_normalized_matching"
					continue
				}
				planOptionRemediationPairAliasRepair(other, canonicalNormalized, "alias_conflicts_with_authoritative_canonical", filtered)
			case optionDiagnosticAliasInvalid:
				if aliasFound {
					planOptionRemediationPairAliasRepair(other, canonicalNormalized, "alias_invalid_canonical_authoritative", filtered)
				} else {
					other.Reason = "alias_absent_runtime_correct"
				}
			}
		case aliasState == optionDiagnosticAliasValid:
			aliasNormalized, err := normalizeGroupRatioOptionValue(pair, aliasRow.Value.String)
			if err != nil {
				markOptionRemediationPairFallback(current, other, "pair_source_indeterminate")
				continue
			}
			other.Reason = "pair_alias_promoted_to_canonical"
			if filtered {
				current.Reason = "action_filtered"
				continue
			}
			// Promote the effective alias value into the canonical row: the
			// normalized value is identical, so the runtime effective value is
			// preserved while the effective source moves alias -> canonical.
			current.Action = OptionRemediationActionGroupRatioSync
			current.Decision = OptionRemediationDecisionRepair
			current.Risk = OptionRemediationRiskMedium
			current.Reason = "canonical_promoted_from_effective_alias"
			current.newValue = aliasNormalized
			current.NewValueHash = optionRemediationHash(aliasNormalized)
		default:
			// No valid source exists on either side; inventing a value is not
			// a determinable repair.
			if canonicalFound {
				current.Decision = OptionRemediationDecisionReject
				current.Reason = "no_determinable_valid_source"
			} else {
				current.Reason = "key_absent"
			}
			if aliasFound {
				other.Decision = OptionRemediationDecisionReject
				other.Reason = "no_determinable_valid_source"
			} else {
				other.Reason = "key_absent"
			}
		}
	}
	return plans
}

func optionRemediationPairValueState(pair groupRatioOptionPair, row OptionDiagnosticRow, found bool) optionDiagnosticAliasValueState {
	if !found {
		return optionDiagnosticAliasInvalid
	}
	if row.KeyTooLarge || !row.Value.Valid || row.TooLarge || !utf8.ValidString(row.Value.String) {
		return optionDiagnosticAliasIndeterminate
	}
	if strings.TrimSpace(row.Value.String) == "" {
		return optionDiagnosticAliasInvalid
	}
	// Mirror the diagnostics' normalization semantics: a value only counts as
	// valid when the pair parser actually accepts it.
	if _, err := normalizeGroupRatioOptionValue(pair, row.Value.String); err != nil {
		return optionDiagnosticAliasInvalid
	}
	return optionDiagnosticAliasValid
}

// planOptionRemediationPairAliasRepair plans the alias-row write that copies
// the authoritative canonical value into an invalid or conflicting alias row.
// The runtime already uses the canonical value, so the risk is low.
func planOptionRemediationPairAliasRepair(plan *OptionRemediationPlan, canonicalNormalized string, reason string, filtered bool) {
	if filtered {
		plan.Reason = "action_filtered"
		return
	}
	plan.Action = OptionRemediationActionGroupRatioSync
	plan.Decision = OptionRemediationDecisionRepair
	plan.Risk = OptionRemediationRiskLow
	plan.Reason = reason
	plan.newValue = canonicalNormalized
	plan.NewValueHash = optionRemediationHash(canonicalNormalized)
}

func markOptionRemediationPairFallback(canonicalPlan *OptionRemediationPlan, aliasPlan *OptionRemediationPlan, reason string) {
	canonicalPlan.Decision = OptionRemediationDecisionFallback
	canonicalPlan.Reason = reason
	aliasPlan.Decision = OptionRemediationDecisionFallback
	aliasPlan.Reason = reason
}

// OptionRemediationCoverage reports how the planning snapshot was obtained.
type OptionRemediationCoverage struct {
	RowsScanned int  `json:"rows_scanned"`
	RowLimit    int  `json:"row_limit"`
	Complete    bool `json:"complete"`
}

// PlanOptionRemediations builds the dry-run plan. With includeAll it reuses
// the bounded diagnostics snapshot; otherwise it reads each requested key
// through the same bounded per-key projection. It performs no writes of any
// kind and never touches OptionMap or a runtime generation.
func PlanOptionRemediations(keys []string, includeAll bool, actionFilter map[string]bool) ([]OptionRemediationPlan, OptionRemediationCoverage, error) {
	if DB == nil {
		return nil, OptionRemediationCoverage{}, ErrOptionRemediationUnavailable
	}
	if includeAll {
		return planOptionRemediationsFromSnapshot(actionFilter)
	}
	if len(keys) == 0 || len(keys) > optionRemediationMaxKeys {
		return nil, OptionRemediationCoverage{}, fmt.Errorf("%w: keys must contain 1-%d entries", ErrOptionRemediationInput, optionRemediationMaxKeys)
	}
	seen := make(map[string]bool, len(keys))
	readSet := make(map[string]bool, len(keys))
	for _, key := range keys {
		if key == "" || seen[key] {
			return nil, OptionRemediationCoverage{}, fmt.Errorf("%w: keys must be non-empty and unique", ErrOptionRemediationInput)
		}
		seen[key] = true
		readSet[key] = true
		// Pair judgments need both rows of the pair even when the caller
		// named only one side.
		if pair, ok := groupRatioOptionPairForKey(key); ok {
			readSet[pair.canonical] = true
			readSet[pair.alias] = true
		}
	}
	rows := make(map[string]OptionDiagnosticRow, len(readSet))
	found := make(map[string]bool, len(readSet))
	for key := range readSet {
		row, rowFound, err := readOptionRemediationRow(DB, key)
		if err != nil {
			return nil, OptionRemediationCoverage{}, ErrOptionRemediationUnavailable
		}
		found[key] = rowFound
		if rowFound {
			rows[key] = row
		}
	}
	plans := planOptionRemediationRows(rows, found, true, actionFilter, keys)
	// Explicit-key planning reads pair partners to judge a requested pair key,
	// but pairs the caller never named stay out of the response.
	scoped := make(map[string]bool, len(readSet))
	for key := range readSet {
		scoped[key] = true
	}
	filtered := plans[:0]
	for _, plan := range plans {
		if scoped[plan.Key] {
			filtered = append(filtered, plan)
		}
	}
	return filtered, OptionRemediationCoverage{RowsScanned: len(keys), RowLimit: optionRemediationMaxKeys, Complete: true}, nil
}

func planOptionRemediationsFromSnapshot(actionFilter map[string]bool) ([]OptionRemediationPlan, OptionRemediationCoverage, error) {
	rows, complete, err := ReadOptionDiagnosticRows()
	if err != nil {
		return nil, OptionRemediationCoverage{}, err
	}
	byKey := make(map[string]OptionDiagnosticRow, len(rows))
	found := make(map[string]bool, len(rows))
	order := make([]string, 0, len(rows))
	for _, row := range rows {
		found[row.Key] = true
		byKey[row.Key] = row
		order = append(order, row.Key)
	}
	plans := planOptionRemediationRows(byKey, found, complete, actionFilter, order)
	return plans, OptionRemediationCoverage{RowsScanned: len(rows), RowLimit: OptionDiagnosticRowLimit, Complete: complete}, nil
}

// planOptionRemediationRows evaluates the pair judgments first, then every
// remaining requested row through the single-key planner. order preserves the
// deterministic caller/snapshot key order; pair keys planned by the pair pass
// are skipped in the row pass.
func planOptionRemediationRows(rows map[string]OptionDiagnosticRow, found map[string]bool, complete bool, actionFilter map[string]bool, order []string) []OptionRemediationPlan {
	pairKeys := make(map[string]bool, len(groupRatioOptionPairs)*2)
	for _, pair := range groupRatioOptionPairs {
		pairKeys[pair.canonical] = true
		pairKeys[pair.alias] = true
	}
	plans := planOptionRemediationGroupRatioPairs(rows, found, complete, actionFilter)
	for _, key := range order {
		if pairKeys[key] {
			continue
		}
		plans = append(plans, planOptionRemediationKey(key, rows[key], found[key], actionFilter))
	}
	return plans
}

// ---------------------------------------------------------------------------
// Apply path
// ---------------------------------------------------------------------------

// OptionRemediationApplyItem is one caller-claimed action instance exactly as
// a dry-run produced it. ExpectedOldValueHash is empty only for the
// create-absent-canonical case.
type OptionRemediationApplyItem struct {
	Key                  string `json:"key"`
	Action               string `json:"action"`
	ExpectedOldValueHash string `json:"expected_old_value_hash"`
	ExpectedNewValueHash string `json:"expected_new_value_hash"`
}

// OptionRemediationResult is the per-row apply outcome.
type OptionRemediationResult struct {
	Key           string `json:"key"`
	Action        string `json:"action"`
	Status        string `json:"status"`
	Reason        string `json:"reason"`
	OldValueHash  string `json:"old_value_hash"`
	NewValueHash  string `json:"new_value_hash"`
	RemediationId int64  `json:"remediation_id"`
}

// ValidateOptionRemediationApplyItems enforces the fail-closed request shape
// before any database work: bounded item count, whitelisted actions,
// well-formed hashes, and whitelisted key families only.
func ValidateOptionRemediationApplyItems(items []OptionRemediationApplyItem) error {
	if len(items) == 0 || len(items) > optionRemediationMaxItems {
		return fmt.Errorf("%w: items must contain 1-%d entries", ErrOptionRemediationInput, optionRemediationMaxItems)
	}
	seen := make(map[string]bool, len(items))
	for _, item := range items {
		if item.Key == "" || seen[item.Key] {
			return fmt.Errorf("%w: item keys must be non-empty and unique", ErrOptionRemediationInput)
		}
		seen[item.Key] = true
		if !optionRemediationActionKnown(item.Action) {
			return fmt.Errorf("%w: unknown remediation action", ErrOptionRemediationInput)
		}
		if !optionRemediationHashValid(item.ExpectedOldValueHash) || !optionRemediationHashPattern.MatchString(item.ExpectedNewValueHash) {
			return fmt.Errorf("%w: malformed value hash", ErrOptionRemediationInput)
		}
		if !optionRemediationActionable(item.Key) {
			return fmt.Errorf("%w: key is not in a remediation whitelist family", ErrOptionRemediationInput)
		}
	}
	return nil
}

// optionRemediationPublish reuses the existing per-key publication chain so a
// committed repair reaches OptionMap and the runtime generation exactly like
// an administrative option update. It is a variable so tests can inject a
// publication failure.
var optionRemediationPublish = func(key string, value string) error {
	if pair, ok := groupRatioOptionPairForKey(key); ok {
		return publishGroupRatioOptionPair(pair, value)
	}
	return updateOptionMap(key, value)
}

// ApplyOptionRemediations applies claimed action instances row by row. Every
// row is independent: the judgment is rebuilt from the current row, verified
// against the claimed hashes, written in its own transaction together with
// the backup and registry rows, and published after its commit. A skipped or
// failed row never pretends the batch was atomic.
func ApplyOptionRemediations(operatorUserId int, items []OptionRemediationApplyItem) ([]OptionRemediationResult, error) {
	if DB == nil {
		return nil, ErrOptionRemediationUnavailable
	}
	if err := ValidateOptionRemediationApplyItems(items); err != nil {
		return nil, err
	}
	optionMutationLock.Lock()
	defer optionMutationLock.Unlock()
	results := make([]OptionRemediationResult, 0, len(items))
	for _, item := range items {
		results = append(results, applyOptionRemediationItem(DB, operatorUserId, item))
	}
	return results, nil
}

func applyOptionRemediationItem(db *gorm.DB, operatorUserId int, item OptionRemediationApplyItem) OptionRemediationResult {
	result := OptionRemediationResult{
		Key: item.Key, Action: item.Action,
		Status: OptionRemediationApplySkipped, Reason: "hash_conflict",
		OldValueHash: item.ExpectedOldValueHash, NewValueHash: item.ExpectedNewValueHash,
	}
	plan, currentHash, err := rebuildOptionRemediationPlan(db, item.Key)
	if err != nil {
		result.Status = OptionRemediationApplyFailed
		result.Reason = "read_failed"
		registerOptionRemediationRow(db, operatorUserId, item, OptionRemediationStatusFailed, result.Reason, &result)
		return result
	}
	// Idempotency: the row already carries the target value. When the same
	// action instance was applied before, report it without any write.
	if currentHash == item.ExpectedNewValueHash {
		if existing, found := findAppliedOptionRemediation(db, item); found {
			result.Status = OptionRemediationApplyAlreadyApplied
			result.Reason = "action_instance_already_applied"
			result.RemediationId = existing.Id
			return result
		}
		result.Reason = "already_at_target_unregistered"
		registerOptionRemediationRow(db, operatorUserId, item, OptionRemediationStatusSkipped, result.Reason, &result)
		return result
	}
	if plan.Decision != OptionRemediationDecisionRepair || plan.oldValueSet != (item.ExpectedOldValueHash != "") {
		result.Reason = "decision_changed"
		registerOptionRemediationRow(db, operatorUserId, item, OptionRemediationStatusSkipped, result.Reason, &result)
		return result
	}
	if plan.Action != item.Action {
		result.Reason = "action_mismatch"
		registerOptionRemediationRow(db, operatorUserId, item, OptionRemediationStatusSkipped, result.Reason, &result)
		return result
	}
	if plan.OldValueHash != item.ExpectedOldValueHash || plan.NewValueHash != item.ExpectedNewValueHash {
		result.Reason = "hash_conflict"
		registerOptionRemediationRow(db, operatorUserId, item, OptionRemediationStatusSkipped, result.Reason, &result)
		return result
	}

	var (
		remediationId int64
		casLost       bool
	)
	txErr := db.Transaction(func(tx *gorm.DB) error {
		now, err := taskRecoveryDBTimestamp(tx)
		if err != nil {
			return err
		}
		// The guarded write is the cross-process compare-and-swap arbiter:
		// the update only lands while the stored bytes still equal the
		// planner-verified old value; the create only lands while the row is
		// still absent. A repair decision always implies a byte-level change,
		// so a no-op update cannot masquerade as success.
		if !plan.oldValueSet {
			create := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "key"}}, DoNothing: true}).Create(&Option{Key: item.Key, Value: plan.newValue})
			if create.Error != nil {
				return create.Error
			}
			if create.RowsAffected != 1 {
				casLost = true
				return errOptionRemediationCASConflict
			}
		} else {
			// The value guard must compare bytes exactly. MySQL text columns
			// commonly use case-insensitive collations where ' true' = 'TRUE'
			// would wrongly match, so MySQL gets an explicit BINARY
			// comparison; PostgreSQL and SQLite comparisons are already
			// case-sensitive by default.
			query := tx.Model(&Option{}).Where("key = ?", item.Key)
			if common.UsingMainDatabase(common.DatabaseTypeMySQL) {
				query = query.Where("BINARY `value` = ?", plan.oldValue)
			} else {
				query = query.Where("value = ?", plan.oldValue)
			}
			update := query.Update("value", plan.newValue)
			if update.Error != nil {
				return update.Error
			}
			if update.RowsAffected != 1 {
				casLost = true
				return errOptionRemediationCASConflict
			}
		}
		backup := &OptionRemediationBackup{TargetKey: item.Key, OldValueHash: item.ExpectedOldValueHash, CreatedAt: now}
		if plan.oldValueSet {
			backup.OldValue = plan.oldValue
		}
		if err := tx.Create(backup).Error; err != nil {
			return err
		}
		remediation := &OptionRemediation{
			OperatorUserId: operatorUserId,
			Action:         item.Action,
			TargetKey:      item.Key,
			OldValueHash:   item.ExpectedOldValueHash,
			NewValueHash:   item.ExpectedNewValueHash,
			BackupId:       backup.Id,
			Status:         OptionRemediationStatusApplied,
			Reason:         plan.Reason,
			CreatedAt:      now,
			AppliedAt:      now,
		}
		if err := tx.Create(remediation).Error; err != nil {
			return err
		}
		remediationId = remediation.Id
		return nil
	})
	if txErr != nil {
		if casLost {
			result.Reason = "cas_conflict"
			registerOptionRemediationRow(db, operatorUserId, item, OptionRemediationStatusSkipped, result.Reason, &result)
			return result
		}
		result.Status = OptionRemediationApplyFailed
		result.Reason = "write_failed"
		registerOptionRemediationRow(db, operatorUserId, item, OptionRemediationStatusFailed, result.Reason, &result)
		return result
	}

	// Post-commit runtime publication through the existing chain. The database
	// commit stands on a publication failure (same documented boundary as the
	// typed bulk writer); the registry row is flipped to failed so the
	// operator sees the divergence instead of a silent drift.
	if err := optionRemediationPublish(item.Key, plan.newValue); err != nil {
		common.SysError("option remediation publication failed after commit for key " + item.Key + ": " + err.Error())
		result.Status = OptionRemediationApplyFailed
		result.Reason = "publish_failed"
		result.RemediationId = remediationId
		markOptionRemediationPublishFailed(db, remediationId)
		return result
	}
	result.Status = OptionRemediationApplyApplied
	result.Reason = plan.Reason
	result.RemediationId = remediationId
	return result
}

var errOptionRemediationCASConflict = errors.New("option remediation compare-and-swap lost")

// rebuildOptionRemediationPlan re-derives the judgment for one key from the
// current database state. Pair keys are planned with both rows of their pair,
// exactly as the dry-run does. The returned hash is the current stored value
// hash, independent of the judgment outcome.
func rebuildOptionRemediationPlan(db *gorm.DB, key string) (OptionRemediationPlan, string, error) {
	targetKeys := []string{key}
	if pair, ok := groupRatioOptionPairForKey(key); ok {
		targetKeys = []string{pair.canonical, pair.alias}
	}
	rows := make(map[string]OptionDiagnosticRow, len(targetKeys))
	found := make(map[string]bool, len(targetKeys))
	for _, target := range targetKeys {
		row, rowFound, err := readOptionRemediationRow(db, target)
		if err != nil {
			return OptionRemediationPlan{}, "", err
		}
		found[target] = rowFound
		if rowFound {
			rows[target] = row
		}
	}
	plans := planOptionRemediationRows(rows, found, true, nil, targetKeys)
	currentHash := ""
	if row, rowFound := rows[key]; rowFound && row.Value.Valid && !row.TooLarge && utf8.ValidString(row.Value.String) {
		currentHash = optionRemediationHash(row.Value.String)
	}
	for _, plan := range plans {
		if plan.Key == key {
			return plan, currentHash, nil
		}
	}
	return OptionRemediationPlan{Key: key, Decision: OptionRemediationDecisionKeep, ValueState: "absent", Reason: "key_absent"}, currentHash, nil
}

func findAppliedOptionRemediation(db *gorm.DB, item OptionRemediationApplyItem) (OptionRemediation, bool) {
	var remediation OptionRemediation
	result := db.Where(
		"action = ? AND target_key = ? AND old_value_hash = ? AND new_value_hash = ? AND status = ?",
		item.Action, item.Key, item.ExpectedOldValueHash, item.ExpectedNewValueHash, OptionRemediationStatusApplied,
	).Order("id DESC").Limit(1).Find(&remediation)
	if result.Error != nil || result.RowsAffected == 0 {
		return OptionRemediation{}, false
	}
	return remediation, true
}

// registerOptionRemediationRow persists a skipped/failed evidence row. These
// rows carry no backup because no write happened; the registration is
// best-effort so an evidence persistence failure never masks the outcome.
func registerOptionRemediationRow(db *gorm.DB, operatorUserId int, item OptionRemediationApplyItem, status string, reason string, result *OptionRemediationResult) {
	remediation := &OptionRemediation{
		OperatorUserId: operatorUserId,
		Action:         item.Action,
		TargetKey:      item.Key,
		OldValueHash:   item.ExpectedOldValueHash,
		NewValueHash:   item.ExpectedNewValueHash,
		Status:         status,
		Reason:         reason,
		CreatedAt:      time.Now().Unix(),
	}
	if status == OptionRemediationStatusApplied {
		remediation.AppliedAt = remediation.CreatedAt
	}
	if err := db.Create(remediation).Error; err != nil {
		common.SysError("failed to persist option remediation evidence: " + err.Error())
		return
	}
	result.RemediationId = remediation.Id
}

func markOptionRemediationPublishFailed(db *gorm.DB, remediationId int64) {
	update := db.Model(&OptionRemediation{}).
		Where("id = ? AND status = ?", remediationId, OptionRemediationStatusApplied).
		Updates(map[string]interface{}{"status": OptionRemediationStatusFailed, "reason": "publish_failed"})
	if update.Error != nil {
		common.SysError("failed to mark option remediation publication failure: " + update.Error.Error())
	}
}
