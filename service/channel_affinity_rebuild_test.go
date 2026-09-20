package service

import (
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/ForceMind/MyAPI/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func updateChannelAffinityConfigForTest(t *testing.T, values map[string]string) {
	t.Helper()
	registered := config.GlobalConfig.Get("channel_affinity_setting")
	require.NotNil(t, registered)
	require.NoError(t, config.UpdateConfigFromMap(registered, values))
}

// restoreChannelAffinityCacheForTest 恢复原配置代并按原代重建缓存实例。
func restoreChannelAffinityCacheForTest(t *testing.T) {
	original := operation_setting.GetChannelAffinitySetting()
	t.Cleanup(func() {
		registered := config.GlobalConfig.Get("channel_affinity_setting")
		require.NotNil(t, registered)
		require.NoError(t, config.UpdateConfigFromMap(registered, map[string]string{
			"max_entries":         strconv.Itoa(original.MaxEntries),
			"default_ttl_seconds": strconv.Itoa(original.DefaultTTLSeconds),
		}))
		RebuildChannelAffinityCache()
	})
}

// 保存后未重建：统计端点显示配置代新参数 + 生效实例旧参数 + rebuild_required。
// 重建后：新代参数生效，rebuild_required 消除。
func TestChannelAffinityCacheRebuildAppliesSavedGeneration(t *testing.T) {
	restoreChannelAffinityCacheForTest(t)

	before := GetChannelAffinityCacheStats()
	require.False(t, before.RebuildRequired)
	require.Equal(t, before.ConfiguredMaxEntries, before.ActiveCacheCapacity)
	require.Equal(t, before.ConfiguredDefaultTTLSeconds, before.ActiveDefaultTTLSeconds)

	newMaxEntries := before.ConfiguredMaxEntries + 123
	newTTL := before.ConfiguredDefaultTTLSeconds + 45
	updateChannelAffinityConfigForTest(t, map[string]string{
		"max_entries":         strconv.Itoa(newMaxEntries),
		"default_ttl_seconds": strconv.Itoa(newTTL),
	})

	pending := GetChannelAffinityCacheStats()
	assert.Equal(t, newMaxEntries, pending.ConfiguredMaxEntries, "统计必须显示配置代（已保存）参数")
	assert.Equal(t, newTTL, pending.ConfiguredDefaultTTLSeconds)
	assert.Equal(t, before.ActiveCacheCapacity, pending.ActiveCacheCapacity, "未重建时生效实例必须保持旧代参数")
	assert.Equal(t, before.ActiveDefaultTTLSeconds, pending.ActiveDefaultTTLSeconds)
	assert.True(t, pending.RebuildRequired, "已保存未生效必须可核对")

	params := RebuildChannelAffinityCache()
	assert.Equal(t, newMaxEntries, params.Capacity)
	assert.Equal(t, newTTL, params.DefaultTTLSeconds)
	assert.False(t, params.RebuildRequired)

	after := GetChannelAffinityCacheStats()
	assert.Equal(t, newMaxEntries, after.ActiveCacheCapacity, "重建后新代参数必须生效")
	assert.Equal(t, newTTL, after.ActiveDefaultTTLSeconds)
	assert.False(t, after.RebuildRequired)
}

// 重建语义：内存模式下旧实例条目随实例废弃（相当于清空重建），
// 进行中持有旧实例指针的调用方不受影响（不迁移）。
func TestChannelAffinityCacheRebuildSwapsInstance(t *testing.T) {
	restoreChannelAffinityCacheForTest(t)
	if getChannelAffinityCacheInstance().cache == nil {
		t.Skip("cache instance unavailable")
	}

	oldInstance := getChannelAffinityCacheInstance()
	keySuffix := "rebuild-swap-check"
	require.NoError(t, oldInstance.cache.SetWithTTL(keySuffix, 4242, time.Minute))

	// 模拟进行中的调用方：重建前已取到旧实例指针。
	inFlightCache := oldInstance.cache

	RebuildChannelAffinityCache()

	newInstance := getChannelAffinityCacheInstance()
	assert.NotSame(t, oldInstance, newInstance, "重建必须换入新实例")

	// 旧实例仍可用（进行中的请求不被打断、不迁移）。
	value, found, err := inFlightCache.Get(keySuffix)
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, 4242, value)

	// 新实例是独立缓存：新请求读新实例。内存模式（测试环境无 Redis）下
	// 旧条目随旧实例废弃，不得出现在新实例中。
	if !common.RedisEnabled {
		_, found, err = newInstance.cache.Get(keySuffix)
		require.NoError(t, err)
		assert.False(t, found, "重建后新实例不得携带旧实例的内存条目")
	}
}

// 代切换并发读写不混代：任何一次取到的实例，其 capacity/TTL 参数对
// 必须属于同一个完整代。
func TestChannelAffinityCacheRebuildConcurrentNoMixedGeneration(t *testing.T) {
	restoreChannelAffinityCacheForTest(t)

	setting := operation_setting.GetChannelAffinitySetting()
	genA := [2]int{setting.MaxEntries, setting.DefaultTTLSeconds}
	genB := [2]int{setting.MaxEntries + 77, setting.DefaultTTLSeconds + 33}

	const iterations = 100
	stop := make(chan struct{})
	errCh := make(chan string, 1)
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			instance := getChannelAffinityCacheInstance()
			pair := [2]int{instance.capacity, instance.defaultTTLSeconds}
			if pair != genA && pair != genB {
				select {
				case errCh <- "mixed generation observed":
				default:
				}
				return
			}
		}
	}()

	for i := 0; i < iterations; i++ {
		gen := genB
		if i%2 == 0 {
			gen = genA
		}
		updateChannelAffinityConfigForTest(t, map[string]string{
			"max_entries":         strconv.Itoa(gen[0]),
			"default_ttl_seconds": strconv.Itoa(gen[1]),
		})
		RebuildChannelAffinityCache()
	}
	close(stop)
	wg.Wait()

	select {
	case msg := <-errCh:
		t.Fatal(msg)
	default:
	}
}
