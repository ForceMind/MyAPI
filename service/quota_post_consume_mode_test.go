package service

import (
	"context"
	"errors"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	relaytypes "github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/ForceMind/MyAPI/setting/config"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// setupPostConsumeModeDB builds an isolated SQLite fixture with the quota
// writer epoch pinned to the requested mode. It mirrors
// setupAuthoritativeBillingDB and additionally migrates
// UserQuotaMutationReceipt so authoritative user-quota writes can land.
func setupPostConsumeModeDB(t *testing.T, mode model.QuotaWriterMode) *gorm.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "post-consume-mode.db")
	db, err := gorm.Open(sqlite.Open(path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(4)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.SubscriptionPlan{}, &model.UserSubscription{}, &model.Channel{},
		&model.UserQuotaMutationReceipt{}, &model.AccountQuotaMutationReceipt{}, &model.AccountQuotaReservationHead{},
		&model.AccountQuotaTerminalRecoveryObligation{}, &model.AccountQuotaRefundFact{}, &model.AccountQuotaSettlementIntent{},
		&model.AccountQuotaSettlementFact{}, &model.SystemTask{}, &model.SystemTaskLock{},
		&model.QuotaWriterEpoch{}, &model.QuotaProjectionObligation{}, &model.QuotaBalanceBatchDrain{}, &model.QuotaBalanceBatchSubject{}, &model.QuotaWorkCursor{}))
	require.True(t, model.RefreshAccountQuotaSettlementIntentSchemaCapability(db))
	require.NoError(t, model.EnsureQuotaWriterEpochStateWithDB(db))
	require.NoError(t, db.Model(&model.QuotaWriterEpoch{}).Where("id = ?", 1).Updates(map[string]interface{}{
		"mode": string(mode), "epoch": int64(41), "lock_version": gorm.Expr("lock_version + ?", 1),
	}).Error)
	oldDB := model.DB
	oldRedis, oldRDB := common.RedisEnabled, common.RDB
	oldBatch := common.BatchUpdateEnabled
	model.DB = db
	model.InitColumnNamesForTest()
	common.RedisEnabled, common.RDB = false, nil
	common.BatchUpdateEnabled = false
	t.Cleanup(func() { model.DB = oldDB; common.RedisEnabled, common.RDB = oldRedis, oldRDB; common.BatchUpdateEnabled = oldBatch })
	return db
}

func loadPostConsumeBalances(t *testing.T, db *gorm.DB, user *model.User, token *model.Token) (int, int, int) {
	t.Helper()
	require.NoError(t, db.First(user, user.Id).Error)
	require.NoError(t, db.First(token, token.Id).Error)
	return user.Quota, token.RemainQuota, token.UsedQuota
}

func countReceiptsWithEventKey(t *testing.T, db *gorm.DB, eventKey string) (userReceipts int64, tokenReceipts int64) {
	t.Helper()
	require.NoError(t, db.Model(&model.UserQuotaMutationReceipt{}).Where("business_event_key = ?", eventKey).Count(&userReceipts).Error)
	require.NoError(t, db.Model(&model.AccountQuotaMutationReceipt{}).Where("event_key = ?", eventKey).Count(&tokenReceipts).Error)
	return userReceipts, tokenReceipts
}

