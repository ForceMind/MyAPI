package controller

import (
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var fetchOpenAIOfficialPricing = service.FetchOpenAIOfficialPricing

var freezeOpenAIPriceSource = service.FreezeOpenAIPriceSource
var loadFrozenOpenAIPriceSource = service.LoadFrozenOpenAIPriceSource

// This endpoint returns source evidence only. Fetching is not publication and
// cannot overwrite any effective price, model ratio, or administrator override.
func GetOpenAIOfficialPricing(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	snapshot, err := fetchOpenAIOfficialPricing(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "code": "official_pricing_unavailable", "message": common.TranslateMessage(c, i18n.MsgOperationFailed)})
		return
	}
	common.ApiSuccess(c, snapshot)
}

// Save fetches the fixed server source. No client URL, rates, model list or
// publication instruction is accepted by this endpoint.
func SaveOpenAIOfficialPriceSource(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if len(c.Request.URL.Query()) != 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "code": "official_pricing_input_forbidden", "message": common.TranslateMessage(c, i18n.MsgInvalidParams)})
		return
	}
	if c.Request.Body != nil {
		body, err := io.ReadAll(io.LimitReader(c.Request.Body, 1))
		if err != nil || len(body) != 0 {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "code": "official_pricing_input_forbidden", "message": common.TranslateMessage(c, i18n.MsgInvalidParams)})
			return
		}
	}
	snapshot, err := fetchOpenAIOfficialPricing(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "code": "official_pricing_unavailable", "message": common.TranslateMessage(c, i18n.MsgOperationFailed)})
		return
	}
	frozen, err := freezeOpenAIPriceSource(c.Request.Context(), model.DB, snapshot)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "official_pricing_source_save_failed", "message": common.TranslateMessage(c, i18n.MsgOperationFailed)})
		return
	}
	common.ApiSuccess(c, frozen)
}

func GetFrozenOpenAIOfficialPriceSource(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	digest := c.Param("digest")
	if len(digest) != 64 || digest != strings.ToLower(digest) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "code": "official_pricing_invalid_version", "message": common.TranslateMessage(c, i18n.MsgInvalidParams)})
		return
	}
	if _, err := hex.DecodeString(digest); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "code": "official_pricing_invalid_version", "message": common.TranslateMessage(c, i18n.MsgInvalidParams)})
		return
	}
	snapshot, err := loadFrozenOpenAIPriceSource(c.Request.Context(), model.DB, digest)
	if err != nil {
		status, code, message := http.StatusInternalServerError, "official_pricing_source_load_failed", i18n.MsgOperationFailed
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status, code, message = http.StatusNotFound, "official_pricing_version_not_found", i18n.MsgNotFound
		}
		c.JSON(status, gin.H{"success": false, "code": code, "message": common.TranslateMessage(c, message)})
		return
	}
	common.ApiSuccess(c, snapshot)
}
