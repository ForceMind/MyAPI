package operation_setting

import (
	"sync"
	"sync/atomic"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/setting/config"
)

type ChannelAffinityKeySource struct {
	Type string `json:"type"` // context_int, context_string, request_header, gjson
	Key  string `json:"key,omitempty"`
	Path string `json:"path,omitempty"`
}

type ChannelAffinityRule struct {
	Name             string                     `json:"name"`
	ModelRegex       []string                   `json:"model_regex"`
	PathRegex        []string                   `json:"path_regex"`
	UserAgentInclude []string                   `json:"user_agent_include,omitempty"`
	KeySources       []ChannelAffinityKeySource `json:"key_sources"`

	ValueRegex string `json:"value_regex"`
	TTLSeconds int    `json:"ttl_seconds"`

	ParamOverrideTemplate map[string]interface{} `json:"param_override_template,omitempty"`

	SkipRetryOnFailure bool `json:"skip_retry_on_failure"`

	IncludeUsingGroup bool `json:"include_using_group"`
	IncludeModelName  bool `json:"include_model_name"`
	IncludeRuleName   bool `json:"include_rule_name"`
}

type ChannelAffinitySetting struct {
	Enabled               bool                  `json:"enabled"`
	SwitchOnSuccess       bool                  `json:"switch_on_success"`
	KeepOnChannelDisabled bool                  `json:"keep_on_channel_disabled"`
	MaxEntries            int                   `json:"max_entries"`
	DefaultTTLSeconds     int                   `json:"default_ttl_seconds"`
	Rules                 []ChannelAffinityRule `json:"rules"`
}

// Keep Codex CLI passthrough aligned with upstream. Codex uses lower-case
// header names, while HTTP matching here is case-insensitive.
// Request session/thread headers:
// https://github.com/openai/codex/commit/7c7b4861d88960f7e3bd5b7f30f8351be666dd84
// Responses metadata headers/client_metadata:
// https://github.com/openai/codex/commit/14df0e8833aad0d6d78287954b61ffac67af936c
// x-codex-turn-state response/request round trip:
// https://github.com/openai/codex/commit/ebdd8795e924a8149b616e46ca2ed7848c207a4b
var codexCliPassThroughHeaders = []string{
	"Originator",
	"Session_id",
	"Thread_id",
	"Session-Id",
	"Thread-Id",
	"X-Client-Request-Id",
	"User-Agent",
	"X-Codex-Beta-Features",
	"X-Codex-Turn-State",
	"X-Codex-Turn-Metadata",
	"X-Codex-Window-Id",
	"X-Codex-Parent-Thread-Id",
	//"X-Codex-Installation-Id",
	"X-OpenAI-Subagent",
	"X-OpenAI-Memgen-Request",
	//"X-OAI-Attestation",
	"X-ResponsesAPI-Include-Timing-Metrics",
	"X-OpenAI-Internal-Codex-Responses-Lite",
}

var claudeCliPassThroughHeaders = []string{
	"X-Stainless-Arch",
	"X-Stainless-Lang",
	"X-Stainless-Os",
	"X-Stainless-Package-Version",
	"X-Stainless-Retry-Count",
	"X-Stainless-Runtime",
	"X-Stainless-Runtime-Version",
	"X-Stainless-Timeout",
	"User-Agent",
	"X-App",
	"Anthropic-Beta",
	"Anthropic-Dangerous-Direct-Browser-Access",
	"Anthropic-Version",
}

func buildPassHeaderTemplate(headers []string) map[string]interface{} {
	clonedHeaders := make([]string, 0, len(headers))
	clonedHeaders = append(clonedHeaders, headers...)
	return map[string]interface{}{
		"operations": []map[string]interface{}{
			{
				"mode":        "pass_headers",
				"value":       clonedHeaders,
				"keep_origin": true,
			},
		},
	}
}

func buildCodexPassHeaderTemplate() map[string]interface{} {
	requestHeaders := make([]string, 0, len(codexCliPassThroughHeaders))
	requestHeaders = append(requestHeaders, codexCliPassThroughHeaders...)
	return map[string]interface{}{
		"operations": []map[string]interface{}{
			{
				"mode":        "pass_headers",
				"value":       requestHeaders,
				"keep_origin": true,
			},
		},
	}
}

var channelAffinitySetting = ChannelAffinitySetting{
	Enabled:               true,
	SwitchOnSuccess:       true,
	KeepOnChannelDisabled: false,
	MaxEntries:            100_000,
	DefaultTTLSeconds:     3600,
	Rules: []ChannelAffinityRule{
		{
			Name:       "codex cli trace",
			ModelRegex: []string{"^gpt-.*$"},
			PathRegex:  []string{"/v1/responses"},
			KeySources: []ChannelAffinityKeySource{
				{Type: "gjson", Path: "prompt_cache_key"},
			},
			ValueRegex:            "",
			TTLSeconds:            0,
			ParamOverrideTemplate: buildCodexPassHeaderTemplate(),
			SkipRetryOnFailure:    true,
			IncludeUsingGroup:     true,
			IncludeRuleName:       true,
			UserAgentInclude:      nil,
		},
		{
			Name:       "claude cli trace",
			ModelRegex: []string{"^claude-.*$"},
			PathRegex:  []string{"/v1/messages"},
			KeySources: []ChannelAffinityKeySource{
				{Type: "gjson", Path: "metadata.user_id"},
			},
			ValueRegex:            "",
			TTLSeconds:            0,
			ParamOverrideTemplate: buildPassHeaderTemplate(claudeCliPassThroughHeaders),
			SkipRetryOnFailure:    true,
			IncludeUsingGroup:     true,
			IncludeRuleName:       true,
			UserAgentInclude:      nil,
		},
	},
}

