package tata

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

// reportsDemoMode: query demo=1|ready|empty|locked, or config reports.demo=true (always ready).
// Unit tests ignore the config flag so real empty/locked paths stay covered.
func reportsDemoMode(r *http.Request) string {
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("demo")))
	switch q {
	case "1", "true", "ready", "empty", "locked":
		if q == "1" || q == "true" {
			return "ready"
		}
		return q
	}
	if testing.Testing() {
		return ""
	}
	if g.Cfg().MustGet(r.Context(), "reports.demo").Bool() {
		return "ready"
	}
	return ""
}

func demoMonthlyReport(month, mode string, now time.Time) map[string]any {
	if month == "" {
		month = now.Format("2006-01")
	}
	months := make([]map[string]any, 0, 12)
	cur, _ := time.Parse("2006-01", month)
	if cur.IsZero() {
		cur = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	}
	for i := 0; i < 12; i++ {
		t := cur.AddDate(0, -i, 0)
		m := t.Format("2006-01")
		n := 0
		saved := false
		if i == 0 && mode != "empty" {
			n = 18
			saved = true
		} else if i == 1 {
			n = 9
			saved = true
		} else if i == 2 {
			n = 4
		}
		months = append(months, map[string]any{"month": m, "exploration_count": n, "saved": saved})
	}
	if mode == "empty" {
		return map[string]any{
			"month": month, "status": "empty", "empty_title": "这个月还没有抽过牌",
			"exploration_count": 0, "companion_days": 0, "focus_topics": []any{},
			"quote": "", "quote_status": "pending", "saved": false, "saved_at": nil,
			"months": months, "fields_confirmed": true, "content_invented": false,
		}
	}
	return map[string]any{
		"month": month, "status": "ready", "empty_title": "",
		"exploration_count": 18, "companion_days": 12,
		"focus_topics": []map[string]any{
			{"title": "感情与关系", "count": 7, "ratio": 0.39},
			{"title": "工作与学业", "count": 5, "ratio": 0.28},
			{"title": "自我成长", "count": 4, "ratio": 0.22},
			{"title": "情绪与疗愈", "count": 2, "ratio": 0.11},
		},
		"quote":         "有连接，但现实推进不足——把期待说清楚，比反复猜测更有用。",
		"quote_status":  "ready",
		"saved":         true,
		"saved_at":      now.UTC().Format(time.RFC3339Nano),
		"months":        months,
		"fields_confirmed": true,
		"content_invented": false,
	}
}

func demoYearlyCard(r *http.Request, pos int, posName, name, ori string, cardID int64) map[string]any {
	return map[string]any{
		"position": pos, "position_name": posName, "card_id": cardID,
		"name": name, "orientation": ori,
		"card_thumbnail": absoluteAssetURL(r, defaultCardThumbnailPath),
	}
}

