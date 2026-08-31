package service

import (
	"math"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCalculateImageTokensPatchUsesFloatAreaForLargeDimensions(t *testing.T) {
	tokens, err := calculateImageTokens(math.MaxInt64, math.MaxInt64, 85, 170, true, 2.46)

	require.NoError(t, err)
	require.Equal(t, 3742, tokens)
}

func TestCalculateImageTokensPatchPreservesNormalRounding(t *testing.T) {
	tokens, err := calculateImageTokens(1024, 1024, 85, 170, true, 1.62)

	require.NoError(t, err)
	require.Equal(t, 1659, tokens)
}

func TestCalculateImageTokensRejectsTileQuotaOverflow(t *testing.T) {
	_, err := calculateImageTokens(1024, 1024, 85, math.MaxInt64, false, 1)

	require.Error(t, err)
	require.Contains(t, err.Error(), "quota conversion")
}

func TestCalculateImageTokensRejectsInvalidDimensions(t *testing.T) {
	for _, dimensions := range [][2]int{{0, 1}, {1, 0}, {-1, 1}, {1, -1}} {
		_, err := calculateImageTokens(dimensions[0], dimensions[1], 85, 170, false, 1)

		require.EqualError(t, err, "invalid image dimensions: width="+strconv.Itoa(dimensions[0])+", height="+strconv.Itoa(dimensions[1]))
	}
}
