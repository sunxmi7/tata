// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// TarotistsDao is the data access object for the table tarotists.
type TarotistsDao struct {
	table    string             // table is the underlying table name of the DAO.
	group    string             // group is the database configuration group name of the current DAO.
	columns  TarotistsColumns   // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler // handlers for customized model modification.
}

// TarotistsColumns defines and stores column names for the table tarotists.
type TarotistsColumns struct {
	Id          string //
	Name        string //
	Persona     string //
	DisplayCopy string //
	Enabled     string //
	CreatedAt   string //
}

// tarotistsColumns holds the columns for the table tarotists.
var tarotistsColumns = TarotistsColumns{
	Id:          "id",
	Name:        "name",
	Persona:     "persona",
	DisplayCopy: "display_copy",
	Enabled:     "enabled",
	CreatedAt:   "created_at",
}

// NewTarotistsDao creates and returns a new DAO object for table data access.
func NewTarotistsDao(handlers ...gdb.ModelHandler) *TarotistsDao {
	return &TarotistsDao{
		group:    "default",
		table:    "tarotists",
		columns:  tarotistsColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *TarotistsDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *TarotistsDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *TarotistsDao) Columns() TarotistsColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *TarotistsDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *TarotistsDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *TarotistsDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
