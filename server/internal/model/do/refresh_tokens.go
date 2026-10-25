// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// RefreshTokens is the golang structure of table refresh_tokens for DAO operations like Where/Data.
type RefreshTokens struct {
	g.Meta    `orm:"table:refresh_tokens, do:true"`
	Id        any         //
	Audience  any         //
	SubjectId any         //
	ExpiresAt *gtime.Time //
	Revoked   any         //
}
