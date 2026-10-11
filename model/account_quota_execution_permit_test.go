package model

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func initializeAccountQuotaExecutionGateForTest(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.AutoMigrate(&AccountQuotaExecutionGate{}, &AccountQuotaExecutionPermit{}))
	require.NoError(t, EnsureFreshAccountQuotaExecutionGate(context.Background(), db))
}

func legacyWalletSettlementInput(fixture accountQuotaFixture, suffix string, delta int64) AccountQuotaSettlementFactInput {
	return AccountQuotaSettlementFactInput{
		EventKey: "billing-settlement:" + suffix + ":v1", RequestID: suffix,
		Kind: AccountQuotaSettlementKindLegacyWallet, UserID: fixture.User.Id,
		TokenID: fixture.Token.Id, Delta: delta, ApplyToken: true,
	}
}

func insertUnpermittedSettlementFact(t *testing.T, db *gorm.DB, input AccountQuotaSettlementFactInput) *AccountQuotaSettlementFact {
	t.Helper()
	normalized, fingerprint, err := normalizeAccountQuotaSettlementInput(input)
	require.NoError(t, err)
	now, err := taskRecoveryDBTimestamp(db)
	require.NoError(t, err)
	fact := &AccountQuotaSettlementFact{
		EventKey: normalized.EventKey, RequestID: normalized.RequestID, Kind: normalized.Kind,
		UserID: normalized.UserID, TokenID: normalized.TokenID, SubscriptionID: normalized.SubscriptionID,
		ReserveReceiptID: normalized.ReserveReceiptID, WriterEpoch: normalized.WriterEpoch, ActualQuota: normalized.ActualQuota,
		Delta: normalized.Delta, ApplyToken: normalized.ApplyToken, RequestFingerprint: fingerprint,
		State: AccountQuotaSettlementPending, LockVersion: 1, CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, db.Create(fact).Error)
	return fact
}

func markUnpermittedSettlementFactAppliedForTest(t *testing.T, db *gorm.DB, fact *AccountQuotaSettlementFact, fundingApplied, tokenApplied bool, terminalReceiptID int64) *AccountQuotaSettlementFact {
	t.Helper()
	require.NoError(t, db.Model(&AccountQuotaSettlementFact{}).Where("id = ?", fact.ID).Updates(map[string]any{
		"state": AccountQuotaSettlementApplied, "funding_applied": fundingApplied, "token_applied": tokenApplied,
		"terminal_receipt_id": terminalReceiptID,
	}).Error)
	var stored AccountQuotaSettlementFact
	require.NoError(t, db.First(&stored, fact.ID).Error)
	return &stored
}

func requireSettlementTerminalConflictForTest(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)
	assert.True(t,
		errors.Is(err, ErrAccountQuotaTerminalRecoveryConflict) || errors.Is(err, ErrAccountQuotaSettlementManualRequired),
		"expected terminal recovery conflict or manual-required error, got %v", err,
	)
}

func TestAccountQuotaExecutionGateFailsClosedWithoutSchema(t *testing.T) {
	db := openAccountQuotaTestDB(t)
	require.NoError(t, db.Migrator().DropTable(&AccountQuotaExecutionPermit{}, &AccountQuotaExecutionGate{}))
	fixture := newAccountQuotaFixture(t, db, "gate-schema-missing", 1000, 500, false, 0, 0, false)

	fact, err := EnsureAccountQuotaSettlementFact(context.Background(), db, legacyWalletSettlementInput(fixture, "gate-schema-missing", 10))
	require.Nil(t, fact)
	require.ErrorIs(t, err, ErrAccountQuotaExecutionGateUninitialized)
}

