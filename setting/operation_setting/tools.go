package operation_setting

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/setting/config"
)

// ---------------------------------------------------------------------------
// Tool call prices ($/1K calls, admin-configurable)
// DB key: tool_price_setting.prices
//
// Key format:
//   - "tool_name"              → default price for all models
//   - "tool_name:model_prefix*" → override for models matching the prefix
//
// Effective index: hardcoded defaults → hardcoded model overrides → valid
// operator values. Lookup uses the longest model prefix before the tool
// default, and a matched numeric zero is terminal.
// ---------------------------------------------------------------------------

const ToolPriceOptionKey = "tool_price_setting.prices"

const (
	defaultWebSearchToolPrice        = 10.0
	defaultWebSearchPreviewToolPrice = 10.0
	defaultFileSearchToolPrice       = 2.5
	defaultGoogleSearchToolPrice     = 14.0
	defaultImageGenerationToolPrice  = 150.0
	defaultSearchPreviewModelPrice   = 25.0
)

// seedHardcodedToolPrices injects compile-time built-in fallbacks (tool
// defaults and model-prefix overrides) into the destination. The source is
// constants, not a mutable package map or operator configuration.
func seedHardcodedToolPrices(prices map[string]float64) {
	prices["web_search"] = defaultWebSearchToolPrice
	prices["web_search_preview"] = defaultWebSearchPreviewToolPrice
	prices["file_search"] = defaultFileSearchToolPrice
	prices["google_search"] = defaultGoogleSearchToolPrice
	prices["image_generation"] = defaultImageGenerationToolPrice
	prices["web_search_preview:gpt-4o*"] = defaultSearchPreviewModelPrice
	prices["web_search_preview:gpt-4.1*"] = defaultSearchPreviewModelPrice
	prices["web_search_preview:gpt-4o-mini*"] = defaultSearchPreviewModelPrice
	prices["web_search_preview:gpt-4.1-mini*"] = defaultSearchPreviewModelPrice
}

// ToolPriceSetting is the public serialization DTO retained for callers that
// construct or decode tool-price configuration values.
type ToolPriceSetting struct {
	Prices map[string]float64 `json:"prices"`
}

// managedToolPriceSetting publishes immutable operator overrides and their
// derived lookup index as one generation. It is intentionally private so the
// live source map cannot be mutated outside its publication methods.
type managedToolPriceSetting struct {
	writeMutex sync.Mutex
	current    atomic.Pointer[toolPriceState]
}

type toolPriceState struct {
	overrides map[string]float64
	index     toolPriceIndex
}

var toolPriceSetting = newManagedToolPriceSetting()

func init() {
	config.GlobalConfig.Register("tool_price_setting", toolPriceSetting)
}

// ---------------------------------------------------------------------------
// Precomputed price index (atomic, lock-free on read path)
// ---------------------------------------------------------------------------

type prefixEntry struct {
	prefix string
	price  float64
}

type toolPriceIndex struct {
	defaults map[string]float64
	prefixes map[string][]prefixEntry
}

func isValidToolPrice(price float64) bool {
	return price >= 0 && !math.IsNaN(price) && !math.IsInf(price, 0)
}

func decodeToolPricesJSON(value string, ignoreInvalidEntries bool) (map[string]float64, error) {
	rawValue := json.RawMessage(strings.TrimSpace(value))
	if common.GetJsonType(rawValue) != "object" {
		return nil, fmt.Errorf("工具价格必须是 JSON 对象")
	}

	var rawPrices map[string]json.RawMessage
	if err := common.Unmarshal(rawValue, &rawPrices); err != nil {
		return nil, fmt.Errorf("解析工具价格失败: %w", err)
	}

	prices := make(map[string]float64, len(rawPrices))
	for name, rawPrice := range rawPrices {
		var entryErr error
		if common.GetJsonType(rawPrice) != "number" {
			entryErr = fmt.Errorf("工具价格 %q 必须是非负数字", name)
		} else {
			var price float64
			if err := common.Unmarshal(rawPrice, &price); err != nil {
				entryErr = fmt.Errorf("解析工具价格 %q 失败: %w", name, err)
			} else if !isValidToolPrice(price) {
				entryErr = fmt.Errorf("工具价格 %q 必须是有限的非负数字", name)
			} else {
				prices[name] = price
			}
		}

		if entryErr == nil {
			continue
		}
		if !ignoreInvalidEntries {
			return nil, entryErr
		}
		common.SysError(entryErr.Error())
	}
	return prices, nil
}

