// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// Operators is the golang structure of table operators for DAO operations like Where/Data.
type Operators struct {
	g.Meta       `orm:"table:operators, do:true"`
	Id           any         //
	Username     any         //
	PasswordHash any         //
	Role         any         //
	Disabled     any         //
	CreatedAt    *gtime.Time //
}
