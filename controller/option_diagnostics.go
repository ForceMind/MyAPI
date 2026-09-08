package controller

import (
	"net/url"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/service"
	"github.com/gin-gonic/gin"
)

// GetOptionDiagnostics serves the root-only, no-store diagnostics route.
// Its business path never reads a runtime generation or cache; platform-level
// authentication and rate limiting remain responsible for their own controls.
func GetOptionDiagnostics(c *gin.Context) {
	includeValid := false
	query, err := url.ParseQuery(c.Request.URL.RawQuery)
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	for name, values := range query {
		if name != "include_valid" || len(values) != 1 {
			common.ApiErrorI18n(c, i18n.MsgInvalidParams)
			return
		}
		if values[0] == "true" {
			includeValid = true
		} else if values[0] != "false" {
			common.ApiErrorI18n(c, i18n.MsgInvalidParams)
			return
		}
	}
	if diagnostics, err := service.GetOptionDiagnostics(includeValid); err == nil {
		common.ApiSuccess(c, diagnostics)
	} else {
		// The service deliberately returns no database/parser details here.
		common.ApiErrorI18n(c, i18n.MsgDatabaseError)
	}
}
