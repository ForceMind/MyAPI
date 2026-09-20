package model

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupLegacySettlementFactTest(t *testing.T, label string, withSubscription bool) (*gormTestFixture, accountQuotaFixture) {
	t.Helper()
	db := openAccountQuotaTestDB(t)
	require.NoError(t, db.AutoMigrate(&AccountQuotaSettlementFact{}))
	setQuotaWriterStateForTest(t, db, QuotaWriterModeLegacy, 91)
	fixture := newAccountQuotaFixture(t, db, label, 1000, 500, withSubscription, 1000, 0, false)
	return &gormTestFixture{DB: db}, fixture
}

func TestLegacyWalletSettlementFactRecoversFundingThenTokenAcrossRestart(t *testing.T) {
	dbFixture, fixture := setupLegacySettlementFactTest(t, "wallet-settlement", false)
	require.NoError(t, dbFixture.DB.Model(&User{}).Where("id = ?", fixture.User.Id).Updates(map[string]any{"quota": 900, "quota_version": 1}).Error)
	require.NoError(t, dbFixture.DB.Model(&Token{}).Where("id = ?", fixture.Token.Id).Updates(map[string]any{
		"remain_quota": 400, "used_quota": 100, "quota_version": 1,
	}).Error)
	input := AccountQuotaSettlementFactInput{
		EventKey: "billing-settlement:wallet-settlement:v1", RequestID: "wallet-settlement",
		Kind: AccountQuotaSettlementKindLegacyWallet, UserID: fixture.User.Id, TokenID: fixture.Token.Id, Delta: 50, ApplyToken: true,
	}
	fact, err := EnsureAccountQuotaSettlementFact(context.Background(), dbFixture.DB, input)
	require.NoError(t, err)
	accountQuotaSettlementAfterFundingCommitHook = func(string) error { return errors.New("wallet funding commit response lost") }
	accountQuotaSettlementBeforeTokenHook = func(*AccountQuotaSettlementFact) error { return errors.New("token step unavailable") }
	t.Cleanup(func() {
		accountQuotaSettlementAfterFundingCommitHook = nil
		accountQuotaSettlementBeforeTokenHook = nil
	})
	stored, err := RecoverAccountQuotaSettlementFact(context.Background(), dbFixture.DB, fact, "wallet-settlement-first")
	require.ErrorIs(t, err, ErrAccountQuotaSettlementPending)
	require.NotNil(t, stored)
	assert.True(t, stored.FundingApplied)
	assert.False(t, stored.TokenApplied)
	assert.Equal(t, AccountQuotaSettlementRetryable, stored.State)
	user, token, _ := loadAccountBalances(t, dbFixture.DB, fixture)
	assert.Equal(t, 850, user.Quota)
	assert.Equal(t, 400, token.RemainQuota)
	assert.Equal(t, 100, token.UsedQuota)

	accountQuotaSettlementAfterFundingCommitHook = nil
	accountQuotaSettlementBeforeTokenHook = nil
	var restarted AccountQuotaSettlementFact
	require.NoError(t, dbFixture.DB.First(&restarted, fact.ID).Error)
	processed, err := RunAccountQuotaSettlementFacts(context.Background(), dbFixture.DB, "wallet-settlement-restart", 10)
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	user, token, _ = loadAccountBalances(t, dbFixture.DB, fixture)
	assert.Equal(t, 850, user.Quota)
	assert.Equal(t, 350, token.RemainQuota)
	assert.Equal(t, 150, token.UsedQuota)
	require.NoError(t, dbFixture.DB.First(&restarted, fact.ID).Error)
	assert.Equal(t, AccountQuotaSettlementApplied, restarted.State)

	processed, err = RunAccountQuotaSettlementFacts(context.Background(), dbFixture.DB, "wallet-settlement-replay", 10)
	require.NoError(t, err)
	assert.Zero(t, processed)
}

