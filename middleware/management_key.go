package middleware

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/service"
	"github.com/gin-gonic/gin"
)

const managementKeyContext = "management_key"

type managementOperation struct {
	ID, Method, Path, Scope, Domain string
	Target                          bool
	Root                            bool
}

var managementOperations = []managementOperation{
	{"channels.list", "GET", "/api/channel/", "channels.read", "channel", false, false},
	{"channels.get", "GET", "/api/channel/:id", "channels.read", "channel", true, false},
	{"channels.ops", "GET", "/api/channel/ops", "channels.read", "channel", false, false},
	{"channels.quota_history", "GET", "/api/channel/:id/quota/history", "channels.read", "channel", true, false},
	{"channels.quota_status", "GET", "/api/channel/quota/status", "channels.read", "channel", false, false},
	{"channels.set_status", "POST", "/api/channel/:id/status", "channels.operate", "channel", true, false},
	{"tokens.list", "GET", "/api/token/", "tokens.read", "token", false, false},
	{"tokens.get", "GET", "/api/token/:id", "tokens.read", "token", true, false},
	{"logs.list", "GET", "/api/log/", "logs.read", "log", false, false},
	{"logs.stats", "GET", "/api/log/stat", "logs.read", "log", false, false},
	{"logs.overview", "GET", "/api/log/overview", "logs.read", "log", false, false},
	{"system.instances", "GET", "/api/system-info/instances", "system.read", "system", false, true},
	{"system.self", "GET", "/api/user/self", "system.read", "system", false, false},
}

func managementForbidden(c *gin.Context, code string) {
	c.Header("Cache-Control", "no-store")
	c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"success": false, "code": code, "message": common.TranslateMessage(c, i18n.MsgAuthInsufficientPrivilege)})
}

// ManagementCredentialBoundary blocks reserved credentials even on public and
// legacy API routes, before any controller or optional authentication can run.
func ManagementCredentialBoundary() gin.HandlerFunc {
	return func(c *gin.Context) {
		raw, ok := authorizationToken(c.GetHeader("Authorization"))
		if !ok || !strings.HasPrefix(raw, model.ManagementKeyPrefix) {
			c.Next()
			return
		}
		c.Header("Cache-Control", "no-store")
		allowed := c.Request.Method == http.MethodGet && c.FullPath() == "/api/management/capabilities"
		for _, op := range managementOperations {
			if op.Method == c.Request.Method && op.Path == c.FullPath() {
				allowed = true
				break
			}
		}
		if !allowed {
			managementForbidden(c, "MANAGEMENT_ROUTE_FORBIDDEN")
			return
		}
		c.Next()
	}
}

func managementOperationAvailable(key model.ManagementKeyMetadata, user *model.UserBase, op managementOperation) bool {
	found := false
	for _, scope := range key.Scopes {
		if scope == op.Scope {
			found = true
			break
		}
	}
	if !found || !service.ManagementScopeAvailable(user, op.Scope) || (op.Root && user.Role < common.RoleRootUser) {
		return false
	}
	switch op.Domain {
	case "channel":
		if len(key.ChannelIDs) > 0 && !op.Target {
			return false
		}
	case "token":
		if len(key.TokenIDs) > 0 && !op.Target {
			return false
		}
	case "log":
		if len(key.ChannelIDs) > 0 || len(key.TokenIDs) > 0 {
			return false
		}
	}
	return true
}

