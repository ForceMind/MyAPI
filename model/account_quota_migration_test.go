package model

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type legacyAccountQuotaMutationReceipt struct {
	ID                 int64                        `gorm:"primaryKey"`
	ReceiptVersion     int                          `gorm:"not null"`
	WriterEpoch        int64                        `gorm:"type:bigint;not null"`
	RequestID          string                       `gorm:"type:varchar(64);not null;index:idx_account_quota_request,priority:1"`
	Phase              string                       `gorm:"type:varchar(16);not null;index:idx_account_quota_request,priority:2"`
	EventKey           string                       `gorm:"type:varchar(128);not null;uniqueIndex:uidx_account_quota_event"`
	MutationSlot       string                       `gorm:"type:varchar(128);not null;uniqueIndex:uidx_account_quota_slot"`
	ParentReceiptID    int64                        `gorm:"type:bigint;not null;default:0;index"`
	AuditKey           string                       `gorm:"type:varchar(96);not null;default:''"`
	RequestFingerprint string                       `gorm:"type:char(64);not null"`
	BillingSource      string                       `gorm:"type:varchar(32);not null"`
	BillingPreference  string                       `gorm:"type:varchar(32);not null"`
	UserID             int                          `gorm:"not null;index"`
	TokenID            int                          `gorm:"not null;index"`
	SubscriptionID     int                          `gorm:"not null;default:0;index"`
	RequestedQuota     int64                        `gorm:"type:bigint;not null"`
	AppliedQuota       int64                        `gorm:"type:bigint;not null"`
	RequestedDeltas    AccountQuotaDeltas           `gorm:"type:text"`
	AppliedDeltas      AccountQuotaDeltas           `gorm:"type:text"`
	Before             QuotaMutationAccountSnapshot `gorm:"type:text"`
	After              QuotaMutationAccountSnapshot `gorm:"type:text"`
	BillingContext     AccountBillingContext        `gorm:"type:text"`
	CreatedAt          int64                        `gorm:"type:bigint;not null;index"`
}

