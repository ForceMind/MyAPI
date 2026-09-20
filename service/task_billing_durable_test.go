package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/model"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupTaskBillingModeDB builds an isolated per-mode fixture on top of
// setupPostConsumeModeDB, adding the task/MJ/log tables and pointing LOG_DB at
// the same database so billing logs are assertable per test.
func setupTaskBillingModeDB(t *testing.T, mode model.QuotaWriterMode) *gorm.DB {
	t.Helper()
	db := setupPostConsumeModeDB(t, mode)
	require.NoError(t, db.AutoMigrate(&model.Task{}, &model.Midjourney{}, &model.Log{}, &model.BillingLogProjectionIdentity{}))
	oldLogDB := model.LOG_DB
	model.LOG_DB = db
	t.Cleanup(func() { model.LOG_DB = oldLogDB })
	return db
}

func getSettlementFact(t *testing.T, db *gorm.DB, eventKey string) *model.AccountQuotaSettlementFact {
	t.Helper()
	var fact model.AccountQuotaSettlementFact
	require.NoError(t, db.Where("event_key = ?", eventKey).First(&fact).Error)
	return &fact
}

func countSettlementFacts(t *testing.T, db *gorm.DB, eventKey string) int64 {
	t.Helper()
	var count int64
	require.NoError(t, db.Model(&model.AccountQuotaSettlementFact{}).Where("event_key = ?", eventKey).Count(&count).Error)
	return count
}

// ===========================================================================
// Task legacy refund facts ("task-refund:{taskID}")
// ===========================================================================

func TestRefundTaskQuotaLegacyFactIdempotent(t *testing.T) {
	db := setupTaskBillingModeDB(t, model.QuotaWriterModeLegacy)
	ctx := context.Background()

	user, token := seedAuthoritativeBilling(t, db, "task-refund-fact", 10000, 5000, false)
	seedChannel(t, 61)
	const preConsumed = 3000
	seedChargedAccounting(t, user.Id, 61, token.Id, preConsumed, 1)

	task := makeTask(user.Id, 61, preConsumed, token.Id, BillingSourceWallet, 0)
	task.TaskID = "task-refund-fact-1"
	require.NoError(t, db.Create(task).Error)
	stale := *task

	require.True(t, RefundTaskQuota(ctx, task, "task failed: upstream error"))
	assert.Equal(t, 10000+preConsumed, getUserQuota(t, user.Id))
	assert.Equal(t, 5000+preConsumed, getTokenRemainQuota(t, token.Id))
	assert.Zero(t, getTokenUsedQuota(t, token.Id))
	usedQuota, requestCount := getUserUsageAccounting(t, user.Id)
	assert.Zero(t, usedQuota)
	assert.Equal(t, 1, requestCount)
	assert.Zero(t, getChannelUsedQuota(t, 61))
	assert.Zero(t, getTaskQuota(t, task.ID))
	assert.Equal(t, int64(1), countLogs(t))

	fact := getSettlementFact(t, db, "task-refund:task-refund-fact-1")
	assert.Equal(t, model.AccountQuotaSettlementApplied, fact.State)
	assert.Equal(t, model.AccountQuotaSettlementKindLegacyWallet, fact.Kind)
	assert.EqualValues(t, -preConsumed, fact.Delta)

	// 重复轮询携带过期的 task.Quota：事实已 applied，按重放处理，不多退。
	require.True(t, RefundTaskQuota(ctx, &stale, "duplicate poll"))
	assert.Equal(t, 10000+preConsumed, getUserQuota(t, user.Id))
	assert.Equal(t, 5000+preConsumed, getTokenRemainQuota(t, token.Id))
	usedQuota, _ = getUserUsageAccounting(t, user.Id)
	assert.Zero(t, usedQuota)
	assert.Zero(t, getChannelUsedQuota(t, 61))
	assert.Equal(t, int64(1), countLogs(t))
	assert.Equal(t, int64(1), countSettlementFacts(t, db, "task-refund:task-refund-fact-1"))
	assert.Zero(t, getTaskQuota(t, task.ID))
}

