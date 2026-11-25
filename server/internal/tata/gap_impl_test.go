package tata

import (
	"context"
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
		VALUES ($1,'normal','关系走向怎么样','COMPLETED','这段关系需要把话说清楚再往前走。',$2,$2),
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
	_ = ensureTimeFlow(t, store)
	spreadID := timeFlowSpreadID(t, ts, token)
	reading := create(t, ts, token, "/api/app/v1/readings", map[string]any{"question": "灵数三张", "type": "normal"})
	rid := asID(reading["id"])
	must(t, ts, token, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/spread", rid), map[string]any{"spread_id": spreadID, "draw_mode": "numerology"})
	var mode string
	if err := store.QueryRow(t.Context(), `SELECT draw_mode FROM readings WHERE id=$1`, rid).Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if mode != "numerology" {
		t.Fatalf("draw mode %s", mode)
	}
	_, dupRaw := call(t, ts, token, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/numerology", rid), map[string]any{"numbers": []int{1, 1, 3}})
	if !strings.Contains(string(dupRaw), "已被使用") {
		t.Fatalf("dup numbers: %s", dupRaw)
	}
	_, shortRaw := call(t, ts, token, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/numerology", rid), map[string]any{"numbers": []int{1, 9}})
	if !strings.Contains(string(shortRaw), "请再输入") {
		t.Fatalf("short draw: %s", shortRaw)
	}
	must(t, ts, token, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/numerology", rid), map[string]any{"numbers": []int{1, 9, 12}})
	drawn := listOne(t, ts, token, fmt.Sprintf("/api/app/v1/readings/%d", rid))
	raw, _ := json.Marshal(drawn["cards"])
	var cards []drawnCard
	if err := json.Unmarshal(raw, &cards); err != nil {
		t.Fatal(err, string(raw))
	}
	if len(cards) != 3 || cards[0].Number != 1 || cards[1].Number != 9 || cards[2].Number != 12 {
		t.Fatalf("count follows spread: %s", raw)
	}
	if cards[0].Orientation != "upright" && cards[0].Orientation != "reversed" {
		t.Fatalf("orientation: %#v", cards[0])
	}
	var mapRaw string
	if err := store.QueryRow(t.Context(), `SELECT numerology_map FROM readings WHERE id=$1`, rid).Scan(&mapRaw); err != nil {
		t.Fatal(err)
	}
	var slots []numerologySlot
	if err := json.Unmarshal([]byte(mapRaw), &slots); err != nil || len(slots) != 78 {
		t.Fatalf("map: %d %v", len(slots), err)
	}
	slotByNumber := map[int]numerologySlot{}
	seenCard := map[int]bool{}
	upright, reversed := 0, 0
	for _, slot := range slots {
		if seenCard[slot.CardID] || slot.Orientation != "upright" && slot.Orientation != "reversed" {
			t.Fatalf("slot: %#v", slot)
		}
		if slot.Orientation == "upright" {
			upright++
		} else {
			reversed++
		}
		seenCard[slot.CardID] = true
		slotByNumber[slot.Number] = slot
	}
	if upright == 0 || reversed == 0 {
		t.Fatal("map is all one orientation")
	}
	if cards[0].CardID != slotByNumber[1].CardID || cards[0].Orientation != slotByNumber[1].Orientation || cards[1].Name != slotByNumber[9].Name {
		t.Fatalf("draw left the map: %s", raw)
	}
	kept := cards[0]
	again := create(t, ts, token, fmt.Sprintf("/api/app/v1/readings/%d/redraw", rid), map[string]any{"number": 20})
	raw, _ = json.Marshal(again["cards"])
	var redrawn []drawnCard
	if err := json.Unmarshal(raw, &redrawn); err != nil {
		t.Fatal(err)
	}
	if len(redrawn) != 4 || redrawn[0].Number != kept.Number || redrawn[0].CardID != kept.CardID || redrawn[0].Orientation != kept.Orientation {
		t.Fatalf("redraw overwrote the hand: %s", raw)
	}
	extra := redrawn[3]
	if extra.Number != 20 || extra.CardID != slotByNumber[20].CardID || extra.Orientation != slotByNumber[20].Orientation || extra.Position != 4 {
		t.Fatalf("appended card: %#v", extra)
	}
	var mapAfter string
	if err := store.QueryRow(t.Context(), `SELECT numerology_map FROM readings WHERE id=$1`, rid).Scan(&mapAfter); err != nil {
		t.Fatal(err)
	}
	if mapAfter != mapRaw {
		t.Fatal("redraw rewrote the map")
	}
	msg, _ := call(t, ts, token, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/redraw", rid), map[string]any{
		"number": 9,
	})
	if !strings.Contains(msg, "不能重复") {
		t.Fatalf("redraw dup: %s", msg)
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
	if fmt.Sprint(item["card_thumbnail"]) == "" || item["card_thumbnail"] == nil {
		t.Fatalf("thumbnail missing: %#v", item["card_thumbnail"])
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
	if fmt.Sprint(daily["card_thumbnail"]) == "" || daily["card_thumbnail"] == nil {
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
		if g, ok := sku["quota_grant"].(float64); ok {
			grants[int(g)] = true
		}
	}
	codes := map[string]bool{}
	for _, sku := range skus {
		code, _ := sku["code"].(string)
		codes[code] = true
	}
	for _, want := range []string{"sub_month", "sub_year"} {
		if !codes[want] {
			t.Fatalf("missing sku %s: %#v", want, codes)
		}
	}
	if codes["pack_quota_20"] {
		t.Fatalf("free user saw quota pack: %#v", codes)
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

func TestSpreadRecommended(t *testing.T) {
	store, ts := newMergeServer(t)
	admin := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "admin", "password": "admin123"})
	token, _ := registerUser(t, ts, store, "spredrec")
	_ = ensureTimeFlow(t, store)
	t.Cleanup(func() {
		_, _ = store.Exec(context.Background(), `DELETE FROM spreads WHERE name<>'时间流'`)
		_, _ = store.Exec(context.Background(), `UPDATE spreads SET recommended=true, enabled=true, thumbnail='' WHERE name='时间流'`)
	})

	groups := listOne(t, ts, token, "/api/app/v1/spreads")["groups"].([]any)
	if len(groups) != 3 || groups[0].(map[string]any)["title"] != "一、快速指引（1-3张）" || groups[1].(map[string]any)["title"] != "二、进阶分析（4-7张）" || groups[2].(map[string]any)["title"] != "三、深度全景（10张）" {
		t.Fatalf("spread groups: %#v", groups)
	}
	reading := create(t, ts, token, "/api/app/v1/readings", map[string]any{"question": "推荐哪几套", "type": "normal"})
	firstRec := create(t, ts, token, fmt.Sprintf("/api/app/v1/readings/%d/recommend", asID(reading["id"])), map[string]any{})
	if !recommendHas(t, firstRec, "时间流") {
		t.Fatalf("seed recommend: %#v", firstRec["recommended_spreads"])
	}

	created := create(t, ts, admin.access, "/api/admin/v1/spreads", map[string]any{
		"name": "多维解读", "description": "多个角度", "card_count": 1,
		"positions":   []map[string]any{{"index": 1, "name": "核心", "meaning": ""}},
		"depth_group": "quick",
		"recommended": true,
	})
	newID := asID(created["id"])
	again := create(t, ts, token, fmt.Sprintf("/api/app/v1/readings/%d/recommend", asID(reading["id"])), map[string]any{})
	if !recommendHas(t, again, "时间流") || !recommendHas(t, again, "多维解读") {
		t.Fatalf("both recommended: %#v", again["recommended_spreads"])
	}
	appList := appSpreadItems(t, ts, token)
	var createdRow map[string]any
	for _, s := range appList {
		if s["name"] == "多维解读" {
			createdRow = s
		}
	}
	if createdRow == nil || createdRow["recommended"] != true || createdRow["depth_group"] != "quick" {
		t.Fatalf("new recommended in quick: %#v", createdRow)
	}
	if createdRow["thumbnail"] != absAsset(ts, defaultCardThumbnailPath) {
		t.Fatalf("new default thumb: %#v", createdRow["thumbnail"])
	}

	must(t, ts, admin.access, http.MethodPut, fmt.Sprintf("/api/admin/v1/spreads/%d", newID), map[string]any{
		"name": "多维解读", "description": "多个角度", "card_count": 1,
		"positions": []map[string]any{{"index": 1, "name": "核心", "meaning": ""}},
		"thumbnail": "https://cdn.example.test/spread.png",
	})
	appList = appSpreadItems(t, ts, token)
	for _, s := range appList {
		if s["name"] == "多维解读" {
			createdRow = s
		}
	}
	if createdRow["thumbnail"] != "https://cdn.example.test/spread.png" {
		t.Fatalf("app thumb: %#v", createdRow)
	}
	adminRow := map[string]any{}
	for _, s := range list(t, ts, admin.access, "/api/admin/v1/spreads") {
		if asID(s["id"]) == newID {
			adminRow = s
			break
		}
	}
	if adminRow["thumbnail"] != "https://cdn.example.test/spread.png" || adminRow["preview"] != "https://cdn.example.test/spread.png" {
		t.Fatalf("admin thumb: %#v", adminRow)
	}

	must(t, ts, admin.access, http.MethodPut, fmt.Sprintf("/api/admin/v1/spreads/%d", newID), map[string]any{
		"name": "多维解读", "description": "多个角度", "card_count": 1,
		"positions":   []map[string]any{{"index": 1, "name": "核心", "meaning": ""}},
		"enabled":     false,
		"recommended": true,
	})
	appList = appSpreadItems(t, ts, token)
	for _, s := range appList {
		if s["name"] == "多维解读" {
			t.Fatalf("disabled recommended still listed: %#v", s)
		}
	}
}

func recommendHas(t *testing.T, payload map[string]any, name string) bool {
	t.Helper()
	items, _ := payload["recommended_spreads"].([]any)
	for _, raw := range items {
		if raw.(map[string]any)["name"] == name {
			return true
		}
	}
	return false
}

func TestAdminExploreThemeFields(t *testing.T) {
	store, ts := newMergeServer(t)
	admin := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "admin", "password": "admin123"})
	name := fmt.Sprintf("探索主题表单%d", time.Now().UnixNano())
	key := fmt.Sprintf("theme_%d", time.Now().UnixNano())
	created := create(t, ts, admin.access, "/api/admin/v1/spreads", map[string]any{
		"name": name, "description": "主题下的牌阵", "card_count": 1,
		"positions":      []map[string]any{{"index": 1, "name": "结果", "meaning": ""}},
		"theme_key":      key,
		"theme_title":    "要不要",
		"theme_subtitle": "先看一句",
		"short_sentence": "只问这一件",
	})
	id := asID(created["id"])
	t.Cleanup(func() {
		_, _ = store.Exec(context.Background(), `DELETE FROM spreads WHERE id=$1`, id)
	})
	must(t, ts, admin.access, http.MethodPut, fmt.Sprintf("/api/admin/v1/spreads/%d", id), map[string]any{
		"name": name, "description": "主题下的牌阵", "card_count": 1,
		"positions":      []map[string]any{{"index": 1, "name": "结果", "meaning": ""}},
		"theme_key":      key,
		"theme_title":    "要不要",
		"theme_subtitle": "改过的副标题",
		"short_sentence": "只问这一件",
	})
	var row map[string]any
	for _, item := range list(t, ts, admin.access, "/api/admin/v1/spreads") {
		if asID(item["id"]) == id {
			row = item
		}
	}
	if row == nil || row["theme_key"] != key || row["theme_title"] != "要不要" || row["theme_subtitle"] != "改过的副标题" || row["short_sentence"] != "只问这一件" {
		t.Fatalf("theme fields: %#v", row)
	}
	if row["quota_priced"] != false {
		t.Fatalf("explore price invented: %#v", row)
	}
}

func seedIAPCode(code string) bool {
	switch code {
	case "sub_month", "sub_year", "sub_times", "pack_quota_20", "pack_quota_60", "pack_quota_100", "unlock_yearly":
		return true
	}
	return false
}

func TestReadingMessagesReplayConversation(t *testing.T) {
	store, ts := newMergeServer(t)
	token, uid := registerUser(t, ts, store, "conv")
	var id int64
	cards := `[{"position":1,"position_name":"现在","card_id":0,"name":"愚者","orientation":"upright"}]`
	if err := store.QueryRow(t.Context(), `INSERT INTO readings(user_id, type, question, status, cards, interpretation)
		VALUES ($1,'normal','工作会怎么变','COMPLETED',$2,'先看清新职责，再小步试。') RETURNING id`, uid, cards).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Exec(t.Context(), `INSERT INTO reading_messages(reading_id, turn_id, seq, role, content) VALUES
		($1,1,0,'user','我下一步该怎么做'),
		($1,1,1,'assistant','先问清成功标准'),
		($1,1,2,'assistant','再做一次小范围尝试')`, id); err != nil {
		t.Fatal(err)
	}
	body := listOne(t, ts, token, fmt.Sprintf("/api/app/v1/readings/%d", id))
	if body["reading_id"].(float64) != float64(id) {
		t.Fatalf("reading id: %#v", body["reading_id"])
	}
	messages, _ := body["messages"].([]any)
	want := []struct{ kind, from, style string }{
		{"question", "user", "labeled"},
		{"cards", "server", "spread"},
		{"interpretation", "server", "body"},
		{"question", "user", "bubble"},
		{"interpretation", "server", "body"},
		{"interpretation", "server", "body"},
	}
	if len(messages) != len(want) {
		t.Fatalf("messages: %#v", messages)
	}
	for i, item := range want {
		got := messages[i].(map[string]any)
		if got["kind"] != item.kind || got["from"] != item.from || got["style"] != item.style {
			t.Fatalf("message %d: %#v", i, got)
		}
	}
	opening := messages[0].(map[string]any)
	if opening["label"] != "你的问题" || opening["text"] != "工作会怎么变" {
		t.Fatalf("question: %#v", opening)
	}
	cardsItem := messages[1].(map[string]any)
	if cardsItem["title"] != "牌面预览" {
		t.Fatalf("spread title: %#v", cardsItem["title"])
	}
	drawn, _ := cardsItem["cards"].([]any)
	if len(drawn) != 1 || drawn[0].(map[string]any)["name"] != "愚者" {
		t.Fatalf("cards: %#v", cardsItem)
	}
	follow := messages[3].(map[string]any)
	if follow["text"] != "我下一步该怎么做" || follow["turn_id"].(float64) != 1 {
		t.Fatalf("follow-up: %#v", follow)
	}
	bubble := messages[5].(map[string]any)
	if bubble["seq"].(float64) != 2 || bubble["text"] != "再做一次小范围尝试" {
		t.Fatalf("bubble: %#v", bubble)
	}
}

func TestConversationShapesMatchOriginals(t *testing.T) {
	store, ts := newMergeServer(t)
	token, uid := registerUser(t, ts, store, "shapes")
	var id int64
	if err := store.QueryRow(t.Context(), `INSERT INTO readings(user_id, type, question, status)
		VALUES ($1,'normal','','QUESTION_CREATED') RETURNING id`, uid).Scan(&id); err != nil {
		t.Fatal(err)
	}
	rows := []struct{ style, content string }{
		{"chip", `{"text":"这份工作还值得留下吗"}`},
		{"option", `{"text":"这份工作还值得留下吗","hint":"看关系，不看去留","selected":false}`},
		{"followup", `{"text":"那我下周要不要先谈"}`},
		{"recommend", `{"options":[{"text":"这份工作还值得留下吗","hint":"看关系，不看去留","selected":true}]}`},
		{"detect", `{"text":"问题里同时问了去留和关系。"}`},
		{"guide", `{"rules":["每个数字对应一张牌","同一个数字不能再用"]}`},
		{"error", `{"text":"数字 12 已被使用，请更换一个数字。","numbers":["12","58"]}`},
		{"pick", `{"slots":[{"position":1,"position_name":"过去","number":"12","invalid":true},{"position":2,"position_name":"现在","number":"58","invalid":false}]}`},
		{"choose", `{"spreads":[{"id":7,"name":"时间流","card_count":3,"recommended":true,"selected":false},{"id":8,"name":"一张牌","card_count":1,"recommended":false,"selected":false}]}`},
		{"shell", `{"text":"空气泡"}`},
	}
	for i, row := range rows {
		if _, err := store.Exec(t.Context(), `INSERT INTO reading_messages(reading_id, turn_id, seq, role, kind, style, content)
			VALUES ($1,$2,0,'assistant','',$3,$4)`, id, i+1, row.style, row.content); err != nil {
			t.Fatal(err)
		}
	}
	body := listOne(t, ts, token, fmt.Sprintf("/api/app/v1/readings/%d", id))
	messages, _ := body["messages"].([]any)
	got := map[string]map[string]any{}
	for _, raw := range messages {
		item := raw.(map[string]any)
		got[item["style"].(string)] = item
	}
	if _, ok := got["shell"]; ok {
		t.Fatalf("client shell entered the conversation: %#v", messages)
	}
	chip := got["chip"]
	if chip["from"] != "server" || chip["text"] != "这份工作还值得留下吗" || chip["title"] != nil || chip["hint"] != nil || chip["selected"] != nil {
		t.Fatalf("chip: %#v", chip)
	}
	option := got["option"]
	if option["text"] != "这份工作还值得留下吗" || option["hint"] != "看关系，不看去留" || option["selected"] != false {
		t.Fatalf("option: %#v", option)
	}
	follow := got["followup"]
	followOptions, _ := follow["options"].([]any)
	if follow["title"] != "继续解读" || follow["text"] != "那我下周要不要先谈" || len(followOptions) != 1 {
		t.Fatalf("followup: %#v", follow)
	}
	recommend := got["recommend"]
	if recommend["title"] != "为你推荐 1 个更清晰的问题" {
		t.Fatalf("recommend title: %#v", recommend["title"])
	}
	options, _ := recommend["options"].([]any)
	if len(options) != 1 || options[0].(map[string]any)["selected"] != true {
		t.Fatalf("recommend options: %#v", recommend["options"])
	}
	detect := got["detect"]
	if detect["title"] != "塔塔 · 问题检测" || detect["status"] != "已完成" || detect["reason_title"] != "为什么建议修改？" {
		t.Fatalf("detect: %#v", detect)
	}
	guide := got["guide"]
	if guide["title"] != "输入专属数字" || guide["placeholder"] != "例如：12、58、62" || len(guide["rules"].([]any)) != 2 {
		t.Fatalf("guide: %#v", guide)
	}
	errItem := got["error"]
	if errItem["text"] != "数字 12 已被使用，请更换一个数字。" || len(errItem["numbers"].([]any)) != 2 {
		t.Fatalf("error: %#v", errItem)
	}
	pick := got["pick"]
	slots, _ := pick["slots"].([]any)
	if pick["from"] != "server" || len(slots) != 2 || slots[0].(map[string]any)["invalid"] != true {
		t.Fatalf("pick: %#v", pick)
	}
	choose := got["choose"]
	spreads, _ := choose["spreads"].([]any)
	if choose["title"] != "先确认牌阵和张数，再选择抽牌方式。" || len(spreads) != 2 || spreads[0].(map[string]any)["recommended"] != true {
		t.Fatalf("choose: %#v", choose)
	}
	modes, _ := choose["draw_modes"].([]any)
	if choose["more_spreads"] != "更多牌阵" || choose["draw_mode_label"] != "抽牌方式" || len(modes) != 2 {
		t.Fatalf("choose draw modes: %#v", choose)
	}
}

func TestFollowupMustOfferClarifier(t *testing.T) {
	store, ts := newMergeServer(t)
	token, uid := registerUser(t, ts, store, "clarify")
	if _, err := store.Exec(t.Context(), `UPDATE app_users SET reading_trial_used=true WHERE id=$1`, uid); err != nil {
		t.Fatal(err)
	}
	setMember(t, store, uid, "month")
	if _, err := store.Exec(t.Context(), `DELETE FROM bindings WHERE task='tarotist'`); err != nil {
		t.Fatal(err)
	}
	reading := create(t, ts, token, "/api/app/v1/readings", map[string]any{"question": "工作会怎么变", "type": "normal"})
	id := asID(reading["id"])
	completeReading(t, store, id)
	if _, err := store.Exec(t.Context(), `UPDATE readings SET spread_id=(SELECT id FROM spreads WHERE enabled AND card_count=1 ORDER BY id LIMIT 1) WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	msg, raw := call(t, ts, token, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/followups", id), map[string]any{"text": "再看清楚一点", "continuation": true})
	if msg != "" {
		t.Fatalf("followup: %s %s", msg, raw)
	}
	body := decodeData(t, raw)
	offer, _ := body["redraw"].(map[string]any)
	actions, _ := offer["actions"].([]any)
	if offer["style"] != "redraw" || offer["title"] != "补抽牌面" || offer["pending"] != true || len(actions) != 2 {
		t.Fatalf("redraw offer: %#v", body["redraw"])
	}
	if actions[0].(map[string]any)["label"] != "去抽牌" || actions[1].(map[string]any)["label"] != "跳过" {
		t.Fatalf("redraw actions: %#v", actions)
	}
	paused := listOne(t, ts, token, fmt.Sprintf("/api/app/v1/readings/%d", id))
	pausedMessages, _ := paused["messages"].([]any)
	foundPending := false
	for _, rawItem := range pausedMessages {
		item := rawItem.(map[string]any)
		if item["style"] == "redraw" && item["pending"] == true && item["title"] == "补抽牌面" {
			foundPending = true
		}
	}
	if !foundPending {
		t.Fatalf("history lost the paused redraw: %#v", pausedMessages)
	}
	opened := listOne(t, ts, token, fmt.Sprintf("/api/app/v1/readings/%d", id))
	if opened["redraw_pending"] != true {
		t.Fatalf("reading reopen: %#v", opened["redraw_pending"])
	}
	reopen, _ := opened["redraw"].(map[string]any)
	if reopen["pending"] != true || reopen["style"] != "redraw" {
		t.Fatalf("reading redraw: %#v", opened["redraw"])
	}
	page := listOne(t, ts, token, "/api/app/v1/readings")
	listed, _ := page["items"].([]any)
	flagged := false
	for _, rawItem := range listed {
		item := rawItem.(map[string]any)
		if item["id"].(float64) == float64(id) && item["redraw_pending"] == true {
			flagged = true
		}
	}
	if !flagged {
		t.Fatalf("history list lost the paused redraw: %#v", listed)
	}
	msg, raw = call(t, ts, token, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/followups/redraw", id), map[string]any{"action": "draw"})
	if msg != "" {
		t.Fatalf("draw: %s %s", msg, raw)
	}
	drawn := decodeData(t, raw)
	card, _ := drawn["card"].(map[string]any)
	cards, _ := card["cards"].([]any)
	if card["style"] != "single" || card["title"] != "牌面" || len(cards) != 1 {
		t.Fatalf("clarifier: %#v", drawn["card"])
	}
	if cards[0].(map[string]any)["name"] == "" || cards[0].(map[string]any)["name"] == "愚者" {
		t.Fatalf("clarifier card: %#v", cards[0])
	}
	history := listOne(t, ts, token, fmt.Sprintf("/api/app/v1/readings/%d", id))
	messages, _ := history["messages"].([]any)
	spreadN, offers, singles := 0, 0, 0
	for _, rawItem := range messages {
		item := rawItem.(map[string]any)
		switch item["style"] {
		case "spread":
			spreadN = len(item["cards"].([]any))
		case "redraw":
			offers++
			if item["choice"] != "draw" {
				t.Fatalf("choice: %#v", item)
			}
		case "single":
			singles++
		}
	}
	if spreadN != 1 || offers != 1 || singles != 1 {
		t.Fatalf("history spread=%d offers=%d singles=%d messages=%#v", spreadN, offers, singles, messages)
	}
	done := listOne(t, ts, token, fmt.Sprintf("/api/app/v1/readings/%d", id))
	if done["redraw_pending"] != false {
		t.Fatalf("redraw still pending after draw: %#v", done["redraw_pending"])
	}
}

func timeFlowSpreadID(t *testing.T, ts *httptest.Server, token string) float64 {
	t.Helper()
	id, _ := timeFlowSpread(t, ts, token)
	return id
}

func timeFlowSpread(t *testing.T, ts *httptest.Server, token string) (float64, []any) {
	t.Helper()
	spreads := list(t, ts, login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "admin", "password": "admin123"}).access, "/api/admin/v1/spreads")
	for _, s := range spreads {
		if s["name"] == "时间流" {
			return s["id"].(float64), s["positions"].([]any)
		}
	}
	t.Fatal("time flow spread missing")
	return 0, nil
}

func messageList(t *testing.T, body map[string]any) []map[string]any {
	t.Helper()
	raw, _ := body["messages"].([]any)
	if len(raw) == 0 {
		t.Fatalf("messages missing: %#v", body)
	}
	out := make([]map[string]any, len(raw))
	for i, item := range raw {
		out[i] = item.(map[string]any)
	}
	return out
}

func TestConventionalFlowCoversEveryType(t *testing.T) {
	store, ts := newMergeServer(t)
	admin := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "admin", "password": "admin123"})
	vendor := create(t, ts, admin.access, "/api/admin/v1/vendors", map[string]any{
		"name": "常规流程", "base_url": ts.URL + "/demo/openai/v1", "api_key": "demo", "models": []string{"demo"}, "enabled": true,
	})
	must(t, ts, admin.access, http.MethodPut, "/api/admin/v1/bindings/interpret", map[string]any{"vendor_id": vendor["id"], "model": "demo"})
	must(t, ts, admin.access, http.MethodPut, "/api/admin/v1/bindings/tarotist", map[string]any{"vendor_id": vendor["id"], "model": "demo"})
	token, uid := registerUser(t, ts, store, "flow")
	if _, err := store.Exec(t.Context(), `UPDATE app_users SET reading_trial_used=true WHERE id=$1`, uid); err != nil {
		t.Fatal(err)
	}
	setMember(t, store, uid, "month")
	resetDiamondCost(t, store)
	setDiamondCost(t, store, 1)
	setQuota(t, store, uid, 20)

	presets := listOne(t, ts, "", "/api/app/v1/preset-questions")
	featured, _ := presets["featured"].([]any)
	if len(featured) == 0 {
		t.Fatal("preset questions missing")
	}
	presetText := featured[0].(map[string]any)["body"].(string)
	ownText := "这是我自己输入、不采用推荐的问题"
	suggestions := create(t, ts, token, "/api/app/v1/question-suggestions", map[string]any{"question": ownText})
	seen := map[string]bool{}
	errorsSeen := map[string]bool{}
	note := func(items []map[string]any) {
		for _, item := range items {
			if item["from"] == "client" {
				t.Fatalf("client chrome entered the response: %#v", item)
			}
			style, _ := item["style"].(string)
			switch style {
			case "shell", "more", "method", "dash", "empty":
				t.Fatalf("client-only type returned: %#v", item)
			}
			seen[style] = true
			if style == "error" {
				errorsSeen[item["text"].(string)] = true
			}
		}
	}
	suggested := messageList(t, suggestions)
	note(suggested)
	for _, item := range suggested {
		if item["style"] == "chip" && item["text"] == ownText {
			t.Fatal("recommendation replaced the user's own question")
		}
		if item["style"] == "detect" && item["text"] != "你的问题涉及的范围比较广，明确你最关注的方向，可以帮助解读更聚焦，也更容易获得具体的建议。" {
			t.Fatalf("detect: %#v", item)
		}
		if item["style"] == "followup" {
			opts, _ := item["options"].([]any)
			if item["title"] != "继续解读" || len(opts) == 0 {
				t.Fatalf("followup: %#v", item)
			}
		}
	}

	spreadID, positions := timeFlowSpread(t, ts, token)
	hand := make([]map[string]any, 3)
	names := []string{"愚者", "魔术师", "女祭司"}
	orients := []string{"upright", "reversed", "upright"}
	for i := range hand {
		pos := positions[i].(map[string]any)
		hand[i] = map[string]any{
			"position": i + 1, "position_name": pos["name"],
			"name": names[i], "orientation": orients[i],
		}
	}
	submit := func(question string, source string) (int64, []map[string]any) {
		t.Helper()
		reading := create(t, ts, token, "/api/app/v1/readings", map[string]any{"question": question, "type": "normal", "source": source})
		id := asID(reading["id"])
		choices := listOne(t, ts, token, fmt.Sprintf("/api/app/v1/readings/%d/spread-choices", id))
		choiceMsgs := messageList(t, choices)
		note(choiceMsgs)
		if len(choiceMsgs) != 1 || choiceMsgs[0]["style"] != "choose" || choiceMsgs[0]["title"] != "先确认牌阵和张数，再选择抽牌方式。" {
			t.Fatalf("spread choices: %#v", choiceMsgs)
		}
		if _, ok := choiceMsgs[0]["draw_modes"].([]any); !ok {
			t.Fatalf("spread choices missing draw_modes: %#v", choiceMsgs[0])
		}
		must(t, ts, token, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/spread", id), map[string]any{"spread_id": spreadID, "draw_mode": "normal"})
		view := create(t, ts, token, fmt.Sprintf("/api/app/v1/readings/%d/interpret", id), map[string]any{"spread_id": spreadID, "cards": hand})
		if view["reading_id"].(float64) != float64(id) {
			t.Fatalf("interpret reading: %#v", view["reading_id"])
		}
		items := messageList(t, view)
		note(items)
		if items[0]["style"] != "labeled" || items[0]["text"] != question || items[0]["label"] != "你的问题" {
			t.Fatalf("question: %#v", items[0])
		}
		if items[1]["style"] != "spread" || items[1]["title"] != "牌面预览" {
			t.Fatalf("spread: %#v", items[1])
		}
		cards, _ := items[1]["cards"].([]any)
		if len(cards) != 3 {
			t.Fatalf("cards: %#v", items[1])
		}
		for i, raw := range cards {
			card := raw.(map[string]any)
			if card["name"] != names[i] || card["orientation"] != orients[i] {
				t.Fatalf("submitted card %d changed: %#v", i, card)
			}
		}
		if items[2]["style"] != "body" || !strings.Contains(items[2]["text"].(string), "演示解读") {
			t.Fatalf("interpretation: %#v", items[2])
		}
		return id, items
	}

	presetID, _ := submit(presetText, "preset")
	_ = presetID
	ownID, _ := submit(ownText, "")
	followed := create(t, ts, token, fmt.Sprintf("/api/app/v1/readings/%d/followups", ownID), map[string]any{"text": "再看清楚一点", "continuation": true})
	if followed["reading_id"].(float64) != float64(ownID) {
		t.Fatalf("followup envelope: %#v", followed["reading_id"])
	}
	note(messageList(t, followed))
	paused := false
	for _, item := range messageList(t, followed) {
		if item["style"] == "bubble" && item["text"] != "再看清楚一点" {
			t.Fatalf("bubble: %#v", item)
		}
		if item["style"] == "redraw" && item["pending"] == true && item["title"] == "补抽牌面" {
			paused = true
			actions, _ := item["actions"].([]any)
			if len(actions) != 2 || actions[0].(map[string]any)["label"] != "去抽牌" || actions[1].(map[string]any)["label"] != "跳过" {
				t.Fatalf("actions: %#v", actions)
			}
		}
	}
	if !paused {
		t.Fatalf("followup lost redraw: %#v", followed["messages"])
	}
	page := listOne(t, ts, token, fmt.Sprintf("/api/app/v1/readings/%d?limit=1&offset=0", ownID))
	firstPage := messageList(t, page)
	if page["has_more"] != true || len(firstPage) != 1 {
		t.Fatalf("page: %#v", page)
	}
	rest := listOne(t, ts, token, fmt.Sprintf("/api/app/v1/readings/%d?limit=1&offset=1", ownID))
	second := messageList(t, rest)
	if rest["has_more"] != true || second[0]["style"] == firstPage[0]["style"] && second[0]["text"] == firstPage[0]["text"] {
		t.Fatalf("offset repeated the first message: %#v", second[0])
	}

	skipped := create(t, ts, token, fmt.Sprintf("/api/app/v1/readings/%d/followups/redraw", ownID), map[string]any{"action": "skip"})
	note(messageList(t, skipped))
	for _, item := range messageList(t, skipped) {
		if item["style"] == "single" {
			t.Fatal("skip added a card")
		}
		if item["style"] == "redraw" && item["choice"] != "skip" {
			t.Fatalf("skip choice: %#v", item)
		}
	}

	drawn := create(t, ts, token, fmt.Sprintf("/api/app/v1/readings/%d/followups", presetID), map[string]any{"text": "帮我补一张", "continuation": true})
	note(messageList(t, drawn))
	drew := create(t, ts, token, fmt.Sprintf("/api/app/v1/readings/%d/followups/redraw", presetID), map[string]any{"action": "draw"})
	note(messageList(t, drew))
	singles := 0
	for _, item := range messageList(t, drew) {
		if item["style"] == "spread" && len(item["cards"].([]any)) != 3 {
			t.Fatalf("clarifier merged into the spread: %#v", item)
		}
		if item["style"] == "single" {
			singles++
			if item["title"] != "牌面" || len(item["cards"].([]any)) != 1 {
				t.Fatalf("single: %#v", item)
			}
		}
		if item["style"] == "redraw" && item["choice"] != "draw" {
			t.Fatalf("draw choice: %#v", item)
		}
	}
	if singles != 1 {
		t.Fatalf("single count %d", singles)
	}

	fresh := create(t, ts, token, fmt.Sprintf("/api/app/v1/readings/%d/followups", ownID), map[string]any{"text": "这是另一件全新的事", "continuation": false})
	if fresh["new_reading"] != true {
		t.Fatalf("new reading: %#v", fresh)
	}

	numer := create(t, ts, token, "/api/app/v1/readings", map[string]any{"question": "用数字看看", "type": "normal"})
	numerID := asID(numer["id"])
	must(t, ts, token, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/spread", numerID), map[string]any{"spread_id": spreadID, "draw_mode": "numerology"})
	guide := create(t, ts, token, fmt.Sprintf("/api/app/v1/readings/%d/numerology", numerID), map[string]any{"numbers": []int{}})
	note(messageList(t, guide))
	short := create(t, ts, token, fmt.Sprintf("/api/app/v1/readings/%d/numerology", numerID), map[string]any{"numbers": []int{12}})
	note(messageList(t, short))
	dup := create(t, ts, token, fmt.Sprintf("/api/app/v1/readings/%d/numerology", numerID), map[string]any{"numbers": []int{12, 12, 58}})
	note(messageList(t, dup))
	markedDup := false
	for _, item := range messageList(t, dup) {
		if item["style"] != "pick" {
			continue
		}
		slots, _ := item["slots"].([]any)
		if len(slots) < 2 {
			t.Fatalf("pick slots: %#v", item)
		}
		first := slots[0].(map[string]any)
		second := slots[1].(map[string]any)
		if first["mark"] != "已选择" || first["invalid"] != false || second["mark"] != "重复数字" || second["invalid"] != true {
			t.Fatalf("duplicate marks: %#v %#v", first, second)
		}
		markedDup = true
	}
	if !markedDup {
		t.Fatal("pick did not mark the repeated number")
	}
	guideItem := messageList(t, guide)[0]
	if guideItem["card_count"].(float64) != 3 || guideItem["input_rows"].(float64) != 1 {
		t.Fatalf("guide count: %#v", guideItem)
	}
	rules, _ := guideItem["rules"].([]any)
	if len(rules) != 2 || rules[0] != "按牌位的顺序输入，数字之间用顿号隔开。" {
		t.Fatalf("guide rules: %#v", rules)
	}

	want := []string{"labeled", "bubble", "chip", "option", "followup", "recommend", "spread", "single", "pick", "body", "detect", "guide", "error", "choose", "redraw"}
	for _, style := range want {
		if !seen[style] {
			t.Fatalf("missing type %s in %#v", style, seen)
		}
	}
	if !errorsSeen["请再输入 2 个数字"] || !errorsSeen["数字 12 已被使用，请更换一个数字。"] {
		t.Fatalf("errors: %#v", errorsSeen)
	}
}
