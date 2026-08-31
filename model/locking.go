package model

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// lockForUpdate makes the next query emit SELECT ... FOR UPDATE so the matched
// rows stay locked until the surrounding transaction ends.
//
// GORM v2 silently ignores the legacy `Set("gorm:query_option", "FOR UPDATE")`
// from GORM v1, so that form does not lock anything. Always use this helper
// instead.
//
// SQLite has no FOR UPDATE syntax (the clause would be a syntax error), so it
// is skipped there; SQLite's single-writer model makes one of two conflicting
// transactions fail instead of both committing.
func lockForUpdate(tx *gorm.DB) *gorm.DB {
	if tx == nil {
		return tx
	}
	// Use the handle's dialector instead of the process-wide configured type.
	// This keeps test/auxiliary handles safe and prevents SQLite from receiving
	// unsupported FOR UPDATE syntax when the global setting differs.
	if tx.Dialector == nil || tx.Dialector.Name() == "sqlite" {
		return tx
	}
	switch tx.Dialector.Name() {
	case "mysql", "postgres":
		return tx.Clauses(clause.Locking{Strength: "UPDATE"})
	default:
		return tx
	}
}
