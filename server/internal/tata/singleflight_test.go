package tata

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestRetryDoesNotDoubleChargeOrRedraw(t *testing.T) {
	store, err := Open(t.Context(), "postgres://tata:tata@127.0.0.1:54329/tata?sslmode=disable", []byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	app := &App{Store: store, JWTSecret: []byte("test-secret")}
	ts := httptest.NewServer(app.Handler())
	defer ts.Close()
	admin := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "admin", "password": "admin123"})
	vendor := create(t, ts, admin.access, "/api/admin/v1/vendors", map[string]any{
		"name": "并发演示", "base_url": ts.URL + "/demo/openai/v1", "api_key": "demo", "models": []string{"demo"}, "enabled": true,
	})
	must(t, ts, admin.access, http.MethodPut, "/api/admin/v1/bindings/interpret", map[string]any{"vendor_id": vendor["id"], "model": "demo"})
	token := register(t, ts, fmt.Sprintf("sf%d@tata.local", time.Now().UnixNano()))
	var spreadID float64
	for _, s := range list(t, ts, token, "/api/app/v1/spreads") {
		if s["name"] == "时间流" && s["enabled"] != false {
			spreadID = s["id"].(float64)
		}
	}
	if spreadID == 0 {
		t.Fatal("no enabled time-flow spread")
	}
	reading := create(t, ts, token, "/api/app/v1/readings", map[string]any{"question": "要不要换工作", "type": "normal"})
	rid := int64(reading["id"].(float64))
	must(t, ts, token, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/spread", rid), map[string]any{"spread_id": spreadID})
	drawn := create(t, ts, token, fmt.Sprintf("/api/app/v1/readings/%d/draw", rid), map[string]any{"mode": "normal"})
	cards := fmt.Sprint(drawn["cards"])
	_, _ = store.Exec(t.Context(), `UPDATE readings SET status='FAILED_RETRYABLE' WHERE id=$1`, rid)
	DemoHold.Store(int64(400 * time.Millisecond))
	t.Cleanup(func() { DemoHold.Store(0) })
	var wg sync.WaitGroup
	msgs := make([]string, 2)
	wg.Add(2)
	for i := range msgs {
		go func(i int) {
			defer wg.Done()
			msgs[i], _ = call(t, ts, admin.access, http.MethodPost, fmt.Sprintf("/api/admin/v1/readings/%d/retry", rid), map[string]any{})
		}(i)
	}
	wg.Wait()
	okN := 0
	for _, msg := range msgs {
		if msg == "" {
			okN++
		}
	}
	if okN != 1 {
		t.Fatalf("expected one success, got %v", msgs)
	}
	var n int
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM ledger WHERE reading_id=$1`, rid).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("ledger %d", n)
	}
	after := listOne(t, ts, admin.access, fmt.Sprintf("/api/admin/v1/readings/%d", rid))
	if fmt.Sprint(after["cards"]) != cards {
		t.Fatal("cards redrawn")
	}
	if after["status"] != "COMPLETED" {
		t.Fatalf("status %v msgs %v", after["status"], msgs)
	}
}
