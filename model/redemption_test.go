package model

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSearchRedemptionsFiltersAndPaginates(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&Redemption{}))
	require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(&Redemption{}).Error)
	t.Cleanup(func() {
		require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(&Redemption{}).Error)
	})

	now := common.GetTimestamp()
	redemptions := []Redemption{
		{Id: 1, Name: "alpha-active", Key: "00000000000000000000000000000001", Status: common.RedemptionCodeStatusEnabled, ExpiredTime: 0},
		{Id: 2, Name: "alpha-future", Key: "00000000000000000000000000000002", Status: common.RedemptionCodeStatusEnabled, ExpiredTime: now + 3600},
		{Id: 3, Name: "alpha-expired", Key: "00000000000000000000000000000003", Status: common.RedemptionCodeStatusEnabled, ExpiredTime: now - 10},
		{Id: 4, Name: "beta-disabled", Key: "00000000000000000000000000000004", Status: common.RedemptionCodeStatusDisabled, ExpiredTime: 0},
		{Id: 5, Name: "beta-used", Key: "00000000000000000000000000000005", Status: common.RedemptionCodeStatusUsed, ExpiredTime: 0},
	}
	require.NoError(t, DB.Create(&redemptions).Error)

	tests := []struct {
		name      string
		keyword   string
		status    string
		startIdx  int
		num       int
		wantTotal int64
		wantIds   []int
	}{
		{
			name:      "no filters returns all rows",
			num:       10,
			wantTotal: 5,
			wantIds:   []int{5, 4, 3, 2, 1},
		},
		{
			name:      "keyword filters by name prefix",
			keyword:   "alpha",
			num:       10,
			wantTotal: 3,
			wantIds:   []int{3, 2, 1},
		},
		{
			name:      "enabled status excludes expired rows",
			status:    "1",
			num:       10,
			wantTotal: 2,
			wantIds:   []int{2, 1},
		},
		{
			name:      "expired status returns enabled expired rows",
			status:    "expired",
			num:       10,
			wantTotal: 1,
			wantIds:   []int{3},
		},
		{
			name:      "disabled status",
			status:    "2",
			num:       10,
			wantTotal: 1,
			wantIds:   []int{4},
		},
		{
			name:      "used status",
			status:    "3",
			num:       10,
			wantTotal: 1,
			wantIds:   []int{5},
		},
		{
			name:      "pagination keeps unpaged total",
			startIdx:  1,
			num:       2,
			wantTotal: 5,
			wantIds:   []int{4, 3},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows, total, err := SearchRedemptions(tt.keyword, tt.status, tt.startIdx, tt.num)
			require.NoError(t, err)
			assert.Equal(t, tt.wantTotal, total)
			gotIds := make([]int, 0, len(rows))
			for _, row := range rows {
				gotIds = append(gotIds, row.Id)
			}
			assert.Equal(t, tt.wantIds, gotIds)
		})
	}
}

func setupRedeemFixture(t *testing.T, quota int) (userId int, key string) {
	t.Helper()
	require.NoError(t, DB.AutoMigrate(&Redemption{}, &Option{}))
	require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(&Redemption{}).Error)
	oldFunding := operation_setting.GetUserFundingSetting()
	var fundingState UserFundingStateSnapshot
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		var err error
		fundingState, err = InitializeUserFundingStateTx(tx, operation_setting.UserFundingModeEnabled)
		return err
	}))
	require.NoError(t, PublishUserFundingState(fundingState))
	t.Cleanup(func() {
		require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(&Redemption{}).Error)
		DB.Exec("DELETE FROM quota_projection_obligations WHERE receipt_kind = ?", "user")
		DB.Exec("DELETE FROM user_quota_mutation_receipts WHERE mutation_type = ?", "redemption")
		DB.Exec("DELETE FROM users")
		DB.Exec("DELETE FROM logs")
		DB.Exec("DELETE FROM options WHERE `key` IN (?, ?, ?)", UserFundingStateOptionKey, operation_setting.UserFundingModeOptionKey, operation_setting.UserFundingEpochOptionKey)
		require.NoError(t, operation_setting.PublishUserFundingSnapshot(oldFunding.Mode, oldFunding.Epoch))
	})

	user := &User{Username: "redeem-user", Password: "password", Status: common.UserStatusEnabled, Quota: 0}
	require.NoError(t, DB.Create(user).Error)

	key = "10000000000000000000000000000001"
	redemption := &Redemption{
		Name:        "redeem-test",
		Key:         key,
		Status:      common.RedemptionCodeStatusEnabled,
		Quota:       quota,
		CreatedTime: common.GetTimestamp(),
	}
	require.NoError(t, DB.Create(redemption).Error)
	return user.Id, key
}

