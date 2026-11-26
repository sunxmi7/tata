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
	protectLiveAIConfig(t, store)
	app := testApp(store)
	ts := httptest.NewServer(app.Handler())
	defer ts.Close()

	// 绑定 demo 供应商（支持 stream:true 的 SSE 输出）到 interpret 任务。
	admin := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "admin", "password": "admin123"})
	vendor := create(t, ts, admin.access, "/api/admin/v1/vendors", map[string]any{
		"name": "流式演示", "base_url": ts.URL + "/demo/openai/v1", "api_key": "k", "models": []string{"demo"}, "enabled": true,
	})
	must(t, ts, admin.access, http.MethodPut, "/api/admin/v1/bindings/interpret",
		map[string]any{"vendor_id": vendor["id"], "model": "demo"})

	token, uid := registerUser(t, ts, store, "stream")
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
	var task, kind, vendorName, modelName string
	var prompt, completion, total, cached, reasoning int
	var gotUser int64
	var ok bool
	if err := store.QueryRow(t.Context(), `SELECT task, kind, vendor_name, model_name, prompt_tokens, completion_tokens, total_tokens, cached_tokens, reasoning_tokens, user_id, ok
		FROM ai_usages WHERE reading_id=$1 ORDER BY id DESC LIMIT 1`, id).
		Scan(&task, &kind, &vendorName, &modelName, &prompt, &completion, &total, &cached, &reasoning, &gotUser, &ok); err != nil {
		t.Fatal(err)
	}
	if task != "interpret" || kind != "interpret" || vendorName != "流式演示" || modelName != "demo" || !ok || gotUser != uid {
		t.Fatalf("usage row: task=%s kind=%s vendor=%s model=%s ok=%v user=%d", task, kind, vendorName, modelName, ok, gotUser)
	}
	if prompt < 1 || completion < 1 || total != prompt+completion || cached != prompt/5 || reasoning != 0 {
		t.Fatalf("vendor tokens: prompt=%d completion=%d total=%d cached=%d reasoning=%d", prompt, completion, total, cached, reasoning)
	}
	if _, err := store.Exec(t.Context(), `INSERT INTO ai_usages(reading_id, user_id, task, kind, vendor_name, model_name, prompt_tokens, completion_tokens, total_tokens)
		VALUES ($1,$2,'tarotist','followup','流式演示','demo',10,4,14)`, id, uid); err != nil {
		t.Fatal(err)
	}
	found := map[string]any{}
	for _, row := range list(t, ts, admin.access, "/api/admin/v1/readings") {
		if asID(row["id"]) == id {
			found = row
			break
		}
	}
	if asID(found["prompt_tokens"]) != int64(prompt+10) || asID(found["completion_tokens"]) != int64(completion+4) || asID(found["total_tokens"]) != int64(total+14) {
		t.Fatalf("admin list tokens: %#v", found)
	}
	one := listOne(t, ts, admin.access, fmt.Sprintf("/api/admin/v1/readings/%d", id))
	if asID(one["prompt_tokens"]) != int64(prompt+10) || asID(one["total_tokens"]) != int64(total+14) {
		t.Fatalf("admin detail tokens: %#v", one)
	}
	usage := listOne(t, ts, admin.access, fmt.Sprintf("/api/admin/v1/readings/%d/usages", id))
	if asID(usage["total_tokens"]) != int64(total+14) || usage["price_minor"] != nil {
		t.Fatalf("usage summary: %#v", usage)
	}
	steps, _ := usage["steps"].([]any)
	if len(steps) != 2 {
		t.Fatalf("steps: %#v", usage["steps"])
	}
	labels := map[string]bool{}
	for _, raw := range steps {
		step := raw.(map[string]any)
		labels[step["kind_label"].(string)] = true
		if step["price_minor"] != nil {
			t.Fatalf("step price invented: %#v", step)
		}
	}
	if !labels["解读"] || !labels["追问"] {
		t.Fatalf("step labels: %#v", usage["steps"])
	}
}

func TestDailyStream(t *testing.T) {
	store, ts := newMergeServer(t)
	admin := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "admin", "password": "admin123"})
	vendor := create(t, ts, admin.access, "/api/admin/v1/vendors", map[string]any{
		"name": "日运流式", "base_url": ts.URL + "/demo/openai/v1", "api_key": "k", "models": []string{"demo"}, "enabled": true,
	})
	must(t, ts, admin.access, http.MethodPut, "/api/admin/v1/bindings/interpret",
		map[string]any{"vendor_id": vendor["id"], "model": "demo"})
	token, uid := registerUser(t, ts, store, "dailystream")
	seedGift(t, store, uid, 1)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/app/v1/daily/stream", strings.NewReader(`{"target":"today"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
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
	if strings.Count(body, "event: delta") < 1 || !strings.Contains(body, "event: done") {
		t.Fatalf("daily stream:\n%s", body)
	}
}

func TestInterpretJobSurvivesClientDrop(t *testing.T) {
	store, ts := newMergeServer(t)
	admin := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "admin", "password": "admin123"})
	vendor := create(t, ts, admin.access, "/api/admin/v1/vendors", map[string]any{
		"name": "断线任务", "base_url": ts.URL + "/demo/openai/v1", "api_key": "k", "models": []string{"demo"}, "enabled": true,
	})
	must(t, ts, admin.access, http.MethodPut, "/api/admin/v1/bindings/interpret",
		map[string]any{"vendor_id": vendor["id"], "model": "demo"})
	token, _ := registerUser(t, ts, store, "jobdrop")
	rd := create(t, ts, token, "/api/app/v1/readings", map[string]any{"question": "断线后还能拿到结果吗", "type": "normal"})
	id := int64(rd["id"].(float64))
	if _, err := store.Exec(t.Context(),
		`UPDATE readings SET status='CARDS_FIXED', quota_settled=true, cards=$2 WHERE id=$1`,
		id, `[{"position":1,"position_name":"过去","card_id":0,"name":"愚者","orientation":"upright"}]`); err != nil {
		t.Fatal(err)
	}
	DemoHold.Store(int64(400 * time.Millisecond))
	t.Cleanup(func() { DemoHold.Store(0) })

	req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/app/v1/readings/%d/interpret/stream", ts.URL, id), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	client := &http.Client{Timeout: 80 * time.Millisecond}
	_, _ = client.Do(req)

	deadline := time.Now().Add(3 * time.Second)
	var status string
	var interp *string
	for time.Now().Before(deadline) {
		if err := store.QueryRow(t.Context(), `SELECT status, interpretation FROM readings WHERE id=$1`, id).Scan(&status, &interp); err != nil {
			t.Fatal(err)
		}
		if status == "COMPLETED" && interp != nil && *interp != "" {
			break
		}
		time.Sleep(40 * time.Millisecond)
	}
	if status != "COMPLETED" || interp == nil || *interp == "" {
		t.Fatalf("job did not persist after client drop: status=%s interp=%v", status, interp)
	}

	var jobs int
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM ai_jobs WHERE reading_id=$1 AND kind='interpret' AND status='succeeded'`, id).Scan(&jobs); err != nil {
		t.Fatal(err)
	}
	if jobs < 1 {
		t.Fatalf("ai_jobs not stored, count=%d", jobs)
	}

	got := listOne(t, ts, token, fmt.Sprintf("/api/app/v1/readings/%d", id))
	if got["status"] != "COMPLETED" {
		t.Fatalf("come back GET: %#v", got)
	}
}
