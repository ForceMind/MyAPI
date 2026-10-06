package model

import (
	"fmt"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/ForceMind/MyAPI/setting/operation_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// creditEdgeTestDB builds an isolated SQLite fixture with the durable quota
// writer schema and an enabled user-funding state, mirroring the combination
// of userFundingPolicyTestDB and the quota projection fixtures.
func creditEdgeTestDB(t *testing.T, writerMode QuotaWriterMode, writerEpoch int64) (*gorm.DB, UserFundingStateSnapshot) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(
		&Option{}, &User{}, &Log{},
		&InviteRewardGrant{}, &AffQuotaTransfer{},
		&UserQuotaMutationReceipt{}, &QuotaWriterEpoch{}, &QuotaProjectionObligation{}, &QuotaWorkCursor{},
	))
	oldDB, oldLogDB, oldRedis := DB, LOG_DB, common.RedisEnabled
	oldMain, oldLog := common.MainDatabaseType(), common.LogDatabaseType()
	oldFunding := operation_setting.GetUserFundingSetting()
	DB, LOG_DB, common.RedisEnabled = db, db, false
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	require.NoError(t, EnsureQuotaWriterEpochStateWithDB(db))
	setQuotaWriterStateForTest(t, db, writerMode, writerEpoch)
	var state UserFundingStateSnapshot
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		var initErr error
		state, initErr = InitializeUserFundingStateTx(tx, operation_setting.UserFundingModeEnabled)
		return initErr
	}))
	require.NoError(t, PublishUserFundingState(state))
	t.Cleanup(func() {
		DB, LOG_DB, common.RedisEnabled = oldDB, oldLogDB, oldRedis
		common.SetDatabaseTypes(oldMain, oldLog)
		require.NoError(t, operation_setting.PublishUserFundingSnapshot(oldFunding.Mode, oldFunding.Epoch))
		_ = RefreshUserQuotaBusinessSchemaCapability(oldDB)
		require.NoError(t, sqlDB.Close())
	})
	return db, state
}

func setCreditEdgeQuotasForTest(t *testing.T, newUser, invitee, inviter int) {
	t.Helper()
	oldNewUser, oldInvitee, oldInviter := common.QuotaForNewUser, common.QuotaForInvitee, common.QuotaForInviter
	common.QuotaForNewUser, common.QuotaForInvitee, common.QuotaForInviter = newUser, invitee, inviter
	t.Cleanup(func() {
		common.QuotaForNewUser, common.QuotaForInvitee, common.QuotaForInviter = oldNewUser, oldInvitee, oldInviter
	})
}

func confirmPaymentComplianceForCreditTest(t *testing.T) {
	t.Helper()
	managed, ok := config.GlobalConfig.Get("payment_setting").(config.MapConfig)
	require.True(t, ok)
	previous, err := managed.ExportConfigMap()
	require.NoError(t, err)
	require.NoError(t, managed.UpdateConfigMap(map[string]string{
		"compliance_confirmed":     "true",
		"compliance_terms_version": operation_setting.CurrentComplianceTermsVersion,
	}))
	t.Cleanup(func() { require.NoError(t, managed.UpdateConfigMap(previous)) })
}

func createCreditEdgeUser(t *testing.T, db *gorm.DB, username string, quota, affQuota int) User {
	t.Helper()
	user := User{
		Username: username,
		AffCode:  username + "-aff",
		Status:   common.UserStatusEnabled,
		Quota:    quota,
		AffQuota: affQuota,
	}
	require.NoError(t, db.Create(&user).Error)
	return user
}

func countUserQuotaReceipts(t *testing.T, db *gorm.DB, businessEventKey string) []UserQuotaMutationReceipt {
	t.Helper()
	var receipts []UserQuotaMutationReceipt
	require.NoError(t, db.Where("business_event_key = ?", businessEventKey).Find(&receipts).Error)
	return receipts
}

func TestLegacyAffQuotaTransferCreditsWallet(t *testing.T) {
	db, funding := creditEdgeTestDB(t, QuotaWriterModeLegacy, 401)
	user := createCreditEdgeUser(t, db, "aff-legacy", 1000, 1000000)

	require.NoError(t, user.TransferAffQuotaToQuota(500000, "req-legacy-1", funding.Epoch))

	var reloaded User
	require.NoError(t, db.First(&reloaded, user.Id).Error)
	assert.Equal(t, 500000, reloaded.AffQuota)
	assert.Equal(t, 501000, reloaded.Quota)

	var transfer AffQuotaTransfer
	require.NoError(t, db.Where("user_id = ? AND request_id = ?", user.Id, "req-legacy-1").First(&transfer).Error)
	assert.Equal(t, AffQuotaTransferStatusSucceeded, transfer.Status)
	assert.Equal(t, 500000, transfer.Quota)

	var receiptCount int64
	require.NoError(t, db.Model(&UserQuotaMutationReceipt{}).Count(&receiptCount).Error)
	assert.Zero(t, receiptCount)
}

