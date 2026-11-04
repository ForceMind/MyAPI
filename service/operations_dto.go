package service

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/ForceMind/MyAPI/common"
)

// Only these fields cross from the privileged daemon to the dashboard.
// Environment values, paths, credentials and arbitrary daemon errors stay out.
type OperationsPlan struct {
	ID                      string           `json:"id"`
	Digest                  string           `json:"digest"`
	InstanceID              string           `json:"instance_id"`
	OwnerID                 int              `json:"owner_id"`
	KeyID                   int              `json:"key_id"`
	State                   string           `json:"state"`
	CreatedAt               int64            `json:"created_at"`
	ExpiresAt               int64            `json:"expires_at"`
	ApprovedAt              *int64           `json:"approved_at,omitempty"`
	ApprovalExpiresAt       *int64           `json:"approval_expires_at,omitempty"`
	ApprovedBy              *int             `json:"approved_by,omitempty"`
	SourceImageID           string           `json:"source_image_id"`
	TargetImageID           string           `json:"target_image_id"`
	SourceContainerID       string           `json:"source_container_id"`
	TargetContainerID       string           `json:"target_container_id,omitempty"`
	SourceConfigDigest      string           `json:"source_config_digest"`
	SourceSHA               string           `json:"source_sha"`
	TreeSHA                 string           `json:"tree_sha"`
	RunID                   string           `json:"run_id"`
	ProducerAttempt         string           `json:"producer_attempt"`
	ArtifactID              string           `json:"artifact_id"`
	ManifestSHA256          string           `json:"manifest_sha256"`
	Version                 string           `json:"version"`
	SourceVersion           string           `json:"source_version,omitempty"`
	Platform                string           `json:"platform"`
	BackupCreated           *bool            `json:"backup_created"`
	TargetStartAttempted    *bool            `json:"target_start_attempted"`
	SourceMutationAttempted *bool            `json:"source_mutation_attempted,omitempty"`
	SourceRestartDisabled   *bool            `json:"source_restart_disabled,omitempty"`
	Code                    string           `json:"code,omitempty"`
	Checks                  map[string]*bool `json:"checks,omitempty"`
}

type OperationsInstance struct {
	ID                 string `json:"id"`
	State              string `json:"state"`
	CurrentImageID     string `json:"current_image_id,omitempty"`
	CurrentContainerID string `json:"current_container_id,omitempty"`
	Code               string `json:"code,omitempty"`
}

func safeOperationsData(action string, raw json.RawMessage) (json.RawMessage, error) {
	invalid := errors.New("operations response invalid")
	validatePlan := func(plan *OperationsPlan) bool {
		if !ValidOperationsPlan(plan.ID, plan.Digest) || !operationsIdentifier.MatchString(plan.InstanceID) || plan.OwnerID <= 0 || plan.KeyID <= 0 || plan.BackupCreated == nil || plan.TargetStartAttempted == nil || plan.CreatedAt <= 0 || plan.ExpiresAt <= plan.CreatedAt {
			return false
		}
		switch plan.State {
		case "prepared", "approved", "queued", "verifying_artifact", "stopping", "backing_up", "creating", "starting", "verifying", "succeeded", "failed_before_start", "recovery_required", "interrupted":
		default:
			return false
		}
		if !operationsDigest.MatchString(plan.SourceConfigDigest) || !operationsDigest.MatchString(plan.ManifestSHA256) || plan.Platform != "linux/amd64" {
			return false
		}
		if !operationsSHA.MatchString(plan.SourceSHA) || !operationsSHA.MatchString(plan.TreeSHA) || !operationsGitHubID.MatchString(plan.RunID) || !operationsGitHubID.MatchString(plan.ProducerAttempt) || !operationsGitHubID.MatchString(plan.ArtifactID) || !operationsVersion.MatchString(plan.Version) || !operationsVersion.MatchString(plan.SourceVersion) || !operationsDigest.MatchString(plan.SourceContainerID) {
			return false
		}
		if plan.Code != "" && !operationsMachineCode.MatchString(plan.Code) {
			return false
		}
		for _, image := range []string{plan.SourceImageID, plan.TargetImageID} {
			if !strings.HasPrefix(image, "sha256:") || !operationsDigest.MatchString(strings.TrimPrefix(image, "sha256:")) {
				return false
			}
		}
		return true
	}
	switch action {
	case "instances":
		var instances []OperationsInstance
		if common.Unmarshal(raw, &instances) != nil || instances == nil {
			return nil, invalid
		}
		for _, instance := range instances {
			if !operationsIdentifier.MatchString(instance.ID) || len(instance.State) == 0 || len(instance.State) > 40 {
				return nil, invalid
			}
		}
		return common.Marshal(instances)
	case "plans":
		var plans []OperationsPlan
		if common.Unmarshal(raw, &plans) != nil || plans == nil {
			return nil, invalid
		}
		for i := range plans {
			if !validatePlan(&plans[i]) {
				return nil, invalid
			}
		}
		return common.Marshal(plans)
	default:
		var plan OperationsPlan
		if common.Unmarshal(raw, &plan) != nil || !validatePlan(&plan) {
			return nil, invalid
		}
		return common.Marshal(plan)
	}
}
