package tata

import "strings"

// readingSelectCols is the column list scanned by scanReading.
const readingSelectCols = `id, user_id, type, question, spread_id, tarotist_id, status, cards, interpretation, vendor_name, model_name, fail_reason, quota_consumed, first_experience, created_at, updated_at, deleted_at, draw_mode, theme_key, business_date, COALESCE(target_date,''), skip_detect`

// momentThemeKeys are explore themes that share the「普通占卜 / 此刻抉择」conversation flow.
var momentThemeKeys = map[string]bool{
	"yes_or_no": true,
	"either_or": true,
}

var knownExploreThemeKeys = map[string]bool{
	"yes_or_no": true, "either_or": true, "relationship": true, "communication": true,
	"career": true, "study": true, "interpersonal": true, "shopping": true,
	"outfit": true, "wellbeing": true, "inner": true,
}

func validateExploreThemeKey(key string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil
	}
	if !knownExploreThemeKeys[key] {
		return biz("探索主题不存在")
	}
	return nil
}

// conversationTypeOf maps storage fields to the three history conversation kinds.
// daily → 今日/明日运势；moment → 普通占卜 + 探索此刻抉择；theme → 主题占卜。
// Explore without theme_key yet returns "" until create/spread fills it.
// Tomorrow fortune stays daily (运势解读 UI); only its label is「普通占卜」.
func conversationTypeOf(kind, themeKey string) string {
	switch kind {
	case "daily":
		return "daily"
	case "normal", "tarotist":
		return "moment"
	case "explore":
		key := strings.TrimSpace(themeKey)
		if key == "" {
			return ""
		}
		if momentThemeKeys[key] {
			return "moment"
		}
		return "theme"
	default:
		return ""
	}
}

func conversationLabelOf(kind, themeKey, target string) string {
	// Advance-drawn tomorrow fortune is filed under the draw day as 普通占卜 for calendar/history UI.
	if kind == "daily" && target == "tomorrow" {
		return "普通占卜"
	}
	switch conversationTypeOf(kind, themeKey) {
	case "daily":
		return "今日运势"
	case "moment":
		if kind == "explore" && momentThemeKeys[strings.TrimSpace(themeKey)] {
			return "此刻抉择"
		}
		if kind == "tarotist" {
			return "占卜师"
		}
		return "普通占卜"
	case "theme":
		return "主题占卜"
	default:
		return ""
	}
}

func conversationTypeForItem(kind, themeKey, target string) string {
	// Tomorrow fortune keeps conversation_type=daily (App「运势解读」屏).
	// Only conversation_label is「普通占卜」; callers still pass target for clarity.
	_ = target
	return conversationTypeOf(kind, themeKey)
}

func dailyTargetOf(kind, businessDate, targetDate, todayCycle string) string {
	if kind != "daily" {
		return ""
	}
	targetDate = strings.TrimSpace(targetDate)
	businessDate = strings.TrimSpace(businessDate)
	fortuneDay := targetDate
	if fortuneDay == "" {
		fortuneDay = businessDate
	}
	if fortuneDay == "" {
		return "today"
	}
	if todayCycle != "" && fortuneDay > todayCycle {
		return "tomorrow"
	}
	return "today"
}

// stageOf returns a client-stable stage key and Chinese label for the conversation kind.
// status / progress stay unchanged for backward compatibility.
func stageOf(conversationType, status string) (stage, label string) {
	switch conversationType {
	case "daily":
		return dailyStage(status)
	case "theme":
		return themeStage(status)
	case "moment":
		return momentStage(status)
	default:
		return momentStage(status)
	}
}

func dailyStage(status string) (string, string) {
	switch status {
	case "QUESTION_CREATED", "SPREAD_SELECTED", "SHUFFLING", "CUTTING":
		return "pending_draw", "待抽牌"
	case "CARDS_FIXED":
		return "pending_interpret", "待解读"
	case "INTERPRETING":
		return "interpreting", "解读生成中"
	case "COMPLETED":
		return "completed", "已完成"
	case "FAILED_RETRYABLE":
		return "failed_retryable", "失败可重试"
	default:
		return strings.ToLower(status), status
	}
}

func momentStage(status string) (string, string) {
	switch status {
	case "QUESTION_CREATED":
		return "question_submitted", "问题已提交"
	case "SPREAD_SELECTED":
		return "spread_selected", "牌阵已选择"
	case "SHUFFLING":
		return "shuffling", "洗牌中"
	case "CUTTING":
		return "cutting", "切牌中"
	case "CARDS_FIXED":
		return "cards_fixed", "牌面已固定"
	case "INTERPRETING":
		return "interpreting", "解读生成中"
	case "COMPLETED":
		return "completed", "已完成"
	case "FAILED_RETRYABLE":
		return "failed_retryable", "失败可重试"
	default:
		return strings.ToLower(status), status
	}
}

func themeStage(status string) (string, string) {
	switch status {
	case "QUESTION_CREATED":
		return "pending_spread", "待选牌阵"
	case "SPREAD_SELECTED":
		return "spread_selected", "牌阵已选择"
	case "SHUFFLING":
		return "shuffling", "洗牌中"
	case "CUTTING":
		return "cutting", "切牌中"
	case "CARDS_FIXED":
		return "cards_fixed", "牌面已固定"
	case "INTERPRETING":
		return "interpreting", "解读生成中"
	case "COMPLETED":
		return "completed", "已完成"
	case "FAILED_RETRYABLE":
		return "failed_retryable", "失败可重试"
	default:
		return strings.ToLower(status), status
	}
}

func attachConversationMeta(item map[string]any, todayCycle string) {
	if item == nil {
		return
	}
	kind, _ := item["type"].(string)
	themeKey, _ := item["theme_key"].(string)
	business, _ := item["business_date"].(string)
	targetDate, _ := item["target_date"].(string)
	drawMode, _ := item["draw_mode"].(string)
	skip, _ := item["skip_detect"].(bool)
	target := dailyTargetOf(kind, business, targetDate, todayCycle)
	conv := conversationTypeForItem(kind, themeKey, target)
	label := conversationLabelOf(kind, themeKey, target)
	status := fmtString(item["status"])
	stage, stageLabel := stageOf(conv, status)
	// Preset/explore skip detect: QUESTION_CREATED should resume at spread select.
	if status == "QUESTION_CREATED" && skip {
		stage, stageLabel = "pending_spread", "待选牌阵"
	}
	progress, next := progressOfReading(status, skip, drawMode)
	// Numerology has no server shuffle/cut/draw. Until POST /numerology locks
	// cards (status=CARDS_FIXED), resume stage is pending_numerology so the
	// client does not open ordinary shuffle/cut UI. After cards are fixed,
	// stage/continue follow the same path as ordinary readings (interpret).
	if strings.TrimSpace(drawMode) == "numerology" {
		switch status {
		case "SPREAD_SELECTED", "SHUFFLING", "CUTTING":
			stage, stageLabel = "pending_numerology", "待灵数抽牌"
		}
	}
	item["conversation_type"] = conv
	item["conversation_label"] = label
	item["stage"] = stage
	item["stage_label"] = stageLabel
	item["progress"] = progress
	item["continue"] = next
	if kind == "daily" {
		item["target"] = target
		if business != "" {
			item["business_date"] = business
		}
		if strings.TrimSpace(targetDate) == "" && business != "" {
			// Legacy rows: fortune day lived in business_date before target_date existed.
			item["target_date"] = business
		} else if strings.TrimSpace(targetDate) != "" {
			item["target_date"] = targetDate
		}
	}
}

func fmtString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case *string:
		if t != nil {
			return *t
		}
	}
	return ""
}