func TestPostConsumeQuotaChargeDualMode(t *testing.T) {
	for _, mode := range []model.QuotaWriterMode{model.QuotaWriterModeLegacy, model.QuotaWriterModeAuthoritative} {
		t.Run(string(mode), func(t *testing.T) {
			db := setupPostConsumeModeDB(t, mode)
			user, token := seedAuthoritativeBilling(t, db, "post-charge-"+string(mode), 1000, 500, false)
			info := authoritativeRelay(user, token, "post-charge-"+string(mode))
			event := postConsumeQuotaEvent{Namespace: "billing-settlement", ReasonCode: "billing_settlement"}

			result, err := postConsumeQuotaWithEvent(info, 30, 0, false, event)
			require.NoError(t, err)
			assert.True(t, result.FundingApplied)
			assert.True(t, result.TokenApplied)
			userQuota, remain, used := loadPostConsumeBalances(t, db, user, token)
			assert.Equal(t, 970, userQuota)
			assert.Equal(t, 470, remain)
			assert.Equal(t, 30, used)

			eventKey := "billing-settlement:" + info.RequestId
			userReceipts, tokenReceipts := countReceiptsWithEventKey(t, db, eventKey)
			if mode == model.QuotaWriterModeLegacy {
				assert.Zero(t, userReceipts, "legacy 模式不得写入 receipt")
				assert.Zero(t, tokenReceipts, "legacy 模式不得写入 receipt")
				return
			}
			assert.EqualValues(t, 1, userReceipts)
			assert.EqualValues(t, 1, tokenReceipts)

			// 同键同额重放：幂等，不产生二次扣减。
			replay, err := postConsumeQuotaWithEvent(info, 30, 0, false, event)
			require.NoError(t, err)
			assert.True(t, replay.FundingApplied)
			assert.True(t, replay.TokenApplied)
			userQuota, remain, used = loadPostConsumeBalances(t, db, user, token)
			assert.Equal(t, 970, userQuota)
			assert.Equal(t, 470, remain)
			assert.Equal(t, 30, used)
			userReceipts, tokenReceipts = countReceiptsWithEventKey(t, db, eventKey)
			assert.EqualValues(t, 1, userReceipts)
			assert.EqualValues(t, 1, tokenReceipts)

			// 同键不同额：指纹冲突，显式报错且余额不变。
			_, err = postConsumeQuotaWithEvent(info, 31, 0, false, event)
			require.Error(t, err)
			userQuota, remain, used = loadPostConsumeBalances(t, db, user, token)
			assert.Equal(t, 970, userQuota)
			assert.Equal(t, 470, remain)
			assert.Equal(t, 30, used)
		})
	}
}

func TestPostConsumeQuotaRefundDirectionDualMode(t *testing.T) {
	for _, mode := range []model.QuotaWriterMode{model.QuotaWriterModeLegacy, model.QuotaWriterModeAuthoritative} {
		t.Run(string(mode), func(t *testing.T) {
			db := setupPostConsumeModeDB(t, mode)
			user, token := seedAuthoritativeBilling(t, db, "post-refund-"+string(mode), 970, 470, false)
			require.NoError(t, db.Model(&model.Token{}).Where("id = ?", token.Id).Update("used_quota", 30).Error)
			info := authoritativeRelay(user, token, "post-refund-"+string(mode))
			event := postConsumeQuotaEvent{Namespace: "billing-settlement", ReasonCode: "billing_settlement"}

			// 负差额（预扣后返还）：user/token 余额回退。
			result, err := postConsumeQuotaWithEvent(info, -20, 100, false, event)
			require.NoError(t, err)
			assert.True(t, result.FundingApplied)
			assert.True(t, result.TokenApplied)
			userQuota, remain, used := loadPostConsumeBalances(t, db, user, token)
			assert.Equal(t, 990, userQuota)
			assert.Equal(t, 490, remain)
			assert.Equal(t, 10, used)

			if mode == model.QuotaWriterModeAuthoritative {
				// 同键重放返还：幂等。
				_, err = postConsumeQuotaWithEvent(info, -20, 100, false, event)
				require.NoError(t, err)
				userQuota, remain, used = loadPostConsumeBalances(t, db, user, token)
				assert.Equal(t, 990, userQuota)
				assert.Equal(t, 490, remain)
				assert.Equal(t, 10, used)
			}
		})
	}
}

