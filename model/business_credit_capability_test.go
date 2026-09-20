package model

import (
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/setting/operation_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func prepareBusinessCreditCapabilityFixture(t *testing.T) (topUpUser, redemptionUser, checkinUser *User, redemption *Redemption) {
	t.Helper()
	require.NoError(t, DB.AutoMigrate(&Checkin{}, &Redemption{}, &Option{}))
	configureFixedCheckinAward(t, 25)
	oldFunding := operation_setting.GetUserFundingSetting()
	var fundingState UserFundingStateSnapshot
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		var err error
		fundingState, err = InitializeUserFundingStateTx(tx, operation_setting.UserFundingModeEnabled)
		return err
	}))
	require.NoError(t, PublishUserFundingState(fundingState))
	t.Cleanup(func() {
		_ = operation_setting.PublishUserFundingSnapshot(oldFunding.Mode, oldFunding.Epoch)
		RefreshUserQuotaBusinessSchemaCapability(DB)
	})
	users := []*User{
		{Id: 2401, Username: "capability-topup", AffCode: "capability-topup-aff", Status: common.UserStatusEnabled},
		{Id: 2402, Username: "capability-redemption", AffCode: "capability-redemption-aff", Status: common.UserStatusEnabled},
		{Id: 2403, Username: "capability-checkin", AffCode: "capability-checkin-aff", Status: common.UserStatusEnabled},
	}
	for _, user := range users {
		require.NoError(t, DB.Create(user).Error)
	}
	order := &TopUp{UserId: users[0].Id, Amount: 1, Money: 1, TradeNo: "capability-topup-order", PaymentMethod: "alipay", PaymentProvider: PaymentProviderEpay, Status: common.TopUpStatusPending}
	require.NoError(t, DB.Create(order).Error)
	redemption = &Redemption{Name: "capability-redemption", Key: "90000000000000000000000000000001", Status: common.RedemptionCodeStatusEnabled, Quota: 25}
	require.NoError(t, DB.Create(redemption).Error)
	t.Cleanup(func() {
		DB.Unscoped().Where("id IN ?", []int{users[0].Id, users[1].Id, users[2].Id}).Delete(&User{})
		DB.Where("trade_no = ?", order.TradeNo).Delete(&TopUp{})
		DB.Unscoped().Where("id = ?", redemption.Id).Delete(&Redemption{})
		DB.Where("user_id = ?", users[2].Id).Delete(&Checkin{})
	})
	return users[0], users[1], users[2], redemption
}

func assertBusinessCreditCapabilityFailureLeavesZeroWrites(t *testing.T, topUpUser, redemptionUser, checkinUser *User, redemption *Redemption) {
	t.Helper()
	_, topUpErr := RechargeEpayTrusted("capability-topup-order", "alipay", "127.0.0.1")
	require.ErrorIs(t, topUpErr, ErrUserQuotaBusinessSchemaUnavailable)
	_, redeemErr := Redeem(redemption.Key, redemptionUser.Id)
	require.Error(t, redeemErr)
	require.True(t, errors.Is(redeemErr, ErrUserQuotaBusinessSchemaUnavailable))
	_, checkinErr := UserCheckin(checkinUser.Id)
	require.ErrorIs(t, checkinErr, ErrUserQuotaBusinessSchemaUnavailable)

	for _, user := range []*User{topUpUser, redemptionUser, checkinUser} {
		var stored User
		require.NoError(t, DB.First(&stored, user.Id).Error)
		assert.Zero(t, stored.Quota)
		assert.Zero(t, stored.QuotaVersion)
	}
	var order TopUp
	require.NoError(t, DB.Where("trade_no = ?", "capability-topup-order").First(&order).Error)
	assert.Equal(t, common.TopUpStatusPending, order.Status)
	require.NoError(t, DB.First(redemption, redemption.Id).Error)
	assert.Equal(t, common.RedemptionCodeStatusEnabled, redemption.Status)
	var checkinCount, receiptCount int64
	require.NoError(t, DB.Model(&Checkin{}).Where("user_id = ?", checkinUser.Id).Count(&checkinCount).Error)
	require.NoError(t, DB.Model(&UserQuotaMutationReceipt{}).Where("user_id IN ?", []int{topUpUser.Id, redemptionUser.Id, checkinUser.Id}).Count(&receiptCount).Error)
	assert.Zero(t, checkinCount)
	assert.Zero(t, receiptCount)
}

