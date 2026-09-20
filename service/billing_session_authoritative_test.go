package service

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupAuthoritativeBillingDB(t *testing.T) *gorm.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "billing-authoritative.db")
	db, err := gorm.Open(sqlite.Open(path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(4)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.SubscriptionPlan{}, &model.UserSubscription{},
		&model.AccountQuotaMutationReceipt{}, &model.AccountQuotaReservationHead{}, &model.AccountQuotaTerminalRecoveryObligation{}, &model.AccountQuotaRefundFact{}, &model.AccountQuotaSettlementIntent{}, &model.AccountQuotaSettlementFact{}, &model.SystemTask{}, &model.SystemTaskLock{},
		&model.QuotaWriterEpoch{}, &model.QuotaProjectionObligation{}, &model.QuotaBalanceBatchDrain{}, &model.QuotaBalanceBatchSubject{}, &model.QuotaWorkCursor{}))
	require.True(t, model.RefreshAccountQuotaSettlementIntentSchemaCapability(db))
	require.NoError(t, model.EnsureQuotaWriterEpochStateWithDB(db))
	require.NoError(t, db.Model(&model.QuotaWriterEpoch{}).Where("id = ?", 1).Updates(map[string]interface{}{
		"mode": string(model.QuotaWriterModeAuthoritative), "epoch": int64(17), "lock_version": gorm.Expr("lock_version + ?", 1),
	}).Error)
	oldDB := model.DB
	oldRedis, oldRDB := common.RedisEnabled, common.RDB
	model.DB = db
	common.RedisEnabled, common.RDB = false, nil
	t.Cleanup(func() { model.DB = oldDB; common.RedisEnabled, common.RDB = oldRedis, oldRDB })
	return db
}

func seedAuthoritativeBilling(t *testing.T, db *gorm.DB, label string, userQuota, tokenQuota int, unlimited bool) (*model.User, *model.Token) {
	t.Helper()
	user := &model.User{Username: "auth-billing-" + label, AffCode: "auth-billing-" + label, Password: "fixture-password", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Quota: userQuota, AuthVersion: 1}
	require.NoError(t, db.Create(user).Error)
	token := &model.Token{UserId: user.Id, Key: "auth-token-" + label, Status: common.TokenStatusEnabled, RemainQuota: tokenQuota, UnlimitedQuota: unlimited, ExpiredTime: -1}
	require.NoError(t, db.Create(token).Error)
	return user, token
}

func authoritativeRelay(user *model.User, token *model.Token, requestID string) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{UserId: user.Id, TokenId: token.Id, TokenKey: token.Key, TokenUnlimited: token.UnlimitedQuota,
		RequestId: requestID, OriginModelName: "fixture-model", UserQuota: user.Quota,
		UserSetting: dto.UserSetting{BillingPreference: "wallet_only"}}
}

func TestAuthoritativeBillingSessionFullLifecycle(t *testing.T) {
	db := setupAuthoritativeBillingDB(t)
	user, token := seedAuthoritativeBilling(t, db, "lifecycle", 1000, 500, false)
	info := authoritativeRelay(user, token, "billing-lifecycle")
	session, apiErr := NewBillingSession(nil, info, 100)
	require.Nil(t, apiErr)
	require.NotNil(t, session.reserveReceipt)
	assert.Equal(t, 100, session.GetPreConsumedQuota())

	require.NoError(t, session.Reserve(140))
	assert.Equal(t, 140, session.GetPreConsumedQuota())
	require.NoError(t, session.Settle(80))
	require.NoError(t, session.Settle(80))

	require.NoError(t, db.First(user, user.Id).Error)
	require.NoError(t, db.First(token, token.Id).Error)
	assert.Equal(t, 920, user.Quota)
	assert.Equal(t, 420, token.RemainQuota)
	assert.Equal(t, 80, token.UsedQuota)
	var receipts []model.AccountQuotaMutationReceipt
	require.NoError(t, db.Where("request_id = ?", info.RequestId).Order("id ASC").Find(&receipts).Error)
	assert.Len(t, receipts, 3)
	assert.Equal(t, model.AccountQuotaPhaseSettle, receipts[2].Phase)
}

func TestAuthoritativeBillingSessionSettlementReplayRequiresSameActualQuota(t *testing.T) {
	db := setupAuthoritativeBillingDB(t)
	user, token := seedAuthoritativeBilling(t, db, "settlement-replay", 1000, 500, false)
	session, apiErr := NewBillingSession(nil, authoritativeRelay(user, token, "settlement-replay"), 100)
	require.Nil(t, apiErr)
	require.NoError(t, session.Settle(80))
	require.NoError(t, session.Settle(80))
	require.ErrorIs(t, session.Settle(81), model.ErrAccountQuotaMutationConflict)
	require.NoError(t, db.First(user, user.Id).Error)
	require.NoError(t, db.First(token, token.Id).Error)
	assert.Equal(t, 920, user.Quota)
	assert.Equal(t, 420, token.RemainQuota)
	assert.Equal(t, 80, token.UsedQuota)
}

func TestAuthoritativeBillingSessionConcurrentDifferentSettlementsConflict(t *testing.T) {
	db := setupAuthoritativeBillingDB(t)
	user, token := seedAuthoritativeBilling(t, db, "settlement-concurrent", 1000, 500, false)
	session, apiErr := NewBillingSession(nil, authoritativeRelay(user, token, "settlement-concurrent"), 100)
	require.Nil(t, apiErr)
	start := make(chan struct{})
	errs := make(chan error, 2)
	for _, actual := range []int{80, 90} {
		go func(actual int) {
			<-start
			errs <- session.Settle(actual)
		}(actual)
	}
	close(start)
	firstErr, secondErr := <-errs, <-errs
	close(errs)
	assert.True(t, firstErr == nil || secondErr == nil)
	assert.True(t, errors.Is(firstErr, model.ErrAccountQuotaMutationConflict) || errors.Is(secondErr, model.ErrAccountQuotaMutationConflict))

	require.NoError(t, db.First(user, user.Id).Error)
	require.NoError(t, db.First(token, token.Id).Error)
	assert.Contains(t, []int{910, 920}, user.Quota)
	assert.Equal(t, user.Quota-500, token.RemainQuota)
	assert.Equal(t, 1000-user.Quota, token.UsedQuota)
	var terminalCount int64
	require.NoError(t, db.Model(&model.AccountQuotaMutationReceipt{}).
		Where("request_id = ? AND phase = ?", "settlement-concurrent", model.AccountQuotaPhaseSettle).Count(&terminalCount).Error)
	assert.EqualValues(t, 1, terminalCount)
}

