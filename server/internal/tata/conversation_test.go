package tata

import (
	"fmt"
	"testing"
)

func TestConversationTypeOf(t *testing.T) {
	cases := []struct {
		kind, theme, want string
	}{
		{"daily", "", "daily"},
		{"normal", "", "moment"},
		{"tarotist", "", "moment"},
		{"explore", "yes_or_no", "moment"},
		{"explore", "either_or", "moment"},
		{"explore", "relationship", "theme"},
		{"explore", "", ""},
	}
	for _, c := range cases {
		if got := conversationTypeOf(c.kind, c.theme); got != c.want {
			t.Fatalf("%s/%s: got %q want %q", c.kind, c.theme, got, c.want)
		}
	}
}

func TestProgressOfReadingContinue(t *testing.T) {
	cases := []struct {
		status string
		skip   bool
		mode   string
		want   string
	}{
		{"QUESTION_CREATED", false, "", "detect"},
		{"QUESTION_CREATED", true, "", "spread"},
		{"SPREAD_SELECTED", false, "normal", "shuffle"},
		{"SPREAD_SELECTED", false, "numerology", "numerology"},
		{"SHUFFLING", false, "numerology", "numerology"},
		{"CARDS_FIXED", false, "numerology", "interpret"},
		{"CARDS_FIXED", false, "normal", "interpret"},
	}
	for _, c := range cases {
		_, next := progressOfReading(c.status, c.skip, c.mode)
		if next != c.want {
			t.Fatalf("%s skip=%v mode=%s: continue=%q want %q", c.status, c.skip, c.mode, next, c.want)
		}
	}
}

func TestNumerologyMetaDoesNotFakeCardsFixed(t *testing.T) {
	// Even if a row somehow has cards while status is still SHUFFLING, the API
	// must not invent CARDS_FIXED / continue=interpret. Truthful status drives
	// continue; POST /numerology is what locks to CARDS_FIXED.
	item := map[string]any{
		"type": "normal", "theme_key": "", "business_date": "", "target_date": "",
		"draw_mode": "numerology", "skip_detect": false, "status": "SHUFFLING",
		"cards": `[{"name":"愚者","orientation":"upright"}]`,
	}
	attachConversationMeta(item, "2099-01-01")
	if item["continue"] != "numerology" {
		t.Fatalf("continue=%v want numerology", item["continue"])
	}
	if item["stage"] != "pending_numerology" {
		t.Fatalf("stage=%v want pending_numerology", item["stage"])
	}
	item2 := map[string]any{
		"type": "normal", "theme_key": "", "business_date": "", "target_date": "",
		"draw_mode": "numerology", "skip_detect": false, "status": "CARDS_FIXED",
		"cards": `[{"name":"愚者","orientation":"upright"}]`,
	}
	attachConversationMeta(item2, "2099-01-01")
	if item2["continue"] != "interpret" {
		t.Fatalf("CARDS_FIXED continue=%v want interpret", item2["continue"])
	}
	if item2["stage"] != "cards_fixed" {
		t.Fatalf("CARDS_FIXED stage=%v want cards_fixed", item2["stage"])
	}
}

func TestConversationLabelAndStage(t *testing.T) {
	if got := conversationLabelOf("daily", "", "tomorrow"); got != "普通占卜" {
		t.Fatalf("daily tomorrow: %q", got)
	}
	if got := conversationTypeForItem("daily", "", "tomorrow"); got != "daily" {
		t.Fatalf("daily tomorrow type: %q", got)
	}
	if got := conversationLabelOf("daily", "", "today"); got != "今日运势" {
		t.Fatalf("daily today: %q", got)
	}
	if got := conversationLabelOf("explore", "yes_or_no", ""); got != "此刻抉择" {
		t.Fatalf("moment explore: %q", got)
	}
	if got := conversationLabelOf("explore", "career", ""); got != "主题占卜" {
		t.Fatalf("theme: %q", got)
	}
	stage, label := stageOf("daily", "QUESTION_CREATED")
	if stage != "pending_draw" || label != "待抽牌" {
		t.Fatalf("daily stage: %s %s", stage, label)
	}
	stage, label = stageOf("theme", "QUESTION_CREATED")
	if stage != "pending_spread" || label != "待选牌阵" {
		t.Fatalf("theme stage: %s %s", stage, label)
	}
	stage, label = stageOf("moment", "QUESTION_CREATED")
	if stage != "question_submitted" || label != "问题已提交" {
		t.Fatalf("moment stage: %s %s", stage, label)
	}
}

