package model

import (
	"errors"
	"fmt"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
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

func TestAuthoritativeTopUpProvidersCreateStableReceiptAndReplay(t *testing.T) {
	truncateTables(t)
	oldQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 100
	t.Cleanup(func() { common.QuotaPerUnit = oldQuotaPerUnit })
	setQuotaWriterStateForTest(t, DB, QuotaWriterModeAuthoritative, 301)

	testCases := []struct {
		name     string
		provider string
		method   string
		amount   int64
		money    float64
		quota    int
		complete func(TopUp) error
	}{
		{"stripe", PaymentProviderStripe, PaymentMethodStripe, 2, 12.34, 1234, func(order TopUp) error { return RechargeTrusted(order.TradeNo, "customer", "127.0.0.1") }},
		{"creem", PaymentProviderCreem, PaymentMethodCreem, 321, 3.21, 321, func(order TopUp) error {
			return RechargeCreemTrusted(order.TradeNo, "user@example.test", "user", "127.0.0.1")
		}},
		{"epay", PaymentProviderEpay, "alipay", 2, 2, 200, func(order TopUp) error {
			_, err := RechargeEpayTrusted(order.TradeNo, "alipay", "127.0.0.1")
			return err
		}},
		{"waffo", PaymentProviderWaffo, PaymentMethodWaffo, 2, 2, 200, func(order TopUp) error { return RechargeWaffoTrusted(order.TradeNo, "127.0.0.1") }},
		{"waffo-pancake", PaymentProviderWaffoPancake, PaymentMethodWaffoPancake, 2, 2, 200, func(order TopUp) error { return RechargeWaffoPancakeTrusted(order.TradeNo) }},
	}
	for index, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			user := createManualUnitsUser(t, 2100+index, 0)
			order := createManualUnitsTopUp(t, user.Id, "authoritative-"+test.name, test.method, test.provider, test.amount, test.money)
			require.NoError(t, test.complete(order))
			require.NoError(t, test.complete(order))
			var reloaded User
			require.NoError(t, DB.First(&reloaded, user.Id).Error)
			assert.Equal(t, test.quota, reloaded.Quota)
			assert.EqualValues(t, 1, reloaded.QuotaVersion)
			eventKey, err := topUpBusinessEventKey(&order)
			require.NoError(t, err)
			var receipts []UserQuotaMutationReceipt
			require.NoError(t, DB.Where("business_event_key = ?", eventKey).Find(&receipts).Error)
			require.Len(t, receipts, 1)
			assert.Equal(t, "topup", receipts[0].MutationType)
			assert.EqualValues(t, test.quota, receipts[0].Delta)
			var projection QuotaProjectionObligation
			require.NoError(t, DB.Where("receipt_kind = ? AND receipt_id = ?", "user", receipts[0].ID).First(&projection).Error)
			assert.Equal(t, eventKey, projection.EventKey)
		})
	}
	manualUser := createManualUnitsUser(t, 2199, 0)
	manualOrder := createManualUnitsTopUp(t, manualUser.Id, "authoritative-manual", PaymentMethodCreem, PaymentProviderCreem, 275, 2.75)
	require.NoError(t, ManualCompleteTopUp(manualOrder.TradeNo, "127.0.0.1"))
	require.NoError(t, ManualCompleteTopUp(manualOrder.TradeNo, "127.0.0.1"))
	manualEventKey, err := topUpBusinessEventKey(&manualOrder)
	require.NoError(t, err)
	var manualReceipt UserQuotaMutationReceipt
	require.NoError(t, DB.Where("business_event_key = ?", manualEventKey).First(&manualReceipt).Error)
	assert.EqualValues(t, 275, manualReceipt.Delta)
}

func TestAuthoritativeTopUpChangedPayloadConflictsWithoutSecondCredit(t *testing.T) {
	truncateTables(t)
	oldQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 100
	t.Cleanup(func() { common.QuotaPerUnit = oldQuotaPerUnit })
	setQuotaWriterStateForTest(t, DB, QuotaWriterModeAuthoritative, 302)
	user := createManualUnitsUser(t, 2200, 0)
	order := createManualUnitsTopUp(t, user.Id, "authoritative-topup-conflict", "alipay", PaymentProviderEpay, 2, 2)
	_, err := RechargeEpayTrusted(order.TradeNo, "alipay", "127.0.0.1")
	require.NoError(t, err)
	require.NoError(t, DB.Model(&TopUp{}).Where("id = ?", order.Id).Update("amount", int64(3)).Error)
	_, err = RechargeEpayTrusted(order.TradeNo, "alipay", "127.0.0.1")
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrUserQuotaMutationConflict))
	var reloaded User
	require.NoError(t, DB.First(&reloaded, user.Id).Error)
	assert.Equal(t, 200, reloaded.Quota)
	var count int64
	require.NoError(t, DB.Model(&UserQuotaMutationReceipt{}).Where("user_id = ? AND mutation_type = ?", user.Id, "topup").Count(&count).Error)
	assert.EqualValues(t, 1, count)
}

