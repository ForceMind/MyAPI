package common

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAny2TypeRoundTripPreservesJSONValues(t *testing.T) {
	got, err := Any2Type[map[string]any](map[string]any{
		"zero":   0,
		"false":  false,
		"nested": map[string]any{"value": "ok"},
	})
	require.NoError(t, err)
	require.Equal(t, float64(0), got["zero"])
	require.Equal(t, false, got["false"])
	require.Equal(t, "ok", got["nested"].(map[string]any)["value"])
}

func TestAny2TypeReturnsErrorsFromJSONWrapper(t *testing.T) {
	_, err := Any2Type[float64](math.Inf(1))
	require.Error(t, err)

	_, err = Any2Type[int](map[string]any{"value": "not-an-int"})
	require.Error(t, err)
}