func TestHistoryConversationFields(t *testing.T) {
	store, ts := newMergeServer(t)
	token, uid := registerUser(t, ts, store, "histconv")
	if _, err := store.Exec(t.Context(), `INSERT INTO readings(user_id, type, question, status, theme_key, cards, interpretation)
		VALUES ($1,'explore','会联系我吗','COMPLETED','yes_or_no','[{"name":"愚者"}]','倾向是会的。')`, uid); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Exec(t.Context(), `INSERT INTO readings(user_id, type, question, status, theme_key)
		VALUES ($1,'explore','','QUESTION_CREATED','relationship')`, uid); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Exec(t.Context(), `INSERT INTO readings(user_id, type, question, status, business_date, target_date, quota_cycle)
		VALUES ($1,'daily','明日运势','CARDS_FIXED',$2,$3,$2)`, uid, "2099-01-01", "2099-01-02"); err != nil {
		t.Fatal(err)
	}
	page := listOne(t, ts, token, "/api/app/v1/readings?limit=10")
	items, _ := page["items"].([]any)
	if len(items) < 3 {
		t.Fatalf("items: %#v", page)
	}
	byType := map[string]map[string]any{}
	for _, raw := range items {
		item := raw.(map[string]any)
		byType[fmtString(item["conversation_type"])+"|"+fmtString(item["conversation_label"])] = item
	}
	moment := byType["moment|此刻抉择"]
	if moment == nil || moment["stage"] != "completed" {
		t.Fatalf("moment: %#v", byType)
	}
	theme := byType["theme|主题占卜"]
	if theme == nil || theme["stage"] != "pending_spread" || theme["stage_label"] != "待选牌阵" {
		t.Fatalf("theme: %#v", theme)
	}
	// Advance-drawn tomorrow fortune: label 普通占卜, type stays daily for 运势解读 UI.
	daily := byType["daily|普通占卜"]
	if daily == nil || daily["target"] != "tomorrow" || daily["stage"] != "pending_interpret" {
		t.Fatalf("daily tomorrow as daily+普通占卜: %#v", daily)
	}
	if daily["business_date"] != "2099-01-01" || daily["target_date"] != "2099-01-02" {
		t.Fatalf("daily dates: %#v", daily)
	}
	if daily["draw_date"] != "2099-01-01" {
		t.Fatalf("daily draw_date: %#v", daily["draw_date"])
	}
}

func TestGetReadingIncludesMessages(t *testing.T) {
	store, ts := newMergeServer(t)
	token, uid := registerUser(t, ts, store, "readdetail")
	if _, err := store.Exec(t.Context(), `INSERT INTO readings(user_id, type, question, status, cards, interpretation)
		VALUES ($1,'normal','工作怎么选','COMPLETED','[{"name":"愚者"}]','先把选择的代价看清楚。')`, uid); err != nil {
		t.Fatal(err)
	}
	var id int64
	if err := store.QueryRow(t.Context(), `SELECT id FROM readings WHERE user_id=$1 ORDER BY id DESC LIMIT 1`, uid).Scan(&id); err != nil {
		t.Fatal(err)
	}
	item := listOne(t, ts, token, fmt.Sprintf("/api/app/v1/readings/%d", id))
	if item["id"] == nil || item["reading_id"] == nil {
		t.Fatalf("missing ids: %#v", item)
	}
	msgs, _ := item["messages"].([]any)
	if len(msgs) == 0 {
		t.Fatalf("expected messages in detail: %#v", item)
	}
	if item["has_more"] != false {
		t.Fatalf("has_more: %#v", item["has_more"])
	}
	if item["conversation_type"] != "moment" {
		t.Fatalf("conversation_type: %#v", item["conversation_type"])
	}
}

func TestCreateExploreThemeKeyAndSpreadBackfill(t *testing.T) {
	store, ts := newMergeServer(t)
	token, _ := registerUser(t, ts, store, "themekey")
	created := create(t, ts, token, "/api/app/v1/readings", map[string]any{
		"type": "explore", "source": "explore", "theme_key": "yes_or_no",
	})
	if created["conversation_type"] != "moment" || created["conversation_label"] != "此刻抉择" {
		t.Fatalf("created: %#v", created)
	}
	if created["skip_detect"] != true || created["continue"] != "spread" || created["stage"] != "pending_spread" {
		t.Fatalf("explore resume fields: %#v", created)
	}
	preset := create(t, ts, token, "/api/app/v1/readings", map[string]any{
		"question": "他还会联系我吗？", "type": "normal", "source": "preset",
	})
	if preset["skip_detect"] != true || preset["continue"] != "spread" || preset["stage"] != "pending_spread" {
		t.Fatalf("preset resume fields: %#v", preset)
	}
	hand := create(t, ts, token, "/api/app/v1/readings", map[string]any{
		"question": "我该不该换工作？", "type": "normal",
	})
	if hand["skip_detect"] != false || hand["continue"] != "detect" {
		t.Fatalf("hand resume fields: %#v", hand)
	}
	var spreadID int64
	if err := store.QueryRow(t.Context(), `SELECT id FROM spreads WHERE theme_key='relationship' AND enabled=true ORDER BY id LIMIT 1`).Scan(&spreadID); err != nil {
		t.Fatal(err)
	}
	blank := create(t, ts, token, "/api/app/v1/readings", map[string]any{
		"type": "explore", "source": "explore",
	})
	if blank["conversation_type"] != "" {
		t.Fatalf("blank explore should wait for theme: %#v", blank)
	}
	id := asID(blank["id"])
	must(t, ts, token, "POST", fmt.Sprintf("/api/app/v1/readings/%d/spread", id), map[string]any{"spread_id": spreadID})
	item := listOne(t, ts, token, fmt.Sprintf("/api/app/v1/readings/%d", id))
	if item["theme_key"] != "relationship" || item["conversation_type"] != "theme" || item["conversation_label"] != "主题占卜" {
		t.Fatalf("backfill: %#v", item)
	}
}
