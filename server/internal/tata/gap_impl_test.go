package tata

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMonthlyReportEmptySaveAndStats(t *testing.T) {
	store, ts := newMergeServer(t)
	token, uid := registerUser(t, ts, store, "monthrep")
	empty := listOne(t, ts, token, "/api/app/v1/monthly-reports")
	if empty["status"] != "empty" || empty["empty_title"] != "这个月还没有抽过牌" {
		t.Fatalf("empty month: %#v", empty)
	}
	if empty["exploration_count"].(float64) != 0 {
		t.Fatalf("empty count: %#v", empty["exploration_count"])
	}
	msg, _ := call(t, ts, token, http.MethodGet, "/api/app/v1/monthly-reports?month=2099-01", nil)
	if !strings.Contains(msg, "未来") {
		t.Fatalf("future month: %s", msg)
	}

	now := time.Now().In(time.FixedZone("CST", 8*3600))
	month := now.Format("2006-01")
	if _, err := store.Exec(t.Context(), `INSERT INTO readings(user_id, type, question, status, interpretation, created_at, updated_at)
		VALUES ($1,'normal','感情怎么样','COMPLETED','这段关系需要把话说清楚再往前走。',$2,$2),
		       ($1,'normal','事业方向','QUESTION_CREATED',NULL,$2,$2)`, uid, now); err != nil {
		t.Fatal(err)
	}
	view := listOne(t, ts, token, "/api/app/v1/monthly-reports?month="+month)
	if view["status"] != "ready" || view["exploration_count"].(float64) != 2 || view["companion_days"].(float64) != 1 {
		t.Fatalf("stats: %#v", view)
	}
	if view["quote_status"] != "ready" || !strings.Contains(fmt.Sprint(view["quote"]), "这段关系") {
		t.Fatalf("quote: %#v", view)
	}
	topics, _ := view["focus_topics"].([]any)
	if len(topics) == 0 {
		t.Fatalf("topics empty: %#v", view["focus_topics"])
	}
	saved := create(t, ts, token, "/api/app/v1/monthly-reports/save", map[string]any{"month": month})
	if saved["saved"] != true {
		t.Fatalf("save: %#v", saved)
	}
}

func TestYearlyReportUnlockRequiresYearCard(t *testing.T) {
	store, ts := newMergeServer(t)
	token, uid := registerUser(t, ts, store, "yearrep")
	locked := listOne(t, ts, token, "/api/app/v1/yearly-reports/2026")
	if locked["unlocked"] != false || locked["year_member_unlocks"] != true {
		t.Fatalf("locked: %#v", locked)
	}
	if locked["content_status"] != "pending" {
		t.Fatalf("should not invent yearly copy: %#v", locked)
	}
	msg, _ := call(t, ts, token, http.MethodPost, "/api/app/v1/yearly-reports/2026/unlock", map[string]any{})
	if !strings.Contains(msg, "年卡未开通") {
		t.Fatalf("unlock without year card: %s", msg)
	}
	if _, err := store.Exec(t.Context(), `INSERT INTO subscriptions(user_id, sku_kind, status, starts_at, expires_at)
		VALUES ($1,'year','active',now(), now() + interval '365 days')`, uid); err != nil {
		t.Fatal(err)
	}
	open := listOne(t, ts, token, "/api/app/v1/yearly-reports/2026")
	if open["unlocked"] != true || open["unlock_source"] != "year_subscription" {
		t.Fatalf("year card should unlock: %#v", open)
	}
	unlocked := create(t, ts, token, "/api/app/v1/yearly-reports/2026/unlock", map[string]any{})
	if unlocked["unlocked"] != true {
		t.Fatalf("unlock: %#v", unlocked)
	}
}

func TestNumerologyDrawMapsDeck(t *testing.T) {
	store, ts := newMergeServer(t)
	token, _ := registerUser(t, ts, store, "numer")
	spreadID := timeFlowSpreadID(t, ts, token)
	reading := create(t, ts, token, "/api/app/v1/readings", map[string]any{"question": "灵数三张", "type": "normal"})
	rid := asID(reading["id"])
	must(t, ts, token, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/spread", rid), map[string]any{"spread_id": spreadID})
	msg, _ := call(t, ts, token, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/draw", rid), map[string]any{
		"mode": "numerology", "numbers": []int{1, 1, 3},
	})
	if !strings.Contains(msg, "不能重复") {
		t.Fatalf("dup numbers: %s", msg)
	}
	drawn := create(t, ts, token, fmt.Sprintf("/api/app/v1/readings/%d/draw", rid), map[string]any{
		"mode": "numerology", "numbers": []int{1, 9, 12},
	})
	raw, _ := json.Marshal(drawn["cards"])
	var cards []drawnCard
	if err := json.Unmarshal(raw, &cards); err != nil {
		t.Fatal(err, string(raw))
	}
	if len(cards) != 3 || cards[0].Name != "愚者" || cards[1].Name != "力量" || cards[2].Name != "正义" {
		t.Fatalf("map: %s", raw)
	}
	if cards[0].Orientation != "upright" {
		t.Fatalf("orientation: %#v", cards[0])
	}
}

