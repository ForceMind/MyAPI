package operation_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdatePayMethodsFailurePreservesStateAndGetterIsIsolated(t *testing.T) {
	previous := PayMethods2JsonString()
	t.Cleanup(func() { require.NoError(t, UpdatePayMethodsByJsonString(previous)) })
	const baseline = `[{"name":"Existing","type":"existing"}]`
	require.NoError(t, UpdatePayMethodsByJsonString(baseline))

	require.Error(t, UpdatePayMethodsByJsonString(`[{"name":"Next"},{"name":1}]`))
	assert.JSONEq(t, baseline, PayMethods2JsonString())
	methods := GetPayMethods()
	require.Len(t, methods, 1)
	methods[0]["name"] = "Mutated"
	assert.JSONEq(t, baseline, PayMethods2JsonString())
	require.NoError(t, UpdatePayMethodsByJsonString(`null`))
	assert.JSONEq(t, `[]`, PayMethods2JsonString())
}
