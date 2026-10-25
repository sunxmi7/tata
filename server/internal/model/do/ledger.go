// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// Ledger is the golang structure of table ledger for DAO operations like Where/Data.
type Ledger struct {
	g.Meta      `orm:"table:ledger, do:true"`
	Id          any         //
	UserId      any         //
	ReadingId   any         //
	Entitlement any         //
	Amount      any         //
	Reason      any         //
	Result      any         //
	CreatedAt   *gtime.Time //
}
