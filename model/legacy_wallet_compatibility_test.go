package model

import (
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/stretchr/testify/require"
)

func TestLegacyAffiliateTransferPreservesWideWalletAndRollsBackUnsafeTarget(t *testing.T) {
	for _, tc := range []struct {
		name   string
		before int64
		valid  bool
	}{
		{"wide state", 1<<32 + 17, true},
		{"upper bound", common.MaxQuotaBalance, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, funding := creditEdgeTestDB(t, QuotaWriterModeLegacy, 901)
			user := createCreditEdgeUser(t, db, "wide-aff-"+tc.name, 0, 1000000)
			require.NoError(t, db.Model(&User{}).Where("id = ?", user.Id).Update("quota", tc.before).Error)
			err := user.TransferAffQuotaToQuota(500000, "wide-aff-transfer", funding.Epoch)
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, common.ErrQuotaBalanceOutOfRange)
			}
			var got User
			require.NoError(t, db.First(&got, user.Id).Error)
			var transfers int64
			require.NoError(t, db.Model(&AffQuotaTransfer{}).Where("user_id = ?", user.Id).Count(&transfers).Error)
			if tc.valid {
				require.Equal(t, tc.before+500000, got.Quota)
				require.Equal(t, 500000, got.AffQuota)
				require.Equal(t, int64(1), got.QuotaVersion)
				require.Equal(t, int64(1), transfers)
			} else {
				require.Equal(t, tc.before, got.Quota)
				require.Equal(t, 1000000, got.AffQuota)
				require.Zero(t, got.QuotaVersion)
				require.Zero(t, transfers)
			}
		})
	}
}

func TestSubscriptionWideUsageAndOverRefundAreExact(t *testing.T) {
	db, _ := subscriptionWalletTestDB(t, QuotaWriterModeLegacy, 902)
	require.NoError(t, db.AutoMigrate(&SubscriptionPreConsumeRecord{}))
	user := createCreditEdgeUser(t, db, "wide-subscription", 0, 0)
	plan := createSubscriptionBalancePlan(t, db, "wide-plan", 1, 0)
	sub, err := CreateUserSubscriptionFromPlanTx(db, user.Id, plan, "admin")
	require.NoError(t, err)
	const wide int64 = 1<<32 + 17
	require.NoError(t, db.Model(sub).Update("amount_used", wide).Error)
	require.NoError(t, PostConsumeUserSubscriptionDelta(sub.Id, 7))
	require.NoError(t, db.First(sub, sub.Id).Error)
	require.Equal(t, wide+7, sub.AmountUsed)
	require.NoError(t, db.Model(sub).Update("amount_used", 2).Error)
	require.ErrorIs(t, PostConsumeUserSubscriptionDelta(sub.Id, -3), common.ErrQuotaBalanceOutOfRange)
	require.NoError(t, db.First(sub, sub.Id).Error)
	require.Equal(t, int64(2), sub.AmountUsed)
	require.NoError(t, db.Model(sub).Update("amount_used", common.MaxQuotaBalance).Error)
	_, err = PreConsumeUserSubscription("wide-subscription-preconsume", user.Id, "synthetic-model", 0, 1)
	require.ErrorIs(t, err, common.ErrQuotaBalanceOutOfRange)
	require.NoError(t, db.First(sub, sub.Id).Error)
	require.Equal(t, common.MaxQuotaBalance, sub.AmountUsed)
	var records int64
	require.NoError(t, db.Model(&SubscriptionPreConsumeRecord{}).Where("request_id = ?", "wide-subscription-preconsume").Count(&records).Error)
	require.Zero(t, records)
}