func TestSubscriptionSettlementLogDistinguishesPendingFromApplied(t *testing.T) {
	info := &relaycommon.RelayInfo{
		BillingSource: BillingSourceSubscription, SubscriptionId: 7,
		SubscriptionPreConsumed: 100, SubscriptionAmountTotal: 1000, SubscriptionAmountUsedAfterPreConsume: 100,
	}
	input := model.AccountQuotaSettlementFactInput{
		EventKey: "billing-settlement:log-state:v2", Kind: model.AccountQuotaSettlementKindAuthoritative, ActualQuota: 80,
	}
	intent := &model.AccountQuotaSettlementIntent{ID: 11, EventKey: input.EventKey, State: model.AccountQuotaSettlementMaterialized, FactID: 12}
	session := &BillingSession{relayInfo: info, settlementPending: true, settlementInput: &input, settlementIntent: intent}
	info.Billing = session
	pendingOther := map[string]interface{}{}
	appendBillingInfo(info, pendingOther)
	assert.NotContains(t, pendingOther, "subscription_post_delta")
	assert.EqualValues(t, 100, pendingOther["subscription_used"])
	pendingAdmin := pendingOther["admin_info"].(map[string]interface{})
	pendingAudit := pendingAdmin["billing_settlement"].(map[string]interface{})
	assert.Equal(t, model.AccountQuotaSettlementMaterialized, pendingAudit["state"])
	assert.Equal(t, input.EventKey, pendingAudit["event_key"])
	assert.EqualValues(t, -20, pendingAudit["intended_subscription_post_delta"])

	intent.State = model.AccountQuotaSettlementManual
	manualOther := map[string]interface{}{}
	appendBillingInfo(info, manualOther)
	manualAudit := manualOther["admin_info"].(map[string]interface{})["billing_settlement"].(map[string]interface{})
	assert.Equal(t, model.AccountQuotaSettlementManual, manualAudit["state"])
	assert.NotContains(t, manualOther, "subscription_post_delta")

	session.settled = true
	session.settlementPending = false
	info.SubscriptionPostDelta = -20
	appliedOther := map[string]interface{}{}
	appendBillingInfo(info, appliedOther)
	assert.EqualValues(t, -20, appliedOther["subscription_post_delta"])
	assert.EqualValues(t, 80, appliedOther["subscription_used"])
	appliedAudit := appliedOther["admin_info"].(map[string]interface{})["billing_settlement"].(map[string]interface{})
	assert.Equal(t, model.AccountQuotaSettlementApplied, appliedAudit["state"])
	assert.NotContains(t, appliedAudit, "intended_subscription_post_delta")
}

func TestBillingSessionSettleWithContextCancellationStillPersistsTerminalWrite(t *testing.T) {
	db := setupAuthoritativeBillingDB(t)
	user, token := seedAuthoritativeBilling(t, db, "settle-context-cancel", 1000, 500, false)
	info := authoritativeRelay(user, token, "settle-context-cancel")
	session, apiErr := NewBillingSession(nil, info, 100)
	require.Nil(t, apiErr)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.NoError(t, session.SettleWithContext(ctx, 80))
	assert.True(t, session.settled)
	var terminalCount int64
	require.NoError(t, db.Model(&model.AccountQuotaMutationReceipt{}).Where("request_id = ? AND phase = ?", info.RequestId, model.AccountQuotaPhaseSettle).Count(&terminalCount).Error)
	assert.EqualValues(t, 1, terminalCount)
	require.NoError(t, db.First(user, user.Id).Error)
	require.NoError(t, db.First(token, token.Id).Error)
	assert.Equal(t, 920, user.Quota)
	assert.Equal(t, 420, token.RemainQuota)
}

func TestAuthoritativeBillingSessionRefundIsSynchronousAndRetrySafe(t *testing.T) {
	db := setupAuthoritativeBillingDB(t)
	user, token := seedAuthoritativeBilling(t, db, "refund", 1000, 500, false)
	info := authoritativeRelay(user, token, "billing-refund")
	session, apiErr := NewBillingSession(nil, info, 100)
	require.Nil(t, apiErr)
	require.True(t, session.NeedsRefund())
	refundErr := session.Refund(nil)
	require.NoError(t, refundErr)
	require.NoError(t, session.Refund(nil))
	require.False(t, session.NeedsRefund())
	require.NoError(t, db.First(user, user.Id).Error)
	require.NoError(t, db.First(token, token.Id).Error)
	assert.Equal(t, 1000, user.Quota)
	assert.Equal(t, 500, token.RemainQuota)
	assert.Zero(t, token.UsedQuota)
	_, err := model.SettleAccountQuota(context.Background(), db, model.AccountQuotaTerminalInput{RequestID: info.RequestId, ReserveReceiptID: session.reserveReceipt.ID, ActualQuota: 50})
	require.ErrorIs(t, err, model.ErrAccountQuotaMutationTerminal)
}

func TestAuthoritativeBillingSessionTrustPlaygroundAndUnlimited(t *testing.T) {
	t.Run("trust", func(t *testing.T) {
		db := setupAuthoritativeBillingDB(t)
		trust := common.GetTrustQuota()
		user, token := seedAuthoritativeBilling(t, db, "trust", trust+1000, trust+1000, false)
		info := authoritativeRelay(user, token, "billing-trust")
		session, apiErr := NewBillingSession(nil, info, 100)
		require.Nil(t, apiErr)
		assert.Zero(t, session.GetPreConsumedQuota())
		require.NoError(t, session.Settle(60))
		require.NoError(t, db.First(user, user.Id).Error)
		require.NoError(t, db.First(token, token.Id).Error)
		assert.Equal(t, trust+940, user.Quota)
		assert.Equal(t, trust+940, token.RemainQuota)
	})

	t.Run("playground", func(t *testing.T) {
		db := setupAuthoritativeBillingDB(t)
		user, token := seedAuthoritativeBilling(t, db, "playground", 1000, 500, false)
		info := authoritativeRelay(user, token, "billing-playground")
		info.IsPlayground = true
		info.ForcePreConsume = true
		session, apiErr := NewBillingSession(nil, info, 100)
		require.Nil(t, apiErr)
		require.NoError(t, session.Settle(80))
		require.NoError(t, db.First(token, token.Id).Error)
		assert.Equal(t, 500, token.RemainQuota)
		assert.Zero(t, token.UsedQuota)
	})

	t.Run("unlimited", func(t *testing.T) {
		db := setupAuthoritativeBillingDB(t)
		user, token := seedAuthoritativeBilling(t, db, "unlimited", 1000, 0, true)
		info := authoritativeRelay(user, token, "billing-unlimited")
		info.ForcePreConsume = true
		session, apiErr := NewBillingSession(nil, info, 100)
		require.Nil(t, apiErr)
		require.NoError(t, session.Settle(100))
		require.NoError(t, db.First(token, token.Id).Error)
		assert.Equal(t, -100, token.RemainQuota)
		assert.Equal(t, 100, token.UsedQuota)
	})
}

func TestAuthoritativeBillingSessionBridgeRejectsAdmission(t *testing.T) {
	db := setupAuthoritativeBillingDB(t)
	user, token := seedAuthoritativeBilling(t, db, "bridge", 1000, 500, false)
	require.NoError(t, db.Model(&model.QuotaWriterEpoch{}).Where("id = ?", 1).Update("mode", string(model.QuotaWriterModeBridge)).Error)
	_, apiErr := NewBillingSession(nil, authoritativeRelay(user, token, "billing-bridge"), 100)
	require.NotNil(t, apiErr)
	require.ErrorIs(t, apiErr, model.ErrDurableQuotaWriterModeDisabled)
}

