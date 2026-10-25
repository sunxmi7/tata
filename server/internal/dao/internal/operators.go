// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// OperatorsDao is the data access object for the table operators.
type OperatorsDao struct {
	table    string             // table is the underlying table name of the DAO.
	group    string             // group is the database configuration group name of the current DAO.
	columns  OperatorsColumns   // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler // handlers for customized model modification.
}

// OperatorsColumns defines and stores column names for the table operators.
type OperatorsColumns struct {
	Id           string //
	Username     string //
	PasswordHash string //
	Role         string //
	Disabled     string //
	CreatedAt    string //
}

// operatorsColumns holds the columns for the table operators.
var operatorsColumns = OperatorsColumns{
	Id:           "id",
	Username:     "username",
	PasswordHash: "password_hash",
	Role:         "role",
	Disabled:     "disabled",
	CreatedAt:    "created_at",
}

// NewOperatorsDao creates and returns a new DAO object for table data access.
func NewOperatorsDao(handlers ...gdb.ModelHandler) *OperatorsDao {
	return &OperatorsDao{
		group:    "default",
		table:    "operators",
		columns:  operatorsColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *OperatorsDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *OperatorsDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *OperatorsDao) Columns() OperatorsColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *OperatorsDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *OperatorsDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *OperatorsDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