func TestSettleBillingFallbackDualMode(t *testing.T) {
	for _, mode := range []model.QuotaWriterMode{model.QuotaWriterModeLegacy, model.QuotaWriterModeAuthoritative} {
		t.Run(string(mode), func(t *testing.T) {
			db := setupPostConsumeModeDB(t, mode)
			user, token := seedAuthoritativeBilling(t, db, "settle-fallback-"+string(mode), 900, 400, false)
			require.NoError(t, db.Model(&model.Token{}).Where("id = ?", token.Id).Update("used_quota", 100).Error)
			info := authoritativeRelay(user, token, "settle-fallback-"+string(mode))
			info.FinalPreConsumedQuota = 100
			info.UserQuota = 1 << 30 // 避免触发异步低额度通知
			require.Nil(t, info.Billing)

			gin.SetMode(gin.TestMode)
			ginCtx, _ := gin.CreateTestContext(httptest.NewRecorder())

			// 实际消耗 80，预扣 100 → 返还 20。
			require.NoError(t, SettleBilling(ginCtx, info, 80))
			userQuota, remain, used := loadPostConsumeBalances(t, db, user, token)
			assert.Equal(t, 920, userQuota)
			assert.Equal(t, 420, remain)
			assert.Equal(t, 80, used)

			// 重放同一结算：legacy 下由上层保证只调用一次；authoritative 下同键幂等。
			if mode == model.QuotaWriterModeAuthoritative {
				require.NoError(t, SettleBilling(ginCtx, info, 80))
				userQuota, remain, used = loadPostConsumeBalances(t, db, user, token)
				assert.Equal(t, 920, userQuota)
				assert.Equal(t, 420, remain)
				assert.Equal(t, 80, used)
				userReceipts, tokenReceipts := countReceiptsWithEventKey(t, db, "billing-settlement:"+info.RequestId)
				assert.EqualValues(t, 1, userReceipts)
				assert.EqualValues(t, 1, tokenReceipts)
			}
		})
	}
}

func TestPreWssConsumeQuotaDualMode(t *testing.T) {
	for _, mode := range []model.QuotaWriterMode{model.QuotaWriterModeLegacy, model.QuotaWriterModeAuthoritative} {
		t.Run(string(mode), func(t *testing.T) {
			db := setupPostConsumeModeDB(t, mode)
			user, token := seedAuthoritativeBilling(t, db, "realtime-"+string(mode), 1000, 500, false)
			info := authoritativeRelay(user, token, "realtime-"+string(mode))
			gin.SetMode(gin.TestMode)
			ginCtx, _ := gin.CreateTestContext(httptest.NewRecorder())

			// 4 个文本输入 token，未知模型默认倍率 37.5，分组倍率 1 → 150 额度。
			usage := &dto.RealtimeUsage{}
			usage.InputTokenDetails.TextTokens = 4
			require.NoError(t, PreWssConsumeQuota(ginCtx, info, usage))
			userQuota, remain, used := loadPostConsumeBalances(t, db, user, token)
			assert.Equal(t, 850, userQuota)
			assert.Equal(t, 350, remain)
			assert.Equal(t, 150, used)
			assert.Equal(t, 1, info.RealtimeConsumeSeq)

			if mode == model.QuotaWriterModeAuthoritative {
				userReceipts, tokenReceipts := countReceiptsWithEventKey(t, db, "realtime:"+info.RequestId+":1")
				assert.EqualValues(t, 1, userReceipts)
				assert.EqualValues(t, 1, tokenReceipts)

				// 同一连接的第二次计费（下一个 response.done）：序号递增、键不同，独立扣减。
				require.NoError(t, PreWssConsumeQuota(ginCtx, info, usage))
				userQuota, remain, used = loadPostConsumeBalances(t, db, user, token)
				assert.Equal(t, 700, userQuota)
				assert.Equal(t, 200, remain)
				assert.Equal(t, 300, used)
				userReceipts, tokenReceipts = countReceiptsWithEventKey(t, db, "realtime:"+info.RequestId+":2")
				assert.EqualValues(t, 1, userReceipts)
				assert.EqualValues(t, 1, tokenReceipts)
			}
		})
	}
}

func TestPostConsumeQuotaRealtimeQualifierReplay(t *testing.T) {
	db := setupPostConsumeModeDB(t, model.QuotaWriterModeAuthoritative)
	user, token := seedAuthoritativeBilling(t, db, "realtime-replay", 1000, 500, false)
	info := authoritativeRelay(user, token, "realtime-replay")

	first := postConsumeQuotaEvent{Namespace: "realtime", Qualifier: "1", ReasonCode: "realtime_consume"}
	second := postConsumeQuotaEvent{Namespace: "realtime", Qualifier: "2", ReasonCode: "realtime_consume"}

	_, err := postConsumeQuotaWithEvent(info, 10, 0, false, first)
	require.NoError(t, err)
	_, err = postConsumeQuotaWithEvent(info, 12, 0, false, second)
	require.NoError(t, err)
	userQuota, remain, used := loadPostConsumeBalances(t, db, user, token)
	assert.Equal(t, 978, userQuota)
	assert.Equal(t, 478, remain)
	assert.Equal(t, 22, used)

	// 重放第一次计费：同键幂等，不重复扣减。
	_, err = postConsumeQuotaWithEvent(info, 10, 0, false, first)
	require.NoError(t, err)
	userQuota, remain, used = loadPostConsumeBalances(t, db, user, token)
	assert.Equal(t, 978, userQuota)
	assert.Equal(t, 478, remain)
	assert.Equal(t, 22, used)
}

