package tata

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestYearlyFlowTwelvePlusFour(t *testing.T) {
	store, ts := newMergeServer(t)
	token, uid := registerUser(t, ts, store, "yearflow")
	locked := listOne(t, ts, token, "/api/app/v1/yearly-reports/2026")
	if locked["step"] != "locked" || locked["unlocked"] != false {
		t.Fatalf("locked: %#v", locked)
	}
	msg, _ := call(t, ts, token, http.MethodPut, "/api/app/v1/yearly-reports/2026/profile", map[string]any{
		"birth_date": "1991-05-20", "birth_time": "08:30", "birth_place": "上海", "gender": "女性",
	})
	if !strings.Contains(msg, "内购") && !strings.Contains(msg, "请先解锁") {
		t.Fatalf("profile before unlock: %s", msg)
	}

	if _, err := store.Exec(t.Context(), `INSERT INTO subscriptions(user_id, sku_kind, status, starts_at, expires_at)
		VALUES ($1,'year','active',now(), now() + interval '365 days')`, uid); err != nil {
		t.Fatal(err)
	}
	open := listOne(t, ts, token, "/api/app/v1/yearly-reports/2026")
	if open["step"] != "profile" || open["unlocked"] != true {
		t.Fatalf("after year card: %#v", open)
	}

	msg, _ = call(t, ts, token, http.MethodPost, "/api/app/v1/yearly-reports/2026/rhythm", map[string]any{})
	if !strings.Contains(msg, "基本信息") {
		t.Fatalf("rhythm before profile: %s", msg)
	}
	msg, _ = call(t, ts, token, http.MethodPut, "/api/app/v1/yearly-reports/2026/profile", map[string]any{
		"birth_date": "2099-01-01",
	})
	if !strings.Contains(msg, "未来") {
		t.Fatalf("future birth: %s", msg)
	}
	msg, _ = call(t, ts, token, http.MethodPut, "/api/app/v1/yearly-reports/2026/profile", map[string]any{
		"birth_date": "1991-05-20", "gender": "女性",
	})
	if !strings.Contains(msg, "出生时间") {
		t.Fatalf("missing birth time: %s", msg)
	}
	msg, _ = call(t, ts, token, http.MethodPut, "/api/app/v1/yearly-reports/2026/profile", map[string]any{
		"birth_date": "1991-05-20", "birth_time": "08:30",
	})
	if !strings.Contains(msg, "出生地点") {
		t.Fatalf("missing birth place: %s", msg)
	}

	profiled := putOne(t, ts, token, "/api/app/v1/yearly-reports/2026/profile", map[string]any{
		"birth_date": "1991-05-20", "birth_time": "08:30", "birth_place": "上海", "gender": "女性",
	})
	if profiled["step"] != "rhythm" {
		t.Fatalf("after profile: %#v", profiled)
	}
	prof, _ := profiled["profile"].(map[string]any)
	if prof["birth_date"] != "1991-05-20" || prof["birth_time"] != "08:30" || prof["birth_place"] != "上海" || prof["completed"] != true {
		t.Fatalf("profile: %#v", prof)
	}

	nums := make([]int, 12)
	for i := range nums {
		nums[i] = i + 1
	}
	rhythm := create(t, ts, token, "/api/app/v1/yearly-reports/2026/rhythm", map[string]any{
		"mode": "pick", "numbers": nums,
	})
	if rhythm["step"] != "domains" {
		t.Fatalf("after rhythm: %#v", rhythm)
	}
	rCards := asYearlyCards(t, rhythm["rhythm_cards"])
	hCards := asYearlyCards(t, rhythm["house_cards"])
	if len(rCards) != 12 || rCards[0].PositionName != "自我" || rCards[0].Name != "愚者" || rCards[11].PositionName != "潜意识" {
		t.Fatalf("house cards: %#v", rCards)
	}
	if len(hCards) != 12 || hCards[0].PositionName != "自我" {
		t.Fatalf("house_cards alias: %#v", hCards)
	}
	msg, _ = call(t, ts, token, http.MethodPost, "/api/app/v1/yearly-reports/2026/rhythm", map[string]any{})
	if !strings.Contains(msg, "已固定") {
		t.Fatalf("redraw rhythm: %s", msg)
	}

	msg, _ = call(t, ts, token, http.MethodPost, "/api/app/v1/yearly-reports/2026/domains", map[string]any{
		"mode": "pick", "numbers": []int{1, 13, 14, 15},
	})
	if !strings.Contains(msg, "已经用过") {
		t.Fatalf("overlap: %s", msg)
	}
	domainNums := []int{13, 14, 15, 16}
	domains := create(t, ts, token, "/api/app/v1/yearly-reports/2026/domains", map[string]any{
		"mode": "pick", "numbers": domainNums,
	})
	if domains["step"] != "generate" {
		t.Fatalf("after domains: %#v", domains)
	}
	dCards := asYearlyCards(t, domains["domain_cards"])
	sCards := asYearlyCards(t, domains["season_cards"])
	if len(dCards) != 4 || dCards[0].PositionName != "春" || dCards[3].PositionName != "冬" {
		t.Fatalf("season cards: %#v", dCards)
	}
	if len(sCards) != 4 || sCards[0].PositionName != "春" {
		t.Fatalf("season_cards alias: %#v", sCards)
	}
	if dCards[0].CardID == rCards[0].CardID {
		t.Fatalf("overlap with rhythm: %#v %#v", dCards[0], rCards[0])
	}

	resetDiamondCost(t, store)
	setDiamondCost(t, store, 1)
	setQuota(t, store, uid, 10)
	if _, err := store.Exec(t.Context(), `DELETE FROM bindings WHERE task='interpret'`); err != nil {
		t.Fatal(err)
	}
	msg, _ = call(t, ts, token, http.MethodPost, "/api/app/v1/yearly-reports/2026/generate", map[string]any{})
	if !strings.Contains(msg, "未绑定") && !strings.Contains(msg, "解读失败") {
		t.Fatalf("generate without vendor: %s", msg)
	}

	admin := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "admin", "password": "admin123"})
	vendor := create(t, ts, admin.access, "/api/admin/v1/vendors", map[string]any{
		"name": "年运演示", "base_url": ts.URL + "/demo/openai/v1", "api_key": "k", "models": []string{"demo"}, "enabled": true,
	})
	must(t, ts, admin.access, http.MethodPut, "/api/admin/v1/bindings/interpret",
		map[string]any{"vendor_id": vendor["id"], "model": "demo"})

	ready := create(t, ts, token, "/api/app/v1/yearly-reports/2026/generate", map[string]any{})
	if ready["step"] != "ready" || ready["report_status"] != "ready" {
		t.Fatalf("generated: %#v", ready)
	}
	if fmt.Sprint(ready["report"]) == "" || ready["report"] == nil {
		t.Fatalf("empty report: %#v", ready)
	}
	again := create(t, ts, token, "/api/app/v1/yearly-reports/2026/generate", map[string]any{})
	if again["step"] != "ready" {
		t.Fatalf("idempotent generate: %#v", again)
	}
}