func TestAuthoritativeBillingSessionFailsClosedWithoutStartupSettlementCapability(t *testing.T) {
	db := setupAuthoritativeBillingDB(t)
	user, token := seedAuthoritativeBilling(t, db, "missing-intent-capability", 1000, 500, false)
	capabilityDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.False(t, model.RefreshAccountQuotaSettlementIntentSchemaCapability(capabilityDB))
	t.Cleanup(func() { model.RefreshAccountQuotaSettlementIntentSchemaCapability(db) })
	_, apiErr := NewBillingSession(nil, authoritativeRelay(user, token, "missing-intent-capability"), 100)
	require.NotNil(t, apiErr)
	require.ErrorIs(t, apiErr, model.ErrQuotaMaintenanceBackfillIncomplete)
	var reserveCount int64
	require.NoError(t, db.Model(&model.AccountQuotaMutationReceipt{}).Where("request_id = ?", "missing-intent-capability").Count(&reserveCount).Error)
	assert.Zero(t, reserveCount)
}

func TestAuthoritativeSubscriptionZeroReservation(t *testing.T) {
	db := setupAuthoritativeBillingDB(t)
	user, token := seedAuthoritativeBilling(t, db, "subscription-zero", 1000, 500, false)
	plan := &model.SubscriptionPlan{Title: "zero-plan", Enabled: true, DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1, QuotaResetPeriod: model.SubscriptionResetNever}
	require.NoError(t, db.Create(plan).Error)
	sub := &model.UserSubscription{UserId: user.Id, PlanId: plan.Id, AmountTotal: 1000, StartTime: time.Now().Add(-time.Hour).Unix(), EndTime: time.Now().Add(time.Hour).Unix(), Status: "active", AllowWalletOverflow: true}
	require.NoError(t, db.Create(sub).Error)
	info := authoritativeRelay(user, token, "billing-sub-zero")
	info.UserSetting.BillingPreference = "subscription_only"
	session, apiErr := NewBillingSession(nil, info, 0)
	require.Nil(t, apiErr)
	assert.Equal(t, 1, session.GetPreConsumedQuota())
	session.Refund(nil)
	require.NoError(t, db.First(sub, sub.Id).Error)
	assert.Zero(t, sub.AmountUsed)
}

func TestAuthoritativeBillingSessionFreeAndPaidZeroMatrix(t *testing.T) {
	for _, preference := range []string{"wallet_only", "subscription_only"} {
		for _, freeModel := range []bool{false, true} {
			for _, actual := range []int{0, 3} {
				name := fmt.Sprintf("%s-free-%t-actual-%d", preference, freeModel, actual)
				t.Run(name, func(t *testing.T) {
					db := setupAuthoritativeBillingDB(t)
					user, token := seedAuthoritativeBilling(t, db, name, 10, 10, false)
					var subscription *model.UserSubscription
					if preference == "subscription_only" {
						plan := &model.SubscriptionPlan{Title: name, Enabled: true, DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1, QuotaResetPeriod: model.SubscriptionResetNever}
						require.NoError(t, db.Create(plan).Error)
						subscription = &model.UserSubscription{UserId: user.Id, PlanId: plan.Id, AmountTotal: 10, StartTime: time.Now().Add(-time.Hour).Unix(), EndTime: time.Now().Add(time.Hour).Unix(), Status: "active"}
						require.NoError(t, db.Create(subscription).Error)
					}
					info := authoritativeRelay(user, token, "billing-"+name)
					info.UserSetting.BillingPreference = preference
					info.PriceData.FreeModel = freeModel
					session, apiErr := NewBillingSession(nil, info, 0)
					require.Nil(t, apiErr)
					if freeModel {
						assert.Zero(t, session.GetPreConsumedQuota())
						assert.Equal(t, BillingSourceFree, info.BillingSource)
					} else {
						assert.Equal(t, 1, session.GetPreConsumedQuota())
					}
					require.NoError(t, session.Settle(actual))
					require.NoError(t, db.First(user, user.Id).Error)
					require.NoError(t, db.First(token, token.Id).Error)
					charged := actual
					if freeModel {
						charged = 0
					}
					assert.Equal(t, 10-charged, token.RemainQuota)
					assert.Equal(t, charged, token.UsedQuota)
					if subscription != nil {
						require.NoError(t, db.First(subscription, subscription.Id).Error)
						assert.EqualValues(t, charged, subscription.AmountUsed)
						assert.Equal(t, 10, user.Quota)
					} else {
						assert.Equal(t, 10-charged, user.Quota)
					}
				})
			}
		}
	}
}

func TestAuthoritativeRefundIgnoresCancelledRequestContext(t *testing.T) {
	db := setupAuthoritativeBillingDB(t)
	user, token := seedAuthoritativeBilling(t, db, "cancelled-refund", 1000, 500, false)
	session, apiErr := NewBillingSession(nil, authoritativeRelay(user, token, "cancelled-refund"), 100)
	require.Nil(t, apiErr)
	requestCtx, cancelRequest := context.WithCancel(context.Background())
	cancelRequest()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil).WithContext(requestCtx)
	session.Refund(c)
	assert.True(t, session.refunded)
	require.NoError(t, db.First(user, user.Id).Error)
	require.NoError(t, db.First(token, token.Id).Error)
	assert.Equal(t, 1000, user.Quota)
	assert.Equal(t, 500, token.RemainQuota)
}

func TestBillingRecoveryEnqueueDetachesCancelledContext(t *testing.T) {
	setupAuthoritativeBillingDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.NoError(t, enqueueAccountQuotaSettlementRecovery(ctx))
	task, err := model.GetActiveSystemTask(model.SystemTaskTypeAccountQuotaRefundRecovery)
	require.NoError(t, err)
	require.NotNil(t, task)
	assert.NotContains(t, task.Payload, "settlement")
}

func TestAuthoritativeRefundCommitSuccessResponseLostIsResolved(t *testing.T) {
	db := setupAuthoritativeBillingDB(t)
	user, token := seedAuthoritativeBilling(t, db, "lost-refund-response", 1000, 500, false)
	session, apiErr := NewBillingSession(nil, authoritativeRelay(user, token, "lost-refund-response"), 100)
	require.Nil(t, apiErr)
	originalRefund := authoritativeRefundAccountQuota
	authoritativeRefundAccountQuota = func(ctx context.Context, db *gorm.DB, input model.AccountQuotaTerminalInput) (*model.AccountQuotaMutationReceipt, error) {
		receipt, err := model.RefundAccountQuota(ctx, db, input)
		if err != nil {
			return receipt, err
		}
		return nil, errors.New("refund commit response lost")
	}
	t.Cleanup(func() { authoritativeRefundAccountQuota = originalRefund })
	session.Refund(nil)
	assert.True(t, session.refunded)
	assert.False(t, session.refundRecoveryScheduled)
	var obligationCount int64
	require.NoError(t, db.Model(&model.AccountQuotaTerminalRecoveryObligation{}).Count(&obligationCount).Error)
	assert.EqualValues(t, 1, obligationCount)
	var obligation model.AccountQuotaTerminalRecoveryObligation
	require.NoError(t, db.Where("request_id = ?", "lost-refund-response").First(&obligation).Error)
	assert.Equal(t, model.AccountQuotaTerminalRecoveryApplied, obligation.State)
	require.NoError(t, db.First(user, user.Id).Error)
	require.NoError(t, db.First(token, token.Id).Error)
	assert.Equal(t, 1000, user.Quota)
	assert.Equal(t, 500, token.RemainQuota)
}

