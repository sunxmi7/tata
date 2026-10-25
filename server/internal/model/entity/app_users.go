// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// AppUsers is the golang structure for table app_users.
type AppUsers struct {
	Id           int64       `json:"id"            orm:"id"            description:""`
	Email        string      `json:"email"         orm:"email"         description:""`
	PasswordHash string      `json:"password_hash" orm:"password_hash" description:""`
	BirthDate    string      `json:"birth_date"    orm:"birth_date"    description:""`
	Gender       string      `json:"gender"        orm:"gender"        description:""`
	CreatedAt    *gtime.Time `json:"created_at"    orm:"created_at"    description:""`
}
