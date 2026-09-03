package setting

import (
	"math"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateModelRequestRateLimitGroupFailurePreservesExistingGroup(t *testing.T) {
	previous := GetModelRequestRateLimitConfig()
	baseline := ModelRequestRateLimitConfig{
		Enabled:         true,
		DurationMinutes: 3,
		Total:           10,
		Success:         8,
		Group:           map[string][2]int{"existing": {7, 5}},
	}
	require.NoError(t, ApplyModelRequestRateLimitConfig(baseline))
	t.Cleanup(func() {
		require.NoError(t, ApplyModelRequestRateLimitConfig(previous))
	})

	err := UpdateModelRequestRateLimitGroupByJSONString(`{"new":[9,4],"wrong":"not-a-pair"}`)
	require.Error(t, err)
	assert.Equal(t, baseline, GetModelRequestRateLimitConfig())
}

func TestModelRequestRateLimitConfigCopiesGroupMap(t *testing.T) {
	previous := GetModelRequestRateLimitConfig()
	t.Cleanup(func() { require.NoError(t, ApplyModelRequestRateLimitConfig(previous)) })
	config := ModelRequestRateLimitConfig{
		Enabled:         true,
		DurationMinutes: 2,
		Total:           20,
		Success:         10,
		Group:           map[string][2]int{"vip": {8, 6}},
	}
	require.NoError(t, ApplyModelRequestRateLimitConfig(config))

	config.Group["vip"] = [2]int{99, 99}
	snapshot := GetModelRequestRateLimitConfig()
	assert.Equal(t, [2]int{8, 6}, snapshot.Group["vip"])
	snapshot.Group["vip"] = [2]int{77, 77}
	assert.Equal(t, [2]int{8, 6}, GetModelRequestRateLimitConfig().Group["vip"])
}

func TestResolveModelRequestRateLimitSeesOnlyAtomicConfigurations(t *testing.T) {
	previous := GetModelRequestRateLimitConfig()
	t.Cleanup(func() { require.NoError(t, ApplyModelRequestRateLimitConfig(previous)) })
	oldConfig := ModelRequestRateLimitConfig{
		Enabled:         false,
		DurationMinutes: 1,
		Total:           10,
		Success:         8,
		Group:           map[string][2]int{"vip": {7, 5}},
	}
	newConfig := ModelRequestRateLimitConfig{
		Enabled:         true,
		DurationMinutes: 9,
		Total:           100,
		Success:         80,
		Group:           map[string][2]int{"vip": {70, 50}},
	}
	require.NoError(t, ApplyModelRequestRateLimitConfig(oldConfig))

	start := make(chan struct{})
	results := make(chan ModelRequestRateLimitSnapshot, 8)
	applyResult := make(chan error, 1)
	var workers sync.WaitGroup
	workers.Add(9)
	for range 8 {
		go func() {
			defer workers.Done()
			<-start
			results <- ResolveModelRequestRateLimit("vip")
		}()
	}
	go func() {
		defer workers.Done()
		<-start
		applyResult <- ApplyModelRequestRateLimitConfig(newConfig)
	}()
	close(start)
	workers.Wait()
	close(results)
	require.NoError(t, <-applyResult)

	wantOld := ModelRequestRateLimitSnapshot{Enabled: false, DurationMinutes: 1, Total: 7, Success: 5}
	wantNew := ModelRequestRateLimitSnapshot{Enabled: true, DurationMinutes: 9, Total: 70, Success: 50}
	for result := range results {
		assert.True(t, result == wantOld || result == wantNew, "unexpected mixed snapshot: %+v", result)
	}
}

func TestValidateModelRequestRateLimitConfigRejectsDurationAndCapacityOverflow(t *testing.T) {
	maxDurationMinutes := int(int64(math.MaxInt64) / int64(time.Minute))
	valid := ModelRequestRateLimitConfig{
		DurationMinutes: maxDurationMinutes,
		Total:           1,
		Success:         1,
		Group:           map[string][2]int{"vip": {1, 1}},
	}
	require.NoError(t, ValidateModelRequestRateLimitConfig(valid))

	durationOverflow := valid
	durationOverflow.DurationMinutes++
	require.Error(t, ValidateModelRequestRateLimitConfig(durationOverflow))

	capacityOverflow := valid
	capacityOverflow.Total = math.MaxInt32
	require.Error(t, ValidateModelRequestRateLimitConfig(capacityOverflow))

	groupCapacityOverflow := valid
	groupCapacityOverflow.Group = map[string][2]int{"vip": {math.MaxInt32, 1}}
	require.Error(t, ValidateModelRequestRateLimitConfig(groupCapacityOverflow))
}

func TestEnabledModelRequestRateLimitRequiresPositiveDuration(t *testing.T) {
	config := ModelRequestRateLimitConfig{
		Enabled:         false,
		DurationMinutes: 0,
		Total:           1,
		Success:         1,
		Group:           map[string][2]int{},
	}
	require.NoError(t, ValidateModelRequestRateLimitConfig(config))
	config.Enabled = true
	require.Error(t, ValidateModelRequestRateLimitConfig(config))

	previous := GetModelRequestRateLimitConfig()
	t.Cleanup(func() { require.NoError(t, ApplyModelRequestRateLimitConfig(previous)) })
	require.NoError(t, ApplyModelRequestRateLimitConfig(ModelRequestRateLimitConfig{
		Enabled:         false,
		DurationMinutes: 0,
		Total:           1,
		Success:         1,
		Group:           map[string][2]int{},
	}))
	require.Error(t, SetModelRequestRateLimitEnabled(true))
	assert.False(t, GetModelRequestRateLimitConfig().Enabled)
}

func TestRateLimitTypedSettersRejectOverflowWithoutPartialMutation(t *testing.T) {
	previous := GetModelRequestRateLimitConfig()
	t.Cleanup(func() { require.NoError(t, ApplyModelRequestRateLimitConfig(previous)) })
	baseline := ModelRequestRateLimitConfig{
		DurationMinutes: 1,
		Total:           math.MaxInt32,
		Success:         1,
		Group:           map[string][2]int{"vip": {math.MaxInt32, 1}},
	}
	require.NoError(t, ApplyModelRequestRateLimitConfig(baseline))
	maxDurationMinutes := int(int64(math.MaxInt64) / int64(time.Minute))

	require.Error(t, SetModelRequestRateLimitDurationMinutes(maxDurationMinutes))
	assert.Equal(t, baseline, GetModelRequestRateLimitConfig())
	groupBaseline := ModelRequestRateLimitConfig{
		DurationMinutes: maxDurationMinutes,
		Total:           0,
		Success:         1,
		Group:           map[string][2]int{"vip": {1, 1}},
	}
	require.NoError(t, ApplyModelRequestRateLimitConfig(groupBaseline))
	require.Error(t, UpdateModelRequestRateLimitGroupByJSONString(`{"vip":[2147483647,1]}`))
	assert.Equal(t, groupBaseline, GetModelRequestRateLimitConfig())
}
