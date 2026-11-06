package tata

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestYearlyFlowTwelvePlusSix(t *testing.T) {
	store, ts := newMergeServer(t)
	token, uid := registerUser(t, ts, store, "yearflow")
	locked := listOne(t, ts, token, "/api/app/v1/yearly-reports/2026")
	if locked["step"] != "locked" || locked["unlocked"] != false {
		t.Fatalf("locked: %#v", locked)
	}
	msg, _ := call(t, ts, token, http.MethodPut, "/api/app/v1/yearly-reports/2026/profile", map[string]any{
		"birth_date": "1991-05-20", "gender": "女性",
	})
	if !strings.Contains(msg, "请先解锁") {
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

	profiled := putOne(t, ts, token, "/api/app/v1/yearly-reports/2026/profile", map[string]any{
		"birth_date": "1991-05-20", "gender": "女性",
	})
	if profiled["step"] != "rhythm" {
		t.Fatalf("after profile: %#v", profiled)
	}
	prof, _ := profiled["profile"].(map[string]any)
	if prof["birth_date"] != "1991-05-20" || prof["completed"] != true {
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
	if len(rCards) != 12 || rCards[0].PositionName != "一月" || rCards[0].Name != "愚者" || rCards[11].PositionName != "十二月" {
		t.Fatalf("rhythm cards: %#v", rCards)
	}
	msg, _ = call(t, ts, token, http.MethodPost, "/api/app/v1/yearly-reports/2026/rhythm", map[string]any{})
	if !strings.Contains(msg, "已固定") {
		t.Fatalf("redraw rhythm: %s", msg)
	}

	msg, _ = call(t, ts, token, http.MethodPost, "/api/app/v1/yearly-reports/2026/domains", map[string]any{
		"mode": "pick", "numbers": []int{1, 13, 14, 15, 16, 17},
	})
	if !strings.Contains(msg, "已经用过") {
		t.Fatalf("overlap: %s", msg)
	}
	domainNums := []int{13, 14, 15, 16, 17, 18}
	domains := create(t, ts, token, "/api/app/v1/yearly-reports/2026/domains", map[string]any{
		"mode": "pick", "numbers": domainNums,
	})
	if domains["step"] != "generate" {
		t.Fatalf("after domains: %#v", domains)
	}
	dCards := asYearlyCards(t, domains["domain_cards"])
	if len(dCards) != 6 || dCards[0].PositionName != "感情" || dCards[5].PositionName != "成长" {
		t.Fatalf("domain cards: %#v", dCards)
	}
	if dCards[0].CardID == rCards[0].CardID {
		t.Fatalf("overlap with rhythm: %#v %#v", dCards[0], rCards[0])
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
	putOne(t, ts, token, "/api/app/v1/yearly-reports/2026/profile", map[string]any{"birth_date": "1988-03-08"})
	drawn := create(t, ts, token, "/api/app/v1/yearly-reports/2026/rhythm", map[string]any{})
	cards := asYearlyCards(t, drawn["rhythm_cards"])
	if len(cards) != 12 {
		t.Fatalf("random 12: %#v", cards)
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
	if len(dCards) != 6 {
		t.Fatalf("random 6: %#v", dCards)
	}
	for _, card := range dCards {
		if seen[card.CardID] {
			t.Fatalf("domain reused rhythm card: %#v", card)
		}
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