func TestLegacySubscriptionSettlementFactHandlesCommitUnknownIdempotently(t *testing.T) {
	dbFixture, fixture := setupLegacySettlementFactTest(t, "subscription-settlement", true)
	require.NotNil(t, fixture.Sub)
	require.NoError(t, dbFixture.DB.Model(&UserSubscription{}).Where("id = ?", fixture.Sub.Id).Updates(map[string]any{"amount_used": int64(100), "quota_version": 1}).Error)
	require.NoError(t, dbFixture.DB.Model(&Token{}).Where("id = ?", fixture.Token.Id).Updates(map[string]any{
		"remain_quota": 400, "used_quota": 100, "quota_version": 1,
	}).Error)
	input := AccountQuotaSettlementFactInput{
		EventKey: "billing-settlement:subscription-settlement:v1", RequestID: "subscription-settlement",
		Kind: AccountQuotaSettlementKindLegacySubscription, UserID: fixture.User.Id, TokenID: fixture.Token.Id,
		SubscriptionID: fixture.Sub.Id, Delta: -40, ApplyToken: true,
	}
	accountQuotaSettlementAfterCreateHook = func(string) error { return errors.New("create response lost") }
	accountQuotaSettlementAfterFundingCommitHook = func(string) error { return errors.New("funding commit response lost") }
	accountQuotaSettlementAfterTokenCommitHook = func(string) error { return errors.New("token commit response lost") }
	t.Cleanup(func() {
		accountQuotaSettlementAfterCreateHook = nil
		accountQuotaSettlementAfterFundingCommitHook = nil
		accountQuotaSettlementAfterTokenCommitHook = nil
	})
	fact, err := EnsureAccountQuotaSettlementFact(context.Background(), dbFixture.DB, input)
	require.NoError(t, err)
	stored, err := RecoverAccountQuotaSettlementFact(context.Background(), dbFixture.DB, fact, "subscription-settlement-worker")
	require.NoError(t, err)
	assert.Equal(t, AccountQuotaSettlementApplied, stored.State)
	_, token, subscription := loadAccountBalances(t, dbFixture.DB, fixture)
	require.NotNil(t, subscription)
	assert.EqualValues(t, 60, subscription.AmountUsed)
	assert.Equal(t, 440, token.RemainQuota)
	assert.Equal(t, 60, token.UsedQuota)

	accountQuotaSettlementAfterCreateHook = nil
	accountQuotaSettlementAfterFundingCommitHook = nil
	accountQuotaSettlementAfterTokenCommitHook = nil
	replayed, err := EnsureAccountQuotaSettlementFact(context.Background(), dbFixture.DB, input)
	require.NoError(t, err)
	stored, err = RecoverAccountQuotaSettlementFact(context.Background(), dbFixture.DB, replayed, "subscription-settlement-restart")
	require.NoError(t, err)
	assert.Equal(t, AccountQuotaSettlementApplied, stored.State)
	_, token, subscription = loadAccountBalances(t, dbFixture.DB, fixture)
	assert.EqualValues(t, 60, subscription.AmountUsed)
	assert.Equal(t, 440, token.RemainQuota)
	assert.Equal(t, 60, token.UsedQuota)
}

func TestAuthoritativeSettlementFactRecoversAfterTerminalCommitResponseLoss(t *testing.T) {
	db := openAccountQuotaTestDB(t)
	require.NoError(t, db.AutoMigrate(&AccountQuotaSettlementFact{}))
	fixture := newAccountQuotaFixture(t, db, "authoritative-settlement", 1000, 500, false, 0, 0, false)
	reserve, err := ReserveAccountQuota(context.Background(), db, accountReserveInput(fixture, "authoritative-settlement", "wallet_only", 100))
	require.NoError(t, err)
	input := AccountQuotaSettlementFactInput{
		EventKey: "billing-settlement:authoritative-settlement:v2", RequestID: reserve.RequestID,
		Kind: AccountQuotaSettlementKindAuthoritative, UserID: reserve.UserID, TokenID: reserve.TokenID,
		SubscriptionID: reserve.SubscriptionID, ReserveReceiptID: reserve.ID, WriterEpoch: reserve.WriterEpoch, ActualQuota: 80,
	}
	fact, err := EnsureAccountQuotaSettlementFact(context.Background(), db, input)
	require.NoError(t, err)
	var hookCalls int
	accountQuotaSettlementAfterTerminalCommitHook = func(string) error {
		hookCalls++
		return errors.New("terminal commit response lost")
	}
	t.Cleanup(func() { accountQuotaSettlementAfterTerminalCommitHook = nil })
	stored, err := RecoverAccountQuotaSettlementFact(context.Background(), db, fact, "authoritative-settlement-first")
	require.ErrorIs(t, err, ErrAccountQuotaSettlementPending)
	require.NotNil(t, stored)
	assert.Equal(t, AccountQuotaSettlementRetryable, stored.State)
	assert.Equal(t, 1, hookCalls)

	accountQuotaSettlementAfterTerminalCommitHook = nil
	processed, err := RunAccountQuotaSettlementFacts(context.Background(), db, "authoritative-settlement-restart", 10)
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	require.NoError(t, db.First(&stored, fact.ID).Error)
	assert.Equal(t, AccountQuotaSettlementApplied, stored.State)
	assert.NotZero(t, stored.TerminalReceiptID)
	user, token, _ := loadAccountBalances(t, db, fixture)
	assert.Equal(t, 920, user.Quota)
	assert.Equal(t, 420, token.RemainQuota)
	assert.Equal(t, 80, token.UsedQuota)
	var terminalCount int64
	require.NoError(t, db.Model(&AccountQuotaMutationReceipt{}).
		Where("request_id = ? AND phase = ?", reserve.RequestID, AccountQuotaPhaseSettle).Count(&terminalCount).Error)
	assert.EqualValues(t, 1, terminalCount)
}

