package model

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLegacyWalletTokenRefundFactIsIdempotentAndRecoversAfterLockContention(t *testing.T) {
	dbFixture, fixture := openLegacyBalanceBatchTest(t, "legacy-refund-fact")
	require.NoError(t, DecreaseUserQuota(fixture.User.Id, 100, false))
	require.NoError(t, DecreaseTokenQuota(fixture.Token.Id, fixture.Token.Key, 100))
	factInput := AccountQuotaRefundFactInput{
		EventKey: "billing-refund:legacy-refund-fact:v1", Kind: AccountQuotaRefundFactKindLegacyWallet,
		RequestID: "legacy-refund-fact", UserID: fixture.User.Id, TokenID: fixture.Token.Id,
		WalletQuota: 100, TokenQuota: 100,
	}
	fact, err := EnsureAccountQuotaRefundFact(context.Background(), dbFixture.DB, factInput)
	require.NoError(t, err)
	replayed, err := EnsureAccountQuotaRefundFact(context.Background(), dbFixture.DB, factInput)
	require.NoError(t, err)
	assert.Equal(t, fact.ID, replayed.ID)

	holder, err := acquireQuotaBalanceSubjectLock(BatchUpdateTypeUserQuota, fixture.User.Id)
	require.NoError(t, err)
	started := time.Now()
	_, _, err = RecoverAccountQuotaRefundFact(context.Background(), dbFixture.DB, fact, "legacy-first-worker")
	require.ErrorIs(t, err, ErrAccountQuotaRefundPending)
	require.ErrorIs(t, err, ErrQuotaBalanceSubjectBusy)
	assert.False(t, errors.Is(err, ErrQuotaBalanceMutationUnknown))
	assert.GreaterOrEqual(t, time.Since(started), 1900*time.Millisecond)
	releaseQuotaBalanceSubjectLock(holder)

	require.NoError(t, dbFixture.DB.First(fact, fact.ID).Error)
	assert.Equal(t, AccountQuotaRefundFactRetryable, fact.State)
	processed, err := RunAccountQuotaRefundFacts(context.Background(), dbFixture.DB, "legacy-restart-worker", 1)
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	require.NoError(t, dbFixture.DB.First(fact, fact.ID).Error)
	assert.Equal(t, AccountQuotaRefundFactApplied, fact.State)
	user, token, _ := loadAccountBalances(t, dbFixture.DB, fixture)
	assert.Equal(t, 1000, user.Quota)
	assert.Equal(t, 500, token.RemainQuota)
	assert.Zero(t, token.UsedQuota)

	processed, err = RunAccountQuotaRefundFacts(context.Background(), dbFixture.DB, "legacy-replay-worker", 1)
	require.NoError(t, err)
	assert.Zero(t, processed)
	user, token, _ = loadAccountBalances(t, dbFixture.DB, fixture)
	assert.Equal(t, 1000, user.Quota)
	assert.Equal(t, 500, token.RemainQuota)
	assert.Zero(t, token.UsedQuota)
}

func TestLegacySubscriptionTokenRefundFactIsAtomicIdempotentAndRestartSafe(t *testing.T) {
	dbFixture, fixture := openLegacyBalanceBatchTest(t, "legacy-subscription-refund")
	require.NoError(t, dbFixture.DB.AutoMigrate(&SubscriptionPreConsumeRecord{}))
	plan := SubscriptionPlan{Title: "legacy-refund-plan", Enabled: true, DurationUnit: SubscriptionDurationMonth, DurationValue: 1, QuotaResetPeriod: SubscriptionResetNever}
	require.NoError(t, dbFixture.DB.Create(&plan).Error)
	subscription := UserSubscription{
		UserId: fixture.User.Id, PlanId: plan.Id, AmountTotal: 1000, AmountUsed: 100, QuotaVersion: 0,
		StartTime: time.Now().Add(-time.Hour).Unix(), EndTime: time.Now().Add(time.Hour).Unix(), Status: "active",
	}
	require.NoError(t, dbFixture.DB.Create(&subscription).Error)
	record := SubscriptionPreConsumeRecord{
		RequestId: "legacy-subscription-refund", UserId: fixture.User.Id, UserSubscriptionId: subscription.Id,
		PreConsumed: 100, Status: "consumed",
	}
	require.NoError(t, dbFixture.DB.Create(&record).Error)
	require.NoError(t, DecreaseTokenQuota(fixture.Token.Id, fixture.Token.Key, 100))

	factInput := AccountQuotaRefundFactInput{
		EventKey: "billing-refund:legacy-subscription-refund:v2", Kind: AccountQuotaRefundFactKindLegacySubscription,
		RequestID: record.RequestId, UserID: fixture.User.Id, TokenID: fixture.Token.Id, SubscriptionID: subscription.Id,
		SubscriptionQuota: 100, TokenQuota: 100,
	}
	fact, err := EnsureAccountQuotaRefundFact(context.Background(), dbFixture.DB, factInput)
	require.NoError(t, err)
	accountQuotaLegacyRefundAfterCommitHook = func(string) error { return errors.New("commit response lost") }
	t.Cleanup(func() { accountQuotaLegacyRefundAfterCommitHook = nil })
	stored, _, err := RecoverAccountQuotaRefundFact(context.Background(), dbFixture.DB, fact, "legacy-subscription-worker")
	require.NoError(t, err)
	require.NotNil(t, stored)
	assert.Equal(t, AccountQuotaRefundFactApplied, stored.State)

	accountQuotaLegacyRefundAfterCommitHook = nil
	var restarted AccountQuotaRefundFact
	require.NoError(t, dbFixture.DB.Where("event_key = ?", factInput.EventKey).First(&restarted).Error)
	stored, _, err = RecoverAccountQuotaRefundFact(context.Background(), dbFixture.DB, &restarted, "legacy-subscription-restart")
	require.NoError(t, err)
	assert.Equal(t, AccountQuotaRefundFactApplied, stored.State)

	require.NoError(t, dbFixture.DB.First(&subscription, subscription.Id).Error)
	require.NoError(t, dbFixture.DB.First(&record, record.Id).Error)
	var token Token
	require.NoError(t, dbFixture.DB.First(&token, fixture.Token.Id).Error)
	assert.EqualValues(t, 0, subscription.AmountUsed)
	assert.Equal(t, "refunded", record.Status)
	assert.Equal(t, 500, token.RemainQuota)
	assert.Zero(t, token.UsedQuota)
}

