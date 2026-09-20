package model

import (
	"errors"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/setting"
	"github.com/ForceMind/MyAPI/setting/operation_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func userFundingPolicyTestDB(t *testing.T, mode operation_setting.UserFundingMode) (*gorm.DB, UserFundingStateSnapshot) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&Option{}, &User{}, &TopUp{}, &SubscriptionPlan{}, &SubscriptionOrder{}, &UserSubscription{}, &SubscriptionPreConsumeRecord{}, &Redemption{}, &Log{}))
	oldDB, oldLogDB, oldRedis := DB, LOG_DB, common.RedisEnabled
	oldMain, oldLog := common.MainDatabaseType(), common.LogDatabaseType()
	oldFunding := operation_setting.GetUserFundingSetting()
	common.OptionMapRWMutex.Lock()
	oldOptionMap := common.OptionMap
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()
	DB, LOG_DB, common.RedisEnabled = db, db, false
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	var state UserFundingStateSnapshot
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		var initErr error
		state, initErr = InitializeUserFundingStateTx(tx, mode)
		return initErr
	}))
	require.NoError(t, PublishUserFundingState(state))
	t.Cleanup(func() {
		DB, LOG_DB, common.RedisEnabled = oldDB, oldLogDB, oldRedis
		common.SetDatabaseTypes(oldMain, oldLog)
		common.OptionMapRWMutex.Lock()
		common.OptionMap = oldOptionMap
		common.OptionMapRWMutex.Unlock()
		require.NoError(t, operation_setting.PublishUserFundingSnapshot(oldFunding.Mode, oldFunding.Epoch))
		require.NoError(t, sqlDB.Close())
	})
	return db, state
}

func createFundingPolicyUser(t *testing.T, db *gorm.DB) User {
	t.Helper()
	user := User{Username: "funding-policy-user", AffCode: "funding-policy", Quota: 1000, Group: "default"}
	require.NoError(t, db.Create(&user).Error)
	return user
}

func TestUserFundingRetirementBarrierAndCutoff(t *testing.T) {
	db, enabled := userFundingPolicyTestDB(t, operation_setting.UserFundingModeEnabled)
	user := createFundingPolicyUser(t, db)
	plan := SubscriptionPlan{Title: "Before cutoff", PriceAmount: 1, Enabled: true, DurationUnit: SubscriptionDurationDay, DurationValue: 1, TotalAmount: 100}
	require.NoError(t, db.Create(&plan).Error)
	beforeTopUp := TopUp{UserId: user.Id, TradeNo: "before-topup", Amount: 1, Money: 1, PaymentProvider: PaymentProviderStripe, PaymentMethod: PaymentMethodStripe, Status: common.TopUpStatusPending}
	require.NoError(t, beforeTopUp.Insert(enabled.Epoch))
	beforeSubscription := SubscriptionOrder{UserId: user.Id, PlanId: plan.Id, TradeNo: "before-subscription", Money: 1, PaymentProvider: PaymentProviderStripe, PaymentMethod: PaymentMethodStripe, Status: common.TopUpStatusPending}
	require.NoError(t, beforeSubscription.Insert(enabled.Epoch))

	retirement, err := TransitionUserFundingMode(operation_setting.UserFundingModeRetirement)
	require.NoError(t, err)
	assert.Equal(t, enabled.Epoch, retirement.RetirementCutoffEpoch)
	assert.Equal(t, beforeTopUp.Id, retirement.RetirementTopUpIDCutoff)
	assert.Equal(t, beforeSubscription.Id, retirement.RetirementSubscriptionIDCutoff)

	after := TopUp{UserId: user.Id, TradeNo: "after-gate", Amount: 1, Money: 1, PaymentProvider: PaymentProviderStripe, Status: common.TopUpStatusPending}
	assert.ErrorIs(t, after.Insert(enabled.Epoch), ErrUserFundingUnavailable)

	topupDecision, err := DecideUserFundingWebhook(beforeTopUp.TradeNo, PaymentProviderStripe)
	require.NoError(t, err)
	assert.True(t, topupDecision.ShouldSettle())
	subscriptionDecision, err := DecideUserFundingWebhook(beforeSubscription.TradeNo, PaymentProviderStripe)
	require.NoError(t, err)
	assert.True(t, subscriptionDecision.ShouldSettle())

	postCutoffLegacy := TopUp{UserId: user.Id, TradeNo: "post-cutoff-legacy", Amount: 1, Money: 1, PaymentProvider: PaymentProviderStripe, Status: common.TopUpStatusPending, FundingEpoch: 0}
	require.NoError(t, db.Create(&postCutoffLegacy).Error)
	decision, err := DecideUserFundingWebhook(postCutoffLegacy.TradeNo, PaymentProviderStripe)
	require.NoError(t, err)
	assert.False(t, decision.ShouldSettle())
	postCutoffSubscription := SubscriptionOrder{
		UserId: user.Id, PlanId: plan.Id, TradeNo: "post-cutoff-subscription",
		Money: 1, PaymentProvider: PaymentProviderStripe,
		Status: common.TopUpStatusPending, FundingEpoch: 0,
	}
	require.NoError(t, db.Create(&postCutoffSubscription).Error)
	decision, err = DecideUserFundingWebhook(postCutoffSubscription.TradeNo, PaymentProviderStripe)
	require.NoError(t, err)
	assert.False(t, decision.ShouldSettle())
}