func TestAccountQuotaAndBalanceMigrationsUseBoundedKeysetBatches(t *testing.T) {
	db := openB2SubmissionSQLite(t)
	for _, table := range []interface{}{&AccountQuotaTerminalRecoveryObligation{}, &AccountQuotaReservationHead{}, &AccountQuotaMutationReceipt{}, &AccountQuotaSettlementIntent{}, &AccountQuotaSettlementFact{}, &QuotaBalanceBatchDrain{}, &QuotaBalanceBatchSubject{}} {
		require.NoError(t, db.Migrator().DropTable(table))
	}
	require.NoError(t, db.AutoMigrate(&legacyAccountQuotaMutationReceipt{}, &legacyQuotaBalanceBatchDrain{}))
	const rowCount = 450
	receipts := make([]legacyAccountQuotaMutationReceipt, 0, rowCount)
	oldApplied := make([]legacyQuotaBalanceBatchDrain, 0, rowCount)
	pending := make([]legacyQuotaBalanceBatchDrain, 0, rowCount)
	for index := 0; index < rowCount; index++ {
		requestID := fmt.Sprintf("bounded-%03d", index)
		receipts = append(receipts, legacyAccountQuotaMutationReceipt{
			ReceiptVersion: AccountQuotaMutationReceiptVersion, WriterEpoch: 9, RequestID: requestID, Phase: AccountQuotaPhaseReserve,
			EventKey: accountQuotaEventKey(requestID, AccountQuotaPhaseReserve), MutationSlot: accountQuotaMutationSlot(requestID, AccountQuotaPhaseReserve),
			RequestFingerprint: fmt.Sprintf("%064x", index+1), BillingSource: "wallet", BillingPreference: "wallet_only",
			UserID: index + 1, TokenID: index + 1, RequestedQuota: 1, AppliedQuota: 1, CreatedAt: int64(index + 1),
		})
		oldApplied = append(oldApplied, legacyQuotaBalanceBatchDrain{
			SchemaVersion: 1, GenerationKey: fmt.Sprintf("old-applied-%03d", index), WriterEpoch: 9,
			Payload: QuotaBalanceBatchPayload{UserQuota: map[int]int{index + 1: -1}}, State: quotaBalanceBatchDrainApplied,
			CreatedAt: 1, UpdatedAt: 1,
		})
		pending = append(pending, legacyQuotaBalanceBatchDrain{
			SchemaVersion: 1, GenerationKey: fmt.Sprintf("pending-%03d", index), WriterEpoch: 9,
			Payload: QuotaBalanceBatchPayload{UserQuota: map[int]int{index + 1: -1}}, State: quotaBalanceBatchDrainPending,
			CreatedAt: int64(index + 2), UpdatedAt: int64(index + 2),
		})
	}
	require.NoError(t, db.CreateInBatches(&receipts, 100).Error)
	require.NoError(t, db.CreateInBatches(&oldApplied, 100).Error)
	require.NoError(t, db.CreateInBatches(&pending, 100).Error)
	require.NoError(t, db.AutoMigrate(&AccountQuotaMutationReceipt{}, &AccountQuotaReservationHead{}, &AccountQuotaTerminalRecoveryObligation{}, &AccountQuotaSettlementIntent{}, &AccountQuotaSettlementFact{}, &QuotaBalanceBatchDrain{}, &QuotaBalanceBatchSubject{}, &QuotaWorkCursor{}))
	maxAccountBatch := 0
	accountQuotaMigrationBatchHook = func(size int) {
		if size > maxAccountBatch {
			maxAccountBatch = size
		}
	}
	maxGenerationBatch := 0
	quotaBalanceMigrationBatchHook = func(size int) {
		if size > maxGenerationBatch {
			maxGenerationBatch = size
		}
	}
	t.Cleanup(func() {
		accountQuotaMigrationBatchHook = nil
		quotaBalanceMigrationBatchHook = nil
	})
	require.NoError(t, InitializeAccountQuotaReservationHeadsWithDB(db))
	require.NoError(t, InitializeQuotaBalanceBatchDrainsWithDB(db))
	assert.LessOrEqual(t, maxAccountBatch, accountQuotaMigrationBatchSize)
	assert.Equal(t, accountQuotaMigrationBatchSize, maxAccountBatch)
	assert.LessOrEqual(t, maxGenerationBatch, quotaBalanceCleanupBatchSize)
	assert.Equal(t, quotaBalanceCleanupBatchSize, maxGenerationBatch)
	var headCount, oldAppliedCount, pendingCount int64
	require.NoError(t, db.Model(&AccountQuotaReservationHead{}).Count(&headCount).Error)
	require.NoError(t, db.Model(&QuotaBalanceBatchDrain{}).Where("generation_key LIKE ?", "old-applied-%").Count(&oldAppliedCount).Error)
	require.NoError(t, db.Model(&QuotaBalanceBatchDrain{}).Where("generation_key LIKE ?", "pending-%").Count(&pendingCount).Error)
	assert.EqualValues(t, rowCount, headCount)
	assert.Zero(t, oldAppliedCount)
	assert.EqualValues(t, rowCount, pendingCount)
}

func (legacyAccountQuotaMutationReceipt) TableName() string { return "account_quota_mutation_receipts" }

type legacyQuotaBalanceBatchDrain struct {
	ID            int64                    `gorm:"primaryKey"`
	SchemaVersion int                      `gorm:"not null"`
	GenerationKey string                   `gorm:"type:varchar(96);not null;uniqueIndex"`
	WriterEpoch   int64                    `gorm:"type:bigint;not null"`
	Payload       QuotaBalanceBatchPayload `gorm:"type:text"`
	State         string                   `gorm:"type:varchar(16);not null;index"`
	Attempts      int                      `gorm:"not null"`
	LastError     string                   `gorm:"type:text;not null"`
	CreatedAt     int64                    `gorm:"type:bigint;not null"`
	UpdatedAt     int64                    `gorm:"type:bigint;not null"`
}

