// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// Banners is the golang structure for table banners.
type Banners struct {
	Id        int64       `json:"id"         orm:"id"         description:""`
	Copy      string      `json:"copy"       orm:"copy"       description:""`
	StartAt   *gtime.Time `json:"start_at"   orm:"start_at"   description:""`
	EndAt     *gtime.Time `json:"end_at"     orm:"end_at"     description:""`
	Priority  int         `json:"priority"   orm:"priority"   description:""`
	Target    string      `json:"target"     orm:"target"     description:""`
	CreatedAt *gtime.Time `json:"created_at" orm:"created_at" description:""`
}
