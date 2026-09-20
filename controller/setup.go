package controller

import (
	"errors"
	"net/http"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type Setup struct {
	Status       bool   `json:"status"`
	RootInit     bool   `json:"root_init"`
	DatabaseType string `json:"database_type"`
}

type SetupRequest struct {
	Username           string `json:"username"`
	Password           string `json:"password"`
	ConfirmPassword    string `json:"confirmPassword"`
	SelfUseModeEnabled bool   `json:"SelfUseModeEnabled"`
	DemoSiteEnabled    bool   `json:"DemoSiteEnabled"`
}

func GetSetup(c *gin.Context) {
	setup := Setup{
		Status: constant.Setup,
	}
	if constant.Setup {
		c.JSON(200, gin.H{
			"success": true,
			"data":    setup,
		})
		return
	}
	setup.RootInit = model.RootUserExists()
	setup.DatabaseType = string(common.MainDatabaseType())
	c.JSON(200, gin.H{
		"success": true,
		"data":    setup,
	})
}

func PostSetup(c *gin.Context) {
	if constant.Setup {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "系统已经初始化完成"})
		return
	}
	rootExists := model.RootUserExists()
	var req SetupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "请求参数有误"})
		return
	}

	var hashedPassword string
	if !rootExists {
		if len(req.Username) > 12 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "用户名长度不能超过12个字符"})
			return
		}
		if req.Password != req.ConfirmPassword {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "两次输入的密码不一致"})
			return
		}
		if len(req.Password) < 8 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "密码长度至少为8个字符"})
			return
		}
		var err error
		hashedPassword, err = common.Password2Hash(req.Password)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "系统错误: " + err.Error()})
			return
		}
	}

	fundingMode := operation_setting.UserFundingModeEnabled
	if req.SelfUseModeEnabled {
		fundingMode = operation_setting.UserFundingModeDisabled
	}
	var fundingState model.UserFundingStateSnapshot
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var rootCount int64
		if err := tx.Model(&model.User{}).Where("role = ?", common.RoleRootUser).Count(&rootCount).Error; err != nil {
			return err
		}
		if rootCount == 0 {
			if hashedPassword == "" {
				return errors.New("root account validation is stale")
			}
			rootUser := model.User{
				Username:    req.Username,
				Password:    hashedPassword,
				Role:        common.RoleRootUser,
				Status:      common.UserStatusEnabled,
				DisplayName: "Root User",
				Quota:       100000000,
				AffCode:     common.GetRandomString(8),
			}
			if err := tx.Create(&rootUser).Error; err != nil {
				return err
			}
		}
		var err error
		fundingState, err = model.PersistSetupFundingOptionsTx(tx, req.SelfUseModeEnabled, req.DemoSiteEnabled, fundingMode)
		if err != nil {
			return err
		}
		return tx.Create(&model.Setup{
			Version:       common.Version,
			InitializedAt: time.Now().Unix(),
		}).Error
	})
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "系统初始化失败: " + err.Error()})
		return
	}
	if err := model.PublishSetupFundingOptions(req.SelfUseModeEnabled, req.DemoSiteEnabled, fundingState); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "系统配置发布失败: " + err.Error()})
		return
	}
	constant.Setup = true
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "系统初始化成功"})
}
