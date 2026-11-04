package service

import (
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOperationsDTORejectsUnknownIdentityAndPreservesFalse(t *testing.T) {
	fixture := map[string]interface{}{
		"id": strings.Repeat("a", 64), "digest": strings.Repeat("b", 64), "instance_id": "primary", "owner_id": 1, "key_id": 1,
		"state": "prepared", "created_at": 100, "expires_at": 1900,
		"source_image_id": "sha256:" + strings.Repeat("1", 64), "target_image_id": "sha256:" + strings.Repeat("2", 64),
		"source_container_id": strings.Repeat("3", 64), "source_config_digest": strings.Repeat("4", 64), "manifest_sha256": strings.Repeat("5", 64),
		"source_sha": strings.Repeat("6", 40), "tree_sha": strings.Repeat("7", 40), "run_id": "42", "producer_attempt": "1", "artifact_id": "43",
		"source_version": "0.2.0-beta.32", "version": "0.2.0-beta.33", "platform": "linux/amd64", "backup_created": false, "target_start_attempted": false,
		"secret": "never-return-this", "checks": map[string]interface{}{"registered_instance": true, "not_observed": nil},
	}
	encoded, err := common.Marshal(fixture)
	require.NoError(t, err)
	output, err := safeOperationsData("plan", encoded)
	require.NoError(t, err)
	assert.Contains(t, string(output), `"backup_created":false`)
	assert.Contains(t, string(output), `"not_observed":null`)
	assert.NotContains(t, string(output), "never-return-this")
	assert.NotContains(t, string(output), "source_mutation_attempted")
	for _, test := range []struct {
		name, field string
		value       interface{}
		remove      bool
	}{
		{"missing key", "key_id", nil, true}, {"null key", "key_id", nil, false}, {"zero key", "key_id", 0, false},
		{"missing backup observation", "backup_created", nil, true}, {"null backup", "backup_created", nil, false},
		{"missing source", "source_sha", nil, true}, {"missing artifact", "artifact_id", nil, true},
		{"bad source version", "source_version", "unknown", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			row := make(map[string]interface{}, len(fixture))
			for key, value := range fixture {
				row[key] = value
			}
			if test.remove {
				delete(row, test.field)
			} else {
				row[test.field] = test.value
			}
			raw, err := common.Marshal(row)
			require.NoError(t, err)
			_, err = safeOperationsData("plan", raw)
			require.Error(t, err)
		})
	}
}

func TestOperationsApprovalScopeIsBoundToPlanAndDigest(t *testing.T) {
	id, digest := strings.Repeat("a", 64), strings.Repeat("b", 64)
	assert.True(t, IsOperationsApprovalScope(OperationsApprovalScopeFor(id, digest)))
	assert.False(t, IsOperationsApprovalScope(OperationsApprovalScope))
	assert.False(t, IsOperationsApprovalScope(OperationsApprovalScope+":../other:"+digest))
	assert.Empty(t, OperationsApprovalScopeFor("wrong", digest))
}
