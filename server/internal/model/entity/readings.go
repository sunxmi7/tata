// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// Readings is the golang structure for table readings.
type Readings struct {
	Id              int64       `json:"id"               orm:"id"               description:""`
	UserId          int64       `json:"user_id"          orm:"user_id"          description:""`
	AnonymousId     string      `json:"anonymous_id"     orm:"anonymous_id"     description:""`
	Type            string      `json:"type"             orm:"type"             description:""`
	Question        string      `json:"question"         orm:"question"         description:""`
	SpreadId        int64       `json:"spread_id"        orm:"spread_id"        description:""`
	TarotistId      int64       `json:"tarotist_id"      orm:"tarotist_id"      description:""`
	Status          string      `json:"status"           orm:"status"           description:""`
	Cards           string      `json:"cards"            orm:"cards"            description:""`
	Interpretation  string      `json:"interpretation"   orm:"interpretation"   description:""`
	VendorName      string      `json:"vendor_name"      orm:"vendor_name"      description:""`
	ModelName       string      `json:"model_name"       orm:"model_name"       description:""`
	FailReason      string      `json:"fail_reason"      orm:"fail_reason"      description:""`
	QuotaConsumed   bool        `json:"quota_consumed"   orm:"quota_consumed"   description:""`
	FirstExperience bool        `json:"first_experience" orm:"first_experience" description:""`
	QuotaCycle      string      `json:"quota_cycle"      orm:"quota_cycle"      description:""`
	CreatedAt       *gtime.Time `json:"created_at"       orm:"created_at"       description:""`
	UpdatedAt       *gtime.Time `json:"updated_at"       orm:"updated_at"       description:""`
	DeletedAt       *gtime.Time `json:"deleted_at"       orm:"deleted_at"       description:""`
}