func TestLegacyTrustedBillingSessionReserveRemainsNoOp(t *testing.T) {
	session := &BillingSession{writerMode: model.QuotaWriterModeLegacy, trusted: true, preConsumedQuota: 0}
	require.NoError(t, session.Reserve(100))
	assert.Zero(t, session.GetPreConsumedQuota())
}

func TestBillingSessionGetPreConsumedQuotaConcurrentWithReserve(t *testing.T) {
	db := setupAuthoritativeBillingDB(t)
	user, token := seedAuthoritativeBilling(t, db, "quota-race", 10000, 10000, false)
	session, apiErr := NewBillingSession(nil, authoritativeRelay(user, token, "quota-race"), 100)
	require.Nil(t, apiErr)
	var wg sync.WaitGroup
	errCh := make(chan error, 1)
	wg.Add(2)
	go func() {
		defer wg.Done()
		for index := 0; index < 25; index++ {
			_ = session.GetPreConsumedQuota()
		}
	}()
	go func() {
		defer wg.Done()
		for index := 1; index <= 25; index++ {
			if err := session.Reserve(100 + index); err != nil {
				errCh <- err
				return
			}
		}
	}()
	wg.Wait()
	close(errCh)
	for err := range errCh {
		require.NoError(t, err)
	}
	assert.Equal(t, 125, session.GetPreConsumedQuota())
}

func TestAuthoritativeRefundPersistentFailureRecoveredByRecoveryWorker(t *testing.T) {
	db := setupAuthoritativeBillingDB(t)
	user, token := seedAuthoritativeBilling(t, db, "worker-refund", 1000, 500, false)
	session, apiErr := NewBillingSession(nil, authoritativeRelay(user, token, "worker-refund"), 100)
	require.Nil(t, apiErr)
	const callbackName = "test:fail_authoritative_refund_receipt_create"
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if receipt, ok := tx.Statement.Dest.(*model.AccountQuotaMutationReceipt); ok && receipt.Phase == model.AccountQuotaPhaseRefund {
			tx.AddError(errors.New("persistent synchronous refund database failure"))
		}
	}))
	t.Cleanup(func() { db.Callback().Create().Remove(callbackName) })
	previousGate := common.TaskRecoveryObligationRecoveryEnabled
	common.TaskRecoveryObligationRecoveryEnabled = false
	t.Cleanup(func() { common.TaskRecoveryObligationRecoveryEnabled = previousGate })
	refundErr := session.Refund(nil)
	require.ErrorIs(t, refundErr, model.ErrAccountQuotaRefundPending)
	assert.True(t, session.refundRecoveryScheduled)
	assert.False(t, session.NeedsRefund())
	var obligation model.AccountQuotaTerminalRecoveryObligation
	require.NoError(t, db.Where("request_id = ?", "worker-refund").First(&obligation).Error)
	assert.Equal(t, model.AccountQuotaTerminalRecoveryPending, obligation.State)
	task, err := model.GetActiveSystemTask(model.SystemTaskTypeAccountQuotaRefundRecovery)
	require.NoError(t, err)
	require.NotNil(t, task)
	require.NoError(t, db.Model(&model.QuotaProjectionObligation{}).Where("receipt_kind = ?", "account").Update("state", string(model.QuotaProjectionObligationStateApplied)).Error)
	db.Callback().Create().Remove(callbackName)
	claimed, won, err := model.ClaimSystemTask(task.ID, model.SystemTaskTypeAccountQuotaRefundRecovery, "account-refund-worker", common.GetTimestamp()+60)
	require.NoError(t, err)
	require.True(t, won)
	accountQuotaRefundRecoveryHandler{}.Run(context.Background(), claimed, "account-refund-worker")
	finished, err := model.GetSystemTaskByTaskID(task.TaskID)
	require.NoError(t, err)
	require.NotNil(t, finished)
	assert.Equal(t, model.SystemTaskStatusSucceeded, finished.Status)
	require.NoError(t, db.First(&obligation, obligation.ID).Error)
	assert.Equal(t, model.AccountQuotaTerminalRecoveryApplied, obligation.State)
	assert.NotZero(t, obligation.TerminalReceiptID)
	require.NoError(t, db.First(user, user.Id).Error)
	require.NoError(t, db.First(token, token.Id).Error)
	assert.Equal(t, 1000, user.Quota)
	assert.Equal(t, 500, token.RemainQuota)
}

func TestAuthoritativeSettlementPersistentFailureKeepsDurableRecoveryInput(t *testing.T) {
	db := setupAuthoritativeBillingDB(t)
	user, token := seedAuthoritativeBilling(t, db, "worker-settlement", 1000, 500, false)
	info := authoritativeRelay(user, token, "worker-settlement")
	session, apiErr := NewBillingSession(nil, info, 100)
	require.Nil(t, apiErr)
	const callbackName = "test:fail_authoritative_settlement_receipt_create"
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if receipt, ok := tx.Statement.Dest.(*model.AccountQuotaMutationReceipt); ok && receipt.Phase == model.AccountQuotaPhaseSettle {
			tx.AddError(errors.New("persistent synchronous settlement database failure"))
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Create().Remove(callbackName) })

	settleErr := session.Settle(80)
	require.ErrorIs(t, settleErr, model.ErrAccountQuotaSettlementPending)
	assert.True(t, session.settlementPending)
	var intent model.AccountQuotaSettlementIntent
	require.NoError(t, db.Where("event_key = ?", "billing-settlement:worker-settlement:v2").First(&intent).Error)
	assert.EqualValues(t, 80, intent.ActualQuota)
	var fact model.AccountQuotaSettlementFact
	require.NoError(t, db.Where("event_key = ?", "billing-settlement:worker-settlement:v2").First(&fact).Error)
	assert.Equal(t, model.AccountQuotaSettlementKindAuthoritative, fact.Kind)
	assert.EqualValues(t, 80, fact.ActualQuota)
	assert.Equal(t, model.AccountQuotaSettlementRetryable, fact.State)
	conflictErr := session.Settle(81)
	require.Error(t, conflictErr)
	require.NotNil(t, session.settlementInput)
	assert.EqualValues(t, 80, session.settlementInput.ActualQuota)
	task, err := model.GetActiveSystemTask(model.SystemTaskTypeAccountQuotaRefundRecovery)
	require.NoError(t, err)
	require.NotNil(t, task)
	assert.NotContains(t, task.Payload, "worker-settlement")
	assert.NotContains(t, task.Payload, "actual_quota")

	require.NoError(t, db.Callback().Create().Remove(callbackName))
	worker := NewTaskRecoveryWorker("account-settlement-worker")
	worker.BatchSize = 10
	processed, err := worker.RecoverAccountQuotaRefundFacts(context.Background(), db)
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	require.NoError(t, db.First(&intent, intent.ID).Error)
	assert.Equal(t, model.AccountQuotaSettlementApplied, intent.State)
	require.NoError(t, db.First(&fact, fact.ID).Error)
	assert.Equal(t, model.AccountQuotaSettlementApplied, fact.State)
	require.NoError(t, db.First(user, user.Id).Error)
	require.NoError(t, db.First(token, token.Id).Error)
	assert.Equal(t, 920, user.Quota)
	assert.Equal(t, 420, token.RemainQuota)
	assert.Equal(t, 80, token.UsedQuota)
}