func TestAuthoritativeTopUpPreservesStrictWalletCeiling(t *testing.T) {
	truncateTables(t)
	oldQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 100
	t.Cleanup(func() { common.QuotaPerUnit = oldQuotaPerUnit })
	setQuotaWriterStateForTest(t, DB, QuotaWriterModeAuthoritative, 303)
	user := createManualUnitsUser(t, 2201, common.MaxQuota-100)
	order := createManualUnitsTopUp(t, user.Id, "authoritative-wallet-ceiling", "alipay", PaymentProviderEpay, 1, 1)
	_, err := RechargeEpayTrusted(order.TradeNo, "alipay", "127.0.0.1")
	require.ErrorIs(t, err, ErrTopUpQuotaLimitExceeded)
	assert.Equal(t, common.TopUpStatusPending, getTopUpStatusForPaymentGuardTest(t, order.TradeNo))
	assert.Equal(t, common.MaxQuota-100, getUserQuotaForPaymentGuardTest(t, user.Id))
	var count int64
	require.NoError(t, DB.Model(&UserQuotaMutationReceipt{}).Where("user_id = ?", user.Id).Count(&count).Error)
	assert.Zero(t, count)
}

func TestAuthoritativeTopUpReceiptFailureRollsBackStatusAndQuota(t *testing.T) {
	truncateTables(t)
	oldQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 100
	t.Cleanup(func() { common.QuotaPerUnit = oldQuotaPerUnit })
	setQuotaWriterStateForTest(t, DB, QuotaWriterModeAuthoritative, 304)
	user := createManualUnitsUser(t, 2202, 0)
	order := createManualUnitsTopUp(t, user.Id, "authoritative-receipt-failure", "alipay", PaymentProviderEpay, 2, 2)
	const callbackName = "test:topup_receipt_failure"
	require.NoError(t, DB.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if _, ok := tx.Statement.Dest.(*UserQuotaMutationReceipt); ok {
			tx.AddError(errors.New("receipt unavailable"))
		}
	}))
	t.Cleanup(func() { _ = DB.Callback().Create().Remove(callbackName) })
	_, err := RechargeEpayTrusted(order.TradeNo, "alipay", "127.0.0.1")
	require.Error(t, err)
	assert.Equal(t, common.TopUpStatusPending, getTopUpStatusForPaymentGuardTest(t, order.TradeNo))
	assert.Zero(t, getUserQuotaForPaymentGuardTest(t, user.Id))
	var count int64
	require.NoError(t, DB.Model(&UserQuotaMutationReceipt{}).Where("user_id = ?", user.Id).Count(&count).Error)
	assert.Zero(t, count)
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
			complete: func(topUp TopUp) error { return RechargeTrusted(topUp.TradeNo, "customer", "127.0.0.1") },
		},
		{
			name:     "creem amount is final quota",
			method:   PaymentMethodCreem,
			provider: PaymentProviderCreem,
			amount:   123456,
			money:    12.34,
			quota:    123456,
			complete: func(topUp TopUp) error {
				return RechargeCreemTrusted(topUp.TradeNo, "customer@example.test", "customer", "127.0.0.1")
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
				_, err := RechargeEpayTrusted(topUp.TradeNo, "alipay", "127.0.0.1")
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
			complete: func(topUp TopUp) error { return RechargeWaffoTrusted(topUp.TradeNo, "127.0.0.1") },
		},
		{
			name:     "waffo pancake uses amount times quota per unit",
			method:   PaymentMethodWaffoPancake,
			provider: PaymentProviderWaffoPancake,
			amount:   2,
			money:    12.34,
			quota:    200,
			complete: func(topUp TopUp) error { return RechargeWaffoPancakeTrusted(topUp.TradeNo) },
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
