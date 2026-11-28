package readingflow

import (
	"strings"
	"testing"
)

func TestInterpretSystemModes(t *testing.T) {
	normal := InterpretSystem(ModeNormal)
	today := InterpretSystem(ModeDailyToday)
	tomorrow := InterpretSystem(ModeDailyTomorrow)
	explore := InterpretSystem(ModeExplore)
	for _, text := range []string{normal, today, tomorrow, explore} {
		if !strings.Contains(text, "问题导向") || !strings.Contains(text, "整体解读") || !strings.Contains(text, InterpretRulesVersion) {
			t.Fatalf("missing common rules: %s", text[:min(80, len(text))])
		}
		if strings.Contains(text, "逐张解释后给总评") {
			t.Fatalf("old per-card summary instruction still present")
		}
	}
	if !strings.Contains(normal, "普通占卜") || !strings.Contains(normal, "整体叙事") {
		t.Fatalf("normal mode: %s", normal[len(normal)-200:])
	}
	if !strings.Contains(today, "今日运势") || !strings.Contains(tomorrow, "明日运势") {
		t.Fatalf("daily modes missing")
	}
	if !strings.Contains(tomorrow, "前瞻") && !strings.Contains(tomorrow, "不得把明日运势写成今日") {
		t.Fatalf("tomorrow mode weak: %s", tomorrow[len(tomorrow)-240:])
	}
	if !strings.Contains(explore, "探索") {
		t.Fatalf("explore mode: %s", explore[len(explore)-200:])
	}
	if InterpretSystem(ModeTarotist) != tarotistSystem {
		t.Fatalf("tarotist should stay persona system")
	}
}

func TestInterpretationLeakReason(t *testing.T) {
	if got := InterpretationLeakReason("今天整体偏稳，适合把精力放在具体事务上。"); got != "" {
		t.Fatalf("clean text flagged: %s", got)
	}
	if got := InterpretationLeakReason("以下仅根据给定牌面、排位与正逆位进行解读。\n今天…"); got != "internal_instruction_leak" {
		t.Fatalf("leak missed: %s", got)
	}
	if got := InterpretationLeakReason(""); got != "empty" {
		t.Fatalf("empty: %s", got)
	}
}

func TestStripInterpretationLeak(t *testing.T) {
	cleaned, ok := StripInterpretationLeak("以下仅根据给定牌面、排位与正逆位进行解读。\n今天整体偏稳，适合把精力放在具体事务上。")
	if !ok || !strings.Contains(cleaned, "今天整体偏稳") || strings.Contains(cleaned, "以下仅根据") {
		t.Fatalf("strip: ok=%v text=%q", ok, cleaned)
	}
	if reason := InterpretationLeakReason(cleaned); reason != "" {
		t.Fatalf("cleaned still leak: %s", reason)
	}
}
