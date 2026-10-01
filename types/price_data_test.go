package types

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPriceDataQuotaUnitSnapshot(t *testing.T) {
	var price PriceData
	assert.Equal(t, 400.0, price.QuotedQuotaUnit(400), "legacy callers keep their existing unit")
	require.NoError(t, price.CaptureQuotaUnit(100))
	assert.Equal(t, 100.0, price.QuotedQuotaUnit(400))
	copy := price
	assert.Equal(t, 100.0, copy.QuotedQuotaUnit(500))

	for _, test := range []struct {
		name string
		unit float64
	}{
		{"zero", 0}, {"negative", -1}, {"nan", math.NaN()},
		{"positive infinity", math.Inf(1)}, {"negative infinity", math.Inf(-1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			require.Error(t, price.CaptureQuotaUnit(test.unit))
			assert.Equal(t, 100.0, price.QuotedQuotaUnit(400), "invalid capture must not replace the quote")
		})
	}
}

func TestPriceDataToolPricesAreCopiedAndValidated(t *testing.T) {
	var price PriceData
	_, captured := price.QuotedToolPrice("free")
	assert.False(t, captured)
	input := map[string]float64{"paid": 5, "free": 0}
	require.NoError(t, price.CaptureToolPrices(input))
	input["paid"] = 20
	for _, test := range []struct {
		name string
		want float64
	}{{"paid", 5}, {"free", 0}, {"missing", 0}} {
		got, captured := price.QuotedToolPrice(test.name)
		assert.True(t, captured)
		assert.Equal(t, test.want, got)
	}
	for _, bad := range []float64{-1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		require.Error(t, price.CaptureToolPrices(map[string]float64{"paid": bad}))
		got, captured := price.QuotedToolPrice("paid")
		assert.True(t, captured)
		assert.Equal(t, 5.0, got)
	}
	require.NoError(t, price.CaptureToolPrices(nil))
	_, captured = price.QuotedToolPrice("missing")
	assert.True(t, captured, "an empty captured generation is not a legacy caller")
}
