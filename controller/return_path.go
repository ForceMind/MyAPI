package controller

import (
	"strings"

	"github.com/ForceMind/MyAPI/setting/system_setting"
)

func paymentReturnPath(suffix string) string {
	base := strings.TrimRight(system_setting.GetServerAddress(), "/")
	return base + suffix
}
