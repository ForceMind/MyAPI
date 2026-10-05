package service

import (
	"context"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/pkg/billingexpr"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func feePriceFixture(t *testing.T, db *gorm.DB) *billingexpr.BillingSnapshot {
	return feePriceFixtureForModel(t, db, "fixture-model")
}

func feePriceFixtureForModel(t *testing.T, db *gorm.DB, modelName string) *billingexpr.BillingSnapshot {
	t.Helper()
	require.NoError(t, db.AutoMigrate(&model.OfficialPriceVersion{}, &model.PricePublication{}))
	source := publicationSourceFixture(strings.ReplaceAll(publicationDocumentFixture(), "fixture-model", modelName))
	source.FetchedAt = 100
	frozen, err := FreezeOpenAIPriceSource(context.Background(), db, source)
	require.NoError(t, err)
	candidate, err := BuildOpenAIPricePublicationCandidate(frozen, modelName)
	require.NoError(t, err)
	id := strings.Repeat("f", 64)
	after, err := common.Marshal(model.PricePublicationSnapshot{Modes: map[string]string{modelName: "tiered_expr"}, Expressions: map[string]string{modelName: candidate.Expression}, State: model.PricePublicationState{Models: map[string]model.PublishedModelPrice{modelName: {PublicationID: id, SourceSHA256: frozen.ContentSHA256, ExpressionSHA256: candidate.ExpressionSHA256}}}})
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.PricePublication{ID: id, Action: "publish", ActorID: 1, AfterJSON: string(after)}).Error)
	return &billingexpr.BillingSnapshot{BillingMode: "tiered_expr", ModelName: modelName, ExprString: candidate.Expression, ExprHash: candidate.ExpressionSHA256, ExprVersion: 1, GroupRatio: 1, QuotaPerUnit: 500000, OfficialPricePublicationID: id, OfficialPriceSourceSHA256: frozen.ContentSHA256}
}

func feeUsageFixture() *dto.Usage {
	usage := &dto.Usage{InputTokens: 100, OutputTokens: 10, TotalTokens: 110, InputTokensDetails: &dto.InputTokenDetails{CachedTokens: 40, CacheWriteTokens: 30}, OutputTokensDetails: &dto.OutputTokenDetails{ReasoningTokens: 4}}
	return &dto.Usage{BillingUsage: &dto.BillingUsage{Source: dto.BillingUsageSourceOAIResponses, Semantic: dto.BillingUsageSemanticOpenAI, OpenAIUsage: usage,
		ResponsesTextEvidence: &dto.ResponsesTextEvidence{Model: "fixture-model", ServiceTier: "default", CacheRead: common.GetPointer(40), CacheWrite: common.GetPointer(30)}}}
}

func TestFeeBudgetFrozenPriceQualificationAndExactCost(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/fee.db"), &gorm.Config{})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	snapshot := feePriceFixture(t, db)
	info := &relaycommon.RelayInfo{TieredBillingSnapshot: snapshot}
	row := &model.TokenBudgetReservation{RequestServiceTier: "default", ModelName: "fixture-model", InputTokens: 100, MaxOutputTokens: 20}
	require.NoError(t, freezeFeeBudgetPrice(context.Background(), db, info, row))
	assert.Equal(t, "0.00045", row.FeeReservedUSD, "bound uses the highest eligible input-category rate")
	cost, err := calculateFeeBudgetUSD(row, feeUsageFixture())
	require.NoError(t, err)
	assert.Equal(t, "0.000239", cost)
	for _, tier := range []string{"", "auto", "priority", "standard"} {
		copy := *row
		copy.RequestServiceTier = tier
		require.ErrorIs(t, freezeFeeBudgetPrice(context.Background(), db, info, &copy), ErrFeeBudgetEvidence)
	}
	snapshot.ExprString = "p * 999"
	cost, err = calculateFeeBudgetUSD(row, feeUsageFixture())
	require.NoError(t, err)
	assert.Equal(t, "0.000239", cost, "in-flight cost does not reprice")
	require.Error(t, freezeFeeBudgetPrice(context.Background(), db, info, &model.TokenBudgetReservation{ModelName: "fixture-model", InputTokens: 100, MaxOutputTokens: 20}))
	for _, change := range []func(*dto.BillingUsage){
		func(b *dto.BillingUsage) { b.ResponsesTextEvidence.CacheRead = nil },
		func(b *dto.BillingUsage) { b.ResponsesTextEvidence.CacheWrite = nil },
		func(b *dto.BillingUsage) { b.ResponsesTextEvidence.Model = "different" },
		func(b *dto.BillingUsage) { b.ResponsesTextEvidence.ServiceTier = "priority" },
		func(b *dto.BillingUsage) {
			b.OpenAIUsage.InputTokensDetails.CachedTokensDetails = dto.NewCachedTokenDetails(39, 0, 0)
		},
		func(b *dto.BillingUsage) { b.Estimated = true },
		func(b *dto.BillingUsage) { b.Incomplete = true },
		func(b *dto.BillingUsage) { b.OpenAIUsage.InputTokensDetails.ImageTokens = 1 },
	} {
		usage := feeUsageFixture()
		change(usage.BillingUsage)
		_, err := calculateFeeBudgetUSD(row, usage)
		require.Error(t, err)
	}
	zero := feeUsageFixture()
	zero.BillingUsage.OpenAIUsage = &dto.Usage{InputTokensDetails: &dto.InputTokenDetails{}}
	zero.BillingUsage.ResponsesTextEvidence.CacheRead = common.GetPointer(0)
	zero.BillingUsage.ResponsesTextEvidence.CacheWrite = common.GetPointer(0)
	cost, err = calculateFeeBudgetUSD(row, zero)
	require.NoError(t, err)
	assert.Equal(t, "0", cost, "explicit zero differs from missing cache counters")
}