func TestUnpermittedFactIsFrozenAndDoesNotStarvePermittedFact(t *testing.T) {
	db := openAccountQuotaTestDB(t)
	initializeAccountQuotaExecutionGateForTest(t, db)
	fixture := newAccountQuotaFixture(t, db, "execution-frozen", 1000, 500, false, 0, 0, false)
	frozen := insertUnpermittedSettlementFact(t, db, legacyWalletSettlementInput(fixture, "execution-frozen-old", 10))
	before := *frozen

	_, err := RecoverAccountQuotaSettlementFact(context.Background(), db, frozen, "frozen-direct")
	require.ErrorIs(t, err, ErrAccountQuotaExecutionFrozen)
	var after AccountQuotaSettlementFact
	require.NoError(t, db.First(&after, frozen.ID).Error)
	assert.Equal(t, before, after)

	permitted, err := EnsureAccountQuotaSettlementFact(context.Background(), db, legacyWalletSettlementInput(fixture, "execution-permitted-new", 25))
	require.NoError(t, err)
	processed, err := RunAccountQuotaSettlementFacts(context.Background(), db, "permitted-scanner", 1)
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	user, token, _ := loadAccountBalances(t, db, fixture)
	assert.Equal(t, int64(975), user.Quota)
	assert.Equal(t, int64(475), token.RemainQuota)
	assert.Equal(t, int64(25), token.UsedQuota)

	processed, err = RunAccountQuotaSettlementFacts(context.Background(), db, "permitted-replay", 10)
	require.NoError(t, err)
	assert.Zero(t, processed)
	stored, err := RecoverAccountQuotaSettlementFact(context.Background(), db, permitted, "permitted-direct-replay")
	require.NoError(t, err)
	assert.Equal(t, AccountQuotaSettlementApplied, stored.State)
	user, token, _ = loadAccountBalances(t, db, fixture)
	assert.Equal(t, int64(975), user.Quota)
	assert.Equal(t, int64(475), token.RemainQuota)
	assert.Equal(t, int64(25), token.UsedQuota)
}

func TestUnpermittedAppliedFactReplayRequiresCoherentTerminalState(t *testing.T) {
	t.Run("coherent legacy terminal remains read-only replayable", func(t *testing.T) {
		db := openAccountQuotaTestDB(t)
		fixture := newAccountQuotaFixture(t, db, "execution-old-applied-coherent", 1000, 500, false, 0, 0, false)
		fact := insertUnpermittedSettlementFact(t, db, legacyWalletSettlementInput(fixture, "execution-old-applied-coherent", 10))
		fact = markUnpermittedSettlementFactAppliedForTest(t, db, fact, true, true, 0)
		before := *fact

		stored, err := RecoverAccountQuotaSettlementFact(context.Background(), db, fact, "old-applied-coherent")
		require.NoError(t, err)
		require.NotNil(t, stored)
		assert.Equal(t, before, *stored)
		var permitCount int64
		require.NoError(t, db.Model(&AccountQuotaExecutionPermit{}).Count(&permitCount).Error)
		assert.Zero(t, permitCount)
		user, token, _ := loadAccountBalances(t, db, fixture)
		assert.Equal(t, int64(1000), user.Quota)
		assert.Equal(t, int64(500), token.RemainQuota)
		assert.Zero(t, token.UsedQuota)
	})

	for _, test := range []struct {
		name           string
		fundingApplied bool
		tokenApplied   bool
	}{
		{name: "neither leg is complete"},
		{name: "only funding leg is complete", fundingApplied: true},
		{name: "only token leg is complete", tokenApplied: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			db := openAccountQuotaTestDB(t)
			fixture := newAccountQuotaFixture(t, db, "execution-old-applied-incoherent", 1000, 500, false, 0, 0, false)
			fact := insertUnpermittedSettlementFact(t, db, legacyWalletSettlementInput(fixture, "execution-old-applied-incoherent", 10))
			fact = markUnpermittedSettlementFactAppliedForTest(t, db, fact, test.fundingApplied, test.tokenApplied, 0)
			before := *fact

			stored, err := RecoverAccountQuotaSettlementFact(context.Background(), db, fact, "old-applied-incoherent")
			requireSettlementTerminalConflictForTest(t, err)
			require.NotNil(t, stored)
			assert.Equal(t, before, *stored)
			var permitCount int64
			require.NoError(t, db.Model(&AccountQuotaExecutionPermit{}).Count(&permitCount).Error)
			assert.Zero(t, permitCount)
			user, token, _ := loadAccountBalances(t, db, fixture)
			assert.Equal(t, int64(1000), user.Quota)
			assert.Equal(t, int64(500), token.RemainQuota)
			assert.Zero(t, token.UsedQuota)
		})
	}
}