// ValidateToolPricesJSON validates an operator-supplied complete price map.
// A numeric zero is valid and intentionally disables the matching rule.
func ValidateToolPricesJSON(value string) error {
	_, err := decodeToolPricesJSON(value, false)
	return err
}

// LoadToolPricesFromJSONString replaces the complete operator price map.
// Invalid legacy entries are ignored individually so valid sibling overrides
// survive, while missing built-in keys continue to use hardcoded fallbacks.
func LoadToolPricesFromJSONString(value string) {
	toolPriceSetting.loadJSONString(value)
}

func (s *managedToolPriceSetting) loadJSONString(value string) {
	prices, err := decodeToolPricesJSON(value, true)
	if err != nil {
		common.SysError("加载工具价格失败，将使用硬编码兜底: " + err.Error())
		prices = make(map[string]float64)
	}
	s.replaceOverrides(prices)
}

func newManagedToolPriceSetting() *managedToolPriceSetting {
	setting := &managedToolPriceSetting{}
	setting.current.Store(buildToolPriceState(nil))
	return setting
}

func buildToolPriceState(overrides map[string]float64) *toolPriceState {
	validOverrides := make(map[string]float64, len(overrides))
	for key, price := range overrides {
		if isValidToolPrice(price) {
			validOverrides[key] = price
		}
	}

	merged := make(map[string]float64, 9+len(validOverrides))
	seedHardcodedToolPrices(merged)
	for k, v := range validOverrides {
		merged[k] = v
	}

	state := &toolPriceState{
		overrides: validOverrides,
		index: toolPriceIndex{
			defaults: make(map[string]float64),
			prefixes: make(map[string][]prefixEntry),
		},
	}

	for key, price := range merged {
		colonIdx := strings.IndexByte(key, ':')
		if colonIdx < 0 {
			state.index.defaults[key] = price
			continue
		}
		toolName := key[:colonIdx]
		modelPart := key[colonIdx+1:]
		prefix := strings.TrimSuffix(modelPart, "*")
		state.index.prefixes[toolName] = append(state.index.prefixes[toolName], prefixEntry{prefix: prefix, price: price})
	}

	for tool := range state.index.prefixes {
		entries := state.index.prefixes[tool]
		sort.Slice(entries, func(i, j int) bool {
			if len(entries[i].prefix) == len(entries[j].prefix) {
				return entries[i].prefix < entries[j].prefix
			}
			return len(entries[i].prefix) > len(entries[j].prefix)
		})
		state.index.prefixes[tool] = entries
	}
	return state
}

func (s *managedToolPriceSetting) replaceOverrides(overrides map[string]float64) {
	candidate := buildToolPriceState(overrides)
	s.writeMutex.Lock()
	s.current.Store(candidate)
	s.writeMutex.Unlock()
}

func (s *managedToolPriceSetting) mutateOverrides(mutate func(map[string]float64)) {
	s.writeMutex.Lock()
	current := s.current.Load()
	overrides := make(map[string]float64)
	if current != nil {
		overrides = make(map[string]float64, len(current.overrides)+1)
		for key, price := range current.overrides {
			overrides[key] = price
		}
	}
	mutate(overrides)
	s.current.Store(buildToolPriceState(overrides))
	s.writeMutex.Unlock()
}

