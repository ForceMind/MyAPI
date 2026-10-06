package model

import (
	"context"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestUsageReviewProjectionRecoversWithoutDuplicateStatistics(t *testing.T) {
	f, account := setupLegacySettlementFactTest(t, "projection", false)
	db := f.DB
	require.NoError(t, db.AutoMigrate(&UsageReviewDecision{}, &AccountQuotaSettlementIntent{}))
	require.NoError(t, db.Model(&User{}).Where("id = ?", account.User.Id).Updates(map[string]interface{}{"quota": 900, "quota_version": 1}).Error)
	require.NoError(t, db.Model(&Token{}).Where("id = ?", account.Token.Id).Updates(map[string]interface{}{"remain_quota": 400, "used_quota": 100, "quota_version": 1}).Error)
	ctx := context.Background()
	row, err := PrepareLegacyUsageReservation(ctx, db, LegacyUsageReservation{RequestID: "review-projection", UserID: account.User.Id, TokenID: account.Token.Id, FundingSource: "wallet", ReservedQuota: 100, TokenReservedQuota: 100})
	require.NoError(t, err)
	require.NoError(t, UpdateLegacyUsageReservation(ctx, db, row.RequestID, 100, 100, LegacyUsageUnknown, "missing", nil))
	root := User{Username: "projection-root", AffCode: "projection-root", Role: common.RoleRootUser, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&root).Error)
	view, err := ReconcileUsageReview(ctx, db, root.Id, row.RequestID, 120, "synthetic verified statement and frozen price")
	require.NoError(t, err)
	require.NotNil(t, view.Decision)
	logDB, err := gorm.Open(sqlite.Open(t.TempDir()+"/projection-logs.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, err := logDB.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = pool.Close() })
	// Log schema unavailable: primary statistics can commit, but delivery stays pending.
	require.Error(t, ProjectUsageReviewDecision(ctx, db, logDB, view.Decision.ID))
	var user User
	require.NoError(t, db.First(&user, account.User.Id).Error)
	assert.Equal(t, 120, user.UsedQuota)
	assert.Equal(t, 1, user.RequestCount)
	require.NoError(t, logDB.AutoMigrate(&Log{}, &BillingLogProjectionIdentity{}))
	require.NoError(t, ProjectUsageReviewDecision(ctx, db, logDB, view.Decision.ID))
	// Simulate losing only the delivery acknowledgement; the immutable event is unchanged.
	require.NoError(t, db.Table("usage_review_decisions").Where("id = ?", view.Decision.ID).Update("log_projected", false).Error)
	require.NoError(t, ProjectUsageReviewDecision(ctx, db, logDB, view.Decision.ID))
	require.NoError(t, db.First(&user, account.User.Id).Error)
	assert.Equal(t, 120, user.UsedQuota)
	assert.Equal(t, 1, user.RequestCount)
	assert.Equal(t, 880, user.Quota)
	oldLog := LOG_DB
	LOG_DB = logDB
	t.Cleanup(func() { LOG_DB = oldLog })
	logs, total, err := GetUserLogs(account.User.Id, LogTypeConsume, 0, 0, "", "", 0, 10, "", row.RequestID, "")
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	require.Len(t, logs, 1)
	assert.Equal(t, 120, logs[0].Quota)
	assert.Contains(t, logs[0].Other, `"token_counts_confirmed":false`)
	stat, err := SumUsedQuota(LogTypeConsume, 0, 0, "", "", "", 0, "")
	require.NoError(t, err)
	assert.Equal(t, 120, stat.Quota)
}

func TestUsageReviewPreservesIndependentKnownObligations(t *testing.T) {
	metadata := `{"quota_unit":1000,"quota_unit_captured":true,"group_ratio":1,"known_tool_obligations":[{"name":"fixture-tool","count":1,"price":5}]}`
	require.ErrorIs(t, validateReviewedQuotaObligations(metadata, 0), ErrAccountQuotaMutationConflict)
	require.NoError(t, validateReviewedQuotaObligations(metadata, 5))
	require.ErrorIs(t, validateReviewedQuotaObligations(`{"known_realtime_quota":16}`, 15), ErrAccountQuotaMutationConflict)
	require.NoError(t, validateReviewedQuotaObligations(`{"known_realtime_quota":16}`, 16))
	require.ErrorIs(t, validateReviewedQuotaObligations(`{"incomplete_pricing_evidence":true}`, 100), ErrAccountQuotaUsageUnresolved)
}