func TestUnpermittedAppliedAuthoritativeFactRequiresMatchingTerminalReceipt(t *testing.T) {
	for _, test := range []struct {
		name              string
		terminalReceiptID func(*testing.T, *gorm.DB, accountQuotaFixture, *AccountQuotaMutationReceipt) int64
	}{
		{
			name: "terminal receipt is missing",
			terminalReceiptID: func(*testing.T, *gorm.DB, accountQuotaFixture, *AccountQuotaMutationReceipt) int64 {
				return 0
			},
		},
		{
			name: "receipt has reserve phase",
			terminalReceiptID: func(_ *testing.T, _ *gorm.DB, _ accountQuotaFixture, reserve *AccountQuotaMutationReceipt) int64 {
				return reserve.ID
			},
		},
		{
			name: "terminal receipt belongs to another subject",
			terminalReceiptID: func(t *testing.T, db *gorm.DB, _ accountQuotaFixture, _ *AccountQuotaMutationReceipt) int64 {
				other := newAccountQuotaFixture(t, db, "execution-old-applied-other-subject", 1000, 500, false, 0, 0, false)
				otherReserve, err := ReserveAccountQuota(context.Background(), db, accountReserveInput(other, "execution-old-applied-other-subject", "wallet_only", 100))
				require.NoError(t, err)
				terminal, err := SettleAccountQuota(context.Background(), db, AccountQuotaTerminalInput{
					RequestID: otherReserve.RequestID, ReserveReceiptID: otherReserve.ID, ActualQuota: 80,
				})
				require.NoError(t, err)
				return terminal.ID
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			db := openAccountQuotaTestDB(t)
			fixture := newAccountQuotaFixture(t, db, "execution-old-applied-authoritative", 1000, 500, false, 0, 0, false)
			reserve, err := ReserveAccountQuota(context.Background(), db, accountReserveInput(fixture, "execution-old-applied-authoritative", "wallet_only", 100))
			require.NoError(t, err)
			input := AccountQuotaSettlementFactInput{
				EventKey: "billing-settlement:execution-old-applied-authoritative:v2", RequestID: reserve.RequestID,
				Kind: AccountQuotaSettlementKindAuthoritative, UserID: reserve.UserID, TokenID: reserve.TokenID,
				SubscriptionID: reserve.SubscriptionID, ReserveReceiptID: reserve.ID, WriterEpoch: reserve.WriterEpoch, ActualQuota: 80,
			}
			fact := insertUnpermittedSettlementFact(t, db, input)
			fact = markUnpermittedSettlementFactAppliedForTest(t, db, fact, true, true, test.terminalReceiptID(t, db, fixture, reserve))
			before := *fact
			userBefore, tokenBefore, _ := loadAccountBalances(t, db, fixture)

			stored, recoverErr := RecoverAccountQuotaSettlementFact(context.Background(), db, fact, "old-applied-authoritative")
			requireSettlementTerminalConflictForTest(t, recoverErr)
			require.NotNil(t, stored)
			assert.Equal(t, before, *stored)
			var permitCount int64
			require.NoError(t, db.Model(&AccountQuotaExecutionPermit{}).Count(&permitCount).Error)
			assert.Zero(t, permitCount)
			userAfter, tokenAfter, _ := loadAccountBalances(t, db, fixture)
			assert.Equal(t, userBefore.Quota, userAfter.Quota)
			assert.Equal(t, tokenBefore.RemainQuota, tokenAfter.RemainQuota)
			assert.Equal(t, tokenBefore.UsedQuota, tokenAfter.UsedQuota)
		})
	}
}

