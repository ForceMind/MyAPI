package model

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"github.com/ForceMind/MyAPI/common"
	"gorm.io/gorm"
)

const ManagementKeyPrefix = "myapi_mgmt_"

// ManagementKey stores only a digest; the random bearer credential is never persisted.
type ManagementKey struct {
	ID             int    `json:"id" gorm:"primaryKey"`
	OwnerID        int    `json:"-" gorm:"index;not null"`
	Name           string `json:"name" gorm:"type:varchar(128);not null"`
	Prefix         string `json:"prefix" gorm:"type:varchar(32);not null"`
	SecretHash     string `json:"-" gorm:"type:varchar(64);uniqueIndex;not null"`
	ScopesJSON     string `json:"-" gorm:"type:text;not null"`
	ChannelIDsJSON string `json:"-" gorm:"type:text;not null"`
	TokenIDsJSON   string `json:"-" gorm:"type:text;not null"`
	CreatedAt      int64  `json:"created_at" gorm:"bigint;not null"`
	ExpiresAt      int64  `json:"expires_at" gorm:"bigint;not null"`
	RevokedAt      int64  `json:"revoked_at" gorm:"bigint;not null"`
}

type ManagementKeyMetadata struct {
	ID         int      `json:"id"`
	Name       string   `json:"name"`
	Prefix     string   `json:"prefix"`
	Scopes     []string `json:"scopes"`
	ChannelIDs []int    `json:"channel_ids"`
	TokenIDs   []int    `json:"token_ids"`
	CreatedAt  int64    `json:"created_at"`
	ExpiresAt  int64    `json:"expires_at"`
	RevokedAt  int64    `json:"revoked_at"`
}

// ManagementKeyAudit is committed atomically with creation/revocation. It contains no credential or request body.
type ManagementKeyAudit struct {
	ID        int    `gorm:"primaryKey"`
	KeyID     int    `gorm:"index;not null"`
	OwnerID   int    `gorm:"index;not null"`
	Action    string `gorm:"type:varchar(32);not null"`
	CreatedAt int64  `gorm:"bigint;not null"`
}

func ManagementKeyDigest(secret string) string {
	digest := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(digest[:])
}

func (key *ManagementKey) Metadata() (ManagementKeyMetadata, error) {
	result := ManagementKeyMetadata{ID: key.ID, Name: key.Name, Prefix: key.Prefix, CreatedAt: key.CreatedAt, ExpiresAt: key.ExpiresAt, RevokedAt: key.RevokedAt}
	if err := common.UnmarshalJsonStr(key.ScopesJSON, &result.Scopes); err != nil {
		return result, err
	}
	if err := common.UnmarshalJsonStr(key.ChannelIDsJSON, &result.ChannelIDs); err != nil {
		return result, err
	}
	if err := common.UnmarshalJsonStr(key.TokenIDsJSON, &result.TokenIDs); err != nil {
		return result, err
	}
	if result.Scopes == nil || result.ChannelIDs == nil || result.TokenIDs == nil {
		return result, errors.New("invalid management key grants")
	}
	return result, nil
}

func InsertManagementKey(key *ManagementKey) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(key).Error; err != nil {
			return err
		}
		return tx.Create(&ManagementKeyAudit{KeyID: key.ID, OwnerID: key.OwnerID, Action: "created", CreatedAt: key.CreatedAt}).Error
	})
}

func ListManagementKeys(ownerID int) ([]ManagementKeyMetadata, error) {
	var rows []ManagementKey
	if err := DB.Where("owner_id = ?", ownerID).Order("id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]ManagementKeyMetadata, 0, len(rows))
	for _, key := range rows {
		metadata, err := key.Metadata()
		if err != nil {
			return nil, err
		}
		result = append(result, metadata)
	}
	return result, nil
}

func RevokeManagementKey(ownerID, keyID int, now int64) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var key ManagementKey
		if err := lockForUpdate(tx).Where("id = ? AND owner_id = ?", keyID, ownerID).First(&key).Error; err != nil {
			return err
		}
		if key.RevokedAt != 0 {
			return nil
		}
		updated := tx.Model(&ManagementKey{}).Where("id = ? AND owner_id = ? AND revoked_at = ?", keyID, ownerID, 0).Update("revoked_at", now)
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected == 0 {
			return nil
		}
		return tx.Create(&ManagementKeyAudit{KeyID: keyID, OwnerID: ownerID, Action: "revoked", CreatedAt: now}).Error
	})
}

// FindLiveManagementKey deliberately bypasses every identity/key cache.
func FindLiveManagementKey(secret string, now int64) (*ManagementKey, *UserBase, error) {
	var key ManagementKey
	if err := DB.Where("secret_hash = ? AND revoked_at = ? AND expires_at > ?", ManagementKeyDigest(secret), 0, now).First(&key).Error; err != nil {
		return nil, nil, err
	}
	var owner User
	if err := DB.Omit("password", "access_token").First(&owner, key.OwnerID).Error; err != nil {
		return nil, nil, err
	}
	return &key, owner.ToBaseUser(), nil
}
