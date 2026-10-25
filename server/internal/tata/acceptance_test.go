package tata

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestAcceptance(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://tata:tata@127.0.0.1:54329/tata?sslmode=disable"
	}
	store, err := Open(t.Context(), dsn, []byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	app := &App{Store: store, JWTSecret: []byte("test-secret")}
	ts := httptest.NewServer(app.Handler())
	defer ts.Close()

	admin := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "admin", "password": "admin123"})
	ro := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "readonly", "password": "readonly123"})
	codes := get(t, ts, ro.access, "/api/admin/v1/auth/codes")
	if strings.Contains(string(codes), "write") {
		t.Fatal("readonly has write code")
	}
	msg, _ := call(t, ts, ro.access, http.MethodPost, "/api/admin/v1/spreads", map[string]any{"name": "x", "card_count": 1, "positions": []map[string]any{{"index": 1, "name": "a", "meaning": ""}}})
	if !strings.Contains(msg, "只读") {
		t.Fatalf("readonly write: %s", msg)
	}

	_, body := call(t, ts, admin.access, http.MethodPost, "/api/admin/v1/spreads", map[string]any{"name": "x", "card_count": 1, "positions": []map[string]any{{"index": 1, "name": "a", "meaning": ""}}, "price": 9})
	if !strings.Contains(string(body), "请求字段不正确") {
		t.Fatalf("price accepted: %s", body)
	}
	qstatus, _ := call(t, ts, admin.access, http.MethodPut, "/api/admin/v1/quota-params", map[string]any{"daily_free_count": 9})
	if !strings.Contains(qstatus, "不可修改") {
		t.Fatalf("quota editable: %s", qstatus)
	}
	lstatus, _ := call(t, ts, admin.access, http.MethodPost, "/api/admin/v1/ledger", map[string]any{"amount": 1})
	if !strings.Contains(lstatus, "不能") {
		t.Fatalf("ledger writable: %s", lstatus)
	}

	secret := "super-secret-key"
	vendor := create(t, ts, admin.access, "/api/admin/v1/vendors", map[string]any{
		"name": "旧供应商", "base_url": ts.URL + "/demo/openai/v1", "api_key": secret, "models": []string{"demo"}, "enabled": true,
	})
	rawVendors := get(t, ts, admin.access, "/api/admin/v1/vendors")
	if strings.Contains(string(rawVendors), secret) || strings.Contains(string(rawVendors), "api_key\"") {
		t.Fatalf("vendor response leaked key: %s", rawVendors)
	}
	var audits int
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM audit_log WHERE detail LIKE $1`, "%"+secret+"%").Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if audits != 0 {
		t.Fatal("audit stored api key")
	}
	must(t, ts, admin.access, http.MethodPut, "/api/admin/v1/bindings/interpret", map[string]any{"vendor_id": vendor["id"], "model": "demo"})

	email := fmt.Sprintf("u%d@tata.local", time.Now().UnixNano())
	appToken := register(t, ts, email)
	if st, _ := call(t, ts, appToken, http.MethodGet, "/api/admin/v1/spreads", nil); !strings.Contains(st, "未登录") && st != "401" {
		// call returns message; 401 body message
	}
	res, err := http.NewRequest(http.MethodGet, ts.URL+"/api/admin/v1/spreads", nil)
	if err != nil {
		t.Fatal(err)
	}
	res.Header.Set("Authorization", "Bearer "+appToken)
	resp, err := http.DefaultClient.Do(res)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("app token reached admin: %d", resp.StatusCode)
	}
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/app/v1/profile", nil)
	req.Header.Set("Authorization", "Bearer "+admin.access)
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusUnauthorized {
		t.Fatalf("admin token reached app: %d", resp2.StatusCode)
	}

	spreads := list(t, ts, appToken, "/api/app/v1/spreads")
	var spreadID float64
	for _, s := range spreads {
		if s["name"] == "时间流" {
			spreadID = s["id"].(float64)
		}
	}
	if spreadID == 0 {
		t.Fatal("time flow spread missing")
	}
	reading := create(t, ts, appToken, "/api/app/v1/readings", map[string]any{"question": "工作怎么选", "type": "normal"})
	rid := int64(reading["id"].(float64))
	if reading["reward_consumed"] != true || reading["quota_consumed"] == true {
		t.Fatalf("new user should spend registration reward first: %#v", reading)
	}
	must(t, ts, appToken, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/spread", rid), map[string]any{"spread_id": spreadID})
	drawn := create(t, ts, appToken, fmt.Sprintf("/api/app/v1/readings/%d/draw", rid), map[string]any{"mode": "normal"})
	cardsBefore, _ := json.Marshal(drawn["cards"])
	if !strings.Contains(string(cardsBefore), "过去") || !strings.Contains(string(cardsBefore), "正义") && !strings.Contains(string(cardsBefore), "力量") {
		// cards are random; positions must be present
	}
	if !strings.Contains(string(cardsBefore), "过去") || !strings.Contains(string(cardsBefore), "现在") || !strings.Contains(string(cardsBefore), "未来") {
		t.Fatalf("positions: %s", cardsBefore)
	}

	// Unbind and prove no hidden vendor, then restore.
	_, _ = store.Exec(t.Context(), `DELETE FROM bindings WHERE task='interpret'`)
	failMsg, _ := call(t, ts, appToken, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/interpret", rid), map[string]any{})
	if !strings.Contains(failMsg, "未绑定") {
		t.Fatalf("unbound interpret: %s", failMsg)
	}
	must(t, ts, admin.access, http.MethodPut, "/api/admin/v1/bindings/interpret", map[string]any{"vendor_id": vendor["id"], "model": "demo"})
	done := create(t, ts, admin.access, fmt.Sprintf("/api/admin/v1/readings/%d/retry", rid), nil)
	text, _ := done["interpretation"].(string)
	if text == "" || done["vendor_name"] != "旧供应商" {
		t.Fatalf("interpret: %#v", done)
	}
	cardsAfter, _ := json.Marshal(done["cards"])
	if string(cardsAfter) != string(cardsBefore) {
		t.Fatalf("cards changed on retry\n%s\n%s", cardsBefore, cardsAfter)
	}
	var ledgerN int
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM ledger WHERE reading_id=$1`, rid).Scan(&ledgerN); err != nil {
		t.Fatal(err)
	}
	if ledgerN != 1 {
		t.Fatalf("ledger rows %d", ledgerN)
	}

	tar := create(t, ts, admin.access, "/api/admin/v1/tarotists", map[string]any{"name": "演示师", "persona": "温和", "display_copy": "温和", "enabled": true})
	must(t, ts, admin.access, http.MethodPut, fmt.Sprintf("/api/admin/v1/tarotists/%.0f", tar["id"]), map[string]any{"name": "演示师", "persona": "改过的人设", "display_copy": "改过", "enabled": true})
	again := listOne(t, ts, admin.access, fmt.Sprintf("/api/admin/v1/readings/%d", rid))
	if again["interpretation"] != text {
		t.Fatal("persona edit rewrote interpretation")
	}

	vendor2 := create(t, ts, admin.access, "/api/admin/v1/vendors", map[string]any{
		"name": "新供应商", "base_url": ts.URL + "/demo/openai/v1", "api_key": "other", "models": []string{"demo"}, "enabled": true,
	})
	must(t, ts, admin.access, http.MethodPut, "/api/admin/v1/bindings/interpret", map[string]any{"vendor_id": vendor2["id"], "model": "demo"})
	if listOne(t, ts, admin.access, fmt.Sprintf("/api/admin/v1/readings/%d", rid))["interpretation"] != text {
		t.Fatal("rebind rewrote completed text")
	}

	var positions any
	for _, s := range list(t, ts, admin.access, "/api/admin/v1/spreads") {
		if s["name"] == "时间流" {
			positions = s["positions"]
		}
	}
	must(t, ts, admin.access, http.MethodPut, fmt.Sprintf("/api/admin/v1/spreads/%.0f", spreadID), map[string]any{
		"name": "时间流", "description": "停用", "card_count": 3,
		"positions": positions, "enabled": false, "charged": false,
	})
	for _, s := range list(t, ts, appToken, "/api/app/v1/spreads") {
		if s["id"] == spreadID {
			t.Fatal("disabled spread still listed")
		}
	}
	if listOne(t, ts, admin.access, fmt.Sprintf("/api/admin/v1/readings/%d", rid))["cards"] == nil {
		t.Fatal("historical cards lost")
	}

	now := time.Now()
	create(t, ts, admin.access, "/api/admin/v1/banners", map[string]any{"copy": "有效", "start_at": now.Add(-time.Hour).Format(time.RFC3339), "end_at": now.Add(time.Hour).Format(time.RFC3339), "priority": 5, "target": ""})
	create(t, ts, admin.access, "/api/admin/v1/banners", map[string]any{"copy": "过期", "start_at": now.Add(-2 * time.Hour).Format(time.RFC3339), "end_at": now.Add(-time.Minute).Format(time.RFC3339), "priority": 9, "target": ""})
	banners := list(t, ts, appToken, "/api/app/v1/banners")
	seenValid, seenExpired := false, false
	for _, b := range banners {
		if b["copy"] == "有效" {
			seenValid = true
		}
		if b["copy"] == "过期" {
			seenExpired = true
		}
	}
	if !seenValid || seenExpired {
		t.Fatalf("banners %#v", banners)
	}

	firstDaily, _ := call(t, ts, appToken, http.MethodPost, "/api/app/v1/daily", map[string]any{})
	if strings.Contains(firstDaily, "已用完") {
		t.Fatalf("daily consumed before the daily quota: %s", firstDaily)
	}
	dailyMsg, _ := call(t, ts, appToken, http.MethodPost, "/api/app/v1/daily", map[string]any{})
	if !strings.Contains(dailyMsg, "已用完") && !strings.Contains(dailyMsg, "已占用") {
		t.Fatalf("second daily: %s", dailyMsg)
	}
	must(t, ts, appToken, http.MethodDelete, fmt.Sprintf("/api/app/v1/readings/%d", rid), nil)
	dailyMsg, _ = call(t, ts, appToken, http.MethodPost, "/api/app/v1/daily", map[string]any{})
	if !strings.Contains(dailyMsg, "已用完") && !strings.Contains(dailyMsg, "已占用") {
		t.Fatalf("delete refunded quota: %s", dailyMsg)
	}
	must(t, ts, admin.access, http.MethodPut, fmt.Sprintf("/api/admin/v1/spreads/%.0f", spreadID), map[string]any{
		"name": "时间流", "description": "过去 / 现在 / 未来", "card_count": 3,
		"positions": positions, "enabled": true, "charged": false,
	})
}

