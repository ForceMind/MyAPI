package operation_setting

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/ForceMind/MyAPI/setting/config"
)

type UserFundingMode string

const (
	UserFundingModeEnabled    UserFundingMode = "enabled"
	UserFundingModeRetirement UserFundingMode = "retirement"
	UserFundingModeDisabled   UserFundingMode = "disabled"

	UserFundingModeOptionKey  = "user_funding_setting.mode"
	UserFundingEpochOptionKey = "user_funding_setting.epoch"
)

type UserFundingSetting struct {
	Mode  UserFundingMode `json:"mode"`
	Epoch uint64          `json:"epoch"`
}

var defaultUserFundingSetting = UserFundingSetting{
	Mode:  UserFundingModeEnabled,
	Epoch: 0,
}

type userFundingSettingGeneration struct {
	setting UserFundingSetting
}

type managedUserFundingSetting struct {
	writeMutex sync.Mutex
	current    atomic.Pointer[userFundingSettingGeneration]
}

func NormalizeUserFundingMode(mode UserFundingMode) (UserFundingMode, error) {
	normalized := UserFundingMode(strings.ToLower(strings.TrimSpace(string(mode))))
	switch normalized {
	case UserFundingModeEnabled, UserFundingModeRetirement, UserFundingModeDisabled:
		return normalized, nil
	default:
		return "", fmt.Errorf("invalid user funding mode %q", mode)
	}
}

func newManagedUserFundingSetting(initial UserFundingSetting) *managedUserFundingSetting {
	mode, err := NormalizeUserFundingMode(initial.Mode)
	if err != nil {
		mode = UserFundingModeDisabled
	}
	state := &managedUserFundingSetting{}
	state.current.Store(&userFundingSettingGeneration{
		setting: UserFundingSetting{Mode: mode, Epoch: initial.Epoch},
	})
	return state
}

func (s *managedUserFundingSetting) snapshot() UserFundingSetting {
	if s != nil {
		if current := s.current.Load(); current != nil {
			return current.setting
		}
	}
	return defaultUserFundingSetting
}

func (s *managedUserFundingSetting) candidate(values map[string]string) (UserFundingSetting, error) {
	candidate := s.snapshot()
	if err := config.UpdateConfigFromMap(&candidate, values); err != nil {
		return UserFundingSetting{}, err
	}
	mode, err := NormalizeUserFundingMode(candidate.Mode)
	if err != nil {
		return UserFundingSetting{}, err
	}
	candidate.Mode = mode
	return candidate, nil
}

func (s *managedUserFundingSetting) ExportConfigMap() (map[string]string, error) {
	setting := s.snapshot()
	return map[string]string{
		"mode":  string(setting.Mode),
		"epoch": strconv.FormatUint(setting.Epoch, 10),
	}, nil
}

func (s *managedUserFundingSetting) ValidateConfigMap(values map[string]string) error {
	_, err := s.candidate(values)
	return err
}

func (s *managedUserFundingSetting) UpdateConfigMap(values map[string]string) error {
	s.writeMutex.Lock()
	defer s.writeMutex.Unlock()

	candidate, err := s.candidate(values)
	if err != nil {
		current := s.snapshot()
		current.Mode = UserFundingModeDisabled
		s.current.Store(&userFundingSettingGeneration{setting: current})
		return err
	}
	s.current.Store(&userFundingSettingGeneration{setting: candidate})
	return nil
}

var userFundingSettingState = newManagedUserFundingSetting(defaultUserFundingSetting)

var _ config.ValidatingMapConfig = (*managedUserFundingSetting)(nil)

func init() {
	config.GlobalConfig.Register("user_funding_setting", userFundingSettingState)
}

func GetUserFundingSetting() UserFundingSetting {
	return userFundingSettingState.snapshot()
}

func GetUserFundingMode() UserFundingMode {
	return userFundingSettingState.snapshot().Mode
}

func SetUserFundingMode(mode UserFundingMode) error {
	return userFundingSettingState.UpdateConfigMap(map[string]string{"mode": string(mode)})
}

func PublishUserFundingSnapshot(mode UserFundingMode, epoch uint64) error {
	return userFundingSettingState.UpdateConfigMap(map[string]string{
		"mode":  string(mode),
		"epoch": strconv.FormatUint(epoch, 10),
	})
}
