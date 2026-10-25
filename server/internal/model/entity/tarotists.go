// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// Tarotists is the golang structure for table tarotists.
type Tarotists struct {
	Id          int64       `json:"id"           orm:"id"           description:""`
	Name        string      `json:"name"         orm:"name"         description:""`
	Persona     string      `json:"persona"      orm:"persona"      description:""`
	DisplayCopy string      `json:"display_copy" orm:"display_copy" description:""`
	Enabled     bool        `json:"enabled"      orm:"enabled"      description:""`
	CreatedAt   *gtime.Time `json:"created_at"   orm:"created_at"   description:""`
}
