// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// Spreads is the golang structure for table spreads.
type Spreads struct {
	Id          int64       `json:"id"          orm:"id"          description:""`
	Name        string      `json:"name"        orm:"name"        description:""`
	Description string      `json:"description" orm:"description" description:""`
	CardCount   int         `json:"card_count"  orm:"card_count"  description:""`
	Positions   string      `json:"positions"   orm:"positions"   description:""`
	Enabled     bool        `json:"enabled"     orm:"enabled"     description:""`
	Charged     bool        `json:"charged"     orm:"charged"     description:""`
	CreatedAt   *gtime.Time `json:"created_at"  orm:"created_at"  description:""`
}
