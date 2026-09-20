package model

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

const (
	quotaWorkCursorAccountMigration = "account_quota_migration_v2"
	quotaWorkCursorBalanceMigration = "quota_balance_migration_v2"
	quotaWorkCursorRefundRecovery   = "account_refund_recovery_v1"
)

var (
	ErrQuotaWorkIncomplete                = errors.New("quota maintenance work remains")
	ErrQuotaMaintenanceBackfillIncomplete = errors.New("quota maintenance backfill is incomplete")
)

type QuotaWorkCursor struct {
	Name          string `json:"name" gorm:"type:varchar(64);primaryKey"`
	LastID        int64  `json:"last_id" gorm:"type:bigint;not null;default:0"`
	HighWatermark int64  `json:"high_watermark" gorm:"type:bigint;not null;default:0"`
	Complete      bool   `json:"complete" gorm:"not null;default:false"`
	UpdatedAt     int64  `json:"updated_at" gorm:"type:bigint;not null;index"`
}

func (QuotaWorkCursor) TableName() string { return "quota_work_cursors" }

func loadQuotaWorkCursor(ctx context.Context, db *gorm.DB, name string) (*QuotaWorkCursor, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
	}
	var cursor QuotaWorkCursor
	result := db.WithContext(ctx).Where("name = ?", name).Limit(1).Find(&cursor)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		cursor = QuotaWorkCursor{Name: name}
	}
	return &cursor, nil
}

func saveQuotaWorkCursor(ctx context.Context, db *gorm.DB, cursor *QuotaWorkCursor) error {
	if db == nil || cursor == nil || cursor.Name == "" {
		return gorm.ErrInvalidDB
	}
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
	}
	cursor.UpdatedAt = time.Now().Unix()
	return db.WithContext(ctx).Save(cursor).Error
}

func beginQuotaWorkCycle(ctx context.Context, db *gorm.DB, cursor *QuotaWorkCursor, tableModel any) error {
	if db == nil || cursor == nil || tableModel == nil {
		return gorm.ErrInvalidDB
	}
	if cursor.HighWatermark > 0 {
		return nil
	}
	var newest struct {
		ID int64 `gorm:"column:id"`
	}
	result := db.WithContext(ctx).Model(tableModel).Select("id").Order("id DESC").Limit(1).Find(&newest)
	if result.Error != nil {
		return result.Error
	}
	cursor.LastID = 0
	cursor.HighWatermark = newest.ID
	return saveQuotaWorkCursor(ctx, db, cursor)
}

func finishQuotaWorkCycle(ctx context.Context, db *gorm.DB, cursor *QuotaWorkCursor) error {
	if cursor == nil {
		return gorm.ErrInvalidDB
	}
	cursor.LastID = 0
	cursor.HighWatermark = 0
	return saveQuotaWorkCursor(ctx, db, cursor)
}

func ensureQuotaWorkCursorInitialized(ctx context.Context, db *gorm.DB, name string, sourceModel any) error {
	cursor, err := loadQuotaWorkCursor(ctx, db, name)
	if err != nil {
		return err
	}
	if cursor.UpdatedAt != 0 {
		return nil
	}
	var row struct {
		ID int64 `gorm:"column:id"`
	}
	result := db.WithContext(ctx).Model(sourceModel).Select("id").Order("id ASC").Limit(1).Find(&row)
	if result.Error != nil {
		return result.Error
	}
	cursor.Complete = result.RowsAffected == 0
	return saveQuotaWorkCursor(ctx, db, cursor)
}