func TestAccountQuotaRefundFactCommitUnknownAndReadbackFailureAreClassified(t *testing.T) {
	db := openAccountQuotaTestDB(t)
	fixture := newAccountQuotaFixture(t, db, "refund-fact-unknown", 1000, 500, false, 0, 0, false)
	input := AccountQuotaRefundFactInput{
		EventKey: "billing-refund:refund-fact-unknown:v2", Kind: AccountQuotaRefundFactKindLegacyWallet,
		RequestID: "refund-fact-unknown", UserID: fixture.User.Id, TokenID: fixture.Token.Id, TokenQuota: 10,
	}
	accountQuotaRefundFactAfterCreateHook = func(string) error { return errors.New("insert commit response lost") }
	t.Cleanup(func() {
		accountQuotaRefundFactAfterCreateHook = nil
		accountQuotaRefundFactReadbackHook = nil
	})
	fact, err := EnsureAccountQuotaRefundFact(context.Background(), db, input)
	require.NoError(t, err)
	require.NotNil(t, fact)

	second := input
	second.EventKey = "billing-refund:refund-fact-readback-failed:v2"
	second.RequestID = "refund-fact-readback-failed"
	accountQuotaRefundFactReadbackHook = func() error { return errors.New("readback unavailable") }
	fact, err = EnsureAccountQuotaRefundFact(context.Background(), db, second)
	require.Nil(t, fact)
	require.ErrorIs(t, err, ErrAccountQuotaRefundFactUnknown)
	require.ErrorIs(t, err, ErrAccountQuotaRefundManualRequired)
	var persisted AccountQuotaRefundFact
	require.NoError(t, db.Where("event_key = ?", second.EventKey).First(&persisted).Error)
	assert.Equal(t, AccountQuotaRefundFactPending, persisted.State)
}

func TestAccountQuotaRefundFactHighWatermarkPreventsNewRowsFromStarvingOldRetryable(t *testing.T) {
	dbFixture, fixture := openLegacyBalanceBatchTest(t, "refund-fact-fairness")
	require.NoError(t, DecreaseTokenQuota(fixture.Token.Id, fixture.Token.Key, 10))
	old, err := EnsureAccountQuotaRefundFact(context.Background(), dbFixture.DB, AccountQuotaRefundFactInput{
		EventKey: "billing-refund:old-retryable:v2", Kind: AccountQuotaRefundFactKindLegacyWallet,
		RequestID: "old-retryable", UserID: fixture.User.Id, TokenID: fixture.Token.Id, TokenQuota: 10,
	})
	require.NoError(t, err)
	require.NoError(t, dbFixture.DB.Model(old).Updates(map[string]any{
		"state": AccountQuotaRefundFactRetryable, "next_attempt_at": time.Now().Add(time.Hour).Unix(),
	}).Error)
	poison := func(key string) AccountQuotaRefundFact {
		return AccountQuotaRefundFact{
			EventKey: key, Kind: AccountQuotaRefundFactKindLegacyWallet, RequestID: key,
			UserID: 999999, TokenID: 999999, TokenQuota: 1, RequestFingerprint: fmt.Sprintf("%064d", 1),
			State: AccountQuotaRefundFactPending, LockVersion: 1, CreatedAt: time.Now().Unix(), UpdatedAt: time.Now().Unix(),
		}
	}
	newer := poison("newer-before-watermark")
	require.NoError(t, dbFixture.DB.Create(&newer).Error)
	_, firstErr := RunAccountQuotaRefundFacts(context.Background(), dbFixture.DB, "fairness-worker", 1)
	require.Error(t, firstErr)

	require.NoError(t, dbFixture.DB.Model(old).Updates(map[string]any{"next_attempt_at": int64(0)}).Error)
	for index := 0; index < 3; index++ {
		row := poison(fmt.Sprintf("continuous-%d", index))
		require.NoError(t, dbFixture.DB.Create(&row).Error)
	}
	processed, err := RunAccountQuotaRefundFacts(context.Background(), dbFixture.DB, "fairness-worker", 1)
	require.NoError(t, err)
	assert.Zero(t, processed, "the completed high-watermark cycle resets before admitting newer rows")
	row := poison("continuous-after-reset")
	require.NoError(t, dbFixture.DB.Create(&row).Error)
	processed, err = RunAccountQuotaRefundFacts(context.Background(), dbFixture.DB, "fairness-worker", 1)
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	require.NoError(t, dbFixture.DB.First(old, old.ID).Error)
	assert.Equal(t, AccountQuotaRefundFactApplied, old.State)
}
