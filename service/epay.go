package service

import (
	"github.com/ForceMind/MyAPI/setting/operation_setting"
	"github.com/ForceMind/MyAPI/setting/system_setting"
)

func GetCallbackAddress() string {
	if operation_setting.CustomCallbackAddress == "" {
		return system_setting.ServerAddress
	}
	return operation_setting.CustomCallbackAddress
}
