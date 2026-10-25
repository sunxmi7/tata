// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// VendorsDao is the data access object for the table vendors.
type VendorsDao struct {
	table    string             // table is the underlying table name of the DAO.
	group    string             // group is the database configuration group name of the current DAO.
	columns  VendorsColumns     // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler // handlers for customized model modification.
}

// VendorsColumns defines and stores column names for the table vendors.
type VendorsColumns struct {
	Id         string //
	Name       string //
	BaseUrl    string //
	ApiKeyEnc  string //
	Models     string //
	TimeoutSec string //
	Enabled    string //
	CreatedAt  string //
}

// vendorsColumns holds the columns for the table vendors.
var vendorsColumns = VendorsColumns{
	Id:         "id",
	Name:       "name",
	BaseUrl:    "base_url",
	ApiKeyEnc:  "api_key_enc",
	Models:     "models",
	TimeoutSec: "timeout_sec",
	Enabled:    "enabled",
	CreatedAt:  "created_at",
}

// NewVendorsDao creates and returns a new DAO object for table data access.
func NewVendorsDao(handlers ...gdb.ModelHandler) *VendorsDao {
	return &VendorsDao{
		group:    "default",
		table:    "vendors",
		columns:  vendorsColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *VendorsDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *VendorsDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *VendorsDao) Columns() VendorsColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *VendorsDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *VendorsDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *VendorsDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
