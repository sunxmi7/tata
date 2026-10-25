// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// Readings is the golang structure of table readings for DAO operations like Where/Data.
type Readings struct {
	g.Meta          `orm:"table:readings, do:true"`
	Id              any         //
	UserId          any         //
	AnonymousId     any         //
	Type            any         //
	Question        any         //
	SpreadId        any         //
	TarotistId      any         //
	Status          any         //
	Cards           any         //
	Interpretation  any         //
	VendorName      any         //
	ModelName       any         //
	FailReason      any         //
	QuotaConsumed   any         //
	FirstExperience any         //
	QuotaCycle      any         //
	CreatedAt       *gtime.Time //
	UpdatedAt       *gtime.Time //
	DeletedAt       *gtime.Time //
}
