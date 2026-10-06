package controller

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/ForceMind/MyAPI/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func GetChannelModelDiscovery(c *gin.Context)     { channelModelDiscovery(c, false) }
func RefreshChannelModelDiscovery(c *gin.Context) { channelModelDiscovery(c, true) }

func channelModelDiscovery(c *gin.Context, refresh bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "status": "invalid_channel"})
		return
	}
	succeeded := true
	if refresh {
		attempt, beginErr := model.BeginChannelModelDiscovery(c.Request.Context(), id)
		if beginErr != nil {
			writeChannelModelDiscoveryError(c, beginErr)
			return
		}
		if model.ChannelModelDiscoverySource(attempt.Channel) != "manual" {
			ids, fetchErr := fetchChannelUpstreamModelIDsWithContext(c.Request.Context(), attempt.Channel, true)
			succeeded = fetchErr == nil
			// Persist the terminal state even if the requesting browser disconnects.
			// The upstream deadline remains bounded independently.
			persistCtx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), 5*time.Second)
			applied, completeErr := model.CompleteChannelModelDiscovery(persistCtx, attempt, ids, succeeded)
			cancel()
			if completeErr != nil {
				writeChannelModelDiscoveryError(c, completeErr)
				return
			}
			succeeded = succeeded && applied
		}
	}
	snapshot, err := model.GetChannelModelDiscovery(c.Request.Context(), id)
	if err != nil {
		writeChannelModelDiscoveryError(c, err)
		return
	}
	if refresh && snapshot.Status == "failed" {
		succeeded = false
	}
	c.JSON(http.StatusOK, gin.H{"success": succeeded, "data": snapshot})
}

func writeChannelModelDiscoveryError(c *gin.Context, err error) {
	code, status := http.StatusInternalServerError, "discovery_unavailable"
	if errors.Is(err, gorm.ErrRecordNotFound) {
		code, status = http.StatusNotFound, "channel_not_found"
	}
	// Never expose database details, upstream bodies, credentials or URLs.
	c.JSON(code, gin.H{"success": false, "status": status})
}