func TestRedeemCreditsQuotaExactlyOnce(t *testing.T) {
	userId, key := setupRedeemFixture(t, 500)

	quota, err := Redeem(key, userId)
	require.NoError(t, err)
	assert.Equal(t, 500, quota)

	var user User
	require.NoError(t, DB.First(&user, "id = ?", userId).Error)
	assert.Equal(t, 500, user.Quota)

	var redemption Redemption
	require.NoError(t, DB.First(&redemption, "name = ?", "redeem-test").Error)
	assert.Equal(t, common.RedemptionCodeStatusUsed, redemption.Status)
	assert.Equal(t, userId, redemption.UsedUserId)

	// Redeeming the same code again must fail and must not credit quota.
	_, err = Redeem(key, userId)
	require.Error(t, err)
	require.NoError(t, DB.First(&user, "id = ?", userId).Error)
	assert.Equal(t, 500, user.Quota)
}

// Exactly one of several concurrent redeems of the same code may win, and
// quota must be credited exactly once.
func TestRedeemConcurrentSingleSuccess(t *testing.T) {
	userId, key := setupRedeemFixture(t, 300)

	const goroutines = 5
	successes := make([]bool, goroutines)
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			if _, err := Redeem(key, userId); err == nil {
				successes[idx] = true
			}
		}(i)
	}
	wg.Wait()

	successCount := 0
	for _, ok := range successes {
		if ok {
			successCount++
		}
	}
	assert.Equal(t, 1, successCount, "exactly one concurrent redeem should succeed")

	var user User
	require.NoError(t, DB.First(&user, "id = ?", userId).Error)
	assert.Equal(t, 300, user.Quota, "quota must be credited exactly once")
}

func TestAuthoritativeRedeemPersistsStableReceiptAndValidatesReplay(t *testing.T) {
	userId, key := setupRedeemFixture(t, 500)
	setQuotaWriterStateForTest(t, DB, QuotaWriterModeAuthoritative, 311)
	quota, err := Redeem(key, userId)
	require.NoError(t, err)
	assert.Equal(t, 500, quota)
	quota, err = Redeem(key, userId)
	require.NoError(t, err)
	assert.Equal(t, 500, quota)

	var redemption Redemption
	require.NoError(t, DB.Where("key = ?", key).First(&redemption).Error)
	var receipts []UserQuotaMutationReceipt
	require.NoError(t, DB.Where("business_event_key = ?", fmt.Sprintf("redemption:%d", redemption.Id)).Find(&receipts).Error)
	require.Len(t, receipts, 1)
	assert.Equal(t, "redemption", receipts[0].MutationType)
	assert.EqualValues(t, 500, receipts[0].Delta)
	var user User
	require.NoError(t, DB.First(&user, userId).Error)
	assert.Equal(t, 500, user.Quota)
	assert.EqualValues(t, 1, user.QuotaVersion)

	other := &User{Username: "redeem-other", AffCode: "redeem-other-aff", Password: "password", Status: common.UserStatusEnabled}
	require.NoError(t, DB.Create(other).Error)
	_, err = Redeem(key, other.Id)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrUserQuotaMutationConflict))

	require.NoError(t, DB.Model(&Redemption{}).Where("id = ?", redemption.Id).Update("quota", 501).Error)
	_, err = Redeem(key, userId)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrUserQuotaMutationConflict))
	require.NoError(t, DB.First(&user, userId).Error)
	assert.Equal(t, 500, user.Quota)
}

func TestAuthoritativeRedeemReceiptFailureRollsBackCodeAndQuota(t *testing.T) {
	userId, key := setupRedeemFixture(t, 250)
	setQuotaWriterStateForTest(t, DB, QuotaWriterModeAuthoritative, 312)
	const callbackName = "test:redemption_receipt_failure"
	require.NoError(t, DB.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if _, ok := tx.Statement.Dest.(*UserQuotaMutationReceipt); ok {
			tx.AddError(errors.New("receipt unavailable"))
		}
	}))
	t.Cleanup(func() { _ = DB.Callback().Create().Remove(callbackName) })
	_, err := Redeem(key, userId)
	require.Error(t, err)
	var redemption Redemption
	require.NoError(t, DB.Where("key = ?", key).First(&redemption).Error)
	assert.Equal(t, common.RedemptionCodeStatusEnabled, redemption.Status)
	assert.Zero(t, redemption.UsedUserId)
	var user User
	require.NoError(t, DB.First(&user, userId).Error)
	assert.Zero(t, user.Quota)
}
