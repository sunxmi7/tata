// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// Idempotency is the golang structure of table idempotency for DAO operations like Where/Data.
type Idempotency struct {
	g.Meta    `orm:"table:idempotency, do:true"`
	Scope     any         //
	Key       any         //
	Status    any         //
	Response  any         //
	CreatedAt *gtime.Time //
}
