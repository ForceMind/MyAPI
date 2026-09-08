package model

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/setting/config"
)

const OptionDiagnosticRowLimit = 1024
const OptionDiagnosticKeyParseCapChars = config.OptionDiagnosticMaxKeyChars
const OptionDiagnosticValueParseCapChars = 65536

var ErrOptionDiagnosticsUnavailable = errors.New("option diagnostics unavailable")

// OptionDiagnosticRow is a bounded database-only representation. Value is
// intentionally not serialized and must not leave the service layer.
type OptionDiagnosticRow struct {
	Key         string         `gorm:"column:key" json:"-"`
	KeyTooLarge bool           `gorm:"column:key_too_large" json:"-"`
	Value       sql.NullString `gorm:"column:value" json:"-"`
	TooLarge    bool           `gorm:"column:too_large" json:"-"`
}

// OptionDiagnosticAliasAssessment is metadata derived from the existing
// GroupRatio load precedence. It contains neither raw source nor normalized values.
type OptionDiagnosticAliasAssessment struct {
	Name            string
	Keys            []string
	Status          string
	EffectiveSource string
}

// ReadOptionDiagnosticRows performs the diagnostics' sole primary-database
// operation. SUBSTR is supported by SQLite, MySQL, and PostgreSQL. Keys are
// projected at their safe maximum and an independent one-character probe marks
// truncation; values retain one extra parse character to preserve the existing
// size-state contract. All SQL identifiers and limits are implementation constants.
func ReadOptionDiagnosticRows() ([]OptionDiagnosticRow, bool, error) {
	if DB == nil {
		return nil, false, ErrOptionDiagnosticsUnavailable
	}
	rows := make([]OptionDiagnosticRow, 0, OptionDiagnosticRowLimit+1)
	keyColumn := commonKeyCol
	if keyColumn == "" {
		if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
			keyColumn = `"key"`
		} else {
			keyColumn = "`key`"
		}
	}
	selectColumns := fmt.Sprintf(
		"SUBSTR(%s, 1, ?) AS %s, CASE WHEN SUBSTR(%s, ?, 1) = '' THEN 0 ELSE 1 END AS key_too_large, "+
			"SUBSTR(value, 1, ?) AS value, CASE WHEN value IS NULL OR SUBSTR(value, ?, 1) = '' THEN 0 ELSE 1 END AS too_large",
		keyColumn, keyColumn, keyColumn,
	)
	err := DB.Model(&Option{}).
		Select(
			selectColumns,
			OptionDiagnosticKeyParseCapChars,
			OptionDiagnosticKeyParseCapChars+1,
			OptionDiagnosticValueParseCapChars+1,
			OptionDiagnosticValueParseCapChars+1,
		).
		Order(keyColumn + " ASC").
		Limit(OptionDiagnosticRowLimit + 1).
		Scan(&rows).Error
	if err != nil {
		return nil, false, ErrOptionDiagnosticsUnavailable
	}
	complete := len(rows) <= OptionDiagnosticRowLimit
	if !complete {
		rows = rows[:OptionDiagnosticRowLimit]
	}
	return rows, complete, nil
}

// AssessOptionDiagnosticAliasGroups mirrors publishLoadedGroupRatioOptionPair
// without publishing, logging, or exposing raw/normalized values.
func AssessOptionDiagnosticAliasGroups(rows []OptionDiagnosticRow, complete bool) []OptionDiagnosticAliasAssessment {
	if !complete {
		assessments := make([]OptionDiagnosticAliasAssessment, 0, len(groupRatioOptionPairs))
		for _, pair := range groupRatioOptionPairs {
			assessments = append(assessments, OptionDiagnosticAliasAssessment{
				Name:            optionDiagnosticAliasName(pair),
				Keys:            []string{pair.canonical, pair.alias},
				Status:          "coverage_incomplete",
				EffectiveSource: "unresolved",
			})
		}
		return assessments
	}
	values := make(map[string]OptionDiagnosticRow, len(rows))
	for _, row := range rows {
		if row.KeyTooLarge {
			continue
		}
		values[row.Key] = row
	}
	assessments := make([]OptionDiagnosticAliasAssessment, 0, len(groupRatioOptionPairs))
	for _, pair := range groupRatioOptionPairs {
		canonical, canonicalPresent := values[pair.canonical]
		alias, aliasPresent := values[pair.alias]
		canonicalNormalized, canonicalState := normalizeOptionDiagnosticAliasValue(pair, canonical, canonicalPresent)
		aliasNormalized, aliasState := normalizeOptionDiagnosticAliasValue(pair, alias, aliasPresent)
		assessment := OptionDiagnosticAliasAssessment{
			Name:            optionDiagnosticAliasName(pair),
			Keys:            []string{pair.canonical, pair.alias},
			Status:          "absent",
			EffectiveSource: "unresolved",
		}
		switch {
		case !canonicalPresent && !aliasPresent:
			// No database source exists; boot/runtime defaults remain in effect.
		case canonicalState == optionDiagnosticAliasIndeterminate || aliasState == optionDiagnosticAliasIndeterminate:
			assessment.Status = "source_indeterminate"
		case canonicalState == optionDiagnosticAliasValid:
			assessment.EffectiveSource = "canonical"
			switch {
			case !aliasPresent:
				assessment.Status = "canonical_valid_alias_absent"
			case aliasState != optionDiagnosticAliasValid:
				assessment.Status = "canonical_valid_alias_invalid"
			case canonicalNormalized == aliasNormalized:
				assessment.Status = "canonical_valid_alias_matching"
			default:
				assessment.Status = "canonical_valid_alias_conflicting"
			}
		case aliasState == optionDiagnosticAliasValid:
			assessment.Status = "canonical_invalid_alias_valid"
			if !canonicalPresent {
				assessment.Status = "canonical_absent_alias_valid"
			}
			assessment.EffectiveSource = "alias"
		case canonicalPresent && aliasPresent:
			assessment.Status = "canonical_invalid_alias_invalid"
			assessment.EffectiveSource = "runtime_boot_fallback"
		case canonicalPresent:
			assessment.Status = "canonical_invalid_alias_absent"
			assessment.EffectiveSource = "runtime_boot_fallback"
		default:
			assessment.Status = "canonical_absent_alias_invalid"
			assessment.EffectiveSource = "runtime_boot_fallback"
		}
		assessments = append(assessments, assessment)
	}
	return assessments
}

type optionDiagnosticAliasValueState uint8

const (
	optionDiagnosticAliasInvalid optionDiagnosticAliasValueState = iota
	optionDiagnosticAliasValid
	optionDiagnosticAliasIndeterminate
)

func normalizeOptionDiagnosticAliasValue(pair groupRatioOptionPair, row OptionDiagnosticRow, present bool) (string, optionDiagnosticAliasValueState) {
	if !present {
		return "", optionDiagnosticAliasInvalid
	}
	if row.KeyTooLarge || !row.Value.Valid || row.TooLarge || !utf8.ValidString(row.Value.String) {
		return "", optionDiagnosticAliasIndeterminate
	}
	if strings.TrimSpace(row.Value.String) == "" {
		return "", optionDiagnosticAliasInvalid
	}
	normalized, err := normalizeGroupRatioOptionValue(pair, row.Value.String)
	if err != nil {
		return "", optionDiagnosticAliasInvalid
	}
	return normalized, optionDiagnosticAliasValid
}

func optionDiagnosticAliasName(pair groupRatioOptionPair) string {
	if pair.canonical == groupRatioOptionKey {
		return "group_ratio"
	}
	return "group_group_ratio"
}
