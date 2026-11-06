package tata

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestInterpretStream(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://tata:tata@127.0.0.1:54329/tata?sslmode=disable"
	}
	store, err := Open(t.Context(), dsn, []byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Skipf("no database: %v", err)
	}
	app := &App{Store: store, JWTSecret: []byte("test-secret")}
	ts := httptest.NewServer(app.Handler())
	defer ts.Close()

	// 绑定 demo 供应商（支持 stream:true 的 SSE 输出）到 interpret 任务。
	admin := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "admin", "password": "admin123"})
	vendor := create(t, ts, admin.access, "/api/admin/v1/vendors", map[string]any{
		"name": "流式演示", "base_url": ts.URL + "/demo/openai/v1", "api_key": "k", "models": []string{"demo"}, "enabled": true,
	})
	must(t, ts, admin.access, http.MethodPut, "/api/admin/v1/bindings/interpret",
		map[string]any{"vendor_id": vendor["id"], "model": "demo"})

	token, _ := registerUser(t, ts, store, "stream")
	rd := create(t, ts, token, "/api/app/v1/readings", map[string]any{"question": "我该怎么选？", "type": "normal"})
	id := int64(rd["id"].(float64))
	// 直接推进到 CARDS_FIXED 且额度已结算，专注验证流式生成。
	if _, err := store.Exec(t.Context(),
		`UPDATE readings SET status='CARDS_FIXED', quota_settled=true, cards=$2 WHERE id=$1`,
		id, `[{"position":1,"position_name":"过去","card_id":0,"name":"愚者","orientation":"upright"}]`); err != nil {
		t.Fatal(err)
	}

	req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/app/v1/readings/%d/interpret/stream", ts.URL, id), strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("not SSE: content-type=%s body=%s", ct, raw)
	}
	raw, _ := io.ReadAll(resp.Body)
	body := string(raw)
	deltas := strings.Count(body, "event: delta")
	if deltas < 2 {
		t.Fatalf("expected multiple delta events, got %d:\n%s", deltas, body)
	}
	if !strings.Contains(body, "event: done") {
		t.Fatalf("missing done event:\n%s", body)
	}
	if !strings.Contains(body, "\"status\":\"COMPLETED\"") {
		t.Fatalf("done event not COMPLETED:\n%s", body)
	}

	// 落库校验：解读写入且状态完成。
	var status string
	var interp *string
	if err := store.QueryRow(t.Context(), `SELECT status, interpretation FROM readings WHERE id=$1`, id).Scan(&status, &interp); err != nil {
		t.Fatal(err)
	}
	if status != "COMPLETED" || interp == nil || *interp == "" {
		t.Fatalf("db not completed: status=%s interp=%v", status, interp)
	}
}