func TestUserFundingWebhookDecisionCannotSettleAfterModeSwitch(t *testing.T) {
	providers := []struct {
		name     string
		provider string
		method   string
		settle   func(TopUp, UserFundingWebhookDecision) error
	}{
		{"epay", PaymentProviderEpay, "alipay", func(order TopUp, decision UserFundingWebhookDecision) error {
			_, err := RechargeEpay(order.TradeNo, "alipay", "127.0.0.1", decision)
			return err
		}},
		{"stripe", PaymentProviderStripe, PaymentMethodStripe, func(order TopUp, decision UserFundingWebhookDecision) error {
			return Recharge(order.TradeNo, "customer", "127.0.0.1", decision)
		}},
		{"creem", PaymentProviderCreem, PaymentMethodCreem, func(order TopUp, decision UserFundingWebhookDecision) error {
			return RechargeCreem(order.TradeNo, "user@example.test", "User", "127.0.0.1", decision)
		}},
		{"waffo", PaymentProviderWaffo, PaymentMethodWaffo, func(order TopUp, decision UserFundingWebhookDecision) error {
			return RechargeWaffo(order.TradeNo, "127.0.0.1", decision)
		}},
		{"waffo-pancake", PaymentProviderWaffoPancake, PaymentMethodWaffoPancake, func(order TopUp, decision UserFundingWebhookDecision) error {
			return RechargeWaffoPancake(order.TradeNo, decision)
		}},
	}
	for _, tc := range providers {
		t.Run(tc.name, func(t *testing.T) {
			db, enabled := userFundingPolicyTestDB(t, operation_setting.UserFundingModeEnabled)
			user := createFundingPolicyUser(t, db)
			amount := int64(1)
			if tc.provider == PaymentProviderCreem {
				amount = 100
			}
			order := TopUp{UserId: user.Id, TradeNo: "switch-" + tc.name, Amount: amount, Money: 1, PaymentProvider: tc.provider, PaymentMethod: tc.method, Status: common.TopUpStatusPending}
			require.NoError(t, order.Insert(enabled.Epoch))
			decision, err := DecideUserFundingWebhook(order.TradeNo, tc.provider)
			require.NoError(t, err)
			require.True(t, decision.ShouldSettle())
			_, err = TransitionUserFundingMode(operation_setting.UserFundingModeDisabled)
			require.NoError(t, err)

			assert.ErrorIs(t, tc.settle(order, decision), ErrUserFundingWebhookAcknowledge)
			var reloaded TopUp
			require.NoError(t, db.First(&reloaded, order.Id).Error)
			assert.Equal(t, common.TopUpStatusPending, reloaded.Status)
			var reloadedUser User
			require.NoError(t, db.First(&reloadedUser, user.Id).Error)
			assert.Equal(t, 1000, reloadedUser.Quota)
		})
	}
}

