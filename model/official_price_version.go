package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const MaxOfficialPriceDocumentBytes = 1 << 20

// Natural content-addressed primary key. No mutable published-price fields:
// storing source evidence cannot change the effective price generation.
type OfficialPriceVersion struct {
	ContentSHA256 string `gorm:"primaryKey;size:64"`
	Document      string `gorm:"size:1048576"`
	FetchedAt     int64
}

func StoreOfficialPriceVersion(ctx context.Context, db *gorm.DB, document string, fetchedAt int64) (*OfficialPriceVersion, error) {
	if document == "" || len(document) > MaxOfficialPriceDocumentBytes || fetchedAt <= 0 {
		return nil, fmt.Errorf("invalid official price document")
	}
	digest := sha256.Sum256([]byte(document))
	candidate := OfficialPriceVersion{ContentSHA256: hex.EncodeToString(digest[:]), Document: document, FetchedAt: fetchedAt}
	var stored OfficialPriceVersion
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&candidate).Error; err != nil {
			return err
		}
		if err := tx.Where("content_sha256 = ?", candidate.ContentSHA256).Take(&stored).Error; err != nil {
			return err
		}
		if stored.Document != document {
			return fmt.Errorf("stored official price version conflicts with source digest")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &stored, nil
}

func GetOfficialPriceVersion(ctx context.Context, db *gorm.DB, digest string) (*OfficialPriceVersion, error) {
	if len(digest) != 64 || digest != strings.ToLower(digest) {
		return nil, fmt.Errorf("invalid official price digest")
	}
	if _, err := hex.DecodeString(digest); err != nil {
		return nil, err
	}
	var version OfficialPriceVersion
	if err := db.WithContext(ctx).Where("content_sha256 = ?", digest).Take(&version).Error; err != nil {
		return nil, err
	}
	if len(version.Document) == 0 || len(version.Document) > MaxOfficialPriceDocumentBytes || version.FetchedAt <= 0 {
		return nil, fmt.Errorf("invalid stored official price source")
	}
	actual := sha256.Sum256([]byte(version.Document))
	if hex.EncodeToString(actual[:]) != version.ContentSHA256 {
		return nil, fmt.Errorf("stored official price digest mismatch")
	}
	return &version, nil
}
