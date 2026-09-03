package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRWMapFailedJSONUpdatesPreserveExistingData(t *testing.T) {
	for _, testCase := range []struct {
		name string
		run  func(*RWMap[string, int]) error
	}{
		{
			name: "UnmarshalJSON",
			run: func(values *RWMap[string, int]) error {
				return values.UnmarshalJSON([]byte(`{"new":2,"wrong":"not-an-int"}`))
			},
		},
		{
			name: "LoadFromJsonString",
			run: func(values *RWMap[string, int]) error {
				return LoadFromJsonString(values, `{"new":2,"wrong":"not-an-int"}`)
			},
		},
		{
			name: "LoadFromJsonStringWithCallback",
			run: func(values *RWMap[string, int]) error {
				callbackCount := 0
				err := LoadFromJsonStringWithCallback(values, `{"new":2,"wrong":"not-an-int"}`, func() {
					callbackCount++
				})
				assert.Zero(t, callbackCount)
				return err
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			values := NewRWMap[string, int]()
			values.Set("old", 1)

			err := testCase.run(values)
			require.Error(t, err)
			assert.Equal(t, map[string]int{"old": 1}, values.ReadAll())
		})
	}
}

func TestLoadFromJsonStringWithCallbackRunsOnceAfterSuccessfulSwap(t *testing.T) {
	values := NewRWMap[string, int]()
	values.Set("old", 1)
	callbackCount := 0

	err := LoadFromJsonStringWithCallback(values, `{"new":2}`, func() {
		callbackCount++
	})
	require.NoError(t, err)
	assert.Equal(t, 1, callbackCount)
	assert.Equal(t, map[string]int{"new": 2}, values.ReadAll())
}

func TestRWMapSuccessfulJSONUpdatesReplaceExistingData(t *testing.T) {
	for _, testCase := range []struct {
		name string
		run  func(*RWMap[string, int]) error
	}{
		{
			name: "UnmarshalJSON",
			run: func(values *RWMap[string, int]) error {
				return values.UnmarshalJSON([]byte(`{"new":2}`))
			},
		},
		{
			name: "LoadFromJsonString",
			run: func(values *RWMap[string, int]) error {
				return LoadFromJsonString(values, `{"new":2}`)
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			values := NewRWMap[string, int]()
			values.Set("old", 1)
			require.NoError(t, testCase.run(values))
			assert.Equal(t, map[string]int{"new": 2}, values.ReadAll())
		})
	}
}
