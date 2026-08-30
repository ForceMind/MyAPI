package controller

import (
	"net/http"
	"strings"

	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/service"
	"github.com/ForceMind/MyAPI/setting"
	"github.com/ForceMind/MyAPI/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

// accessProfileMetadata is kept as an alias for controller compatibility;
// resolution now lives in the model domain layer.
type accessProfileMetadata = model.AccessProfileMetadata

func getAccessProfileMetadata(groupName, configuredDescription string) accessProfileMetadata {
	return model.ResolveAccessProfile(groupName, configuredDescription)
}

func GetGroups(c *gin.Context) {
	groupNames := make([]string, 0)
	for groupName := range ratio_setting.GetGroupRatioCopy() {
		groupNames = append(groupNames, groupName)
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    groupNames,
	})
}

func GetUserGroups(c *gin.Context) {
	usableGroups := make(map[string]map[string]interface{})
	userGroup := ""
	userId := c.GetInt("id")
	userGroup, _ = model.GetUserGroup(userId, false)
	userUsableGroups := service.GetUserUsableGroups(userGroup)
	// Keep account tier metadata separate from the per-key access profiles.
	// The legacy group remains the routing/eligibility source during migration,
	// while clients can explain what the user's account level means without
	// guessing from names such as "default" or "vip".
	accountTier := resolveUserAccountTier(userId, userGroup)
	for groupName, _ := range ratio_setting.GetGroupRatioCopy() {
		// UserUsableGroups contains the groups that the user can use
		if desc, ok := userUsableGroups[groupName]; ok {
			profile := getAccessProfileMetadata(groupName, desc)
			usableGroups[groupName] = map[string]interface{}{
				"ratio":   service.GetUserGroupRatio(userGroup, groupName),
				"desc":    desc,
				"profile": profile,
			}
		}
	}
	if _, ok := userUsableGroups["auto"]; ok {
		profile := getAccessProfileMetadata("auto", setting.GetUsableGroupDescription("auto"))
		usableGroups["auto"] = map[string]interface{}{
			"ratio":   "自动",
			"desc":    setting.GetUsableGroupDescription("auto"),
			"profile": profile,
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"message":      "",
		"data":         usableGroups,
		"account_tier": accountTier,
	})
}

// resolveUserAccountTier keeps the explicit account-level identity visible to
// clients while retaining the legacy group as a safe fallback for old rows.
// A read failure must not make the groups endpoint unavailable; it simply uses
// the compatibility mapping already used before account tiers were added.
func resolveUserAccountTier(userID int, legacyGroup string) model.AccountTierMetadata {
	if userID > 0 && model.DB != nil {
		if user, err := model.GetUserById(userID, false); err == nil && user != nil {
			if id := strings.TrimSpace(user.AccountTierID); id != "" {
				return model.ResolveAccountTierID(id, "")
			}
		}
	}
	return model.ResolveAccountTier(legacyGroup, "")
}
