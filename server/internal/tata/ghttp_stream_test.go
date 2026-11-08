package tata

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

// TestInterpretStreamViaGoFrame 证明 SSE 经真实 ghttp 服务器也能逐段下发（非 httptest）。
// setup 走共享同一 App/Store 的 httptest，仅 SSE 调用打到 ghttp 端口。
func TestInterpretStreamViaGoFrame(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://tata:tata@127.0.0.1:54329/tata?sslmode=disable"
	}
	store, err := Open(t.Context(), dsn, []byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Skipf("no database: %v", err)
	}
	protectLiveAIConfig(t, store)
	app := testApp(store)

	ts := httptest.NewServer(app.Handler())
	defer ts.Close()

	s := g.Server("ssetest-" + fmt.Sprint(time.Now().UnixNano()))
	s.SetAddr("127.0.0.1:0")
	s.SetDumpRouterMap(false)
	s.SetLogStdout(false)
	s.SetAccessLogEnabled(false)
	s.SetErrorLogEnabled(false)
	app.Bind(s)
	if err := s.Start(); err != nil {
		t.Skipf("ghttp start: %v", err)
	}
	defer func() { _ = s.Shutdown() }()
	var port int
	for i := 0; i < 100; i++ {
		if port = s.GetListenedPort(); port > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if port <= 0 {
		t.Skip("ghttp no listened port")
	}
	base := fmt.Sprintf("http://127.0.0.1:%d", port)

	// 绑定 demo 供应商（指向 httptest 的 /demo，支持 stream）。
	admin := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "admin", "password": "admin123"})
	vendor := create(t, ts, admin.access, "/api/admin/v1/vendors", map[string]any{
		"name": "ghttp流式", "base_url": ts.URL + "/demo/openai/v1", "api_key": "k", "models": []string{"demo"}, "enabled": true,
	})
	must(t, ts, admin.access, http.MethodPut, "/api/admin/v1/bindings/interpret",
		map[string]any{"vendor_id": vendor["id"], "model": "demo"})

	token, _ := registerUser(t, ts, store, "ghttpstream")
	rd := create(t, ts, token, "/api/app/v1/readings", map[string]any{"question": "经 ghttp 的流式？", "type": "normal"})
	id := int64(rd["id"].(float64))
	if _, err := store.Exec(t.Context(),
		`UPDATE readings SET status='CARDS_FIXED', quota_settled=true, cards=$2 WHERE id=$1`,
		id, `[{"position":1,"position_name":"过去","card_id":0,"name":"愚者","orientation":"upright"}]`); err != nil {
		t.Fatal(err)
	}

	// 经 ghttp 端口发起 SSE
	req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/app/v1/readings/%d/interpret/stream", base, id), strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("ghttp not SSE: content-type=%s body=%s", ct, raw)
	}
	raw, _ := io.ReadAll(resp.Body)
	body := string(raw)
	if strings.Count(body, "event: delta") < 2 || !strings.Contains(body, "event: done") || !strings.Contains(body, "\"status\":\"COMPLETED\"") {
		t.Fatalf("ghttp SSE body unexpected:\n%s", body)
	}
}
