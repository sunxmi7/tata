// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// LedgerDao is the data access object for the table ledger.
type LedgerDao struct {
	table    string             // table is the underlying table name of the DAO.
	group    string             // group is the database configuration group name of the current DAO.
	columns  LedgerColumns      // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler // handlers for customized model modification.
}

// LedgerColumns defines and stores column names for the table ledger.
type LedgerColumns struct {
	Id          string //
	UserId      string //
	ReadingId   string //
	Entitlement string //
	Amount      string //
	Reason      string //
	Result      string //
	CreatedAt   string //
}

// ledgerColumns holds the columns for the table ledger.
var ledgerColumns = LedgerColumns{
	Id:          "id",
	UserId:      "user_id",
	ReadingId:   "reading_id",
	Entitlement: "entitlement",
	Amount:      "amount",
	Reason:      "reason",
	Result:      "result",
	CreatedAt:   "created_at",
}

// NewLedgerDao creates and returns a new DAO object for table data access.
func NewLedgerDao(handlers ...gdb.ModelHandler) *LedgerDao {
	return &LedgerDao{
		group:    "default",
		table:    "ledger",
		columns:  ledgerColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *LedgerDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *LedgerDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *LedgerDao) Columns() LedgerColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *LedgerDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *LedgerDao) Ctx(ctx context.Context) *gdb.Model {
	model := dao.DB().Model(dao.table)
	for _, handler := range dao.handlers {
		model = handler(model)
	}
	return model.Safe().Ctx(ctx)
}

// Transaction wraps the transaction logic using function f.
// It rolls back the transaction and returns the error if function f returns a non-nil error.
// It commits the transaction and returns nil if function f returns nil.
//
// Note: Do not commit or roll back the transaction in function f,
// as it is automatically handled by this function.
func (dao *LedgerDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
