package model

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLegacyPostgreSQLCharPaddingDoesNotChangeEmptyEvidenceHashes(t *testing.T) {
	event := TaskBillingEvent{EvidenceHash: strings.Repeat(" ", 64)}
	require.NoError(t, event.AfterFind(nil))
	assert.Empty(t, event.EvidenceHash)

	receipt := QuotaMutationReceipt{EvidenceHash: strings.Repeat(" ", 64)}
	require.NoError(t, receipt.AfterFind(nil))
	assert.Empty(t, receipt.EvidenceHash)

	observation := TaskTerminalObservation{
		EvidenceHash:            strings.Repeat(" ", 64),
		LastConflictFingerprint: strings.Repeat(" ", 64),
	}
	require.NoError(t, observation.AfterFind(nil))
	assert.Empty(t, observation.EvidenceHash)
	assert.Empty(t, observation.LastConflictFingerprint)
}