func enforceManagementRequest(c *gin.Context, user *model.UserBase) bool {
	value, exists := c.Get(managementKeyContext)
	if !exists {
		return true
	}
	key, ok := value.(model.ManagementKeyMetadata)
	if !ok {
		managementForbidden(c, "MANAGEMENT_KEY_INVALID")
		return false
	}
	if c.Request.Method == http.MethodGet && c.FullPath() == "/api/management/capabilities" {
		return true
	}
	for _, op := range managementOperations {
		if c.Request.Method != op.Method || c.FullPath() != op.Path {
			continue
		}
		if !managementOperationAvailable(key, user, op) {
			managementForbidden(c, "MANAGEMENT_SCOPE_FORBIDDEN")
			return false
		}
		if op.Target {
			id, err := strconv.Atoi(c.Param("id"))
			if err != nil || id <= 0 {
				managementForbidden(c, "MANAGEMENT_TARGET_FORBIDDEN")
				return false
			}
			ids := key.ChannelIDs
			if op.Domain == "token" {
				ids = key.TokenIDs
			}
			if len(ids) > 0 {
				found := false
				for _, target := range ids {
					if target == id {
						found = true
						break
					}
				}
				if !found {
					managementForbidden(c, "MANAGEMENT_TARGET_FORBIDDEN")
					return false
				}
			}
			if op.ID == "channels.set_status" {
				// Rebind exactly the checked payload. Duplicate/unknown fields and multiple
				// JSON values cannot change what the existing controller subsequently sees.
				body, err := io.ReadAll(io.LimitReader(c.Request.Body, 4097))
				if err != nil || len(body) > 4096 || common.ValidateUniqueJSONKeys(body) != nil {
					managementForbidden(c, "MANAGEMENT_CONFIRMATION_REQUIRED")
					return false
				}
				var payload map[string]int
				if common.Unmarshal(body, &payload) != nil || len(payload) != 1 {
					managementForbidden(c, "MANAGEMENT_CONFIRMATION_REQUIRED")
					return false
				}
				status, exists := payload["status"]
				if !exists || (status != common.ChannelStatusEnabled && status != common.ChannelStatusManuallyDisabled) || len(c.Request.Header.Values("X-MyAPI-Confirm")) != 1 || c.GetHeader("X-MyAPI-Confirm") != fmt.Sprintf("channel.status:%d:%d", id, status) {
					managementForbidden(c, "MANAGEMENT_CONFIRMATION_REQUIRED")
					return false
				}
				c.Request.Body = io.NopCloser(bytes.NewReader(body))
			}
		}
		c.Set("management_operation", op.ID)
		return true
	}
	managementForbidden(c, "MANAGEMENT_ROUTE_FORBIDDEN")
	return false
}

// RequireManagementSession disallows PATs and management keys for every key
// management operation, including reads, and rechecks the owner directly.
func RequireManagementSession() gin.HandlerFunc {
	return func(c *gin.Context) {
		identity, ok := GetSessionAuthIdentity(c)
		if !ok {
			managementForbidden(c, "MANAGEMENT_SESSION_REQUIRED")
			return
		}
		owner, err := model.GetUserById(identity.UserID, false)
		if err != nil || owner.Status != common.UserStatusEnabled || owner.Role < common.RoleAdminUser {
			managementForbidden(c, "MANAGEMENT_SESSION_REQUIRED")
			return
		}
		var session model.UserSession
		if err := model.DB.Where("sid = ? AND user_id = ?", identity.SessionID, identity.UserID).First(&session).Error; err != nil || session.Status != model.UserSessionStatusActive || session.RevokedAt != 0 || session.ExpiresAt <= time.Now().Unix() || session.Version != identity.SessionVersion || session.UserAuthVersion != identity.UserAuthVersion || owner.AuthVersion != identity.UserAuthVersion {
			managementForbidden(c, "MANAGEMENT_SESSION_REQUIRED")
			return
		}
		c.Next()
	}
}

func ManagementCapabilities(c *gin.Context) {
	value, ok := c.Get(managementKeyContext)
	key, valid := value.(model.ManagementKeyMetadata)
	if !ok || !valid {
		managementForbidden(c, "MANAGEMENT_KEY_REQUIRED")
		return
	}
	owner, err := model.GetUserById(c.GetInt("id"), false)
	if err != nil {
		writeDashboardAuthError(c, service.ErrAuthTokenInvalid)
		return
	}
	scopes := []string{}
	for _, scope := range key.Scopes {
		if service.ManagementScopeAvailable(owner.ToBaseUser(), scope) {
			scopes = append(scopes, scope)
		}
	}
	operations := []string{}
	for _, op := range managementOperations {
		if managementOperationAvailable(key, owner.ToBaseUser(), op) {
			operations = append(operations, op.ID)
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"key_id": key.ID, "scopes": scopes, "channel_ids": key.ChannelIDs, "token_ids": key.TokenIDs, "expires_at": key.ExpiresAt, "operations": operations}})
}

func classifyManagementCredential(c *gin.Context, raw string) (*model.UserBase, service.AuthIdentity, error) {
	key, user, err := model.FindLiveManagementKey(raw, time.Now().Unix())
	if err != nil {
		return nil, service.AuthIdentity{}, service.ErrAuthTokenInvalid
	}
	metadata, err := key.Metadata()
	if err != nil || user.Status != common.UserStatusEnabled || user.Role < common.RoleAdminUser {
		return nil, service.AuthIdentity{}, service.ErrAuthTokenInvalid
	}
	c.Set(managementKeyContext, metadata)
	c.Set("management_key_id", metadata.ID)
	return user, service.AuthIdentity{UserID: user.Id, UserAuthVersion: user.AuthVersion}, nil
}
