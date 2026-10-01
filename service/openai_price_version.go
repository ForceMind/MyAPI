package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/ForceMind/MyAPI/model"
	"gorm.io/gorm"
)

// FreezeOpenAIPriceSource accepts only source material retained by the server
// fetcher, not client-posted rates or a mutation of the displayed model list.
func FreezeOpenAIPriceSource(ctx context.Context, db *gorm.DB, snapshot *OpenAIOfficialPriceSnapshot) (*OpenAIOfficialPriceSnapshot, error) {
	if snapshot == nil || snapshot.SourceURL != openAIOfficialPricingURL || snapshot.Currency != "USD" || snapshot.UnitTokens != 1_000_000 ||
		snapshot.ServiceTier != "standard" || snapshot.Scope != "text-token-price-source-not-published" || snapshot.sourceDocument == "" {
		return nil, fmt.Errorf("official price source cannot be frozen")
	}
	digest := sha256.Sum256([]byte(snapshot.sourceDocument))
	if hex.EncodeToString(digest[:]) != snapshot.ContentSHA256 {
		return nil, fmt.Errorf("official price source digest mismatch")
	}
	if _, err := parseOpenAIStandardTextPrices([]byte(snapshot.sourceDocument)); err != nil {
		return nil, err
	}
	version, err := model.StoreOfficialPriceVersion(ctx, db, snapshot.sourceDocument, snapshot.FetchedAt)
	if err != nil {
		return nil, err
	}
	return LoadFrozenOpenAIPriceSource(ctx, db, version.ContentSHA256)
}

func LoadFrozenOpenAIPriceSource(ctx context.Context, db *gorm.DB, digest string) (*OpenAIOfficialPriceSnapshot, error) {
	version, err := model.GetOfficialPriceVersion(ctx, db, digest)
	if err != nil {
		return nil, err
	}
	models, err := parseOpenAIStandardTextPrices([]byte(version.Document))
	if err != nil {
		return nil, err
	}
	return &OpenAIOfficialPriceSnapshot{sourceDocument: version.Document, SourceURL: openAIOfficialPricingURL, FetchedAt: version.FetchedAt, ContentSHA256: version.ContentSHA256,
		Currency: "USD", UnitTokens: 1_000_000, ServiceTier: "standard", Scope: "text-token-price-source-not-published", Models: models}, nil
}

func CalculateFrozenOpenAITextCost(ctx context.Context, db *gorm.DB, digest, modelName, profile string, usage OpenAITextCostUsage) (*OpenAITextSourceCost, error) {
	snapshot, err := LoadFrozenOpenAIPriceSource(ctx, db, digest)
	if err != nil {
		return nil, err
	}
	return CalculateOpenAITextSourceCost(snapshot, modelName, profile, usage)
}
