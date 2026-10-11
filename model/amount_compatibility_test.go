package model

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func createWideAccountFixture(t *testing.T, db *gorm.DB, label string) accountQuotaFixture {
	t.Helper()
	const wideQuota int64 = 1<<31 + 100
	name := fmt.Sprintf("wide-amount-%s", label)
	user := User{
		Username: name, AffCode: name, Password: "fixture-password",
		Status: common.UserStatusEnabled, Role: common.RoleCommonUser,
		Quota: wideQuota, AuthVersion: 1,
	}
	require.NoError(t, db.Create(&user).Error)
	token := Token{
		UserId: user.Id, Key: name, Status: common.TokenStatusEnabled,
		RemainQuota: wideQuota, UsedQuota: wideQuota, ExpiredTime: -1,
	}
	require.NoError(t, db.Create(&token).Error)
	return accountQuotaFixture{User: user, Token: token}
}

func TestWideAccountQuotaReserveRefundAndAtomicRollback(t *testing.T) {
	t.Run("small reserve and refund preserve wide state", func(t *testing.T) {
		db := openAccountQuotaTestDB(t)
		fixture := createWideAccountFixture(t, db, "round-trip")

		reserve, err := ReserveAccountQuota(context.Background(), db, accountReserveInput(fixture, "wide-round-trip", "wallet_only", 25))
		require.NoError(t, err)
		require.NotNil(t, reserve)
		user, token, _ := loadAccountBalances(t, db, fixture)
		require.Equal(t, int64(1<<31+75), user.Quota)
		require.Equal(t, int64(1<<31+75), token.RemainQuota)
		require.Equal(t, int64(1<<31+125), token.UsedQuota)
		require.Equal(t, user.Quota, reserve.After.User.Quota)
		require.Equal(t, token.RemainQuota, reserve.After.Token.RemainQuota)
		require.Equal(t, token.UsedQuota, reserve.After.Token.UsedQuota)

		refund, err := RefundAccountQuota(context.Background(), db, AccountQuotaTerminalInput{
			RequestID: reserve.RequestID, ReserveReceiptID: reserve.ID, AuditKey: "wide-round-trip-refund",
		})
		require.NoError(t, err)
		require.NotNil(t, refund)
		user, token, _ = loadAccountBalances(t, db, fixture)
		require.Equal(t, int64(1<<31+100), user.Quota)
		require.Equal(t, int64(1<<31+100), token.RemainQuota)
		require.Equal(t, int64(1<<31+100), token.UsedQuota)
	})

	t.Run("second balance leg failure rolls back the first", func(t *testing.T) {
		db := openAccountQuotaTestDB(t)
		fixture := createWideAccountFixture(t, db, "rollback")
		failure := errors.New("forced token balance failure")
		callbackName := "amount-compatibility-token-update-failure"
		require.NoError(t, db.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
			if tx.Statement.Table == "tokens" {
				tx.AddError(failure)
			}
		}))
		t.Cleanup(func() { _ = db.Callback().Update().Remove(callbackName) })

		_, err := ReserveAccountQuota(context.Background(), db, accountReserveInput(fixture, "wide-rollback", "wallet_only", 25))
		require.ErrorIs(t, err, failure)
		user, token, _ := loadAccountBalances(t, db, fixture)
		require.Equal(t, int64(1<<31+100), user.Quota)
		require.Equal(t, int64(1<<31+100), token.RemainQuota)
		require.Equal(t, int64(1<<31+100), token.UsedQuota)
		var receiptCount int64
		require.NoError(t, db.Model(&AccountQuotaMutationReceipt{}).Where("request_id = ?", "wide-rollback").Count(&receiptCount).Error)
		require.Zero(t, receiptCount)
	})
}

func TestUsageAccumulationCrossesInt32AndRejectsInvalidTargets(t *testing.T) {
	setupUserUpdateTestState(t)
	user := User{
		Id: 61001, Username: "wide-usage-user", Password: "fixture-password",
		Status: common.UserStatusEnabled, UsedQuota: 1<<31 - 1, RequestCount: 7,
	}
	channel := Channel{Id: 61001, Name: "wide-usage-channel", Status: common.ChannelStatusEnabled, UsedQuota: 1<<31 - 1}
	require.NoError(t, DB.Create(&user).Error)
	require.NoError(t, DB.Create(&channel).Error)

	UpdateUserUsedQuotaAndRequestCount(user.Id, 1)
	UpdateChannelUsedQuota(channel.Id, 1)
	require.NoError(t, DB.Select("used_quota", "request_count").First(&user, user.Id).Error)
	require.NoError(t, DB.Select("used_quota").First(&channel, channel.Id).Error)
	require.Equal(t, int64(1<<31), user.UsedQuota)
	require.Equal(t, 8, user.RequestCount)
	require.Equal(t, int64(1<<31), channel.UsedQuota)

	require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Updates(map[string]any{"used_quota": common.MaxQuotaBalance, "request_count": 8}).Error)
	require.NoError(t, DB.Model(&Channel{}).Where("id = ?", channel.Id).Update("used_quota", common.MaxQuotaBalance).Error)
	UpdateUserUsedQuotaAndRequestCount(user.Id, 1)
	UpdateChannelUsedQuota(channel.Id, 1)
	require.NoError(t, DB.Select("used_quota", "request_count").First(&user, user.Id).Error)
	require.NoError(t, DB.Select("used_quota").First(&channel, channel.Id).Error)
	require.Equal(t, common.MaxQuotaBalance, user.UsedQuota)
	require.Equal(t, 8, user.RequestCount)
	require.Equal(t, common.MaxQuotaBalance, channel.UsedQuota)

	require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Update("used_quota", 0).Error)
	require.NoError(t, DB.Model(&Channel{}).Where("id = ?", channel.Id).Update("used_quota", 0).Error)
	UpdateUserUsedQuota(user.Id, -1)
	UpdateChannelUsedQuota(channel.Id, -1)
	require.NoError(t, DB.Select("used_quota").First(&user, user.Id).Error)
	require.NoError(t, DB.Select("used_quota").First(&channel, channel.Id).Error)
	require.Zero(t, user.UsedQuota)
	require.Zero(t, channel.UsedQuota)
}