func TestAuthoritativeSettlementIntentCommitUnknownReplayAndConflict(t *testing.T) {
	db := openAccountQuotaTestDB(t)
	require.NoError(t, db.AutoMigrate(&AccountQuotaSettlementIntent{}, &AccountQuotaSettlementFact{}))
	fixture := newAccountQuotaFixture(t, db, "settlement-intent", 1000, 500, false, 0, 0, false)
	reserve, err := ReserveAccountQuota(context.Background(), db, accountReserveInput(fixture, "settlement-intent", "wallet_only", 100))
	require.NoError(t, err)
	input := AccountQuotaSettlementFactInput{
		EventKey: "billing-settlement:settlement-intent:v2", RequestID: reserve.RequestID,
		Kind: AccountQuotaSettlementKindAuthoritative, UserID: reserve.UserID, TokenID: reserve.TokenID,
		ReserveReceiptID: reserve.ID, WriterEpoch: reserve.WriterEpoch, ActualQuota: 80,
	}
	accountQuotaSettlementIntentAfterCreateHook = func(string) error { return errors.New("intent commit response lost") }
	t.Cleanup(func() {
		accountQuotaSettlementIntentAfterCreateHook = nil
		accountQuotaSettlementIntentReadbackHook = nil
	})
	intent, err := EnsureAccountQuotaSettlementIntent(context.Background(), db, input)
	require.NoError(t, err)
	require.NotNil(t, intent)
	accountQuotaSettlementIntentAfterCreateHook = nil
	replayed, err := EnsureAccountQuotaSettlementIntent(context.Background(), db, input)
	require.NoError(t, err)
	assert.Equal(t, intent.ID, replayed.ID)
	conflict := input
	conflict.ActualQuota = 81
	_, err = EnsureAccountQuotaSettlementIntent(context.Background(), db, conflict)
	require.ErrorIs(t, err, ErrAccountQuotaTerminalRecoveryConflict)

	processed, err := RunAccountQuotaSettlementIntents(context.Background(), db, "settlement-intent-worker", 10)
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	require.NoError(t, db.First(intent, intent.ID).Error)
	assert.Equal(t, AccountQuotaSettlementApplied, intent.State)
	assert.NotZero(t, intent.FactID)
	assert.NotZero(t, intent.TerminalReceiptID)
	user, token, _ := loadAccountBalances(t, db, fixture)
	assert.Equal(t, 920, user.Quota)
	assert.Equal(t, 420, token.RemainQuota)
	assert.Equal(t, 80, token.UsedQuota)
}

func TestSettlementManualEvidenceCommitUnknownRequiresFullIdentity(t *testing.T) {
	t.Run("matching readback is accepted", func(t *testing.T) {
		db := openAccountQuotaTestDB(t)
		fixture := newAccountQuotaFixture(t, db, "manual-evidence-readback", 1000, 500, false, 0, 0, false)
		reserve, err := ReserveAccountQuota(context.Background(), db, accountReserveInput(fixture, "manual-evidence-readback", "wallet_only", 100))
		require.NoError(t, err)
		accountQuotaSettlementManualEvidenceAfterCommitHook = func(string) error { return errors.New("manual evidence commit response lost") }
		t.Cleanup(func() {
			accountQuotaSettlementManualEvidenceAfterCommitHook = nil
			accountQuotaSettlementManualEvidenceReadbackHook = nil
		})
		evidence, err := PersistAccountQuotaSettlementManualEvidence(context.Background(), db, AccountQuotaTerminalInput{
			RequestID: reserve.RequestID, ReserveReceiptID: reserve.ID, ActualQuota: 70,
		}, errors.New("intent and fact unavailable"))
		require.NoError(t, err)
		require.NotNil(t, evidence)
		assert.Equal(t, reserve.WriterEpoch, evidence.WriterEpoch)
		assert.Equal(t, reserve.ID, evidence.ReserveReceiptID)
		assert.EqualValues(t, 70, evidence.ActualQuota)
		assert.Equal(t, AccountQuotaPhaseSettle, evidence.Phase)
	})

	t.Run("damaged readback is rejected", func(t *testing.T) {
		db := openAccountQuotaTestDB(t)
		fixture := newAccountQuotaFixture(t, db, "manual-evidence-damaged", 1000, 500, false, 0, 0, false)
		reserve, err := ReserveAccountQuota(context.Background(), db, accountReserveInput(fixture, "manual-evidence-damaged", "wallet_only", 100))
		require.NoError(t, err)
		accountQuotaSettlementManualEvidenceAfterCommitHook = func(requestID string) error {
			require.NoError(t, db.Model(&AccountQuotaTerminalRecoveryObligation{}).Where("request_id = ?", requestID).
				Update("request_fingerprint", strings.Repeat("0", 64)).Error)
			return errors.New("manual evidence commit response lost")
		}
		t.Cleanup(func() {
			accountQuotaSettlementManualEvidenceAfterCommitHook = nil
			accountQuotaSettlementManualEvidenceReadbackHook = nil
		})
		evidence, err := PersistAccountQuotaSettlementManualEvidence(context.Background(), db, AccountQuotaTerminalInput{
			RequestID: reserve.RequestID, ReserveReceiptID: reserve.ID, ActualQuota: 70,
		}, errors.New("intent and fact unavailable"))
		assert.Nil(t, evidence)
		require.ErrorIs(t, err, ErrAccountQuotaTerminalRecoveryConflict)
	})
}
