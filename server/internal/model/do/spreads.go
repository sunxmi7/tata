// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// Spreads is the golang structure of table spreads for DAO operations like Where/Data.
type Spreads struct {
	g.Meta      `orm:"table:spreads, do:true"`
	Id          any         //
	Name        any         //
	Description any         //
	CardCount   any         //
	Positions   any         //
	Enabled     any         //
	Charged     any         //
	CreatedAt   *gtime.Time //
}