func (s *managedToolPriceSetting) ExportConfigMap() (map[string]string, error) {
	state := s.current.Load()
	overrides := map[string]float64{}
	if state != nil {
		overrides = state.overrides
	}
	raw, err := common.Marshal(overrides)
	if err != nil {
		return nil, err
	}
	return map[string]string{"prices": string(raw)}, nil
}

func (s *managedToolPriceSetting) UpdateConfigMap(values map[string]string) error {
	raw, ok := values["prices"]
	if !ok {
		return nil
	}
	prices, err := decodeToolPricesJSON(raw, false)
	if err != nil {
		return err
	}
	s.replaceOverrides(prices)
	return nil
}

// RebuildToolPriceIndex republishes the current immutable overrides together
// with a freshly derived index. It is not used on the billing hot path.
func RebuildToolPriceIndex() {
	toolPriceSetting.mutateOverrides(func(map[string]float64) {})
}

// GetToolPriceForModel returns the price ($/1K calls) for a tool given a model name.
// Lookup: longest prefix match → tool default → 0.
func GetToolPriceForModel(toolName, modelName string) float64 {
	return toolPriceSetting.getPriceForModel(toolName, modelName)
}

func (s *managedToolPriceSetting) getPriceForModel(toolName, modelName string) float64 {
	state := s.current.Load()
	if state == nil {
		return 0
	}
	idx := &state.index

	if entries, ok := idx.prefixes[toolName]; ok && modelName != "" {
		for _, e := range entries {
			if strings.HasPrefix(modelName, e.prefix) {
				return e.price
			}
		}
	}

	if p, ok := idx.defaults[toolName]; ok {
		return p
	}
	return 0
}

// GetToolPrice is a convenience wrapper when no model name is needed.
func GetToolPrice(toolName string) float64 {
	return GetToolPriceForModel(toolName, "")
}

// SetToolPriceForTest injects a tool price and rebuilds the lookup index. Tests only.
func SetToolPriceForTest(name string, price float64) {
	toolPriceSetting.mutateOverrides(func(overrides map[string]float64) {
		overrides[name] = price
	})
}

// DeleteToolPriceForTest removes an injected tool price and rebuilds the index. Tests only.
func DeleteToolPriceForTest(name string) {
	toolPriceSetting.mutateOverrides(func(overrides map[string]float64) {
		delete(overrides, name)
	})
}

// ---------------------------------------------------------------------------
// Gemini audio input pricing (per-million tokens, model-specific)
// ---------------------------------------------------------------------------

const (
	Gemini25FlashPreviewInputAudioPrice     = 1.00
	Gemini25FlashProductionInputAudioPrice  = 1.00
	Gemini25FlashLitePreviewInputAudioPrice = 0.50
	Gemini25FlashNativeAudioInputAudioPrice = 3.00
	Gemini20FlashInputAudioPrice            = 0.70
	GeminiRoboticsER15InputAudioPrice       = 1.00
)

func GetGeminiInputAudioPricePerMillionTokens(modelName string) float64 {
	if strings.HasPrefix(modelName, "gemini-2.5-flash-preview-native-audio") {
		return Gemini25FlashNativeAudioInputAudioPrice
	}
	if strings.HasPrefix(modelName, "gemini-2.5-flash-preview-lite") {
		return Gemini25FlashLitePreviewInputAudioPrice
	}
	if strings.HasPrefix(modelName, "gemini-2.5-flash-preview") {
		return Gemini25FlashPreviewInputAudioPrice
	}
	if strings.HasPrefix(modelName, "gemini-2.5-flash") {
		return Gemini25FlashProductionInputAudioPrice
	}
	if strings.HasPrefix(modelName, "gemini-2.0-flash") {
		return Gemini20FlashInputAudioPrice
	}
	if strings.HasPrefix(modelName, "gemini-robotics-er-1.5") {
		return GeminiRoboticsER15InputAudioPrice
	}
	return 0
}