func TestInvalidPersistedUserFundingModeFailsClosed(t *testing.T) {
	db, _ := userFundingPolicyTestDB(t, operation_setting.UserFundingModeEnabled)
	require.NoError(t, db.Where("key = ?", UserFundingStateOptionKey).Delete(&Option{}).Error)
	require.NoError(t, db.Model(&Option{}).Where("key = ?", operation_setting.UserFundingModeOptionKey).Update("value", "invalid").Error)
	require.NoError(t, operation_setting.PublishUserFundingSnapshot(operation_setting.UserFundingModeEnabled, 0))

	snapshot, err := GetUserFundingRequestSnapshot()
	require.NoError(t, err)
	assert.Equal(t, operation_setting.UserFundingModeDisabled, snapshot.State.Mode)
	assert.False(t, snapshot.State.Valid)
	assert.False(t, snapshot.Ready)

	loadOptionsFromDatabase()
	assert.Equal(t, operation_setting.UserFundingModeDisabled, operation_setting.GetUserFundingMode())
}

func TestInvalidPersistedModeProjectionFailsClosedWithValidStateRow(t *testing.T) {
	db, state := userFundingPolicyTestDB(t, operation_setting.UserFundingModeEnabled)
	require.NoError(t, db.Model(&Option{}).
		Where("key = ?", operation_setting.UserFundingModeOptionKey).
		Update("value", "invalid").Error)

	snapshot, err := GetUserFundingRequestSnapshot()
	require.NoError(t, err)
	assert.Equal(t, operation_setting.UserFundingModeDisabled, snapshot.State.Mode)
	assert.Equal(t, state.Epoch, snapshot.State.Epoch)
	assert.False(t, snapshot.State.Valid)
	assert.False(t, snapshot.Ready)
}

func TestPaymentFundingBulkRollbackKeepsStateAndRuntime(t *testing.T) {
	db, before := userFundingPolicyTestDB(t, operation_setting.UserFundingModeDisabled)
	for key, value := range map[string]string{
		string(setting.PaymentOptionComplianceConfirmed):    "true",
		string(setting.PaymentOptionComplianceTermsVersion): operation_setting.CurrentComplianceTermsVersion,
	} {
		require.NoError(t, db.Create(&Option{Key: key, Value: value}).Error)
	}
	injected := errors.New("injected payment funding write failure")
	writes := 0
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:payment-funding-failure", func(tx *gorm.DB) {
		if tx.Statement.Table == "options" {
			writes++
			if writes == 2 {
				tx.AddError(injected)
			}
		}
	}))
	_, err := UpdatePaymentFundingOptionsBulk(map[string]string{
		operation_setting.UserFundingModeOptionKey:   "enabled",
		string(setting.PaymentOptionStripeAPISecret): "sk_test_atomic",
	})
	assert.ErrorIs(t, err, injected)
	require.NoError(t, db.Callback().Update().Remove("test:payment-funding-failure"))

	after, err := GetUserFundingStateSnapshot()
	require.NoError(t, err)
	assert.Equal(t, before.Mode, after.Mode)
	assert.Equal(t, before.Epoch, after.Epoch)
	local := operation_setting.GetUserFundingSetting()
	assert.Equal(t, before.Mode, local.Mode)
	assert.Equal(t, before.Epoch, local.Epoch)
}

func TestPaymentFundingBulkPublishesConfigurationBeforeEnabledBarrier(t *testing.T) {
	db, before := userFundingPolicyTestDB(t, operation_setting.UserFundingModeDisabled)
	for key, value := range map[string]string{
		string(setting.PaymentOptionComplianceConfirmed):    "true",
		string(setting.PaymentOptionComplianceTermsVersion): operation_setting.CurrentComplianceTermsVersion,
	} {
		require.NoError(t, db.Create(&Option{Key: key, Value: value}).Error)
	}
	state, err := UpdatePaymentFundingOptionsBulk(map[string]string{
		operation_setting.UserFundingModeOptionKey:       "enabled",
		string(setting.PaymentOptionStripeAPISecret):     "sk_test_atomic",
		string(setting.PaymentOptionStripeWebhookSecret): "whsec_atomic",
		string(setting.PaymentOptionStripePriceID):       "price_atomic",
	})
	require.NoError(t, err)
	assert.Greater(t, state.Epoch, before.Epoch)
	assert.Equal(t, operation_setting.UserFundingModeEnabled, state.Mode)
	assert.Equal(t, "sk_test_atomic", setting.StripeApiSecret)
	local := operation_setting.GetUserFundingSetting()
	assert.Equal(t, state.Mode, local.Mode)
	assert.Equal(t, state.Epoch, local.Epoch)
	requestSnapshot, err := GetUserFundingRequestSnapshot()
	require.NoError(t, err)
	assert.True(t, requestSnapshot.Ready)
}

