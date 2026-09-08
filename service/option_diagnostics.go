package service

import (
	"strings"
	"unicode/utf8"

	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/setting/config"
)

const OptionDiagnosticSchemaVersion = "c09-option-diagnostic-v1"

type OptionDiagnostics struct {
	SchemaVersion string                       `json:"schema_version"`
	Source        string                       `json:"source"`
	Mode          string                       `json:"mode"`
	Coverage      OptionDiagnosticCoverage     `json:"coverage"`
	Summary       OptionDiagnosticSummary      `json:"summary"`
	Items         []OptionDiagnosticItem       `json:"items"`
	AliasGroups   []OptionDiagnosticAliasGroup `json:"alias_groups"`
}

type OptionDiagnosticCoverage struct {
	RowsScanned        int      `json:"rows_scanned"`
	RowLimit           int      `json:"row_limit"`
	KeyParseCapChars   int      `json:"key_parse_cap_chars"`
	ValueParseCapChars int      `json:"value_parse_cap_chars"`
	Complete           bool     `json:"complete"`
	IncompleteReasons  []string `json:"incomplete_reasons"`
}

type OptionDiagnosticSummary struct {
	ByKeyClass    map[string]int `json:"by_key_class"`
	ByValueState  map[string]int `json:"by_value_state"`
	ByValidation  map[string]int `json:"by_validation"`
	ByAliasStatus map[string]int `json:"by_alias_status"`
}

type OptionDiagnosticItem struct {
	Key             string `json:"key,omitempty"`
	KeyRedacted     bool   `json:"key_redacted,omitempty"`
	KeyClass        string `json:"key_class"`
	ValueState      string `json:"value_state"`
	Validation      string `json:"validation,omitempty"`
	EffectiveSource string `json:"effective_source,omitempty"`
	LoadOrder       string `json:"load_order,omitempty"`
}

type OptionDiagnosticAliasGroup struct {
	Name            string   `json:"name"`
	Keys            []string `json:"keys"`
	Status          string   `json:"status"`
	EffectiveSource string   `json:"effective_source"`
}

// GetOptionDiagnostics builds a metadata-only snapshot. includeValid controls
// whether entries without a stable diagnostic finding are returned; values are
// never copied into the response, errors, or logs.
func GetOptionDiagnostics(includeValid bool) (*OptionDiagnostics, error) {
	rows, complete, err := model.ReadOptionDiagnosticRows()
	if err != nil {
		return nil, err
	}
	diagnostics := &OptionDiagnostics{
		SchemaVersion: OptionDiagnosticSchemaVersion,
		Source:        "primary_database.options",
		Mode:          "read_only",
		Coverage: OptionDiagnosticCoverage{
			RowsScanned:        len(rows),
			RowLimit:           model.OptionDiagnosticRowLimit,
			KeyParseCapChars:   model.OptionDiagnosticKeyParseCapChars,
			ValueParseCapChars: model.OptionDiagnosticValueParseCapChars,
			Complete:           complete,
			IncompleteReasons:  []string{},
		},
		Summary: OptionDiagnosticSummary{ByKeyClass: map[string]int{}, ByValueState: map[string]int{}, ByValidation: map[string]int{}, ByAliasStatus: map[string]int{}},
		Items:   []OptionDiagnosticItem{},
	}
	if !complete {
		diagnostics.Coverage.IncompleteReasons = append(diagnostics.Coverage.IncompleteReasons, "row_limit_reached")
	}

	for _, row := range rows {
		item := assessOptionDiagnosticRow(row)
		diagnostics.Summary.ByKeyClass[item.KeyClass]++
		diagnostics.Summary.ByValueState[item.ValueState]++
		if item.Validation != "" {
			diagnostics.Summary.ByValidation[item.Validation]++
		}
		if includeValid || optionDiagnosticFinding(item) {
			diagnostics.Items = append(diagnostics.Items, item)
		}
	}
	diagnostics.AliasGroups = optionDiagnosticAliasGroups(rows, complete)
	for _, group := range diagnostics.AliasGroups {
		diagnostics.Summary.ByAliasStatus[group.Status]++
	}
	return diagnostics, nil
}

