// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// Vendors is the golang structure of table vendors for DAO operations like Where/Data.
type Vendors struct {
	g.Meta     `orm:"table:vendors, do:true"`
	Id         any         //
	Name       any         //
	BaseUrl    any         //
	ApiKeyEnc  []byte      //
	Models     any         //
	TimeoutSec any         //
	Enabled    any         //
	CreatedAt  *gtime.Time //
}
