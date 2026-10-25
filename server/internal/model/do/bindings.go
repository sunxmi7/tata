// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// Bindings is the golang structure of table bindings for DAO operations like Where/Data.
type Bindings struct {
	g.Meta    `orm:"table:bindings, do:true"`
	Task      any         //
	VendorId  any         //
	Model     any         //
	UpdatedAt *gtime.Time //
}
