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

// subscriptionWalletTestDB mirrors creditEdgeTestDB and additionally migrates
// the subscription order/plan tables used by balance-paid purchases.
func subscriptionWalletTestDB(t *testing.T, writerMode QuotaWriterMode, writerEpoch int64) (*gorm.DB, UserFundingStateSnapshot) {
	t.Helper()
	db, funding := creditEdgeTestDB(t, writerMode, writerEpoch)
	require.NoError(t, db.AutoMigrate(&SubscriptionPlan{}, &SubscriptionOrder{}, &UserSubscription{}))
	return db, funding
}

func setQuotaPerUnitForTest(t *testing.T, quotaPerUnit float64) {
	t.Helper()
	old := common.QuotaPerUnit
	common.QuotaPerUnit = quotaPerUnit
	t.Cleanup(func() { common.QuotaPerUnit = old })
}

func createSubscriptionBalancePlan(t *testing.T, db *gorm.DB, title string, price float64, total int64) *SubscriptionPlan {
	t.Helper()
	plan := &SubscriptionPlan{
		Title: title, PriceAmount: price, Currency: "USD",
		DurationUnit: SubscriptionDurationDay, DurationValue: 1,
		Enabled: true, TotalAmount: total,
	}
	plan.NormalizeDefaults()
	require.NoError(t, db.Create(plan).Error)
	return plan
}

func loadUserQuota(t *testing.T, db *gorm.DB, userId int) int {
	t.Helper()
	var user User
	require.NoError(t, db.First(&user, userId).Error)
	return user.Quota
}

func countSubscriptionOrders(t *testing.T, db *gorm.DB, userId int) int64 {
	t.Helper()
	var count int64
	require.NoError(t, db.Model(&SubscriptionOrder{}).Where("user_id = ?", userId).Count(&count).Error)
	return count
}

func latestSubscriptionOrder(t *testing.T, db *gorm.DB, userId int) SubscriptionOrder {
	t.Helper()
	var order SubscriptionOrder
	require.NoError(t, db.Where("user_id = ?", userId).Order("id desc").Limit(1).First(&order).Error)
	return order
}

func TestSubscriptionBalancePurchaseDualMode(t *testing.T) {
	for index, mode := range []QuotaWriterMode{QuotaWriterModeLegacy, QuotaWriterModeAuthoritative} {
		t.Run(string(mode), func(t *testing.T) {
			db, funding := subscriptionWalletTestDB(t, mode, int64(601+index))
			setQuotaPerUnitForTest(t, 100)
			user := createCreditEdgeUser(t, db, "sub-wallet-"+string(mode), 1000, 0)
			plan := createSubscriptionBalancePlan(t, db, "plan-"+string(mode), 1.0, 5000)

			require.NoError(t, PurchaseSubscriptionWithBalance(user.Id, plan.Id, funding.Epoch))
			assert.Equal(t, 900, loadUserQuota(t, db, user.Id))
			assert.EqualValues(t, 1, countSubscriptionOrders(t, db, user.Id))

			var subCount int64
			require.NoError(t, db.Model(&UserSubscription{}).Where("user_id = ? AND plan_id = ? AND status = ?", user.Id, plan.Id, "active").Count(&subCount).Error)
			assert.EqualValues(t, 1, subCount)

			order := latestSubscriptionOrder(t, db, user.Id)
			assert.Equal(t, PaymentMethodBalance, order.PaymentMethod)
			eventKey := fmt.Sprintf("subscription-wallet:%s", order.TradeNo)
			receipts := countUserQuotaReceipts(t, db, eventKey)
			if mode == QuotaWriterModeLegacy {
				assert.Empty(t, receipts, "legacy 模式不得写入 receipt")
			} else {
				require.Len(t, receipts, 1)
				assert.Equal(t, "subscription_wallet", receipts[0].MutationType)
				assert.Equal(t, "subscription_balance_purchase", receipts[0].ReasonCode)
				assert.EqualValues(t, -100, receipts[0].Delta)
				assert.Equal(t, 1000, receipts[0].QuotaBefore)
				assert.Equal(t, 900, receipts[0].QuotaAfter)

				// 同键重放：幂等，不产生二次扣减。
				var replayed bool
				err := db.Transaction(func(tx *gorm.DB) error {
					_, rp, err := mutateUserQuotaAuthoritative(tx, subscriptionWalletMutationInput(user.Id, plan, order.TradeNo, 100))
					replayed = rp
					return err
				})
				require.NoError(t, err)
				assert.True(t, replayed)
				assert.Equal(t, 900, loadUserQuota(t, db, user.Id))

				// 同键不同额：指纹冲突，显式报错且余额不变。
				err = db.Transaction(func(tx *gorm.DB) error {
					_, _, err := mutateUserQuotaAuthoritative(tx, subscriptionWalletMutationInput(user.Id, plan, order.TradeNo, 101))
					return err
				})
				require.ErrorIs(t, err, ErrUserQuotaMutationConflict)
				assert.Equal(t, 900, loadUserQuota(t, db, user.Id))
			}

			// 第二次购买生成新订单身份，再扣一次。
			require.NoError(t, PurchaseSubscriptionWithBalance(user.Id, plan.Id, funding.Epoch))
			assert.Equal(t, 800, loadUserQuota(t, db, user.Id))
			assert.EqualValues(t, 2, countSubscriptionOrders(t, db, user.Id))
		})
	}
}