func TestPaymentFundingBulkRejectsEnableBeforeCompliance(t *testing.T) {
	_, before := userFundingPolicyTestDB(t, operation_setting.UserFundingModeDisabled)
	_, err := UpdatePaymentFundingOptionsBulk(map[string]string{
		operation_setting.UserFundingModeOptionKey: "enabled",
	})
	assert.ErrorIs(t, err, ErrUserFundingUnavailable)
	after, readErr := GetUserFundingStateSnapshot()
	require.NoError(t, readErr)
	assert.Equal(t, before.Mode, after.Mode)
	assert.Equal(t, before.Epoch, after.Epoch)
}

func TestFundingMutationTransactionsRejectEpochAfterRetirement(t *testing.T) {
	db, enabled := userFundingPolicyTestDB(t, operation_setting.UserFundingModeEnabled)
	user := createFundingPolicyUser(t, db)
	user.AffQuota = 500000
	require.NoError(t, db.Save(&user).Error)
	redemption := Redemption{Key: "funding-cutoff-code", Name: "cutoff", Quota: 100, Status: common.RedemptionCodeStatusEnabled}
	require.NoError(t, db.Create(&redemption).Error)
	plan := SubscriptionPlan{Title: "Cutoff plan", PriceAmount: 1, Enabled: true, AllowBalancePay: common.GetPointer(true), DurationUnit: SubscriptionDurationDay, DurationValue: 1, TotalAmount: 100}
	require.NoError(t, db.Create(&plan).Error)

	_, err := TransitionUserFundingMode(operation_setting.UserFundingModeRetirement)
	require.NoError(t, err)
	_, err = Redeem(redemption.Key, user.Id, enabled.Epoch)
	assert.Error(t, err)
	assert.ErrorIs(t, PurchaseSubscriptionWithBalance(user.Id, plan.Id, enabled.Epoch), ErrUserFundingUnavailable)
	assert.ErrorIs(t, user.TransferAffQuotaToQuota(500000, "funding-cutoff-transfer", enabled.Epoch), ErrUserFundingUnavailable)

	var reloaded User
	require.NoError(t, db.First(&reloaded, user.Id).Error)
	assert.Equal(t, 1000, reloaded.Quota)
	assert.Equal(t, 500000, reloaded.AffQuota)
	var reloadedCode Redemption
	require.NoError(t, db.First(&reloadedCode, redemption.Id).Error)
	assert.Equal(t, common.RedemptionCodeStatusEnabled, reloadedCode.Status)
	var subscriptionCount int64
	require.NoError(t, db.Model(&UserSubscription{}).Count(&subscriptionCount).Error)
	assert.Zero(t, subscriptionCount)
	var orderCount int64
	require.NoError(t, db.Model(&SubscriptionOrder{}).Count(&orderCount).Error)
	assert.Zero(t, orderCount)
}

func TestInvalidPersistedPaymentConfigurationKeepsFundingFailClosed(t *testing.T) {
	db, _ := userFundingPolicyTestDB(t, operation_setting.UserFundingModeEnabled)
	require.NoError(t, db.Create(&Option{Key: "PayMethods", Value: "not-json"}).Error)

	loadOptionsFromDatabase()
	local := operation_setting.GetUserFundingSetting()
	assert.Equal(t, operation_setting.UserFundingModeDisabled, local.Mode)
	snapshot, err := GetUserFundingRequestSnapshot()
	require.NoError(t, err)
	assert.Equal(t, operation_setting.UserFundingModeEnabled, snapshot.State.Mode)
	assert.False(t, snapshot.Ready)
}