func TestAuthoritativeAffQuotaTransferWritesReceipt(t *testing.T) {
	db, funding := creditEdgeTestDB(t, QuotaWriterModeAuthoritative, 402)
	user := createCreditEdgeUser(t, db, "aff-authoritative", 1000, 1000000)

	require.NoError(t, user.TransferAffQuotaToQuota(500000, "req-auth-1", funding.Epoch))

	var reloaded User
	require.NoError(t, db.First(&reloaded, user.Id).Error)
	assert.Equal(t, 500000, reloaded.AffQuota)
	assert.Equal(t, 501000, reloaded.Quota)
	assert.EqualValues(t, 1, reloaded.QuotaVersion)

	receipts := countUserQuotaReceipts(t, db, fmt.Sprintf("aff_transfer:%d:req-auth-1", user.Id))
	require.Len(t, receipts, 1)
	assert.Equal(t, "aff_transfer", receipts[0].MutationType)
	assert.Equal(t, "aff_transfer_credit", receipts[0].ReasonCode)
	assert.EqualValues(t, 500000, receipts[0].Delta)
	assert.Equal(t, 1000, receipts[0].QuotaBefore)
	assert.Equal(t, 501000, receipts[0].QuotaAfter)

	var transfer AffQuotaTransfer
	require.NoError(t, db.Where("user_id = ? AND request_id = ?", user.Id, "req-auth-1").First(&transfer).Error)
	assert.Equal(t, AffQuotaTransferStatusSucceeded, transfer.Status)
}

func TestAffQuotaTransferRepeatedRequestIdDoesNotMoveQuotaTwice(t *testing.T) {
	db, funding := creditEdgeTestDB(t, QuotaWriterModeAuthoritative, 403)
	user := createCreditEdgeUser(t, db, "aff-retry", 0, 1000000)

	require.NoError(t, user.TransferAffQuotaToQuota(500000, "req-retry-1", funding.Epoch))
	require.NoError(t, user.TransferAffQuotaToQuota(500000, "req-retry-1", funding.Epoch))

	var reloaded User
	require.NoError(t, db.First(&reloaded, user.Id).Error)
	assert.Equal(t, 500000, reloaded.AffQuota)
	assert.Equal(t, 500000, reloaded.Quota)

	var transferCount int64
	require.NoError(t, db.Model(&AffQuotaTransfer{}).Where("user_id = ?", user.Id).Count(&transferCount).Error)
	assert.EqualValues(t, 1, transferCount)
	assert.Len(t, countUserQuotaReceipts(t, db, fmt.Sprintf("aff_transfer:%d:req-retry-1", user.Id)), 1)
}

func TestAffQuotaTransferReusedRequestIdWithDifferentQuotaConflicts(t *testing.T) {
	db, funding := creditEdgeTestDB(t, QuotaWriterModeLegacy, 404)
	user := createCreditEdgeUser(t, db, "aff-conflict", 0, 2000000)

	require.NoError(t, user.TransferAffQuotaToQuota(500000, "req-conflict-1", funding.Epoch))
	err := user.TransferAffQuotaToQuota(1000000, "req-conflict-1", funding.Epoch)
	require.ErrorIs(t, err, ErrAffQuotaTransferConflict)

	var reloaded User
	require.NoError(t, db.First(&reloaded, user.Id).Error)
	assert.Equal(t, 1500000, reloaded.AffQuota)
	assert.Equal(t, 500000, reloaded.Quota)
}

func TestAffQuotaTransferInsufficientAffLeavesNoTransferRecord(t *testing.T) {
	db, funding := creditEdgeTestDB(t, QuotaWriterModeAuthoritative, 405)
	user := createCreditEdgeUser(t, db, "aff-poor", 1000, 500000)

	err := user.TransferAffQuotaToQuota(1000000, "req-poor-1", funding.Epoch)
	require.Error(t, err)

	var reloaded User
	require.NoError(t, db.First(&reloaded, user.Id).Error)
	assert.Equal(t, 500000, reloaded.AffQuota)
	assert.Equal(t, 1000, reloaded.Quota)

	var transferCount int64
	require.NoError(t, db.Model(&AffQuotaTransfer{}).Where("user_id = ?", user.Id).Count(&transferCount).Error)
	assert.Zero(t, transferCount)
	assert.Empty(t, countUserQuotaReceipts(t, db, fmt.Sprintf("aff_transfer:%d:req-poor-1", user.Id)))
}

