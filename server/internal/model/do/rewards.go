// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// Rewards is the golang structure of table rewards for DAO operations like Where/Data.
type Rewards struct {
	g.Meta     `orm:"table:rewards, do:true"`
	UserId     any         //
	GrantedAt  *gtime.Time //
	ExpiresAt  *gtime.Time //
	ConsumedAt *gtime.Time //
}
