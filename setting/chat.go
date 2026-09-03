package setting

import (
	"sync"

	"github.com/ForceMind/MyAPI/common"
)

// Keep third-party client links opt-in. Administrators can add only the
// integrations they trust from System Settings without shipping promotional
// entries to every deployment by default.
var Chats = []map[string]string{}
var chatsMutex sync.RWMutex

func GetChats() []map[string]string {
	chatsMutex.RLock()
	defer chatsMutex.RUnlock()
	result := make([]map[string]string, len(Chats))
	for i, chat := range Chats {
		if chat == nil {
			continue
		}
		result[i] = make(map[string]string, len(chat))
		for k, v := range chat {
			result[i][k] = v
		}
	}
	return result
}

func ValidateChatsJSON(value string) error {
	var chats []map[string]string
	return common.Unmarshal([]byte(value), &chats)
}

func UpdateChatsByJsonString(jsonString string) error {
	var chats []map[string]string
	if err := common.Unmarshal([]byte(jsonString), &chats); err != nil {
		return err
	}
	if chats == nil {
		chats = []map[string]string{}
	}
	chatsMutex.Lock()
	Chats = chats
	chatsMutex.Unlock()
	return nil
}

func Chats2JsonString() string {
	chatsMutex.RLock()
	defer chatsMutex.RUnlock()
	jsonBytes, err := common.Marshal(Chats)
	if err != nil {
		common.SysLog("error marshalling chats: " + err.Error())
		return "[]"
	}
	return string(jsonBytes)
}