func assessOptionDiagnosticRow(row model.OptionDiagnosticRow) OptionDiagnosticItem {
	item := OptionDiagnosticItem{ValueState: optionDiagnosticValueState(row)}
	if row.KeyTooLarge {
		item.KeyClass = "malformed_key"
		item.KeyRedacted = true
		item.Validation = optionDiagnosticValueStateValidation(item.ValueState)
		return item
	}
	assessment := config.GlobalConfig.AssessOptionDiagnostic(row.Key, "")
	item.KeyClass = assessment.KeyClass
	if item.ValueState == "present" {
		assessment = config.GlobalConfig.AssessOptionDiagnostic(row.Key, row.Value.String)
		item.KeyClass = assessment.KeyClass
		item.Validation = assessment.Validation
	} else {
		item.Validation = optionDiagnosticValueStateValidation(item.ValueState)
	}
	if isRetiredOptionDiagnosticKey(row.Key) {
		item.KeyClass = "retired_legacy"
	}
	if strings.HasPrefix(row.Key, "model_deployment.ionet.") {
		item.KeyClass = "known_external"
	}
	if row.Key == "GroupRatio" || row.Key == "GroupGroupRatio" {
		item.KeyClass = "legacy_flat"
	}
	if row.Key == "DisplayInCurrencyEnabled" || row.Key == "general_setting.quota_display_type" || row.Key == "ChannelQuotaAlertSettings" || strings.HasPrefix(row.Key, "ChannelQuota") {
		item.LoadOrder = "load_order_ambiguous"
		item.EffectiveSource = "unresolved"
	}
	if optionDiagnosticSafeKey(row.Key, item.KeyClass) {
		item.Key = row.Key
	} else {
		item.KeyRedacted = true
	}
	return item
}

func optionDiagnosticValueState(row model.OptionDiagnosticRow) string {
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

func optionDiagnosticValueStateValidation(valueState string) string {
	switch valueState {
	case "raw_null", "empty", "blank", "invalid_utf8", "too_large":
		return valueState
	default:
		return ""
	}
}

func optionDiagnosticFinding(item OptionDiagnosticItem) bool {
	return item.ValueState != "present" || item.Validation != "" || item.KeyRedacted || item.KeyClass == "schema_indeterminate" || item.KeyClass == "retired_legacy" || item.LoadOrder != ""
}

func optionDiagnosticAliasGroups(rows []model.OptionDiagnosticRow, complete bool) []OptionDiagnosticAliasGroup {
	assessments := model.AssessOptionDiagnosticAliasGroups(rows, complete)
	groups := make([]OptionDiagnosticAliasGroup, 0, len(assessments))
	for _, assessment := range assessments {
		groups = append(groups, OptionDiagnosticAliasGroup{
			Name:            assessment.Name,
			Keys:            assessment.Keys,
			Status:          assessment.Status,
			EffectiveSource: assessment.EffectiveSource,
		})
	}
	return groups
}

func isRetiredOptionDiagnosticKey(key string) bool {
	switch key {
	case "theme.frontend", "ApiInfo", "Announcements", "FAQ", "UptimeKumaUrl", "UptimeKumaSlug":
		return true
	default:
		return false
	}
}

func optionDiagnosticSafeKey(key, keyClass string) bool {
	if keyClass == "registered_field" || keyClass == "retired_legacy" {
		return true
	}
	if keyClass != "legacy_flat" {
		return false
	}
	switch key {
	case "GroupRatio", "GroupGroupRatio", "DisplayInCurrencyEnabled", "ChannelQuotaAlertSettings":
		return true
	default:
		return false
	}
}