func TestRefundTaskQuotaLegacyFactClearFailureRetryNoDoubleRefund(t *testing.T) {
	db := setupTaskBillingModeDB(t, model.QuotaWriterModeLegacy)
	ctx := context.Background()

	user, token := seedAuthoritativeBilling(t, db, "task-refund-clear-fail", 8000, 4000, false)
	seedChannel(t, 62)
	const preConsumed = 1500
	seedChargedAccounting(t, user.Id, 62, token.Id, preConsumed, 1)

	task := makeTask(user.Id, 62, preConsumed, token.Id, BillingSourceWallet, 0)
	task.TaskID = "task-refund-clear-fail-1"
	require.NoError(t, db.Create(task).Error)

	// 注入清零失败：退款事实已应用，但 task.Quota 回写失败。
	require.NoError(t, db.Exec(`
		CREATE TRIGGER fail_task_quota_clear
		BEFORE UPDATE ON tasks
		BEGIN
			SELECT RAISE(ABORT, 'forced task quota clear failure');
		END;
	`).Error)

	require.True(t, RefundTaskQuota(ctx, task, "task failed"))
	assert.Equal(t, 8000+preConsumed, getUserQuota(t, user.Id))
	assert.Equal(t, preConsumed, getTaskQuota(t, task.ID), "清零失败时 DB 标记保留")

	require.NoError(t, db.Exec("DROP TRIGGER IF EXISTS fail_task_quota_clear").Error)

	// 重试：事实重放，不重复退款，仅补齐清零。
	var reloaded model.Task
	require.NoError(t, db.First(&reloaded, task.ID).Error)
	require.True(t, RefundTaskQuota(ctx, &reloaded, "retry after clear failure"))
	assert.Equal(t, 8000+preConsumed, getUserQuota(t, user.Id))
	assert.Equal(t, 4000+preConsumed, getTokenRemainQuota(t, token.Id))
	assert.Zero(t, getTokenUsedQuota(t, token.Id))
	usedQuota, _ := getUserUsageAccounting(t, user.Id)
	assert.Zero(t, usedQuota, "统计列只在事实首次应用时回减一次")
	assert.Zero(t, getChannelUsedQuota(t, 62))
	assert.Equal(t, int64(1), countLogs(t))
	assert.Zero(t, getTaskQuota(t, task.ID))
	assert.Equal(t, int64(1), countSettlementFacts(t, db, "task-refund:task-refund-clear-fail-1"))
}

func TestRefundTaskQuotaLegacyFactSubscriptionIdempotent(t *testing.T) {
	db := setupTaskBillingModeDB(t, model.QuotaWriterModeLegacy)
	ctx := context.Background()

	user, token := seedAuthoritativeBilling(t, db, "task-refund-sub", 0, 8000, false)
	seedChannel(t, 63)
	const preConsumed = 2000
	const subTotal, subUsed int64 = 100000, 50000
	seedSubscription(t, 63, user.Id, subTotal, subUsed)
	seedChargedAccounting(t, user.Id, 63, token.Id, preConsumed, 1)

	task := makeTask(user.Id, 63, preConsumed, token.Id, BillingSourceSubscription, 63)
	task.TaskID = "task-refund-sub-1"
	require.NoError(t, db.Create(task).Error)
	stale := *task

	require.True(t, RefundTaskQuota(ctx, task, "subscription task failed"))
	assert.Equal(t, subUsed-int64(preConsumed), getSubscriptionUsed(t, 63))
	assert.Equal(t, 8000+preConsumed, getTokenRemainQuota(t, token.Id))
	assert.Zero(t, getTaskQuota(t, task.ID))

	fact := getSettlementFact(t, db, "task-refund:task-refund-sub-1")
	assert.Equal(t, model.AccountQuotaSettlementKindLegacySubscription, fact.Kind)
	assert.Equal(t, model.AccountQuotaSettlementApplied, fact.State)

	require.True(t, RefundTaskQuota(ctx, &stale, "duplicate refund"))
	assert.Equal(t, subUsed-int64(preConsumed), getSubscriptionUsed(t, 63))
	assert.Equal(t, 8000+preConsumed, getTokenRemainQuota(t, token.Id))
	assert.Equal(t, int64(1), countLogs(t))
	assert.Equal(t, int64(1), countSettlementFacts(t, db, "task-refund:task-refund-sub-1"))
}