func (legacyQuotaBalanceBatchDrain) TableName() string { return "quota_balance_batch_drains" }

func runAccountQuotaConfiguredSchemaContract(t *testing.T, db *gorm.DB) {
	t.Helper()
	for _, table := range []interface{}{&AccountQuotaTerminalRecoveryObligation{}, &AccountQuotaReservationHead{}, &AccountQuotaMutationReceipt{}, &AccountQuotaSettlementIntent{}, &AccountQuotaSettlementFact{}, &QuotaBalanceBatchDrain{}, &QuotaBalanceBatchSubject{}} {
		require.NoError(t, db.Migrator().DropTable(table))
	}
	require.NoError(t, db.AutoMigrate(&legacyAccountQuotaMutationReceipt{}, &legacyQuotaBalanceBatchDrain{}))
	fingerprint := "a000000000000000000000000000000000000000000000000000000000000001"
	legacyReceipt := legacyAccountQuotaMutationReceipt{
		ReceiptVersion: AccountQuotaMutationReceiptVersion, WriterEpoch: 9, RequestID: "migration-account", Phase: AccountQuotaPhaseReserve,
		EventKey: accountQuotaEventKey("migration-account", AccountQuotaPhaseReserve), MutationSlot: accountQuotaMutationSlot("migration-account", AccountQuotaPhaseReserve),
		RequestFingerprint: fingerprint, BillingSource: "wallet", BillingPreference: "wallet_only", UserID: 10, TokenID: 20,
		RequestedQuota: 5, AppliedQuota: 5, RequestedDeltas: AccountQuotaDeltas{UserQuota: -5, TokenRemainQuota: -5, TokenUsedQuota: 5},
		AppliedDeltas:  AccountQuotaDeltas{UserQuota: -5, TokenRemainQuota: -5, TokenUsedQuota: 5},
		BillingContext: AccountBillingContext{Version: 1, ModelName: "migration-model", Metadata: map[string]string{"key": "value"}}, CreatedAt: 100,
	}
	require.NoError(t, db.Create(&legacyReceipt).Error)
	legacyAdjust := legacyReceipt
	legacyAdjust.ID = 0
	legacyAdjust.Phase = AccountQuotaPhaseAdjust
	legacyAdjust.EventKey = "migration-account:adjust"
	legacyAdjust.MutationSlot = "migration-account:adjust"
	legacyAdjust.ParentReceiptID = legacyReceipt.ID
	legacyAdjust.RequestFingerprint = "b000000000000000000000000000000000000000000000000000000000000002"
	legacyAdjust.RequestedQuota = 8
	legacyAdjust.AppliedQuota = 8
	legacyAdjust.CreatedAt = 101
	require.NoError(t, db.Create(&legacyAdjust).Error)
	legacySettle := legacyAdjust
	legacySettle.ID = 0
	legacySettle.Phase = AccountQuotaPhaseSettle
	legacySettle.EventKey = accountQuotaEventKey(legacyReceipt.RequestID, AccountQuotaPhaseSettle)
	legacySettle.MutationSlot = accountQuotaMutationSlot(legacyReceipt.RequestID, AccountQuotaPhaseSettle)
	legacySettle.ParentReceiptID = legacyAdjust.ID
	legacySettle.RequestFingerprint = "c000000000000000000000000000000000000000000000000000000000000003"
	legacySettle.RequestedQuota = 7
	legacySettle.AppliedQuota = 7
	legacySettle.CreatedAt = 102
	require.NoError(t, db.Create(&legacySettle).Error)

	legacyRefundRoot := legacyReceipt
	legacyRefundRoot.ID = 0
	legacyRefundRoot.RequestID = "migration-refund"
	legacyRefundRoot.EventKey = accountQuotaEventKey(legacyRefundRoot.RequestID, AccountQuotaPhaseReserve)
	legacyRefundRoot.MutationSlot = accountQuotaMutationSlot(legacyRefundRoot.RequestID, AccountQuotaPhaseReserve)
	legacyRefundRoot.RequestFingerprint = "d000000000000000000000000000000000000000000000000000000000000004"
	legacyRefundRoot.CreatedAt = 103
	require.NoError(t, db.Create(&legacyRefundRoot).Error)
	legacyRefund := legacyRefundRoot
	legacyRefund.ID = 0
	legacyRefund.Phase = AccountQuotaPhaseRefund
	legacyRefund.EventKey = accountQuotaEventKey(legacyRefundRoot.RequestID, AccountQuotaPhaseRefund)
	legacyRefund.MutationSlot = accountQuotaMutationSlot(legacyRefundRoot.RequestID, AccountQuotaPhaseRefund)
	legacyRefund.ParentReceiptID = legacyRefundRoot.ID
	legacyRefund.AuditKey = "migration-refund"
	legacyRefund.RequestFingerprint = "e000000000000000000000000000000000000000000000000000000000000005"
	legacyRefund.RequestedQuota = 0
	legacyRefund.AppliedQuota = 0
	legacyRefund.CreatedAt = 104
	require.NoError(t, db.Create(&legacyRefund).Error)
	legacyDrain := legacyQuotaBalanceBatchDrain{
		SchemaVersion: 1, GenerationKey: "migration-generation", WriterEpoch: 9,
		Payload: QuotaBalanceBatchPayload{UserQuota: map[int]int{10: -5}, TokenQuota: map[int]int{20: -5}},
		State:   quotaBalanceBatchDrainPending, LastError: "", CreatedAt: 100, UpdatedAt: 100,
	}
	require.NoError(t, db.Create(&legacyDrain).Error)

	for range 2 {
		require.NoError(t, db.AutoMigrate(&AccountQuotaMutationReceipt{}, &AccountQuotaReservationHead{}, &AccountQuotaTerminalRecoveryObligation{}, &AccountQuotaSettlementIntent{}, &AccountQuotaSettlementFact{}, &QuotaBalanceBatchDrain{}, &QuotaBalanceBatchSubject{}, &QuotaWorkCursor{}))
		require.NoError(t, InitializeAccountQuotaReservationHeadsWithDB(db))
		require.NoError(t, InitializeQuotaBalanceBatchDrainsWithDB(db))
	}
	migrator := db.Migrator()
	assert.True(t, migrator.HasIndex(&AccountQuotaMutationReceipt{}, "uidx_account_quota_event"))
	assert.True(t, migrator.HasIndex(&AccountQuotaReservationHead{}, "idx_account_quota_reservation_heads_request_id"))
	assert.True(t, migrator.HasIndex(&AccountQuotaTerminalRecoveryObligation{}, "idx_account_quota_terminal_recovery_obligations_request_id"))
	assert.True(t, migrator.HasColumn(&AccountQuotaTerminalRecoveryObligation{}, "ActualQuota"))
	assert.True(t, migrator.HasIndex(&QuotaBalanceBatchDrain{}, "idx_quota_balance_batch_drains_generation_key"))
	assert.True(t, migrator.HasIndex(&QuotaBalanceBatchDrain{}, "idx_quota_balance_subject_preparation"))
	assert.True(t, migrator.HasIndex(&QuotaBalanceBatchSubject{}, "idx_quota_balance_subject_generation"))
	assert.True(t, migrator.HasIndex(&AccountQuotaSettlementFact{}, "idx_account_quota_settlement_facts_event_key"))
	assert.True(t, migrator.HasIndex(&AccountQuotaSettlementIntent{}, "idx_account_quota_settlement_intents_event_key"))
	for _, field := range []string{"ReserveReceiptID", "WriterEpoch", "ActualQuota", "TerminalReceiptID"} {
		assert.True(t, migrator.HasColumn(&AccountQuotaSettlementFact{}, field), "missing authoritative settlement field %s", field)
	}
	assert.True(t, migrator.HasColumn(&QuotaWorkCursor{}, "high_watermark"))

	var receipt AccountQuotaMutationReceipt
	require.NoError(t, db.First(&receipt, legacyReceipt.ID).Error)
	assert.Equal(t, AccountQuotaFingerprintVersion, receipt.FingerprintVersion)
	assert.Equal(t, receipt.RequestFingerprint, receipt.ReserveFingerprint)
	recomputed, err := RecomputeAccountQuotaReceiptFingerprint(&receipt)
	require.NoError(t, err)
	assert.Equal(t, receipt.RequestFingerprint, recomputed)
	assert.Equal(t, "migration-model", receipt.BillingContext.ModelName)
	assert.Equal(t, "value", receipt.BillingContext.Metadata["key"])
	assert.EqualValues(t, -5, receipt.AppliedDeltas.UserQuota)
	var head AccountQuotaReservationHead
	require.NoError(t, db.Where("request_id = ?", legacyReceipt.RequestID).First(&head).Error)
	assert.Equal(t, receipt.ID, head.RootReceiptID)
	assert.Equal(t, legacyAdjust.ID, head.CurrentReceiptID)
	assert.Equal(t, legacySettle.ID, head.TerminalReceiptID)
	assert.EqualValues(t, 1, head.LockVersion)
	var lifecycle AccountQuotaTerminalRecoveryObligation
	require.NoError(t, db.Where("request_id = ?", legacyReceipt.RequestID).First(&lifecycle).Error)
	assert.Equal(t, AccountQuotaTerminalRecoveryApplied, lifecycle.State)
	assert.Equal(t, legacyAdjust.ID, lifecycle.ReserveReceiptID)
	assert.Equal(t, legacySettle.ID, lifecycle.TerminalReceiptID)
	var chain []AccountQuotaMutationReceipt
	require.NoError(t, db.Where("request_id IN ?", []string{legacyReceipt.RequestID, legacyRefundRoot.RequestID}).Order("id ASC").Find(&chain).Error)
	require.Len(t, chain, 5)
	for index := range chain {
		assert.Equal(t, AccountQuotaFingerprintVersion, chain[index].FingerprintVersion)
		fingerprint, fingerprintErr := RecomputeAccountQuotaReceiptFingerprint(&chain[index])
		require.NoError(t, fingerprintErr)
		assert.Equal(t, chain[index].RequestFingerprint, fingerprint)
	}
	var refundLifecycle AccountQuotaTerminalRecoveryObligation
	require.NoError(t, db.Where("request_id = ?", legacyRefundRoot.RequestID).First(&refundLifecycle).Error)
	assert.Equal(t, AccountQuotaTerminalRecoveryApplied, refundLifecycle.State)
	assert.Equal(t, legacyRefund.ID, refundLifecycle.TerminalReceiptID)
	var drain QuotaBalanceBatchDrain
	require.NoError(t, db.First(&drain, legacyDrain.ID).Error)
	assert.Len(t, drain.PayloadFingerprint, 64)
	assert.True(t, drain.CacheApplied)
	assert.EqualValues(t, 1, drain.LockVersion)
	assert.Equal(t, quotaBalanceBatchDrainSchemaVersion, drain.SchemaVersion)
	assert.Equal(t, -5, drain.Payload.UserQuota[10])
	assert.Equal(t, -5, drain.Payload.TokenQuota[20])

	duplicateHead := head
	duplicateHead.ID = 0
	require.Error(t, db.Create(&duplicateHead).Error)
	duplicateDrain := drain
	duplicateDrain.ID = 0
	require.Error(t, db.Create(&duplicateDrain).Error)
	recovery := AccountQuotaTerminalRecoveryObligation{
		RequestID: "migration-recovery", ReserveReceiptID: receipt.ID, Phase: AccountQuotaPhaseRefund, AuditKey: "migration",
		WriterEpoch: receipt.WriterEpoch, RequestFingerprint: fingerprint, State: AccountQuotaTerminalRecoveryPending,
		LockVersion: 1, CreatedAt: 100, UpdatedAt: 100,
	}
	require.NoError(t, db.Create(&recovery).Error)
	duplicateRecovery := recovery
	duplicateRecovery.ID = 0
	require.Error(t, db.Create(&duplicateRecovery).Error)
	require.ErrorIs(t, db.Model(&receipt).Update("applied_quota", 6).Error, ErrAccountQuotaReceiptImmutable)
}

