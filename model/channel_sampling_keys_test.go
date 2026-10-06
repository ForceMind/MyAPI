package model

import (
	"github.com/ForceMind/MyAPI/common"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestQuotaSamplingKeysPreserveSingleFormattedCredential(t *testing.T) {
	key := " {\n  \"access_token\":\"synthetic\",\n  \"account_id\":\"account\"\n} "
	channel := Channel{Key: key}
	assert.Equal(t, []string{key}, channel.GetEnabledQuotaSamplingKeys())
}

func TestQuotaSamplingKeysExcludeDisabledBlankAndDuplicateEntries(t *testing.T) {
	channel := Channel{Key: " a \nb\na\n \nc", ChannelInfo: ChannelInfo{IsMultiKey: true, MultiKeyStatusList: map[int]int{1: common.ChannelStatusManuallyDisabled}}}
	assert.Equal(t, []string{" a ", "c"}, channel.GetEnabledQuotaSamplingKeys(), "preserve the raw credential for persistence CAS")
}