func TestAuthoritativeSettlementFactCreateFailureRecoversFromIndependentIntent(t *testing.T) {
	db := setupAuthoritativeBillingDB(t)
	user, token := seedAuthoritativeBilling(t, db, "fact-create-failure", 1000, 500, false)
	info := authoritativeRelay(user, token, "fact-create-failure")
	session, apiErr := NewBillingSession(nil, info, 100)
	require.Nil(t, apiErr)
	const callbackName = "test:fail_authoritative_settlement_fact_create"
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if _, ok := tx.Statement.Dest.(*model.AccountQuotaSettlementFact); ok {
			tx.AddError(errors.New("settlement fact storage unavailable"))
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Create().Remove(callbackName) })

	settleErr := session.Settle(70)
	require.ErrorIs(t, settleErr, model.ErrAccountQuotaSettlementPending)
	secondUser, secondToken := seedAuthoritativeBilling(t, db, "fact-create-failure-second", 800, 400, false)
	secondSession, secondAPIErr := NewBillingSession(nil, authoritativeRelay(secondUser, secondToken, "fact-create-failure-second"), 90)
	require.Nil(t, secondAPIErr)
	require.ErrorIs(t, secondSession.Settle(60), model.ErrAccountQuotaSettlementPending)
	var intent model.AccountQuotaSettlementIntent
	require.NoError(t, db.Where("event_key = ?", "billing-settlement:fact-create-failure:v2").First(&intent).Error)
	assert.EqualValues(t, 70, intent.ActualQuota)
	assert.Equal(t, model.AccountQuotaSettlementPending, intent.State)
	var secondIntent model.AccountQuotaSettlementIntent
	require.NoError(t, db.Where("event_key = ?", "billing-settlement:fact-create-failure-second:v2").First(&secondIntent).Error)
	assert.EqualValues(t, 60, secondIntent.ActualQuota)
	var factCount int64
	require.NoError(t, db.Model(&model.AccountQuotaSettlementFact{}).Count(&factCount).Error)
	assert.Zero(t, factCount)
	task, err := model.GetActiveSystemTask(model.SystemTaskTypeAccountQuotaRefundRecovery)
	require.NoError(t, err)
	require.NotNil(t, task)
	assert.NotContains(t, task.Payload, intent.EventKey)

	require.NoError(t, db.Callback().Create().Remove(callbackName))
	claimed, won, err := model.ClaimSystemTask(task.ID, model.SystemTaskTypeAccountQuotaRefundRecovery, "settlement-intent-restart", common.GetTimestamp()+60)
	require.NoError(t, err)
	require.True(t, won)
	accountQuotaRefundRecoveryHandler{}.Run(context.Background(), claimed, "settlement-intent-restart")
	finished, err := model.GetSystemTaskByTaskID(task.TaskID)
	require.NoError(t, err)
	require.NotNil(t, finished)
	assert.Equal(t, model.SystemTaskStatusSucceeded, finished.Status)
	require.NoError(t, db.First(&intent, intent.ID).Error)
	assert.Equal(t, model.AccountQuotaSettlementApplied, intent.State)
	assert.NotZero(t, intent.FactID)
	assert.NotZero(t, intent.TerminalReceiptID)
	require.NoError(t, db.First(&secondIntent, secondIntent.ID).Error)
	assert.Equal(t, model.AccountQuotaSettlementApplied, secondIntent.State)
	assert.NotZero(t, secondIntent.FactID)
	require.NoError(t, db.First(user, user.Id).Error)
	require.NoError(t, db.First(token, token.Id).Error)
	assert.Equal(t, 930, user.Quota)
	assert.Equal(t, 430, token.RemainQuota)
	assert.Equal(t, 70, token.UsedQuota)
	require.NoError(t, db.First(secondUser, secondUser.Id).Error)
	require.NoError(t, db.First(secondToken, secondToken.Id).Error)
	assert.Equal(t, 740, secondUser.Quota)
	assert.Equal(t, 340, secondToken.RemainQuota)
	assert.Equal(t, 60, secondToken.UsedQuota)
}

func TestAuthoritativeSettlementIntentInsertFailureFallsBackToDurableFact(t *testing.T) {
	db := setupAuthoritativeBillingDB(t)
	user, token := seedAuthoritativeBilling(t, db, "intent-insert-failure", 1000, 500, false)
	info := authoritativeRelay(user, token, "intent-insert-failure")
	session, apiErr := NewBillingSession(nil, info, 100)
	require.Nil(t, apiErr)
	info.Billing = session
	const callbackName = "test:fail_settlement_intent_insert"
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if _, ok := tx.Statement.Dest.(*model.AccountQuotaSettlementIntent); ok {
			tx.AddError(errors.New("intent insert failed"))
		}
		if receipt, ok := tx.Statement.Dest.(*model.AccountQuotaMutationReceipt); ok && receipt.Phase == model.AccountQuotaPhaseSettle {
			tx.AddError(errors.New("terminal settlement temporarily unavailable"))
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Create().Remove(callbackName) })

	require.ErrorIs(t, session.Settle(70), model.ErrAccountQuotaSettlementPending)
	assert.False(t, session.settled)
	assert.Nil(t, session.settlementIntent)
	require.NotNil(t, session.settlementFact)
	assert.Equal(t, model.AccountQuotaSettlementRetryable, session.settlementFact.State)
	assert.EqualValues(t, 70, session.settlementFact.ActualQuota)
	require.ErrorIs(t, session.Settle(71), model.ErrAccountQuotaMutationConflict)
	assert.EqualValues(t, 70, session.settlementInput.ActualQuota)
	var lifecycle model.AccountQuotaTerminalRecoveryObligation
	require.NoError(t, db.Where("request_id = ?", info.RequestId).First(&lifecycle).Error)
	assert.Equal(t, model.AccountQuotaTerminalRecoveryOpen, lifecycle.State)
	assert.Zero(t, lifecycle.ActualQuota)
	auditOther := map[string]interface{}{}
	appendBillingInfo(info, auditOther)
	audit := auditOther["admin_info"].(map[string]interface{})["billing_settlement"].(map[string]interface{})
	assert.EqualValues(t, 70, audit["actual_quota"])
	assert.Equal(t, model.AccountQuotaSettlementRetryable, audit["state"])
	var intentCount int64
	require.NoError(t, db.Model(&model.AccountQuotaSettlementIntent{}).Count(&intentCount).Error)
	assert.Zero(t, intentCount)
	task, taskErr := model.GetActiveSystemTask(model.SystemTaskTypeAccountQuotaRefundRecovery)
	require.NoError(t, taskErr)
	require.NotNil(t, task)
	assert.NotContains(t, task.Payload, "intent-insert-failure")
	require.NoError(t, db.Callback().Create().Remove(callbackName))
	worker := NewTaskRecoveryWorker("emergency-fact-recovery")
	worker.BatchSize = 10
	processed, err := worker.RecoverAccountQuotaRefundFacts(context.Background(), db)
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	require.NoError(t, db.First(session.settlementFact, session.settlementFact.ID).Error)
	assert.Equal(t, model.AccountQuotaSettlementApplied, session.settlementFact.State)
	require.NoError(t, db.First(user, user.Id).Error)
	require.NoError(t, db.First(token, token.Id).Error)
	assert.Equal(t, 930, user.Quota)
	assert.Equal(t, 430, token.RemainQuota)
}

