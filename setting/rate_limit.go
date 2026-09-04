package setting

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/common/limiter"
)

var ModelRequestRateLimitEnabled = false
var ModelRequestRateLimitDurationMinutes = 1
var ModelRequestRateLimitCount = 0
var ModelRequestRateLimitSuccessCount = 1000
var ModelRequestRateLimitGroup = map[string][2]int{}
var ModelRequestRateLimitMutex sync.RWMutex

type ModelRequestRateLimitConfig struct {
	Enabled         bool
	DurationMinutes int
	Total           int
	Success         int
	Group           map[string][2]int
}

type ModelRequestRateLimitSnapshot struct {
	Enabled         bool
	DurationMinutes int
	Total           int
	Success         int
}

func GetModelRequestRateLimitConfig() ModelRequestRateLimitConfig {
	ModelRequestRateLimitMutex.RLock()
	defer ModelRequestRateLimitMutex.RUnlock()
	return ModelRequestRateLimitConfig{
		Enabled:         ModelRequestRateLimitEnabled,
		DurationMinutes: ModelRequestRateLimitDurationMinutes,
		Total:           ModelRequestRateLimitCount,
		Success:         ModelRequestRateLimitSuccessCount,
		Group:           cloneModelRequestRateLimitGroup(ModelRequestRateLimitGroup),
	}
}

func ApplyModelRequestRateLimitConfig(config ModelRequestRateLimitConfig) error {
	if err := ValidateModelRequestRateLimitConfig(config); err != nil {
		return err
	}
	group := cloneModelRequestRateLimitGroup(config.Group)
	ModelRequestRateLimitMutex.Lock()
	ModelRequestRateLimitEnabled = config.Enabled
	ModelRequestRateLimitDurationMinutes = config.DurationMinutes
	ModelRequestRateLimitCount = config.Total
	ModelRequestRateLimitSuccessCount = config.Success
	ModelRequestRateLimitGroup = group
	ModelRequestRateLimitMutex.Unlock()
	return nil
}

func SetModelRequestRateLimitEnabled(enabled bool) error {
	ModelRequestRateLimitMutex.Lock()
	config := ModelRequestRateLimitConfig{
		Enabled:         enabled,
		DurationMinutes: ModelRequestRateLimitDurationMinutes,
		Total:           ModelRequestRateLimitCount,
		Success:         ModelRequestRateLimitSuccessCount,
		Group:           ModelRequestRateLimitGroup,
	}
	if err := ValidateModelRequestRateLimitConfig(config); err != nil {
		ModelRequestRateLimitMutex.Unlock()
		return err
	}
	ModelRequestRateLimitEnabled = enabled
	ModelRequestRateLimitMutex.Unlock()
	return nil
}

func SetModelRequestRateLimitDurationMinutes(durationMinutes int) error {
	ModelRequestRateLimitMutex.Lock()
	config := ModelRequestRateLimitConfig{
		Enabled:         ModelRequestRateLimitEnabled,
		DurationMinutes: durationMinutes,
		Total:           ModelRequestRateLimitCount,
		Success:         ModelRequestRateLimitSuccessCount,
		Group:           ModelRequestRateLimitGroup,
	}
	if err := ValidateModelRequestRateLimitConfig(config); err != nil {
		ModelRequestRateLimitMutex.Unlock()
		return err
	}
	ModelRequestRateLimitDurationMinutes = durationMinutes
	ModelRequestRateLimitMutex.Unlock()
	return nil
}

func SetModelRequestRateLimitCount(count int) error {
	ModelRequestRateLimitMutex.Lock()
	config := ModelRequestRateLimitConfig{
		Enabled:         ModelRequestRateLimitEnabled,
		DurationMinutes: ModelRequestRateLimitDurationMinutes,
		Total:           count,
		Success:         ModelRequestRateLimitSuccessCount,
		Group:           ModelRequestRateLimitGroup,
	}
	if err := ValidateModelRequestRateLimitConfig(config); err != nil {
		ModelRequestRateLimitMutex.Unlock()
		return err
	}
	ModelRequestRateLimitCount = count
	ModelRequestRateLimitMutex.Unlock()
	return nil
}

func SetModelRequestRateLimitSuccessCount(count int) error {
	ModelRequestRateLimitMutex.Lock()
	config := ModelRequestRateLimitConfig{
		Enabled:         ModelRequestRateLimitEnabled,
		DurationMinutes: ModelRequestRateLimitDurationMinutes,
		Total:           ModelRequestRateLimitCount,
		Success:         count,
		Group:           ModelRequestRateLimitGroup,
	}
	if err := ValidateModelRequestRateLimitConfig(config); err != nil {
		ModelRequestRateLimitMutex.Unlock()
		return err
	}
	ModelRequestRateLimitSuccessCount = count
	ModelRequestRateLimitMutex.Unlock()
	return nil
}

func ResolveModelRequestRateLimit(group string) ModelRequestRateLimitSnapshot {
	ModelRequestRateLimitMutex.RLock()
	defer ModelRequestRateLimitMutex.RUnlock()
	snapshot := ModelRequestRateLimitSnapshot{
		Enabled:         ModelRequestRateLimitEnabled,
		DurationMinutes: ModelRequestRateLimitDurationMinutes,
		Total:           ModelRequestRateLimitCount,
		Success:         ModelRequestRateLimitSuccessCount,
	}
	if limits, found := ModelRequestRateLimitGroup[group]; found {
		snapshot.Total = limits[0]
		snapshot.Success = limits[1]
	}
	return snapshot
}

