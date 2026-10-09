package common

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateUniqueJSONKeysRejectsAmbiguityWithoutFoldingUserFields(t *testing.T) {
	for _, input := range []string{`{"type":"image_url","type":"text"}`, `{"nested":{"url":"first","\u0075rl":"second"}}`, `{"a":1} {"b":2}`, `{"a":[}`, strings.Repeat("[", 66) + "0" + strings.Repeat("]", 66)} {
		assert.Error(t, ValidateUniqueJSONKeys([]byte(input)))
	}
	// Tool argument names may legitimately differ in case; protocol-level
	// aliases are rejected by the owning request validator, not this wrapper.
	require.NoError(t, ValidateUniqueJSONKeys([]byte(`{"tools":[{"parameters":{"X":1,"x":2}}],"n":1e2}`)))
}
