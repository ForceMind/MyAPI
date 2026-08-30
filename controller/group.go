package controller

import (
	"net/http"

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
		"success": true,
		"message": "",
		"data":    usableGroups,
	})
}
