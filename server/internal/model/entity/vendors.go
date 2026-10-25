// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// Vendors is the golang structure for table vendors.
type Vendors struct {
	Id         int64       `json:"id"          orm:"id"          description:""`
	Name       string      `json:"name"        orm:"name"        description:""`
	BaseUrl    string      `json:"base_url"    orm:"base_url"    description:""`
	ApiKeyEnc  []byte      `json:"api_key_enc" orm:"api_key_enc" description:""`
	Models     string      `json:"models"      orm:"models"      description:""`
	TimeoutSec int         `json:"timeout_sec" orm:"timeout_sec" description:""`
	Enabled    bool        `json:"enabled"     orm:"enabled"     description:""`
	CreatedAt  *gtime.Time `json:"created_at"  orm:"created_at"  description:""`
}
