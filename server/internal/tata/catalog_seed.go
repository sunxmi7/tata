package tata

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
)

func validDepthGroup(group string) bool {
	switch group {
	case "", "quick", "advanced", "panorama":
		return true
	default:
		return false
	}
}

func depthLabel(group string) string {
	switch group {
	case "quick":
		return "快速指引"
	case "advanced":
		return "进阶分析"
	case "panorama":
		return "深度全景"
	default:
		return ""
	}
}

func quotaCostValue(n sql.NullInt64) any {
	if !n.Valid {
		return nil
	}
	return n.Int64
}

func quotaLedger(n sql.NullInt64) any {
	if !n.Valid {
		return nil
	}
	return "quota_count"
}

func spreadDepth(raw *string) (string, error) {
	if raw == nil {
		return "", nil
	}
	group := strings.TrimSpace(*raw)
	if !validDepthGroup(group) {
		return "", biz("牌阵分组不正确")
	}
	return group, nil
}

func spreadQuota(priced *bool, cost *int, fallback sql.NullInt64) (sql.NullInt64, error) {
	if priced == nil {
		return fallback, nil
	}
	if !*priced {
		return sql.NullInt64{}, nil
	}
	if cost == nil || *cost < 0 {
		return sql.NullInt64{}, biz("请填写扣次")
	}
	return sql.NullInt64{Int64: int64(*cost), Valid: true}, nil
}

type ordinarySpreadSeed struct {
	Name, NameEN, Desc, Group string
	Cards, Sort               int
	Cost                      *int
}

func quotaPtr(n int) *int { return &n }

func ordinarySpreadSeeds() []ordinarySpreadSeed {
	five, eight, ten := quotaPtr(5), quotaPtr(8), quotaPtr(10)
	return []ordinarySpreadSeed{
		{"单张牌占卜", "One Card Reading", "聚焦问题核心，快速获得启发与指引。", "quick", 1, 1, five},
		{"过去·现在·未来", "Past, Present and Future", "梳理事情的过往背景、当前状态与未来发展。", "quick", 3, 2, five},
		{"三角牌阵", "Three Card Triangle", "观察问题的起因、发展与走向。", "quick", 3, 3, five},
		{"三张牌叙事牌阵", "Three Card Narrative", "结合牌面象征与相互关系，形成完整的问题解读。", "quick", 3, 4, five},
		{"选择A·选择B·综合建议", "Choice A, Choice B & Advice", "对比两个选项的利弊，并给出综合建议。", "quick", 3, 5, five},
		{"四阶段十字牌阵", "Four-Stage Cross", "从开始、衰落与收束、衰退到发展高峰，梳理事情的四个阶段。", "advanced", 4, 6, five},
		{"马赛十字牌阵", "Marseille Cross", "支持、反对、讨论与结果等角度分析。", "advanced", 4, 7, five},
		{"简单十字牌阵", "Simple Cross", "核心、阻碍、潜在基础与行动方向展开解读。", "advanced", 4, 8, five},
		{"五张十字牌阵", "Five Card Cross", "核心问题、过去影响、未来发展、潜在因素与建议。", "advanced", 5, 9, five},
		{"五张线性牌阵", "Five Card Linear Spread", "事情的过去、现在、隐藏因素、未来趋势与行动建议。", "advanced", 5, 10, five},
		{"问题解决牌阵", "Problem-Solving Spread", "拆解问题现状、成因、资源、阻碍与可行解决方向。", "advanced", 5, 11, five},
		{"六张十字牌阵", "Six Card Cross", "分析当前状况、阻碍、问题根源、近期发展与应对方向。", "advanced", 6, 12, eight},
		{"六张关系牌阵", "Six Card Relationship Spread", "观察双方状态、关系基础、潜在矛盾与发展趋势。", "advanced", 6, 13, eight},
		{"马蹄铁牌阵", "Horseshoe Spread", "从过去、现在、隐藏影响、阻碍、外部环境、建议与结果等方面综合分析。", "advanced", 7, 14, eight},
		{"魔法七张牌阵", "Magic Seven", "结合个人态度、外部环境、内在期待与未来趋势进行综合解读。", "advanced", 7, 15, eight},
		{"七张综合分析牌阵", "Seven-Card Comprehensive Analysis", "从多角度综合分析问题的背景、现状与可能结果。", "advanced", 7, 16, eight},
		{"七张双路径选择牌阵", "Seven-Card Dual Path", "比较两条行动路径的过程与结果，辅助做出选择。", "advanced", 7, 17, eight},
		{"凯尔特十字牌阵", "Celtic Cross", "全面分析问题现状、根源、阻碍、个人状态、外部环境及未来发展。", "panorama", 10, 18, ten},
		{"生命之树牌阵", "Tree of Life Spread", "依据卡巴拉生命之树的十个质点，逐层分析问题的不同层面。", "panorama", 10, 19, nil},
	}
}