func TestYearlyRhythmRandomDraw(t *testing.T) {
	store, ts := newMergeServer(t)
	token, uid := registerUser(t, ts, store, "yearrand")
	if _, err := store.Exec(t.Context(), `INSERT INTO subscriptions(user_id, sku_kind, status, starts_at, expires_at)
		VALUES ($1,'year','active',now(), now() + interval '365 days')`, uid); err != nil {
		t.Fatal(err)
	}
	putOne(t, ts, token, "/api/app/v1/yearly-reports/2026/profile", map[string]any{
		"birth_date": "1988-03-08", "birth_time": "14:05", "birth_place": "杭州",
	})
	drawn := create(t, ts, token, "/api/app/v1/yearly-reports/2026/rhythm", map[string]any{})
	cards := asYearlyCards(t, drawn["rhythm_cards"])
	if len(cards) != 12 {
		t.Fatalf("random 12: %#v", cards)
	}
	if cards[0].PositionName != "自我" || cards[11].PositionName != "潜意识" {
		t.Fatalf("house names: %#v", cards)
	}
	seen := map[int]bool{}
	for _, card := range cards {
		if seen[card.CardID] {
			t.Fatalf("dup card: %#v", cards)
		}
		seen[card.CardID] = true
	}
	second := create(t, ts, token, "/api/app/v1/yearly-reports/2026/domains", map[string]any{})
	dCards := asYearlyCards(t, second["domain_cards"])
	if len(dCards) != 4 {
		t.Fatalf("random 4: %#v", dCards)
	}
	if dCards[0].PositionName != "春" || dCards[3].PositionName != "冬" {
		t.Fatalf("season names: %#v", dCards)
	}
	for _, card := range dCards {
		if seen[card.CardID] {
			t.Fatalf("domain reused rhythm card: %#v", card)
		}
	}
}

