// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// Ledger is the golang structure for table ledger.
type Ledger struct {
	Id          int64       `json:"id"          orm:"id"          description:""`
	UserId      int64       `json:"user_id"     orm:"user_id"     description:""`
	ReadingId   int64       `json:"reading_id"  orm:"reading_id"  description:""`
	Entitlement string      `json:"entitlement" orm:"entitlement" description:""`
	Amount      int         `json:"amount"      orm:"amount"      description:""`
	Reason      string      `json:"reason"      orm:"reason"      description:""`
	Result      string      `json:"result"      orm:"result"      description:""`
	CreatedAt   *gtime.Time `json:"created_at"  orm:"created_at"  description:""`
}
