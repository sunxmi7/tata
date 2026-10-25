// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// QuotaUseDao is the data access object for the table quota_use.
type QuotaUseDao struct {
	table    string             // table is the underlying table name of the DAO.
	group    string             // group is the database configuration group name of the current DAO.
	columns  QuotaUseColumns    // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler // handlers for customized model modification.
}

// QuotaUseColumns defines and stores column names for the table quota_use.
type QuotaUseColumns struct {
	UserId    string //
	Cycle     string //
	ReadingId string //
}

// quotaUseColumns holds the columns for the table quota_use.
var quotaUseColumns = QuotaUseColumns{
	UserId:    "user_id",
	Cycle:     "cycle",
	ReadingId: "reading_id",
}

// NewQuotaUseDao creates and returns a new DAO object for table data access.
func NewQuotaUseDao(handlers ...gdb.ModelHandler) *QuotaUseDao {
	return &QuotaUseDao{
		group:    "default",
		table:    "quota_use",
		columns:  quotaUseColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *QuotaUseDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *QuotaUseDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *QuotaUseDao) Columns() QuotaUseColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *QuotaUseDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *QuotaUseDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *QuotaUseDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