func placeholderPositions(n int) string {
	pos := make([]position, n)
	for i := range pos {
		pos[i] = position{Index: i + 1}
	}
	b, _ := json.Marshal(pos)
	return string(b)
}

func (s *Store) seedOrdinarySpreads(ctx context.Context) error {
	for _, item := range ordinarySpreadSeeds() {
		var n int
		if err := s.QueryRow(ctx, `SELECT count(*) FROM spreads WHERE name=$1 AND theme_key=''`, item.Name).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			if _, err := s.Exec(ctx, `UPDATE spreads SET
				name_en=CASE WHEN name_en='' THEN $1 ELSE name_en END,
				description=CASE WHEN description='' THEN $2 ELSE description END
				WHERE name=$3 AND theme_key=''`, item.NameEN, item.Desc, item.Name); err != nil {
				return err
			}
			continue
		}
		var cost any
		if item.Cost != nil {
			cost = *item.Cost
		}
		if _, err := s.Exec(ctx, `INSERT INTO spreads(name, name_en, description, card_count, positions, enabled, charged, recommended, depth_group, quota_cost, sort_order)
			VALUES ($1,$2,$3,$4,$5,true,false,false,$6,$7,$8)`,
			item.Name, item.NameEN, item.Desc, item.Cards, placeholderPositions(item.Cards), item.Group, cost, item.Sort); err != nil {
			return err
		}
	}
	return nil
}

type exploreSpreadSeed struct {
	ThemeKey, ThemeTitle, ThemeSubtitle string
	Name, Short, Desc                   string
	Cards, Sort                         int
}