func enableGrokViolationFeeForTest(t *testing.T) {
	t.Helper()
	registered := config.GlobalConfig.Get("grok")
	managed, ok := registered.(config.MapConfig)
	require.True(t, ok, "grok settings must implement config.MapConfig")
	previous, err := managed.ExportConfigMap()
	require.NoError(t, err)
	require.NoError(t, managed.UpdateConfigMap(map[string]string{
		"violation_deduction_enabled": "true",
		"violation_deduction_amount":  "0.0001",
	}))
	t.Cleanup(func() { require.NoError(t, managed.UpdateConfigMap(previous)) })
}

func newViolationFeeAPIError() *relaytypes.NewAPIError {
	// 与生产路径一致：上游错误携带 OpenAIError 与 CSAM 标记。
	return relaytypes.WithOpenAIError(relaytypes.OpenAIError{
		Message: "Failed check: SAFETY_CHECK_TYPE",
		Type:    "content_policy",
		Code:    "content_filter",
	}, 400)
}

func TestChargeViolationFeeDualMode(t *testing.T) {
	// fee 额度 = 0.0001 * QuotaPerUnit * 分组倍率(1) = 50
	const expectedFee = 50
	for _, mode := range []model.QuotaWriterMode{model.QuotaWriterModeLegacy, model.QuotaWriterModeAuthoritative} {
		t.Run(string(mode), func(t *testing.T) {
			db := setupPostConsumeModeDB(t, mode)
			enableGrokViolationFeeForTest(t)
			oldLogConsume := common.LogConsumeEnabled
			common.LogConsumeEnabled = false
			t.Cleanup(func() { common.LogConsumeEnabled = oldLogConsume })
			user, token := seedAuthoritativeBilling(t, db, "violation-"+string(mode), 1000, 500, false)
			info := authoritativeRelay(user, token, "violation-"+string(mode))
			info.StartTime = time.Now().Add(-time.Second)
			info.PriceData.GroupRatioInfo.GroupRatio = 1
			info.UserQuota = 1 << 30 // 避免触发异步低额度通知
			info.ChannelMeta = &relaycommon.ChannelMeta{}

			gin.SetMode(gin.TestMode)
			ginCtx, _ := gin.CreateTestContext(httptest.NewRecorder())

			charged := ChargeViolationFeeIfNeeded(ginCtx, info, newViolationFeeAPIError())
			require.True(t, charged)
			userQuota, remain, used := loadPostConsumeBalances(t, db, user, token)
			assert.Equal(t, 1000-expectedFee, userQuota)
			assert.Equal(t, 500-expectedFee, remain)
			assert.Equal(t, expectedFee, used)

			if mode == model.QuotaWriterModeLegacy {
				userReceipts, tokenReceipts := countReceiptsWithEventKey(t, db, "violation-fee:"+info.RequestId)
				assert.Zero(t, userReceipts)
				assert.Zero(t, tokenReceipts)
				return
			}
			userReceipts, tokenReceipts := countReceiptsWithEventKey(t, db, "violation-fee:"+info.RequestId)
			assert.EqualValues(t, 1, userReceipts)
			assert.EqualValues(t, 1, tokenReceipts)

			// 同一请求重复触发违规费：同键幂等，不重复扣减。
			charged = ChargeViolationFeeIfNeeded(ginCtx, info, newViolationFeeAPIError())
			require.True(t, charged)
			userQuota, remain, used = loadPostConsumeBalances(t, db, user, token)
			assert.Equal(t, 1000-expectedFee, userQuota)
			assert.Equal(t, 500-expectedFee, remain)
			assert.Equal(t, expectedFee, used)
		})
	}
}

