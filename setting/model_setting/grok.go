package model_setting

import (
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/ForceMind/MyAPI/setting/config"
)

// GrokSettings defines Grok model configuration.
type GrokSettings struct {
	ViolationDeductionEnabled bool    `json:"violation_deduction_enabled"`
	ViolationDeductionAmount  float64 `json:"violation_deduction_amount"`
}

var defaultGrokSettings = GrokSettings{
	ViolationDeductionEnabled: true,
	ViolationDeductionAmount:  0.05,
}

// grokSettingsGeneration is immutable after publication. A violation-fee
// decision must read its enablement and amount from the same configuration
// generation.
type grokSettingsGeneration struct {
	settings GrokSettings
}

// managedGrokSettings owns the synchronized runtime snapshot registered with
// the generic config manager.
type managedGrokSettings struct {
	writeMutex sync.Mutex
	current    atomic.Pointer[grokSettingsGeneration]
}

func newManagedGrokSettings(initial GrokSettings) *managedGrokSettings {
	state := &managedGrokSettings{}
	state.current.Store(&grokSettingsGeneration{settings: initial})
	return state
}

func (s *managedGrokSettings) snapshot() GrokSettings {
	if s != nil {
		if current := s.current.Load(); current != nil {
			return current.settings
		}
	}
	return defaultGrokSettings
}

func (s *managedGrokSettings) candidate(values map[string]string) (GrokSettings, error) {
	candidate := s.snapshot()
	if err := config.UpdateConfigFromMap(&candidate, values); err != nil {
		return GrokSettings{}, err
	}
	return candidate, nil
}

func (s *managedGrokSettings) ExportConfigMap() (map[string]string, error) {
	settings := s.snapshot()
	return map[string]string{
		"violation_deduction_enabled": strconv.FormatBool(settings.ViolationDeductionEnabled),
		"violation_deduction_amount":  strconv.FormatFloat(settings.ViolationDeductionAmount, 'f', -1, 64),
	}, nil
}

func (s *managedGrokSettings) ValidateConfigMap(values map[string]string) error {
	_, err := s.candidate(values)
	return err
}

func (s *managedGrokSettings) UpdateConfigMap(values map[string]string) error {
	s.writeMutex.Lock()
	defer s.writeMutex.Unlock()

	candidate, err := s.candidate(values)
	if err != nil {
		return err
	}
	s.current.Store(&grokSettingsGeneration{settings: candidate})
	return nil
}

var grokSettingsState = newManagedGrokSettings(defaultGrokSettings)

var _ config.ValidatingMapConfig = (*managedGrokSettings)(nil)

func init() {
	config.GlobalConfig.Register("grok", grokSettingsState)
}

func GetGrokSettings() *GrokSettings {
	settings := grokSettingsState.snapshot()
	return &settings
}
