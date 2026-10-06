package model

import (
	"context"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLegacyUsageReviewRetainsReservationAndRecoversOnce(t *testing.T) {
	f, account := setupLegacySettlementFactTest(t, "usage-review", false)
	db := f.DB
	require.NoError(t, db.AutoMigrate(&AccountQuotaSettlementIntent{}, &LegacyUsageReservation{}))
	// The existing reserve path has consumed 100; the journal does not repeat it.
	require.NoError(t, db.Model(&User{}).Where("id = ?", account.User.Id).Updates(map[string]interface{}{"quota": 900, "quota_version": 1}).Error)
	require.NoError(t, db.Model(&Token{}).Where("id = ?", account.Token.Id).Updates(map[string]interface{}{"remain_quota": 400, "used_quota": 100, "quota_version": 1}).Error)
	ctx := context.Background()
	row, err := PrepareLegacyUsageReservation(ctx, db, LegacyUsageReservation{RequestID: "legacy-review", UserID: account.User.Id, TokenID: account.Token.Id, FundingSource: "wallet", ReservedQuota: 100, TokenReservedQuota: 100})
	require.NoError(t, err)
	require.NoError(t, UpdateLegacyUsageReservation(ctx, db, row.RequestID, 100, 100, LegacyUsageUnknown, "missing", nil))
	_, err = EnsureAccountQuotaSettlementFact(ctx, db, AccountQuotaSettlementFactInput{EventKey: "billing-settlement:legacy-review:v1", RequestID: row.RequestID, Kind: AccountQuotaSettlementKindLegacyWallet, UserID: account.User.Id, TokenID: account.Token.Id, Delta: 20, ApplyToken: true})
	require.ErrorIs(t, err, ErrAccountQuotaUsageUnresolved)
	_, err = EnsureAccountQuotaRefundFact(ctx, db, AccountQuotaRefundFactInput{EventKey: "billing-refund:legacy-review:v2", RequestID: row.RequestID, Kind: AccountQuotaRefundFactKindLegacyWallet, UserID: account.User.Id, TokenID: account.Token.Id, WalletQuota: 100, TokenQuota: 100})
	require.ErrorIs(t, err, ErrAccountQuotaUsageUnresolved)
	_, err = ResolveLegacyUnknownUsage(ctx, db, account.User.Id, row.RequestID, 120, strings.Repeat("c", 64))
	require.ErrorIs(t, err, ErrAccountQuotaMutationIneligible)
	root := User{Username: "legacy-review-root", AffCode: "legacy-review-root", Status: common.UserStatusEnabled, Role: common.RoleRootUser}
	require.NoError(t, db.Create(&root).Error)
	resolved, err := ResolveLegacyUnknownUsage(ctx, db, root.Id, row.RequestID, 120, strings.Repeat("c", 64))
	require.NoError(t, err)
	assert.Equal(t, LegacyUsageSettled, resolved.State)
	require.NotNil(t, resolved.ActualQuota)
	assert.EqualValues(t, 120, *resolved.ActualQuota)
	_, err = ResolveLegacyUnknownUsage(ctx, db, root.Id, row.RequestID, 120, strings.Repeat("c", 64))
	require.NoError(t, err)
	_, err = ResolveLegacyUnknownUsage(ctx, db, root.Id, row.RequestID, 130, strings.Repeat("c", 64))
	require.ErrorIs(t, err, ErrAccountQuotaMutationConflict)
	user, token, _ := loadAccountBalances(t, db, account)
	assert.Equal(t, 880, user.Quota)
	assert.Equal(t, 380, token.RemainQuota)
	assert.Equal(t, 120, token.UsedQuota)
}
