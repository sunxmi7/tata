// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// RefreshTokens is the golang structure for table refresh_tokens.
type RefreshTokens struct {
	Id        string      `json:"id"         orm:"id"         description:""`
	Audience  string      `json:"audience"   orm:"audience"   description:""`
	SubjectId int64       `json:"subject_id" orm:"subject_id" description:""`
	ExpiresAt *gtime.Time `json:"expires_at" orm:"expires_at" description:""`
	Revoked   bool        `json:"revoked"    orm:"revoked"    description:""`
}
