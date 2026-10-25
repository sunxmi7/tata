// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

// QuotaUse is the golang structure for table quota_use.
type QuotaUse struct {
	UserId    int64  `json:"user_id"    orm:"user_id"    description:""`
	Cycle     string `json:"cycle"      orm:"cycle"      description:""`
	ReadingId int64  `json:"reading_id" orm:"reading_id" description:""`
}
