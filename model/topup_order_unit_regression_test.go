package model

import (
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTopUpOrderKeepsQuotaAcrossUnitChange(t *testing.T) {
	db, enabled := userFundingPolicyTestDB(t, operation_setting.UserFundingModeEnabled)
	require.NoError(t, db.AutoMigrate(&UserQuotaMutationReceipt{}, &QuotaWriterEpoch{}, &QuotaProjectionObligation{}))
	require.NoError(t, RefreshUserQuotaBusinessSchemaCapability(db))
	setQuotaWriterStateForTest(t, db, QuotaWriterModeAuthoritative, 420)
	oldUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 100
	t.Cleanup(func() { common.QuotaPerUnit = oldUnit })
	user := createFundingPolicyUser(t, db)
	order := TopUp{UserId: user.Id, Amount: 2, Money: 2, TradeNo: "r1-frozen-order-unit", PaymentProvider: PaymentProviderEpay, PaymentMethod: "alipay", Status: common.TopUpStatusPending}
	require.NoError(t, order.Insert(enabled.Epoch))
	before := getUserQuotaForPaymentGuardTest(t, user.Id)
	common.QuotaPerUnit = 200
	_, err := RechargeEpayTrusted(order.TradeNo, "alipay", "127.0.0.1")
	require.NoError(t, err)
	assert.Equal(t, before+200, getUserQuotaForPaymentGuardTest(t, user.Id), "settlement must use the order's original unit, not the live configuration")
}

func TestTopUpLegacyPendingOrderDoesNotGuessQuotaUnit(t *testing.T) {
	db, _ := userFundingPolicyTestDB(t, operation_setting.UserFundingModeEnabled)
	require.NoError(t, db.AutoMigrate(&UserQuotaMutationReceipt{}, &QuotaWriterEpoch{}, &QuotaProjectionObligation{}))
	require.NoError(t, RefreshUserQuotaBusinessSchemaCapability(db))
	setQuotaWriterStateForTest(t, db, QuotaWriterModeAuthoritative, 421)
	oldUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 100
	t.Cleanup(func() { common.QuotaPerUnit = oldUnit })
	user := createFundingPolicyUser(t, db)
	order := TopUp{UserId: user.Id, Amount: 2, Money: 2, TradeNo: "r1-legacy-unresolved-unit", PaymentProvider: PaymentProviderEpay, PaymentMethod: "alipay", Status: common.TopUpStatusPending}
	require.NoError(t, db.Create(&order).Error)
	before := getUserQuotaForPaymentGuardTest(t, user.Id)
	_, err := RechargeEpayTrusted(order.TradeNo, "alipay", "127.0.0.1")
	assert.Error(t, err, "a historical pending order without a unit snapshot needs reconciliation")
	assert.Equal(t, before, getUserQuotaForPaymentGuardTest(t, user.Id))
	assert.Equal(t, common.TopUpStatusPending, getTopUpStatusForPaymentGuardTest(t, order.TradeNo))
}

func TestCompletedLegacyTopUpReplaysWithoutNewCreditOrReceipt(t *testing.T) {
	for _, tc := range []struct {
		name, provider string
		complete       func(string) error
	}{
		{"epay", PaymentProviderEpay, func(trade string) error { _, err := RechargeEpayTrusted(trade, "alipay", "127.0.0.1"); return err }},
		{"stripe", PaymentProviderStripe, func(trade string) error { return RechargeTrusted(trade, "synthetic", "127.0.0.1") }},
		{"waffo", PaymentProviderWaffo, func(trade string) error { return RechargeWaffoTrusted(trade, "127.0.0.1") }},
		{"pancake", PaymentProviderWaffoPancake, RechargeWaffoPancakeTrusted},
		{"manual", PaymentProviderEpay, func(trade string) error { return ManualCompleteTopUp(trade, "127.0.0.1") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, _ := userFundingPolicyTestDB(t, operation_setting.UserFundingModeEnabled)
			require.NoError(t, db.AutoMigrate(&UserQuotaMutationReceipt{}))
			user := createFundingPolicyUser(t, db)
			before := user.Quota
			order := TopUp{UserId: user.Id, TradeNo: "completed-legacy-" + tc.name, PaymentProvider: tc.provider, Amount: 2, Money: 2, Status: common.TopUpStatusSuccess, CompleteTime: 123}
			require.NoError(t, db.Create(&order).Error)
			require.NoError(t, tc.complete(order.TradeNo))
			require.NoError(t, tc.complete(order.TradeNo))
			require.NoError(t, db.First(&user, user.Id).Error)
			assert.Equal(t, before, user.Quota)
			require.NoError(t, db.First(&order, order.Id).Error)
			assert.Empty(t, order.QuotaPerUnitSnapshot)
			assert.EqualValues(t, 123, order.CompleteTime)
			var receipts int64
			require.NoError(t, db.Model(&UserQuotaMutationReceipt{}).Count(&receipts).Error)
			assert.Zero(t, receipts)
		})
	}
}
