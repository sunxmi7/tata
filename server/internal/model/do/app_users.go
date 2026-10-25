// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// AppUsers is the golang structure of table app_users for DAO operations like Where/Data.
type AppUsers struct {
	g.Meta       `orm:"table:app_users, do:true"`
	Id           any         //
	Email        any         //
	PasswordHash any         //
	BirthDate    any         //
	Gender       any         //
	CreatedAt    *gtime.Time //
}
