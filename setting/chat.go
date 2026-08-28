package setting

import (
	"encoding/json"

	"github.com/QuantumNous/new-api/common"
)

// Keep third-party client links opt-in. Administrators can add only the
// integrations they trust from System Settings without shipping promotional
// entries to every deployment by default.
var Chats = []map[string]string{}

func UpdateChatsByJsonString(jsonString string) error {
	Chats = make([]map[string]string, 0)
	return json.Unmarshal([]byte(jsonString), &Chats)
}

func Chats2JsonString() string {
	jsonBytes, err := json.Marshal(Chats)
	if err != nil {
		common.SysLog("error marshalling chats: " + err.Error())
		return "[]"
	}
	return string(jsonBytes)
}
