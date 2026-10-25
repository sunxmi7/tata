// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// Bindings is the golang structure for table bindings.
type Bindings struct {
	Task      string      `json:"task"       orm:"task"       description:""`
	VendorId  int64       `json:"vendor_id"  orm:"vendor_id"  description:""`
	Model     string      `json:"model"      orm:"model"      description:""`
	UpdatedAt *gtime.Time `json:"updated_at" orm:"updated_at" description:""`
}
