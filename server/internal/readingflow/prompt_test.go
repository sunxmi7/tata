package readingflow

import (
	"strings"
	"testing"
)

func TestInterpretSystemHasSharedFramework(t *testing.T) {
	if !strings.Contains(interpretSystem, "牌面") || !strings.Contains(interpretSystem, "牌位") ||
		!strings.Contains(interpretSystem, "关系") || !strings.Contains(interpretSystem, "启发") ||
		!strings.Contains(interpretSystem, "不得改牌") || !strings.Contains(interpretSystem, "必然") {
		t.Fatalf("interpret system missing framework: %s", interpretSystem)
	}
}

func TestTarotistSystemHasSharedFramework(t *testing.T) {
	if !strings.Contains(tarotistSystem, "牌面→牌位→关系→启发") ||
		!strings.Contains(tarotistSystem, "不得改牌") ||
		!strings.Contains(tarotistSystem, "必然") ||
		!strings.Contains(tarotistSystem, "判断") {
		t.Fatalf("tarotist system missing framework: %s", tarotistSystem)
	}
}