func TestRefundTaskQuotaLegacyFactSkipsDeletedToken(t *testing.T) {
	db := setupTaskBillingModeDB(t, model.QuotaWriterModeLegacy)
	ctx := context.Background()

	user, _ := seedAuthoritativeBilling(t, db, "task-refund-no-token", 7000, 3000, false)
	seedChannel(t, 64)
	const preConsumed = 1200
	seedChargedAccounting(t, user.Id, 64, 0, preConsumed, 1)

	task := makeTask(user.Id, 64, preConsumed, 9999, BillingSourceWallet, 0)
	task.TaskID = "task-refund-deleted-token-1"
	require.NoError(t, db.Create(task).Error)
	stale := *task

	// 令牌已删除：与 taskAdjustTokenQuota 一致跳过令牌侧，钱包退款完成。
	require.True(t, RefundTaskQuota(ctx, task, "token deleted mid-task"))
	assert.Equal(t, 7000+preConsumed, getUserQuota(t, user.Id))
	assert.Zero(t, getTaskQuota(t, task.ID))

	fact := getSettlementFact(t, db, "task-refund:task-refund-deleted-token-1")
	assert.False(t, fact.ApplyToken)
	assert.Equal(t, model.AccountQuotaSettlementApplied, fact.State)

	require.True(t, RefundTaskQuota(ctx, &stale, "duplicate refund"))
	assert.Equal(t, 7000+preConsumed, getUserQuota(t, user.Id))
	assert.Equal(t, int64(1), countLogs(t))
}

func TestRefundTaskQuotaBridgeFailsClosed(t *testing.T) {
	db := setupTaskBillingModeDB(t, model.QuotaWriterModeBridge)
	ctx := context.Background()

	user, token := seedAuthoritativeBilling(t, db, "task-refund-bridge", 10000, 5000, false)
	seedChannel(t, 65)
	const preConsumed = 1000
	seedChargedAccounting(t, user.Id, 65, token.Id, preConsumed, 1)

	task := makeTask(user.Id, 65, preConsumed, token.Id, BillingSourceWallet, 0)
	task.TaskID = "task-refund-bridge-1"
	require.NoError(t, db.Create(task).Error)

	// bridge 模式：legacy 直写在 model 层守卫 fail-closed，标记保留待受控切换。
	assert.False(t, RefundTaskQuota(ctx, task, "bridge mode"))
	assert.Equal(t, 10000, getUserQuota(t, user.Id))
	assert.Equal(t, 5000, getTokenRemainQuota(t, token.Id))
	assert.Equal(t, preConsumed, getTaskQuota(t, task.ID))
	assert.Equal(t, int64(0), countLogs(t))
	assert.Zero(t, countSettlementFacts(t, db, "task-refund:task-refund-bridge-1"))
}

// ===========================================================================
// Task legacy settlement facts ("task-settle:{taskID}")
// ===========================================================================

func TestRecalculateTaskQuotaLegacyFactIdempotentSettle(t *testing.T) {
	db := setupTaskBillingModeDB(t, model.QuotaWriterModeLegacy)
	ctx := context.Background()

	user, token := seedAuthoritativeBilling(t, db, "task-settle-fact", 10000, 5000, false)
	seedChannel(t, 66)
	const preConsumed, actualQuota = 5000, 3000
	seedChargedAccounting(t, user.Id, 66, token.Id, preConsumed, 1)

	task := makeTask(user.Id, 66, preConsumed, token.Id, BillingSourceWallet, 0)
	task.TaskID = "task-settle-fact-1"
	require.NoError(t, db.Create(task).Error)

	RecalculateTaskQuota(ctx, task, actualQuota, "adaptor计费调整")
	assert.Equal(t, 10000+(preConsumed-actualQuota), getUserQuota(t, user.Id))
	assert.Equal(t, 5000+(preConsumed-actualQuota), getTokenRemainQuota(t, token.Id))
	assert.Equal(t, actualQuota, getTokenUsedQuota(t, token.Id))
	usedQuota, requestCount := getUserUsageAccounting(t, user.Id)
	assert.Equal(t, actualQuota, usedQuota)
	assert.Equal(t, 1, requestCount)
	assert.Equal(t, int64(actualQuota), getChannelUsedQuota(t, 66))
	assert.Equal(t, int64(1), countLogs(t))

	fact := getSettlementFact(t, db, "task-settle:task-settle-fact-1")
	assert.Equal(t, model.AccountQuotaSettlementApplied, fact.State)
	assert.EqualValues(t, actualQuota-preConsumed, fact.Delta)

	// 重复轮询携带过期预扣值：同键重放，不重复调整，不重复记日志。
	stale := *task
	stale.Quota = preConsumed
	RecalculateTaskQuota(ctx, &stale, actualQuota, "adaptor计费调整")
	assert.Equal(t, 10000+(preConsumed-actualQuota), getUserQuota(t, user.Id))
	assert.Equal(t, 5000+(preConsumed-actualQuota), getTokenRemainQuota(t, token.Id))
	usedQuota, _ = getUserUsageAccounting(t, user.Id)
	assert.Equal(t, actualQuota, usedQuota)
	assert.Equal(t, int64(actualQuota), getChannelUsedQuota(t, 66))
	assert.Equal(t, int64(1), countLogs(t))
	assert.Equal(t, int64(1), countSettlementFacts(t, db, "task-settle:task-settle-fact-1"))
	assert.Equal(t, actualQuota, getTaskQuota(t, task.ID))
}

