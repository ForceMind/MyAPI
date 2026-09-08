package model

import (
	"testing"

	"github.com/ForceMind/MyAPI/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelInfoDatabaseValueUsesJSONTextAndScansDriverForms(t *testing.T) {
	original := ChannelInfo{IsMultiKey: true, MultiKeySize: 2, MultiKeyMode: constant.MultiKeyModePolling}
	value, err := original.Value()
	require.NoError(t, err)
	encoded, ok := value.(string)
	require.True(t, ok)

	for _, databaseValue := range []interface{}{encoded, []byte(encoded)} {
		var decoded ChannelInfo
		require.NoError(t, decoded.Scan(databaseValue))
		assert.Equal(t, original.IsMultiKey, decoded.IsMultiKey)
		assert.Equal(t, original.MultiKeySize, decoded.MultiKeySize)
		assert.Equal(t, original.MultiKeyMode, decoded.MultiKeyMode)
	}
}
