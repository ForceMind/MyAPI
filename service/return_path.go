package service

import (
	"strings"

	"github.com/ForceMind/MyAPI/setting/system_setting"
)

func PaymentReturnURL(suffix string) string {
	base := strings.TrimRight(system_setting.ServerAddress, "/")
	return base + suffix
}
