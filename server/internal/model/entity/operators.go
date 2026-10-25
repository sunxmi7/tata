// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// Operators is the golang structure for table operators.
type Operators struct {
	Id           int64       `json:"id"            orm:"id"            description:""`
	Username     string      `json:"username"      orm:"username"      description:""`
	PasswordHash string      `json:"password_hash" orm:"password_hash" description:""`
	Role         string      `json:"role"          orm:"role"          description:""`
	Disabled     bool        `json:"disabled"      orm:"disabled"      description:""`
	CreatedAt    *gtime.Time `json:"created_at"    orm:"created_at"    description:""`
}
