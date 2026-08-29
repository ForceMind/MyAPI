package controller

import (
	"net/http"

	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/service"
	"github.com/ForceMind/MyAPI/setting"
	"github.com/ForceMind/MyAPI/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

// accessProfileMetadata gives the API/UI a stable, user-facing meaning for
// the legacy token group field. The group value remains the source of truth
// for routing compatibility; this metadata is presentation-only.
type accessProfileMetadata struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

func getAccessProfileMetadata(groupName, configuredDescription string) accessProfileMetadata {
	profile := accessProfileMetadata{
		ID:          groupName,
		Kind:        "custom",
		Label:       groupName,
		Description: configuredDescription,
	}
	switch groupName {
	case "", "default":
		profile.ID = "standard"
		profile.Kind = "standard"
		profile.Label = "Standard access"
		profile.Description = "Uses the standard channel pool and billing rules."
	case "vip":
		profile.ID = "priority"
		profile.Kind = "priority"
		profile.Label = "Priority access"
		profile.Description = "Uses the priority channel pool when your account allows it."
	case "auto":
		profile.ID = "automatic"
		profile.Kind = "automatic"
		profile.Label = "Automatic routing"
		profile.Description = "Tries eligible channel groups in order and can fail over when enabled."
	}
	return profile
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
