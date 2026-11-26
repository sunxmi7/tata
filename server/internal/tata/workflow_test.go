package tata

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAdminSimulateUsesEinoWithoutWriting(t *testing.T) {
	store, err := Open(t.Context(), "postgres://tata:tata@127.0.0.1:54329/tata?sslmode=disable", []byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	protectLiveAIConfig(t, store)
	app := testApp(store)
	ts := httptest.NewServer(app.Handler())
	defer ts.Close()
	admin := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "admin", "password": "admin123"})
	vendor := create(t, ts, admin.access, "/api/admin/v1/vendors", map[string]any{
		"name": "eino-demo", "base_url": ts.URL + "/demo/openai/v1", "api_key": "demo", "models": []string{"demo"}, "enabled": true,
	})
	for _, task := range []string{"detect", "rewrite", "recommend", "interpret"} {
		must(t, ts, admin.access, http.MethodPut, "/api/admin/v1/bindings/"+task, map[string]any{"vendor_id": vendor["id"], "model": "demo"})
	}
	var before int
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM readings`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	data := create(t, ts, admin.access, "/api/admin/v1/workflow/simulate", map[string]any{"question": "要不要换工作"})
	if data["engine"] != "eino" || data["persisted"] != false {
		t.Fatalf("simulate meta %#v", data)
	}
	raw := mustJSON(data["steps"])
	if !strings.Contains(raw, "演示解读") {
		t.Fatalf("eino output missing: %s", raw)
	}
	var after int
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM readings`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("simulate wrote readings %d -> %d", before, after)
	}
}

func TestAppWorkflowDetectAndRecommendStayLocal(t *testing.T) {
	store, ts := newMergeServer(t)
	admin := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "admin", "password": "admin123"})
	vendor := create(t, ts, admin.access, "/api/admin/v1/vendors", map[string]any{
		"name": "流程检测", "base_url": ts.URL + "/demo/openai/v1", "api_key": "k", "models": []string{"demo"}, "enabled": true,
	})
	for _, task := range []string{"detect", "recommend"} {
		must(t, ts, admin.access, http.MethodPut, "/api/admin/v1/bindings/"+task, map[string]any{"vendor_id": vendor["id"], "model": "demo"})
	}
	token, _ := registerUser(t, ts, store, "workflowlocal")
	rd := create(t, ts, token, "/api/app/v1/readings", map[string]any{"question": "要不要换工作", "type": "normal"})
	id := asID(rd["id"])
	out := create(t, ts, token, fmt.Sprintf("/api/app/v1/readings/%d/workflow", id), map[string]any{
		"steps": []string{"detect", "recommend"},
	})
	raw := mustJSON(out["steps"])
	if !strings.Contains(raw, "model_called") || !strings.Contains(raw, "false") || !strings.Contains(raw, "正式推荐未配置") || !strings.Contains(raw, "单张牌占卜") {
		t.Fatalf("workflow steps: %s", raw)
	}
	var jobs int
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM ai_jobs WHERE reading_id=$1 AND kind IN ('detect','recommend')`, id).Scan(&jobs); err != nil {
		t.Fatal(err)
	}
	if jobs != 0 {
		t.Fatalf("workflow called the model: %d", jobs)
	}
}

func TestEnableVendorAppliesAllBindings(t *testing.T) {
	_, ts := newMergeServer(t)
	admin := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "admin", "password": "admin123"})
	first := create(t, ts, admin.access, "/api/admin/v1/vendors", map[string]any{
		"name": "自动绑定甲", "base_url": ts.URL + "/demo/openai/v1", "api_key": "a", "models": []string{"demo"}, "enabled": true,
	})
	assertAllTasksBound(t, ts, admin.access, asID(first["id"]), "demo")

	second := create(t, ts, admin.access, "/api/admin/v1/vendors", map[string]any{
		"name": "自动绑定乙", "base_url": ts.URL + "/demo/openai/v1", "api_key": "b", "models": []string{"demo"}, "enabled": true,
	})
	assertAllTasksBound(t, ts, admin.access, asID(second["id"]), "demo")

	must(t, ts, admin.access, http.MethodPut, fmt.Sprintf("/api/admin/v1/vendors/%d", asID(first["id"])), map[string]any{
		"name": "自动绑定甲", "base_url": ts.URL + "/demo/openai/v1", "api_key": "a", "models": []string{"demo"}, "enabled": true,
	})
	assertAllTasksBound(t, ts, admin.access, asID(first["id"]), "demo")
}

func assertAllTasksBound(t *testing.T, ts *httptest.Server, token string, vendorID int64, model string) {
	t.Helper()
	for _, item := range list(t, ts, token, "/api/admin/v1/bindings") {
		if asID(item["vendor_id"]) != vendorID || item["model"] != model {
			t.Fatalf("task %v bound to %#v, want vendor=%d model=%s", item["task"], item, vendorID, model)
		}
	}
}
