package operation_setting

import (
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/ForceMind/MyAPI/setting/config"
)

// TokenSetting 令牌相关配置
type TokenSetting struct {
	MaxUserTokens int `json:"max_user_tokens"` // 每用户最大令牌数量
}

// 默认配置
var defaultTokenSetting = TokenSetting{
	MaxUserTokens: 1000, // 默认每用户最多 1000 个令牌
}

// tokenSettingGeneration is immutable after publication. Token creation and
// token searching each read one published configuration generation.
type tokenSettingGeneration struct {
	setting TokenSetting
}

// managedTokenSetting owns the runtime snapshot registered with the generic
// config manager. Its public pointer getter returns a detached copy so callers
// cannot mutate the published generation.
type managedTokenSetting struct {
	writeMutex sync.Mutex
	current    atomic.Pointer[tokenSettingGeneration]
}

func newManagedTokenSetting(initial TokenSetting) *managedTokenSetting {
	setting := &managedTokenSetting{}
	setting.current.Store(&tokenSettingGeneration{setting: initial})
	return setting
}

func (s *managedTokenSetting) snapshot() TokenSetting {
	if s != nil {
		if current := s.current.Load(); current != nil {
			return current.setting
		}
	}
	return defaultTokenSetting
}

func (s *managedTokenSetting) candidate(values map[string]string) (TokenSetting, error) {
	candidate := s.snapshot()
	if err := config.UpdateConfigFromMap(&candidate, values); err != nil {
		return TokenSetting{}, err
	}
	return candidate, nil
}

func (s *managedTokenSetting) ExportConfigMap() (map[string]string, error) {
	setting := s.snapshot()
	return map[string]string{
		"max_user_tokens": strconv.Itoa(setting.MaxUserTokens),
	}, nil
}

func (s *managedTokenSetting) ValidateConfigMap(values map[string]string) error {
	_, err := s.candidate(values)
	return err
}

func (s *managedTokenSetting) UpdateConfigMap(values map[string]string) error {
	s.writeMutex.Lock()
	defer s.writeMutex.Unlock()

	candidate, err := s.candidate(values)
	if err != nil {
		return err
	}
	s.current.Store(&tokenSettingGeneration{setting: candidate})
	return nil
}

var tokenSettingState = newManagedTokenSetting(defaultTokenSetting)

var _ config.ValidatingMapConfig = (*managedTokenSetting)(nil)

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("token_setting", tokenSettingState)
}

// GetTokenSetting 获取令牌配置
func GetTokenSetting() *TokenSetting {
	setting := tokenSettingState.snapshot()
	return &setting
}

// GetMaxUserTokens 获取每用户最大令牌数量
func GetMaxUserTokens() int {
	setting := tokenSettingState.snapshot()
	return setting.MaxUserTokens
}