func TestTokenMetadataUpdateDoesNotRestoreStaleQuota(t *testing.T) {
	setupUserUpdateTestState(t)
	const wideQuota int64 = 1<<31 + 100
	user := User{Id: 62001, Username: "token-metadata-user", Password: "fixture-password", Status: common.UserStatusEnabled}
	require.NoError(t, DB.Create(&user).Error)
	token := Token{
		Id: 62001, UserId: user.Id, Key: "token-metadata-wide", Name: "before",
		Status: common.TokenStatusEnabled, RemainQuota: wideQuota, UsedQuota: wideQuota,
		QuotaVersion: 4, ExpiredTime: -1,
	}
	require.NoError(t, DB.Create(&token).Error)
	stale, err := GetTokenById(token.Id)
	require.NoError(t, err)
	require.NoError(t, DB.Model(&Token{}).Where("id = ?", token.Id).Updates(map[string]any{
		"remain_quota": wideQuota - 25, "used_quota": wideQuota + 25, "quota_version": 5,
	}).Error)

	stale.Name = "after"
	require.NoError(t, stale.Update())
	var got Token
	require.NoError(t, DB.First(&got, token.Id).Error)
	require.Equal(t, "after", got.Name)
	require.Equal(t, int64(wideQuota-25), got.RemainQuota)
	require.Equal(t, int64(wideQuota+25), got.UsedQuota)
	require.Equal(t, int64(5), got.QuotaVersion)
}

func TestTokenQuotaEditUsesVersionCASAndPreservesUsedQuota(t *testing.T) {
	setupUserUpdateTestState(t)
	setQuotaWriterStateForTest(t, DB, QuotaWriterModeLegacy, 31)
	const wideQuota int64 = 1<<31 + 100
	user := User{Id: 63001, Username: "token-quota-edit-user", Password: "fixture-password", Status: common.UserStatusEnabled}
	require.NoError(t, DB.Create(&user).Error)
	token := Token{
		Id: 63001, UserId: user.Id, Key: "token-quota-edit", Name: "before",
		Status: common.TokenStatusEnabled, RemainQuota: wideQuota, UsedQuota: wideQuota,
		QuotaVersion: 4, ExpiredTime: -1,
	}
	require.NoError(t, DB.Create(&token).Error)

	requested, err := GetTokenById(token.Id)
	require.NoError(t, err)
	stale := *requested
	metadataFingerprint, err := requested.QuotaMetadataFingerprint()
	require.NoError(t, err)
	requested.Name = "first edit"
	requested.RemainQuota = 500
	_, replayed, err := requested.UpdateWithQuota(newTokenQuotaAdjustmentCommandForTest(t, requested, 4, metadataFingerprint))
	require.NoError(t, err)
	require.False(t, replayed)
	require.Equal(t, int64(500), requested.RemainQuota)
	require.Equal(t, int64(wideQuota), requested.UsedQuota)
	require.Equal(t, int64(5), requested.QuotaVersion)

	stale.Name = "stale retry"
	stale.RemainQuota = 600
	_, _, err = stale.UpdateWithQuota(newTokenQuotaAdjustmentCommandForTest(t, &stale, 4, metadataFingerprint))
	require.ErrorIs(t, err, ErrTokenQuotaVersionConflict)
	var got Token
	require.NoError(t, DB.First(&got, token.Id).Error)
	require.Equal(t, "first edit", got.Name)
	require.Equal(t, int64(500), got.RemainQuota)
	require.Equal(t, int64(wideQuota), got.UsedQuota)
	require.Equal(t, int64(5), got.QuotaVersion)
}

