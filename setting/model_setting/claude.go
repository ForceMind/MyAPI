package model_setting

import (
	"fmt"
	"math"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/setting/config"
)

// ClaudeMaxTokensLimit matches the request-side billing bound.  Defaults are
// injected after request validation, so they need the same upper limit.
const ClaudeMaxTokensLimit = math.MaxInt32 / 2

// ClaudeDefaultMaxTokens is supplied to request snapshots when the persisted
// configuration has no default entry. It is never written back implicitly.
const ClaudeDefaultMaxTokens = 8192

//var claudeHeadersSettings = map[string][]string{}
//
//var ClaudeThinkingAdapterEnabled = true
//var ClaudeThinkingAdapterMaxTokens = 8192
//var ClaudeThinkingAdapterBudgetTokensPercentage = 0.8

// ClaudeSettings 定义Claude模型的配置
type ClaudeSettings struct {
	HeadersSettings                       map[string]map[string][]string `json:"model_headers_settings"`
	DefaultMaxTokens                      map[string]int                 `json:"default_max_tokens"`
	ThinkingAdapterEnabled                bool                           `json:"thinking_adapter_enabled"`
	ThinkingAdapterBudgetTokensPercentage float64                        `json:"thinking_adapter_budget_tokens_percentage"`
}

// 默认配置
var defaultClaudeSettings = ClaudeSettings{
	HeadersSettings:        map[string]map[string][]string{},
	ThinkingAdapterEnabled: true,
	DefaultMaxTokens: map[string]int{
		"default": ClaudeDefaultMaxTokens,
	},
	ThinkingAdapterBudgetTokensPercentage: 0.8,
}

type managedClaudeSettings struct {
	current    atomic.Pointer[ClaudeSettings]
	writeMutex sync.Mutex
}

func newManagedClaudeSettings(initial ClaudeSettings) *managedClaudeSettings {
	managed := &managedClaudeSettings{}
	managed.current.Store(cloneClaudeSettings(&initial))
	return managed
}

var claudeSettingState = newManagedClaudeSettings(defaultClaudeSettings)

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("claude", claudeSettingState)
}

// GetClaudeSettings 获取Claude配置
func GetClaudeSettings() *ClaudeSettings {
	snapshot := claudeSettingState.snapshot()
	if snapshot.DefaultMaxTokens == nil {
		snapshot.DefaultMaxTokens = make(map[string]int, 1)
	}
	if _, ok := snapshot.DefaultMaxTokens["default"]; !ok {
		snapshot.DefaultMaxTokens["default"] = ClaudeDefaultMaxTokens
	}
	return snapshot
}

func (s *managedClaudeSettings) snapshot() *ClaudeSettings {
	return cloneClaudeSettings(s.current.Load())
}

func cloneClaudeSettings(source *ClaudeSettings) *ClaudeSettings {
	if source == nil {
		return &ClaudeSettings{}
	}
	clone := &ClaudeSettings{
		ThinkingAdapterEnabled:                source.ThinkingAdapterEnabled,
		ThinkingAdapterBudgetTokensPercentage: source.ThinkingAdapterBudgetTokensPercentage,
	}
	if source.DefaultMaxTokens != nil {
		clone.DefaultMaxTokens = make(map[string]int, len(source.DefaultMaxTokens))
		for model, maxTokens := range source.DefaultMaxTokens {
			clone.DefaultMaxTokens[model] = maxTokens
		}
	}
	if source.HeadersSettings != nil {
		clone.HeadersSettings = make(map[string]map[string][]string, len(source.HeadersSettings))
		for model, headers := range source.HeadersSettings {
			if headers == nil {
				clone.HeadersSettings[model] = nil
				continue
			}
			headersClone := make(map[string][]string, len(headers))
			for name, values := range headers {
				if values == nil {
					headersClone[name] = nil
					continue
				}
				valuesClone := make([]string, len(values))
				copy(valuesClone, values)
				headersClone[name] = valuesClone
			}
			clone.HeadersSettings[model] = headersClone
		}
	}
	return clone
}

func (s *managedClaudeSettings) ExportConfigMap() (map[string]string, error) {
	return config.ConfigToMap(s.snapshot())
}

func (s *managedClaudeSettings) ValidateConfigMap(values map[string]string) error {
	_, err := s.buildCandidate(values)
	return err
}

