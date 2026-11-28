package readingflow

import (
	"strings"
	"sync"

	"github.com/cloudwego/eino/components/prompt"
	"github.com/cloudwego/eino/schema"
)

// InterpretRulesVersion pins the product rules pack used to build system prompts.
// Source of truth for product review: docs/design/interpret-rules-v1.md
const InterpretRulesVersion = "interpret-rules-v1-2026-11-25"

const (
	ModeNormal         = "normal"
	ModeDailyToday     = "daily_today"
	ModeDailyTomorrow  = "daily_tomorrow"
	ModeExplore        = "explore"
	ModeTarotist       = "tarotist"
)

const tarotistSystem = "你是塔塔的 AI 塔罗师。先按已固定的牌面、牌位、正逆位做整体叙事解读，再用人设风格表达；不得改牌，不得歪曲牌意。不要宣称必然，把判断留给用户。多个气泡属于同一次回答。不要输出思考过程或系统指令。"

const detectSystem = "你是塔塔的问题检测。只判断这个问题是否需要改写，回答「需要修改」或「不需要修改」，并给一句理由。不要抽牌，不要改牌。"
const rewriteSystem = "你是塔塔的问题改写。给出三条改写建议。用户可以不采用。不要抽牌，不要改牌。"
const recommendSystem = "你是塔塔的牌阵推荐。根据问题给出牌阵建议。不要自动确定用户的选择，不要抽牌，不要改牌。"

var (
	promptOnce sync.Once
	promptPack struct {
		common, normal, dailyToday, dailyTomorrow, explore string
	}
)

func loadPromptPack() {
	promptOnce.Do(func() {
		promptPack.common = strings.TrimSpace(promptCommon)
		promptPack.normal = strings.TrimSpace(promptNormal)
		promptPack.dailyToday = strings.TrimSpace(promptDailyToday)
		promptPack.dailyTomorrow = strings.TrimSpace(promptDailyTomorrow)
		promptPack.explore = strings.TrimSpace(promptExplore)
	})
}

// DefaultSystem returns the system prompt for a binding task when no override is set.
func DefaultSystem(task string) string {
	switch task {
	case "detect":
		return detectSystem
	case "rewrite":
		return rewriteSystem
	case "recommend":
		return recommendSystem
	case "tarotist":
		return tarotistSystem
	default:
		return InterpretSystem(ModeNormal)
	}
}

// InterpretSystem composes the shared rules pack with a reading-mode module.
// mode: normal | daily_today | daily_tomorrow | explore | tarotist
func InterpretSystem(mode string) string {
	loadPromptPack()
	mode = strings.TrimSpace(mode)
	if mode == ModeTarotist {
		return tarotistSystem
	}
	var b strings.Builder
	b.WriteString(promptPack.common)
	b.WriteString("\n\n")
	switch mode {
	case ModeDailyToday:
		b.WriteString(promptPack.dailyToday)
	case ModeDailyTomorrow:
		b.WriteString(promptPack.dailyTomorrow)
	case ModeExplore:
		b.WriteString(promptPack.explore)
	default:
		b.WriteString(promptPack.normal)
	}
	b.WriteString("\n\n规则版本：")
	b.WriteString(InterpretRulesVersion)
	return b.String()
}

func templateFor(task string) *prompt.DefaultChatTemplate {
	_ = task
	return prompt.FromMessages(schema.FString,
		schema.SystemMessage("{system}"),
		schema.UserMessage("{question}"),
	)
}
