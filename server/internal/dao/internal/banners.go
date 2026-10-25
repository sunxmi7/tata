// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// BannersDao is the data access object for the table banners.
type BannersDao struct {
	table    string             // table is the underlying table name of the DAO.
	group    string             // group is the database configuration group name of the current DAO.
	columns  BannersColumns     // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler // handlers for customized model modification.
}

// BannersColumns defines and stores column names for the table banners.
type BannersColumns struct {
	Id        string //
	Copy      string //
	StartAt   string //
	EndAt     string //
	Priority  string //
	Target    string //
	CreatedAt string //
}

// bannersColumns holds the columns for the table banners.
var bannersColumns = BannersColumns{
	Id:        "id",
	Copy:      "copy",
	StartAt:   "start_at",
	EndAt:     "end_at",
	Priority:  "priority",
	Target:    "target",
	CreatedAt: "created_at",
}

// NewBannersDao creates and returns a new DAO object for table data access.
func NewBannersDao(handlers ...gdb.ModelHandler) *BannersDao {
	return &BannersDao{
		group:    "default",
		table:    "banners",
		columns:  bannersColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *BannersDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *BannersDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *BannersDao) Columns() BannersColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *BannersDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *BannersDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *BannersDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
