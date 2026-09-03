package ratio_setting

import (
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExposedDataIgnoresCacheFromOlderGeneration(t *testing.T) {
	InvalidateExposedDataCache()
	currentGeneration := exposedGeneration.Load()
	exposedData.Store(&exposedCache{
		data:       gin.H{"model_ratio": map[string]float64{"stale": 99}},
		expiresAt:  time.Now().Add(time.Minute),
		generation: currentGeneration - 1,
	})
	t.Cleanup(InvalidateExposedDataCache)

	data := GetExposedData()
	assert.Equal(t, GetModelRatioCopy(), data["model_ratio"])
	cache, ok := exposedData.Load().(*exposedCache)
	require.True(t, ok)
	require.NotNil(t, cache)
	assert.Equal(t, exposedGeneration.Load(), cache.generation)
}

func TestExposedDataReturnsIndependentRatioMaps(t *testing.T) {
	InvalidateExposedDataCache()
	t.Cleanup(InvalidateExposedDataCache)

	first := GetExposedData()
	modelRatios, ok := first["model_ratio"].(map[string]float64)
	require.True(t, ok)
	modelRatios["local-mutation"] = 99

	second := GetExposedData()
	secondRatios, ok := second["model_ratio"].(map[string]float64)
	require.True(t, ok)
	_, mutated := secondRatios["local-mutation"]
	assert.False(t, mutated)
}
