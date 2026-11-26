package tata

import (
	"strings"
	"testing"
)

func TestLookupMajorMeaningFoolUpright(t *testing.T) {
	kw, meaning, ok := LookupMajorMeaning(0, "upright")
	if !ok {
		t.Fatal("expected entry for 愚者 upright")
	}
	if len(kw) < 3 {
		t.Fatalf("keywords: %#v", kw)
	}
	if meaning == "" {
		t.Fatal("empty meaning_short")
	}
}

func TestLookupMajorMeaningSkipsMinor(t *testing.T) {
	_, _, ok := LookupMajorMeaning(22, "upright")
	if ok {
		t.Fatal("minor card should not lookup")
	}
}

func TestLookupMajorMeaningTowerReversed(t *testing.T) {
	kw, meaning, ok := LookupMajorMeaning(16, "reversed")
	if !ok {
		t.Fatal("expected 高塔 reversed")
	}
	if !strings.Contains(strings.Join(kw, ""), "延迟") && !strings.Contains(strings.Join(kw, ""), "恐惧") {
		t.Fatalf("unexpected keywords: %#v", kw)
	}
	if !strings.Contains(meaning, "危机") && meaning == "" {
		t.Fatal("missing meaning")
	}
}

func TestBuildPromptIncludesMajorMeanings(t *testing.T) {
	cards := `[{"position":1,"position_name":"现状","card_id":0,"name":"愚者","orientation":"upright"},{"position":2,"position_name":"建议","card_id":16,"name":"高塔","orientation":"reversed"}]`
	out := buildPrompt("normal", "近期工作是否该变动？", cards)
	if !strings.Contains(out, "牌意参考") {
		t.Fatalf("missing reference block: %s", out)
	}
	if !strings.Contains(out, "关键词：新开始") {
		t.Fatalf("missing fool keywords: %s", out)
	}
	if !strings.Contains(out, "简义：") {
		t.Fatal("missing meaning_short label")
	}
	if !strings.Contains(out, "高塔") || !strings.Contains(out, "逆位") {
		t.Fatalf("missing tower reversed line: %s", out)
	}
}

func TestBuildPromptTarotistStillHasMeanings(t *testing.T) {
	cards := `[{"position":1,"position_name":"核心","card_id":19,"name":"太阳","orientation":"upright"}]`
	out := buildPrompt("tarotist", "感情走向？", cards)
	if !strings.Contains(out, "按所选塔罗师人设表达") {
		t.Fatal("tarotist hint missing")
	}
	if !strings.Contains(out, "太阳") || !strings.Contains(out, "清晰") {
		t.Fatalf("missing sun meaning: %s", out)
	}
}

func TestBuildInterpretPromptExploreUsesTheme(t *testing.T) {
	cards := `[{"position":1,"position_name":"当前问题的答案倾向与核心影响因素","card_id":0,"name":"愚者","orientation":"upright"}]`
	out := buildInterpretPrompt("explore", "", "Yes or No", "单张是非牌阵", cards, nil)
	if strings.Contains(out, "问题：") {
		t.Fatalf("should not fake a user question: %s", out)
	}
	if !strings.Contains(out, "用户未填写问题") {
		t.Fatalf("missing empty-question hint: %s", out)
	}
	if !strings.Contains(out, "主题：Yes or No") {
		t.Fatalf("missing theme: %s", out)
	}
	if !strings.Contains(out, "牌阵：单张是非牌阵") {
		t.Fatalf("missing spread: %s", out)
	}
	if !strings.Contains(out, "牌面：") {
		t.Fatalf("missing cards: %s", out)
	}
}

func TestBuildInterpretPromptKeepsRealQuestion(t *testing.T) {
	cards := `[{"position":1,"name":"愚者","orientation":"upright"}]`
	out := buildInterpretPrompt("explore", "他近期会主动联系我吗？", "Yes or No", "单张是非牌阵", cards, nil)
	if !strings.Contains(out, "问题：他近期会主动联系我吗？") {
		t.Fatalf("expected real question: %s", out)
	}
	if strings.Contains(out, "逐张解释后给总评") {
		t.Fatalf("old summary instruction leaked into user prompt: %s", out)
	}
	if interpretPromptMode("daily", "明日运势") != "daily_tomorrow" || interpretPromptMode("daily", "今日一牌") != "daily_today" {
		t.Fatalf("daily mode mapping failed")
	}
	if interpretPromptMode("normal", "x") != "normal" || interpretPromptMode("explore", "") != "explore" {
		t.Fatalf("kind mode mapping failed")
	}
	if strings.Contains(out, "主题：") {
		t.Fatalf("theme should not override real question: %s", out)
	}
}

func TestFormatPositionsBlockLightweight(t *testing.T) {
	cards := `[{"position":1,"position_name":"过去","card_id":0,"name":"愚者","orientation":"upright"},{"position":2,"position_name":"现在","card_id":1,"name":"魔术师","orientation":"reversed"}]`
	positions := []position{
		{Index: 1, Name: "过去", Meaning: "已经发生或仍在影响的背景"},
		{Index: 2, Name: "现在", Meaning: "当下状态与主要力量"},
		{Index: 3, Name: "未来", Meaning: "可能的发展方向"},
	}
	normal := buildInterpretPrompt("normal", "工作怎么办？", "", "时间流", cards, positions)
	if !strings.Contains(normal, "牌位（按抽牌顺序）：") {
		t.Fatalf("missing positions header: %s", normal)
	}
	if !strings.Contains(normal, "1. 过去：已经发生或仍在影响的背景 / 愚者 正位") {
		t.Fatalf("missing first position line: %s", normal)
	}
	if !strings.Contains(normal, "2. 现在：当下状态与主要力量 / 魔术师 逆位") {
		t.Fatalf("missing second position line: %s", normal)
	}
	if strings.Contains(normal, "3. 未来") {
		t.Fatalf("should follow drawn cards only, not unused positions: %s", normal)
	}

	daily := buildInterpretPrompt("daily", "今日一牌", "", "", `[{"position":1,"position_name":"今日","card_id":0,"name":"愚者","orientation":"upright"}]`,
		[]position{{Index: 1, Name: "今日", Meaning: "当天整体状态"}})
	if !strings.Contains(daily, "牌位：今日") {
		t.Fatalf("daily should only list slot name: %s", daily)
	}
	if strings.Contains(daily, "当天整体状态") || strings.Contains(daily, "牌位（按抽牌顺序）") {
		t.Fatalf("daily should not expand meaning: %s", daily)
	}
}
