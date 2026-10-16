package model

import (
	"context"
	"errors"
	"strings"
	"unicode"

	"github.com/ForceMind/MyAPI/common"
	"gorm.io/gorm"
)

var (
	ErrTokenModelLimitsInvalid  = errors.New("invalid token model limits")
	ErrTokenModelLimitsDenied   = errors.New("token model limits access denied")
	ErrTokenModelLimitsConflict = errors.New("token model limits changed")
)

// Model limits are stored in a TEXT column, whose portable MySQL limit is
// 65,535 bytes. Keep model identifiers exact; matching belongs to relay policy.
const MaxTokenModelLimitsBytes = 65535

type TokenModelLimitsView struct {
	TokenID            int      `json:"token_id"`
	TokenName          string   `json:"token_name"`
	ModelLimitsEnabled bool     `json:"model_limits_enabled"`
	Models             []string `json:"models"`
	Revision           string   `json:"revision"`
}

type TokenModelLimitsCommand struct {
	ExpectedRevision   string
	ModelLimitsEnabled bool
	Models             []string
}

func (command TokenModelLimitsCommand) Validate() error {
	if len(command.ExpectedRevision) != 64 {
		return ErrTokenModelLimitsInvalid
	}
	for _, char := range command.ExpectedRevision {
		if !strings.ContainsRune("0123456789abcdef", char) {
			return ErrTokenModelLimitsInvalid
		}
	}
	seen := make(map[string]struct{}, len(command.Models))
	bytes := 0
	for index, name := range command.Models {
		if _, exists := seen[name]; exists {
			return ErrTokenModelLimitsInvalid
		}
		seen[name] = struct{}{}
		if index > 0 {
			bytes++
		}
		bytes += len(name)
		if bytes > MaxTokenModelLimitsBytes {
			return ErrTokenModelLimitsInvalid
		}
	}
	return nil
}

// Legacy identifiers remain exact when retained. Only newly introduced IDs
// must satisfy the current delimiter/whitespace rules; never silently repair
// an old allowlist into a different authorization set.
func (command TokenModelLimitsCommand) ValidateNewModels(currentModels []string) error {
	remaining := make(map[string]int, len(currentModels))
	for _, name := range currentModels {
		remaining[name]++
	}
	for _, name := range command.Models {
		if remaining[name] > 0 {
			remaining[name]--
			continue
		}
		if name == "" || strings.ContainsRune(name, ',') || strings.ContainsFunc(name, func(char rune) bool {
			return unicode.IsSpace(char) || unicode.IsControl(char) || char == '\uFEFF'
		}) {
			return ErrTokenModelLimitsInvalid
		}
	}
	return nil
}

func (token *Token) ModelLimitsView() (*TokenModelLimitsView, error) {
	revision, err := token.QuotaMetadataFingerprint()
	if err != nil {
		return nil, err
	}
	return &TokenModelLimitsView{
		TokenID: token.Id, TokenName: token.Name,
		ModelLimitsEnabled: token.ModelLimitsEnabled,
		Models:             token.GetModelLimits(), Revision: revision,
	}, nil
}

func ReadTokenModelLimits(ctx context.Context, userID, tokenID int) (*TokenModelLimitsView, error) {
	if DB == nil {
		return nil, gorm.ErrInvalidDB
	}
	if userID <= 0 || tokenID <= 0 {
		return nil, ErrTokenModelLimitsInvalid
	}
	var actor User
	if err := DB.WithContext(ctx).Select("id", "status").First(&actor, userID).Error; err != nil {
		return nil, err
	}
	if actor.Status != common.UserStatusEnabled {
		return nil, ErrTokenModelLimitsDenied
	}
	var token Token
	if err := DB.WithContext(ctx).Where("id = ? AND user_id = ?", tokenID, userID).First(&token).Error; err != nil {
		return nil, err
	}
	return token.ModelLimitsView()
}

// UpdateTokenModelLimits never uses Token.Update: that method writes unrelated
// metadata from its input. Accounting and credential fields remain untouched.
func UpdateTokenModelLimits(ctx context.Context, userID, tokenID int, command TokenModelLimitsCommand) (*TokenModelLimitsView, error) {
	if DB == nil {
		return nil, gorm.ErrInvalidDB
	}
	if userID <= 0 || tokenID <= 0 {
		return nil, ErrTokenModelLimitsInvalid
	}
	if err := command.Validate(); err != nil {
		return nil, err
	}
	var identity Token
	if err := DB.WithContext(ctx).Select("id", "user_id", "key").Where("id = ? AND user_id = ?", tokenID, userID).First(&identity).Error; err != nil {
		return nil, err
	}
	var view *TokenModelLimitsView
	err := mutateTokenMetadataWithCacheFence(identity.Key, func(tx *gorm.DB) error {
		tx = tx.WithContext(ctx)
		var actor User
		if err := lockForUpdate(tx).Select("id", "status").First(&actor, userID).Error; err != nil {
			return err
		}
		if actor.Status != common.UserStatusEnabled {
			return ErrTokenModelLimitsDenied
		}
		var current Token
		if err := lockForUpdate(tx).Where("id = ? AND user_id = ?", tokenID, userID).First(&current).Error; err != nil {
			return err
		}
		revision, err := current.QuotaMetadataFingerprint()
		if err != nil {
			return err
		}
		if current.Key != identity.Key || revision != command.ExpectedRevision {
			return ErrTokenModelLimitsConflict
		}
		if err := command.ValidateNewModels(current.GetModelLimits()); err != nil {
			return err
		}
		current.ModelLimitsEnabled = command.ModelLimitsEnabled
		current.ModelLimits = strings.Join(command.Models, ",")
		if err := tx.Model(&Token{}).Where("id = ? AND user_id = ?", tokenID, userID).Updates(map[string]any{
			"model_limits_enabled": current.ModelLimitsEnabled,
			"model_limits":         current.ModelLimits,
		}).Error; err != nil {
			return err
		}
		view, err = current.ModelLimitsView()
		return err
	})
	if err != nil {
		return nil, err
	}
	return view, nil
}
