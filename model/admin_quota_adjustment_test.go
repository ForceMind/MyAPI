package model

import (
	"fmt"
	"sync"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func adminAdjustmentTestDB(t *testing.T, writerMode QuotaWriterMode, writerEpoch int64) *gorm.DB {
	t.Helper()
	db, _ := creditEdgeTestDB(t, writerMode, writerEpoch)
	require.NoError(t, db.AutoMigrate(&AdminQuotaAdjustment{}))
	return db
}

func getAdminAdjustment(t *testing.T, db *gorm.DB, targetUserId int, requestId string) AdminQuotaAdjustment {
	t.Helper()
	var record AdminQuotaAdjustment
	require.NoError(t, db.Where("target_user_id = ? AND request_id = ?", targetUserId, requestId).First(&record).Error)
	return record
}

func countAdminAdjustments(t *testing.T, db *gorm.DB, targetUserId int) int64 {
	t.Helper()
	var count int64
	require.NoError(t, db.Model(&AdminQuotaAdjustment{}).Where("target_user_id = ?", targetUserId).Count(&count).Error)
	return count
}

func TestAdminAdjustUserQuotaAddSubtractDualMode(t *testing.T) {
	for index, mode := range []QuotaWriterMode{QuotaWriterModeLegacy, QuotaWriterModeAuthoritative} {
		t.Run(string(mode), func(t *testing.T) {
			db := adminAdjustmentTestDB(t, mode, int64(701+index))
			user := createCreditEdgeUser(t, db, "admin-add-"+string(mode), 100, 0)

			adjustment, replayed, err := AdminAdjustUserQuota(1, user.Id, AdminQuotaAdjustmentOperationAdd, 50, "req-add-1")
			require.NoError(t, err)
			assert.False(t, replayed)
			assert.Equal(t, AdminQuotaAdjustmentStatusSucceeded, adjustment.Status)
			assert.EqualValues(t, 50, adjustment.Delta)
			assert.EqualValues(t, 100, adjustment.AppliedQuotaBefore)
			assert.EqualValues(t, 150, adjustment.AppliedQuotaAfter)
			assert.Equal(t, 150, loadUserQuota(t, db, user.Id))

			adjustment, replayed, err = AdminAdjustUserQuota(1, user.Id, AdminQuotaAdjustmentOperationSubtract, 30, "req-sub-1")
			require.NoError(t, err)
			assert.False(t, replayed)
			assert.EqualValues(t, -30, adjustment.Delta)
			assert.EqualValues(t, 150, adjustment.AppliedQuotaBefore)
			assert.EqualValues(t, 120, adjustment.AppliedQuotaAfter)
			assert.Equal(t, 120, loadUserQuota(t, db, user.Id))

			addReceipts := countUserQuotaReceipts(t, db, fmt.Sprintf("admin-adjust:%d:req-add-1", user.Id))
			subReceipts := countUserQuotaReceipts(t, db, fmt.Sprintf("admin-adjust:%d:req-sub-1", user.Id))
			if mode == QuotaWriterModeLegacy {
				assert.Empty(t, addReceipts)
				assert.Empty(t, subReceipts)
			} else {
				require.Len(t, addReceipts, 1)
				assert.Equal(t, "admin_adjust", addReceipts[0].MutationType)
				assert.Equal(t, "admin_adjust_add", addReceipts[0].ReasonCode)
				assert.Equal(t, 1, addReceipts[0].OperatorUserID)
				assert.EqualValues(t, 50, addReceipts[0].Delta)
				require.Len(t, subReceipts, 1)
				assert.Equal(t, "admin_adjust_subtract", subReceipts[0].ReasonCode)
				assert.EqualValues(t, -30, subReceipts[0].Delta)
			}
		})
	}
}

func TestAdminAdjustUserQuotaLegacySubtractMayOverdraw(t *testing.T) {
	// legacy 直写语义保持迁移前行为：减额度不校验下限，允许余额转负。
	db := adminAdjustmentTestDB(t, QuotaWriterModeLegacy, 711)
	user := createCreditEdgeUser(t, db, "admin-overdraw", 100, 0)

	_, _, err := AdminAdjustUserQuota(1, user.Id, AdminQuotaAdjustmentOperationSubtract, 150, "req-overdraw")
	require.NoError(t, err)
	assert.Equal(t, -50, loadUserQuota(t, db, user.Id))
}

func TestAdminAdjustUserQuotaAuthoritativeSubtractInsufficientFails(t *testing.T) {
	// authoritative 内核坚持余额非负不变量：透支减法整体失败且零写。
	db := adminAdjustmentTestDB(t, QuotaWriterModeAuthoritative, 712)
	user := createCreditEdgeUser(t, db, "admin-insufficient", 100, 0)

	_, _, err := AdminAdjustUserQuota(1, user.Id, AdminQuotaAdjustmentOperationSubtract, 150, "req-insufficient")
	require.ErrorIs(t, err, ErrInsufficientUserQuota)
	assert.Equal(t, 100, loadUserQuota(t, db, user.Id))
	assert.Zero(t, countAdminAdjustments(t, db, user.Id))
}

func TestAdminAdjustUserQuotaOverrideDualMode(t *testing.T) {
	for index, mode := range []QuotaWriterMode{QuotaWriterModeLegacy, QuotaWriterModeAuthoritative} {
		t.Run(string(mode), func(t *testing.T) {
			db := adminAdjustmentTestDB(t, mode, int64(721+index))
			user := createCreditEdgeUser(t, db, "admin-override-"+string(mode), 100, 0)

			adjustment, replayed, err := AdminAdjustUserQuota(1, user.Id, AdminQuotaAdjustmentOperationOverride, 500, "req-override-1")
			require.NoError(t, err)
			assert.False(t, replayed)
			assert.EqualValues(t, 500, adjustment.TargetQuota)
			assert.EqualValues(t, 400, adjustment.Delta)
			assert.EqualValues(t, 100, adjustment.AppliedQuotaBefore)
			assert.EqualValues(t, 500, adjustment.AppliedQuotaAfter)
			assert.Equal(t, 500, loadUserQuota(t, db, user.Id))

			// 同 request_id 同目标值重放：零写成功，不产生二次 delta。
			replay, replayed, err := AdminAdjustUserQuota(1, user.Id, AdminQuotaAdjustmentOperationOverride, 500, "req-override-1")
			require.NoError(t, err)
			assert.True(t, replayed)
			assert.Equal(t, adjustment.Id, replay.Id)
			assert.Equal(t, 500, loadUserQuota(t, db, user.Id))
			assert.EqualValues(t, 1, countAdminAdjustments(t, db, user.Id))

			// 同 request_id 不同目标值：冲突。
			_, _, err = AdminAdjustUserQuota(1, user.Id, AdminQuotaAdjustmentOperationOverride, 600, "req-override-1")
			require.ErrorIs(t, err, ErrAdminQuotaAdjustmentConflict)
			assert.Equal(t, 500, loadUserQuota(t, db, user.Id))

			// 向下覆盖（delta 为负）。
			adjustment, _, err = AdminAdjustUserQuota(1, user.Id, AdminQuotaAdjustmentOperationOverride, 40, "req-override-2")
			require.NoError(t, err)
			assert.EqualValues(t, -460, adjustment.Delta)
			assert.Equal(t, 40, loadUserQuota(t, db, user.Id))

			// 覆盖到当前值：delta 为 0，记录 before==after，authoritative 下无 receipt。
			adjustment, _, err = AdminAdjustUserQuota(1, user.Id, AdminQuotaAdjustmentOperationOverride, 40, "req-override-3")
			require.NoError(t, err)
			assert.EqualValues(t, 0, adjustment.Delta)
			assert.EqualValues(t, 40, adjustment.AppliedQuotaBefore)
			assert.EqualValues(t, 40, adjustment.AppliedQuotaAfter)
			assert.Equal(t, 40, loadUserQuota(t, db, user.Id))
			assert.Empty(t, countUserQuotaReceipts(t, db, fmt.Sprintf("admin-adjust:%d:req-override-3", user.Id)))
		})
	}
}

func TestAdminAdjustUserQuotaReplayZeroWriteDualMode(t *testing.T) {
	for index, mode := range []QuotaWriterMode{QuotaWriterModeLegacy, QuotaWriterModeAuthoritative} {
		t.Run(string(mode), func(t *testing.T) {
			db := adminAdjustmentTestDB(t, mode, int64(731+index))
			user := createCreditEdgeUser(t, db, "admin-replay-"+string(mode), 100, 0)

			first, replayed, err := AdminAdjustUserQuota(1, user.Id, AdminQuotaAdjustmentOperationAdd, 50, "req-replay-1")
			require.NoError(t, err)
			assert.False(t, replayed)
			assert.Equal(t, 150, loadUserQuota(t, db, user.Id))

			second, replayed, err := AdminAdjustUserQuota(1, user.Id, AdminQuotaAdjustmentOperationAdd, 50, "req-replay-1")
			require.NoError(t, err)
			assert.True(t, replayed)
			assert.Equal(t, first.Id, second.Id)
			assert.Equal(t, 150, loadUserQuota(t, db, user.Id))
			assert.EqualValues(t, 1, countAdminAdjustments(t, db, user.Id))
			replayReceipts := countUserQuotaReceipts(t, db, fmt.Sprintf("admin-adjust:%d:req-replay-1", user.Id))
			if mode == QuotaWriterModeAuthoritative {
				assert.Len(t, replayReceipts, 1)
			} else {
				assert.Empty(t, replayReceipts)
			}

			// 异参冲突：同 request_id 不同额度。
			_, _, err = AdminAdjustUserQuota(1, user.Id, AdminQuotaAdjustmentOperationAdd, 60, "req-replay-1")
			require.ErrorIs(t, err, ErrAdminQuotaAdjustmentConflict)
			// 异参冲突：同 request_id 不同操作。
			_, _, err = AdminAdjustUserQuota(1, user.Id, AdminQuotaAdjustmentOperationSubtract, 50, "req-replay-1")
			require.ErrorIs(t, err, ErrAdminQuotaAdjustmentConflict)
			assert.Equal(t, 150, loadUserQuota(t, db, user.Id))
		})
	}
}

func TestAdminAdjustUserQuotaConcurrentDuplicateConvergesOnce(t *testing.T) {
	for index, mode := range []QuotaWriterMode{QuotaWriterModeLegacy, QuotaWriterModeAuthoritative} {
		t.Run(string(mode), func(t *testing.T) {
			db := adminAdjustmentTestDB(t, mode, int64(741+index))
			user := createCreditEdgeUser(t, db, "admin-concurrent-"+string(mode), 100, 0)

			const workers = 8
			var wg sync.WaitGroup
			errs := make([]error, workers)
			replays := make([]bool, workers)
			for i := 0; i < workers; i++ {
				wg.Add(1)
				go func(index int) {
					defer wg.Done()
					_, replayed, err := AdminAdjustUserQuota(1, user.Id, AdminQuotaAdjustmentOperationAdd, 50, "req-concurrent-1")
					errs[index] = err
					replays[index] = replayed
				}(i)
			}
			wg.Wait()

			applied := 0
			for i := 0; i < workers; i++ {
				require.NoError(t, errs[i])
				if !replays[i] {
					applied++
				}
			}
			assert.Equal(t, 1, applied, "并发重复提交必须恰好应用一次")
			assert.Equal(t, 150, loadUserQuota(t, db, user.Id))
			assert.EqualValues(t, 1, countAdminAdjustments(t, db, user.Id))
		})
	}
}

func TestAdminAdjustUserQuotaBridgeFailClosed(t *testing.T) {
	db := adminAdjustmentTestDB(t, QuotaWriterModeBridge, 751)
	user := createCreditEdgeUser(t, db, "admin-bridge", 100, 0)

	_, _, err := AdminAdjustUserQuota(1, user.Id, AdminQuotaAdjustmentOperationAdd, 50, "req-bridge-1")
	require.ErrorIs(t, err, ErrDurableQuotaWriterModeDisabled)
	assert.Equal(t, 100, loadUserQuota(t, db, user.Id))
	assert.Zero(t, countAdminAdjustments(t, db, user.Id))

	_, _, err = AdminAdjustUserQuota(1, user.Id, AdminQuotaAdjustmentOperationOverride, 500, "req-bridge-2")
	require.ErrorIs(t, err, ErrDurableQuotaWriterModeDisabled)
	assert.Equal(t, 100, loadUserQuota(t, db, user.Id))
}

func TestAdminAdjustUserQuotaValidation(t *testing.T) {
	db := adminAdjustmentTestDB(t, QuotaWriterModeLegacy, 761)
	user := createCreditEdgeUser(t, db, "admin-validation", 100, 0)

	longRequestId := make([]byte, 65)
	for i := range longRequestId {
		longRequestId[i] = 'x'
	}
	cases := []struct {
		name      string
		operation string
		value     int
		requestId string
	}{
		{"empty request id", AdminQuotaAdjustmentOperationAdd, 10, ""},
		{"overlong request id", AdminQuotaAdjustmentOperationAdd, 10, string(longRequestId)},
		{"invalid operation", "multiply", 10, "req-v-1"},
		{"zero add", AdminQuotaAdjustmentOperationAdd, 0, "req-v-2"},
		{"negative add", AdminQuotaAdjustmentOperationAdd, -10, "req-v-3"},
		{"negative subtract", AdminQuotaAdjustmentOperationSubtract, -10, "req-v-4"},
		{"negative override", AdminQuotaAdjustmentOperationOverride, -1, "req-v-5"},
		{"oversized value", AdminQuotaAdjustmentOperationAdd, common.MaxQuota + 1, "req-v-6"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := AdminAdjustUserQuota(1, user.Id, tc.operation, tc.value, tc.requestId)
			require.Error(t, err)
		})
	}
	assert.Equal(t, 100, loadUserQuota(t, db, user.Id))
	assert.Zero(t, countAdminAdjustments(t, db, user.Id))

	_, _, err := AdminAdjustUserQuota(1, 999999, AdminQuotaAdjustmentOperationAdd, 10, "req-v-ghost")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "目标用户不存在")
}
