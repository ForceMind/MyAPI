package model

import (
	"github.com/ForceMind/MyAPI/common"
	"gorm.io/gorm"
)

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
