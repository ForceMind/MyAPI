package middleware

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// PlaygroundSessionAuth follows UserAuth. An opaque dashboard PAT must not
// acquire the browser playground's ability to select a user's relay keys.
func PlaygroundSessionAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, ok := GetSessionAuthIdentity(c); !ok || c.GetBool("use_access_token") {
			abortWithOpenAiMessage(c, http.StatusForbidden, common.TranslateMessage(c, i18n.MsgAuthAccessTokenInvalid), types.ErrorCodeAccessDenied)
			return
		}
		c.Next()
	}
}

// PlaygroundKeyAuth resolves a key ID, never a browser-supplied key secret.
// Ownership and revocation are read from the database on every request, even
// when relay credential caches contain an older snapshot.
func PlaygroundKeyAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		identity, ok := GetSessionAuthIdentity(c)
		if !ok || c.GetBool("use_access_token") || identity.UserID != c.GetInt("id") {
			abortWithOpenAiMessage(c, http.StatusForbidden, common.TranslateMessage(c, i18n.MsgAuthAccessTokenInvalid), types.ErrorCodeAccessDenied)
			return
		}
		rawID := strings.TrimSpace(c.GetHeader("X-MyAPI-Key-ID"))
		if rawID == "" {
			abortWithOpenAiMessage(c, http.StatusBadRequest, common.TranslateMessage(c, i18n.MsgTokenNotProvided), types.ErrorCode("playground_key_required"))
			return
		}
		id, err := strconv.Atoi(rawID)
		if err != nil || id <= 0 || strconv.Itoa(id) != rawID || len(c.Request.Header.Values("X-MyAPI-Key-ID")) != 1 {
			abortWithOpenAiMessage(c, http.StatusBadRequest, common.TranslateMessage(c, i18n.MsgTokenInvalid), types.ErrorCode("playground_key_invalid"))
			return
		}
		token, err := model.GetTokenByIds(id, identity.UserID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				abortWithOpenAiMessage(c, http.StatusUnauthorized, common.TranslateMessage(c, i18n.MsgTokenInvalid), types.ErrorCode("playground_key_invalid"))
			} else {
				common.SysError("PlaygroundKeyAuth key lookup failed: " + err.Error())
				abortWithOpenAiMessage(c, http.StatusInternalServerError, common.TranslateMessage(c, i18n.MsgDatabaseError))
			}
			return
		}
		if err := model.ValidateTokenSnapshot(token); err != nil {
			abortWithOpenAiMessage(c, http.StatusUnauthorized, common.TranslateMessage(c, i18n.MsgTokenInvalid), types.ErrorCode("playground_key_invalid"))
			return
		}

		// These exact routes are aliases of the public relay contract. Normalize
		// only the internal URL before admission, distribution and billing so no
		// playground-specific group or quota bypass can be activated. FullPath,
		// RequestURI and the logged origin retain the browser-facing route.
		canonicalPath := ""
		switch {
		case c.Request.Method == http.MethodPost && c.Request.URL.Path == "/pg/chat/completions":
			canonicalPath = "/v1/chat/completions"
		case c.Request.Method == http.MethodGet && c.Request.URL.Path == "/pg/models":
			canonicalPath = "/v1/models"
		default:
			abortWithOpenAiMessage(c, http.StatusNotFound, common.TranslateMessage(c, i18n.MsgTokenInvalid), types.ErrorCodeAccessDenied)
			return
		}
		// Browser credentials authenticate this local session only. A channel's
		// explicit/wildcard header override and parameter snapshots must never
		// forward them to its upstream. Leave the caller's request untouched.
		originalRequest := c.Request
		c.Request = originalRequest.Clone(originalRequest.Context())
		defer func() {
			// Retain context updates from dispatch/billing while restoring the
			// browser-facing URL and headers for outer middleware.
			c.Request.URL = originalRequest.URL
			c.Request.Header = originalRequest.Header
		}()
		for name := range c.Request.Header {
			switch strings.ToLower(name) {
			case "authorization", "cookie", "proxy-authorization", "x-api-key",
				"x-goog-api-key", "mj-api-secret", "x-myapi-key-id", "x-auth-token", "x-access-token",
				"sec-websocket-protocol", "new-api-user":
				delete(c.Request.Header, name)
			}
		}
		c.Set("playground_original_path", originalRequest.URL.Path)
		c.Request.URL.Path = canonicalPath
		c.Request.URL.RawPath = ""
		if !authenticateTokenIdentity(c, token) {
			return
		}
		common.MarkSensitiveRequestDiagnostics(c)
		c.Next()
	}
}