func TestAuthoritativeSettlementDualInsertFailurePersistsManualEvidenceWithoutEmptyTask(t *testing.T) {
	db := setupAuthoritativeBillingDB(t)
	user, token := seedAuthoritativeBilling(t, db, "manual-evidence", 1000, 500, false)
	info := authoritativeRelay(user, token, "manual-evidence")
	session, apiErr := NewBillingSession(nil, info, 100)
	require.Nil(t, apiErr)
	info.Billing = session
	const callbackName = "test:fail_settlement_intent_and_fact_insert"
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		switch tx.Statement.Dest.(type) {
		case *model.AccountQuotaSettlementIntent, *model.AccountQuotaSettlementFact:
			tx.AddError(errors.New("settlement evidence insert failed"))
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Create().Remove(callbackName) })

	settleErr := session.Settle(70)
	require.ErrorIs(t, settleErr, model.ErrAccountQuotaSettlementManualRequired)
	var evidence model.AccountQuotaTerminalRecoveryObligation
	require.NoError(t, db.Where("request_id = ?", info.RequestId).First(&evidence).Error)
	assert.Equal(t, model.AccountQuotaTerminalRecoveryManual, evidence.State)
	assert.Equal(t, model.AccountQuotaPhaseSettle, evidence.Phase)
	assert.EqualValues(t, 70, evidence.ActualQuota)
	assert.NotEmpty(t, evidence.RequestFingerprint)
	assert.NotNil(t, session.settlementInput)
	assert.EqualValues(t, 70, session.settlementInput.ActualQuota)

	conflictErr := session.Settle(71)
	require.Error(t, conflictErr)
	assert.EqualValues(t, 70, session.settlementInput.ActualQuota, "conflicting retry must not replace durable audit input")
	other := map[string]interface{}{}
	appendBillingInfo(info, other)
	audit := other["admin_info"].(map[string]interface{})["billing_settlement"].(map[string]interface{})
	assert.Equal(t, model.AccountQuotaSettlementManual, audit["state"])
	assert.EqualValues(t, 70, audit["actual_quota"])
	assert.EqualValues(t, evidence.ID, audit["manual_evidence_id"])
	task, taskErr := model.GetActiveSystemTask(model.SystemTaskTypeAccountQuotaRefundRecovery)
	require.NoError(t, taskErr)
	assert.Nil(t, task, "manual evidence must not be replaced by an empty scanner task")
}

func TestAuthoritativeSettlementAppliedFactWaitsForTerminalReadback(t *testing.T) {
	db := setupAuthoritativeBillingDB(t)
	user, token := seedAuthoritativeBilling(t, db, "terminal-readback", 1000, 500, false)
	info := authoritativeRelay(user, token, "terminal-readback")
	session, apiErr := NewBillingSession(nil, info, 100)
	require.Nil(t, apiErr)
	info.Billing = session
	originalFind := findAccountQuotaTerminalReceipt
	findAccountQuotaTerminalReceipt = func(*gorm.DB, string) (*model.AccountQuotaMutationReceipt, error) {
		return nil, errors.New("terminal receipt readback unavailable")
	}
	t.Cleanup(func() { findAccountQuotaTerminalReceipt = originalFind })

	settleErr := session.Settle(80)
	require.ErrorIs(t, settleErr, model.ErrAccountQuotaSettlementPending)
	assert.False(t, session.settled)
	require.NotNil(t, session.settlementFact)
	assert.Equal(t, model.AccountQuotaSettlementApplied, session.settlementFact.State)
	other := map[string]interface{}{}
	appendBillingInfo(info, other)
	audit := other["admin_info"].(map[string]interface{})["billing_settlement"].(map[string]interface{})
	assert.Equal(t, "applied_readback_pending", audit["state"])
	assert.EqualValues(t, 80, audit["actual_quota"])

	conflictErr := session.Settle(81)
	require.Error(t, conflictErr)
	assert.EqualValues(t, 80, session.settlementInput.ActualQuota)
	findAccountQuotaTerminalReceipt = originalFind
	require.NoError(t, session.Settle(80))
	assert.True(t, session.settled)
	require.NoError(t, db.First(user, user.Id).Error)
	require.NoError(t, db.First(token, token.Id).Error)
	assert.Equal(t, 920, user.Quota)
	assert.Equal(t, 420, token.RemainQuota)
}

