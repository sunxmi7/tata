// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
)

// QuotaUse is the golang structure of table quota_use for DAO operations like Where/Data.
type QuotaUse struct {
	g.Meta    `orm:"table:quota_use, do:true"`
	UserId    any //
	Cycle     any //
	ReadingId any //
}