func TestAdminAdjustmentPreservesWideBalanceAndRejectsOutOfRangeTarget(t *testing.T) {
	for index, mode := range []QuotaWriterMode{QuotaWriterModeLegacy, QuotaWriterModeAuthoritative} {
		t.Run(string(mode), func(t *testing.T) {
			db := adminAdjustmentTestDB(t, mode, int64(64001+index))
			const wideQuota int64 = 1<<31 + 100
			user := User{
				Username: fmt.Sprintf("wide-admin-%s", mode), AffCode: fmt.Sprintf("wide-admin-%s", mode),
				Password: "fixture-password", Status: common.UserStatusEnabled, Quota: wideQuota,
			}
			require.NoError(t, db.Create(&user).Error)

			adjustment, replayed, err := AdminAdjustUserQuota(1, user.Id, AdminQuotaAdjustmentOperationAdd, 25, "wide-admin-add")
			require.NoError(t, err)
			require.False(t, replayed)
			require.Equal(t, wideQuota, adjustment.AppliedQuotaBefore)
			require.Equal(t, wideQuota+25, adjustment.AppliedQuotaAfter)
			var stored User
			require.NoError(t, db.First(&stored, user.Id).Error)
			require.Equal(t, wideQuota+25, stored.Quota)
			if mode == QuotaWriterModeAuthoritative {
				receipts := countUserQuotaReceipts(t, db, fmt.Sprintf("admin-adjust:%d:wide-admin-add", user.Id))
				require.Len(t, receipts, 1)
				require.Equal(t, wideQuota, receipts[0].QuotaBefore)
				require.Equal(t, wideQuota+25, receipts[0].QuotaAfter)
			}

			require.NoError(t, db.Model(&User{}).Where("id = ?", user.Id).Update("quota", common.MaxQuotaBalance).Error)
			require.NoError(t, db.First(&stored, user.Id).Error)
			versionBefore := stored.QuotaVersion
			countBefore := countAdminAdjustments(t, db, user.Id)
			_, _, err = AdminAdjustUserQuota(1, user.Id, AdminQuotaAdjustmentOperationAdd, 1, "wide-admin-overflow")
			require.ErrorIs(t, err, common.ErrQuotaBalanceOutOfRange)
			require.NoError(t, db.First(&stored, user.Id).Error)
			require.Equal(t, common.MaxQuotaBalance, stored.Quota)
			require.Equal(t, versionBefore, stored.QuotaVersion)
			require.Equal(t, countBefore, countAdminAdjustments(t, db, user.Id))
		})
	}
}

func TestQuotaStateBoundaryFailuresWriteNothingAndRequestLimitStaysInt32(t *testing.T) {
	db := openAccountQuotaTestDB(t)
	name := "amount-boundary-zero-write"
	user := User{
		Username: name, AffCode: name, Password: "fixture-password",
		Status: common.UserStatusEnabled, Role: common.RoleCommonUser,
		Quota: common.MaxQuotaBalance, AuthVersion: 1,
	}
	require.NoError(t, db.Create(&user).Error)
	token := Token{
		UserId: user.Id, Key: name, Status: common.TokenStatusEnabled,
		RemainQuota: 100, UsedQuota: 0, ExpiredTime: -1,
	}
	require.NoError(t, db.Create(&token).Error)

	_, err := MutateUserQuotaAuthoritative(db, UserQuotaMutationInput{
		UserID: user.Id, Delta: 1, MutationType: "test_credit",
		BusinessEventKey: "amount-boundary-credit", ReasonCode: "test",
	})
	require.ErrorIs(t, err, common.ErrQuotaBalanceOutOfRange)
	_, err = MutateTokenQuotaAuthoritative(context.Background(), db, "amount-boundary-token-refund", user.Id, token.Id, 1)
	require.Error(t, err)
	_, err = ReserveAccountQuota(context.Background(), db, AccountQuotaReserveInput{
		RequestID: "amount-over-request-limit", UserID: user.Id, TokenID: token.Id,
		RequestedQuota: int64(common.MaxQuota) + 1, BillingPreference: "wallet_only",
		BillingContext: AccountBillingContext{Version: 1, OriginModelName: "fixture", BillingPreference: "wallet_only"},
	})
	require.ErrorIs(t, err, ErrAccountQuotaMutationInvalidInput)

	var gotUser User
	var gotToken Token
	require.NoError(t, db.First(&gotUser, user.Id).Error)
	require.NoError(t, db.First(&gotToken, token.Id).Error)
	require.Equal(t, common.MaxQuotaBalance, gotUser.Quota)
	require.Zero(t, gotUser.QuotaVersion)
	require.Equal(t, int64(100), gotToken.RemainQuota)
	require.Zero(t, gotToken.UsedQuota)
	require.Zero(t, gotToken.QuotaVersion)
	var userReceipts int64
	var accountReceipts int64
	require.NoError(t, db.Model(&UserQuotaMutationReceipt{}).Count(&userReceipts).Error)
	require.NoError(t, db.Model(&AccountQuotaMutationReceipt{}).Count(&accountReceipts).Error)
	require.Zero(t, userReceipts)
	require.Zero(t, accountReceipts)
}