func (s *managedClaudeSettings) UpdateConfigMap(values map[string]string) error {
	s.writeMutex.Lock()
	defer s.writeMutex.Unlock()

	candidate, err := s.buildCandidate(values)
	if err != nil {
		return err
	}
	s.current.Store(candidate)
	return nil
}

func (s *managedClaudeSettings) buildCandidate(values map[string]string) (*ClaudeSettings, error) {
	candidate := s.snapshot()
	if err := config.UpdateConfigFromMap(candidate, values); err != nil {
		return nil, err
	}
	if candidate.DefaultMaxTokens != nil {
		if err := validateClaudeDefaultMaxTokensMap(candidate.DefaultMaxTokens); err != nil {
			return nil, err
		}
	}
	if err := ValidateClaudeThinkingAdapterBudgetTokensPercentage(candidate.ThinkingAdapterBudgetTokensPercentage); err != nil {
		return nil, err
	}
	return candidate, nil
}

func (c *ClaudeSettings) WriteHeaders(originModel string, httpHeader *http.Header) {
	if headers, ok := c.HeadersSettings[originModel]; ok {
		for headerKey, headerValues := range headers {
			mergedValues := normalizeHeaderListValues(
				append(append([]string(nil), httpHeader.Values(headerKey)...), headerValues...),
			)
			if len(mergedValues) == 0 {
				continue
			}
			httpHeader.Set(headerKey, strings.Join(mergedValues, ","))
		}
	}
}

func normalizeHeaderListValues(values []string) []string {
	normalizedValues := make([]string, 0, len(values))
	seenValues := make(map[string]struct{}, len(values))
	for _, value := range values {
		for _, item := range strings.Split(value, ",") {
			normalizedItem := strings.TrimSpace(item)
			if normalizedItem == "" {
				continue
			}
			if _, exists := seenValues[normalizedItem]; exists {
				continue
			}
			seenValues[normalizedItem] = struct{}{}
			normalizedValues = append(normalizedValues, normalizedItem)
		}
	}
	return normalizedValues
}

func (c *ClaudeSettings) GetDefaultMaxTokens(model string) int {
	if c == nil {
		return ClaudeDefaultMaxTokens
	}
	if maxTokens, ok := c.DefaultMaxTokens[model]; ok {
		return maxTokens
	}
	if maxTokens, ok := c.DefaultMaxTokens["default"]; ok {
		return maxTokens
	}
	return ClaudeDefaultMaxTokens
}

// ValidateClaudeDefaultMaxTokens validates the JSON persisted by the option
// API. Zero stays allowed — the current Messages API accepts max_tokens: 0 as
// cache pre-warming — but negative values are rejected because they would
// wrap into huge unsigned values during request conversion.
func ValidateClaudeDefaultMaxTokens(value string) error {
	var settings map[string]int
	if err := common.UnmarshalJsonStr(value, &settings); err != nil {
		return fmt.Errorf("Claude default max tokens must be a JSON map of model to integer: %w", err)
	}
	if settings == nil {
		return fmt.Errorf("Claude default max tokens must be a JSON map of model to integer")
	}
	return validateClaudeDefaultMaxTokensMap(settings)
}

func validateClaudeDefaultMaxTokensMap(settings map[string]int) error {
	for model, maxTokens := range settings {
		if maxTokens < 0 || maxTokens > ClaudeMaxTokensLimit {
			if maxTokens > ClaudeMaxTokensLimit {
				return fmt.Errorf("Claude default max_tokens %d for %q exceeds limit %d", maxTokens, model, ClaudeMaxTokensLimit)
			}
			return fmt.Errorf("negative Claude default max_tokens %d for %q", maxTokens, model)
		}
	}
	return nil
}

// ValidateClaudeThinkingAdapterBudgetTokensPercentage validates the fraction
// used to derive thinking budget_tokens.  Restricting it to (0,1] keeps the
// derived budget finite, non-negative, and no larger than max_tokens.
func ValidateClaudeThinkingAdapterBudgetTokensPercentage(value float64) error {
	if math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 || value > 1 {
		return fmt.Errorf("Claude thinking adapter budget percentage must be finite and in (0,1], got %v", value)
	}
	return nil
}
