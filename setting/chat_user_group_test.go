package setting

import (
	"reflect"
	"sync"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChatsFailedUpdatePreservesExistingValueAndGetterIsIsolated(t *testing.T) {
	previous := Chats2JsonString()
	t.Cleanup(func() {
		require.NoError(t, UpdateChatsByJsonString(previous))
	})

	const baseline = `[{"name":"Existing","url":"https://existing.example"}]`
	require.NoError(t, UpdateChatsByJsonString(baseline))
	require.Error(t, UpdateChatsByJsonString(`[{"name":"Next"},{"name":1}]`))
	assert.JSONEq(t, baseline, Chats2JsonString())

	copyOfChats := GetChats()
	require.Len(t, copyOfChats, 1)
	copyOfChats[0]["name"] = "Mutated"
	copyOfChats[0]["extra"] = "local-only"
	assert.JSONEq(t, baseline, Chats2JsonString())

	require.NoError(t, UpdateChatsByJsonString(`[]`))
	assert.Empty(t, GetChats())
	require.NoError(t, UpdateChatsByJsonString(`[null]`))
	require.Len(t, GetChats(), 1)
	assert.Nil(t, GetChats()[0], "deep-copying a legacy null entry must preserve its JSON shape")
	require.NoError(t, UpdateChatsByJsonString(`null`))
	assert.JSONEq(t, `[]`, Chats2JsonString())
}

func TestChatsConcurrentSnapshotIsAlwaysComplete(t *testing.T) {
	previous := Chats2JsonString()
	t.Cleanup(func() {
		require.NoError(t, UpdateChatsByJsonString(previous))
	})

	const oldValue = `[{"name":"Old","url":"https://old.example"}]`
	const newValue = `[{"name":"New","url":"https://new.example"}]`
	require.NoError(t, UpdateChatsByJsonString(oldValue))

	start := make(chan struct{})
	results := make(chan string, 8)
	updateResult := make(chan error, 1)
	var workers sync.WaitGroup
	workers.Add(9)
	for range 8 {
		go func() {
			defer workers.Done()
			<-start
			results <- Chats2JsonString()
		}()
	}
	go func() {
		defer workers.Done()
		<-start
		updateResult <- UpdateChatsByJsonString(newValue)
	}()
	close(start)
	workers.Wait()
	close(results)
	require.NoError(t, <-updateResult)

	for result := range results {
		var snapshot []map[string]string
		require.NoError(t, common.Unmarshal([]byte(result), &snapshot))
		oldSnapshot := []map[string]string{{"name": "Old", "url": "https://old.example"}}
		newSnapshot := []map[string]string{{"name": "New", "url": "https://new.example"}}
		assert.True(t, reflect.DeepEqual(snapshot, oldSnapshot) || reflect.DeepEqual(snapshot, newSnapshot), "unexpected partial chat snapshot: %s", result)
	}
}

func TestUserUsableGroupsFailedUpdatePreservesExistingValueAndGetterIsIsolated(t *testing.T) {
	previous := UserUsableGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, UpdateUserUsableGroupsByJSONString(previous))
	})

	const baseline = `{"default":"Default","vip":"VIP"}`
	require.NoError(t, UpdateUserUsableGroupsByJSONString(baseline))
	require.Error(t, UpdateUserUsableGroupsByJSONString(`{"next":"Next","wrong":1}`))
	assert.JSONEq(t, baseline, UserUsableGroups2JSONString())

	groups := GetUserUsableGroupsCopy()
	groups["default"] = "Mutated"
	groups["extra"] = "local-only"
	assert.JSONEq(t, baseline, UserUsableGroups2JSONString())

	require.NoError(t, UpdateUserUsableGroupsByJSONString(`{}`))
	assert.Empty(t, GetUserUsableGroupsCopy())
	require.NoError(t, UpdateUserUsableGroupsByJSONString(`null`))
	assert.JSONEq(t, `{}`, UserUsableGroups2JSONString())
}