type session struct{ access string }

func login(t *testing.T, ts *httptest.Server, path string, body map[string]string) session {
	t.Helper()
	data := create(t, ts, "", path, body)
	return session{access: data["accessToken"].(string)}
}

func register(t *testing.T, ts *httptest.Server, email string) string {
	t.Helper()
	data := create(t, ts, "", "/api/app/v1/auth/register", map[string]string{"email": email, "password": "demo1234"})
	return data["accessToken"].(string)
}

func create(t *testing.T, ts *httptest.Server, token, path string, body any) map[string]any {
	t.Helper()
	msg, raw := call(t, ts, token, http.MethodPost, path, body)
	if msg != "" {
		t.Fatalf("POST %s: %s", path, msg)
	}
	var env struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatal(err)
	}
	return env.Data
}

func must(t *testing.T, ts *httptest.Server, token, method, path string, body any) {
	t.Helper()
	if msg, _ := call(t, ts, token, method, path, body); msg != "" {
		t.Fatalf("%s %s: %s", method, path, msg)
	}
}

func list(t *testing.T, ts *httptest.Server, token, path string) []map[string]any {
	t.Helper()
	raw := get(t, ts, token, path)
	var env struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatal(err, string(raw))
	}
	return env.Data
}

func listOne(t *testing.T, ts *httptest.Server, token, path string) map[string]any {
	t.Helper()
	raw := get(t, ts, token, path)
	var env struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatal(err)
	}
	return env.Data
}

func get(t *testing.T, ts *httptest.Server, token, path string) []byte {
	t.Helper()
	_, raw := call(t, ts, token, http.MethodGet, path, nil)
	return raw
}

func call(t *testing.T, ts *httptest.Server, token, method, path string, body any) (string, []byte) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, ts.URL+path, rdr)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode == http.StatusUnauthorized {
		return "401", raw
	}
	var env struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(raw, &env)
	if env.Code != 0 {
		return env.Message, raw
	}
	return "", raw
}
