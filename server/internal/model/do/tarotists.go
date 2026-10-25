// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// Tarotists is the golang structure of table tarotists for DAO operations like Where/Data.
type Tarotists struct {
	g.Meta      `orm:"table:tarotists, do:true"`
	Id          any         //
	Name        any         //
	Persona     any         //
	DisplayCopy any         //
	Enabled     any         //
	CreatedAt   *gtime.Time //
}
