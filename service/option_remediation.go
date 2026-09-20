package service

import (
	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
)

const OptionRemediationSchemaVersion = "c09-option-remediation-v1"

// OptionRemediationDryRunItem is the metadata-only preview of one planned
// judgment: hashes, decision, and runtime conflict markers. Values are never
// copied into this layer, matching the diagnostics privacy contract.
type OptionRemediationDryRunItem struct {
	Key              string `json:"key,omitempty"`
	KeyRedacted      bool   `json:"key_redacted,omitempty"`
	Action           string `json:"action,omitempty"`
	Decision         string `json:"decision"`
	Reason           string `json:"reason"`
	Risk             string `json:"risk,omitempty"`
	ValueState       string `json:"value_state"`
	OldValueHash     string `json:"old_value_hash,omitempty"`
	NewValueHash     string `json:"new_value_hash,omitempty"`
	RuntimeValueHash string `json:"runtime_value_hash,omitempty"`
	RuntimeMatchesDB bool   `json:"runtime_matches_db"`
}

type OptionRemediationDryRun struct {
	SchemaVersion string                            `json:"schema_version"`
	Mode          string                            `json:"mode"`
	Coverage      model.OptionRemediationCoverage   `json:"coverage"`
	Summary       OptionRemediationDryRunSummary    `json:"summary"`
	Items         []OptionRemediationDryRunItem     `json:"items"`
}

type OptionRemediationDryRunSummary struct {
	Planned    int            `json:"planned"`
	Returned   int            `json:"returned"`
	ByDecision map[string]int `json:"by_decision"`
	ByAction   map[string]int `json:"by_action"`
}

type OptionRemediationApplyResponse struct {
	SchemaVersion string                          `json:"schema_version"`
	Mode          string                          `json:"mode"`
	Summary       OptionRemediationApplySummary   `json:"summary"`
	Items         []model.OptionRemediationResult `json:"items"`
}

type OptionRemediationApplySummary struct {
	Items          int `json:"items"`
	Applied        int `json:"applied"`
	Skipped        int `json:"skipped"`
	Failed         int `json:"failed"`
	AlreadyApplied int `json:"already_applied"`
}

// DryRunOptionRemediations previews whitelist judgments without writing
// anything: no options row, no OptionMap/runtime mutation, no registry row.
// Explicit keys return one item per requested key (plus their pair partners);
// a bounded full snapshot returns only rows with a finding or a non-keep
// decision and reports the rest in the summary.
func DryRunOptionRemediations(keys []string, includeAll bool, actions []string) (*OptionRemediationDryRun, error) {
	actionFilter := make(map[string]bool, len(actions))
	for _, action := range actions {
		known := false
		for _, candidate := range model.OptionRemediationKnownActions() {
			if action == candidate {
				known = true
				break
			}
		}
		if !known {
			return nil, model.ErrOptionRemediationInput
		}
		actionFilter[action] = true
	}
	plans, coverage, err := model.PlanOptionRemediations(keys, includeAll, actionFilter)
	if err != nil {
		return nil, err
	}
	response := &OptionRemediationDryRun{
		SchemaVersion: OptionRemediationSchemaVersion,
		Mode:          "dry_run",
		Coverage:      coverage,
		Summary: OptionRemediationDryRunSummary{
			ByDecision: map[string]int{},
			ByAction:   map[string]int{},
		},
		Items: []OptionRemediationDryRunItem{},
	}
	for _, plan := range plans {
		response.Summary.Planned++
		response.Summary.ByDecision[plan.Decision]++
		if plan.Action != "" {
			response.Summary.ByAction[plan.Action]++
		}
		if includeAll && plan.Decision == model.OptionRemediationDecisionKeep && !optionRemediationFindingState(plan.ValueState) {
			continue
		}
		response.Items = append(response.Items, optionRemediationDryRunItem(plan))
	}
	response.Summary.Returned = len(response.Items)
	return response, nil
}

func optionRemediationFindingState(valueState string) bool {
	switch valueState {
	case "raw_null", "too_large", "invalid_utf8", "empty", "blank":
		return true
	default:
		return false
	}
}

func optionRemediationDryRunItem(plan model.OptionRemediationPlan) OptionRemediationDryRunItem {
	item := OptionRemediationDryRunItem{
		Action:       plan.Action,
		Decision:     plan.Decision,
		Reason:       plan.Reason,
		Risk:         plan.Risk,
		ValueState:   plan.ValueState,
		OldValueHash: plan.OldValueHash,
		NewValueHash: plan.NewValueHash,
	}
	if optionDiagnosticSafeKey(plan.Key, plan.KeyClass) {
		item.Key = plan.Key
		item.RuntimeValueHash, item.RuntimeMatchesDB = optionRemediationRuntimeHash(plan.Key, plan.OldValueHash)
	} else {
		item.KeyRedacted = true
	}
	return item
}

// optionRemediationRuntimeHash hashes the current OptionMap projection of a
// key so the preview can flag drift between the stored row and the running
// configuration without exposing either value.
func optionRemediationRuntimeHash(key string, oldValueHash string) (string, bool) {
	common.OptionMapRWMutex.RLock()
	value, ok := common.OptionMap[key]
	common.OptionMapRWMutex.RUnlock()
	if !ok {
		return "", oldValueHash == ""
	}
	hash := model.OptionRemediationHashForPreview(value)
	return hash, hash == oldValueHash
}

// ApplyOptionRemediations applies caller-claimed dry-run action instances.
// The model rebuilds every judgment and enforces the per-row value-hash CAS;
// results are per row and already value-free.
func ApplyOptionRemediations(operatorUserId int, items []model.OptionRemediationApplyItem) (*OptionRemediationApplyResponse, error) {
	results, err := model.ApplyOptionRemediations(operatorUserId, items)
	if err != nil {
		return nil, err
	}
	response := &OptionRemediationApplyResponse{
		SchemaVersion: OptionRemediationSchemaVersion,
		Mode:          "apply",
		Items:         results,
	}
	response.Summary.Items = len(results)
	for _, result := range results {
		switch result.Status {
		case model.OptionRemediationApplyApplied:
			response.Summary.Applied++
		case model.OptionRemediationApplySkipped:
			response.Summary.Skipped++
		case model.OptionRemediationApplyFailed:
			response.Summary.Failed++
		case model.OptionRemediationApplyAlreadyApplied:
			response.Summary.AlreadyApplied++
		}
	}
	return response, nil
}

func ListOptionRemediations(startIdx int, pageSize int) ([]model.OptionRemediation, int64, error) {
	return model.ListOptionRemediations(model.DB, startIdx, pageSize)
}
