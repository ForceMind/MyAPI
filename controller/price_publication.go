package controller

import (
	"encoding/hex"
	"errors"
	"net/http"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func GetPricePublicationState(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	snapshot, err := model.ReadPricePublicationSnapshot(c.Request.Context())
	if err != nil {
		writePricePublicationError(c, err)
		return
	}
	digest, err := snapshot.Digest()
	if err != nil {
		writePricePublicationError(c, err)
		return
	}
	receipts := make([]model.PricePublication, 0)
	if err := model.DB.WithContext(c.Request.Context()).Order("revision DESC").Order("id DESC").Limit(20).Find(&receipts).Error; err != nil {
		writePricePublicationError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"snapshot": snapshot, "expected_digest": digest, "receipts": receipts, "runtime_ready": model.PricingRuntimeReady()})
}

func PreviewOpenAIPricePublication(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	digest := c.Param("digest")
	decoded, err := hex.DecodeString(digest)
	if err != nil || len(decoded) != 32 || hex.EncodeToString(decoded) != digest {
		writePricePublicationError(c, service.ErrPricePublicationInput)
		return
	}
	preview, err := service.PreviewOpenAIPricePublication(c.Request.Context(), c.Param("digest"))
	if err != nil {
		writePricePublicationError(c, err)
		return
	}
	common.ApiSuccess(c, preview)
}

func ApplyOpenAIPricePublication(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var request service.PricePublicationRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16384)
	if len(c.Request.URL.Query()) != 0 || common.DecodeJsonStrict(c.Request.Body, &request) != nil {
		writePricePublicationError(c, service.ErrPricePublicationInput)
		return
	}
	for _, digest := range []string{request.ID, request.ExpectedDigest} {
		decoded, err := hex.DecodeString(digest)
		if err != nil || len(decoded) != 32 || hex.EncodeToString(decoded) != digest {
			writePricePublicationError(c, service.ErrPricePublicationInput)
			return
		}
	}
	receipt, err := service.ApplyOpenAIPricePublication(c.Request.Context(), c.GetInt("id"), request)
	if receipt != nil && (err != nil || !model.PricingRuntimeReady()) {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "code": "price_publication_committed_runtime_unavailable", "data": gin.H{"receipt": receipt, "runtime_ready": false}, "message": common.TranslateMessage(c, i18n.MsgOperationFailed)})
		return
	}
	if err != nil {
		writePricePublicationError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"receipt": receipt, "runtime_ready": true})
}

func writePricePublicationError(c *gin.Context, err error) {
	status, code, message := http.StatusInternalServerError, "price_publication_failed", i18n.MsgOperationFailed
	switch {
	case errors.Is(err, service.ErrPricePublicationInput):
		status, code, message = http.StatusBadRequest, "price_publication_invalid_input", i18n.MsgInvalidParams
	case errors.Is(err, model.ErrPricePublicationConflict):
		status, code = http.StatusConflict, "price_publication_conflict"
	case errors.Is(err, model.ErrPricePublicationLocked):
		status, code = http.StatusConflict, "price_publication_locked"
	case errors.Is(err, gorm.ErrRecordNotFound):
		status, code, message = http.StatusNotFound, "price_publication_not_found", i18n.MsgNotFound
	}
	c.JSON(status, gin.H{"success": false, "code": code, "message": common.TranslateMessage(c, message)})
}
