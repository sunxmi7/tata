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