func TestPostConsumeQuotaBridgeFailsClosed(t *testing.T) {
	db := setupPostConsumeModeDB(t, model.QuotaWriterModeBridge)
	user, token := seedAuthoritativeBilling(t, db, "bridge-closed", 1000, 500, false)
	info := authoritativeRelay(user, token, "bridge-closed")

	// 已迁移 caller：bridge 模式 fail-closed。
	_, err := postConsumeQuotaWithEvent(info, 30, 0, false, postConsumeQuotaEvent{Namespace: "billing-settlement", ReasonCode: "billing_settlement"})
	require.ErrorIs(t, err, model.ErrDurableQuotaWriterModeDisabled)

	// 未迁移 caller（无业务键）：保持 legacy 直写，由 model 层守卫 fail-closed。
	_, err = postConsumeQuotaWithResult(info, 30, 0, false)
	require.ErrorIs(t, err, model.ErrLegacyQuotaWriterModeDisabled)

	userQuota, remain, used := loadPostConsumeBalances(t, db, user, token)
	assert.Equal(t, 1000, userQuota)
	assert.Equal(t, 500, remain)
	assert.Zero(t, used)
}

func TestPostConsumeQuotaAuthoritativeFailClosedBoundaries(t *testing.T) {
	t.Run("missing request id", func(t *testing.T) {
		db := setupPostConsumeModeDB(t, model.QuotaWriterModeAuthoritative)
		user, token := seedAuthoritativeBilling(t, db, "no-request-id", 1000, 500, false)
		info := authoritativeRelay(user, token, "no-request-id")
		info.RequestId = "  "

		_, err := postConsumeQuotaWithEvent(info, 30, 0, false, postConsumeQuotaEvent{Namespace: "billing-settlement", ReasonCode: "billing_settlement"})
		require.ErrorIs(t, err, model.ErrAccountQuotaMutationInvalidInput)
		userQuota, remain, used := loadPostConsumeBalances(t, db, user, token)
		assert.Equal(t, 1000, userQuota)
		assert.Equal(t, 500, remain)
		assert.Zero(t, used)
	})

	t.Run("subscription funding has no direct authoritative writer", func(t *testing.T) {
		db := setupPostConsumeModeDB(t, model.QuotaWriterModeAuthoritative)
		user, token := seedAuthoritativeBilling(t, db, "subscription-direct", 1000, 500, false)
		info := authoritativeRelay(user, token, "subscription-direct")
		info.BillingSource = BillingSourceSubscription
		info.SubscriptionId = 1

		_, err := postConsumeQuotaWithEvent(info, 30, 0, false, postConsumeQuotaEvent{Namespace: "violation-fee", ReasonCode: "violation_fee"})
		require.ErrorIs(t, err, model.ErrDurableQuotaWriterModeDisabled)
		userQuota, remain, used := loadPostConsumeBalances(t, db, user, token)
		assert.Equal(t, 1000, userQuota)
		assert.Equal(t, 500, remain)
		assert.Zero(t, used)
	})
}

func TestLegacyTokenRollbackUsesStableEventKey(t *testing.T) {
	db := setupPostConsumeModeDB(t, model.QuotaWriterModeLegacy)
	user, token := seedAuthoritativeBilling(t, db, "token-rollback-key", 1000, 500, false)
	info := authoritativeRelay(user, token, "token-rollback-key")
	info.ForcePreConsume = true

	callbackKey := "test:token-rollback-stable-key"
	funding := &failingLegacyFunding{db: db, failure: ErrInsufficientWalletQuota, callbackKey: callbackKey}
	session := &BillingSession{relayInfo: info, funding: funding, writerMode: model.QuotaWriterModeLegacy}
	oldRefund := legacyTokenQuotaRefund
	legacyTokenQuotaRefund = func(int, string, int) error { return errors.New("injected immediate token rollback failure") }
	t.Cleanup(func() { legacyTokenQuotaRefund = oldRefund })

	gin.SetMode(gin.TestMode)
	ginCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	apiErr := session.preConsume(ginCtx, 100)
	require.NotNil(t, apiErr)
	assert.Equal(t, 100, session.tokenConsumed)
	assert.True(t, session.refundRecoveryScheduled)

	// 补偿事实使用固定业务键，不含时间戳。
	var facts []model.AccountQuotaRefundFact
	require.NoError(t, db.Where("request_id = ?", info.RequestId).Find(&facts).Error)
	require.Len(t, facts, 1)
	assert.Equal(t, "billing-token-rollback:"+info.RequestId, facts[0].EventKey)
	assert.Equal(t, model.AccountQuotaRefundFactRetryable, facts[0].State)
	_, remain, used := loadPostConsumeBalances(t, db, user, token)
	assert.Equal(t, 400, remain)
	assert.Equal(t, 100, used)

	// 恢复重放：同一事实只应用一次；重复跑恢复不再产生退回。
	require.NoError(t, db.Callback().Update().Remove(callbackKey))
	processed, err := model.RunAccountQuotaRefundFacts(context.Background(), db, "token-rollback-worker", 10)
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	_, remain, used = loadPostConsumeBalances(t, db, user, token)
	assert.Equal(t, 500, remain)
	assert.Zero(t, used)

	processed, err = model.RunAccountQuotaRefundFacts(context.Background(), db, "token-rollback-worker", 10)
	require.NoError(t, err)
	assert.Zero(t, processed)
	_, remain, used = loadPostConsumeBalances(t, db, user, token)
	assert.Equal(t, 500, remain)
	assert.Zero(t, used)
	var factCount int64
	require.NoError(t, db.Model(&model.AccountQuotaRefundFact{}).Where("request_id = ?", info.RequestId).Count(&factCount).Error)
	assert.EqualValues(t, 1, factCount)
}