func ModelRequestRateLimitGroup2JSONString() string {
	ModelRequestRateLimitMutex.RLock()
	group := cloneModelRequestRateLimitGroup(ModelRequestRateLimitGroup)
	ModelRequestRateLimitMutex.RUnlock()

	jsonBytes, err := common.Marshal(group)
	if err != nil {
		common.SysLog("error marshalling model request rate limit group: " + err.Error())
		return "{}"
	}
	return string(jsonBytes)
}

func ParseModelRequestRateLimitGroupJSON(jsonStr string) (map[string][2]int, error) {
	group := make(map[string][2]int)
	if err := common.Unmarshal([]byte(jsonStr), &group); err != nil {
		return nil, err
	}
	if group == nil {
		return nil, fmt.Errorf("model request rate limit group must be a JSON object")
	}
	if err := validateModelRequestRateLimitGroup(group); err != nil {
		return nil, err
	}
	return group, nil
}

func UpdateModelRequestRateLimitGroupByJSONString(jsonStr string) error {
	group, err := ParseModelRequestRateLimitGroupJSON(jsonStr)
	if err != nil {
		return err
	}
	ModelRequestRateLimitMutex.Lock()
	config := ModelRequestRateLimitConfig{
		Enabled:         ModelRequestRateLimitEnabled,
		DurationMinutes: ModelRequestRateLimitDurationMinutes,
		Total:           ModelRequestRateLimitCount,
		Success:         ModelRequestRateLimitSuccessCount,
		Group:           group,
	}
	if err := ValidateModelRequestRateLimitConfig(config); err != nil {
		ModelRequestRateLimitMutex.Unlock()
		return err
	}
	ModelRequestRateLimitGroup = cloneModelRequestRateLimitGroup(group)
	ModelRequestRateLimitMutex.Unlock()
	return nil
}

func GetGroupRateLimit(group string) (totalCount, successCount int, found bool) {
	ModelRequestRateLimitMutex.RLock()
	defer ModelRequestRateLimitMutex.RUnlock()
	limits, found := ModelRequestRateLimitGroup[group]
	if !found {
		return 0, 0, false
	}
	return limits[0], limits[1], true
}

func CheckModelRequestRateLimitGroup(jsonStr string) error {
	_, err := ParseModelRequestRateLimitGroupJSON(jsonStr)
	return err
}

func cloneModelRequestRateLimitGroup(group map[string][2]int) map[string][2]int {
	if group == nil {
		return nil
	}
	cloned := make(map[string][2]int, len(group))
	for name, limits := range group {
		cloned[name] = limits
	}
	return cloned
}

func ValidateModelRequestRateLimitConfig(config ModelRequestRateLimitConfig) error {
	maxDurationMinutes := int64(math.MaxInt64) / int64(time.Minute)
	if config.DurationMinutes < 0 || int64(config.DurationMinutes) > maxDurationMinutes {
		return fmt.Errorf("model request rate limit duration must be between 0 and %d", maxDurationMinutes)
	}
	if config.Enabled && config.DurationMinutes == 0 {
		return fmt.Errorf("model request rate limit duration must be at least 1 minute when enabled")
	}
	if config.Total < 0 || config.Total > math.MaxInt32 {
		return fmt.Errorf("model request rate limit count must be between 0 and %d", math.MaxInt32)
	}
	if config.Success < 1 || config.Success > math.MaxInt32 {
		return fmt.Errorf("model request rate limit success count must be between 1 and %d", math.MaxInt32)
	}
	if err := validateModelRequestRateLimitGroup(config.Group); err != nil {
		return err
	}
	durationSeconds := int64(config.DurationMinutes) * 60
	if err := validateModelRequestRateLimitBucket("model request", config.Total, durationSeconds); err != nil {
		return err
	}
	for name, limits := range config.Group {
		if err := validateModelRequestRateLimitBucket("group "+name, limits[0], durationSeconds); err != nil {
			return err
		}
	}
	return nil
}

func validateModelRequestRateLimitBucket(name string, total int, durationSeconds int64) error {
	// A zero total disables this bucket, and a zero duration remains valid while
	// the complete model request limiter configuration is disabled.
	if total == 0 || durationSeconds == 0 {
		return nil
	}
	rate := int64(total)
	if durationSeconds > limiter.MaxExactInteger/rate {
		return fmt.Errorf("%s rate limit count and duration exceed the limiter exact integer capacity of %d", name, limiter.MaxExactInteger)
	}
	config := limiter.Config{
		Capacity:  rate * durationSeconds,
		Rate:      rate,
		Requested: durationSeconds,
	}
	if err := limiter.ValidateConfig(config); err != nil {
		return fmt.Errorf("%s rate limit configuration is invalid: %w", name, err)
	}
	return nil
}

func validateModelRequestRateLimitGroup(group map[string][2]int) error {
	for name, limits := range group {
		if limits[0] < 0 || limits[1] < 1 {
			return fmt.Errorf("group %s has negative rate limit values: [%d, %d]", name, limits[0], limits[1])
		}
		if limits[0] > math.MaxInt32 || limits[1] > math.MaxInt32 {
			return fmt.Errorf("group %s [%d, %d] has max rate limits value 2147483647", name, limits[0], limits[1])
		}
	}
	return nil
}
