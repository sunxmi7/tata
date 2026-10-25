// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// Idempotency is the golang structure for table idempotency.
type Idempotency struct {
	Scope     string      `json:"scope"      orm:"scope"      description:""`
	Key       string      `json:"key"        orm:"key"        description:""`
	Status    string      `json:"status"     orm:"status"     description:""`
	Response  string      `json:"response"   orm:"response"   description:""`
	CreatedAt *gtime.Time `json:"created_at" orm:"created_at" description:""`
}
