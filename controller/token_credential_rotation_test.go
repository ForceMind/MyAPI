package controller

import (
	"errors"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestTokenCredentialRotationStrictRequestContract(t *testing.T) {
	valid := `{"operation_id":"` + strings.Repeat("a", 64) + `","preview_revision":"` + strings.Repeat("b", 64) + `","confirmed":true}`
	for _, test := range []struct {
		name       string
		body       string
		committing bool
		valid      bool
	}{
		{"empty preview object", `{}`, false, true},
		{"explicit confirmed command", valid, true, true},
		{"empty body", ``, false, false},
		{"null preview", `null`, false, false},
		{"array preview", `[]`, false, false},
		{"trailing preview", `{} {}`, false, false},
		{"client controls secret", `{"secret":"synthetic-forbidden"}`, false, false},
		{"commit fields in preview", valid, false, false},
		{"missing command", `{}`, true, false},
		{"case alias", strings.Replace(valid, "operation_id", "Operation_ID", 1), true, false},
		{"duplicate operation", strings.Replace(valid, `"confirmed":true`, `"confirmed":true,"operation_id":"duplicate"`, 1), true, false},
		{"null operation", strings.Replace(valid, `"`+strings.Repeat("a", 64)+`"`, `null`, 1), true, false},
		{"unknown owner", strings.Replace(valid, `"confirmed":true`, `"confirmed":true,"owner":1`, 1), true, false},
		{"boolean alias", strings.Replace(valid, `"confirmed":true`, `"confirmed":"true"`, 1), true, false},
		{"oversized", `{"extra":"` + strings.Repeat("x", 8192) + `"}`, true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			c, _ := tokenMaintenanceContext(t, 1, "11", test.body)
			command, err := decodeTokenCredentialRotationRequest(c, test.committing)
			if !test.valid {
				require.ErrorIs(t, err, model.ErrTokenMaintenanceInvalid)
				return
			}
			require.NoError(t, err)
			if test.committing {
				assert.Equal(t, strings.Repeat("a", 64), command.OperationID)
				assert.Equal(t, strings.Repeat("b", 64), command.PreviewRevision)
				assert.True(t, command.Confirmed)
			}
		})
	}
}

func TestTokenCredentialRotationAcceptanceErrors(t *testing.T) {
	require.NoError(t, i18n.Init())
	for _, test := range []struct {
		name        string
		err         error
		status      int
		disposition string
	}{
		{"invalid pre-apply", model.ErrTokenMaintenanceInvalid, 400, "not_accepted"},
		{"denied pre-apply", model.ErrTokenMaintenanceDenied, 403, "not_accepted"},
		{"unsupported pre-apply", model.ErrTokenMaintenanceBlocked, 409, "not_accepted"},
		{"stale preview rejected", model.ErrTokenMaintenanceConflict, 409, "not_accepted"},
		{"unknown precondition rejected", model.ErrTokenMaintenancePreconditionUnknown, 412, "not_accepted"},
		{"operation already has another request", model.ErrTokenCredentialRotationOperationConflict, 409, "unknown"},
		{"postcommit cache failure", model.ErrTokenCredentialRotationOutcomeUnknown, 503, "unknown"},
		{"postcommit read missing", errors.Join(model.ErrTokenCredentialRotationOutcomeUnknown, gorm.ErrRecordNotFound), 503, "unknown"},
		{"database outcome ambiguous", gorm.ErrInvalidDB, 503, "unknown"},
	} {
		t.Run(test.name, func(t *testing.T) {
			c, recorder := tokenMaintenanceContext(t, 1, "11", "{}")
			c.Header(tokenCredentialRotationDispositionHeader, "not_accepted")
			tokenCredentialRotationError(c, test.err)
			assert.Equal(t, test.status, recorder.Code)
			assert.Equal(t, test.disposition, recorder.Header().Get(tokenCredentialRotationDispositionHeader))
			var response struct {
				Success bool `json:"success"`
				Data    struct {
					Disposition string `json:"commit_disposition"`
				} `json:"data"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			assert.False(t, response.Success)
			assert.Equal(t, test.disposition, response.Data.Disposition)
		})
	}
}
