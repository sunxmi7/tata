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