func demoYearlyReport(r *http.Request, year int, mode string) map[string]any {
	houses := []map[string]any{
		demoYearlyCard(r, 1, "自我", "愚者", "upright", 0),
		demoYearlyCard(r, 2, "财富", "魔术师", "upright", 1),
		demoYearlyCard(r, 3, "沟通", "女皇", "reversed", 3),
		demoYearlyCard(r, 4, "家庭", "皇帝", "upright", 4),
		demoYearlyCard(r, 5, "创造", "恋人", "upright", 6),
		demoYearlyCard(r, 6, "日常", "力量", "reversed", 8),
		demoYearlyCard(r, 7, "伴侣", "隐者", "reversed", 9),
		demoYearlyCard(r, 8, "深层", "死神", "upright", 13),
		demoYearlyCard(r, 9, "远行", "星星", "upright", 16),
		demoYearlyCard(r, 10, "事业", "战车", "upright", 7),
		demoYearlyCard(r, 11, "社群", "太阳", "upright", 19),
		demoYearlyCard(r, 12, "潜意识", "月亮", "reversed", 18),
	}
	seasons := []map[string]any{
		demoYearlyCard(r, 1, "春", "审判", "upright", 17),
		demoYearlyCard(r, 2, "夏", "命运之轮", "upright", 10),
		demoYearlyCard(r, 3, "秋", "节制", "reversed", 14),
		demoYearlyCard(r, 4, "冬", "世界", "upright", 21),
	}
	sections := []map[string]any{
		{"key": "theme", "title": "主题", "body": "这一年的主题，是把「想清楚」慢慢落到「做得到」。你会更频繁地面对选择：留下什么、放下什么、把精力投向哪里。", "content_status": "ready"},
		{"key": "domain", "title": "领域", "body": "感情、事业与自我成长会轮流成为主线。关系里更需要说清楚期待；工作上适合做可验证的小推进，而不是一次押上全部。", "content_status": "ready"},
		{"key": "rhythm", "title": "节奏", "body": "上半年偏探索与铺垫，下半年更考验耐心与节奏。春夏适合试错，秋冬适合复盘与收束。", "content_status": "ready"},
		{"key": "action", "title": "行动", "body": "每月留一次认真提问的时间；把模糊不安写成具体问题；用小行动验证判断，而不是反复空转。", "content_status": "ready"},
	}
	bodies := map[string]string{}
	for _, s := range sections {
		bodies[s["key"].(string)] = s["body"].(string)
	}
	base := map[string]any{
		"year": year, "year_member_unlocks": true,
		"unlock_price_minor": 2880, "unlock_price_currency": "CNY",
		"content_status": "ready",
		"theme": bodies["theme"], "domain": bodies["domain"],
		"rhythm": bodies["rhythm"], "action": bodies["action"],
		"sections": sections, "fields_confirmed": true, "content_invented": false,
	}
	if mode == "locked" {
		base["unlocked"] = false
		base["unlock_source"] = ""
		base["standalone_purchase"] = true
		base["step"] = "locked"
		base["profile"] = map[string]any{
			"birth_date": nil, "birth_time": nil, "birth_place": nil, "gender": nil, "completed": false,
		}
		base["rhythm_cards"] = []any{}
		base["house_cards"] = []any{}
		base["domain_cards"] = []any{}
		base["season_cards"] = []any{}
		base["report"] = nil
		base["report_status"] = "pending"
		base["fail_reason"] = nil
		base["generated_at"] = nil
		base["steps"] = []map[string]any{
			{"key": "profile", "title": "填写基本信息", "done": false, "required": 0, "drawn": 0},
			{"key": "rhythm", "title": "抽取十二宫牌阵", "done": false, "required": 12, "drawn": 0},
			{"key": "domains", "title": "抽取四季牌阵", "done": false, "required": 4, "drawn": 0},
			{"key": "generate", "title": "生成年度报告", "done": false, "required": 0, "drawn": 0},
		}
		return base
	}
	report := "按已固定的十二宫与四季牌面解读如下。\n\n### 十二宫总览\n自我宫的愚者正位，提示这一年适合带着好奇开启新阶段。财富宫魔术师正位，强调资源需要主动组合。沟通宫女皇逆位，提醒表达与滋养可能失衡。家庭宫皇帝正位，适合整理生活根基。创造宫恋人正位，创作与选择都会更有温度。日常宫力量逆位，提示节奏容易被硬撑透支。伴侣宫隐者逆位，关系里可能回避深入沟通。深层宫死神正位，支持必要的告别与换挡。远行宫星星正位，视野与希望感会回来。事业宫战车正位，方向明确就适合推进。社群宫太阳正位，合作与曝光会带来正反馈。潜意识宫月亮逆位，适合把模糊不安摊开看清。\n\n### 四季节奏\n春·审判正位：适合复盘后重新出发。夏·命运之轮正位：机会窗口打开。秋·节制逆位：需要主动刹车。冬·世界正位：适合收束成果。\n\n### 总评与行动建议\n2026 更像「选择—验证—收束」的一年。关系与事业都不要只停在感觉层，把期待和下一周能做的一步写下来。"
	base["unlocked"] = true
	base["unlock_source"] = "year_subscription"
	base["standalone_purchase"] = false
	base["step"] = "ready"
	base["profile"] = map[string]any{
		"birth_date": "1996-08-18", "birth_time": "08:30", "birth_place": "上海",
		"gender": "不愿透露", "completed": true,
	}
	base["rhythm_cards"] = houses
	base["house_cards"] = houses
	base["domain_cards"] = seasons
	base["season_cards"] = seasons
	base["report"] = report
	base["report_status"] = "ready"
	base["fail_reason"] = nil
	base["generated_at"] = time.Now().UTC().Format(time.RFC3339Nano)
	base["steps"] = []map[string]any{
		{"key": "profile", "title": "填写基本信息", "done": true, "required": 0, "drawn": 0},
		{"key": "rhythm", "title": "抽取十二宫牌阵", "done": true, "required": 12, "drawn": 12},
		{"key": "domains", "title": "抽取四季牌阵", "done": true, "required": 4, "drawn": 4},
		{"key": "generate", "title": "生成年度报告", "done": true, "required": 0, "drawn": 0},
	}
	return base
}
