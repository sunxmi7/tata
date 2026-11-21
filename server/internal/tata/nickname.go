package tata

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"strings"
	"unicode"
	"unicode/utf8"
)

// 随机昵称词库：神秘、轻松、有个性；主题为塔罗 / 命运 / 宇宙 / 好运 / 探索 / 未知。
// 只用常用汉字，避免生僻字、土味网名和过度文艺词。
var nickHeads = []string{
	"星轨", "夜雾", "好运", "星河", "命运", "未知", "塔罗", "宇宙",
	"月光", "银星", "雾海", "灵光", "晚风", "静夜", "微光", "远星",
	"暗涌", "轻尘", "牌缘", "探路", "拾光", "星尘", "夜航", "流星",
	"潮汐", "星湾", "晴夜", "牌阵", "夜色", "星途",
}

var nickTails2 = []string{
	"旅人", "探客", "牌客", "行者", "漫游", "拾光", "星客", "旅伴", "过客", "寻光",
}

var nickTails3 = []string{
	"探索家", "占卜人", "探路人", "星旅人", "漫游者", "拾光者", "寻路人",
}

var nickTails4 = []string{
	"星空旅人", "命运行者", "探牌旅人", "宇宙旅人",
}

func randomNickname() string {
	for i := 0; i < 16; i++ {
		nick := composeNickname()
		if validGeneratedNickname(nick) {
			return nick
		}
	}
	return "星河旅人"
}

func composeNickname() string {
	head := nickHeads[randN(len(nickHeads))]
	switch randN(3) {
	case 0:
		return head + nickTails2[randN(len(nickTails2))]
	case 1:
		return head + nickTails3[randN(len(nickTails3))]
	default:
		return head + nickTails4[randN(len(nickTails4))]
	}
}

func validGeneratedNickname(nick string) bool {
	n := utf8.RuneCountInString(nick)
	if n < 4 || n > 6 {
		return false
	}
	for _, r := range nick {
		if !unicode.Is(unicode.Han, r) {
			return false
		}
	}
	return true
}

func randN(n int) int {
	if n <= 0 {
		return 0
	}
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return 0
	}
	return int(binary.BigEndian.Uint64(buf[:]) % uint64(n))
}

func (a *App) ensureProfileDefaults(ctx context.Context, uid int64) {
	if a == nil || a.Store == nil || uid <= 0 {
		return
	}
	var name string
	if err := a.Store.QueryRow(ctx, `SELECT COALESCE(name,'') FROM app_users WHERE id=$1`, uid).Scan(&name); err != nil {
		return
	}
	if strings.TrimSpace(name) != "" {
		return
	}
	_ = a.assignRandomNickname(ctx, uid)
}

func (a *App) assignRandomNickname(ctx context.Context, uid int64) string {
	for i := 0; i < 24; i++ {
		nick := randomNickname()
		var n int
		_ = a.Store.QueryRow(ctx, `SELECT count(*) FROM app_users WHERE name=$1 AND id<>$2`, nick, uid).Scan(&n)
		if n > 0 {
			continue
		}
		tag, err := a.Store.Exec(ctx, `UPDATE app_users SET name=$1 WHERE id=$2 AND name=''`, nick, uid)
		if err != nil || tag.RowsAffected() == 0 {
			var current string
			_ = a.Store.QueryRow(ctx, `SELECT COALESCE(name,'') FROM app_users WHERE id=$1`, uid).Scan(&current)
			return current
		}
		return nick
	}
	fallback := "星河旅人"
	_, _ = a.Store.Exec(ctx, `UPDATE app_users SET name=$1 WHERE id=$2 AND name=''`, fallback, uid)
	return fallback
}
