// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// AuditLogDao is the data access object for the table audit_log.
type AuditLogDao struct {
	table    string             // table is the underlying table name of the DAO.
	group    string             // group is the database configuration group name of the current DAO.
	columns  AuditLogColumns    // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler // handlers for customized model modification.
}

// AuditLogColumns defines and stores column names for the table audit_log.
type AuditLogColumns struct {
	Id         string //
	OperatorId string //
	Action     string //
	ObjectType string //
	ObjectId   string //
	Detail     string //
	CreatedAt  string //
}

// auditLogColumns holds the columns for the table audit_log.
var auditLogColumns = AuditLogColumns{
	Id:         "id",
	OperatorId: "operator_id",
	Action:     "action",
	ObjectType: "object_type",
	ObjectId:   "object_id",
	Detail:     "detail",
	CreatedAt:  "created_at",
}

// NewAuditLogDao creates and returns a new DAO object for table data access.
func NewAuditLogDao(handlers ...gdb.ModelHandler) *AuditLogDao {
	return &AuditLogDao{
		group:    "default",
		table:    "audit_log",
		columns:  auditLogColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *AuditLogDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *AuditLogDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *AuditLogDao) Columns() AuditLogColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *AuditLogDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *AuditLogDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *AuditLogDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