func TestBusinessCreditSchemaCatalogFailureFailsClosedWithoutWrites(t *testing.T) {
	topUpUser, redemptionUser, checkinUser, redemption := prepareBusinessCreditCapabilityFixture(t)
	originalCatalog := userQuotaBusinessSchemaCatalog
	userQuotaBusinessSchemaCatalog = func(*gorm.DB) ([]string, error) { return nil, errors.New("schema catalog unavailable") }
	t.Cleanup(func() { userQuotaBusinessSchemaCatalog = originalCatalog })
	require.ErrorIs(t, RefreshUserQuotaBusinessSchemaCapability(DB), ErrUserQuotaBusinessSchemaUnavailable)
	assertBusinessCreditCapabilityFailureLeavesZeroWrites(t, topUpUser, redemptionUser, checkinUser, redemption)
}

func TestBusinessCreditMissingPublishedCapabilityFailsClosedWithoutWrites(t *testing.T) {
	topUpUser, redemptionUser, checkinUser, redemption := prepareBusinessCreditCapabilityFixture(t)
	userQuotaBusinessSchemaCapability.Store(nil)
	assertBusinessCreditCapabilityFailureLeavesZeroWrites(t, topUpUser, redemptionUser, checkinUser, redemption)
}

func TestBusinessCreditPartialDurableSchemaFailsClosedWithoutWrites(t *testing.T) {
	topUpUser, redemptionUser, checkinUser, redemption := prepareBusinessCreditCapabilityFixture(t)
	originalCatalog := userQuotaBusinessSchemaCatalog
	userQuotaBusinessSchemaCatalog = func(*gorm.DB) ([]string, error) {
		return []string{"users", "user_quota_mutation_receipts", "quota_projection_obligations"}, nil
	}
	t.Cleanup(func() { userQuotaBusinessSchemaCatalog = originalCatalog })
	require.ErrorIs(t, RefreshUserQuotaBusinessSchemaCapability(DB), ErrUserQuotaBusinessSchemaUnavailable)
	assertBusinessCreditCapabilityFailureLeavesZeroWrites(t, topUpUser, redemptionUser, checkinUser, redemption)
}

func TestBusinessCreditExplicitLegacySchemaUsesLegacyMode(t *testing.T) {
	originalCatalog := userQuotaBusinessSchemaCatalog
	userQuotaBusinessSchemaCatalog = func(*gorm.DB) ([]string, error) {
		return []string{"users", "top_ups", "system_tasks", "system_task_locks", "task_recovery_identities"}, nil
	}
	t.Cleanup(func() {
		userQuotaBusinessSchemaCatalog = originalCatalog
		RefreshUserQuotaBusinessSchemaCapability(DB)
	})
	require.NoError(t, RefreshUserQuotaBusinessSchemaCapability(DB))
	mode, err := businessUserQuotaWriterMode(DB)
	require.NoError(t, err)
	assert.Equal(t, QuotaWriterModeLegacy, mode)
}

