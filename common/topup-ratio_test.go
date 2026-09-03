package common

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUpdateTopupGroupRatioKeepsPreviousValueOnInvalidInput(t *testing.T) {
	previous := TopupGroupRatio2JSONString()
	t.Cleanup(func() { require.NoError(t, UpdateTopupGroupRatioByJSONString(previous)) })
	require.NoError(t, UpdateTopupGroupRatioByJSONString(`{"default":1,"vip":2}`))
	require.NoError(t, UpdateTopupGroupRatioByJSONString(`{"default":1,"vip":2,"free":0}`))
	require.Equal(t, 0.0, GetTopupGroupRatio("free"))
	for _, raw := range []string{
		`{"default":-1}`,
		`{"default":1e309}`,
		`{"default":NaN}`,
		`{`,
		`{}`,
	} {
		t.Run(raw, func(t *testing.T) {
			require.Error(t, UpdateTopupGroupRatioByJSONString(raw))
			require.Equal(t, 2.0, GetTopupGroupRatio("vip"))
		})
	}
}

func TestValidateTopupGroupRatioDoesNotPublish(t *testing.T) {
	previous := TopupGroupRatio2JSONString()
	t.Cleanup(func() { require.NoError(t, UpdateTopupGroupRatioByJSONString(previous)) })
	require.NoError(t, UpdateTopupGroupRatioByJSONString(`{"default":1}`))
	require.NoError(t, ValidateTopupGroupRatioJSON(`{"default":2}`))
	require.Equal(t, 1.0, GetTopupGroupRatio("default"))
}
