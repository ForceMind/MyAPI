package model

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm/schema"
)

func TestOptionalTaskEvidenceUsesUnpaddedPostgresStrings(t *testing.T) {
	dialect := postgres.New(postgres.Config{})
	for _, record := range []struct {
		model  any
		fields []string
	}{
		{&TaskBillingEvent{}, []string{"EvidenceHash"}},
		{&QuotaMutationReceipt{}, []string{"EvidenceHash"}},
		{&TaskTerminalObservation{}, []string{"EvidenceHash", "LastConflictFingerprint"}},
	} {
		parsed, err := schema.Parse(record.model, &sync.Map{}, schema.NamingStrategy{})
		require.NoError(t, err)
		for _, name := range record.fields {
			assert.Equal(t, "varchar(64)", dialect.DataTypeOf(parsed.FieldsByName[name]), parsed.Name+"."+name+" must preserve the empty/no-evidence sentinel")
		}
	}
}