func TestRecalculateTaskQuotaLegacyFactConflictFailsClosed(t *testing.T) {
	db := setupTaskBillingModeDB(t, model.QuotaWriterModeLegacy)
	ctx := context.Background()

	user, token := seedAuthoritativeBilling(t, db, "task-settle-conflict", 10000, 5000, false)
	seedChannel(t, 67)
	const preConsumed, actualQuota = 5000, 3000
	seedChargedAccounting(t, user.Id, 67, token.Id, preConsumed, 1)

	task := makeTask(user.Id, 67, preConsumed, token.Id, BillingSourceWallet, 0)
	task.TaskID = "task-settle-conflict-1"
	require.NoError(t, db.Create(task).Error)

	RecalculateTaskQuota(ctx, task, actualQuota, "adaptor计费调整")
	balanceAfterFirst := getUserQuota(t, user.Id)

	// 同键不同差额（指纹冲突）：fail-closed，不产生二次调整与新日志。
	stale := *task
	stale.Quota = preConsumed
	RecalculateTaskQuota(ctx, &stale, actualQuota+500, "conflicting settle")
	assert.Equal(t, balanceAfterFirst, getUserQuota(t, user.Id))
	assert.Equal(t, 5000+(preConsumed-actualQuota), getTokenRemainQuota(t, token.Id))
	assert.Equal(t, int64(1), countLogs(t))
	assert.Equal(t, int64(1), countSettlementFacts(t, db, "task-settle:task-settle-conflict-1"))
}

// ===========================================================================
// Midjourney settle ("mj-billing:{mjId}") / refund ("mj-refund:{mjId}")
// ===========================================================================

func seedMidjourneyBillingTask(t *testing.T, userID, tokenID, channelID, quota int, mjID string) (*relaycommon.RelayInfo, *model.Midjourney) {
	t.Helper()
	relayInfo := &relaycommon.RelayInfo{
		UserId:     userID,
		TokenId:    tokenID,
		TokenKey:   "auth-token-mj-" + mjID,
		UserQuota:  1 << 30,
		UsingGroup: "default",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId: channelID,
		},
	}
	task := &model.Midjourney{
		UserId:    userID,
		Action:    "IMAGINE",
		MjId:      mjID,
		ChannelId: channelID,
		Progress:  "0%",
	}
	prepared, err := PrepareMidjourneyTaskBilling(relayInfo, task, quota, true)
	require.NoError(t, err)
	require.True(t, prepared)
	require.NoError(t, task.Insert())
	return relayInfo, task
}