func TestAuthoritativeSubscriptionSettlementRecoveryKeepsAuditAndProjectionConsistent(t *testing.T) {
	db := setupAuthoritativeBillingDB(t)
	user, token := seedAuthoritativeBilling(t, db, "subscription-recovery", 1000, 500, false)
	plan := &model.SubscriptionPlan{Title: "recovery-plan", Enabled: true, DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1, QuotaResetPeriod: model.SubscriptionResetNever}
	require.NoError(t, db.Create(plan).Error)
	subscription := &model.UserSubscription{UserId: user.Id, PlanId: plan.Id, AmountTotal: 1000, StartTime: time.Now().Add(-time.Hour).Unix(), EndTime: time.Now().Add(time.Hour).Unix(), Status: "active"}
	require.NoError(t, db.Create(subscription).Error)
	info := authoritativeRelay(user, token, "subscription-recovery")
	info.UserSetting.BillingPreference = "subscription_only"
	info.TaskRelayInfo = &relaycommon.TaskRelayInfo{Action: "generate"}
	info.ChannelMeta = &relaycommon.ChannelMeta{}
	session, apiErr := NewBillingSession(nil, info, 100)
	require.Nil(t, apiErr)
	info.Billing = session
	const callbackName = "test:fail_subscription_settlement_receipt_create"
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if receipt, ok := tx.Statement.Dest.(*model.AccountQuotaMutationReceipt); ok && receipt.Phase == model.AccountQuotaPhaseSettle {
			tx.AddError(errors.New("subscription settlement unavailable"))
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Create().Remove(callbackName) })

	settleErr := session.Settle(80)
	require.ErrorIs(t, settleErr, model.ErrAccountQuotaSettlementPending)
	pendingOther := map[string]interface{}{}
	appendBillingInfo(info, pendingOther)
	assert.NotContains(t, pendingOther, "subscription_post_delta")
	assert.EqualValues(t, 100, pendingOther["subscription_used"])
	pendingAudit := pendingOther["admin_info"].(map[string]interface{})["billing_settlement"].(map[string]interface{})
	assert.Contains(t, []string{model.AccountQuotaSettlementPending, model.AccountQuotaSettlementMaterialized}, pendingAudit["state"])
	assert.EqualValues(t, -20, pendingAudit["intended_subscription_post_delta"])

	previousLogDB, previousLogConsume := model.LOG_DB, common.LogConsumeEnabled
	model.LOG_DB = db
	common.LogConsumeEnabled = true
	t.Cleanup(func() { model.LOG_DB, common.LogConsumeEnabled = previousLogDB, previousLogConsume })
	require.NoError(t, db.AutoMigrate(&model.Log{}, &model.BillingLogProjectionIdentity{}, &model.Channel{}))
	ginCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ginCtx.Request = httptest.NewRequest("POST", "/v1/videos", nil)
	ginCtx.Set(common.RequestIdKey, info.RequestId)
	info.PriceData.Quota = 80
	LogTaskConsumption(ginCtx, info)
	var consumeLog model.Log
	require.NoError(t, db.Where("request_id = ? AND type = ?", info.RequestId, model.LogTypeConsume).First(&consumeLog).Error)
	var loggedOther map[string]interface{}
	require.NoError(t, common.UnmarshalJsonStr(consumeLog.Other, &loggedOther))
	loggedAudit := loggedOther["admin_info"].(map[string]interface{})["billing_settlement"].(map[string]interface{})
	assert.Contains(t, []interface{}{model.AccountQuotaSettlementPending, model.AccountQuotaSettlementMaterialized}, loggedAudit["state"])
	assert.NotContains(t, loggedOther, "subscription_post_delta")

	require.NoError(t, db.Callback().Create().Remove(callbackName))
	worker := NewTaskRecoveryWorker("subscription-settlement-recovery")
	worker.BatchSize = 10
	processed, err := worker.RecoverAccountQuotaRefundFacts(context.Background(), db)
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	var intent model.AccountQuotaSettlementIntent
	require.NoError(t, db.Where("event_key = ?", "billing-settlement:subscription-recovery:v2").First(&intent).Error)
	assert.Equal(t, model.AccountQuotaSettlementApplied, intent.State)
	var terminal model.AccountQuotaMutationReceipt
	require.NoError(t, db.First(&terminal, intent.TerminalReceiptID).Error)
	assert.Equal(t, model.AccountQuotaPhaseSettle, terminal.Phase)
	assert.EqualValues(t, 80, terminal.RequestedQuota)
	require.NoError(t, db.First(subscription, subscription.Id).Error)
	require.NoError(t, db.First(token, token.Id).Error)
	assert.EqualValues(t, 80, subscription.AmountUsed)
	assert.Equal(t, 420, token.RemainQuota)
	assert.Equal(t, 80, token.UsedQuota)
	var projection model.QuotaProjectionObligation
	require.NoError(t, db.Where("receipt_kind = ? AND receipt_id = ?", "account", terminal.ID).First(&projection).Error)
	assert.Equal(t, terminal.EventKey, projection.EventKey)
	assert.Equal(t, terminal.After.Token.QuotaVersion, projection.ExpectedTokenVersion)

	processed, err = worker.RecoverAccountQuotaRefundFacts(context.Background(), db)
	require.NoError(t, err)
	assert.Zero(t, processed)
	var consumeLogCount int64
	require.NoError(t, db.Model(&model.Log{}).Where("request_id = ? AND type = ?", info.RequestId, model.LogTypeConsume).Count(&consumeLogCount).Error)
	assert.EqualValues(t, 1, consumeLogCount)
	require.NoError(t, db.First(subscription, subscription.Id).Error)
	assert.EqualValues(t, 80, subscription.AmountUsed)
}

func TestAuthoritativeTrustedReserveRemainsNoOp(t *testing.T) {
	db := setupAuthoritativeBillingDB(t)
	trust := common.GetTrustQuota()
	user, token := seedAuthoritativeBilling(t, db, "trusted-reserve-noop", trust+1000, trust+1000, false)
	session, apiErr := NewBillingSession(nil, authoritativeRelay(user, token, "trusted-reserve-noop"), 100)
	require.Nil(t, apiErr)
	require.True(t, session.trusted)
	require.NoError(t, session.Reserve(500))
	assert.Zero(t, session.GetPreConsumedQuota())
	require.NoError(t, db.First(user, user.Id).Error)
	require.NoError(t, db.First(token, token.Id).Error)
	assert.Equal(t, trust+1000, user.Quota)
	assert.Equal(t, trust+1000, token.RemainQuota)
}

func TestAuthoritativeRefundRetriesIntentPersistenceAndExposesPersistentFailure(t *testing.T) {
	t.Run("transient update failures are retried", func(t *testing.T) {
		db := setupAuthoritativeBillingDB(t)
		user, token := seedAuthoritativeBilling(t, db, "intent-retry", 1000, 500, false)
		session, apiErr := NewBillingSession(nil, authoritativeRelay(user, token, "intent-retry"), 100)
		require.Nil(t, apiErr)
		var failures atomic.Int32
		const callbackName = "test:transient_refund_intent_update_failure"
		require.NoError(t, db.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
			if tx.Statement.Table == (model.AccountQuotaTerminalRecoveryObligation{}).TableName() && failures.Add(1) <= 2 {
				tx.AddError(errors.New("transient refund intent update failure"))
			}
		}))
		t.Cleanup(func() { _ = db.Callback().Update().Remove(callbackName) })
		session.Refund(nil)
		assert.True(t, session.refunded)
		assert.Nil(t, session.refundIntentErr)
		assert.GreaterOrEqual(t, failures.Load(), int32(3))
	})

	t.Run("persistent update failure remains observable and retryable", func(t *testing.T) {
		db := setupAuthoritativeBillingDB(t)
		user, token := seedAuthoritativeBilling(t, db, "intent-persistent", 1000, 500, false)
		session, apiErr := NewBillingSession(nil, authoritativeRelay(user, token, "intent-persistent"), 100)
		require.Nil(t, apiErr)
		const callbackName = "test:persistent_refund_intent_update_failure"
		require.NoError(t, db.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
			if tx.Statement.Table == (model.AccountQuotaTerminalRecoveryObligation{}).TableName() {
				tx.AddError(errors.New("persistent refund intent update failure"))
			}
		}))
		refundErr := session.Refund(nil)
		require.ErrorIs(t, refundErr, model.ErrAccountQuotaRefundPending)
		require.Error(t, session.refundIntentErr)
		assert.False(t, session.NeedsRefund())
		var fact model.AccountQuotaRefundFact
		require.NoError(t, db.Where("event_key = ?", "billing-refund:intent-persistent:v1").First(&fact).Error)
		assert.Equal(t, model.AccountQuotaRefundFactRetryable, fact.State)
		var obligation model.AccountQuotaTerminalRecoveryObligation
		require.NoError(t, db.Where("request_id = ?", "intent-persistent").First(&obligation).Error)
		assert.Equal(t, model.AccountQuotaTerminalRecoveryOpen, obligation.State)
		require.NoError(t, db.Callback().Update().Remove(callbackName))
		restartedSession, restartedErr := NewBillingSession(nil, authoritativeRelay(user, token, "intent-persistent"), 100)
		require.Nil(t, restartedErr)
		require.NoError(t, restartedSession.Refund(nil))
		assert.True(t, restartedSession.refunded)
		assert.Nil(t, restartedSession.refundIntentErr)
	})
}