func TestBusinessCreditCapabilityBindsPoolAcrossInterleavedRefreshAndDBSwitch(t *testing.T) {
	productionDB, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "production.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	legacyDB, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "legacy.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	foreignReadyDB, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "foreign-ready.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, productionDB.AutoMigrate(&QuotaWriterEpoch{}, &QuotaWorkCursor{}))
	require.NoError(t, foreignReadyDB.AutoMigrate(&QuotaWriterEpoch{}, &QuotaWorkCursor{}))
	previousDB := DB
	DB = productionDB
	t.Cleanup(func() {
		DB = previousDB
		userQuotaBusinessBeforePublishHook = nil
		if previousDB != nil {
			_ = RefreshUserQuotaBusinessSchemaCapability(previousDB)
		}
	})
	require.NoError(t, EnsureQuotaWriterEpochStateWithDB(productionDB))
	initial := userQuotaBusinessSchemaCapability.Load()
	require.NotNil(t, initial)
	productionPool, err := productionDB.DB()
	require.NoError(t, err)
	assert.Equal(t, productionPool, initial.pool)
	assert.NotZero(t, initial.dbGeneration)
	assert.Equal(t, userQuotaBusinessSchemaReady, initial.state)

	require.ErrorIs(t, RefreshUserQuotaBusinessSchemaCapability(legacyDB), ErrUserQuotaBusinessSchemaUnavailable)
	afterForeignRefresh := userQuotaBusinessSchemaCapability.Load()
	assert.Equal(t, initial, afterForeignRefresh, "foreign DB refresh must not replace the production snapshot")
	require.NoError(t, EnsureQuotaWriterEpochStateWithDB(foreignReadyDB))
	assert.Equal(t, initial, userQuotaBusinessSchemaCapability.Load(), "foreign DB ensure must not replace the production snapshot")

	const workers = 12
	type refreshResult struct {
		foreign bool
		err     error
	}
	results := make(chan refreshResult, workers)
	var wait sync.WaitGroup
	wait.Add(workers)
	for index := 0; index < workers; index++ {
		go func(index int) {
			defer wait.Done()
			if index%2 == 0 {
				results <- refreshResult{err: RefreshUserQuotaBusinessSchemaCapability(productionDB)}
				return
			}
			results <- refreshResult{foreign: true, err: RefreshUserQuotaBusinessSchemaCapability(legacyDB)}
		}(index)
	}
	wait.Wait()
	close(results)
	successfulProductionRefreshes := 0
	for result := range results {
		if !result.foreign {
			if result.err == nil {
				successfulProductionRefreshes++
			} else {
				require.ErrorIs(t, result.err, ErrUserQuotaBusinessSchemaUnavailable)
			}
		} else {
			require.ErrorIs(t, result.err, ErrUserQuotaBusinessSchemaUnavailable)
		}
	}
	assert.GreaterOrEqual(t, successfulProductionRefreshes, 1)
	current := userQuotaBusinessSchemaCapability.Load()
	require.NotNil(t, current)
	assert.Equal(t, productionPool, current.pool)
	assert.Equal(t, initial.dbGeneration, current.dbGeneration)
	assert.Greater(t, current.snapshotSequence, initial.snapshotSequence)

	DB = legacyDB
	_, err = businessUserQuotaWriterMode(legacyDB)
	require.ErrorIs(t, err, ErrUserQuotaBusinessSchemaUnavailable, "old production snapshot must be invalid after DB switch")
	require.NoError(t, RefreshUserQuotaBusinessSchemaCapability(legacyDB))
	mode, err := businessUserQuotaWriterMode(legacyDB)
	require.NoError(t, err)
	assert.Equal(t, QuotaWriterModeLegacy, mode)
}

func TestBusinessCreditSlowOldRefreshCannotOverwriteNewDBSnapshot(t *testing.T) {
	oldDB, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "old.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	newDB, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "new.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	for _, db := range []*gorm.DB{oldDB, newDB} {
		require.NoError(t, db.AutoMigrate(&User{}, &UserQuotaMutationReceipt{}, &QuotaWriterEpoch{}, &QuotaProjectionObligation{}, &QuotaWorkCursor{}))
	}
	previousDB := DB
	DB = oldDB
	t.Cleanup(func() {
		DB = previousDB
		if previousDB != nil {
			_ = RefreshUserQuotaBusinessSchemaCapability(previousDB)
		}
	})
	require.NoError(t, EnsureQuotaWriterEpochStateWithDB(oldDB))
	oldPool, err := oldDB.DB()
	require.NoError(t, err)
	entered := make(chan struct{})
	release := make(chan struct{})
	var blocked atomic.Bool
	userQuotaBusinessBeforePublishHook = func(expected *userQuotaBusinessPublishExpectation) {
		if expected.pool == oldPool && blocked.CompareAndSwap(false, true) {
			close(entered)
			<-release
		}
	}
	oldResult := make(chan error, 1)
	go func() { oldResult <- RefreshUserQuotaBusinessSchemaCapability(oldDB) }()
	<-entered
	DB = newDB
	require.NoError(t, EnsureQuotaWriterEpochStateWithDB(newDB))
	setQuotaWriterStateForTest(t, newDB, QuotaWriterModeAuthoritative, 902)
	require.NoError(t, RefreshUserQuotaBusinessSchemaCapability(newDB))
	newSnapshot := userQuotaBusinessSchemaCapability.Load()
	require.NotNil(t, newSnapshot)
	newPool, err := newDB.DB()
	require.NoError(t, err)
	assert.Equal(t, newPool, newSnapshot.pool)
	close(release)
	require.ErrorIs(t, <-oldResult, ErrUserQuotaBusinessSchemaUnavailable)
	assert.Equal(t, newSnapshot, userQuotaBusinessSchemaCapability.Load(), "late old refresh must not overwrite the new DB snapshot")

	user := &User{Username: "new-db-consumer", AffCode: "new-db-consumer-aff", Status: common.UserStatusEnabled}
	require.NoError(t, newDB.Create(user).Error)
	receipt, err := MutateUserQuota(newDB, UserQuotaMutationInput{
		UserID: user.Id, Delta: 40, MutationType: "checkin", BusinessEventKey: "checkin:new-db:2026-09-15", ReasonCode: "checkin_credit",
	})
	require.NoError(t, err)
	require.NotNil(t, receipt)
	require.NoError(t, newDB.First(user, user.Id).Error)
	assert.Equal(t, 40, user.Quota)
}