func exploreSpreadSeeds() []exploreSpreadSeed {
	themes := []struct {
		key, title, subtitle string
		spreads              []struct {
			name, short, desc string
			cards             int
		}
	}{
		{"yes_or_no", "Yes or No", "听听此刻的倾向", []struct {
			name, short, desc string
			cards             int
		}{
			{"单张是非牌阵", "快速看清当下倾向", "用一张牌观察问题当前的发展倾向，适合明确、范围较小的二元问题。", 1},
		}},
		{"either_or", "二选一", "换个角度看选择", []struct {
			name, short, desc string
			cards             int
		}{
			{"双项对照牌阵", "先看两个选项的总体倾向", "快速比较两个选项的总体倾向，适合选择明确、希望先获得初步视角的场景。", 3},
			{"双路径决策牌阵", "分别看清 A / B 两条路", "分别观察 A、B 两条路径的优势、挑战和可能发展，避免只比较表面好坏。", 7},
		}},
		{"relationship", "关系走向", "看见关系状态，找到更适合你的理解方式", []struct {
			name, short, desc string
			cards             int
		}{
			{"关系现状牌阵", "看清彼此当下状态", "从你、对方、关系整体三个角度，快速理解这段关系此刻的能量。", 3},
			{"双方视角牌阵", "理解彼此在想什么", "查看双方感受、期待与顾虑，帮助理解误会和卡点来自哪里。", 4},
			{"关系走向牌阵", "看看这段关系会如何发展", "结合当前基础、影响因素与未来趋势，帮助判断接下来的发展方向。", 5},
		}},
		{"communication", "沟通建议", "找到表达方式", []struct {
			name, short, desc string
			cards             int
		}{
			{"沟通三张牌阵", "理清这一次怎么说", "聚焦一次具体沟通，帮助用户梳理表达重点、沟通方式与可能带来的影响。", 3},
			{"沟通桥梁牌阵", "找到可执行的沟通入口", "从双方立场、误解来源和共同点出发，寻找更可执行的沟通入口。", 5},
		}},
		{"career", "职业方向", "梳理下一步", []struct {
			name, short, desc string
			cards             int
		}{
			{"职业发展牌阵", "看清下一步怎么走", "从当前职业位置、个人优势、阻碍、机会与行动建议，梳理下一步发展。", 5},
			{"凯尔特十字牌阵", "复杂职业问题的全景梳理", "适合复杂、牵涉多个因素的职业问题；牌位覆盖核心处境、挑战、根源、时间线、内外部影响与可能结果。", 10},
		}},
		{"study", "学习状态", "调整学习节奏", []struct {
			name, short, desc string
			cards             int
		}{
			{"学习三张牌阵", "快速看清学习卡点", "快速了解学习状态、主要阻碍与可以尝试的调整方向。", 3},
			{"马蹄形学习牌阵", "把学习进程摊开看", "采用七张马蹄形结构，从过去、现在、隐性影响、阻碍、外部因素、建议到可能发展，观察学习进程。", 7},
		}},
		{"interpersonal", "人际相处", "理清相处边界", []struct {
			name, short, desc string
			cards             int
		}{
			{"人际互动三张牌阵", "看清彼此互动模式", "快速观察自己、对方与双方互动模式，适合朋友、同事和家庭关系。", 3},
			{"冲突解决牌阵", "拆开冲突找到解法", "把冲突拆分为双方需求、冲突根源、共同点与可行的解决方式。", 5},
		}},
		{"shopping", "购物决策", "看清购买动机", []struct {
			name, short, desc string
			cards             int
		}{
			{"购买决策三张牌阵", "分清需求还是冲动", "快速检视购买动机、潜在代价和综合建议，帮助用户区分真实需求与冲动。", 3},
			{"购买决策牌阵", "更完整地权衡这笔消费", "更完整地分析需求、价值、成本、替代方案与决策建议；不替代真实商品信息比较。", 5},
		}},
		{"outfit", "穿搭灵感", "找到今日风格", []struct {
			name, short, desc string
			cards             int
		}{
			{"今日风格牌阵", "把牌义变成今日穿搭灵感", "把牌面转译为当日风格、色彩和搭配重点，强调灵感而非预测。", 3},
			{"个人风格探索牌阵", "梳理更长期的风格方向", "将牌义转化为个人气质、风格方向、色彩材质、服装廓形和配饰建议。", 5},
		}},
		{"wellbeing", "身心关照", "给自己一点照顾", []struct {
			name, short, desc string
			cards             int
		}{
			{"身心灵牌阵", "关照心智、身体与意义", "从心智、身体感受和内在意义三个角度进行自我关照，不作医学判断。", 3},
			{"身心平衡牌阵", "找到可执行的日常关照", "识别压力来源、情绪需求与支持资源，给出可执行的日常关照建议。", 5},
		}},
		{"inner", "内心需求", "听见自己的心", []struct {
			name, short, desc string
			cards             int
		}{
			{"内心三问牌阵", "分清愿望、需求与行动", "区分意识到的愿望、尚未察觉的需求与可采取的行动。", 3},
			{"阴影探索牌阵", "看见反复出现的内在模式", "探索反复出现的内在模式、触发因素、保护机制、未满足需求与整合行动；避免将牌义解释成心理诊断。", 5},
		}},
	}
	out := make([]exploreSpreadSeed, 0, 22)
	sort := 100
	for _, theme := range themes {
		for _, sp := range theme.spreads {
			sort++
			out = append(out, exploreSpreadSeed{
				ThemeKey: theme.key, ThemeTitle: theme.title, ThemeSubtitle: theme.subtitle,
				Name: sp.name, Short: sp.short, Desc: sp.desc, Cards: sp.cards, Sort: sort,
			})
		}
	}
	return out
}

