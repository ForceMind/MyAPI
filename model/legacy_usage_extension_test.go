package model

import (
	"context"
	"fmt"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func verifyLegacyUsageExtension(t *testing.T, db *gorm.DB, prefix string) {
	t.Helper()
	state, err := GetQuotaWriterEpochState(db)
	require.NoError(t, err)
	setQuotaWriterStateForTest(t, db, QuotaWriterModeLegacy, state.Epoch+1)
	ctx := context.Background()
	for index, scenario := range []string{"wallet", "subscription", "journal-failure", "key-insufficient", "lost-commit-ack", "refund-pending"} {
		t.Run(scenario, func(t *testing.T) {
			id := fmt.Sprintf("%s%d", prefix, index)
			user := User{Username: id, AffCode: id, Status: common.UserStatusEnabled, Quota: 900}
			require.NoError(t, db.Create(&user).Error)
			token := Token{UserId: user.Id, Key: id, Status: common.TokenStatusEnabled, RemainQuota: 900, UsedQuota: 100, ExpiredTime: -1}
			if scenario == "key-insufficient" {
				token.RemainQuota = 50
			}
			require.NoError(t, db.Create(&token).Error)
			input := LegacyUsageReservation{RequestID: id, UserID: user.Id, TokenID: token.Id, FundingSource: "wallet", ReservedQuota: 100, TokenReservedQuota: 100}
			var sub UserSubscription
			if scenario == "subscription" {
				sub = UserSubscription{UserId: user.Id, AmountTotal: 1000, AmountUsed: 100}
				require.NoError(t, db.Create(&sub).Error)
				input.FundingSource, input.SubscriptionID = "subscription", sub.Id
				require.NoError(t, db.Create(&SubscriptionPreConsumeRecord{RequestId: id, UserId: user.Id, UserSubscriptionId: sub.Id, PreConsumed: 100, Status: "consumed"}).Error)
			}
			_, err := PrepareLegacyUsageReservation(ctx, db, input)
			require.NoError(t, err)
			if scenario == "journal-failure" {
				require.NoError(t, db.Callback().Update().Before("gorm:update").Register("legacy-extension-failure", func(tx *gorm.DB) {
					if tx.Statement.Schema != nil && tx.Statement.Schema.Table == "legacy_usage_reservations" {
						tx.AddError(ErrAccountQuotaMutationCASLost)
					}
				}))
			}
			if scenario == "refund-pending" {
				_, err := EnsureAccountQuotaRefundFact(ctx, db, AccountQuotaRefundFactInput{EventKey: "refund:" + id, Kind: AccountQuotaRefundFactKindLegacyWallet, RequestID: id, UserID: user.Id, TokenID: token.Id, WalletQuota: 100, TokenQuota: 100})
				require.NoError(t, err)
			}
			candidate := db
			if scenario == "lost-commit-ack" {
				pool, err := db.DB()
				require.NoError(t, err)
				candidate = db.Session(&gorm.Session{NewDB: true, Initialized: true})
				candidate.Statement.ConnPool = &taskQuotaCommitAcknowledgementPool{DB: pool}
			}
			row, err := ExtendLegacyUsageReservation(ctx, candidate, id, user.Id, token.Id, 200, true)
			failed := scenario == "journal-failure" || scenario == "key-insufficient" || scenario == "refund-pending"
			if scenario == "journal-failure" {
				require.NoError(t, db.Callback().Update().Remove("legacy-extension-failure"))
			}
			if failed {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotNil(t, row)
				assert.EqualValues(t, 200, row.ReservedQuota)
				_, err = ExtendLegacyUsageReservation(ctx, db, id, user.Id, token.Id, 200, true)
				require.NoError(t, err)
			}
			require.NoError(t, db.First(&user, user.Id).Error)
			require.NoError(t, db.First(&token, token.Id).Error)
			want := 800
			if failed || scenario == "subscription" {
				want = 900
			}
			assert.Equal(t, want, user.Quota)
			used := 200
			if failed {
				used = 100
			}
			assert.Equal(t, used, token.UsedQuota)
			remain := 1000 - used
			if scenario == "key-insufficient" {
				remain = 50
			}
			assert.Equal(t, remain, token.RemainQuota)
			row, err = FindLegacyUsageReservation(ctx, db, id)
			require.NoError(t, err)
			assert.EqualValues(t, used, row.ReservedQuota)
			assert.EqualValues(t, used, row.TokenReservedQuota)
			assert.Nil(t, row.ActualQuota)
			if scenario == "subscription" {
				require.NoError(t, db.First(&sub, sub.Id).Error)
				assert.EqualValues(t, 200, sub.AmountUsed)
			}
			require.NoError(t, UpdateLegacyUsageReservation(ctx, db, id, int64(used), int64(used), LegacyUsageUnknown, "missing", nil))
			_, err = ExtendLegacyUsageReservation(ctx, db, id, user.Id, token.Id, 300, true)
			require.ErrorIs(t, err, ErrAccountQuotaUsageUnresolved)
			require.NoError(t, db.First(&user, user.Id).Error)
			require.NoError(t, db.First(&token, token.Id).Error)
			assert.Equal(t, want, user.Quota)
			assert.Equal(t, used, token.UsedQuota)
		})
	}
}

func TestLegacyUsageExtensionAtomicity(t *testing.T) {
	db := openAccountQuotaTestDB(t)
	require.NoError(t, db.AutoMigrate(&LegacyUsageReservation{}, &SubscriptionPreConsumeRecord{}, &AccountQuotaSettlementIntent{}, &AccountQuotaSettlementFact{}))
	verifyLegacyUsageExtension(t, db, "extend-")
}

func TestLegacyUsageExtensionDrainsExistingBatchAndPreservesCache(t *testing.T) {
	f, account := openLegacyBalanceBatchTest(t, "extend-cache")
	db := f.DB
	require.NoError(t, db.AutoMigrate(&LegacyUsageReservation{}, &SubscriptionPreConsumeRecord{}, &AccountQuotaSettlementIntent{}, &AccountQuotaSettlementFact{}))
	require.NoError(t, DecreaseUserQuota(account.User.Id, 100, false))
	ok, err := TryReserveTokenQuota(account.Token.Id, account.Token.Key, 100, false)
	require.NoError(t, err)
	require.True(t, ok)
	_, err = PrepareLegacyUsageReservation(context.Background(), db, LegacyUsageReservation{RequestID: "extend-cache", UserID: account.User.Id, TokenID: account.Token.Id, FundingSource: "wallet", ReservedQuota: 100, TokenReservedQuota: 100})
	require.NoError(t, err)
	for range 2 {
		_, err = ExtendLegacyUsageReservation(context.Background(), db, "extend-cache", account.User.Id, account.Token.Id, 200, true)
		require.NoError(t, err)
	}
	require.NoError(t, drainQuotaBalanceGenerationsForSubjectWithDB(db, BatchUpdateTypeUserQuota, account.User.Id))
	require.NoError(t, drainQuotaBalanceGenerationsForSubjectWithDB(db, BatchUpdateTypeTokenQuota, account.Token.Id))
	require.NoError(t, db.First(&account.User, account.User.Id).Error)
	require.NoError(t, db.First(&account.Token, account.Token.Id).Error)
	assert.Equal(t, 800, account.User.Quota)
	assert.Equal(t, 300, account.Token.RemainQuota)
	assert.Equal(t, 200, account.Token.UsedQuota)
	user, err := GetUserCache(account.User.Id)
	require.NoError(t, err)
	token, err := GetTokenByKey(account.Token.Key, false)
	require.NoError(t, err)
	assert.Equal(t, 800, user.Quota)
	assert.Equal(t, 300, token.RemainQuota)
	assert.Equal(t, 200, token.UsedQuota)
}