func TestSettleMidjourneyTaskBillingDualMode(t *testing.T) {
	for _, mode := range []model.QuotaWriterMode{model.QuotaWriterModeLegacy, model.QuotaWriterModeAuthoritative} {
		t.Run(string(mode), func(t *testing.T) {
			db := setupTaskBillingModeDB(t, mode)
			user, token := seedAuthoritativeBilling(t, db, "mj-settle-"+string(mode), 10000, 5000, false)
			seedChannel(t, 71)
			const chargedQuota = 3000
			mjID := "mj-settle-" + string(mode)

			relayInfo, task := seedMidjourneyBillingTask(t, user.Id, token.Id, 71, chargedQuota, mjID)
			relayInfo.TokenKey = token.Key

			billed, err := SettleMidjourneyTaskBilling(relayInfo, task, true)
			require.NoError(t, err)
			require.True(t, billed)
			assert.Equal(t, 10000-chargedQuota, getUserQuota(t, user.Id))
			assert.Equal(t, 5000-chargedQuota, getTokenRemainQuota(t, token.Id))
			assert.Equal(t, chargedQuota, getTokenUsedQuota(t, token.Id))
			persisted := getMidjourneyTask(t, task.Id)
			assert.Equal(t, chargedQuota, persisted.Quota)
			assert.Equal(t, token.Id, persisted.TokenId)

			eventKey := "mj-billing:" + mjID
			userReceipts, tokenReceipts := countReceiptsWithEventKey(t, db, eventKey)
			if mode == model.QuotaWriterModeLegacy {
				assert.Zero(t, userReceipts, "legacy 模式不得写入 receipt")
				assert.Zero(t, tokenReceipts, "legacy 模式不得写入 receipt")
				return
			}
			assert.EqualValues(t, 1, userReceipts)
			assert.EqualValues(t, 1, tokenReceipts)

			// 重复提交同一任务计费：同键幂等，不重复扣减。
			billed, err = SettleMidjourneyTaskBilling(relayInfo, task, true)
			require.NoError(t, err)
			require.True(t, billed)
			assert.Equal(t, 10000-chargedQuota, getUserQuota(t, user.Id))
			assert.Equal(t, 5000-chargedQuota, getTokenRemainQuota(t, token.Id))
			userReceipts, tokenReceipts = countReceiptsWithEventKey(t, db, eventKey)
			assert.EqualValues(t, 1, userReceipts)
			assert.EqualValues(t, 1, tokenReceipts)
		})
	}
}

func TestRefundMidjourneyQuotaDualModeIdempotent(t *testing.T) {
	for _, mode := range []model.QuotaWriterMode{model.QuotaWriterModeLegacy, model.QuotaWriterModeAuthoritative} {
		t.Run(string(mode), func(t *testing.T) {
			db := setupTaskBillingModeDB(t, mode)
			ctx := context.Background()
			user, token := seedAuthoritativeBilling(t, db, "mj-refund-"+string(mode), 10000, 5000, false)
			seedChannel(t, 72)
			const chargedQuota = 3000
			mjID := "mj-refund-" + string(mode)

			relayInfo, task := seedMidjourneyBillingTask(t, user.Id, token.Id, 72, chargedQuota, mjID)
			relayInfo.TokenKey = token.Key
			billed, err := SettleMidjourneyTaskBilling(relayInfo, task, true)
			require.NoError(t, err)
			require.True(t, billed)
			seedChargedAccounting(t, user.Id, 72, token.Id, chargedQuota, 1)

			require.True(t, RefundMidjourneyQuota(ctx, task, "构图失败"))
			assert.Equal(t, 10000, getUserQuota(t, user.Id))
			assert.Equal(t, 5000, getTokenRemainQuota(t, token.Id))
			assert.Zero(t, getTokenUsedQuota(t, token.Id))
			usedQuota, requestCount := getUserUsageAccounting(t, user.Id)
			assert.Zero(t, usedQuota)
			assert.Equal(t, 1, requestCount)
			assert.Zero(t, getChannelUsedQuota(t, 72))
			assert.Zero(t, getMidjourneyTask(t, task.Id).Quota)
			assert.Equal(t, int64(1), countLogs(t))

			if mode == model.QuotaWriterModeLegacy {
				fact := getSettlementFact(t, db, "mj-refund:"+mjID)
				assert.Equal(t, model.AccountQuotaSettlementApplied, fact.State)
				assert.Equal(t, model.AccountQuotaSettlementKindLegacyWallet, fact.Kind)
			} else {
				userReceipts, tokenReceipts := countReceiptsWithEventKey(t, db, "mj-refund:"+mjID)
				assert.EqualValues(t, 1, userReceipts)
				assert.EqualValues(t, 1, tokenReceipts)
			}

			// 重复 notify / 过期 task.Quota 重试：最多退一次。
			stale := getMidjourneyTask(t, task.Id)
			stale.Quota = chargedQuota
			require.True(t, RefundMidjourneyQuota(ctx, &stale, "duplicate notify"))
			assert.Equal(t, 10000, getUserQuota(t, user.Id))
			assert.Equal(t, 5000, getTokenRemainQuota(t, token.Id))
			usedQuota, _ = getUserUsageAccounting(t, user.Id)
			assert.Zero(t, usedQuota)
			assert.Zero(t, getChannelUsedQuota(t, 72))
			assert.Equal(t, int64(1), countLogs(t))
			assert.Zero(t, getMidjourneyTask(t, task.Id).Quota)
			if mode == model.QuotaWriterModeLegacy {
				assert.Equal(t, int64(1), countSettlementFacts(t, db, "mj-refund:"+mjID))
			} else {
				userReceipts, tokenReceipts := countReceiptsWithEventKey(t, db, "mj-refund:"+mjID)
				assert.EqualValues(t, 1, userReceipts)
				assert.EqualValues(t, 1, tokenReceipts)
			}
		})
	}
}

