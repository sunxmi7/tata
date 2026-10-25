// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// Rewards is the golang structure for table rewards.
type Rewards struct {
	UserId     int64       `json:"user_id"     orm:"user_id"     description:""`
	GrantedAt  *gtime.Time `json:"granted_at"  orm:"granted_at"  description:""`
	ExpiresAt  *gtime.Time `json:"expires_at"  orm:"expires_at"  description:""`
	ConsumedAt *gtime.Time `json:"consumed_at" orm:"consumed_at" description:""`
}