// EnsureQuotaMaintenanceBackfillCursorsWithDB creates only the small durable
// cursors required by asynchronous backfill. It never scans or rewrites source
// rows, so startup schema migration remains bounded.
func EnsureQuotaMaintenanceBackfillCursorsWithDB(db *gorm.DB) error {
	if db == nil {
		return gorm.ErrInvalidDB
	}
	if !db.Migrator().HasTable(&QuotaWorkCursor{}) {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if db.Migrator().HasTable(&AccountQuotaMutationReceipt{}) {
		if err := ensureQuotaWorkCursorInitialized(ctx, db, quotaWorkCursorAccountMigration, &AccountQuotaMutationReceipt{}); err != nil {
			return err
		}
	}
	if db.Migrator().HasTable(&QuotaBalanceBatchDrain{}) {
		if err := ensureQuotaWorkCursorInitialized(ctx, db, quotaWorkCursorBalanceMigration, &QuotaBalanceBatchDrain{}); err != nil {
			return err
		}
	}
	return nil
}

func accountQuotaBackfillPending(ctx context.Context, db *gorm.DB) (bool, error) {
	if !db.Migrator().HasTable(&AccountQuotaMutationReceipt{}) {
		return false, nil
	}
	var receipt AccountQuotaMutationReceipt
	result := db.WithContext(ctx).Select("id").Where("fingerprint_version <> ?", AccountQuotaFingerprintVersion).Limit(1).Find(&receipt)
	if result.Error != nil || result.RowsAffected > 0 {
		return result.RowsAffected > 0, result.Error
	}
	if !db.Migrator().HasTable(&AccountQuotaReservationHead{}) {
		return true, nil
	}
	var missingHead struct {
		ID int64 `gorm:"column:id"`
	}
	result = db.WithContext(ctx).Table("account_quota_mutation_receipts AS receipts").Select("receipts.id").
		Where("receipts.phase = ? AND NOT EXISTS (SELECT 1 FROM account_quota_reservation_heads AS heads WHERE heads.request_id = receipts.request_id)", AccountQuotaPhaseReserve).
		Limit(1).Find(&missingHead)
	return result.RowsAffected > 0, result.Error
}

func quotaBalanceBackfillPending(ctx context.Context, db *gorm.DB) (bool, error) {
	if !db.Migrator().HasTable(&QuotaBalanceBatchDrain{}) {
		return false, nil
	}
	var generation QuotaBalanceBatchDrain
	result := db.WithContext(ctx).Select("id").Where(
		"schema_version < ? OR lock_version < ? OR (cache_applied = ? AND subject_id = ?)",
		quotaBalanceBatchDrainSchemaVersion, 1, false, 0,
	).Limit(1).Find(&generation)
	if result.Error != nil || result.RowsAffected > 0 {
		return result.RowsAffected > 0, result.Error
	}
	if !db.Migrator().HasTable(&QuotaBalanceBatchSubject{}) {
		var anyGeneration QuotaBalanceBatchDrain
		result = db.WithContext(ctx).Select("id").Limit(1).Find(&anyGeneration)
		return result.RowsAffected > 0, result.Error
	}
	var missingSubject struct {
		ID int64 `gorm:"column:id"`
	}
	result = db.WithContext(ctx).Table("quota_balance_batch_drains AS generations").Select("generations.id").
		Where("NOT EXISTS (SELECT 1 FROM quota_balance_batch_subjects AS subjects WHERE subjects.generation_id = generations.id)").
		Limit(1).Find(&missingSubject)
	return result.RowsAffected > 0, result.Error
}

func quotaMaintenanceCursorComplete(ctx context.Context, db *gorm.DB, cursorName string, sourceModel any) (bool, error) {
	if db == nil {
		return false, gorm.ErrInvalidDB
	}
	if !db.Migrator().HasTable(sourceModel) {
		return true, nil
	}
	if !db.Migrator().HasTable(&QuotaWorkCursor{}) {
		return false, nil
	}
	cursor, err := loadQuotaWorkCursor(ctx, db, cursorName)
	if err != nil {
		return false, err
	}
	return cursor.Complete, nil
}

// QuotaMaintenanceBackfillsComplete is the admission gate for authoritative
// account writes. The durable cursor is part of the state, not merely a
// progress hint: a cursor that has not reached a terminal complete state keeps
// the gate closed even when the current row-shape probes happen to find no
// work. This prevents a partially migrated v1 chain from being replayed by the
// authoritative writer.
func QuotaMaintenanceBackfillsComplete(ctx context.Context, db *gorm.DB) (bool, error) {
	if db == nil {
		return false, gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	accountComplete, err := quotaMaintenanceCursorComplete(ctx, db, quotaWorkCursorAccountMigration, &AccountQuotaMutationReceipt{})
	if err != nil || !accountComplete {
		return false, err
	}
	accountPending, err := accountQuotaBackfillPending(ctx, db)
	if err != nil {
		return false, err
	}
	if accountPending {
		return false, nil
	}
	balanceComplete, err := quotaMaintenanceCursorComplete(ctx, db, quotaWorkCursorBalanceMigration, &QuotaBalanceBatchDrain{})
	if err != nil || !balanceComplete {
		return false, err
	}
	balancePending, err := quotaBalanceBackfillPending(ctx, db)
	if err != nil {
		return false, err
	}
	return !balancePending, nil
}
