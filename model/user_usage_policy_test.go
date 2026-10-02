package model

import (
	"context"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSelfUsePolicyPreservesQuotaAndRequiresAuditedRootChange(t *testing.T) {
	db := setupTokenBudgetDB(t)
	require.NoError(t, db.AutoMigrate(&UserUsagePolicyChange{}))
	require.NoError(t, db.Model(&User{}).Where("id = ?", 2).Updates(map[string]any{"quota": 0, "used_quota": 75}).Error)
	input := UserUsagePolicyInput{ID: strings.Repeat("d", 64), UserID: 2, NoBalance: true}
	_, err := ConfigureUserUsagePolicy(context.Background(), db, 2, input)
	require.Error(t, err)
	view, err := ConfigureUserUsagePolicy(context.Background(), db, 1, input)
	require.NoError(t, err)
	assert.True(t, view.NoBalance)
	assert.EqualValues(t, 1, view.Revision)
	replay, err := ConfigureUserUsagePolicy(context.Background(), db, 1, input)
	require.NoError(t, err)
	assert.Equal(t, view, replay)
	input.ID = strings.Repeat("e", 64)
	_, err = ConfigureUserUsagePolicy(context.Background(), db, 1, input)
	require.ErrorIs(t, err, ErrUserUsagePolicyConflict)
	var user User
	require.NoError(t, db.First(&user, 2).Error)
	assert.Zero(t, user.Quota)
	assert.Equal(t, 75, user.UsedQuota)
	var count int64
	require.NoError(t, db.Model(&UserUsagePolicyChange{}).Count(&count).Error)
	assert.EqualValues(t, 1, count)
	owner, err := ReadUserUsagePolicy(context.Background(), db, 2, 2)
	require.NoError(t, err)
	assert.True(t, owner.NoBalance)
	_, err = ReadUserUsagePolicy(context.Background(), db, 2, 1)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	var injected User
	require.NoError(t, common.UnmarshalJsonStr(`{"self_use_no_balance":true,"SelfUseNoBalance":true,"usage_policy_revision":99}`, &injected))
	assert.False(t, injected.SelfUseNoBalance)
	assert.Zero(t, injected.UsagePolicyRevision)
}

func TestSelfUseAdmissionRequiresValidDisabledFundingState(t *testing.T) {
	db := setupTokenBudgetDB(t)
	require.NoError(t, db.AutoMigrate(&Option{}))
	var user User
	require.NoError(t, db.First(&user, 2).Error)
	eligible, err := selfUseNoBalanceAdmissionTx(db, &user)
	require.NoError(t, err)
	assert.False(t, eligible, "old users keep their explicit user allowance")
	user.SelfUseNoBalance, user.UsagePolicyRevision = true, 1
	var state UserFundingStateSnapshot
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		var err error
		state, err = InitializeUserFundingStateTx(tx, operation_setting.UserFundingModeDisabled)
		return err
	}))
	eligible, err = selfUseNoBalanceAdmissionTx(db, &user)
	require.NoError(t, err)
	assert.True(t, eligible)
	raw, err := encodeUserFundingState(UserFundingStateSnapshot{Mode: operation_setting.UserFundingModeEnabled, Epoch: state.Epoch + 1})
	require.NoError(t, err)
	require.NoError(t, db.Model(&Option{}).Where("key = ?", UserFundingStateOptionKey).Update("value", raw).Error)
	// Updating only the private row is not a valid published transition. An
	// inconsistent projection must fail closed, not infer disabled mode.
	eligible, err = selfUseNoBalanceAdmissionTx(db, &user)
	assert.False(t, eligible)
	assert.Error(t, err)
}