func TestBusinessCreditSlowOldEnsureCannotRebindAfterDBSwitch(t *testing.T) {
	oldDB, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "ensure-old.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	newDB, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "ensure-new.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	for _, db := range []*gorm.DB{oldDB, newDB} {
		require.NoError(t, db.AutoMigrate(&QuotaWriterEpoch{}, &QuotaWorkCursor{}))
	}
	previousDB := DB
	DB = oldDB
	t.Cleanup(func() {
		DB = previousDB
		userQuotaBusinessBeforePublishHook = nil
		if previousDB != nil {
			_ = RefreshUserQuotaBusinessSchemaCapability(previousDB)
		}
	})
	oldPool, err := oldDB.DB()
	require.NoError(t, err)
	entered := make(chan struct{})
	release := make(chan struct{})
	var blocked atomic.Bool
	userQuotaBusinessBeforePublishHook = func(expected *userQuotaBusinessPublishExpectation) {
		if expected.pool == oldPool && blocked.CompareAndSwap(false, true) {
			close(entered)
			<-release
		}
	}
	oldResult := make(chan error, 1)
	go func() { oldResult <- EnsureQuotaWriterEpochStateWithDB(oldDB) }()
	<-entered
	DB = newDB
	require.NoError(t, EnsureQuotaWriterEpochStateWithDB(newDB))
	newSnapshot := userQuotaBusinessSchemaCapability.Load()
	require.NotNil(t, newSnapshot)
	newPool, err := newDB.DB()
	require.NoError(t, err)
	assert.Equal(t, newPool, newSnapshot.pool)
	close(release)
	require.ErrorIs(t, <-oldResult, ErrUserQuotaBusinessSchemaUnavailable)
	assert.Equal(t, newSnapshot, userQuotaBusinessSchemaCapability.Load())
	assert.Equal(t, newPool, userQuotaBusinessSchemaBinding.Load().pool)
}

func TestBusinessCreditSamePoolCASPreventsOlderStateOverwrite(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "same-pool.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	previousDB := DB
	DB = db
	t.Cleanup(func() {
		DB = previousDB
		if previousDB != nil {
			_ = RefreshUserQuotaBusinessSchemaCapability(previousDB)
		}
	})
	first, err := captureUserQuotaBusinessPublishExpectation(db)
	require.NoError(t, err)
	second, err := captureUserQuotaBusinessPublishExpectation(db)
	require.NoError(t, err)
	require.Equal(t, first.previousSnapshot, second.previousSnapshot)
	require.NoError(t, publishUserQuotaBusinessSchemaCapability(first, userQuotaBusinessSchemaReady))
	published := userQuotaBusinessSchemaCapability.Load()
	require.NotNil(t, published)
	require.ErrorIs(t, publishUserQuotaBusinessSchemaCapability(second, userQuotaBusinessSchemaLegacy), ErrUserQuotaBusinessSchemaUnavailable)
	assert.Equal(t, published, userQuotaBusinessSchemaCapability.Load())
	assert.Equal(t, userQuotaBusinessSchemaReady, published.state)
}
