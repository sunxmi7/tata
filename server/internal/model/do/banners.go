// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// Banners is the golang structure of table banners for DAO operations like Where/Data.
type Banners struct {
	g.Meta    `orm:"table:banners, do:true"`
	Id        any         //
	Copy      any         //
	StartAt   *gtime.Time //
	EndAt     *gtime.Time //
	Priority  any         //
	Target    any         //
	CreatedAt *gtime.Time //
}