func TestSubscriptionBalancePurchaseBridgeFailClosed(t *testing.T) {
	db, funding := subscriptionWalletTestDB(t, QuotaWriterModeBridge, 611)
	setQuotaPerUnitForTest(t, 100)
	user := createCreditEdgeUser(t, db, "sub-wallet-bridge", 1000, 0)
	plan := createSubscriptionBalancePlan(t, db, "plan-bridge", 1.0, 5000)

	err := PurchaseSubscriptionWithBalance(user.Id, plan.Id, funding.Epoch)
	require.ErrorIs(t, err, ErrDurableQuotaWriterModeDisabled)
	assert.Equal(t, 1000, loadUserQuota(t, db, user.Id))
	assert.Zero(t, countSubscriptionOrders(t, db, user.Id))
	var subCount int64
	require.NoError(t, db.Model(&UserSubscription{}).Where("user_id = ?", user.Id).Count(&subCount).Error)
	assert.Zero(t, subCount)
}

func TestSubscriptionBalancePurchaseInsufficientDualMode(t *testing.T) {
	for index, mode := range []QuotaWriterMode{QuotaWriterModeLegacy, QuotaWriterModeAuthoritative} {
		t.Run(string(mode), func(t *testing.T) {
			db, funding := subscriptionWalletTestDB(t, mode, int64(621+index))
			setQuotaPerUnitForTest(t, 100)
			user := createCreditEdgeUser(t, db, "sub-poor-"+string(mode), 50, 0)
			plan := createSubscriptionBalancePlan(t, db, "plan-poor-"+string(mode), 1.0, 5000)

			err := PurchaseSubscriptionWithBalance(user.Id, plan.Id, funding.Epoch)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "余额不足")
			assert.Equal(t, 50, loadUserQuota(t, db, user.Id))
			assert.Zero(t, countSubscriptionOrders(t, db, user.Id))
			var receiptCount int64
			require.NoError(t, db.Model(&UserQuotaMutationReceipt{}).Count(&receiptCount).Error)
			assert.Zero(t, receiptCount)
		})
	}
}

func TestSubscriptionBalancePurchaseLegacyIdempotentOrderIdentity(t *testing.T) {
	// legacy 模式保持迁移前语义：直写扣减、无 receipt；订单行与扣减同事务提交。
	db, funding := subscriptionWalletTestDB(t, QuotaWriterModeLegacy, 631)
	setQuotaPerUnitForTest(t, 100)
	user := createCreditEdgeUser(t, db, "sub-legacy-order", 1000, 0)
	plan := createSubscriptionBalancePlan(t, db, "plan-legacy-order", 1.0, 5000)

	require.NoError(t, PurchaseSubscriptionWithBalance(user.Id, plan.Id, funding.Epoch))
	order := latestSubscriptionOrder(t, db, user.Id)
	assert.NotEmpty(t, order.TradeNo)
	assert.Equal(t, common.TopUpStatusSuccess, order.Status)
	assert.Equal(t, "charged_quota=100", order.ProviderPayload)
	assert.Equal(t, 900, loadUserQuota(t, db, user.Id))
}

func TestSubscriptionBalancePurchaseAuthoritativeReceiptFailureRollsBackOrder(t *testing.T) {
	db, funding := subscriptionWalletTestDB(t, QuotaWriterModeAuthoritative, 641)
	setQuotaPerUnitForTest(t, 100)
	user := createCreditEdgeUser(t, db, "sub-receipt-fail", 1000, 0)
	plan := createSubscriptionBalancePlan(t, db, "plan-receipt-fail", 1.0, 5000)

	const callbackName = "test:subscription_wallet_receipt_failure"
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if _, ok := tx.Statement.Dest.(*UserQuotaMutationReceipt); ok {
			tx.AddError(errors.New("receipt unavailable"))
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Create().Remove(callbackName) })

	err := PurchaseSubscriptionWithBalance(user.Id, plan.Id, funding.Epoch)
	require.Error(t, err)
	assert.Equal(t, 1000, loadUserQuota(t, db, user.Id))
	assert.Zero(t, countSubscriptionOrders(t, db, user.Id))
}
