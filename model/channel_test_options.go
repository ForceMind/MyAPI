package model

import (
	"fmt"
	"strings"

	"github.com/ForceMind/MyAPI/common"
	"gorm.io/gorm"
)

// ChannelTestOptions contains only the non-secret choices that made a manual
// channel test succeed. Keeping them on the channel row avoids browser-local
// behavior differences between administrators or application instances.
type ChannelTestOptions struct {
	Model        string `json:"model"`
	EndpointType string `json:"endpoint_type"`
	Stream       bool   `json:"stream"`
	ChannelType  int    `json:"channel_type"`
}

func GetLastSuccessfulChannelTestOptions(channelID int) (*ChannelTestOptions, error) {
	var channel Channel
	if err := DB.Select("last_successful_test_options").First(&channel, "id = ?", channelID).Error; err != nil {
		return nil, err
	}
	if channel.LastSuccessfulTestOptions == "" {
		return nil, nil
	}
	var options ChannelTestOptions
	if err := common.UnmarshalJsonStr(channel.LastSuccessfulTestOptions, &options); err != nil {
		return nil, fmt.Errorf("decode last successful channel test options: %w", err)
	}
	return &options, nil
}

func SaveLastSuccessfulChannelTestOptions(channelID int, options ChannelTestOptions) error {
	if DB == nil {
		return gorm.ErrInvalidDB
	}
	if channelID <= 0 {
		return fmt.Errorf("successful channel test channel id is invalid")
	}
	options.Model = strings.TrimSpace(options.Model)
	options.EndpointType = strings.TrimSpace(options.EndpointType)
	if options.Model == "" {
		return fmt.Errorf("successful channel test model is empty")
	}
	encoded, err := common.Marshal(options)
	if err != nil {
		return err
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var channel Channel
		if err := lockForUpdate(tx).Select("id", "type", "models", "last_successful_test_options").Where("id = ?", channelID).Take(&channel).Error; err != nil {
			return err
		}
		modelAvailable := false
		for _, availableModel := range channel.GetModels() {
			if strings.TrimSpace(availableModel) == options.Model {
				modelAvailable = true
				break
			}
		}
		if channel.Type != options.ChannelType || !modelAvailable {
			return fmt.Errorf("successful channel test options no longer match channel %d", channelID)
		}
		if channel.LastSuccessfulTestOptions == string(encoded) {
			return nil
		}
		result := tx.Model(&Channel{}).
			Where("id = ? AND type = ? AND models = ?", channelID, channel.Type, channel.Models).
			Update("last_successful_test_options", string(encoded))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("successful channel test options no longer match channel %d", channelID)
		}
		return nil
	})
}
