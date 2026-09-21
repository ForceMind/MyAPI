package model

import (
	"testing"

	"github.com/ForceMind/MyAPI/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelInfoDatabaseValueUsesTextAndScansSupportedDriverValues(t *testing.T) {
	expected := ChannelInfo{
		IsMultiKey:             true,
		MultiKeySize:           2,
		MultiKeyStatusList:     map[int]int{0: 1},
		MultiKeyDisabledReason: map[int]string{1: "provider rejected key"},
		MultiKeyDisabledTime:   map[int]int64{1: 1_700_000_000},
		MultiKeyPollingIndex:   1,
		MultiKeyMode:           constant.MultiKeyModePolling,
	}

	value, err := expected.Value()
	require.NoError(t, err)
	text, ok := value.(string)
	require.True(t, ok, "JSON columns must receive a text driver value")

	for _, value := range []interface{}{text, []byte(text)} {
		var restored ChannelInfo
		require.NoError(t, restored.Scan(value))
		assert.Equal(t, expected, restored)
	}

	stale := expected
	require.NoError(t, stale.Scan(nil))
	assert.Equal(t, ChannelInfo{}, stale)
	assert.Error(t, stale.Scan(1))
}
