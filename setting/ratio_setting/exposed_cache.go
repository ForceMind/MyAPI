package ratio_setting

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

const exposedDataTTL = 30 * time.Second

type exposedCache struct {
	data       gin.H
	expiresAt  time.Time
	generation uint64
}

var (
	exposedData       atomic.Value
	exposedGeneration atomic.Uint64
	rebuildMu         sync.Mutex
)

func InvalidateExposedDataCache() {
	exposedGeneration.Add(1)
	exposedData.Store((*exposedCache)(nil))
}

func cloneGinH(src gin.H) gin.H {
	dst := make(gin.H, len(src))
	for k, v := range src {
		if values, ok := v.(map[string]float64); ok {
			copied := make(map[string]float64, len(values))
			for name, value := range values {
				copied[name] = value
			}
			dst[k] = copied
			continue
		}
		dst[k] = v
	}
	return dst
}

func GetExposedData() gin.H {
	generation := exposedGeneration.Load()
	if c, ok := exposedData.Load().(*exposedCache); ok && c != nil && c.generation == generation && time.Now().Before(c.expiresAt) {
		return cloneGinH(c.data)
	}
	for {
		rebuildMu.Lock()
		generation = exposedGeneration.Load()
		if c, ok := exposedData.Load().(*exposedCache); ok && c != nil && c.generation == generation && time.Now().Before(c.expiresAt) {
			data := cloneGinH(c.data)
			rebuildMu.Unlock()
			return data
		}
		newData := gin.H{
			"model_ratio":        GetModelRatioCopy(),
			"completion_ratio":   GetCompletionRatioCopy(),
			"cache_ratio":        GetCacheRatioCopy(),
			"create_cache_ratio": GetCreateCacheRatioCopy(),
			"model_price":        GetModelPriceCopy(),
		}
		if exposedGeneration.Load() != generation {
			rebuildMu.Unlock()
			continue
		}
		cache := &exposedCache{
			data:       newData,
			expiresAt:  time.Now().Add(exposedDataTTL),
			generation: generation,
		}
		exposedData.Store(cache)
		if exposedGeneration.Load() != generation {
			rebuildMu.Unlock()
			continue
		}
		result := cloneGinH(newData)
		rebuildMu.Unlock()
		return result
	}
}
