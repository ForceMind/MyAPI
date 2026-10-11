package model

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHistoricalFundingOnlyPermitDoesNotRequireTokenLegApproval(t *testing.T) {
	db := openAccountQuotaTestDB(t)
	initializeAccountQuotaExecutionGateForTest(t, db)
	fixture := newAccountQuotaFixture(t, db, "execution-history-funding-only", 1000, 500, false, 0, 0, false)
	input := legacyWalletSettlementInput(fixture, "execution-history-funding-only", 40)
	input.ApplyToken = false
	fact := insertUnpermittedSettlementFact(t, db, input)

	permit, err := PermitHistoricalAccountQuotaSettlement(context.Background(), db, HistoricalAccountQuotaExecutionPermitInput{
		FactID: fact.ID, Settlement: input, RequestFingerprint: fact.RequestFingerprint,
		InitialFundingApplied: false, InitialTokenApplied: false,
		AllowFunding: true, AllowToken: false,
		ApprovalID: "review-funding-only", EvidenceDigest: strings.Repeat("c", 64),
	})
	require.NoError(t, err)
	require.NotNil(t, permit)

	stored, err := RecoverAccountQuotaSettlementFact(context.Background(), db, fact, "historical-funding-only")
	require.NoError(t, err)
	require.Equal(t, AccountQuotaSettlementApplied, stored.State)
	assert.True(t, stored.FundingApplied)
	assert.True(t, stored.TokenApplied, "the non-applicable token leg is only a completion marker")

	user, token, _ := loadAccountBalances(t, db, fixture)
	assert.Equal(t, int64(960), user.Quota)
	assert.Equal(t, int64(500), token.RemainQuota)
	assert.Zero(t, token.UsedQuota)
}

func TestHistoricalFundingOnlyPartialFactCompletesWithoutTokenLeg(t *testing.T) {
	db := openAccountQuotaTestDB(t)
	initializeAccountQuotaExecutionGateForTest(t, db)
	fixture := newAccountQuotaFixture(t, db, "execution-history-funding-complete", 1000, 500, false, 0, 0, false)
	input := legacyWalletSettlementInput(fixture, "execution-history-funding-complete", 40)
	input.ApplyToken = false
	fact := insertUnpermittedSettlementFact(t, db, input)
	require.NoError(t, db.Model(&AccountQuotaSettlementFact{}).Where("id = ?", fact.ID).
		Update("funding_applied", true).Error)
	fact.FundingApplied = true

	permit, err := PermitHistoricalAccountQuotaSettlement(context.Background(), db, HistoricalAccountQuotaExecutionPermitInput{
		FactID: fact.ID, Settlement: input, RequestFingerprint: fact.RequestFingerprint,
		InitialFundingApplied: true, InitialTokenApplied: false,
		AllowFunding: false, AllowToken: false,
		ApprovalID: "review-funding-complete", EvidenceDigest: strings.Repeat("d", 64),
	})
	require.NoError(t, err)
	require.NotNil(t, permit)

	stored, err := RecoverAccountQuotaSettlementFact(context.Background(), db, fact, "historical-funding-complete")
	require.NoError(t, err)
	assert.Equal(t, AccountQuotaSettlementApplied, stored.State)
	assert.True(t, stored.FundingApplied)
	assert.True(t, stored.TokenApplied, "the non-applicable token leg must be completed during restart")
	beforeReplay := *stored
	replayed, err := RecoverAccountQuotaSettlementFact(context.Background(), db, stored, "historical-funding-complete-replay")
	require.NoError(t, err)
	assert.Equal(t, beforeReplay, *replayed, "an applied fact replay must be read-only")

	user, token, _ := loadAccountBalances(t, db, fixture)
	assert.Equal(t, int64(1000), user.Quota)
	assert.Equal(t, int64(500), token.RemainQuota)
	assert.Zero(t, token.UsedQuota)
}