func TestLegacyTokenRollbackWithoutRequestIDFailsClosed(t *testing.T) {
	db := setupPostConsumeModeDB(t, model.QuotaWriterModeLegacy)
	user, token := seedAuthoritativeBilling(t, db, "token-rollback-no-request", 1000, 500, false)
	info := authoritativeRelay(user, token, "token-rollback-no-request")
	info.RequestId = ""
	info.ForcePreConsume = true

	callbackKey := "test:token-rollback-no-request-id"
	funding := &failingLegacyFunding{db: db, failure: ErrInsufficientWalletQuota, callbackKey: callbackKey}
	session := &BillingSession{relayInfo: info, funding: funding, writerMode: model.QuotaWriterModeLegacy}
	oldRefund := legacyTokenQuotaRefund
	legacyTokenQuotaRefund = func(int, string, int) error { return errors.New("injected immediate token rollback failure") }
	t.Cleanup(func() { legacyTokenQuotaRefund = oldRefund })

	gin.SetMode(gin.TestMode)
	ginCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	apiErr := session.preConsume(ginCtx, 100)
	require.NotNil(t, apiErr)
	require.ErrorIs(t, apiErr, model.ErrAccountQuotaMutationInvalidInput)
	assert.Equal(t, 100, session.tokenConsumed, "缺少稳定请求身份时不得编造不稳定键，token 扣减保持可见")
	assert.True(t, session.refundRecoveryScheduled)

	// 不写入任何无法幂等重放的补偿事实。
	var factCount int64
	require.NoError(t, db.Model(&model.AccountQuotaRefundFact{}).Count(&factCount).Error)
	assert.Zero(t, factCount)
	_, remain, used := loadPostConsumeBalances(t, db, user, token)
	assert.Equal(t, 400, remain)
	assert.Equal(t, 100, used)
}

func TestPreConsumeRejectsNonLegacyWriterMode(t *testing.T) {
	for _, mode := range []model.QuotaWriterMode{model.QuotaWriterModeAuthoritative, model.QuotaWriterModeBridge} {
		t.Run(string(mode), func(t *testing.T) {
			db := setupPostConsumeModeDB(t, model.QuotaWriterModeLegacy)
			user, token := seedAuthoritativeBilling(t, db, "preconsume-guard-"+string(mode), 1000, 500, false)
			info := authoritativeRelay(user, token, "preconsume-guard-"+string(mode))
			session := &BillingSession{relayInfo: info, funding: &WalletFunding{userId: user.Id}, writerMode: mode}

			gin.SetMode(gin.TestMode)
			ginCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
			apiErr := session.preConsume(ginCtx, 100)
			require.NotNil(t, apiErr)
			require.ErrorIs(t, apiErr, model.ErrDurableQuotaWriterModeDisabled)
			userQuota, remain, used := loadPostConsumeBalances(t, db, user, token)
			assert.Equal(t, 1000, userQuota)
			assert.Equal(t, 500, remain)
			assert.Zero(t, used)
		})
	}
}