func TestHistoryListPreviewFields(t *testing.T) {
	store, ts := newMergeServer(t)
	token, uid := registerUser(t, ts, store, "histprev")
	if _, err := store.Exec(t.Context(), `INSERT INTO readings(user_id, type, question, status, cards, interpretation)
		VALUES ($1,'normal','工作怎么选','COMPLETED','[{"name":"愚者"}]','先把选择的代价看清楚。')`, uid); err != nil {
		t.Fatal(err)
	}
	page := listOne(t, ts, token, "/api/app/v1/readings?limit=10")
	items, _ := page["items"].([]any)
	if len(items) == 0 {
		t.Fatalf("empty list: %#v", page)
	}
	item := items[0].(map[string]any)
	if item["status_label"] != "已解读" || item["card_name"] != "愚者" || !strings.Contains(fmt.Sprint(item["short_text"]), "先把选择") {
		t.Fatalf("preview: %#v", item)
	}
	if item["card_thumbnail"] != defaultCardThumbnailPath {
		t.Fatalf("default thumbnail: %#v", item["card_thumbnail"])
	}
}

func TestCalendarTypeFilter(t *testing.T) {
	store, ts := newMergeServer(t)
	token, uid := registerUser(t, ts, store, "caltype")
	day := time.Now().Format("2006-01-02")
	if _, err := store.Exec(t.Context(), `INSERT INTO readings(user_id, type, question, status, business_date)
		VALUES ($1,'normal','普通','COMPLETED',$2), ($1,'tarotist','占卜师','COMPLETED',$2)`, uid, day); err != nil {
		t.Fatal(err)
	}
	all := listOne(t, ts, token, "/api/app/v1/calendar")
	days, _ := all["days"].([]any)
	if len(days) < 2 {
		t.Fatalf("all days: %#v", all)
	}
	filtered := listOne(t, ts, token, "/api/app/v1/calendar?type=tarotist")
	only, _ := filtered["days"].([]any)
	if len(only) != 1 {
		t.Fatalf("tarotist filter: %#v", filtered)
	}
	row := only[0].(map[string]any)
	if row["type"] != "tarotist" {
		t.Fatalf("row: %#v", row)
	}
	msg, _ := call(t, ts, token, http.MethodGet, "/api/app/v1/calendar?type=nope", nil)
	if !strings.Contains(msg, "type") {
		t.Fatalf("bad type: %s", msg)
	}
}

func TestCalendarDayPreviewFields(t *testing.T) {
	store, ts := newMergeServer(t)
	token, uid := registerUser(t, ts, store, "calprev")
	tid := firstTarotist(t, ts, token)
	day := time.Now().Format("2006-01-02")
	at := time.Date(2026, 10, 19, 10, 24, 0, 0, time.UTC)
	if _, err := store.Exec(t.Context(), `INSERT INTO readings(user_id, type, question, status, business_date, cards, interpretation, created_at, updated_at)
		VALUES ($1,'daily','今日一牌','COMPLETED',$2,'[{"name":"女皇","orientation":"upright"}]','把温柔留给生活，也留一点给自己。',$3,$3)`, uid, day, at); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Exec(t.Context(), `INSERT INTO readings(user_id, type, question, status, business_date, cards, interpretation, created_at, updated_at)
		VALUES ($1,'normal','我该如何面对最近的工作变化？','COMPLETED',$2,'[{"name":"愚者","orientation":"upright"},{"name":"魔术师","orientation":"reversed"},{"name":"女祭司","orientation":"upright"}]','先把选择看清楚。',$3,$3)`, uid, day, at); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Exec(t.Context(), `INSERT INTO readings(user_id, type, question, status, business_date, tarotist_id, created_at, updated_at)
		VALUES ($1,'tarotist','怎样更好地表达自己的感受？','QUESTION_CREATED',$2,$3,$4,$4)`, uid, day, tid, at); err != nil {
		t.Fatal(err)
	}

	tarotists := list(t, ts, token, "/api/app/v1/tarotists")
	wantName := ""
	for _, row := range tarotists {
		if asID(row["id"]) == tid {
			wantName = fmt.Sprint(row["name"])
			break
		}
	}
	if wantName == "" {
		t.Fatalf("tarotist name: %#v", tarotists)
	}

	view := listOne(t, ts, token, "/api/app/v1/calendar")
	days, _ := view["days"].([]any)
	byType := map[string]map[string]any{}
	for _, raw := range days {
		row := raw.(map[string]any)
		byType[fmt.Sprint(row["type"])] = row
	}

	daily := byType["daily"]
	if daily["card_name"] != "女皇" || daily["orientation"] != "upright" || daily["orientation_label"] != "正位" || daily["type_label"] != "今日一牌" {
		t.Fatalf("daily: %#v", daily)
	}
	if daily["card_thumbnail"] != defaultCardThumbnailPath {
		t.Fatalf("daily thumb: %#v", daily["card_thumbnail"])
	}
	if !strings.Contains(fmt.Sprint(daily["short_text"]), "把温柔") {
		t.Fatalf("daily text: %#v", daily)
	}

	normal := byType["normal"]
	thumbs, _ := normal["card_thumbnails"].([]any)
	if len(thumbs) != 3 || normal["type_label"] != "普通占卜" || normal["status_label"] != "已解读" {
		t.Fatalf("normal: %#v", normal)
	}

	tar := byType["tarotist"]
	if tar["type_label"] != "占卜师" || tar["status_label"] != "待继续" || tar["tarotist_name"] != wantName {
		t.Fatalf("tarotist: %#v", tar)
	}
	if tar["created_at"] == nil || tar["card_thumbnails"] == nil {
		t.Fatalf("time/thumbs: %#v", tar)
	}
}