func TestUnpermittedAppliedAuthoritativeFactWithMatchingTerminalReceiptReplaysReadOnly(t *testing.T) {
	db := openAccountQuotaTestDB(t)
	fixture := newAccountQuotaFixture(t, db, "execution-old-applied-authoritative-valid", 1000, 500, false, 0, 0, false)
	reserve, err := ReserveAccountQuota(context.Background(), db, accountReserveInput(fixture, "execution-old-applied-authoritative-valid", "wallet_only", 100))
	require.NoError(t, err)
	terminal, err := SettleAccountQuota(context.Background(), db, AccountQuotaTerminalInput{
		RequestID: reserve.RequestID, ReserveReceiptID: reserve.ID, ActualQuota: 80,
	})
	require.NoError(t, err)
	input := AccountQuotaSettlementFactInput{
		EventKey: "billing-settlement:execution-old-applied-authoritative-valid:v2", RequestID: reserve.RequestID,
		Kind: AccountQuotaSettlementKindAuthoritative, UserID: reserve.UserID, TokenID: reserve.TokenID,
		SubscriptionID: reserve.SubscriptionID, ReserveReceiptID: reserve.ID, WriterEpoch: reserve.WriterEpoch, ActualQuota: 80,
	}
	fact := insertUnpermittedSettlementFact(t, db, input)
	fact = markUnpermittedSettlementFactAppliedForTest(t, db, fact, true, true, terminal.ID)
	before := *fact
	userBefore, tokenBefore, _ := loadAccountBalances(t, db, fixture)

	stored, recoverErr := RecoverAccountQuotaSettlementFact(context.Background(), db, fact, "old-applied-authoritative-valid")
	require.NoError(t, recoverErr)
	require.NotNil(t, stored)
	assert.Equal(t, before, *stored)
	var permitCount int64
	require.NoError(t, db.Model(&AccountQuotaExecutionPermit{}).Count(&permitCount).Error)
	assert.Zero(t, permitCount)
	userAfter, tokenAfter, _ := loadAccountBalances(t, db, fixture)
	assert.Equal(t, userBefore.Quota, userAfter.Quota)
	assert.Equal(t, tokenBefore.RemainQuota, tokenAfter.RemainQuota)
	assert.Equal(t, tokenBefore.UsedQuota, tokenAfter.UsedQuota)
}

