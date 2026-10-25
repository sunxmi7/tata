// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

// DailySlot is the golang structure for table daily_slot.
type DailySlot struct {
	UserId    int64  `json:"user_id"    orm:"user_id"    description:""`
	Cycle     string `json:"cycle"      orm:"cycle"      description:""`
	ReadingId int64  `json:"reading_id" orm:"reading_id" description:""`
}
