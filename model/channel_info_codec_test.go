package model

import (
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelInfoDatabaseValueUsesJSONTextAndRoundTrips(t *testing.T) {
	want := ChannelInfo{IsMultiKey: true, MultiKeySize: 2, MultiKeyStatusList: map[int]int{0: 1, 1: 2}}
	value, err := want.Value()
	require.NoError(t, err)
	encoded, ok := value.(string)
	require.True(t, ok, "PostgreSQL simple protocol must receive JSON text, not bytea")
	var decoded ChannelInfo
	require.NoError(t, common.UnmarshalJsonStr(encoded, &decoded))
	assert.Equal(t, want, decoded)
	for _, databaseValue := range []any{encoded, []byte(encoded)} {
		var scanned ChannelInfo
		require.NoError(t, scanned.Scan(databaseValue))
		assert.Equal(t, want, scanned)
	}
}