type failingLegacyFunding struct {
	db          *gorm.DB
	failure     error
	callbackKey string
}

func (f *failingLegacyFunding) Source() string   { return BillingSourceWallet }
func (f *failingLegacyFunding) Settle(int) error { return nil }
func (f *failingLegacyFunding) Refund() error    { return nil }
func (f *failingLegacyFunding) PreConsume(int) error {
	_ = f.db.Callback().Update().Before("gorm:update").Register(f.callbackKey, func(tx *gorm.DB) {
		if tx.Statement.Table == "tokens" {
			tx.AddError(errors.New("injected token compensation write failure"))
		}
	})
	return f.failure
}

func TestLegacyFundingFailureKeepsTokenDebitUntilDurableCompensationApplies(t *testing.T) {
	db := setupAuthoritativeBillingDB(t)
	require.NoError(t, db.Model(&model.QuotaWriterEpoch{}).Where("id = ?", 1).Update("mode", string(model.QuotaWriterModeLegacy)).Error)
	user, token := seedAuthoritativeBilling(t, db, "legacy-token-compensation", 1000, 500, false)
	oldBatch := common.BatchUpdateEnabled
	common.BatchUpdateEnabled = false
	t.Cleanup(func() { common.BatchUpdateEnabled = oldBatch })

	callbackKey := "test:legacy-token-compensation-write"
	funding := &failingLegacyFunding{db: db, failure: ErrInsufficientWalletQuota, callbackKey: callbackKey}
	info := authoritativeRelay(user, token, "legacy-token-compensation")
	info.ForcePreConsume = true
	session := &BillingSession{relayInfo: info, funding: funding, writerMode: model.QuotaWriterModeLegacy}
	oldRefund := legacyTokenQuotaRefund
	legacyTokenQuotaRefund = func(int, string, int) error { return errors.New("injected immediate token rollback failure") }
	t.Cleanup(func() { legacyTokenQuotaRefund = oldRefund })

	gin.SetMode(gin.TestMode)
	ginCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	apiErr := session.preConsume(ginCtx, 100)
	require.NotNil(t, apiErr)
	assert.Equal(t, 100, session.tokenConsumed, "unproven token rollback must remain recorded")
	assert.True(t, session.refundRecoveryScheduled)
	var fact model.AccountQuotaRefundFact
	require.NoError(t, db.Where("request_id = ?", info.RequestId).First(&fact).Error)
	assert.Equal(t, model.AccountQuotaRefundFactRetryable, fact.State)
	require.NoError(t, db.First(token, token.Id).Error)
	assert.Equal(t, 400, token.RemainQuota)
	assert.Equal(t, 100, token.UsedQuota)

	require.NoError(t, db.Callback().Update().Remove(callbackKey))
	processed, err := model.RunAccountQuotaRefundFacts(context.Background(), db, "legacy-token-restart", 10)
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	require.NoError(t, db.First(token, token.Id).Error)
	assert.Equal(t, 500, token.RemainQuota)
	assert.Zero(t, token.UsedQuota)
	require.NoError(t, db.First(&fact, fact.ID).Error)
	assert.Equal(t, model.AccountQuotaRefundFactApplied, fact.State)
}

func TestLegacyBillingSessionSettlementFailureKeepsDurableRecoveryEntry(t *testing.T) {
	db := setupAuthoritativeBillingDB(t)
	require.NoError(t, db.Model(&model.QuotaWriterEpoch{}).Where("id = ?", 1).Update("mode", string(model.QuotaWriterModeLegacy)).Error)
	user, token := seedAuthoritativeBilling(t, db, "legacy-settlement-recovery", 900, 400, false)
	require.NoError(t, db.Model(&model.Token{}).Where("id = ?", token.Id).Updates(map[string]any{"used_quota": 100, "quota_version": 1}).Error)
	info := authoritativeRelay(user, token, "legacy-settlement-recovery")
	session := &BillingSession{
		relayInfo: info, funding: &WalletFunding{userId: user.Id, consumed: 100}, writerMode: model.QuotaWriterModeLegacy,
		preConsumedQuota: 100, tokenConsumed: 100,
	}
	callbackKey := "test:legacy-settlement-token-failure"
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register(callbackKey, func(tx *gorm.DB) {
		if tx.Statement.Table == "tokens" {
			tx.AddError(errors.New("injected settlement token failure"))
		}
	}))
	settleErr := session.Settle(150)
	require.ErrorIs(t, settleErr, model.ErrAccountQuotaSettlementPending)
	assert.False(t, session.settled)
	assert.True(t, session.settlementPending)
	assert.True(t, session.fundingSettled)
	var fact model.AccountQuotaSettlementFact
	require.NoError(t, db.Where("request_id = ?", info.RequestId).First(&fact).Error)
	assert.True(t, fact.FundingApplied)
	assert.False(t, fact.TokenApplied)
	assert.Equal(t, model.AccountQuotaSettlementRetryable, fact.State)
	task, taskErr := model.GetActiveSystemTask(model.SystemTaskTypeAccountQuotaRefundRecovery)
	require.NoError(t, taskErr)
	require.NotNil(t, task)
	require.NoError(t, db.First(user, user.Id).Error)
	require.NoError(t, db.First(token, token.Id).Error)
	assert.Equal(t, 850, user.Quota)
	assert.Equal(t, 400, token.RemainQuota)
	assert.Equal(t, 100, token.UsedQuota)

	require.NoError(t, db.Callback().Update().Remove(callbackKey))
	worker := NewTaskRecoveryWorker("legacy-settlement-restart")
	worker.BatchSize = 10
	processed, err := worker.RecoverAccountQuotaRefundFacts(context.Background(), db)
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	require.NoError(t, db.First(user, user.Id).Error)
	require.NoError(t, db.First(token, token.Id).Error)
	assert.Equal(t, 850, user.Quota)
	assert.Equal(t, 350, token.RemainQuota)
	assert.Equal(t, 150, token.UsedQuota)
	require.NoError(t, db.First(&fact, fact.ID).Error)
	assert.Equal(t, model.AccountQuotaSettlementApplied, fact.State)
}