func TestUnpermittedAppliedAuthoritativeFactRejectsTamperedTerminalReceiptIdentity(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*testing.T, *gorm.DB, int64)
	}{
		{
			name: "stored request fingerprint is changed",
			mutate: func(t *testing.T, db *gorm.DB, receiptID int64) {
				require.NoError(t, db.Exec(
					"UPDATE account_quota_mutation_receipts SET request_fingerprint = ? WHERE id = ?",
					strings.Repeat("0", 64), receiptID,
				).Error)
			},
		},
		{
			name: "fingerprint input is changed",
			mutate: func(t *testing.T, db *gorm.DB, receiptID int64) {
				require.NoError(t, db.Exec(
					"UPDATE account_quota_mutation_receipts SET audit_key = ? WHERE id = ?",
					"tampered-terminal-audit", receiptID,
				).Error)
			},
		},
		{
			name: "receipt version is unknown",
			mutate: func(t *testing.T, db *gorm.DB, receiptID int64) {
				require.NoError(t, db.Exec(
					"UPDATE account_quota_mutation_receipts SET receipt_version = ? WHERE id = ?",
					AccountQuotaMutationReceiptVersion+1, receiptID,
				).Error)
			},
		},
		{
			name: "fingerprint version is unknown but self consistent",
			mutate: func(t *testing.T, db *gorm.DB, receiptID int64) {
				var receipt AccountQuotaMutationReceipt
				require.NoError(t, db.First(&receipt, receiptID).Error)
				receipt.FingerprintVersion = AccountQuotaFingerprintVersion + 1
				fingerprint, err := RecomputeAccountQuotaReceiptFingerprint(&receipt)
				require.NoError(t, err)
				require.NoError(t, db.Exec(
					"UPDATE account_quota_mutation_receipts SET fingerprint_version = ?, request_fingerprint = ? WHERE id = ?",
					receipt.FingerprintVersion, fingerprint, receiptID,
				).Error)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			db := openAccountQuotaTestDB(t)
			fixture := newAccountQuotaFixture(t, db, "execution-old-applied-authoritative-tampered", 1000, 500, false, 0, 0, false)
			reserve, err := ReserveAccountQuota(context.Background(), db, accountReserveInput(fixture, "execution-old-applied-authoritative-tampered", "wallet_only", 100))
			require.NoError(t, err)
			terminal, err := SettleAccountQuota(context.Background(), db, AccountQuotaTerminalInput{
				RequestID: reserve.RequestID, ReserveReceiptID: reserve.ID, ActualQuota: 80,
			})
			require.NoError(t, err)
			input := AccountQuotaSettlementFactInput{
				EventKey: "billing-settlement:execution-old-applied-authoritative-tampered:v2", RequestID: reserve.RequestID,
				Kind: AccountQuotaSettlementKindAuthoritative, UserID: reserve.UserID, TokenID: reserve.TokenID,
				SubscriptionID: reserve.SubscriptionID, ReserveReceiptID: reserve.ID, WriterEpoch: reserve.WriterEpoch, ActualQuota: 80,
			}
			fact := insertUnpermittedSettlementFact(t, db, input)
			fact = markUnpermittedSettlementFactAppliedForTest(t, db, fact, true, true, terminal.ID)
			test.mutate(t, db, terminal.ID)

			var receiptBefore AccountQuotaMutationReceipt
			require.NoError(t, db.First(&receiptBefore, terminal.ID).Error)
			factBefore := *fact
			userBefore, tokenBefore, _ := loadAccountBalances(t, db, fixture)

			stored, recoverErr := RecoverAccountQuotaSettlementFact(context.Background(), db, fact, "old-applied-authoritative-tampered")
			requireSettlementTerminalConflictForTest(t, recoverErr)
			require.NotNil(t, stored)
			assert.Equal(t, factBefore, *stored)
			var factAfter AccountQuotaSettlementFact
			require.NoError(t, db.First(&factAfter, fact.ID).Error)
			assert.Equal(t, factBefore, factAfter)
			var receiptAfter AccountQuotaMutationReceipt
			require.NoError(t, db.First(&receiptAfter, terminal.ID).Error)
			assert.Equal(t, receiptBefore, receiptAfter)
			var permitCount int64
			require.NoError(t, db.Model(&AccountQuotaExecutionPermit{}).Count(&permitCount).Error)
			assert.Zero(t, permitCount)
			userAfter, tokenAfter, _ := loadAccountBalances(t, db, fixture)
			assert.Equal(t, userBefore, userAfter)
			assert.Equal(t, tokenBefore, tokenAfter)
		})
	}
}

func TestOldIntentCannotMintPermitWhileMaterializingFact(t *testing.T) {
	db := openAccountQuotaTestDB(t)
	initializeAccountQuotaExecutionGateForTest(t, db)
	fixture := newAccountQuotaFixture(t, db, "execution-old-intent", 1000, 500, false, 0, 0, false)
	input := legacyWalletSettlementInput(fixture, "execution-old-intent", 20)
	normalized, fingerprint, err := normalizeAccountQuotaSettlementInput(input)
	require.NoError(t, err)
	now, err := taskRecoveryDBTimestamp(db)
	require.NoError(t, err)
	intent := &AccountQuotaSettlementIntent{
		EventKey: normalized.EventKey, RequestID: normalized.RequestID, Kind: normalized.Kind,
		UserID: normalized.UserID, TokenID: normalized.TokenID, Delta: normalized.Delta, ApplyToken: normalized.ApplyToken,
		RequestFingerprint: fingerprint, State: AccountQuotaSettlementPending, LockVersion: 1, CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, db.Create(intent).Error)
	before := *intent

	stored, fact, err := RecoverAccountQuotaSettlementIntent(context.Background(), db, intent, "old-intent")
	require.ErrorIs(t, err, ErrAccountQuotaExecutionFrozen)
	require.Nil(t, fact)
	assert.Equal(t, before, *stored)
	var factCount, permitCount int64
	require.NoError(t, db.Model(&AccountQuotaSettlementFact{}).Count(&factCount).Error)
	require.NoError(t, db.Model(&AccountQuotaExecutionPermit{}).Count(&permitCount).Error)
	assert.Zero(t, factCount)
	assert.Zero(t, permitCount)
}

