package readingflow

import "strings"

// leakPrefixes are work-note openings that must never reach the user (interpret-rules §六).
var leakPrefixes = []string{
	"以下仅根据给定牌面",
	"不修改牌面、不修改牌位",
	"不修改牌面",
	"以下将结合过去、现在、未来",
	"首先分析第一张牌",
	"通过以上分析可以得出",
	"我将根据系统提供的信息",
	"本次解读遵循以下规则",
	"系统提示词",
	"开发者指令",
}

// StripInterpretationLeak removes leading work-note / fence / JSON-shaped prefixes.
// Returns cleaned text; ok=false when nothing usable remains.
func StripInterpretationLeak(text string) (string, bool) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "", false
	}
	for {
		trimmed = strings.TrimSpace(trimmed)
		if trimmed == "" {
			return "", false
		}
		changed := false
		if strings.HasPrefix(trimmed, "```") {
			if end := strings.Index(trimmed[3:], "```"); end >= 0 {
				trimmed = strings.TrimSpace(trimmed[3+end+3:])
				changed = true
				continue
			}
			trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "```"))
			changed = true
			continue
		}
		for _, p := range leakPrefixes {
			if strings.HasPrefix(trimmed, p) {
				rest := strings.TrimSpace(trimmed[len(p):])
				rest = strings.TrimLeft(rest, "：:。；;，,\n\r\t ")
				trimmed = rest
				changed = true
				break
			}
			needle := "\n" + p
			if i := strings.Index(trimmed, needle); i >= 0 && i < 240 {
				trimmed = strings.TrimSpace(trimmed[:i] + trimmed[i+len(needle):])
				changed = true
				break
			}
		}
		if !changed {
			break
		}
	}
	if trimmed == "" {
		return "", false
	}
	if strings.HasPrefix(trimmed, "{") && strings.Contains(trimmed, "\"") {
		return "", false
	}
	return trimmed, true
}

// InterpretationLeakReason returns a non-empty reason when output looks like leaked internals.
// Only the opening window is checked for work-note prefixes, to avoid false positives in long body text.
func InterpretationLeakReason(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "empty"
	}
	head := trimmed
	if rs := []rune(trimmed); len(rs) > 240 {
		head = string(rs[:240])
	}
	for _, p := range leakPrefixes {
		if strings.HasPrefix(head, p) || strings.Contains(head, "\n"+p) {
			return "internal_instruction_leak"
		}
	}
	if strings.HasPrefix(trimmed, "```") {
		return "code_fence"
	}
	if strings.HasPrefix(trimmed, "{") && strings.Contains(head, "\"") {
		return "json_shaped"
	}
	return ""
}
