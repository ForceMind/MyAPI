package model

import (
	"fmt"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createManualUnitsTopUp(t *testing.T, userID int, tradeNo, method, provider string, amount int64, money float64) TopUp {
	t.Helper()
	topUp := TopUp{
		UserId:          userID,
		Amount:          amount,
		Money:           money,
		TradeNo:         tradeNo,
		PaymentMethod:   method,
		PaymentProvider: provider,
		CreateTime:      common.GetTimestamp(),
		Status:          common.TopUpStatusPending,
	}
	require.NoError(t, DB.Create(&topUp).Error)
	return topUp
}

func createManualUnitsUser(t *testing.T, id, quota int) *User {
	t.Helper()
	user := &User{
		Id:       id,
		Username: fmt.Sprintf("manual-units-%d", id),
		AffCode:  fmt.Sprintf("manual-units-aff-%d", id),
		Status:   common.UserStatusEnabled,
		Quota:    quota,
	}
	require.NoError(t, DB.Create(user).Error)
	return user
}

func topUpLogCount(t *testing.T, userID int) int64 {
	t.Helper()
	var count int64
	require.NoError(t, DB.Model(&Log{}).Where("user_id = ? AND type = ?", userID, LogTypeTopup).Count(&count).Error)
	return count
}

func topUpLogTotalCount(t *testing.T) int64 {
	t.Helper()
	var count int64
	require.NoError(t, DB.Model(&Log{}).Where("type = ?", LogTypeTopup).Count(&count).Error)
	return count
}

func TestManualCompleteTopUpMatchesProviderSettlementUnits(t *testing.T) {
	oldQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 100
	t.Cleanup(func() { common.QuotaPerUnit = oldQuotaPerUnit })

	testCases := []struct {
		name     string
		method   string
		provider string
		amount   int64
		money    float64
		quota    int
		complete func(TopUp) error
	}{
		{
			name:     "stripe uses money times quota per unit",
			method:   PaymentMethodStripe,
			provider: PaymentProviderStripe,
			amount:   2,
			money:    12.34,
			quota:    1234,
			complete: func(topUp TopUp) error { return Recharge(topUp.TradeNo, "customer", "127.0.0.1") },
		},
		{
			name:     "creem amount is final quota",
			method:   PaymentMethodCreem,
			provider: PaymentProviderCreem,
			amount:   123456,
			money:    12.34,
			quota:    123456,
			complete: func(topUp TopUp) error {
				return RechargeCreem(topUp.TradeNo, "customer@example.test", "customer", "127.0.0.1")
			},
		},
		{
			name:     "epay uses amount times quota per unit",
			method:   "alipay",
			provider: PaymentProviderEpay,
			amount:   2,
			money:    12.34,
			quota:    200,
			complete: func(topUp TopUp) error {
				_, err := RechargeEpay(topUp.TradeNo, "alipay", "127.0.0.1")
				return err
			},
		},
		{
			name:     "waffo uses amount times quota per unit",
			method:   PaymentMethodWaffo,
			provider: PaymentProviderWaffo,
			amount:   2,
			money:    12.34,
			quota:    200,
			complete: func(topUp TopUp) error { return RechargeWaffo(topUp.TradeNo, "127.0.0.1") },
		},
		{
			name:     "waffo pancake uses amount times quota per unit",
			method:   PaymentMethodWaffoPancake,
			provider: PaymentProviderWaffoPancake,
			amount:   2,
			money:    12.34,
			quota:    200,
			complete: func(topUp TopUp) error { return RechargeWaffoPancake(topUp.TradeNo) },
		},
	}

	for index, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			truncateTables(t)
			callbackUser := createManualUnitsUser(t, 800+index*2, 0)
			manualUser := createManualUnitsUser(t, 801+index*2, 0)
			callbackOrder := createManualUnitsTopUp(t, callbackUser.Id, "callback-"+tc.provider, tc.method, tc.provider, tc.amount, tc.money)
			manualOrder := createManualUnitsTopUp(t, manualUser.Id, "manual-"+tc.provider, tc.method, tc.provider, tc.amount, tc.money)

			require.NoError(t, tc.complete(callbackOrder))
			require.NoError(t, ManualCompleteTopUp(manualOrder.TradeNo, "127.0.0.1"))
			assert.Equal(t, tc.quota, getUserQuotaForPaymentGuardTest(t, callbackUser.Id))
			assert.Equal(t, tc.quota, getUserQuotaForPaymentGuardTest(t, manualUser.Id))
			assert.Equal(t, common.TopUpStatusSuccess, getTopUpStatusForPaymentGuardTest(t, callbackOrder.TradeNo))
			assert.Equal(t, common.TopUpStatusSuccess, getTopUpStatusForPaymentGuardTest(t, manualOrder.TradeNo))
			assert.Equal(t, int64(1), topUpLogCount(t, manualUser.Id))

			logsBeforeRetry := topUpLogTotalCount(t)
			require.NoError(t, ManualCompleteTopUp(manualOrder.TradeNo, "127.0.0.1"))
			assert.Equal(t, tc.quota, getUserQuotaForPaymentGuardTest(t, manualUser.Id))
			assert.Equal(t, int64(1), topUpLogCount(t, manualUser.Id))
			assert.Equal(t, logsBeforeRetry, topUpLogTotalCount(t))
		})
	}
}

func TestManualCompleteTopUpRejectsInvalidCreemFinalQuota(t *testing.T) {
	oldQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 100
	t.Cleanup(func() { common.QuotaPerUnit = oldQuotaPerUnit })

	maxInt64 := int64(^uint64(0) >> 1)
	testCases := []struct {
		name         string
		amount       int64
		currentQuota int
	}{
		{name: "zero", amount: 0, currentQuota: 17},
		{name: "negative", amount: -1, currentQuota: 17},
		{name: "maximum quota is not a valid credit", amount: int64(common.MaxQuota), currentQuota: 17},
		{name: "int64 maximum is not a valid credit", amount: maxInt64, currentQuota: 17},
		{name: "wallet ceiling", amount: 100, currentQuota: common.MaxQuota - 100},
	}

	for index, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			truncateTables(t)
			user := createManualUnitsUser(t, 900+index, tc.currentQuota)
			order := createManualUnitsTopUp(t, user.Id, "creem-invalid-"+tc.name, PaymentMethodCreem, PaymentProviderCreem, tc.amount, 12.34)

			require.Error(t, ManualCompleteTopUp(order.TradeNo, "127.0.0.1"))
			assert.Equal(t, tc.currentQuota, getUserQuotaForPaymentGuardTest(t, user.Id))
			assert.Equal(t, common.TopUpStatusPending, getTopUpStatusForPaymentGuardTest(t, order.TradeNo))
			assert.Zero(t, topUpLogCount(t, user.Id))
		})
	}
}