func TestAffQuotaTransferRejectsInvalidRequestId(t *testing.T) {
	db, funding := creditEdgeTestDB(t, QuotaWriterModeLegacy, 406)
	user := createCreditEdgeUser(t, db, "aff-invalid-request", 0, 1000000)

	require.Error(t, user.TransferAffQuotaToQuota(500000, "", funding.Epoch))
	oversized := make([]byte, 65)
	for i := range oversized {
		oversized[i] = 'x'
	}
	require.Error(t, user.TransferAffQuotaToQuota(500000, string(oversized), funding.Epoch))

	var transferCount int64
	require.NoError(t, db.Model(&AffQuotaTransfer{}).Where("user_id = ?", user.Id).Count(&transferCount).Error)
	assert.Zero(t, transferCount)
}

func TestInviteRewardGrantedOnceAcrossRepeatedFinishInsert(t *testing.T) {
	db, _ := creditEdgeTestDB(t, QuotaWriterModeLegacy, 407)
	setCreditEdgeQuotasForTest(t, 0, 0, 250000)
	confirmPaymentComplianceForCreditTest(t)
	inviter := createCreditEdgeUser(t, db, "inviter-once", 0, 0)
	invitee := createCreditEdgeUser(t, db, "invitee-once", 0, 0)

	invitee.finishInsert(inviter.Id)
	invitee.finishInsert(inviter.Id)

	var reloaded User
	require.NoError(t, db.First(&reloaded, inviter.Id).Error)
	assert.Equal(t, 1, reloaded.AffCount)
	assert.Equal(t, 250000, reloaded.AffQuota)
	assert.Equal(t, 250000, reloaded.AffHistoryQuota)

	var grantCount int64
	require.NoError(t, db.Model(&InviteRewardGrant{}).Where("inviter_id = ? AND invitee_id = ?", inviter.Id, invitee.Id).Count(&grantCount).Error)
	assert.EqualValues(t, 1, grantCount)
}

func TestAuthoritativeInviteeRewardProducesSingleReceipt(t *testing.T) {
	db, _ := creditEdgeTestDB(t, QuotaWriterModeAuthoritative, 408)
	setCreditEdgeQuotasForTest(t, 0, 300000, 0)
	confirmPaymentComplianceForCreditTest(t)
	inviter := createCreditEdgeUser(t, db, "inviter-auth", 0, 0)
	invitee := createCreditEdgeUser(t, db, "invitee-auth", 0, 0)

	invitee.finishInsert(inviter.Id)
	invitee.finishInsert(inviter.Id)

	var reloaded User
	require.NoError(t, db.First(&reloaded, invitee.Id).Error)
	assert.Equal(t, 300000, reloaded.Quota)
	assert.EqualValues(t, 1, reloaded.QuotaVersion)

	receipts := countUserQuotaReceipts(t, db, fmt.Sprintf("invite:invitee:%d", invitee.Id))
	require.Len(t, receipts, 1)
	assert.Equal(t, "invite", receipts[0].MutationType)
	assert.Equal(t, "invite_reward", receipts[0].ReasonCode)
}

func TestAuthoritativeNewUserGrantWritesReceipt(t *testing.T) {
	db, _ := creditEdgeTestDB(t, QuotaWriterModeAuthoritative, 409)
	setCreditEdgeQuotasForTest(t, 500, 0, 0)

	user := User{Username: "new-auth-user"}
	require.NoError(t, user.Insert(0))

	var reloaded User
	require.NoError(t, db.First(&reloaded, user.Id).Error)
	assert.Equal(t, 500, reloaded.Quota)
	assert.EqualValues(t, 1, reloaded.QuotaVersion)

	receipts := countUserQuotaReceipts(t, db, fmt.Sprintf("user_init:%d", user.Id))
	require.Len(t, receipts, 1)
	assert.Equal(t, "system_init", receipts[0].MutationType)
	assert.Equal(t, "new_user_grant", receipts[0].ReasonCode)
	assert.EqualValues(t, 500, receipts[0].Delta)
	assert.Equal(t, 0, receipts[0].QuotaBefore)
	assert.Equal(t, 500, receipts[0].QuotaAfter)
}

func TestLegacyNewUserGrantKeepsDirectAssignment(t *testing.T) {
	db, _ := creditEdgeTestDB(t, QuotaWriterModeLegacy, 410)
	setCreditEdgeQuotasForTest(t, 500, 0, 0)

	user := User{Username: "new-legacy-user"}
	require.NoError(t, user.Insert(0))

	var reloaded User
	require.NoError(t, db.First(&reloaded, user.Id).Error)
	assert.Equal(t, 500, reloaded.Quota)

	var receiptCount int64
	require.NoError(t, db.Model(&UserQuotaMutationReceipt{}).Count(&receiptCount).Error)
	assert.Zero(t, receiptCount)
}
