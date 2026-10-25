// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// AuditLog is the golang structure for table audit_log.
type AuditLog struct {
	Id         int64       `json:"id"          orm:"id"          description:""`
	OperatorId int64       `json:"operator_id" orm:"operator_id" description:""`
	Action     string      `json:"action"      orm:"action"      description:""`
	ObjectType string      `json:"object_type" orm:"object_type" description:""`
	ObjectId   string      `json:"object_id"   orm:"object_id"   description:""`
	Detail     string      `json:"detail"      orm:"detail"      description:""`
	CreatedAt  *gtime.Time `json:"created_at"  orm:"created_at"  description:""`
}