func TestAccountQuotaConfiguredSchemaSQLite(t *testing.T) {
	db := openB2SubmissionSQLite(t)
	runAccountQuotaConfiguredSchemaContract(t, db)
}

func TestAccountQuotaConfiguredSchemaMySQLAndPostgres(t *testing.T) {
	if os.Getenv("MYAPI_B2_DATABASE_TESTS") != "1" {
		t.Skip("disposable B2 database tests require MYAPI_B2_DATABASE_TESTS=1")
	}
	for _, engine := range []struct{ name, env string }{{"mysql", "MYAPI_B2_MYSQL_DSN"}, {"postgres", "MYAPI_B2_POSTGRES_DSN"}} {
		t.Run(engine.name, func(t *testing.T) {
			dsn := os.Getenv(engine.env)
			require.NotEmpty(t, dsn)
			dialector, err := b2SubmissionDatabaseDialector(engine.name, dsn)
			require.NoError(t, err)
			db, err := gorm.Open(dialector, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			require.NoError(t, err)
			runAccountQuotaConfiguredSchemaContract(t, db)
		})
	}
}

func TestMigratedV1ReserveReplaysExactlyAfterResponseLoss(t *testing.T) {
	db := openAccountQuotaTestDB(t)
	fixture := newAccountQuotaFixture(t, db, "v1-replay", 1000, 500, false, 0, 0, false)
	input := accountReserveInput(fixture, "v1-replay", "wallet_only", 100)
	legacyFingerprint, err := accountQuotaFingerprint(accountQuotaFingerprintPayload{
		Version: 1, RequestID: input.RequestID, Phase: AccountQuotaPhaseReserve,
		UserID: input.UserID, TokenID: input.TokenID, RequestedQuota: input.RequestedQuota,
		Preference: input.BillingPreference, Playground: input.Playground, BillingContext: input.BillingContext,
	})
	require.NoError(t, err)
	before := quotaMutationSnapshot(&fixture.User, &fixture.Token, nil)
	after, err := accountQuotaAfter(before, accountQuotaRequestedDeltas("wallet", 100, false), 100)
	require.NoError(t, err)
	legacy := AccountQuotaMutationReceipt{
		ReceiptVersion: AccountQuotaMutationReceiptVersion, FingerprintVersion: 1, WriterEpoch: 7,
		RequestID: input.RequestID, Phase: AccountQuotaPhaseReserve, EventKey: accountQuotaEventKey(input.RequestID, AccountQuotaPhaseReserve),
		MutationSlot: accountQuotaMutationSlot(input.RequestID, AccountQuotaPhaseReserve), RequestFingerprint: legacyFingerprint,
		BillingSource: "wallet", BillingPreference: input.BillingPreference, UserID: input.UserID, TokenID: input.TokenID,
		RequestedQuota: 100, AppliedQuota: 100, RequestedDeltas: accountQuotaRequestedDeltas("wallet", 100, false),
		AppliedDeltas: accountQuotaRequestedDeltas("wallet", 100, false), Before: before, After: after,
		BillingContext: input.BillingContext, CreatedAt: 100,
	}
	require.NoError(t, db.Session(&gorm.Session{SkipHooks: true}).Create(&legacy).Error)
	require.NoError(t, db.Model(&QuotaWorkCursor{}).Where("name = ?", quotaWorkCursorAccountMigration).Updates(map[string]any{
		"complete": false, "last_id": int64(0),
	}).Error)
	require.NoError(t, db.Model(&User{}).Where("id = ?", fixture.User.Id).Updates(map[string]interface{}{"quota": 900, "quota_version": 1}).Error)
	require.NoError(t, db.Model(&Token{}).Where("id = ?", fixture.Token.Id).Updates(map[string]interface{}{"remain_quota": 400, "used_quota": 100, "quota_version": 1}).Error)
	require.NoError(t, InitializeAccountQuotaReservationHeadsWithDB(db))
	var migrated AccountQuotaMutationReceipt
	require.NoError(t, db.First(&migrated, legacy.ID).Error)
	assert.Equal(t, AccountQuotaFingerprintVersion, migrated.FingerprintVersion)
	recomputed, err := RecomputeAccountQuotaReceiptFingerprint(&migrated)
	require.NoError(t, err)
	assert.Equal(t, migrated.RequestFingerprint, recomputed)
	accountQuotaTransactionAfterCommitHook = func(string) error { return errors.New("reserve response lost") }
	t.Cleanup(func() { accountQuotaTransactionAfterCommitHook = nil })
	replayed, err := ReserveAccountQuota(context.Background(), db, input)
	require.NoError(t, err)
	require.NotNil(t, replayed)
	assert.Equal(t, legacy.ID, replayed.ID)
	var receiptCount int64
	require.NoError(t, db.Model(&AccountQuotaMutationReceipt{}).Where("request_id = ?", input.RequestID).Count(&receiptCount).Error)
	assert.EqualValues(t, 1, receiptCount)
}

func TestQuotaMigrationsPersistCursorAtRunBudget(t *testing.T) {
	db := openB2SubmissionSQLite(t)
	for _, table := range []interface{}{&AccountQuotaTerminalRecoveryObligation{}, &AccountQuotaRefundFact{}, &AccountQuotaReservationHead{}, &AccountQuotaMutationReceipt{}, &QuotaBalanceBatchDrain{}, &QuotaBalanceBatchSubject{}, &QuotaWorkCursor{}} {
		require.NoError(t, db.Migrator().DropTable(table))
	}
	require.NoError(t, db.AutoMigrate(&legacyAccountQuotaMutationReceipt{}, &legacyQuotaBalanceBatchDrain{}))
	const rows = accountQuotaMigrationRunBudget + 1
	receipts := make([]legacyAccountQuotaMutationReceipt, 0, rows)
	generations := make([]legacyQuotaBalanceBatchDrain, 0, rows)
	for index := 0; index < rows; index++ {
		requestID := fmt.Sprintf("budget-%04d", index)
		receipts = append(receipts, legacyAccountQuotaMutationReceipt{
			ReceiptVersion: AccountQuotaMutationReceiptVersion, WriterEpoch: 9, RequestID: requestID, Phase: AccountQuotaPhaseReserve,
			EventKey: accountQuotaEventKey(requestID, AccountQuotaPhaseReserve), MutationSlot: accountQuotaMutationSlot(requestID, AccountQuotaPhaseReserve),
			RequestFingerprint: fmt.Sprintf("%064x", index+1), BillingSource: "wallet", BillingPreference: "wallet_only",
			UserID: index + 1, TokenID: index + 1, RequestedQuota: 1, AppliedQuota: 1, CreatedAt: int64(index + 1),
		})
		generations = append(generations, legacyQuotaBalanceBatchDrain{
			SchemaVersion: 1, GenerationKey: fmt.Sprintf("budget-generation-%04d", index), WriterEpoch: 9,
			Payload: QuotaBalanceBatchPayload{UserQuota: map[int]int{index + 1: -1}}, State: quotaBalanceBatchDrainPending,
			CreatedAt: int64(index + 1), UpdatedAt: int64(index + 1),
		})
	}
	require.NoError(t, db.CreateInBatches(&receipts, 100).Error)
	require.NoError(t, db.CreateInBatches(&generations, 100).Error)
	require.NoError(t, db.AutoMigrate(&AccountQuotaMutationReceipt{}, &AccountQuotaReservationHead{}, &AccountQuotaTerminalRecoveryObligation{}, &AccountQuotaRefundFact{}, &QuotaBalanceBatchDrain{}, &QuotaBalanceBatchSubject{}, &QuotaWorkCursor{}))
	require.ErrorIs(t, InitializeAccountQuotaReservationHeadsWithDB(db), ErrQuotaWorkIncomplete)
	var accountCursor QuotaWorkCursor
	require.NoError(t, db.Where("name = ?", quotaWorkCursorAccountMigration).First(&accountCursor).Error)
	assert.EqualValues(t, accountQuotaMigrationRunBudget, accountCursor.LastID)
	assert.False(t, accountCursor.Complete)
	require.NoError(t, InitializeAccountQuotaReservationHeadsWithDB(db))
	require.NoError(t, db.Where("name = ?", quotaWorkCursorAccountMigration).First(&accountCursor).Error)
	assert.True(t, accountCursor.Complete)

	require.ErrorIs(t, InitializeQuotaBalanceBatchDrainsWithDB(db), ErrQuotaWorkIncomplete)
	var balanceCursor QuotaWorkCursor
	require.NoError(t, db.Where("name = ?", quotaWorkCursorBalanceMigration).First(&balanceCursor).Error)
	assert.False(t, balanceCursor.Complete)
	require.NoError(t, InitializeQuotaBalanceBatchDrainsWithDB(db))
	require.NoError(t, db.Where("name = ?", quotaWorkCursorBalanceMigration).First(&balanceCursor).Error)
	assert.True(t, balanceCursor.Complete)
}

func TestQuotaBackfillInitializationIsStartupBoundedAndAuthoritativeGateFailsClosed(t *testing.T) {
	db := openB2SubmissionSQLite(t)
	require.NoError(t, db.AutoMigrate(&legacyAccountQuotaMutationReceipt{}))
	receipts := make([]legacyAccountQuotaMutationReceipt, 0, accountQuotaMigrationRunBudget+1)
	for index := 0; index <= accountQuotaMigrationRunBudget; index++ {
		requestID := fmt.Sprintf("startup-bounded-%04d", index)
		receipts = append(receipts, legacyAccountQuotaMutationReceipt{
			ReceiptVersion: AccountQuotaMutationReceiptVersion, WriterEpoch: 1, RequestID: requestID, Phase: AccountQuotaPhaseReserve,
			EventKey: accountQuotaEventKey(requestID, AccountQuotaPhaseReserve), MutationSlot: accountQuotaMutationSlot(requestID, AccountQuotaPhaseReserve),
			RequestFingerprint: fmt.Sprintf("%064x", index+1), BillingSource: "wallet", BillingPreference: "wallet_only",
			UserID: 1, TokenID: 1, RequestedQuota: 1, AppliedQuota: 1, CreatedAt: int64(index + 1),
		})
	}
	require.NoError(t, db.CreateInBatches(&receipts, 100).Error)
	require.NoError(t, db.AutoMigrate(
		&AccountQuotaMutationReceipt{}, &AccountQuotaReservationHead{}, &AccountQuotaTerminalRecoveryObligation{},
		&QuotaBalanceBatchDrain{}, &QuotaBalanceBatchSubject{}, &QuotaWorkCursor{}, &QuotaWriterEpoch{}, &QuotaProjectionObligation{},
	))
	accountQuotaMigrationBatchHook = func(int) { t.Fatal("startup cursor initialization must not run backfill batches") }
	t.Cleanup(func() { accountQuotaMigrationBatchHook = nil })
	require.NoError(t, EnsureQuotaWriterEpochStateWithDB(db))
	var cursor QuotaWorkCursor
	require.NoError(t, db.Where("name = ?", quotaWorkCursorAccountMigration).First(&cursor).Error)
	assert.False(t, cursor.Complete)
	assert.Zero(t, cursor.LastID)
	require.NoError(t, db.Model(&QuotaWriterEpoch{}).Where("id = ?", 1).Update("mode", string(QuotaWriterModeAuthoritative)).Error)
	complete, err := QuotaMaintenanceBackfillsComplete(context.Background(), db)
	require.NoError(t, err)
	assert.False(t, complete)
	audit, err := CanEnableDurableQuotaWrites(context.Background(), db)
	require.NoError(t, err)
	assert.False(t, audit.CanEnable)
	assert.False(t, audit.MaintenanceBackfillDone)
	assert.Contains(t, audit.MissingOrFailedChecks, "maintenance_backfill")
}
