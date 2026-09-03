package setting

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateMaxTokenAutoGroupsAcceptsAnyPositiveInteger(t *testing.T) {
	original := GetMaxTokenAutoGroups()
	t.Cleanup(func() {
		require.NoError(t, UpdateMaxTokenAutoGroups(fmt.Sprintf("%d", original)))
	})

	require.NoError(t, UpdateMaxTokenAutoGroups("123456"))
	assert.Equal(t, 123456, GetMaxTokenAutoGroups())
}

func TestUpdateMaxTokenAutoGroupsRejectsInvalidValuesWithoutChangingState(t *testing.T) {
	original := GetMaxTokenAutoGroups()
	for _, value := range []string{"", "0", "-1", "1.5", "not-a-number"} {
		t.Run(value, func(t *testing.T) {
			assert.Error(t, UpdateMaxTokenAutoGroups(value))
			assert.Equal(t, original, GetMaxTokenAutoGroups())
		})
	}
}

func TestUpdateAutoGroupsFailurePreservesStateAndGetterIsIsolated(t *testing.T) {
	previous := AutoGroups2JsonString()
	t.Cleanup(func() { require.NoError(t, UpdateAutoGroupsByJsonString(previous)) })
	require.NoError(t, UpdateAutoGroupsByJsonString(`["default","vip"]`))

	require.Error(t, UpdateAutoGroupsByJsonString(`["next",1]`))
	assert.JSONEq(t, `["default","vip"]`, AutoGroups2JsonString())
	groups := GetAutoGroups()
	groups[0] = "mutated"
	assert.Equal(t, []string{"default", "vip"}, GetAutoGroups())
	require.NoError(t, UpdateAutoGroupsByJsonString(`null`))
	assert.JSONEq(t, `[]`, AutoGroups2JsonString())
}
