package readingflow

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRunTaskKeepsVendorUsage(t *testing.T) {
	usage := map[string]any{
		"prompt_tokens":             20,
		"completion_tokens":         7,
		"total_tokens":              27,
		"prompt_tokens_details":     map[string]any{"cached_tokens": 4},
		"completion_tokens_details": map[string]any{"reasoning_tokens": 2},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Stream bool `json:"stream"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprintf(w, "data: %s\n\n", `{"choices":[{"index":0,"delta":{"content":"你好"}}]}`)
			raw, _ := json.Marshal(map[string]any{"choices": []any{}, "usage": usage})
			fmt.Fprintf(w, "data: %s\n\n", raw)
			fmt.Fprint(w, "data: [DONE]\n\n")
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"index": 0, "message": map[string]string{"role": "assistant", "content": "你好"}}},
			"usage":   usage,
		})
	}))
	defer srv.Close()

	spec := ModelSpec{
		Vendor:  "演示",
		Model:   "demo",
		BaseURL: srv.URL + "/v1",
		APIKey:  "test",
		Timeout: 5 * time.Second,
	}
	var got UsageEvent
	ctx := WithUsageHook(t.Context(), func(ev UsageEvent) { got = ev })
	text, err := RunTask(ctx, spec, "interpret", map[string]any{"question": "怎么选"})
	if err != nil {
		t.Fatal(err)
	}
	if text != "你好" {
		t.Fatalf("text: %q", text)
	}
	assertUsage(t, got)

	got = UsageEvent{}
	text, err = RunTaskStream(ctx, spec, "interpret", map[string]any{"question": "怎么选"}, func(string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if text != "你好" {
		t.Fatalf("stream text: %q", text)
	}
	assertUsage(t, got)
}

func assertUsage(t *testing.T, got UsageEvent) {
	t.Helper()
	if got.Err != nil || got.Task != "interpret" || got.Vendor != "演示" || got.Model != "demo" {
		t.Fatalf("event: %+v", got)
	}
	u := got.Usage
	if u.PromptTokens != 20 || u.CompletionTokens != 7 || u.TotalTokens != 27 || u.CachedTokens != 4 || u.ReasoningTokens != 2 {
		t.Fatalf("usage: %+v", u)
	}
}