// channelAffinitySettingGeneration 发布后不可变。
// 规则匹配一次读取整代（Enabled/Rules/DefaultTTLSeconds），不得在代切换期间混代。
type channelAffinitySettingGeneration struct {
	setting ChannelAffinitySetting
}

// managedChannelAffinitySetting 持有注册到通用配置管理器的同步运行时快照。
type managedChannelAffinitySetting struct {
	writeMutex sync.Mutex
	current    atomic.Pointer[channelAffinitySettingGeneration]
}

func cloneChannelAffinitySetting(setting ChannelAffinitySetting) ChannelAffinitySetting {
	clone := ChannelAffinitySetting{
		Enabled:               setting.Enabled,
		SwitchOnSuccess:       setting.SwitchOnSuccess,
		KeepOnChannelDisabled: setting.KeepOnChannelDisabled,
		MaxEntries:            setting.MaxEntries,
		DefaultTTLSeconds:     setting.DefaultTTLSeconds,
	}
	if setting.Rules != nil {
		clone.Rules = make([]ChannelAffinityRule, len(setting.Rules))
		for i, rule := range setting.Rules {
			clone.Rules[i] = cloneChannelAffinityRule(rule)
		}
	}
	return clone
}

func cloneChannelAffinityRule(rule ChannelAffinityRule) ChannelAffinityRule {
	clone := rule
	if rule.ModelRegex != nil {
		clone.ModelRegex = append([]string{}, rule.ModelRegex...)
	}
	if rule.PathRegex != nil {
		clone.PathRegex = append([]string{}, rule.PathRegex...)
	}
	if rule.UserAgentInclude != nil {
		clone.UserAgentInclude = append([]string{}, rule.UserAgentInclude...)
	}
	if rule.KeySources != nil {
		clone.KeySources = append([]ChannelAffinityKeySource{}, rule.KeySources...)
	}
	if rule.ParamOverrideTemplate != nil {
		// ParamOverrideTemplate 是 JSON 型数据（规则经 JSON 配置进出），
		// 用 JSON 往返做深拷贝，保证发布后代不可变。
		if encoded, err := common.Marshal(rule.ParamOverrideTemplate); err == nil {
			var fresh map[string]interface{}
			if err := common.Unmarshal(encoded, &fresh); err == nil {
				clone.ParamOverrideTemplate = fresh
			}
		}
	}
	return clone
}

func newManagedChannelAffinitySetting(initial ChannelAffinitySetting) *managedChannelAffinitySetting {
	state := &managedChannelAffinitySetting{}
	state.current.Store(&channelAffinitySettingGeneration{setting: cloneChannelAffinitySetting(initial)})
	return state
}

func (s *managedChannelAffinitySetting) snapshot() ChannelAffinitySetting {
	if s != nil {
		if current := s.current.Load(); current != nil {
			return cloneChannelAffinitySetting(current.setting)
		}
	}
	return cloneChannelAffinitySetting(channelAffinitySetting)
}

func (s *managedChannelAffinitySetting) candidate(values map[string]string) (ChannelAffinitySetting, error) {
	candidate := s.snapshot()
	if err := config.UpdateConfigFromMap(&candidate, values); err != nil {
		return ChannelAffinitySetting{}, err
	}
	return candidate, nil
}

func (s *managedChannelAffinitySetting) ExportConfigMap() (map[string]string, error) {
	setting := s.snapshot()
	return config.ConfigToMap(&setting)
}

func (s *managedChannelAffinitySetting) ValidateConfigMap(values map[string]string) error {
	_, err := s.candidate(values)
	return err
}

func (s *managedChannelAffinitySetting) UpdateConfigMap(values map[string]string) error {
	s.writeMutex.Lock()
	defer s.writeMutex.Unlock()

	candidate, err := s.candidate(values)
	if err != nil {
		return err
	}
	s.current.Store(&channelAffinitySettingGeneration{setting: cloneChannelAffinitySetting(candidate)})
	return nil
}

var channelAffinitySettingState = newManagedChannelAffinitySetting(channelAffinitySetting)

var _ config.ValidatingMapConfig = (*managedChannelAffinitySetting)(nil)

func init() {
	config.GlobalConfig.Register("channel_affinity_setting", channelAffinitySettingState)
}

// GetChannelAffinitySetting 获取渠道亲和设置的分离快照。
// 返回值是当代配置的深拷贝；调用方修改它不会影响运行时状态。
//
// 生效语义（D14/D15 合同）：Enabled/Rules 等匹配字段保存后即对新请求生效；
// MaxEntries/DefaultTTLSeconds 只更新配置代——运行中的亲和缓存容量/TTL 在
// 显式维护重建（POST /api/option/channel_affinity_cache/rebuild）或重启后
// 才按新代参数重建。
func GetChannelAffinitySetting() *ChannelAffinitySetting {
	setting := channelAffinitySettingState.snapshot()
	return &setting
}