func TestYearlyStandalonePurchaseCompletes(t *testing.T) {
	store, ts := newMergeServer(t)
	token, uid := registerUser(t, ts, store, "yearbuy")
	if _, err := store.Exec(t.Context(), `INSERT INTO yearly_reports(user_id, year, unlocked, unlock_source, unlocked_at)
		VALUES ($1, 2026, TRUE, 'purchase', now())`, uid); err != nil {
		t.Fatal(err)
	}
	open := listOne(t, ts, token, "/api/app/v1/yearly-reports/2026")
	if open["unlocked"] != true || open["unlock_source"] != "purchase" || open["standalone_purchase"] != true {
		t.Fatalf("purchased: %#v", open)
	}
	putOne(t, ts, token, "/api/app/v1/yearly-reports/2026/profile", map[string]any{
		"birth_date": "1992-02-02", "birth_time": "09:15", "birth_place": "南京", "gender": "女性",
	})
	create(t, ts, token, "/api/app/v1/yearly-reports/2026/rhythm", map[string]any{})
	create(t, ts, token, "/api/app/v1/yearly-reports/2026/domains", map[string]any{})
	if _, err := store.Exec(t.Context(), `DELETE FROM bindings WHERE task='interpret'`); err != nil {
		t.Fatal(err)
	}
	msg, _ := call(t, ts, token, http.MethodPost, "/api/app/v1/yearly-reports/2026/generate", map[string]any{})
	if strings.Contains(msg, "开通会员") || strings.Contains(msg, "内购") {
		t.Fatalf("purchase still blocked: %s", msg)
	}
	if !strings.Contains(msg, "未绑定") && !strings.Contains(msg, "解读失败") {
		t.Fatalf("generate: %s", msg)
	}
	if quotaOf(t, store, uid) != 0 {
		t.Fatal("standalone generate spent quota")
	}

	memberToken, memberID := registerUser(t, ts, store, "yearmember")
	setMember(t, store, memberID, "month")
	seedGift(t, store, memberID, 1)
	resetDiamondCost(t, store)
	setDiamondCost(t, store, 3)
	hidden := list(t, ts, memberToken, "/api/app/v1/iap/skus?kind=yearly_unlock")
	if len(hidden) != 0 {
		t.Fatalf("member yearly sku: %#v", hidden)
	}
	view := listOne(t, ts, memberToken, "/api/app/v1/yearly-reports/2026")
	if view["unlocked"] != true || view["standalone_purchase"] != false {
		t.Fatalf("member yearly: %#v", view)
	}
	putOne(t, ts, memberToken, "/api/app/v1/yearly-reports/2026/profile", map[string]any{
		"birth_date": "1990-01-01", "birth_time": "10:00", "birth_place": "北京",
	})
	create(t, ts, memberToken, "/api/app/v1/yearly-reports/2026/rhythm", map[string]any{})
	create(t, ts, memberToken, "/api/app/v1/yearly-reports/2026/domains", map[string]any{})
	msg, _ = call(t, ts, memberToken, http.MethodPost, "/api/app/v1/yearly-reports/2026/generate", map[string]any{})
	if !strings.Contains(msg, "额度不足") {
		t.Fatalf("empty pool: %s", msg)
	}
	var left int
	if err := store.QueryRow(t.Context(), `SELECT remaining FROM registration_gifts WHERE user_id=$1`, memberID).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 1 {
		t.Fatalf("yearly used the gift %d", left)
	}
}

func putOne(t *testing.T, ts *httptest.Server, token, path string, body any) map[string]any {
	t.Helper()
	msg, raw := call(t, ts, token, http.MethodPut, path, body)
	if msg != "" {
		t.Fatalf("PUT %s: %s", path, msg)
	}
	var env struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatal(err)
	}
	return env.Data
}

func asYearlyCards(t *testing.T, raw any) []drawnCard {
	t.Helper()
	b, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	var cards []drawnCard
	if err := json.Unmarshal(b, &cards); err != nil {
		t.Fatal(err, string(b))
	}
	return cards
}
