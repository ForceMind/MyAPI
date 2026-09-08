package operation_setting

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/setting/config"
)

const (
	ChannelRoutingPolicyOptionKey = "routing_policy_setting.policy"

	MinChannelRoutingSessionTTLSeconds  = 3_600
	MaxChannelRoutingSessionTTLSeconds  = 2_592_000
	MinChannelRoutingQuotaMaxAgeSeconds = 60
	MaxChannelRoutingQuotaMaxAgeSeconds = 86_400
)

type ChannelRoutingPolicy struct {
	Enabled            bool `json:"enabled"`
	StickyEnabled      bool `json:"sticky_enabled"`
	SessionTTLSeconds  int  `json:"session_ttl_seconds"`
	QuotaMaxAgeSeconds int  `json:"quota_max_age_seconds"`
}

var defaultChannelRoutingPolicy = ChannelRoutingPolicy{
	Enabled:            false,
	StickyEnabled:      true,
	SessionTTLSeconds:  86_400,
	QuotaMaxAgeSeconds: 300,
}

type channelRoutingPolicyGeneration struct {
	policy ChannelRoutingPolicy
}

type managedChannelRoutingPolicy struct {
	writeMutex sync.Mutex
	current    atomic.Pointer[channelRoutingPolicyGeneration]
}

func newManagedChannelRoutingPolicy(initial ChannelRoutingPolicy) *managedChannelRoutingPolicy {
	state := &managedChannelRoutingPolicy{}
	state.current.Store(&channelRoutingPolicyGeneration{policy: initial})
	return state
}

func (s *managedChannelRoutingPolicy) snapshot() ChannelRoutingPolicy {
	if s != nil {
		if current := s.current.Load(); current != nil {
			return current.policy
		}
	}
	return defaultChannelRoutingPolicy
}

func validateChannelRoutingPolicy(policy ChannelRoutingPolicy) error {
	if policy.SessionTTLSeconds < MinChannelRoutingSessionTTLSeconds || policy.SessionTTLSeconds > MaxChannelRoutingSessionTTLSeconds {
		return fmt.Errorf("session_ttl_seconds must be between %d and %d", MinChannelRoutingSessionTTLSeconds, MaxChannelRoutingSessionTTLSeconds)
	}
	if policy.QuotaMaxAgeSeconds < MinChannelRoutingQuotaMaxAgeSeconds || policy.QuotaMaxAgeSeconds > MaxChannelRoutingQuotaMaxAgeSeconds {
		return fmt.Errorf("quota_max_age_seconds must be between %d and %d", MinChannelRoutingQuotaMaxAgeSeconds, MaxChannelRoutingQuotaMaxAgeSeconds)
	}
	return nil
}

func parseChannelRoutingPolicy(values map[string]string) (ChannelRoutingPolicy, error) {
	raw, ok := values["policy"]
	if !ok {
		return ChannelRoutingPolicy{}, fmt.Errorf("policy is required")
	}
	var policy ChannelRoutingPolicy
	if err := common.Unmarshal([]byte(raw), &policy); err != nil {
		return ChannelRoutingPolicy{}, fmt.Errorf("invalid routing policy: %w", err)
	}
	if err := validateChannelRoutingPolicy(policy); err != nil {
		return ChannelRoutingPolicy{}, err
	}
	return policy, nil
}

func (s *managedChannelRoutingPolicy) ExportConfigMap() (map[string]string, error) {
	encoded, err := common.Marshal(s.snapshot())
	if err != nil {
		return nil, err
	}
	return map[string]string{"policy": string(encoded)}, nil
}

func (s *managedChannelRoutingPolicy) ValidateConfigMap(values map[string]string) error {
	_, err := parseChannelRoutingPolicy(values)
	return err
}

func (s *managedChannelRoutingPolicy) UpdateConfigMap(values map[string]string) error {
	s.writeMutex.Lock()
	defer s.writeMutex.Unlock()
	policy, err := parseChannelRoutingPolicy(values)
	if err != nil {
		return err
	}
	s.current.Store(&channelRoutingPolicyGeneration{policy: policy})
	return nil
}

var channelRoutingPolicyState = newManagedChannelRoutingPolicy(defaultChannelRoutingPolicy)

var _ config.ValidatingMapConfig = (*managedChannelRoutingPolicy)(nil)

func init() {
	config.GlobalConfig.Register("routing_policy_setting", channelRoutingPolicyState)
}

func GetChannelRoutingPolicy() ChannelRoutingPolicy {
	return channelRoutingPolicyState.snapshot()
}

func MarshalChannelRoutingPolicy(policy ChannelRoutingPolicy) (string, error) {
	if err := validateChannelRoutingPolicy(policy); err != nil {
		return "", err
	}
	encoded, err := common.Marshal(policy)
	return string(encoded), err
}
