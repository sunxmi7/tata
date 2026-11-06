package tata

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestErrorCodes(t *testing.T) {
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

	token, _ := registerUser(t, ts, store, "errcode")

	// 读取 error_code 的辅助。
	errCode := func(method, path, token, body string) (int, string) {
		var rdr io.Reader
		if body != "" {
			rdr = strings.NewReader(body)
		}
		req, _ := http.NewRequest(method, ts.URL+path, rdr)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		var env struct {
			Code      int    `json:"code"`
			ErrorCode string `json:"error_code"`
		}
		_ = json.Unmarshal(raw, &env)
		return resp.StatusCode, env.ErrorCode
	}

	// 未登录 → unauthorized / 401
	if st, code := errCode(http.MethodGet, "/api/app/v1/profile", "", ""); st != 401 || code != "unauthorized" {
		t.Fatalf("unauth: status=%d code=%s", st, code)
	}
	// 非法路径 id → invalid_path_id
	if _, code := errCode(http.MethodGet, "/api/app/v1/readings/abc", token, ""); code != "invalid_path_id" {
		t.Fatalf("path id code=%s", code)
	}
	// 非法请求体（未知字段）→ invalid_request_body
	if _, code := errCode(http.MethodPost, "/api/app/v1/feedback", token, `{"nope":1}`); code != "invalid_request_body" {
		t.Fatalf("body code=%s", code)
	}
	// 记录不存在 → reading_not_found
	if _, code := errCode(http.MethodGet, "/api/app/v1/readings/999999999", token, ""); code != "reading_not_found" {
		t.Fatalf("not found code=%s", code)
	}
}
