package model

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openAccountMigrationResumeDB(t *testing.T, path string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(path+"?_pragma=journal_mode(WAL)"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, registerTaskRecoveryGormGuards(db))
	pool, err := db.DB()
	require.NoError(t, err)
	pool.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	return db
}

func TestAccountQuotaMigrationPreservesCancellationAndDatabaseError(t *testing.T) {
	db := openAccountMigrationResumeDB(t, filepath.Join(t.TempDir(), "cancel-errors.db"))
	require.NoError(t, db.AutoMigrate(&AccountQuotaMutationReceipt{}, &AccountQuotaReservationHead{}, &AccountQuotaTerminalRecoveryObligation{}, &QuotaWorkCursor{}))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("fixture_cancel_and_rollback_error", func(tx *gorm.DB) {
		if tx.Statement.Table == "account_quota_mutation_receipts" {
			cancel()
			// Reproduce GORM's cancellation followed by an already-rolled-back
			// transaction without depending on database/sql goroutine scheduling.
			tx.AddError(context.Canceled)
			tx.AddError(sql.ErrTxDone)
		}
	}))
	err := InitializeAccountQuotaReservationHeadsWithDB(db.WithContext(ctx))
	require.ErrorIs(t, err, context.Canceled)
	require.ErrorIs(t, err, sql.ErrTxDone)
}

func TestAccountQuotaMigrationResumesPartialHeadAfterDatabaseReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "migration.db")
	db := openAccountMigrationResumeDB(t, path)
	require.NoError(t, db.AutoMigrate(&legacyAccountQuotaMutationReceipt{}))
	const rows = 2
	receipts := make([]legacyAccountQuotaMutationReceipt, rows)
	for i := range receipts {
		requestID := fmt.Sprintf("resume-%03d", i)
		receipts[i] = legacyAccountQuotaMutationReceipt{
			ReceiptVersion: AccountQuotaMutationReceiptVersion, WriterEpoch: 9,
			RequestID: requestID, Phase: AccountQuotaPhaseReserve,
			EventKey: accountQuotaEventKey(requestID, AccountQuotaPhaseReserve), MutationSlot: accountQuotaMutationSlot(requestID, AccountQuotaPhaseReserve),
			RequestFingerprint: fmt.Sprintf("%064x", i+1), BillingSource: "wallet", BillingPreference: "wallet_only",
			UserID: i + 1, TokenID: i + 1, RequestedQuota: 1, AppliedQuota: 1, CreatedAt: int64(i + 1),
		}
	}
	require.NoError(t, db.Create(&receipts[0]).Error)
	require.NoError(t, db.AutoMigrate(&AccountQuotaMutationReceipt{}, &AccountQuotaReservationHead{}, &AccountQuotaTerminalRecoveryObligation{}, &QuotaWorkCursor{}))
	require.NoError(t, InitializeAccountQuotaReservationHeadsWithDB(db))
	var cursor QuotaWorkCursor
	require.NoError(t, db.Where("name = ?", quotaWorkCursorAccountMigration).First(&cursor).Error)
	require.True(t, cursor.Complete)
	require.Equal(t, receipts[0].ID, cursor.LastID)
	var firstHead AccountQuotaReservationHead
	require.NoError(t, db.Where("request_id = ?", receipts[0].RequestID).First(&firstHead).Error)

	require.NoError(t, db.Create(&receipts[1]).Error)
	// Model a durable checkpoint for a completed prior page with a later receipt
	// still waiting to be processed.
	cursor.Complete = false
	require.NoError(t, db.Save(&cursor).Error)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	interrupted := false
	lastRequest := receipts[rows-1].RequestID
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("fixture_interrupt_account_migration", func(tx *gorm.DB) {
		obligation, ok := tx.Statement.Dest.(*AccountQuotaTerminalRecoveryObligation)
		if ok && obligation.RequestID == lastRequest && !interrupted {
			interrupted = true
			cancel()
		}
	}))
	require.ErrorIs(t, InitializeAccountQuotaReservationHeadsWithDB(db.WithContext(ctx)), context.Canceled)
	require.True(t, interrupted, "cancel after the final head exists, before its obligation is saved")
	require.NoError(t, db.Where("name = ?", quotaWorkCursorAccountMigration).First(&cursor).Error)
	assert.Equal(t, receipts[rows-2].ID, cursor.LastID)
	assert.False(t, cursor.Complete)
	var partialHead AccountQuotaReservationHead
	require.NoError(t, db.Where("request_id = ?", lastRequest).First(&partialHead).Error)
	var obligations int64
	require.NoError(t, db.Model(&AccountQuotaTerminalRecoveryObligation{}).Count(&obligations).Error)
	assert.EqualValues(t, rows-1, obligations)
	pool, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, pool.Close())

	reopened := openAccountMigrationResumeDB(t, path)
	for range 2 {
		require.NoError(t, InitializeAccountQuotaReservationHeadsWithDB(reopened))
	}
	require.NoError(t, reopened.Where("name = ?", quotaWorkCursorAccountMigration).First(&cursor).Error)
	assert.True(t, cursor.Complete)
	assert.Equal(t, receipts[rows-1].ID, cursor.LastID)
	var heads, migrated int64
	require.NoError(t, reopened.Model(&AccountQuotaReservationHead{}).Count(&heads).Error)
	require.NoError(t, reopened.Model(&AccountQuotaTerminalRecoveryObligation{}).Count(&obligations).Error)
	require.NoError(t, reopened.Model(&AccountQuotaMutationReceipt{}).Where("fingerprint_version = ?", AccountQuotaFingerprintVersion).Count(&migrated).Error)
	assert.EqualValues(t, rows, heads)
	assert.EqualValues(t, rows, obligations)
	assert.EqualValues(t, rows, migrated)
	var reopenedFirstHead AccountQuotaReservationHead
	require.NoError(t, reopened.Where("request_id = ?", receipts[0].RequestID).First(&reopenedFirstHead).Error)
	assert.Equal(t, firstHead.ID, reopenedFirstHead.ID)
	assert.Equal(t, firstHead.ReserveFingerprint, reopenedFirstHead.ReserveFingerprint)
	assert.Equal(t, firstHead.AppliedQuota, reopenedFirstHead.AppliedQuota)
	var finalHead AccountQuotaReservationHead
	require.NoError(t, reopened.Where("request_id = ?", lastRequest).First(&finalHead).Error)
	assert.Equal(t, partialHead.ID, finalHead.ID)
	assert.EqualValues(t, 1, finalHead.AppliedQuota)
	assert.Equal(t, partialHead.ReserveFingerprint, finalHead.ReserveFingerprint)
}