func TestDeckPreferenceAndIAPPacks(t *testing.T) {
	store, ts := newMergeServer(t)
	token, _ := registerUser(t, ts, store, "deckiap")
	pref := listOne(t, ts, token, "/api/app/v1/deck-preference")
	if pref["code"] != "rider-waite" {
		t.Fatalf("default deck: %#v", pref)
	}
	must(t, ts, token, http.MethodPut, "/api/app/v1/deck-preference", map[string]any{"code": "rider-waite"})
	msg, _ := call(t, ts, token, http.MethodPut, "/api/app/v1/deck-preference", map[string]any{"code": "marseille"})
	if !strings.Contains(msg, "尚未开放") {
		t.Fatalf("unknown deck: %s", msg)
	}

	skus := list(t, ts, token, "/api/app/v1/iap/skus")
	grants := map[int]bool{}
	var yearSKU map[string]any
	for _, sku := range skus {
		code, _ := sku["code"].(string)
		if code == "sub_month" && sku["price_amount_minor"].(float64) != 3900 {
			t.Fatalf("month price: %#v", sku)
		}
		if sku["kind"] == "year_subscription" {
			yearSKU = sku
		}
		if g, ok := sku["diamond_grant"].(float64); ok {
			grants[int(g)] = true
		}
	}
	for _, want := range []int{60, 180, 300, 680, 1280, 3280} {
		if !grants[want] {
			t.Fatalf("missing pack %d: %#v", want, grants)
		}
	}
	if yearSKU == nil {
		t.Fatal("year sku missing")
	}
	benefits, _ := yearSKU["benefits"].([]any)
	foundYearly := false
	for _, raw := range benefits {
		b := raw.(map[string]any)
		if b["key"] == "yearly_report" && b["included"] == true {
			foundYearly = true
		}
		if b["key"] == "followup" && b["count"].(float64) != 5 {
			t.Fatalf("followup count: %#v", b)
		}
	}
	if !foundYearly {
		t.Fatalf("year benefits: %#v", benefits)
	}
}

func TestPublicIDStableAndEmailFallback(t *testing.T) {
	store, ts := newMergeServer(t)
	token, _ := registerUser(t, ts, store, "pubid")
	first := listOne(t, ts, token, "/api/app/v1/profile")
	id1, _ := first["public_id"].(string)
	if len(id1) != 6 {
		t.Fatalf("public_id: %#v", first["public_id"])
	}
	display, _ := first["account_display"].(string)
	if !strings.Contains(display, "***@") {
		t.Fatalf("email fallback: %#v", first["account_display"])
	}
	second := listOne(t, ts, token, "/api/app/v1/profile")
	if second["public_id"] != id1 {
		t.Fatalf("public_id changed %s -> %s", id1, second["public_id"])
	}
}

func seedIAPCode(code string) bool {
	switch code {
	case "sub_month", "sub_year", "sub_times", "pack_consumable", "pack_60", "pack_180", "pack_300", "pack_680", "pack_1280", "pack_3280", "unlock_yearly":
		return true
	}
	return false
}

func timeFlowSpreadID(t *testing.T, ts *httptest.Server, token string) float64 {
	t.Helper()
	spreads := list(t, ts, token, "/api/app/v1/spreads")
	for _, s := range spreads {
		if s["name"] == "时间流" {
			return s["id"].(float64)
		}
	}
	t.Fatal("time flow spread missing")
	return 0
}