func TestHistoricalPermitBindsCompleteFactAndEachEdgeRevalidates(t *testing.T) {
	db := openAccountQuotaTestDB(t)
	initializeAccountQuotaExecutionGateForTest(t, db)
	fixture := newAccountQuotaFixture(t, db, "execution-historical", 1000, 500, false, 0, 0, false)
	input := legacyWalletSettlementInput(fixture, "execution-historical", 40)
	fact := insertUnpermittedSettlementFact(t, db, input)

	permit, err := PermitHistoricalAccountQuotaSettlement(context.Background(), db, HistoricalAccountQuotaExecutionPermitInput{
		FactID: fact.ID, Settlement: input, RequestFingerprint: fact.RequestFingerprint,
		InitialFundingApplied: false, InitialTokenApplied: false, AllowFunding: true, AllowToken: true,
		ApprovalID: "review-batch-synthetic", EvidenceDigest: strings.Repeat("a", 64),
	})
	require.NoError(t, err)
	require.NotNil(t, permit)

	accountQuotaSettlementBeforeTokenHook = func(*AccountQuotaSettlementFact) error {
		return db.Model(&AccountQuotaExecutionPermit{}).Where("id = ?", permit.ID).
			Update("state", AccountQuotaExecutionPermitStateRevoked).Error
	}
	t.Cleanup(func() { accountQuotaSettlementBeforeTokenHook = nil })
	stored, err := RecoverAccountQuotaSettlementFact(context.Background(), db, fact, "historical-edge-check")
	require.ErrorIs(t, err, ErrAccountQuotaExecutionFrozen)
	assert.True(t, stored.FundingApplied)
	assert.False(t, stored.TokenApplied)
	user, token, _ := loadAccountBalances(t, db, fixture)
	assert.Equal(t, int64(960), user.Quota)
	assert.Equal(t, int64(500), token.RemainQuota)
	assert.Zero(t, token.UsedQuota)

	accountQuotaSettlementBeforeTokenHook = nil
	require.NoError(t, db.Model(&AccountQuotaExecutionPermit{}).Where("id = ?", permit.ID).
		Update("state", AccountQuotaExecutionPermitStateActive).Error)
	processed, err := RunAccountQuotaSettlementFacts(context.Background(), db, "historical-edge-restart", 10)
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	user, token, _ = loadAccountBalances(t, db, fixture)
	assert.Equal(t, int64(960), user.Quota)
	assert.Equal(t, int64(460), token.RemainQuota)
	assert.Equal(t, int64(40), token.UsedQuota)
}

func TestAuthoritativePermitIsRevalidatedInsideTerminalTransaction(t *testing.T) {
	db := openAccountQuotaTestDB(t)
	initializeAccountQuotaExecutionGateForTest(t, db)
	fixture := newAccountQuotaFixture(t, db, "execution-authoritative", 1000, 500, false, 0, 0, false)
	reserve, err := ReserveAccountQuota(context.Background(), db, accountReserveInput(fixture, "execution-authoritative", "wallet_only", 100))
	require.NoError(t, err)
	input := AccountQuotaSettlementFactInput{
		EventKey: "billing-settlement:execution-authoritative:v2", RequestID: reserve.RequestID,
		Kind: AccountQuotaSettlementKindAuthoritative, UserID: reserve.UserID, TokenID: reserve.TokenID,
		ReserveReceiptID: reserve.ID, WriterEpoch: reserve.WriterEpoch, ActualQuota: 80,
	}
	fact, err := EnsureAccountQuotaSettlementFact(context.Background(), db, input)
	require.NoError(t, err)
	accountQuotaSettlementBeforeTerminalApplyHook = func(*AccountQuotaSettlementFact) error {
		return db.Model(&AccountQuotaExecutionPermit{}).Where("fact_id = ?", fact.ID).
			Update("state", AccountQuotaExecutionPermitStateRevoked).Error
	}
	t.Cleanup(func() { accountQuotaSettlementBeforeTerminalApplyHook = nil })

	_, err = RecoverAccountQuotaSettlementFact(context.Background(), db, fact, "authoritative-guard")
	require.ErrorIs(t, err, ErrAccountQuotaExecutionFrozen)
	user, token, _ := loadAccountBalances(t, db, fixture)
	assert.Equal(t, int64(900), user.Quota)
	assert.Equal(t, int64(400), token.RemainQuota)
	assert.Equal(t, int64(100), token.UsedQuota)
	var terminalCount int64
	require.NoError(t, db.Model(&AccountQuotaMutationReceipt{}).
		Where("request_id = ? AND phase = ?", reserve.RequestID, AccountQuotaPhaseSettle).Count(&terminalCount).Error)
	assert.Zero(t, terminalCount)
}

