package model

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func configureFixedCheckinAward(t *testing.T, quota int) {
	t.Helper()
	managed, ok := config.GlobalConfig.Get("checkin_setting").(config.MapConfig)
	require.True(t, ok)
	previous, err := managed.ExportConfigMap()
	require.NoError(t, err)
	require.NoError(t, managed.UpdateConfigMap(map[string]string{
		"enabled": "true", "min_quota": fmt.Sprintf("%d", quota), "max_quota": fmt.Sprintf("%d", quota),
	}))
	t.Cleanup(func() { require.NoError(t, managed.UpdateConfigMap(previous)) })
}

func createCheckinUser(t *testing.T, id int) *User {
	t.Helper()
	user := &User{Id: id, Username: fmt.Sprintf("checkin-%d", id), AffCode: fmt.Sprintf("checkin-aff-%d", id), Status: common.UserStatusEnabled}
	require.NoError(t, DB.Create(user).Error)
	t.Cleanup(func() {
		DB.Where("user_id = ?", user.Id).Delete(&Checkin{})
		DB.Exec("DELETE FROM quota_projection_obligations WHERE receipt_kind = ? AND user_id = ?", "user", user.Id)
		DB.Exec("DELETE FROM user_quota_mutation_receipts WHERE user_id = ?", user.Id)
		DB.Delete(&User{}, user.Id)
	})
	return user
}

func TestAuthoritativeCheckinIsAtomicAndReplaySafe(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&Checkin{}))
	configureFixedCheckinAward(t, 50)
	setQuotaWriterStateForTest(t, DB, QuotaWriterModeAuthoritative, 321)
	user := createCheckinUser(t, 2301)

	first, err := UserCheckin(user.Id)
	require.NoError(t, err)
	second, err := UserCheckin(user.Id)
	require.NoError(t, err)
	assert.Equal(t, first.Id, second.Id)
	assert.Equal(t, time.Now().Format("2006-01-02"), first.CheckinDate)
	var reloaded User
	require.NoError(t, DB.First(&reloaded, user.Id).Error)
	assert.Equal(t, 50, reloaded.Quota)
	assert.EqualValues(t, 1, reloaded.QuotaVersion)
	var receipts []UserQuotaMutationReceipt
	require.NoError(t, DB.Where("business_event_key = ?", fmt.Sprintf("checkin:%d:%s", user.Id, first.CheckinDate)).Find(&receipts).Error)
	require.Len(t, receipts, 1)
	assert.Equal(t, "checkin", receipts[0].MutationType)

	require.NoError(t, DB.Model(&Checkin{}).Where("id = ?", first.Id).Update("quota_awarded", 51).Error)
	_, err = UserCheckin(user.Id)
	require.ErrorIs(t, err, ErrUserQuotaMutationConflict)
	require.NoError(t, DB.First(&reloaded, user.Id).Error)
	assert.Equal(t, 50, reloaded.Quota)
}

func TestAuthoritativeCheckinReceiptFailureRollsBackRecordAndQuota(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&Checkin{}))
	configureFixedCheckinAward(t, 40)
	setQuotaWriterStateForTest(t, DB, QuotaWriterModeAuthoritative, 322)
	user := createCheckinUser(t, 2302)
	const callbackName = "test:checkin_receipt_failure"
	require.NoError(t, DB.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if _, ok := tx.Statement.Dest.(*UserQuotaMutationReceipt); ok {
			tx.AddError(errors.New("receipt unavailable"))
		}
	}))
	t.Cleanup(func() { _ = DB.Callback().Create().Remove(callbackName) })
	_, err := UserCheckin(user.Id)
	require.Error(t, err)
	var count int64
	require.NoError(t, DB.Model(&Checkin{}).Where("user_id = ?", user.Id).Count(&count).Error)
	assert.Zero(t, count)
	var reloaded User
	require.NoError(t, DB.First(&reloaded, user.Id).Error)
	assert.Zero(t, reloaded.Quota)
}

func TestLegacyAndBridgeCheckinKeepDuplicateErrorBehavior(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&Checkin{}))
	configureFixedCheckinAward(t, 30)
	for index, mode := range []QuotaWriterMode{QuotaWriterModeLegacy, QuotaWriterModeBridge} {
		t.Run(string(mode), func(t *testing.T) {
			setQuotaWriterStateForTest(t, DB, mode, int64(330+index))
			user := createCheckinUser(t, 2310+index)
			_, err := UserCheckin(user.Id)
			require.NoError(t, err)
			_, err = UserCheckin(user.Id)
			require.Error(t, err)
			var reloaded User
			require.NoError(t, DB.First(&reloaded, user.Id).Error)
			assert.Equal(t, 30, reloaded.Quota)
			var receiptCount int64
			require.NoError(t, DB.Model(&UserQuotaMutationReceipt{}).Where("user_id = ?", user.Id).Count(&receiptCount).Error)
			assert.Zero(t, receiptCount)
		})
	}
}
