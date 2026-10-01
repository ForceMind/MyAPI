package model

import (
	"context"
	"github.com/ForceMind/MyAPI/common"
	"gorm.io/gorm"
)

// ReadDatabaseUnixTime never substitutes a node clock when the database clock fails.
func ReadDatabaseUnixTime(ctx context.Context, db *gorm.DB) (int64, error) {
	if db == nil {
		return 0, gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return taskRecoveryDBTimestamp(db.WithContext(ctx))
}

// GetDBTimestamp returns a UNIX timestamp from database time.
// Falls back to application time on error.
func GetDBTimestamp() int64 {
	return getDBTimestampTx(DB)
}

// getDBTimestampTx uses the caller's connection, including an active
// transaction, rather than borrowing another connection from the global pool.
func getDBTimestampTx(tx *gorm.DB) int64 {
	var ts int64
	var err error
	switch tx.Dialector.Name() {
	case "postgres":
		err = tx.Raw("SELECT EXTRACT(EPOCH FROM NOW())::bigint").Scan(&ts).Error
	case "sqlite":
		err = tx.Raw("SELECT strftime('%s','now')").Scan(&ts).Error
	default:
		err = tx.Raw("SELECT UNIX_TIMESTAMP()").Scan(&ts).Error
	}
	if err != nil || ts <= 0 {
		return common.GetTimestamp()
	}
	return ts
}