func TestRuntimePermitIsCreatedOnceWithIntent(t *testing.T) {
	dbFixture, fixture, _ := openLegacyBalanceBatchTestWithRedis(t, "execution-runtime-intent")
	db := dbFixture.DB
	initializeAccountQuotaExecutionGateForTest(t, db)
	input := legacyWalletSettlementInput(fixture, "execution-runtime-intent", 30)

	intent, err := EnsureAccountQuotaSettlementIntent(context.Background(), db, input)
	require.NoError(t, err)
	replayed, err := EnsureAccountQuotaSettlementIntent(context.Background(), db, input)
	require.NoError(t, err)
	assert.Equal(t, intent.ID, replayed.ID)
	var permits []AccountQuotaExecutionPermit
	require.NoError(t, db.Find(&permits).Error)
	require.Len(t, permits, 1)
	assert.Equal(t, intent.ID, permits[0].IntentID)
	assert.Zero(t, permits[0].FactID)

	processed, err := RunAccountQuotaSettlementIntents(context.Background(), db, "runtime-intent", 10)
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	require.NoError(t, db.Find(&permits).Error)
	require.Len(t, permits, 1)
	assert.Positive(t, permits[0].FactID)
	assert.Equal(t, permits[0].BindingDigest, permits[0].EvidenceDigest)
	user, token, _ := loadAccountBalances(t, db, fixture)
	assert.Equal(t, int64(970), user.Quota)
	assert.Equal(t, int64(470), token.RemainQuota)
	assert.Equal(t, int64(30), token.UsedQuota)

	processed, err = RunAccountQuotaSettlementIntents(context.Background(), db, "runtime-intent-replay", 10)
	require.NoError(t, err)
	assert.Zero(t, processed)
	_, _, err = RecoverAccountQuotaSettlementIntent(context.Background(), db, replayed, "runtime-intent-direct-replay")
	require.NoError(t, err)
	user, token, _ = loadAccountBalances(t, db, fixture)
	assert.Equal(t, int64(970), user.Quota)
	assert.Equal(t, int64(470), token.RemainQuota)
	assert.Equal(t, int64(30), token.UsedQuota)
}

func TestHistoricalPermitRejectsIdentityMismatchWithoutInsertion(t *testing.T) {
	db := openAccountQuotaTestDB(t)
	initializeAccountQuotaExecutionGateForTest(t, db)
	fixture := newAccountQuotaFixture(t, db, "execution-history-mismatch", 1000, 500, false, 0, 0, false)
	input := legacyWalletSettlementInput(fixture, "execution-history-mismatch", 15)
	fact := insertUnpermittedSettlementFact(t, db, input)
	wrong := input
	wrong.Delta++

	permit, err := PermitHistoricalAccountQuotaSettlement(context.Background(), db, HistoricalAccountQuotaExecutionPermitInput{
		FactID: fact.ID, Settlement: wrong, RequestFingerprint: fact.RequestFingerprint,
		AllowFunding: true, AllowToken: true, ApprovalID: "review-batch-synthetic", EvidenceDigest: strings.Repeat("b", 64),
	})
	require.Nil(t, permit)
	require.True(t, errors.Is(err, ErrAccountQuotaTerminalRecoveryConflict) || errors.Is(err, ErrAccountQuotaMutationInvalidInput))
	var count int64
	require.NoError(t, db.Model(&AccountQuotaExecutionPermit{}).Count(&count).Error)
	assert.Zero(t, count)
}