func TestAccountQuotaMigrationReadsTerminalBeyondFullReceiptPage(t *testing.T) {
	db := openB2SubmissionSQLite(t)
	require.NoError(t, db.AutoMigrate(&legacyAccountQuotaMutationReceipt{}))
	const requestID = "migration-long-chain"
	var previousID int64
	for i := 0; i <= accountQuotaMigrationBatchSize; i++ {
		phase := AccountQuotaPhaseAdjust
		if i == 0 {
			phase = AccountQuotaPhaseReserve
		} else if i == accountQuotaMigrationBatchSize {
			phase = AccountQuotaPhaseSettle
		}
		receipt := legacyAccountQuotaMutationReceipt{
			ReceiptVersion: AccountQuotaMutationReceiptVersion, WriterEpoch: 9, RequestID: requestID, Phase: phase,
			EventKey: fmt.Sprintf("chain-event-%03d", i), MutationSlot: fmt.Sprintf("chain-slot-%03d", i), ParentReceiptID: previousID,
			RequestFingerprint: fmt.Sprintf("%064x", i+1), BillingSource: "wallet", BillingPreference: "wallet_only",
			UserID: 1, TokenID: 1, RequestedQuota: 1, AppliedQuota: 1, CreatedAt: int64(i + 1),
		}
		require.NoError(t, db.Create(&receipt).Error)
		previousID = receipt.ID
	}
	require.NoError(t, db.AutoMigrate(&AccountQuotaMutationReceipt{}, &AccountQuotaReservationHead{}, &AccountQuotaTerminalRecoveryObligation{}, &QuotaWorkCursor{}))
	require.NoError(t, InitializeAccountQuotaReservationHeadsWithDB(db))
	var head AccountQuotaReservationHead
	require.NoError(t, db.Where("request_id = ?", requestID).First(&head).Error)
	assert.Equal(t, previousID, head.TerminalReceiptID)
	var obligation AccountQuotaTerminalRecoveryObligation
	require.NoError(t, db.Where("request_id = ?", requestID).First(&obligation).Error)
	assert.Equal(t, AccountQuotaTerminalRecoveryApplied, obligation.State)
	assert.Equal(t, previousID, obligation.TerminalReceiptID)
	assert.EqualValues(t, 1, obligation.ActualQuota)
}

func TestQuotaBalanceMigrationRecognizesPaddedLegacyFingerprintWithoutInferringCurrentCacheState(t *testing.T) {
	for _, version := range []int{1, quotaBalanceBatchDrainSchemaVersion} {
		t.Run(fmt.Sprintf("schema-%d", version), func(t *testing.T) {
			db := openB2SubmissionSQLite(t)
			require.NoError(t, db.AutoMigrate(&QuotaBalanceBatchDrain{}, &QuotaBalanceBatchSubject{}, &QuotaWorkCursor{}))
			generation := QuotaBalanceBatchDrain{
				SchemaVersion: version, GenerationKey: "padded-fingerprint", WriterEpoch: 1,
				Payload:            QuotaBalanceBatchPayload{UserQuota: map[int]int64{10: -5}},
				PayloadFingerprint: strings.Repeat(" ", 64), State: quotaBalanceBatchDrainPending,
				CreatedAt: 1, UpdatedAt: 1,
			}
			require.NoError(t, db.Create(&generation).Error)
			for range 2 {
				require.NoError(t, InitializeQuotaBalanceBatchDrainsWithDB(db))
			}
			var stored QuotaBalanceBatchDrain
			require.NoError(t, db.First(&stored, generation.ID).Error)
			expectedSchema := version
			if version < quotaBalanceBatchDrainLegacySchemaVersion {
				expectedSchema = quotaBalanceBatchDrainLegacySchemaVersion
			}
			assert.Equal(t, expectedSchema, stored.SchemaVersion)
			assert.Equal(t, generation.Payload, stored.Payload)
			if version == 1 {
				fingerprint, err := quotaBalanceBatchPayloadFingerprintForSchema(quotaBalanceBatchDrainLegacySchemaVersion, stored.Payload)
				require.NoError(t, err)
				assert.Equal(t, fingerprint, stored.PayloadFingerprint)
				assert.True(t, stored.CacheApplied)
				require.NoError(t, validateQuotaBalanceGenerationIdentity(&stored, &stored))
			} else {
				assert.False(t, stored.CacheApplied, "already-current rows cannot gain cache-application evidence from padding")
				assert.Equal(t, generation.PayloadFingerprint, stored.PayloadFingerprint)
				require.ErrorIs(t, validateQuotaBalanceGenerationIdentity(&stored, &stored), ErrQuotaBalanceGenerationConflict)
			}
		})
	}
}
