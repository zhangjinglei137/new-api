package model

import (
	"github.com/QuantumNous/new-api/common"

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
	if common.UsingMainDatabase(common.DatabaseTypeSQLite) {
		return tx
	}
	return tx.Clauses(clause.Locking{Strength: "UPDATE"})
}

// LockForUpdate 是 lockForUpdate 对其它包（如 controller 的事务内
// read-modify-write 流程）的导出薄包装：FOR UPDATE / SQLite 跳过等方言逻辑
// 只允许存在于 lockForUpdate，调用方不得各自复制 clause.Locking。
func LockForUpdate(tx *gorm.DB) *gorm.DB {
	return lockForUpdate(tx)
}
