package tencent

import (
	"strings"

	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/relay/channel"
	"github.com/ForceMind/MyAPI/relay/channel/claude"
	"github.com/ForceMind/MyAPI/relay/channel/openai"
	"github.com/ForceMind/MyAPI/relay/channel/tokenhub"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
)

const tokenHubBaseURL = "https://tokenhub.tencentmaas.com"

// DispatchAdaptor 按密钥格式分流:三段式 ak/sk 走原生 TC3,单段 TokenHub key 走 OpenAI 兼容。
type DispatchAdaptor struct {
	channel.Adaptor
}

func (a *DispatchAdaptor) Init(info *relaycommon.RelayInfo) {
	if info == nil {
		a.Adaptor = &openai.Adaptor{}
		return
	}
	// An explicit TokenHub protocol is preferred over the legacy key-shape
	// heuristic. Existing Tencent channels remain unchanged when settings are
	// absent or invalid (save-time validation reports invalid settings).
	if config, err := tokenhub.FromSettings(info.ChannelOtherSettings.TokenHub); err == nil {
		if config.BaseURL != "" {
			info.ChannelBaseUrl = config.BaseURL
		}
		switch config.Protocol {
		case tokenhub.ProtocolAnthropic:
			a.Adaptor = &claude.Adaptor{}
		default:
			a.Adaptor = &openai.Adaptor{}
		}
		a.Adaptor.Init(info)
		return
	}
	if strings.Contains(info.ApiKey, "|") {
		a.Adaptor = &Adaptor{}
	} else {
		a.Adaptor = &openai.Adaptor{}
		if info.ChannelBaseUrl == "" || info.ChannelBaseUrl == constant.ChannelBaseURLs[constant.ChannelTypeTencent] {
			info.ChannelBaseUrl = tokenHubBaseURL
		}
	}
	a.Adaptor.Init(info)
}

func (a *DispatchAdaptor) GetModelList() []string {
	return ModelList
}

func (a *DispatchAdaptor) GetChannelName() string {
	return ChannelName
}