func TestRefundMidjourneyQuotaLegacyFactConcurrent(t *testing.T) {
	db := setupTaskBillingModeDB(t, model.QuotaWriterModeLegacy)
	ctx := context.Background()
	user, token := seedAuthoritativeBilling(t, db, "mj-refund-race", 10000, 5000, false)
	seedChannel(t, 73)
	const chargedQuota = 2000
	mjID := "mj-refund-race"

	relayInfo, task := seedMidjourneyBillingTask(t, user.Id, token.Id, 73, chargedQuota, mjID)
	relayInfo.TokenKey = token.Key
	billed, err := SettleMidjourneyTaskBilling(relayInfo, task, true)
	require.NoError(t, err)
	require.True(t, billed)
	seedChargedAccounting(t, user.Id, 73, token.Id, chargedQuota, 1)

	// 并发 poll/notify 各自携带过期 task.Quota：事实状态机收敛，最多退一次。
	const workers = 4
	var wg sync.WaitGroup
	results := make(chan bool, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok := false
			for attempt := 0; attempt < 50 && !ok; attempt++ {
				stale := getMidjourneyTask(t, task.Id)
				stale.Quota = chargedQuota
				ok = RefundMidjourneyQuota(ctx, &stale, "构图失败")
				if !ok {
					time.Sleep(2 * time.Millisecond)
				}
			}
			results <- ok
		}()
	}
	wg.Wait()
	close(results)
	succeeded := 0
	for ok := range results {
		if ok {
			succeeded++
		}
	}
	assert.Equal(t, workers, succeeded)
	assert.Equal(t, 10000, getUserQuota(t, user.Id))
	assert.Equal(t, 5000, getTokenRemainQuota(t, token.Id))
	assert.Zero(t, getTokenUsedQuota(t, token.Id))
	usedQuota, _ := getUserUsageAccounting(t, user.Id)
	assert.Zero(t, usedQuota)
	assert.Zero(t, getChannelUsedQuota(t, 73))
	assert.Equal(t, int64(1), countLogs(t))
	assert.Equal(t, int64(1), countSettlementFacts(t, db, "mj-refund:"+mjID))
	assert.Zero(t, getMidjourneyTask(t, task.Id).Quota)
}

func TestMidjourneyBillingBridgeFailsClosed(t *testing.T) {
	db := setupTaskBillingModeDB(t, model.QuotaWriterModeBridge)
	ctx := context.Background()
	user, token := seedAuthoritativeBilling(t, db, "mj-bridge", 10000, 5000, false)
	seedChannel(t, 74)
	const chargedQuota = 3000

	relayInfo, task := seedMidjourneyBillingTask(t, user.Id, token.Id, 74, chargedQuota, "mj-bridge")
	relayInfo.TokenKey = token.Key

	// settle：bridge 模式 fail-closed，计费标记清除，余额不变。
	billed, err := SettleMidjourneyTaskBilling(relayInfo, task, true)
	require.ErrorIs(t, err, model.ErrDurableQuotaWriterModeDisabled)
	assert.False(t, billed)
	assert.Equal(t, 10000, getUserQuota(t, user.Id))
	assert.Equal(t, 5000, getTokenRemainQuota(t, token.Id))
	persisted := getMidjourneyTask(t, task.Id)
	assert.Zero(t, persisted.Quota)
	assert.Zero(t, persisted.TokenId)

	// refund：bridge 模式 fail-closed，quota 标记保留等待受控切换。
	persisted.Quota = chargedQuota
	require.NoError(t, db.Model(&model.Midjourney{}).Where("id = ?", persisted.Id).Update("quota", chargedQuota).Error)
	assert.False(t, RefundMidjourneyQuota(ctx, &persisted, "bridge mode"))
	assert.Equal(t, 10000, getUserQuota(t, user.Id))
	assert.Equal(t, 5000, getTokenRemainQuota(t, token.Id))
	assert.Equal(t, chargedQuota, getMidjourneyTask(t, task.Id).Quota)
	assert.Equal(t, int64(0), countLogs(t))
}