func (s *Store) seedExploreSpreads(ctx context.Context) error {
	for _, item := range exploreSpreadSeeds() {
		var n int
		if err := s.QueryRow(ctx, `SELECT count(*) FROM spreads WHERE name=$1 AND theme_key=$2`, item.Name, item.ThemeKey).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			continue
		}
		if _, err := s.Exec(ctx, `INSERT INTO spreads(name, description, card_count, positions, enabled, charged, recommended, theme_key, theme_title, theme_subtitle, short_sentence, depth_group, quota_cost, sort_order)
			VALUES ($1,$2,$3,$4,true,false,false,$5,$6,$7,$8,'',NULL,$9)`,
			item.Name, item.Desc, item.Cards, placeholderPositions(item.Cards),
			item.ThemeKey, item.ThemeTitle, item.ThemeSubtitle, item.Short, item.Sort); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) seedDeckShop(ctx context.Context) error {
	decks := []struct {
		code, name, tags string
		sort             int
	}{
		{"star-realm", "星辰之境", `["经典"]`, 10},
		{"moon-journey", "月影之旅", `["治愈"]`, 20},
		{"forest-song", "林风之歌", `["自然"]`, 30},
		{"secret-flower", "秘境之花", `["梦幻"]`, 40},
	}
	for _, d := range decks {
		if _, err := s.Exec(ctx, `INSERT INTO decks(code, name, enabled, sort_order, tags, unlock_required)
			VALUES ($1,$2,true,$3,$4,true)
			ON CONFLICT (code) DO NOTHING`, d.code, d.name, d.sort, d.tags); err != nil {
			return err
		}
	}
	four := `["star-realm","moon-journey","forest-song","secret-flower"]`
	products := []struct {
		code, kind, name, codes, note string
		price, sort                   int
		future                        bool
	}{
		{"deck_year", "deck_year", "年度畅享", four, "会员有效期内，后续新上架的牌组免费使用。", 9800, 1, true},
		{"deck_bundle_4", "deck_bundle", "四套打包", four, "单独购买过的牌组，再购买四套套餐不抵扣已购金额。", 1200, 2, false},
		{"deck_single_star", "deck_single", "星辰之境", `["star-realm"]`, "", 660, 3, false},
		{"deck_single_moon", "deck_single", "月影之旅", `["moon-journey"]`, "", 660, 4, false},
		{"deck_single_forest", "deck_single", "林风之歌", `["forest-song"]`, "", 660, 5, false},
		{"deck_single_flower", "deck_single", "秘境之花", `["secret-flower"]`, "", 660, 6, false},
		{"deck_addon", "deck_addon", "后续牌组打包", `[]`, "后续新增的牌组，将按 ¥3.00 / 套打包售卖。", 300, 7, false},
	}
	for _, p := range products {
		if _, err := s.Exec(ctx, `INSERT INTO iap_skus(code, kind, display_name, price_amount_minor, price_currency, deck_codes, covers_future, deduct_owned, note, sort_order)
			VALUES ($1,$2,$3,$4,'CNY',$5,$6,false,$7,$8)
			ON CONFLICT (code) DO NOTHING`,
			p.code, p.kind, p.name, p.price, p.codes, p.future, p.note, p.sort); err != nil {
			return err
		}
	}
	return nil
}
